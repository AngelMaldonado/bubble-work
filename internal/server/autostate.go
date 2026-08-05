package server

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// autoWriteLimit bounds how many cards one tick may move. A calibration change
// can reclassify a whole board at once; without a cap the first sweep after it
// would rewrite hundreds of work items in one burst.
const autoWriteLimit = 50

// autoTarget maps a derived level to the Plane state GROUP a thread belongs in.
// "" means leave the card alone: 🏆 is Plane's own business, and anything we
// can't classify is not our business either.
func autoTarget(level string) string {
	switch level {
	case "in_progress":
		return "started"
	case "zzzz":
		return "backlog"
	case "rip":
		return "cancelled"
	default:
		return "" // done / reviewed / unknown → hands off
	}
}

// autoState reflects derived thread levels back onto Plane for the instances
// that opted in (THREAD-LIFECYCLE.md Phase B).
//
// The whole feature is governed by three rules:
//
//   - **Opt-in per instance, off by default.** An instance with auto_state = 0 is
//     never written to. Bubble is a lens until an operator says otherwise.
//   - **Resolve by group, never by name.** State names are project-configured and
//     localized ("En Progreso", "Cancelado"), so we pick the target group's
//     default state per project.
//   - **Never fight a human.** We remember the state we wrote. If a card's
//     current state is not what we left it as, someone else moved it — we hand
//     that thread off permanently and never touch it again. A thread we have
//     never written to is fair game exactly once.
//
// Auto-writes are housekeeping, not evidence: they use the instance key, are
// logged, notify the inbox, and never warm anything.
func (s *Server) autoState(ctx context.Context, bubbles []domain.Bubble) int {
	byInstance := map[string][]domain.Bubble{}
	for _, b := range bubbles {
		if b.Closed {
			continue // a closed bubble's threads are nobody's business
		}
		byInstance[b.Instance] = append(byInstance[b.Instance], b)
	}

	written := 0
	slugs := make([]string, 0, len(byInstance))
	for slug := range byInstance {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		inst, ok, err := s.instanceBySlug(slug)
		if err != nil || !ok || !inst.AutoState {
			continue // not opted in
		}
		for _, b := range byInstance[slug] {
			if written >= autoWriteLimit {
				log.Printf("autostate: hit the %d-write cap; the rest wait for the next tick", autoWriteLimit)
				return written
			}
			written += s.autoStateBubble(ctx, inst, b, autoWriteLimit-written)
		}
	}
	return written
}

// autoStateBubble moves the cards of one bubble, returning how many it wrote.
func (s *Server) autoStateBubble(ctx context.Context, inst domain.Instance, b domain.Bubble, budget int) int {
	cl := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, b.Project)
	// Target states come from the mirror (Phase 3). This used to be a cached
	// Plane call with a 30-minute TTL, which meant a newly added workflow state
	// could be invisible for half an hour while we wrote cards to the wrong one.
	states, err := s.mirrorStates(inst.Slug, b.Project)
	if err != nil || len(states) == 0 {
		return 0 // can't resolve targets; do nothing rather than guess
	}

	buoy := s.threadBuoyancy(b)
	ids := make([]string, 0, len(b.Threads))
	for _, t := range b.Threads {
		ids = append(ids, t.ID)
	}
	prov, err := s.store.AutoStateFor(ids)
	if err != nil {
		log.Printf("autostate: provenance load for %s: %v", b.ID, err)
		return 0
	}

	moved := map[string]int{} // target group → count, for the notification
	n := 0
	for _, t := range b.Threads {
		if n >= budget {
			break
		}
		group := autoTarget(buoy[t.ID].Level)
		if group == "" || t.StateGroup == group {
			continue // hands off, or already where it belongs
		}

		// Provenance: only act on threads we've never touched, or whose state is
		// still exactly what we left it as.
		if p, seen := prov[t.ID]; seen {
			if p.HandedOff {
				continue
			}
			if p.StateID != stateIDOf(t, states) {
				// Someone moved it since our last write. Back off for good.
				if err := s.store.HandOffAutoState(t.ID); err != nil {
					log.Printf("autostate: hand-off %s: %v", t.ID, err)
				}
				log.Printf("autostate: %s was moved by hand; no longer auto-managed", t.ID)
				continue
			}
		}

		target, ok := defaultStateOf(states, group)
		if !ok {
			continue // this project has no state in that group
		}
		if err := cl.SetWorkItemState(ctx, t.ID, target.ID); err != nil {
			log.Printf("autostate: move %s → %s: %v", t.ID, group, err)
			continue
		}
		if err := s.store.RecordAutoState(t.ID, target.ID, s.now()); err != nil {
			log.Printf("autostate: record %s: %v", t.ID, err)
		}
		log.Printf("autostate: %s (%s) %q → %q [%s]", t.ID, b.Instance, t.State, target.Name, group)
		moved[group]++
		n++
	}

	if n > 0 {
		s.notifyAutoState(b, moved)
		s.dropInstanceCache(b.Instance) // the snapshot's state names are now stale
	}
	return n
}

// stateIDOf recovers a thread's current state id from its (localized) name, via
// the project's state table. The snapshot carries the name and group, not the id.
func stateIDOf(t domain.Thread, states map[string]plane.State) string {
	for id, st := range states {
		if st.Name == t.State && st.Group == t.StateGroup {
			return id
		}
	}
	return ""
}

// defaultStateOf picks the state to move a card into for a group: the group's
// default when it has one, else its first by name so the choice is stable.
func defaultStateOf(states map[string]plane.State, group string) (plane.State, bool) {
	var pick plane.State
	found := false
	for _, st := range states {
		if st.Group != group {
			continue
		}
		if st.Default {
			return st, true
		}
		if !found || st.Name < pick.Name {
			pick, found = st, true
		}
	}
	return pick, found
}

// notifyAutoState puts an auto-write in the inbox. These moves have no human
// caller, so the audit trail is the only way anyone learns they happened.
func (s *Server) notifyAutoState(b domain.Bubble, moved map[string]int) {
	parts := make([]string, 0, len(moved))
	for _, g := range []string{"started", "backlog", "cancelled"} {
		if n := moved[g]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d → %s", n, autoGroupLabel(g)))
		}
	}
	if len(parts) == 0 {
		return
	}
	msg := fmt.Sprintf("%s: moved %s in Plane", b.Name, joinAnd(parts))
	if err := s.store.AddNotification(domain.Notification{
		At: s.now().Format(time.RFC3339), Instance: b.Instance, BubbleID: b.ID,
		BubbleName: b.Name, Kind: "autostate", Message: msg,
	}); err != nil {
		log.Printf("autostate: notify: %v", err)
		return
	}
	log.Printf("notify: %s", msg)
}

func autoGroupLabel(group string) string {
	switch group {
	case "started":
		return "In Progress"
	case "backlog":
		return "Backlog"
	case "cancelled":
		return "Cancelled"
	}
	return group
}

func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	out := ""
	for i, p := range parts {
		switch {
		case i == 0:
			out = p
		case i == len(parts)-1:
			out += " and " + p
		default:
			out += ", " + p
		}
	}
	return out
}

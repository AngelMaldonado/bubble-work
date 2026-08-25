package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/mirror"
	"github.com/AngelMaldonado/bubble-work/internal/store"
)

// Importing an edit made in Plane (docs/decisions/0001).
//
// Plane stays a WRITABLE surface. Bubble Work owns the format — markdown, its
// renderer, its splice engine — and is where documents are authored, but it does not
// own the exclusive right to write: an edit made in Plane's editor wins, and the
// version it replaced is kept.
//
// The cost is stated plainly because decision 0001 was written to avoid it:
// importing runs md.FromHTML, so a Plane-side edit takes the same fidelity loss as
// the old model did. The difference is that it now applies only to bodies actually
// edited in Plane, instead of to every read and write of every thread. Losing an
// edit is worse than importing it imperfectly.
//
// Three cases, and the outbox already arbitrates the only ambiguous one:
//
//	Plane edited, no publication queued  → Plane wins, import it
//	Plane edited, publication queued     → we win; the field lock holds, ours lands
//	nobody edited Plane                  → nothing to do
func (s *Server) importPlaneEdits(slug string, bodies map[string]string) {
	if len(bodies) == 0 {
		return
	}
	ids := make([]string, 0, len(bodies))
	for id := range bodies {
		ids = append(ids, id)
	}
	// A queued publication means Plane is serving something OLDER than what we
	// hold, so its body is not an edit to adopt — it is the state we are about to
	// overwrite.
	locked, err := s.store.LockedFields(slug, ids)
	if err != nil {
		log.Printf("docs: reading write locks for %s: %v", slug, err)
		return
	}

	imported := 0
	for id, html := range bodies {
		pub, err := s.store.Published(id)
		if err != nil {
			log.Printf("docs: publish state of %s: %v", id, err)
			continue
		}
		if pub.PublishedHash == "" {
			// We have never published this thread, so there is nothing to compare
			// against and nothing to adopt: Plane's body simply IS the body, and the
			// read path falls back to the mirror for it.
			continue
		}
		if mirror.HashBody(html) == pub.PublishedHash {
			continue // this is our own publication coming back
		}
		if locked[id]["description"] {
			continue // ours is queued and wins
		}
		if s.importOne(id, html) {
			imported++
		}
	}
	if imported > 0 {
		log.Printf("docs: imported %d edit(s) made in Plane on %s", imported, slug)
	}
}

// importOne adopts one body edited in Plane, keeping what it replaced.
func (s *Server) importOne(wid, html string) bool {
	now := s.now()
	docs := make([]store.ThreadDoc, 0, len(editRegions)+1)
	for _, r := range editRegions {
		m, _ := md.RegionMarkdown(html, r.region)
		docs = append(docs, store.ThreadDoc{
			ThreadID: wid, Region: string(r.region), Markdown: m,
			Hash: md.Hash(m), UpdatedAt: now, UpdatedBy: importedBy,
		})
	}
	tail, _ := md.TailMarkdown(html)
	docs = append(docs, store.ThreadDoc{
		ThreadID: wid, Region: docRegionTail, Markdown: tail,
		Hash: md.Hash(tail), UpdatedAt: now, UpdatedBy: importedBy,
	})

	if err := s.store.ImportedDocs(wid, docs, now); err != nil {
		log.Printf("docs: import %s: %v", wid, err)
		return false
	}
	// The imported state IS what Plane has, by definition. Recording it as published
	// is what stops the next pass importing the same edit again — and, since our own
	// publications are recognised the same way, is the single fact that keeps the two
	// directions from feeding each other.
	if err := s.store.SetPublished(wid, mirror.HashBody(html), now); err != nil {
		log.Printf("docs: record imported publish state of %s: %v", wid, err)
	}
	// An import is not evidence in itself. It flows through the ordinary progress
	// diff, so editing a plan in Plane warms the thread exactly as editing it here
	// does, and editing prose warms nothing.
	s.broadcastThread(wid)
	return true
}

// importedBy marks a stored region as having come from Plane rather than from an
// Actor. It is deliberately not an email: nothing here knows which Plane user made
// the edit, and inventing an author would be worse than admitting that.
const importedBy = "plane"

// Adopt imports every mirrored body into the document store, once, so an instance
// that predates the inversion stops depending on the fallback read path
// (docs/decisions/0001).
//
// It is explicit for the same reason a migration always is: afterwards Bubble Work
// owns the FORMAT of these documents — markdown, its renderer, its splice engine —
// and Plane's copy becomes a rendering. Editing in Plane keeps working and is
// imported (importPlaneEdits), so this is not the one-way door it would have been
// had the server claimed exclusive write access.
//
// Threads that already have a stored document are skipped rather than overwritten:
// re-running this must never clobber writing done since the last run.
func (s *Server) Adopt(ctx context.Context, inst domain.Instance) (domain.AdoptResult, error) {
	if s.mirror == nil {
		return domain.AdoptResult{}, fmt.Errorf("mirror is not available on this server")
	}
	res := domain.AdoptResult{Instance: inst.Slug, At: s.now()}
	projects, err := s.mirror.Projects(inst.Slug)
	if err != nil {
		return res, fmt.Errorf("projects: %w", err)
	}
	now := s.now()
	for _, p := range projects {
		items, err := s.mirror.Items(inst.Slug, p.ID)
		if err != nil {
			return res, fmt.Errorf("items: %w", err)
		}
		for _, it := range items {
			res.Threads++
			existing, err := s.store.ThreadDocs(it.ID)
			if err != nil {
				return res, fmt.Errorf("read docs %s: %w", it.ID, err)
			}
			if len(existing) > 0 {
				res.Skipped++
				continue
			}
			if strings.TrimSpace(it.DescriptionHTML) == "" {
				res.Empty++
				continue
			}
			docs := make([]store.ThreadDoc, 0, len(editRegions)+1)
			for _, r := range editRegions {
				m, _ := md.RegionMarkdown(it.DescriptionHTML, r.region)
				docs = append(docs, store.ThreadDoc{
					ThreadID: it.ID, Region: string(r.region), Markdown: m,
					Hash: md.Hash(m), UpdatedAt: now, UpdatedBy: adoptedBy,
				})
			}
			tail, _ := md.TailMarkdown(it.DescriptionHTML)
			docs = append(docs, store.ThreadDoc{
				ThreadID: it.ID, Region: docRegionTail, Markdown: tail,
				Hash: md.Hash(tail), UpdatedAt: now, UpdatedBy: adoptedBy,
			})
			if err := s.store.PutThreadDocs(docs); err != nil {
				return res, fmt.Errorf("store docs %s: %w", it.ID, err)
			}
			// Plane already holds exactly this body, so it is published by
			// definition — recording that is what stops the first sync after
			// adoption reading every thread as edited in Plane.
			if err := s.store.SetPublished(it.ID, it.DescriptionHash, now); err != nil {
				return res, fmt.Errorf("record publish state %s: %w", it.ID, err)
			}
			res.Adopted++
		}
	}
	actor, _ := domain.ActorFrom(ctx)
	log.Printf("adopt %s by %s: %d adopted, %d already stored, %d empty of %d thread(s)",
		inst.Slug, actor.Label(), res.Adopted, res.Skipped, res.Empty, res.Threads)
	return res, nil
}

// adoptedBy marks a region imported by a bulk adoption rather than by somebody
// editing in Plane. The distinction matters on the read path: every adopted body
// technically came from Plane, so marking them all "written in Plane" would make
// that signal meaningless on exactly the threads where it should mean something.
const adoptedBy = "adopted"

func (s *Server) handleAdminAdopt(w http.ResponseWriter, r *http.Request) {
	inst, ok := s.syncInstance(w, r)
	if !ok {
		return
	}
	res, err := s.Adopt(r.Context(), inst)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// Round-trip fidelity over a whole instance (docs/ARTIFACT-EDITING.md Phase 0).
//
// Sibling of `sync diff`, and deliberately its opposite in cost: diff walks Plane
// to prove the mirror is faithful, this walks only the mirror to prove the
// markdown bridge is. No Plane calls, so it is a GET and safe to run at will.

// unstableSample caps how many damaged threads a report lists. The number that
// matters is the count; the sample is there to show which construct drifted.
const unstableSample = 8

// SyncFidelity round-trips every mirrored body for an instance and reports what
// a write would destroy.
func (s *Server) SyncFidelity(slug string) (domain.SyncFidelity, error) {
	if s.mirror == nil {
		return domain.SyncFidelity{}, fmt.Errorf("mirror is not available on this server")
	}
	started := s.now()
	out := domain.SyncFidelity{Instance: slug}

	projects, err := s.mirror.Projects(slug)
	if err != nil {
		return domain.SyncFidelity{}, err
	}
	for _, p := range projects {
		items, err := s.mirror.Items(slug, p.ID)
		if err != nil {
			return domain.SyncFidelity{}, err
		}
		for _, it := range items {
			// An empty or near-empty description has nothing to round-trip and
			// would only inflate the "stable" count with threads that carry no
			// body at all.
			if len(strings.TrimSpace(it.DescriptionHTML)) < 40 {
				continue
			}
			out.Bodies++
			f := md.Check(it.DescriptionHTML)
			out.Mentions += f.Mentions
			out.Assets += f.Assets
			if f.Stable {
				out.Stable++
				continue
			}
			out.Unstable = append(out.Unstable, domain.FidelityIssue{
				ThreadID: it.ID, Title: it.Name,
				Line: f.Line, Before: f.Before, After: f.After,
				Mentions: f.Mentions, Assets: f.Assets,
			})
		}
	}

	// Most-damaged first: a thread carrying mentions and images loses more than
	// one whose ordered list renumbers.
	sort.SliceStable(out.Unstable, func(i, j int) bool {
		a, b := out.Unstable[i], out.Unstable[j]
		return a.Mentions+a.Assets > b.Mentions+b.Assets
	})
	if len(out.Unstable) > unstableSample {
		out.Elided = len(out.Unstable) - unstableSample
		out.Unstable = out.Unstable[:unstableSample]
	}
	out.TookMS = s.now().Sub(started).Milliseconds()
	return out, nil
}

func (s *Server) handleSyncFidelity(w http.ResponseWriter, r *http.Request) {
	inst, ok := s.syncInstance(w, r)
	if !ok {
		return
	}
	rep, err := s.SyncFidelity(inst.Slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// Exporting the writing to disk (docs/decisions/0001).
//
// The overlay is about to become the record for artifact bodies, which changes
// what losing bubble.db costs: today a backfill, afterwards real work. This is
// the counterweight, and it is deliberately built BEFORE that inversion rather
// than after it — a backup story that arrives second is not a backup story.
//
// What it writes is plain markdown in an obvious tree, so it is useful for the
// thing people actually want it for: putting the work in git, grepping it, or
// walking away from this tool entirely with everything intact.
//
// It reads the mirror rather than Plane, so it costs no rate budget and can be
// run as often as anyone likes. Local pages come from the overlay, because on an
// instance whose Plane cannot hold pages they are the only copy in existence
// (docs/modules/pages.md).

// unsafeName matches everything that has no business in a path segment. Names
// come from Plane, so they contain slashes, colons, emoji and newlines.
var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}\-_. ]+`)

// safeName turns a Plane name into one path segment. It never returns empty, and
// never returns something that could escape the export root.
func safeName(name string, fallback string) string {
	s := strings.TrimSpace(unsafeName.ReplaceAllString(name, " "))
	s = strings.Join(strings.Fields(s), " ") // collapse the runs the replace left
	s = strings.Trim(s, ". ")                // no leading dots, no trailing space
	if len(s) > 80 {
		s = strings.TrimSpace(s[:80])
	}
	if s == "" {
		return fallback
	}
	return s
}

// Export writes an instance's work to disk as a markdown tree and returns what it
// wrote. dir may be empty, in which case it exports under the server's own home.
func (s *Server) Export(ctx context.Context, inst domain.Instance, dir string) (domain.ExportResult, error) {
	if s.mirror == nil {
		return domain.ExportResult{}, fmt.Errorf("mirror is not available on this server")
	}
	if dir == "" {
		home, err := config.Home()
		if err != nil {
			return domain.ExportResult{}, err
		}
		dir = filepath.Join(home, "export", inst.Slug+"-"+s.now().UTC().Format("20060102-150405"))
	}
	if !filepath.IsAbs(dir) {
		// The server may be running anywhere, and a relative path would mean
		// something different depending on how it was started.
		return domain.ExportResult{}, fmt.Errorf("%w: export needs an absolute path, got %q", errBadRequest, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.ExportResult{}, err
	}

	res := domain.ExportResult{Instance: inst.Slug, Dir: dir, At: s.now()}

	projects, err := s.mirror.Projects(inst.Slug)
	if err != nil {
		return res, fmt.Errorf("projects: %w", err)
	}
	for _, p := range projects {
		projDir := filepath.Join(dir, safeName(p.Name, p.ID))
		res.Workspaces++

		// Which bubble each thread belongs to. A thread can be in several modules
		// in Plane; it is exported under the first, and threads in none land in
		// _unfiled rather than being dropped — the point of a backup is that
		// nothing is silently missing.
		home := map[string]string{}
		modules, err := s.mirror.Modules(inst.Slug, p.ID)
		if err != nil {
			return res, fmt.Errorf("modules: %w", err)
		}
		for _, m := range modules {
			items, err := s.mirror.ModuleItems(inst.Slug, m.ID)
			if err != nil {
				return res, fmt.Errorf("module items: %w", err)
			}
			res.Bubbles++
			for _, it := range items {
				if _, taken := home[it.ID]; !taken {
					home[it.ID] = safeName(m.Name, m.ID)
				}
			}
		}

		items, err := s.mirror.Items(inst.Slug, p.ID)
		if err != nil {
			return res, fmt.Errorf("items: %w", err)
		}
		// Revisions are sub-work-items, so they are written inside their parent's
		// directory rather than as threads of their own.
		children := map[string][]int{}
		for i, it := range items {
			if it.ParentID != "" {
				children[it.ParentID] = append(children[it.ParentID], i)
			}
		}

		for _, it := range items {
			if it.ParentID != "" {
				continue // written with its parent
			}
			bubble := home[it.ID]
			if bubble == "" {
				bubble = "_unfiled"
			}
			threadDir := filepath.Join(projDir, bubble, safeName(it.Name, it.ID))
			body := md.FromHTML(it.DescriptionHTML)
			arts, logbook := md.ParseThread(body, it.Name)

			var brief strings.Builder
			brief.WriteString("# " + it.Name + "\n\n")
			for _, a := range arts {
				brief.WriteString(a.Markdown)
				brief.WriteString("\n")
			}
			n, err := writeFile(threadDir, "BRIEF.md", brief.String())
			if err != nil {
				return res, err
			}
			res.Files += n
			if logbook != nil {
				lb := logbook.Markdown
				if strings.TrimSpace(logbook.DoDHTML) != "" || len(logbook.DoD) > 0 {
					// The DoD is parsed out of the body separately, so writing only
					// logbook.Markdown would drop the finish line.
					lb += "\n\n## Definition of Done\n"
					for _, d := range logbook.DoD {
						box := "[ ]"
						if d.Done {
							box = "[x]"
						}
						lb += "- " + box + " " + d.Text + "\n"
					}
				}
				n, err := writeFile(threadDir, "LOGBOOK.md", lb)
				if err != nil {
					return res, err
				}
				res.Files += n
			}
			for _, ci := range children[it.ID] {
				c := items[ci]
				n, err := writeFile(filepath.Join(threadDir, "revisions"),
					safeName(c.Name, c.ID)+".md", "# "+c.Name+"\n\n"+md.FromHTML(c.DescriptionHTML))
				if err != nil {
					return res, err
				}
				res.Files += n
				res.Revisions++
			}
			res.Threads++
		}

		// Pages held HERE. Plane-held pages are left alone: Plane is their record,
		// and this walk deliberately spends no Plane calls.
		pages, err := s.store.LocalPages(inst.Slug, p.ID)
		if err != nil {
			return res, fmt.Errorf("local pages: %w", err)
		}
		for _, pg := range pages {
			n, err := writeFile(filepath.Join(projDir, "pages"),
				safeName(pg.Title, pg.ID)+".md", "# "+pg.Title+"\n\n"+pg.Body)
			if err != nil {
				return res, err
			}
			res.Files += n
			res.Pages++
		}
	}
	return res, nil
}

// writeFile writes one export file, creating its directory. Empty content is
// skipped and reported as zero files — an empty BRIEF.md is noise in a backup.
func writeFile(dir, name, content string) (int, error) {
	if strings.TrimSpace(content) == "" {
		return 0, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		return 0, err
	}
	return 1, nil
}

func (s *Server) handleAdminExport(w http.ResponseWriter, r *http.Request) {
	inst, ok := s.syncInstance(w, r)
	if !ok {
		return
	}
	var req struct {
		Dir string `json:"dir"`
	}
	// An empty body is fine: it means "somewhere sensible".
	_ = json.NewDecoder(r.Body).Decode(&req)

	res, err := s.Export(r.Context(), inst, strings.TrimSpace(req.Dir))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

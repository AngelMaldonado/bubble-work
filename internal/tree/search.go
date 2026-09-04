package tree

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Hit is one matching line.
type Hit struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Area  Area   `json:"area"`
	Line  int    `json:"line"`
	Text  string `json:"text"`
}

// Search greps a workspace's documents.
//
// The tree LISTS; this is what reads. Without it an agent can only ever act on
// what it was told about, which is the difference between a tool it can work with
// and a tool it has to be walked through.
//
// Plain substring matching, case-insensitive. Not a regular expression: a wrong
// regex from an agent is a silent empty result or a runaway scan, and "find me the
// place that says X" is what actually gets asked. Not an index either — at this
// size the walk is cheaper than anything that would have to be kept in step with
// the files, and an index that can be stale is another source of truth.
func (t *Tree) Search(repo, needle string, limit int) ([]Hit, error) {
	dir, err := t.repoDir(repo)
	if err != nil {
		return nil, err
	}
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	lower := strings.ToLower(needle)

	var hits []Hit
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if len(hits) >= limit {
			return filepath.SkipAll
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		area, cerr := Classify(rel)
		if cerr != nil {
			return nil // not part of the layout: not ours to search
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		title := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		for i, line := range strings.Split(string(b), "\n") {
			if !strings.Contains(strings.ToLower(line), lower) {
				continue
			}
			hits = append(hits, Hit{
				Path: rel, Title: title, Area: area,
				Line: i + 1, Text: clip(strings.TrimSpace(line)),
			})
			if len(hits) >= limit {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// Threads before wiki pages: somebody searching is usually looking for work.
	sort.SliceStable(hits, func(i, j int) bool {
		if (hits[i].Area == AreaThread) != (hits[j].Area == AreaThread) {
			return hits[i].Area == AreaThread
		}
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	return hits, nil
}

func clip(s string) string {
	const max = 200
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

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
	// Around son las líneas de alrededor, cuando se piden: el ciclo real de
	// quien busca para editar es buscar → mirar el trozo → citar el contexto, y
	// sin esto el paso del medio es una lectura entera del documento.
	Around string `json:"around,omitempty"`
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
// `around` pide N líneas de contexto a cada lado de cada acierto. Cero es lo de
// siempre: sólo la línea, recortada.
func (t *Tree) Search(repo, needle string, limit, around int) ([]Hit, error) {
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
		all := strings.Split(string(b), "\n")
		for i, line := range all {
			if !strings.Contains(strings.ToLower(line), lower) {
				continue
			}
			h := Hit{
				Path: rel, Title: title, Area: area,
				Line: i + 1, Text: clip(strings.TrimSpace(line)),
			}
			if around > 0 {
				// Sin recortar cada línea: lo que se va a citar en un `edit`
				// tiene que ser el texto EXACTO, y un «…» al final lo convierte
				// en una cita que no encuentra nada.
				lo := max(0, i-around)
				hi := min(len(all), i+around+1)
				h.Around = strings.Join(all[lo:hi], "\n")
			}
			hits = append(hits, h)
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

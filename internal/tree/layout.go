package tree

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The layout of a workspace repository.
//
//	<repo>/
//	  README.md          the workspace's front page
//	  threads/           planning and execution — one file per thread, FLAT
//	    1-primer-thread.md
//	  docs/              the wiki — guides, references, whatever the team keeps
//	    onboarding.md
//	    arquitectura/decisiones.md
//
// The two directories are the whole of the "belonging" rule: a write under
// `threads/` is attributed to the thread that owns that path, and everything else
// is attributed to the workspace. Nothing else is writable, because a tree where
// files can appear anywhere is a tree nobody can reason about — and attribution
// would have nowhere to come from.
//
// `threads/` is flat on purpose. Threads are enumerated by `seq`, nesting adds
// nothing, and it would make the rename that already moves files ambiguous.
// `docs/` nests freely, because a wiki wants folders.
const (
	DirThreads = "threads"
	DirDocs    = "docs"
	Readme     = "README.md"
)

// Area says which half of the layout a path belongs to.
type Area string

const (
	AreaThread Area = "thread" // threads/<file> — owned by one thread
	AreaDoc    Area = "doc"    // docs/** and README.md — owned by the workspace
)

// writable extensions.
//
// `.excalidraw.md` needs no special case: it already ends in `.md`. Bare
// `.excalidraw` is the plugin's own JSON and is allowed so a drawing can live
// beside the page that embeds it.
//
// Binary attachments — images, PDFs — are NOT writable here. They want an upload
// path with size limits of its own, and a git repository is a poor place to put
// them without deciding that first.
var writableExt = map[string]bool{
	".md":         true,
	".excalidraw": true,
}

// Classify says where a document path sits in the layout, refusing anything that
// does not fit it.
func Classify(doc string) (Area, error) {
	rel, err := safeRel(doc)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)

	if !writableExt[strings.ToLower(path.Ext(rel))] {
		return "", fmt.Errorf("%w: %q — only .md and .excalidraw files live here", ErrOutside, doc)
	}

	switch {
	case rel == Readme:
		return AreaDoc, nil

	case strings.HasPrefix(rel, DirThreads+"/"):
		// Flat: exactly one segment after the directory.
		if strings.Contains(rel[len(DirThreads)+1:], "/") {
			return "", fmt.Errorf("%w: %q — %s/ does not nest", ErrOutside, doc, DirThreads)
		}
		return AreaThread, nil

	case strings.HasPrefix(rel, DirDocs+"/"):
		return AreaDoc, nil
	}
	return "", fmt.Errorf("%w: %q — a document lives in %s/, in %s/, or is %s",
		ErrOutside, doc, DirThreads, DirDocs, Readme)
}

// Entry is one node of a workspace's tree.
type Entry struct {
	Path  string `json:"path"`           // relative to the repository root
	Name  string `json:"name"`           // the file name
	Title string `json:"title"`          // the name without its extension — a file IS its title
	Dir   bool   `json:"dir"`            // a directory
	Area  Area   `json:"area,omitempty"` // which half of the layout it is in
	Size  int64  `json:"size,omitempty"` // bytes, for files
}

// Tree walks a workspace repository, newest layout first: README, threads, docs.
//
// The directory IS the index. There is no row mirroring these files, which is the
// point — one source, and `docs/` needs no schema to grow. The title is the file
// name, so naming a document is the whole of naming it.
func (t *Tree) Tree(repo string) ([]Entry, error) {
	dir, err := t.repoDir(repo)
	if err != nil {
		return nil, err
	}
	var out []Entry
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		// Git's own directory is machinery, not content.
		if d.IsDir() && (d.Name() == ".git") {
			return filepath.SkipDir
		}
		e := Entry{Path: rel, Name: d.Name(), Dir: d.IsDir()}
		if d.IsDir() {
			out = append(out, e)
			return nil
		}
		area, cerr := Classify(rel)
		if cerr != nil {
			return nil // not part of the layout: not ours to show
		}
		e.Area = area
		e.Title = strings.TrimSuffix(d.Name(), path.Ext(d.Name()))
		if info, ierr := d.Info(); ierr == nil {
			e.Size = info.Size()
		}
		out = append(out, e)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

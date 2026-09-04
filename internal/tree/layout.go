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
//	  assets/            images, referenced from anywhere as assets/<name>
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
	DirAssets  = "assets"
	Readme     = "README.md"
)

// Area says which half of the layout a path belongs to.
type Area string

const (
	AreaThread Area = "thread" // threads/<file> — owned by one thread
	AreaDoc    Area = "doc"    // docs/** and README.md — owned by the workspace
	AreaAsset  Area = "asset"  // assets/** — images, owned by the workspace
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

// Images, and only under assets/.
//
// One directory rather than beside the page that uses them: a thread would have to
// write `../docs/…` and a nested page `../../…`, and a path that depends on where
// the writer happens to be is a path people get wrong. From anywhere it is
// `assets/<name>`, and the renderer resolves it against the workspace root.
//
// SVG is here because it is what diagrams arrive as. It is also markup that can
// carry script, so every asset is served with a Content-Security-Policy that
// permits nothing and with nosniff — see the serving route.
var imageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".svg": true, ".avif": true,
}

// IsImage reports whether a path is an image this store accepts.
func IsImage(p string) bool { return imageExt[strings.ToLower(path.Ext(p))] }

// Classify says where a document path sits in the layout, refusing anything that
// does not fit it.
func Classify(doc string) (Area, error) {
	rel, err := safeRel(doc)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)

	ext := strings.ToLower(path.Ext(rel))

	// assets/ is the one place a non-text file lives, and the only thing that
	// lives there.
	if strings.HasPrefix(rel, DirAssets+"/") {
		if !imageExt[ext] {
			return "", fmt.Errorf("%w: %q — %s/ holds images", ErrOutside, doc, DirAssets)
		}
		return AreaAsset, nil
	}
	if !writableExt[ext] {
		return "", fmt.Errorf("%w: %q — documents are .md or .excalidraw; images go in %s/",
			ErrOutside, doc, DirAssets)
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
	return "", fmt.Errorf("%w: %q — a file lives in %s/, %s/ or %s/, or is %s",
		ErrOutside, doc, DirThreads, DirDocs, DirAssets, Readme)
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

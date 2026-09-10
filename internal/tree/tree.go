// Package tree is the artifact store: markdown files in a readable directory
// tree, versioned by git.
//
// The file is the RECORD. PocketBase holds the path and the metadata, not the
// bytes (PLAN.md). That buys real history, blame and diff, and makes "export" stop
// being a feature — at the cost of two sources that can drift, which is what the
// lock in here exists to prevent.
package tree

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ErrConflict is returned when a write's base hash does not match what is on
// disk: somebody else changed the file since it was read.
var ErrConflict = errors.New("the file changed since you read it")

// ErrOutside is returned for a path that would land outside its workspace root.
var ErrOutside = errors.New("path escapes the workspace root")

// Tree is the whole artifact store: one directory holding one git repository per
// workspace.
type Tree struct {
	root string

	mu    sync.Mutex
	locks map[string]*sync.Mutex // absolute path -> its content lock
	repos map[string]*sync.Mutex // repo name    -> its git lock
}

// New returns a Tree rooted at dir, creating it if needed.
func New(dir string) (*Tree, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Tree{
		root:  abs,
		locks: map[string]*sync.Mutex{},
		repos: map[string]*sync.Mutex{},
	}, nil
}

// Root is the directory every workspace repository lives under.
func (t *Tree) Root() string { return t.root }

// Hash fingerprints content. It is what a write sends back as its base, and what
// the next write is checked against.
func Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:16])
}

// safeRel cleans a relative path and REFUSES anything that climbs out or is
// absolute.
//
// The usual trick is `filepath.Clean("/" + p)`, which neutralises traversal by
// turning "../x" into "/x". That is safe but silent: the caller asked for one
// path and their file appears at another. On an API, being told the path is wrong
// beats finding the document somewhere unexpected — so this rejects instead.
//
// Checked on the CLEANED result rather than by scanning the input for "..":
// "a/../../b" and "a/b/../../../c" only reveal themselves once normalised, and a
// blocklist of shapes is a blocklist somebody gets around.
func safeRel(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("%w: empty path", ErrOutside)
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("%w: %q is absolute", ErrOutside, p)
	}
	c := filepath.Clean(p)
	if c == ".." || strings.HasPrefix(c, ".."+string(os.PathSeparator)) || c == "." {
		return "", fmt.Errorf("%w: %q", ErrOutside, p)
	}
	return c, nil
}

// repoDir is a workspace's repository directory.
func (t *Tree) repoDir(repo string) (string, error) {
	rel, err := safeRel(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(t.root, rel), nil
}

// resolve turns a workspace repo and a document path into an absolute path.
func (t *Tree) resolve(repo, doc string) (string, error) {
	base, err := t.repoDir(repo)
	if err != nil {
		return "", err
	}
	rel, err := safeRel(doc)
	if err != nil {
		return "", err
	}
	// The layout decides what may exist at all, and where.
	if _, err := Classify(rel); err != nil {
		return "", err
	}
	full := filepath.Join(base, rel)
	// Belt and braces: whatever the input looked like, this is where it lands.
	if !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q", ErrOutside, doc)
	}
	return full, nil
}

// lockFor returns the mutex guarding one absolute path. One lock per file rather
// than one for the whole tree: two people editing different threads must not
// queue behind each other, and two editing the same one must.
func (t *Tree) lockFor(path string) *sync.Mutex {
	t.mu.Lock()
	defer t.mu.Unlock()
	l, ok := t.locks[path]
	if !ok {
		l = &sync.Mutex{}
		t.locks[path] = l
	}
	return l
}

// gitLockFor returns the mutex guarding one REPOSITORY.
//
// The per-path lock is not enough: git takes a repository-wide index.lock, so two
// commits in one repository collide however different their files are —
//
//	fatal: Unable to create '.../.git/index.lock': File exists.
//
// Content is still written concurrently; only the commit serialises. The order is
// always path lock then git lock, so nothing can deadlock against a move that
// holds two path locks.
func (t *Tree) gitLockFor(repo string) *sync.Mutex {
	t.mu.Lock()
	defer t.mu.Unlock()
	l, ok := t.repos[repo]
	if !ok {
		l = &sync.Mutex{}
		t.repos[repo] = l
	}
	return l
}

// ReadBytes returns a file's raw bytes — what an image needs.
func (t *Tree) ReadBytes(repo, doc string) ([]byte, error) {
	full, err := t.resolve(repo, doc)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

// WriteBytes replaces a file with raw bytes and commits it.
//
// No base hash: this is for assets, which are replaced whole or not at all. There
// is no such thing as a surgical edit to a PNG, and a conflict on one is a person
// uploading the same picture twice.
func (t *Tree) WriteBytes(repo, doc string, content []byte, actor, message string) error {
	full, err := t.resolve(repo, doc)
	if err != nil {
		return err
	}
	l := t.lockFor(full)
	l.Lock()
	defer l.Unlock()

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		return err
	}
	return t.commit(repo, []string{full}, actor, message)
}

// Read returns a document's content and its hash. A file that does not exist
// reads as empty with the hash of empty, so a first write and a rewrite are the
// same operation to the caller.
func (t *Tree) Read(repo, doc string) (content, hash string, err error) {
	full, err := t.resolve(repo, doc)
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(full)
	if errors.Is(err, os.ErrNotExist) {
		return "", Hash(""), nil
	}
	if err != nil {
		return "", "", err
	}
	return string(b), Hash(string(b)), nil
}

// Write replaces a document and commits it.
//
// base is the hash the writer read. An empty base means "I know there is nothing
// there"; any other mismatch is a conflict rather than a silent overwrite —
// replacing a whole file to change one line is how one writer destroys a
// paragraph they never read.
//
// The lock is held across read-check-write-commit, so a concurrent writer sees
// the finished state rather than a half-applied one.
func (t *Tree) Write(repo, doc, base, content, actor, message string) (hash string, err error) {
	full, err := t.resolve(repo, doc)
	if err != nil {
		return "", err
	}
	l := t.lockFor(full)
	l.Lock()
	defer l.Unlock()

	cur, curHash, err := t.readLocked(full)
	if err != nil {
		return "", err
	}
	if base != curHash {
		return "", fmt.Errorf("%w (on disk %s, you had %s)", ErrConflict, curHash, base)
	}
	if cur == content {
		return curHash, nil // nothing changed: not a commit, not an error
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return "", err
	}
	if err := t.commit(repo, []string{full}, actor, message); err != nil {
		return "", err
	}
	return Hash(content), nil
}

// Remove deletes a document and commits the deletion.
func (t *Tree) Remove(repo, doc, actor, message string) error {
	full, err := t.resolve(repo, doc)
	if err != nil {
		return err
	}
	l := t.lockFor(full)
	l.Lock()
	defer l.Unlock()

	if err := os.Remove(full); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return t.commit(repo, []string{full}, actor, message)
}

// Move renames a document, keeping git's rename detection intact by moving the
// file rather than writing a new one and deleting the old.
func (t *Tree) Move(repo, from, to, actor, message string) error {
	src, err := t.resolve(repo, from)
	if err != nil {
		return err
	}
	dst, err := t.resolve(repo, to)
	if err != nil {
		return err
	}
	if src == dst {
		return nil
	}
	// Both locks, in a stable order, so two moves that cross cannot deadlock.
	first, second := src, dst
	if first > second {
		first, second = second, first
	}
	l1, l2 := t.lockFor(first), t.lockFor(second)
	l1.Lock()
	defer l1.Unlock()
	l2.Lock()
	defer l2.Unlock()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return t.commit(repo, []string{src, dst}, actor, message)
}

func (t *Tree) readLocked(full string) (content, hash string, err error) {
	b, err := os.ReadFile(full)
	if errors.Is(err, os.ErrNotExist) {
		return "", Hash(""), nil
	}
	if err != nil {
		return "", "", err
	}
	return string(b), Hash(string(b)), nil
}

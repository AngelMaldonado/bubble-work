package tree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Git is the content history. `events` (phase 2) is the queryable index — the two
// are not redundant: git cannot cheaply answer "all evidence this cycle across
// workspaces", and most evidence is not a file change at all.
//
// Shelling out to git rather than using a library: the repository has to be
// readable and usable by a person with their own git, and the surface used here is
// four commands.

// EnsureRepo creates a workspace's repository if it does not exist yet.
func (t *Tree) EnsureRepo(repo string) error {
	dir, err := t.repoDir(repo)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// -b main so the branch name does not depend on the machine's git config.
	if out, err := run(dir, "init", "-b", "main"); err != nil {
		return fmt.Errorf("git init: %w: %s", err, out)
	}
	return nil
}

// commit stages exactly the given paths and records one commit authored as the
// actor.
//
// Only the given paths, never `add -A`: with two writers in flight, staging the
// whole repository sweeps the other one's file into this commit. The content
// would still be saved, but the history would attribute it to the wrong person —
// and "who wrote this" is the reason git is here at all.
func (t *Tree) commit(repo string, paths []string, actor, message string) error {
	dir, err := t.repoDir(repo)
	if err != nil {
		return err
	}
	// The repository-wide index lock is git's, not ours; taking ours first turns a
	// collision into a wait instead of an error.
	gl := t.gitLockFor(repo)
	gl.Lock()
	defer gl.Unlock()

	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		// -A so a deletion stages as a deletion.
		if out, err := run(dir, "add", "-A", "--", rel); err != nil {
			return fmt.Errorf("git add: %w: %s", err, out)
		}
	}
	// `diff --cached --quiet` exits 0 when nothing is staged. Committing anyway
	// would record an empty commit for every no-op write.
	if _, err := run(dir, "diff", "--cached", "--quiet"); err == nil {
		return nil
	}
	name, email := authorOf(actor)
	args := []string{
		"-c", "user.name=" + name,
		"-c", "user.email=" + email,
		"commit", "--no-gpg-sign", "-m", message,
	}
	if out, err := run(dir, args...); err != nil {
		// A commit with nothing staged is not a failure worth propagating.
		if strings.Contains(out, "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %w: %s", err, out)
	}
	return nil
}

// Log returns the commit history of one document, newest first, as
// "<short-sha> <iso-date> <author> <subject>" lines.
func (t *Tree) Log(repo, doc string, limit int) ([]string, error) {
	full, err := t.resolve(repo, doc)
	if err != nil {
		return nil, err
	}
	dir, err := t.repoDir(repo)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(dir, full)
	if err != nil {
		return nil, err
	}
	out, err := run(dir, "log", "--follow", fmt.Sprintf("-%d", limit),
		"--format=%h %aI %an %s", "--", rel)
	if err != nil {
		return nil, nil // no history yet is not an error
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// authorOf turns an actor label into a git identity. The actor is whoever the
// server authenticated, so the history answers "who wrote this" without anybody
// maintaining a second record of it.
func authorOf(actor string) (name, email string) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "bubble", "bubble@localhost"
	}
	if strings.Contains(actor, "@") {
		return actor, actor
	}
	return actor, actor + "@localhost"
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(gitCleanEnv(),
		// A machine's git config must not change what the server records.
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"HOME="+dir,
	)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// gitCleanEnv is the environment with every inherited GIT_* variable removed.
//
// Not paranoia — a bug this caught. Git exports GIT_DIR, GIT_INDEX_FILE and
// friends to the processes it runs, so anything started from inside a git hook
// inherits them. Our commands set cmd.Dir and would still reach for the OUTER
// repository's index:
//
//	error: invalid object 100644 … for '.codegraph/.gitignore'
//	error: Error building trees
//
// It surfaced in the pre-commit hook, but the hook is only the easiest way to
// meet it: any parent that exports those variables would corrupt what the server
// records, in a workspace repository it was never asked to touch. The server
// decides where it is writing, so it inherits nothing about where that is.
func gitCleanEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Churn is how much a file changed over a window, in lines.
//
// Lines and not bytes because a line is what a person wrote; and added and
// removed apart rather than a net number, because "+120 −4" and "+124 −8" are
// different afternoons and a single "+116" hides which.
type Churn struct {
	Path  string `json:"path"`
	Added int    `json:"added"`
	Gone  int    `json:"removed"`
}

// Changed reports what moved in a repository since an instant, per file.
//
// One git call for the whole tree rather than one per document: the numbers are
// for a sidebar, and a sidebar that costs a process per row is a sidebar that
// stops being drawn.
//
// `-M` is what keeps a rename from reading as "+everything −everything". A
// thread renamed mid-cycle moves its file, and without rename detection that
// move would look like the biggest piece of work in the workspace.
func (t *Tree) Changed(repo, since string) ([]Churn, error) {
	dir, err := t.repoDir(repo)
	if err != nil {
		return nil, err
	}
	out, err := run(dir, "log", "-M", "--since="+since, "--numstat", "--format=")
	if err != nil {
		return nil, nil // a repository with no history is not an error
	}
	by := map[string]*Churn{}
	for _, line := range strings.Split(out, "\n") {
		cols := strings.Split(strings.TrimSpace(line), "\t")
		if len(cols) != 3 {
			continue
		}
		// A binary file's counts are "-", and an image has no lines to speak of.
		added, err1 := strconv.Atoi(cols[0])
		gone, err2 := strconv.Atoi(cols[1])
		if err1 != nil || err2 != nil {
			continue
		}
		// After a rename git writes `old => new`; the file people mean is the
		// one it is called now.
		p := cols[2]
		if i := strings.Index(p, " => "); i >= 0 {
			p = renamed(p)
		}
		c, ok := by[p]
		if !ok {
			c = &Churn{Path: p}
			by[p] = c
		}
		c.Added += added
		c.Gone += gone
	}
	list := make([]Churn, 0, len(by))
	for _, c := range by {
		list = append(list, *c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	return list, nil
}

// renamed reads git's rename notation into the path a file has NOW.
//
// Two shapes: `old.md => new.md`, and the compact `dir/{old => new}.md` git
// writes when only part of the path moved.
func renamed(p string) string {
	if i, j := strings.Index(p, "{"), strings.Index(p, "}"); i >= 0 && j > i {
		inner := p[i+1 : j]
		if k := strings.Index(inner, " => "); k >= 0 {
			return p[:i] + inner[k+4:] + p[j+1:]
		}
	}
	if k := strings.Index(p, " => "); k >= 0 {
		return strings.TrimSpace(p[k+4:])
	}
	return p
}

// Diff returns a unified diff of one document over a window, as git writes it.
//
// The text comes from git rather than from a diffing library in the browser for
// the same reason the markdown is rendered on the server: there is already one
// answer to "what changed", and a second implementation agrees with it until it
// does not.
func (t *Tree) Diff(repo, doc, since string) (string, error) {
	full, err := t.resolve(repo, doc)
	if err != nil {
		return "", err
	}
	dir, err := t.repoDir(repo)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(dir, full)
	if err != nil {
		return "", err
	}
	// The oldest commit inside the window, then everything after it. `git diff
	// --since` does not exist: `since` selects commits, and a diff needs two
	// ends.
	from, err := run(dir, "log", "-M", "--since="+since, "--format=%H", "--", rel)
	if err != nil || from == "" {
		return "", nil // nothing changed in this window
	}
	lines := strings.Split(from, "\n")
	oldest := strings.TrimSpace(lines[len(lines)-1])
	out, err := run(dir, "diff", "-M", "--unified=3", oldest+"^", "HEAD", "--", rel)
	if err != nil {
		// The oldest commit in the window may be the file's first: there is no
		// parent to diff against, so show it whole.
		out, err = run(dir, "diff", "-M", "--unified=3",
			"4b825dc642cb6eb9a060e54bf8d69288fbee4904", "HEAD", "--", rel)
		if err != nil {
			return "", nil
		}
	}
	return out, nil
}

// LastDiff is the newest change to one document, whenever it happened.
//
// The other half of the pair: `Diff` answers "what moved in this window", which
// is empty for a document nobody touched this cycle — and "nothing this cycle"
// is a true answer that is useless when what you wanted was to see the last
// thing somebody did to it.
func (t *Tree) LastDiff(repo, doc string) (string, error) {
	dir, err := t.repoDir(repo)
	if err != nil {
		return "", err
	}
	full, err := t.resolve(repo, doc)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(dir, full)
	if err != nil {
		return "", err
	}
	sha, err := run(dir, "log", "-M", "-1", "--format=%H", "--", rel)
	if err != nil || sha == "" {
		return "", nil
	}
	out, err := run(dir, "diff", "-M", "--unified=3", sha+"^", sha, "--", rel)
	if err != nil {
		out, err = run(dir, "diff", "-M", "--unified=3",
			"4b825dc642cb6eb9a060e54bf8d69288fbee4904", sha, "--", rel)
		if err != nil {
			return "", nil
		}
	}
	return out, nil
}

// Before returns a document as it stood at the START of a window, so a diff has
// two ends to compare.
//
// A unified diff is what git prints; two documents is what a merge view needs.
// Both come from the same place on purpose — a browser that reconstructs one
// side from a patch is a second implementation of "what changed".
//
// A file that did not exist yet comes back empty, which is exactly right: what
// changed is that all of it appeared.
func (t *Tree) Before(repo, doc, since string) (string, error) {
	dir, err := t.repoDir(repo)
	if err != nil {
		return "", err
	}
	full, err := t.resolve(repo, doc)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(dir, full)
	if err != nil {
		return "", err
	}
	var oldest string
	if since == "last" {
		oldest, err = run(dir, "log", "-M", "-1", "--format=%H", "--", rel)
	} else {
		var list string
		list, err = run(dir, "log", "-M", "--since="+since, "--format=%H", "--", rel)
		if list != "" {
			lines := strings.Split(list, "\n")
			oldest = strings.TrimSpace(lines[len(lines)-1])
		}
	}
	if err != nil || oldest == "" {
		// Nothing changed in the window: the two ends are the same text, and the
		// caller draws a diff with nothing in it.
		body, _, rerr := t.Read(repo, doc)
		return body, rerr
	}
	out, err := run(dir, "show", oldest+"^:"+rel)
	if err != nil {
		return "", nil // its first commit: before this, there was nothing
	}
	return out, nil
}

package tree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

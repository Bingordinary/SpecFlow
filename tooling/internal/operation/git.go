package operation

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
)

// git runs one read-only git command in dir and returns stdout.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// requireWorkTreeTop verifies that repoRoot is the git worktree top level, so
// the repository-relative paths reported by git match the operation state's
// path space.
func requireWorkTreeTop(repoRoot string) error {
	return repofiles.RequireWorkTreeTop(repoRoot)
}

// resolveCommit resolves a git ref to the full commit SHA.
func resolveCommit(repoRoot, ref string) (string, error) {
	out, err := git(repoRoot, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve baseline %q: %w", ref, err)
	}
	sha := strings.TrimSpace(out)
	if sha == "" {
		return "", fmt.Errorf("resolve baseline %q: empty commit sha", ref)
	}
	return sha, nil
}

// changedPaths computes the repository-relative change set against a baseline
// commit: tracked changes (committed, staged, and unstaged, including
// deletions) plus untracked non-ignored files. Renames are decomposed into
// delete + add so both sides of a rename are evaluated. The tooling's own
// process state and derived caches (meta/, docs/specs/meta/) are excluded.
func changedPaths(repoRoot, sha string) ([]string, error) {
	diffOut, err := git(repoRoot, "diff", "--name-only", "--no-renames", "-z", sha, "--")
	if err != nil {
		return nil, fmt.Errorf("diff against baseline %s: %w", sha, err)
	}
	untrackedOut, err := git(repoRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}

	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		if p == "" {
			return
		}
		p = filepath.ToSlash(filepath.Clean(p))
		if p == "." || p == "" || excluded(p) || seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	for _, p := range strings.Split(diffOut, "\x00") {
		add(p)
	}
	for _, p := range strings.Split(untrackedOut, "\x00") {
		add(p)
	}
	sort.Strings(paths)
	return paths, nil
}

// excluded reports whether a repository-relative path is outside the
// operation's change set by construction: the tooling's local process state
// (meta/) and its derived caches (docs/specs/meta/) are not operation
// deliverables (tooling/README.md §Operation scope).
func excluded(p string) bool {
	return p == "meta" || strings.HasPrefix(p, "meta/") ||
		p == "docs/specs/meta" || strings.HasPrefix(p, "docs/specs/meta/")
}

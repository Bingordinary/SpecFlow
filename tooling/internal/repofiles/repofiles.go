// Package repofiles enumerates the files that constitute repository content
// under a directory. The definition is Git's: a file belongs to repository
// content when Git tracks it or when it is untracked and not ignored by any
// ignore rule (.gitignore, .git/info/exclude, or the global excludes file).
// Ignored dependencies and build output are therefore never part of a code
// surface. Every consumer — gate snapshots, promote baselines, and mechanical
// validation — expands directories through this one definition, so the three
// can never disagree about what a declared directory contains.
package repofiles

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// File is one repository-content file under an expanded directory.
type File struct {
	Path string // canonical repository-relative slash path
	Hash string // normalized content hash (specpaths.FileHash)
}

// RequireWorkTreeTop verifies that repoRoot is the git worktree top level, so
// the repository-relative paths reported by Git match repoRoot's path space.
// The comparison resolves symlinks on both sides (macOS /var -> /private/var)
// and then checks directory identity (os.SameFile) instead of string
// equality: on case-insensitive filesystems (macOS, Windows) a repoRoot whose
// spelling differs only in letter case from the worktree top — for example a
// host shell whose PWD environment variable disagrees with the on-disk case —
// is the same directory and must be accepted, while on case-sensitive
// filesystems a case variant either does not exist or is genuinely a
// different directory and still fails closed.
func RequireWorkTreeTop(repoRoot string) error {
	out, err := runGit(repoRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("resolve git worktree: %w", err)
	}
	top := strings.TrimSpace(out)
	if top == "" {
		return fmt.Errorf("resolve git worktree: empty top-level path")
	}
	top = filepath.Clean(top)
	resolvedRoot := repoRoot
	if r, err := filepath.EvalSymlinks(repoRoot); err == nil {
		resolvedRoot = r
	}
	resolvedTop := top
	if r, err := filepath.EvalSymlinks(top); err == nil {
		resolvedTop = r
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil {
		return fmt.Errorf("--repo-root %q is not accessible: %w", repoRoot, err)
	}
	topInfo, err := os.Stat(resolvedTop)
	if err != nil {
		return fmt.Errorf("git worktree top level %q is not accessible: %w", top, err)
	}
	if !os.SameFile(rootInfo, topInfo) {
		return fmt.Errorf("--repo-root %q is not the git worktree top level (%q); run from the repository root", repoRoot, top)
	}
	return nil
}

// ExpandDir lists every repository-content file under dir, hashed and sorted
// by path. dir may be repository-relative or absolute and must resolve inside
// repoRoot. Expansion is defined by Git's content set, so a repoRoot that is
// not the worktree top level fails closed instead of falling back to a
// filesystem walk that would include ignored dependencies and build output.
// Non-file entries — submodule gitlinks, directory symlinks, and tracked paths
// absent from the working tree — are not repository-content files and are
// skipped; a directory holding only ignored files therefore expands to zero
// files.
//
// ExpandDir is the one-shot form of Expander.Expand: it resolves the worktree
// and expands dir through a fresh expander. Consumers that expand more than
// one directory per invocation should hold one Expander instead, so repeated
// declarations of the same directory expand once.
func ExpandDir(repoRoot, dir string) ([]File, error) {
	e, err := NewExpander(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("expand directory %q: %w", dir, err)
	}
	return e.Expand(dir)
}

// IsRepositoryContent reports whether path — absolute or repository-relative —
// is a repository-content file under the package's Git definition: Git tracks
// it, or it is untracked and not ignored. A path outside repoRoot is never
// repository content. The check keeps scratch inputs (for example a gate-plan
// input manifest) out of the evidence surface.
func IsRepositoryContent(repoRoot, path string) (bool, error) {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, abs)
	}
	abs = filepath.Clean(abs)
	// Resolve symlinks on both sides so a repository reached through a
	// symlinked path cannot hide a manifest that physically lives inside it.
	root := repoRoot
	if resolved, err := filepath.EvalSymlinks(repoRoot); err == nil {
		root = resolved
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		// Different volumes (Windows): the path cannot be relative to the
		// repository, so it is outside it.
		return false, nil
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return false, nil
	}
	out, err := runGit(repoRoot, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", rel)
	if err != nil {
		return false, err
	}
	for _, p := range strings.Split(out, "\x00") {
		if p == rel {
			return true, nil
		}
	}
	return false, nil
}

// Expander is one invocation's view of the repository's directory content.
// While a read-only invocation runs, repository content is fixed, so a
// directory's expansion is a pure function of the repository and the
// directory's canonical path: the expander resolves the git worktree once
// and expands each distinct directory once, no matter how many declarations
// name it.
type Expander struct {
	repoRoot   string
	cache      map[string]expansion
	expansions int
}

type expansion struct {
	files []File
	err   error
}

// NewExpander verifies that repoRoot is the git worktree top level once for
// the whole invocation. A root that is not the worktree top fails here
// instead of once per expansion.
func NewExpander(repoRoot string) (*Expander, error) {
	if err := RequireWorkTreeTop(repoRoot); err != nil {
		return nil, err
	}
	return &Expander{repoRoot: repoRoot, cache: map[string]expansion{}}, nil
}

// Expansions reports how many distinct directories were expanded through git,
// so the O(distinct directories) guarantee is assertable in tests.
func (e *Expander) Expansions() int {
	return e.expansions
}

// Expand lists every repository-content file under dir with the ExpandDir
// contract. The memo is keyed by the canonical path, so different spellings
// of one directory expand once. The returned slice is shared across the
// invocation's callers and must not be modified.
func (e *Expander) Expand(dir string) ([]File, error) {
	canonical := "."
	var err error
	if dir != "." {
		canonical, err = repopath.Canonical(e.repoRoot, dir)
		if err != nil {
			return nil, fmt.Errorf("expand directory %q: %w", dir, err)
		}
	}
	if previous, ok := e.cache[canonical]; ok {
		if previous.err != nil {
			return nil, fmt.Errorf("expand directory %q: %w", dir, previous.err)
		}
		return previous.files, nil
	}
	e.expansions++
	files, err := expandDir(e.repoRoot, canonical)
	e.cache[canonical] = expansion{files: files, err: err}
	if err != nil {
		return nil, fmt.Errorf("expand directory %q: %w", dir, err)
	}
	return files, nil
}

// expandDir runs the git expansion for an already-canonical directory: every
// repository-content file under it, hashed and sorted by path.
func expandDir(repoRoot, canonical string) ([]File, error) {
	out, err := runGit(repoRoot, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", canonical)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, p := range strings.Split(out, "\x00") {
		if p == "" {
			continue
		}
		rel, err := repopath.Canonical(repoRoot, p)
		if err != nil {
			return nil, err
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		hash, err := specpaths.FileHash(abs)
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: rel, Hash: hash})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// runGit runs one read-only git command in dir and returns stdout.
func runGit(dir string, args ...string) (string, error) {
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

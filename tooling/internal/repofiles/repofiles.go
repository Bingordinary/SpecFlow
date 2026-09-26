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
// The comparison resolves symlinks on both sides (macOS /var -> /private/var).
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
	if resolvedRoot != resolvedTop {
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
func ExpandDir(repoRoot, dir string) ([]File, error) {
	if err := RequireWorkTreeTop(repoRoot); err != nil {
		return nil, fmt.Errorf("expand directory %q: %w", dir, err)
	}
	canonical, err := repopath.Canonical(repoRoot, dir)
	if err != nil {
		return nil, fmt.Errorf("expand directory %q: %w", dir, err)
	}
	out, err := runGit(repoRoot, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", canonical)
	if err != nil {
		return nil, fmt.Errorf("expand directory %q: %w", dir, err)
	}
	var files []File
	for _, p := range strings.Split(out, "\x00") {
		if p == "" {
			continue
		}
		rel, err := repopath.Canonical(repoRoot, p)
		if err != nil {
			return nil, fmt.Errorf("expand directory %q: %w", dir, err)
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("expand directory %q: %w", dir, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		hash, err := specpaths.FileHash(abs)
		if err != nil {
			return nil, fmt.Errorf("expand directory %q: %w", dir, err)
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

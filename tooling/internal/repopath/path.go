// Package repopath canonicalizes repository-owned paths and rejects lexical
// or symlink-based escapes from the repository root.
package repopath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Canonical returns the canonical lexical repository-relative spelling of p
// after proving that its real filesystem location stays inside repoRoot.
// Missing targets are allowed: the nearest existing ancestor is resolved so
// an escaping parent symlink still fails closed.
func Canonical(repoRoot, p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`)
	if p == "" {
		return "", fmt.Errorf("path is empty")
	}

	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	targetAbs := p
	if !filepath.IsAbs(targetAbs) {
		targetAbs = filepath.Join(rootAbs, filepath.FromSlash(p))
	}
	targetAbs = filepath.Clean(targetAbs)
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || rel == "." || escapesRoot(rel) {
		return "", fmt.Errorf("path %q is outside the repository root", p)
	}

	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolvedTarget, err := ResolveExistingAncestor(targetAbs)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", p, err)
	}
	resolvedRel, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil || resolvedRel == "." || escapesRoot(resolvedRel) {
		return "", fmt.Errorf("path %q resolves outside the repository root", p)
	}
	return filepath.ToSlash(rel), nil
}

// ResolveExistingAncestor resolves the nearest existing ancestor of abs and
// reattaches the missing suffix, proving the parent chain is resolvable. An
// ENOENT on a component that is itself a symlink means the link cannot be
// resolved (broken link or missing symlink target) — treating it as a plain
// missing suffix would reattach the rest of the path lexically and accept a
// declaration that escapes the repository once the target appears, so it
// fails closed. This is the shared implementation for every repository-owned
// path boundary (see tooling/README.md §Operation scope and
// framework/operations/operation_scope.md).
func ResolveExistingAncestor(abs string) (string, error) {
	probe := abs
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		// An ENOENT on a path component that is itself a symlink means the
		// link cannot be resolved (broken link or a missing symlink target).
		// Treating it as a plain missing suffix would reattach the rest of
		// the path lexically and accept a declaration that escapes the
		// repository once the target appears — fail closed instead.
		if info, lerr := os.Lstat(probe); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path component %q is an unresolvable symlink", probe)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", err
		}
		suffix = append(suffix, filepath.Base(probe))
		probe = parent
	}
}

func escapesRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

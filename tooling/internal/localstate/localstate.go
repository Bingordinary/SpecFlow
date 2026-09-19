// Package localstate owns the identity and path boundary shared by local
// governance state carriers. A state id is data, never a path fragment: every
// caller validates the generated id form and every filesystem target is
// proven to remain below its declared state root before use.
package localstate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
)

var idPattern = regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{6}$`)

// ValidateID accepts only the form emitted by the gate-run and operation id
// generators. In particular, path separators and traversal components are
// not part of the id space.
func ValidateID(id string) error {
	id = strings.TrimSpace(id)
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid local-state id %q: expected YYYYMMDD-HHMMSS-<6 lowercase hex>", id)
	}
	return nil
}

// BindID proves that a loaded state's embedded identity is the identity the
// caller requested. A state file cannot redirect later writes or cleanup by
// carrying a different id in its JSON payload.
func BindID(requested, embedded string) error {
	requested = strings.TrimSpace(requested)
	embedded = strings.TrimSpace(embedded)
	if err := ValidateID(requested); err != nil {
		return err
	}
	if err := ValidateID(embedded); err != nil {
		return fmt.Errorf("embedded id: %w", err)
	}
	if embedded != requested {
		return fmt.Errorf("embedded id %q does not match requested id %q", embedded, requested)
	}
	return nil
}

// Path joins elems below relStateRoot and proves both the lexical path and
// the symlink-resolved path remain inside the repository and state root. It
// supports not-yet-existing targets by resolving the nearest existing
// ancestor and reattaching the missing suffix.
func Path(repoRoot, relStateRoot string, elems ...string) (string, error) {
	repoAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	repoAbs = filepath.Clean(repoAbs)
	stateAbs := filepath.Join(repoAbs, filepath.FromSlash(relStateRoot))
	targetAbs := filepath.Join(append([]string{stateAbs}, elems...)...)

	if !inside(repoAbs, stateAbs) {
		return "", fmt.Errorf("state root %q is outside repository root", relStateRoot)
	}
	if !inside(stateAbs, targetAbs) || filepath.Clean(targetAbs) == filepath.Clean(stateAbs) {
		return "", fmt.Errorf("local-state path escapes %s", relStateRoot)
	}

	resolvedRepo, err := filepath.EvalSymlinks(repoAbs)
	if err != nil {
		return "", fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolvedState, err := repopath.ResolveExistingAncestor(stateAbs)
	if err != nil {
		return "", fmt.Errorf("resolve state root %s: %w", relStateRoot, err)
	}
	if !inside(resolvedRepo, resolvedState) {
		return "", fmt.Errorf("state root %q resolves outside repository root", relStateRoot)
	}
	resolvedTarget, err := repopath.ResolveExistingAncestor(targetAbs)
	if err != nil {
		return "", fmt.Errorf("resolve local-state target: %w", err)
	}
	if !inside(resolvedState, resolvedTarget) || filepath.Clean(resolvedTarget) == filepath.Clean(resolvedState) {
		return "", fmt.Errorf("local-state target resolves outside %s", relStateRoot)
	}
	return filepath.Clean(targetAbs), nil
}

func inside(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

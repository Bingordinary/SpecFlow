// Package writezone implements the governed write-zone classification: given
// a path, report whether ordinary governance may write it. The zone table in
// tooling/README.md §Governed write zones is the authoritative contract; this
// package implements it verbatim.
package writezone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
)

// Result is the outcome of classifying one path against the write zones.
type Result struct {
	Allowed bool
	Reason  string
	Path    string
}

// Classifier applies the governed write zones for one resolved repository
// layout. Layout resolution happens once so every path in one command uses the
// same framework root.
type Classifier struct {
	repoRoot      string
	frameworkRoot string
}

// New resolves the repository layout and constructs its write-zone classifier.
// Missing or ambiguous layouts are errors: write governance must not guess a
// framework root and fall through to the default allowed branch.
func New(repoRoot string) (*Classifier, error) {
	layout, err := specflowlayout.Resolve(repoRoot)
	if err != nil {
		return nil, err
	}
	return &Classifier{
		repoRoot:      repoRoot,
		frameworkRoot: layout.FrameworkRoot,
	}, nil
}

// Classify matches both the lexical and symlink-resolved identities of a path
// against the governed write zones. A denial for either identity is final: a
// symlink cannot hide a protected source or target behind an allowed prefix.
// A path whose symlink identity cannot be resolved (a broken or dangling link
// on the path) is denied rather than falling back to the lexical identity
// alone, because a later write through the link would land in the unresolved
// location.
func (c *Classifier) Classify(path string) Result {
	original := path
	target := path
	if !filepath.IsAbs(target) {
		if cwd, err := os.Getwd(); err == nil {
			target = filepath.Join(cwd, target)
		}
	}
	target = filepath.Clean(target)
	repoRoot, err := filepath.Abs(c.repoRoot)
	if err != nil {
		repoRoot = filepath.Clean(c.repoRoot)
	}

	var identities []string
	if rel, ok := relativeInside(repoRoot, target); ok {
		identities = append(identities, rel)
	}

	resolvedRoot := repoRoot
	if r, err := filepath.EvalSymlinks(repoRoot); err == nil {
		resolvedRoot = r
	}
	resolvedTarget, err := repopath.ResolveExistingAncestor(target)
	if err != nil {
		// The resolved identity cannot be established (a broken/dangling
		// symlink on the path, or an unreadable component). The lexical
		// identity alone is not trustworthy: a write through the link lands at
		// the unresolved target, so fail closed instead of matching only the
		// lexical prefix.
		return Result{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q has an unresolvable symlink identity: %v", original, err),
			Path:    original,
		}
	}
	if rel, ok := relativeInside(resolvedRoot, resolvedTarget); ok && !containsString(identities, rel) {
		identities = append(identities, rel)
	}

	var allowed *Result
	for _, identity := range identities {
		result := c.classifyIdentity(identity, target)
		if !result.Allowed {
			return result
		}
		if allowed == nil || !strings.Contains(result.Reason, "not governed") {
			copy := result
			allowed = &copy
		}
	}
	if allowed != nil {
		return *allowed
	}
	return Result{
		Allowed: true,
		Reason:  fmt.Sprintf("path %q is not governed by specFlow write restrictions", original),
		Path:    original,
	}
}

func (c *Classifier) classifyIdentity(normalizedPath, displayPath string) Result {
	// Deny pattern: the resolved layout's framework files are never writable.
	if inZone(normalizedPath, c.frameworkRoot) {
		return Result{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q has repository identity %q under %s/ and is not writable", displayPath, normalizedPath, c.frameworkRoot),
			Path:    displayPath,
		}
	}

	// Deny pattern: stable spec files are not directly writable (use promote)
	if inZone(normalizedPath, "docs/specs/units/stable") {
		return Result{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q has repository identity %q under docs/specs/units/stable/; use promote to write stable specs", displayPath, normalizedPath),
			Path:    displayPath,
		}
	}

	// Deny pattern: rule files are not directly writable (use rule governance flows)
	if inZone(normalizedPath, "docs/specs/rules/stable") {
		return Result{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q has repository identity %q under docs/specs/rules/stable/; use rule governance flows", displayPath, normalizedPath),
			Path:    displayPath,
		}
	}

	// Candidate spec files are writable
	if strings.HasPrefix(normalizedPath, "docs/specs/units/candidate/") {
		return Result{
			Allowed: true,
			Reason:  fmt.Sprintf("path %q is a candidate spec file and is writable", displayPath),
			Path:    displayPath,
		}
	}

	// Candidate rule files are writable
	if strings.HasPrefix(normalizedPath, "docs/specs/rules/candidate/") {
		return Result{
			Allowed: true,
			Reason:  fmt.Sprintf("path %q is a candidate rule file and is writable", displayPath),
			Path:    displayPath,
		}
	}

	// Source code files are writable by default
	return Result{
		Allowed: true,
		Reason:  fmt.Sprintf("path %q is not governed by specFlow write restrictions", displayPath),
		Path:    displayPath,
	}
}

func relativeInside(root, target string) (string, bool) {
	rel, err := filepath.Rel(root, target)
	if err != nil || escapesRoot(rel) {
		return "", false
	}
	return filepath.ToSlash(filepath.Clean(rel)), true
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// escapesRoot reports whether a repository-relative path escapes the
// repository root (e.g. ".." or "../other"). Paths that escape the root
// are outside the specFlow write-zone contract and must keep the default
// "not governed" result instead of being matched against write-zone prefixes.
func escapesRoot(rel string) bool {
	rel = filepath.Clean(rel)
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// inZone reports whether a normalized repo-relative path is the zone root
// itself or lies under it. The root spelling (no trailing slash) names the
// directory and must be denied exactly like its contents — otherwise a
// declaration like `framework` (the zone root without a trailing slash)
// would slip past the prefix match.
func inZone(path, zoneRoot string) bool {
	return path == zoneRoot || strings.HasPrefix(path, zoneRoot+"/")
}

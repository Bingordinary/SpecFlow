package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
)

// SurfacePending is the exact implementation_surface placeholder for
// design-first rounds: the item's implementation path is not yet known.
const SurfacePending = "<pending>"

// SurfaceProblem is one acceptance item whose implementation_surface cannot
// produce a code surface: the item id, the declared value (empty when the
// field has no value), and an actionable reason.
type SurfaceProblem struct {
	ItemID string
	Value  string
	Reason string
}

func (p SurfaceProblem) String() string {
	if p.Value == "" {
		return fmt.Sprintf("item %s: implementation_surface is empty — %s", p.ItemID, p.Reason)
	}
	return fmt.Sprintf("item %s: implementation_surface %q — %s", p.ItemID, p.Value, p.Reason)
}

// FormatSurfaceProblems renders problems as one semicolon-separated line for
// gate errors and validate details.
func FormatSurfaceProblems(problems []SurfaceProblem) string {
	parts := make([]string, 0, len(problems))
	for _, p := range problems {
		parts = append(parts, p.String())
	}
	return strings.Join(parts, "; ")
}

// CheckImplementationSurfaces validates every structurally located acceptance
// item's implementation_surface against the working tree. The exact
// <pending> placeholder is the legal design-first value and is skipped; every
// other item must declare a single repository-relative path that resolves, as
// written, to at least one real file (a directory expands to the files Git
// tracks or leaves untracked and unignored, so a directory holding only
// ignored files is not a usable surface). Resolution is the only judgment:
// the value is matched literally, so an existing path is accepted whatever
// characters it contains, and a value that does not resolve — a semicolon
// list or wildcard pattern is not a path — is reported with the item id and
// the mechanical reason. An empty result means every declared surface is
// usable. Mechanical validate Check 3 and gate-plan share this check, so a
// declared surface that cannot expand to a code file never silently produces
// an empty file set.
func CheckImplementationSurfaces(repoRoot, specContent string) []SurfaceProblem {
	var problems []SurfaceProblem
	for _, item := range parseAcceptanceItems(specContent) {
		value := strings.TrimSpace(item.implementationSurface)
		if value == SurfacePending {
			continue
		}
		if value == "" {
			problems = append(problems, SurfaceProblem{
				ItemID: item.id,
				Reason: "use the <pending> placeholder during design-first rounds; a missing or empty value cannot derive a code surface",
			})
			continue
		}
		if reason := surfaceValueProblem(repoRoot, value); reason != "" {
			problems = append(problems, SurfaceProblem{ItemID: item.id, Value: value, Reason: reason})
		}
	}
	return problems
}

// surfaceValueProblem returns "" when value is a single repository-contained
// path that resolves to at least one real file, or the mechanical reason it
// does not. The value is matched literally and is never classified by shape:
// a list or pattern is rejected because it is not a resolvable path, not
// because of its characters.
func surfaceValueProblem(repoRoot, value string) string {
	canonical, err := repopath.Canonical(repoRoot, value)
	if err != nil {
		return fmt.Sprintf("cannot be resolved: %v", err)
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(canonical))
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "path does not exist — implementation_surface must be a single repository-relative file or directory (framework/spec_writing_guide.md §7)"
		}
		return fmt.Sprintf("cannot be inspected: %v", err)
	}
	if !info.IsDir() {
		return ""
	}
	files, err := repofiles.ExpandDir(repoRoot, canonical)
	if err != nil {
		return fmt.Sprintf("cannot be expanded: %v", err)
	}
	if len(files) == 0 {
		return "directory contains no files — a directory surface expands to the files Git tracks or leaves untracked and unignored"
	}
	return ""
}

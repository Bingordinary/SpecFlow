// Package specvalidation validates candidate spec structure.
// It implements checks derived from the unit_check lifecycle:
//
//  1. Frontmatter completeness
//  2. Acceptance items format
//  3. Anchor integrity (affects.files paths exist)
//  4. Reference integrity (unit_refs/rule_refs files exist)
//  5. Appendix files exist
//  6. Version/ref consistency
//  7. Body layer-path check (candidate-layer spec paths)
//  8. Dependency cycle check (unit_refs graph has no cycles through the unit)
//  9. Region locatability (section regions are splittable and locatable)
package specvalidation

import (
	"fmt"
	"strings"
)

// CheckResult describes one check outcome.
type CheckResult struct {
	Name    string       // check name
	Status  CheckStatus  // pass or fail
	Details string       // human-readable diagnostic
}

// CheckStatus is the outcome of a single check.
type CheckStatus int

const (
	Pass CheckStatus = iota
	Fail
)

func (s CheckStatus) String() string {
	switch s {
	case Pass:
		return "PASS"
	case Fail:
		return "FAIL"
	default:
		return "UNKNOWN"
	}
}

// Result holds all check results for a candidate validation.
type Result struct {
	Unit       string
	Passed     bool
	Checks     []CheckResult
}

// ValidateCandidate runs all 9 checks on the given unit's candidate spec.
func ValidateCandidate(repoRoot, unitName string) *Result {
	r := &Result{Unit: unitName}

	r.Checks = append(r.Checks, checkFrontmatter(repoRoot, unitName))
	r.Checks = append(r.Checks, checkAcceptanceItems(repoRoot, unitName))
	r.Checks = append(r.Checks, checkAnchors(repoRoot, unitName))
	r.Checks = append(r.Checks, checkReferences(repoRoot, unitName))
	r.Checks = append(r.Checks, checkAppendices(repoRoot, unitName))
	r.Checks = append(r.Checks, checkVersionConsistency(repoRoot, unitName))
	r.Checks = append(r.Checks, checkLayerPaths(repoRoot, unitName))
	r.Checks = append(r.Checks, checkDependencyCycles(repoRoot, unitName))
	r.Checks = append(r.Checks, checkRegionLocatability(repoRoot, unitName))

	r.Passed = true
	for _, c := range r.Checks {
		if c.Status == Fail {
			r.Passed = false
			break
		}
	}
	return r
}

// FormatResult formats the validation result as readable output.
func FormatResult(r *Result) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Unit: %s\n", r.Unit)
	fmt.Fprintf(&buf, "Validate result: ")
	if r.Passed {
		buf.WriteString("PASS\n")
	} else {
		buf.WriteString("FAIL\n")
	}
	fmt.Fprintf(&buf, "Failed checks: %d\n\n", countFailed(r.Checks))

	for _, c := range r.Checks {
		fmt.Fprintf(&buf, "%d. %s: %s", indexOf(c, r.Checks)+1, c.Name, c.Status)
		if c.Details != "" {
			fmt.Fprintf(&buf, " — %s", c.Details)
		}
		buf.WriteString("\n")
	}

	if !r.Passed {
		buf.WriteString("\nFix the issues above and re-run validate.\n")
	}
	return buf.String()
}

func countFailed(checks []CheckResult) int {
	count := 0
	for _, c := range checks {
		if c.Status == Fail {
			count++
		}
	}
	return count
}

func indexOf(c CheckResult, checks []CheckResult) int {
	for i, ch := range checks {
		if ch.Name == c.Name {
			return i
		}
	}
	return -1
}

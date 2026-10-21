// Package specvalidation validates candidate spec structure.
// It implements checks derived from the unit_check lifecycle:
//
//  1. Frontmatter completeness
//  2. Acceptance items format (required fields, Gherkin-style descriptions for testable items, .feature syntax rejected)
//  3. Anchor integrity (affects.files and affects.evidence_files paths exist; implementation_surface values resolve to real code; implementation_surface and affects.files reject a spec-document value)
//  4. Reference integrity (rule_refs files exist)
//  5. Appendix files exist
//  6. Body layer-path check (candidate-layer spec paths)
//  7. Region locatability (section and acceptance item regions are splittable and locatable)
//  8. Surface associations (declarations resolve; file sharing is allowed)
//  9. Prose path hygiene (source paths in narrative text — WARNING, advisory)
//  10. Environment agnosticism (dev-machine paths, local addresses, credentials)
//  11. Reserved content headings (changelog/revision sections — guide §15.1)
//  12. Process traces (change-history narration — WARNING, guide §15.1)
//  13. Test doubles (test-double names in the spec — WARNING, guide §15.3)
//  14. verification_method shape (one-sentence method — guide §15.3)
//  15. Promise model (three-section shape, promise schema, coverage, and promise graph)
package specvalidation

import (
	"fmt"
	"os"
	"strings"
)

// CheckResult describes one check outcome.
type CheckResult struct {
	Name    string      // check name
	Status  CheckStatus // pass, fail, or warning (advisory — never blocks)
	Details string      // human-readable diagnostic
}

// CheckStatus is the outcome of a single check.
type CheckStatus int

const (
	Pass CheckStatus = iota
	Fail
	Warn
)

func (s CheckStatus) String() string {
	switch s {
	case Pass:
		return "PASS"
	case Fail:
		return "FAIL"
	case Warn:
		return "WARNING"
	default:
		return "UNKNOWN"
	}
}

// Result holds all check results for a candidate validation.
type Result struct {
	Unit   string
	Passed bool
	Checks []CheckResult
}

// ValidateCandidate runs all 15 checks on the given unit's candidate spec.
func ValidateCandidate(repoRoot, unitName string) *Result {
	r := &Result{Unit: unitName}

	r.Checks = append(r.Checks, checkFrontmatter(repoRoot, unitName))
	r.Checks = append(r.Checks, checkAcceptanceItems(repoRoot, unitName))
	r.Checks = append(r.Checks, checkAnchors(repoRoot, unitName))
	r.Checks = append(r.Checks, checkReferences(repoRoot, unitName))
	r.Checks = append(r.Checks, checkAppendices(repoRoot, unitName))
	r.Checks = append(r.Checks, checkLayerPaths(repoRoot, unitName))
	r.Checks = append(r.Checks, checkRegionLocatability(repoRoot, unitName))
	r.Checks = append(r.Checks, checkSurfaceAssociations(repoRoot, unitName))
	r.Checks = append(r.Checks, checkProseHygiene(repoRoot, unitName))
	r.Checks = append(r.Checks, checkEnvironmentAgnosticism(repoRoot, unitName))
	r.Checks = append(r.Checks, checkReservedContentHeadings(repoRoot, unitName))
	r.Checks = append(r.Checks, checkProcessTraces(repoRoot, unitName))
	r.Checks = append(r.Checks, checkTestDoubles(repoRoot, unitName))
	r.Checks = append(r.Checks, checkVerificationMethodShape(repoRoot, unitName))
	r.Checks = append(r.Checks, checkPromiseModel(repoRoot, unitName))

	r.Passed = true
	for _, c := range r.Checks {
		if c.Status == Fail {
			r.Passed = false
			break
		}
	}
	return r
}

// checkPromiseModel runs the promise-model checks. A unit without the
// `## Promises` section does not follow the deployed three-section unit shape
// and fails this check.
func checkPromiseModel(repoRoot, unitName string) CheckResult {
	data, err := os.ReadFile(specPath(repoRoot, unitName))
	if err != nil {
		return CheckResult{Name: "Promise model", Status: Fail, Details: fmt.Sprintf("cannot read candidate spec: %v", err)}
	}
	content := string(data)
	if !HasPromisesSection(content) {
		return CheckResult{
			Name:    "Promise model",
			Status:  Fail,
			Details: "no `## Promises` section: a unit spec follows the three-section promise model (see framework/core/promise_model.md)",
		}
	}
	var issues []string
	for _, check := range CheckPromiseSpec(content, unitName) {
		if check.Status == Fail {
			issues = append(issues, check.Name+": "+check.Details)
		}
	}
	if graph := CheckPromiseGraph(repoRoot, unitName); graph.Status == Fail {
		issues = append(issues, graph.Name+": "+graph.Details)
	}
	if len(issues) > 0 {
		return CheckResult{Name: "Promise model", Status: Fail, Details: strings.Join(issues, "; ")}
	}
	return CheckResult{Name: "Promise model", Status: Pass, Details: "shape, schema, coverage, declarations, and graph pass"}
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

package rulevalidation

import (
	"fmt"
	"strings"
)

type CheckResult struct {
	Name    string
	Status  CheckStatus
	Details string
}

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
		return "WARN"
	default:
		return "UNKNOWN"
	}
}

type RuleResult struct {
	RuleID string
	Passed bool
	Checks []CheckResult
}

func ValidateRule(repoRoot, ruleID string) *RuleResult {
	r := &RuleResult{RuleID: ruleID}

	r.Checks = append(r.Checks, checkFrontmatter(repoRoot, ruleID))
	r.Checks = append(r.Checks, checkIDScopeConsistency(repoRoot, ruleID))
	r.Checks = append(r.Checks, checkFilePath(repoRoot, ruleID))
	r.Checks = append(r.Checks, checkVersionSemantics(repoRoot, ruleID))
	r.Checks = append(r.Checks, checkPromotionOwner(repoRoot, ruleID))
	r.Checks = append(r.Checks, checkProhibitedFields(repoRoot, ruleID))
	r.Checks = append(r.Checks, checkUnboundRetention(repoRoot, ruleID))

	r.Passed = true
	for _, c := range r.Checks {
		if c.Status == Fail {
			r.Passed = false
			break
		}
	}
	return r
}

func FormatResult(r *RuleResult) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Rule: %s\n", r.RuleID)
	fmt.Fprintf(&buf, "Validate result: ")
	if r.Passed {
		buf.WriteString("PASS\n")
	} else {
		buf.WriteString("FAIL\n")
	}
	fmt.Fprintf(&buf, "Failed checks: %d\n\n", countFailed(r.Checks))

	for _, c := range r.Checks {
		fmt.Fprintf(&buf, "%s: %s", c.Name, c.Status)
		if c.Details != "" {
			fmt.Fprintf(&buf, " — %s", c.Details)
		}
		buf.WriteString("\n")
	}

	if !r.Passed {
		buf.WriteString("\nFix the issues above and re-run validate.\n")
	} else {
		buf.WriteString("\nGo-level checks passed. Agent should complete Check 8 (Rule Body Quality) before writing cache.\n")
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

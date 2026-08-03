package rulevalidation

import (
	"strings"
	"testing"
)

func TestFormatResult_FailedChecksCount(t *testing.T) {
	result := &RuleResult{
		RuleID: "test_rule",
		Passed: false,
		Checks: []CheckResult{
			{Name: "frontmatter", Status: Fail, Details: "missing rule_id"},
			{Name: "id/scope consistency", Status: Pass},
			{Name: "file path", Status: Warn, Details: "unconventional path"},
		},
	}

	output := FormatResult(result)
	if !strings.Contains(output, "Failed checks: 1") {
		t.Fatalf("expected \"Failed checks: 1\" in output, got:\n%s", output)
	}
}

func TestFormatResult_PassFailedChecksZero(t *testing.T) {
	result := &RuleResult{
		RuleID: "test_rule",
		Passed: true,
		Checks: []CheckResult{
			{Name: "frontmatter", Status: Pass},
			{Name: "id/scope consistency", Status: Pass},
		},
	}

	output := FormatResult(result)
	if !strings.Contains(output, "Failed checks: 0") {
		t.Fatalf("expected \"Failed checks: 0\" in output, got:\n%s", output)
	}
}

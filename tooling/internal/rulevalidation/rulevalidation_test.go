package rulevalidation

import (
	"os"
	"path/filepath"
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

func TestUnboundRuleNeedsNoRetentionMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs/specs/rules/candidate/b_rule_future.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nrule_id: b_rule_future\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Future pipeline constraint\n\nKeep this rule for the next pipeline.\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	result := ValidateRule(root, "b_rule_future")
	if !result.Passed || len(result.Checks) != 5 {
		t.Fatalf("unbound rule rejected: %+v", result)
	}
}

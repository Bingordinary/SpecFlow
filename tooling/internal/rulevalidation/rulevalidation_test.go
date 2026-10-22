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
	content := "---\nrule_id: b_rule_future\nrule_scope: bound\n---\n\n# Future pipeline constraint\n\nKeep this rule for the next pipeline.\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	result := ValidateRule(root, "b_rule_future")
	if !result.Passed || len(result.Checks) != 5 {
		t.Fatalf("unbound rule rejected: %+v", result)
	}
}

func TestFilenameConsistencyRejectsRuleIDDisagreement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs/specs/rules/candidate/b_rule_auth.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	// The filename says b_rule_auth; the frontmatter says g_rule_auth with a
	// scope that matches the filename prefix, so the scope-vs-prefix check
	// alone cannot catch the disagreement.
	content := "---\nrule_id: g_rule_auth\nrule_scope: bound\n---\n\n# Auth constraint\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	result := ValidateRule(root, "b_rule_auth")
	if result.Passed {
		t.Fatalf("filename/rule_id disagreement must fail validation: %+v", result)
	}
	var check *CheckResult
	for i := range result.Checks {
		if result.Checks[i].Name == "File path consistency" {
			check = &result.Checks[i]
		}
	}
	if check == nil {
		t.Fatalf("File path consistency check missing: %+v", result)
	}
	if check.Status != Fail || !strings.Contains(check.Details, "rule_id \"g_rule_auth\"") {
		t.Fatalf("unexpected File path consistency result: %+v", *check)
	}
}

func TestFilenameConsistencyReportsMissingCanonicalFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs/specs/rules/candidate"), 0755); err != nil {
		t.Fatal(err)
	}
	result := ValidateRule(root, "b_rule_absent")
	if result.Passed {
		t.Fatalf("missing canonical file must fail validation: %+v", result)
	}
	found := false
	for _, c := range result.Checks {
		if c.Name == "File path consistency" && c.Status == Fail && strings.Contains(c.Details, "canonical path") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a failing File path consistency check: %+v", result)
	}
}

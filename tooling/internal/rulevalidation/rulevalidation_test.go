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

func TestCheckVersionSemantics_RetiredRuleSkipsVersionComparison(t *testing.T) {
	repoRoot := t.TempDir()

	// Stable rule already exists with the same version the candidate carries —
	// normally a violation, but a retired rule is removed, not versioned.
	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	stable := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: stable\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(stableDir, "b_rule_test.md"), []byte(stable), 0644); err != nil {
		t.Fatal(err)
	}

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\nstatus: retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkVersionSemantics(repoRoot, "b_rule_test")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired rule, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckVersionSemantics_NonLiteralRetiredStatusFailsVersionGate(t *testing.T) {
	repoRoot := t.TempDir()

	// Only the literal `status: retired` triggers the retirement path. A
	// case-variant value must keep the normal version gate — the candidate
	// version equals the stable version, so the check must FAIL.
	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	stable := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: stable\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(stableDir, "b_rule_test.md"), []byte(stable), 0644); err != nil {
		t.Fatal(err)
	}

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\nstatus: Retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkVersionSemantics(repoRoot, "b_rule_test")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for non-literal retired status, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_RetiredBoundRuleNoConsumersPass(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	// A retiring bound rule with no remaining consumers must not be forced to
	// declare unbound_retention fields — the rule is going away.
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\nstatus: retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "b_rule_test")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired rule without consumers, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_RetiredBoundRuleWithConsumerFail(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\nstatus: retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}
	// A current-layer unit still lists the rule in rule_refs.
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - b_rule_test\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "b_rule_test")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for retired rule with a consumer, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_RetiredGlobalRuleExplicitConsumerFail(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: g_rule_http\nrule_scope: global\nlayer: candidate\nrule_version: 0.1.0\nstatus: retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "g_rule_http.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}
	// A unit explicitly lists the retiring global rule in rule_refs.
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "g_rule_http")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for retired global rule with explicit consumer, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_RetiredGlobalRuleDefaultApplicabilityPass(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: g_rule_http\nrule_scope: global\nlayer: candidate\nrule_version: 0.1.0\nstatus: retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "g_rule_http.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}
	// Other units exist but none explicitly references the rule — a global
	// rule's default applicability lifts automatically when its stable file
	// disappears, so this retirement is legal.
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "g_rule_http")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired global rule without explicit references, got %s: %s", result.Status, result.Details)
	}
}

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

func TestCheckVersionSemantics_StatusRetiredStillEnforcesVersionGate(t *testing.T) {
	repoRoot := t.TempDir()

	// The `status: retired` declaration no longer skips the version gate —
	// retirement ceremonies were removed; the candidate is versioned like any
	// other rule. Same version as stable is a violation.
	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	stable := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(stableDir, "b_rule_test.md"), []byte(stable), 0644); err != nil {
		t.Fatal(err)
	}

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\nstatus: retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkVersionSemantics(repoRoot, "b_rule_test")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for same-version candidate, got %s: %s", result.Status, result.Details)
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
	stable := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(stableDir, "b_rule_test.md"), []byte(stable), 0644); err != nil {
		t.Fatal(err)
	}

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\nstatus: Retired\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkVersionSemantics(repoRoot, "b_rule_test")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for non-literal retired status, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_BoundRuleNoConsumersRequiresFields(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	// A bound rule with no current-layer consumers must declare the
	// unbound_retention fields — the retention declaration is what exempts it
	// from `specflowctl remove`.
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "b_rule_test")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for unbound rule without retention fields, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_BoundRuleWithConsumerPassesWithoutFields(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}
	// A current-layer unit lists the rule in rule_refs — the rule is bound,
	// so the retention fields must not be present (they are not).
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - b_rule_test\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "b_rule_test")
	if result.Status != Pass {
		t.Fatalf("expected PASS for bound rule with a consumer and no retention fields, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_BoundRuleWithConsumerAndFieldsFail(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\nunbound_retention: intentional\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - b_rule_test\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "b_rule_test")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for bound rule with consumer and retention fields, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnboundRetention_EffectiveSemanticsIgnoresStaleStableRef(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}
	// The divergence scenario: the candidate unit dropped the reference while
	// its stable predecessor still lists it. Consumer discovery resolves the
	// current-layer (effective) file — the stale stable reference no longer
	// counts, so the rule is unbound and must declare retention.
	candUnitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	dropped := "---\nid: consumer\nversion: 0.2.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(filepath.Join(candUnitDir, "unit_consumer.md"), []byte(dropped), 0644); err != nil {
		t.Fatal(err)
	}
	stableUnitDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	stale := "---\nid: consumer\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - b_rule_test\n---\n"
	if err := os.WriteFile(filepath.Join(stableUnitDir, "unit_consumer.md"), []byte(stale), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "b_rule_test")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for unbound rule without retention fields, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "requires fields") {
		t.Fatalf("expected retention-fields requirement detail, got: %s", result.Details)
	}
}

func TestCheckUnboundRetention_GlobalRuleSkipped(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	// The retention model applies to bound rules only — a global rule is
	// never judged here, with or without explicit references.
	candidate := "---\nrule_id: g_rule_http\nrule_scope: global\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(candDir, "g_rule_http.md"), []byte(candidate), 0644); err != nil {
		t.Fatal(err)
	}
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	result := checkUnboundRetention(repoRoot, "g_rule_http")
	if result.Status != Pass {
		t.Fatalf("expected PASS for global rule (skipped), got %s: %s", result.Status, result.Details)
	}
}

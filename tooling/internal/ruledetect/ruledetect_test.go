package ruledetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRule(t *testing.T, repoRoot, layer, ruleID, content string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/rules", layer)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ruleID+".md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeUnit(t *testing.T, repoRoot, layer, unitName, ruleRefs string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + unitName + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: " + ruleRefs + "\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unitName+".md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func boundRule(version string) string {
	return "---\nrule_id: b_rule_x\nrule_scope: bound\nrule_version: " + version + "\n---\n\n# Rule\n"
}

func TestDetectRuleRemovable(t *testing.T) {
	repoRoot := t.TempDir()
	writeRule(t, repoRoot, "stable", "b_rule_x", boundRule("0.1.0"))
	writeRule(t, repoRoot, "candidate", "b_rule_x", boundRule("0.2.0"))

	result, err := DetectRule(repoRoot, "b_rule_x")
	if err != nil {
		t.Fatalf("DetectRule: %v", err)
	}
	if !result.HasCandidate || !result.HasStable {
		t.Fatalf("expected both layers present, got %+v", result)
	}
	if len(result.Consumers) != 0 {
		t.Fatalf("expected no consumers, got %v", result.Consumers)
	}
	if result.UnboundRetention {
		t.Fatal("expected no retention declaration")
	}
	if !result.Removable {
		t.Fatal("expected removable")
	}
}

func TestDetectRuleWithConsumerNotRemovable(t *testing.T) {
	repoRoot := t.TempDir()
	writeRule(t, repoRoot, "stable", "b_rule_x", boundRule("0.1.0"))
	writeUnit(t, repoRoot, "candidate", "consumer", "b_rule_x")

	result, err := DetectRule(repoRoot, "b_rule_x")
	if err != nil {
		t.Fatalf("DetectRule: %v", err)
	}
	if len(result.Consumers) != 1 || result.Consumers[0] != "consumer" {
		t.Fatalf("expected consumer unit, got %v", result.Consumers)
	}
	if result.Removable {
		t.Fatal("expected not removable with a consumer")
	}
}

func TestDetectRuleUnboundRetentionExempt(t *testing.T) {
	repoRoot := t.TempDir()
	rule := "---\nrule_id: b_rule_x\nrule_scope: bound\nrule_version: 0.1.0\nunbound_retention: intentional\nunbound_retention_reason: future reuse\nunbound_retention_owner: demo_flow\n---\n\n# Rule\n"
	writeRule(t, repoRoot, "stable", "b_rule_x", rule)

	result, err := DetectRule(repoRoot, "b_rule_x")
	if err != nil {
		t.Fatalf("DetectRule: %v", err)
	}
	if !result.UnboundRetention {
		t.Fatal("expected retention declaration detected")
	}
	if result.Removable {
		t.Fatal("expected not removable while retained")
	}
}

func TestDetectRuleDivergenceEffectiveSemantics(t *testing.T) {
	repoRoot := t.TempDir()
	writeRule(t, repoRoot, "stable", "b_rule_x", boundRule("0.1.0"))
	// The candidate unit dropped the reference; the stale stable predecessor
	// still lists it. Current-layer resolution must not count the stale file.
	writeUnit(t, repoRoot, "candidate", "consumer", "none")
	writeUnit(t, repoRoot, "stable", "consumer", "b_rule_x")

	result, err := DetectRule(repoRoot, "b_rule_x")
	if err != nil {
		t.Fatalf("DetectRule: %v", err)
	}
	if len(result.Consumers) != 0 {
		t.Fatalf("expected no effective consumers, got %v", result.Consumers)
	}
	if !result.Removable {
		t.Fatal("expected removable under effective semantics")
	}
}

func TestDetectRuleNotFound(t *testing.T) {
	repoRoot := t.TempDir()
	_, err := DetectRule(repoRoot, "b_rule_ghost")
	if err == nil {
		t.Fatal("expected error for missing rule")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

func TestDetectRuleGlobalRuleExplicitConsumersOnly(t *testing.T) {
	repoRoot := t.TempDir()
	writeRule(t, repoRoot, "stable", "g_rule_http", "---\nrule_id: g_rule_http\nrule_scope: global\nrule_version: 0.1.0\n---\n")

	// A global rule without explicit references is removable — the default
	// applicability lifts with the file.
	result, err := DetectRule(repoRoot, "g_rule_http")
	if err != nil {
		t.Fatalf("DetectRule: %v", err)
	}
	if len(result.Consumers) != 0 {
		t.Fatalf("expected no explicit consumers, got %v", result.Consumers)
	}
	if !result.Removable {
		t.Fatal("expected removable for global rule without explicit references")
	}

	// With an explicit reference the global rule must not be removable.
	writeUnit(t, repoRoot, "candidate", "consumer", "g_rule_http")
	result, err = DetectRule(repoRoot, "g_rule_http")
	if err != nil {
		t.Fatalf("DetectRule: %v", err)
	}
	if len(result.Consumers) != 1 {
		t.Fatalf("expected explicit consumer, got %v", result.Consumers)
	}
	if result.Removable {
		t.Fatal("expected not removable with an explicit consumer")
	}
}

func TestDetectAllOnlyBoundRules(t *testing.T) {
	repoRoot := t.TempDir()
	writeRule(t, repoRoot, "stable", "b_rule_keep", "---\nrule_id: b_rule_keep\nrule_scope: bound\nrule_version: 0.1.0\nunbound_retention: intentional\n---\n")
	writeRule(t, repoRoot, "stable", "b_rule_gone", boundRule("0.1.0"))
	writeRule(t, repoRoot, "candidate", "g_rule_http", "---\nrule_id: g_rule_http\nrule_scope: global\nrule_version: 0.1.0\n---\n")

	results, err := DetectAll(repoRoot)
	if err != nil {
		t.Fatalf("DetectAll: %v", err)
	}
	byID := map[string]DetectResult{}
	for _, r := range results {
		byID[r.RuleID] = r
	}
	if _, ok := byID["g_rule_http"]; ok {
		t.Fatal("global rule must not be listed")
	}
	if !byID["b_rule_gone"].Removable {
		t.Fatal("expected b_rule_gone removable")
	}
	if byID["b_rule_keep"].Removable {
		t.Fatal("expected b_rule_keep retained")
	}
}

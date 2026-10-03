package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func removeRun(t *testing.T, repoRoot string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	fullArgs := append(args, "--repo-root", repoRoot)
	err := runRemove(fullArgs, &stdout, &stderr)
	return stdout.String(), err
}

func writeBoundRule(t *testing.T, repoRoot, layer, ruleID string, extra string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/rules", layer)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nrule_id: " + ruleID + "\nrule_scope: bound\nrule_version: 0.1.0\n" + extra + "---\n\n# Rule\n"
	if err := os.WriteFile(filepath.Join(dir, ruleID+".md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeRuleMetadata(t *testing.T, repoRoot, ruleID string) {
	t.Helper()
	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/rule", ruleID+".yaml")
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(basePath, []byte("kind: rule\nname: "+ruleID+"\nsurfaces: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/rule", ruleID, "validate_result.md")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("---\ncommand: validate\nrule: "+ruleID+"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRuleSuccess(t *testing.T) {
	repoRoot := t.TempDir()
	writeBoundRule(t, repoRoot, "stable", "b_rule_x", "")
	writeBoundRule(t, repoRoot, "candidate", "b_rule_x", "")
	writeRuleMetadata(t, repoRoot, "b_rule_x")

	output, err := removeRun(t, repoRoot, "--rule", "b_rule_x")
	if err != nil {
		t.Fatalf("remove failed: %v\n%s", err, output)
	}
	for _, p := range []string{
		"docs/specs/rules/stable/b_rule_x.md",
		"docs/specs/rules/candidate/b_rule_x.md",
		"docs/specs/meta/baseline/rule/b_rule_x.yaml",
		"docs/specs/meta/validation/rule/b_rule_x/validate_result.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after remove", p)
		}
	}
	if !strings.Contains(output, "Removal complete") {
		t.Fatalf("expected success line, got:\n%s", output)
	}
}

func TestRemoveRuleWithConsumersRejected(t *testing.T) {
	repoRoot := t.TempDir()
	writeBoundRule(t, repoRoot, "stable", "b_rule_x", "")

	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nunit_refs: none\nrule_refs: b_rule_x\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := removeRun(t, repoRoot, "--rule", "b_rule_x")
	if err == nil {
		t.Fatal("expected removal rejection")
	}
	if !strings.Contains(output, "consumer") {
		t.Fatalf("expected the referrer to be listed, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_x.md")); err != nil {
		t.Fatalf("rule must survive the rejection: %v", err)
	}
}

func TestRemoveRuleMetadataOnlyCleanup(t *testing.T) {
	repoRoot := t.TempDir()
	// A known target can consist only of explicitly owned current records.
	writeRuleMetadata(t, repoRoot, "b_rule_x")

	output, err := removeRun(t, repoRoot, "--rule", "b_rule_x")
	if err != nil {
		t.Fatalf("remove failed: %v\n%s", err, output)
	}
	for _, p := range []string{
		"docs/specs/meta/baseline/rule/b_rule_x.yaml",
		"docs/specs/meta/validation/rule/b_rule_x/validate_result.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after cleanup", p)
		}
	}
	if !strings.Contains(output, "Removal complete") {
		t.Fatalf("expected completion report, got:\n%s", output)
	}
}

func TestRemoveRuleGlobalRuleSuccess(t *testing.T) {
	repoRoot := t.TempDir()
	dir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// A global rule is removable by explicit user invocation when no
	// current-layer unit explicitly references it — its default applicability
	// lifts automatically with the file.
	if err := os.WriteFile(filepath.Join(dir, "g_rule_http.md"), []byte("---\nrule_id: g_rule_http\nrule_scope: global\nrule_version: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := removeRun(t, repoRoot, "--rule", "g_rule_http")
	if err != nil {
		t.Fatalf("remove failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(dir, "g_rule_http.md")); !os.IsNotExist(err) {
		t.Fatalf("global rule must be removed: %v", err)
	}
}

func TestRemoveRuleGlobalRuleWithExplicitConsumerRejected(t *testing.T) {
	repoRoot := t.TempDir()
	dir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "g_rule_http.md"), []byte("---\nrule_id: g_rule_http\nrule_scope: global\nrule_version: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	unit := "---\nid: consumer\nunit_refs: none\nrule_refs: g_rule_http\n---\n"
	if err := os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := removeRun(t, repoRoot, "--rule", "g_rule_http")
	if err == nil {
		t.Fatal("expected rejection while an explicit consumer remains")
	}
	if _, err := os.Stat(filepath.Join(dir, "g_rule_http.md")); err != nil {
		t.Fatalf("rule must survive the rejection: %v", err)
	}
}

func TestRemoveRuleMissingFlag(t *testing.T) {
	repoRoot := t.TempDir()
	_, err := removeRun(t, repoRoot)
	if err == nil {
		t.Fatal("expected error for missing --rule")
	}
}

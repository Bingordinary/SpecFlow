package baseline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const unitSpec = `---
id: demo
layer: candidate
version: 0.1.0
unit_refs: none
rule_refs: none
---
acceptance_item_set:
  - id: demo.core
    description: Demo behavior.
    verification_type: auto
    verification_surface: internal_flow
    implementation_surface: internal/demo
    verification_method: check
    pass_condition: ok
    runnable: yes
    affects:
      files:
        - src/a.go
  - id: demo.aux
    description: Aux behavior.
    verification_type: auto
    verification_surface: internal_flow
    implementation_surface: internal/demo
    verification_method: check
    pass_condition: ok
    runnable: yes
`

func setupRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	files := map[string]string{
		"internal/demo/handler.go": "package demo\n",
		"internal/demo/util.go":    "package demo\n",
		"src/a.go":                 "package main\n",
	}
	for p, c := range files {
		full := filepath.Join(repoRoot, p)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return repoRoot
}

func writeUnitBaseline(t *testing.T, repoRoot string) {
	t.Helper()
	if err := WriteUnitBaseline(repoRoot, "demo", unitSpec); err != nil {
		t.Fatalf("WriteUnitBaseline: %v", err)
	}
}

func TestWriteUnitBaseline_RoundTrip(t *testing.T) {
	repoRoot := setupRepo(t)
	writeUnitBaseline(t, repoRoot)

	path := filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("baseline file not written: %v", err)
	}
	if _, err := readBaseline(path); err != nil {
		t.Fatalf("baseline cannot be read back: %v", err)
	}

	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusOK {
		t.Fatalf("expected OK for unchanged surface, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckUnitBaseline_FileChanged(t *testing.T) {
	repoRoot := setupRepo(t)
	writeUnitBaseline(t, repoRoot)

	full := filepath.Join(repoRoot, "internal/demo/handler.go")
	if err := os.WriteFile(full, []byte("package demo\n// changed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED, got %s", result.Status)
	}
	if !strings.Contains(result.Details, "handler.go") {
		t.Fatalf("expected handler.go in details, got: %s", result.Details)
	}
}

func TestCheckUnitBaseline_FileDeleted(t *testing.T) {
	repoRoot := setupRepo(t)
	writeUnitBaseline(t, repoRoot)

	full := filepath.Join(repoRoot, "src/a.go")
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}

	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED, got %s", result.Status)
	}
	if !strings.Contains(result.Details, "missing: src/a.go") {
		t.Fatalf("expected missing src/a.go in details, got: %s", result.Details)
	}
}

func TestCheckUnitBaseline_FileAdded(t *testing.T) {
	repoRoot := setupRepo(t)
	writeUnitBaseline(t, repoRoot)

	dir := filepath.Join(repoRoot, "internal/demo")
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package demo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED, got %s", result.Status)
	}
	if !strings.Contains(result.Details, "added: internal/demo/new.go") {
		t.Fatalf("expected added internal/demo/new.go in details, got: %s", result.Details)
	}
}

func TestWriteRuleBaseline_RoundTrip(t *testing.T) {
	repoRoot := t.TempDir()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	rulePath := filepath.Join(ruleDir, "g_rule_demo.md")
	if err := os.WriteFile(rulePath, []byte("---\nrule_id: g_rule_demo\n---\nrule truth\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteRuleBaseline(repoRoot, "g_rule_demo"); err != nil {
		t.Fatalf("WriteRuleBaseline: %v", err)
	}
	result := CheckRuleBaseline(repoRoot, "g_rule_demo")
	if result.Status != StatusOK {
		t.Fatalf("expected OK, got %s: %s", result.Status, result.Details)
	}

	if err := os.WriteFile(rulePath, []byte("---\nrule_id: g_rule_demo\n---\nmodified truth\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result = CheckRuleBaseline(repoRoot, "g_rule_demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED after rule edit, got %s", result.Status)
	}
}

func TestRemoveBaseline(t *testing.T) {
	repoRoot := setupRepo(t)
	writeUnitBaseline(t, repoRoot)

	if err := RemoveBaseline(repoRoot, "unit", "demo"); err != nil {
		t.Fatalf("RemoveBaseline: %v", err)
	}
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusMissing {
		t.Fatalf("expected MISSING after removal, got %s", result.Status)
	}

	// Removing an already-missing baseline is not an error.
	if err := RemoveBaseline(repoRoot, "unit", "demo"); err != nil {
		t.Fatalf("second RemoveBaseline: %v", err)
	}
}

func TestCheckUnitBaseline_NoBaseline(t *testing.T) {
	repoRoot := setupRepo(t)
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusMissing {
		t.Fatalf("expected MISSING with no baseline, got %s", result.Status)
	}
}

func TestWriteUnitBaseline_PendingPlaceholderSkipped(t *testing.T) {
	repoRoot := setupRepo(t)
	pendingSpec := strings.Replace(unitSpec, "implementation_surface: internal/demo", "implementation_surface: <pending>", 1)
	if err := WriteUnitBaseline(repoRoot, "demo", pendingSpec); err != nil {
		t.Fatalf("WriteUnitBaseline: %v", err)
	}
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusOK {
		t.Fatalf("expected OK for pending-only surface, got %s: %s", result.Status, result.Details)
	}
}

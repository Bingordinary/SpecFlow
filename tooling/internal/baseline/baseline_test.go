package baseline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

const unitSpec = `---
id: demo
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
	cmd := exec.Command("git", "-C", repoRoot, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
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

func writeBaselineTestFile(t *testing.T, repoRoot, rel, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestCheckUnitBaseline_ExcludesGitIgnoredFiles verifies the baseline records
// repository content only: ignored dependencies and build output are neither
// snapshotted at promote nor reported as drift when they appear later, while
// untracked non-ignored files still are.
func TestCheckUnitBaseline_ExcludesGitIgnoredFiles(t *testing.T) {
	repoRoot := setupRepo(t)
	writeBaselineTestFile(t, repoRoot, ".gitignore", "internal/demo/node_modules/\ninternal/demo/build/\n")
	writeBaselineTestFile(t, repoRoot, "internal/demo/node_modules/dep/index.js", "module.exports = {};\n")
	writeBaselineTestFile(t, repoRoot, "internal/demo/build/out.js", "out\n")

	writeUnitBaseline(t, repoRoot)

	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")
	data, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("baseline not written: %v", err)
	}
	if strings.Contains(string(data), "node_modules") || strings.Contains(string(data), "build/") {
		t.Fatalf("ignored files must not enter the baseline:\n%s", data)
	}
	if !strings.Contains(string(data), "internal/demo/handler.go") {
		t.Fatalf("repository-content files must enter the baseline:\n%s", data)
	}
	if result := CheckUnitBaseline(repoRoot, "demo"); result.Status != StatusOK {
		t.Fatalf("expected OK, got %s: %s", result.Status, result.Details)
	}

	writeBaselineTestFile(t, repoRoot, "internal/demo/node_modules/dep/new.js", "new\n")
	if result := CheckUnitBaseline(repoRoot, "demo"); result.Status != StatusOK {
		t.Fatalf("a new ignored file is not drift, got %s: %s", result.Status, result.Details)
	}

	writeBaselineTestFile(t, repoRoot, "internal/demo/new.go", "package demo\n")
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged || !strings.Contains(result.Details, "internal/demo/new.go") {
		t.Fatalf("expected the untracked non-ignored addition to be drift, got %s: %s", result.Status, result.Details)
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

func TestWriteUnitBaseline_ChunksRoundTrip(t *testing.T) {
	repoRoot := setupRepo(t)
	writeUnitBaseline(t, repoRoot)

	path := filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")
	b, err := readBaseline(path)
	if err != nil {
		t.Fatalf("baseline cannot be read back: %v", err)
	}
	found := false
	for _, s := range b.surfaces {
		for _, e := range s.Entries {
			if e.Path != "internal/demo/handler.go" {
				continue
			}
			found = true
			if e.Chunker != contenthash.ChunkerVersion || len(e.Chunks) == 0 {
				t.Fatalf("expected recorded chunk evidence, got chunker=%q chunks=%d", e.Chunker, len(e.Chunks))
			}
		}
	}
	if !found {
		t.Fatal("handler.go entry not found in the baseline")
	}

	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusOK {
		t.Fatalf("expected OK with unchanged content, got %s: %s", result.Status, result.Details)
	}
}

// TestCheckUnitBaseline_LocalizesChange pins the drift detail contract: a
// changed file is reported with the line span of the change, computed from
// the recorded chunk sequence with the same ordered bidirectional diff the
// gates use.
func TestCheckUnitBaseline_LocalizesChange(t *testing.T) {
	repoRoot := setupRepo(t)
	full := filepath.Join(repoRoot, "internal/demo/handler.go")
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&sb, "line %03d: some padding content to grow the chunk set beyond one chunk\n", i)
	}
	if err := os.WriteFile(full, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
	writeUnitBaseline(t, repoRoot)

	modified := strings.Replace(sb.String(), "line 150:", "line 150: modified", 1)
	if err := os.WriteFile(full, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "handler.go") || !strings.Contains(result.Details, "lines") {
		t.Fatalf("expected a localized line span in the details, got: %s", result.Details)
	}
}

// TestCheckUnitBaseline_LegacyEntryHashFallback pins the graceful display
// fallback for baselines written before chunk evidence existed: the drift is
// detected by whole-file hash and reported without localization.
func TestCheckUnitBaseline_LegacyEntryHashFallback(t *testing.T) {
	repoRoot := setupRepo(t)
	full := filepath.Join(repoRoot, "internal/demo/handler.go")
	text, err := contenthash.FileText(full)
	if err != nil {
		t.Fatal(err)
	}
	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	yaml := fmt.Sprintf("kind: unit\nname: demo\ntimestamp: 2026-08-09T00:00:00Z\nsurfaces:\n  - path: %q\n    entries:\n      - path: %q\n        hash: %q\n",
		"internal/demo", "internal/demo/handler.go", contenthash.FileHashText(text))
	if err := os.WriteFile(basePath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("package demo\n// changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED from the hash fallback, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "no localization recorded") {
		t.Fatalf("expected the fallback wording in the details, got: %s", result.Details)
	}
}

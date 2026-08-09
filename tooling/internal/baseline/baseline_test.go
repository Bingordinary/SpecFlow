package baseline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
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
	if err := WriteUnitBaseline(repoRoot, "demo", unitSpec, nil); err != nil {
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
	if err := WriteUnitBaseline(repoRoot, "demo", pendingSpec, nil); err != nil {
		t.Fatalf("WriteUnitBaseline: %v", err)
	}
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusOK {
		t.Fatalf("expected OK for pending-only surface, got %s: %s", result.Status, result.Details)
	}
}

func TestWriteUnitBaseline_DepsRoundTrip(t *testing.T) {
	repoRoot := setupRepo(t)
	full := filepath.Join(repoRoot, "internal/demo/handler.go")
	fc, err := contenthash.ChunkFile(full)
	if err != nil {
		t.Fatal(err)
	}
	var cids []string
	for _, c := range fc.Chunks {
		cids = append(cids, c.CID)
	}
	if err := WriteUnitBaseline(repoRoot, "demo", unitSpec, map[string][]string{"internal/demo/handler.go": cids}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")
	b, err := readBaseline(path)
	if err != nil {
		t.Fatalf("baseline cannot be read back: %v", err)
	}
	var got []string
	for _, s := range b.surfaces {
		for _, e := range s.Entries {
			if e.Path == "internal/demo/handler.go" {
				got = e.Deps
			}
		}
	}
	if len(got) != len(cids) {
		t.Fatalf("deps did not round-trip: got %v want %v", got, cids)
	}

	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusOK {
		t.Fatalf("expected OK with unchanged deps, got %s: %s", result.Status, result.Details)
	}
	if result.Note != "" {
		t.Fatalf("expected no note on unchanged content, got: %s", result.Note)
	}
}

func TestCheckUnitBaseline_DepChanged(t *testing.T) {
	repoRoot := setupRepo(t)
	full := filepath.Join(repoRoot, "internal/demo/handler.go")
	if err := os.WriteFile(full, []byte("package demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fc, err := contenthash.ChunkFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteUnitBaseline(repoRoot, "demo", unitSpec, map[string][]string{"internal/demo/handler.go": {fc.Chunks[0].CID}}); err != nil {
		t.Fatal(err)
	}

	// Changing the declared chunk content removes its CID.
	if err := os.WriteFile(full, []byte("package demo\n// changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED when a declared dep chunk changes, got %s", result.Status)
	}
	if !strings.Contains(result.Details, "handler.go") {
		t.Fatalf("expected handler.go in details, got: %s", result.Details)
	}
}

func TestCheckUnitBaseline_DepCIDMissing(t *testing.T) {
	repoRoot := setupRepo(t)
	if err := WriteUnitBaseline(repoRoot, "demo", unitSpec, map[string][]string{"src/a.go": {"sha256:deadbeef"}}); err != nil {
		t.Fatal(err)
	}
	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusChanged {
		t.Fatalf("expected CHANGED for a missing dep CID, got %s", result.Status)
	}
	if !strings.Contains(result.Details, "src/a.go") {
		t.Fatalf("expected src/a.go in details, got: %s", result.Details)
	}
}

func TestCheckUnitBaseline_DepOutsideChangeOKWithNote(t *testing.T) {
	repoRoot := setupRepo(t)
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&sb, "line %03d: some padding content to grow the chunk set beyond one chunk\n", i)
	}
	content := sb.String()
	full := filepath.Join(repoRoot, "internal/demo/handler.go")
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Baseline records the middle chunk only.
	fc, err := contenthash.ChunkFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.Chunks) < 3 {
		t.Fatalf("test setup expected multiple chunks, got %d", len(fc.Chunks))
	}
	mid := fc.Chunks[len(fc.Chunks)/2]
	if err := WriteUnitBaseline(repoRoot, "demo", unitSpec, map[string][]string{"internal/demo/handler.go": {mid.CID}}); err != nil {
		t.Fatal(err)
	}

	// Modify the first line — a different chunk than the declared one. The
	// declared chunk's CID must survive (content-defined chunking), while the
	// whole-file hash changes.
	newline := strings.Index(content, "\n")
	modified := "modified first line\n" + content[newline+1:]
	if err := os.WriteFile(full, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}

	result := CheckUnitBaseline(repoRoot, "demo")
	if result.Status != StatusOK {
		t.Fatalf("expected OK (declared dep unchanged), got %s: %s", result.Status, result.Details)
	}
	if result.Note == "" {
		t.Fatal("expected note when content changed outside the declared dependencies")
	}
	if !strings.Contains(result.Note, "handler.go") {
		t.Fatalf("expected handler.go in note, got: %s", result.Note)
	}
}

package specvalidation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// createMinimalCandidate writes a valid candidate spec with the given unit name
// and no acceptance items. Returns the absolute path to the spec.
func createMinimalCandidate(t *testing.T, repoRoot, unitName string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unit_"+unitName+".md")
	content := "---\n" +
		"id: " + unitName + "\n" +
		"layer: candidate\n" +
		"version: 0.1.0\n" +
		"unit_refs: none\n" +
		"rule_refs: none\n" +
		"---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeCandidate writes arbitrary content as the candidate spec for unitName.
func writeCandidate(t *testing.T, repoRoot, unitName, content string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unit_"+unitName+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Check 1: Frontmatter completeness
// ---------------------------------------------------------------------------

func TestCheckFrontmatter_Pass(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	result := checkFrontmatter(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS, got %s: %s", result.Status, result.Details)
	}
	if result.Name != "Frontmatter completeness" {
		t.Fatalf("unexpected check name: %q", result.Name)
	}
}

func TestCheckFrontmatter_MissingSpec(t *testing.T) {
	repoRoot := t.TempDir()
	result := checkFrontmatter(repoRoot, "nonexistent")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing spec file")
	}
}

func TestCheckFrontmatter_WrongID(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: other_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n")
	result := checkFrontmatter(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for id mismatch")
	}
}

func TestCheckFrontmatter_WrongLayer(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: stable\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n")
	result := checkFrontmatter(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for wrong layer")
	}
}

func TestCheckFrontmatter_MissingField(t *testing.T) {
	repoRoot := t.TempDir()
	// Missing version field
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nunit_refs: none\nrule_refs: none\n---\n")
	result := checkFrontmatter(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing version field")
	}
}

// ---------------------------------------------------------------------------
// Check 2: Acceptance items
// ---------------------------------------------------------------------------

func TestCheckAcceptanceItems_Pass(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: first acceptance item\n"+
			"    verification_type: manual\n"+
			"    verification_surface: docs/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: visual inspection\n"+
			"    pass_condition: ok\n"+
			"    runnable: yes\n")
	result := checkAcceptanceItems(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAcceptanceItems_MissingSet(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit") // no acceptance_item_set
	result := checkAcceptanceItems(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing acceptance_item_set")
	}
}

func TestCheckAcceptanceItems_MissingItems(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n")
	// acceptance_item_set exists but has no items with "- id:"
	result := checkAcceptanceItems(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing items")
	}
}

func TestCheckAcceptanceItems_MissingRequiredField(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: only description\n"+
			"    # missing verification_type and others\n")
	// Has acceptance_item_set with items, but required fields are missing
	result := checkAcceptanceItems(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing required fields")
	}
}

func TestCheckAcceptanceItems_InvalidNotRunnableYet(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: test item\n"+
			"    verification_type: auto\n"+
			"    verification_surface: src/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: check\n"+
			"    pass_condition: ok\n"+
			"    runnable: true\n")
	result := checkAcceptanceItems(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for invalid runnable value (true)")
	}
}

// ---------------------------------------------------------------------------
// Check 3: Anchor integrity
// ---------------------------------------------------------------------------

func TestCheckAnchors_NoEntriesPass(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit") // no affects.files
	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for no entries, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAnchors_ExistingFilePass(t *testing.T) {
	repoRoot := t.TempDir()
	// Create a source file that will be referenced
	srcDir := filepath.Join(repoRoot, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "handler.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: test\n"+
			"    verification_type: auto\n"+
			"    verification_surface: src/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: check\n"+
			"    pass_condition: ok\n"+
			"    runnable: yes\n"+
			"    affects:\n"+
			"      files:\n"+
			"        - src/handler.go\n")
	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for existing file, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAnchors_MissingFileFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: test\n"+
			"    verification_type: auto\n"+
			"    verification_surface: src/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: check\n"+
			"    pass_condition: ok\n"+
			"    runnable: yes\n"+
			"    affects:\n"+
			"      files:\n"+
			"        - src/nonexistent.go\n")
	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing anchor file")
	}
}

func TestCheckAnchors_RetiredSpecExempt(t *testing.T) {
	repoRoot := t.TempDir()
	// A retiring spec is removed from stable — its affects.files anchors are
	// not required, even when the referenced implementation is gone.
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: test\n"+
			"    verification_type: auto\n"+
			"    verification_surface: src/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: check\n"+
			"    pass_condition: ok\n"+
			"    runnable: yes\n"+
			"    affects:\n"+
			"      files:\n"+
			"        - src/nonexistent.go\n")
	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired spec with missing anchors, got %s: %s", result.Status, result.Details)
	}
}

// ---------------------------------------------------------------------------
// Check 4: Reference integrity
// ---------------------------------------------------------------------------

func TestCheckReferences_PassNone(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit") // unit_refs: none
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for unit_refs=none, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckReferences_MissingRefFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - auth@0.1.0\nrule_refs: none\n---\n")
	// auth does not exist in candidate or stable
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing unit ref")
	}
}

func TestCheckReferences_CandidateRefPass(t *testing.T) {
	repoRoot := t.TempDir()
	// Create the referenced candidate unit (candidate-first resolution)
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candidateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidateDir, "unit_auth.md"),
		[]byte("---\nid: auth\nlayer: candidate\nversion: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - auth@0.1.0\nrule_refs: none\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for candidate ref, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckReferences_StableRefPass(t *testing.T) {
	repoRoot := t.TempDir()
	// Create the referenced stable unit (no candidate, fallback to stable)
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stableDir, "unit_auth.md"),
		[]byte("---\nid: auth\nlayer: stable\nversion: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - auth@0.1.0\nrule_refs: none\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for stable ref, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckReferences_RefNotFoundFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - nonexistent_unit\nrule_refs: none\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for nonexistent ref")
	}
}

func TestCheckReferences_RetiredTargetFail(t *testing.T) {
	repoRoot := t.TempDir()
	// The referenced unit exists in the candidate layer but is being retired
	// — its stable copy will be deleted on promote, so the reference cannot
	// survive.
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candidateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidateDir, "unit_auth.md"),
		[]byte("---\nid: auth\nlayer: candidate\nversion: 0.1.0\nstatus: retired\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - auth\nrule_refs: none\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for ref to retiring unit, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "being retired") {
		t.Fatalf("expected 'being retired' in details, got: %s", result.Details)
	}
}

func TestCheckReferences_RetiredRuleTargetFail(t *testing.T) {
	repoRoot := t.TempDir()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "b_rule_auth.md"),
		[]byte("---\nrule_id: b_rule_auth\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\nstatus: retired\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs: none\nrule_refs:\n  - b_rule_auth\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for ref to retiring rule, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckReferences_RetiredAppendixInAffectsFail(t *testing.T) {
	repoRoot := t.TempDir()
	// An acceptance item references a candidate appendix that is being
	// retired — the reference breaks on promote and must be rejected.
	writeAppendix(t, repoRoot, "test_unit", "legacy",
		"unit: test_unit\nlayer: candidate\nstatus: retired\n")
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: test\n"+
			"    verification_type: auto\n"+
			"    verification_surface: src/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: check\n"+
			"    pass_condition: ok\n"+
			"    runnable: yes\n"+
			"    affects:\n"+
			"      appendices:\n"+
			"        - unit_test_unit_legacy.md\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for affects.appendices ref to retiring appendix, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "appendix being retired") {
		t.Fatalf("expected 'appendix being retired' in details, got: %s", result.Details)
	}
}

func TestCheckReferences_RetiredEvidenceRefFail(t *testing.T) {
	repoRoot := t.TempDir()
	// evidence_appendix_ref points at a candidate appendix that is being
	// retired — the field must be dropped before the appendix retires.
	writeAppendix(t, repoRoot, "test_unit", "evidence",
		"unit: test_unit\nlayer: candidate\nstatus: retired\n")
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n"+
			"evidence_appendix_ref: unit_test_unit_evidence.md\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for evidence_appendix_ref to retiring appendix, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "evidence appendix being retired") {
		t.Fatalf("expected 'evidence appendix being retired' in details, got: %s", result.Details)
	}
}

func TestCheckReferences_ActiveAppendixRefPass(t *testing.T) {
	repoRoot := t.TempDir()
	// An affects.appendices entry pointing at an active appendix is legal.
	writeAppendix(t, repoRoot, "test_unit", "api",
		"unit: test_unit\nlayer: candidate\n")
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n"+
			"evidence_appendix_ref: unit_test_unit_api.md\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for ref to active appendix, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckReferences_RetiredAppendixInAffectsInlineFlowFail(t *testing.T) {
	repoRoot := t.TempDir()
	// The inline YAML flow form of affects.appendices must be rejected like
	// the block form when it references a retiring appendix.
	writeAppendix(t, repoRoot, "test_unit", "legacy",
		"unit: test_unit\nlayer: candidate\nstatus: retired\n")
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: test\n"+
			"    verification_type: auto\n"+
			"    verification_surface: src/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: check\n"+
			"    pass_condition: ok\n"+
			"    runnable: yes\n"+
			"    affects:\n"+
			"      appendices: [unit_test_unit_legacy.md, unit_test_unit_api.md]\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for inline-flow affects.appendices ref to retiring appendix, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "appendix being retired") {
		t.Fatalf("expected 'appendix being retired' in details, got: %s", result.Details)
	}
}

func TestExtractAffectsAppendices(t *testing.T) {
	block := "---\nid: demo\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: item_1\n" +
		"    description: test\n" +
		"    affects:\n" +
		"      appendices:\n" +
		"        - unit_demo_evidence.md\n" +
		"      rules: []\n"

	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "block form",
			content: block,
			want:    []string{"unit_demo_evidence.md"},
		},
		{
			name: "inline empty list",
			content: "---\nid: demo\n---\n" +
				"acceptance_item_set:\n" +
				"  - id: item_1\n" +
				"    affects:\n" +
				"      appendices: []\n",
			want: nil,
		},
		{
			name: "inline flow list",
			content: "---\nid: demo\n---\n" +
				"acceptance_item_set:\n" +
				"  - id: item_1\n" +
				"    affects:\n" +
				"      appendices: [unit_demo_evidence.md, unit_demo_api.md]\n",
			want: []string{"unit_demo_evidence.md", "unit_demo_api.md"},
		},
		{
			name: "inline flow list with quotes",
			content: "---\nid: demo\n---\n" +
				"acceptance_item_set:\n" +
				"  - id: item_1\n" +
				"    affects:\n" +
				"      appendices: [\"unit_demo_evidence.md\"]\n",
			want: []string{"unit_demo_evidence.md"},
		},
		{
			name: "single value without brackets",
			content: "---\nid: demo\n---\n" +
				"acceptance_item_set:\n" +
				"  - id: item_1\n" +
				"    affects:\n" +
				"      appendices: unit_demo_evidence.md\n",
			want: []string{"unit_demo_evidence.md"},
		},
		{
			name: "appendices outside acceptance block ignored",
			content: "---\nid: demo\n---\n" +
				"appendices:\n" +
				"  - unit_demo_evidence.md\n",
			want: nil,
		},
		{
			name: "affects outside acceptance block ignored",
			content: "---\nid: demo\n---\n" +
				"acceptance_item_set:\n" +
				"  - id: item_1\n" +
				"    description: test\n" +
				"\n" +
				"## Design\n" +
				"See affects.appendices: [unit_demo_evidence.md]\n",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractAffectsAppendices(tc.content)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestCheckReferences_RetiredSpecOwnRefsExempt(t *testing.T) {
	repoRoot := t.TempDir()
	// A retiring spec's own references disappear with it — referencing a
	// retiring appendix from a retiring spec is not checked.
	writeAppendix(t, repoRoot, "test_unit", "evidence",
		"unit: test_unit\nlayer: candidate\nstatus: retired\n")
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n"+
			"status: retired\nevidence_appendix_ref: unit_test_unit_evidence.md\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired spec with own refs, got %s: %s", result.Status, result.Details)
	}
}

// ---------------------------------------------------------------------------
// Check 5: Appendix files
// ---------------------------------------------------------------------------

func writeAppendix(t *testing.T, repoRoot, unitName, appendixName, frontmatter string) {
	t.Helper()
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" + frontmatter + "\n---\n"
	path := filepath.Join(appendixDir, "unit_" + unitName + "_" + appendixName + ".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAppendices_NoAppendicesPass(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for no appendices, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAppendices_CorrectFrontmatterPass(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	writeAppendix(t, repoRoot, "test_unit", "api",
		"unit: test_unit\nlayer: candidate\n")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for correct frontmatter, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAppendices_WrongUnitFail(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	writeAppendix(t, repoRoot, "test_unit", "api",
		"unit: wrong_unit\nlayer: candidate\n")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for wrong unit, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "unit") {
		t.Fatalf("expected error about unit mismatch, got: %s", result.Details)
	}
}

func TestCheckAppendices_WrongLayerFail(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	writeAppendix(t, repoRoot, "test_unit", "api",
		"unit: test_unit\nlayer: stable\n")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for wrong layer, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "layer") {
		t.Fatalf("expected error about layer mismatch, got: %s", result.Details)
	}
}

func TestCheckAppendices_ExemptAppendixSkip(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	writeAppendix(t, repoRoot, "test_unit", "old",
		"unit: test_unit\nlayer: candidate\nstatus: exempt\n")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for exempt appendix (skipped), got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAppendices_ExemptAppendixWithBadFrontmatterSkip(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	writeAppendix(t, repoRoot, "test_unit", "old",
		"unit: wrong_unit\nlayer: stable\nstatus: exempt\n")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for exempt appendix even with bad frontmatter (skip), got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAppendices_RetiredAppendixSkip(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	writeAppendix(t, repoRoot, "test_unit", "old",
		"unit: wrong_unit\nlayer: stable\nstatus: retired\n")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired appendix (skipped), got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAcceptanceItems_RetiredSpecExempt(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs: none\nrule_refs: none\nstatus: retired\n---\n")
	result := checkAcceptanceItems(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired spec without acceptance items, got %s: %s", result.Status, result.Details)
	}
}

// ---------------------------------------------------------------------------
// Check 6: Version consistency
// ---------------------------------------------------------------------------

func TestCheckVersionConsistency_PassNoVersionRefs(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit") // unit_refs: none
	result := checkVersionConsistency(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckVersionConsistency_MismatchFail(t *testing.T) {
	repoRoot := t.TempDir()
	// Create stable unit with version 0.2.0
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stableDir, "unit_auth.md"),
		[]byte("---\nid: auth\nlayer: stable\nversion: 0.2.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Candidate references auth@0.1.0 (wrong version)
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - auth@0.1.0\nrule_refs: none\n---\n")
	result := checkVersionConsistency(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for version mismatch")
	}
}

func TestCheckVersionConsistency_MatchPass(t *testing.T) {
	repoRoot := t.TempDir()
	// Create stable unit with version 0.1.0
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stableDir, "unit_auth.md"),
		[]byte("---\nid: auth\nlayer: stable\nversion: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Candidate references auth@0.1.0 (correct)
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - auth@0.1.0\nrule_refs: none\n---\n")
	result := checkVersionConsistency(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for matching version, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckVersionConsistency_RetiredSpecExempt(t *testing.T) {
	repoRoot := t.TempDir()
	// The referenced unit has moved to 0.2.0, leaving the retiring spec's
	// version pin stale — the pin disappears with the retiring spec, so the
	// version-consistency check is skipped like the reference-integrity check.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stableDir, "unit_auth.md"),
		[]byte("---\nid: auth\nlayer: stable\nversion: 0.2.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - auth@0.1.0\nrule_refs: none\nstatus: retired\n---\n")
	result := checkVersionConsistency(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired spec with stale version pin, got %s: %s", result.Status, result.Details)
	}
}

// ---------------------------------------------------------------------------
// Check 7: Body layer-path check
// ---------------------------------------------------------------------------

func TestCheckLayerPaths_Pass(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"# Body\n\nThis unit depends on the token claims design of unit_auth_account_token_claims.\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_AbsoluteUnitPathFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"# Body\n\nSee docs/specs/units/candidate/unit_auth.md for details.\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for absolute candidate unit path, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_AbsoluteRulePathFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"# Body\n\nApplies docs/specs/rules/candidate/g_rule_naming.md.\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for absolute candidate rule path, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_RelativeUnitPathFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"# Body\n\nSee candidate/unit_auth.md for details.\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for relative candidate unit path, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_RelativeAppendixPathFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"# Body\n\nClaims structure: candidate/appendix/unit_auth_account_token_claims.md\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for relative candidate appendix path, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_CodePathNoFalsePositive(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"# Body\n\nThe handler lives at src/candidate/handler.go.\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for non-spec code path, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_StablePathNotChecked(t *testing.T) {
	// Stable-layer spec paths are legal in structured fields (they stay
	// valid after promote) and are not mechanically distinguishable from
	// prose by a string-level check — the agent checklist covers prose.
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"+
			"# Body\n\nSee docs/specs/units/stable/unit_payment.md.\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for stable path (not in scope of mechanical check), got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_AppendixFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n")
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	appendix := "---\nunit: test_unit\nlayer: candidate\n---\n\nReferences candidate/unit_auth.md.\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_test_unit_extra.md"), []byte(appendix), 0644); err != nil {
		t.Fatal(err)
	}
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for appendix body reference, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_ExemptAppendixSkip(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n")
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	appendix := "---\nunit: test_unit\nlayer: candidate\nstatus: exempt\n---\n\nReferences candidate/unit_auth.md.\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_test_unit_exempt.md"), []byte(appendix), 0644); err != nil {
		t.Fatal(err)
	}
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for exempt appendix, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckLayerPaths_RetiredSpecExempt(t *testing.T) {
	repoRoot := t.TempDir()
	// A retiring spec is removed from stable — layer-prefix references in its
	// body have no post-promote target and are not checked.
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n"+
			"\nReferences candidate/unit_auth.md in the body.\n")
	result := checkLayerPaths(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for retired spec body, got %s: %s", result.Status, result.Details)
	}
}

// ---------------------------------------------------------------------------
// Integration: ValidateCandidate end-to-end
// ---------------------------------------------------------------------------

// createFullCandidate writes a candidate spec that passes all 7 checks.
func createFullCandidate(t *testing.T, repoRoot, unitName string) {
	t.Helper()
	writeCandidate(t, repoRoot, unitName,
		"---\nid: "+unitName+"\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs: none\nrule_refs: none\n---\n"+
			"acceptance_item_set:\n"+
			"  - id: item_1\n"+
			"    description: integration test item\n"+
			"    verification_type: manual\n"+
			"    verification_surface: docs/\n"+
			"    implementation_surface: src/\n"+
			"    verification_method: review\n"+
			"    pass_condition: ok\n"+
			"    runnable: yes\n")
}

func TestValidateCandidate_IntegrationPass(t *testing.T) {
	repoRoot := t.TempDir()
	createFullCandidate(t, repoRoot, "test_unit")

	result := ValidateCandidate(repoRoot, "test_unit")
	if !result.Passed {
		for _, c := range result.Checks {
			if c.Status == Fail {
				t.Logf("  FAIL: %s — %s", c.Name, c.Details)
			}
		}
		t.Fatal("expected PASS for valid full candidate")
	}
	if len(result.Checks) != 7 {
		t.Fatalf("expected 7 checks, got %d", len(result.Checks))
	}
}

func TestValidateCandidate_UnitNameInOutput(t *testing.T) {
	repoRoot := t.TempDir()
	createFullCandidate(t, repoRoot, "my_unit")

	result := ValidateCandidate(repoRoot, "my_unit")
	if result.Unit != "my_unit" {
		t.Fatalf("expected Unit=my_unit, got %q", result.Unit)
	}
}

func TestFormatResult_Output(t *testing.T) {
	repoRoot := t.TempDir()
	createFullCandidate(t, repoRoot, "test_unit")

	result := ValidateCandidate(repoRoot, "test_unit")
	output := FormatResult(result)

	// Verify PASS header
	if result.Passed {
		if !strings.Contains(output, "PASS") {
			t.Fatal("expected PASS in output")
		}
		if !strings.Contains(output, "Failed checks: 0") {
			t.Fatal("expected \"Failed checks: 0\" in output")
		}
	}

	// Verify all checks appear in output
	for _, c := range result.Checks {
		if !strings.Contains(output, c.Name) {
			t.Fatalf("expected check name %q in output", c.Name)
		}
	}
}

func TestValidateCandidate_PassOutput(t *testing.T) {
	repoRoot := t.TempDir()
	createFullCandidate(t, repoRoot, "test_unit")

	result := ValidateCandidate(repoRoot, "test_unit")
	if !result.Passed {
		t.Fatal("expected PASS for valid full candidate")
	}

	output := FormatResult(result)
	if !strings.Contains(output, "PASS") {
		t.Fatal("expected PASS in output")
	}
	if !strings.Contains(output, "Failed checks: 0") {
		t.Fatal("expected \"Failed checks: 0\" in PASS output")
	}
}

func TestFormatResult_FailedChecksCount(t *testing.T) {
	result := &Result{
		Unit:   "test_unit",
		Passed: false,
		Checks: []CheckResult{
			{Name: "frontmatter", Status: Fail, Details: "missing id"},
			{Name: "acceptance items", Status: Pass},
			{Name: "anchors", Status: Fail, Details: "bad path"},
		},
	}

	output := FormatResult(result)
	if !strings.Contains(output, "Failed checks: 2") {
		t.Fatalf("expected \"Failed checks: 2\" in output, got:\n%s", output)
	}
}

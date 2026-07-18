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
	path := filepath.Join(dir, "c_unit_"+unitName+".md")
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
	path := filepath.Join(dir, "c_unit_"+unitName+".md")
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
			"    not_runnable_yet: no\n")
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
			"    not_runnable_yet: true\n")
	result := checkAcceptanceItems(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for invalid not_runnable_yet value (true)")
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
			"    not_runnable_yet: no\n"+
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
			"    not_runnable_yet: no\n"+
			"    affects:\n"+
			"      files:\n"+
			"        - src/nonexistent.go\n")
	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing anchor file")
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

func TestCheckReferences_MissingStableRefFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - s_unit_auth@0.1.0\nrule_refs: none\n---\n")
	// s_unit_auth.md does not exist in stable
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatal("expected FAIL for missing stable unit ref")
	}
}

func TestCheckReferences_ExistingRefPass(t *testing.T) {
	repoRoot := t.TempDir()
	// Create the referenced stable unit
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stableDir, "s_unit_auth.md"),
		[]byte("---\nid: auth\nlayer: stable\nversion: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - s_unit_auth@0.1.0\nrule_refs: none\n---\n")
	result := checkReferences(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for existing ref, got %s: %s", result.Status, result.Details)
	}
}

// ---------------------------------------------------------------------------
// Check 5: Appendix files (always PASS — optional)
// ---------------------------------------------------------------------------

func TestCheckAppendices_NoAppendicesPass(t *testing.T) {
	repoRoot := t.TempDir()
	createMinimalCandidate(t, repoRoot, "test_unit")
	result := checkAppendices(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for no appendices, got %s: %s", result.Status, result.Details)
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
	if err := os.WriteFile(filepath.Join(stableDir, "s_unit_auth.md"),
		[]byte("---\nid: auth\nlayer: stable\nversion: 0.2.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Candidate references auth@0.1.0 (wrong version)
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - s_unit_auth@0.1.0\nrule_refs: none\n---\n")
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
	if err := os.WriteFile(filepath.Join(stableDir, "s_unit_auth.md"),
		[]byte("---\nid: auth\nlayer: stable\nversion: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Candidate references auth@0.1.0 (correct)
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nlayer: candidate\nversion: 0.1.0\n"+
			"unit_refs:\n  - s_unit_auth@0.1.0\nrule_refs: none\n---\n")
	result := checkVersionConsistency(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for matching version, got %s: %s", result.Status, result.Details)
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
			"    not_runnable_yet: no\n")
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
	if len(result.Checks) != 6 {
		t.Fatalf("expected 6 checks, got %d", len(result.Checks))
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
}

package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// surfaceSpec builds a candidate spec whose acceptance items carry the given
// implementation_surface values, in order (item_1, item_2, ...).
func surfaceSpec(values ...string) string {
	content := "---\nid: test_unit\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n" +
		"acceptance_item_set:\n"
	for i, value := range values {
		content += fmt.Sprintf(
			"  - id: item_%d\n    description: test\n    verification_type: auto\n"+
				"    verification_surface: src/\n    implementation_surface: %s\n"+
				"    verification_method: check\n    pass_condition: ok\n    runnable: yes\n",
			i+1, value)
	}
	return content
}

func writeSurfaceFile(t *testing.T, repoRoot, rel, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustSurfaceReason(t *testing.T, problems []SurfaceProblem, wantItem, wantReason string) {
	t.Helper()
	for _, p := range problems {
		if p.ItemID == wantItem && strings.Contains(p.Reason, wantReason) {
			return
		}
	}
	t.Fatalf("expected item %s with reason containing %q, got %s", wantItem, wantReason, FormatSurfaceProblems(problems))
}

func TestCheckImplementationSurfaces_PendingSkipped(t *testing.T) {
	problems := CheckImplementationSurfaces(t.TempDir(), surfaceSpec("<pending>"))
	if len(problems) != 0 {
		t.Fatalf("expected <pending> to be skipped, got %v", problems)
	}
}

func TestCheckImplementationSurfaces_ResolvableFileAndDirectoryPass(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeSurfaceFile(t, repoRoot, "internal/demo/sub/b.go", "package sub\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("internal/demo/a.go", "internal/demo"))
	if len(problems) != 0 {
		t.Fatalf("expected resolvable surfaces to pass, got %v", problems)
	}
}

func TestCheckImplementationSurfaces_MixedItemsReportsBadOnly(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("<pending>", "internal/demo/a.go", "internal/missing"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_3", "path does not exist")
}

func TestCheckImplementationSurfaces_EmptyValueFails(t *testing.T) {
	problems := CheckImplementationSurfaces(t.TempDir(), surfaceSpec(""))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "use the <pending> placeholder")
}

func TestCheckImplementationSurfaces_SemicolonListFails(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeSurfaceFile(t, repoRoot, "internal/demo/b.go", "package demo\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("internal/demo/a.go; internal/demo/b.go"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "path does not exist")
}

func TestCheckImplementationSurfaces_WildcardFails(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "internal/tool/main.go", "package tool\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("internal/tool/**"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "path does not exist")
}

// TestCheckImplementationSurfaces_LiteralMetacharacterPathPasses verifies the
// check resolves the value literally: an existing path is accepted whatever
// characters it contains, so a real bracketed route path is not misread as a
// wildcard pattern.
func TestCheckImplementationSurfaces_LiteralMetacharacterPathPasses(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "app/[id]/route.ts", "export {}\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("app/[id]/route.ts", "app/[id]"))
	if len(problems) != 0 {
		t.Fatalf("expected literal bracketed paths to pass, got %v", problems)
	}
}

func TestCheckImplementationSurfaces_PlaceholderVariantFails(t *testing.T) {
	problems := CheckImplementationSurfaces(t.TempDir(), surfaceSpec("<Pending>"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "path does not exist")
}

func TestCheckImplementationSurfaces_EmptyDirectoryFails(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "internal/empty"), 0755); err != nil {
		t.Fatal(err)
	}

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("internal/empty"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "directory contains no files")
}

func TestCheckImplementationSurfaces_OutsideRepositoryFails(t *testing.T) {
	problems := CheckImplementationSurfaces(t.TempDir(), surfaceSpec("../outside"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "cannot be resolved")
}

// TestCheckAnchors_UnresolvableImplementationSurfaceFails verifies the
// mechanical validate surfaces the defect that gate-plan rejects.
func TestCheckAnchors_UnresolvableImplementationSurfaceFails(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit", surfaceSpec("internal/demo/a.go; internal/demo/b.go"))

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "item_1") || !strings.Contains(result.Details, "path does not exist") {
		t.Fatalf("expected item id and reason in details, got %s", result.Details)
	}
}

// TestCheckAnchors_ItemRelativeIndentSurfaceResolves verifies Check 3 reads
// item fields at the item's own nesting: a consistently nested item block
// resolves its surface instead of reporting it as empty.
func TestCheckAnchors_ItemRelativeIndentSurfaceResolves(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n"+
			"acceptance_item_set:\n"+
			"    - id: item_1\n"+
			"      description: test\n"+
			"      verification_type: auto\n"+
			"      verification_surface: src/\n"+
			"      implementation_surface: internal/demo/a.go\n"+
			"      verification_method: check\n"+
			"      pass_condition: ok\n"+
			"      runnable: yes\n")

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for a consistently nested item block, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAnchors_EmptyImplementationSurfaceFails(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit", surfaceSpec(""))

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "item_1") || !strings.Contains(result.Details, "empty") {
		t.Fatalf("expected item id and empty-value reason in details, got %s", result.Details)
	}
}

func TestCheckAnchors_PlaceholderOnlyPass(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidate(t, repoRoot, "test_unit", surfaceSpec("<pending>", "<pending>"))

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for <pending> surfaces, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAnchors_ResolvableSurfaceAndMissingAnchorFail(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeCandidate(t, repoRoot, "test_unit", surfaceSpec("internal/demo/a.go")+
		"    affects:\n      files:\n        - internal/demo/gone.go\n")

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for the missing affects.files anchor, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "affects.files paths not found: internal/demo/gone.go") {
		t.Fatalf("expected the affects.files failure detail, got %s", result.Details)
	}
	if strings.Contains(result.Details, "internal/demo/a.go") {
		t.Fatalf("resolvable implementation_surface must not be reported: %s", result.Details)
	}
}

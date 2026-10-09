package specvalidation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo returns a temporary directory that is a git worktree. Directory code
// surfaces expand over Git repository content, so a test project root must be
// a worktree; files written into it are untracked and unignored, which is
// repository content.
func newRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	cmd := exec.Command("git", "-C", repoRoot, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return repoRoot
}

// surfaceSpec builds a candidate spec whose acceptance items carry the given
// implementation_surface values, in order (item_1, item_2, ...).
func surfaceSpec(values ...string) string {
	content := "---\nid: test_unit\nunit_refs: none\nrule_refs: none\n---\n\n" +
		"acceptance_item_set:\n"
	for i, value := range values {
		content += fmt.Sprintf(
			"  - id: item_%d\n    description: test\n    verification_type: testable\n"+
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
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeSurfaceFile(t, repoRoot, "internal/demo/sub/b.go", "package sub\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("internal/demo/a.go", "internal/demo"))
	if len(problems) != 0 {
		t.Fatalf("expected resolvable surfaces to pass, got %v", problems)
	}
}

func TestCheckImplementationSurfaces_MixedItemsReportsBadOnly(t *testing.T) {
	repoRoot := newRepo(t)
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
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeSurfaceFile(t, repoRoot, "internal/demo/b.go", "package demo\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("internal/demo/a.go; internal/demo/b.go"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "path does not exist")
}

func TestCheckImplementationSurfaces_WildcardFails(t *testing.T) {
	repoRoot := newRepo(t)
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
	repoRoot := newRepo(t)
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
	repoRoot := newRepo(t)
	if err := os.MkdirAll(filepath.Join(repoRoot, "internal/empty"), 0755); err != nil {
		t.Fatal(err)
	}

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("internal/empty"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "directory contains no files")
}

// TestCheckImplementationSurfaces_OnlyIgnoredFilesFails verifies a directory
// whose files are all Git-ignored is not a usable surface: it expands to zero
// repository-content files.
func TestCheckImplementationSurfaces_OnlyIgnoredFilesFails(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, ".gitignore", "dist/\n")
	writeSurfaceFile(t, repoRoot, "dist/bundle.js", "bundle\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("dist"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "directory contains no files")
}

// TestCheckImplementationSurfaces_NonWorktreeRootFailsClosed verifies
// directory expansion fails closed outside a git worktree instead of falling
// back to a filesystem walk.
func TestCheckImplementationSurfaces_NonWorktreeRootFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()
	writeSurfaceFile(t, repoRoot, "src/a.go", "package src\n")

	problems := CheckImplementationSurfaces(repoRoot, surfaceSpec("src"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "cannot be expanded")
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
	repoRoot := newRepo(t)
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
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeCandidate(t, repoRoot, "test_unit",
		"---\nid: test_unit\nunit_refs: none\nrule_refs: none\n---\n\n"+
			"acceptance_item_set:\n"+
			"    - id: item_1\n"+
			"      description: test\n"+
			"      verification_type: testable\n"+
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
	repoRoot := newRepo(t)
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
	repoRoot := newRepo(t)
	writeCandidate(t, repoRoot, "test_unit", surfaceSpec("<pending>", "<pending>"))

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Pass {
		t.Fatalf("expected PASS for <pending> surfaces, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckAnchors_ResolvableSurfaceAndMissingAnchorFail(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeCandidate(t, repoRoot, "test_unit", surfaceSpec("internal/demo/a.go")+
		"    affects:\n      files:\n        - internal/demo/gone.go\n")

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Fail {
		t.Fatalf("expected FAIL for the missing affects.files anchor, got %s: %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "affects.files") || !strings.Contains(result.Details, "internal/demo/gone.go") || !strings.Contains(result.Details, "path does not exist") {
		t.Fatalf("expected the affects.files failure detail, got %s", result.Details)
	}
	if strings.Contains(result.Details, "internal/demo/a.go") {
		t.Fatalf("resolvable implementation_surface must not be reported: %s", result.Details)
	}
}

// evidenceFilesSpec builds a candidate spec whose single acceptance item
// declares the given affects.evidence_files values.
func evidenceFilesSpec(values ...string) string {
	content := "---\nid: test_unit\nunit_refs: none\nrule_refs: none\n---\n\n" +
		"acceptance_item_set:\n" +
		"  - id: item_1\n    description: test\n    verification_type: testable\n" +
		"    verification_surface: src/\n    implementation_surface: <pending>\n" +
		"    verification_method: check\n    pass_condition: ok\n    runnable: yes\n" +
		"    affects:\n      evidence_files:\n"
	for _, value := range values {
		content += fmt.Sprintf("        - %s\n", value)
	}
	return content
}

func TestCheckEvidenceFiles_ResolvableFilePasses(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "internal/peer/peer_test.go", "package peer\n")

	problems := CheckEvidenceFiles(repoRoot, evidenceFilesSpec("internal/peer/peer_test.go"))
	if len(problems) != 0 {
		t.Fatalf("expected a resolvable evidence file to pass, got %v", problems)
	}
}

func TestCheckEvidenceFiles_MissingDirectoryAndPendingFail(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "internal/peer/a.go", "package peer\n")

	problems := CheckEvidenceFiles(repoRoot, evidenceFilesSpec("internal/missing_test.go", "internal/peer", "<pending>"))
	if len(problems) != 3 {
		t.Fatalf("expected three problems, got %v", problems)
	}
	for _, p := range problems {
		if p.Field != "affects.evidence_files" {
			t.Fatalf("problem field = %q, want affects.evidence_files (%v)", p.Field, problems)
		}
	}
	mustSurfaceReason(t, problems, "item_1", "path does not exist")
	mustSurfaceReason(t, problems, "item_1", "single file")
	mustSurfaceReason(t, problems, "item_1", "<pending> placeholder does not apply")
}

// affectsFilesSpec builds a candidate spec whose single acceptance item
// declares the given affects.files values.
func affectsFilesSpec(values ...string) string {
	content := "---\nid: test_unit\nunit_refs: none\nrule_refs: none\n---\n\n" +
		"acceptance_item_set:\n" +
		"  - id: item_1\n    description: test\n    verification_type: testable\n" +
		"    verification_surface: src/\n    implementation_surface: <pending>\n" +
		"    verification_method: check\n    pass_condition: ok\n    runnable: yes\n" +
		"    affects:\n      files:\n"
	for _, value := range values {
		content += fmt.Sprintf("        - %s\n", value)
	}
	return content
}

func TestCheckAffectsFiles_ResolvableFileAndDirectoryPass(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeSurfaceFile(t, repoRoot, "internal/demo/sub/b.go", "package sub\n")

	problems := CheckAffectsFiles(repoRoot, affectsFilesSpec("internal/demo/a.go", "internal/demo"))
	if len(problems) != 0 {
		t.Fatalf("expected resolvable affects.files to pass, got %v", problems)
	}
}

func TestCheckAffectsFiles_MissingFileFails(t *testing.T) {
	repoRoot := newRepo(t)
	problems := CheckAffectsFiles(repoRoot, affectsFilesSpec("internal/demo/gone.go"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "path does not exist")
}

// TestCheckAffectsFiles_OutsideRepositoryFails pins the issue-#69 regression
// repair: after validate stopped deriving affects refs, the field lost its
// canonical resolution. An affects.files value that escapes the repository
// must still fail the declaration check instead of being stat-ed literally.
func TestCheckAffectsFiles_OutsideRepositoryFails(t *testing.T) {
	problems := CheckAffectsFiles(t.TempDir(), affectsFilesSpec("../outside.go"))
	if len(problems) != 1 {
		t.Fatalf("expected one problem, got %v", problems)
	}
	mustSurfaceReason(t, problems, "item_1", "cannot be resolved")
}

// TestCheckAnchors_OutsideRepositoryAffectsFileFails verifies mechanical
// validate Check 3 rejects the escaping declaration end to end, so a validate
// run fails before any session is planned.
func TestCheckAnchors_OutsideRepositoryAffectsFileFails(t *testing.T) {
	repoRoot := newRepo(t)
	writeCandidate(t, repoRoot, "test_unit", affectsFilesSpec("../outside.go"))

	result := checkAnchors(repoRoot, "test_unit")
	if result.Status != Fail || !strings.Contains(result.Details, "cannot be resolved") {
		t.Fatalf("expected FAIL naming the unresolvable affects.files path, got %s: %s", result.Status, result.Details)
	}
}

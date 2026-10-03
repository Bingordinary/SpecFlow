package promote

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// initGitRepo makes repoRoot a git worktree so directory code surfaces can
// expand over Git repository content, the deployment layout of every real
// project.
func initGitRepo(t *testing.T, repoRoot string) {
	t.Helper()
	cmd := exec.Command("git", "-C", repoRoot, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func writeCandidateUnit(t *testing.T, repoRoot, unit string) {
	t.Helper()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: " + unit + "\nunit_refs: none\nrule_refs: none\n---\n\n# " + unit + "\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_"+unit+".md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}
	appendix := "---\nunit: " + unit + "\n---\n\n# Appendix\n"
	if err := os.WriteFile(filepath.Join(appendixDir, "unit_"+unit+"_extra.md"), []byte(appendix), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeVerifyCache writes a minimal passing verify cache for the unit so
// promote can read the verify-time dependency evidence (ReadVerifyDeps).
func writeVerifyCache(t *testing.T, repoRoot, unit string) {
	t.Helper()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit", unit)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	cache := "---\ncommand: verify\nunit: " + unit + "\nmode: full\nresult: pass\nblocking: false\ntimestamp: \"2026-01-01T00:00:00Z\"\nfiles:\n  - path: \"docs/specs/units/candidate/unit_" + unit + ".md\"\n    hash: \"sha256:abc\"\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cache), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteUnitSuccess(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidateUnit(t, repoRoot, "demo")
	writeVerifyCache(t, repoRoot, "demo")

	candPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_demo.md")
	candInfo, err := os.Stat(candPath)
	if err != nil {
		t.Fatalf("candidate spec stat failed: %v", err)
	}

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass, issues: %v", result.Issues)
	}

	stableSpec := filepath.Join(repoRoot, "docs/specs/units/stable/unit_demo.md")
	content, err := os.ReadFile(stableSpec)
	if err != nil {
		t.Fatalf("stable spec missing: %v", err)
	}
	if !strings.Contains(string(content), "id: demo") {
		t.Fatalf("stable spec content missing:\n%s", content)
	}

	// Promoted artifacts must keep the source file's permissions (copy
	// semantics) — the staging temp file defaults to 0600 and must be
	// corrected before the rename.
	stableInfo, err := os.Stat(stableSpec)
	if err != nil {
		t.Fatalf("stable spec stat failed: %v", err)
	}
	if got, want := stableInfo.Mode().Perm(), candInfo.Mode().Perm(); got != want {
		t.Fatalf("stable spec permissions mismatch: got %v, want %v", got, want)
	}

	stableAppendix := filepath.Join(repoRoot, "docs/specs/units/stable/appendix/unit_demo_extra.md")
	if _, err := os.Stat(stableAppendix); err != nil {
		t.Fatalf("stable appendix missing: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_demo.md")); !os.IsNotExist(err) {
		t.Fatal("candidate spec should be removed after promote")
	}

	// No staged temp files may remain after a successful promote.
	for _, dir := range []string{"docs/specs/units/stable", "docs/specs/units/stable/appendix"} {
		leftover, _ := filepath.Glob(filepath.Join(repoRoot, dir, ".sf-tmp-*"))
		if len(leftover) > 0 {
			t.Fatalf("staged temp files left behind under %s: %v", dir, leftover)
		}
	}
}

func TestPromoteUnitBodyRelativeLayerPathWarning(t *testing.T) {
	repoRoot := t.TempDir()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nClaims structure: candidate/appendix/unit_demo_extra.md\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	found := false
	for _, a := range result.Actions {
		if strings.Contains(a, "WARNING: body contains candidate-layer path references") && strings.Contains(a, "candidate/appendix/unit_demo_extra.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected relative-form candidate-layer path WARNING, actions: %v", result.Actions)
	}
}

func TestPromoteUnitBodyAbsoluteLayerPathWarning(t *testing.T) {
	repoRoot := t.TempDir()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nSee docs/specs/units/candidate/unit_auth.md.\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	found := false
	for _, a := range result.Actions {
		if strings.Contains(a, "WARNING: body contains candidate-layer path references") && strings.Contains(a, "docs/specs/units/candidate/") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected absolute-form candidate-layer path WARNING, actions: %v", result.Actions)
	}
}

func TestPromoteUnitBodyCodePathNoWarning(t *testing.T) {
	repoRoot := t.TempDir()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nThe handler lives at src/candidate/handler.go.\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	for _, a := range result.Actions {
		if strings.Contains(a, "WARNING: body contains candidate-layer path references") {
			t.Fatalf("unexpected layer-path WARNING for code path, actions: %v", result.Actions)
		}
	}
}

func TestPromoteUnitStageFailureCleansUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory simulation is not portable to Windows")
	}
	repoRoot := t.TempDir()
	writeCandidateUnit(t, repoRoot, "demo")

	// Pre-create the directory tree, then make the stable dir read-only so
	// the main-spec staging fails after the appendix was already staged.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	appendixDir := filepath.Join(stableDir, "appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stableDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(stableDir, 0755)

	result := Promote(repoRoot, "demo")
	if result.Passed {
		t.Fatal("expected promote to fail when staging the main spec fails")
	}

	os.Chmod(stableDir, 0755)
	leftover, _ := filepath.Glob(filepath.Join(appendixDir, ".sf-tmp-*"))
	if len(leftover) > 0 {
		t.Fatalf("staged temp files left behind after failure: %v", leftover)
	}
	if _, err := os.Stat(filepath.Join(stableDir, "unit_demo.md")); !os.IsNotExist(err) {
		t.Fatal("stable spec must not exist after a failed promote")
	}
}

func writeRuleValidateCache(t *testing.T, repoRoot, ruleID string) {
	t.Helper()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule", ruleID)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	rulePath := filepath.Join(repoRoot, "docs/specs/rules/candidate", ruleID+".md")
	ruleHash, err := specpaths.FileHash(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	fc, err := contenthash.ChunkFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	var deps strings.Builder
	if len(fc.Chunks) > 0 {
		deps.WriteString("    deps:\n")
	}
	for _, c := range fc.Chunks {
		fmt.Fprintf(&deps, "      - %s\n", c.CID)
	}
	cache := "---\ncommand: validate\nrule: " + ruleID + "\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-07-31T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/" + ruleID + ".md\n    hash: sha256:" + ruleHash + "\n" + deps.String() + "---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cache), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteRuleSuccess(t *testing.T) {
	repoRoot := t.TempDir()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	rule := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Rule\n"
	if err := os.WriteFile(filepath.Join(ruleDir, "b_rule_test.md"), []byte(rule), 0644); err != nil {
		t.Fatal(err)
	}
	writeRuleValidateCache(t, repoRoot, "b_rule_test")

	result := PromoteRule(repoRoot, "b_rule_test")
	if !result.Passed {
		t.Fatalf("expected rule promote to pass, issues: %v", result.Issues)
	}

	stableRule := filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_test.md")
	content, err := os.ReadFile(stableRule)
	if err != nil {
		t.Fatalf("stable rule missing: %v", err)
	}
	if !strings.Contains(string(content), "rule_id: b_rule_test") {
		t.Fatalf("stable rule content missing:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(ruleDir, "b_rule_test.md")); !os.IsNotExist(err) {
		t.Fatal("candidate rule should be removed after promote")
	}
}

func writePromotableUnit(t *testing.T, repoRoot, unit, unitRefs, ruleRefs string) {
	t.Helper()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: " + unit + "\nunit_refs: " + unitRefs + "\nrule_refs: " + ruleRefs + "\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: " + unit + ".core\n" +
		"    description: Behavior.\n" +
		"    verification_type: testable\n" +
		"    verification_surface: internal/demo\n" +
		"    implementation_surface: internal/demo\n" +
		"    verification_method: Go test\n" +
		"    pass_condition: passes.\n" +
		"    runnable: yes\n"
	os.WriteFile(filepath.Join(candDir, "unit_"+unit+".md"), []byte(spec), 0644)
}

func TestPromoteUnitDroppedRuleRefIsRetained(t *testing.T) {
	repoRoot := t.TempDir()

	// The divergence fixture from spec_writing_guide.md §6.3: the stable unit
	// still lists b_rule_old in rule_refs while the candidate dropped it. The
	// promote commits the candidate, and the rule — left with no current-layer
	// consumers. Normal publication must retain it.
	stableUnitDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableUnit := "---\nid: demo\nunit_refs: none\nrule_refs: b_rule_old\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableRule := "---\nrule_id: b_rule_old\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Rule\n"
	os.WriteFile(filepath.Join(stableRuleDir, "b_rule_old.md"), []byte(stableRule), 0644)
	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/rule/b_rule_old.yaml")
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(basePath, []byte("kind: rule\nname: b_rule_old\nsurfaces: []\n"), 0644)
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_old/validate_result.md")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(cachePath, []byte("---\ncommand: validate\nrule: b_rule_old\n---\n"), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(candidate), 0644)
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass, issues: %v", result.Issues)
	}
	for _, p := range []string{
		"docs/specs/rules/stable/b_rule_old.md",
		"docs/specs/meta/baseline/rule/b_rule_old.yaml",
		"docs/specs/meta/validation/rule/b_rule_old/validate_result.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); err != nil {
			t.Fatalf("%s must survive normal promote", p)
		}
	}
}

func TestPromoteUnitDroppedRuleRefAlreadyRemoved(t *testing.T) {
	repoRoot := t.TempDir()

	// The dangling fixture from spec_writing_guide.md §6.5: the rule file is
	// already gone (removed before this promote committed), while the stable
	// unit still lists it in rule_refs and residual baseline/cache metadata
	// remains. The promote must not fail on the missing rule — it degrades to
	// residual metadata cleanup, the same path `specflowctl remove` takes.
	stableUnitDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableUnit := "---\nid: demo\nunit_refs: none\nrule_refs: b_rule_ghost\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/rule/b_rule_ghost.yaml")
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(basePath, []byte("kind: rule\nname: b_rule_ghost\nsurfaces: []\n"), 0644)
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_ghost/validate_result.md")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(cachePath, []byte("---\ncommand: validate\nrule: b_rule_ghost\n---\n"), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(candidate), 0644)
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass with the rule already removed, issues: %v", result.Issues)
	}
	for _, p := range []string{
		"docs/specs/meta/baseline/rule/b_rule_ghost.yaml",
		"docs/specs/meta/validation/rule/b_rule_ghost/validate_result.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); err != nil {
			t.Fatalf("%s must not exist after degraded cleanup", p)
		}
	}
}

func TestPromoteUnitDroppedRuleRefWithRetentionReasonIsRetained(t *testing.T) {
	repoRoot := t.TempDir()

	stableUnitDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableUnit := "---\nid: demo\nunit_refs: none\nrule_refs: b_rule_kept\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Retention rationale is ordinary rule body prose.
	retainedRule := "---\nrule_id: b_rule_kept\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Rule\n\nKeep this constraint for the planned audit pipeline.\n"
	os.WriteFile(filepath.Join(stableRuleDir, "b_rule_kept.md"), []byte(retainedRule), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(candidate), 0644)
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass, issues: %v", result.Issues)
	}
	if strings.Contains(strings.Join(result.Actions, "\n"), "Removed unbound rule") {
		t.Fatalf("retained rule must not be auto-removed, actions:\n%s", strings.Join(result.Actions, "\n"))
	}
	if _, err := os.Stat(filepath.Join(stableRuleDir, "b_rule_kept.md")); err != nil {
		t.Fatalf("retained rule must still exist: %v", err)
	}
}

func TestPromoteUnitDroppedRuleRefStillConsumedNotRemoved(t *testing.T) {
	repoRoot := t.TempDir()

	stableUnitDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableUnit := "---\nid: demo\nunit_refs: none\nrule_refs: b_rule_shared\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableRuleDir, "b_rule_shared.md"), []byte("---\nrule_id: b_rule_shared\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Rule\n"), 0644)

	// Another current-layer unit still references the rule.
	otherUnitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(otherUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	other := "---\nid: other\nunit_refs: none\nrule_refs: b_rule_shared\n---\n\n# other\n"
	os.WriteFile(filepath.Join(otherUnitDir, "unit_other.md"), []byte(other), 0644)

	candidate := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	os.WriteFile(filepath.Join(otherUnitDir, "unit_demo.md"), []byte(candidate), 0644)
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass, issues: %v", result.Issues)
	}
	if strings.Contains(strings.Join(result.Actions, "\n"), "Removed unbound rule") {
		t.Fatalf("still-consumed rule must not be auto-removed, actions:\n%s", strings.Join(result.Actions, "\n"))
	}
	if _, err := os.Stat(filepath.Join(stableRuleDir, "b_rule_shared.md")); err != nil {
		t.Fatalf("still-consumed rule must exist: %v", err)
	}
}

func TestPromoteUnitDroppedGlobalRuleNotAutoRemoved(t *testing.T) {
	repoRoot := t.TempDir()

	stableUnitDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableUnit := "---\nid: demo\nunit_refs: none\nrule_refs: g_rule_governance\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableRuleDir, "g_rule_governance.md"), []byte("---\nrule_id: g_rule_governance\nrule_scope: global\nrule_version: 0.1.0\n---\n\n# Rule\n"), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(candidate), 0644)
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass, issues: %v", result.Issues)
	}
	if strings.Contains(strings.Join(result.Actions, "\n"), "Removed unbound rule") {
		t.Fatalf("global rules must never be auto-removed, actions:\n%s", strings.Join(result.Actions, "\n"))
	}
	if _, err := os.Stat(filepath.Join(stableRuleDir, "g_rule_governance.md")); err != nil {
		t.Fatalf("global rule must still exist: %v", err)
	}
}

func TestCommitStagedRollsBack(t *testing.T) {
	dir := t.TempDir()

	dst1 := filepath.Join(dir, "a.md")
	if err := os.WriteFile(dst1, []byte("old-a"), 0644); err != nil {
		t.Fatal(err)
	}
	dst2 := filepath.Join(dir, "b.md")

	tmp1 := filepath.Join(dir, "t1.md")
	if err := os.WriteFile(tmp1, []byte("new-a"), 0644); err != nil {
		t.Fatal(err)
	}
	tmp2 := filepath.Join(dir, "t2.md")
	if err := os.WriteFile(tmp2, []byte("new-b"), 0644); err != nil {
		t.Fatal(err)
	}

	staged := []stagedCopy{{tmp: tmp1, dst: dst1}, {tmp: tmp2, dst: dst2}}
	commit := func(tmp, dst string) error {
		if dst == dst2 {
			return errors.New("injected commit failure")
		}
		return os.Rename(tmp, dst)
	}

	err := commitStagedWith(staged, commit)
	if err == nil {
		t.Fatal("expected commit failure to be returned")
	}

	content, err := os.ReadFile(dst1)
	if err != nil {
		t.Fatalf("dst1 must be restored after rollback: %v", err)
	}
	if string(content) != "old-a" {
		t.Fatalf("dst1 not rolled back: got %q, want %q", content, "old-a")
	}
	if _, err := os.Stat(dst2); !os.IsNotExist(err) {
		t.Fatal("dst2 must not exist after rollback (no original file)")
	}
	for _, p := range []string{tmp1, tmp2} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("staged temp file left behind after rollback: %s", p)
		}
	}
	leftover, _ := filepath.Glob(filepath.Join(dir, ".sf-backup-*"))
	if len(leftover) > 0 {
		t.Fatalf("backup files left behind after rollback: %v", leftover)
	}
}

func TestPromoteUnitCandidateRemovalFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory simulation is not portable to Windows")
	}
	repoRoot := t.TempDir()
	writeCandidateUnit(t, repoRoot, "demo")

	candAppendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	writeVerifyCache(t, repoRoot, "demo")
	if err := os.Chmod(candAppendixDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(candAppendixDir, 0755)

	result := Promote(repoRoot, "demo")
	if result.Passed {
		t.Fatal("expected promote to fail when candidate cleanup fails")
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "failed to remove candidate appendix") {
		t.Fatalf("expected candidate cleanup issue, got: %v", result.Issues)
	}

	// The stable layer is fully archived and the candidate spec is still in
	// place, so a re-run completes the promote.
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/units/stable/unit_demo.md")); err != nil {
		t.Fatalf("stable spec missing after cleanup failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_demo.md")); err != nil {
		t.Fatal("candidate spec must survive a cleanup failure so promote is re-runnable")
	}

	os.Chmod(candAppendixDir, 0755)
	result2 := Promote(repoRoot, "demo")
	if !result2.Passed {
		t.Fatalf("expected re-run promote to pass, issues: %v", result2.Issues)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_demo.md")); !os.IsNotExist(err) {
		t.Fatal("candidate spec should be removed after the re-run")
	}
}

func TestPromoteRuleCandidateRemovalFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory simulation is not portable to Windows")
	}
	repoRoot := t.TempDir()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	rule := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Rule\n"
	if err := os.WriteFile(filepath.Join(ruleDir, "b_rule_test.md"), []byte(rule), 0644); err != nil {
		t.Fatal(err)
	}
	writeRuleValidateCache(t, repoRoot, "b_rule_test")

	if err := os.Chmod(ruleDir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(ruleDir, 0755)

	result := PromoteRule(repoRoot, "b_rule_test")
	if result.Passed {
		t.Fatal("expected rule promote to fail when candidate cleanup fails")
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "failed to remove candidate rule") {
		t.Fatalf("expected candidate rule cleanup issue, got: %v", result.Issues)
	}

	// The stable rule is fully archived and the failure message names the
	// recovery path (the rule version gate makes an automatic re-run
	// impossible once the stable version equals the candidate version).
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_test.md")); err != nil {
		t.Fatalf("stable rule missing after cleanup failure: %v", err)
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "delete docs/specs/rules/candidate/b_rule_test.md manually") {
		t.Fatalf("expected manual cleanup guidance, got: %v", result.Issues)
	}
	if _, err := os.Stat(filepath.Join(ruleDir, "b_rule_test.md")); err != nil {
		t.Fatal("candidate rule must survive a cleanup failure")
	}
}

func TestPromoteUnit_WritesBaseline(t *testing.T) {
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)
	writeCandidateUnit(t, repoRoot, "demo")

	// Code surface: the candidate's implementation_surface (internal/demo).
	codeDir := filepath.Join(repoRoot, "internal/demo")
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		t.Fatal(err)
	}
	codeContent := "package demo\n\nfunc Core() string { return \"core\" }\n"
	codePath := filepath.Join(codeDir, "core.go")
	if err := os.WriteFile(codePath, []byte(codeContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify cache carrying declared dependency CIDs for the code file. The
	// code file path is recorded as an absolute path — the same variant the
	// gate's resolvePath tolerates — proving ReadVerifyDeps canonicalizes
	// keys before the baseline matching (a non-canonical key would silently
	// drop the deps and the assertion below would fail).
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/demo")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	depCID := contenthash.CID([]byte(codeContent))
	cache := "---\ncommand: verify\nunit: demo\nmode: full\nresult: pass\nblocking: false\ntimestamp: \"2026-01-01T00:00:00Z\"\nfiles:\n  - path: \"docs/specs/units/candidate/unit_demo.md\"\n    hash: \"sha256:abc\"\n  - path: \"" + codePath + "\"\n    hash: \"" + depCID + "\"\n    deps:\n      - \"" + depCID + "\"\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cache), 0644); err != nil {
		t.Fatal(err)
	}

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass, issues: %v", result.Issues)
	}

	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")
	data, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("baseline not written: %v", err)
	}
	if !strings.Contains(string(data), "deps:") || !strings.Contains(string(data), depCID) {
		t.Fatalf("baseline missing verify dependency CIDs:\n%s", data)
	}
}

func TestPromoteUnit_NoVerifyCacheFails(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidateUnit(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	if result.Passed {
		t.Fatal("expected promote to fail without verify dependency evidence")
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "failed to read verify dependency evidence") {
		t.Fatalf("expected verify dependency issue, got: %v", result.Issues)
	}
}

func TestPromoteRule_WritesBaseline(t *testing.T) {
	repoRoot := t.TempDir()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	rule := "---\nrule_id: g_rule_test\nrule_scope: global\nrule_version: 0.1.0\n---\n\n# Rule\n"
	if err := os.WriteFile(filepath.Join(ruleDir, "g_rule_test.md"), []byte(rule), 0644); err != nil {
		t.Fatal(err)
	}
	writeRuleValidateCache(t, repoRoot, "g_rule_test")

	result := PromoteRule(repoRoot, "g_rule_test")
	if !result.Passed {
		t.Fatalf("expected rule promote to pass, issues: %v", result.Issues)
	}

	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/rule/g_rule_test.yaml")
	if _, err := os.Stat(basePath); err != nil {
		t.Fatalf("rule baseline not written: %v", err)
	}
}

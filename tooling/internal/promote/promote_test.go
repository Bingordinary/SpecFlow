package promote

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func writeCandidateUnit(t *testing.T, repoRoot, unit string) {
	t.Helper()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: " + unit + "\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# " + unit + "\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_"+unit+".md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}
	appendix := "---\nunit: " + unit + "\nlayer: candidate\n---\n\n# Appendix\n"
	if err := os.WriteFile(filepath.Join(appendixDir, "unit_"+unit+"_extra.md"), []byte(appendix), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteUnitSuccess(t *testing.T) {
	repoRoot := t.TempDir()
	writeCandidateUnit(t, repoRoot, "demo")

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
	if !strings.Contains(string(content), "layer: stable") {
		t.Fatalf("stable spec layer not transformed to stable:\n%s", content)
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
	spec := "---\nid: demo\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nClaims structure: candidate/appendix/unit_demo_extra.md\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

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
	spec := "---\nid: demo\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nSee docs/specs/units/candidate/unit_auth.md.\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

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
	spec := "---\nid: demo\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nThe handler lives at src/candidate/handler.go.\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

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
	cache := "---\ncommand: validate\nrule: " + ruleID + "\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-07-31T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/" + ruleID + ".md\n    hash: sha256:" + ruleHash + "\n---\n"
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
	rule := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\n---\n\n# Rule\n"
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
	if !strings.Contains(string(content), "layer: stable") {
		t.Fatalf("stable rule layer not transformed to stable:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(ruleDir, "b_rule_test.md")); !os.IsNotExist(err) {
		t.Fatal("candidate rule should be removed after promote")
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
	rule := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\n---\n\n# Rule\n"
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

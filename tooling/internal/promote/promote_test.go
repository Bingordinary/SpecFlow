package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func writeCandidateUnit(t *testing.T, repoRoot, unit string) {
	t.Helper()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: " + unit + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# " + unit + "\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nClaims structure: candidate/appendix/unit_demo_extra.md\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nSee docs/specs/units/candidate/unit_auth.md.\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nThe handler lives at src/candidate/handler.go.\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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

func TestPromoteUnitRetiredAppendix(t *testing.T) {
	repoRoot := t.TempDir()

	// Stable layer already holds both appendix copies from a previous round.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	stableAppendixDir := filepath.Join(stableDir, "appendix")
	if err := os.MkdirAll(stableAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableSpec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n"
	stableExtra := "---\nunit: demo\n---\n\nOld extra content\n"
	stableLegacy := "---\nunit: demo\n---\n\nOld legacy content\n"
	os.WriteFile(filepath.Join(stableDir, "unit_demo.md"), []byte(stableSpec), 0644)
	os.WriteFile(filepath.Join(stableAppendixDir, "unit_demo_extra.md"), []byte(stableExtra), 0644)
	os.WriteFile(filepath.Join(stableAppendixDir, "unit_demo_legacy.md"), []byte(stableLegacy), 0644)

	// Candidate: active spec + active extra appendix + retiring legacy appendix.
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\n---\n\nNew extra content\n"), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_legacy.md"), []byte("---\nunit: demo\nstatus: retired\n---\n\nTo be retired\n"), 0644)
	writeVerifyCache(t, repoRoot, "demo")

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected promote to pass, issues: %v", result.Issues)
	}

	// The retiring appendix's stable copy must be removed; the active
	// appendix and the main spec must be archived normally.
	if _, err := os.Stat(filepath.Join(stableAppendixDir, "unit_demo_legacy.md")); !os.IsNotExist(err) {
		t.Fatal("retired appendix stable copy must be removed")
	}
	extra, err := os.ReadFile(filepath.Join(stableAppendixDir, "unit_demo_extra.md"))
	if err != nil {
		t.Fatalf("active appendix stable copy missing: %v", err)
	}
	if !strings.Contains(string(extra), "New extra content") {
		t.Fatalf("active appendix not updated: %s", extra)
	}
	if _, err := os.Stat(filepath.Join(stableDir, "unit_demo.md")); err != nil {
		t.Fatalf("stable spec missing: %v", err)
	}
	// Candidate layer fully cleaned.
	if _, err := os.Stat(filepath.Join(candAppendixDir, "unit_demo_legacy.md")); !os.IsNotExist(err) {
		t.Fatal("retired candidate appendix must be removed")
	}
	if _, err := os.Stat(filepath.Join(candAppendixDir, "unit_demo_extra.md")); !os.IsNotExist(err) {
		t.Fatal("candidate appendix must be removed after promote")
	}
	foundRetireAction := false
	for _, a := range result.Actions {
		if strings.Contains(a, "Retiring appendix") && strings.Contains(a, "unit_demo_legacy.md") {
			foundRetireAction = true
		}
	}
	if !foundRetireAction {
		t.Fatalf("expected retiring action for legacy appendix, actions: %v", result.Actions)
	}
}

func TestPromoteUnitRetiredSpec(t *testing.T) {
	repoRoot := t.TempDir()

	// Stable layer holds the unit from a previous round, including an
	// exempt appendix that must also disappear with the unit.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	stableAppendixDir := filepath.Join(stableDir, "appendix")
	if err := os.MkdirAll(stableAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableSpec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableDir, "unit_demo.md"), []byte(stableSpec), 0644)
	os.WriteFile(filepath.Join(stableAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\n---\n\nExtra\n"), 0644)
	os.WriteFile(filepath.Join(stableAppendixDir, "unit_demo_exempt.md"), []byte("---\nunit: demo\nstatus: exempt\n---\n\nExempt\n"), 0644)

	// Candidate declares the whole unit retired — no acceptance item set.
	// The candidate appendix is also marked retired (a retiring unit carries
	// no active candidate content).
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: demo\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# demo\n\nThe unit is retired.\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(retiredSpec), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\nstatus: retired\n---\n\nExtra\n"), 0644)

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass, issues: %v", result.Issues)
	}

	// The whole unit is gone from stable (main spec + every appendix,
	// including the exempt one) and from candidate.
	for _, p := range []string{
		"docs/specs/units/stable/unit_demo.md",
		"docs/specs/units/stable/appendix/unit_demo_extra.md",
		"docs/specs/units/stable/appendix/unit_demo_exempt.md",
		"docs/specs/units/candidate/unit_demo.md",
		"docs/specs/units/candidate/appendix/unit_demo_extra.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after retire", p)
		}
	}

	foundRetireAction := false
	for _, a := range result.Actions {
		if strings.Contains(a, "Retiring: docs/specs/units/stable/unit_demo.md") {
			foundRetireAction = true
		}
	}
	if !foundRetireAction {
		t.Fatalf("expected retire action, actions: %v", result.Actions)
	}
}

func TestRetiredUnitBackupFailureRollbackPreservesStableAppendix(t *testing.T) {
	repoRoot := t.TempDir()

	// Stable layer holds the unit and a same-named appendix from a previous
	// round. The candidate carries the same appendix name, so the retired-unit
	// removal list must schedule the stable destination exactly once — a
	// duplicate entry would break the backup-phase rollback (the duplicate's
	// empty backup entry deletes the just-restored file).
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	stableAppendixDir := filepath.Join(stableDir, "appendix")
	if err := os.MkdirAll(stableAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableDir, "unit_demo.md"), []byte("---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"), 0644)
	os.WriteFile(filepath.Join(stableAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\n---\n\nORIGINAL EXTRA\n"), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte("---\nid: demo\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n"), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\n---\n"), 0644)

	// Sabotage the backup phase: a directory occupying the main spec's backup
	// name makes the rename fail after the appendix was already moved aside.
	if err := os.MkdirAll(filepath.Join(stableDir, ".sf-backup-unit_demo.md"), 0755); err != nil {
		t.Fatal(err)
	}

	result := Promote(repoRoot, "demo")
	if result.Passed {
		t.Fatal("promote must fail when the backup phase fails")
	}

	// The stable appendix must survive the rollback with its original content.
	data, err := os.ReadFile(filepath.Join(stableAppendixDir, "unit_demo_extra.md"))
	if err != nil {
		t.Fatalf("stable appendix lost on rollback: %v", err)
	}
	if !strings.Contains(string(data), "ORIGINAL EXTRA") {
		t.Fatalf("stable appendix content corrupted: %s", data)
	}
}

func TestPromoteUnitRetiredKeepsNoCandidateAppendix(t *testing.T) {
	repoRoot := t.TempDir()

	// Stable layer holds the unit from a previous round, including a
	// non-retired appendix that must disappear with the unit.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	stableAppendixDir := filepath.Join(stableDir, "appendix")
	if err := os.MkdirAll(stableAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableSpec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableDir, "unit_demo.md"), []byte(stableSpec), 0644)
	os.WriteFile(filepath.Join(stableAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\n---\n\nExtra\n"), 0644)

	// The candidate main spec is retired, but the candidate appendix is NOT
	// marked retired — the documented unit-retirement procedure only requires
	// the status on the main spec.
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: demo\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# demo\n\nThe unit is retired.\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(retiredSpec), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\n---\n\nExtra\n"), 0644)

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass, issues: %v", result.Issues)
	}

	// The whole unit is gone from every layer — no stable appendix may
	// survive a retiring unit, even one whose candidate copy was not marked
	// retired.
	for _, p := range []string{
		"docs/specs/units/stable/unit_demo.md",
		"docs/specs/units/stable/appendix/unit_demo_extra.md",
		"docs/specs/units/candidate/unit_demo.md",
		"docs/specs/units/candidate/appendix/unit_demo_extra.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after retire", p)
		}
	}

	// No candidate appendix may be promoted by a retire.
	for _, a := range result.Actions {
		if strings.Contains(a, "Promoted appendix") {
			t.Fatalf("retire promote must not promote appendices, got action: %s", a)
		}
	}
}

func TestPromoteUnitRetiredNeverPromoted(t *testing.T) {
	repoRoot := t.TempDir()

	// A unit that was created and retired before ever reaching stable: retire
	// must not create any stable files.
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(retiredSpec), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_extra.md"), []byte("---\nunit: demo\n---\n\nExtra\n"), 0644)

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass, issues: %v", result.Issues)
	}

	for _, p := range []string{
		"docs/specs/units/stable/unit_demo.md",
		"docs/specs/units/stable/appendix/unit_demo_extra.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not be created by retire", p)
		}
	}
	for _, p := range []string{
		"docs/specs/units/candidate/unit_demo.md",
		"docs/specs/units/candidate/appendix/unit_demo_extra.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must be cleaned up after retire", p)
		}
	}
}

func TestPromoteUnitRetiredReferrerBlocked(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: demo\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(retiredSpec), 0644)
	consumer := "---\nid: consumer\nversion: 0.1.0\nunit_refs: demo\nrule_refs: none\n---\n\n# consumer\n"
	os.WriteFile(filepath.Join(candDir, "unit_consumer.md"), []byte(consumer), 0644)

	result := Promote(repoRoot, "demo")
	if result.Passed {
		t.Fatal("expected retire promote to be rejected while a unit still references it")
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "still referenced by") {
		t.Fatalf("expected referrer issue, got: %v", result.Issues)
	}
}

func writePromotableUnit(t *testing.T, repoRoot, unit, unitRefs, ruleRefs string) {
	t.Helper()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: " + unit + "\nversion: 0.1.0\nunit_refs: " + unitRefs + "\nrule_refs: " + ruleRefs + "\n---\n" +
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

func TestPromoteUnitRefToRetiringCandidateRejects(t *testing.T) {
	repoRoot := t.TempDir()

	// The referenced unit exists only in the candidate layer and is marked
	// retired — promoting it is impossible, so the reference must be removed.
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: retiring\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# retiring\n"
	os.WriteFile(filepath.Join(candDir, "unit_retiring.md"), []byte(retiredSpec), 0644)
	writePromotableUnit(t, repoRoot, "consumer", "retiring", "none")

	result := Promote(repoRoot, "consumer")
	if result.Passed {
		t.Fatalf("expected promote to be rejected for a reference to a retiring unit, issues: %v", result.Issues)
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "is being retired") {
		t.Fatalf("expected retiring-target issue, got: %v", result.Issues)
	}
}

func TestPromoteUnitRefToRetiringUnitWithStableRejects(t *testing.T) {
	repoRoot := t.TempDir()

	// The referenced unit still has a stable copy today, but the candidate
	// layer declares retirement — the stable copy is removed on promote, so
	// the reference cannot survive.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableDir, "unit_retiring.md"), []byte("---\nid: retiring\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# retiring\n"), 0644)
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: retiring\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# retiring\n"
	os.WriteFile(filepath.Join(candDir, "unit_retiring.md"), []byte(retiredSpec), 0644)
	writePromotableUnit(t, repoRoot, "consumer", "retiring", "none")

	result := Promote(repoRoot, "consumer")
	if result.Passed {
		t.Fatalf("expected promote to be rejected for a reference to a retiring unit, issues: %v", result.Issues)
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "is being retired") {
		t.Fatalf("expected retiring-target issue, got: %v", result.Issues)
	}
}

func TestPromoteRuleRefToRetiringCandidateRejects(t *testing.T) {
	repoRoot := t.TempDir()

	// The referenced rule exists only in the candidate layer and is marked
	// retired — the reference must be removed before the rule is retired.
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredRule := "---\nrule_id: b_rule_x\nrule_scope: bound\nrule_version: 0.1.0\nstatus: retired\n---\n\n# Rule\n"
	os.WriteFile(filepath.Join(ruleDir, "b_rule_x.md"), []byte(retiredRule), 0644)
	writePromotableUnit(t, repoRoot, "consumer", "none", "b_rule_x")

	result := Promote(repoRoot, "consumer")
	if result.Passed {
		t.Fatalf("expected promote to be rejected for a reference to a retiring rule, issues: %v", result.Issues)
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "is being retired") {
		t.Fatalf("expected retiring-target issue, got: %v", result.Issues)
	}
}

func TestPromoteRuleRefToRetiringRuleWithStableRejects(t *testing.T) {
	repoRoot := t.TempDir()

	// The referenced rule still has a stable copy today, but the candidate
	// layer declares retirement — the stable copy is removed on promote, so
	// the reference cannot survive.
	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableRuleDir, "b_rule_x.md"), []byte("---\nrule_id: b_rule_x\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Rule\n"), 0644)
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredRule := "---\nrule_id: b_rule_x\nrule_scope: bound\nrule_version: 0.1.1\nstatus: retired\n---\n\n# Rule\n"
	os.WriteFile(filepath.Join(ruleDir, "b_rule_x.md"), []byte(retiredRule), 0644)
	writePromotableUnit(t, repoRoot, "consumer", "none", "b_rule_x")

	result := Promote(repoRoot, "consumer")
	if result.Passed {
		t.Fatalf("expected promote to be rejected for a reference to a retiring rule, issues: %v", result.Issues)
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "is being retired") {
		t.Fatalf("expected retiring-target issue, got: %v", result.Issues)
	}
}

func TestPromoteRejectsRetiringAppendixRef(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n" +
		"evidence_appendix_ref: unit_demo_evidence.md\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: demo.core\n" +
		"    description: Behavior.\n" +
		"    verification_type: testable\n" +
		"    verification_surface: internal/demo\n" +
		"    implementation_surface: internal/demo\n" +
		"    verification_method: Go test\n" +
		"    pass_condition: passes.\n" +
		"    runnable: yes\n" +
		"    affects:\n" +
		"      appendices:\n" +
		"        - unit_demo_evidence.md\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_evidence.md"), []byte("---\nunit: demo\nstatus: retired\n---\n\nLegacy\n"), 0644)

	result := Promote(repoRoot, "demo")
	if result.Passed {
		t.Fatal("expected promote to be rejected while the spec references a retiring appendix")
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "retiring appendix") {
		t.Fatalf("expected retiring-appendix issue, got: %v", result.Issues)
	}
}

func TestPromoteRejectsRetiringAppendixInlineFlowRef(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: demo.core\n" +
		"    description: Behavior.\n" +
		"    verification_type: testable\n" +
		"    verification_surface: internal/demo\n" +
		"    implementation_surface: internal/demo\n" +
		"    verification_method: Go test\n" +
		"    pass_condition: passes.\n" +
		"    runnable: yes\n" +
		"    affects:\n" +
		"      appendices: [unit_demo_evidence.md]\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_evidence.md"), []byte("---\nunit: demo\nstatus: retired\n---\n\nLegacy\n"), 0644)

	result := Promote(repoRoot, "demo")
	if result.Passed {
		t.Fatal("expected promote to be rejected while the spec references a retiring appendix via inline flow form")
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "retiring appendix") {
		t.Fatalf("expected retiring-appendix issue, got: %v", result.Issues)
	}
}

func TestPromoteRetiredUnitWithOwnAppendixRefs(t *testing.T) {
	repoRoot := t.TempDir()

	// A retiring unit may still reference its own retiring appendix — both
	// disappear together, so the reference is not checked.
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candAppendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(candAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\n" +
		"status: retired\nevidence_appendix_ref: unit_demo_evidence.md\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: demo.core\n" +
		"    description: Behavior.\n" +
		"    verification_type: testable\n" +
		"    verification_surface: internal/demo\n" +
		"    implementation_surface: internal/demo\n" +
		"    verification_method: Go test\n" +
		"    pass_condition: passes.\n" +
		"    runnable: yes\n" +
		"    affects:\n" +
		"      appendices:\n" +
		"        - unit_demo_evidence.md\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(spec), 0644)
	os.WriteFile(filepath.Join(candAppendixDir, "unit_demo_evidence.md"), []byte("---\nunit: demo\nstatus: retired\n---\n\nLegacy\n"), 0644)

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass with own appendix refs, issues: %v", result.Issues)
	}
	for _, p := range []string{
		"docs/specs/units/candidate/unit_demo.md",
		"docs/specs/units/candidate/appendix/unit_demo_evidence.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after retire", p)
		}
	}
}

func TestPromoteRuleRetired(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableRule := "---\nrule_id: b_rule_old\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Rule\n"
	os.WriteFile(filepath.Join(stableDir, "b_rule_old.md"), []byte(stableRule), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Same version as stable: the version gate is skipped for retired rules.
	retiredRule := "---\nrule_id: b_rule_old\nrule_scope: bound\nrule_version: 0.1.0\nstatus: retired\n---\n\n# Rule retired\n"
	os.WriteFile(filepath.Join(candDir, "b_rule_old.md"), []byte(retiredRule), 0644)
	writeRuleValidateCache(t, repoRoot, "b_rule_old")

	result := PromoteRule(repoRoot, "b_rule_old")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass, issues: %v", result.Issues)
	}
	if _, err := os.Stat(filepath.Join(stableDir, "b_rule_old.md")); !os.IsNotExist(err) {
		t.Fatal("retired rule stable copy must be removed")
	}
	if _, err := os.Stat(filepath.Join(candDir, "b_rule_old.md")); !os.IsNotExist(err) {
		t.Fatal("retired candidate rule must be removed")
	}
}

func TestPromoteRuleRetiredReferrerBlocked(t *testing.T) {
	repoRoot := t.TempDir()

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredRule := "---\nrule_id: b_rule_x\nrule_scope: bound\nrule_version: 0.1.0\nstatus: retired\n---\n\n# Rule\n"
	os.WriteFile(filepath.Join(candDir, "b_rule_x.md"), []byte(retiredRule), 0644)
	writeRuleValidateCache(t, repoRoot, "b_rule_x")

	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	consumer := "---\nid: consumer\nversion: 0.1.0\nunit_refs: none\nrule_refs: b_rule_x\n---\n\n# consumer\n"
	os.WriteFile(filepath.Join(unitDir, "unit_consumer.md"), []byte(consumer), 0644)

	result := PromoteRule(repoRoot, "b_rule_x")
	if result.Passed {
		t.Fatal("expected retire promote to be rejected while a unit still references it")
	}
	if !strings.Contains(strings.Join(result.Issues, " "), "still referenced by") {
		t.Fatalf("expected referrer issue, got: %v", result.Issues)
	}
}

func TestPromoteUnitRetiredOwnRefsExempt(t *testing.T) {
	repoRoot := t.TempDir()

	// The retiring unit's own refs point to a unit that exists only in the
	// candidate layer — normally a promote rejection, but the retiring spec's
	// references disappear with it and are not checked.
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: demo\nversion: 0.1.1\nunit_refs: only_candidate\nrule_refs: none\nstatus: retired\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(retiredSpec), 0644)
	os.WriteFile(filepath.Join(candDir, "unit_only_candidate.md"), []byte("---\nid: only_candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# only_candidate\n"), 0644)

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass with own candidate-only refs, issues: %v", result.Issues)
	}
	if _, err := os.Stat(filepath.Join(candDir, "unit_demo.md")); !os.IsNotExist(err) {
		t.Fatal("retired candidate spec must be removed after promote")
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

func TestPromoteUnitRetired_RemovesBaseline(t *testing.T) {
	repoRoot := t.TempDir()

	// Simulate a previous round: stable unit exists and a baseline was
	// recorded at that promote.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableSpec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n"
	os.WriteFile(filepath.Join(stableDir, "unit_demo.md"), []byte(stableSpec), 0644)
	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(basePath, []byte("kind: unit\nname: demo\nsurfaces: []\n"), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: demo\nversion: 0.1.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# demo\n\nThe unit is retired.\n"
	os.WriteFile(filepath.Join(candDir, "unit_demo.md"), []byte(retiredSpec), 0644)

	result := Promote(repoRoot, "demo")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass, issues: %v", result.Issues)
	}

	if _, err := os.Stat(basePath); !os.IsNotExist(err) {
		t.Fatalf("baseline must be removed on retire, stat err: %v", err)
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

func TestPromoteRuleRetired_RemovesBaseline(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableDir, "b_rule_test.md"), []byte("---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 1.0.0\n---\n\nTruth\n"), 0644)
	basePath := filepath.Join(repoRoot, "docs/specs/meta/baseline/rule/b_rule_test.yaml")
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(basePath, []byte("kind: rule\nname: b_rule_test\nsurfaces: []\n"), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredRule := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 1.0.1\nstatus: retired\n---\n\nRetired\n"
	os.WriteFile(filepath.Join(candDir, "b_rule_test.md"), []byte(retiredRule), 0644)
	writeRuleValidateCache(t, repoRoot, "b_rule_test")

	result := PromoteRule(repoRoot, "b_rule_test")
	if !result.Passed {
		t.Fatalf("expected retire promote to pass, issues: %v", result.Issues)
	}

	if _, err := os.Stat(basePath); !os.IsNotExist(err) {
		t.Fatalf("rule baseline must be removed on retire, stat err: %v", err)
	}
}

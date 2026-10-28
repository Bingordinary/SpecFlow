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
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/testsupport"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
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

func writePromiseState(t *testing.T, repoRoot, unit, layer string) {
	t.Helper()
	specPath := filepath.Join(repoRoot, "docs/specs/units", layer, "unit_"+unit+".md")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	promises, issues := specvalidation.ParsePromises(string(data))
	if len(issues) > 0 {
		t.Fatalf("promise fixture: %s", strings.Join(issues, "; "))
	}
	state := validationcache.PromiseStateFile{Unit: unit, Layer: layer}
	content := specpaths.NormalizeText(string(data))
	for _, promise := range promises {
		fingerprint, err := specvalidation.PromiseFingerprint(promise)
		if err != nil {
			t.Fatal(err)
		}
		state.Promises = append(state.Promises, validationcache.PromisePin{ID: promise.ID, Fingerprint: fingerprint})
		for _, anchor := range promise.Source {
			cid, ok := contenthash.NarrativeSourceCID(content, anchor)
			if !ok {
				t.Fatalf("promise %s source %q does not resolve", promise.ID, anchor)
			}
			state.SourcePins = append(state.SourcePins, validationcache.PromiseSourcePin{ID: promise.ID, Anchor: anchor, CID: cid})
		}
	}
	path, err := validationcache.PromiseStatePath(repoRoot, unit, layer)
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(path, state); err != nil {
		t.Fatal(err)
	}
}

// minimalUnitSpec builds a deployed-format unit spec fixture for tests that
// need a valid current-layer spec without exercising promise details.
func minimalUnitSpec(unit string, ruleRefs []string) string {
	refs := "none"
	if len(ruleRefs) > 0 {
		refs = "[" + strings.Join(ruleRefs, ", ") + "]"
	}
	return "---\nid: " + unit + "\nrule_refs: " + refs + "\n---\n\n# " + unit + "\n\n" +
		"## Design Narrative\n### Narrative\n\nProse.\n\n" +
		"## Promises\n\npromises:\n  - id: " + unit + ".core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    promise: " + unit + ".core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: inspectable\n    verification_surface: api\n    implementation_surface: src/" + unit + ".go\n    verification_method: inspect.\n    pass_condition: passes.\n    runnable: yes\n"
}

func writeCandidateUnit(t *testing.T, repoRoot, unit string) {
	t.Helper()
	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candDir, "appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: " + unit + "\nrule_refs: none\n---\n\n# " + unit + "\n\n" +
		"## Design Narrative\n### Narrative\n\nProse.\n\n" +
		"## Promises\n\npromises:\n  - id: " + unit + ".core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    promise: " + unit + ".core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(candDir, "unit_"+unit+".md"), []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}
	appendix := "---\nunit: " + unit + "\n---\n\n# Appendix\n"
	if err := os.WriteFile(filepath.Join(appendixDir, "unit_"+unit+"_extra.md"), []byte(appendix), 0644); err != nil {
		t.Fatal(err)
	}
	writePromiseState(t, repoRoot, unit, "candidate")
}

// writeVerifyCache writes a minimal passing verify cache for the unit so the
// promote flow consumes a realistically shaped candidate cache set.
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
	writePromiseState(t, repoRoot, unit, "candidate")
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

	stableState, err := validationcache.PromiseStatePath(repoRoot, "demo", "stable")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stableState); err != nil {
		t.Fatalf("stable promise state missing: %v", err)
	}
	candidateState, err := validationcache.PromiseStatePath(repoRoot, "demo", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidateState); !os.IsNotExist(err) {
		t.Fatalf("candidate promise state must be removed, got %v", err)
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
	spec := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\nClaims structure: candidate/appendix/unit_demo_extra.md\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	spec := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\nSee docs/specs/units/candidate/unit_auth.md.\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	spec := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\nThe handler lives at src/candidate/handler.go.\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	writeVerifyCache(t, repoRoot, "demo")

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

// fixtureStandard is the judging standard a fixture validate cache records:
// the value gate-finalize writes for the same repository. Fixture repositories
// deploy no framework layout, so the fallback digest is the standard.
func fixtureStandard(t *testing.T, repoRoot string) string {
	t.Helper()
	testsupport.DeployStandard(t, repoRoot)
	return judgments.MustProtocol(repoRoot, "validate", "rule")
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
	text, err := contenthash.FileText(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	records := contenthash.ChunkRecords(text)
	var chunks strings.Builder
	if len(records) > 0 {
		chunks.WriteString("    chunker: " + contenthash.ChunkerVersion + "\n    chunks:\n")
	}
	for _, c := range records {
		fmt.Fprintf(&chunks, "      - cid: \"%s\"\n        start: %d\n        end: %d\n", c.CID, c.StartLine, c.EndLine)
	}
	cache := "---\ncommand: validate\nstandard: " + fixtureStandard(t, repoRoot) + "\nrule: " + ruleID + "\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-07-31T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/" + ruleID + ".md\n    hash: sha256:" + ruleHash + "\n" + chunks.String() + "---\n"
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
	rule := "---\nrule_id: b_rule_test\nrule_scope: bound\n---\n\n# Rule\n"
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
	stableUnit := minimalUnitSpec("demo", []string{"b_rule_old"})
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableRule := "---\nrule_id: b_rule_old\nrule_scope: bound\n---\n\n# Rule\n"
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
	candidate := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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

	// The dangling fixture from spec_writing_guide.md §6.4: the rule file is
	// already gone (removed before this promote committed), while the stable
	// unit still lists it in rule_refs and residual baseline/cache metadata
	// remains. The promote must not fail on the missing rule — it degrades to
	// residual metadata cleanup, the same path `specflowctl remove` takes.
	stableUnitDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableUnit := minimalUnitSpec("demo", []string{"b_rule_ghost"})
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
	candidate := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	stableUnit := minimalUnitSpec("demo", []string{"b_rule_kept"})
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Retention rationale is ordinary rule body prose.
	retainedRule := "---\nrule_id: b_rule_kept\nrule_scope: bound\n---\n\n# Rule\n\nKeep this constraint for the planned audit pipeline.\n"
	os.WriteFile(filepath.Join(stableRuleDir, "b_rule_kept.md"), []byte(retainedRule), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	stableUnit := minimalUnitSpec("demo", []string{"b_rule_shared"})
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableRuleDir, "b_rule_shared.md"), []byte("---\nrule_id: b_rule_shared\nrule_scope: bound\n---\n\n# Rule\n"), 0644)

	// Another current-layer unit still references the rule.
	otherUnitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(otherUnitDir, 0755); err != nil {
		t.Fatal(err)
	}
	other := minimalUnitSpec("other", []string{"b_rule_shared"})
	os.WriteFile(filepath.Join(otherUnitDir, "unit_other.md"), []byte(other), 0644)

	candidate := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	stableUnit := minimalUnitSpec("demo", []string{"g_rule_governance"})
	os.WriteFile(filepath.Join(stableUnitDir, "unit_demo.md"), []byte(stableUnit), 0644)

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(stableRuleDir, "g_rule_governance.md"), []byte("---\nrule_id: g_rule_governance\nrule_scope: global\n---\n\n# Rule\n"), 0644)

	candDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candDir, 0755); err != nil {
		t.Fatal(err)
	}
	candidate := "---\nid: demo\nrule_refs: none\n---\n\n# demo\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: demo.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: demo.core\n    promise: demo.core\n    source: [Narrative]\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"
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
	cases := []struct {
		name     string
		existing []bool
	}{
		{"single-existing-rule", []bool{true}},
		{"single-new-rule", []bool{false}},
		{"existing-unit-files", []bool{true, true, true}},
		{"new-unit-files", []bool{false, false, false}},
		{"mixed-unit-files", []bool{true, false, true}},
		{"mixed-new-first", []bool{false, true, false}},
	}
	for _, tc := range cases {
		for failAt := -1; failAt < len(tc.existing); failAt++ {
			t.Run(fmt.Sprintf("%s/fail-at-%d", tc.name, failAt), func(t *testing.T) {
				dir := t.TempDir()
				var staged []stagedCopy
				originalModes := make([]os.FileMode, len(tc.existing))
				for i, existing := range tc.existing {
					src := filepath.Join(dir, fmt.Sprintf("candidate-%d.md", i))
					dst := filepath.Join(dir, fmt.Sprintf("stable-%d.md", i))
					if err := os.WriteFile(src, []byte(fmt.Sprintf("new-%d", i)), 0644); err != nil {
						t.Fatal(err)
					}
					if existing {
						if err := os.WriteFile(dst, []byte(fmt.Sprintf("old-%d", i)), 0600); err != nil {
							t.Fatal(err)
						}
						info, err := os.Stat(dst)
						if err != nil {
							t.Fatal(err)
						}
						originalModes[i] = info.Mode().Perm()
					}
					tmp, err := stageCopy(src, dst)
					if err != nil {
						t.Fatal(err)
					}
					staged = append(staged, stagedCopy{tmp: tmp, dst: dst})
				}

				commitFailure := errors.New("injected commit failure")
				calls := 0
				err := commitStagedWith(staged, func(tmp, dst string) error {
					i := calls
					calls++
					if i == failAt {
						return commitFailure
					}
					return os.Rename(tmp, dst)
				})
				if failAt >= 0 && !errors.Is(err, commitFailure) {
					t.Fatalf("expected commit failure, got %v", err)
				}
				if failAt < 0 && err != nil {
					t.Fatalf("successful archive failed: %v", err)
				}
				for i, s := range staged {
					if failAt >= 0 && !tc.existing[i] {
						if _, err := os.Lstat(s.dst); !os.IsNotExist(err) {
							t.Errorf("new destination remains after rollback: %s (%v)", s.dst, err)
						}
					} else {
						want := fmt.Sprintf("old-%d", i)
						if failAt < 0 {
							want = fmt.Sprintf("new-%d", i)
						}
						got, err := os.ReadFile(s.dst)
						if err != nil || string(got) != want {
							t.Errorf("destination %d: got %q, error %v; want %q", i, got, err, want)
						}
						if failAt >= 0 {
							info, err := os.Stat(s.dst)
							if err != nil || info.Mode().Perm() != originalModes[i] {
								t.Errorf("original destination permissions not restored: %s (%v)", s.dst, err)
							}
						}
					}
					candidate, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("candidate-%d.md", i)))
					if err != nil || string(candidate) != fmt.Sprintf("new-%d", i) {
						t.Errorf("candidate source changed: %d (%v)", i, err)
					}
				}
				for _, pattern := range []string{".sf-tmp-*", ".sf-backup-*"} {
					leftover, err := filepath.Glob(filepath.Join(dir, pattern))
					if err != nil || len(leftover) > 0 {
						t.Errorf("archive temp files remain: %v (%v)", leftover, err)
					}
				}
			})
		}
	}
}

func TestCommitStagedReportsRollbackErrorsAndPreservesBackups(t *testing.T) {
	dir := t.TempDir()
	var staged []stagedCopy
	for i := 0; i < 3; i++ {
		dst := filepath.Join(dir, fmt.Sprintf("stable-%d.md", i))
		tmp := filepath.Join(dir, fmt.Sprintf("stage-%d.md", i))
		if err := os.WriteFile(dst, []byte(fmt.Sprintf("old-%d", i)), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tmp, []byte(fmt.Sprintf("new-%d", i)), 0644); err != nil {
			t.Fatal(err)
		}
		staged = append(staged, stagedCopy{tmp: tmp, dst: dst})
	}
	commitFailure := errors.New("injected commit failure")
	archiveErr := commitStagedWith(staged, func(tmp, dst string) error {
		// Non-empty directories prevent restoration at two paths; the other
		// original must still be restored, and both failures must be reported.
		for _, i := range []int{0, 2} {
			if err := os.Mkdir(staged[i].dst, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(staged[i].dst, "obstruction"), []byte("blocked"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		return commitFailure
	})
	if !errors.Is(archiveErr, commitFailure) {
		t.Fatalf("original commit error was lost: %v", archiveErr)
	}
	for _, i := range []int{0, 2} {
		if !strings.Contains(archiveErr.Error(), "restore "+staged[i].dst) {
			t.Errorf("restoration error for destination %d not reported: %v", i, archiveErr)
		}
	}
	got, readErr := os.ReadFile(staged[1].dst)
	if readErr != nil || string(got) != "old-1" {
		t.Errorf("unblocked original was not restored: %q (%v)", got, readErr)
	}
	backups, globErr := filepath.Glob(filepath.Join(dir, ".sf-backup-*"))
	if globErr != nil || len(backups) != 2 {
		t.Fatalf("expected two preserved backups, got %v (%v)", backups, globErr)
	}
	remaining := map[string]bool{"old-0": false, "old-2": false}
	for _, backup := range backups {
		data, err := os.ReadFile(backup)
		if err != nil {
			t.Fatal(err)
		}
		remaining[string(data)] = true
		if !strings.Contains(archiveErr.Error(), backup) {
			t.Errorf("preserved backup path missing from restoration error: %s", backup)
		}
	}
	for content, present := range remaining {
		if !present {
			t.Errorf("original content %q is not preserved", content)
		}
	}
	for _, s := range staged {
		if _, err := os.Stat(s.tmp); !os.IsNotExist(err) {
			t.Errorf("staged copy remains: %s (%v)", s.tmp, err)
		}
	}
}

func TestCommitStagedBackupFailureRestoresPreparedDestinations(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "stable.md")
	blocker := filepath.Join(dir, "not-a-directory")
	for path, content := range map[string]string{dst: "old", blocker: "blocker"} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var staged []stagedCopy
	for i, target := range []string{dst, filepath.Join(blocker, "stable.md")} {
		tmp := filepath.Join(dir, fmt.Sprintf("stage-%d.md", i))
		if err := os.WriteFile(tmp, []byte("new"), 0644); err != nil {
			t.Fatal(err)
		}
		staged = append(staged, stagedCopy{tmp: tmp, dst: target})
	}
	err := commitStagedWith(staged, func(tmp, dst string) error {
		t.Fatal("commit must not start when destination inspection or backup fails")
		return nil
	})
	if err == nil {
		t.Fatal("expected destination preparation failure")
	}
	for path, want := range map[string]string{dst: "old", blocker: "blocker"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("original %s changed: %q (%v)", path, got, err)
		}
	}
	for _, s := range staged {
		if _, err := os.Stat(s.tmp); !os.IsNotExist(err) {
			t.Errorf("staged copy remains after backup failure: %s (%v)", s.tmp, err)
		}
	}
	leftover, err := filepath.Glob(filepath.Join(dir, ".sf-backup-*"))
	if err != nil || len(leftover) != 0 {
		t.Errorf("backup files remain after successful restoration: %v (%v)", leftover, err)
	}
}

func TestCommitStagedPreservesEarlierRecoveryBackup(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "stable.md")
	previousBackup := filepath.Join(dir, ".sf-backup-stable.md")
	tmp := filepath.Join(dir, "stage.md")
	for path, content := range map[string]string{dst: "old", previousBackup: "earlier original", tmp: "new"} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := commitStagedWith([]stagedCopy{{tmp: tmp, dst: dst}}, os.Rename); err != nil {
		t.Fatalf("archive failed: %v", err)
	}
	got, err := os.ReadFile(previousBackup)
	if err != nil || string(got) != "earlier original" {
		t.Fatalf("backup from an earlier restoration failure was overwritten: %q (%v)", got, err)
	}
}

func TestCommitStagedRestoresCandidateRemovals(t *testing.T) {
	for failAt := -1; failAt < 2; failAt++ {
		t.Run(fmt.Sprintf("fail-at-%d", failAt), func(t *testing.T) {
			dir := t.TempDir()
			var staged []stagedCopy
			var candidates []string
			for i := 0; i < 2; i++ {
				src := filepath.Join(dir, fmt.Sprintf("candidate-%d.md", i))
				dst := filepath.Join(dir, fmt.Sprintf("published-%d.md", i))
				if err := os.WriteFile(src, []byte(fmt.Sprintf("new-%d", i)), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(dst, []byte(fmt.Sprintf("old-%d", i)), 0644); err != nil {
					t.Fatal(err)
				}
				tmp, err := stageCopy(src, dst)
				if err != nil {
					t.Fatal(err)
				}
				staged = append(staged, stagedCopy{tmp: tmp, dst: dst})
				candidates = append(candidates, src)
			}
			failure := errors.New("controlled publication failure")
			calls := 0
			err := commitStagedWith(staged, func(tmp, dst string) error {
				for _, candidate := range candidates {
					if _, err := os.Stat(candidate); !os.IsNotExist(err) {
						t.Errorf("candidate removal was not prepared: %v", err)
					}
				}
				i := calls
				calls++
				if i == failAt {
					return failure
				}
				return os.Rename(tmp, dst)
			}, candidates...)
			if failAt >= 0 && !errors.Is(err, failure) {
				t.Fatalf("expected failure: %v", err)
			}
			if failAt < 0 && err != nil {
				t.Fatal(err)
			}
			for i, s := range staged {
				want := fmt.Sprintf("new-%d", i)
				if failAt >= 0 {
					want = fmt.Sprintf("old-%d", i)
				}
				got, err := os.ReadFile(s.dst)
				if err != nil || string(got) != want {
					t.Errorf("incorrect published state: %q (%v)", got, err)
				}
				got, err = os.ReadFile(candidates[i])
				if failAt >= 0 && (err != nil || string(got) != fmt.Sprintf("new-%d", i)) {
					t.Errorf("candidate not restored: %q (%v)", got, err)
				}
				if failAt < 0 && !os.IsNotExist(err) {
					t.Errorf("candidate remains: %v", err)
				}
			}
			leftovers, err := filepath.Glob(filepath.Join(dir, ".sf-*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("transaction debris: %v (%v)", leftovers, err)
			}
		})
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
	if !strings.Contains(strings.Join(result.Issues, " "), "Failed to commit promote") {
		t.Fatalf("expected candidate cleanup issue, got: %v", result.Issues)
	}

	// No accepted truth or baseline is published on cleanup failure.
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/units/stable/unit_demo.md")); !os.IsNotExist(err) {
		t.Fatalf("stable spec must stay absent after cleanup failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/meta/baseline/unit/demo.yaml")); !os.IsNotExist(err) {
		t.Fatalf("baseline must stay absent after cleanup failure: %v", err)
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
	rule := "---\nrule_id: b_rule_test\nrule_scope: bound\n---\n\n# Rule\n"
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
	if !strings.Contains(strings.Join(result.Issues, " "), "Failed to commit rule") {
		t.Fatalf("expected candidate rule cleanup issue, got: %v", result.Issues)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_test.md")); !os.IsNotExist(err) {
		t.Fatalf("stable rule must stay absent after cleanup failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/meta/baseline/rule/b_rule_test.yaml")); !os.IsNotExist(err) {
		t.Fatalf("baseline must stay absent after cleanup failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ruleDir, "b_rule_test.md")); err != nil {
		t.Fatal("candidate rule must survive a cleanup failure")
	}
	if err := os.Chmod(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if retry := PromoteRule(repoRoot, "b_rule_test"); !retry.Passed {
		t.Fatalf("normal rule retry must pass: %v", retry.Issues)
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

	// Verify pass cache for the code surface. The baseline no longer copies
	// verify-time dependency declarations: promote computes each surface
	// file's whole-file hash and ordered chunk sequence itself.
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/demo")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	depCID := contenthash.CID([]byte(codeContent))
	cache := "---\ncommand: verify\nunit: demo\nmode: full\nresult: pass\nblocking: false\ntimestamp: \"2026-01-01T00:00:00Z\"\nfiles:\n  - path: \"docs/specs/units/candidate/unit_demo.md\"\n    hash: \"sha256:abc\"\n  - path: \"" + codePath + "\"\n    hash: \"" + depCID + "\"\n---\n"
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
	if !strings.Contains(string(data), "chunker:") || !strings.Contains(string(data), "chunks:") {
		t.Fatalf("baseline missing chunk evidence:\n%s", data)
	}
}

func TestPromoteRule_WritesBaseline(t *testing.T) {
	repoRoot := t.TempDir()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	rule := "---\nrule_id: g_rule_test\nrule_scope: global\n---\n\n# Rule\n"
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

func writePromiseSpecFile(t *testing.T, repoRoot, layer, unit, promiseID, contractText, dependsOn string) string {
	t.Helper()
	depends := ""
	if dependsOn != "" {
		depends = "    depends_on: [" + dependsOn + "]\n"
	}
	content := "---\nid: " + unit + "\nrule_refs: none\n---\n\n" +
		"## Design Narrative\n### Narrative\n\nNarrative.\n\n" +
		"## Promises\n\npromises:\n  - id: " + promiseID + "\n    summary: Contract.\n    source: [Narrative]\n    contract: {invariants: [" + contractText + "]}\n" + depends + "\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    promise: " + promiseID + "\n    source: [Narrative]\n    description: Given X, When Y, Then Z.\n    verification_type: inspectable\n    verification_surface: api\n    implementation_surface: src/" + unit + ".go\n    verification_method: inspect.\n    pass_condition: Passes.\n    runnable: yes\n"
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unit_"+unit+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return content
}

func sourcePinsForContent(t *testing.T, content, promiseID string) []validationcache.PromiseSourcePin {
	t.Helper()
	normalized := specpaths.NormalizeText(content)
	promises, issues := specvalidation.ParsePromises(normalized)
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	for _, promise := range promises {
		if promise.ID != promiseID {
			continue
		}
		var pins []validationcache.PromiseSourcePin
		for _, anchor := range promise.Source {
			cid, ok := contenthash.NarrativeSourceCID(normalized, anchor)
			if !ok {
				t.Fatalf("promise %s source %q does not resolve", promise.ID, anchor)
			}
			pins = append(pins, validationcache.PromiseSourcePin{ID: promise.ID, Anchor: anchor, CID: cid})
		}
		return pins
	}
	t.Fatalf("promise %q not found", promiseID)
	return nil
}

func promiseContractFP(t *testing.T, content, promiseID string) string {
	t.Helper()
	promises, issues := specvalidation.ParsePromises(content)
	if len(issues) > 0 {
		t.Fatalf("promise fixture: %s", strings.Join(issues, "; "))
	}
	for _, promise := range promises {
		if promise.ID == promiseID {
			fingerprint, err := specvalidation.PromiseFingerprint(promise)
			if err != nil {
				t.Fatal(err)
			}
			return fingerprint
		}
	}
	t.Fatalf("promise %q not found", promiseID)
	return ""
}

func TestPromoteRejectsUnpublishedProviderPromise(t *testing.T) {
	repoRoot := t.TempDir()
	writePromiseSpecFile(t, repoRoot, "candidate", "web", "web.session_api", "session", "auth.login_api")
	providerCandidate := writePromiseSpecFile(t, repoRoot, "candidate", "auth", "auth.login_api", "candidate login", "")
	webContent := writePromiseSpecFile(t, repoRoot, "candidate", "web", "web.session_api", "session", "auth.login_api")
	writePromiseSpecFile(t, repoRoot, "stable", "auth", "auth.login_api", "stable login", "")

	state := &validationcache.PromiseStateFile{
		Unit:       "web",
		Layer:      "candidate",
		Promises:   []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: promiseContractFP(t, webContent, "web.session_api")}},
		Pins:       []validationcache.PromisePin{{ID: "auth.login_api", Fingerprint: promiseContractFP(t, providerCandidate, "auth.login_api")}},
		SourcePins: sourcePinsForContent(t, webContent, "web.session_api"),
	}
	issues := checkPromiseStateForPromotion(repoRoot, "web", webContent, state)
	if len(issues) == 0 || !strings.Contains(strings.Join(issues, "; "), "unpublished candidate changes") {
		t.Fatalf("expected unpublished-provider rejection, got %v", issues)
	}

	// Publishing the provider contract makes the same consumer state valid.
	providerStable := writePromiseSpecFile(t, repoRoot, "stable", "auth", "auth.login_api", "candidate login", "")
	state.Pins[0].Fingerprint = promiseContractFP(t, providerStable, "auth.login_api")
	issues = checkPromiseStateForPromotion(repoRoot, "web", webContent, state)
	if len(issues) > 0 {
		t.Fatalf("expected promotion-ready state, got %v", issues)
	}

	// A candidate-only provider is not publishable truth either.
	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/units/stable/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	issues = checkPromiseStateForPromotion(repoRoot, "web", webContent, state)
	if len(issues) == 0 || !strings.Contains(strings.Join(issues, "; "), "not in the stable layer") {
		t.Fatalf("expected stable-provider rejection, got %v", issues)
	}
}

func TestPromoteRejectsCandidateOnlyProvider(t *testing.T) {
	repoRoot := t.TempDir()
	authCandidate := writePromiseSpecFile(t, repoRoot, "candidate", "auth", "auth.login_api", "candidate login", "")
	webContent := writePromiseSpecFile(t, repoRoot, "candidate", "web", "web.session_api", "session", "auth.login_api")
	path, err := validationcache.PromiseStatePath(repoRoot, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(path, validationcache.PromiseStateFile{
		Unit:       "web",
		Layer:      "candidate",
		Promises:   []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: promiseContractFP(t, webContent, "web.session_api")}},
		Pins:       []validationcache.PromisePin{{ID: "auth.login_api", Fingerprint: promiseContractFP(t, authCandidate, "auth.login_api")}},
		SourcePins: sourcePinsForContent(t, webContent, "web.session_api"),
	}); err != nil {
		t.Fatal(err)
	}

	result := Promote(repoRoot, "web")
	if result.Passed {
		t.Fatalf("expected promote to reject a candidate-only provider")
	}
	joined := strings.Join(result.Issues, "; ")
	if !strings.Contains(joined, "not in the stable layer") || !strings.Contains(joined, "promote provider") {
		t.Fatalf("expected provider publication issue, got %v", result.Issues)
	}
}

func TestPromoteRejectsChangedPromiseSource(t *testing.T) {
	repoRoot := t.TempDir()
	webContent := writePromiseSpecFile(t, repoRoot, "candidate", "web", "web.session_api", "session", "auth.login_api")
	authCandidate := writePromiseSpecFile(t, repoRoot, "candidate", "auth", "auth.login_api", "candidate login", "")
	writePromiseSpecFile(t, repoRoot, "stable", "auth", "auth.login_api", "candidate login", "")

	state := &validationcache.PromiseStateFile{
		Unit:       "web",
		Layer:      "candidate",
		Promises:   []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: promiseContractFP(t, webContent, "web.session_api")}},
		Pins:       []validationcache.PromisePin{{ID: "auth.login_api", Fingerprint: promiseContractFP(t, authCandidate, "auth.login_api")}},
		SourcePins: sourcePinsForContent(t, webContent, "web.session_api"),
	}
	changed := strings.Replace(webContent, "Narrative.", "Narrative changed.", 1)
	if changed == webContent {
		t.Fatal("source edit did not apply")
	}
	issues := checkPromiseStateForPromotion(repoRoot, "web", changed, state)
	if len(issues) == 0 || !strings.Contains(strings.Join(issues, "; "), "source block changed") {
		t.Fatalf("expected a changed-source rejection, got %v", issues)
	}
}

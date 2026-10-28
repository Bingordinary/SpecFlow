package gaterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// writeDesignAnchorUnit writes a unit whose Design Narrative is split into two
// addressable blocks, so a design record can declare one of them and stay
// untouched by an edit to the other.
func writeDesignAnchorUnit(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: auth\nrule_refs: none\n---\n\n# auth\n\n" +
		"## Design Narrative\n\n" +
		"### Declared\n\nDeclared rationale.\n\n" +
		"### Undeclared\n\nUndeclared rationale.\n\n" +
		"## Promises\n\npromises:\n  - id: auth.core\n    summary: Core contract.\n    source: [Declared]\n    contract: {invariants: [core]}\n\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n" +
		"  - id: auth.core\n    promise: auth.core\n    source: [Declared]\n    description: Given X, When Y, Then Z.\n" +
		"    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n" +
		"    verification_method: test.\n    pass_condition: Passes.\n    runnable: yes\n"
	path := filepath.Join(dir, "unit_auth.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// designAnchorBaseline plans a delta verify run whose baseline design record
// declared the given spec anchors, after mutate has edited the spec on disk.
// The baseline is written against the unedited spec, so the plan compares the
// edit against it.
func designAnchorBaseline(t *testing.T, root, specPath string, anchors []string, mutate func(string) string) *Run {
	t.Helper()
	writeFile(t, root, "src/main.go", "package main\n")
	entry := evidenceEntryWithChecks(t, root, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeGateBaseline(t, root, GateVerify, "pass", "full", false, []validationcache.FileEntry{entry}, nil, anchors)
	if err := os.WriteFile(specPath, []byte(mutate(readTestFile(t, specPath))), 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := Plan(root, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func sourceOf(run *Run, key string) string {
	for _, ck := range run.Coverage {
		if ck.Key == key {
			return ck.Source
		}
	}
	return ""
}

// TestVerifyDeltaCarriesDesignWhenUndeclaredBlockChanges is the design-anchor
// reuse contract: a design record pins exactly the blocks its report declared,
// so editing a sibling narrative block must not re-run it.
func TestVerifyDeltaCarriesDesignWhenUndeclaredBlockChanges(t *testing.T) {
	root := newRepo(t)
	specPath := writeDesignAnchorUnit(t, root)
	run := designAnchorBaseline(t, root, specPath, []string{"Declared"}, func(spec string) string {
		return strings.Replace(spec, "Undeclared rationale.", "Undeclared rationale, edited.", 1)
	})

	if got := sourceOf(run, "design:auth:src/main.go"); got == "executed" {
		t.Fatalf("an edit to an undeclared narrative block must not re-run the design judgment, coverage=%+v notices=%v", run.Coverage, run.Notices)
	}
	// The unit-level scan pins the whole spec file, so it must still re-run.
	if got := sourceOf(run, "unit:auth"); got != "executed" {
		t.Fatalf("a spec edit must re-run the unit-level scan, got %q", got)
	}
}

// TestVerifyDeltaRerunsDesignWhenDeclaredBlockChanges is the other half: an
// edit to the block the report declared re-runs the judgment.
func TestVerifyDeltaRerunsDesignWhenDeclaredBlockChanges(t *testing.T) {
	root := newRepo(t)
	specPath := writeDesignAnchorUnit(t, root)
	run := designAnchorBaseline(t, root, specPath, []string{"Declared"}, func(spec string) string {
		return strings.Replace(spec, "Declared rationale.", "Declared rationale, edited.", 1)
	})

	if got := sourceOf(run, "design:auth:src/main.go"); got != "executed" {
		t.Fatalf("an edit to a declared narrative block must re-run the design judgment, got %q notices=%v", got, run.Notices)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "design:auth:src/main.go re-runs") {
		t.Fatalf("the plan must state why the design judgment re-runs, notices=%v", run.Notices)
	}
}

// TestVerifyDeltaRerunsDesignWhenDeclaredPromiseChanges pins the promise-entry
// anchor: a promise the report did not declare is not a dependency, while the
// declared one is.
func TestVerifyDeltaRerunsDesignWhenDeclaredPromiseChanges(t *testing.T) {
	// Declaring the promise makes a promise edit re-run the design judgment.
	root := newRepo(t)
	specPath := writeDesignAnchorUnit(t, root)
	run := designAnchorBaseline(t, root, specPath, []string{"Declared", "auth.core"}, func(spec string) string {
		return strings.Replace(spec, "summary: Core contract.", "summary: Core contract, restated.", 1)
	})
	if got := sourceOf(run, "design:auth:src/main.go"); got != "executed" {
		t.Fatalf("a declared promise edit must re-run the design judgment, got %q notices=%v", got, run.Notices)
	}
}

// TestVerifyDeltaCarriesDesignWhenUndeclaredPromiseChanges is the complement: a
// promise the report never declared is not a dependency of the judgment.
func TestVerifyDeltaCarriesDesignWhenUndeclaredPromiseChanges(t *testing.T) {
	root := newRepo(t)
	specPath := writeDesignAnchorUnit(t, root)
	run := designAnchorBaseline(t, root, specPath, []string{"Declared"}, func(spec string) string {
		return strings.Replace(spec, "summary: Core contract.", "summary: Core contract, restated.", 1)
	})
	if got := sourceOf(run, "design:auth:src/main.go"); got == "executed" {
		t.Fatalf("a promise the report never declared must not re-run the design judgment, notices=%v", run.Notices)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestSaveDesignJudgmentPinsDeclaredAnchors asserts the derived record pins
// exactly the declared anchors, and that the record cannot be written without
// them.
func TestSaveDesignJudgmentPinsDeclaredAnchors(t *testing.T) {
	root := newRepo(t)
	writeDesignAnchorUnit(t, root)
	writeFile(t, root, "src/main.go", "package main\n")
	derivation, err := NewDerivation(root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := derivation.resolveRun(GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	run.RunID = "20260101-000000-abcdef"
	coverage, err := derivation.computeCoverage(run)
	if err != nil {
		t.Fatal(err)
	}
	var ck *CoverageKey
	for i := range coverage {
		if coverage[i].Key == "design:auth:src/main.go" {
			ck = &coverage[i]
		}
	}
	if ck == nil {
		t.Fatalf("the design key must be part of the coverage set: %v", coverageKeysOf(run))
	}
	base := SessionResult{Kind: ck.Kind, Verdicts: map[string]string{ck.Key: "acceptable"}, EffectiveStatus: map[string]string{ck.Key: "pass"}, ReportDigest: "sha256:fixture"}

	// No declaration: rejected.
	if _, err := SaveJudgment(root, run, *ck, &base, "fixture", nil); err == nil || !strings.Contains(err.Error(), "no Spec basis anchors") {
		t.Fatalf("a design record without declared anchors must be rejected, got %v", err)
	}

	// Unresolvable anchor: rejected.
	bad := base
	bad.SpecBasis = map[string][]string{ck.Key: {"No Such Block"}}
	if _, err := SaveJudgment(root, run, *ck, &bad, "fixture", nil); err == nil || !strings.Contains(err.Error(), "does not resolve") {
		t.Fatalf("an unresolvable anchor must be rejected, got %v", err)
	}

	// Declared anchors: pinned exactly, and nothing else.
	declared := base
	declared.SpecBasis = map[string][]string{ck.Key: {"Declared", "auth.core"}}
	ref, err := SaveJudgment(root, run, *ck, &declared, "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := judgments.Load(root, ref)
	if err != nil {
		t.Fatal(err)
	}
	var deps []string
	for _, d := range record.Dependencies {
		deps = append(deps, d.Deps...)
	}
	joined := strings.Join(deps, ",")
	for _, want := range []string{"region:subsection:Declared:", "region:promise:auth.core:"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("declared anchor %q must be pinned, deps=%v", want, deps)
		}
	}
	for _, unwanted := range []string{"region:subsection:Undeclared:", "region:section:Promises:", "region:section:Design Narrative:"} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("undeclared dependency %q must not be pinned, deps=%v", unwanted, deps)
		}
	}
	if len(record.Dependencies) != 3 {
		t.Fatalf("the design record pins its file plus two spec anchors, deps=%+v", record.Dependencies)
	}
}

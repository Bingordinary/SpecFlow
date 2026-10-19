package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestDeltaItemEditDoesNotForceUnrelatedDesignKeys pins the verify delta scope
// fix for issue #76: an acceptance-item text edit re-runs that item (and the
// whole-unit architecture judgment), but not the unit's per-file design or
// public code judgments. Their recorded rationale — the unit's design prose
// sections — is unchanged, so the mechanical floor must not invalidate them.
func TestDeltaItemEditDoesNotForceUnrelatedDesignKeys(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// Edit only the acceptance item text — the design prose sections that the
	// design judgment pins are untouched.
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Then it is accepted.", "Then it is accepted, with an edit.", 1)
	if edited == string(data) {
		t.Fatal("item edit did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if grContainsString(coverageKeysOf(run), "review") {
		t.Fatalf("a verify delta plan has no change review, got %v", coverageKeysOf(run))
	}
	if run.CoverageByKey("item:auth:auth.core") == nil {
		t.Fatalf("the edited acceptance item must re-run, got %v", coverageKeysOf(run))
	}
	for _, key := range []string{"code:src/auth.go", "design:auth:src/auth.go"} {
		if run.CoverageByKey(key) != nil {
			t.Fatalf("an acceptance-item edit must not mechanically re-run %s, got %v", key, coverageKeysOf(run))
		}
		if !grContainsString(run.CarriedKeys, key) {
			t.Fatalf("expected %s among the carry candidates, got %v", key, run.CarriedKeys)
		}
	}

	// Complete the run and confirm the carried conclusions are recorded as
	// carried in the cache.
	grSubmitOK(t, repoRoot, deltaRun, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, deltaRun)
	grSubmitOK(t, repoRoot, deltaRun, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, deltaRun, "--result", "pass")
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, `"source":"carried"`) {
		t.Fatalf("expected the carried conclusions recorded as carried, got:\n%s", cache)
	}
	for _, key := range []string{"code:src/auth.go", "design:auth:src/auth.go"} {
		if !strings.Contains(cache, key) {
			t.Fatalf("expected the carried conclusion %s recorded in the cache, got:\n%s", key, cache)
		}
	}
}

// TestDeltaChangedEvidenceFileStaysFresh pins the conclusion-bound verify
// delta scope: a supplied evidence file is not a local conclusion's
// dependency. When the run's final synthesis pinned it, the synthesis is the
// only conclusion that dies, and with no relationship scope to re-run the
// plan covers the full scope rather than silently carrying a stale synthesis.
func TestDeltaChangedEvidenceFileStaysFresh(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")
	grWriteFile(t, repoRoot, "helper.go", "package auth\n\nfunc helper() {}\n")
	inputs := grInputsManifest(t, "helper.go")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", inputs)
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: helper.go: all\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// Change only the supplied evidence file the synthesis cited.
	grWriteFile(t, repoRoot, "helper.go", "package auth\n\nfunc helper() {}\n\nfunc helper2() {}\n")
	if check, err := checkUnitVerifyMerged(freshDerivation(t, repoRoot), repoRoot, "auth", "candidate"); err != nil || check.Fresh {
		t.Fatalf("a changed pinned synthesis evidence file must stale the cache: %+v %v", check, err)
	}

	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--inputs-file", inputs)
	run := mustLoadRun(t, repoRoot, deltaID)
	if !grNoticeContains(run.Notices, "synthesis evidence changed (helper.go)") {
		t.Fatalf("expected the plan to disclose the changed synthesis evidence, got %v", run.Notices)
	}
	if !grContainsString(coverageKeysOf(run), "item:auth:auth.core") {
		t.Fatalf("expected the fail-closed full scope, got %v", coverageKeysOf(run))
	}
}

// TestDeltaUnpinnedChangedFileReportedOnFreshStop pins the blind-spot report on
// the fresh-stop path: a recorded file that changed while no conclusion depends
// on it neither stales the cache nor re-runs anything, so the delta plan stops —
// and the stop must still disclose the changed file and suggest the full run,
// exactly as `fresh` does, instead of dropping the report with the plan
// (framework/verification_scope.md §Delta Runs → Verify scope and the mechanical
// floor).
func TestDeltaUnpinnedChangedFileReportedOnFreshStop(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")
	grWriteFile(t, repoRoot, "context/notes.md", "extra context\n")
	inputs := grInputsManifest(t, "context/notes.md")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", inputs)
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, runID)
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// Change only the recorded evidence no conclusion pins: the cache stays
	// fresh, so the plan has nothing to re-run and stops.
	grWriteFile(t, repoRoot, "context/notes.md", "extra context changed\n")

	_, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	if err == nil {
		t.Fatal("expected the fresh stop for an unpinned-only change")
	}
	for _, want := range []string{"cache is fresh", "context/notes.md", "verify@auth", "(full)"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the fresh stop must disclose the blind-spot report (%q missing), got: %v", want, err)
		}
	}
}

// TestDeltaImplementationChangeForcesOwningItem pins the reverse asymmetry
// from issue #76 proposal 5: an acceptance item pins its declared code surface
// (implementation_surface / affects.files), so changing the implementation it
// verifies re-runs the item instead of carrying the alignment judgment
// silently.
func TestDeltaImplementationChangeForcesOwningItem(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// Change only the implementation file the item declares (src/auth.go); the
	// item's spec region is untouched.
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n\nfunc More() {}\n")

	deltaRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if run.CoverageByKey("item:auth:auth.core") == nil {
		t.Fatalf("an implementation change must mechanically re-run the owning acceptance item, got %v", coverageKeysOf(run))
	}
}

// TestDeltaNewSurfaceFileRerunsWholeUnitConclusions pins the required-evidence
// half of verify conclusion reuse: a new implementation file inside a declared
// surface makes the conclusions whose key now requires that file re-run — the
// acceptance item, the unit-level scan and the architecture judgment — instead
// of carrying a conclusion made before the file existed. The planner and the
// gate's merged freshness check use one rule, so the finalized delta cache must
// be fresh (before this rule was shared, the delta published a cache the gate
// immediately rejected as "has newly required evidence").
func TestDeltaNewSurfaceFileRerunsWholeUnitConclusions(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, runID)
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// A brand-new implementation file joins the item's declared surface.
	grWriteFile(t, repoRoot, "src/new.go", "package auth\n\nfunc NewThing() {}\n")

	deltaRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	for _, key := range []string{"item:auth:auth.core", "unit:auth", "architecture:auth", "code:src/new.go"} {
		if run.CoverageByKey(key) == nil {
			t.Fatalf("a new declared-surface file must re-run %s, got coverage %v", key, coverageKeysOf(run))
		}
		if grContainsString(run.CarriedKeys, key) {
			t.Fatalf("%s must leave the carry set, got %v", key, run.CarriedKeys)
		}
	}

	grSubmitOK(t, repoRoot, deltaRun, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, deltaRun)
	grFinalizeOK(t, repoRoot, deltaRun, "--result", "pass")

	// The gate's own freshness check must accept what the delta just published.
	check, err := checkUnitVerifyMerged(freshDerivation(t, repoRoot), repoRoot, "auth", "candidate")
	if err != nil || !check.Fresh {
		t.Fatalf("the delta cache must be fresh after re-running the conclusions it now requires: %+v %v", check, err)
	}
}

// TestDeltaItemEditCarriesUnchangedItems pins the mechanical verify delta
// scope: editing one acceptance item re-runs that item (and the whole-unit
// conclusions that pin the spec file), while an untouched item carries with no
// review session to adjudicate the carry.
func TestDeltaItemEditCarriesUnchangedItems(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.login", grVerifyItemRegionReport("auth.login", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "auth.logout", grVerifyItemRegionReport("auth.logout", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// Edit only auth.login; auth.logout stays a carry candidate.
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Then it is accepted.", "Then it is accepted, with an edit.", 1)
	if edited == string(data) {
		t.Fatal("item edit did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if grContainsString(coverageKeysOf(run), "review") {
		t.Fatalf("a verify delta plan has no change review, got %v", coverageKeysOf(run))
	}
	if run.CoverageByKey("item:auth:auth.login") == nil {
		t.Fatalf("the edited item must re-run, got %v", coverageKeysOf(run))
	}
	if run.CoverageByKey("item:auth:auth.logout") != nil {
		t.Fatalf("an untouched item must carry, got %v", coverageKeysOf(run))
	}
	if !grContainsString(run.CarriedKeys, "item:auth:auth.logout") {
		t.Fatalf("expected the untouched item among the carry candidates, got %v", run.CarriedKeys)
	}
	if run.Review != nil {
		t.Fatal("a verify delta run records no review")
	}
}

// TestDeltaNewEvidenceNoticeOnPlan pins the verify delta plan disclosure: a
// physical evidence file the baseline never recorded is background material —
// it creates no coverage key and re-runs no conclusion — but the plan names
// the new path so the coordinator can confirm its manifest
// (framework/validation_cache.md §Write Rules → gate-plan).
func TestDeltaNewEvidenceNoticeOnPlan(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grAutoSubmitRemaining(t, repoRoot, runID)
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// Give the delta plan work (an item edit) and supply a new evidence file.
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Then it is accepted.", "Then it is accepted, with an edit.", 1)
	if edited == string(data) {
		t.Fatal("item edit did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, repoRoot, "context/notes.md", "extra context\n")

	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta",
		"--inputs-file", grInputsManifest(t, "context/notes.md"))
	run := mustLoadRun(t, repoRoot, deltaID)
	if !grNoticeContains(run.Notices, "new evidence not in the baseline: context/notes.md") {
		t.Fatalf("the plan must name the newly supplied evidence, got %v", run.Notices)
	}
}

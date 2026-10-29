package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// validateFailureBaseline plans and finalizes a candidate validate full run
// that FAILs on a design finding, returning the repo root and main-spec path.
// relationships names the standing relationships the run checks (or "none").
// The fixture spec's narrative is padded so the promise block sits several
// content-defined chunks below the narrative: a narrative edit then changes a
// chunk that does not cover the promise region, which is what the
// promise-region trigger must distinguish.
func validateFailureBaseline(t *testing.T, relationships string) (string, string) {
	t.Helper()
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	pad, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	var padBuilder strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&padBuilder, "Background narrative line %d carries no contract value and no behavior.\n", i)
	}
	padded := strings.Replace(string(pad), "\n## Promises\n", "\n"+padBuilder.String()+"\n## Promises\n", 1)
	if padded == string(pad) {
		t.Fatal("narrative padding was not applied to the fixture")
	}
	if err := os.WriteFile(main, []byte(padded), 0644); err != nil {
		t.Fatal(err)
	}
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", relationships)
	grSubmitOK(t, root, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Acceptance Items"},
		"6": {main + ": Acceptance Items"},
	}))
	grSubmitOK(t, root, runID, "design", "2. Design soundness: FAIL — contradiction\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n[P1] design — contradiction (actionable)\n\ncheck-2: "+main+": Description\ncheck-4: "+main+": Description\n"+fmt.Sprintf("Finding affects: %s = 2\n", grRunFindingID(runID, "design", 1)))
	grSubmitOK(t, root, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Acceptance Items"}}))
	grSubmitOK(t, root, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, root, runID, main)
	if relationships == "" || relationships == "none" {
		grSubmitOK(t, root, runID, "cross", grCrossReport(main, "Description"))
	} else {
		if _, err := grSubmitRaw(t, root, runID, "cross", relationshipReport(t, root, runID, map[string]string{"coverage_scope": main}, "")); err != nil {
			t.Fatal(err)
		}
	}
	grFinalizeOK(t, root, runID, "--result", "fail", "--p0-count", "1")
	return root, main
}

// TestGateRepairPromiseRegionSynthesis: a content-changing validate repair run
// reviews its change set; a change set intersecting the unit's `## Promises`
// region assigns every gate relationship and requires the final synthesis.
func TestGateRepairPromiseRegionSynthesis(t *testing.T) {
	t.Run("promise-region change assigns relationships", func(t *testing.T) {
		root, main := validateFailureBaseline(t, "coverage_scope")
		data, err := os.ReadFile(main)
		if err != nil {
			t.Fatal(err)
		}
		changed := strings.Replace(string(data), "contract: {invariants: [core]}", "contract: {invariants: [core, extended]}", 1)
		if changed == string(data) {
			t.Fatal("promise edit was not applied to the fixture")
		}
		if err := os.WriteFile(main, []byte(changed), 0644); err != nil {
			t.Fatal(err)
		}
		repair := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair")
		run := mustLoadRun(t, root, repair)
		if got := strings.Join(coverageKeysOf(run), ","); got != "review,design" {
			t.Fatalf("a content-changing repair must review the change set and re-run the failed group, got %v", got)
		}
		if strings.Join(run.Relationships, ",") != "coverage_scope,cross_unit_cohesion,design_constraints" {
			t.Fatalf("a change set touching the `## Promises` region must assign every gate relationship, got %v", run.Relationships)
		}
		noticed := false
		for _, notice := range run.Notices {
			if strings.Contains(notice, "`## Promises` region") {
				noticed = true
			}
		}
		if !noticed {
			t.Fatalf("expected the promise-region trigger notice, got %v", run.Notices)
		}
		if view, err := gateRunSnapshot(root, run); err != nil || view.NextAction != "execute" {
			t.Fatalf("assigned relationships require the synthesis after the re-run; before it the next action is execute: %+v %v", view, err)
		}
		grReviewAccept(t, root, repair)
		grSubmitPlannedValidateSessions(t, root, repair, main)
		if view, err := gateRunSnapshot(root, mustLoadRun(t, root, repair)); err != nil || view.NextAction != "synthesize" {
			t.Fatalf("assigned relationships require the final synthesis after the review and re-run: %+v %v", view, err)
		}
	})

	t.Run("promise-region change assigns every relationship even when the baseline declared none", func(t *testing.T) {
		root, main := validateFailureBaseline(t, "none")
		data, err := os.ReadFile(main)
		if err != nil {
			t.Fatal(err)
		}
		changed := strings.Replace(string(data), "contract: {invariants: [core]}", "contract: {invariants: [core, extended]}", 1)
		if changed == string(data) {
			t.Fatal("promise edit was not applied to the fixture")
		}
		if err := os.WriteFile(main, []byte(changed), 0644); err != nil {
			t.Fatal(err)
		}
		repair := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair")
		run := mustLoadRun(t, root, repair)
		if strings.Join(run.Relationships, ",") != "coverage_scope,cross_unit_cohesion,design_constraints" {
			t.Fatalf("the promise trigger must not depend on the baseline's standing relationships, got %v", run.Relationships)
		}
		noticed := false
		for _, notice := range run.Notices {
			if strings.Contains(notice, "relationships assigned (coverage_scope, cross_unit_cohesion, design_constraints)") {
				noticed = true
			}
		}
		if !noticed {
			t.Fatalf("the trigger notice must name the assigned relationships, got %v", run.Notices)
		}
	})

	t.Run("narrative-only change assigns no relationships", func(t *testing.T) {
		root, main := validateFailureBaseline(t, "coverage_scope")
		data, err := os.ReadFile(main)
		if err != nil {
			t.Fatal(err)
		}
		changed := strings.Replace(string(data), "Prose.", "Revised prose.", 1)
		if changed == string(data) {
			t.Fatal("narrative edit was not applied to the fixture")
		}
		if err := os.WriteFile(main, []byte(changed), 0644); err != nil {
			t.Fatal(err)
		}
		repair := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair")
		run := mustLoadRun(t, root, repair)
		if len(run.Relationships) != 0 {
			t.Fatalf("a narrative-only change must not assign relationships, got %v", run.Relationships)
		}
		if got := strings.Join(coverageKeysOf(run), ","); got != "review,design" {
			t.Fatalf("the review and the failed group re-run, got %v", got)
		}
	})
}

// TestGateRepairFindingsRequireSynthesis: a validate repair run that still
// retains a blocking finding runs the final synthesis — the synthesis is where
// the Check 2 Step 1 / Check 5 extraction artifacts are re-verified — and then
// writes a failure record carrying the sessions' findings.
func TestGateRepairFindingsRequireSynthesis(t *testing.T) {
	root, main := validateFailureBaseline(t, "none")
	repair := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, root, repair)
	if len(run.Relationships) != 0 {
		t.Fatalf("a repair run without a promise-region change and no explicit relationships has none, got %v", run.Relationships)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "design" {
		t.Fatalf("expected the failed group, got %v", got)
	}
	// The re-run still FAILs: the session's P1 requires the final synthesis.
	grSubmitOK(t, root, repair, "design", "2. Design soundness: FAIL — contradiction\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n[P1] design — contradiction (actionable)\n\ncheck-2: "+main+": Description\ncheck-4: "+main+": Description\n"+fmt.Sprintf("Finding affects: %s = 2\n", grRunFindingID(repair, "design", 1)))
	states, err := gaterun.LoadSessionStates(root, run)
	if err != nil {
		t.Fatal(err)
	}
	if action := gateNextAction(run, states, nil); action != "synthesize" {
		t.Fatalf("a blocking repair finding requires the final synthesis — expected synthesize, got %q", action)
	}
	cross := grCompleteCrossReport(t, root, run, "Cross-check: PASS — findings disposed\n")
	if _, err := grSubmitRaw(t, root, repair, "cross", cross); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, repair, "--result", "fail", "--p0-count", "1")
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "basis: repair") || !strings.Contains(cache, "blocking: true") {
		t.Fatalf("expected a repair failure record, got:\n%s", cache)
	}
	if !strings.Contains(cache, "review_result: skipped (repair — mechanical floor scope)") {
		t.Fatalf("expected the repair review-skipped record, got:\n%s", cache)
	}
}

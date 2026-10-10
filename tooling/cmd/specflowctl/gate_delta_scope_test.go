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
	grAutoSubmitQuality(t, repoRoot, runID)
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
	if !grContainsString(coverageKeysOf(run), "review") {
		t.Fatalf("expected the review session in the delta plan, got %v", coverageKeysOf(run))
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

	// Complete the run and confirm the cache records the declined candidates
	// (the scope decision is explicit beyond review_recheck).
	grReviewAccept(t, repoRoot, deltaRun)
	grSubmitOK(t, repoRoot, deltaRun, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitQuality(t, repoRoot, deltaRun)
	grSubmitOK(t, repoRoot, deltaRun, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, deltaRun, "--result", "pass")
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "review_declined:") {
		t.Fatalf("expected the declined candidates recorded in the cache, got:\n%s", cache)
	}
	for _, key := range []string{"code:src/auth.go", "design:auth:src/auth.go"} {
		if !strings.Contains(cache, key) {
			t.Fatalf("expected the declined candidate %s recorded in the cache, got:\n%s", key, cache)
		}
	}
}

// TestDeltaChangedEvidenceFileDoesNotForceEveryCodeKey pins the other half of
// issue #76: a supplied evidence file is readable but is not a public code
// record's dependency, so changing it does not mechanically invalidate every
// public code judgment of the run. A newly supplied path still re-runs the
// keys whose recorded input surface does not cover it.
func TestDeltaChangedEvidenceFileDoesNotForceEveryCodeKey(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")
	grWriteFile(t, repoRoot, "helper.go", "package auth\n\nfunc helper() {}\n")
	inputs := grInputsManifest(t, "helper.go")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", inputs)
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemRegionReport("auth.core", main, "src/auth.go"))
	grAutoSubmitQuality(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// Change only the supplied evidence file.
	grWriteFile(t, repoRoot, "helper.go", "package auth\n\nfunc helper() {}\n\nfunc helper2() {}\n")

	deltaRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--inputs-file", inputs)
	run := mustLoadRun(t, repoRoot, deltaRun)
	if !grContainsString(coverageKeysOf(run), "review") {
		t.Fatalf("expected the review session in the delta plan, got %v", coverageKeysOf(run))
	}
	if run.CoverageByKey("code:src/auth.go") != nil {
		t.Fatalf("a changed supplied evidence file must not mechanically re-run the code judgment, got %v", coverageKeysOf(run))
	}
	if !grContainsString(run.CarriedKeys, "code:src/auth.go") {
		t.Fatalf("expected code:src/auth.go among the carry candidates, got %v", run.CarriedKeys)
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
	grAutoSubmitQuality(t, repoRoot, runID)
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

// TestDeltaReviewRecheckNormalizesShortKeys pins the accepted scope-decision
// record for issue #76 acceptance criterion 3: a verify key may be named by
// its short item id, but the recorded review_recheck and the derived
// review_declined must agree on one spelling. A key the review re-ran by a
// short name must be stored canonical and must not also appear as declined.
func TestDeltaReviewRecheckNormalizesShortKeys(t *testing.T) {
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

	// The reviewer names the carried item by its short id, an accepted verify
	// spelling.
	grReviewRecheck(t, repoRoot, deltaRun, "auth.logout")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if run.Review == nil {
		t.Fatal("expected a recorded review")
	}
	if !grContainsString(run.Review.Recheck, "item:auth:auth.logout") {
		t.Fatalf("the short recheck key must be stored canonical, got %v", run.Review.Recheck)
	}
	if declined := declinedReviewCandidates(run.Review); grContainsString(declined, "item:auth:auth.logout") {
		t.Fatalf("a rechecked key must not also be recorded as declined, got %v", declined)
	}
}

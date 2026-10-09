package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// TestE2EReviewLifecycle walks the complete gate lifecycle with the real
// command entry points: full validate + verify, a spec change, delta review
// (accept for validate, forced re-runs for verify), and a promote that
// consumes the reviewed delta caches.
func TestE2EReviewLifecycle(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	specPath := grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.core"})
	grWriteFile(t, root, "src/auth.go", "package auth\n\nfunc Core() {}\n")
	main := "docs/specs/units/candidate/unit_auth.md"

	// Full validate + verify publish fresh caches.
	validateID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, root, validateID, main)
	grFinalizeOK(t, root, validateID)

	verifyID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, root, verifyID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grAutoSubmitQuality(t, root, verifyID)
	grFinalizeOK(t, root, verifyID)

	if out, err := freshRun(t, root, "--unit", "auth"); err != nil || !strings.Contains(out, "READY FOR PROMOTE: yes") {
		t.Fatalf("fresh before the change: %v\n%s", err, out)
	}

	// The spec changes: both gates go STALE and report the change set.
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose. Edited.", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := freshRun(t, root, "--unit", "auth")
	if err != nil || !strings.Contains(out, "validate  STALE") || !strings.Contains(out, "CHANGE REPORT (validate):") {
		t.Fatalf("expected stale gates with change reports: %v\n%s", err, out)
	}

	// Delta validate: the reviewer accepts the change; the cache is rewritten
	// with the recorded review.
	deltaValidate := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	grReviewAccept(t, root, deltaValidate)
	grFinalizeOK(t, root, deltaValidate)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "basis: delta") || !strings.Contains(cache, "review_result: accept") || !strings.Contains(cache, "reviewed_change_set") {
		t.Fatalf("expected a reviewed delta cache, got:\n%s", cache)
	}

	// Delta verify: the mechanical floor forces the affected item/design/
	// architecture records; the reviewer accepts and the forced sessions run.
	deltaVerify := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, root, deltaVerify)
	if !grContainsString(coverageKeysOf(deltaRun), "review") {
		t.Fatalf("expected the review session in the delta plan, got %v", coverageKeysOf(deltaRun))
	}
	grReviewAccept(t, root, deltaVerify)
	deltaRun = mustLoadRun(t, root, deltaVerify)
	for _, ck := range deltaRun.Coverage {
		if ck.Kind == gaterun.SessionKindItem {
			grSubmitOK(t, root, deltaVerify, ck.Key, grVerifyItemReport(ck.Item, main, "src/auth.go"))
		}
	}
	grAutoSubmitQuality(t, root, deltaVerify)
	grFinalizeOK(t, root, deltaVerify)

	// Both reviewed delta caches satisfy the promote boundary.
	out, err = freshRun(t, root, "--unit", "auth")
	if err != nil || !strings.Contains(out, "READY FOR PROMOTE: yes") {
		t.Fatalf("fresh after the reviewed deltas: %v\n%s", err, out)
	}
	promoteOut, err := publicationPromote(t, root, "unit", "auth")
	if err != nil {
		t.Fatalf("promote rejected reviewed delta caches: %v\n%s", err, promoteOut)
	}
	cache = grReadCache(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "target: stable") {
		t.Fatalf("expected the confirmation cache rewrite after promote, got:\n%s", cache)
	}
}

// TestE2EDeltaReviewEscalates pins the escalation path: an escalate-full
// review abandons the run and finalize refuses it.
func TestE2EDeltaReviewEscalates(t *testing.T) {
	root := createCLITestRepo(t)
	specPath := grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, root, "src/auth.go", "package auth\n")

	validateID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, root, validateID, main)
	grFinalizeOK(t, root, validateID)

	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose. Edited.", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	deltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	grSubmitOK(t, root, deltaID, "review", "Review result: escalate-full — rewrite-scale change\n")
	if _, err := grFinalize(t, root, deltaID); err == nil || !strings.Contains(err.Error(), "escalated") {
		t.Fatalf("expected finalize to refuse the escalated run, got %v", err)
	}
}

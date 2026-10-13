package main

import (
	"bytes"
	"encoding/json"
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
	submitOut := grSubmitOK(t, root, deltaID, "review", "Review result: escalate-full — rewrite-scale change\n")
	if !strings.Contains(submitOut, "abandon") || !strings.Contains(submitOut, "--mode full") {
		t.Fatalf("escalate-full submit must point at the full run, got:\n%s", submitOut)
	}
	if finOut, finErr := grFinalize(t, root, deltaID); finErr == nil || !strings.Contains(finErr.Error(), "escalated") {
		t.Fatalf("expected finalize to refuse the escalated run, got %v\n%s", finErr, finOut)
	} else if !strings.Contains(finErr.Error(), "--relationships") {
		// The refusal clue must be a runnable command: a full unit run requires
		// --relationships, so the shared message has to carry the flag. Without the
		// flag gate-plan rejects the plan, which would strand the coordinator on the
		// single recovery step the message exists to name.
		t.Fatalf("the finalize refusal must print a runnable full-run plan, got %v", finErr)
	}
	var noRel, noRelErr bytes.Buffer
	if err := runGatePlan([]string{"--repo-root", root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "full"}, &noRel, &noRelErr); err == nil || !strings.Contains(err.Error(), "--relationships") {
		t.Fatalf("premise: a full unit plan without --relationships must be rejected, got %v", err)
	}

	// Status reports the escalated state and the abandon action, not synthesize:
	// the recovery point must never route a coordinator into a synthesis the
	// finalize refuses.
	var statusJSON, statusErr bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", root, "--run", deltaID, "--format", "json"}, &statusJSON, &statusErr); err != nil {
		t.Fatalf("gate-status JSON failed: %v (%s)", err, statusErr.String())
	}
	var view gateRunView
	if err := json.Unmarshal(statusJSON.Bytes(), &view); err != nil {
		t.Fatalf("parse gate-status JSON: %v\n%s", err, statusJSON.String())
	}
	if view.NextAction != "abandon" || !view.Escalated {
		t.Fatalf("escalated run must report next_action abandon and escalated true, got next_action=%q escalated=%v", view.NextAction, view.Escalated)
	}

	var statusText, statusTextErr bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", root, "--run", deltaID}, &statusText, &statusTextErr); err != nil {
		t.Fatalf("gate-status text failed: %v", err)
	}
	if !strings.Contains(statusText.String(), "Next:") || !strings.Contains(statusText.String(), "--mode full") || !strings.Contains(statusText.String(), "--relationships") {
		t.Fatalf("expected the text next action to name a runnable full-run plan, got:\n%s", statusText.String())
	}

	// Mission generation is fail-closed on the abandoned run: no new reviewer
	// work is materialized, including the final synthesis.
	var missionOut, missionErr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", deltaID, "--final"}, &missionOut, &missionErr); err == nil || !strings.Contains(err.Error(), "escalated") || !strings.Contains(err.Error(), "--relationships") {
		t.Fatalf("expected gate-mission --final to refuse the escalated run with a runnable plan, got %v", err)
	}

	// Submission is fail-closed on the abandoned run too: a report a reviewer
	// produced before the escalation cannot land on it and record a judgment —
	// for a verify code key PublishPublic would also publish public evidence and
	// a shared-task pointer inside a run that can never finalize. The guard runs
	// before the key and terminal-session checks, so even a re-submission of the
	// accepted review key is refused with the escalated plan, not "already
	// accepted".
	if subOut, subErr := grSubmit(t, root, deltaID, "review", "Review result: accept — late report\n"); subErr == nil || !strings.Contains(subErr.Error(), "escalated") || !strings.Contains(subErr.Error(), "--relationships") {
		t.Fatalf("expected gate-submit to refuse the escalated run with a runnable plan, got %v\n%s", subErr, subOut)
	}

	// The recovery path completes: the named full run plans, and planning it
	// replaces the abandoned delta run.
	fullID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "full")
	if fullID == deltaID {
		t.Fatalf("the full run must replace the abandoned delta run, got the same id %s", fullID)
	}
	if _, err := gaterun.Load(root, deltaID); err == nil {
		t.Fatalf("the abandoned delta run %s must be replaced by the full plan", deltaID)
	}
}

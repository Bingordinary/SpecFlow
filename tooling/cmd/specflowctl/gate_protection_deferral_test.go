package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// Issue #55: a protected stable record that lags its own implementation must
// route to the protected unit (peer-owned record drift) instead of blocking
// the reviewing unit, while a genuinely broken or indeterminate protected
// requirement still blocks.

const protectionDriftKey = "preserve:auth:auth.core"

// protectionDriftFixture verifies auth, publishes a drifted stable record
// (its declared mapping names a moved path while the behavior lives at
// contracts.js), and plans order's verify so order protects auth's stable
// requirement — the multi-unit realignment shape of issue #55. The drifted
// content also makes auth's decision non-reusable, exactly like a realignment
// in flight.
func protectionDriftFixture(t *testing.T) (string, *gaterun.Run) {
	t.Helper()
	root, authSpec, _ := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	driftStableSpec(t, root, authSpec, "docs/specs/units/stable/unit_auth.md", "moved.js")
	if _, err := validationcache.RewriteCachesToStable(root, "unit", "auth"); err != nil {
		t.Fatal(err)
	}
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run := mustLoadRun(t, root, order)
	ck := run.CoverageByKey(protectionDriftKey)
	if ck == nil {
		t.Fatal("fixture has no stable protection for auth")
	}
	if ck.Source == "reused" {
		t.Fatal("a drifted protected record must not be reused")
	}
	return root, run
}

// driftStableSpec writes the verified spec content to the stable layer with its
// declared implementation surface replaced by a moved path. The item still
// declares the shared implementation file through affects.files: current
// declarations are the only protection association, so the stale mapping must
// stay observable to the peer that shares the file.
func driftStableSpec(t *testing.T, root, specPath, stablePath, movedPath string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, specPath))
	if err != nil {
		t.Fatal(err)
	}
	drifted := strings.Replace(string(data), "implementation_surface: contracts.js",
		"implementation_surface: "+movedPath+"\n    affects:\n      files:\n        - contracts.js", 1)
	if drifted == string(data) {
		t.Fatal("fixture could not author the stale declaration")
	}
	grWriteFile(t, root, stablePath, drifted)
}

// protectionDriftReport authors the preserve MISMATCH of peer-owned record
// drift: the declared mapping no longer resolves while the behavior is
// implemented at its current location.
func protectionDriftReport(key, codeFile string) string {
	parts := strings.SplitN(key, ":", 3)
	unit, item := parts[1], parts[2]
	stableSpec := "docs/specs/units/stable/unit_" + unit + ".md"
	return grVerifyItemBody(key, "MISMATCH (scope)", codeFile+":1") +
		fmt.Sprintf("  Problem: the protected stable requirement %s declares an implementation mapping that no longer resolves, while its behavior is implemented at the current location\n  Evidence:\n    - spec: the declared mapping no longer resolves — %s item %s\n    - code: the behavior is implemented at the current location — %s:1\n  Impact: the protected stable record lags its implementation; only the protected unit can refresh it through its own promote\n  Fix: refresh the protected unit's stable mapping in its own promote round\n  Root cause: stale\n  Suggested direction: spec_gap\n  Severity: P1\n  Confidence: high\n", item, stableSpec, item, codeFile) +
		fmt.Sprintf("%s: %s: acceptance_item:%s\n%s: %s: all\n", key, stableSpec, item, key, codeFile)
}

func protectionDriftPreserve(ck gaterun.CoverageKey) string {
	return protectionDriftReport(ck.Key, "contracts.js")
}

func protectionAlignedPreserve(ck gaterun.CoverageKey) string {
	return grVerifyItemReport(ck.Key, "docs/specs/units/stable/unit_"+ck.Unit+".md", "contracts.js")
}

// protectionSubmitRun covers a protection fixture run: every report is healthy
// except the protected stable requirements, whose report the caller supplies.
// It returns the drift finding id when the caller submitted a drift MISMATCH
// for protectionDriftKey.
func protectionSubmitRun(t *testing.T, root string, run *gaterun.Run, preserveReport func(gaterun.CoverageKey) string) string {
	t.Helper()
	for _, ck := range run.Coverage {
		states, err := gaterun.LoadSessionStates(root, run)
		if err != nil {
			t.Fatal(err)
		}
		covered, _, err := gaterun.CoverageProgress(run, states)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := covered[ck.Key]; ok {
			continue
		}
		report := grDefaultQualityReport(run, ck)
		switch {
		case ck.Kind == gaterun.SessionKindPreserve:
			report = preserveReport(ck)
		case gaterun.IsItemKind(ck.Kind):
			report = grVerifyItemReport(ck.Key, run.RequiredFiles[0], "contracts.js")
		}
		if err := sharedSubmit(t, root, run.RunID, ck.Key, report); err != nil {
			t.Fatalf("%s: %v", ck.Key, err)
		}
	}
	return run.RunID + "/" + protectionDriftKey + "/F1"
}

// protectionDriftRoute submits the final synthesis that routes a drift finding
// to its protected unit, keeping the reviewing unit passing.
func protectionDriftRoute(t *testing.T, root string, run *gaterun.Run, driftID, owner, evidence, reason string) {
	t.Helper()
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + driftID + " = retained\n" +
		"Finding ownership: " + driftID + " = owned_by " + owner + " — evidence: " + evidence + "; reason: " + reason + "\n" +
		"Effective status: " + protectionDriftKey + " = pass\n" +
		"cross: " + evidence + ": all\n"
	grSubmitOK(t, root, run.RunID, "cross", report)
	grFinalizeOK(t, root, run.RunID)
}

func TestProtectionDriftRoutesToProtectedUnit(t *testing.T) {
	root, run := protectionDriftFixture(t)
	driftID := protectionSubmitRun(t, root, run, protectionDriftPreserve)
	protectionDriftRoute(t, root, run, driftID, "auth", "contracts.js", "auth's stable mapping lags its implementation")

	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/order/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "blocking: false") || !strings.Contains(cache, "p1_count: 0") {
		t.Fatalf("routed protection drift blocked order:\n%s", cache)
	}
	baseline := grReadJudgmentBaseline(t, root, "unit", "order", "verify")
	if baseline.LogicalStatus[protectionDriftKey] != "pass" {
		t.Fatalf("routed protection drift marked its key failed: %v", baseline.LogicalStatus)
	}
	if len(baseline.Findings) != 0 {
		t.Fatalf("routed protection drift entered the gate-driving findings: %+v", baseline.Findings)
	}
	if len(baseline.DeferredFindings) != 1 || baseline.DeferredFindings[0].ID != driftID || baseline.DeferredFindings[0].OwnedBy != "auth" {
		t.Fatalf("routed finding is not recorded for audit: %+v", baseline.DeferredFindings)
	}
	ledger := grReadDeferredLedger(t, root)
	if len(ledger.Entries) != 1 || ledger.Entries[0].FindingID != driftID || ledger.Entries[0].OwnerUnit != "auth" || ledger.Entries[0].SourceUnit != "order" {
		t.Fatalf("routed finding is not pending for the owner: %+v", ledger.Entries)
	}
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("order's routed run is not promote-ready: %+v %v", check, err)
	}
}

func TestProtectionDriftOwnerDisposesDeferral(t *testing.T) {
	root, orderRun := protectionDriftFixture(t)
	driftID := protectionSubmitRun(t, root, orderRun, protectionDriftPreserve)
	protectionDriftRoute(t, root, orderRun, driftID, "auth", "contracts.js", "auth's stable mapping lags its implementation")

	authRunID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	authRun := mustLoadRun(t, root, authRunID)
	if len(authRun.DeferredFindings) != 1 {
		t.Fatalf("owner run did not load the pending deferral: %+v", authRun.DeferredFindings)
	}
	deferred := authRun.DeferredFindings[0]
	if deferred.Finding.ID != driftID || deferred.Finding.SourceKey != "item:auth:auth.core" || deferred.SourceUnit != "order" || deferred.Finding.Detail == "" {
		t.Fatalf("deferral lost its owner-side identity: %+v", deferred)
	}
	protectionSubmitRun(t, root, authRun, protectionAlignedPreserve)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + driftID + " = suppressed — the candidate declares the behavior at its current location and this round's promote refreshes the stale stable mapping\n" +
		"cross: contracts.js: all\n"
	grSubmitOK(t, root, authRunID, "cross", report)
	grFinalizeOK(t, root, authRunID)

	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") {
		t.Fatalf("disposed drift must not block the owner:\n%s", cache)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(validationcache.DeferredLedgerRelPath))); !os.IsNotExist(err) {
		t.Fatalf("expected the consumed ledger to be removed, stat err=%v", err)
	}
}

func TestProtectionDriftRetainedBlocksOwner(t *testing.T) {
	root, orderRun := protectionDriftFixture(t)
	driftID := protectionSubmitRun(t, root, orderRun, protectionDriftPreserve)
	protectionDriftRoute(t, root, orderRun, driftID, "auth", "contracts.js", "auth's stable mapping lags its implementation")

	authRunID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	authRun := mustLoadRun(t, root, authRunID)
	protectionSubmitRun(t, root, authRun, protectionAlignedPreserve)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + driftID + " = retained\n" +
		"Effective status: item:auth:auth.core = fail\n" +
		"cross: contracts.js: all\n"
	grSubmitOK(t, root, authRunID, "cross", report)
	grFinalizeOK(t, root, authRunID)

	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("retained drift must block the owner:\n%s", cache)
	}
	body := grReadCacheBody(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(body, "Additional retained findings:") || !strings.Contains(body, "declares an implementation mapping that no longer resolves") {
		t.Fatalf("owner cache does not render the retained drift finding:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(validationcache.DeferredLedgerRelPath))); !os.IsNotExist(err) {
		t.Fatalf("expected the consumed ledger to be removed, stat err=%v", err)
	}
}

func TestProtectionDriftMustRouteToProtectedUnit(t *testing.T) {
	root, run := protectionDriftFixture(t)
	driftID := protectionSubmitRun(t, root, run, protectionDriftPreserve)
	for _, owner := range []string{"order", "ghost"} {
		report := "Cross-check: 5/5 PASS — checked\n" +
			"Finding disposition: " + driftID + " = retained\n" +
			"Finding ownership: " + driftID + " = owned_by " + owner + " — evidence: contracts.js; reason: recorded owner\n" +
			"Effective status: " + protectionDriftKey + " = pass\n" +
			"cross: contracts.js: all\n"
		if _, err := grSubmit(t, root, run.RunID, "cross", report); err == nil || !strings.Contains(err.Error(), "stable-record drift must route to the protected unit") {
			t.Fatalf("owner %q: expected a routing rejection, got %v", owner, err)
		}
	}
}

func TestProtectionDriftCannotPassUnrouted(t *testing.T) {
	root, run := protectionDriftFixture(t)
	driftID := protectionSubmitRun(t, root, run, protectionDriftPreserve)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + driftID + " = retained\n" +
		"Effective status: " + protectionDriftKey + " = pass\n" +
		"cross: contracts.js: all\n"
	if _, err := grSubmit(t, root, run.RunID, "cross", report); err == nil || !strings.Contains(err.Error(), "cannot be cleared by synthesis") {
		t.Fatalf("an unrouted protected mismatch passed synthesis: %v", err)
	}
}

func TestProtectionDriftUnroutedBlocksRun(t *testing.T) {
	root, run := protectionDriftFixture(t)
	driftID := protectionSubmitRun(t, root, run, protectionDriftPreserve)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + driftID + " = retained\n" +
		"Effective status: " + protectionDriftKey + " = fail\n" +
		"cross: contracts.js: all\n"
	grSubmitOK(t, root, run.RunID, "cross", report)
	grFinalizeOK(t, root, run.RunID)

	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/order/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("an unrouted protected mismatch must block the reviewing unit:\n%s", cache)
	}
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || check.Fresh || check.Category != validationcache.CategoryBlocked {
		t.Fatalf("genuinely broken protection must stay blocked: %+v %v", check, err)
	}
}

// An indeterminate protected requirement is not record drift: it cannot be
// cleared by routing the projected finding — even the synthetic projection
// sharedStates authors for a reused CANNOT_DETERMINE decision.
func TestProtectionIndeterminateCannotBeRoutedPass(t *testing.T) {
	root, _, _ := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	main := "docs/specs/units/candidate/unit_auth.md"
	report := strings.Replace(grVerifyItemReport("item:auth:auth.core", main, "contracts.js"), "ALIGNED", "CANNOT_DETERMINE", 1)
	if err := sharedSubmit(t, root, auth, "item:auth:auth.core", report); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, auth)
	synthesisPromote(t, root, "auth")

	orderRunID := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run := mustLoadRun(t, root, orderRunID)
	states, err := gaterun.LoadSessionStates(root, run)
	if err != nil {
		t.Fatal(err)
	}
	syntheticID := ""
	for _, state := range states {
		if state.SessionID != protectionDriftKey || state.Result == nil {
			continue
		}
		for _, finding := range state.Result.Findings {
			syntheticID = finding.ID
		}
	}
	if syntheticID == "" {
		t.Fatal("fixture did not reuse the indeterminate protected decision")
	}
	protectionSubmitRun(t, root, run, protectionAlignedPreserve)
	for _, ownership := range []string{
		"",
		"Finding ownership: " + syntheticID + " = owned_by auth — evidence: contracts.js; reason: recorded drift\n",
	} {
		cross := "Cross-check: 5/5 PASS — checked\n" +
			"Finding disposition: " + syntheticID + " = retained\n" + ownership +
			"Effective status: " + protectionDriftKey + " = pass\n" +
			"cross: contracts.js: all\n"
		if _, err := grSubmit(t, root, run.RunID, "cross", cross); err == nil || !strings.Contains(err.Error(), "cannot be cleared by synthesis") {
			t.Fatalf("indeterminate protection passed synthesis (ownership=%q): %v", ownership, err)
		}
	}
}

// The issue's deadlock shape: two units' stable records lag their code. The
// first mover verifies (routing the peer's drift), promotes, and the owner's
// round disposes the loaded drift and promotes — no unit is blocked by a
// record only its owner can refresh, and the ledger is consumed.
func TestMutualDriftReconciliationSequence(t *testing.T) {
	root, authSpec, orderSpec := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)
	driftStableSpec(t, root, authSpec, "docs/specs/units/stable/unit_auth.md", "moved_a.js")
	driftStableSpec(t, root, orderSpec, "docs/specs/units/stable/unit_order.md", "moved_order.js")
	if _, err := validationcache.RewriteCachesToStable(root, "unit", "auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.RewriteCachesToStable(root, "unit", "order"); err != nil {
		t.Fatal(err)
	}

	// First mover: order routes auth's drift and promotes.
	orderRunID := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	orderRun := mustLoadRun(t, root, orderRunID)
	driftID := protectionSubmitRun(t, root, orderRun, protectionDriftPreserve)
	protectionDriftRoute(t, root, orderRun, driftID, "auth", "contracts.js", "auth's stable mapping lags its implementation")
	synthesisPromote(t, root, "order")

	// The owner's round loads the routed drift, disposes it against its current
	// reconciliation, and promotes — refreshing the stale stable record.
	authRunID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	authRun := mustLoadRun(t, root, authRunID)
	if len(authRun.DeferredFindings) != 1 || authRun.DeferredFindings[0].Finding.ID != driftID || authRun.DeferredFindings[0].Finding.SourceKey != "item:auth:auth.core" {
		t.Fatalf("owner round did not load the routed drift: %+v", authRun.DeferredFindings)
	}
	protectionSubmitRun(t, root, authRun, protectionAlignedPreserve)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + driftID + " = suppressed — the candidate declares the behavior at its current location and this round's promote refreshes the stale stable mapping\n" +
		"cross: contracts.js: all\n"
	grSubmitOK(t, root, authRunID, "cross", report)
	grFinalizeOK(t, root, authRunID)
	synthesisPromote(t, root, "auth")

	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(validationcache.DeferredLedgerRelPath))); !os.IsNotExist(err) {
		t.Fatalf("expected the consumed ledger to be removed, stat err=%v", err)
	}
}

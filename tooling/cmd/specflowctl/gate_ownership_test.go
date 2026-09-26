package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// ------------------------------------------------------------
// Ownership / deferred findings fixtures
// ------------------------------------------------------------

// grWriteSharedUnits writes two candidate units (tool, agent) whose declared
// implementation surface is the same src directory, so the shared file carries
// behavior of both units.
func grWriteSharedUnits(t *testing.T, repoRoot string) {
	t.Helper()
	grWriteSpec(t, repoRoot, "tool")
	grWriteSpec(t, repoRoot, "agent")
	grWriteFile(t, repoRoot, "src/shared.go", "package src\n")
	grWriteFile(t, repoRoot, "src/tool_only.go", "package src\n")
}

const grToolMain = "docs/specs/units/candidate/unit_tool.md"
const grAgentMain = "docs/specs/units/candidate/unit_agent.md"

// grDeferSharedFinding runs review@tool and defers the shared file's P1 to the
// agent unit by recorded ownership. It returns the source run id and the
// deferred finding id.
func grDeferSharedFinding(t *testing.T, repoRoot string) (string, string) {
	t.Helper()
	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate")
	finding := "[P1] src/shared.go:1 — timeout summary counts session bookkeeping as executed actions (actionable)"
	report := grReviewArchitecture("unacceptable — shared-file defect") + "\n" + finding +
		"\n  problem: the timeout summary counts SESSION_END bookkeeping as an executed action" +
		"\n  impact: the reported action count is inflated on timeout" +
		"\n  fix: exclude the SESSION_END bookkeeping from the executed-action count" +
		"\n\nsrc/shared.go: " + grToolMain + ": Description\nsrc/shared.go: src/shared.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/shared.go", report)
	grSubmitOK(t, repoRoot, runID, "src/tool_only.go", grReviewReport("src/tool_only.go", grToolMain))
	id := grRunFindingID(runID, "src/shared.go", 1)
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + id + " = retained\n" +
		"Finding ownership: " + id + " = owned_by agent — evidence: " + grToolMain + "; reason: unit_tool.md records the shared-file behavior as agent-owned\n" +
		"cross: " + grToolMain + ": Description\n" +
		"Severity confirmation: " + id + " = confirmed P1 — evidence: " + grToolMain + "; reason: the defect stays blocking for its owner\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)
	return runID, id
}

func grReadDeferredLedger(t *testing.T, repoRoot string) validationcache.DeferredLedger {
	t.Helper()
	ledger, err := validationcache.ReadDeferredLedger(repoRoot)
	if err != nil {
		t.Fatalf("read deferred ledger: %v", err)
	}
	return ledger
}

// grReadCacheBody returns the human-readable portion of a cache file — the
// text after the machine-readable GATE_JUDGMENTS block.
func grReadCacheBody(t *testing.T, repoRoot, rel string) string {
	t.Helper()
	cache := grReadCache(t, repoRoot, rel)
	parts := strings.SplitN(cache, "GATE_JUDGMENTS_END -->", 2)
	if len(parts) != 2 {
		t.Fatalf("cache %s has no judgment marker", rel)
	}
	return parts[1]
}

// ------------------------------------------------------------
// Source side: deferral routes the finding out of this unit's gate
// ------------------------------------------------------------

func TestReviewOwnershipDefersFindingToAnotherUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	runID, id := grDeferSharedFinding(t, repoRoot)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/tool/review_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "blocking: false") || !strings.Contains(cache, "p1_count: 0") {
		t.Fatalf("expected the deferred P1 not to block tool, got:\n%s", cache)
	}
	if !strings.Contains(cache, "Finding ownership: "+id+" = owned_by agent") {
		t.Fatalf("expected the ownership record in the cache body, got:\n%s", cache)
	}

	baseline := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "tool", gaterun.GateReview)
	if len(baseline.Findings) != 0 {
		t.Fatalf("expected no gate-driving retained findings, got %+v", baseline.Findings)
	}
	if baseline.LogicalStatus["src/shared.go"] != "pass" {
		t.Fatalf("a deferred finding must not mark its key failed, got %v", baseline.LogicalStatus)
	}
	if len(baseline.DeferredFindings) != 1 {
		t.Fatalf("expected one deferred finding in the audit baseline, got %+v", baseline.DeferredFindings)
	}
	deferred := baseline.DeferredFindings[0]
	if deferred.ID != id || deferred.OwnedBy != "agent" || deferred.Severity != "P1" {
		t.Fatalf("unexpected deferred finding record: %+v", deferred)
	}

	ledger := grReadDeferredLedger(t, repoRoot)
	if len(ledger.Entries) != 1 {
		t.Fatalf("expected one pending deferral, got %+v", ledger.Entries)
	}
	entry := ledger.Entries[0]
	if entry.FindingID != id || entry.OwnerUnit != "agent" || entry.SourceUnit != "tool" || entry.SourceRun != runID {
		t.Fatalf("unexpected ledger entry: %+v", entry)
	}
	if entry.EvidencePath != grToolMain || entry.Reason == "" || entry.Detail == "" {
		t.Fatalf("expected the ownership evidence, reason, and finding detail in the ledger, got %+v", entry)
	}
}

func TestReviewOwnershipMixedWithGateDrivingFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate")

	sharedFinding := "[P1] src/shared.go:1 — agent-owned defect (actionable)"
	sharedReport := grReviewArchitecture("unacceptable — shared-file defect") + "\n" + sharedFinding +
		"\n  problem: agent-owned defect\n  impact: inflated count\n  fix: exclude the bookkeeping\n" +
		"\nsrc/shared.go: " + grToolMain + ": Description\nsrc/shared.go: src/shared.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/shared.go", sharedReport)

	ownFinding := "[P1] src/tool_only.go:1 — tool-owned defect (actionable)"
	ownReport := grReviewArchitecture("unacceptable — tool defect") + "\n" + ownFinding +
		"\n  problem: tool-owned defect\n  impact: broken behavior\n  fix: repair the tool path\n" +
		"\nsrc/tool_only.go: " + grToolMain + ": Description\nsrc/tool_only.go: src/tool_only.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/tool_only.go", ownReport)

	sharedID := grRunFindingID(runID, "src/shared.go", 1)
	ownID := grRunFindingID(runID, "src/tool_only.go", 1)
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + sharedID + " = retained\n" +
		"Finding disposition: " + ownID + " = retained\n" +
		"Finding ownership: " + sharedID + " = owned_by agent — evidence: " + grToolMain + "; reason: recorded agent ownership\n" +
		"cross: " + grToolMain + ": Description\n" +
		"Severity confirmation: " + sharedID + " = confirmed P1 — evidence: " + grToolMain + "; reason: owner-side impact\n" +
		"Severity confirmation: " + ownID + " = confirmed P1 — evidence: " + grToolMain + "; reason: tool-side impact\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = fail\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/tool/review_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("expected only the tool-owned P1 to block, got:\n%s", cache)
	}
	baseline := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "tool", gaterun.GateReview)
	if len(baseline.Findings) != 1 || baseline.Findings[0].ID != ownID {
		t.Fatalf("expected the gate-driving finding to stay in the baseline, got %+v", baseline.Findings)
	}
	if len(baseline.DeferredFindings) != 1 || baseline.DeferredFindings[0].ID != sharedID {
		t.Fatalf("expected the agent-owned finding deferred, got %+v", baseline.DeferredFindings)
	}
}

// ------------------------------------------------------------
// Owner side: the next review of the owner disposes the deferral
// ------------------------------------------------------------

func TestOwnerReviewDisposesDeferredFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	agentRunID := grPlan(t, repoRoot, "--gate", "review", "--unit", "agent", "--target", "candidate")
	agentRun := mustLoadRun(t, repoRoot, agentRunID)
	if len(agentRun.DeferredFindings) != 1 || agentRun.DeferredFindings[0].Finding.ID != deferredID {
		t.Fatalf("expected the plan to load the pending deferral, got %+v", agentRun.DeferredFindings)
	}

	// The cross synthesis must dispose the pending deferral: retaining it makes
	// it the owner's finding and blocks the owner's gate.
	grSubmitOK(t, repoRoot, agentRunID, "src/shared.go", grReviewReport("src/shared.go", grAgentMain))
	grSubmitOK(t, repoRoot, agentRunID, "src/tool_only.go", grReviewReport("src/tool_only.go", grAgentMain))

	var packetOut, packetErr bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", agentRunID, "--packet", "cross"}, &packetOut, &packetErr); err != nil {
		t.Fatalf("gate-packet failed: %v (stderr=%s)", err, strings.TrimSpace(packetErr.String()))
	}
	if !strings.Contains(packetOut.String(), deferredID) || !strings.Contains(packetOut.String(), "Pending deferred findings") {
		t.Fatalf("expected the deferred finding in the cross packet context, got:\n%s", packetOut.String())
	}

	// The file packet that reviews the deferred finding's file also carries it.
	var fileOut, fileErr bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", agentRunID, "--packet", "src/shared.go"}, &fileOut, &fileErr); err != nil {
		t.Fatalf("gate-packet failed: %v (stderr=%s)", err, strings.TrimSpace(fileErr.String()))
	}
	if !strings.Contains(fileOut.String(), deferredID) {
		t.Fatalf("expected the deferred finding in the matching file packet context, got:\n%s", fileOut.String())
	}

	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + deferredID + " = retained\n" +
		"cross: " + grAgentMain + ": Description\n" +
		"cross: src/shared.go: all\n" +
		"Severity confirmation: " + deferredID + " = confirmed P1 — evidence: src/shared.go; reason: the defect is in this unit's declared surface\n" +
		"Effective status: src/shared.go = fail\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, agentRunID, "cross", cross)
	grFinalizeOK(t, repoRoot, agentRunID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/agent/review_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("expected the retained deferral to block the owner, got:\n%s", cache)
	}
	if !strings.Contains(cache, "Additional retained findings:") || !strings.Contains(cache, "timeout summary counts session bookkeeping") {
		t.Fatalf("expected the disposed deferral's block in the owner's findings body, got:\n%s", cache)
	}
	body := grReadCacheBody(t, repoRoot, "docs/specs/meta/validation/unit/agent/review_result.md")
	if !strings.Contains(body, "problem: the timeout summary counts SESSION_END bookkeeping as an executed action") {
		t.Fatalf("expected the retained pending deferral's block in the human-readable body, got:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(validationcache.DeferredLedgerRelPath))); !os.IsNotExist(err) {
		t.Fatalf("expected the consumed ledger to be removed, stat err=%v", err)
	}
}

func TestOwnerReviewMustDisposeDeferredFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	agentRunID := grPlan(t, repoRoot, "--gate", "review", "--unit", "agent", "--target", "candidate")
	grSubmitOK(t, repoRoot, agentRunID, "src/shared.go", grReviewReport("src/shared.go", grAgentMain))
	grSubmitOK(t, repoRoot, agentRunID, "src/tool_only.go", grReviewReport("src/tool_only.go", grAgentMain))
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"cross: " + grAgentMain + ": Description\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, agentRunID, "cross", cross); err == nil || !strings.Contains(err.Error(), deferredID) {
		t.Fatalf("expected the undisposed pending deferral to reject the cross report, got %v", err)
	}
}

func TestOwnerReviewSuppressesDeferredFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	agentRunID := grPlan(t, repoRoot, "--gate", "review", "--unit", "agent", "--target", "candidate")
	grSubmitOK(t, repoRoot, agentRunID, "src/shared.go", grReviewReport("src/shared.go", grAgentMain))
	grSubmitOK(t, repoRoot, agentRunID, "src/tool_only.go", grReviewReport("src/tool_only.go", grAgentMain))
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + deferredID + " = suppressed — the recorded behavior was removed from the file\n" +
		"cross: " + grAgentMain + ": Description\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, agentRunID, "cross", cross)
	grFinalizeOK(t, repoRoot, agentRunID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/agent/review_result.md")
	if !strings.Contains(cache, "result: pass") {
		t.Fatalf("expected a suppressed deferral to leave the owner passing, got:\n%s", cache)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(validationcache.DeferredLedgerRelPath))); !os.IsNotExist(err) {
		t.Fatalf("expected the consumed ledger to be removed, stat err=%v", err)
	}
}

// ------------------------------------------------------------
// Ownership record validation
// ------------------------------------------------------------

func TestOwnershipRecordsAreMechanicallyValidated(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate")
	report := grReviewArchitecture("unacceptable — shared-file defect") + "\n[P1] src/shared.go:1 — shared-file defect (actionable)" +
		"\n  problem: defect\n  impact: impact\n  fix: fix\n" +
		"\nsrc/shared.go: " + grToolMain + ": Description\nsrc/shared.go: src/shared.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/shared.go", report)
	grSubmitOK(t, repoRoot, runID, "src/tool_only.go", grReviewReport("src/tool_only.go", grToolMain))
	id := grRunFindingID(runID, "src/shared.go", 1)
	base := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + id + " = retained\n" +
		"cross: " + grToolMain + ": Description\n" +
		"Severity confirmation: " + id + " = confirmed P1 — evidence: " + grToolMain + "; reason: owner-side impact\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"

	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "evidence outside the read refs",
			line: "Finding ownership: " + id + " = owned_by agent — evidence: docs/elsewhere.md; reason: recorded\n",
			want: "outside packet",
		},
		{
			name: "evidence without a scope declaration",
			line: "Finding ownership: " + id + " = owned_by agent — evidence: src/tool_only.go; reason: recorded\n",
			want: "Dependency scope",
		},
		{
			name: "unknown owner unit",
			line: "Finding ownership: " + id + " = owned_by ghost — evidence: " + grToolMain + "; reason: recorded\n",
			want: "exists in no layer",
		},
		{
			name: "non-terminal finding",
			line: "Finding ownership: " + runID + "/cross/F9 = owned_by agent — evidence: " + grToolMain + "; reason: recorded\n",
			want: "non-terminal",
		},
		{
			name: "malformed line",
			line: "Finding ownership: " + id + " = owned_by agent\n",
			want: "malformed Finding ownership",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := grSubmit(t, repoRoot, runID, "cross", base+tc.line); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestOwnershipRecordsAreReviewOnly(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "tool")
	grWriteSpec(t, repoRoot, "agent")
	grWriteFile(t, repoRoot, "src/a.go", "package src\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "tool", "--target", "candidate")
	run := mustLoadRun(t, repoRoot, runID)
	if len(run.DeferredFindings) != 0 {
		t.Fatalf("expected no deferred findings for a validate run, got %+v", run.DeferredFindings)
	}
}

func TestDeferredFindingMustNameAValidKey(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "src/shared.go", grReviewReport("src/shared.go", grToolMain))
	grSubmitOK(t, repoRoot, runID, "src/tool_only.go", grReviewReport("src/tool_only.go", grToolMain))
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"cross: " + grToolMain + ": Description\n" +
		"[P2] src/shared.go:1 — new cross finding (actionable)\n" +
		"Finding affects: " + runID + "/cross/F1 = docs/not-a-key.go\n" +
		"Finding ownership: " + runID + "/cross/F1 = owned_by agent — evidence: " + grToolMain + "; reason: recorded\n" +
		"Severity confirmation: " + runID + "/cross/F1 = confirmed P2 — evidence: " + grToolMain + "; reason: owner-side impact\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", cross); err == nil || !strings.Contains(err.Error(), "invalid logical key") {
		t.Fatalf("expected a deferred finding with a bogus key to be rejected, got %v", err)
	}
}

func TestCrossFailRequiresGateDrivingCrossFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "src/shared.go", grReviewReport("src/shared.go", grToolMain))
	grSubmitOK(t, repoRoot, runID, "src/tool_only.go", grReviewReport("src/tool_only.go", grToolMain))
	crossFindingID := runID + "/cross/F1"
	cross := "Cross-check: 0/4 FAIL — agent-owned cross defect\n\n" +
		"cross: " + grToolMain + ": Description\n" +
		"[P0] src/shared.go:1 — agent-owned cross defect (actionable)\n" +
		"Finding affects: " + crossFindingID + " = src/shared.go\n" +
		"Finding ownership: " + crossFindingID + " = owned_by agent — evidence: " + grToolMain + "; reason: recorded\n" +
		"Severity confirmation: " + crossFindingID + " = confirmed P0 — evidence: " + grToolMain + "; reason: owner-side impact\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = fail\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", cross); err == nil || !strings.Contains(err.Error(), "owned by this unit or unassigned") {
		t.Fatalf("expected a cross FAIL backed only by a deferred finding to be rejected, got %v", err)
	}
}

// ------------------------------------------------------------
// Delta: deferred findings are not carried, the ledger stays the routing state
// ------------------------------------------------------------

func TestDeferredFindingIsNotCarriedIntoDelta(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	grWriteFile(t, repoRoot, "src/tool_only.go", "package src\n// changed\n")
	deltaID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if got := strings.Join(deltaRun.CarriedKeys, ","); got != "src/shared.go" {
		t.Fatalf("expected src/shared.go to be carried, got %q", got)
	}
	for _, result := range deltaRun.CarriedResults {
		for _, finding := range result.Findings {
			if finding.ID == deferredID {
				t.Fatalf("a deferred finding must not be carried into a later run of the same unit: %+v", finding)
			}
		}
	}

	grSubmitOK(t, repoRoot, deltaID, "src/tool_only.go", grReviewReport("src/tool_only.go", grToolMain))
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"cross: " + grToolMain + ": Description\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, deltaID, "cross", cross)
	grFinalizeOK(t, repoRoot, deltaID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/tool/review_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected a passing delta cache, got:\n%s", cache)
	}
	ledger := grReadDeferredLedger(t, repoRoot)
	if len(ledger.Entries) != 1 || ledger.Entries[0].FindingID != deferredID {
		t.Fatalf("expected the pending deferral to survive the delta run, got %+v", ledger.Entries)
	}
}

func TestFreshReportsPendingDeferrals(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	var stdout, stderr bytes.Buffer
	if err := runFresh([]string{"--repo-root", repoRoot, "--unit", "agent"}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh failed: %v (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	if !strings.Contains(stdout.String(), "deferred finding(s) pending") || !strings.Contains(stdout.String(), deferredID) {
		t.Fatalf("expected the pending deferral in the fresh report, got:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := runFresh([]string{"--repo-root", repoRoot, "--unit", "tool"}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh failed: %v (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	if strings.Contains(stdout.String(), "deferred finding(s) pending") {
		t.Fatalf("a unit with no pending deferrals must not report any, got:\n%s", stdout.String())
	}
}

func TestReDeferralMovesLedgerEntryToTheNewOwner(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	grWriteSpec(t, repoRoot, "contracts")
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	agentRunID := grPlan(t, repoRoot, "--gate", "review", "--unit", "agent", "--target", "candidate")
	grSubmitOK(t, repoRoot, agentRunID, "src/shared.go", grReviewReport("src/shared.go", grAgentMain))
	grSubmitOK(t, repoRoot, agentRunID, "src/tool_only.go", grReviewReport("src/tool_only.go", grAgentMain))
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + deferredID + " = retained\n" +
		"Finding ownership: " + deferredID + " = owned_by contracts — evidence: " + grAgentMain + "; reason: the contracts unit records this boundary\n" +
		"cross: " + grAgentMain + ": Description\n" +
		"cross: src/shared.go: all\n" +
		"Severity confirmation: " + deferredID + " = confirmed P1 — evidence: src/shared.go; reason: owner-side impact\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, agentRunID, "cross", cross)
	grFinalizeOK(t, repoRoot, agentRunID)

	ledger := grReadDeferredLedger(t, repoRoot)
	if len(ledger.Entries) != 1 {
		t.Fatalf("expected exactly one pending deferral after re-routing, got %+v", ledger.Entries)
	}
	entry := ledger.Entries[0]
	if entry.FindingID != deferredID || entry.OwnerUnit != "contracts" || entry.SourceUnit != "agent" || entry.SourceRun != agentRunID {
		t.Fatalf("expected the entry re-routed to contracts from the agent run, got %+v", entry)
	}
}

// TestDeferredCarriedFindingRendersIntoCacheBody covers the delta path where
// the cross synthesis defers a finding carried over from the baseline: no
// packet report authors the finding, yet its complete block must stay in the
// human-readable body exactly once (the machine block alone is not the record).
func TestDeferredCarriedFindingRendersIntoCacheBody(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)

	fullID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate")
	sharedFinding := "[P1] src/shared.go:1 — shared-file defect (actionable)"
	sharedReport := grReviewArchitecture("unacceptable — shared-file defect") + "\n" + sharedFinding +
		"\n  problem: shared-file defect\n  impact: inflated count\n  fix: exclude the bookkeeping\n" +
		"\nsrc/shared.go: " + grToolMain + ": Description\nsrc/shared.go: src/shared.go: all\n"
	grSubmitOK(t, repoRoot, fullID, "src/shared.go", sharedReport)

	ownFinding := "[P2] src/tool_only.go:1 — local maintainability concern (actionable)"
	ownDetail := "  spec_context: the boundary remains correct\n  recommendation: simplify the helper"
	ownReport := grReviewArchitecture("needs_attention") + "\n" + ownFinding + "\n" + ownDetail +
		"\n\nsrc/tool_only.go: " + grToolMain + ": Description\nsrc/tool_only.go: src/tool_only.go: all\n"
	grSubmitOK(t, repoRoot, fullID, "src/tool_only.go", ownReport)

	sharedID := grRunFindingID(fullID, "src/shared.go", 1)
	ownID := grRunFindingID(fullID, "src/tool_only.go", 1)
	cross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + sharedID + " = retained\n" +
		"Finding disposition: " + ownID + " = retained\n" +
		"Finding ownership: " + sharedID + " = owned_by agent — evidence: " + grToolMain + "; reason: recorded agent ownership\n" +
		"cross: " + grToolMain + ": Description\n" +
		"Severity confirmation: " + sharedID + " = confirmed P1 — evidence: " + grToolMain + "; reason: owner-side impact\n" +
		"Severity confirmation: " + ownID + " = confirmed P2 — evidence: " + grToolMain + "; reason: impact remains local\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = fail\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, fullID, "cross", cross)
	grFinalizeOK(t, repoRoot, fullID)

	// A later delta re-reviews the shared file, carries src/tool_only.go's P2
	// finding, and the cross defers that carried finding to the agent unit.
	grWriteFile(t, repoRoot, "src/shared.go", "package src\n// changed\n")
	deltaID := grPlan(t, repoRoot, "--gate", "review", "--unit", "tool", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if got := strings.Join(deltaRun.CarriedKeys, ","); got != "src/tool_only.go" {
		t.Fatalf("expected src/tool_only.go to be carried, got %q", got)
	}
	grSubmitOK(t, repoRoot, deltaID, "src/shared.go", grReviewReport("src/shared.go", grToolMain))
	deltaCross := "Cross-check: 4/4 PASS — consistent\n\n" +
		"Finding disposition: " + ownID + " = retained\n" +
		"Finding ownership: " + ownID + " = owned_by agent — evidence: " + grToolMain + "; reason: recorded agent ownership\n" +
		"cross: " + grToolMain + ": Description\n" +
		"Severity confirmation: " + ownID + " = confirmed P2 — evidence: " + grToolMain + "; reason: impact remains local\n" +
		"Effective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, deltaID, "cross", deltaCross)
	grFinalizeOK(t, repoRoot, deltaID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/tool/review_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "p2_count: 0") {
		t.Fatalf("expected the deferred carried finding not to block or count, got:\n%s", cache)
	}
	body := grReadCacheBody(t, repoRoot, "docs/specs/meta/validation/unit/tool/review_result.md")
	if strings.Count(body, ownFinding) != 1 || !strings.Contains(body, ownDetail) {
		t.Fatalf("expected the deferred carried finding's block exactly once in the human-readable body, got:\n%s", body)
	}
}

package main

import (
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

const grToolMain = "docs/specs/units/candidate/unit_tool.md"
const grAgentMain = "docs/specs/units/candidate/unit_agent.md"

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

// grDeferSharedFinding runs verify@tool and defers a new cross finding over the
// shared file to the agent unit by recorded ownership. It returns the source
// run id and the deferred finding id.
func grDeferSharedFinding(t *testing.T, repoRoot string) (string, string) {
	t.Helper()
	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "tool", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "tool.core", grVerifyItemReport("tool.core", grToolMain, "src/shared.go"))
	quality := strings.Replace(grQualityReport("src/shared.go", grToolMain), "conclusion: acceptable", "conclusion: unacceptable — shared-file defect", 1)
	quality = strings.Replace(quality, "gate_findings: none", "gate_findings: [P1] shared-file defect", 1)
	quality += "[P1] src/shared.go:1 — shared-file defect (actionable)\n  problem: shared-file defect\n  evidence: package src\n  impact: caller behavior breaks\n  fix: correct the shared behavior\n"
	grSubmitOK(t, repoRoot, runID, "src/shared.go", quality)
	id := grRunFindingID(runID, "src/shared.go", 1)
	cross := "Cross-check: 5/5 PASS — synthesis complete\n\n" +
		"Finding disposition: " + id + " = retained\n" +
		"Finding ownership: " + id + " = owned_by agent — evidence: src/shared.go; reason: unit_agent.md records the shared-file behavior as agent-owned\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: tool.core = pass\n" +
		"Effective status: src/shared.go = pass\n" +
		"Effective status: src/tool_only.go = pass\n" +
		"Effective status: cross = pass\n"
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

func TestVerifyOwnershipDefersFindingToAnotherUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	runID, id := grDeferSharedFinding(t, repoRoot)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/tool/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "blocking: false") || !strings.Contains(cache, "p1_count: 0") {
		t.Fatalf("expected the deferred P1 not to block tool, got:\n%s", cache)
	}
	if !strings.Contains(cache, "Finding ownership: "+id+" = owned_by agent") {
		t.Fatalf("expected the ownership record in the cache body, got:\n%s", cache)
	}

	baseline := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "tool", gaterun.GateVerify)
	if len(baseline.Findings) != 0 {
		t.Fatalf("expected no gate-driving retained findings, got %+v", baseline.Findings)
	}
	if baseline.LogicalStatus["design:tool:src/shared.go"] != "pass" {
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
	if entry.EvidencePath != "src/shared.go" || entry.Reason == "" || entry.Detail == "" {
		t.Fatalf("expected the ownership evidence, reason, and finding detail in the ledger, got %+v", entry)
	}
}

// ------------------------------------------------------------
// Ownership record validation
// ------------------------------------------------------------

func TestOwnershipRecordsAreMechanicallyValidated(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "tool", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "tool.core", grVerifyItemReport("tool.core", grToolMain, "src/shared.go"))
	id := grRunFindingID(runID, "cross", 1)
	base := "Cross-check: 5/5 PASS — synthesis complete\n\n" +
		"[P1] src/shared.go:1 — shared-file defect (actionable)\n" +
		"Finding affects: " + id + " = src/shared.go\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: tool.core = pass\n" +
		"Effective status: src/shared.go = pass\n" +
		"Effective status: src/tool_only.go = pass\n" +
		"Effective status: cross = pass\n"

	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "evidence outside the read refs",
			line: "Finding ownership: " + id + " = owned_by agent — evidence: docs/elsewhere.md; reason: recorded\n",
			want: "outside session",
		},
		{
			name: "evidence without a cross scope declaration",
			line: "Finding ownership: " + id + " = owned_by agent — evidence: " + grToolMain + "; reason: recorded\n",
			want: "Dependency scope",
		},
		{
			name: "unknown owner unit",
			line: "Finding ownership: " + id + " = owned_by ghost — evidence: src/shared.go; reason: recorded\n",
			want: "exists in no layer",
		},
		{
			name: "non-terminal finding",
			line: "Finding ownership: " + runID + "/cross/F9 = owned_by agent — evidence: src/shared.go; reason: recorded\n",
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

// ------------------------------------------------------------
// Owner side: the next verify of the owner disposes the deferral
// ------------------------------------------------------------

func TestOwnerVerifyDisposesDeferredFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	agentRunID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "agent", "--target", "candidate")
	agentRun := mustLoadRun(t, repoRoot, agentRunID)
	if len(agentRun.DeferredFindings) != 1 || agentRun.DeferredFindings[0].Finding.ID != deferredID {
		t.Fatalf("expected the plan to load the pending deferral, got %+v", agentRun.DeferredFindings)
	}
	if agentRun.DeferredFindings[0].SourceUnit != "tool" || agentRun.DeferredFindings[0].Finding.Detail == "" {
		t.Fatalf("plan lost pending deferral details: %+v", agentRun.DeferredFindings)
	}

	grSubmitOK(t, repoRoot, agentRunID, "agent.core", grVerifyItemReport("agent.core", grAgentMain, "src/shared.go"))
	cross := "Cross-check: 5/5 PASS — synthesis complete\n\n" +
		"Finding disposition: " + deferredID + " = retained\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: agent.core = pass\n" +
		"Effective status: src/shared.go = fail\n" +
		"Effective status: src/tool_only.go = pass\n" +
		"Effective status: cross = pass\n"
	grSubmitOK(t, repoRoot, agentRunID, "cross", cross)
	grFinalizeOK(t, repoRoot, agentRunID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/agent/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("expected the retained deferral to block the owner, got:\n%s", cache)
	}
	body := grReadCacheBody(t, repoRoot, "docs/specs/meta/validation/unit/agent/verify_result.md")
	if !strings.Contains(body, "Additional retained findings:") || !strings.Contains(body, "shared-file defect") {
		t.Fatalf("expected the disposed deferral's block in the owner's findings body, got:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(validationcache.DeferredLedgerRelPath))); !os.IsNotExist(err) {
		t.Fatalf("expected the consumed ledger to be removed, stat err=%v", err)
	}
}

func TestOwnerVerifyMustDisposeDeferredFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	agentRunID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "agent", "--target", "candidate")
	grSubmitOK(t, repoRoot, agentRunID, "agent.core", grVerifyItemReport("agent.core", grAgentMain, "src/shared.go"))
	cross := "Cross-check: 5/5 PASS — synthesis complete\n\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: agent.core = pass\n" +
		"Effective status: src/shared.go = pass\n" +
		"Effective status: src/tool_only.go = pass\n" +
		"Effective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, agentRunID, "cross", cross); err == nil || !strings.Contains(err.Error(), deferredID) {
		t.Fatalf("expected the undisposed pending deferral to reject the cross report, got %v", err)
	}
}

func TestOwnerVerifySuppressesDeferredFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSharedUnits(t, repoRoot)
	_, deferredID := grDeferSharedFinding(t, repoRoot)

	agentRunID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "agent", "--target", "candidate")
	grSubmitOK(t, repoRoot, agentRunID, "agent.core", grVerifyItemReport("agent.core", grAgentMain, "src/shared.go"))
	cross := "Cross-check: 5/5 PASS — synthesis complete\n\n" +
		"Finding disposition: " + deferredID + " = suppressed — the recorded behavior was removed from the file\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: agent.core = pass\n" +
		"Effective status: src/shared.go = pass\n" +
		"Effective status: src/tool_only.go = pass\n" +
		"Effective status: cross = pass\n"
	grSubmitOK(t, repoRoot, agentRunID, "cross", cross)
	grFinalizeOK(t, repoRoot, agentRunID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/agent/verify_result.md")
	if !strings.Contains(cache, "result: pass") {
		t.Fatalf("expected a suppressed deferral to leave the owner passing, got:\n%s", cache)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(validationcache.DeferredLedgerRelPath))); !os.IsNotExist(err) {
		t.Fatalf("expected the consumed ledger to be removed, stat err=%v", err)
	}
}

// ------------------------------------------------------------
// Malformed ledger: the plan fails closed
// ------------------------------------------------------------

func TestMalformedDeferredLedgerFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "invalid json", content: "{not json"},
		{name: "unsupported schema", content: `{"schema_version": 99, "entries": []}`},
		{name: "invalid entry", content: `{"schema_version": 1, "entries": [{"finding_id": "run/cross/F1", "owner_unit": "", "source_unit": "tool", "source_run": "run", "severity": "P1", "text": "t", "detail": "d", "source_key": "cross", "affected_keys": ["src/shared.go"], "evidence_path": "src/shared.go", "reason": "r"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := createCLITestRepo(t)
			grWriteSharedUnits(t, repoRoot)
			grWriteFile(t, repoRoot, validationcache.DeferredLedgerRelPath, tc.content)

			if _, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "agent", "--target", "candidate"); err == nil || !strings.Contains(err.Error(), "deferred-findings ledger") {
				t.Fatalf("expected the malformed ledger to fail the plan closed, got %v", err)
			}
		})
	}
}

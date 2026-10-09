package main

import (
	"fmt"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// TestGateRunCoBatchedRetainedObservation pins the co-batched design retain
// path: a retained public observation must become this unit's own design
// finding with a session-unique report-order id, alongside the design block's
// own finding. The shared report-order counter spans the code block too, so a
// retained observation must not reuse the design block's finding id, and it
// must be attributed to the design key (never the code key).
func TestGateRunCoBatchedRetainedObservation(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))

	keys := []string{"code:src/auth.go", "design:auth:src/auth.go"}
	obsID := fmt.Sprintf("%s/%s/F1", runID, gaterun.SessionID(keys))
	report := "File: code:src/auth.go\n" +
		"conclusion: FACTS\n" +
		"facts: token shape may discard caller identity\n" +
		"[P1] src/auth.go:1 — token shape may discard caller identity (actionable)\n" +
		"  problem: the token shape differs between callers\n" +
		"  evidence: caller requires token\n" +
		"  impact: login may fail\n" +
		"  fix: reconcile the contract\n" +
		"\n" +
		"File: design:auth:src/auth.go\n" +
		"conclusion: unacceptable — the retained observation blocks the design\n" +
		"spec_requirements: token behavior required by the auth design\n" +
		"gate_findings: [P1] token shape\n" +
		"Observation disposition: " + obsID + " = retained — the unit design cannot accept the discarded identity\n" +
		"[P2] src/auth.go:2 — responsibility placement is unclear (actionable)\n" +
		"  problem: responsibility placement is unclear\n" +
		"  evidence: package auth\n" +
		"  impact: maintainers may misplace the change\n" +
		"  fix: document the boundary\n"
	grSubmitKeys(t, repoRoot, runID, keys, report)

	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := grLoadSessionState(repoRoot, run, gaterun.SessionID(keys))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Result.Findings) != 2 {
		t.Fatalf("want the design finding and the retained observation, got %d: %+v", len(state.Result.Findings), state.Result.Findings)
	}
	seen := map[string]bool{}
	retained := false
	for _, f := range state.Result.Findings {
		if seen[f.ID] {
			t.Fatalf("co-batched finding ids collide at %q: %+v", f.ID, state.Result.Findings)
		}
		seen[f.ID] = true
		if f.SourceKey == "design:auth:src/auth.go" && f.Severity == "P1" {
			retained = true
		}
	}
	if !retained {
		t.Fatalf("retained observation must be this unit's P1 design finding: %+v", state.Result.Findings)
	}

	// The retained unit finding must not leak into the code key's public
	// record: the public record keeps the observation, not the unit finding.
	designCk := run.CoverageByKey("design:auth:src/auth.go")
	public, err := gaterun.PublicResultForDesignKey(repoRoot, run, *designCk)
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Findings) != 0 {
		t.Fatalf("public code record must not carry the unit's design finding: %+v", public.Findings)
	}
}

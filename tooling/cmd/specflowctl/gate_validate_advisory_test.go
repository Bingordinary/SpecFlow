package main

import (
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// TestValidateNonBlockingAdvisory: a validate P2 finding is a non-blocking
// advisory. It is recorded in the cache (p2_count) with the gate result PASS,
// it never blocks promote, and a delta run carries it with its check's carried
// judgment instead of re-raising it.
func TestValidateNonBlockingAdvisory(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	grSubmitOK(t, root, runID, "document", grValidateReport([]string{"1", gaterun.ClarityCheck}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Acceptance Items"},
		"6": {main + ": Acceptance Items"},
	}))
	advisory := "[P2] " + main + ": Description — the same statement is carried twice (actionable)\n" +
		"  problem: the same invariant is stated in two places\n" +
		"  evidence: \"invariants: [core]\" — Promises\n" +
		"  impact: a future edit must land in two places or reopen a contradiction\n" +
		"  fix: keep the statement once, in the promise\n"
	grSubmitOK(t, root, runID, "design", "2. Design soundness: PASS — ok\n3. Scope integrity: PASS — checked\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n"+
		advisory+"check-2: "+main+": Description\ncheck-4: "+main+": Description\n"+grFindingAffects(runID, "design", 1)+"\n")
	grSubmitOK(t, root, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Acceptance Items"}}))
	grSubmitOK(t, root, runID, "external", grDependenciesReport(map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, root, runID, main)
	// The advisory does not require a synthesis and never blocks: the run
	// finalizes directly with the P2 recorded.
	planned := mustLoadRun(t, root, runID)
	states, err := gaterun.LoadSessionStates(root, planned)
	if err != nil {
		t.Fatal(err)
	}
	if action := gateNextAction(planned, states, nil); action != "finalize" {
		t.Fatalf("a P2-only validate run finalizes without a synthesis, got %q", action)
	}
	grFinalizeOK(t, root, runID, "--result", "pass", "--p2-count", "1")
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "p2_count: 1") || !strings.Contains(cache, "blocking: false") {
		t.Fatalf("expected a passing cache recording the P2 advisory, got:\n%s", cache)
	}
	if !strings.Contains(cache, "result: pass") {
		t.Fatalf("a P2 advisory must not block the gate, got:\n%s", cache)
	}
}

func grFindingAffects(runID, session string, n int) string {
	return "Finding affects: " + grRunFindingID(runID, session, n) + " = 2"
}

// TestValidateP3RequiresFactAnchor: a unit-validate P3 finding is reportable
// only with a reproducible fact anchor (framework/severity_policy.md §10/§11.3).
func TestValidateP3RequiresFactAnchor(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	grSubmitOK(t, root, runID, "document", grValidateReport([]string{"1", gaterun.ClarityCheck}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Acceptance Items"},
		"6": {main + ": Acceptance Items"},
	}))
	block := "[P3] " + main + ": Description — a localized wording gap (actionable)\n" +
		"  problem: one sentence is worded loosely\n" +
		"  evidence: \"the loosely worded sentence\"\n" +
		"  impact: a reader may need a second pass\n" +
		"  fix: tighten the sentence\n"
	report := "2. Design soundness: PASS — ok\n3. Scope integrity: PASS — checked\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n" +
		block + "check-2: " + main + ": Description\ncheck-4: " + main + ": Description\n" + grFindingAffects(runID, "design", 1) + "\n"
	if _, err := grSubmitRaw(t, root, runID, "design", report); err == nil || !strings.Contains(err.Error(), "fact_anchor") {
		t.Fatalf("a validate P3 without fact_anchor must be rejected, got %v", err)
	}
	anchored := strings.Replace(block, "  fix: tighten the sentence\n", "  fix: tighten the sentence\n  fact_anchor: the sentence uses \"loosely\" without naming the condition\n", 1)
	report = "2. Design soundness: PASS — ok\n3. Scope integrity: PASS — checked\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n" +
		anchored + "check-2: " + main + ": Description\ncheck-4: " + main + ": Description\n" + grFindingAffects(runID, "design", 1) + "\n"
	grSubmitOK(t, root, runID, "design", report)
}

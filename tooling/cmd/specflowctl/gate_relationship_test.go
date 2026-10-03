package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// Relationship reports deliberately omit cross evidence: their dependency
// declarations belong to each relationship, not to a repeated local audit.
func relationshipReport(t *testing.T, root, runID string, evidence map[string]string, failed string) string {
	t.Helper()
	run := mustLoadRun(t, root, runID)
	var b strings.Builder
	passed := len(run.Relationships)
	verdict := "PASS"
	if failed != "" {
		passed--
		verdict = "FAIL"
	}
	for _, name := range run.Relationships {
		status := "PASS"
		if name == failed {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "Cross item: %s = %s — compared both sides of the relationship\n", name, status)
		if name == failed {
			fmt.Fprintf(&b, "Cross item finding: %s = %s/cross/F1\n", name, runID)
		}
		fmt.Fprintf(&b, "%s: %s: all\n", gaterun.RelationshipKey(name), evidence[name])
		fmt.Fprintf(&b, "Effective status: %s = %s\n", gaterun.RelationshipKey(name), strings.ToLower(status))
	}
	fmt.Fprintf(&b, "Cross-check: %d/%d %s — relationship check complete\n", passed, len(run.Relationships), verdict)
	if failed != "" {
		fmt.Fprintf(&b, "[P1] relationship — two otherwise valid parts disagree (actionable)\n  problem: definitions disagree\n  evidence: both definitions read\n  impact: combined behavior is wrong\n  fix: reconcile the definitions\nFinding affects: %s/cross/F1 = %s\n", runID, gaterun.RelationshipKey(failed))
	}
	fmt.Fprintf(&b, "Effective status: cross = %s\n", strings.ToLower(verdict))
	return grCompleteCrossReport(t, root, run, b.String())
}

func TestGateRelationshipRunsWithoutLocalFindings(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	main := grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency")
	grSubmitOK(t, root, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grAutoSubmitQuality(t, root, runID)
	run := mustLoadRun(t, root, runID)
	view, err := gateRunSnapshot(root, run)
	if err != nil || view.NextAction != "synthesize" {
		t.Fatalf("clean local pass must check the assigned relationship: %+v %v", view, err)
	}
	if _, err := grFinalize(t, root, runID); err == nil {
		t.Fatal("finalize skipped an assigned relationship")
	}
	mission := missionJSON(t, root, runID, "cross")
	template := mission.Sessions[0].ReportContract.Template
	if !strings.Contains(template, "Cross item: contract_consistency") || strings.Contains(template, "Cross item: data_definition_drift") {
		t.Fatalf("mission does not restrict the relationship scope: %s", template)
	}
	if !strings.Contains(mission.Sessions[0].Mission, "Do not repeat local checks") {
		t.Fatal("mission must exclude local rechecks")
	}
	report := relationshipReport(t, root, runID, map[string]string{"contract_consistency": main}, "contract_consistency")
	if _, err := grSubmitRaw(t, root, runID, "cross", report); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	check, err := checkUnitVerifyMerged(root, "auth", "candidate")
	if err != nil || check.Category != validationcache.CategoryBlocked {
		t.Fatalf("relationship failure must block fresh/promote: %+v %v", check, err)
	}
}

func TestGateConsecutiveRelationshipOnlyDeltas(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	a, b := "evidence/design.md", "evidence/coverage.md"
	grWriteFile(t, root, a, "first relationship\n")
	grWriteFile(t, root, b, "second relationship\n")
	evidence := map[string]string{"design_constraints": a, "coverage_scope": b}
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "design_constraints,coverage_scope", "--input", a, "--input", b)
	grSubmitPlannedValidateSessions(t, root, runID, main)
	if _, err := grSubmitRaw(t, root, runID, "cross", relationshipReport(t, root, runID, evidence, "")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	for _, change := range []struct{ name, path string }{{"design_constraints", a}, {"coverage_scope", b}} {
		grWriteFile(t, root, change.path, "changed relationship: "+change.name+"\n")
		preview, err := gaterun.PreviewDeltaScope(root, "validate", "unit", "auth", "candidate")
		if err != nil || preview.Degraded || strings.Join(preview.Rerun, ",") != gaterun.RelationshipKey(change.name) {
			t.Fatalf("fresh preview must select only the stale relationship: %+v %v", preview, err)
		}
		deltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--relationships", "none", "--input", a, "--input", b)
		run := mustLoadRun(t, root, deltaID)
		if len(run.Coverage) != 0 || strings.Join(run.Relationships, ",") != change.name {
			t.Fatalf("delta repeated local work or lost relationship scope: %+v", run)
		}
		if view, err := gateRunSnapshot(root, run); err != nil || view.NextAction != "synthesize" {
			t.Fatalf("relationship-only delta must synthesize: %+v %v", view, err)
		}
		if _, err := grSubmitRaw(t, root, deltaID, "cross", relationshipReport(t, root, deltaID, evidence, "")); err != nil {
			t.Fatal(err)
		}
		grFinalizeOK(t, root, deltaID)
		state := grReadJudgmentBaseline(t, root, "unit", "auth", "validate")
		if len(state.Relationships) != 2 {
			t.Fatalf("delta dropped a carried relationship: %+v", state)
		}
		check, err := validationcache.CheckWriteResult(root, "unit", "auth", "validate", "candidate")
		if err != nil || !check.Fresh {
			t.Fatalf("relationship-only delta did not publish a fresh complete cache: %+v %v", check, err)
		}
	}
}

func TestGateRelationshipOnlyRepair(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	path := "evidence/contract.md"
	grWriteFile(t, root, path, "inconsistent relationship\n")
	evidence := map[string]string{"coverage_scope": path}
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "coverage_scope", "--input", path)
	grSubmitPlannedValidateSessions(t, root, runID, main)
	if _, err := grSubmitRaw(t, root, runID, "cross", relationshipReport(t, root, runID, evidence, "coverage_scope")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	grWriteFile(t, root, path, "reconciled relationship\n")
	repairID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair", "--relationships", "none", "--input", path)
	run := mustLoadRun(t, root, repairID)
	if len(run.Coverage) != 0 || strings.Join(run.Relationships, ",") != "coverage_scope" {
		t.Fatalf("repair must recheck the failed relationship only: %+v", run)
	}
	if _, err := grSubmitRaw(t, root, repairID, "cross", relationshipReport(t, root, repairID, evidence, "")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, repairID)
	state := grReadJudgmentBaseline(t, root, "unit", "auth", "validate")
	if len(state.Findings) != 0 || state.LogicalStatus[gaterun.RelationshipKey("coverage_scope")] != "pass" {
		t.Fatalf("repaired relationship kept stale failure state: %+v", state)
	}
}

func TestGateRelationshipScopeDeclaration(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpec(t, root, "auth")
	for _, names := range []string{"", ",", "unknown", "none,coverage_scope"} {
		args := []string{"--repo-root", root, "--gate", "validate", "--unit", "auth", "--target", "candidate"}
		if names != "" {
			args = append(args, "--relationships", names)
		}
		var out, errOut bytes.Buffer
		if err := runGatePlan(args, &out, &errOut); err == nil {
			t.Fatalf("invalid or missing relationship scope %q accepted", names)
		}
	}
}

func TestGateFullRunPreservesUnchangedRelationship(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	evidence := map[string]string{"coverage_scope": main}
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "coverage_scope")
	grSubmitPlannedValidateSessions(t, root, runID, main)
	if _, err := grSubmitRaw(t, root, runID, "cross", relationshipReport(t, root, runID, evidence, "")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	fullID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	run := mustLoadRun(t, root, fullID)
	if len(run.Relationships) != 0 || !stringInList(run.CarriedKeys, gaterun.RelationshipKey("coverage_scope")) {
		t.Fatalf("full local review should preserve unchanged relationship evidence: %+v", run)
	}
	grSubmitPlannedValidateSessions(t, root, fullID, main)
	grFinalizeOK(t, root, fullID)
	state := grReadJudgmentBaseline(t, root, "unit", "auth", "validate")
	if strings.Join(state.Relationships, ",") != "coverage_scope" {
		t.Fatalf("full review erased the relationship baseline: %+v", state)
	}
}

func TestGateDeltaIntroducesRelationshipWithoutLocalChanges(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	grSubmitPlannedValidateSessions(t, root, runID, main)
	grFinalizeOK(t, root, runID)
	deltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--relationships", "coverage_scope")
	run := mustLoadRun(t, root, deltaID)
	if len(run.Coverage) != 0 || strings.Join(run.Relationships, ",") != "coverage_scope" {
		t.Fatalf("new relationship should not repeat fresh local judgments: %+v", run)
	}
	if _, err := grSubmitRaw(t, root, deltaID, "cross", relationshipReport(t, root, deltaID, map[string]string{"coverage_scope": main}, "")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, deltaID)
}

func TestGateRelationshipReportRejectsUnassignedOrUnboundWork(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "coverage_scope")
	grSubmitPlannedValidateSessions(t, root, runID, main)
	pass := relationshipReport(t, root, runID, map[string]string{"coverage_scope": main}, "")
	fail := relationshipReport(t, root, runID, map[string]string{"coverage_scope": main}, "coverage_scope")
	for _, tc := range []struct{ report, want string }{
		{pass + "Cross item: design_constraints = PASS — unassigned\n", "unknown Cross item"},
		{strings.ReplaceAll(pass, "relationship:coverage_scope: "+main+": all\n", ""), "declares no Dependency scope"},
		{strings.ReplaceAll(fail, "Finding affects: "+runID+"/cross/F1 = relationship:coverage_scope", "Finding affects: "+runID+"/cross/F1 = 2"), "finding must affect"},
		{pass + "[P2] local — repeats a local audit (actionable)\nFinding affects: " + runID + "/cross/F1 = 2\n", "must explain a failed assigned relationship"},
	} {
		if _, err := grSubmitRaw(t, root, runID, "cross", tc.report); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected %q rejection, got %v", tc.want, err)
		}
	}
}

func TestGateVerifyRelationshipOnlyRepairPreservesBothLenses(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	evidence := map[string]string{"data_definition_drift": main}
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "data_definition_drift")
	grSubmitOK(t, root, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grAutoSubmitQuality(t, root, runID)
	if _, err := grSubmitRaw(t, root, runID, "cross", relationshipReport(t, root, runID, evidence, "data_definition_drift")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	repairID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair", "--relationships", "none")
	run := mustLoadRun(t, root, repairID)
	if len(run.Coverage) != 0 || strings.Join(run.Relationships, ",") != "data_definition_drift" {
		t.Fatalf("verify repair repeated local work or skipped failed relationship: %+v", run)
	}
	if _, err := grSubmitRaw(t, root, repairID, "cross", relationshipReport(t, root, repairID, evidence, "")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, repairID)
	check, err := checkUnitVerifyMerged(root, "auth", "candidate")
	if err != nil || !check.Fresh {
		t.Fatalf("relationship-only verify repair lost complete alignment/quality coverage: %+v %v", check, err)
	}
}

func TestGateFullFailureWithCarriedRelationshipKeepsRepairSmall(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	code := "src/auth.go"
	grWriteFile(t, root, code, "package auth\n")
	first := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency")
	grSubmitOK(t, root, first, "auth.core", grVerifyItemReport("auth.core", main, code))
	grAutoSubmitQuality(t, root, first)
	if _, err := grSubmitRaw(t, root, first, "cross", relationshipReport(t, root, first, map[string]string{"contract_consistency": main}, "")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, first)
	second := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	grSubmitOK(t, root, second, "auth.core", grVerifyItemReport("auth.core", main, code))
	bad := strings.Replace(grQualityReport(code, main), "conclusion: acceptable", "conclusion: unacceptable — local defect", 1)
	bad = strings.Replace(bad, "gate_findings: none", "gate_findings: [P1] local defect", 1)
	bad += "[P1] src/auth.go — local defect (actionable)\n  problem: local defect\n  evidence: package auth\n  impact: caller fails\n  fix: correct the implementation\n"
	grSubmitOK(t, root, second, code, bad)
	grSubmitOK(t, root, second, "cross", "Cross-check: PASS — finding disposition only\ncross: "+code+": all\n")
	grFinalizeOK(t, root, second)
	repair := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair", "--relationships", "none")
	run := mustLoadRun(t, root, repair)
	if len(run.Coverage) != 1 || run.Coverage[0].Key != "design:auth:"+code || len(run.Relationships) != 0 {
		t.Fatalf("carried relationship in a full failure widened repair scope: %+v", run)
	}
	grSubmitOK(t, root, repair, code, grQualityReport(code, main))
	grFinalizeOK(t, root, repair)
	check, err := checkUnitVerifyMerged(root, "auth", "candidate")
	if err != nil || !check.Fresh {
		t.Fatalf("local-only repair lost the carried relationship or lens evidence: %+v %v", check, err)
	}
}

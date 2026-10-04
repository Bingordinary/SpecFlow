package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func TestGateAlignmentFindingCannotBeDeferred(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSharedUnits(t, root)
	runID := grPlan(t, root, "--gate", "verify", "--unit", "tool", "--target", "candidate")
	grSubmitOK(t, root, runID, "tool.core", grVerifyMismatchReport("tool.core", grToolMain, "src/shared.go", "P1"))
	id := grRunFindingID(runID, "tool.core", 1)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + id + " = retained\n" +
		"Finding ownership: " + id + " = owned_by agent — evidence: src/shared.go; reason: agent owns shared behavior\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: tool.core = pass\nEffective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, root, runID, "cross", report); err == nil || !strings.Contains(err.Error(), "quality") {
		t.Fatalf("alignment deferral must be rejected, got %v", err)
	}
	if _, err := grFinalize(t, root, runID); err == nil {
		t.Fatal("an alignment mismatch must not publish a pass cache without an accepted synthesis")
	}
	report = strings.ReplaceAll(report, "Finding ownership: "+id+" = owned_by agent — evidence: src/shared.go; reason: agent owns shared behavior\n", "")
	report = strings.ReplaceAll(report, "Effective status: tool.core = pass", "Effective status: tool.core = fail")
	grSubmitOK(t, root, runID, "cross", report)
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/tool/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("retained alignment mismatch must block, got:\n%s", cache)
	}
	check, err := checkUnitVerifyMerged(root, "tool", "candidate")
	if err != nil || check.Fresh || check.Category != validationcache.CategoryBlocked {
		t.Fatalf("fresh/promote must reject the alignment failure: %+v err=%v", check, err)
	}
}

func TestGateQualityBatchKeepsPerFileResults(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	a, b := "src/a.go", "src/b.go"
	grWriteFile(t, root, a, "package src\n")
	grWriteFile(t, root, b, "package src\n")
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	keys := []string{"design:auth:" + a, "design:auth:" + b}
	run := mustLoadRun(t, root, runID)
	spec, err := grBuildSessionSpec(root, run, keys)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if !strings.Contains(reportContractFor(run, spec).Template, "File: "+key+"\n") {
			t.Errorf("quality mission lacks the file block for %s", key)
		}
	}
	bad := strings.Replace(grQualityReport(a, main), "conclusion: acceptable", "conclusion: unacceptable — boundary defect", 1)
	bad = strings.Replace(bad, "gate_findings: none", "gate_findings: [P1] boundary defect", 1)
	bad += "[P1] src/a.go:1 — boundary defect (actionable)\n  problem: responsibility placed in the wrong module\n  evidence: package src\n  impact: consumers receive the wrong behavior\n  fix: move the responsibility to its owner\n"
	grSubmitKeys(t, root, runID, keys, bad+grQualityReport(b, main))
	state, err := grLoadSessionState(root, run, gaterun.SessionID(keys))
	if err != nil {
		t.Fatal(err)
	}
	if state.Result.Verdicts[keys[0]] != "unacceptable" || state.Result.Verdicts[keys[1]] != "acceptable" {
		t.Fatalf("file conclusions were combined: %v", state.Result.Verdicts)
	}
	if len(state.Result.Findings) != 1 || state.Result.Findings[0].SourceKey != keys[0] {
		t.Fatalf("finding lost its file: %+v", state.Result.Findings)
	}
	grSubmitOK(t, root, runID, "auth.core", grVerifyItemReport("auth.core", main, a))
	grSubmitOK(t, root, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, root, runID)
	baseline := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	if baseline.LogicalStatus[keys[0]] != "fail" || baseline.LogicalStatus[keys[1]] != "pass" {
		t.Fatalf("published wrong file statuses: %v", baseline.LogicalStatus)
	}
	check, err := checkUnitVerifyMerged(root, "auth", "candidate")
	if err != nil || check.Fresh || check.Category != validationcache.CategoryBlocked {
		t.Fatalf("fresh/promote must reject the failed file: %+v err=%v", check, err)
	}
	repair := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	if got := mustLoadRun(t, root, repair).Coverage; len(got) != 1 || got[0].Key != keys[0] {
		t.Fatalf("repair must recheck only the failed file, got %+v", got)
	}
	grSubmitOK(t, root, repair, a, grQualityReport(a, main))
	grFinalizeOK(t, root, repair)
	check, err = checkUnitVerifyMerged(root, "auth", "candidate")
	if err != nil || !check.Fresh {
		t.Fatalf("fresh/promote must accept the repaired merged verify cache: %+v err=%v", check, err)
	}
}

func TestGateAlignmentBatchSuppressionKeepsOtherItemPassing(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.a", "auth.b"})
	grWriteFile(t, root, "src/auth.go", "package src\n")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	keys := []string{"auth.a", "auth.b"}
	grSubmitKeys(t, root, runID, keys, grVerifyMismatchReport(keys[0], main, "src/auth.go", "P1")+grVerifyMismatchReport(keys[1], main, "src/auth.go", "P1"))
	sessionID := gaterun.SessionID(keys)
	f1, f2 := grRunFindingID(runID, sessionID, 1), grRunFindingID(runID, sessionID, 2)
	cross := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + f1 + " = retained\n" +
		"Finding disposition: " + f2 + " = suppressed — implementation satisfies auth.b\n" +
		"Effective status: auth.a = fail\nEffective status: auth.b = pass\nEffective status: src/auth.go = pass\nEffective status: cross = pass\n" +
		"cross: " + main + ": Description\n"
	grSubmitOK(t, root, runID, "cross", cross)
	grFinalizeOK(t, root, runID)
	baseline := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	if baseline.LogicalStatus["item:auth:auth.b"] != "pass" {
		t.Fatalf("suppressed mismatch still fails auth.b: %v", baseline.LogicalStatus)
	}
	repair := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	if got := mustLoadRun(t, root, repair).Coverage; len(got) != 1 || got[0].Key != "item:auth:auth.a" {
		t.Fatalf("repair must recheck only auth.a, got %+v", got)
	}
	grSubmitOK(t, root, repair, "auth.a", grVerifyItemReport("auth.a", main, "src/auth.go"))
	grFinalizeOK(t, root, repair)
	check, err := checkUnitVerifyMerged(root, "auth", "candidate")
	if err != nil || !check.Fresh {
		t.Fatalf("repair must keep the suppressed item passing: %+v err=%v", check, err)
	}
}

func TestGateRuleFindingRoutesDirectlyToFinalize(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	rule := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, root, rule, "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nUse TLS.\n")
	runID := grPlan(t, root, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var report strings.Builder
	for i := 1; i <= 6; i++ {
		verdict := "PASS"
		if i == 6 {
			verdict = "FAIL"
		}
		fmt.Fprintf(&report, "%d. Check: %s — checked\ncheck-%d: %s: Constraint\n", i, verdict, i, rule)
	}
	report.WriteString("[P1] rule — contradictory constraint (actionable)\n  problem: rule body contradicts itself\n  evidence: Use TLS\n  impact: consumers cannot follow both requirements\n  fix: reconcile the constraint\n")
	fmt.Fprintf(&report, "Finding affects: %s = 6\n", grRunFindingID(runID, gaterun.SessionID([]string{"1", "2", "3", "4", "5", "6"}), 1))
	output := grSubmitOK(t, root, runID, "checks", report.String())
	if strings.Contains(output, "--final") || !strings.Contains(output, "gate-finalize") {
		t.Errorf("rule submission recommends an invalid next step:\n%s", output)
	}
	run := mustLoadRun(t, root, runID)
	view, err := gateRunSnapshot(root, run)
	if err != nil || view.NextAction != "finalize" {
		t.Errorf("rule JSON next action = %q, err=%v", view.NextAction, err)
	}
	var out, errOut bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--final"}, &out, &errOut); err == nil {
		t.Error("a rule run must reject a final-synthesis mission")
	}
	if _, err := grSubmitRaw(t, root, runID, "cross", "Cross-check: PASS — checked\n"); err == nil || !strings.Contains(err.Error(), "rule") {
		t.Errorf("a rule run must reject final-synthesis submission before recording it: %v", err)
	}
	grFinalizeOK(t, root, runID)
}

func TestGateJSONWaitsForFindingSynthesis(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package src\n")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, root, runID, "auth.core", grVerifyMismatchReport("auth.core", main, "src/auth.go", "P1"))
	grAutoSubmitQuality(t, root, runID)
	view, err := gateRunSnapshot(root, mustLoadRun(t, root, runID))
	if err != nil || view.NextAction != "synthesize" {
		t.Fatalf("findings without synthesis must not recommend finalize: action=%q err=%v", view.NextAction, err)
	}
	grSubmitOK(t, root, runID, "cross", grCrossReport(main, "Description"))
	view, err = gateRunSnapshot(root, mustLoadRun(t, root, runID))
	if err != nil || view.NextAction != "finalize" {
		t.Fatalf("accepted synthesis must permit finalize: action=%q err=%v", view.NextAction, err)
	}
}

func TestGateQualityBatchRejectsSharedOrMisassignedAssessments(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpec(t, root, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	a, b := "src/a.go", "src/b.go"
	grWriteFile(t, root, a, "package src\n")
	grWriteFile(t, root, b, "package src\n")
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	keys := []string{"design:auth:" + a, "design:auth:" + b}
	valid := grQualityReport(a, main) + grQualityReport(b, main)
	cases := []struct{ name, report string }{
		{"missing file", grQualityReport(a, main)},
		{"shared report", strings.ReplaceAll(strings.ReplaceAll(valid, "File: "+a+"\n", ""), "File: "+b+"\n", "")},
		{"duplicate file", grQualityReport(a, main) + grQualityReport(a, main)},
		{"unassigned file", strings.Replace(valid, "File: "+b, "File: src/outside.go", 1)},
		{"shared dimension", strings.Replace(valid, "  module_boundaries: sound — reviewed module boundary\n", "", 1)},
		{"shared dependency", strings.ReplaceAll(grQualityReport(a, main), a+": ", b+": ") + grQualityReport(b, main)},
		{"contradictory findings", valid + "[P1] src/b.go:1 — defect (actionable)\n  problem: defect\n  evidence: package src\n  impact: incorrect behavior\n  fix: correct it\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := grWriteFile(t, t.TempDir(), "report.md", tc.report)
			var out, errOut bytes.Buffer
			if err := runGateSubmit([]string{"--repo-root", root, "--run", runID, "--session", gaterun.SessionID(keys), "--keys", strings.Join(keys, ","), "--report", path}, &out, &errOut); err == nil {
				t.Fatalf("invalid per-file report accepted:\n%s", tc.report)
			}
		})
	}
}

func TestGateOwnershipRejectsAlignmentMergedIntoQuality(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSharedUnits(t, root)
	runID := grPlan(t, root, "--gate", "verify", "--unit", "tool", "--target", "candidate")
	grSubmitOK(t, root, runID, "tool.core", grVerifyMismatchReport("tool.core", grToolMain, "src/shared.go", "P1"))
	local, merged := grRunFindingID(runID, "tool.core", 1), grRunFindingID(runID, "cross", 1)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"[P1] src/shared.go:1 — shared defect (actionable)\n  problem: incorrect behavior\n  evidence: package src\n  impact: consumers fail\n  fix: correct it\n" +
		"Finding affects: " + merged + " = src/shared.go\n" +
		"Finding disposition: " + local + " = merged -> " + merged + " — same underlying defect\n" +
		"Finding ownership: " + merged + " = owned_by agent — evidence: src/shared.go; reason: recorded owner\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: tool.core = pass\nEffective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, root, runID, "cross", report); err == nil || !strings.Contains(err.Error(), "non-quality key") {
		t.Fatalf("merging must not allow alignment deferral: %v", err)
	}
}

func TestGateCarriedAlignmentFindingCannotBeDeferred(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSharedUnits(t, root)
	full := grPlan(t, root, "--gate", "verify", "--unit", "tool", "--target", "candidate")
	grSubmitOK(t, root, full, "tool.core", grVerifyMismatchReport("tool.core", grToolMain, "src/shared.go", "P2"))
	grSubmitOK(t, root, full, "cross", grCrossReport(grToolMain, "Description"))
	grFinalizeOK(t, root, full)
	delta := grPlan(t, root, "--gate", "verify", "--unit", "tool", "--target", "candidate", "--mode", "delta", "--rerun", "design:tool:src/shared.go")
	run := mustLoadRun(t, root, delta)
	if run.CoverageByKey("item:tool:tool.core") != nil || len(run.CarriedResults) == 0 {
		t.Fatal("fixture must carry the alignment judgment")
	}
	grSubmitOK(t, root, delta, "src/shared.go", grQualityReport("src/shared.go", grToolMain))
	id := grRunFindingID(full, "tool.core", 1)
	report := "Cross-check: 5/5 PASS — checked\n" +
		"Finding disposition: " + id + " = retained\n" +
		"Finding ownership: " + id + " = owned_by agent — evidence: src/shared.go; reason: recorded owner\n" +
		"cross: src/shared.go: all\n" +
		"Effective status: tool.core = pass\nEffective status: src/shared.go = pass\nEffective status: src/tool_only.go = pass\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, root, delta, "cross", report); err == nil || !strings.Contains(err.Error(), "non-quality key") {
		t.Fatalf("carried alignment deferral must be rejected: %v", err)
	}
}

func TestGateValidateBatchRequiresExactFindingKeys(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpec(t, root, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	report := "2. Design: FAIL — contradiction\n4. Consistency: PASS — checked\n" +
		"[P1] design — contradiction (actionable)\n  problem: inconsistent design\n  evidence: conflicting declarations\n  impact: ambiguous implementation\n  fix: reconcile them\n" +
		"check-2: " + main + ": Description\ncheck-4: " + main + ": Description\n"
	id := grRunFindingID(runID, "design", 1)
	for _, suffix := range []string{"", "Finding affects: " + id + " = 4\n", "Finding affects: " + id + " = 2,4\n", "Finding affects: " + id + " = 1\n"} {
		if _, err := grSubmitRaw(t, root, runID, "design", report+suffix); err == nil {
			t.Fatalf("ambiguous or contradictory finding keys accepted: %q", suffix)
		}
	}
	grSubmitOK(t, root, runID, "design", report+"Finding affects: "+id+" = 2\n")
	state, err := grLoadSessionState(root, mustLoadRun(t, root, runID), "design")
	if err != nil || len(state.Result.Findings) != 1 || state.Result.Findings[0].SourceKey != "2" || len(state.Result.Findings[0].AffectedKeys) != 0 {
		t.Fatalf("validate finding must belong only to check 2: state=%+v err=%v", state, err)
	}
}

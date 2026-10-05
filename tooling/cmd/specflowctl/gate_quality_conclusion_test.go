package main

import (
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

func qualityRecord(t *testing.T, root, key string) *judgments.Record {
	t.Helper()
	state := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	record, err := judgments.Load(root, state.Records[key].Reference)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestQualityConclusionSurvivesPublicationAndDelta(t *testing.T) {
	for _, key := range []string{"design:auth:contracts.js", "architecture:auth"} {
		for _, severity := range []string{"", "P2", "P3"} {
			t.Run(key+"/"+severity, func(t *testing.T) {
				root, main, _ := sharedFixture(t)
				id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
				run := mustLoadRun(t, root, id)
				report := strings.Replace(grDefaultQualityReport(run, *run.CoverageByKey(key)), "conclusion: acceptable", "conclusion: needs_attention", 1)
				if severity != "" {
					report += "[" + severity + "] contracts.js:1 — responsibility name is unclear (actionable)\n  problem: the name obscures responsibility\n  evidence: response\n  impact: code is harder to understand\n  fix: clarify the name\n"
					if severity == "P3" {
						report += "  fact_anchor: contracts.js:1\n"
					}
				}
				grSubmitOK(t, root, id, key, report)
				grSubmitOK(t, root, id, "auth.core", grVerifyItemReport("auth.core", main, "contracts.js"))
				if severity != "" {
					grSubmitOK(t, root, id, "cross", grCrossReport(main, "Description"))
				}
				grFinalizeOK(t, root, id)
				assertConclusion := func() {
					t.Helper()
					state := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
					record, err := judgments.Load(root, state.Records[key].Reference)
					if err != nil {
						t.Fatal(err)
					}
					if record.Verdict != "needs_attention" {
						t.Fatalf("accepted needs_attention became %s", record.Verdict)
					}
					if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || !check.Fresh {
						t.Fatalf("nonblocking quality assessment lost gate freshness: %+v %v", check, err)
					}
				}
				assertConclusion()
				id = grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--relationships", "contract_consistency")
				grSubmitOK(t, root, id, "cross", relationshipReport(t, root, id, map[string]string{"contract_consistency": main}, ""))
				grFinalizeOK(t, root, id)
				assertConclusion()
			})
		}
	}
}

func TestFinalQualityConclusionUsesDispositionAndOwnEvidence(t *testing.T) {
	for _, disposition := range []string{"retained", "suppressed"} {
		t.Run(disposition, func(t *testing.T) {
			root, main, _ := sharedFixture(t)
			proof := "evidence/quality.md"
			grWriteFile(t, root, proof, "Responsibility evidence.\n")
			id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, proof))
			run := mustLoadRun(t, root, id)
			key := "architecture:auth"
			report := strings.Replace(grDefaultQualityReport(run, *run.CoverageByKey(key)), "conclusion: acceptable", "conclusion: unacceptable — responsibility violation", 1)
			report = strings.Replace(report, "gate_findings: none", "gate_findings: [P1] responsibility violation", 1)
			report += "[P1] contracts.js:1 — responsibility violation (actionable)\n  problem: responsibility is misplaced\n  evidence: response\n  impact: required behavior is unavailable\n  fix: correct responsibility placement\n"
			grSubmitOK(t, root, id, key, report)
			grSubmitOK(t, root, id, "auth.core", grVerifyItemReport("auth.core", main, "contracts.js"))
			conclusion, status := "unacceptable", "fail"
			dispositionLine := "retained"
			if disposition == "suppressed" {
				conclusion, status = "needs_attention", "pass"
				dispositionLine = "suppressed — the evidence resolves the violation but leaves a naming concern"
			}
			cross := "Cross-check: PASS — local finding disposition complete\nFinding disposition: " + id + "/architecture:auth/F1 = " + dispositionLine + "\nEffective status: " + key + " = " + status + "\nQuality conclusion: " + key + " = " + conclusion + " — final responsibility assessment\ncross: " + proof + ": all\n"
			grSubmitOK(t, root, id, "cross", cross)
			grFinalizeOK(t, root, id)
			record := qualityRecord(t, root, key)
			if record.Verdict != conclusion {
				t.Fatalf("final quality conclusion became %s, expected %s", record.Verdict, conclusion)
			}
			if disposition == "suppressed" {
				grWriteFile(t, root, proof, "Changed responsibility evidence.\n")
				if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || check.Fresh {
					t.Fatalf("changed synthesis evidence left quality fresh: %+v %v", check, err)
				}
			}
		})
	}
}

func TestFinalQualityConclusionContract(t *testing.T) {
	root, id, main := grReadyVerifyCross(t)
	report := grVerifyCrossReport(id, main, 5, "", "PASS")
	line := "Quality conclusion: architecture:auth = acceptable — fixture final assessment\n"
	for _, tc := range []struct{ name, report, want string }{
		{"missing", strings.Replace(report, line, "", 1), "missing Quality conclusion"},
		{"duplicate", report + line, "more than once"},
		{"no reason", strings.Replace(report, line, "Quality conclusion: architecture:auth = needs_attention\n", 1), "invalid Quality conclusion"},
		{"contradictory blocking", strings.Replace(report, line, "Quality conclusion: architecture:auth = unacceptable — concern\n", 1), "finalized P0/P1"},
		{"unexpected key", report + "Quality conclusion: item:auth:auth.core = acceptable — concern\n", "unexpected Quality conclusion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := grSubmitRaw(t, root, id, "cross", tc.report); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestFinalAttentionWithoutFindingKeepsSynthesisEvidence(t *testing.T) {
	root, main, _ := sharedFixture(t)
	proof := "evidence/relationship.md"
	grWriteFile(t, root, proof, "Responsibility names need attention.\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency", "--inputs-file", grInputsManifest(t, proof))
	grSubmitOK(t, root, id, "auth.core", grVerifyItemReport("auth.core", main, "contracts.js"))
	grAutoSubmitQuality(t, root, id)
	report := relationshipReport(t, root, id, map[string]string{"contract_consistency": proof}, "")
	report = strings.Replace(report, "Quality conclusion: architecture:auth = acceptable", "Quality conclusion: architecture:auth = needs_attention", 1)
	report += "cross: " + proof + ": all\n"
	grSubmitOK(t, root, id, "cross", report)
	grFinalizeOK(t, root, id)
	if record := qualityRecord(t, root, "architecture:auth"); record.Verdict != "needs_attention" {
		t.Fatalf("final attention assessment became %s", record.Verdict)
	}
	grWriteFile(t, root, proof, "Responsibility names are resolved.\n")
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || check.Fresh {
		t.Fatalf("changed final-assessment evidence left quality fresh: %+v %v", check, err)
	}
}

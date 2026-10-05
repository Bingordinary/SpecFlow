package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func synthesisPromote(t *testing.T, root, unit string) {
	t.Helper()
	sharedValidateFixture(t, root, unit)
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", unit}, &out, &errOut); err != nil {
		t.Fatalf("promote %s: %v %s", unit, err, out.String())
	}
}

func synthesisItemRecord(t *testing.T, root, unit string) *judgments.Record {
	t.Helper()
	state := grReadJudgmentBaseline(t, root, "unit", unit, "verify")
	r, err := judgments.Load(root, state.Records["item:"+unit+":"+unit+".core"].Reference)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func synthesisAssertBlocked(t *testing.T, root, unit string) {
	t.Helper()
	check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, unit, "candidate")
	if err != nil || check.Category != validationcache.CategoryBlocked {
		t.Fatalf("unconfirmed protected requirement did not block %s: %+v %v", unit, check, err)
	}
	sharedValidateFixture(t, root, unit)
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", unit}, &out, &errOut); err == nil {
		t.Fatal("promote accepted an unconfirmed protected requirement")
	}
}

func TestIndeterminateItemCannotBecomeAlignedProtection(t *testing.T) {
	for _, mode := range []string{"full", "delta"} {
		t.Run(mode, func(t *testing.T) {
			root, main, _ := sharedFixture(t)
			id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
			key := "item:auth:auth.core"
			report := strings.Replace(grVerifyItemReport(key, main, "contracts.js"), "ALIGNED", "CANNOT_DETERMINE", 1)
			if err := sharedSubmit(t, root, id, key, report); err != nil {
				t.Fatal(err)
			}
			sharedFinish(t, root, id)
			if mode == "delta" {
				id = grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", mode, "--relationships", "contract_consistency")
				run := mustLoadRun(t, root, id)
				carried := false
				for _, carriedKey := range run.CarriedKeys {
					if carriedKey == key {
						carried = true
					}
				}
				if !carried {
					t.Fatal("delta did not carry the unchanged item")
				}
				record, err := judgments.Load(root, run.Records[key].Reference)
				if err != nil || record.Verdict != "CANNOT_DETERMINE" {
					t.Fatalf("delta lost the original indeterminate decision: %+v %v", record, err)
				}
				if _, err := grSubmitRaw(t, root, id, "cross", relationshipReport(t, root, id, map[string]string{"contract_consistency": main}, "")); err != nil {
					t.Fatal(err)
				}
				grFinalizeOK(t, root, id)
			}
			record := synthesisItemRecord(t, root, "auth")
			if record.Verdict != "CANNOT_DETERMINE" {
				t.Fatalf("finalization changed an unconfirmed requirement to %s", record.Verdict)
			}
			// The owning unit's advisory policy does not prove a stable requirement
			// for another unit. Preserve must apply its own strict policy on reuse.
			synthesisPromote(t, root, "auth")
			order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			run := mustLoadRun(t, root, order)
			binding := run.Records["preserve:auth:auth.core"]
			reused, err := judgments.Load(root, binding.Reference)
			if err != nil || reused.Verdict != "CANNOT_DETERMINE" {
				t.Fatalf("protected peer reused an incorrect decision: %+v %v", reused, err)
			}
			grSubmitOK(t, root, order, "order.core", grVerifyItemReport("order.core", run.RequiredFiles[0], "contracts.js"))
			grSubmitOK(t, root, order, "cross", grCrossReport(run.RequiredFiles[0], "Description"))
			grFinalizeOK(t, root, order)
			synthesisAssertBlocked(t, root, "order")
		})
	}
}

func TestItemRequiredCodeEvidenceCannotBeOmittedFromDependencies(t *testing.T) {
	for _, changed := range []string{"contracts.js", "agreement.js"} {
		t.Run(changed, func(t *testing.T) {
			root, main, _ := sharedFixture(t)
			grWriteFile(t, root, "agreement.js", "export const tokenRequired = true;\n")
			id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, "agreement.js"))
			key := "item:auth:auth.core"
			report := grVerifyItemBody(key, "ALIGNED", "contracts.js:1") + key + ": " + main + ": acceptance_item:auth.core\n"
			if err := sharedSubmit(t, root, id, key, report); err != nil {
				t.Fatal(err)
			}
			sharedFinish(t, root, id)
			record := synthesisItemRecord(t, root, "auth")
			for _, path := range []string{"contracts.js", "agreement.js"} {
				found := false
				for _, dep := range record.Dependencies {
					if dep.Path == path && dep.Hash != "" {
						found = true
					}
				}
				if !found {
					t.Fatalf("required code input %s has no whole-file dependency", path)
				}
			}
			synthesisPromote(t, root, "auth")
			order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			sharedFinish(t, root, order)
			if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !check.Fresh {
				t.Fatalf("unchanged evidence was not reusable: %+v %v", check, err)
			}
			grWriteFile(t, root, changed, "export function response(token) { return {}; }\n")
			if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || check.Fresh {
				t.Fatalf("changed protected evidence left peer fresh: %+v %v", check, err)
			}
			sharedValidateFixture(t, root, "order")
			var out, errOut bytes.Buffer
			if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err == nil {
				t.Fatal("promote reused protection after required code evidence changed")
			}
			order = grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			run := mustLoadRun(t, root, order)
			ck := run.CoverageByKey("preserve:auth:auth.core")
			if ck.Source != "executed" {
				t.Fatal("changed protected evidence reused the old ALIGNED judgment")
			}
			report = strings.ReplaceAll(grVerifyMismatchReport(ck.Key, "docs/specs/units/stable/unit_auth.md", "contracts.js", "P1"), "acceptance_item:"+ck.Key, "acceptance_item:auth.core")
			if err := sharedSubmit(t, root, order, ck.Key, report); err != nil {
				t.Fatal(err)
			}
			grSubmitOK(t, root, order, "order.core", grVerifyItemReport("order.core", run.RequiredFiles[0], "contracts.js"))
			grSubmitOK(t, root, order, "cross", grCrossReport(run.RequiredFiles[0], "Description"))
			grFinalizeOK(t, root, order)
			synthesisAssertBlocked(t, root, "order")
		})
	}
}

func TestProtectedFindingSeverityFloorKeepsP0AndCanonicalDetail(t *testing.T) {
	for _, source := range []string{"executed", "reused"} {
		for _, severity := range []string{"P0", "P1", "P2", "P3"} {
			t.Run(source+"/"+severity, func(t *testing.T) {
				root, main, _ := sharedFixture(t)
				stable := "docs/specs/units/stable/unit_auth.md"
				data, err := os.ReadFile(filepath.Join(root, main))
				if err != nil {
					t.Fatal(err)
				}
				var original *judgments.Record
				var originalRef judgments.Reference
				if source == "reused" {
					id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
					grSubmitOK(t, root, id, "auth.core", grVerifyMismatchReport("auth.core", main, "contracts.js", severity))
					grSubmitOK(t, root, id, "cross", grCrossReport(main, "Description"))
					grFinalizeOK(t, root, id)
					original = synthesisItemRecord(t, root, "auth")
					originalRef = grReadJudgmentBaseline(t, root, "unit", "auth", "verify").Records["item:auth:auth.core"].Reference
				}
				grWriteFile(t, root, stable, string(data))
				id := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
				run := mustLoadRun(t, root, id)
				key := "preserve:auth:auth.core"
				if got := run.CoverageByKey(key).Source; got != source {
					t.Fatalf("expected %s protection, got %s", source, got)
				}
				if source == "executed" {
					report := strings.ReplaceAll(grVerifyMismatchReport(key, stable, "contracts.js", severity), "acceptance_item:"+key, "acceptance_item:auth.core")
					if err := sharedSubmit(t, root, id, key, report); err != nil {
						t.Fatal(err)
					}
				}
				states, err := gaterun.LoadSessionStates(root, run)
				if err != nil {
					t.Fatal(err)
				}
				want := "P1"
				if severity == "P0" {
					want = "P0"
				}
				found := false
				for _, state := range states {
					if state.Result == nil {
						continue
					}
					for _, finding := range state.Result.Findings {
						if finding.SourceKey == key {
							found = true
							if finding.Severity != want || !strings.HasPrefix(finding.Detail, "["+want+"]") {
								t.Fatalf("protected severity/detail disagree: %+v, want %s", finding, want)
							}
							if severity != want && strings.Contains(finding.Detail, "Severity: "+severity) {
								t.Fatalf("protected detail still contains the source severity: %s", finding.Detail)
							}
						}
					}
				}
				if !found {
					t.Fatal("protected failure has no finding")
				}
				grSubmitOK(t, root, id, "order.core", grVerifyItemReport("order.core", run.RequiredFiles[0], "contracts.js"))
				grSubmitOK(t, root, id, "cross", grCrossReport(run.RequiredFiles[0], "Description"))
				grFinalizeOK(t, root, id)
				synthesisAssertBlocked(t, root, "order")
				if original != nil {
					unchanged, err := judgments.Load(root, originalRef)
					if err != nil || unchanged.Verdict != original.Verdict || string(unchanged.Result) != string(original.Result) {
						t.Fatalf("protection modified the source decision: %+v %v", unchanged, err)
					}
				}
			})
		}
	}
}

func TestFinalizedSuppressionIsReusableByProtectedPeer(t *testing.T) {
	root, main, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, root, id, "auth.core", grVerifyMismatchReport("auth.core", main, "contracts.js", "P1"))
	fid := grRunFindingID(id, "auth.core", 1)
	grSubmitOK(t, root, id, "cross", "Cross-check: PASS — local false positive resolved\nFinding disposition: "+fid+" = suppressed — the complete code path satisfies the requirement\nEffective status: auth.core = pass\nEffective status: contracts.js = pass\nEffective status: cross = pass\ncross: "+main+": Testability / Acceptance Criteria\n")
	grFinalizeOK(t, root, id)
	record := synthesisItemRecord(t, root, "auth")
	var result gaterun.SessionResult
	if err := json.Unmarshal(record.Result, &result); err != nil {
		t.Fatal(err)
	}
	if record.Verdict != "ALIGNED" || len(result.Findings) != 0 {
		t.Fatalf("resolved false positive survived in immutable record: %s %+v", record.Verdict, result.Findings)
	}
	synthesisPromote(t, root, "auth")
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)
	check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
	if err != nil || !check.Fresh {
		t.Fatalf("suppressed false positive blocked protected peer: %+v %v", check, err)
	}
	synthesisPromote(t, root, "order")
}

func TestFinalizedFailureSupersedesEarlierAlignedProtection(t *testing.T) {
	for _, mode := range []string{"full", "delta"} {
		t.Run(mode, func(t *testing.T) {
			root, _, _ := sharedFixture(t)
			proof := "agreement.txt"
			grWriteFile(t, root, proof, "agreed interface\n")
			initial := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, proof))
			sharedFinish(t, root, initial)
			previous := grReadJudgmentBaseline(t, root, "unit", "auth", "verify").Records["item:auth:auth.core"]
			synthesisPromote(t, root, "auth")
			stable := "docs/specs/units/stable/unit_auth.md"
			id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "stable", "--mode", mode, "--relationships", "contract_consistency", "--inputs-file", grInputsManifest(t, proof))
			if mode == "full" {
				grSubmitOK(t, root, id, "auth.core", grVerifyItemReport("auth.core", stable, "contracts.js"))
				grAutoSubmitQuality(t, root, id)
			}
			report := relationshipReport(t, root, id, map[string]string{"contract_consistency": proof}, "contract_consistency")
			report = strings.ReplaceAll(report, "Finding affects: "+id+"/cross/F1 = relationship:contract_consistency", "Finding affects: "+id+"/cross/F1 = relationship:contract_consistency, item:auth:auth.core")
			report = strings.ReplaceAll(report, "Effective status: item:auth:auth.core = pass", "Effective status: item:auth:auth.core = fail")
			if _, err := grSubmitRaw(t, root, id, "cross", report); err != nil {
				t.Fatal(err)
			}
			grFinalizeOK(t, root, id)
			record := synthesisItemRecord(t, root, "auth")
			if record.Verdict != "MISMATCH" {
				t.Fatalf("final synthesis failure lost from %s judgment: %s", mode, record.Verdict)
			}
			if err := judgments.Check(root, previous.Reference, "stable", judgments.Protocol(root)); err == nil {
				t.Fatal("earlier consumers can still rely on a superseded ALIGNED decision")
			}
			order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			run := mustLoadRun(t, root, order)
			binding := run.Records["preserve:auth:auth.core"]
			if binding.ID == "" {
				t.Fatal("latest finalized stable judgment was not reused")
			}
			reused, err := judgments.Load(root, binding.Reference)
			if err != nil || reused.Verdict != "MISMATCH" {
				t.Fatalf("older ALIGNED record displaced final failure: %+v %v", reused, err)
			}
			states, err := gaterun.LoadSessionStates(root, run)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range states {
				if s.Result != nil && s.Result.Verdicts["preserve:auth:auth.core"] != "" {
					for _, finding := range s.Result.Findings {
						if finding.SourceKey != "preserve:auth:auth.core" || len(finding.AffectedKeys) != 0 {
							t.Fatalf("foreign synthesis keys leaked into protection: %+v", finding)
						}
					}
				}
			}
			grSubmitOK(t, root, order, "order.core", grVerifyItemReport("order.core", run.RequiredFiles[0], "contracts.js"))
			grSubmitOK(t, root, order, "cross", grCrossReport(run.RequiredFiles[0], "Description"))
			grFinalizeOK(t, root, order)
			check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
			if err != nil || check.Category != validationcache.CategoryBlocked {
				t.Fatalf("confirmed stable failure did not block peer: %+v %v", check, err)
			}
			sharedValidateFixture(t, root, "order")
			var out, errOut bytes.Buffer
			if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err == nil {
				t.Fatal("promote accepted a failed protected requirement")
			}
			if err := os.WriteFile(filepath.Join(root, proof), []byte("changed interface\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := judgments.Check(root, binding.Reference, binding.Layer, run.Protocol); err == nil {
				t.Fatal("changed final-synthesis evidence left protection judgment fresh")
			}
			if err := judgments.Check(root, previous.Reference, "stable", run.Protocol); err == nil {
				t.Fatal("stale current evidence resurrected the earlier ALIGNED decision")
			}
			recheck := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			planned := mustLoadRun(t, root, recheck)
			if planned.CoverageByKey("preserve:auth:auth.core").Source != "executed" {
				t.Fatal("stale current decision was replaced by a historical ALIGNED result")
			}
		})
	}
}

func TestCandidateDecisionDoesNotReplaceDifferentStableRequirement(t *testing.T) {
	root, candidate, _ := sharedFixture(t)
	initial := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, initial)
	synthesisPromote(t, root, "auth")
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)
	data, err := os.ReadFile(filepath.Join(root, "docs/specs/units/stable/unit_auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(data), "pass_condition:", "pass_condition: changed requirement —", 1)
	grWriteFile(t, root, candidate, changed)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, root, id, "auth.core", grVerifyMismatchReport("auth.core", candidate, "contracts.js", "P1"))
	grSubmitOK(t, root, id, "cross", grCrossReport(candidate, "Description"))
	grFinalizeOK(t, root, id)
	check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
	if err != nil || !check.Fresh {
		t.Fatalf("different candidate requirement changed accepted stable truth: %+v %v", check, err)
	}
	synthesisPromote(t, root, "order")
}

func TestCandidateDecisionCannotResurrectSupersededStableProtection(t *testing.T) {
	root, candidate, _ := sharedFixture(t)
	initial := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, initial)
	synthesisPromote(t, root, "auth")
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)

	stable := "docs/specs/units/stable/unit_auth.md"
	failure := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "stable")
	grSubmitOK(t, root, failure, "auth.core", grVerifyMismatchReport("auth.core", stable, "contracts.js", "P1"))
	grSubmitOK(t, root, failure, "cross", grCrossReport(stable, "Description"))
	grFinalizeOK(t, root, failure)
	failed := grReadJudgmentBaseline(t, root, "unit", "auth", "verify").Records["item:auth:auth.core"]
	before, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
	if err != nil || before.Fresh {
		t.Fatalf("stable failure must invalidate earlier protection: %+v %v", before, err)
	}

	data, err := os.ReadFile(filepath.Join(root, stable))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, candidate, strings.Replace(string(data), "pass_condition:", "pass_condition: different candidate requirement —", 1))
	draft := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, draft)
	after, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
	if err != nil || after.Fresh {
		t.Fatalf("different candidate resurrected superseded stable protection: %+v %v", after, err)
	}
	sharedValidateFixture(t, root, "order")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err == nil {
		t.Fatal("promote accepted an unresolved stable failure")
	}

	recheck := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run := mustLoadRun(t, root, recheck)
	if run.Records["preserve:auth:auth.core"].Reference != failed.Reference {
		t.Fatal("protection did not select the current stable failure")
	}
}

func TestPublishedStableContextInvalidatesHistoricalProtection(t *testing.T) {
	root, candidate, _ := sharedFixture(t)
	initial := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, initial)
	synthesisPromote(t, root, "auth")
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)

	// Keep the acceptance region unchanged, so only the newly published
	// context's current decision can invalidate the old protected evidence.
	data, err := os.ReadFile(filepath.Join(root, "docs/specs/units/stable/unit_auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, candidate, strings.Replace(string(data), "Prose.", "Updated design context.", 1))
	draft := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	report := strings.Replace(grVerifyItemReport("auth.core", candidate, "contracts.js"), "ALIGNED", "CANNOT_DETERMINE", 1)
	grSubmitOK(t, root, draft, "auth.core", report)
	sharedFinish(t, root, draft)
	before, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
	if err != nil || !before.Fresh {
		t.Fatalf("unpublished candidate affected stable protection: %+v %v", before, err)
	}
	synthesisPromote(t, root, "auth")
	after, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
	if err != nil || after.Fresh {
		t.Fatalf("published indeterminate decision left historical protection valid: %+v %v", after, err)
	}
	sharedValidateFixture(t, root, "order")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err == nil {
		t.Fatal("promote bypassed the newly published stable decision")
	}
}

func TestUnusableCurrentProtectionCannotReuseAlignedHistory(t *testing.T) {
	for _, fault := range []string{"missing", "damaged", "invalidated"} {
		t.Run(fault, func(t *testing.T) {
			root, _, _ := sharedFixture(t)
			initial := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
			sharedFinish(t, root, initial)
			synthesisPromote(t, root, "auth")
			stable := "docs/specs/units/stable/unit_auth.md"
			id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "stable")
			grSubmitOK(t, root, id, "auth.core", grVerifyMismatchReport("auth.core", stable, "contracts.js", "P1"))
			grSubmitOK(t, root, id, "cross", grCrossReport(stable, "Description"))
			grFinalizeOK(t, root, id)
			binding := grReadJudgmentBaseline(t, root, "unit", "auth", "verify").Records["item:auth:auth.core"]
			path := filepath.Join(root, judgments.Directory, binding.ID+".json")
			var err error
			switch fault {
			case "missing":
				err = os.Remove(path)
			case "damaged":
				err = os.WriteFile(path, []byte("{}"), 0644)
			case "invalidated":
				err = judgments.Invalidate(root, binding.ID, "current evidence requires independent rechecking")
			}
			if err != nil {
				t.Fatal(err)
			}
			order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			run := mustLoadRun(t, root, order)
			if run.CoverageByKey("preserve:auth:auth.core").Source != "executed" {
				t.Fatal("unusable current decision resurrected historical ALIGNED protection")
			}
		})
	}
}

func TestDivergenceAnalysisCompletesBeforeItemAcceptance(t *testing.T) {
	root, main, _ := sharedFixture(t)
	grEnableMissionLayout(t, root)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	var out, errOut bytes.Buffer
	args := []string{"--repo-root", root, "--run", id, "--keys", "item:auth:auth.core", "--format", "prompt"}
	if err := runGateMission(args, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Steps 1-7") {
		t.Fatal("initial item mission omits divergence analysis")
	}
	grSubmitOK(t, root, id, "auth.core", grVerifyMismatchReport("auth.core", main, "contracts.js", "P1"))
	if err := runGateMission(args, &out, &errOut); err == nil {
		t.Fatal("accepted item task was reopened for a second analysis")
	}
	doc, err := os.ReadFile("../../../framework/unit_verify_checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(doc), "The tool supplies the accepted alignment result and report") {
		t.Fatal("Step 7 still requests a mission after item acceptance")
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// assertSynthesisEvidenceRecorded verifies that the evidence a final
// synthesis relied on is part of the recorded input surface (its file is a
// cache entry), so a later change to it is visible to the change report, and
// that the final synthesis never creates an independent cache check.
func assertSynthesisEvidenceRecorded(t *testing.T, root, gate, path string) {
	t.Helper()
	baseline, err := validationcache.ReadGateBaseline(root, "unit", "auth", gate)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range baseline.Entries {
		for _, check := range entry.Checks {
			if check.Check == gaterun.CrossKey {
				t.Fatal("final synthesis created an independent cache check")
			}
		}
		if gaterun.CanonicalDeclPath(root, entry.Path) == gaterun.CanonicalDeclPath(root, path) {
			found = true
		}
	}
	if !found {
		t.Fatalf("synthesis evidence %s is missing from the recorded input surface", path)
	}
}

func TestValidateSuppressionEvidenceControlsFreshnessAndPromote(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	proof := "evidence/decision.md"
	grWriteFile(t, root, proof, "The complete source resolves the apparent contradiction.\n")
	id := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, proof))
	desc := main + ": Description"
	accept := main + ": Testability / Acceptance Criteria"
	grSubmitOK(t, root, id, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{"1": {main + ": frontmatter", desc}, "3": {accept}, "6": {accept}}))
	grSubmitOK(t, root, id, "design", "2. Design soundness: FAIL — apparent contradiction\n4. Evidence-driven consistency: PASS — checked\n[P1] design — apparent contradiction (actionable)\n  problem: two design statements appear to disagree\n  evidence: both design statements\n  impact: implementation direction is unclear\n  fix: resolve the apparent contradiction\ncheck-2: "+desc+"\ncheck-4: "+desc+"\nFinding affects: "+grRunFindingID(id, "design", 1)+" = 2\n")
	grSubmitOK(t, root, id, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {accept}}))
	grSubmitOK(t, root, id, "dependencies", grDependenciesReport(map[string][]string{"7": {desc}, "8": {desc}}))
	grSubmitClarity(t, root, id, main)
	grSubmitOK(t, root, id, "cross", "Cross-check: PASS — complete decision resolves the apparent contradiction\nFinding disposition: "+grRunFindingID(id, "design", 1)+" = suppressed — the decision evidence resolves the apparent contradiction\nEffective status: 2 = pass\nEffective status: cross = pass\ncross: "+proof+": all\n")
	grFinalizeOK(t, root, id)
	assertSynthesisEvidenceRecorded(t, root, "validate", proof)
	currentVerifyFixture(t, root, "auth", "candidate", "")
	check, err := validationcache.CheckValidate(root, "auth")
	if err != nil || !check.Fresh {
		t.Fatalf("unchanged suppression evidence must be fresh: %+v %v", check, err)
	}

	// An unrelated delta must carry the suppression's dependency with check 2.
	delta := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--rerun", "5", "--inputs-file", grInputsManifest(t, proof))
	grReviewAccept(t, root, delta)
	grSubmitPlannedValidateSessions(t, root, delta, main)
	grFinalizeOK(t, root, delta)
	assertSynthesisEvidenceRecorded(t, root, "validate", proof)

	grWriteFile(t, root, proof, "The decision now confirms the design contradiction.\n")
	check, err = validationcache.CheckValidate(root, "auth")
	if err != nil || check.Fresh || check.Category != validationcache.CategoryStale {
		t.Fatalf("changed suppression evidence must stale validate: %+v %v", check, err)
	}
	var out, errOut bytes.Buffer
	if err := runFresh([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "validate  STALE") {
		t.Fatalf("fresh command did not report stale validate evidence: %s", out.String())
	}
	out.Reset()
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err == nil || !strings.Contains(out.String(), "Validate cache check: FAIL") {
		t.Fatalf("promote accepted stale suppression evidence: %v %s", err, out.String())
	}
	if _, err := os.Stat(main); err != nil {
		t.Fatalf("rejected promotion changed the candidate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs/specs/units/stable/unit_auth.md")); !os.IsNotExist(err) {
		t.Fatalf("rejected promotion created a stable unit: %v", err)
	}

	delta = grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--inputs-file", grInputsManifest(t, proof))
	run := mustLoadRun(t, root, delta)
	if got := strings.Join(coverageKeysOf(run), ","); got != "review" {
		t.Fatalf("expected the review coverage only, got %s", got)
	}
	// Changed suppression evidence must re-run the design group only.
	grReviewRecheck(t, root, delta, "2")
	run = mustLoadRun(t, root, delta)
	if len(run.Coverage) != 2 || run.Coverage[0].Key != "review" || run.Coverage[1].Key != "design" || len(run.Relationships) != 0 {
		t.Fatalf("changed suppression evidence must rerun only design checks 2/4: %+v", run)
	}
	grSubmitOK(t, root, delta, "design", grValidateReport([]string{"2", "4"}, map[string][]string{"2": {desc, proof + ": all"}, "4": {desc}}))
	grFinalizeOK(t, root, delta)
	check, err = validationcache.CheckValidate(root, "auth")
	if err != nil || !check.Fresh {
		t.Fatalf("rechecked evidence must restore freshness: %+v %v", check, err)
	}
	out.Reset()
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatalf("promote rejected the rechecked evidence: %v %s", err, out.String())
	}
}

func TestValidateSynthesisExtendsCarriedJudgmentEvidence(t *testing.T) {
	root := createCLITestRepo(t)
	main := grWriteSpec(t, root, "auth")
	id := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitPlannedValidateSessions(t, root, id, main)
	grFinalizeOK(t, root, id)
	proof := "evidence/decision.md"
	grWriteFile(t, root, proof, "The combined design violates the contract.\n")
	contract := "evidence/contract.md"
	grWriteFile(t, root, contract, "The contract requires a consistent combined design.\n")
	delta := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--relationships", "design_constraints", "--inputs-file", grInputsManifest(t, proof, contract))
	if run := mustLoadRun(t, root, delta); strings.Join(coverageKeysOf(run), ",") != "review" || !stringInList(run.CarriedKeys, "2") {
		t.Fatalf("relationship-only run must carry the local design judgment: %+v", run)
	}
	grReviewAccept(t, root, delta)
	relationship := gaterun.RelationshipKey("design_constraints")
	report := relationshipReport(t, root, delta, map[string]string{"design_constraints": contract}, "design_constraints")
	report = strings.ReplaceAll(report, "Finding affects: "+delta+"/cross/F1 = "+relationship, "Finding affects: "+delta+"/cross/F1 = "+relationship+", 2")
	report = strings.ReplaceAll(report, "Effective status: 2 = pass", "Effective status: 2 = fail")
	report += "cross: " + proof + ": all\n"
	grSubmitOK(t, root, delta, "cross", report)
	grFinalizeOK(t, root, delta)
	assertSynthesisEvidenceRecorded(t, root, "validate", proof)

	// A finding that connects a new relationship and a carried local judgment
	// must keep both sides' original evidence, not only the new cross input.
	original, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(original), "\n## Description\n", "\n## Description\n\nRevised design details.\n", 1)
	if err := os.WriteFile(main, []byte(changed), 0644); err != nil {
		t.Fatal(err)
	}
	changeReport, rerr := validationcache.DeriveChangeReport(root, "unit", "auth", "validate")
	if rerr != nil || changeReport.Empty() {
		t.Fatalf("a recorded main-spec change must appear in the change report: %+v %v", changeReport, rerr)
	}
	if err := os.WriteFile(main, original, 0644); err != nil {
		t.Fatal(err)
	}

	grWriteFile(t, root, proof, "The combined design now satisfies the contract.\n")
	changeReport, rerr = validationcache.DeriveChangeReport(root, "unit", "auth", "validate")
	if rerr != nil {
		t.Fatal(err)
	}
	localized := false
	for _, entry := range changeReport.Entries {
		if gaterun.CanonicalDeclPath(root, entry.Path) == gaterun.CanonicalDeclPath(root, proof) {
			localized = true
		}
	}
	if !localized {
		t.Fatalf("new synthesis evidence must appear in the change report: %+v", changeReport)
	}
	repair := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair", "--inputs-file", grInputsManifest(t, proof))
	run := mustLoadRun(t, root, repair)
	if strings.Join(coverageKeysOf(run), ",") != "review,design" || strings.Join(run.Relationships, ",") != "design_constraints" {
		t.Fatalf("repair must review the changed evidence and recheck the failed judgment and relationship: %+v", run)
	}
	grReviewAccept(t, root, repair)
	grSubmitPlannedValidateSessions(t, root, repair, main)
	grSubmitOK(t, root, repair, "cross", relationshipReport(t, root, repair, map[string]string{"design_constraints": proof}, ""))
	grFinalizeOK(t, root, repair)
	check, err := validationcache.CheckValidate(root, "auth")
	if err != nil || !check.Fresh {
		t.Fatalf("repaired synthesis must restore freshness: %+v %v", check, err)
	}
}

func TestVerifySuppressionExtendsCarriedItemEvidence(t *testing.T) {
	root, main, _ := sharedFixture(t)
	proof := "evidence/decision.md"
	grWriteFile(t, root, proof, "The full contract resolves the local mismatch.\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency", "--inputs-file", grInputsManifest(t, proof))
	grSubmitOK(t, root, id, "auth.core", grVerifyMismatchReport("auth.core", main, "contracts.js", "P2"))
	grAutoSubmitQuality(t, root, id)
	grSubmitOK(t, root, id, "cross", relationshipReport(t, root, id, map[string]string{"contract_consistency": proof}, ""))
	grFinalizeOK(t, root, id)
	finding := grReadJudgmentBaseline(t, root, "unit", "auth", "verify").Findings[0].ID

	delta := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--relationships", "contract_consistency", "--inputs-file", grInputsManifest(t, proof))
	key := "item:auth:auth.core"
	if run := mustLoadRun(t, root, delta); strings.Join(coverageKeysOf(run), ",") != "review" || !stringInList(run.CarriedKeys, key) {
		t.Fatalf("relationship-only run must carry the item: %+v", run)
	}
	grReviewAccept(t, root, delta)
	report := relationshipReport(t, root, delta, map[string]string{"contract_consistency": proof}, "")
	report = strings.ReplaceAll(report, "Finding disposition: "+finding+" = retained", "Finding disposition: "+finding+" = suppressed — the complete decision proves alignment")
	report = strings.ReplaceAll(report, "Effective status: "+key+" = fail", "Effective status: "+key+" = pass")
	report += "cross: " + proof + ": all\ncross: " + main + ": Description\n"
	grSubmitOK(t, root, delta, "cross", report)
	grFinalizeOK(t, root, delta)
	assertSynthesisEvidenceRecorded(t, root, "verify", proof)
	record := synthesisItemRecord(t, root, "auth")
	if record.Verdict != "ALIGNED" {
		t.Fatalf("carried mismatch was not suppressed: %s", record.Verdict)
	}
	state := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	binding := state.Records[key]
	grWriteFile(t, root, proof, "The full contract confirms the local mismatch.\n")
	// The item record pins only its own spec object: a changed synthesis
	// evidence file does not invalidate it mechanically — the recorded change
	// set below is what the delta review judges.
	if err := judgments.Check(root, binding.Reference, binding.Layer, judgments.Protocol(root)); err != nil {
		t.Fatalf("the item record's own object is unchanged; it must stay valid: %v", err)
	}
	check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate")
	if err != nil || check.Fresh {
		t.Fatalf("changed synthesis evidence left verify fresh: %+v %v", check, err)
	}
	changeReport, rerr := validationcache.DeriveChangeReport(root, "unit", "auth", "verify")
	if rerr != nil || changeReport.Empty() {
		t.Fatalf("changed synthesis evidence did not appear in the change report: %+v %v", changeReport, rerr)
	}
}

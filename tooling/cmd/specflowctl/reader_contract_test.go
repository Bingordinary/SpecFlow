package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// readerPacketResult loads the reader packet's accepted result.
func readerPacketResult(t *testing.T, repoRoot, runID string) *gaterun.PacketResult {
	t.Helper()
	run := mustLoadRun(t, repoRoot, runID)
	state, err := gaterun.LoadPacketState(repoRoot, run, "reader")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != gaterun.PacketAccepted || state.Result == nil {
		t.Fatalf("reader packet is %s with no accepted result", state.Status)
	}
	return state.Result
}

func verifierPacketResult(t *testing.T, repoRoot, runID string) *gaterun.PacketResult {
	t.Helper()
	run := mustLoadRun(t, repoRoot, runID)
	state, err := gaterun.LoadPacketState(repoRoot, run, "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != gaterun.PacketAccepted || state.Result == nil {
		t.Fatalf("verifier packet is %s with no accepted result", state.Status)
	}
	return state.Result
}

func TestReaderReconstructionRejectsInvalidReports(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nAppendix prose.\n")

	cases := []struct {
		name     string
		report   string
		contains string
	}{
		{
			name:     "missing Reconstruction header",
			report:   "The design.\n\nUndetermined:\n- none\n\nDependency scope:\ncheck-10: " + main + ": Description\n",
			contains: "must start with",
		},
		{
			name:     "blank line splits the restatement",
			report:   grReaderReconCustom(main, "Description", "First thought.\n\nSecond thought."),
			contains: "one contiguous block",
		},
		{
			name:     "missing Undetermined list",
			report:   "Reconstruction:\nThe design is stated here in one block.\n\nDependency scope:\ncheck-10: " + main + ": Description\n",
			contains: "missing the `Undetermined:` list",
		},
		{
			name:     "empty Undetermined list",
			report:   "Reconstruction:\nThe design is stated here in one block.\n\nUndetermined:\n\nDependency scope:\ncheck-10: " + main + ": Description\n",
			contains: "Undetermined list is empty",
		},
		{
			name:     "none combined with entries",
			report:   grReaderReconCustom(main, "Description", "", "state location unclear", "none"),
			contains: "cannot be combined",
		},
		{
			name:     "entry without the list marker",
			report:   "Reconstruction:\nThe design is stated here in one block.\n\nUndetermined:\nstate location unclear\n\nDependency scope:\ncheck-10: " + main + ": Description\n",
			contains: "`- {point}` lines",
		},
		{
			name:     "authored finding entry",
			report:   grReaderReconstruction(main, "Description") + "\n[P1] reader — authored finding (actionable)\n",
			contains: "evidence only",
		},
		{
			name:     "reader scope must be the main spec only",
			report:   grReaderReconstruction(main, "Description") + "check-10: " + appendix + ": all\n",
			contains: "reads the unit main spec",
		},
		{
			name:     "reader scope must cover exactly the narrative sections",
			report:   grReaderReconstruction(main, "Description") + "check-10: " + main + ": Testability / Acceptance Criteria\n",
			contains: "human-readable",
		},
		{
			name:     "whole-file reader scope is not the section list",
			report:   "Reconstruction:\nThe design is stated here in one block.\n\nUndetermined:\n- none\n\nDependency scope:\ncheck-10: " + main + ": all\n",
			contains: "not a human-readable section",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
			_, err := grSubmit(t, repoRoot, runID, "reader", tc.report)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected rejection containing %q, got %v", tc.contains, err)
			}
		})
	}
}

func TestReaderReconstructionAcceptsValidReport(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))
	result := readerPacketResult(t, repoRoot, runID)
	if len(result.Verdicts) != 0 {
		t.Fatalf("the reader authors no verdict, got %v", result.Verdicts)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("the reader authors no findings, got %+v", result.Findings)
	}
}

func TestVerifierReconciliationScopeContract(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nAppendix prose.\n")

	cases := []struct {
		name     string
		spec     *grVerifierReconSpec
		contains string
	}{
		{
			name:     "missing acceptance_items line",
			spec:     &grVerifierReconSpec{Scope: []string{"check-10: " + main + ": Description", "check-10: " + appendix + ": all"}},
			contains: "acceptance_items",
		},
		{
			name:     "whole-file main scope is not the section list",
			spec:     &grVerifierReconSpec{Scope: []string{"check-10: " + main + ": all"}},
			contains: "acceptance_items",
		},
		{
			name:     "an extra declared section breaks the exact set",
			spec:     &grVerifierReconSpec{Appendices: []string{appendix}},
			contains: "", // filled below: extra line appended via Extra is not a scope; use Scope
		},
		{
			name:     "missing appendix line",
			spec:     &grVerifierReconSpec{},
			contains: "protocol appendices",
		},
		{
			name:     "appendix range declaration is rejected",
			spec:     &grVerifierReconSpec{Scope: []string{"check-10: " + main + ": Description", "check-10: " + main + ": acceptance_items", "check-10: " + appendix + ": 1-2"}},
			contains: "whole file",
		},
		{
			name:     "a non-appendix path is rejected",
			spec:     &grVerifierReconSpec{Scope: []string{"check-10: " + main + ": Description", "check-10: " + main + ": acceptance_items", "check-10: docs/other.md: all"}},
			contains: "the verifier packet reads the main spec and its protocol appendices",
		},
	}
	cases[2].spec = &grVerifierReconSpec{Scope: []string{
		"check-10: " + main + ": Description",
		"check-10: " + main + ": Testability / Acceptance Criteria",
		"check-10: " + main + ": acceptance_items",
		"check-10: " + appendix + ": all",
	}}
	cases[2].contains = "human-readable"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
			items := grCarrierItemIDs(t, repoRoot, runID)
			grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))
			report := grVerifierReconciliation(main, "Description", items, tc.spec)
			if _, err := grSubmit(t, repoRoot, runID, "verifier", report); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected rejection containing %q, got %v", tc.contains, err)
			}
		})
	}
}

func TestVerifierReconciliationMalformedLines(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	cases := []struct {
		name     string
		spec     *grVerifierReconSpec
		contains string
	}{
		{
			name:     "no claim lines",
			spec:     &grVerifierReconSpec{DropClaims: true},
			contains: "at least one claim",
		},
		{
			name: "non-sequential claim ids",
			spec: &grVerifierReconSpec{Claims: []string{
				"Claim: C01 = supported — stated",
				"Claim: C03 = supported — stated",
			}},
			contains: "sequential from C01",
		},
		{
			name:     "malformed carrier line",
			spec:     &grVerifierReconSpec{Extra: []string{"Carrier: auth.core seen"}},
			contains: "malformed Carrier line",
		},
		{
			name:     "missing carrier mapping",
			spec:     &grVerifierReconSpec{Drop: "carriers"},
			contains: "one Carrier line per acceptance item",
		},
		{
			name: "carrier in wrong order",
			spec: &grVerifierReconSpec{
				Drop:    "carriers",
				Extra:   []string{"Carrier: wrong.id = seen — stated"},
				Verdict: "PASS",
			},
			contains: "expected",
		},
		{
			name:     "missing must-close mapping",
			spec:     &grVerifierReconSpec{Drop: "mustcloses"},
			contains: "one Must-close line per §9 decision",
		},
		{
			name:     "missing consistency line",
			spec:     &grVerifierReconSpec{Drop: "consistency"},
			contains: "exactly one `Consistency:",
		},
	}
	cases[3].contains = "one Carrier line per acceptance item"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
			items := grCarrierItemIDs(t, repoRoot, runID)
			grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))
			report := grVerifierReconciliation(main, "Description", items, tc.spec)
			if _, err := grSubmit(t, repoRoot, runID, "verifier", report); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected rejection containing %q, got %v", tc.contains, err)
			}
		})
	}
}

func TestVerifierReconciliationClaimsOrderAndClosure(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	items := grCarrierItemIDs(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))

	// A blocking classification with a PASS verdict contradicts the closure rule.
	blocking := grVerifierReconciliation(main, "Description", items, &grVerifierReconSpec{
		Verdict: "PASS",
		Claims:  []string{"Claim: C01 = unsupported-central — the narrative never states it"},
	})
	if _, err := grSubmit(t, repoRoot, runID, "verifier", blocking); err == nil || !strings.Contains(err.Error(), "contradicts its classifications") {
		t.Fatalf("expected the verdict-closure rejection, got %v", err)
	}
	grSubmitOK(t, repoRoot, runID, "verifier", grVerifierReconciliation(main, "Description", items, &grVerifierReconSpec{
		Verdict: "FAIL",
		Claims:  []string{"Claim: C01 = unsupported-central — the narrative never states it"},
	}))
	result := verifierPacketResult(t, repoRoot, runID)
	if got := result.Verdicts[gaterun.ReaderContractCheck]; got != "FAIL" {
		t.Fatalf("verifier verdict = %q, want FAIL", got)
	}
	if len(result.Findings) != 1 || result.Findings[0].Severity != "P1" || result.Findings[0].SourceKey != gaterun.ReaderContractCheck {
		t.Fatalf("unsupported-central must compose one P1 finding on check 10, got %+v", result.Findings)
	}
	if !strings.Contains(result.Findings[0].Detail, "C01") {
		t.Fatalf("the finding must name the claim, got %s", result.Findings[0].Detail)
	}
}

func TestVerifierReconciliationCarrierMustCloseConsistencyFindings(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	cases := []struct {
		name     string
		spec     *grVerifierReconSpec
		contains string
	}{
		{
			name: "missing carrier subject",
			spec: &grVerifierReconSpec{
				Verdict:  "FAIL",
				Carriers: map[string]string{"auth.core": "missing"},
				Claims:   []string{"Claim: C01 = supported — stated"},
			},
			contains: "behavioral subject is missing",
		},
		{
			name: "missing must-close decision",
			spec: &grVerifierReconSpec{
				Verdict:    "FAIL",
				MustCloses: map[string]string{"Q13": "missing"},
				Claims:     []string{"Claim: C01 = supported — stated"},
			},
			contains: "must-close decision Q13",
		},
		{
			name: "incoherent restatement",
			spec: &grVerifierReconSpec{
				Verdict:     "FAIL",
				Consistency: "incoherent",
				Claims:      []string{"Claim: C01 = supported — stated"},
			},
			contains: "internally incoherent",
		},
		{
			name: "contradicted claim",
			spec: &grVerifierReconSpec{
				Verdict: "FAIL",
				Claims:  []string{"Claim: C01 = contradicted — the acceptance item states otherwise"},
			},
			contains: "contradicted by the formal carrier",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
			items := grCarrierItemIDs(t, repoRoot, runID)
			grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))
			grSubmitOK(t, repoRoot, runID, "verifier", grVerifierReconciliation(main, "Description", items, tc.spec))
			result := verifierPacketResult(t, repoRoot, runID)
			if got := result.Verdicts[gaterun.ReaderContractCheck]; got != "FAIL" {
				t.Fatalf("verifier verdict = %q, want FAIL", got)
			}
			if len(result.Findings) != 1 || result.Findings[0].Severity != "P1" {
				t.Fatalf("want exactly one P1 finding, got %+v", result.Findings)
			}
			if !strings.Contains(result.Findings[0].Detail, tc.contains) {
				t.Fatalf("finding detail must contain %q, got %s", tc.contains, result.Findings[0].Detail)
			}
		})
	}
}

func TestVerifierReconciliationAdvisoryAndReaderErrorPass(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	items := grCarrierItemIDs(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))

	// An unsupported-minor claim is advisory; a reader-error claim is the
	// reader's own invention. Neither blocks and neither composes a finding.
	grSubmitOK(t, repoRoot, runID, "verifier", grVerifierReconciliation(main, "Description", items, &grVerifierReconSpec{
		Verdict: "PASS",
		Claims: []string{
			"Claim: C01 = unsupported-minor — a detail the narrative omits",
			"Claim: C02 = reader-error — the reader invented this",
			"Claim: C03 = supported — stated",
		},
	}))
	result := verifierPacketResult(t, repoRoot, runID)
	if got := result.Verdicts[gaterun.ReaderContractCheck]; got != "PASS" {
		t.Fatalf("verifier verdict = %q, want PASS", got)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("advisory and reader-error claims compose no findings, got %+v", result.Findings)
	}
}

func TestVerifierReconciliationAuthoredFindingsRejected(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	items := grCarrierItemIDs(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))
	report := grVerifierReconciliation(main, "Description", items, nil) + "\n[P1] verifier — authored finding (actionable)\n"
	if _, err := grSubmit(t, repoRoot, runID, "verifier", report); err == nil || !strings.Contains(err.Error(), "classifications only") {
		t.Fatalf("expected authored-finding rejection, got %v", err)
	}
}

func TestReaderContractFailureBlocksGate(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	desc := main + ": Description"
	accept := main + ": Testability / Acceptance Criteria"
	front := main + ": frontmatter"
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {front, desc},
		"3": {accept},
		"6": {accept},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {desc},
		"4": {desc},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {accept},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {desc},
		"8": {desc},
	}))
	grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))
	items := grCarrierItemIDs(t, repoRoot, runID)
	grSubmitOK(t, repoRoot, runID, "verifier", grVerifierReconciliation(main, "Description", items, &grVerifierReconSpec{
		Verdict:  "FAIL",
		Carriers: map[string]string{items[0]: "missing"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))

	out := grFinalizeOK(t, repoRoot, runID)
	if !strings.Contains(out, "Self-check: BLOCKED") {
		t.Fatalf("a reader contract failure must write a blocking failure record, got:\n%s", out)
	}
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "check: \"10\"") {
		t.Fatalf("the failure record must carry the failed check 10 verdict, got:\n%s", cache)
	}
}

func TestReaderContractFencedMarkerDoesNotShiftBoundary(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, main,
		"---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Background\n\nIntro prose.\n\n```text\nacceptance_item_set:\n```\n\n## Design narrative\n\nThe entry point is Main.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, repoRoot, runID)
	reader := run.PacketByID("reader")
	if reader == nil {
		t.Fatal("expected the reader packet")
	}
	if got := strings.Join(reader.Context, " | "); !strings.Contains(got, "Background") || !strings.Contains(got, "Design narrative") {
		t.Fatalf("the fenced example must not shift the narrative boundary, got context %q", got)
	}
	// The reading scope covers both narrative sections: the boundary is the
	// real acceptance section, not the fenced marker.
	report := grReaderReconCustom(main, "Design narrative", "The entry point is Main; auth verifies and returns.", "the token store shape") +
		"check-10: " + main + ": Background\n"
	grSubmitOK(t, repoRoot, runID, "reader", report)
}

func TestReaderContractEmptyCarrierSkipsProbe(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, main,
		"---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n")

	// An empty acceptance set gives the reconciliation no carrier: the
	// probe is skipped (Check 2 reports the defect instead).
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, repoRoot, runID)
	if ids := strings.Join(packetIDsOf(run), ","); strings.Contains(ids, "reader") || strings.Contains(ids, "verifier") {
		t.Fatalf("an empty acceptance set must skip the reader contract probe, got packets %s", ids)
	}
}

func TestReaderContractEmptyNarrativeFailsPlan(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, main,
		"---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n")

	_, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	if err == nil || !strings.Contains(err.Error(), "human-readable part") {
		t.Fatalf("expected the empty-narrative plan rejection, got %v", err)
	}
}

func TestReaderContractMarkerOutsideSectionsFailsPlan(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, main,
		"---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\nacceptance_item_set:\n  - id: auth.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n\n## Testability / Acceptance Criteria\n\n## After Section\n\nAfter prose.\n")

	_, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	if err == nil || !strings.Contains(err.Error(), "human-readable part") || !strings.Contains(err.Error(), "outside every ## section") {
		t.Fatalf("expected the marker-outside-sections plan rejection, got %v", err)
	}
}

func TestReaderContractStableBoundaryFailsPlanWithForkGuidance(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := "docs/specs/units/stable/unit_auth.md"
	grWriteFile(t, repoRoot, main,
		"---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n")

	_, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "stable")
	if err == nil || !strings.Contains(err.Error(), "specflowctl fork --unit auth") || !strings.Contains(err.Error(), "Stable-only Targets") {
		t.Fatalf("expected the stable boundary plan rejection to route through fork, got %v", err)
	}
}

func TestReaderContractMissionCarriesReconstructionAndCarrier(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	grWriteSpec(t, repoRoot, "auth")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	var out, errOut bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "reader", "--format", "prompt"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	prompt := out.String()
	for _, want := range []string{
		"Reconstruction:",
		"Undetermined:",
		"no question bank",
		"human-readable part (Check 10 reading scope):",
		"Description",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("reader mission is missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Question: Q01") || strings.Contains(prompt, "Quote:") {
		t.Fatalf("reader mission must not carry a question bank:\n%s", prompt)
	}
	if strings.Contains(prompt, "Previous submission was rejected") {
		t.Fatalf("first-attempt mission carries retry feedback:\n%s", prompt)
	}

	out.Reset()
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "verifier", "--format", "prompt"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "reader") {
		t.Fatalf("verifier mission must wait for the accepted reader packet, got %v", err)
	}

	grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction("docs/specs/units/candidate/unit_auth.md", "Description"))
	out.Reset()
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "verifier", "--format", "prompt"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	prompt = out.String()
	for _, want := range []string{
		"Carrier:",
		"Must-close: Q11",
		"Consistency:",
		"acceptance item set (Check 10 carrier backbone):",
		"auth.core",
		"protocol appendices (Check 10 carrier):",
		"Accepted report:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("verifier mission is missing %q:\n%s", want, prompt)
		}
	}
}

func TestReaderContractMissionReadRefs(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nAppendix prose.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	readRefsOf := func(packetID string) string {
		t.Helper()
		var out, errOut bytes.Buffer
		if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", packetID, "--format", "prompt"}, &out, &errOut); err != nil {
			t.Fatalf("%s mission: %v", packetID, err)
		}
		prompt := out.String()
		start := strings.Index(prompt, "Read refs:")
		if start < 0 {
			t.Fatalf("%s mission carries no Read refs section:\n%s", packetID, prompt)
		}
		readRefs := prompt[start:]
		if end := strings.Index(readRefs, "Packet context"); end >= 0 {
			readRefs = readRefs[:end]
		}
		return readRefs
	}

	readerRefs := readRefsOf("reader")
	if strings.Contains(readerRefs, appendix) {
		t.Fatalf("reader mission must not carry the appendix as a read ref:\n%s", readerRefs)
	}
	if !strings.Contains(readerRefs, main) {
		t.Fatalf("reader mission must carry the main spec %s as a read ref:\n%s", main, readerRefs)
	}
	grSubmitOK(t, repoRoot, runID, "reader", grReaderReconstruction(main, "Description"))
	verifierRefs := readRefsOf("verifier")
	if !strings.Contains(verifierRefs, main) || !strings.Contains(verifierRefs, appendix) {
		t.Fatalf("verifier mission must carry the main spec and the protocol appendix as read refs:\n%s", verifierRefs)
	}
}

func TestReaderContractDeltaCoupling(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, fullRun, main)
	grFinalizeOK(t, repoRoot, fullRun, "--result", "pass")

	// A narrative edit re-runs both packets together. The delta run is
	// completed so the baseline is a fresh pass for the next coupling case.
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	narrativeEdit := strings.Replace(string(data), "Prose.", "Prose. Extended narrative.", 1)
	if narrativeEdit == string(data) {
		t.Fatal("fixture assumption broken: narrative edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(narrativeEdit), 0644); err != nil {
		t.Fatal(err)
	}
	narrativeRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	narrative := mustLoadRun(t, repoRoot, narrativeRun)
	ids := strings.Join(packetIDsOf(narrative), ",")
	if !strings.Contains(ids, "reader,verifier") {
		t.Fatalf("narrative edit must re-run the reader and verifier packets together, got %s", ids)
	}
	if strings.Contains(strings.Join(narrative.CarriedKeys, ","), "10") {
		t.Fatalf("check 10 must not be carried over after a narrative edit, got %v", narrative.CarriedKeys)
	}
	grSubmitPlannedValidatePackets(t, repoRoot, narrativeRun, main)
	grFinalizeOK(t, repoRoot, narrativeRun, "--result", "pass")

	// An acceptance-item edit changes the formal carrier: check 10 re-runs.
	data, err = os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	acceptanceEdit := strings.Replace(string(data), "description: Behavior.", "description: Behavior updated.", 1)
	if acceptanceEdit == string(data) {
		t.Fatal("fixture assumption broken: acceptance edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(acceptanceEdit), 0644); err != nil {
		t.Fatal(err)
	}
	acceptanceRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	acceptance := mustLoadRun(t, repoRoot, acceptanceRun)
	ids = strings.Join(packetIDsOf(acceptance), ",")
	if !strings.Contains(ids, "reader,verifier") {
		t.Fatalf("an acceptance-item edit re-runs the reader and verifier packets together (the carrier changed), got %s", ids)
	}
	if strings.Contains(strings.Join(acceptance.CarriedKeys, ","), "10") {
		t.Fatalf("check 10 must not be carried over after an acceptance-item edit, got %v", acceptance.CarriedKeys)
	}
}

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// readerReportCustom builds a reader evidence report from the fixed bank,
// letting a test override individual question blocks (the override value
// replaces the Status/Answer/Quote/Location lines of that block).
func readerReportCustom(specPath, section, defaultQuote string, overrides map[string]string) string {
	var b strings.Builder
	for _, q := range readerContractQuestions {
		fmt.Fprintf(&b, "Question: %s — %s\n", q.ID, q.Title)
		if value, ok := overrides[q.ID]; ok {
			b.WriteString(value)
			b.WriteString("\n\n")
			continue
		}
		fmt.Fprintf(&b, "Status: answered\nAnswer: the fixture answer for %s\nQuote: %s\nLocation: %s\n\n", q.ID, defaultQuote, section)
	}
	fmt.Fprintf(&b, "Dependency scope:\ncheck-10: %s: %s\n", specPath, section)
	return b.String()
}

// verifierReportCustom builds a verifier report whose judgments are all
// supported except one override question.
func verifierReportCustom(specPath, section, verdict, overrideID, overrideStatus string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "10. Reader contract: %s — checked\n\n", verdict)
	for _, q := range readerContractQuestions {
		status := "supported"
		if q.ID == overrideID {
			status = overrideStatus
		}
		fmt.Fprintf(&b, "Judgment: %s = %s — the cited quote answers the question\n", q.ID, status)
	}
	fmt.Fprintf(&b, "\nDependency scope:\ncheck-10: %s: %s\n", specPath, section)
	return b.String()
}

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

func TestReaderContractPacketRejectsInvalidEvidence(t *testing.T) {
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
			name: "non-verbatim quote",
			report: readerReportCustom(main, "Description", "Prose.", map[string]string{
				"Q03": "Status: answered\nAnswer: the entry point\nQuote: this text does not exist in the spec\nLocation: Description",
			}),
			contains: "does not exist verbatim",
		},
		{
			name: "acceptance section is not a citation source",
			report: readerReportCustom(main, "Description", "Prose.", map[string]string{
				"Q04": "Status: answered\nAnswer: the normal path\nQuote: acceptance_item_set:\nLocation: Testability / Acceptance Criteria",
			}),
			contains: "not a human-readable section",
		},
		{
			name: "unknown section",
			report: readerReportCustom(main, "Description", "Prose.", map[string]string{
				"Q05": "Status: answered\nAnswer: boundaries\nQuote: Prose.\nLocation: Nonexistent Section",
			}),
			contains: "not a human-readable section",
		},
		{
			name: "quote below the length bound",
			report: readerReportCustom(main, "Description", "Prose.", map[string]string{
				"Q06": "Status: answered\nAnswer: state\nQuote: ro\nLocation: Description",
			}),
			contains: "bound is",
		},
		{
			name: "no_local_answer must not carry an approximation",
			report: readerReportCustom(main, "Description", "Prose.", map[string]string{
				"Q07": "Status: no_local_answer\nAnswer: approximated\nQuote: Prose.\nLocation: Description",
			}),
			contains: "do not approximate",
		},
		{
			name: "wrong question title",
			report: strings.Replace(readerReportCustom(main, "Description", "Prose.", nil),
				"Question: Q02 — "+readerQuestionByID("Q02"), "Question: Q02 — something else", 1),
			contains: "does not match the fixed bank",
		},
		{
			name:     "authored finding entry",
			report:   readerReportCustom(main, "Description", "Prose.", nil) + "\n[P1] reader — authored finding\n",
			contains: "evidence only",
		},
		{
			name:     "scope must cover exactly the narrative sections",
			report:   readerReportCustom(main, "Description", "Prose.", nil) + "check-10: " + main + ": Testability / Acceptance Criteria\n",
			contains: "human-readable",
		},
		{
			name:     "citation outside the main spec is rejected",
			report:   grReaderEvidenceReport(main, "Description", "Prose.") + "check-10: " + appendix + ": all\n",
			contains: "cite only the unit main spec",
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

func TestReaderContractNoAnswerFailsAndVerifierMustAgree(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	report := readerReportCustom(main, "Description", "Prose.", map[string]string{
		"Q07": "Status: no_local_answer",
	})
	grSubmitOK(t, repoRoot, runID, "reader", report)
	result := readerPacketResult(t, repoRoot, runID)
	if got := result.Verdicts[gaterun.ReaderContractCheck]; got != "FAIL" {
		t.Fatalf("reader verdict = %q, want FAIL", got)
	}
	if len(result.Findings) != 1 || result.Findings[0].Severity != "P1" || result.Findings[0].SourceKey != gaterun.ReaderContractCheck {
		t.Fatalf("no_local_answer must compose one P1 finding on check 10, got %+v", result.Findings)
	}
	if !strings.Contains(result.Findings[0].Detail, "Q07") {
		t.Fatalf("the finding must name the unanswered question, got %s", result.Findings[0].Detail)
	}

	// A verifier PASS contradicts the accepted reader's no_local_answer.
	if _, err := grSubmit(t, repoRoot, runID, "verifier", verifierReportCustom(main, "Description", "PASS", "", "supported")); err == nil || !strings.Contains(err.Error(), "must judge it unsupported") {
		t.Fatalf("expected the no_local_answer closure rejection, got %v", err)
	}
	// Judging the question partial is not enough — it must be unsupported.
	if _, err := grSubmit(t, repoRoot, runID, "verifier", verifierReportCustom(main, "Description", "FAIL", "Q07", "partial")); err == nil || !strings.Contains(err.Error(), "must judge it unsupported") {
		t.Fatalf("expected the unsupported-required rejection, got %v", err)
	}
	grSubmitOK(t, repoRoot, runID, "verifier", verifierReportCustom(main, "Description", "FAIL", "Q07", "unsupported"))
}

func TestReaderContractVerifierUnsupportedComposesFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "reader", grReaderEvidenceReport(main, "Description", "Prose."))

	// An unsupported judgment with a PASS verdict contradicts the closure rule.
	if _, err := grSubmit(t, repoRoot, runID, "verifier", verifierReportCustom(main, "Description", "PASS", "Q09", "unsupported")); err == nil || !strings.Contains(err.Error(), "contradicts") {
		t.Fatalf("expected the verdict-closure rejection, got %v", err)
	}
	grSubmitOK(t, repoRoot, runID, "verifier", verifierReportCustom(main, "Description", "FAIL", "Q09", "unsupported"))

	run := mustLoadRun(t, repoRoot, runID)
	state, err := gaterun.LoadPacketState(repoRoot, run, "verifier")
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Result.Verdicts[gaterun.ReaderContractCheck]; got != "FAIL" {
		t.Fatalf("verifier verdict = %q, want FAIL", got)
	}
	if len(state.Result.Findings) != 1 || !strings.Contains(state.Result.Findings[0].Detail, "Q09") {
		t.Fatalf("unsupported judgment must compose one finding naming Q09, got %+v", state.Result.Findings)
	}
}

func TestReaderContractAuthoredVerifierFindingsRejected(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "reader", grReaderEvidenceReport(main, "Description", "Prose."))
	report := grVerifierReport(main, "Description", "PASS", "supported") + "\n[P1] verifier — authored finding (actionable)\n"
	if _, err := grSubmit(t, repoRoot, runID, "verifier", report); err == nil || !strings.Contains(err.Error(), "judgments only") {
		t.Fatalf("expected authored-finding rejection, got %v", err)
	}
}

func TestReaderContractVerifierScopeMustMatchNarrative(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	cases := []struct {
		name     string
		scope    string
		contains string
	}{
		{
			name:     "whole-file scope is not the exact section list",
			scope:    "check-10: " + main + ": all\n",
			contains: "not a human-readable section",
		},
		{
			name:     "an extra declared section breaks the exact set",
			scope:    "check-10: " + main + ": Description\ncheck-10: " + main + ": Testability / Acceptance Criteria\n",
			contains: "human-readable sections",
		},
		{
			name:     "the acceptance section is not a citation source",
			scope:    "check-10: " + main + ": Testability / Acceptance Criteria\n",
			contains: "not a human-readable section",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
			grSubmitOK(t, repoRoot, runID, "reader", grReaderEvidenceReport(main, "Description", "Prose."))
			report := "10. Reader contract: PASS — checked\n\n"
			for _, q := range readerContractQuestions {
				report += fmt.Sprintf("Judgment: %s = supported — the cited quote answers the question\n", q.ID)
			}
			report += "\nDependency scope:\n" + tc.scope
			if _, err := grSubmit(t, repoRoot, runID, "verifier", report); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("expected rejection containing %q, got %v", tc.contains, err)
			}
		})
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
	grSubmitOK(t, repoRoot, runID, "reader", readerReportCustom(main, "Description", "Prose.", map[string]string{
		"Q13": "Status: no_local_answer",
	}))
	grSubmitOK(t, repoRoot, runID, "verifier", verifierReportCustom(main, "Description", "FAIL", "Q13", "unsupported"))
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
	// A citation from the section after the fenced example is legal: the
	// boundary is the real acceptance section, not the fenced marker.
	report := readerReportCustom(main, "Design narrative", "The entry point is Main.", nil) +
		"check-10: " + main + ": Background\n"
	grSubmitOK(t, repoRoot, runID, "reader", report)
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

func TestReaderContractMissionCarriesBankAndContext(t *testing.T) {
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
		"Question: Q01 — intended user, actor, or caller",
		"Question: Q17 — how acceptance proves the stated responsibility",
		"human-readable part (Check 10 citation source):",
		"Description",
		"no_local_answer",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("reader mission is missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Previous submission was rejected") {
		t.Fatalf("first-attempt mission carries retry feedback:\n%s", prompt)
	}

	out.Reset()
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "verifier", "--format", "prompt"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "reader") {
		t.Fatalf("verifier mission must wait for the accepted reader packet, got %v", err)
	}
}

func TestReaderContractMissionReadRefsAreMainSpecOnly(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nAppendix prose.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	assertReadRefs := func(packetID string) {
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
		if strings.Contains(readRefs, appendix) {
			t.Fatalf("%s mission must not carry the appendix as a read ref:\n%s", packetID, readRefs)
		}
		if !strings.Contains(readRefs, main) {
			t.Fatalf("%s mission must carry the main spec %s as a read ref:\n%s", packetID, main, readRefs)
		}
	}

	assertReadRefs("reader")
	grSubmitOK(t, repoRoot, runID, "reader", grReaderEvidenceReport(main, "Description", "Prose."))
	assertReadRefs("verifier")
}

func TestReaderContractDeltaCoupling(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, fullRun, main)
	grFinalizeOK(t, repoRoot, fullRun, "--result", "pass")

	// An acceptance-only edit does not touch the narrative: both reader
	// contract packets are carried over.
	data, err := os.ReadFile(specPath)
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
	deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	delta := mustLoadRun(t, repoRoot, deltaRun)
	if ids := strings.Join(packetIDsOf(delta), ","); strings.Contains(ids, "reader") || strings.Contains(ids, "verifier") {
		t.Fatalf("acceptance-only edit must carry the reader contract, got packets %s", ids)
	}
	if !strings.Contains(strings.Join(delta.CarriedKeys, ","), "10") {
		t.Fatalf("check 10 must be carried over, got %v", delta.CarriedKeys)
	}

	// A narrative edit re-runs both packets together.
	narrativeEdit := strings.Replace(acceptanceEdit, "Prose.", "Prose. Extended narrative.", 1)
	if narrativeEdit == acceptanceEdit {
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
}

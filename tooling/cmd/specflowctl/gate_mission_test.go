package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulevalidation"
)

func TestRuleMissionIncludesTargetLayerApplicability(t *testing.T) {
	cases := []struct {
		name, target      string
		candidate, stable bool
	}{
		{"stable confirmation", "stable", false, true},
		{"candidate over stable", "candidate", true, true},
		{"new candidate", "candidate", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := createCLITestRepo(t)
			grEnableMissionLayout(t, root)
			const id = "b_rule_consistency"
			for layer, present := range map[string]bool{"candidate": tc.candidate, "stable": tc.stable} {
				if present {
					grWriteFile(t, root, "docs/specs/rules/"+layer+"/"+id+".md", "---\nrule_id: "+id+"\nrule_scope: bound\n---\n\n# Constraint\n\nValidate input before processing.\n")
				}
			}
			runID := grPlan(t, root, "--gate", "validate", "--rule", id, "--target", tc.target)
			var out, errOut bytes.Buffer
			if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", "3,4", "--format", "prompt"}, &out, &errOut); err != nil {
				t.Fatalf("mission: %v (%s)", err, errOut.String())
			}
			for _, want := range []string{"Target layer: " + tc.target, "Spec source: docs/specs/rules/" + tc.target + "/" + id + ".md", "Target-layer applicability", "checks 3, 4"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("mission omits %q:\n%s", want, out.String())
				}
			}
			if tc.target == "candidate" {
				result := rulevalidation.ValidateRule(root, id)
				if !result.Passed {
					t.Fatalf("candidate validation failed: %+v", result)
				}
			}
		})
	}
}

func TestGateMissionPlanStatusAndPrompt(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	var out, errOut bytes.Buffer
	if err := runGatePlan([]string{"--repo-root", root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", "none", "--format", "json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var planned gateRunView
	if err := json.Unmarshal(out.Bytes(), &planned); err != nil {
		t.Fatal(err)
	}
	if planned.SchemaVersion != 3 || planned.NextAction != "execute" || len(planned.UncoveredKeys) != 5 || strings.Contains(strings.Join(planned.UncoveredKeys, ","), "cross") {
		t.Fatalf("wrong planned coverage: %+v", planned)
	}
	// The plan's next step is execute, so the JSON carries the tooling-computed
	// default reviewer batches: one batch holding every still-uncovered
	// validate check key, and no split-required keys.
	if len(planned.ReviewerBatches) != 1 || len(planned.ReviewerBatches[0]) != 5 || len(planned.SplitRequiredKeys) != 0 || !strings.Contains(out.String(), `"reviewer_batches"`) {
		t.Fatalf("plan JSON must expose the computed reviewer batches: %+v", planned)
	}
	if len(planned.Coverage) != 5 || planned.Coverage[0].CoveredBy != "" {
		t.Fatalf("wrong planned coverage detail: %+v", planned.Coverage)
	}
	if len(planned.DeferredFindings) != 0 || !strings.Contains(out.String(), `"deferred_findings": []`) {
		t.Fatalf("plan must show an empty deferred findings array: %s", out.String())
	}
	var status bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", root, "--run", planned.RunID, "--format", "json"}, &status, &errOut); err != nil {
		t.Fatal(err)
	}
	var current gateRunView
	if err := json.Unmarshal(status.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if current.RunID != planned.RunID || len(current.UncoveredKeys) != 5 {
		t.Fatalf("wrong status: %+v", current)
	}
	if len(current.DeferredFindings) != 0 || !strings.Contains(status.String(), `"deferred_findings": []`) {
		t.Fatalf("status must show an empty deferred findings array: %s", status.String())
	}
	var jsonOut, prompt bytes.Buffer
	args := []string{"--repo-root", root, "--run", planned.RunID, "--keys", "structural"}
	if err := runGateMission(append(append([]string{}, args...), "--format", "json"), &jsonOut, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := runGateMission(append(append([]string{}, args...), "--format", "prompt"), &prompt, &errOut); err != nil {
		t.Fatal(err)
	}
	var mission gateMission
	if err := json.Unmarshal(jsonOut.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	if len(mission.Sessions) != 1 || mission.Sessions[0].ProtocolRef != "framework/unit_validate_checklist.md" || mission.Sessions[0].SessionID != "structural" {
		t.Fatalf("wrong mission: %+v", mission)
	}
	if mission.Sessions[0].LastRejection != "" || strings.Contains(jsonOut.String(), "last_rejection") || strings.Contains(prompt.String(), "Previous submission was rejected:") {
		t.Fatalf("first-attempt mission carries retry feedback: %s", prompt.String())
	}
	for _, want := range []string{mission.RunID, mission.Sessions[0].ProtocolRef, "Checks: 1, 3, 6", "gate-submit"} {
		if !strings.Contains(prompt.String(), want) {
			t.Fatalf("prompt lacks %q:\n%s", want, prompt.String())
		}
	}
	report := mission.Sessions[0].ReportContract.Template
	for _, pair := range [][2]string{
		{"1. <check name>: <PASS|WARNING|FAIL> — <reason>", "1. Structural integrity: PASS — checked"},
		{"3. <check name>: <PASS|WARNING|FAIL> — <reason>", "3. Scope integrity: PASS — checked"},
		{"6. <check name>: <PASS|WARNING|FAIL> — <reason>", "6. Affects-source validity: PASS — checked"},
	} {
		report = strings.Replace(report, pair[0], pair[1], 1)
	}
	report = strings.Replace(report, "[P1] <location> — <finding> (actionable|needs_decision)\n  problem: <problem>\n  evidence: <evidence>\n  impact: <impact>\n  fix: <repair>\n", "", 1)
	report = strings.Replace(report, "Finding affects: <finding_id> = <check_key>[,<check_key>...]", "", 1)
	if strings.Contains(report, "<") {
		t.Fatalf("unfilled report template:\n%s", report)
	}
	invalid := strings.Replace(report, "3. Scope integrity: PASS — checked\n", "", 1)
	if _, err := grSubmit(t, root, planned.RunID, "structural", invalid); err == nil || !strings.Contains(err.Error(), "check \"3\" has no verdict") {
		t.Fatalf("missing template field was not rejected: %v", err)
	}
	retry := missionJSON(t, root, planned.RunID, "structural")
	if !strings.Contains(retry.Sessions[0].LastRejection, `check "3" has no verdict`) {
		t.Fatalf("retry mission lost rejection reason: %+v", retry.Sessions[0])
	}
	prompt.Reset()
	if err := runGateMission(append(append([]string{}, args...), "--format", "prompt"), &prompt, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.String(), "Previous submission was rejected: "+retry.Sessions[0].LastRejection) {
		t.Fatalf("retry prompt lost rejection reason: %s", prompt.String())
	}
	invalidAgain := strings.Replace(report, "1. Structural integrity: PASS — checked\n", "", 1)
	if _, err := grSubmit(t, root, planned.RunID, "structural", invalidAgain); err == nil || !strings.Contains(err.Error(), "check \"1\" has no verdict") {
		t.Fatalf("second invalid report was not rejected: %v", err)
	}
	retry = missionJSON(t, root, planned.RunID, "structural")
	if !strings.Contains(retry.Sessions[0].LastRejection, `check "1" has no verdict`) || strings.Contains(retry.Sessions[0].LastRejection, `check "3" has no verdict`) {
		t.Fatalf("retry mission did not use latest rejection: %+v", retry.Sessions[0])
	}
	prompt.Reset()
	if err := runGateMission(append(append([]string{}, args...), "--format", "prompt"), &prompt, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.String(), "Previous submission was rejected: "+retry.Sessions[0].LastRejection) || strings.Contains(prompt.String(), `check "3" has no verdict`) {
		t.Fatalf("retry prompt did not use latest rejection: %s", prompt.String())
	}
	status.Reset()
	if err := runGateStatus([]string{"--repo-root", root, "--run", planned.RunID, "--format", "json"}, &status, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(status.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(current.UncoveredKeys, ","), "structural") {
		t.Fatalf("rejected session's key is not uncovered: %+v", current)
	}
	grSubmitOK(t, root, planned.RunID, "structural", report)
	status.Reset()
	if err := runGateStatus([]string{"--repo-root", root, "--run", planned.RunID, "--format", "json"}, &status, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(status.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(current.UncoveredKeys, ","), "structural") {
		t.Fatalf("accepted session's key still uncovered: %+v", current)
	}
	if err := runGateMission(args, &prompt, &errOut); err == nil || !strings.Contains(err.Error(), "already covered") {
		t.Fatalf("accepted session received another mission: %v", err)
	}
}

// TestGateMissionPromptInlinesProtocol pins the self-contained mission: the
// prompt carries the protocol sections inline and never sends the reviewer to
// read the framework documents.
func TestGateMissionPromptInlinesProtocol(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	var out, errOut bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", "structural", "--format", "prompt"}, &out, &errOut); err != nil {
		t.Fatalf("mission: %v (%s)", err, errOut.String())
	}
	prompt := out.String()
	for _, want := range []string{
		"inlined judging standard",
		"----- Inlined protocol from framework/unit_validate_checklist.md",
		"## Check 1 — Structural integrity",
		"## Check 3 — Scope integrity",
		"## Check 6 — Affects-source validity",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks inlined protocol %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Read this scope") {
		t.Errorf("prompt still points the reviewer at the framework documents:\n%s", prompt)
	}
	var jsonOut bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", "structural", "--format", "json"}, &jsonOut, &errOut); err != nil {
		t.Fatalf("mission json: %v (%s)", err, errOut.String())
	}
	var mission gateMission
	if err := json.Unmarshal(jsonOut.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	inline := mission.Sessions[0].ProtocolInline
	if len(inline) != 7 {
		t.Fatalf("got %d inlined sections, want 7 (prerequisite, execution rules, severity, section 15.2, checks 1/3/6): %+v", len(inline), inline)
	}
	for _, chunk := range inline {
		if chunk.Content == "" {
			t.Fatalf("bad inlined section: %+v", chunk)
		}
	}
}

// TestGateMissionValidateFinalSynthesis pins the validate run's final
// synthesis: a validate run whose checks produced findings must materialize
// its final-synthesis mission from the same gate-independent section set as
// verify (§Final synthesis plus the severity policy), never by mapping the
// `cross` key onto a checklist check.
func TestGateMissionValidateFinalSynthesis(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, root, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, root, runID, "design", "2. Design soundness: FAIL — contradiction\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n[P1] design — contradiction (actionable)\n\ncheck-2: "+main+": Description\ncheck-4: "+main+": Description\n"+fmt.Sprintf("Finding affects: %s = 2\n", grRunFindingID(runID, "design", 1)))
	grSubmitOK(t, root, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Testability / Acceptance Criteria"}}))
	grSubmitOK(t, root, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, root, runID, main)
	var out, errOut bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--final"}, &out, &errOut); err != nil {
		t.Fatalf("validate final-synthesis mission: %v (%s)", err, errOut.String())
	}
	prompt := out.String()
	for _, want := range []string{
		"----- Inlined protocol from framework/verification_scope.md (Final synthesis)",
		"----- Inlined protocol from framework/severity_policy.md (4. Severity Levels)",
		"----- Inlined protocol from framework/severity_policy.md (9. Severity Consistency Check)",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks inlined protocol %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Check cross") {
		t.Errorf("prompt mapped the cross key onto a checklist check:\n%s", prompt)
	}
}

func TestGateMissionUsesInstalledFrameworkPath(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteFile(t, root, "specflow/tooling/manifest.tsv", "fixture\n")
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	var out, errOut bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", "structural", "--format", "json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var mission gateMission
	if err := json.Unmarshal(out.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	if mission.Sessions[0].ProtocolRef != "specflow/framework/unit_validate_checklist.md" {
		t.Fatalf("wrong deployed path: %s", mission.Sessions[0].ProtocolRef)
	}
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", "not-a-key", "--format", "json"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "not part of run") {
		t.Fatalf("unknown key must be rejected: %v", err)
	}
}

func missionJSON(t *testing.T, root, runID, sessionID string) gateMission {
	t.Helper()
	run, err := gaterun.Load(root, runID)
	if err != nil {
		t.Fatal(err)
	}
	keys := grSessionKeys(run, sessionID)
	var out, errOut bytes.Buffer
	args := []string{"--repo-root", root, "--run", runID, "--format", "json"}
	if sessionID == "cross" {
		args = append(args, "--final")
	} else {
		args = append(args, "--keys", strings.Join(keys, ","))
	}
	if err := runGateMission(args, &out, &errOut); err != nil {
		t.Fatalf("mission %s: %v (%s)", sessionID, err, errOut.String())
	}
	var mission gateMission
	if err := json.Unmarshal(out.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	return mission
}

func TestGateMissionsCoverRuleVerifyAndCross(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteFile(t, root, "docs/specs/rules/candidate/b_rule_http.md", "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")
	ruleRun := grPlan(t, root, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	rule := missionJSON(t, root, ruleRun, "checks")
	if rule.Sessions[0].ProtocolRef != "framework/rule_validate_checklist.md" || len(rule.Sessions[0].ReportContract.Verdicts) != 6 {
		t.Fatalf("wrong rule mission: %+v", rule.Sessions[0])
	}

	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	verifyRun := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	item := missionJSON(t, root, verifyRun, "auth.core")
	if item.Sessions[0].ProtocolScope != "Steps 2, 3, 4, 6, 7 for acceptance item(s) item:auth:auth.core" || !strings.Contains(item.Sessions[0].ReportContract.Template, "Part A:") || strings.Contains(item.Sessions[0].ReportContract.Template, "Part B:") || !strings.Contains(item.Sessions[0].ReportContract.Template, "Severity:") {
		t.Fatalf("wrong verify item mission: %+v", item.Sessions[0])
	}
	if !strings.Contains(strings.Join(item.Constraints, "\n"), "missing read ref: <repo-relative path>") {
		t.Fatalf("item mission lacks the missing-read-ref constraint: %v", item.Constraints)
	}
	grSubmitOK(t, root, verifyRun, "auth.core", grVerifyItemBody("auth.core", "MISMATCH (acceptance)", "broken at src/auth.go:1")+grVerifyAnalysisFields("auth.core")+"auth.core: "+main+": acceptance_item:auth.core\nauth.core: src/auth.go: all\n")

	code := missionJSON(t, root, verifyRun, "code:src/auth.go")
	if !strings.Contains(code.Sessions[0].ReportContract.Template, "conclusion: FACTS") ||
		strings.Contains(code.Sessions[0].ReportContract.Template, "Dependency scope:") {
		t.Fatalf("public code mission template must carry facts only, got:\n%s", code.Sessions[0].ReportContract.Template)
	}
	for _, req := range code.Sessions[0].ReportContract.Requirements {
		if strings.Contains(req.Description, "Dependency scope") {
			t.Fatalf("public code mission must not carry a dependency-scope requirement, got %+v", req)
		}
	}
	if !strings.Contains(strings.Join(code.Constraints, "\n"), "missing read ref: <repo-relative path>") {
		t.Fatalf("public code mission lacks the missing-read-ref constraint: %v", code.Constraints)
	}

	grPreparePublic(t, root, mustLoadRun(t, root, verifyRun), []string{"design:auth:src/auth.go"})
	file := missionJSON(t, root, verifyRun, "src/auth.go")
	if file.Sessions[0].ProtocolRef != "framework/unit_verify_checklist.md" || !strings.Contains(file.Sessions[0].ReportContract.Template, "spec_requirements:") {
		t.Fatalf("wrong quality file mission: %+v", file.Sessions[0])
	}
	grSubmitOK(t, root, verifyRun, "src/auth.go", grQualityReport("src/auth.go", main))
	cross := missionJSON(t, root, verifyRun, "cross")
	if len(cross.Sessions[0].Dependencies) != 3 || cross.Sessions[0].Dependencies[0].Digest == "" || len(cross.Sessions[0].Dependencies[0].Verdicts) == 0 ||
		len(cross.Sessions[0].ProtocolInline) == 0 || !strings.Contains(cross.Sessions[0].ReportContract.Template, "Finding disposition:") || strings.Contains(cross.Sessions[0].ReportContract.Template, "Effective status:") {
		t.Fatalf("wrong cross mission: %+v", cross.Sessions[0])
	}
}

// TestGateMissionGrantsReadOnlyFrameworkInspection pins the reviewer
// permission boundary in the generated mission: the read-only framework
// inspection subcommands the validate checks instruct (deps, surfaces,
// validate candidate) are granted, while the prohibition side still forbids
// file modification, state-changing commands, and sub-agent launch.
func TestGateMissionGrantsReadOnlyFrameworkInspection(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")

	grant := "specflowctl deps, specflowctl surfaces, specflowctl validate candidate"
	prohibition := "do not modify files, run state-changing commands, or launch sub-agents"

	validateRun := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	validateMission := missionJSON(t, root, validateRun, "dependencies")
	validateConstraints := strings.Join(validateMission.Constraints, "\n")
	if !strings.Contains(validateConstraints, grant) {
		t.Fatalf("validate mission does not grant the read-only framework inspection subcommands: %v", validateMission.Constraints)
	}
	if !strings.Contains(validateConstraints, prohibition) {
		t.Fatalf("validate mission lost the prohibition constraint: %v", validateMission.Constraints)
	}

	verifyRun := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	codeMission := missionJSON(t, root, verifyRun, "code:src/auth.go")
	if !strings.Contains(strings.Join(codeMission.Constraints, "\n"), grant) {
		t.Fatalf("verify code mission does not grant the read-only framework inspection subcommands: %v", codeMission.Constraints)
	}
}

// TestGateMissionCoBatchedContractFollowsEachKind pins the co-batched
// code+design session's report contract: each block follows its own coverage
// kind — the code block carries the facts form with no scope lines, and the
// design block carries the design template with its scope example.
func TestGateMissionCoBatchedContractFollowsEachKind(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")

	var out, errOut bytes.Buffer
	keys := "code:src/auth.go,design:auth:src/auth.go"
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", keys, "--format", "json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var mission gateMission
	if err := json.Unmarshal(out.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	template := mission.Sessions[0].ReportContract.Template
	if !strings.Contains(template, "File: code:src/auth.go\nconclusion: FACTS\nfacts:") {
		t.Fatalf("co-batched template must carry the code block's facts form, got:\n%s", template)
	}
	if strings.Contains(template, "File: code:src/auth.go\nconclusion: <acceptable") {
		t.Fatalf("co-batched template must not ask the code block for a design conclusion:\n%s", template)
	}
	if !strings.Contains(template, "File: design:auth:src/auth.go\nconclusion: <acceptable|needs_attention|unacceptable>") {
		t.Fatalf("co-batched template must carry the design block's conclusion form, got:\n%s", template)
	}
	// The report contract must state the two rules the validator enforces and
	// the short template form omitted: where a public finding block goes, and
	// the exact gate_findings grammar.
	if !strings.Contains(template, "a public potential finding is a finding block written inside this code block") {
		t.Fatalf("co-batched template must place public finding blocks inside the code block, got:\n%s", template)
	}
	if !strings.Contains(template, "gate_findings: none | [P0|P1] {finding};") {
		t.Fatalf("co-batched template must state the gate_findings grammar, got:\n%s", template)
	}
	requirementIDs := map[string]bool{}
	for _, rule := range mission.Sessions[0].ReportContract.Requirements {
		requirementIDs[rule.ID] = true
	}
	for _, id := range []string{"public-finding-placement", "quality-finding-id-order"} {
		if !requirementIDs[id] {
			t.Fatalf("co-batched report contract is missing requirement %q", id)
		}
	}

	// The printed submission command must be self-consistent: its --keys list
	// is exactly the batch its session id is derived from, so following the
	// printed command submits successfully.
	batch := []string{"code:src/auth.go", "design:auth:src/auth.go"}
	wantSession := gaterun.SessionID(batch)
	if mission.Sessions[0].SessionID != wantSession {
		t.Fatalf("session id = %q, want %q", mission.Sessions[0].SessionID, wantSession)
	}
	wantSubmission := "specflowctl gate-submit --run " + runID + " --session " + wantSession + " --keys code:src/auth.go,design:auth:src/auth.go --report REPORT_PATH"
	if mission.Submission != wantSubmission {
		t.Fatalf("submission command = %q, want %q", mission.Submission, wantSubmission)
	}
}

// TestGateMissionCoBatchedPrintedSubmissionSubmits follows the mission's
// printed submission command verbatim (its session id and --keys list) and
// confirms gate-submit accepts it — the exact path that failed before the fix,
// when the printed --keys dropped the code key and no longer derived the
// printed session id.
func TestGateMissionCoBatchedPrintedSubmissionSubmits(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, root, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))

	var out, errOut bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", "code:src/auth.go,design:auth:src/auth.go", "--format", "json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var mission gateMission
	if err := json.Unmarshal(out.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	// Read the --keys value out of the printed command, exactly as the
	// coordinator would.
	prefix := "specflowctl gate-submit --run " + runID + " --session " + mission.Sessions[0].SessionID + " --keys "
	printedKeys := strings.TrimSuffix(strings.TrimPrefix(mission.Submission, prefix), " --report REPORT_PATH")
	if printedKeys != "code:src/auth.go,design:auth:src/auth.go" {
		t.Fatalf("printed --keys = %q, want the full assigned batch", printedKeys)
	}
	if err := grSubmitPrinted(t, root, runID, mission.Sessions[0].SessionID, printedKeys, grCoBatchedPairedReport()); err != nil {
		t.Fatalf("following the printed submission command failed: %v", err)
	}
}

// TestGateMissionReportLineFormsMatchValidator pins the prompt's "Required
// report lines and values" section to the line prefixes the validator parses:
// the logical check key is not itself a report line for checks, file, and
// cross sessions.
func TestGateMissionReportLineFormsMatchValidator(t *testing.T) {
	cases := []struct {
		name   string
		gate   string
		kind   string
		key    string
		line   string
		sample string
		token  string
	}{
		{"checks", gaterun.GateValidate, gaterun.SessionKindChecks, "1", "1. <check name>", "1. Structural integrity: PASS — checked", "PASS"},
		{"item", gaterun.GateVerify, gaterun.SessionKindItem, "auth.core", "auth.core", "auth.core: MISMATCH (acceptance) — src/auth.go:1", "MISMATCH"},
		{"file", gaterun.GateVerify, gaterun.SessionKindDesign, "src/auth.go", "conclusion", "conclusion: acceptable — ok", "acceptable"},
		{"cross", gaterun.GateVerify, gaterun.SessionKindCross, "cross", "Cross-check", "Cross-check: 5/5 PASS — ok", "PASS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := &gaterun.Run{Gate: tc.gate, TargetKind: gaterun.TargetKindUnit, TargetName: "auth", Target: "candidate", Mode: "full"}
			session := &gaterun.SessionSpec{Kind: tc.kind, SessionID: "p", CheckKeys: []string{tc.key}}
			contract := reportContractFor(run, session)
			if contract.Verdicts[0].Line != tc.line {
				t.Fatalf("verdict line = %q, want %q", contract.Verdicts[0].Line, tc.line)
			}
			prompt := renderMissionPrompt(t, run, session)
			if !strings.Contains(prompt, "- "+tc.line+": ") {
				t.Fatalf("prompt does not render the report line %q:\n%s", tc.line, prompt)
			}
			if tc.line != tc.key && strings.Contains(prompt, "- "+tc.key+": ") {
				t.Fatalf("prompt still renders the logical key %q as a report line:\n%s", tc.key, prompt)
			}
			token, _, _, err := extractVerdict(session, tc.key, tc.sample)
			if err != nil || token != tc.token {
				t.Fatalf("sample line %q does not parse as %s: token=%q err=%v", tc.sample, tc.token, token, err)
			}
		})
	}
}

func renderMissionPrompt(t *testing.T, run *gaterun.Run, session *gaterun.SessionSpec) string {
	t.Helper()
	mission := gateMission{
		SchemaVersion: 3, RunID: "20260101-000000-abc123", Gate: run.Gate,
		TargetKind: run.TargetKind, TargetName: run.TargetName, Target: run.Target, Mode: run.Mode,
		SpecSource: "docs/specs/units/candidate/unit_auth.md",
		Sessions: []missionSession{{
			SessionID:      session.SessionID,
			Kind:           session.Kind,
			Mission:        missionTextFor(session.Kind),
			CheckKeys:      append([]string{}, session.CheckKeys...),
			ProtocolRef:    "framework/unit_verify_checklist.md",
			ReportContract: reportContractFor(run, session),
		}},
		FailurePath: failureLineFor(run.Gate),
	}
	var buf bytes.Buffer
	writeGatePrompt(&buf, mission)
	return buf.String()
}

// TestFinalSynthesisMissionExcludesPublicObservations pins the consumer-scoped
// mission projection: per-file public observations are design input only, so
// the final synthesis carries the findings it must dispose — with their detail
// blocks — but not the observation transcript the findings already summarize.
func TestFinalSynthesisMissionExcludesPublicObservations(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	main := grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")

	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, root, id)

	codeKey := "code:src/auth.go"
	public := grDefaultQualityReport(run, *run.CoverageByKey(codeKey))
	public += "[P2] src/auth.go:1 — responsibility is unclear (actionable)\n  problem: responsibility is unclear\n  evidence: package auth\n  impact: maintenance is harder\n  fix: clarify the boundary\n"
	if err := sharedSubmit(t, root, id, codeKey, public); err != nil {
		t.Fatalf("public code session: %v", err)
	}

	// The design mission consumes the public record and must dispose it.
	var missionOut, missionErr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", id, "--keys", "design:auth:src/auth.go", "--format", "json"}, &missionOut, &missionErr); err != nil {
		t.Fatalf("design mission: %v (%s)", err, missionErr.String())
	}
	var designMission gateMission
	if err := json.Unmarshal(missionOut.Bytes(), &designMission); err != nil {
		t.Fatal(err)
	}
	design := designMission.Sessions[0]
	if len(design.Dependencies) != 1 || len(design.Dependencies[0].Observations) != 1 {
		t.Fatalf("design mission lost the public record it must dispose: %+v", design.Dependencies)
	}
	observation := design.Dependencies[0].Observations[0]
	designReport := grDefaultQualityReport(run, *run.CoverageByKey("design:auth:src/auth.go")) +
		"Observation disposition: " + observation.ID + " = retained — the file's responsibility belongs to the unit\n"
	if err := sharedSubmit(t, root, id, "design:auth:src/auth.go", designReport); err != nil {
		t.Fatalf("design session: %v", err)
	}
	designState, err := gaterun.LoadSessionState(root, run, "design:auth:src/auth.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(designState.Result.Findings) != 1 {
		t.Fatalf("retained observation did not become one unit finding: %+v", designState.Result.Findings)
	}
	finding := designState.Result.Findings[0]

	grSubmitOK(t, root, id, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))

	var crossOut, crossErr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", id, "--final", "--format", "json"}, &crossOut, &crossErr); err != nil {
		t.Fatalf("final mission: %v (%s)", err, crossErr.String())
	}
	var crossMission gateMission
	if err := json.Unmarshal(crossOut.Bytes(), &crossMission); err != nil {
		t.Fatal(err)
	}
	cross := crossMission.Sessions[0]
	var carried *gaterun.Finding
	for _, dependency := range cross.Dependencies {
		if len(dependency.Observations) != 0 {
			t.Fatalf("final synthesis received public observations it does not dispose: %+v", dependency.Observations)
		}
		for i := range dependency.Findings {
			if dependency.Findings[i].ID == finding.ID {
				carried = &dependency.Findings[i]
			}
		}
	}
	if carried == nil {
		t.Fatalf("final synthesis lost the finding it must dispose: %s", finding.ID)
	}
	if !strings.Contains(carried.Detail, "Public observation: "+observation.ID) {
		t.Fatalf("the finding no longer carries its observation evidence: %s", carried.Detail)
	}

	var promptOut, promptErr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", id, "--final", "--format", "prompt"}, &promptOut, &promptErr); err != nil {
		t.Fatalf("final prompt: %v (%s)", err, promptErr.String())
	}
	prompt := promptOut.String()
	if strings.Contains(prompt, "  Observation ") {
		t.Fatalf("final synthesis prompt still inlines public observations:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Dependency judgments (accepted results this session consumes):") || !strings.Contains(prompt, finding.ID) {
		t.Fatalf("final synthesis prompt lost its judgment records:\n%s", prompt)
	}

	grSubmitOK(t, root, id, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, root, id)
}

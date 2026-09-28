package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func TestGateMissionPlanStatusAndPrompt(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	var out, errOut bytes.Buffer
	if err := runGatePlan([]string{"--repo-root", root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--format", "json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var planned gateRunView
	if err := json.Unmarshal(out.Bytes(), &planned); err != nil {
		t.Fatal(err)
	}
	if planned.SchemaVersion != 1 || planned.NextAction != "execute" || len(planned.ReadyPacketIDs) != 4 || strings.Contains(strings.Join(planned.ReadyPacketIDs, ","), "cross") {
		t.Fatalf("wrong planned work: %+v", planned)
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
	if current.RunID != planned.RunID || len(current.ReadyPacketIDs) != 4 {
		t.Fatalf("wrong status: %+v", current)
	}
	if len(current.DeferredFindings) != 0 || !strings.Contains(status.String(), `"deferred_findings": []`) {
		t.Fatalf("status must show an empty deferred findings array: %s", status.String())
	}
	var jsonOut, prompt bytes.Buffer
	args := []string{"--repo-root", root, "--run", planned.RunID, "--packet", "structural"}
	if err := runGatePacket(append(append([]string{}, args...), "--format", "json"), &jsonOut, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := runGatePacket(append(append([]string{}, args...), "--format", "prompt"), &prompt, &errOut); err != nil {
		t.Fatal(err)
	}
	var mission gateMission
	if err := json.Unmarshal(jsonOut.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	if len(mission.Packets) != 1 || mission.Packets[0].ProtocolRef != "framework/unit_validate_checklist.md" || mission.Packets[0].PacketID != "structural" {
		t.Fatalf("wrong mission: %+v", mission)
	}
	if mission.Packets[0].LastRejection != "" || strings.Contains(jsonOut.String(), "last_rejection") || strings.Contains(prompt.String(), "Previous submission was rejected:") {
		t.Fatalf("first-attempt mission carries retry feedback: %s", prompt.String())
	}
	for _, want := range []string{mission.RunID, mission.Packets[0].ProtocolRef, "Checks: 1, 3, 6", "Dependency scope:", "gate-submit"} {
		if !strings.Contains(prompt.String(), want) {
			t.Fatalf("prompt lacks %q:\n%s", want, prompt.String())
		}
	}
	main := "docs/specs/units/candidate/unit_auth.md"
	report := mission.Packets[0].ReportContract.Template
	for _, pair := range [][2]string{
		{"1. <check name>: <PASS|WARNING|FAIL> — <reason>", "1. Structural integrity: PASS — checked"},
		{"3. <check name>: <PASS|WARNING|FAIL> — <reason>", "3. Scope integrity: PASS — checked"},
		{"6. <check name>: <PASS|WARNING|FAIL> — <reason>", "6. Affects-source validity: PASS — checked"},
	} {
		report = strings.Replace(report, pair[0], pair[1], 1)
	}
	report = strings.Replace(report, "[P1] <location> — <finding> (actionable|needs_decision)\n  problem: <problem>\n  evidence: <evidence>\n  impact: <impact>\n  fix: <repair>\n", "", 1)
	report = strings.Replace(report, "  <check_key>: <read_ref>: <section|range|acceptance_item:id|acceptance_items|all>", "  check-1: "+main+": frontmatter\n  check-3: "+main+": Description\n  check-6: "+main+": Description", 1)
	if strings.Contains(report, "<") {
		t.Fatalf("unfilled report template:\n%s", report)
	}
	invalid := strings.Replace(report, "3. Scope integrity: PASS — checked\n", "", 1)
	if _, err := grSubmit(t, root, planned.RunID, "structural", invalid); err == nil || !strings.Contains(err.Error(), "check \"3\" has no verdict") {
		t.Fatalf("missing template field was not rejected: %v", err)
	}
	retry := missionJSON(t, root, planned.RunID, "structural")
	if !strings.Contains(retry.Packets[0].LastRejection, `check "3" has no verdict`) {
		t.Fatalf("retry mission lost rejection reason: %+v", retry.Packets[0])
	}
	prompt.Reset()
	if err := runGatePacket(append(append([]string{}, args...), "--format", "prompt"), &prompt, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.String(), "Previous submission was rejected: "+retry.Packets[0].LastRejection) {
		t.Fatalf("retry prompt lost rejection reason: %s", prompt.String())
	}
	invalidAgain := strings.Replace(report, "1. Structural integrity: PASS — checked\n", "", 1)
	if _, err := grSubmit(t, root, planned.RunID, "structural", invalidAgain); err == nil || !strings.Contains(err.Error(), "check \"1\" has no verdict") {
		t.Fatalf("second invalid report was not rejected: %v", err)
	}
	retry = missionJSON(t, root, planned.RunID, "structural")
	if !strings.Contains(retry.Packets[0].LastRejection, `check "1" has no verdict`) || strings.Contains(retry.Packets[0].LastRejection, `check "3" has no verdict`) {
		t.Fatalf("retry mission did not use latest rejection: %+v", retry.Packets[0])
	}
	prompt.Reset()
	if err := runGatePacket(append(append([]string{}, args...), "--format", "prompt"), &prompt, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.String(), "Previous submission was rejected: "+retry.Packets[0].LastRejection) || strings.Contains(prompt.String(), `check "3" has no verdict`) {
		t.Fatalf("retry prompt did not use latest rejection: %s", prompt.String())
	}
	status.Reset()
	if err := runGateStatus([]string{"--repo-root", root, "--run", planned.RunID, "--format", "json"}, &status, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(status.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(current.ReadyPacketIDs, ","), "structural") {
		t.Fatalf("rejected packet is not ready to retry: %+v", current)
	}
	grSubmitOK(t, root, planned.RunID, "structural", report)
	status.Reset()
	if err := runGateStatus([]string{"--repo-root", root, "--run", planned.RunID, "--format", "json"}, &status, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(status.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(current.ReadyPacketIDs, ","), "structural") {
		t.Fatalf("accepted packet still ready: %+v", current)
	}
	if err := runGatePacket(args, &prompt, &errOut); err == nil || !strings.Contains(err.Error(), "only pending or rejected") {
		t.Fatalf("accepted packet received another mission: %v", err)
	}
}

func TestGateMissionUsesInstalledFrameworkPathAndRejectsBlockedPacket(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteFile(t, root, "specflow/tooling/manifest.tsv", "fixture\n")
	grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	var out, errOut bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", root, "--run", runID, "--packet", "cross", "--format", "json"}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("blocked cross mission: %v", err)
	}
	if err := runGatePacket([]string{"--repo-root", root, "--run", runID, "--packet", "structural", "--format", "json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var mission gateMission
	if err := json.Unmarshal(out.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	if mission.Packets[0].ProtocolRef != "specflow/framework/unit_validate_checklist.md" {
		t.Fatalf("wrong deployed path: %s", mission.Packets[0].ProtocolRef)
	}
}

func missionJSON(t *testing.T, root, runID, packetID string) gateMission {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", root, "--run", runID, "--packet", packetID, "--format", "json"}, &out, &errOut); err != nil {
		t.Fatalf("mission %s: %v (%s)", packetID, err, errOut.String())
	}
	var mission gateMission
	if err := json.Unmarshal(out.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	return mission
}

func TestGateMissionsCoverRuleVerifyAnalysisReviewAndCross(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteFile(t, root, "docs/specs/rules/candidate/b_rule_http.md", "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")
	ruleRun := grPlan(t, root, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	rule := missionJSON(t, root, ruleRun, "checks")
	if rule.Packets[0].ProtocolRef != "framework/rule_validate_checklist.md" || len(rule.Packets[0].ReportContract.Verdicts) != 8 {
		t.Fatalf("wrong rule mission: %+v", rule.Packets[0])
	}

	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	verifyRun := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	detect := missionJSON(t, root, verifyRun, "detect:auth.core")
	if detect.Packets[0].ProtocolScope != "Steps 1-6 for acceptance item auth.core" || !strings.Contains(detect.Packets[0].ReportContract.Template, "Part B:") {
		t.Fatalf("wrong detection mission: %+v", detect.Packets[0])
	}
	grSubmitOK(t, root, verifyRun, "detect:auth.core", grVerifyItemBody("auth.core", "MISMATCH (acceptance)", "broken at src/auth.go:1")+"auth.core: "+main+": acceptance_item:auth.core\nauth.core: src/auth.go: all\n")
	analysis := missionJSON(t, root, verifyRun, "analysis:auth.core")
	if analysis.Packets[0].ProtocolScope != "Step 7 for acceptance item auth.core" || len(analysis.Packets[0].Dependencies) != 1 || !strings.Contains(analysis.Packets[0].Dependencies[0].Report, "MISMATCH (acceptance)") {
		t.Fatalf("wrong analysis mission: %+v", analysis.Packets[0])
	}

	reviewRun := grPlan(t, root, "--gate", "review", "--unit", "auth", "--target", "candidate")
	file := missionJSON(t, root, reviewRun, "src/auth.go")
	if file.Packets[0].ProtocolRef != "framework/spec_review_checklist.md" || !strings.Contains(file.Packets[0].ReportContract.Template, "module_boundaries:") {
		t.Fatalf("wrong review mission: %+v", file.Packets[0])
	}
	grSubmitOK(t, root, reviewRun, "src/auth.go", grReviewReport("src/auth.go", main))
	cross := missionJSON(t, root, reviewRun, "cross")
	if len(cross.Packets[0].Dependencies) != 1 || cross.Packets[0].Dependencies[0].Digest == "" || len(cross.Packets[0].AdditionalRefs) != 2 || !strings.Contains(cross.Packets[0].ReportContract.Template, "Effective status:") {
		t.Fatalf("wrong cross mission: %+v", cross.Packets[0])
	}
}

// TestGateMissionReportLineFormsMatchValidator pins the prompt's "Required
// report lines and values" section to the line prefixes the validator parses:
// the logical check key is not itself a report line for checks, file, and
// cross packets.
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
		{"checks", gaterun.GateValidate, gaterun.PacketKindChecks, "1", "1. <check name>", "1. Structural integrity: PASS — checked", "PASS"},
		{"item", gaterun.GateVerify, gaterun.PacketKindItem, "auth.core", "auth.core", "auth.core: MISMATCH (acceptance) — src/auth.go:1", "MISMATCH"},
		{"file", gaterun.GateReview, gaterun.PacketKindFile, "src/auth.go", "conclusion", "conclusion: acceptable — ok", "acceptable"},
		{"cross", gaterun.GateVerify, gaterun.PacketKindCross, "cross", "Cross-check", "Cross-check: 5/5 PASS — ok", "PASS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := &gaterun.Run{Gate: tc.gate, TargetKind: gaterun.TargetKindUnit, TargetName: "auth", Target: "candidate", Mode: "full"}
			packet := &gaterun.PacketSpec{Kind: tc.kind, PacketID: "p", CheckKeys: []string{tc.key}}
			contract := reportContractFor(run, packet)
			if contract.Verdicts[0].Line != tc.line {
				t.Fatalf("verdict line = %q, want %q", contract.Verdicts[0].Line, tc.line)
			}
			prompt := renderMissionPrompt(t, run, packet)
			if !strings.Contains(prompt, "- "+tc.line+": ") {
				t.Fatalf("prompt does not render the report line %q:\n%s", tc.line, prompt)
			}
			if tc.line != tc.key && strings.Contains(prompt, "- "+tc.key+": ") {
				t.Fatalf("prompt still renders the logical key %q as a report line:\n%s", tc.key, prompt)
			}
			token, _, _, err := extractVerdict(packet, tc.key, tc.sample)
			if err != nil || token != tc.token {
				t.Fatalf("sample line %q does not parse as %s: token=%q err=%v", tc.sample, tc.token, token, err)
			}
		})
	}
}

func renderMissionPrompt(t *testing.T, run *gaterun.Run, packet *gaterun.PacketSpec) string {
	t.Helper()
	mission := gateMission{
		SchemaVersion: 1, RunID: "20260101-000000-abc123", Gate: run.Gate,
		TargetKind: run.TargetKind, TargetName: run.TargetName, Target: run.Target, Mode: run.Mode,
		SpecSource: "docs/specs/units/candidate/unit_auth.md",
		Packets: []missionPacket{{
			PacketID:       packet.PacketID,
			Kind:           packet.Kind,
			Mission:        missionTextFor(packet.Kind),
			CheckKeys:      append([]string{}, packet.CheckKeys...),
			ProtocolRef:    "framework/unit_verify_checklist.md",
			ReportContract: reportContractFor(run, packet),
		}},
		FailurePath: failureLineFor(run.Gate),
	}
	var buf bytes.Buffer
	writeGatePrompt(&buf, mission)
	return buf.String()
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func grReadyVerifyCross(t *testing.T) (string, string, string) {
	t.Helper()
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, root, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	return root, runID, main
}

func grVerifyCrossItems(runID string, passed int, withFinding bool) string {
	var b strings.Builder
	for i, item := range verifyCrossItems {
		verdict := "FAIL"
		if i < passed {
			verdict = "PASS"
		}
		fmt.Fprintf(&b, "Cross item: %s = %s — checked\n", item, verdict)
		if verdict == "FAIL" && withFinding {
			fmt.Fprintf(&b, "Cross item finding: %s = %s/cross/F1\n", item, runID)
		}
	}
	return b.String()
}

func grVerifyCrossReport(runID, main string, passed int, severity, verdict string) string {
	var b strings.Builder
	b.WriteString(grVerifyCrossItems(runID, passed, severity != ""))
	fmt.Fprintf(&b, "Cross-check: %d/5 %s — synthesis complete\n", passed, verdict)
	if severity != "" {
		fmt.Fprintf(&b, "[%s] cross — combined claim needs repair (actionable)\n", severity)
		fmt.Fprintf(&b, "Finding affects: %s/cross/F1 = auth.core\n", runID)
	}
	fmt.Fprintf(&b, "cross: %s: Description\n", main)
	if severity != "" {
		fmt.Fprintf(&b, "Severity confirmation: %s/cross/F1 = confirmed %s — evidence: %s; reason: the combined impact is local\n", runID, severity, main)
		b.WriteString("Effective status: auth.core = fail\n")
	} else {
		b.WriteString("Effective status: auth.core = pass\n")
	}
	fmt.Fprintf(&b, "Effective status: cross = %s\n", strings.ToLower(verdict))
	return b.String()
}

func TestVerifyCrossContractRejectsIncompleteOrContradictoryReports(t *testing.T) {
	first := "Cross item: contract_consistency = PASS — checked\n"
	cases := []struct {
		name   string
		change func(string) string
		want   string
	}{
		{"missing item", func(s string) string { return strings.Replace(s, first, "", 1) }, "missing Cross item"},
		{"duplicate item", func(s string) string { return first + s }, "duplicate Cross item"},
		{"unknown item", func(s string) string { return strings.Replace(s, "contract_consistency", "unknown_check", 1) }, "unknown Cross item"},
		{"empty reason", func(s string) string {
			return strings.Replace(s, first, "Cross item: contract_consistency = PASS — \n", 1)
		}, "malformed Cross item"},
		{"invalid verdict", func(s string) string {
			return strings.Replace(s, first, "Cross item: contract_consistency = WARNING — checked\n", 1)
		}, "malformed Cross item"},
		{"wrong passed count", func(s string) string { return strings.Replace(s, "Cross-check: 5/5", "Cross-check: 4/5", 1) }, "contradicts Cross item results"},
		{"overflowed passed count", func(s string) string {
			return strings.Replace(s, "Cross-check: 5/5", "Cross-check: 999999999999999999999999999999/5", 1)
		}, "invalid Cross-check passed count"},
		{"wrong total count", func(s string) string { return strings.Replace(s, "Cross-check: 5/5", "Cross-check: 5/3", 1) }, "contradicts Cross item results"},
		{"missing count", func(s string) string { return strings.Replace(s, "Cross-check: 5/5", "Cross-check:", 1) }, "requires exactly one"},
		{"duplicate summary", func(s string) string { return "Cross-check: 5/5 PASS — duplicate\n" + s }, "verdict lines"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, runID, main := grReadyVerifyCross(t)
			report := tc.change(grVerifyCrossReport(runID, main, 5, "", "PASS"))
			if _, err := grSubmitRaw(t, root, runID, "cross", report); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestVerifyCrossContractKeepsNonblockingFourOfFivePass(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	mission := missionJSON(t, root, runID, "cross")
	for _, item := range verifyCrossItems {
		if !strings.Contains(mission.Packets[0].ReportContract.Template, "Cross item: "+item) {
			t.Fatalf("mission omitted %s", item)
		}
	}
	if !strings.Contains(mission.Packets[0].ReportContract.Template, "Cross item finding: <failed_item_key> = <new_cross_finding_id>") {
		t.Fatalf("mission omitted failed-item finding links: %s", mission.Packets[0].ReportContract.Template)
	}
	if _, err := grSubmitRaw(t, root, runID, "cross", grVerifyCrossReport(runID, main, 4, "P2", "PASS")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "p2_count: 1") {
		t.Fatalf("nonblocking cross result was lost:\n%s", cache)
	}
}

func TestVerifyCrossPassRejectsBlockingNewFinding(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	if _, err := grSubmitRaw(t, root, runID, "cross", grVerifyCrossReport(runID, main, 4, "P1", "PASS")); err == nil || !strings.Contains(err.Error(), "Cross-check PASS contradicts") {
		t.Fatalf("expected blocking cross finding to contradict PASS, got %v", err)
	}
}

func TestVerifyCrossItemFindingLinks(t *testing.T) {
	cases := []struct {
		name   string
		change func(string, string) string
		want   string
	}{
		{"missing link", func(report, runID string) string {
			return strings.Replace(report, "Cross item finding: cross_reference_integrity = "+runID+"/cross/F1\n", "", 1)
		}, "has no Cross item finding"},
		{"duplicate link", func(report, runID string) string {
			link := "Cross item finding: cross_reference_integrity = " + runID + "/cross/F1\n"
			return strings.Replace(report, link, link+link, 1)
		}, "duplicate Cross item finding"},
		{"unknown item", func(report, runID string) string {
			return strings.Replace(report, "Cross item finding: cross_reference_integrity", "Cross item finding: unknown_check", 1)
		}, "unknown Cross item finding key"},
		{"unknown finding", func(report, runID string) string {
			return strings.Replace(report, runID+"/cross/F1", runID+"/cross/F9", 1)
		}, "not a finding created"},
		{"passing item link", func(report, runID string) string {
			return "Cross item finding: contract_consistency = " + runID + "/cross/F1\n" + report
		}, "must not declare a Cross item finding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, runID, main := grReadyVerifyCross(t)
			report := tc.change(grVerifyCrossReport(runID, main, 4, "P2", "PASS"), runID)
			if _, err := grSubmitRaw(t, root, runID, "cross", report); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestVerifyCrossOneFindingSupportsTwoFailedItems(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	report := grVerifyCrossReport(runID, main, 3, "P3", "PASS")
	if _, err := grSubmitRaw(t, root, runID, "cross", report); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "p3_count: 1") || !strings.Contains(cache, "Cross item finding: error_code_conflict = "+runID+"/cross/F1") || !strings.Contains(cache, "Cross item finding: cross_reference_integrity = "+runID+"/cross/F1") {
		t.Fatalf("shared nonblocking cross finding was lost:\n%s", cache)
	}
}

func TestVerifyCrossBlockingFindingFailsFinalGate(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	if _, err := grSubmitRaw(t, root, runID, "cross", grVerifyCrossReport(runID, main, 4, "P1", "FAIL")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "blocking: true") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("blocking cross finding did not fail the gate:\n%s", cache)
	}
}

func TestCrossItemFindingLinkMustBeRetained(t *testing.T) {
	id := "run/cross/F1"
	parsed := &parsedReport{Findings: []gaterun.Finding{{ID: id, Severity: "P1"}}, CrossItemFindings: map[string]string{"contract_consistency": id}}
	if err := validateCrossItemFindingLinks(gaterun.GateVerify, parsed, nil); err == nil || !strings.Contains(err.Error(), "not retained") {
		t.Fatalf("non-retained cross finding was accepted: %v", err)
	}
	parsed.Findings[0].Severity = "P2"
	if err := validateCrossItemFindingLinks(gaterun.GateValidate, parsed, parsed.Findings); err == nil || !strings.Contains(err.Error(), "must be P0 or P1") {
		t.Fatalf("validate accepted a P2 cross finding: %v", err)
	}
}

func TestValidateCrossFailedItemHasBlockingFinding(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	main := grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	for _, packet := range []struct {
		id     string
		checks []string
	}{
		{"structural", []string{"1", "3", "6"}},
		{"design", []string{"2", "4"}},
		{"acceptance", []string{"5"}},
		{"dependencies", []string{"7", "8", "9"}},
	} {
		scopes := make(map[string][]string)
		for _, check := range packet.checks {
			scopes[check] = []string{main + ": Description"}
		}
		grSubmitOK(t, root, runID, packet.id, grValidateReport(packet.checks, scopes))
	}
	id := runID + "/cross/F1"
	report := "Cross item: design_constraints = FAIL — combined design violates a constraint\n" +
		"Cross item finding: design_constraints = " + id + "\n" +
		"Cross item: coverage_scope = PASS — checked\n" +
		"Cross item: cross_unit_cohesion = PASS — checked\n" +
		"Cross-check: 2/3 FAIL — blocking cross finding\n" +
		"[P1] cross — combined design violates a constraint\n" +
		"Finding affects: " + id + " = 2\n" +
		"cross: " + main + ": Description\n" +
		"Severity confirmation: " + id + " = confirmed P1 — evidence: " + main + "; reason: dependent design is affected\n"
	for _, check := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		status := "pass"
		if check == "2" {
			status = "fail"
		}
		report += "Effective status: " + check + " = " + status + "\n"
	}
	report += "Effective status: cross = fail\n"
	if _, err := grSubmitRaw(t, root, runID, "cross", report); err != nil {
		t.Fatal(err)
	}
	if out := grFinalizeOK(t, root, runID); !strings.Contains(out, "Full-run validation failed") {
		t.Fatalf("blocking validate cross finding did not fail the gate: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "docs/specs/meta/validation/unit/auth/validate_result.md")); !os.IsNotExist(err) {
		t.Fatalf("failed candidate validate unexpectedly published a cache: %v", err)
	}
}

func TestValidateCrossContractUsesThreeFixedItems(t *testing.T) {
	run := &gaterun.Run{Gate: gaterun.GateValidate}
	packet := &gaterun.PacketSpec{Kind: gaterun.PacketKindCross, CheckKeys: []string{gaterun.CrossKey}}
	contract := reportContractFor(run, packet)
	for _, item := range validateCrossItems {
		if !strings.Contains(contract.Template, "Cross item: "+item) {
			t.Fatalf("validate template omitted %s", item)
		}
	}
	if !strings.Contains(contract.Template, "Cross-check: <passed>/3") {
		t.Fatalf("validate summary has wrong denominator: %s", contract.Template)
	}
	var report strings.Builder
	for _, item := range validateCrossItems {
		fmt.Fprintf(&report, "Cross item: %s = PASS — checked\n", item)
	}
	report.WriteString("Cross-check: 3/3 PASS — consistent\n")
	if err := validateCrossItemBody(gaterun.GateValidate, report.String(), &parsedReport{}); err != nil {
		t.Fatalf("complete validate cross report rejected: %v", err)
	}
	missing := strings.Replace(report.String(), "Cross item: design_constraints = PASS — checked\n", "", 1)
	if err := validateCrossItemBody(gaterun.GateValidate, missing, &parsedReport{}); err == nil || !strings.Contains(err.Error(), "missing Cross item") {
		t.Fatalf("incomplete validate cross report accepted: %v", err)
	}
}

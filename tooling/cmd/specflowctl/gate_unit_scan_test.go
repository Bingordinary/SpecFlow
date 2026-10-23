package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// TestUnitScanCoverageAndMission locks the unit-level scan's split from the item
// sessions: the coverage set carries one alignment key for the whole unit, the
// unit mission runs Steps 1 and 5 only, and the item missions run Steps 2, 3, 4,
// 6 and 7 only.
func TestUnitScanCoverageAndMission(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, root, runID)

	ck := run.CoverageByKey("unit:auth")
	if ck == nil || ck.Kind != gaterun.SessionKindUnit || ck.Lens != gaterun.LensAlignment {
		t.Fatalf("verify coverage must carry the unit-level alignment key, got %+v", run.Coverage)
	}

	unit := missionJSON(t, root, runID, "unit:auth")
	if unit.Sessions[0].ProtocolScope != "Target-layer applicability and Steps 1 and 5 over the whole unit" {
		t.Fatalf("unit mission protocol scope = %q", unit.Sessions[0].ProtocolScope)
	}
	template := unit.Sessions[0].ReportContract.Template
	for _, class := range unitScanClasses {
		if !strings.Contains(template, "declarations."+class+": checked=<n> — <result>") {
			t.Fatalf("unit mission template lacks the %s scan line:\n%s", class, template)
		}
	}
	if !strings.Contains(template, "surplus: found=<n> — <result>") {
		t.Fatalf("unit mission template lacks the surplus line:\n%s", template)
	}
	if !strings.Contains(template, "non_runnable: count=<n> — <result>") {
		t.Fatalf("unit mission template lacks the non-runnable line:\n%s", template)
	}
	if !strings.Contains(template, "MISMATCH (structural|surplus)") {
		t.Fatalf("unit mission template must restrict the mismatch types:\n%s", template)
	}

	item := missionJSON(t, root, runID, "auth.core")
	if item.Sessions[0].ProtocolScope != "Target-layer applicability and Steps 2, 3, 4, 6, 7 for acceptance item(s) item:auth:auth.core" {
		t.Fatalf("item mission protocol scope = %q", item.Sessions[0].ProtocolScope)
	}
	if strings.Contains(strings.Join(item.Sessions[0].Context, " "), "Steps 1 (structural alignment) and 5") == false {
		t.Fatalf("item mission must state that Steps 1 and 5 belong to the unit scan: %v", item.Sessions[0].Context)
	}
}

// TestUnitScanReportRejectsSkippedClassAndForeignMismatchType pins the anti-skip
// contract: every declaration class must be reported (a skipped class is a
// rejected report, not a silent gap), and a scope mismatch stays with the item
// sessions.
func TestUnitScanReportRejectsSkippedClassAndForeignMismatchType(t *testing.T) {
	root, main, _ := sharedFixture(t)
	grEnableMissionLayout(t, root)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")

	missing := strings.Replace(grVerifyUnitScanReport("unit:auth"), "declarations.error_types: checked=0 — fixture scan\n", "", 1)
	if _, err := grSubmit(t, root, id, "unit:auth", missing); err == nil || !strings.Contains(err.Error(), "declarations.error_types") {
		t.Fatalf("a report that skips a declaration class must be rejected, got %v", err)
	}

	noRunnable := strings.Replace(grVerifyUnitScanReport("unit:auth"), "non_runnable: count=0 — fixture scan\n", "", 1)
	if _, err := grSubmit(t, root, id, "unit:auth", noRunnable); err == nil || !strings.Contains(err.Error(), "non_runnable") {
		t.Fatalf("a report that omits the Step 5 Part 2 line must be rejected, got %v", err)
	}

	scope := grVerifyUnitScanReport("unit:auth")
	scope = strings.Replace(scope, "unit:auth: ALIGNED — fixture scan", "unit:auth: MISMATCH (scope) — declared surface points elsewhere", 1)
	scope += grVerifyAnalysisFields("unit:auth")
	if _, err := grSubmit(t, root, id, "unit:auth", scope); err == nil || !strings.Contains(err.Error(), "structural | surplus") {
		t.Fatalf("the unit scan must reject a scope mismatch type, got %v", err)
	}

	grSubmitOK(t, root, id, "unit:auth", grVerifyUnitScanReport("unit:auth"))
	key := "item:auth:auth.core"
	sharedSubmit(t, root, id, key, grVerifyItemBody(key, "ALIGNED", "contracts.js:1")+key+": "+main+": acceptance_item:auth.core\n")
	sharedFinish(t, root, id)

	state, ok := grReadJudgmentBaseline(t, root, "unit", "auth", "verify").Records["unit:auth"]
	if !ok {
		t.Fatalf("the finalized unit scan must publish its judgment")
	}
	record, err := judgments.Load(root, state.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != judgments.Unit {
		t.Fatalf("unit scan judgment kind = %q, want %q", record.Kind, judgments.Unit)
	}
	// The unit scan pins the whole unit: its main spec is an owned dependency.
	found := false
	for _, dep := range record.Dependencies {
		if dep.Own && strings.HasSuffix(dep.Path, ".md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the unit scan judgment must pin the unit's own spec: %+v", record.Dependencies)
	}
}

// unitScanMismatchReport renders a unit scan report that retains two independent
// mismatches, each as its own `Problem:` finding block. The blocks are the
// contiguous indented lines directly under the verdict line (the unindented
// declaration lines come after, as the parser reads the finding block from the
// verdict line's continuation).
func unitScanMismatchReport() string {
	var b strings.Builder
	b.WriteString("unit:auth: MISMATCH (structural) — two retained mismatches\n")
	b.WriteString(grVerifyAnalysisFields("unit:auth", "P1"))
	b.WriteString(grVerifyAnalysisFields("unit:auth", "P2"))
	for _, class := range unitScanClasses {
		fmt.Fprintf(&b, "declarations.%s: checked=1 — fixture scan\n", class)
	}
	b.WriteString("surplus: found=1 — one surplus construct\n")
	b.WriteString("non_runnable: count=0 — none\n")
	return b.String()
}

// TestUnitScanRetainsMultipleFindings locks the anti-loss contract: one MISMATCH
// verdict carries one finding block per retained mismatch, so a scan that finds
// several Step 1 or Step 5 mismatches does not have to drop or merge them.
func TestUnitScanRetainsMultipleFindings(t *testing.T) {
	root, _, _ := sharedFixture(t)
	grEnableMissionLayout(t, root)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")

	grSubmitOK(t, root, id, "unit:auth", unitScanMismatchReport())

	run := mustLoadRun(t, root, id)
	states, err := gaterun.LoadSessionStates(root, run)
	if err != nil {
		t.Fatal(err)
	}
	var result *gaterun.SessionResult
	for i := range states {
		for _, key := range states[i].Keys {
			if key == "unit:auth" {
				result = states[i].Result
			}
		}
	}
	if result == nil {
		t.Fatal("the unit scan session must be accepted")
	}
	if len(result.Findings) != 2 {
		t.Fatalf("a two-mismatch unit scan must retain two findings, got %d: %+v", len(result.Findings), result.Findings)
	}
	if !strings.HasSuffix(result.Findings[0].ID, "/F1") || !strings.HasSuffix(result.Findings[1].ID, "/F2") {
		t.Fatalf("findings must be numbered in report order, got %q and %q", result.Findings[0].ID, result.Findings[1].ID)
	}
	if result.Findings[0].Severity != "P1" || result.Findings[1].Severity != "P2" {
		t.Fatalf("each finding must keep its own severity, got %q and %q", result.Findings[0].Severity, result.Findings[1].Severity)
	}
}

// TestUnitScanMismatchRequiresAFindingBlock pins the failure path: a MISMATCH
// verdict with no `Problem:` block is rejected instead of being accepted with
// no finding.
func TestUnitScanMismatchRequiresAFindingBlock(t *testing.T) {
	root, _, _ := sharedFixture(t)
	grEnableMissionLayout(t, root)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")

	report := "unit:auth: MISMATCH (structural) — no finding block\n"
	for _, class := range unitScanClasses {
		report += fmt.Sprintf("declarations.%s: checked=1 — fixture scan\n", class)
	}
	report += "surplus: found=0 — none\n"
	report += "non_runnable: count=0 — none\n"
	if _, err := grSubmit(t, root, id, "unit:auth", report); err == nil || !strings.Contains(err.Error(), "at least one finding block") {
		t.Fatalf("a MISMATCH without a finding block must be rejected, got %v", err)
	}
}

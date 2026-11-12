package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// coveredKeys lists the coverage keys an accepted session (or a plan-time
// reused public record) already covers.
func coveredKeys(t *testing.T, root string, run *gaterun.Run) map[string]bool {
	t.Helper()
	states, err := gaterun.LoadSessionStates(root, run)
	if err != nil {
		t.Fatal(err)
	}
	covered, _, err := gaterun.CoverageProgress(run, states)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for key := range covered {
		out[key] = true
	}
	return out
}

// TestUnitScanRowScopedDeltaCompletes pins the row-level scan recovery end to
// end: a one-file code edit re-runs only that file's surplus row and the
// declaration rows that cited the file; the other rows carry, the scoped
// session judges only the dead rows, and the rewritten record merges both
// sides (framework/validation_cache.md §Unit-scan row bookkeeping).
func TestUnitScanRowScopedDeltaCompletes(t *testing.T) {
	root, main := deltaBaseline(t, "    affects:\n      files:\n        - helper.js\n", map[string]string{"helper.js": "export const helper = true;\n"})

	// One-line code edit: the surplus row for contracts.js and the
	// declaration row that cited it die; nothing else.
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token, edited: true}; }\n")
	delta := deltaPlan(t, root)
	if len(delta.ScanRows) == 0 {
		t.Fatalf("expected a row-scoped scan re-run, got a full one (coverage %v)", coverageKeysOf(delta))
	}
	wantRows := []string{"decl:promise:auth.core", "surplus:contracts.js"}
	got := append([]string(nil), delta.ScanRows...)
	if strings.Join(got, ",") != strings.Join(wantRows, ",") {
		t.Fatalf("scan rows to re-run = %v, want %v", got, wantRows)
	}
	// The scan session's mission carries the row assignment.
	spec, err := gaterun.BuildSessionSpec(root, delta, []string{"unit:auth"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(spec.ScanRows, ",") != strings.Join(wantRows, ",") {
		t.Fatalf("mission scan rows = %v, want %v", spec.ScanRows, wantRows)
	}
	if !strings.Contains(strings.Join(spec.Context, " "), "row-scoped scan re-run") {
		t.Fatalf("mission does not state the row scope: %v", spec.Context)
	}
	joinedContext := strings.Join(spec.Context, "\n")
	if strings.Contains(joinedContext, "Report one `declarations.<class>:` line") {
		t.Fatalf("a row-scoped mission must not instruct the full report's aggregate lines: %v", spec.Context)
	}
	if !strings.Contains(joinedContext, "Assigned rows: "+strings.Join(wantRows, ", ")) {
		t.Fatalf("a row-scoped mission must list its assigned rows: %v", spec.Context)
	}

	// Submit the scoped scan report: only the assigned rows.
	report := "row decl:promise:auth.core: match — files: contracts.js\nrow surplus:contracts.js: clean — no undeclared behavior\n"
	if err := sharedSubmit(t, root, delta.RunID, "unit:auth", report); err != nil {
		t.Fatalf("scoped scan submit: %v", err)
	}
	// The remaining delta sessions: item, code, design carry? The item and
	// design both pinned the file, so they re-run too; complete them.
	if err := sharedSubmit(t, root, delta.RunID, "item:auth:auth.core", grVerifyItemReport("item:auth:auth.core", main, "contracts.js")); err != nil {
		t.Fatal(err)
	}
	if err := sharedSubmit(t, root, delta.RunID, "code:contracts.js", grDefaultQualityReport(delta, *delta.CoverageByKey("code:contracts.js"))); err != nil {
		t.Fatal(err)
	}
	design := "File: design:auth:contracts.js\nconclusion: acceptable\nspec_requirements: token behavior checked — auth contract uses this shape\nSpec basis: Narrative\nSpec basis: Promises\ngate_findings: none\nDependency scope:\ndesign:auth:contracts.js: " + main + ": Description\ndesign:auth:contracts.js: contracts.js: all\n"
	if err := sharedSubmit(t, root, delta.RunID, "design:auth:contracts.js", design); err != nil {
		t.Fatal(err)
	}
	if err := sharedSubmit(t, root, delta.RunID, "architecture:auth", grDefaultQualityReport(delta, *delta.CoverageByKey("architecture:auth"))); err != nil {
		t.Fatal(err)
	}
	cross := "Cross-check: 1/1 PASS — consistent\ncross: " + main + ": Description\n"
	for _, ck := range delta.Coverage {
		cross += "Effective status: " + ck.Key + " = pass\n"
	}
	cross += "Effective status: cross = pass\n"
	grSubmitOK(t, root, delta.RunID, "cross", cross)
	grFinalizeOK(t, root, delta.RunID)

	// The rewritten record keeps every catalog row: the carried ones keep
	// their verdicts and pins, the re-run ones are fresh.
	baseline := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	binding, ok := baseline.Records["unit:auth"]
	if !ok {
		t.Fatalf("delta finalize did not publish a unit-scan record")
	}
	record, err := judgments.Load(root, binding.Reference)
	if err != nil {
		t.Fatal(err)
	}
	verdicts := map[string]string{}
	for _, row := range record.Rows {
		verdicts[row.ID] = row.Verdict
	}
	if len(verdicts) != 4 {
		t.Fatalf("merged record rows = %v, want the full catalog (4 rows)", verdicts)
	}
	coverageOK := false
	for _, row := range record.Rows {
		if row.Kind == judgments.RowKindCoverage {
			coverageOK = row.Verdict == judgments.RowOK
		}
	}
	if !coverageOK {
		t.Fatalf("the merged record lost its Step 5 Part 2 coverage row: %v", verdicts)
	}
	if verdicts["decl:promise:auth.core"] != judgments.RowMatch || verdicts["surplus:contracts.js"] != judgments.RowClean || verdicts["surplus:helper.js"] != judgments.RowClean {
		t.Fatalf("merged row verdicts = %v", verdicts)
	}
	if record.Verdict != "ALIGNED" {
		t.Fatalf("aggregate verdict = %q, want ALIGNED", record.Verdict)
	}
	if baseline.LogicalStatus["unit:auth"] != "pass" {
		t.Fatalf("the scoped run must record the unit-scan logical status, got %v", baseline.LogicalStatus)
	}
	var result gaterun.SessionResult
	if err := json.Unmarshal(record.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.EffectiveStatus["unit:auth"] != "pass" {
		t.Fatalf("the scoped record must carry the recomposed effective status, got %v", result.EffectiveStatus)
	}
	// The gate is fresh again.
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("delta finalize must leave the gate fresh: %+v %v", check, err)
	}
	_ = os.Remove // keep os imported for future edits
}

// TestUnitScanRowScopedDeltaSkipsRemovedCatalogRows pins the removed-row half
// of the delta scope: a baseline row whose declaration left the unit spec (and
// whose file was deleted) is neither re-runnable nor carriable — it must not
// enter the re-run assignment, because the scoped report contract demands
// exactly the assigned rows while the record builder rejects rows outside the
// current catalog (framework/validation_cache.md §Unit-scan row bookkeeping →
// Delta scope).
func TestUnitScanRowScopedDeltaSkipsRemovedCatalogRows(t *testing.T) {
	root, main := deltaBaseline(t, "    affects:\n      files:\n        - helper.js\n", map[string]string{"helper.js": "export const helper = true;\n"})

	// Remove contracts.js from the declared surface and delete the file in one
	// edit: the surplus row leaves the catalog, and the promise declaration
	// row (which cited the file) dies with it.
	data, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "implementation_surface: contracts.js", "implementation_surface: helper.js", 1)
	if edited == string(data) {
		t.Fatalf("spec edit did not apply")
	}
	if err := os.WriteFile(main, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "contracts.js")); err != nil {
		t.Fatal(err)
	}

	delta := deltaPlan(t, root)
	catalog, err := gaterun.UnitScanRowCatalog(root, delta)
	if err != nil {
		t.Fatal(err)
	}
	coverageID := ""
	for _, row := range catalog {
		if row.Kind == gaterun.RowKindCoverage {
			coverageID = row.ID
		}
	}
	if coverageID == "" {
		t.Fatalf("the current catalog carries no coverage row: %v", catalog)
	}
	wantRows := []string{coverageID, "decl:promise:auth.core"}
	if strings.Join(delta.ScanRows, ",") != strings.Join(wantRows, ",") {
		t.Fatalf("scan rows to re-run = %v, want %v — a removed baseline row must not be assigned", delta.ScanRows, wantRows)
	}

	// The scoped session judges only the assigned rows.
	report := fmt.Sprintf("row %s: ok — runnable coverage\nrow decl:promise:auth.core: match — files: helper.js\n", coverageID)
	if err := sharedSubmit(t, root, delta.RunID, "unit:auth", report); err != nil {
		t.Fatalf("scoped scan submit: %v", err)
	}
	for _, ck := range delta.Coverage {
		if ck.Key == "unit:auth" {
			continue
		}
		rowReport := grDefaultQualityReport(delta, ck)
		if gaterun.IsItemKind(ck.Kind) {
			rowReport = grVerifyItemReport(ck.Key, main, "helper.js")
		}
		if err := sharedSubmit(t, root, delta.RunID, ck.Key, rowReport); err != nil {
			t.Fatalf("submit %s: %v", ck.Key, err)
		}
	}
	cross := "Cross-check: 1/1 PASS — consistent\ncross: " + main + ": Description\n"
	for _, ck := range delta.Coverage {
		cross += "Effective status: " + ck.Key + " = pass\n"
	}
	cross += "Effective status: cross = pass\n"
	grSubmitOK(t, root, delta.RunID, "cross", cross)
	grFinalizeOK(t, root, delta.RunID)

	// The rewritten record carries the current catalog only: the removed row
	// is gone, the surviving rows are fresh or carried.
	baseline := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	binding, ok := baseline.Records["unit:auth"]
	if !ok {
		t.Fatalf("delta finalize did not publish a unit-scan record")
	}
	record, err := judgments.Load(root, binding.Reference)
	if err != nil {
		t.Fatal(err)
	}
	verdicts := map[string]string{}
	for _, row := range record.Rows {
		verdicts[row.ID] = row.Verdict
	}
	if len(verdicts) != 3 {
		t.Fatalf("merged record rows = %v, want the current catalog (3 rows)", verdicts)
	}
	if verdicts[coverageID] != judgments.RowOK {
		t.Fatalf("merged coverage row = %q, want %q: %v", verdicts[coverageID], judgments.RowOK, verdicts)
	}
	if _, removed := verdicts["surplus:contracts.js"]; removed {
		t.Fatalf("the removed row survived into the merged record: %v", verdicts)
	}
	if verdicts["decl:promise:auth.core"] != judgments.RowMatch || verdicts["surplus:helper.js"] != judgments.RowClean {
		t.Fatalf("merged row verdicts = %v", verdicts)
	}
	if record.Verdict != "ALIGNED" {
		t.Fatalf("aggregate verdict = %q, want ALIGNED", record.Verdict)
	}
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("delta finalize must leave the gate fresh: %+v %v", check, err)
	}
}

// TestUnitScanNewDeclaredFileReopensWholeScan pins the declared-surface growth
// rule: a new file in a declared surface can change a Step 1 declaration
// verdict, so the whole scan re-runs; a new surplus row alone would judge only
// Step 5 Part A.
func TestUnitScanNewDeclaredFileReopensWholeScan(t *testing.T) {
	root, main := deltaBaseline(t, "", nil)

	data, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data),
		"implementation_surface: contracts.js\n",
		"implementation_surface: contracts.js\n    affects:\n      files:\n        - extra.js\n", 1)
	if edited == string(data) {
		t.Fatalf("spec edit did not apply")
	}
	if err := os.WriteFile(main, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, "extra.js", "export function response(token) { return {token, extra: true}; }\n")

	run := deltaPlan(t, root)
	if !grContainsString(coverageKeysOf(run), "unit:auth") {
		t.Fatalf("the unit scan must re-run when the declared surface grew, got coverage %v", coverageKeysOf(run))
	}
	if len(run.ScanRows) != 0 {
		t.Fatalf("a declared-surface growth must re-run the whole scan, got a row scope %v", run.ScanRows)
	}
}

// TestUnitScanRemovedRunnableItemReopensCoverageRow pins the acceptance-item
// set dependency: removing the only runnable item changes the Step 5 Part 2
// coverage judgment, so the new coverage row re-runs and a delta recovery plan
// exists. Carrying the old scan here would leave the cache stale behind a
// fresh-looking plan.
func TestUnitScanRemovedRunnableItemReopensCoverageRow(t *testing.T) {
	root, main := deltaDependentBaseline(t)

	data, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	set := strings.Index(text, "acceptance_item_set:\n")
	if set < 0 {
		t.Fatal("the spec carries no acceptance_item_set block")
	}
	start := strings.Index(text[set:], "  - id: auth.core\n")
	end := strings.Index(text[set:], "  - id: auth.manual\n")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("probe markers not found: set=%d start=%d end=%d", set, start, end)
	}
	edited := text[:set+start] + text[set+end:]
	if err := os.WriteFile(main, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	run := deltaPlan(t, root)
	if !grContainsString(coverageKeysOf(run), "unit:auth") {
		t.Fatalf("the unit scan must re-run when the item set changed, got coverage %v", coverageKeysOf(run))
	}
	coverageRow := false
	for _, row := range run.ScanRows {
		if strings.HasPrefix(row, "coverage:") {
			coverageRow = true
		}
	}
	if !coverageRow {
		t.Fatalf("the Step 5 Part 2 coverage row did not re-run: %v", run.ScanRows)
	}
}

// TestUnitScanRowScopedReportRejectsFullReportMachinery pins the scoped report
// contract mechanically: a report that carries the full report's overall
// verdict line or any aggregate summary line is rejected, while the same
// report without them is accepted. Findings written under a stray aggregate
// line would sit outside every parsed row block and be silently dropped.
func TestUnitScanRowScopedReportRejectsFullReportMachinery(t *testing.T) {
	root, _ := deltaBaseline(t, "    affects:\n      files:\n        - helper.js\n", map[string]string{"helper.js": "export const helper = true;\n"})
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token, edited: true}; }\n")
	delta := deltaPlan(t, root)

	body := "row decl:promise:auth.core: match — files: contracts.js\nrow surplus:contracts.js: clean — no undeclared behavior\n"
	strays := []string{
		"unit:auth: ALIGNED — fixture scan",
		"declarations.api_endpoints: checked=1 — fixture scan",
		"declarations.error_types: checked=0 — fixture scan",
		"surplus: found=1 — fixture scan",
		"non_runnable: count=0 — fixture scan",
	}
	for _, stray := range strays {
		err := sharedSubmit(t, root, delta.RunID, "unit:auth", body+stray+"\n")
		if err == nil {
			t.Fatalf("a row-scoped report carrying %q must be rejected", stray)
		}
		if !strings.Contains(err.Error(), "row-scoped unit-scan report") {
			t.Fatalf("unexpected rejection for %q: %v", stray, err)
		}
	}
	// The same report without the stray lines is accepted.
	if err := sharedSubmit(t, root, delta.RunID, "unit:auth", body); err != nil {
		t.Fatalf("the conforming scoped report must be accepted: %v", err)
	}
}

// TestUnitScanFailedRepairRerunsWholeScan pins the repair half of the row
// bookkeeping end to end: a failure record's unit-scan MISMATCH row is not a
// carry candidate. Row-level reuse is derived only from a passing conclusion,
// so the repair re-judges the whole catalog, the retained mismatch is reported
// afresh, and its finding stays in the repaired cache
// (framework/verification_scope.md §Coverage Model → Unit-scan rows;
// framework/validation_cache.md §Unit-scan row bookkeeping → Delta scope).
func TestUnitScanFailedRepairRerunsWholeScan(t *testing.T) {
	root, main := deltaBaseline(t, "    affects:\n      files:\n        - helper.js\n", map[string]string{"helper.js": "export const helper = true;\n"})

	// A full run whose unit scan retains one mismatch: the promise's
	// declaration row is a mismatch citing contracts.js only.
	failID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency")
	failRun := mustLoadRun(t, root, failID)
	for _, ck := range failRun.Coverage {
		if coveredKeys(t, root, failRun)[ck.Key] {
			continue // the plan reused a still-valid public record
		}
		report := grDefaultQualityReport(failRun, ck)
		if ck.Kind == gaterun.SessionKindUnit {
			report = grVerifyUnitScanReport(t, root, failRun, ck.Key)
			report = strings.Replace(report,
				"unit:auth: ALIGNED — fixture scan\n",
				"unit:auth: MISMATCH (structural) — one retained mismatch\n"+grVerifyAnalysisFields("unit:auth"), 1)
			report = strings.Replace(report,
				"row decl:promise:auth.core: match — files: contracts.js, helper.js",
				"row decl:promise:auth.core: mismatch — files: contracts.js", 1)
		}
		if gaterun.IsItemKind(ck.Kind) {
			report = grVerifyItemReport(ck.Key, main, "contracts.js")
		}
		if err := sharedSubmit(t, root, failID, ck.Key, report); err != nil {
			t.Fatalf("submit %s: %v", ck.Key, err)
		}
	}
	cross := "Cross-check: 1/1 PASS — consistent\ncross: " + main + ": Description\n"
	for _, ck := range failRun.Coverage {
		cross += "Effective status: " + ck.Key + " = pass\n"
	}
	cross += "Effective status: cross = pass\n"
	grSubmitOK(t, root, failID, "cross", cross)
	grFinalizeOK(t, root, failID)

	// The failure record is the repair baseline. Being a failed conclusion, it
	// must re-run whole — the live mismatch row is re-judged, not carried.
	grWriteFile(t, root, "helper.js", "export const helper = false;\n")
	repair := deltaPlan(t, root)
	if repair.Mode != gaterun.ModeRepair {
		t.Fatalf("a failure record must derive a repair, got mode %q", repair.Mode)
	}
	if len(repair.ScanRows) != 0 {
		t.Fatalf("a failed unit scan must re-run whole, got a row-scoped re-run %v", repair.ScanRows)
	}

	// The full scan report re-judges the retained mismatch and carries its
	// finding under the aggregate verdict line.
	report := grVerifyUnitScanReport(t, root, repair, "unit:auth")
	report = strings.Replace(report,
		"unit:auth: ALIGNED — fixture scan\n",
		"unit:auth: MISMATCH (structural) — one retained mismatch\n"+grVerifyAnalysisFields("unit:auth"), 1)
	report = strings.Replace(report,
		"row decl:promise:auth.core: match — files: contracts.js, helper.js",
		"row decl:promise:auth.core: mismatch — files: contracts.js", 1)
	if err := sharedSubmit(t, root, repair.RunID, "unit:auth", report); err != nil {
		t.Fatalf("the full repair scan report was rejected: %v", err)
	}
	for _, ck := range repair.Coverage {
		if ck.Key == "unit:auth" || coveredKeys(t, root, repair)[ck.Key] {
			continue
		}
		rowReport := grDefaultQualityReport(repair, ck)
		if gaterun.IsItemKind(ck.Kind) {
			rowReport = grVerifyItemReport(ck.Key, main, "contracts.js")
		}
		if err := sharedSubmit(t, root, repair.RunID, ck.Key, rowReport); err != nil {
			t.Fatalf("submit %s: %v", ck.Key, err)
		}
	}
	cross = "Cross-check: 1/1 PASS — consistent\ncross: " + main + ": Description\n"
	for _, ck := range repair.Coverage {
		cross += "Effective status: " + ck.Key + " = pass\n"
	}
	cross += "Effective status: cross = pass\n"
	grSubmitOK(t, root, repair.RunID, "cross", cross)
	grFinalizeOK(t, root, repair.RunID)

	// The failed finding is retained: the repair re-judged it instead of
	// silently dropping it while publishing a pass cache.
	baseline := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	if len(baseline.Findings) != 1 || baseline.Findings[0].SourceKey != "unit:auth" {
		t.Fatalf("the repaired cache lost the retained unit-scan finding: %+v", baseline.Findings)
	}
	cache, err := validationcache.ReadGateBaseline(root, "unit", "auth", "verify")
	if err != nil {
		t.Fatal(err)
	}
	if cache.Result != "fail" || !cache.Blocking {
		t.Fatalf("the repaired gate must remain failed while its P1 finding is retained, got result=%q blocking=%v", cache.Result, cache.Blocking)
	}
	binding, ok := baseline.Records["unit:auth"]
	if !ok {
		t.Fatalf("repair finalize did not publish a unit-scan record")
	}
	record, err := judgments.Load(root, binding.Reference)
	if err != nil {
		t.Fatal(err)
	}
	verdicts := map[string]string{}
	for _, row := range record.Rows {
		verdicts[row.ID] = row.Verdict
	}
	if len(verdicts) != 4 {
		t.Fatalf("merged record rows = %v, want the full catalog (4 rows)", verdicts)
	}
	if verdicts["decl:promise:auth.core"] != judgments.RowMismatch || verdicts["surplus:contracts.js"] != judgments.RowClean || verdicts["surplus:helper.js"] != judgments.RowClean {
		t.Fatalf("merged row verdicts = %v", verdicts)
	}
	if record.Verdict != "MISMATCH" {
		t.Fatalf("aggregate verdict = %q, want MISMATCH", record.Verdict)
	}
}

// TestUnitScanEvidenceRowPinsItsOwnSection pins the exact region routing of
// evidence-section declaration rows: a section heading that is a prefix of a
// neighboring heading ("Auth behavior" vs "Auth behavior: extended") must pin
// its own region, so an edit to the section re-runs exactly that row instead
// of a stale verdict pinned to its neighbor.
func TestUnitScanEvidenceRowPinsItsOwnSection(t *testing.T) {
	evidence := "---\nunit: auth\n---\n\n# Evidence\n\n" +
		"## Auth behavior: extended\n\nKeys:\n  - item:auth.manual\n\nExtended observed behavior.\n\n" +
		"## Auth behavior\n\nKeys:\n  - item:auth.core\n\nObserved core behavior.\n"
	root, _ := deltaBaseline(t,
		"  - id: auth.manual\n    promise: auth.core\n    source: [Narrative]\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: contracts.js\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n",
		map[string]string{"docs/specs/units/candidate/evidence/unit_auth.md": evidence})

	// The baseline record pins the evidence row to its own section, not to the
	// earlier heading whose text extends it.
	baseline := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	binding, ok := baseline.Records["unit:auth"]
	if !ok {
		t.Fatalf("the baseline records no unit-scan judgment")
	}
	record, err := judgments.Load(root, binding.Reference)
	if err != nil {
		t.Fatal(err)
	}
	own, foreign := false, false
	for _, row := range record.Rows {
		if row.ID != "decl:evidence:auth.core" {
			continue
		}
		for _, dep := range row.Deps {
			for _, region := range dep.Deps {
				if !strings.Contains(region, "region:section:Auth behavior:") {
					continue
				}
				if strings.Contains(region, "extended") {
					foreign = true
					continue
				}
				own = true
			}
		}
	}
	if foreign {
		t.Fatalf("the evidence row for auth.core pinned the neighboring section's region: %v", record.Rows)
	}
	if !own {
		t.Fatalf("the evidence row for auth.core does not pin its own section region: %v", record.Rows)
	}

	// An edit to the row's own section must re-run that row; a wrong pin would
	// leave it live and carry a stale verdict.
	evidencePath := filepath.Join(root, "docs", "specs", "units", "candidate", "evidence", "unit_auth.md")
	if err := os.WriteFile(evidencePath, []byte(strings.Replace(evidence, "Observed core behavior.", "Observed core behavior. Edited.", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	delta := deltaPlan(t, root)
	if !strings.Contains(strings.Join(delta.ScanRows, ","), "decl:evidence:auth.core") {
		t.Fatalf("an edit to the row's own section must re-run that row: scan rows = %v", delta.ScanRows)
	}
}

// TestUnitScanCatalogExcludesInputSurfaces pins the declared-surface rule for
// checklist rows: an evidence directory supplied through the input manifest is
// readable background, not a declared code file, so it must not add a surplus
// row (framework/verification_scope.md §Coverage Model → Unit-scan rows).
func TestUnitScanCatalogExcludesInputSurfaces(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecSurface(t, root, "auth", "contracts.js", "")
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")
	grWriteFile(t, root, "tests/auth_test.go", "package tests\n")

	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "none", "--inputs-file", grInputsManifest(t, "tests"))
	run := mustLoadRun(t, root, id)
	catalog, err := gaterun.UnitScanRowCatalog(root, run)
	if err != nil {
		t.Fatal(err)
	}
	foundDeclared := false
	for _, row := range catalog {
		if row.ID == "surplus:tests/auth_test.go" {
			t.Fatalf("an input-directory file became a surplus row: %v", catalog)
		}
		if row.ID == "surplus:contracts.js" {
			foundDeclared = true
		}
	}
	if !foundDeclared {
		t.Fatalf("the declared code file is missing from the catalog: %v", catalog)
	}
}

// TestUnitScanFullReportRequiresAssignedCatalogRows pins the submit-time
// completeness rule: a full-run report that omits a catalog row is rejected at
// submission, where the accepted session can still be corrected, instead of
// stranding the run at finalize.
func TestUnitScanFullReportRequiresAssignedCatalogRows(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecSurface(t, root, "auth", "contracts.js", "")
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")

	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	run := mustLoadRun(t, root, id)
	spec, err := gaterun.BuildSessionSpec(root, run, []string{"unit:auth"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.ScanRowScoped {
		t.Fatalf("a full-run scan mission must not be row-scoped")
	}
	if len(spec.ScanRows) == 0 {
		t.Fatalf("a full-run scan mission must carry the catalog")
	}
	if !strings.Contains(strings.Join(spec.Context, "\n"), "The assigned catalog rows are: "+strings.Join(spec.ScanRows, ", ")) {
		t.Fatalf("a full-run scan mission must list the assigned catalog: %v", spec.Context)
	}
	report := grVerifyUnitScanReport(t, root, run, "unit:auth")
	missing := strings.Replace(report, "row surplus:contracts.js: clean — no undeclared behavior\n", "", 1)
	if err := sharedSubmit(t, root, id, "unit:auth", missing); err == nil || !strings.Contains(err.Error(), "does not judge assigned row") {
		t.Fatalf("a full report missing a catalog row must be rejected at submit, got %v", err)
	}
	// A row outside the catalog is rejected too.
	foreign := report + "row surplus:not-declared.js: clean — not in the catalog\n"
	if err := sharedSubmit(t, root, id, "unit:auth", foreign); err == nil || !strings.Contains(err.Error(), "did not assign") {
		t.Fatalf("a full report with a foreign row must be rejected at submit, got %v", err)
	}
}

// TestUnitScanExplicitRerunRejudgesWholeCatalog pins the override rule: an
// explicit --rerun names the conclusion, so the row scope must widen to the
// complete catalog instead of the mechanically derived subset.
func TestUnitScanExplicitRerunRejudgesWholeCatalog(t *testing.T) {
	root, _ := deltaBaseline(t, "    affects:\n      files:\n        - helper.js\n", map[string]string{"helper.js": "export const helper = true;\n"})
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token, edited: true}; }\n")

	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--rerun", "unit:auth")
	run := mustLoadRun(t, root, id)
	if len(run.ScanRows) != 0 {
		t.Fatalf("an explicit --rerun must not be narrowed to rows %v", run.ScanRows)
	}
	spec, err := gaterun.BuildSessionSpec(root, run, []string{"unit:auth"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.ScanRowScoped {
		t.Fatalf("an explicit --rerun must produce a full-catalog scan mission")
	}
	catalog, err := gaterun.UnitScanRowCatalog(root, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.ScanRows) != len(catalog) {
		t.Fatalf("mission rows = %v, want the full catalog (%d rows)", spec.ScanRows, len(catalog))
	}
}

// TestUnitScanFullReportRejectsFewerFindingsThanMismatchRows pins the anti-loss
// contract for the full report: each retained mismatch/surplus row carries its
// own finding block (framework/unit_verify_checklist.md §Output Format).
func TestUnitScanFullReportRejectsFewerFindingsThanMismatchRows(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecSurface(t, root, "auth", "contracts.js", "")
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")

	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	run := mustLoadRun(t, root, id)
	report := grVerifyUnitScanReport(t, root, run, "unit:auth")
	report = strings.Replace(report,
		"unit:auth: ALIGNED — fixture scan\n",
		"unit:auth: MISMATCH (structural) — two retained mismatches\n"+grVerifyAnalysisFields("unit:auth"), 1)
	report = strings.Replace(report,
		"row decl:promise:auth.core: match — files: contracts.js",
		"row decl:promise:auth.core: mismatch — files: contracts.js", 1)
	report = strings.Replace(report,
		"row surplus:contracts.js: clean — no undeclared behavior",
		"row surplus:contracts.js: surplus — one surplus construct", 1)

	err := sharedSubmit(t, root, id, "unit:auth", report)
	if err == nil || !strings.Contains(err.Error(), "finding block") {
		t.Fatalf("a report with two mismatched rows and one finding block must be rejected, got %v", err)
	}
}

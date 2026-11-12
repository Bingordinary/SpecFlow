package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// This file pins the verify delta blast radius for one-line edits against the
// real tooling. Each scenario builds a minimal full verify baseline (one
// acceptance item, one code file, one standing relationship), makes a single
// one-line edit, runs `gate-plan --mode delta`, and asserts the derived re-run
// scope (framework/validation_cache.md §Unit-scan row bookkeeping).
//
// The relationship scope is per relationship: only one whose consumed key
// classes re-ran re-checks (framework/verification_scope.md §Incremental
// relationship work).

// deltaBaseline builds a fresh fixture and drives one full verify run to a pass
// cache with one standing relationship (contract_consistency). extraItems is
// appended to the unit spec's single acceptance item (e.g. an affects block),
// and extraFiles are written before the plan so they join the derived surface.
func deltaBaseline(t *testing.T, extraItems string, extraFiles map[string]string) (root, main string) {
	t.Helper()
	root = createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	main = grWriteSpecSurface(t, root, "auth", "contracts.js", extraItems)
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")
	for path, content := range extraFiles {
		grWriteFile(t, root, path, content)
	}

	fullID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency")
	run := mustLoadRun(t, root, fullID)
	for _, ck := range run.Coverage {
		report := grDefaultQualityReport(run, ck)
		if ck.Kind == gaterun.SessionKindUnit {
			report = grVerifyUnitScanReport(t, root, run, ck.Key)
		}
		if gaterun.IsItemKind(ck.Kind) {
			report = grVerifyItemReport(ck.Key, main, "contracts.js")
		}
		if err := sharedSubmit(t, root, fullID, ck.Key, report); err != nil {
			t.Fatalf("submit %s: %v", ck.Key, err)
		}
	}
	cross := "Cross-check: 1/1 PASS — consistent\ncross: " + main + ": Description\n"
	for _, ck := range run.Coverage {
		cross += "Effective status: " + ck.Key + " = pass\n"
	}
	cross += "Effective status: cross = pass\n"
	grSubmitOK(t, root, fullID, "cross", cross)
	grFinalizeOK(t, root, fullID)
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("baseline must be fresh: %+v %v", check, err)
	}
	return root, main
}

// deltaPlan plans a verify delta without declaring a relationship scope, so
// the tooling derives the re-run scope mechanically from the baseline.
func deltaPlan(t *testing.T, root string) *gaterun.Run {
	t.Helper()
	var stdout, stderr strings.Builder
	err := runGatePlan([]string{"--repo-root", root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("delta gate-plan failed: %v (stderr=%s stdout=%s)", err, strings.TrimSpace(stderr.String()), stdout.String())
	}
	var id string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if after, ok := strings.CutPrefix(line, "Gate run planned: "); ok {
			id = strings.TrimSpace(after)
		}
	}
	if id == "" {
		t.Fatalf("no run id in delta plan output:\n%s", stdout.String())
	}
	return mustLoadRun(t, root, id)
}

func deltaExecuted(run *gaterun.Run) map[string]bool {
	executed := map[string]bool{}
	for _, ck := range run.Coverage {
		executed[ck.Key] = true
	}
	return executed
}

func deltaAssert(t *testing.T, name string, run *gaterun.Run, wantExecuted, wantCarried []string) {
	t.Helper()
	executed := deltaExecuted(run)
	for _, key := range wantExecuted {
		if !executed[key] {
			t.Fatalf("%s: key %q must re-run; coverage=%v carried=%v", name, key, coverageKeysOf(run), run.CarriedKeys)
		}
	}
	for _, key := range wantCarried {
		if executed[key] {
			t.Fatalf("%s: key %q must carry; coverage=%v carried=%v", name, key, coverageKeysOf(run), run.CarriedKeys)
		}
	}
}

// deltaAssertRelationship pins whether the standing relationship re-checks or
// carries with its consumed keys unchanged.
func deltaAssertRelationship(t *testing.T, name string, run *gaterun.Run, wantRerun bool) {
	t.Helper()
	assigned := false
	for _, rel := range run.Relationships {
		if rel == "contract_consistency" {
			assigned = true
		}
	}
	if assigned != wantRerun {
		t.Fatalf("%s: contract_consistency re-check = %t, want %t (relationships=%v carried=%v)", name, assigned, wantRerun, run.Relationships, run.CarriedKeys)
	}
}

func TestVerifyDeltaBlastRadius(t *testing.T) {
	t.Run("A_narrative_one_line_edit", func(t *testing.T) {
		root, main := deltaBaseline(t, "", nil)
		data, err := os.ReadFile(main)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(main, []byte(strings.Replace(string(data), "Prose.", "Prose. Edited.", 1)), 0644); err != nil {
			t.Fatal(err)
		}
		run := deltaPlan(t, root)
		// The item and design declared the narrative as their input; the
		// architecture assessment declares the same block, so it re-runs too.
		// The unit scan's rows pin promise/evidence regions and code files
		// only, so the scan carries.
		deltaAssert(t, "A", run, []string{"item:auth:auth.core", "design:auth:contracts.js", "architecture:auth"}, []string{"unit:auth", "code:contracts.js"})
		// The design judgment consumes the relationship's contract side.
		deltaAssertRelationship(t, "A", run, true)
	})

	t.Run("B_item_pass_condition_one_line_edit", func(t *testing.T) {
		root, main := deltaBaseline(t, "", nil)
		data, err := os.ReadFile(main)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(main, []byte(strings.Replace(string(data), "pass_condition: Passes.", "pass_condition: Passes quickly.", 1)), 0644); err != nil {
			t.Fatal(err)
		}
		run := deltaPlan(t, root)
		// The item edit changes its region, so the Step 5 Part 2 coverage row
		// has a new identity and re-judges the runnable-set assessment. Nothing
		// else reads this line: the code and design records carry, the
		// architecture assessment pins its declared anchors, and the scan's
		// declaration/surplus rows carry.
		deltaAssert(t, "B", run, []string{"item:auth:auth.core", "unit:auth"}, []string{"code:contracts.js", "design:auth:contracts.js", "architecture:auth"})
		// contract_consistency consumes the unit scan key, so the coverage-row
		// re-run re-checks it; it is a conservative re-check, not an item edit.
		deltaAssertRelationship(t, "B", run, true)
	})

	t.Run("C_code_one_line_edit", func(t *testing.T) {
		root, _ := deltaBaseline(t, "", nil)
		grWriteFile(t, root, "contracts.js", "export function response(token) { return {token, edited: true}; }\n")
		run := deltaPlan(t, root)
		// Every judgment that pinned the file re-runs: both scan rows cite it
		// (declaration rows cite their verified files, the surplus row pins the
		// file), so the scan row scope covers its whole catalog and runs whole.
		deltaAssert(t, "C", run, []string{"item:auth:auth.core", "unit:auth", "code:contracts.js", "design:auth:contracts.js", "architecture:auth"}, nil)
		deltaAssertRelationship(t, "C", run, true)
	})

	t.Run("D_framework_judging_section_one_line_edit", func(t *testing.T) {
		root, _ := deltaBaseline(t, "", nil)
		doc := filepath.Join(root, "framework", "unit_verify_checklist.md")
		data, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "## Step 2 — Acceptance alignment" {
				lines = append(lines[:i+1], append([]string{"Extra judging sentence for step two."}, lines[i+1:]...)...)
				break
			}
		}
		if err := os.WriteFile(doc, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			t.Fatal(err)
		}
		run := deltaPlan(t, root)
		// Per-kind protocol fingerprints: only the item sessions apply Step 2,
		// so only the item record dies; the scan, code, design and architecture
		// records bind their own classes' standards and carry.
		deltaAssert(t, "D", run, []string{"item:auth:auth.core"}, []string{"unit:auth", "code:contracts.js", "design:auth:contracts.js", "architecture:auth"})
		deltaAssertRelationship(t, "D", run, false)
	})

	t.Run("E_provider_contract_edit_reopens_the_whole_scan", func(t *testing.T) {
		root, main := deltaDependentBaseline(t)

		// One delta kills a record-level promise pin (the provider contract
		// changed) and exactly one row pin (the non-runnable item's text).
		// The record-level pins are not rows: the scan re-runs whole —
		// row-scoping the re-run would carry row verdicts judged under the
		// old provider contract.
		providerSpec := filepath.Join(root, "docs", "specs", "units", "candidate", "unit_provider.md")
		data, err := os.ReadFile(providerSpec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(providerSpec, []byte(strings.Replace(string(data), "v1", "v2", 1)), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(main, []byte(strings.Replace(mustRead(t, main), "manual reason one", "manual reason one edited", 1)), 0644); err != nil {
			t.Fatal(err)
		}
		run := deltaPlan(t, root)
		deltaAssert(t, "E", run, []string{"unit:auth"}, nil)
		if len(run.ScanRows) != 0 {
			t.Fatalf("a provider-contract change must re-run the whole scan, got a row-scoped re-run %v", run.ScanRows)
		}
	})

	t.Run("F_new_promise_scopes_to_its_row_and_the_surplus_rows", func(t *testing.T) {
		root, main := deltaBaseline(t, "", nil)

		// Add a promise entry plus its covering acceptance item: the row
		// catalog grows decl:promise:auth.api, a row the baseline never
		// judged, and the declaration-row set changes, which reopens the
		// surplus row — its undeclared-behavior question is answered against
		// the complete declaration catalog.
		edited := strings.Replace(mustRead(t, main),
			"## Acceptance Items",
			"  - id: auth.api\n    summary: New API contract.\n    source: [Narrative]\n    contract: {invariants: [api]}\n\n## Acceptance Items",
			1)
		if edited == mustRead(t, main) {
			t.Fatalf("spec edit did not apply")
		}
		edited = strings.Replace(edited,
			"    pass_condition: Passes.\n    runnable: yes\n",
			"    pass_condition: Passes.\n    runnable: yes\n  - id: auth.api\n    promise: auth.api\n    source: [Narrative]\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: contracts.js\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n",
			1)
		if err := os.WriteFile(main, []byte(edited), 0644); err != nil {
			t.Fatal(err)
		}
		run := deltaPlan(t, root)
		deltaAssert(t, "F", run, []string{"unit:auth", "item:auth:auth.api"}, nil)
		catalog, err := gaterun.UnitScanRowCatalog(root, run)
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
		wantRows := []string{coverageID, "decl:promise:auth.api", "surplus:contracts.js"}
		if strings.Join(run.ScanRows, ",") != strings.Join(wantRows, ",") {
			t.Fatalf("scan rows to re-run = %v, want %v — the new declaration row, the new coverage row, and the reopened surplus row", run.ScanRows, wantRows)
		}
	})
}

// deltaDependentBaseline builds a blast-radius baseline whose unit depends on
// a provider promise and carries one non-runnable acceptance item, so a single
// delta can kill a record-level promise pin and one row pin at once.
func deltaDependentBaseline(t *testing.T) (root, main string) {
	t.Helper()
	root = createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	writePromiseStateUnit(t, root, "provider", "provider.core", "", "v1")
	authContent := "---\nid: auth\nrule_refs: none\n---\n\n# auth\n\n" +
		"## Design Narrative\n### Narrative\n\nNarrative.\n\n" +
		"## Promises\n\npromises:\n  - id: auth.core\n    summary: Contract.\n    source: [Narrative]\n    contract: {invariants: [v1]}\n    depends_on: [provider.core]\n\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n" +
		"  - id: auth.core\n    promise: auth.core\n    source: [Narrative]\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/contracts.js\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n" +
		"  - id: auth.manual\n    promise: auth.core\n    source: [Narrative]\n    description: Given X, When Y, Then Z.\n    verification_type: manual\n    verification_surface: api\n    implementation_surface: src/contracts.js\n    verification_method: manual\n    pass_condition: Reviewed.\n    runnable: no\n    not_runnable_reason: manual reason one — external evidence: manual suite\n"
	main = grWriteFile(t, root, "docs/specs/units/candidate/unit_auth.md", authContent)
	grWriteFile(t, root, "src/contracts.js", "export function response(token) { return {token}; }\n")

	fullID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency")
	run := mustLoadRun(t, root, fullID)
	for _, ck := range run.Coverage {
		report := grDefaultQualityReport(run, ck)
		if ck.Kind == gaterun.SessionKindUnit {
			report = grVerifyUnitScanReport(t, root, run, ck.Key)
		}
		if gaterun.IsItemKind(ck.Kind) {
			report = grVerifyItemReport(ck.Key, main, "src/contracts.js")
		}
		if err := sharedSubmit(t, root, fullID, ck.Key, report); err != nil {
			t.Fatalf("submit %s: %v", ck.Key, err)
		}
	}
	cross := "Cross-check: 1/1 PASS — consistent\ncross: " + main + ": Description\n"
	for _, ck := range run.Coverage {
		cross += "Effective status: " + ck.Key + " = pass\n"
	}
	cross += "Effective status: cross = pass\n"
	grSubmitOK(t, root, fullID, "cross", cross)
	grFinalizeOK(t, root, fullID)
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("baseline must be fresh: %+v %v", check, err)
	}
	return root, main
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

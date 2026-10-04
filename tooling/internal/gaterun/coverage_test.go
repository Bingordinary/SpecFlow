package gaterun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// writeValidateBaseline writes a validate cache for unit auth with the given
// result/blocking and per-check behaviour.
func writeValidateBaseline(t *testing.T, repoRoot string, entries []validationcache.FileEntry, result string, blocking bool, basis string) {
	t.Helper()
	statuses := map[string]string{}
	for _, entry := range entries {
		for _, check := range entry.Checks {
			status := check.Status
			if status == "" {
				status = "pass"
			}
			statuses[check.Check] = status
		}
	}
	judgments, err := json.Marshal(JudgmentBaseline{SchemaVersion: 3, LogicalStatus: statuses, SynthesisDigest: "sha256:test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   "validate",
		Unit:      "auth",
		Mode:      "full",
		Basis:     basis,
		Result:    result,
		Target:    "candidate",
		Blocking:  blocking,
		Timestamp: "2026-01-01T00:00:00Z",
		Judgments: string(judgments),
		Entries:   entries,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mainCheckDecls declares every validate check on the main spec. Check 10
// (clarity) declares the Description section.
func mainCheckDecls() []validationcache.CheckDeclaration {
	var checks []validationcache.CheckDeclaration
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		checks = append(checks, validationcache.CheckDeclaration{Check: key, Sections: []string{"Description"}})
	}
	return checks
}

// coverageKeysOf lists a run's coverage keys in plan order.
func coverageKeysOf(run *Run) []string {
	var keys []string
	for _, ck := range run.Coverage {
		keys = append(keys, ck.Key)
	}
	return keys
}

func TestUnitValidateCoverageReadRefs(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src/dep", "")
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "b_rule_http", "src/auth", "")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"structural", "dependencies"} {
		spec, serr := BuildSessionSpec(repoRoot, run, []string{key})
		if serr != nil {
			t.Fatal(serr)
		}
		for _, ref := range []string{"unit:dep", "rule:b_rule_http"} {
			if !stringInSlice(spec.ReadRefs, ref) {
				t.Fatalf("%s session read refs = %v, want %s", key, spec.ReadRefs, ref)
			}
		}
	}
	for _, key := range []string{"design", "acceptance"} {
		spec, serr := BuildSessionSpec(repoRoot, run, []string{key})
		if serr != nil {
			t.Fatal(serr)
		}
		for _, ref := range []string{"unit:dep", "rule:b_rule_http"} {
			if stringInSlice(spec.ReadRefs, ref) {
				t.Fatalf("%s session received unrelated logical ref %s: %v", key, ref, spec.ReadRefs)
			}
		}
	}
}

func TestUnitValidateStructuralSessionCarriesUnresolvedReference(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "missing", "none", "src/auth", "")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{"structural"})
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(spec.ReadRefs, "unit:missing") {
		t.Fatalf("structural session must expose the unresolved Check 1 reference, got %+v", spec.ReadRefs)
	}
}

func TestUnitValidateDeltaStructuralSessionCarriesLogicalReferences(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src/dep", "")
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "b_rule_http", "src/auth", "")

	mainEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{mainEntry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, []string{"1"}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(coverageKeysOf(run), "structural") {
		t.Fatalf("expected the structural coverage key in the delta plan, got %v", coverageKeysOf(run))
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{"structural"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"unit:dep", "rule:b_rule_http"} {
		if !stringInSlice(spec.ReadRefs, ref) {
			t.Fatalf("delta structural session read refs = %v, want %s", spec.ReadRefs, ref)
		}
	}
}

func TestLoadCarriedResultsAssociatesCrossFindingWithAffectedKey(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		statuses[key] = "pass"
	}
	judgments, err := json.Marshal(JudgmentBaseline{
		SchemaVersion: 3,
		LogicalStatus: statuses,
		Findings: []Finding{{
			ID:           "cross/F1",
			Severity:     "P2",
			Text:         "combined advisory",
			Detail:       "[P2] combined advisory (actionable)\n  recommendation: resolve it",
			SourceKey:    CrossKey,
			AffectedKeys: []string{"1"},
		}},
		SynthesisDigest: "sha256:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   GateValidate,
		Unit:      "auth",
		Mode:      ModeFull,
		Basis:     ModeFull,
		Result:    "pass",
		Target:    TargetCandidate,
		Timestamp: "2026-01-01T00:00:00Z",
		Judgments: string(judgments),
		Entries:   []validationcache.FileEntry{entry},
	}); err != nil {
		t.Fatal(err)
	}

	results, err := loadCarriedResults(repoRoot, &Run{Gate: GateValidate, TargetKind: TargetKindUnit, TargetName: "auth"}, []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || len(results[0].Findings) != 1 || results[0].Findings[0].ID != "cross/F1" {
		t.Fatalf("expected the cross finding on affected key 1, got %+v", results)
	}
	if len(results[1].Findings) != 0 {
		t.Fatalf("unaffected key 2 must not carry the cross finding, got %+v", results[1].Findings)
	}
}

func TestLoadCarriedResultsRejectsOldJudgmentSchema(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	judgments, err := json.Marshal(JudgmentBaseline{
		SchemaVersion:   1,
		LogicalStatus:   map[string]string{"1": "pass"},
		SynthesisDigest: "sha256:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   GateValidate,
		Unit:      "auth",
		Mode:      ModeFull,
		Result:    "pass",
		Target:    TargetCandidate,
		Timestamp: "2026-01-01T00:00:00Z",
		Judgments: string(judgments),
		Entries:   []validationcache.FileEntry{entry},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCarriedResults(repoRoot, &Run{Gate: GateValidate, TargetKind: TargetKindUnit, TargetName: "auth"}, []string{"1"}); err == nil || !strings.Contains(err.Error(), "run the full command") {
		t.Fatalf("expected old schema to require a full run, got %v", err)
	}
}

func TestDeltaMapsUnclaimedDependencyUnit(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "none", "src", "")

	mainEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	depEntry, err := validationcache.BuildEntry(repoRoot, validationcache.EntryDeclaration{Path: "unit:dep"})
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{mainEntry, depEntry}, "pass", false, "full")

	// Change the dependency unit — its whole-file deps go stale. No check
	// declared the unit:dep entry, so the plan must map it to the group
	// owning Check 7.
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "extra: changed\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "dependencies" {
		t.Fatalf("expected the dependencies coverage key, got %s", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "1,10,2,3,4,5,6" {
		t.Fatalf("expected checks 1-6 and 10 carried over, got %v", run.CarriedKeys)
	}
	if len(run.Notices) == 0 {
		t.Fatal("expected a scope notice")
	}
}

func TestDeltaRerunForcesGroup(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "none", "src", "")

	mainEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	depEntry, err := validationcache.BuildEntry(repoRoot, validationcache.EntryDeclaration{Path: "unit:dep"})
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{mainEntry, depEntry}, "pass", false, "full")

	// No stale source, but a targeted finding on check 2 forces the design
	// group back into the re-run set.
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, []string{"2"}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "design" {
		t.Fatalf("expected the design coverage key, got %s", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "1,10,3,5,6,7,8,9" {
		t.Fatalf("expected the other checks carried over, got %v", run.CarriedKeys)
	}
}

func TestDeltaDegradesWithoutEvidence(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	// A pass cache without per-check evidence cannot be derived from.
	entry, err := validationcache.BuildEntry(repoRoot, validationcache.EntryDeclaration{Path: "docs/specs/units/candidate/unit_auth.md"})
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 || len(run.CarriedKeys) != 0 {
		t.Fatalf("expected a degraded full coverage set, got %v / carried %v", coverageKeysOf(run), run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "no per-check evidence") {
		t.Fatalf("expected a degradation notice, got %v", run.Notices)
	}
}

func TestRepairWithoutStatusMapDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "fail", true, "delta")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected a degraded full coverage set, got %v", coverageKeysOf(run))
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "incomplete per-check status map") {
		t.Fatalf("expected a degradation notice, got %v", run.Notices)
	}
}

// TestDeltaRequiresPassBaseline verifies that a delta plan refuses a baseline
// that is not a pass cache and degrades a status-less failure record to the
// full coverage set.
func TestDeltaRequiresPassBaseline(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "no baseline cache") {
		t.Fatalf("expected a missing-baseline error, got %v", err)
	}

	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "fail", true, "delta")
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "failure record") {
		t.Fatalf("expected the failure-record guidance, got %v", err)
	}
	// Repair on a status-less record degrades to the full coverage set.
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 || !strings.Contains(strings.Join(run.Notices, " "), "incomplete per-check status map") {
		t.Fatalf("expected the degraded repair plan, got %v / %v", coverageKeysOf(run), run.Notices)
	}
}

// TestVerifyDeltaRejectsConflictingBaseline verifies that a verify baseline
// whose result and blocking declarations disagree is rejected instead of being
// accepted as a pass baseline.
func TestVerifyDeltaRejectsConflictingBaseline(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}

	writeGateBaseline(t, repoRoot, "verify", "fail", "full", false, []validationcache.FileEntry{entry}, map[string]string{"1": "fail"})
	if _, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "declarations conflict") {
		t.Fatalf("expected a fail/non-blocking verify baseline to be rejected, got %v", err)
	}

	writeGateBaseline(t, repoRoot, "verify", "pass", "full", true, []validationcache.FileEntry{entry}, map[string]string{"1": "pass"})
	if _, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "declarations conflict") {
		t.Fatalf("expected a pass/blocking verify baseline to be rejected, got %v", err)
	}
}

// writeUnitItemsSpec writes a unit spec with the given acceptance item ids.
// The items declare implementation_surface src, so the helper also creates a
// real file there — verify planning rejects a surface that does not
// resolve.
func writeUnitItemsSpec(t *testing.T, repoRoot string, itemIDs ...string) string {
	t.Helper()
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_auth.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n"
	for _, id := range itemIDs {
		content += "  - id: " + id + "\n    description: " + id + ".\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	}
	if err := os.WriteFile(specPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return specPath
}

// writeGateBaseline writes a unit gate baseline cache with the given logical
// statuses.
func writeGateBaseline(t *testing.T, repoRoot, command, result, basis string, blocking bool, entries []validationcache.FileEntry, statuses map[string]string) {
	t.Helper()
	state := JudgmentBaseline{SchemaVersion: 3, LogicalStatus: statuses, SynthesisDigest: "sha256:test"}
	if command == GateVerify {
		entries, state = buildVerifyBaselineFixture(t, repoRoot, entries, statuses)
	}
	judgments, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   command,
		Unit:      "auth",
		Mode:      "full",
		Basis:     basis,
		Result:    result,
		Target:    TargetCandidate,
		Blocking:  blocking,
		Timestamp: "2026-01-01T00:00:00Z",
		Judgments: string(judgments),
		Entries:   entries,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestVerifyCoverageUnionsLenses verifies a merged verify plan covers the
// alignment items and the declared code files, each tagged with its lens.
func TestVerifyCoverageUnionsLenses(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "auth.core", "auth.aux")

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ck := range run.Coverage {
		got[ck.Key] = ck.Lens
	}
	want := map[string]string{"item:auth:auth.core": LensAlignment, "item:auth:auth.aux": LensAlignment, "code:src/main.go": LensQuality, "design:auth:src/main.go": LensQuality, "architecture:auth": LensQuality}
	if len(got) != len(want) {
		t.Fatalf("coverage = %v, want %v", got, want)
	}
	for key, lens := range want {
		if got[key] != lens {
			t.Fatalf("coverage key %q lens = %q, want %q (coverage %v)", key, got[key], lens, got)
		}
	}
}

// TestBuildSessionSpecRejectsMixedLenses verifies lens purity: a session's
// assigned key batch must be entirely within one lens.
func TestBuildSessionSpecRejectsMixedLenses(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "auth.core")

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildSessionSpec(repoRoot, run, []string{"item:auth:auth.core", "code:src/main.go"}); err == nil || !strings.Contains(err.Error(), "mixes") {
		t.Fatalf("expected a mixed-lens batch to be rejected, got %v", err)
	}
	// Each lens alone is a legal batch.
	if _, err := BuildSessionSpec(repoRoot, run, []string{"item:auth:auth.core"}); err != nil {
		t.Fatalf("alignment-only batch rejected: %v", err)
	}
	if _, err := BuildSessionSpec(repoRoot, run, []string{"code:src/main.go"}); err != nil {
		t.Fatalf("quality-only batch rejected: %v", err)
	}
}

// TestDeltaVerifyPlansNewItems verifies that a delta verify plan re-runs
// acceptance items the baseline never declared: a new item has no evidence to
// carry over, so it must execute like a stale judgment.
func TestDeltaVerifyPlansNewItems(t *testing.T) {
	repoRoot := newRepo(t)
	specPath := writeUnitItemsSpec(t, repoRoot, "auth.core", "auth.aux")

	decls := []validationcache.CheckDeclaration{
		{Check: "auth.core", Lens: LensAlignment, AcceptanceItemIDs: []string{"auth.core"}},
		{Check: "auth.aux", Lens: LensAlignment, AcceptanceItemIDs: []string{"auth.aux"}},
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	qualityEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "src/main.go", []validationcache.CheckDeclaration{{Check: "src/main.go", Lens: LensQuality}})
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "verify", "pass", "full", false, []validationcache.FileEntry{entry, qualityEntry},
		map[string]string{"auth.core": "pass", "auth.aux": "pass", "src/main.go": "pass"})

	// Add a new acceptance item after the baseline was written.
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("  - id: auth.new\n    description: New.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n")...)
	if err := os.WriteFile(specPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "item:auth:auth.new" {
		t.Fatalf("expected the new item's coverage key, got %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "architecture:auth,code:src/main.go,design:auth:src/main.go,item:auth:auth.aux,item:auth:auth.core" {
		t.Fatalf("expected the declared items and the quality key carried over, got %v", run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "re-run coverage keys item:auth:auth.new") {
		t.Fatalf("expected the new-key disclosure, got %v", run.Notices)
	}
}

func TestVerifyNewEvidenceForcesFullScopeInDeltaAndRepair(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         string
		result       string
		blocking     bool
		newPath      string
		wantCoverage string
	}{
		{"delta outside surface", ModeDelta, "pass", false, "tests/new_test.go", "item:auth:auth.core,item:auth:auth.aux,code:src/main.go,design:auth:src/main.go,architecture:auth"},
		{"repair outside surface", ModeRepair, "fail", true, "tests/new_test.go", "item:auth:auth.core,item:auth:auth.aux,code:src/main.go,design:auth:src/main.go,architecture:auth"},
		{"delta inside surface", ModeDelta, "pass", false, "src/helper.go", "item:auth:auth.core,item:auth:auth.aux,code:src/helper.go,design:auth:src/helper.go,code:src/main.go,design:auth:src/main.go,architecture:auth"},
		{"repair inside surface", ModeRepair, "fail", true, "src/helper.go", "item:auth:auth.core,item:auth:auth.aux,code:src/helper.go,design:auth:src/helper.go,code:src/main.go,design:auth:src/main.go,architecture:auth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := newRepo(t)
			writeUnitItemsSpec(t, repoRoot, "auth.core", "auth.aux")
			writeFile(t, repoRoot, "tests/existing_test.go", "package tests\n")
			coreStatus := "pass"
			if tc.blocking {
				coreStatus = "fail"
			}
			mainEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", []validationcache.CheckDeclaration{
				{Check: "auth.core", Status: coreStatus, Lens: LensAlignment, AcceptanceItemIDs: []string{"auth.core"}},
				{Check: "auth.aux", Status: "pass", Lens: LensAlignment, AcceptanceItemIDs: []string{"auth.aux"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			testEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "tests/existing_test.go", []validationcache.CheckDeclaration{{Check: "auth.core", Status: coreStatus}})
			if err != nil {
				t.Fatal(err)
			}
			qualityEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "src/main.go", []validationcache.CheckDeclaration{{Check: "src/main.go", Status: coreStatus, Lens: LensQuality}})
			if err != nil {
				t.Fatal(err)
			}
			writeGateBaseline(t, repoRoot, "verify", tc.result, "full", tc.blocking, []validationcache.FileEntry{mainEntry, testEntry, qualityEntry},
				map[string]string{"auth.core": coreStatus, "auth.aux": "pass", "src/main.go": coreStatus})

			writeFile(t, repoRoot, tc.newPath, "package fixture\n")
			run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, tc.mode,
				[]string{"tests/existing_test.go", tc.newPath}, nil, nil, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(coverageKeysOf(run), ","); got != tc.wantCoverage {
				t.Fatalf("new evidence did not re-run the full scope: %s", got)
			}
			if len(run.CarriedKeys) != 0 || !strings.Contains(strings.Join(run.Notices, " "), tc.newPath) {
				t.Fatalf("new evidence must prevent carry-over and name its cause: carried=%v notices=%v", run.CarriedKeys, run.Notices)
			}
		})
	}
}

func TestVerifyRecordedEvidenceKeepsDeltaScope(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "auth.core", "auth.aux")
	writeFile(t, repoRoot, "tests/existing_test.go", "package tests\n")
	mainEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", []validationcache.CheckDeclaration{
		{Check: "auth.core", Lens: LensAlignment, AcceptanceItemIDs: []string{"auth.core"}},
		{Check: "auth.aux", Lens: LensAlignment, AcceptanceItemIDs: []string{"auth.aux"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	testEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "tests/existing_test.go", []validationcache.CheckDeclaration{{Check: "auth.core"}})
	if err != nil {
		t.Fatal(err)
	}
	qualityEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "src/main.go", []validationcache.CheckDeclaration{{Check: "src/main.go", Lens: LensQuality}})
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "verify", "pass", "full", false, []validationcache.FileEntry{mainEntry, testEntry, qualityEntry},
		map[string]string{"auth.core": "pass", "auth.aux": "pass", "src/main.go": "pass"})

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeDelta,
		[]string{"tests/existing_test.go"}, []string{"item:auth:auth.core"}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "item:auth:auth.core" {
		t.Fatalf("recorded evidence widened the delta run: %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "architecture:auth,code:src/main.go,design:auth:src/main.go,item:auth:auth.aux" {
		t.Fatalf("unaffected item was not carried: %s", got)
	}
}

// TestDeltaRepairPartialStatusMapDegrades verifies the fail-closed repair path:
// a failure record that declares a status for only some checks must not carry
// the status-less ones over.
func TestDeltaRepairPartialStatusMapDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		decl := validationcache.CheckDeclaration{Check: key, Sections: []string{"Description"}}
		if key == "1" {
			decl.Status = "fail"
		}
		decls = append(decls, decl)
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{"1": "fail"}
	writeGateBaseline(t, repoRoot, "validate", "fail", "delta", true, []validationcache.FileEntry{entry}, statuses)

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected a degraded full coverage set, got %v", coverageKeysOf(run))
	}
	notice := strings.Join(run.Notices, " ")
	if !strings.Contains(notice, "incomplete per-check status map") || !strings.Contains(notice, "missing: 2, 3, 4, 5, 6, 7, 8, 9, 10") {
		t.Fatalf("expected the incomplete-status degradation with the missing checks, got %v", run.Notices)
	}
}

// TestDeltaRepairInvalidStatusValueDegrades verifies the fail-closed repair
// path: a status value outside pass/fail/carried cannot say which judgments
// failed, so nothing may be carried over.
func TestDeltaRepairInvalidStatusValueDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "3" {
			status = "bogus"
		}
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: status, Sections: []string{"Description"}})
		statuses[key] = status
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "fail", "delta", true, []validationcache.FileEntry{entry}, statuses)

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected a degraded full coverage set, got %v", coverageKeysOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "invalid per-check status map") || !strings.Contains(notice, "3=bogus") {
		t.Fatalf("expected the invalid-status degradation, got %v", run.Notices)
	}
}

// TestDeltaRepairCarriedInFullRunRecordDegrades verifies that a full-run
// failure record (`basis: full`) declaring `carried` is malformed state: a full
// run re-executed every judgment, so the entry cannot be trusted and the plan
// degrades to the full coverage set.
func TestDeltaRepairCarriedInFullRunRecordDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "3" {
			status = "carried"
		}
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: status, Sections: []string{"Description"}})
		statuses[key] = status
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "fail", "full", true, []validationcache.FileEntry{entry}, statuses)

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected a degraded full coverage set, got %v", coverageKeysOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "invalid per-check status map") || !strings.Contains(notice, "3=carried") {
		t.Fatalf("expected the illegal-carried degradation, got %v", run.Notices)
	}
}

// TestDeltaRepairStatusMapJudgmentMismatchDegrades verifies that a status map
// and the record's structured judgment baseline must agree on the key set: a
// disagreement cannot say which judgments failed.
func TestDeltaRepairStatusMapJudgmentMismatchDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: "pass", Sections: []string{"Description"}})
		statuses[key] = "pass"
	}
	delete(statuses, "3")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "fail", "delta", true, []validationcache.FileEntry{entry}, statuses)

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected a degraded full coverage set, got %v", coverageKeysOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "does not match its judgment baseline") || !strings.Contains(notice, "unmatched: 3") {
		t.Fatalf("expected the key-set mismatch degradation, got %v", run.Notices)
	}
}

// TestDeltaRepairCompleteStatusMapCarriesPassingGroups verifies the positive
// repair path: a complete, valid status map keeps the incremental plan and
// carries the passing groups over.
func TestDeltaRepairCompleteStatusMapCarriesPassingGroups(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: status, Sections: []string{"Description"}})
		statuses[key] = status
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "fail", "delta", true, []validationcache.FileEntry{entry}, statuses)

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "structural" {
		t.Fatalf("expected the failed group to re-run, got %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "10,2,4,5,7,8,9" {
		t.Fatalf("expected the passing groups carried over, got %v", run.CarriedKeys)
	}
	for _, notice := range run.Notices {
		if strings.Contains(notice, "degradation") || strings.Contains(notice, "full scope") {
			t.Fatalf("a complete status map must not degrade, got %v", run.Notices)
		}
	}
}

// TestDeltaRepairUsesPersistedTargetedInvalidation is the cross-session
// regression: no caller supplies --rerun after the targeted finding. The
// repair plan must read the persisted invalidation and exclude that judgment
// from carry-over.
func TestDeltaRepairUsesPersistedTargetedInvalidation(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: status, Sections: []string{"Description"}})
		statuses[key] = status
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "fail", "delta", true, []validationcache.FileEntry{entry}, statuses)

	if _, err := InvalidateTargeted(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, []string{"2"}); err != nil {
		t.Fatal(err)
	}
	// Simulated session boundary: the repair receives no transient --rerun key.
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "structural,design" {
		t.Fatalf("persisted check 2 must add its design group, got %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "10,5,7,8,9" {
		t.Fatalf("invalidated judgment must not be carried, got %v", run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "persisted targeted invalidations: 2") {
		t.Fatalf("plan did not disclose the persisted invalidation: %v", run.Notices)
	}
}

func TestDeltaRepairUnknownPersistedInvalidationDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: status, Sections: []string{"Description"}})
		statuses[key] = status
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "fail", "delta", true, []validationcache.FileEntry{entry}, statuses)
	if _, err := InvalidateTargeted(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, []string{"removed-check"}); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 || len(run.CarriedKeys) != 0 {
		t.Fatalf("unknown invalidation must degrade to full scope, got %v / carried %v", coverageKeysOf(run), run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), `re-run judgment "removed-check" is not in the current coverage surface`) {
		t.Fatalf("missing unknown-invalidation degradation notice: %v", run.Notices)
	}
}

func TestInvalidateTargetedMarksMatchingOpenRunOnly(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")

	validateRun, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	verifyRun, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	result, err := InvalidateTargeted(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, []string{"2"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.InvalidatedRunIDs, ",") != validateRun.RunID {
		t.Fatalf("invalidated runs = %v, want %s", result.InvalidatedRunIDs, validateRun.RunID)
	}
	loadedValidate, err := Load(repoRoot, validateRun.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedValidate.Status != StatusInvalidated {
		t.Fatalf("matching run status = %q", loadedValidate.Status)
	}
	loadedVerify, err := Load(repoRoot, verifyRun.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedVerify.Status != StatusOpen {
		t.Fatalf("unrelated run status = %q", loadedVerify.Status)
	}
}

// TestDeltaLegacyCrossOnlyBaselineDegrades verifies the fail-closed handling of
// a baseline written under the removed mandatory cross-check model: `cross` is
// not a coverage key, so a stale `cross` judgment is refused and the plan
// degrades to the full coverage set instead of trusting it.
func TestDeltaLegacyCrossOnlyBaselineDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "\n## Scope\n\nIn scope.\n")

	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md",
		[]validationcache.CheckDeclaration{{Check: CrossKey, Sections: []string{"Scope"}}})
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "pass", "full", false, []validationcache.FileEntry{entry}, map[string]string{})

	// Edit the Scope section so the legacy cross judgment is stale.
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_auth.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte(strings.Replace(string(content), "In scope.", "Changed scope.", 1)), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("a cross-only baseline carries nothing over, got %v", run.CarriedKeys)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected the full coverage set, got %v", coverageKeysOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, `re-run judgment "cross" is not in the current coverage surface`) {
		t.Fatalf("expected the legacy-cross degradation, got %v", run.Notices)
	}
}

// TestDeltaDegradesOnUntrackableEntry verifies that a baseline entry with no
// dependency chunks over a file with content degrades the plan: no check can
// attribute its staleness and the freshness chain can never see it fresh.
func TestDeltaDegradesOnUntrackableEntry(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	if err := os.MkdirAll(filepath.Join(repoRoot, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "src", "extra.go"), []byte("package src\n\nvar Extra = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	untrackable := validationcache.FileEntry{Path: "src/extra.go"}
	writeGateBaseline(t, repoRoot, "validate", "pass", "full", false, []validationcache.FileEntry{entry, untrackable}, map[string]string{})

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected a degraded full coverage set, got %v", coverageKeysOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "cannot attribute (no dependency chunks)") || !strings.Contains(notice, "src/extra.go") {
		t.Fatalf("expected the untrackable-entry degradation, got %v", run.Notices)
	}
}

// TestDeltaDegradesWhenMainFileMissingEvenWithReRuns verifies that a baseline
// files list without the target's main file degrades the plan even when the
// per-check derivation found a re-run reason: the finalize main-file check
// would reject the partial run.
func TestDeltaDegradesWhenMainFileMissingEvenWithReRuns(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	if err := os.MkdirAll(filepath.Join(repoRoot, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	aPath := filepath.Join(repoRoot, "src", "a.txt")
	bPath := filepath.Join(repoRoot, "src", "b.txt")
	if err := os.WriteFile(aPath, []byte("alpha\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("beta\n"), 0644); err != nil {
		t.Fatal(err)
	}

	entryA, err := validationcache.BuildEntryFromChecks(repoRoot, "src/a.txt", []validationcache.CheckDeclaration{{Check: "2"}})
	if err != nil {
		t.Fatal(err)
	}
	entryB, err := validationcache.BuildEntryFromChecks(repoRoot, "src/b.txt", []validationcache.CheckDeclaration{{Check: "1"}})
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "pass", "full", false, []validationcache.FileEntry{entryA, entryB},
		map[string]string{"1": "pass", "2": "pass"})

	// Edit b.txt: check 1 goes stale, so the old derivation would plan a
	// partial run (carrying check 2) even though the main file is missing.
	if err := os.WriteFile(bPath, []byte("beta changed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 5 {
		t.Fatalf("expected a degraded full coverage set, got %v", coverageKeysOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "does not include the main file") || !strings.Contains(notice, "unit_auth.md") {
		t.Fatalf("expected the missing-main-file degradation, got %v", run.Notices)
	}
}

// TestPlanRejectsReservedCrossItemID verifies that an acceptance item named
// with the reserved final-synthesis key is rejected before any run state is
// written (reserved-id collision).
func TestPlanRejectsReservedCrossItemID(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "cross", "auth.core")

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil || run.CoverageByKey("item:auth:cross") == nil {
		t.Fatalf("namespaced item must be distinct from synthesis: %v", err)
	}
}

// TestDeltaStableMissingBaselineMessage verifies that a stable-only target
// with no usable confirmation baseline gets the documented message (full
// confirmation run or fork) in both delta and repair modes.
func TestDeltaStableMissingBaselineMessage(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "stable", "auth", "none", "none", "src", "")

	for _, mode := range []string{ModeDelta, ModeRepair} {
		_, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetStable, mode, nil, nil, nil, time.Now())
		if err == nil {
			t.Fatalf("%s: expected the no-baseline error", mode)
		}
		if !strings.Contains(err.Error(), "No usable confirmation baseline") || !strings.Contains(err.Error(), "fork --unit auth") {
			t.Fatalf("%s: expected the documented stable-only message, got %v", mode, err)
		}
	}
}

// TestPreviewDeltaScopeFailureRecordUsesRepair verifies that the fresh report's
// delta scope preview derives the repair plan for a failure-record baseline —
// the recovery mode gate-plan requires — so the report and the planner never
// disagree (see framework/verification_scope.md §Delta Runs).
func TestPreviewDeltaScopeFailureRecordUsesRepair(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: status, Sections: []string{"Description"}})
		statuses[key] = status
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "fail", true, "delta")

	preview, err := PreviewDeltaScope(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PlanError != "" {
		t.Fatalf("expected a repair plan, got refusal %q", preview.PlanError)
	}
	if preview.Degraded {
		t.Fatalf("expected a derived repair plan, got degradation %q", preview.Reason)
	}
	if got := strings.Join(preview.Rerun, ","); got != "1,3,6" {
		t.Fatalf("expected the failed group's checks re-run, got %s", got)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(preview.Carried, ","); got != strings.Join(run.CarriedKeys, ",") {
		t.Fatalf("preview carried %v, plan carried %v", preview.Carried, run.CarriedKeys)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "10,2,4,5,7,8,9" {
		t.Fatalf("expected the other checks carried over, got %s", got)
	}
}

// TestPreviewDeltaScopePassBaselineUsesDelta verifies that a pass baseline
// still previews the delta plan, and that the preview equals the plan the
// planner generates for the same baseline.
func TestPreviewDeltaScopePassBaselineUsesDelta(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	// The declared section changes after the baseline: every declared check
	// is stale.
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_auth.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "Prose.", "Changed prose.", 1))
	if err := os.WriteFile(specPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewDeltaScope(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PlanError != "" || preview.Degraded {
		t.Fatalf("expected a derived delta plan, got refusal %q / degradation %q", preview.PlanError, preview.Reason)
	}
	if !preview.CoversFull {
		t.Fatalf("expected the stale section to cover every declared check, got rerun %v carried %v", preview.Rerun, preview.Carried)
	}
	if got := strings.Join(preview.Rerun, ","); got != "1,10,2,3,4,5,6,7,8,9" {
		t.Fatalf("expected every check re-run, got %s", got)
	}
	if len(preview.Carried) != 0 {
		t.Fatalf("expected nothing carried over, got %v", preview.Carried)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("expected the plan to carry nothing over, got %v", run.CarriedKeys)
	}
}

// TestDeltaPlanRefusesBaselineWithoutJudgments verifies that the derivation
// shared by fresh@ and gate-plan refuses a partial run that must carry
// judgments when the baseline cache has no structured judgment state — the
// preview must report the refusal instead of a carry plan the planner rejects.
func TestDeltaPlanRefusesBaselineWithoutJudgments(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "\n## Scope\n\nIn scope.\n")

	var decls []validationcache.CheckDeclaration
	decls = append(decls, validationcache.CheckDeclaration{Check: "1", Sections: []string{"Description"}})
	for _, key := range []string{"2", "3", "4", "5", "6", "7", "8", "9"} {
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Sections: []string{"Scope"}})
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	// A legacy baseline: per-check evidence, no GATE_JUDGMENTS block.
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   "validate",
		Unit:      "auth",
		Mode:      "full",
		Basis:     "full",
		Result:    "pass",
		Target:    TargetCandidate,
		Timestamp: "2026-01-01T00:00:00Z",
		Entries:   []validationcache.FileEntry{entry},
	}); err != nil {
		t.Fatal(err)
	}

	// Edit only the Description section: check 1 re-runs, checks 2-8 would be
	// carried over, so the baseline's judgment state is required.
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte(strings.Replace(string(content), "Prose.", "Changed prose.", 1)), 0644); err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewDeltaScope(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.PlanError, "no structured judgment state") {
		t.Fatalf("expected the judgment-state refusal in the preview, got %q (rerun %v carried %v)", preview.PlanError, preview.Rerun, preview.Carried)
	}

	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "no structured judgment state") {
		t.Fatalf("expected the planner to refuse the same baseline, got %v", err)
	}
}

// TestDeltaPlanWithoutJudgmentsWhenNothingIsCarried verifies the boundary of
// that refusal: a re-run that covers every declared check carries nothing, so
// it needs no judgment state and must still plan.
func TestDeltaPlanWithoutJudgmentsWhenNothingIsCarried(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   "validate",
		Unit:      "auth",
		Mode:      "full",
		Basis:     "full",
		Result:    "pass",
		Target:    TargetCandidate,
		Timestamp: "2026-01-01T00:00:00Z",
		Entries:   []validationcache.FileEntry{entry},
	}); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte(strings.Replace(string(content), "Prose.", "Changed prose.", 1)), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("expected nothing carried over, got %v", run.CarriedKeys)
	}
}

// The semantic checklist owns rule checks; its headings and the generated
// coverage must agree without another independently maintained count.
func TestRuleChecklistMatchesPlannedCoverage(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "framework", "rule_validate_checklist.md"))
	if err != nil {
		t.Fatal(err)
	}
	var checklistKeys []string
	for _, match := range regexp.MustCompile(`(?m)^### Check ([0-9]+) — `).FindAllStringSubmatch(string(content), -1) {
		checklistKeys = append(checklistKeys, match[1])
	}
	if len(checklistKeys) == 0 {
		t.Fatal("rule checklist has no semantic check headings")
	}
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	run, err := Plan(repoRoot, GateValidate, TargetKindRule, "b_rule_http", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(coverageKeysOf(run), ","), strings.Join(checklistKeys, ","); got != want {
		t.Fatalf("planned rule coverage %s differs from semantic checklist %s", got, want)
	}
	if got, want := strings.Join(RuleValidateChecks(), ","), strings.Join(checklistKeys, ","); got != want {
		t.Fatalf("finalization check set %s differs from semantic checklist %s", got, want)
	}
}

// TestRulePlanIncludesDirectoryInputEvidence verifies that a rule validate plan
// exposes the files expanded from a directory --input to its session
// (framework/verification_scope.md §Input roles: --input evidence is readable
// and declarable by every session).
func TestRulePlanIncludesDirectoryInputEvidence(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeRule(t, repoRoot, "stable", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeUnit(t, repoRoot, "stable", "dep", "none", "none", "src", "")
	writeFile(t, repoRoot, "docs/evidence/note_a.md", "note a\n")
	writeFile(t, repoRoot, "docs/evidence/note_b.md", "note b\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindRule, "b_rule_http", TargetCandidate, ModeFull, []string{"docs/evidence"}, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 6 {
		t.Fatalf("expected the six rule check keys, got %v", coverageKeysOf(run))
	}
	spec, err := BuildSessionSpec(repoRoot, run, coverageKeysOf(run))
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]bool{}
	for _, ref := range spec.ReadRefs {
		refs[ref] = true
	}
	for _, want := range []string{"docs/evidence/note_a.md", "docs/evidence/note_b.md"} {
		if !refs[want] {
			t.Fatalf("expected directory input %q in the rule session read refs, got %v", want, spec.ReadRefs)
		}
	}
}

// TestRuleDeltaPlanIncludesDirectoryInputEvidence covers the delta/repair rule
// plan path: the re-run session must expose the same --input evidence.
func TestRuleDeltaPlanIncludesDirectoryInputEvidence(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeRule(t, repoRoot, "stable", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	ruleEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/rules/candidate/b_rule_http.md", []validationcache.CheckDeclaration{
		{Check: "1"}, {Check: "2"}, {Check: "3"}, {Check: "4"}, {Check: "5"}, {Check: "6"},
	})
	if err != nil {
		t.Fatal(err)
	}
	consumerEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "unit:auth", []validationcache.CheckDeclaration{
		{Check: "4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	judgments := JudgmentBaseline{SchemaVersion: 3, LogicalStatus: statuses, SynthesisDigest: "sha256:test"}
	for _, key := range []string{"1", "2", "3", "4", "5", "6"} {
		statuses[key] = "pass"
	}
	judgmentsData, err := json.Marshal(judgments)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "rule", "b_rule_http", validationcache.CacheWrite{
		Command:   "validate",
		Unit:      "b_rule_http",
		Mode:      "full",
		Basis:     "full",
		Result:    "pass",
		Target:    TargetCandidate,
		Timestamp: "2026-01-01T00:00:00Z",
		Judgments: string(judgmentsData),
		Entries:   []validationcache.FileEntry{ruleEntry, consumerEntry},
	}); err != nil {
		t.Fatal(err)
	}

	// The consumer unit changes: check 4 goes stale, the rule-body checks
	// stay fresh and are carried over.
	unitPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	content, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte(strings.Replace(string(content), "Prose.", "Changed prose.", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repoRoot, "docs/evidence/note.md", "note\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindRule, "b_rule_http", TargetCandidate, ModeDelta, []string{"docs/evidence"}, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "1,2,3,5,6" {
		t.Fatalf("expected the rule-body checks carried over, got %v", run.CarriedKeys)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "4" {
		t.Fatalf("expected the stale check keys as coverage, got %v", got)
	}
	spec, err := BuildSessionSpec(repoRoot, run, coverageKeysOf(run))
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(spec.ReadRefs, "docs/evidence/note.md") {
		t.Fatalf("expected the directory input evidence in the rule re-run session, got %v", spec.ReadRefs)
	}
}

// TestDeltaRepairQuotedStatusStillReruns pins the parser's single value form: a
// failure record that spells a check status as `" fail "` (quotes plus
// whitespace) is a valid `fail` for the closed-set validation, so the failed
// judgment must enter the repair re-run set — never the carried set.
func TestDeltaRepairQuotedStatusStillReruns(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Sections: []string{"Description"}, Status: status})
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "fail", true, "delta")

	// Rewrite the rendered failure record's check status into the
	// quoted-with-whitespace spelling an external writer could produce.
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	content, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(string(content), "status: fail", "status: \" fail \"", 1)
	if patched == string(content) {
		t.Fatal("expected one rendered `status: fail` line to patch")
	}
	if err := os.WriteFile(cachePath, []byte(patched), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "structural" {
		t.Fatalf("expected the failed group to re-run, got %s", got)
	}
	for _, key := range run.CarriedKeys {
		if key == "1" {
			t.Fatalf("a failed judgment must never be carried over, got %v", run.CarriedKeys)
		}
	}
	if notice := strings.Join(run.Notices, " "); strings.Contains(notice, "degrad") || strings.Contains(notice, "incomplete") {
		t.Fatalf("a canonical `fail` status must not degrade the plan, got %v", run.Notices)
	}
}

// TestSessionIDAndBatching pins the agent-chosen batching identity: a
// single-key session keeps the key as its id and a batch gets a stable derived
// id, and the read surface is the union of the batch's keys.
func TestSessionIDAndBatching(t *testing.T) {
	if got := SessionID([]string{"item:auth:auth.core"}); got != "item:auth:auth.core" {
		t.Fatalf("single-key session id = %q, want the key", got)
	}
	batch := SessionID([]string{"design", "structural"})
	if batch != SessionID([]string{"structural", "design"}) || !strings.HasPrefix(batch, "batch-") {
		t.Fatalf("batch session id must be order-independent and derived, got %q", batch)
	}

	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "none", "src", "")
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{"structural", "design"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.SessionID != batch {
		t.Fatalf("spec id = %q, want %q", spec.SessionID, batch)
	}
	if strings.Join(spec.CheckKeys, ",") != "1,3,6,2,4" {
		t.Fatalf("batched check keys = %v, want the union in coverage order", spec.CheckKeys)
	}
	if _, err := BuildSessionSpec(repoRoot, run, []string{"not-a-key"}); err == nil {
		t.Fatal("a key outside the coverage set must be rejected")
	}
}

// TestCoverageProgressClosure pins the mechanical coverage closure helper: an
// accepted session covers its keys; a duplicate cover is an error.
func TestCoverageProgressClosure(t *testing.T) {
	run := &Run{
		RunID: "20260917-123456-a0b1c2",
		Coverage: []CoverageKey{
			{Key: "structural", Kind: SessionKindChecks},
			{Key: "design", Kind: SessionKindChecks},
		},
	}
	states := []*SessionState{
		{SessionID: "structural", Keys: []string{"structural"}, Status: SessionAccepted},
	}
	covered, uncovered, err := CoverageProgress(run, states)
	if err != nil || covered["structural"] != "structural" || strings.Join(uncovered, ",") != "design" {
		t.Fatalf("progress = %v / %v / %v", covered, uncovered, err)
	}
	states = append(states, &SessionState{SessionID: "batch-x", Keys: []string{"structural"}, Status: SessionAccepted})
	if _, _, err := CoverageProgress(run, states); err == nil {
		t.Fatal("a key covered by two accepted sessions must be an error")
	}
}

func buildVerifyBaselineFixture(t *testing.T, root string, entries []validationcache.FileEntry, statuses map[string]string) ([]validationcache.FileEntry, JudgmentBaseline) {
	t.Helper()
	var extra []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, "tests/") {
			extra = append(extra, entry.Path)
		}
	}
	derivation, err := NewDerivation(root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := derivation.resolveRun(GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, extra, nil)
	if err != nil {
		t.Fatal(err)
	}
	run.RunID = "20260101-000000-abcdef"
	coverage, err := derivation.computeCoverage(run)
	if err != nil {
		t.Fatal(err)
	}
	state := JudgmentBaseline{SchemaVersion: 4, LogicalStatus: map[string]string{}, SynthesisDigest: "sha256:test", Records: map[string]judgments.Binding{}}
	var generated []validationcache.FileEntry
	for _, ck := range coverage {
		old := ck.File
		if ck.Kind == SessionKindItem {
			old = ck.Item
		}
		verdict := "acceptable"
		if ck.Kind == SessionKindCode {
			verdict = "facts"
		}
		if ck.Kind == SessionKindItem {
			verdict = "ALIGNED"
		}
		status := statuses[old]
		if status == "" {
			status = "pass"
		}
		state.LogicalStatus[ck.Key] = status
		result := SessionResult{Kind: ck.Kind, Verdicts: map[string]string{ck.Key: verdict}, EffectiveStatus: map[string]string{ck.Key: status}, ReportDigest: judgments.Digest([]byte("fixture"))}
		for _, p := range coverageReadRefs(root, run, ck) {
			decl := validationcache.CheckDeclaration{Check: ck.Key, Lens: ck.Lens, Status: status}
			scope := "all"
			if p == mainSpecRef(run) {
				if ck.Kind == SessionKindItem {
					decl.AcceptanceItemIDs = []string{ck.Item}
					scope = "acceptance_item:" + ck.Item
				} else {
					decl.Sections = []string{"Description"}
					scope = "Description"
				}
			}
			result.Scopes = append(result.Scopes, Scope{Key: ck.Key, Path: p, Declaration: scope})
			entry, err := validationcache.BuildEntryFromChecks(root, p, []validationcache.CheckDeclaration{decl})
			if err != nil {
				t.Fatal(err)
			}
			generated = append(generated, entry)
		}
		ref, err := SaveJudgment(root, run, ck, &result, "fixture", func() []judgments.Binding {
			if ck.Kind == SessionKindDesign {
				return []judgments.Binding{state.Records["code:"+ck.File]}
			}
			return nil
		}())
		if err != nil {
			t.Fatal(err)
		}
		state.Records[ck.Key] = judgments.Binding{Reference: ref, Layer: TargetCandidate, Source: "executed"}
	}
	return generated, state
}

func TestLoadDeferredFindingsRebindsQualityKeysAndPreservesSource(t *testing.T) {
	root := newRepo(t)
	entry := validationcache.DeferredEntry{
		FindingID: "source-run/architecture:source/F1", OwnerUnit: "owner",
		SourceUnit: "source", SourceRun: "source-run", Severity: "P1",
		Text: "shared boundary violation", Detail: "full finding detail",
		SourceKey:    "architecture:source",
		AffectedKeys: []string{"architecture:source", "design:source:shared.go", "code:shared.go", "relationship:shared", "design:source-extra:shared.go", "code:design:source:shared.go"},
		EvidencePath: "shared.go", Reason: "owner is responsible",
	}
	if err := validationcache.WriteDeferredLedger(root, validationcache.DeferredLedger{SchemaVersion: 1, Entries: []validationcache.DeferredEntry{entry}}); err != nil {
		t.Fatal(err)
	}
	findings, err := loadDeferredFindings(root, "owner")
	if err != nil || len(findings) != 1 {
		t.Fatalf("load deferral: %+v %v", findings, err)
	}
	got := findings[0]
	if got.SourceUnit != entry.SourceUnit || got.SourceRun != entry.SourceRun || got.Finding.ID != entry.FindingID || got.Finding.SourceKey != "architecture:owner" {
		t.Fatalf("owner key or provenance lost: %+v", got)
	}
	want := "architecture:owner,design:owner:shared.go,code:shared.go,relationship:shared,design:source-extra:shared.go,code:design:source:shared.go"
	if strings.Join(got.Finding.AffectedKeys, ",") != want {
		t.Fatalf("affected keys = %v", got.Finding.AffectedKeys)
	}
	ledger, err := validationcache.ReadDeferredLedger(root)
	if err != nil || len(ledger.Entries) != 1 || ledger.Entries[0].SourceKey != entry.SourceKey || strings.Join(ledger.Entries[0].AffectedKeys, ",") != strings.Join(entry.AffectedKeys, ",") {
		t.Fatalf("source ledger was rewritten: %+v %v", ledger, err)
	}
}

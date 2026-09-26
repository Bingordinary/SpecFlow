package gaterun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	judgments, err := json.Marshal(JudgmentBaseline{SchemaVersion: 2, LogicalStatus: statuses, SynthesisDigest: "sha256:test"})
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

// mainCheckDecls declares every validate check on the main spec.
func mainCheckDecls() []validationcache.CheckDeclaration {
	var checks []validationcache.CheckDeclaration
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
		checks = append(checks, validationcache.CheckDeclaration{Check: key, Sections: []string{"Description"}})
	}
	return checks
}

// packetIDsOf lists a run's packet ids in plan order.
func packetIDsOf(run *Run) []string {
	var ids []string
	for _, p := range run.Packets {
		ids = append(ids, p.PacketID)
	}
	return ids
}

func TestUnitValidatePacketReadRefsMatchOwnedChecks(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src/dep", "")
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "b_rule_http", "src/auth", "")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, packetID := range []string{"structural", "dependencies"} {
		packet := run.PacketByID(packetID)
		if packet == nil {
			t.Fatalf("missing %s packet", packetID)
		}
		for _, ref := range []string{"unit:dep", "rule:b_rule_http"} {
			if !stringInSlice(packet.ReadRefs, ref) {
				t.Fatalf("%s packet read refs = %v, want %s", packetID, packet.ReadRefs, ref)
			}
		}
	}
	for _, packetID := range []string{"design", "acceptance"} {
		packet := run.PacketByID(packetID)
		if packet == nil {
			t.Fatalf("missing %s packet", packetID)
		}
		for _, ref := range []string{"unit:dep", "rule:b_rule_http"} {
			if stringInSlice(packet.ReadRefs, ref) {
				t.Fatalf("%s packet received unrelated logical ref %s: %v", packetID, ref, packet.ReadRefs)
			}
		}
	}
}

func TestUnitValidateStructuralPacketCarriesUnresolvedReference(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "missing", "none", "src/auth", "")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	structural := run.PacketByID("structural")
	if structural == nil || !stringInSlice(structural.ReadRefs, "unit:missing") {
		t.Fatalf("structural packet must expose the unresolved Check 1 reference, got %+v", structural)
	}
}

func TestUnitValidateDeltaStructuralPacketCarriesLogicalReferences(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src/dep", "")
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "b_rule_http", "src/auth", "")

	mainEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{mainEntry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, []string{"1"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	structural := run.PacketByID("structural")
	if structural == nil {
		t.Fatal("expected the structural packet in the delta plan")
	}
	for _, ref := range []string{"unit:dep", "rule:b_rule_http"} {
		if !stringInSlice(structural.ReadRefs, ref) {
			t.Fatalf("delta structural packet read refs = %v, want %s", structural.ReadRefs, ref)
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
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
		statuses[key] = "pass"
	}
	judgments, err := json.Marshal(JudgmentBaseline{
		SchemaVersion: 2,
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

func TestDerivedPlanMapsUnclaimedDependencyUnit(t *testing.T) {
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
	// declared the unit:dep entry, so the plan must map it to the packet
	// owning Check 7.
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "extra: changed\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(packetIDsOf(run), ",")
	if got != "dependencies,cross" {
		t.Fatalf("expected the dependencies packet + cross, got %s", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "1,2,3,4,5,6" {
		t.Fatalf("expected checks 1-6 carried over, got %v", run.CarriedKeys)
	}
	if len(run.Notices) == 0 {
		t.Fatal("expected a scope notice")
	}
}

func TestDerivedPlanRerunForcesGroup(t *testing.T) {
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
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, []string{"2"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(packetIDsOf(run), ",")
	if got != "design,cross" {
		t.Fatalf("expected the design packet + cross, got %s", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "1,3,5,6,7,8" {
		t.Fatalf("expected the other checks carried over, got %v", run.CarriedKeys)
	}
}

func TestDerivedPlanDegradesWithoutEvidence(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	// A pass cache without per-check evidence cannot be derived from.
	entry, err := validationcache.BuildEntry(repoRoot, validationcache.EntryDeclaration{Path: "docs/specs/units/candidate/unit_auth.md"})
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 || len(run.CarriedKeys) != 0 {
		t.Fatalf("expected a degraded full plan, got %v / carried %v", packetIDsOf(run), run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "no per-check evidence") {
		t.Fatalf("expected a degradation notice, got %v", run.Notices)
	}
}

func TestDerivedPlanRepairWithoutStatusMapDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "fail", true, "delta")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected a degraded full plan, got %v", packetIDsOf(run))
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "incomplete per-check status map") {
		t.Fatalf("expected a degradation notice, got %v", run.Notices)
	}
}

// TestDerivedPlanDeltaRequiresPassBaseline verifies that a delta plan refuses
// a baseline that is not a pass cache and degrades a status-less failure record
// to the full plan.
func TestDerivedPlanDeltaRequiresPassBaseline(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "no baseline cache") {
		t.Fatalf("expected a missing-baseline error, got %v", err)
	}

	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "fail", true, "delta")
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "failure record") {
		t.Fatalf("expected the failure-record guidance, got %v", err)
	}
	// Repair on a status-less record degrades to the full plan.
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 || !strings.Contains(strings.Join(run.Notices, " "), "incomplete per-check status map") {
		t.Fatalf("expected the degraded repair plan, got %v / %v", packetIDsOf(run), run.Notices)
	}
}

// TestDerivedPlanReviewDeltaRejectsConflictingBaseline verifies that a review
// baseline whose result and blocking declarations disagree is rejected instead
// of being accepted as a pass baseline.
func TestDerivedPlanReviewDeltaRejectsConflictingBaseline(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckDecls())
	if err != nil {
		t.Fatal(err)
	}

	writeGateBaseline(t, repoRoot, "review", "fail", "full", false, []validationcache.FileEntry{entry}, map[string]string{"1": "fail"})
	if _, err := Plan(repoRoot, GateReview, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "declarations conflict") {
		t.Fatalf("expected a fail/non-blocking review baseline to be rejected, got %v", err)
	}

	writeGateBaseline(t, repoRoot, "review", "pass", "full", true, []validationcache.FileEntry{entry}, map[string]string{"1": "pass"})
	if _, err := Plan(repoRoot, GateReview, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "declarations conflict") {
		t.Fatalf("expected a pass/blocking review baseline to be rejected, got %v", err)
	}
}

// writeUnitItemsSpec writes a unit spec with the given acceptance item ids.
// The items declare implementation_surface src, so the helper also creates a
// real file there — verify/review planning rejects a surface that does not
// resolve.
func writeUnitItemsSpec(t *testing.T, repoRoot string, itemIDs ...string) string {
	t.Helper()
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_auth.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n"
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
	judgments, err := json.Marshal(JudgmentBaseline{SchemaVersion: 2, LogicalStatus: statuses, SynthesisDigest: "sha256:test"})
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

// TestDerivedPlanVerifyPlansNewItems verifies that a delta verify plan
// re-runs acceptance items the baseline never declared: a new item has no
// evidence to carry over, so it must execute like a stale judgment.
func TestDerivedPlanVerifyPlansNewItems(t *testing.T) {
	repoRoot := newRepo(t)
	specPath := writeUnitItemsSpec(t, repoRoot, "auth.core", "auth.aux")

	decls := []validationcache.CheckDeclaration{
		{Check: "auth.core", AcceptanceItemIDs: []string{"auth.core"}},
		{Check: "auth.aux", AcceptanceItemIDs: []string{"auth.aux"}},
		{Check: CrossKey, Sections: []string{"Description"}},
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "verify", "pass", "full", false, []validationcache.FileEntry{entry},
		map[string]string{"auth.core": "pass", "auth.aux": "pass", CrossKey: "pass"})

	// Add a new acceptance item after the baseline was written.
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("  - id: auth.new\n    description: New.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n")...)
	if err := os.WriteFile(specPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(packetIDsOf(run), ","); got != "detect:auth.new,analysis:auth.new,cross" {
		t.Fatalf("expected re-run packets for the new item plus cross, got %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "auth.aux,auth.core" {
		t.Fatalf("expected the declared items carried over, got %v", run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "new checks not in the baseline: auth.new") {
		t.Fatalf("expected the new-key disclosure, got %v", run.Notices)
	}
}

// TestDerivedPlanDeltaCrossOnlyPlansCrossPacket verifies that a delta whose
// only stale declaration belongs to the cross-check plans the single cross
// packet and carries every declared check over.
func TestDerivedPlanDeltaCrossOnlyPlansCrossPacket(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "\n## Scope\n\nIn scope.\n")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Sections: []string{"Description"}})
		statuses[key] = "pass"
	}
	decls = append(decls, validationcache.CheckDeclaration{Check: CrossKey, Sections: []string{"Scope"}})
	statuses[CrossKey] = "pass"
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	// Edit only the Scope section: the cross declaration goes stale, every
	// check declaration stays fresh.
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte(strings.Replace(string(content), "In scope.", "In scope, edited.", 1)), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(packetIDsOf(run), ","); got != CrossKey {
		t.Fatalf("expected a cross-only plan, got %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "1,2,3,4,5,6,7,8" {
		t.Fatalf("expected every declared check carried over, got %v", run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "re-run checks cross") {
		t.Fatalf("expected the cross-only re-run disclosure, got %v", run.Notices)
	}
}

// TestDerivedPlanRepairPartialStatusMapDegrades verifies the fail-closed
// repair path: a failure record that declares a status for only some checks
// must not carry the status-less ones over.
func TestDerivedPlanRepairPartialStatusMapDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected a degraded full plan, got %v", packetIDsOf(run))
	}
	notice := strings.Join(run.Notices, " ")
	if !strings.Contains(notice, "incomplete per-check status map") || !strings.Contains(notice, "missing: 2, 3, 4, 5, 6, 7, 8, cross") {
		t.Fatalf("expected the incomplete-status degradation with the missing checks, got %v", run.Notices)
	}
}

// TestDerivedPlanRepairInvalidStatusValueDegrades verifies the fail-closed
// repair path: a status value outside pass/fail/carried cannot say which
// judgments failed, so nothing may be carried over.
func TestDerivedPlanRepairInvalidStatusValueDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected a degraded full plan, got %v", packetIDsOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "invalid per-check status map") || !strings.Contains(notice, "3=bogus") {
		t.Fatalf("expected the invalid-status degradation, got %v", run.Notices)
	}
}

// TestDerivedPlanRepairCarriedInFullRunRecordDegrades verifies that a
// full-run failure record (`basis: full`) declaring `carried` is malformed
// state: a full run re-executed every judgment, so the entry cannot be
// trusted and the plan degrades to the full packet set.
func TestDerivedPlanRepairCarriedInFullRunRecordDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected a degraded full plan, got %v", packetIDsOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "invalid per-check status map") || !strings.Contains(notice, "3=carried") {
		t.Fatalf("expected the illegal-carried degradation, got %v", run.Notices)
	}
}

// TestDerivedPlanRepairStatusMapJudgmentMismatchDegrades verifies that a
// status map and the record's structured judgment baseline must agree on the
// key set: a disagreement cannot say which judgments failed.
func TestDerivedPlanRepairStatusMapJudgmentMismatchDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: "pass", Sections: []string{"Description"}})
		statuses[key] = "pass"
	}
	delete(statuses, "3")
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "fail", "delta", true, []validationcache.FileEntry{entry}, statuses)

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected a degraded full plan, got %v", packetIDsOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "does not match its judgment baseline") || !strings.Contains(notice, "unmatched: 3") {
		t.Fatalf("expected the key-set mismatch degradation, got %v", run.Notices)
	}
}

// TestDerivedPlanRepairCompleteStatusMapCarriesPassingGroups verifies the
// positive repair path: a complete, valid status map keeps the incremental
// plan and carries the passing groups over.
func TestDerivedPlanRepairCompleteStatusMapCarriesPassingGroups(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(packetIDsOf(run), ","); got != "structural,cross" {
		t.Fatalf("expected the failed group plus cross to re-run, got %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "2,4,5,7,8" {
		t.Fatalf("expected the passing groups carried over, got %v", run.CarriedKeys)
	}
	for _, notice := range run.Notices {
		if strings.Contains(notice, "degradation") || strings.Contains(notice, "full scope") {
			t.Fatalf("a complete status map must not degrade, got %v", run.Notices)
		}
	}
}

// TestDerivedPlanRepairUsesPersistedTargetedInvalidation is the cross-session
// regression: no caller supplies --rerun after the targeted finding. The
// repair plan must read the persisted invalidation and exclude that judgment
// from carry-over.
func TestDerivedPlanRepairUsesPersistedTargetedInvalidation(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
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
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(packetIDsOf(run), ","); got != "structural,design,cross" {
		t.Fatalf("persisted check 2 must add its design packet, got %s", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "5,7,8" {
		t.Fatalf("invalidated judgment must not be carried, got %v", run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "persisted targeted invalidations: 2") {
		t.Fatalf("plan did not disclose the persisted invalidation: %v", run.Notices)
	}
}

func TestDerivedPlanRepairUnknownPersistedInvalidationDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 || len(run.CarriedKeys) != 0 {
		t.Fatalf("unknown invalidation must degrade to full scope, got %v / carried %v", packetIDsOf(run), run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), `re-run judgment "removed-check" is not in the spec surface`) {
		t.Fatalf("missing unknown-invalidation degradation notice: %v", run.Notices)
	}
}

func TestInvalidateTargetedMarksMatchingOpenRunOnly(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")

	validateRun, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	verifyRun, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now().Add(time.Second))
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

// TestDerivedPlanDeltaCrossOnlyBaselineReportsFullScope verifies that a
// baseline declaring only the cross-check is reported as a full-scope re-run:
// nothing is carried over, so the re-run covers every declared check.
func TestDerivedPlanDeltaCrossOnlyBaselineReportsFullScope(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "\n## Scope\n\nIn scope.\n")

	entry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_auth.md",
		[]validationcache.CheckDeclaration{{Check: CrossKey, Sections: []string{"Scope"}}})
	if err != nil {
		t.Fatal(err)
	}
	writeGateBaseline(t, repoRoot, "validate", "pass", "full", false, []validationcache.FileEntry{entry}, map[string]string{})

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("a cross-only baseline carries nothing over, got %v", run.CarriedKeys)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected the full packet set, got %v", packetIDsOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "the re-run covers every declared check — the plan covers the full scope") {
		t.Fatalf("expected the full-scope coverage statement, got %v", run.Notices)
	}
}

// TestDerivedPlanDeltaDegradesOnUntrackableEntry verifies that a baseline
// entry with no dependency chunks over a file with content degrades the plan:
// no check can attribute its staleness and the freshness chain can never see
// it fresh.
func TestDerivedPlanDeltaDegradesOnUntrackableEntry(t *testing.T) {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected a degraded full plan, got %v", packetIDsOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "cannot attribute (no dependency chunks)") || !strings.Contains(notice, "src/extra.go") {
		t.Fatalf("expected the untrackable-entry degradation, got %v", run.Notices)
	}
}

// TestDerivedPlanDeltaDegradesWhenMainFileMissingEvenWithReRuns verifies that
// a baseline files list without the target's main file degrades the plan even
// when the per-check derivation found a re-run reason: the finalize
// main-file check would reject the partial run.
func TestDerivedPlanDeltaDegradesWhenMainFileMissingEvenWithReRuns(t *testing.T) {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 5 {
		t.Fatalf("expected a degraded full plan, got %v", packetIDsOf(run))
	}
	if notice := strings.Join(run.Notices, " "); !strings.Contains(notice, "does not include the main file") || !strings.Contains(notice, "unit_auth.md") {
		t.Fatalf("expected the missing-main-file degradation, got %v", run.Notices)
	}
}

// TestPlanRejectsReservedCrossItemID verifies that an acceptance item named
// with the reserved cross-check key is rejected before any run state is
// written (reserved-id collision).
func TestPlanRejectsReservedCrossItemID(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "cross", "auth.core")

	_, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), `reserved check key "cross"`) {
		t.Fatalf("expected the reserved-key rejection, got %v", err)
	}
}

// TestDerivedPlanStableMissingBaselineMessage verifies that a stable-only
// target with no usable confirmation baseline gets the documented message
// (full confirmation run or fork) in both delta and repair modes.
func TestDerivedPlanStableMissingBaselineMessage(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "stable", "auth", "none", "none", "src", "")

	for _, mode := range []string{ModeDelta, ModeRepair} {
		_, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetStable, mode, nil, nil, time.Now())
		if err == nil {
			t.Fatalf("%s: expected the no-baseline error", mode)
		}
		if !strings.Contains(err.Error(), "No usable confirmation baseline") || !strings.Contains(err.Error(), "fork --unit auth") {
			t.Fatalf("%s: expected the documented stable-only message, got %v", mode, err)
		}
	}
}

// TestPreviewDeltaScopeFailureRecordUsesRepair verifies that the fresh
// report's delta scope preview derives the repair plan for a failure-record
// baseline — the recovery mode gate-plan requires — so the report and the
// planner never disagree (see framework/verification_scope.md §Delta Runs).
func TestPreviewDeltaScopeFailureRecordUsesRepair(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Status: "pass", Sections: []string{"Description"}})
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
	if got := strings.Join(preview.Rerun, ","); got != CrossKey {
		t.Fatalf("expected the cross packet re-run, got %s", got)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(preview.Carried, ","); got != strings.Join(run.CarriedKeys, ",") {
		t.Fatalf("preview carried %v, plan carried %v", preview.Carried, run.CarriedKeys)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "1,2,3,4,5,6,7,8" {
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
	if got := strings.Join(preview.Rerun, ","); got != "1,2,3,4,5,6,7,8,cross" {
		t.Fatalf("expected every check plus cross re-run, got %s", got)
	}
	if len(preview.Carried) != 0 {
		t.Fatalf("expected nothing carried over, got %v", preview.Carried)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
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
	for _, key := range []string{"2", "3", "4", "5", "6", "7", "8"} {
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Sections: []string{"Scope"}})
	}
	decls = append(decls, validationcache.CheckDeclaration{Check: CrossKey, Sections: []string{"Scope"}})
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

	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "no structured judgment state") {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("expected nothing carried over, got %v", run.CarriedKeys)
	}
}

// TestRulePlanIncludesDirectoryInputEvidence verifies that a rule validate
// plan exposes the files expanded from a directory --input to its packet
// (framework/verification_scope.md §Input roles: --input evidence is readable
// and declarable by every packet).
func TestRulePlanIncludesDirectoryInputEvidence(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeRule(t, repoRoot, "stable", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeUnit(t, repoRoot, "stable", "dep", "none", "none", "src", "")
	writeFile(t, repoRoot, "docs/evidence/note_a.md", "note a\n")
	writeFile(t, repoRoot, "docs/evidence/note_b.md", "note b\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindRule, "b_rule_http", TargetCandidate, ModeFull, []string{"docs/evidence"}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 1 {
		t.Fatalf("expected the single rule checks packet, got %v", packetIDsOf(run))
	}
	refs := map[string]bool{}
	for _, ref := range run.Packets[0].ReadRefs {
		refs[ref] = true
	}
	for _, want := range []string{"docs/evidence/note_a.md", "docs/evidence/note_b.md"} {
		if !refs[want] {
			t.Fatalf("expected directory input %q in the rule packet read refs, got %v", want, run.Packets[0].ReadRefs)
		}
	}
}

// TestRuleDeltaPlanIncludesDirectoryInputEvidence covers the delta/repair
// rule plan path: the re-run packet must expose the same --input evidence.
func TestRuleDeltaPlanIncludesDirectoryInputEvidence(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeRule(t, repoRoot, "stable", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	ruleEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/rules/candidate/b_rule_http.md", []validationcache.CheckDeclaration{
		{Check: "1"}, {Check: "2"}, {Check: "3"}, {Check: "4"}, {Check: "5"}, {Check: "6"}, {Check: "7"}, {Check: "8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	consumerEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "unit:auth", []validationcache.CheckDeclaration{
		{Check: "5"}, {Check: "7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	judgments := JudgmentBaseline{SchemaVersion: 2, LogicalStatus: statuses, SynthesisDigest: "sha256:test"}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
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

	// The consumer unit changes: checks 5/7 go stale, the rule-body checks
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

	run, err := Plan(repoRoot, GateValidate, TargetKindRule, "b_rule_http", TargetCandidate, ModeDelta, []string{"docs/evidence"}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "1,2,3,4,6,8" {
		t.Fatalf("expected the rule-body checks carried over, got %v", run.CarriedKeys)
	}
	if len(run.Packets) != 1 {
		t.Fatalf("expected the single rule checks packet, got %v", packetIDsOf(run))
	}
	found := false
	for _, ref := range run.Packets[0].ReadRefs {
		if ref == "docs/evidence/note.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the directory input evidence in the rule re-run packet, got %v", run.Packets[0].ReadRefs)
	}
}

// TestDerivedPlanRepairQuotedStatusStillReruns pins the parser's single value
// form: a failure record that spells a check status as `" fail "` (quotes plus
// whitespace) is a valid `fail` for the closed-set validation, so the failed
// judgment must enter the repair re-run set — never the carried set.
// Without the canonicalizing parser, the trimmed validator accepted the value
// while the raw union comparison missed it (see framework/verification_scope.md
// §Delta Runs → Failure recovery).
func TestDerivedPlanRepairQuotedStatusStillReruns(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckDeclaration
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", CrossKey} {
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

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(packetIDsOf(run), ","); got != "structural,cross" {
		t.Fatalf("expected the failed check's group plus cross to re-run, got %s", got)
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

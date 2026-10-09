package gaterun

import (
	"encoding/json"
	"errors"
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

// mainCheckMarkers lists every validate check as a cache check marker.
func mainCheckMarkers() []validationcache.CheckEntry {
	var checks []validationcache.CheckEntry
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		checks = append(checks, validationcache.CheckEntry{Check: key})
	}
	return checks
}

// evidenceEntryWithChecks builds one evidence entry for path with the given
// check markers attached.
func evidenceEntryWithChecks(t *testing.T, root, path string, checks []validationcache.CheckEntry) validationcache.FileEntry {
	t.Helper()
	entry, err := validationcache.BuildEvidenceEntry(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil {
		t.Fatalf("no evidence entry resolves for %s", path)
	}
	entry.Checks = append(entry.Checks, checks...)
	return *entry
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

// TestUnitValidateExcludesCodeSurface pins issue #69's boundary: a unit validate
// run's input surface and every session read ref carry no implementation file.
// implementation_surface, affects.files, and affects.evidence_files are verify
// inputs — validate judges only their declarations, never their contents.
func TestUnitValidateExcludesCodeSurface(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src/auth",
		"    affects:\n      files:\n        - src/extra.go\n      evidence_files:\n        - peer/peer_test.go\n")
	writeFile(t, repoRoot, "src/auth/impl.go", "package auth\n")
	writeFile(t, repoRoot, "src/extra.go", "package auth\n")
	writeFile(t, repoRoot, "peer/peer_test.go", "package peer\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"src/auth/impl.go", "src/extra.go", "peer/peer_test.go"} {
		if _, ok := refByRef(run, code); ok {
			t.Fatalf("validate input surface carries code file %q: %+v", code, run.Refs)
		}
		if _, ok := surfaceByPath(run, code); ok {
			t.Fatalf("validate run carries a code surface entry %q", code)
		}
	}
	for i := range run.Coverage {
		ck := run.Coverage[i]
		if ck.Kind != SessionKindChecks {
			continue
		}
		reads := coverageReadRefs(repoRoot, run, ck)
		for _, ref := range reads {
			if !isLogicalRef(ref) && strings.HasSuffix(ref, ".go") {
				t.Fatalf("%s validate group read refs carry a code file %q: %v", ck.Key, ref, reads)
			}
		}
	}
}

// TestUnitValidateExcludesUnrelatedPeers pins the one-way dependency direction:
// a unit validate reads its declared unit_refs providers and rule_refs, never
// unrelated peers. Adding an unrelated unit in either layer must not change the
// target's input surface or its session read refs (issue #68).
func TestUnitValidateExcludesUnrelatedPeers(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src/dep", "")
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "b_rule_http", "src/auth", "")

	plan := func() *Run {
		t.Helper()
		run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return run
	}

	before := plan()
	writeUnit(t, repoRoot, "candidate", "peer_candidate", "none", "none", "src/peer_candidate", "")
	writeUnit(t, repoRoot, "stable", "peer_stable", "none", "none", "src/peer_stable", "")
	after := plan()

	for _, ref := range []string{"unit:peer_candidate", "unit:peer_stable"} {
		if _, ok := refByRef(after, ref); ok {
			t.Fatalf("unit validate read surface carries unrelated peer %s: %+v", ref, after.Refs)
		}
	}
	if len(before.Refs) != len(after.Refs) {
		t.Fatalf("adding unrelated peers changed the read surface: before=%+v after=%+v", before.Refs, after.Refs)
	}
	for i := range before.Refs {
		if before.Refs[i] != after.Refs[i] {
			t.Fatalf("adding unrelated peers changed read surface entry %d: before=%+v after=%+v", i, before.Refs, after.Refs)
		}
	}
	for _, key := range []string{"structural", "dependencies"} {
		spec, err := BuildSessionSpec(repoRoot, after, []string{key})
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range []string{"unit:dep", "rule:b_rule_http"} {
			if !stringInSlice(spec.ReadRefs, ref) {
				t.Fatalf("%s session read refs = %v, want %s", key, spec.ReadRefs, ref)
			}
		}
		for _, ref := range spec.ReadRefs {
			if strings.HasPrefix(ref, "unit:peer_") {
				t.Fatalf("%s session received unrelated peer ref %s: %v", key, ref, spec.ReadRefs)
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

func TestLoadCarriedResultsAssociatesCrossFindingWithAffectedKey(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
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
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
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

func TestRepairWithoutStatusMapDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
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

	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
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
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())

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

// TestBuildSessionSpecCoBatchesCodeAndDesign verifies the code+design pairing:
// one session may hold a file's code key and its unit's design key — it is
// self-contained (no DependsOn for the paired file) and carries the combined
// instruction context — while every other kind mix stays rejected.
func TestBuildSessionSpecCoBatchesCodeAndDesign(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "auth.core")

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{"code:src/main.go", "design:auth:src/main.go"})
	if err != nil {
		t.Fatalf("code+design pairing rejected: %v", err)
	}
	if spec.Kind != SessionKindDesign {
		t.Fatalf("paired session kind = %q, want %q", spec.Kind, SessionKindDesign)
	}
	if len(spec.DependsOn) != 0 {
		t.Fatalf("paired session depends on %v — it must be self-contained", spec.DependsOn)
	}
	combined := false
	for _, c := range spec.Context {
		if strings.Contains(c, "Co-batched public+design session") {
			combined = true
		}
	}
	if !combined {
		t.Fatalf("paired session lacks the combined context, got %v", spec.Context)
	}
	// An unpaired design key in the same batch keeps its dependency.
	spec, err = BuildSessionSpec(repoRoot, run, []string{"design:auth:src/main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.DependsOn) == 0 {
		t.Fatal("unpaired design session must still depend on its code session")
	}
	// Architecture keys cannot join a pair.
	if _, err := BuildSessionSpec(repoRoot, run, []string{"code:src/main.go", "architecture:auth"}); err == nil || !strings.Contains(err.Error(), "pair") {
		t.Fatalf("expected code+architecture mix to be rejected, got %v", err)
	}
}

// TestDeltaRepairPartialStatusMapDegrades verifies the fail-closed repair path:
// a failure record that declares a status for only some checks must not carry
// the status-less ones over.
func TestDeltaRepairPartialStatusMapDegrades(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckEntry
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		decl := validationcache.CheckEntry{Check: key}
		if key == "1" {
			decl.Status = "fail"
		}
		decls = append(decls, decl)
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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

	var decls []validationcache.CheckEntry
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "3" {
			status = "bogus"
		}
		decls = append(decls, validationcache.CheckEntry{Check: key, Status: status})
		statuses[key] = status
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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

	var decls []validationcache.CheckEntry
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "3" {
			status = "carried"
		}
		decls = append(decls, validationcache.CheckEntry{Check: key, Status: status})
		statuses[key] = status
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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

	var decls []validationcache.CheckEntry
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		decls = append(decls, validationcache.CheckEntry{Check: key, Status: "pass"})
		statuses[key] = "pass"
	}
	delete(statuses, "3")
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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

	var decls []validationcache.CheckEntry
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckEntry{Check: key, Status: status})
		statuses[key] = status
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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

	var decls []validationcache.CheckEntry
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckEntry{Check: key, Status: status})
		statuses[key] = status
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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

	var decls []validationcache.CheckEntry
	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckEntry{Check: key, Status: status})
		statuses[key] = status
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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
	if _, err := Load(repoRoot, validateRun.RunID); err == nil || !os.IsNotExist(errors.Unwrap(err)) {
		t.Fatalf("the invalidated run state must be removed, got %v", err)
	}
	loadedVerify, err := Load(repoRoot, verifyRun.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedVerify.Status != StatusOpen {
		t.Fatalf("unrelated run status = %q", loadedVerify.Status)
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

// TestDeltaPlanRefusesBaselineWithoutJudgments verifies that the derivation
// refuses a partial run that must carry judgments when the baseline cache
// has no structured judgment state.
func TestDeltaPlanRefusesBaselineWithoutJudgments(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "\n## Scope\n\nIn scope.\n")

	var decls []validationcache.CheckEntry
	decls = append(decls, validationcache.CheckEntry{Check: "1"})
	for _, key := range []string{"2", "3", "4", "5", "6", "7", "8", "9"} {
		decls = append(decls, validationcache.CheckEntry{Check: key})
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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

	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "no structured judgment state") {
		t.Fatalf("expected the planner to refuse the baseline, got %v", err)
	}
}

// TestRuleChecklistMatchesPlannedCoverage verifies the semantic checklist's
// rule-check headings and the generated coverage agree without another
// independently maintained count.
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
// exposes the files expanded from a directory input to its session
// (framework/verification_scope.md §Input roles: manifest evidence is readable
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

// TestDeltaRepairQuotedStatusStillReruns pins the parser's single value form: a
// failure record that spells a check status as `" fail "` (quotes plus
// whitespace) is a valid `fail` for the closed-set validation, so the failed
// judgment must enter the repair re-run set — never the carried set.
func TestDeltaRepairQuotedStatusStillReruns(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	var decls []validationcache.CheckEntry
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", ClarityCheck} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		decls = append(decls, validationcache.CheckEntry{Check: key, Status: status})
	}
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", decls)
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
			generated = append(generated, evidenceEntryWithChecks(t, root, p, []validationcache.CheckEntry{{Check: ck.Key, Lens: ck.Lens, Status: status}}))
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

// TestVerifyEvidenceFilesAreReadOnlyEvidence pins the affects.evidence_files
// contract: a cited evidence file joins the run snapshot and the item
// session's read refs, but it never becomes a quality coverage key
// (code:/design:) and never enters the code surface.
func TestVerifyEvidenceFilesAreReadOnlyEvidence(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	evidence := "peer/peer_test.go"
	writeFile(t, repoRoot, evidence, "package peer\n")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src",
		"    affects:\n      evidence_files:\n        - "+evidence+"\n")

	d, err := NewDerivation(repoRoot)
	run, err := d.resolveRun(GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := d.computeCoverage(run)
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := refByRef(run, evidence)
	if !ok || ref.Source != SourceDerivedAffects {
		t.Fatalf("evidence file is not a derived read-only ref: %+v (found=%v)", ref, ok)
	}
	var item *CoverageKey
	for i := range coverage {
		ck := &coverage[i]
		if ck.Key == "code:"+evidence || ck.Key == "design:auth:"+evidence {
			t.Fatalf("evidence file became a quality coverage key: %s", ck.Key)
		}
		if ck.Key == "item:auth:auth.core" {
			item = ck
		}
	}
	if _, ok := surfaceByPath(run, evidence); ok {
		t.Fatal("evidence file entered the code surface")
	}
	if item == nil {
		t.Fatal("missing item coverage key")
	}
	reads := coverageReadRefs(repoRoot, run, *item)
	found := false
	for _, p := range reads {
		if p == evidence {
			found = true
		}
	}
	if !found {
		t.Fatalf("item session cannot read its cited evidence: %v", reads)
	}
}

// TestVerifyMissingEvidenceFileFailsPlanning pins the fail-closed rule: an
// affects.evidence_files entry that does not resolve rejects the verify plan
// before any run state is written.
func TestVerifyMissingEvidenceFileFailsPlanning(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src",
		"    affects:\n      evidence_files:\n        - peer/missing_test.go\n")

	d, err := NewDerivation(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.resolveRun(GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil); err == nil {
		t.Fatal("missing evidence file did not fail verify planning")
	}
}

// TestExtendAddsInputsInPlaceKeepsCoverage pins the gate-extend contract: an
// open run gains supplementary evidence in place (same run id, same coverage
// set), the addition is persisted, and duplicate input is refused.
func TestExtendAddsInputsInPlaceKeepsCoverage(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	writeFile(t, repoRoot, "extra/helper_test.go", "package extra\n")
	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	before := strings.Join(coverageKeysOf(run), ",")
	extended, err := Extend(repoRoot, run.RunID, []string{"extra/helper_test.go"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if extended.RunID != run.RunID {
		t.Fatalf("extension replaced the run: %s -> %s", run.RunID, extended.RunID)
	}
	if after := strings.Join(coverageKeysOf(extended), ","); after != before {
		t.Fatalf("extension changed the coverage set: %s -> %s", before, after)
	}
	ref, ok := refByRef(extended, "extra/helper_test.go")
	if !ok || ref.Source != SourceInput {
		t.Fatalf("extended input is not a SourceInput ref: %+v (found=%v)", ref, ok)
	}
	if !stringInSlice(extended.ExtraInputs, "extra/helper_test.go") {
		t.Fatalf("extended input missing from ExtraInputs: %v", extended.ExtraInputs)
	}
	if len(extended.Notices) == 0 || !strings.Contains(extended.Notices[len(extended.Notices)-1], "evidence extended") {
		t.Fatalf("extension notice missing: %v", extended.Notices)
	}
	reloaded, err := Load(repoRoot, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := refByRef(reloaded, "extra/helper_test.go"); !ok {
		t.Fatal("extended input was not persisted")
	}
	if _, err := Extend(repoRoot, run.RunID, []string{"extra/helper_test.go"}, time.Now()); err == nil || !strings.Contains(err.Error(), "no new inputs") {
		t.Fatalf("duplicate extension not refused: %v", err)
	}
}

// TestExtendRefreshesUncoveredSessionReadRefs pins the recovery contract: a
// supplement must reach the read refs of every session that re-missions over
// it — including a public code key, whose read surface is materialized at
// plan time and must be refreshed, not left at its plan-time value.
func TestExtendRefreshesUncoveredSessionReadRefs(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	writeFile(t, repoRoot, "extra/helper.go", "package extra\n")
	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	extended, err := Extend(repoRoot, run.RunID, []string{"extra/helper.go"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"item:auth:auth.core", "code:src/main.go"} {
		spec, err := BuildSessionSpec(repoRoot, extended, []string{key})
		if err != nil {
			t.Fatal(err)
		}
		if !stringInSlice(spec.ReadRefs, "extra/helper.go") {
			t.Fatalf("%s session cannot read the extended input: %v", key, spec.ReadRefs)
		}
	}
}

func TestExtendRejectsClosedRun(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	run.Status = StatusConsumed
	if err := writeRun(repoRoot, run); err != nil {
		t.Fatal(err)
	}
	if _, err := Extend(repoRoot, run.RunID, []string{"src/main.go"}, time.Now()); err == nil || !strings.Contains(err.Error(), "only an open run") {
		t.Fatalf("closed run was not rejected: %v", err)
	}
}

// TestItemReadRefsAreDeclaredSurfaceOnly pins that an item session reads the
// spec-declared surface, not a name-token closure: an undeclared file that
// merely mentions a declared file is not part of the read surface. Related
// context is supplied explicitly (`--inputs-file` / `gate-extend`).
func TestItemReadRefsAreDeclaredSurfaceOnly(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, repoRoot, "lib/lib.go", "package lib\n\n// calls main.go helpers\n")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	d, err := NewDerivation(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	run, err := d.resolveRun(GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := d.computeCoverage(run)
	if err != nil {
		t.Fatal(err)
	}
	var item *CoverageKey
	for i := range coverage {
		if coverage[i].Key == "item:auth:auth.core" {
			item = &coverage[i]
		}
	}
	if item == nil {
		t.Fatal("missing item coverage key")
	}
	reads := coverageReadRefs(repoRoot, run, *item)
	if stringInSlice(reads, "lib/lib.go") {
		t.Fatalf("undeclared name-token match leaked into the item read refs: %v", reads)
	}
	if !stringInSlice(reads, "src/main.go") {
		t.Fatalf("declared surface missing from the item read refs: %v", reads)
	}
}

// TestStubScanNoticeListsCandidates pins the Step 6 mechanization: gate-plan
// scans the declared code surface and records candidate hits in the run's
// notice; a clean surface produces no scan notice.
func TestStubScanNoticeListsCandidates(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n\n// placeholder implementation\n")
	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	notices := strings.Join(run.Notices, "\n")
	if !strings.Contains(notices, "stub scan: 1 candidate hit") || !strings.Contains(notices, "src/main.go:3") {
		t.Fatalf("expected a stub-scan notice with the candidate, got:\n%s", notices)
	}

	// A clean surface produces no scan notice.
	writeUnit(t, repoRoot, "candidate", "beta", "none", "none", "src-beta", "")
	writeFile(t, repoRoot, "src-beta/clean.go", "package main\n\nfunc f() int { return 1 }\n")
	clean, err := Plan(repoRoot, GateVerify, TargetKindUnit, "beta", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(clean.Notices, "\n"), "stub scan:") {
		t.Fatalf("clean surface must produce no scan notice, got: %v", clean.Notices)
	}
}

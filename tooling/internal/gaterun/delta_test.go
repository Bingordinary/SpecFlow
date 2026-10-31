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

// writeValidateBaselineWithRelationships writes a pass validate cache whose
// judgment baseline declares the given standing relationship conclusions.
func writeValidateBaselineWithRelationships(t *testing.T, repoRoot string, entries []validationcache.FileEntry, relationships []string) {
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
	for _, name := range relationships {
		statuses[RelationshipKey(name)] = "pass"
		entries[0].Checks = append(entries[0].Checks, validationcache.CheckEntry{Check: RelationshipKey(name), Status: "pass"})
	}
	judgments, err := json.Marshal(JudgmentBaseline{SchemaVersion: validationcache.RecordStateSchemaValidate, LogicalStatus: statuses, Relationships: relationships, SynthesisDigest: "sha256:test"})
	if err != nil {
		t.Fatal(err)
	}
	pinFormat, pins, pinsSet := promiseCacheFormat(t, repoRoot, "auth", GateValidate, "pass")
	_, err = validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:        "validate",
		Unit:           "auth",
		Mode:           "full",
		Basis:          "full",
		Result:         "pass",
		Target:         "candidate",
		PinFormat:      pinFormat,
		PromisePins:    pins,
		PromisePinsSet: pinsSet,
		Standard:       baselineStandard(t, repoRoot),
		Timestamp:      "2026-01-01T00:00:00Z",
		Judgments:      string(judgments),
		Entries:        entries,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func appendToAuthSpec(t *testing.T, repoRoot, extra string) {
	t.Helper()
	path := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Insert under Design Narrative, before the fixed Promises section, so the
	// three-section shape stays valid.
	marker := []byte("\n## Promises")
	if idx := strings.Index(string(data), string(marker)); idx >= 0 {
		data = append(append(append([]byte{}, data[:idx]...), []byte(extra)...), data[idx:]...)
	} else {
		data = append(data, []byte(extra)...)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// validateDeltaBaseline writes a pass validate baseline for unit auth whose
// entry carries real chunk evidence, and returns the entry.
func validateDeltaBaseline(t *testing.T, repoRoot string) validationcache.FileEntry {
	t.Helper()
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")
	return entry
}

// TestDeltaPlanOwnContentChangeCoversFullScope pins the mechanical boundary:
// a validate check is a whole-document judgment, so any change to the unit's
// own spec covers the full scope and the run is recorded as a full run.
func TestDeltaPlanOwnContentChangeCoversFullScope(t *testing.T) {
	repoRoot := newRepo(t)
	validateDeltaBaseline(t, repoRoot)
	appendToAuthSpec(t, repoRoot, "\n### New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeFull {
		t.Fatalf("an own-content change must upgrade the run to full, got mode %q", run.Mode)
	}
	if got, want := strings.Join(coverageKeysOf(run), ","), "document,design,acceptance,external"; got != want {
		t.Fatalf("coverage = %s, want %s", got, want)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("a full-scope plan carries nothing, got %v", run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "own spec content changed") {
		t.Fatalf("expected the own-content notice, got %v", run.Notices)
	}
}

// TestDeltaPlanExternalChangeRunsExternalGroup pins the one sound increment:
// the unit's own spec is unchanged and an external ref (a bound rule) moved,
// so only the external group re-runs.
func TestDeltaPlanExternalChangeRunsExternalGroup(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_x")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "b_rule_x", "src", "")
	authEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	ruleEntry := evidenceEntryWithChecks(t, repoRoot, "rule:b_rule_x", nil)
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{authEntry, ruleEntry}, "pass", false, "full")

	writeFile(t, repoRoot, "docs/specs/rules/candidate/b_rule_x.md", "---\nid: b_rule_x\nscope: unit\n---\n\n# b_rule_x\n\n## Constraint\n\nMust use mTLS.\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("an external-only change keeps the delta basis, got mode %q", run.Mode)
	}
	if got, want := strings.Join(coverageKeysOf(run), ","), "external"; got != want {
		t.Fatalf("coverage = %s, want %s", got, want)
	}
	for _, key := range []string{"1", "2", "3", "4", "5", ClarityCheck} {
		if !stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s must carry, got %v", key, run.CarriedKeys)
		}
	}
	for _, key := range []string{"6", "7", "8", "9"} {
		if stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s re-runs in the external group, got carried %v", key, run.CarriedKeys)
		}
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "external-only change") {
		t.Fatalf("expected the external-only notice, got %v", run.Notices)
	}
}

// TestDeltaPlanUnattributedBaselineInputCoversFullScope pins the fail-safe
// boundary for an input the current plan does not declare: a baseline evidence
// entry that is neither the unit's own spec union nor a derived external ref
// cannot be attributed to the external group. This happens when an earlier
// --inputs-file manifest supplied a shared evidence file and the delta plan does
// not re-supply it; every validate group read that file, so the plan must cover
// the full scope instead of carrying the whole-document checks.
func TestDeltaPlanUnattributedBaselineInputCoversFullScope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, repoRoot string)
	}{
		{"changed", func(t *testing.T, repoRoot string) {
			writeFile(t, repoRoot, "notes/evidence.md", "version two\n")
		}},
		{"removed", func(t *testing.T, repoRoot string) {
			if err := os.Remove(filepath.Join(repoRoot, "notes/evidence.md")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := newRepo(t)
			writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
			writeFile(t, repoRoot, "notes/evidence.md", "version one\n")
			mainEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
			extraEntry := evidenceEntryWithChecks(t, repoRoot, "notes/evidence.md", nil)
			writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{mainEntry, extraEntry}, "pass", false, "full")

			tc.mutate(t, repoRoot)
			run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if run.Mode != ModeFull {
				t.Fatalf("an unattributed baseline input must cover the full scope, got mode %q", run.Mode)
			}
			if got, want := strings.Join(coverageKeysOf(run), ","), "document,design,acceptance,external"; got != want {
				t.Fatalf("coverage = %s, want %s", got, want)
			}
			if len(run.CarriedKeys) != 0 {
				t.Fatalf("a full-scope plan carries nothing, got %v", run.CarriedKeys)
			}
			if !strings.Contains(strings.Join(run.Notices, " "), "own spec content changed") {
				t.Fatalf("expected the own-content notice, got %v", run.Notices)
			}
		})
	}
}

// TestDeltaPlanExternalChangeChecksAffectedRelationship pins the mechanical
// relationship assignment: a rule change re-checks design_constraints (the
// design × constraints conclusion), not coverage_scope, which depends only on
// the unit's own content.
func TestDeltaPlanExternalChangeChecksAffectedRelationship(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_x")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "b_rule_x", "src", "")
	authEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	ruleEntry := evidenceEntryWithChecks(t, repoRoot, "rule:b_rule_x", nil)
	writeValidateBaselineWithRelationships(t, repoRoot, []validationcache.FileEntry{authEntry, ruleEntry}, []string{"design_constraints", "coverage_scope", "cross_unit_cohesion"})

	writeFile(t, repoRoot, "docs/specs/rules/candidate/b_rule_x.md", "---\nid: b_rule_x\nscope: unit\n---\n\n# b_rule_x\n\n## Constraint\n\nMust use mTLS.\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(run.Relationships, "design_constraints") {
		t.Fatalf("a rule change must re-check design_constraints, got %v", run.Relationships)
	}
	if stringInSlice(run.Relationships, "coverage_scope") || stringInSlice(run.Relationships, "cross_unit_cohesion") {
		t.Fatalf("unaffected relationships must carry, got %v", run.Relationships)
	}
}

func TestDeltaPlanFreshRefuses(t *testing.T) {
	repoRoot := newRepo(t)
	validateDeltaBaseline(t, repoRoot)
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "fresh") {
		t.Fatalf("expected the fresh refusal, got %v", err)
	}
}

// TestDeltaPlanUnusableBaselineCoversFullScope pins the fail-safe direction: a
// baseline without the required chunk evidence cannot localize anything, so
// revalidate covers the full scope instead of refusing.
func TestDeltaPlanUnusableBaselineCoversFullScope(t *testing.T) {
	repoRoot := newRepo(t)
	entry := validateDeltaBaseline(t, repoRoot)
	entry.Chunks = nil
	entry.Chunker = ""
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeFull {
		t.Fatalf("an unusable baseline must upgrade to full, got %q", run.Mode)
	}
	if len(run.Coverage) != 4 {
		t.Fatalf("expected the four coverage groups, got %v", coverageKeysOf(run))
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "unusable") {
		t.Fatalf("expected the unusable-baseline notice, got %v", run.Notices)
	}
}

// TestDeltaPlanInvalidBaselineCoversFullScope pins the documented INVALID
// recovery: a validate cache without the promise-pin format marker cannot
// carry its judgments, so the plan covers the full scope instead of refusing
// the run. Verify keeps its full-command refusal.
func TestDeltaPlanInvalidBaselineCoversFullScope(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	dir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "validate_result.md"), []byte("---\ncommand: validate\nunit: auth\nmode: full\nresult: pass\ntarget: candidate\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeFull {
		t.Fatalf("an INVALID validate baseline must cover the full scope, got %q", run.Mode)
	}
	if got, want := strings.Join(coverageKeysOf(run), ","), "document,design,acceptance,external"; got != want {
		t.Fatalf("coverage = %s, want %s", got, want)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("a full-scope plan carries nothing, got %v", run.CarriedKeys)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "unusable as a delta baseline") {
		t.Fatalf("expected the unusable-baseline notice, got %v", run.Notices)
	}
}

// TestDeltaPlanContradictoryEvidenceCoversFullScope pins that recorded evidence
// that contradicts itself (the whole-file hash disagrees with its own matching
// chunk sequence) cannot localize a change, so the plan covers the full scope.
func TestDeltaPlanContradictoryEvidenceCoversFullScope(t *testing.T) {
	repoRoot := newRepo(t)
	entry := validateDeltaBaseline(t, repoRoot)
	entry.Hash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeFull {
		t.Fatalf("contradictory evidence must upgrade to full, got %q", run.Mode)
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "contradicts itself") {
		t.Fatalf("expected the contradictory-evidence notice, got %v", run.Notices)
	}
}

// TestRuleDeltaRejected pins that a rule target has no partial run: its only
// external input is the global rule set, and its own file changing is own
// content, so recovery is always the full confirmation run.
func TestRuleDeltaRejected(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_x")
	if _, err := Plan(repoRoot, GateValidate, TargetKindRule, "b_rule_x", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "full validate run only") {
		t.Fatalf("expected the rule-delta refusal, got %v", err)
	}
}

// TestValidateRepairRejected pins that validate has no repair mode: a failure
// record or a salvaged cache recovers through the derived run, which covers the
// full scope.
func TestValidateRepairRejected(t *testing.T) {
	repoRoot := newRepo(t)
	validateDeltaBaseline(t, repoRoot)
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "no repair mode") {
		t.Fatalf("expected the validate-repair refusal, got %v", err)
	}
}

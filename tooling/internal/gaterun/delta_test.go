package gaterun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
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
	catalog := validateObjectBaseline(t, repoRoot, "auth")
	judgments, err := json.Marshal(JudgmentBaseline{SchemaVersion: validationcache.RecordStateSchemaValidate, LogicalStatus: statuses, Relationships: relationships, SynthesisDigest: "sha256:test", ValidateObjects: catalog, FilePresence: filePresenceForKeys(t, repoRoot, catalog)})
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
		Standards:      baselineStandards(t, repoRoot),
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
	// Declare the appended narrative heading non-behavior so the fixture
	// satisfies the narrative coverage rule; the test exercises an own-content
	// delta, not narrative coverage.
	heading := ""
	for _, line := range strings.Split(extra, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			heading = strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
			break
		}
	}
	if heading != "" && !strings.Contains(string(data), "non_behavior_blocks:") {
		if parts := strings.SplitN(string(data), "\n---", 2); len(parts) == 2 {
			data = []byte(parts[0] + "\nnon_behavior_blocks: [" + heading + "]\n---" + parts[1])
		}
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

// TestDeltaPlanWholeFileChangeCoversFullScope pins the coarse-change path: the
// fixture's change set covers the whole file, so every object's read region
// moves. Every object re-runs and the plan is recorded as a full run. A
// localizable edit re-runs only the changed objects (see the evidence-scope
// delta tests).
func TestDeltaPlanWholeFileChangeCoversFullScope(t *testing.T) {
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
	if !strings.Contains(strings.Join(run.Notices, " "), "every object has changed") {
		t.Fatalf("expected the full-scope notice, got %v", run.Notices)
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
	for _, key := range []string{"6", "9"} {
		if !stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s has no object depending on the changed rule and must carry, got %v", key, run.CarriedKeys)
		}
	}
	for _, key := range []string{"7", "8"} {
		if stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s is judged by the external session, got carried %v", key, run.CarriedKeys)
		}
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "object(s) re-run") {
		t.Fatalf("expected the object re-run notice, got %v", run.Notices)
	}
}

// TestDeltaPlanDerivedRuleChangeRunsExternalGroup pins the derived-rule
// closure: a stable global rule applies by default and is never declared in
// rule_refs, so the spec-derived catalog alone has no object for it. The
// catalog must still carry one — a changed global rule selects the external
// group instead of reporting "no changes" against a cache that `fresh`
// reports STALE, and instead of carrying check 8 over the changed rule.
func TestDeltaPlanDerivedRuleChangeRunsExternalGroup(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "stable", "g_rule_x")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	authEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	ruleEntry := evidenceEntryWithChecks(t, repoRoot, "rule:g_rule_x", nil)
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{authEntry, ruleEntry}, "pass", false, "full")

	full, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !derivedRuleCataloged(full, "rule:g_rule_x") {
		t.Fatalf("the derived rule is missing from the full-run catalog: %+v", full.ValidateObjects)
	}

	writeFile(t, repoRoot, "docs/specs/rules/stable/g_rule_x.md", "---\nid: g_rule_x\nscope: unit\n---\n\n# g_rule_x\n\n## Constraint\n\nMust use mTLS.\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("a changed derived rule keeps the delta basis, got mode %q", run.Mode)
	}
	if got, want := strings.Join(coverageKeysOf(run), ","), "external"; got != want {
		t.Fatalf("coverage = %s, want %s", got, want)
	}
	for _, key := range []string{"1", "2", "3", "4", "5", ClarityCheck} {
		if !stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s must carry, got %v", key, run.CarriedKeys)
		}
	}
	for _, key := range []string{"6", "9"} {
		if !stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s has no object depending on the changed rule and must carry, got %v", key, run.CarriedKeys)
		}
	}
	for _, key := range []string{"7", "8"} {
		if stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s is judged by the external session, got carried %v", key, run.CarriedKeys)
		}
	}
}

// TestDeltaPlanNewDerivedRuleRunsExternalGroup pins the growth direction: a
// stable global rule published after the baseline has no recorded verdict,
// so the external group must re-run instead of carrying rule alignment over
// a rule the baseline never judged.
func TestDeltaPlanNewDerivedRuleRunsExternalGroup(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	authEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{authEntry}, "pass", false, "full")

	writeRule(t, repoRoot, "stable", "g_rule_new")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !stringInSlice(coverageKeysOf(run), "external") {
		t.Fatalf("a newly published derived rule must re-run the external group, got coverage %v", coverageKeysOf(run))
	}
	for _, key := range []string{"6", "9"} {
		if !stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s has no object depending on the changed rule and must carry, got %v", key, run.CarriedKeys)
		}
	}
	for _, key := range []string{"7", "8"} {
		if stringInSlice(run.CarriedKeys, key) {
			t.Fatalf("check %s is judged by the external session, got carried %v", key, run.CarriedKeys)
		}
	}
}

// derivedRuleCataloged reports whether the run's per-object catalog carries
// an object for the given derived rule reference.
func derivedRuleCataloged(run *Run, ref string) bool {
	for _, object := range run.ValidateObjects {
		if strings.HasPrefix(object.ID, ref+":") {
			return true
		}
	}
	return false
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
			if !strings.Contains(strings.Join(run.Notices, " "), "not part of the unit's own spec union") {
				t.Fatalf("expected the unattributed-input notice, got %v", run.Notices)
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

// TestRepairIsNotAnEntryMode pins that repair is a derived state, never an
// entry mode: the incremental entry is delta for both gates, and the verify
// planner derives the repair from a failure-record or salvaged baseline.
func TestRepairIsNotAnEntryMode(t *testing.T) {
	repoRoot := newRepo(t)
	validateDeltaBaseline(t, repoRoot)
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeRepair, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "must be full or delta") {
		t.Fatalf("expected the entry-mode refusal, got %v", err)
	}
}

// writeLargeNarrativeSpec writes a multi-chunk unit spec whose narrative block
// is sourced by the promise and acceptance item. The varied lines keep the
// content-defined chunk sequence localizable, so a one-line edit produces a
// subset delta instead of a whole-file coarse change.
func writeLargeNarrativeSpec(t *testing.T, repoRoot string) string {
	t.Helper()
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_auth.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0755); err != nil {
		t.Fatal(err)
	}
	words := []string{"authentication", "session", "token", "rotation", "policy", "audit", "retry", "boundary", "cache", "scope", "provider", "digest"}
	var narrative strings.Builder
	for i := 0; i < 300; i++ {
		narrative.WriteString(fmt.Sprintf("Statement %d: the %s subsystem keeps %s behavior %d stable and records %s evidence for round %d.\n",
			i, words[i%len(words)], words[(i*3+1)%len(words)], i*7+3, words[(i*5+2)%len(words)], i))
	}
	content := "---\nid: auth\nrule_refs: none\n---\n\n# auth\n\n" +
		"## Design Narrative\n### Narrative\n\n" + narrative.String() + "\n" +
		"## Promises\n\npromises:\n  - id: auth.core\n    summary: Core contract.\n    source: [Narrative]\n    contract: {invariants: [core]}\n\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n" +
		"  - id: auth.core\n    promise: auth.core\n    source: [Narrative]\n    description: Core behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return specPath
}

// TestDeltaPlanSubsetReRunsStandingRelationships pins the relationship
// freshness rule: a relationship is a conclusion over the local results, so a
// delta that re-runs any local coverage group must re-check the baseline's
// standing relationships instead of carrying a verdict computed over
// superseded input. The localized narrative edit re-runs document, design and
// acceptance while the external group carries.
func TestDeltaPlanSubsetReRunsStandingRelationships(t *testing.T) {
	repoRoot := newRepo(t)
	specPath := writeLargeNarrativeSpec(t, repoRoot)
	authEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeValidateBaselineWithRelationships(t, repoRoot, []validationcache.FileEntry{authEntry}, []string{"coverage_scope"})

	original, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(original),
		"Statement 150: the retry subsystem keeps boundary behavior 1053 stable and records cache evidence for round 150.",
		"Statement 150: the retry subsystem keeps a different behavior 1053 stable and records cache evidence for round 150.", 1)
	if updated == string(original) {
		t.Fatal("localized narrative edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("expected a subset delta, got mode %q", run.Mode)
	}
	if stringInSlice(coverageKeysOf(run), "external") {
		t.Fatalf("the external group must carry for a narrative-only edit, got coverage %v", coverageKeysOf(run))
	}
	if !stringInSlice(run.Relationships, "coverage_scope") {
		t.Fatalf("a subset delta must re-check the standing relationship, got %v", run.Relationships)
	}
	for _, key := range run.CarriedKeys {
		if key == "relationship:coverage_scope" {
			t.Fatalf("the relationship must not carry over changed local results, got carried %v", run.CarriedKeys)
		}
	}
}

// TestDeltaPlanDesignChangeRerunsRuleObject pins the rule-alignment input:
// Check 8 reads the unit design, so a design edit must re-run the rule object
// even when the rule file itself did not move. Without the whole-unit ref in
// the rule object's reads the plan carries external over a changed design.
func TestDeltaPlanDesignChangeRerunsRuleObject(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "stable", "g_rule_x")
	specPath := writeLargeNarrativeSpec(t, repoRoot)
	full, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{"1": "pass", "2": "pass", "3": "pass", "4": "pass", "5": "pass", "6": "pass", "7": "pass", "8": "pass", "9": "pass", ClarityCheck: "pass"}
	judgments, err := json.Marshal(JudgmentBaseline{
		SchemaVersion:   validationcache.RecordStateSchemaValidate,
		LogicalStatus:   statuses,
		SynthesisDigest: "sha256:test",
		ValidateObjects: full.ValidateObjects,
		FilePresence:    filePresenceForKeys(t, repoRoot, full.ValidateObjects),
	})
	if err != nil {
		t.Fatal(err)
	}
	pinFormat, pins, pinsSet := promiseCacheFormat(t, repoRoot, "auth", GateValidate, "pass")
	specEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	ruleEntry := evidenceEntryWithChecks(t, repoRoot, "rule:g_rule_x", nil)
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:        "validate",
		Unit:           "auth",
		Mode:           "full",
		Basis:          "full",
		Result:         "pass",
		Target:         "candidate",
		PinFormat:      pinFormat,
		PromisePins:    pins,
		PromisePinsSet: pinsSet,
		Standards:      baselineStandards(t, repoRoot),
		Timestamp:      "2026-01-01T00:00:00Z",
		Judgments:      string(judgments),
		Entries:        []validationcache.FileEntry{specEntry, ruleEntry},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data),
		"Statement 150: the retry subsystem keeps boundary behavior 1053 stable and records cache evidence for round 150.",
		"Statement 150: the retry subsystem keeps a different behavior 1053 stable and records cache evidence for round 150.", 1)
	if updated == string(data) {
		t.Fatal("localized narrative edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(coverageKeysOf(run), "external") {
		t.Fatalf("a changed design must re-run the external rule-alignment judgment, got coverage %v (carried %v)", coverageKeysOf(run), run.CarriedKeys)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("a section-local design change must keep the object-level delta, got mode %q", run.Mode)
	}
	external, err := BuildSessionSpec(repoRoot, run, []string{"external"})
	if err != nil {
		t.Fatal(err)
	}
	allExternal := 0
	for _, object := range run.ValidateObjects {
		if validateObjectGroup(object) == "external" {
			allExternal++
		}
	}
	if len(external.ValidateObjects) >= allExternal {
		t.Fatalf("a section-local design change must not assign every external object: assigned %d of %d", len(external.ValidateObjects), allExternal)
	}
	for _, object := range external.ValidateObjects {
		if !stringInSlice(run.RerunObjects, object.ID) {
			t.Fatalf("a delta session received a carried object %q", object.ID)
		}
	}
}

// TestDeltaPlanEvidenceRecordNamedRuleIsNotRuleChange pins exact ref
// matching: a unit name that happens to contain "rule_" is own content, not a
// rule reference. Its Keys-bound design section re-runs; the rule file is not
// treated as the changed input.
func TestDeltaPlanEvidenceRecordNamedRuleIsNotRuleChange(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_x")
	extra := "    affects:\n      rules:\n        - b_rule_x\n"
	writeUnit(t, repoRoot, "candidate", "auth_rule_notes", "none", "b_rule_x", "src", extra)
	record := "docs/specs/units/candidate/evidence/unit_auth_rule_notes.md"
	writeFile(t, repoRoot, record, "---\nunit: auth_rule_notes\n---\n\n# Support\n\n## Notes\n\nKeys:\n  - item:auth_rule_notes.core\n\nobserved one\n")
	specEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth_rule_notes.md", mainCheckMarkers())
	evidenceEntry := evidenceEntryWithChecks(t, repoRoot, record, nil)
	ruleEntry := evidenceEntryWithChecks(t, repoRoot, "rule:b_rule_x", nil)
	writeValidateBaselineFor(t, repoRoot, "auth_rule_notes", []validationcache.FileEntry{specEntry, evidenceEntry, ruleEntry}, "pass", false, "full")

	if err := os.WriteFile(filepath.Join(repoRoot, record), []byte("---\nunit: auth_rule_notes\n---\n\n# Support\n\n## Notes\n\nKeys:\n  - item:auth_rule_notes.core\n\nobserved two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth_rule_notes", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("an evidence-record path containing rule_ must not be treated as a rule change, got mode %q (notices %v)", run.Mode, run.Notices)
	}
	if !containsRerunObject(run, "item:auth_rule_notes.core:evidence") {
		t.Fatalf("the record's Keys-bound evidence object must re-run, got %v", run.RerunObjects)
	}
}

// TestValidateScopeRerunsReadSetChange pins the planner contract: a changed
// dependency surface re-runs the object even when no current line overlaps its
// own region. The old implementation only set a graph flag and carried the
// stale verdict.
func TestValidateScopeRerunsReadSetChange(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "auth.core")
	run := &Run{
		Gate:         GateValidate,
		TargetKind:   TargetKindUnit,
		TargetName:   "auth",
		OwnSpecFiles: []string{"docs/specs/units/candidate/unit_auth.md"},
		ValidateObjects: []validatekeys.Key{
			{ID: "item:auth.core:source", Family: validatekeys.FamilyItem, Check: "5", Reads: []string{"item:auth.core", "section:Narrative"}},
		},
	}
	state := JudgmentBaseline{
		ValidateObjects: []validatekeys.Key{
			{ID: "item:auth.core:source", Family: validatekeys.FamilyItem, Check: "5", Reads: []string{"item:auth.core", "section:Design Narrative"}},
		},
	}
	scope, err := (&Derivation{root: repoRoot}).selectValidateObjectScope(run, state, &validationcache.ChangeReport{})
	if err != nil {
		t.Fatal(err)
	}
	if !scope.RerunObjects["item:auth.core:source"] {
		t.Fatalf("an object whose read set changed must re-run, got %v", scope.RerunObjects)
	}
}

// TestDeltaPlanDeclaredFilePresenceChangeRerunsAffects pins the file-existence
// input: Check 6 requires every affects.files path to exist, so removing a
// declared file must re-run the object that reads it. Content changes are not
// a validate dependency; presence is.
func TestDeltaPlanDeclaredFilePresenceChangeRerunsAffects(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	extra := "    affects:\n      files:\n        - src/main.go\n"
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", extra)
	specEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{specEntry}, "pass", false, "full")

	if err := os.Remove(filepath.Join(repoRoot, "src/main.go")); err != nil {
		t.Fatal(err)
	}
	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(coverageKeysOf(run), "external") {
		t.Fatalf("a removed declared file must re-run the external affects judgment, got coverage %v (carried %v)", coverageKeysOf(run), run.CarriedKeys)
	}
}

// TestDeltaRerunCheckForcesObjects pins the explicit override at object
// granularity: --rerun 5 must put every Check 5 object into the batch even
// when no dependency changed.
func TestDeltaRerunCheckForcesObjects(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnitItemsSpec(t, repoRoot, "auth.core")
	specEntry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{specEntry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, []string{"5"}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{"acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, object := range spec.ValidateObjects {
		if object.ID == "item:auth.core:substance" {
			found = true
		}
	}
	if !found {
		t.Fatalf("--rerun 5 must force Check 5 objects into the session, got %+v", spec.ValidateObjects)
	}
}

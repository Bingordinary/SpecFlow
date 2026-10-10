package gaterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// validateReviewBaseline writes a pass validate baseline for unit auth whose
// entry carries real chunk evidence, and returns the entry.
func validateReviewBaseline(t *testing.T, repoRoot string) validationcache.FileEntry {
	t.Helper()
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")
	return entry
}

func appendToAuthSpec(t *testing.T, repoRoot, extra string) {
	t.Helper()
	path := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(extra)...), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDeltaPlanCreatesReviewCoverage(t *testing.T) {
	repoRoot := newRepo(t)
	validateReviewBaseline(t, repoRoot)
	appendToAuthSpec(t, repoRoot, "\n## New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != DeltaReviewKey {
		t.Fatalf("expected the review coverage only, got %v", got)
	}
	if run.ChangeSetFP == "" || run.ChangeReport == nil || run.ChangeReport.Empty() {
		t.Fatalf("expected a populated change set, got %+v", run.ChangeReport)
	}
	if len(run.CarriedKeys) == 0 {
		t.Fatal("expected standing conclusions as carry candidates")
	}
	for _, key := range run.CarriedKeys {
		if run.BaselineStatus[key] != "pass" {
			t.Fatalf("candidate %q is not a baseline conclusion: %v", key, run.BaselineStatus)
		}
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "change review") {
		t.Fatalf("expected the review notice, got %v", run.Notices)
	}
}

// TestReviewSessionReadsWholeInputSurface pins the review session's read
// surface: it spans the run's whole input surface so the reviewer can follow
// a changed file to a conclusion anchored elsewhere — not just the target's
// own spec plus the changed files
// (framework/verification_scope.md §Delta Runs → The change review).
func TestReviewSessionReadsWholeInputSurface(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src/dep", "")
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "none", "src", "")
	entry := evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers())
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")
	appendToAuthSpec(t, repoRoot, "\n## New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{DeltaReviewKey})
	if err != nil {
		t.Fatal(err)
	}
	// unit:dep is unchanged and absent from the change set, but it is part of
	// the run's input surface and a standard conclusion may be anchored in it.
	for _, ref := range []string{"unit:dep", "docs/specs/units/candidate/unit_auth.md"} {
		if !stringInSlice(spec.ReadRefs, ref) {
			t.Fatalf("review read refs = %v, want %s", spec.ReadRefs, ref)
		}
	}
}

func TestDeltaPlanFreshRefuses(t *testing.T) {
	repoRoot := newRepo(t)
	validateReviewBaseline(t, repoRoot)
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "fresh") {
		t.Fatalf("expected the fresh refusal, got %v", err)
	}
}

func TestDeltaPlanLegacyEvidenceRefuses(t *testing.T) {
	repoRoot := newRepo(t)
	entry := validateReviewBaseline(t, repoRoot)
	entry.Chunks = nil
	entry.Chunker = ""
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")
	appendToAuthSpec(t, repoRoot, "\nchanged\n")

	// A baseline without the required chunk evidence is not current-format: it
	// is treated as no baseline and delta planning directs the full command.
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "not in the current format") {
		t.Fatalf("expected the not-current-format refusal, got %v", err)
	}
}

// TestDeltaPlanContradictoryEvidenceRefuses pins that a readable cache whose
// recorded evidence contradicts itself (its whole-file hash disagrees with its
// own, matching chunk sequence) cannot serve as a delta baseline: the change
// cannot be localized, so delta planning refuses and directs the full command.
func TestDeltaPlanContradictoryEvidenceRefuses(t *testing.T) {
	repoRoot := newRepo(t)
	entry := validateReviewBaseline(t, repoRoot)
	entry.Hash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "cannot be localized") {
		t.Fatalf("expected the contradictory-evidence refusal, got %v", err)
	}
}

// TestFullPlanToleratesLegacyCache pins that a full run is not blocked by the
// old-format cache it is meant to rebuild: a baseline without the required
// chunk evidence is not current-format and degrades to no carried conclusions
// and no change review, while the full coverage set runs. Delta still refuses
// it, because only the full command rebuilds the cache.
func TestFullPlanToleratesLegacyCache(t *testing.T) {
	repoRoot := newRepo(t)
	entry := validateReviewBaseline(t, repoRoot)
	entry.Chunks = nil
	entry.Chunker = ""
	writeValidateBaseline(t, repoRoot, []validationcache.FileEntry{entry}, "pass", false, "full")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("a full run must not be blocked by the old-format cache it rebuilds: %v", err)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("an unusable baseline carries nothing, got %v", run.CarriedKeys)
	}
	for _, ck := range run.Coverage {
		if ck.Key == DeltaReviewKey {
			t.Fatalf("an unusable baseline adds no change review, got %v", coverageKeysOf(run))
		}
	}
	if len(run.Coverage) == 0 {
		t.Fatal("expected the full coverage set")
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "unusable") {
		t.Fatalf("expected a notice that the existing cache is unusable, got %v", run.Notices)
	}

	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now()); err == nil {
		t.Fatal("expected delta planning to still refuse the old-format cache")
	}
}

func TestApplyReviewRecheckWidensCoverage(t *testing.T) {
	repoRoot := newRepo(t)
	validateReviewBaseline(t, repoRoot)
	appendToAuthSpec(t, repoRoot, "\n## New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(run.CarriedKeys, "2") {
		t.Fatalf("check 2 must start as a carry candidate, got %v", run.CarriedKeys)
	}
	if err := ApplyReviewLocked(repoRoot, run, ReviewRecord{Result: "recheck", Recheck: []string{"2"}, Reason: "Description changed", Session: DeltaReviewKey}); err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(repoRoot, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	coverage := strings.Join(coverageKeysOf(reloaded), ",")
	if !strings.Contains(coverage, DeltaReviewKey) || !strings.Contains(coverage, "design") {
		t.Fatalf("expected the review and the design group in coverage, got %s", coverage)
	}
	if stringInSlice(reloaded.CarriedKeys, "2") || stringInSlice(reloaded.CarriedKeys, "4") {
		t.Fatalf("the design group must not stay carried, got %v", reloaded.CarriedKeys)
	}
	if !stringInSlice(reloaded.CarriedKeys, "1") {
		t.Fatalf("unrelated checks must stay carried, got %v", reloaded.CarriedKeys)
	}
	if reloaded.Review == nil || reloaded.Review.ChangeSet != reloaded.ChangeSetFP {
		t.Fatalf("expected the review bound to the change-set fingerprint, got %+v", reloaded.Review)
	}
}

func TestApplyReviewEscalates(t *testing.T) {
	repoRoot := newRepo(t)
	validateReviewBaseline(t, repoRoot)
	appendToAuthSpec(t, repoRoot, "\n## New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyReviewLocked(repoRoot, run, ReviewRecord{Result: "escalate-full", Reason: "too broad", Session: DeltaReviewKey}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(repoRoot, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Escalated || reloaded.Review == nil {
		t.Fatalf("expected an escalated run with a recorded review, got %+v", reloaded.Review)
	}
}

func TestApplyReviewRejectsUnknownKey(t *testing.T) {
	repoRoot := newRepo(t)
	validateReviewBaseline(t, repoRoot)
	appendToAuthSpec(t, repoRoot, "\n## New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	err = ApplyReviewLocked(repoRoot, run, ReviewRecord{Result: "recheck", Recheck: []string{"999"}, Reason: "?", Session: DeltaReviewKey})
	if err == nil {
		t.Fatal("expected an unknown recheck key to fail closed")
	}
	// The rejection is self-correcting: it names the accepted spellings (the
	// standing conclusions), so the reviewer does not have to read tooling
	// source to find them.
	if !strings.Contains(err.Error(), "standing conclusions") || !strings.Contains(err.Error(), "1") {
		t.Fatalf("recheck rejection must name the accepted standing conclusions, got %v", err)
	}
}

// TestApplyReviewRejectsGroupAndReportSpellings pins the single accepted
// recheck vocabulary: the coverage-group id and the report's `check-{n}` label
// are both rejected, and each rejection names the standing conclusions.
func TestApplyReviewRejectsGroupAndReportSpellings(t *testing.T) {
	repoRoot := newRepo(t)
	validateReviewBaseline(t, repoRoot)
	appendToAuthSpec(t, repoRoot, "\n## New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"dependencies", "check-8"} {
		err := ApplyReviewLocked(repoRoot, run, ReviewRecord{Result: "recheck", Recheck: []string{key}, Reason: "?", Session: DeltaReviewKey})
		if err == nil {
			t.Fatalf("recheck key %q must fail closed", key)
		}
		if !strings.Contains(err.Error(), "standing conclusions") {
			t.Fatalf("rejection for %q must name the accepted spellings, got %v", key, err)
		}
	}
}

func TestDeltaPlanRunsForcedAndNewKeys(t *testing.T) {
	repoRoot := newRepo(t)
	validateReviewBaseline(t, repoRoot)
	appendToAuthSpec(t, repoRoot, "\n## New Requirements\n\nbrand new content\n")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, []string{"1"}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	coverage := strings.Join(coverageKeysOf(run), ",")
	if !strings.Contains(coverage, DeltaReviewKey) || !strings.Contains(coverage, "structural") {
		t.Fatalf("expected the review and the forced structural group, got %s", coverage)
	}
	if stringInSlice(run.CarriedKeys, "1") || stringInSlice(run.CarriedKeys, "3") {
		t.Fatalf("forced checks must not stay carried, got %v", run.CarriedKeys)
	}
}

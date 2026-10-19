package gaterun

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// Relationship names describe judgments between parts, not local checks.
// The coordinator selects the relationships touched by the current change;
// dependency evidence determines which previous judgments must also rerun.
func RelationshipNames(gate string) []string {
	switch gate {
	case GateValidate:
		return []string{"design_constraints", "coverage_scope", "cross_unit_cohesion"}
	case GateVerify:
		return []string{"contract_consistency", "data_definition_drift", "state_machine_coherence", "error_code_conflict", "cross_reference_integrity"}
	}
	return nil
}

func RelationshipKey(name string) string { return "relationship:" + name }

func IsRelationshipKey(key string) bool { return strings.HasPrefix(key, "relationship:") }

func relationshipName(key string) string { return strings.TrimPrefix(key, "relationship:") }

func validateRelationships(run *Run) error {
	if run.TargetKind == TargetKindRule && len(run.Relationships) > 0 {
		return fmt.Errorf("rule validate has no relationship synthesis")
	}
	for _, name := range run.Relationships {
		if !stringInSlice(RelationshipNames(run.Gate), name) {
			return fmt.Errorf("unknown relationship %q for %s", name, run.Gate)
		}
	}
	return nil
}

// AllRelationships includes unchanged relationship judgments carried from
// the baseline. They remain recorded even when this run needs no synthesis.
func (r *Run) AllRelationships() []string {
	names := append([]string(nil), r.Relationships...)
	for _, key := range r.CarriedKeys {
		if IsRelationshipKey(key) {
			names = append(names, relationshipName(key))
		}
	}
	return append([]string{}, dedupeSorted(names)...)
}

// planFullRunCarry preserves the baseline's standing relationship
// conclusions in a full run. Failed conclusions re-run. A validate run routes
// unchanged passing conclusions through its change review when the recorded
// change set is non-empty (the run gains the `review` coverage key and the
// reviewer accepts or names rechecks) and carries them mechanically when
// nothing changed. A verify run has no change review: unchanged passing
// conclusions carry mechanically when nothing changed, and re-run directly
// when the unit changed, so the synthesis is never carried over superseded
// input.
func planFullRunCarry(repoRoot string, run *Run) ([]string, []string, error) {
	if run.TargetKind == TargetKindRule {
		return nil, nil, nil
	}
	baseline, err := validationcache.ReadGateBaseline(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, nil, err
	}
	if !baseline.Exists {
		// An existing but unusable baseline carries nothing; this full run
		// rebuilds it. Report why so the skipped carry is not silent.
		if baseline.UnusableReason != "" {
			return nil, []string{"existing gate cache is unusable — no carried conclusions: " + baseline.UnusableReason}, nil
		}
		return nil, nil, nil
	}
	state, err := validatedJudgmentState(baseline)
	if err != nil {
		// A full run does not rely on an unusable baseline: the assigned
		// relationships re-run, the rest cannot be enumerated.
		return nil, nil, nil
	}
	var candidates, notices []string
	for _, name := range state.Relationships {
		key := RelationshipKey(name)
		if !stringInSlice(RelationshipNames(run.Gate), name) {
			return nil, nil, fmt.Errorf("baseline declares unknown relationship %q", name)
		}
		if stringInSlice(run.Relationships, name) {
			continue
		}
		if state.LogicalStatus[key] != "pass" {
			run.Relationships = appendUnique(run.Relationships, name)
			notices = append(notices, "baseline relationship "+key+" did not pass — it re-runs")
			continue
		}
		candidates = append(candidates, key)
	}
	run.Relationships = dedupeSorted(run.Relationships)
	if len(candidates) == 0 {
		return nil, notices, nil
	}
	report, err := validationcache.DeriveChangeReport(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, nil, err
	}
	// New physical inputs the baseline never recorded are part of a validate
	// change set: the reviewer sees them as added files. Verify has no review,
	// so its added inputs are background material and join nothing. Logical
	// references resolve through the freshness chain and are not duplicated.
	if run.Gate != GateVerify {
		recorded := map[string]bool{}
		for _, entry := range baseline.Entries {
			recorded[filepath.ToSlash(filepath.Clean(entry.Path))] = true
		}
		for _, p := range extraInputPaths(repoRoot, run) {
			if isLogicalRef(p) {
				continue
			}
			clean := filepath.ToSlash(filepath.Clean(p))
			if recorded[clean] {
				continue
			}
			report.Entries = append(report.Entries, validationcache.ChangeReportEntry{Path: p, Kind: "added"})
		}
	}
	if len(report.Inconsistent()) > 0 {
		// The cache records evidence that contradicts itself, so the change is
		// known but not localizable: the standing conclusions re-run instead
		// of being carried or reviewed unanchored.
		for _, key := range candidates {
			run.Relationships = appendUnique(run.Relationships, relationshipName(key))
		}
		run.Relationships = dedupeSorted(run.Relationships)
		notices = append(notices, "the baseline cache records contradictory evidence — the standing relationship conclusions re-run: "+strings.Join(candidates, ", "))
		return nil, notices, nil
	}
	if run.BaselineStatus == nil {
		run.BaselineStatus = map[string]string{}
	}
	for _, key := range candidates {
		run.BaselineKeys = append(run.BaselineKeys, key)
		run.BaselineStatus[key] = state.LogicalStatus[key]
	}
	run.BaselineKeys = dedupeSorted(run.BaselineKeys)
	if report.Empty() {
		// Nothing changed since promote: the standing conclusions carry
		// mechanically and the run needs no review session.
		notices = append(notices, "no content change — the standing relationship conclusions carry mechanically: "+strings.Join(candidates, ", "))
		return candidates, notices, nil
	}
	if run.Gate == GateVerify {
		// Verify has no change review: a changed unit re-runs its standing
		// relationship conclusions rather than carrying a verdict computed
		// over superseded input.
		for _, key := range candidates {
			run.Relationships = appendUnique(run.Relationships, relationshipName(key))
		}
		run.Relationships = dedupeSorted(run.Relationships)
		notices = append(notices, "the unit changed — the standing relationship conclusions re-run: "+strings.Join(candidates, ", "))
		return nil, notices, nil
	}
	fingerprint, err := report.Fingerprint()
	if err != nil {
		return nil, nil, err
	}
	run.ChangeSetFP = fingerprint
	run.ChangeReport = report
	return candidates, notices, nil
}

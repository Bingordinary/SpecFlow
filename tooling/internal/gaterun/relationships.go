package gaterun

import (
	"fmt"
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
// conclusions in a full run. Failed conclusions re-run. Unchanged passing
// conclusions carry mechanically when nothing changed, and re-run directly
// when the unit changed, so the synthesis is never carried over superseded
// input. There is no review session.
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
	// A relationship conclusion is judged by the final-synthesis standard. If
	// that standard moved, the baseline's standing relationships must re-check
	// even on a full run: the local results they consume are rebuilt, but the
	// synthesis rules that produced the verdict are not the current ones. The
	// final synthesis is the one conclusion with no judgment record, so its
	// standard is the cache's `standards.cross` entry.
	if run.TargetKind == TargetKindUnit && hasRecordedSynthesis(state) {
		recorded, recordedOK := baseline.Standards[CrossKey]
		current, currentOK := run.Standards[CrossKey]
		if !recordedOK || !currentOK || recorded != current {
			for _, name := range state.Relationships {
				run.Relationships = appendUnique(run.Relationships, name)
			}
			run.Relationships = dedupeSorted(run.Relationships)
			run.SynthesisRerun = true
			notices = append(notices, "the final-synthesis judging standard moved — the synthesis re-runs over the carried local results")
			return nil, notices, nil
		}
	}
	for _, name := range state.Relationships {
		key := RelationshipKey(name)
		if !stringInSlice(RelationshipNames(run.Gate), name) {
			return nil, nil, fmt.Errorf("baseline declares unknown relationship %q", name)
		}
		if stringInSlice(run.Relationships, name) {
			continue
		}
		// A carried relationship is a passing conclusion whose content was
		// unchanged at the previous run; it is a carry candidate like a pass
		// conclusion, not a failed one that must re-run.
		if status := state.LogicalStatus[key]; status != "pass" && status != "carried" {
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
	if len(report.Inconsistent()) > 0 {
		// The cache records evidence that contradicts itself, so the change is
		// known but not localizable: the standing conclusions re-run instead of
		// being carried.
		for _, key := range candidates {
			run.Relationships = appendUnique(run.Relationships, relationshipName(key))
		}
		run.Relationships = dedupeSorted(run.Relationships)
		notices = append(notices, "the baseline cache records contradictory evidence — the standing relationship conclusions re-run: "+strings.Join(candidates, ", "))
		return nil, notices, nil
	}
	if report.Empty() {
		// Nothing changed since promote: the standing conclusions carry
		// mechanically.
		notices = append(notices, "no content change — the standing relationship conclusions carry mechanically: "+strings.Join(candidates, ", "))
		return candidates, notices, nil
	}
	// A changed unit re-runs its standing relationship conclusions rather than
	// carrying a verdict computed over superseded input.
	for _, key := range candidates {
		run.Relationships = appendUnique(run.Relationships, relationshipName(key))
	}
	run.Relationships = dedupeSorted(run.Relationships)
	notices = append(notices, "the unit changed — the standing relationship conclusions re-run: "+strings.Join(candidates, ", "))
	return nil, notices, nil
}

// hasRecordedSynthesis reports whether a baseline carries a final-synthesis
// conclusion: recorded relationship verdicts, a recorded `cross` status (a
// synthesis that disposed findings without an assigned relationship), or a
// retained cross finding. The synthesis has no judgment record, so a cache
// that carries one binds its standard under `standards.cross`.
func hasRecordedSynthesis(state JudgmentBaseline) bool {
	if len(state.Relationships) > 0 || state.LogicalStatus[CrossKey] != "" {
		return true
	}
	for _, finding := range state.Findings {
		if finding.SourceKey == CrossKey {
			return true
		}
	}
	return false
}

// VerifyRelationshipConsumedKinds maps each verify relationship to the
// coverage-key classes whose re-judgment can change its cross-check verdict.
// A relationship compares contracts and shared definitions across source
// parts — the unit-level declaration scan and the code files that carry the
// definitions — so an acceptance-item-only re-run leaves it sound: the item
// checks behavior, not the contract structure the relationship compares. A new
// finding the item raises is still disposed by the synthesis; only the cross
// item itself carries.
var VerifyRelationshipConsumedKinds = map[string][]string{
	"contract_consistency":      {"unit:", "code:", "design:"},
	"data_definition_drift":     {"unit:", "code:"},
	"state_machine_coherence":   {"unit:", "code:"},
	"error_code_conflict":       {"unit:", "code:"},
	"cross_reference_integrity": {"unit:", "code:", "design:", "item:"},
}

// RelationshipConsumesChangedKey reports whether the named verify relationship
// consumes any of the given re-run keys. A relationship whose consumed keys all
// carried keeps its recorded verdict instead of re-checking over unchanged
// inputs (framework/verification_scope.md §Incremental relationship work).
func RelationshipConsumesChangedKey(name string, changed map[string]bool) bool {
	prefixes, known := VerifyRelationshipConsumedKinds[name]
	if !known {
		return true
	}
	for key := range changed {
		if IsRelationshipKey(key) {
			continue
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
	}
	return false
}

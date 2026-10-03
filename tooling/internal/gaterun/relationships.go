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

// ScopeKeys separates the final session's summary verdict from each
// relationship's evidence. Cross evidence is for finding dispositions only.
func (s *SessionSpec) ScopeKeys() []string {
	keys := append([]string(nil), s.CheckKeys...)
	for _, name := range s.Relationships {
		keys = append(keys, RelationshipKey(name))
	}
	return keys
}

func (s *SessionSpec) RequiredScopeKeys() []string {
	if s.Kind != SessionKindCross || len(s.Relationships) == 0 {
		return append([]string(nil), s.CheckKeys...)
	}
	var keys []string
	for _, name := range s.Relationships {
		keys = append(keys, RelationshipKey(name))
	}
	return keys
}

// A full local review still preserves unchanged relationship evidence. This
// keeps later re* runs aware of relationships established by earlier changes.
func fullRelationshipScope(repoRoot string, run *Run) ([]string, error) {
	if run.TargetKind == TargetKindRule {
		return nil, nil
	}
	baseline, err := validationcache.ReadGateBaseline(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	if !baseline.Exists {
		return nil, nil
	}
	state, err := validatedJudgmentState(baseline)
	if err != nil {
		return nil, nil
	} // A full run does not rely on an unusable baseline.
	scope, err := validationcache.DeriveStaleScope(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	var carried []string
	for _, name := range state.Relationships {
		key := RelationshipKey(name)
		if !stringInSlice(RelationshipNames(run.Gate), name) {
			return nil, fmt.Errorf("baseline declares unknown relationship %q", name)
		}
		if stringInSlice(run.Relationships, name) || state.LogicalStatus[key] != "pass" || stringInSlice(scope.Affected, key) || len(scope.Unreadable) > 0 || len(scope.Untrackable) > 0 {
			run.Relationships = appendUnique(run.Relationships, name)
		} else {
			carried = append(carried, key)
		}
	}
	run.Relationships = dedupeSorted(run.Relationships)
	return dedupeSorted(carried), nil
}

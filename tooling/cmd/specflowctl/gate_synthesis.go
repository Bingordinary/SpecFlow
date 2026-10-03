package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func synthesisInputFindings(run *gaterun.Run, reports []reportRef) []gaterun.Finding {
	var findings []gaterun.Finding
	for _, report := range reports {
		if report.spec.Kind != gaterun.SessionKindCross {
			findings = append(findings, resultFindings(report.result)...)
		}
	}
	for i := range run.CarriedResults {
		findings = append(findings, resultFindings(&run.CarriedResults[i])...)
	}
	for _, deferred := range run.DeferredFindings {
		findings = append(findings, deferred.Finding)
	}
	return findings
}

// synthesisEvidenceKeys maps each affected judgment to the source judgments
// and final-synthesis evidence its decision consumes. Input findings are kept
// here even when suppression removes them from the canonical outcome. Final
// groups also include new findings, merge sources, and deferred ownership.
func synthesisEvidenceKeys(run *gaterun.Run, reports []reportRef, outcome *gateOutcome) map[string]map[string]bool {
	dependencies := map[string]map[string]bool{}
	if crossReport(reports) == nil {
		return dependencies
	}
	originalConclusions := map[string]string{}
	for _, report := range reports {
		if report.spec.Kind != gaterun.SessionKindCross {
			for key, verdict := range report.result.Verdicts {
				originalConclusions[key] = verdict
			}
		}
	}
	for _, result := range run.CarriedResults {
		for key, verdict := range result.Verdicts {
			originalConclusions[key] = verdict
		}
	}
	for key, conclusion := range outcome.QualityConclusions {
		if originalConclusions[key] != conclusion {
			dependencies[key] = map[string]bool{gaterun.CrossKey: true, key: true}
		}
	}
	findings := synthesisInputFindings(run, reports)
	findings = append(findings, outcome.Findings...)
	findings = append(findings, outcome.DeferredFindings...)
	for _, finding := range findings {
		keys := findingKeySet(finding)
		for key := range keys {
			// Pending findings routed from another unit may retain that run's
			// keys. Only the consuming run's judgments can own cache evidence.
			if _, current := outcome.EffectiveStatus[key]; !current {
				continue
			}
			if dependencies[key] == nil {
				dependencies[key] = map[string]bool{gaterun.CrossKey: true}
			}
			for source := range keys {
				dependencies[key][source] = true
			}
		}
	}
	return dependencies
}

// applyOwnerships validates the cross report's ownership records against the
// terminal retained findings and stamps each finding's OwnedBy. A finding
// without a record stays unassigned (empty) and drives this unit's gate; a
// record naming another unit defers the finding (see
// framework/verification_scope.md §Coverage Model → Deferred findings).
func applyOwnerships(findings []gaterun.Finding, ownerships []gaterun.FindingOwnership) ([]gaterun.Finding, error) {
	out := append([]gaterun.Finding(nil), findings...)
	index := make(map[string]int, len(out))
	for i, finding := range out {
		if _, exists := index[finding.ID]; exists {
			return nil, fmt.Errorf("retained finding %q appears more than once", finding.ID)
		}
		index[finding.ID] = i
	}
	seen := map[string]bool{}
	for _, ownership := range ownerships {
		i, ok := index[ownership.FindingID]
		if !ok {
			return nil, fmt.Errorf("ownership record names non-terminal finding %q", ownership.FindingID)
		}
		if seen[ownership.FindingID] {
			return nil, fmt.Errorf("finding %q declares ownership more than once", ownership.FindingID)
		}
		seen[ownership.FindingID] = true
		if strings.TrimSpace(ownership.OwnerUnit) == "" || strings.TrimSpace(ownership.EvidencePath) == "" || strings.TrimSpace(ownership.Reason) == "" {
			return nil, fmt.Errorf("ownership record for finding %q requires an owner unit, evidence, and reason", ownership.FindingID)
		}
		out[i].OwnedBy = ownership.OwnerUnit
	}
	return out, nil
}

// gateDriving reports whether a terminal retained finding drives the run's
// gate: unassigned findings and findings owned by the run's own unit do;
// findings owned by another unit are deferred — recorded, routed to the
// owner's verify run, and not blocking here.
func gateDriving(finding gaterun.Finding, unit string) bool {
	return finding.OwnedBy == "" || finding.OwnedBy == unit
}

// splitDeferred partitions terminal retained findings into the gate-driving
// set and the deferred set (owned by another unit). Order is preserved.
func splitDeferred(findings []gaterun.Finding, unit string) (driving, deferred []gaterun.Finding) {
	for _, finding := range findings {
		if gateDriving(finding, unit) {
			driving = append(driving, finding)
			continue
		}
		deferred = append(deferred, finding)
	}
	return driving, deferred
}

func severityRank(severity string) (int, bool) {
	switch severity {
	case "P0":
		return 0, true
	case "P1":
		return 1, true
	case "P2":
		return 2, true
	case "P3":
		return 3, true
	default:
		return 0, false
	}
}

// resultFindings preserves the logical keys bound to each finding at submit
// time. Sharing a reviewer session does not make two judgments related.
func resultFindings(result *gaterun.SessionResult) []gaterun.Finding {
	out := make([]gaterun.Finding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		finding.AffectedKeys = sortedFindingKeys(findingKeySet(finding), finding.SourceKey)
		out = append(out, finding)
	}
	return out
}

// resolveCrossFindings validates the complete disposition graph and returns
// the canonical retained findings. A merged finding is never removed: its
// logical keys flow to the terminal retained input or new cross finding.
func resolveCrossFindings(input, cross []gaterun.Finding, dispositions []gaterun.FindingDisposition) ([]gaterun.Finding, error) {
	inputByID := map[string]gaterun.Finding{}
	var inputOrder []string
	for _, finding := range input {
		if prior, ok := inputByID[finding.ID]; ok {
			if prior.Severity != finding.Severity || prior.Text != finding.Text || prior.Detail != finding.Detail || prior.SourceKey != finding.SourceKey {
				return nil, fmt.Errorf("input finding %q has conflicting canonical content", finding.ID)
			}
			keys := findingKeySet(prior)
			for key := range findingKeySet(finding) {
				keys[key] = true
			}
			prior.AffectedKeys = sortedFindingKeys(keys, prior.SourceKey)
			inputByID[finding.ID] = prior
			continue
		}
		inputByID[finding.ID] = finding
		inputOrder = append(inputOrder, finding.ID)
	}

	crossByID := map[string]gaterun.Finding{}
	var crossOrder []string
	for _, finding := range cross {
		if _, exists := inputByID[finding.ID]; exists {
			return nil, fmt.Errorf("new cross finding %q duplicates an input finding id", finding.ID)
		}
		if _, exists := crossByID[finding.ID]; exists {
			return nil, fmt.Errorf("new cross finding %q is declared more than once", finding.ID)
		}
		crossByID[finding.ID] = finding
		crossOrder = append(crossOrder, finding.ID)
	}

	dispositionByID := map[string]gaterun.FindingDisposition{}
	for _, disposition := range dispositions {
		if _, ok := inputByID[disposition.FindingID]; !ok {
			return nil, fmt.Errorf("cross report disposes unknown finding %q", disposition.FindingID)
		}
		if _, exists := dispositionByID[disposition.FindingID]; exists {
			return nil, fmt.Errorf("cross report disposes finding %q more than once", disposition.FindingID)
		}
		dispositionByID[disposition.FindingID] = disposition
	}
	for _, id := range inputOrder {
		if _, ok := dispositionByID[id]; !ok {
			return nil, fmt.Errorf("cross report does not dispose input finding %q", id)
		}
	}

	resolved := map[string]string{}
	visiting := map[string]bool{}
	var resolve func(string) (string, error)
	resolve = func(id string) (string, error) {
		if terminal, ok := resolved[id]; ok {
			return terminal, nil
		}
		if visiting[id] {
			return "", fmt.Errorf("finding merge graph contains a cycle at %q", id)
		}
		if _, ok := crossByID[id]; ok {
			resolved[id] = id
			return id, nil
		}
		disposition, ok := dispositionByID[id]
		if !ok {
			return "", fmt.Errorf("finding merge target %q does not exist", id)
		}
		switch disposition.Action {
		case "retained":
			resolved[id] = id
			return id, nil
		case "suppressed":
			resolved[id] = ""
			return "", nil
		case "merged":
			if disposition.TargetID == id {
				return "", fmt.Errorf("finding %q merges into itself", id)
			}
			if _, inputOK := inputByID[disposition.TargetID]; !inputOK {
				if _, crossOK := crossByID[disposition.TargetID]; !crossOK {
					return "", fmt.Errorf("finding %q merges into missing target %q", id, disposition.TargetID)
				}
			}
			visiting[id] = true
			terminal, err := resolve(disposition.TargetID)
			delete(visiting, id)
			if err != nil {
				return "", err
			}
			if terminal == "" {
				return "", fmt.Errorf("finding %q merge chain terminates at suppressed finding %q", id, disposition.TargetID)
			}
			resolved[id] = terminal
			return terminal, nil
		default:
			return "", fmt.Errorf("finding %q has invalid disposition %q", id, disposition.Action)
		}
	}

	groupKeys := map[string]map[string]bool{}
	// groupSeverity records the canonical severity of each terminal retained
	// group: the most severe grade among the findings retained or merged into
	// it (conservative — disagreement raises, never lowers).
	groupSeverity := map[string]string{}
	raiseSeverity := func(id, severity string) {
		current, ok := groupSeverity[id]
		if !ok {
			groupSeverity[id] = severity
			return
		}
		currentRank, currentOK := severityRank(current)
		newRank, newOK := severityRank(severity)
		if !currentOK || (newOK && newRank < currentRank) {
			groupSeverity[id] = severity
		}
	}
	for _, id := range inputOrder {
		terminal, err := resolve(id)
		if err != nil {
			return nil, err
		}
		if terminal == "" {
			continue
		}
		if groupKeys[terminal] == nil {
			groupKeys[terminal] = map[string]bool{}
		}
		for key := range findingKeySet(inputByID[id]) {
			groupKeys[terminal][key] = true
		}
		raiseSeverity(terminal, inputByID[id].Severity)
	}
	for _, id := range crossOrder {
		if groupKeys[id] == nil {
			groupKeys[id] = map[string]bool{}
		}
		for key := range findingKeySet(crossByID[id]) {
			groupKeys[id][key] = true
		}
		raiseSeverity(id, crossByID[id].Severity)
	}

	var out []gaterun.Finding
	for _, id := range inputOrder {
		if dispositionByID[id].Action != "retained" {
			continue
		}
		finding := withRaisedSeverity(inputByID[id], groupSeverity[id])
		finding.AffectedKeys = sortedFindingKeys(groupKeys[id], finding.SourceKey)
		out = append(out, finding)
	}
	for _, id := range crossOrder {
		finding := withRaisedSeverity(crossByID[id], groupSeverity[id])
		finding.AffectedKeys = sortedFindingKeys(groupKeys[id], finding.SourceKey)
		out = append(out, finding)
	}
	return out, nil
}

// withRaisedSeverity applies the canonical (conservatively raised) severity of
// a finding's terminal retain/merge group. When it raises the grade it rewrites
// the stored detail's leading [Px] prefix so the human-readable block and the
// machine severity agree.
func withRaisedSeverity(finding gaterun.Finding, severity string) gaterun.Finding {
	return finding.WithMinimumSeverity(severity)
}

func findingKeySet(finding gaterun.Finding) map[string]bool {
	keys := map[string]bool{}
	if finding.SourceKey != "" && finding.SourceKey != gaterun.CrossKey {
		keys[finding.SourceKey] = true
	}
	for _, key := range finding.AffectedKeys {
		if key != "" && key != gaterun.CrossKey {
			keys[key] = true
		}
	}
	return keys
}

func sortedFindingKeys(keys map[string]bool, sourceKey string) []string {
	values := make([]string, 0, len(keys))
	for key := range keys {
		if key != sourceKey && key != gaterun.CrossKey {
			values = append(values, key)
		}
	}
	sort.Strings(values)
	return values
}

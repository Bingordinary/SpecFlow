package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func applySeverityConfirmations(findings []gaterun.Finding, confirmations []gaterun.SeverityConfirmation) ([]gaterun.Finding, error) {
	out := append([]gaterun.Finding(nil), findings...)
	index := make(map[string]int, len(out))
	for i, finding := range out {
		if _, exists := index[finding.ID]; exists {
			return nil, fmt.Errorf("retained finding %q appears more than once", finding.ID)
		}
		index[finding.ID] = i
	}
	sequences := make(map[string][]gaterun.SeverityConfirmation, len(out))
	for _, confirmation := range confirmations {
		_, ok := index[confirmation.FindingID]
		if !ok {
			return nil, fmt.Errorf("severity confirmation names non-terminal finding %q", confirmation.FindingID)
		}
		sequences[confirmation.FindingID] = append(sequences[confirmation.FindingID], confirmation)
		if len(sequences[confirmation.FindingID]) > 2 {
			return nil, fmt.Errorf("finding %q has more than two severity confirmation records", confirmation.FindingID)
		}
	}
	for id, i := range index {
		sequence := sequences[id]
		if len(sequence) == 0 {
			return nil, fmt.Errorf("terminal retained finding %q has no severity confirmation", id)
		}
		if len(sequence) == 2 && sequence[0].Outcome != "adjusted" {
			return nil, fmt.Errorf("finding %q has a second severity confirmation after a final confirmed result", id)
		}
		initial := out[i].Severity
		current := initial
		for step, confirmation := range sequence {
			if confirmation.OriginalSeverity != current {
				return nil, fmt.Errorf("severity confirmation for finding %q starts at %s, want %s", id, confirmation.OriginalSeverity, current)
			}
			switch confirmation.Outcome {
			case "confirmed":
				if confirmation.FinalSeverity != current {
					return nil, fmt.Errorf("confirmed severity for finding %q changes %s to %s", id, current, confirmation.FinalSeverity)
				}
				if step != len(sequence)-1 {
					return nil, fmt.Errorf("confirmed severity for finding %q is final and cannot be followed by another record", id)
				}
			case "adjusted":
				from, fromOK := severityRank(current)
				to, toOK := severityRank(confirmation.FinalSeverity)
				if !fromOK || !toOK || from-to > 1 || to-from > 1 || from == to {
					return nil, fmt.Errorf("adjusted severity for finding %q must move exactly one level (got %s -> %s)", id, current, confirmation.FinalSeverity)
				}
				current = confirmation.FinalSeverity
				if step == 0 && len(sequence) != 2 {
					return nil, fmt.Errorf("adjusted severity for finding %q requires exactly one final second confirmation", id)
				}
			case "":
				return nil, fmt.Errorf("severity confirmation for finding %q has no outcome", id)
			default:
				return nil, fmt.Errorf("severity confirmation for finding %q has invalid outcome %q", id, confirmation.Outcome)
			}
		}
		out[i].Detail = rewriteFindingDetailSeverity(out[i].Detail, initial, current)
		out[i].Severity = current
	}
	return out, nil
}

func rewriteFindingDetailSeverity(detail, from, to string) string {
	if from == to || detail == "" {
		return detail
	}
	lineEnd := strings.IndexByte(detail, '\n')
	if lineEnd < 0 {
		lineEnd = len(detail)
	}
	needle := "[" + from + "]"
	idx := strings.Index(detail[:lineEnd], needle)
	if idx < 0 {
		return detail
	}
	return detail[:idx] + "[" + to + "]" + detail[idx+len(needle):]
}

// applyOwnerships validates the cross report's ownership records against the
// terminal retained findings and stamps each finding's OwnedBy. A finding
// without a record stays unassigned (empty) and drives this unit's gate; a
// record naming another unit defers the finding (see
// framework/verification_scope.md §Gate Work Packets → Deferred findings).
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
// owner's review, and not blocking here.
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

// resultFindings returns a result's findings with every mechanically known
// logical source attached. Single-key packets already carry that key as the
// finding source; multi-key packets additionally contribute each blocking
// verdict key so merge synthesis cannot lose the judgment that raised a
// blocking finding.
func resultFindings(result *gaterun.PacketResult) []gaterun.Finding {
	out := make([]gaterun.Finding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		keys := map[string]bool{}
		for _, key := range finding.AffectedKeys {
			if key != "" && key != gaterun.CrossKey {
				keys[key] = true
			}
		}
		if finding.SourceKey != result.PacketID || len(result.Verdicts) <= 1 {
			if finding.SourceKey != "" && finding.SourceKey != gaterun.CrossKey {
				keys[finding.SourceKey] = true
			}
		}
		for key, verdict := range result.Verdicts {
			if verdict == "FAIL" || verdict == "MISMATCH" || verdict == "unacceptable" {
				keys[key] = true
			}
		}
		if finding.SourceKey == result.PacketID && len(result.Verdicts) > 1 {
			logicalKeys := sortedFindingKeys(keys, "")
			if len(logicalKeys) > 0 {
				finding.SourceKey = logicalKeys[0]
			}
		}
		finding.AffectedKeys = sortedFindingKeys(keys, finding.SourceKey)
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
	}
	for _, id := range crossOrder {
		if groupKeys[id] == nil {
			groupKeys[id] = map[string]bool{}
		}
		for key := range findingKeySet(crossByID[id]) {
			groupKeys[id][key] = true
		}
	}

	var out []gaterun.Finding
	for _, id := range inputOrder {
		if dispositionByID[id].Action != "retained" {
			continue
		}
		finding := inputByID[id]
		finding.AffectedKeys = sortedFindingKeys(groupKeys[id], finding.SourceKey)
		out = append(out, finding)
	}
	for _, id := range crossOrder {
		finding := crossByID[id]
		finding.AffectedKeys = sortedFindingKeys(groupKeys[id], finding.SourceKey)
		out = append(out, finding)
	}
	return out, nil
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

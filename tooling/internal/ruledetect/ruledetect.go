// Package ruledetect implements the rule detection primitive behind
// `specflowctl detect` and the `remove --rule` verification. It reports, for
// a rule, its current-layer (effective) explicit consumers — candidate
// preferred, stable fallback, matching `specflowctl deps` — and whether the
// rule declares `unbound_retention` (intentional retention, exempt from
// removal). A rule is removable when it has no consumers and no retention
// declaration.
//
// ErrRuleNotFound marks a rule that has no file in either layer. It lets
// `remove --rule` distinguish "nothing to protect" (degrade to metadata
// cleanup, keeping the command re-entrant as the recovery path after a
// partial deletion) from a real read failure.
//
// The removal-candidate model applies to bound rules (b_rule_*): they are the
// rules that can be unbound and carry the unbound_retention exemption. Global
// rules (g_rule_*) apply to every unit by default, so "no consumers" is not a
// meaningful state for them — they are never listed as removable candidates
// and are removed only by an explicit user-invoked `remove --rule`, whose
// verification checks effective explicit references only (the default
// applicability lifts automatically when the rule file disappears).
package ruledetect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulerefs"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitgraph"
)

// ErrRuleNotFound reports that the rule has no file in the candidate or
// stable layer.
var ErrRuleNotFound = errors.New("rule not found")

// DetectResult describes one rule's removal-readiness state.
type DetectResult struct {
	RuleID           string
	HasCandidate     bool
	HasStable        bool
	Consumers        []string // current-layer explicit rule_refs consumers
	UnboundRetention bool     // rule declares unbound_retention: intentional
	Removable        bool     // no consumers and no retention declaration
}

// DetectRule reports the removal-readiness of one rule. It returns an error
// when the rule file exists in neither the candidate nor the stable layer.
// For a global rule (g_rule_*) the consumers are the effective explicit
// references only — the default applicability to every unit lifts
// automatically when the rule file disappears, so it does not block removal.
func DetectRule(repoRoot, ruleID string) (*DetectResult, error) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return nil, fmt.Errorf("rule id is required")
	}
	if _, err := effectiveRuleFile(repoRoot, ruleID); err != nil {
		return nil, err
	}
	consumers, err := rulerefs.FindExplicitRuleConsumers(repoRoot, ruleID)
	if err != nil {
		return nil, err
	}
	hasCandidate, err := ruleExists(repoRoot, specpaths.RuleCandidateFileRef(ruleID))
	if err != nil {
		return nil, err
	}
	hasStable, err := ruleExists(repoRoot, specpaths.RuleStableFileRef(ruleID))
	if err != nil {
		return nil, err
	}
	unbound, err := hasUnboundRetention(repoRoot, ruleID)
	if err != nil {
		return nil, err
	}
	return &DetectResult{
		RuleID:           ruleID,
		HasCandidate:     hasCandidate,
		HasStable:        hasStable,
		Consumers:        consumers,
		UnboundRetention: unbound,
		Removable:        len(consumers) == 0 && !unbound,
	}, nil
}

// DetectAll reports the removal-readiness of every bound rule in the
// candidate and stable layers (union, sorted by rule id).
func DetectAll(repoRoot string) ([]DetectResult, error) {
	ids, err := allRuleIDs(repoRoot)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	graph, err := unitgraph.Build(repoRoot, "all")
	if err != nil {
		return nil, err
	}
	consumersByRule := map[string][]string{}
	for _, node := range graph.Nodes() {
		for _, ref := range node.RuleRefs {
			consumersByRule[ref] = append(consumersByRule[ref], node.Name)
		}
	}

	results := make([]DetectResult, 0, len(ids))
	for _, id := range ids {
		consumers := consumersByRule[id]
		sort.Strings(consumers)
		unbound, err := hasUnboundRetention(repoRoot, id)
		if err != nil {
			return nil, err
		}
		hasCandidate, err := ruleExists(repoRoot, specpaths.RuleCandidateFileRef(id))
		if err != nil {
			return nil, err
		}
		hasStable, err := ruleExists(repoRoot, specpaths.RuleStableFileRef(id))
		if err != nil {
			return nil, err
		}
		results = append(results, DetectResult{
			RuleID:           id,
			HasCandidate:     hasCandidate,
			HasStable:        hasStable,
			Consumers:        consumers,
			UnboundRetention: unbound,
			Removable:        len(consumers) == 0 && !unbound,
		})
	}
	return results, nil
}

// isBoundRule reports whether the rule id is a bound rule (b_rule_*).
func isBoundRule(ruleID string) bool {
	return strings.HasPrefix(ruleID, "b_rule_")
}

// allRuleIDs returns the union of candidate and stable rule ids, sorted.
func allRuleIDs(repoRoot string) ([]string, error) {
	seen := map[string]bool{}
	var ids []string
	for _, dirRef := range []string{specpaths.RuleCandidateDir, specpaths.RuleStableDir} {
		matches, err := filepath.Glob(filepath.Join(repoRoot, filepath.FromSlash(dirRef), "*.md"))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			id := strings.TrimSuffix(filepath.Base(m), ".md")
			if !isBoundRule(id) {
				continue
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// effectiveRuleFile resolves the rule file to its current-layer file
// (candidate preferred, stable fallback) and reports the path.
func effectiveRuleFile(repoRoot, ruleID string) (string, error) {
	for _, ref := range []string{specpaths.RuleCandidateFileRef(ruleID), specpaths.RuleStableFileRef(ruleID)} {
		path := filepath.Join(repoRoot, filepath.FromSlash(ref))
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: rule %s not found in docs/specs/rules/ (candidate or stable)", ErrRuleNotFound, ruleID)
}

// ruleExists reports whether the rule file exists at the given layer ref.
func ruleExists(repoRoot, ref string) (bool, error) {
	_, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(ref)))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// hasUnboundRetention reports whether the rule's current-layer file declares
// `unbound_retention` (intentional retention, exempt from removal).
func hasUnboundRetention(repoRoot, ruleID string) (bool, error) {
	path, err := effectiveRuleFile(repoRoot, ruleID)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read rule file: %w", err)
	}
	fm := specpaths.ReadFrontmatterStringMap(string(data))
	return strings.TrimSpace(fm["unbound_retention"]) != "", nil
}

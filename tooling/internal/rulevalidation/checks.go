package rulevalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulerefs"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func readRuleFile(repoRoot, ruleID string) (string, error) {
	path := filepath.Join(repoRoot, specpaths.RuleCandidateFileRef(ruleID))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read candidate rule: %v", err)
	}
	return string(data), nil
}

func frontmatterKeys(repoRoot, ruleID string) map[string]string {
	content, err := readRuleFile(repoRoot, ruleID)
	if err != nil {
		return nil
	}
	return specpaths.ReadFrontmatterStringMap(content)
}

func candidateRulePath(repoRoot, ruleID string) string {
	return filepath.Join(repoRoot, specpaths.RuleCandidateFileRef(ruleID))
}

func stableRulePath(repoRoot, ruleID string) string {
	return filepath.Join(repoRoot, specpaths.RuleStableFileRef(ruleID))
}

func checkFrontmatter(repoRoot, ruleID string) CheckResult {
	path := candidateRulePath(repoRoot, ruleID)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Frontmatter completeness",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate rule: %v", err),
		}
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))

	required := []struct {
		field string
		label string
	}{
		{"rule_id", "rule_id"},
		{"rule_scope", "rule_scope"},
		{"rule_version", "rule_version"},
	}

	var missing []string
	for _, r := range required {
		if strings.TrimSpace(fm[r.field]) == "" {
			missing = append(missing, r.label)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:    "Frontmatter completeness",
			Status:  Fail,
			Details: fmt.Sprintf("missing required fields: %s", strings.Join(missing, ", ")),
		}
	}

	return CheckResult{Name: "Frontmatter completeness", Status: Pass}
}

func checkIDScopeConsistency(repoRoot, ruleID string) CheckResult {
	fm := frontmatterKeys(repoRoot, ruleID)
	if fm == nil {
		return CheckResult{
			Name:    "ID/Scope consistency",
			Status:  Fail,
			Details: "cannot read frontmatter",
		}
	}

	scope := strings.TrimSpace(fm["rule_scope"])
	if scope == "" {
		return CheckResult{
			Name:    "ID/Scope consistency",
			Status:  Fail,
			Details: "rule_scope is missing",
		}
	}

	hasGlobalPrefix := strings.HasPrefix(ruleID, "g_rule_")
	hasBoundPrefix := strings.HasPrefix(ruleID, "b_rule_")

	if scope == "global" && !hasGlobalPrefix {
		return CheckResult{
			Name:    "ID/Scope consistency",
			Status:  Fail,
			Details: fmt.Sprintf("rule_scope is %q but rule_id %q should start with g_rule_", scope, ruleID),
		}
	}

	if scope == "bound" && !hasBoundPrefix {
		return CheckResult{
			Name:    "ID/Scope consistency",
			Status:  Fail,
			Details: fmt.Sprintf("rule_scope is %q but rule_id %q should start with b_rule_", scope, ruleID),
		}
	}

	if scope != "global" && scope != "bound" {
		return CheckResult{
			Name:    "ID/Scope consistency",
			Status:  Fail,
			Details: fmt.Sprintf("invalid rule_scope %q; must be 'global' or 'bound'", scope),
		}
	}

	return CheckResult{Name: "ID/Scope consistency", Status: Pass}
}

func isVersionGreater(v1, v2 string) bool {
	var m1, n1, p1, m2, n2, p2 int
	if _, err := fmt.Sscanf(v1, "%d.%d.%d", &m1, &n1, &p1); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(v2, "%d.%d.%d", &m2, &n2, &p2); err != nil {
		return false
	}
	if m1 != m2 {
		return m1 > m2
	}
	if n1 != n2 {
		return n1 > n2
	}
	return p1 > p2
}

func checkVersionSemantics(repoRoot, ruleID string) CheckResult {
	fm := frontmatterKeys(repoRoot, ruleID)
	if fm == nil {
		return CheckResult{
			Name:    "Version semantics",
			Status:  Fail,
			Details: "cannot read frontmatter",
		}
	}

	// A retiring rule is removed from stable, not versioned — the candidate
	// version needs no comparison against the stable version.
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Version semantics",
			Status:  Pass,
			Details: "rule is marked retired — version comparison skipped",
		}
	}

	candidateVersion := strings.TrimSpace(fm["rule_version"])
	if candidateVersion == "" {
		return CheckResult{
			Name:    "Version semantics",
			Status:  Fail,
			Details: "rule_version is missing",
		}
	}

	stablePath := stableRulePath(repoRoot, ruleID)
	if _, err := os.Stat(stablePath); os.IsNotExist(err) {
		if candidateVersion != "0.1.0" {
			return CheckResult{
				Name:    "Version semantics",
				Status:  Fail,
				Details: fmt.Sprintf("new rule must start at 0.1.0, got %s", candidateVersion),
			}
		}
		return CheckResult{Name: "Version semantics", Status: Pass}
	}

	stableData, err := os.ReadFile(stablePath)
	if err != nil {
		return CheckResult{
			Name:    "Version semantics",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read stable rule: %v", err),
		}
	}
	stableFM := specpaths.ReadFrontmatterStringMap(string(stableData))
	stableVersion := strings.TrimSpace(stableFM["rule_version"])
	if stableVersion == "" {
		return CheckResult{
			Name:    "Version semantics",
			Status:  Fail,
			Details: "cannot read stable rule_version",
		}
	}

	if !isVersionGreater(candidateVersion, stableVersion) {
		return CheckResult{
			Name:    "Version semantics",
			Status:  Fail,
			Details: fmt.Sprintf("candidate version %s must be greater than stable version %s", candidateVersion, stableVersion),
		}
	}

	return CheckResult{Name: "Version semantics", Status: Pass}
}

func checkPromotionOwner(repoRoot, ruleID string) CheckResult {
	fm := frontmatterKeys(repoRoot, ruleID)
	if fm == nil {
		return CheckResult{
			Name:    "promotion_owner_unit",
			Status:  Pass,
			Details: "cannot read frontmatter (skipped)",
		}
	}

	owner := strings.TrimSpace(fm["promotion_owner_unit"])
	if owner == "" {
		return CheckResult{
			Name:    "promotion_owner_unit",
			Status:  Pass,
			Details: "not present (optional)",
		}
	}

	unitCandidate := filepath.Join(repoRoot, "docs/specs/units/candidate", fmt.Sprintf("unit_%s.md", owner))
	unitStable := filepath.Join(repoRoot, "docs/specs/units/stable", fmt.Sprintf("unit_%s.md", owner))

	if _, err := os.Stat(unitCandidate); err == nil {
		return CheckResult{Name: "promotion_owner_unit", Status: Pass, Details: fmt.Sprintf("references unit %s (candidate)", owner)}
	}
	if _, err := os.Stat(unitStable); err == nil {
		return CheckResult{Name: "promotion_owner_unit", Status: Pass, Details: fmt.Sprintf("references unit %s (stable)", owner)}
	}

	return CheckResult{
		Name:    "promotion_owner_unit",
		Status:  Warn,
		Details: fmt.Sprintf("references unit %q which does not exist in docs/specs/units/", owner),
	}
}

func checkProhibitedFields(repoRoot, ruleID string) CheckResult {
	fm := frontmatterKeys(repoRoot, ruleID)
	if fm == nil {
		return CheckResult{
			Name:    "Prohibited fields",
			Status:  Fail,
			Details: "cannot read frontmatter",
		}
	}

	if _, ok := fm["bound_objects"]; ok {
		return CheckResult{
			Name:    "Prohibited fields",
			Status:  Fail,
			Details: "bound_objects is forbidden in rule files; consumers are derived from unit rule_refs",
		}
	}

	return CheckResult{Name: "Prohibited fields", Status: Pass}
}

func checkUnboundRetention(repoRoot, ruleID string) CheckResult {
	fm := frontmatterKeys(repoRoot, ruleID)
	if fm == nil {
		return CheckResult{
			Name:    "unbound_retention correctness",
			Status:  Fail,
			Details: "cannot read frontmatter",
		}
	}

	// A retiring rule is removed from stable. Every explicit rule_refs
	// reference must be dropped first (a global rule's default applicability
	// lifts automatically when the rule file disappears, so only explicit
	// references are checked); the unbound_retention fields are not required
	// for a rule that is going away.
	if strings.TrimSpace(fm["status"]) == "retired" {
		consumers, err := rulerefs.FindExplicitRuleConsumers(repoRoot, ruleID)
		if err != nil {
			return CheckResult{
				Name:    "unbound_retention correctness",
				Status:  Fail,
				Details: fmt.Sprintf("cannot search for consumers: %v", err),
			}
		}
		if len(consumers) > 0 {
			return CheckResult{
				Name:    "unbound_retention correctness",
				Status:  Fail,
				Details: fmt.Sprintf("rule is marked retired but still referenced in rule_refs by: %s — remove the references before retiring", strings.Join(consumers, ", ")),
			}
		}
		return CheckResult{
			Name:    "unbound_retention correctness",
			Status:  Pass,
			Details: "rule is marked retired — no rule_refs references remain",
		}
	}

	if !strings.HasPrefix(ruleID, "b_rule_") {
		return CheckResult{
			Name:    "unbound_retention correctness",
			Status:  Pass,
			Details: "not a bound rule (b_rule_) — skipped",
		}
	}

	consumers, err := rulerefs.FindRuleConsumers(repoRoot, ruleID)
	if err != nil {
		return CheckResult{
			Name:    "unbound_retention correctness",
			Status:  Fail,
			Details: fmt.Sprintf("cannot search for consumers: %v", err),
		}
	}

	hasUnbound := strings.TrimSpace(fm["unbound_retention"]) != ""
	hasUnboundReason := strings.TrimSpace(fm["unbound_retention_reason"]) != ""
	hasUnboundOwner := strings.TrimSpace(fm["unbound_retention_owner"]) != ""

	if len(consumers) == 0 {
		missing := make([]string, 0, 3)
		if !hasUnbound {
			missing = append(missing, "unbound_retention")
		}
		if !hasUnboundReason {
			missing = append(missing, "unbound_retention_reason")
		}
		if !hasUnboundOwner {
			missing = append(missing, "unbound_retention_owner")
		}
		if len(missing) > 0 {
			return CheckResult{
				Name:    "unbound_retention correctness",
				Status:  Fail,
				Details: fmt.Sprintf("b_rule_ with no consumers requires fields: %s", strings.Join(missing, ", ")),
			}
		}
	} else {
		if hasUnbound || hasUnboundReason || hasUnboundOwner {
			return CheckResult{
				Name:    "unbound_retention correctness",
				Status:  Fail,
				Details: "b_rule_ with consumers must not have unbound_retention fields",
			}
		}
	}

	return CheckResult{Name: "unbound_retention correctness", Status: Pass}
}

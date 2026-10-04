package rulevalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

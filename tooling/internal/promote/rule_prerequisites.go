package promote

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// RulePrerequisite explains one unpublished or missing rule dependency.
type RulePrerequisite struct {
	RuleID string
	Reason string
}

// RulePrerequisites is a read-only publication check, separate from gate caches.
type RulePrerequisites struct {
	Blockers   []RulePrerequisite
	Advisories []RulePrerequisite
}

// CheckUnitRulePrerequisites checks the candidate unit's direct rule_refs and
// unpublished global rules. It never advances rules or changes gate evidence.
// Scope follows the same ID-based applicability as specpaths.ResolveRuleFile;
// rule validation independently enforces ID/scope consistency.
func CheckUnitRulePrerequisites(repoRoot, unitName string) (RulePrerequisites, error) {
	var report RulePrerequisites
	if err := specpaths.ValidateTargetName("unit", unitName); err != nil {
		return report, err
	}
	unitPath := filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateUnitSpecFileRef(unitName)))
	data, err := os.ReadFile(unitPath)
	if err != nil {
		return report, fmt.Errorf("read %s: %w", unitPath, err)
	}
	fm, _, err := specpaths.ParseFrontmatterFields(string(data))
	if err != nil {
		return report, fmt.Errorf("read %s: %w", unitPath, err)
	}

	refs := map[string]bool{}
	if raw := fm["rule_refs"]; raw != "" && !strings.EqualFold(raw, "none") {
		for _, id := range specpaths.ParseRefList(raw) {
			if err := specpaths.ValidateTargetName("rule", id); err != nil {
				return report, fmt.Errorf("%s rule_refs: %w", unitPath, err)
			}
			refs[id] = true
		}
	}
	ids := make([]string, 0, len(refs))
	for id := range refs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	blocked := map[string]bool{}
	for _, id := range ids {
		candidateHash, stableHash, err := ruleLayerHashes(repoRoot, id)
		if err != nil {
			return report, err
		}
		reason := ""
		switch {
		case stableHash == "" && candidateHash == "":
			reason = "rule_refs target does not exist in stable or candidate"
		case stableHash == "":
			reason = "rule_refs target exists only in candidate layer — promote it first"
		case !strings.HasPrefix(id, "g_rule_") && candidateHash != "" && candidateHash != stableHash:
			reason = "candidate content differs from stable — promote the rule first"
		}
		if reason != "" {
			report.Blockers = append(report.Blockers, RulePrerequisite{RuleID: id, Reason: reason})
			blocked[id] = true
		}
	}

	dir := filepath.Join(repoRoot, filepath.FromSlash(specpaths.RuleCandidateDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("read %s: %w", dir, err)
	}
	// ReadDir sorts by filename. Only global candidates are discovery inputs;
	// unrelated bound candidates are neither read nor publication prerequisites.
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "g_rule_") || !strings.HasSuffix(name, ".md") {
			continue
		}
		id := strings.TrimSuffix(name, ".md")
		if blocked[id] {
			continue
		}
		candidateHash, stableHash, err := ruleLayerHashes(repoRoot, id)
		if err != nil {
			return report, err
		}
		if candidateHash != "" && candidateHash != stableHash {
			report.Advisories = append(report.Advisories, RulePrerequisite{
				RuleID: id,
				Reason: "unpublished global draft — publish it first if this unit's current round will adopt it",
			})
		}
	}
	return report, nil
}

func ruleLayerHashes(repoRoot, ruleID string) (candidate, stable string, err error) {
	candidate, err = optionalRuleHash(filepath.Join(repoRoot, filepath.FromSlash(specpaths.RuleCandidateFileRef(ruleID))))
	if err != nil {
		return "", "", err
	}
	stable, err = optionalRuleHash(filepath.Join(repoRoot, filepath.FromSlash(specpaths.RuleStableFileRef(ruleID))))
	return candidate, stable, err
}

func optionalRuleHash(path string) (string, error) {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	hash, err := specpaths.FileHash(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return hash, nil
}

package rulebinding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulerefs"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

type ResolvedRef struct {
	VersionRef  string
	FileRef     string
	Layer       string
	RuleID      string
	RuleScope   string
	RuleVersion string
	Content     string
}

func ResolveRef(repoRoot, moduleLayer, ref string) (ResolvedRef, error) {
	prefix, _, err := splitVersionRef(ref)
	if err != nil {
		return ResolvedRef{}, err
	}

	var layer string
	var fileRef string
	var content string

	if moduleLayer == "stable" {
		layer = "stable"
		fileRef = ruleFileRef(prefix, layer)
		contentBytes, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
		if err != nil {
			return ResolvedRef{}, fmt.Errorf("rule %s not found in stable: %w", prefix, err)
		}
		content = string(contentBytes)
	} else {
		fileRef = ruleFileRef(prefix, "candidate")
		contentBytes, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
		if err == nil {
			layer = "candidate"
			content = string(contentBytes)
		} else {
			fileRef = ruleFileRef(prefix, "stable")
			contentBytes, err = os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
			if err != nil {
				return ResolvedRef{}, fmt.Errorf("rule %s not found in candidate or stable", prefix)
			}
			layer = "stable"
			content = string(contentBytes)
		}
	}

	hasBoundObjects, err := rulerefs.HasRuleBoundObjects(fileRef, content)
	if err != nil {
		return ResolvedRef{}, err
	}
	if hasBoundObjects {
		return ResolvedRef{}, fmt.Errorf("%s: bound_objects is forbidden; derive consumers from current-layer rule_refs", fileRef)
	}

	frontmatter, err := parseFrontmatter(content)
	if err != nil {
		return ResolvedRef{}, fmt.Errorf("%s: %w", fileRef, err)
	}
	ruleID := strings.TrimSpace(frontmatter["rule_id"])
	ruleScope := strings.TrimSpace(frontmatter["rule_scope"])
	actualLayer := strings.TrimSpace(frontmatter["layer"])
	actualVersion := strings.TrimSpace(frontmatter["rule_version"])
	if ruleID == "" || ruleScope == "" || actualLayer == "" || actualVersion == "" {
		return ResolvedRef{}, fmt.Errorf("%s: missing rule_id/rule_scope/layer/rule_version", fileRef)
	}
	if ruleScope != "global" && ruleScope != "bound" {
		return ResolvedRef{}, fmt.Errorf("%s: rule_scope must be global or bound", fileRef)
	}
	if actualLayer != layer {
		return ResolvedRef{}, fmt.Errorf("%s: frontmatter.layer=%s does not match bound layer %s", fileRef, actualLayer, layer)
	}
	if err := ValidatePromotionOwnerUnit(repoRoot, fileRef, actualLayer, strings.TrimSpace(frontmatter["promotion_owner_unit"])); err != nil {
		return ResolvedRef{}, err
	}

	return ResolvedRef{
		VersionRef:  prefix,
		FileRef:     fileRef,
		Layer:       actualLayer,
		RuleID:      ruleID,
		RuleScope:   ruleScope,
		RuleVersion: actualVersion,
		Content:     content,
	}, nil
}

func ValidatePromotionOwnerUnit(repoRoot, fileRef, layer, promotionOwnerUnit string) error {
	owner := strings.TrimSpace(promotionOwnerUnit)
	if layer != "candidate" {
		if owner != "" {
			return fmt.Errorf("%s: promotion_owner_unit is allowed only on candidate-layer rule files with a stable sibling", fileRef)
		}
		return nil
	}

	stableSiblingRef := filepath.Join("docs/specs/rules/stable", filepath.Base(fileRef))
	_, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(stableSiblingRef)))
	hasStableSibling := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", stableSiblingRef, err)
	}
	if !hasStableSibling {
		if owner != "" {
			return fmt.Errorf("%s: promotion_owner_unit must not be recorded when no stable-layer sibling exists", fileRef)
		}
		return nil
	}
	if owner == "" {
		return fmt.Errorf("%s: missing promotion_owner_unit for candidate-layer rule file with stable sibling %s", fileRef, stableSiblingRef)
	}
	// Check that the unit exists as a formal spec file (file existence is state)
	stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/s_unit_%s.md", owner))
	candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/c_unit_%s.md", owner))
	if _, err := os.Stat(stablePath); os.IsNotExist(err) {
		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			return fmt.Errorf("%s: promotion_owner_unit %q has no spec file in stable or candidate", fileRef, owner)
		}
	}
	return nil
}

func splitVersionRef(ref string) (string, string, error) {
	ref = strings.TrimSpace(ref)
	if strings.Contains(ref, "@") {
		return "", "", fmt.Errorf("rule ref %q must not include a version number; use bare rule name", ref)
	}
	if ref == "" {
		return "", "", fmt.Errorf("invalid rule ref %q", ref)
	}
	return ref, "", nil
}

func ruleFileRef(prefix, layer string) string {
	return fmt.Sprintf("docs/specs/rules/%s/%s.md", layer, prefix)
}

func parseFrontmatter(content string) (map[string]string, error) {
	fm, _, err := specpaths.ParseFrontmatterFields(content)
	return fm, err
}

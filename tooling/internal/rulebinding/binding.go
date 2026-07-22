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

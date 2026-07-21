package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// specPath is a shorthand for specpaths.MainSpecFileRef("candidate", unitName).
// It produces docs/specs/units/candidate/c_unit_<unitName>.md.
func specPath(repoRoot, unitName string) string {
	ref, err := specpaths.MainSpecFileRef("candidate", unitName)
	if err != nil {
		// candidate is always a supported layer; err is unreachable here.
		panic(err)
	}
	return filepath.Join(repoRoot, ref)
}

// ------------------------------------------------------------
// Check 1: Frontmatter completeness
// ------------------------------------------------------------
func checkFrontmatter(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Frontmatter completeness",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))

	required := []struct {
		field string
		label string
	}{
		{"id", "id"},
		{"layer", "layer"},
		{"version", "version"},
		{"unit_refs", "unit_refs"},
		{"rule_refs", "rule_refs"},
	}

	var missing []string
	for _, r := range required {
		if strings.TrimSpace(fm[r.field]) == "" {
			missing = append(missing, r.label)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:   "Frontmatter completeness",
			Status: Fail,
			Details: fmt.Sprintf("missing required fields: %s", strings.Join(missing, ", ")),
		}
	}

	if fm["id"] != unitName {
		return CheckResult{
			Name:    "Frontmatter completeness",
			Status:  Fail,
			Details: fmt.Sprintf("frontmatter id %q does not match unit name %q", fm["id"], unitName),
		}
	}

	if !strings.EqualFold(fm["layer"], "candidate") {
		return CheckResult{
			Name:    "Frontmatter completeness",
			Status:  Fail,
			Details: fmt.Sprintf("layer must be 'candidate', got %q", fm["layer"]),
		}
	}

	return CheckResult{Name: "Frontmatter completeness", Status: Pass}
}

// ------------------------------------------------------------
// Check 2: Acceptance items format
// ------------------------------------------------------------
func checkAcceptanceItems(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	content := string(data)

	if !strings.Contains(content, "acceptance_item_set:") {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: "acceptance_item_set not found",
		}
	}

	requiredItemFields := []string{
		"id:",
		"description:",
		"verification_type:",
		"verification_surface:",
		"implementation_surface:",
		"verification_method:",
		"pass_condition:",
		"not_runnable_yet:",
	}

	itemBlocks := strings.Count(content, "\n  - id:")
	if itemBlocks == 0 {
		itemBlocks = strings.Count(content, "- id:")
	}
	if itemBlocks == 0 {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: "acceptance_item_set exists but no items found with - id:",
		}
	}

	itemSection := content[strings.Index(content, "acceptance_item_set"):]
	if strings.Contains(itemSection, "\n---") {
		itemSection = itemSection[:strings.Index(itemSection, "\n---")]
	}

	var missingFields []string
	for _, field := range requiredItemFields {
		if !strings.Contains(itemSection, field) {
			missingFields = append(missingFields, strings.TrimSuffix(field, ":"))
		}
	}

	if len(missingFields) > 0 {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: fmt.Sprintf("%d item(s) found, but missing fields in item section: %s", itemBlocks, strings.Join(missingFields, ", ")),
		}
	}

	// Validate not_runnable_yet values (must be "yes" or "no" per spec_writing_guide.md)
	itemLines := strings.Split(itemSection, "\n")
	var invalidValues []string
	for _, line := range itemLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "not_runnable_yet:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				value := strings.TrimSpace(parts[1])
				if value != "yes" && value != "no" {
					invalidValues = append(invalidValues, fmt.Sprintf("%q", value))
				}
			}
		}
	}
	if len(invalidValues) > 0 {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: fmt.Sprintf("invalid not_runnable_yet value(s): %s; must be 'yes' or 'no'", strings.Join(invalidValues, ", ")),
		}
	}

	return CheckResult{
		Name:    "Acceptance items",
		Status:  Pass,
		Details: fmt.Sprintf("%d item(s) found with required fields", itemBlocks),
	}
}

// ------------------------------------------------------------
// Check 3: Anchor integrity (affects.files paths exist)
// ------------------------------------------------------------
func checkAnchors(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	content := string(data)

	var anchorFiles []string
	lines := strings.Split(content, "\n")
	inAcceptanceBlock := false
	inFilesBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "acceptance_item_set:") {
			inAcceptanceBlock = true
			continue
		}

		if inAcceptanceBlock {
			// Stop if we hit a new top-level section
			if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
				if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
					if trimmed == "acceptance_item_set:" || strings.HasPrefix(trimmed, "acceptance_item_set") {
						continue
					}
					if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "- ") {
						break
					}
				}
			}

			if strings.TrimSpace(line) == "files:" || (strings.Contains(line, "files:") && strings.HasPrefix(line, "      ")) {
				inFilesBlock = true
				continue
			}

			if inFilesBlock {
				trimFile := strings.TrimSpace(line)
				if strings.HasPrefix(trimFile, "- ") {
					fpath := trimFile[2:]
					if fpath != "" {
						anchorFiles = append(anchorFiles, fpath)
					}
				} else if trimFile != "" && !strings.HasPrefix(line, "      ") && !strings.HasPrefix(line, "        ") {
					inFilesBlock = false
				}
			}
		}
	}

	if len(anchorFiles) == 0 {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Pass,
			Details: "no affects.files entries to check",
		}
	}

	var missingFiles []string
	for _, af := range anchorFiles {
		fullPath := filepath.Join(repoRoot, filepath.FromSlash(af))
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, af)
		}
	}

	if len(missingFiles) > 0 {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Fail,
			Details: fmt.Sprintf("affects.files paths not found: %s", strings.Join(missingFiles, ", ")),
		}
	}

	return CheckResult{
		Name:    "Anchor integrity",
		Status:  Pass,
		Details: fmt.Sprintf("%d affects.files path(s) exist", len(anchorFiles)),
	}
}

// ------------------------------------------------------------
// Check 4: Reference integrity (unit_refs/rule_refs files exist)
// ------------------------------------------------------------
func checkReferences(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Reference integrity",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))

	unitRefs := fm["unit_refs"]
	var missingRefs []string

	if unitRefs != "" && !strings.EqualFold(unitRefs, "none") {
		refs := specpaths.ParseRefList(unitRefs)
		for _, ref := range refs {
			refName := ref
			if atIdx := strings.LastIndex(ref, "@"); atIdx > 0 {
				refName = ref[:atIdx]
			}

			if strings.HasPrefix(refName, "s_unit_") || strings.HasPrefix(refName, "c_unit_") {
				missingRefs = append(missingRefs, fmt.Sprintf("%s (old format: remove s_/c_ prefix, use bare unit name)", ref))
				continue
			}

			candidatePath := filepath.Join(repoRoot, "docs/specs/units/candidate", fmt.Sprintf("c_unit_%s.md", refName))
			if _, err := os.Stat(candidatePath); err == nil {
				continue
			}

			stablePath := filepath.Join(repoRoot, "docs/specs/units/stable", fmt.Sprintf("s_unit_%s.md", refName))
			if _, err := os.Stat(stablePath); err == nil {
				continue
			}

			missingRefs = append(missingRefs, ref)
		}
	}

	ruleRefs := fm["rule_refs"]
	if ruleRefs != "" && !strings.EqualFold(ruleRefs, "none") {
		refs := specpaths.ParseRefList(ruleRefs)
		for _, ref := range refs {
			refName := ref
			if atIdx := strings.LastIndex(ref, "@"); atIdx > 0 {
				refName = ref[:atIdx]
			}

			if strings.HasPrefix(refName, "s_") || strings.HasPrefix(refName, "c_") {
				missingRefs = append(missingRefs, fmt.Sprintf("%s (old format: remove s_/c_ prefix, use rule ref without layer prefix)", ref))
				continue
			}

			candidatePath := filepath.Join(repoRoot, "docs/specs/rules/candidate", fmt.Sprintf("c_%s.md", refName))
			if _, err := os.Stat(candidatePath); err == nil {
				continue
			}

			stablePath := filepath.Join(repoRoot, "docs/specs/rules/stable", fmt.Sprintf("s_%s.md", refName))
			if _, err := os.Stat(stablePath); err == nil {
				continue
			}

			missingRefs = append(missingRefs, ref)
		}
	}

	if len(missingRefs) > 0 {
		return CheckResult{
			Name:    "Reference integrity",
			Status:  Fail,
			Details: fmt.Sprintf("referenced files not found: %s", strings.Join(missingRefs, ", ")),
		}
	}

	return CheckResult{Name: "Reference integrity", Status: Pass}
}

// ------------------------------------------------------------
// Check 5: Appendix files exist
// ------------------------------------------------------------
func checkAppendices(repoRoot, unitName string) CheckResult {
	appendixGlob := specpaths.CandidateAppendixGlob(unitName)
	fullGlob := filepath.Join(repoRoot, filepath.FromSlash(appendixGlob))
	matches, err := filepath.Glob(fullGlob)
	if err != nil {
		return CheckResult{
			Name:    "Appendix files",
			Status:  Pass,
			Details: fmt.Sprintf("error globbing appendices: %v", err),
		}
	}

	if len(matches) == 0 {
		return CheckResult{
			Name:    "Appendix files",
			Status:  Pass,
			Details: "no appendix files (optional)",
		}
	}

	var relPaths []string
	for _, m := range matches {
		rel, _ := filepath.Rel(repoRoot, m)
		relPaths = append(relPaths, rel)
	}

	return CheckResult{
		Name:    "Appendix files",
		Status:  Pass,
		Details: fmt.Sprintf("%d appendix file(s): %s", len(matches), strings.Join(relPaths, ", ")),
	}
}

// ------------------------------------------------------------
// Check 6: Version/ref consistency
// ------------------------------------------------------------
func checkVersionConsistency(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Version consistency",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))

	unitRefs := fm["unit_refs"]
	var versionMismatches []string

	if unitRefs != "" && !strings.EqualFold(unitRefs, "none") {
		refs := specpaths.ParseRefList(unitRefs)
		for _, ref := range refs {
			refName := ref
			expectedVersion := ""
			if atIdx := strings.LastIndex(ref, "@"); atIdx > 0 {
				refName = ref[:atIdx]
				expectedVersion = ref[atIdx+1:]
			}
			if expectedVersion == "" {
				continue
			}

			targetFile := filepath.Join(repoRoot, "docs/specs/units/candidate", fmt.Sprintf("c_unit_%s.md", refName))
			targetData, err := os.ReadFile(targetFile)
			if err != nil {
				targetFile = filepath.Join(repoRoot, "docs/specs/units/stable", fmt.Sprintf("s_unit_%s.md", refName))
				targetData, err = os.ReadFile(targetFile)
				if err != nil {
					versionMismatches = append(versionMismatches, fmt.Sprintf("%s: cannot read target spec", ref))
					continue
				}
			}
			targetFM := specpaths.ReadFrontmatterStringMap(string(targetData))
			actualVersion := strings.TrimSpace(targetFM["version"])
			if actualVersion != expectedVersion {
				versionMismatches = append(versionMismatches, fmt.Sprintf("%s: expected version %q, target has %q", refName, expectedVersion, actualVersion))
			}
		}
	}

	if len(versionMismatches) > 0 {
		return CheckResult{
			Name:    "Version consistency",
			Status:  Fail,
			Details: strings.Join(versionMismatches, "; "),
		}
	}

	return CheckResult{Name: "Version consistency", Status: Pass}
}


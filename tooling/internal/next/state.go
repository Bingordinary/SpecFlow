// Package next provides file discovery for specFlow units.
package next

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// UnitInfo describes a unit's file state.
type UnitInfo struct {
	Name           string
	HasCandidate   bool
	CandidateSpec  string
	HasStable      bool
	StableSpec     string
	Appendices     []string
	RuleRefs       []string
	RelatedUnits   []string
}

// DiscoverUnit reads the file system to discover a unit's file state.
func DiscoverUnit(repoRoot, unitName string) (*UnitInfo, error) {
	info := &UnitInfo{Name: unitName}

	candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/c_unit_%s.md", unitName))
	stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/s_unit_%s.md", unitName))

	if _, err := os.Stat(candidatePath); err == nil {
		info.HasCandidate = true
		info.CandidateSpec = fmt.Sprintf("docs/specs/units/candidate/c_unit_%s.md", unitName)
	}

	if _, err := os.Stat(stablePath); err == nil {
		info.HasStable = true
		info.StableSpec = fmt.Sprintf("docs/specs/units/stable/s_unit_%s.md", unitName)
	}

	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	pattern := fmt.Sprintf("c_unit_%s_*.md", unitName)
	matches, _ := filepath.Glob(filepath.Join(appendixDir, pattern))
	for _, m := range matches {
		rel, _ := filepath.Rel(repoRoot, m)
		info.Appendices = append(info.Appendices, rel)
	}

	stableAppendixDir := filepath.Join(repoRoot, "docs/specs/units/stable/appendix")
	stableMatches, _ := filepath.Glob(filepath.Join(stableAppendixDir, pattern))
	for _, m := range stableMatches {
		rel, _ := filepath.Rel(repoRoot, m)
		info.Appendices = append(info.Appendices, rel)
	}

	specPath, err := specpaths.ObjectMainSpecFileRef("unit", "candidate", unitName)
	if err == nil {
		info.RelatedUnits = discoverRelatedUnits(repoRoot, unitName, specPath)
		// Read rule_refs from candidate spec frontmatter
		fullPath := filepath.Join(repoRoot, specPath)
		if data, readErr := os.ReadFile(fullPath); readErr == nil {
			fm := specpaths.ReadFrontmatterStringMap(string(data))
			if fm["rule_refs"] != "" && !strings.EqualFold(fm["rule_refs"], "none") {
				info.RuleRefs = specpaths.ParseRefList(fm["rule_refs"])
			}
		}
	} else if info.HasStable {
		// Fall back to stable spec
		stablePath := fmt.Sprintf("docs/specs/units/stable/s_unit_%s.md", unitName)
		fullPath := filepath.Join(repoRoot, stablePath)
		if data, readErr := os.ReadFile(fullPath); readErr == nil {
			fm := specpaths.ReadFrontmatterStringMap(string(data))
			if fm["rule_refs"] != "" && !strings.EqualFold(fm["rule_refs"], "none") {
				info.RuleRefs = specpaths.ParseRefList(fm["rule_refs"])
			}
		}
	}

	return info, nil
}

func discoverRelatedUnits(repoRoot, unitName, specPath string) []string {
	fullPath := filepath.Join(repoRoot, specPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil
	}

	content := string(data)
	var refs []string
	seen := map[string]bool{}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "unit_refs:") {
			rest := strings.TrimPrefix(line, "unit_refs:")
			rest = strings.TrimSpace(rest)
			for _, ref := range strings.Split(rest, ",") {
				ref = strings.TrimSpace(ref)
				ref = strings.Split(ref, "@")[0]
				if ref != "" && ref != unitName && !seen[ref] {
					seen[ref] = true
					refs = append(refs, ref)
				}
			}
		}
	}
	return refs
}

// FormatInfo formats the unit info as a readable output.
func FormatInfo(info *UnitInfo) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Unit: %s\n", info.Name)
	fmt.Fprintf(&buf, "Candidate: %v", info.HasCandidate)
	if info.HasCandidate {
		fmt.Fprintf(&buf, " (%s)", info.CandidateSpec)
	}
	buf.WriteString("\n")

	fmt.Fprintf(&buf, "Stable: %v", info.HasStable)
	if info.HasStable {
		fmt.Fprintf(&buf, " (%s)", info.StableSpec)
	}
	buf.WriteString("\n")

	if len(info.Appendices) > 0 {
		buf.WriteString("Appendices:\n")
		for _, a := range info.Appendices {
			fmt.Fprintf(&buf, "  - %s\n", a)
		}
	}

	if len(info.RuleRefs) > 0 {
		buf.WriteString("Rule refs:\n")
		for _, r := range info.RuleRefs {
			fmt.Fprintf(&buf, "  - %s\n", r)
		}
	}

	if len(info.RelatedUnits) > 0 {
		buf.WriteString("Related units:\n")
		for _, u := range info.RelatedUnits {
			fmt.Fprintf(&buf, "  - %s\n", u)
		}
	}

	return buf.String()
}

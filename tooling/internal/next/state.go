// Package next provides file discovery for specFlow units.
package next

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

// UnitInfo describes a unit's file state.
type UnitInfo struct {
	Name                   string
	HasCandidate           bool
	CandidateSpec          string
	HasStable              bool
	StableSpec             string
	Appendices             []string
	RuleRefs               []string
	RelatedUnits           []string
	ImplementationSurfaces []string
	AffectsFiles           []string
	AcceptanceItems        []string
}

// DiscoverUnit reads the file system to discover a unit's file state.
func DiscoverUnit(repoRoot, unitName string) (*UnitInfo, error) {
	info := &UnitInfo{Name: unitName}

	candidateRef := specpaths.CandidateUnitSpecFileRef(unitName)
	stableRef := specpaths.StableUnitSpecFileRef(unitName)
	candidatePath := filepath.Join(repoRoot, filepath.FromSlash(candidateRef))
	stablePath := filepath.Join(repoRoot, filepath.FromSlash(stableRef))

	if _, err := os.Stat(candidatePath); err == nil {
		info.HasCandidate = true
		info.CandidateSpec = candidateRef
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat %s: %w", candidateRef, err)
	}

	if _, err := os.Stat(stablePath); err == nil {
		info.HasStable = true
		info.StableSpec = stableRef
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat %s: %w", stableRef, err)
	}

	// Candidate is the current spec when present. A read failure is an error,
	// not permission to substitute stable content or an empty state.
	specRef := info.CandidateSpec
	if specRef == "" {
		specRef = info.StableSpec
	}
	if specRef != "" {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(specRef)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", specRef, err)
		}
		populateSpecInfo(info, string(data))
	}

	for _, layer := range []string{"candidate", "stable"} {
		appendices, err := specpaths.UnitAppendices(repoRoot, unitName, layer)
		if err != nil {
			return nil, err
		}
		for _, appendix := range appendices {
			if appendix.Status != "exempt" {
				info.Appendices = append(info.Appendices, appendix.Path)
			}
		}
	}

	return info, nil
}

// populateSpecInfo fills references and acceptance-item-derived fields from
// one selected spec read (deduplicated, document order).
func populateSpecInfo(info *UnitInfo, content string) {
	fm := specpaths.ReadFrontmatterStringMap(content)
	if raw := fm["unit_refs"]; raw != "" && !strings.EqualFold(raw, "none") {
		var refs []string
		for _, ref := range specpaths.ParseRefList(raw) {
			if ref != "" && ref != info.Name {
				refs = append(refs, ref)
			}
		}
		info.RelatedUnits = dedupe(refs)
	}
	if fm["rule_refs"] != "" && !strings.EqualFold(fm["rule_refs"], "none") {
		info.RuleRefs = specpaths.ParseRefList(fm["rule_refs"])
	}
	info.ImplementationSurfaces = dedupe(specvalidation.ExtractImplementationSurfaces(content))
	info.AffectsFiles = dedupe(specvalidation.ExtractAffectsFiles(content))
	info.AcceptanceItems = dedupe(specvalidation.ExtractAcceptanceItemIDs(content))
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
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

	if !info.HasCandidate && !info.HasStable {
		buf.WriteString("(no design recorded)\n")
	}

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

	if len(info.ImplementationSurfaces) > 0 {
		buf.WriteString("Implementation surface:\n")
		for _, s := range info.ImplementationSurfaces {
			fmt.Fprintf(&buf, "  - %s\n", s)
		}
	}

	if len(info.AffectsFiles) > 0 {
		buf.WriteString("Affects files:\n")
		for _, f := range info.AffectsFiles {
			fmt.Fprintf(&buf, "  - %s\n", f)
		}
	}

	if len(info.AcceptanceItems) > 0 {
		buf.WriteString("Acceptance items:\n")
		for _, id := range info.AcceptanceItems {
			fmt.Fprintf(&buf, "  - %s\n", id)
		}
	}

	return buf.String()
}

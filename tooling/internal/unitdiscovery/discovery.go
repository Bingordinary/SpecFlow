package unitdiscovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UnitInfo describes one unit discovered from the filesystem.
// It has no lifecycle state — a unit exists if its spec file exists on disk.
type UnitInfo struct {
	ID           string
	HasStable    bool
	HasCandidate bool
}

// Layer returns the active layer for this unit.
// If candidate exists, candidate is active (being edited).
// Otherwise stable is active.
func (u UnitInfo) Layer() string {
	if u.HasCandidate {
		return "candidate"
	}
	return "stable"
}

// DiscoverUnits scans the filesystem for unit spec files and returns
// unit info without lifecycle state (matching "file existence is state").
func DiscoverUnits(repoRoot string) ([]UnitInfo, error) {
	byID := map[string]*UnitInfo{}

	// Scan stable units
	stablePattern := filepath.Join(repoRoot, "docs/specs/units/stable/unit_*.md")
	stableMatches, err := filepath.Glob(stablePattern)
	if err != nil {
		return nil, fmt.Errorf("scan stable units: %w", err)
	}
	for _, absPath := range stableMatches {
		base := filepath.Base(absPath)
		id := strings.TrimPrefix(base, "unit_")
		id = strings.TrimSuffix(id, ".md")
		if id == "" || id == base {
			continue
		}
		if _, ok := byID[id]; !ok {
			byID[id] = &UnitInfo{ID: id}
		}
		byID[id].HasStable = true
	}

	// Scan candidate units
	candidatePattern := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_*.md")
	candidateMatches, err := filepath.Glob(candidatePattern)
	if err != nil {
		return nil, fmt.Errorf("scan candidate units: %w", err)
	}
	for _, absPath := range candidateMatches {
		base := filepath.Base(absPath)
		id := strings.TrimPrefix(base, "unit_")
		id = strings.TrimSuffix(id, ".md")
		if id == "" || id == base {
			continue
		}
		if _, ok := byID[id]; !ok {
			byID[id] = &UnitInfo{ID: id}
		}
		byID[id].HasCandidate = true
	}

	// Build sorted result
	result := make([]UnitInfo, 0, len(byID))
	for _, info := range byID {
		result = append(result, *info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, nil
}

// DiscoverUnitLayers returns which layers a specific unit has files for.
func DiscoverUnitLayers(repoRoot, unitName string) (hasStable, hasCandidate bool, err error) {
	stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", unitName))
	if _, err := os.Stat(stablePath); err == nil {
		hasStable = true
	} else if !os.IsNotExist(err) {
		return false, false, fmt.Errorf("stat stable spec for %s: %w", unitName, err)
	}

	candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName))
	if _, err := os.Stat(candidatePath); err == nil {
		hasCandidate = true
	} else if !os.IsNotExist(err) {
		return false, false, fmt.Errorf("stat candidate spec for %s: %w", unitName, err)
	}

	return hasStable, hasCandidate, nil
}

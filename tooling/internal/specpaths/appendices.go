package specpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
)

type UnitAppendix struct {
	Path   string
	Status string
}

type UnitAppendixCopy struct {
	Source      string
	Destination string
}

func appendixOwner(path string, fields map[string]string) (string, error) {
	owner := fields["unit"]
	if ValidateTargetName("unit", owner) != nil || !strings.HasPrefix(filepath.Base(path), "unit_"+owner+"_") {
		return "", fmt.Errorf("%s: appendix ownership is missing or inconsistent", path)
	}
	return owner, nil
}

// UnitAppendices resolves exact ownership. A filename prefix only narrows the
// search; frontmatter identifies the owner, including units sharing a prefix.
func UnitAppendices(root, unit, layer string) ([]UnitAppendix, error) {
	if err := ValidateTargetName("unit", unit); err != nil {
		return nil, err
	}
	if layer != "candidate" && layer != "stable" {
		return nil, fmt.Errorf("invalid unit spec layer %q", layer)
	}
	directory := "docs/specs/units/" + layer + "/appendix"
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(directory)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read appendix directory %s: %w", directory, err)
	}
	var out []UnitAppendix
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "unit_"+unit+"_") || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(directory), entry.Name())
		canonical, err := repopath.Canonical(root, path)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", canonical, err)
		}
		fm, _, err := ParseFrontmatterFields(string(data))
		owner, ownerErr := appendixOwner(canonical, fm)
		if err != nil || ownerErr != nil {
			if fm["status"] == "exempt" {
				continue
			}
			return nil, fmt.Errorf("%s: appendix ownership is missing or inconsistent", canonical)
		}
		if owner != unit {
			continue
		}
		status := fm["status"]
		if status != "" && status != "active" && status != "exempt" {
			return nil, fmt.Errorf("%s: appendix status must be active, exempt or absent", canonical)
		}
		out = append(out, UnitAppendix{Path: canonical, Status: status})
	}
	return out, nil
}

// PlanUnitAppendixCopies resolves the complete opposite-layer copy set before
// callers write any lifecycle artifact. Existing destinations must have the
// same declared owner. Stable exempt appendices are omitted from unit forks.
// Copy paths and the returned skipped paths are repository-relative.
func PlanUnitAppendixCopies(root, unit, sourceLayer string) ([]UnitAppendixCopy, []string, error) {
	appendices, err := UnitAppendices(root, unit, sourceLayer)
	if err != nil {
		return nil, nil, err
	}
	destinationLayer := "stable"
	if sourceLayer == "stable" {
		destinationLayer = "candidate"
	}
	var copies []UnitAppendixCopy
	var skipped []string
	for _, appendix := range appendices {
		if sourceLayer == "stable" && appendix.Status == "exempt" {
			skipped = append(skipped, appendix.Path)
			continue
		}
		destination := "docs/specs/units/" + destinationLayer + "/appendix/" + filepath.Base(appendix.Path)
		if _, err := repopath.Canonical(root, destination); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", destination, err)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(destination)))
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("read destination appendix %s: %w", destination, err)
		}
		if err == nil {
			fields, _, err := ParseFrontmatterFields(string(data))
			if err != nil {
				return nil, nil, fmt.Errorf("%s: cannot determine destination appendix ownership: %w", destination, err)
			}
			owner, err := appendixOwner(destination, fields)
			if err != nil {
				return nil, nil, err
			}
			if owner != unit {
				return nil, nil, fmt.Errorf("%s: destination appendix belongs to unit %q, cannot copy appendix for unit %q", destination, owner, unit)
			}
		}
		copies = append(copies, UnitAppendixCopy{Source: appendix.Path, Destination: destination})
	}
	return copies, skipped, nil
}

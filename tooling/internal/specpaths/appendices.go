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

// UnitAppendices resolves exact ownership. A filename prefix only narrows the
// search; frontmatter identifies the owner, including units sharing a prefix.
func UnitAppendices(root, unit, layer string) ([]UnitAppendix, error) {
	if err := ValidateTargetName("unit", unit); err != nil {
		return nil, err
	}
	if layer != "candidate" && layer != "stable" {
		return nil, fmt.Errorf("invalid unit spec layer %q", layer)
	}
	pattern := "docs/specs/units/" + layer + "/appendix/unit_" + unit + "_*.md"
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
	if err != nil {
		return nil, err
	}
	var out []UnitAppendix
	for _, path := range matches {
		canonical, err := repopath.Canonical(root, path)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", canonical, err)
		}
		fm, _, err := ParseFrontmatterFields(string(data))
		owner := fm["unit"]
		if err != nil || ValidateTargetName("unit", owner) != nil || !strings.HasPrefix(filepath.Base(path), "unit_"+owner+"_") {
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

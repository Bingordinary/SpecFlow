package judgments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// SpecContext identifies the unit's complete active spec content independently
// of its layer. Region dependencies still decide which judgments must rerun;
// this identity only separates their current accepted decisions.
func SpecContext(root, unit, layer string) (string, error) {
	if err := specpaths.ValidateTargetName("unit", unit); err != nil {
		return "", err
	}
	if layer != "candidate" && layer != "stable" {
		return "", fmt.Errorf("invalid item spec layer %q", layer)
	}
	dir := "docs/specs/units/" + layer
	paths := []string{filepath.Join(root, filepath.FromSlash(dir), "unit_"+unit+".md")}
	appendices, err := specpaths.UnitAppendices(root, unit, layer)
	if err != nil {
		return "", err
	}
	for _, appendix := range appendices {
		if appendix.Status != "exempt" {
			paths = append(paths, filepath.Join(root, filepath.FromSlash(appendix.Path)))
		}
	}
	var entries []string
	for _, path := range paths {
		canonical, err := repopath.Canonical(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(canonical)))
		if err != nil {
			return "", err
		}
		// Use the declared own-spec name rather than a layer-specific path.
		rel, err := filepath.Rel(filepath.Join(root, filepath.FromSlash(dir)), path)
		if err != nil {
			return "", err
		}
		entries = append(entries, filepath.ToSlash(rel)+"="+Digest([]byte(specpaths.NormalizeText(string(data)))))
	}
	return Digest([]byte(strings.Join(entries, "\n"))), nil
}

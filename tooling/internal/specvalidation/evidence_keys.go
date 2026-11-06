package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
)

// CheckEvidenceKeys enforces the evidence record format: every `##` section
// declares a Keys block with exactly one resolving `item:` key. It also
// rejects the removed `affects.appendices` item field because
// validatekeys.Parse fails on it.
func CheckEvidenceKeys(repoRoot, unitName string) CheckResult {
	const name = "Evidence keys"
	data, err := os.ReadFile(specPath(repoRoot, unitName))
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: fmt.Sprintf("cannot read candidate spec: %v", err)}
	}
	graph, err := validatekeys.Parse(unitName, string(data))
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: err.Error()}
	}
	record, err := specpaths.UnitEvidenceRecord(repoRoot, unitName, "candidate")
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: err.Error()}
	}
	if record == nil {
		return CheckResult{Name: name, Status: Pass, Details: "no evidence record"}
	}
	path := record.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, filepath.FromSlash(path))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: fmt.Sprintf("%s: %v", record.Path, err)}
	}
	base := strings.TrimSuffix(filepath.Base(record.Path), ".md")
	if _, err := validatekeys.ParseEvidenceRecord(base, string(content), graph); err != nil {
		return CheckResult{Name: name, Status: Fail, Details: err.Error()}
	}
	return CheckResult{Name: name, Status: Pass, Details: "every evidence-record section declares one resolving item key"}
}

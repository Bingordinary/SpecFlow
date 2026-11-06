package main

import (
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// TestGateValidateSessionRequiresNoObjectTable pins the report contract: a
// unit validate session reports its check verdicts and findings only. The
// per-object catalog is recorded by the tool at finalize; the reviewer writes
// no Object/Verdict/Read table.
func TestGateValidateSessionRequiresNoObjectTable(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	reports := map[string]string{
		"document": grValidateReport([]string{"1", gaterun.ClarityCheck}, map[string][]string{
			"1":                  {specPath + ": frontmatter", specPath + ": Description"},
			gaterun.ClarityCheck: {specPath + ": all"},
		}),
		"design": grValidateReport([]string{"2", "3", "4"}, map[string][]string{
			"2": {specPath + ": Description"},
			"3": {specPath + ": Acceptance Items"},
			"4": {specPath + ": Description"},
		}),
		"acceptance": grValidateReport([]string{"5"}, map[string][]string{
			"5": {specPath + ": Acceptance Items"},
		}),
		"external": grDependenciesReport(map[string][]string{
			"6": {specPath + ": Acceptance Items"},
			"7": {specPath + ": Description"},
			"8": {specPath + ": Description"},
		}),
	}
	for _, group := range []string{"document", "design", "acceptance", "external"} {
		grSubmitOK(t, repoRoot, runID, group, reports[group])
	}
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "validate_objects") {
		t.Fatalf("validate cache does not record the per-object catalog:\n%s", cache)
	}
	if strings.Contains(cache, "object_verdicts") {
		t.Fatalf("validate cache must not record a per-object verdict table:\n%s", cache)
	}
}

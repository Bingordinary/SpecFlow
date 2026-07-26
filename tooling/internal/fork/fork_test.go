package fork

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForkUnit(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	stableContent := `---
id: test_unit
layer: stable
version: 1.0.0
unit_refs: none
rule_refs: none
---

Stable spec content.
`
	os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte(stableContent), 0644)

	appendixDir := filepath.Join(stableDir, "appendix")
	os.MkdirAll(appendixDir, 0755)
	appendixContent := `---
unit: test_unit
layer: stable
---
Appendix content.
`
	os.WriteFile(filepath.Join(appendixDir, "unit_test_unit_helper.md"), []byte(appendixContent), 0644)

	result := Fork(repoRoot, "test_unit")
	if !result.Passed {
		t.Fatalf("expected PASSED, got FAILED: %v", result.Issues)
	}

	candidateSpec := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_test_unit.md")
	if _, err := os.Stat(candidateSpec); os.IsNotExist(err) {
		t.Fatal("candidate spec was not created")
	}

	data, err := os.ReadFile(candidateSpec)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "layer: candidate") {
		t.Fatalf("expected layer: candidate, got:\n%s", content)
	}
	if !strings.Contains(content, "version: 1.0.1") {
		t.Fatalf("expected version: 1.0.1, got:\n%s", content)
	}

	candidateAppendix := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix/unit_test_unit_helper.md")
	if _, err := os.Stat(candidateAppendix); os.IsNotExist(err) {
		t.Fatal("candidate appendix was not created")
	}
	appendixData, err := os.ReadFile(candidateAppendix)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appendixData), "layer: candidate") {
		t.Fatalf("expected appendix layer: candidate, got:\n%s", string(appendixData))
	}

	stableSpec := filepath.Join(repoRoot, "docs/specs/units/stable/unit_test_unit.md")
	if _, err := os.Stat(stableSpec); os.IsNotExist(err) {
		t.Fatal("stable spec should still exist after fork")
	}
}

func TestForkUnitNoStable(t *testing.T) {
	repoRoot := t.TempDir()
	result := Fork(repoRoot, "nonexistent")
	if result.Passed {
		t.Fatal("expected FAILED when stable does not exist")
	}
	hasIssue := false
	for _, i := range result.Issues {
		if strings.Contains(i, "not found") {
			hasIssue = true
			break
		}
	}
	if !hasIssue {
		t.Fatalf("expected 'not found' issue, got: %v", result.Issues)
	}
}

func TestForkUnitCandidateExists(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte("---\nid: test_unit\nlayer: stable\nversion: 1.0.0\n---\n"), 0644)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_test_unit.md"), []byte("---\nid: test_unit\nlayer: candidate\n---\n"), 0644)

	result := Fork(repoRoot, "test_unit")
	if result.Passed {
		t.Fatal("expected FAILED when candidate already exists")
	}
	hasIssue := false
	for _, i := range result.Issues {
		if strings.Contains(i, "already exists") {
			hasIssue = true
			break
		}
	}
	if !hasIssue {
		t.Fatalf("expected 'already exists' issue, got: %v", result.Issues)
	}
}

func TestForkUnitExemptAppendix(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	stableContent := `---
id: test_unit
layer: stable
version: 1.0.0
unit_refs: none
rule_refs: none
---
`
	os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte(stableContent), 0644)

	appendixDir := filepath.Join(stableDir, "appendix")
	os.MkdirAll(appendixDir, 0755)

	os.WriteFile(filepath.Join(appendixDir, "unit_test_unit_active.md"), []byte("---\nunit: test_unit\nlayer: stable\n---\nActive\n"), 0644)
	os.WriteFile(filepath.Join(appendixDir, "unit_test_unit_exempt.md"), []byte("---\nunit: test_unit\nlayer: stable\nstatus: exempt\n---\nExempt\n"), 0644)

	result := Fork(repoRoot, "test_unit")
	if !result.Passed {
		t.Fatalf("expected PASSED: %v", result.Issues)
	}

	candidateAppendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if _, err := os.Stat(filepath.Join(candidateAppendixDir, "unit_test_unit_active.md")); os.IsNotExist(err) {
		t.Fatal("active appendix should have been forked")
	}
	if _, err := os.Stat(filepath.Join(candidateAppendixDir, "unit_test_unit_exempt.md")); err == nil {
		t.Fatal("exempt appendix should NOT have been forked")
	}

	hasSkip := false
	for _, a := range result.Actions {
		if strings.Contains(a, "Skipped exempt") {
			hasSkip = true
			break
		}
	}
	if !hasSkip {
		t.Fatal("expected action mentioning exempt skip")
	}
}

func TestForkRule(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	os.MkdirAll(stableDir, 0755)
	ruleContent := `---
rule_id: b_rule_auth
rule_scope: bound
layer: stable
rule_version: 1.0.0
---
`
	os.WriteFile(filepath.Join(stableDir, "b_rule_auth.md"), []byte(ruleContent), 0644)

	result := ForkRule(repoRoot, "b_rule_auth")
	if !result.Passed {
		t.Fatalf("expected PASSED: %v", result.Issues)
	}

	candidateRule := filepath.Join(repoRoot, "docs/specs/rules/candidate/b_rule_auth.md")
	if _, err := os.Stat(candidateRule); os.IsNotExist(err) {
		t.Fatal("candidate rule was not created")
	}

	data, err := os.ReadFile(candidateRule)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "layer: candidate") {
		t.Fatalf("expected layer: candidate, got:\n%s", content)
	}
	if !strings.Contains(content, "rule_version: 1.0.1") {
		t.Fatalf("expected rule_version: 1.0.1, got:\n%s", content)
	}
}

func TestForkRuleNoStable(t *testing.T) {
	repoRoot := t.TempDir()
	result := ForkRule(repoRoot, "nonexistent")
	if result.Passed {
		t.Fatal("expected FAILED when stable does not exist")
	}
}

func TestForkRuleCandidateExists(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	os.MkdirAll(stableDir, 0755)
	os.WriteFile(filepath.Join(stableDir, "b_rule_auth.md"), []byte("---\nrule_id: b_rule_auth\nrule_scope: bound\nlayer: stable\nrule_version: 1.0.0\n---\n"), 0644)

	candidateDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "b_rule_auth.md"), []byte("---\nrule_id: b_rule_auth\nlayer: candidate\n---\n"), 0644)

	result := ForkRule(repoRoot, "b_rule_auth")
	if result.Passed {
		t.Fatal("expected FAILED when candidate already exists")
	}
}

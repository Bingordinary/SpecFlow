package fork

import (
	"fmt"
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
	if !strings.Contains(content, "version: 1.0.1") {
		t.Fatalf("expected version: 1.0.1, got:\n%s", content)
	}

	candidateAppendix := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix/unit_test_unit_helper.md")
	if _, err := os.Stat(candidateAppendix); os.IsNotExist(err) {
		t.Fatal("candidate appendix was not created")
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
	os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte("---\nid: test_unit\nversion: 1.0.0\n---\n"), 0644)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_test_unit.md"), []byte("---\nid: test_unit\n---\n"), 0644)

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
version: 1.0.0
unit_refs: none
rule_refs: none
---
`
	os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte(stableContent), 0644)

	appendixDir := filepath.Join(stableDir, "appendix")
	os.MkdirAll(appendixDir, 0755)

	os.WriteFile(filepath.Join(appendixDir, "unit_test_unit_active.md"), []byte("---\nunit: test_unit\n---\nActive\n"), 0644)
	os.WriteFile(filepath.Join(appendixDir, "unit_test_unit_exempt.md"), []byte("---\nunit: test_unit\nstatus: exempt\n---\nExempt\n"), 0644)

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
	os.WriteFile(filepath.Join(stableDir, "b_rule_auth.md"), []byte("---\nrule_id: b_rule_auth\nrule_scope: bound\nrule_version: 1.0.0\n---\n"), 0644)

	candidateDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "b_rule_auth.md"), []byte("---\nrule_id: b_rule_auth\n---\n"), 0644)

	result := ForkRule(repoRoot, "b_rule_auth")
	if result.Passed {
		t.Fatal("expected FAILED when candidate already exists")
	}
}

func writeUnitConfirmationCache(t *testing.T, repoRoot, name, command, extraFrontmatter string, files []string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("command: " + command + "\n")
	sb.WriteString("unit: " + name + "\n")
	sb.WriteString("mode: full\n")
	sb.WriteString("result: pass\n")
	sb.WriteString("timestamp: \"2026-06-30T10:00:00Z\"\n")
	if extraFrontmatter != "" {
		sb.WriteString(extraFrontmatter)
	}
	sb.WriteString("files:\n")
	for _, f := range files {
		fmt.Fprintf(&sb, "  - path: %s\n    hash: sha256:abc\n", f)
	}
	sb.WriteString("---\nok\n")
	if err := os.WriteFile(filepath.Join(dir, command+"_result.md"), []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeStableUnitForFork(t *testing.T, repoRoot string) {
	t.Helper()
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: test_unit\nversion: 1.0.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestForkUnitInheritsConfirmationCaches verifies that pass stable
// confirmation caches are rewritten to the candidate layer during fork: the
// files' physical paths move from stable/ to candidate/, and review's target
// declaration becomes candidate.
func TestForkUnitInheritsConfirmationCaches(t *testing.T) {
	repoRoot := t.TempDir()
	writeStableUnitForFork(t, repoRoot)
	srcDir := filepath.Join(repoRoot, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	writeUnitConfirmationCache(t, repoRoot, "test_unit", "validate", "target: stable\n",
		[]string{"docs/specs/units/stable/unit_test_unit.md"})
	writeUnitConfirmationCache(t, repoRoot, "test_unit", "verify", "target: stable\n",
		[]string{"docs/specs/units/stable/unit_test_unit.md", "src/a.go"})
	writeUnitConfirmationCache(t, repoRoot, "test_unit", "review", "target: stable\nblocking: false\n",
		[]string{"src/a.go"})

	result := Fork(repoRoot, "test_unit")
	if !result.Passed {
		t.Fatalf("expected PASSED: %v", result.Issues)
	}

	for _, want := range []string{
		"Inherited validate confirmation cache (rewritten to candidate layer)",
		"Inherited verify confirmation cache (rewritten to candidate layer)",
		"Inherited review confirmation cache (rewritten to candidate layer)",
	} {
		found := false
		for _, a := range result.Actions {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected action %q, got:\n%v", want, result.Actions)
		}
	}

	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit/validate_result.md")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "target: candidate") {
		t.Fatalf("expected target: candidate in inherited validate cache, got:\n%s", content)
	}
	if !strings.Contains(content, "- path: docs/specs/units/candidate/unit_test_unit.md") {
		t.Fatalf("expected candidate path in inherited validate cache, got:\n%s", content)
	}
	if strings.Contains(content, "docs/specs/units/stable/") {
		t.Fatalf("inherited validate cache still references stable paths, got:\n%s", content)
	}

	verifyCache, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit/verify_result.md"))
	if err != nil {
		t.Fatal(err)
	}
	verifyContent := string(verifyCache)
	if !strings.Contains(verifyContent, "- path: docs/specs/units/candidate/unit_test_unit.md") {
		t.Fatalf("expected candidate main spec path in inherited verify cache, got:\n%s", verifyContent)
	}
	if !strings.Contains(verifyContent, "- path: src/a.go") {
		t.Fatalf("expected code file path preserved in inherited verify cache, got:\n%s", verifyContent)
	}

	reviewCache, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit/review_result.md"))
	if err != nil {
		t.Fatal(err)
	}
	reviewContent := string(reviewCache)
	if !strings.Contains(reviewContent, "target: candidate") {
		t.Fatalf("expected target: candidate in inherited review cache, got:\n%s", reviewContent)
	}
}

// TestForkUnitSkipsUnusableCaches verifies the fork skips confirmation caches
// without a usable baseline: a blocking review cache (P0/P1) and a missing
// cache are reported as skipped, and only the usable validate cache is
// inherited.
func TestForkUnitSkipsUnusableCaches(t *testing.T) {
	repoRoot := t.TempDir()
	writeStableUnitForFork(t, repoRoot)

	writeUnitConfirmationCache(t, repoRoot, "test_unit", "validate", "target: stable\n",
		[]string{"docs/specs/units/stable/unit_test_unit.md"})
	writeUnitConfirmationCache(t, repoRoot, "test_unit", "review", "target: stable\nblocking: true\n",
		[]string{"src/a.go"})

	result := Fork(repoRoot, "test_unit")
	if !result.Passed {
		t.Fatalf("expected PASSED: %v", result.Issues)
	}

	joined := strings.Join(result.Actions, "\n")
	if !strings.Contains(joined, "Inherited validate confirmation cache") {
		t.Fatalf("expected validate inheritance, got:\n%s", joined)
	}
	if !strings.Contains(joined, "review: confirmation cache is blocking (P0/P1 findings)") {
		t.Fatalf("expected review skip reason, got:\n%s", joined)
	}
	if !strings.Contains(joined, "verify: no confirmation cache to inherit") {
		t.Fatalf("expected verify skip reason, got:\n%s", joined)
	}

	data, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit/review_result.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "target: candidate") {
		t.Fatalf("blocking review cache must not be inherited (kept as stable confirmation), got:\n%s", string(data))
	}
}

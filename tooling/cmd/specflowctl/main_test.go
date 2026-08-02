package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextCLI(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := runNext([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("next failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected usage output, got %s", output)
	}
}

func TestPromoteFailsOnMissingUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runPromote([]string{"--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing --unit flag")
	}
	output := stderr.String()
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected usage in stderr, got: %s", output)
	}
}

func TestPromoteFailsOnMissingCache(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runPromote([]string{"--unit", "nonexistent", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing cache")
	}
	output := stdout.String()
	if !strings.Contains(output, "cache not found") {
		t.Fatalf("expected 'cache not found' message, got %s", output)
	}
}

func TestPromoteWithValidSpec(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// Create a valid candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candidateDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `---
id: test_unit
layer: candidate
version: 1.0.0
unit_refs: none
rule_refs: none
---

## Description

Test unit for promote testing.

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: test.check
    description: Test check passes.
    verification_type: testable
    verification_surface: internal
    implementation_surface: internal
    verification_method: test
    pass_condition: passes
    runnable: yes
`
	specPath := filepath.Join(candidateDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a candidate appendix file
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	appendixContent := `---
unit: test_unit
layer: candidate
---
Appendix content for test.
`
	appendixPath := filepath.Join(appendixDir, "unit_test_unit_helper.md")
	if err := os.WriteFile(appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create validate cache with correct hashes
	specHash := computeHash(specPath)
	appendixHash := computeHash(appendixPath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit")
	os.MkdirAll(cacheDir, 0755)

	validateCache := fmt.Sprintf(`---
command: validate
unit: test_unit
result: pass
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
Validate passed.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644); err != nil {
		t.Fatal(err)
	}

	// Create verify cache
	verifyCache := fmt.Sprintf(`---
command: verify
unit: test_unit
result: pass
target: candidate
timestamp: "2026-06-30T11:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
Verify passed.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(verifyCache), 0644); err != nil {
		t.Fatal(err)
	}

	// Create review cache (required gate: full mode, non-blocking, fresh)
	reviewCache := fmt.Sprintf(`---
command: review
unit: test_unit
mode: full
result: pass
p0_count: 0
p1_count: 0
p2_count: 0
p3_count: 0
blocking: false
target: candidate
timestamp: "2026-06-30T12:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
No P0/P1 findings.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(reviewCache), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := runPromote([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("promote failed: %v\nstderr=%s\nstdout=%s", err, stderr.String(), stdout.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "PASSED") {
		t.Fatalf("expected PASSED result, got %s", output)
	}
	if !strings.Contains(output, "Promoted:") {
		t.Fatalf("expected promotion action, got %s", output)
	}

	// Check stable file was created
	stablePath := filepath.Join(repoRoot, "docs/specs/units/stable/unit_test_unit.md")
	if _, err := os.Stat(stablePath); os.IsNotExist(err) {
		t.Fatal("stable spec was not created after promote")
	}

	// Verify caches were cleared after successful promote
	cacheFiles, _ := filepath.Glob(filepath.Join(cacheDir, "*"))
	if len(cacheFiles) > 0 {
		t.Fatalf("expected cache cleared after promote, found: %v", cacheFiles)
	}

	// Verify candidate file was removed after promote (file existence is state)
	if _, statErr := os.Stat(specPath); statErr == nil {
		t.Fatal("candidate spec should be removed after promote, but still exists")
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("unexpected error checking candidate spec: %v", statErr)
	}

	// Verify candidate appendix was removed after promote
	if _, statErr := os.Stat(appendixPath); statErr == nil {
		t.Fatal("candidate appendix should be removed after promote, but still exists")
	} else if !os.IsNotExist(statErr) {
		t.Fatalf("unexpected error checking candidate appendix: %v", statErr)
	}

	// Verify stable appendix was created after promote
	stableAppendixPath := filepath.Join(repoRoot, "docs/specs/units/stable/appendix/unit_test_unit_helper.md")
	if _, err := os.Stat(stableAppendixPath); os.IsNotExist(err) {
		t.Fatal("stable appendix was not created after promote")
	}

	// Verify frontmatter layer was transformed from candidate to stable
	stableContent, err := os.ReadFile(stablePath)
	if err != nil {
		t.Fatalf("failed to read stable spec: %v", err)
	}
	if !strings.Contains(string(stableContent), "layer: stable") {
		t.Fatalf("stable spec frontmatter should contain 'layer: stable', got:\n%s", string(stableContent))
	}
	if strings.Contains(string(stableContent), "layer: candidate") {
		t.Fatalf("stable spec frontmatter should not contain 'layer: candidate', got:\n%s", string(stableContent))
	}

	stableAppendixContent, err := os.ReadFile(stableAppendixPath)
	if err != nil {
		t.Fatalf("failed to read stable appendix: %v", err)
	}
	if !strings.Contains(string(stableAppendixContent), "layer: stable") {
		t.Fatalf("stable appendix frontmatter should contain 'layer: stable', got:\n%s", string(stableAppendixContent))
	}
}

func TestForkFailsOnMissingUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runFork([]string{"--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing --unit flag")
	}
	output := stderr.String()
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected usage in stderr, got: %s", output)
	}
}

func TestForkUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	specContent := `---
id: test_unit
layer: stable
version: 1.0.0
unit_refs: none
rule_refs: none
---

Test unit for fork testing.
`
	specPath := filepath.Join(stableDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	appendixDir := filepath.Join(stableDir, "appendix")
	os.MkdirAll(appendixDir, 0755)
	appendixContent := `---
unit: test_unit
layer: stable
---
Appendix for test.
`
	if err := os.WriteFile(filepath.Join(appendixDir, "unit_test_unit_helper.md"), []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := runFork([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("fork failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "PASSED") {
		t.Fatalf("expected PASSED result, got %s", output)
	}
	if !strings.Contains(output, "Forked:") {
		t.Fatalf("expected forked action, got %s", output)
	}
	if !strings.Contains(output, "Forked appendix") {
		t.Fatalf("expected appendix forked action, got %s", output)
	}

	candidateSpec := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_test_unit.md")
	if _, err := os.Stat(candidateSpec); os.IsNotExist(err) {
		t.Fatal("candidate spec was not created after fork")
	}

	candidateData, err := os.ReadFile(candidateSpec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(candidateData), "layer: candidate") {
		t.Fatalf("expected layer: candidate, got:\n%s", string(candidateData))
	}
	if !strings.Contains(string(candidateData), "version: 1.0.1") {
		t.Fatalf("expected version: 1.0.1, got:\n%s", string(candidateData))
	}

	candidateAppendix := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix/unit_test_unit_helper.md")
	if _, err := os.Stat(candidateAppendix); os.IsNotExist(err) {
		t.Fatal("candidate appendix was not created after fork")
	}

	stableSpec := filepath.Join(repoRoot, "docs/specs/units/stable/unit_test_unit.md")
	if _, err := os.Stat(stableSpec); os.IsNotExist(err) {
		t.Fatal("stable spec should still exist after fork")
	}
}

func TestForkRule(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	stableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	os.MkdirAll(stableDir, 0755)
	ruleContent := `---
rule_id: b_rule_auth
rule_scope: bound
layer: stable
rule_version: 2.0.0
---
`
	if err := os.WriteFile(filepath.Join(stableDir, "b_rule_auth.md"), []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := runFork([]string{"--rule", "b_rule_auth", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("fork rule failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "PASSED") {
		t.Fatalf("expected PASSED result, got %s", output)
	}

	candidateRule := filepath.Join(repoRoot, "docs/specs/rules/candidate/b_rule_auth.md")
	if _, err := os.Stat(candidateRule); os.IsNotExist(err) {
		t.Fatal("candidate rule was not created after fork")
	}

	candidateData, err := os.ReadFile(candidateRule)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(candidateData), "layer: candidate") {
		t.Fatalf("expected layer: candidate, got:\n%s", string(candidateData))
	}
	if !strings.Contains(string(candidateData), "rule_version: 2.0.1") {
		t.Fatalf("expected rule_version: 2.0.1, got:\n%s", string(candidateData))
	}
}

func TestPromoteWithNonBlockingVerifyMismatch(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// Create a valid candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candidateDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `---
id: test_unit
layer: candidate
version: 1.0.0
unit_refs: none
rule_refs: none
---

## Description

Test unit for promote testing.

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: test.check
    description: Test check passes.
    verification_type: testable
    verification_surface: internal
    implementation_surface: internal
    verification_method: test
    pass_condition: passes
    runnable: yes
`
	specPath := filepath.Join(candidateDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a candidate appendix file
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	appendixContent := `---
unit: test_unit
layer: candidate
---
Appendix content for test.
`
	appendixPath := filepath.Join(appendixDir, "unit_test_unit_helper.md")
	if err := os.WriteFile(appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create validate cache with correct hashes
	specHash := computeHash(specPath)
	appendixHash := computeHash(appendixPath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit")
	os.MkdirAll(cacheDir, 0755)

	validateCache := fmt.Sprintf(`---
command: validate
unit: test_unit
result: pass
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
Validate passed.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644); err != nil {
		t.Fatal(err)
	}

	// Create verify cache with only P2/P3 findings: non-blocking, promote allowed
	verifyCache := fmt.Sprintf(`---
command: verify
unit: test_unit
mode: full
result: pass
target: candidate
blocking: false
p2_count: 1
p3_count: 2
timestamp: "2026-06-30T11:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
Non-blocking findings found.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(verifyCache), 0644); err != nil {
		t.Fatal(err)
	}

	// Create review cache (required gate: full mode, non-blocking, fresh)
	reviewCache := fmt.Sprintf(`---
command: review
unit: test_unit
mode: full
result: pass
p0_count: 0
p1_count: 0
p2_count: 0
p3_count: 0
blocking: false
target: candidate
timestamp: "2026-06-30T12:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
No P0/P1 findings.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(reviewCache), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := runPromote([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("promote failed: %v\nstderr=%s\nstdout=%s", err, stderr.String(), stdout.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "PASSED") {
		t.Fatalf("expected PASSED result, got %s", output)
	}
	if !strings.Contains(output, "Promoted:") {
		t.Fatalf("expected promotion action, got %s", output)
	}
}

func TestPromoteWithBlockingVerifyMismatch(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// Create a valid candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candidateDir, 0755); err != nil {
		t.Fatal(err)
	}
	specContent := `---
id: test_unit
layer: candidate
version: 1.0.0
unit_refs: none
rule_refs: none
---

## Description

Test unit for promote testing.

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: test.check
    description: Test check passes.
    verification_type: testable
    verification_surface: internal
    implementation_surface: internal
    verification_method: test
    pass_condition: passes
    runnable: yes
`
	specPath := filepath.Join(candidateDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a candidate appendix file
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	appendixContent := `---
unit: test_unit
layer: candidate
---
Appendix content for test.
`
	appendixPath := filepath.Join(appendixDir, "unit_test_unit_helper.md")
	if err := os.WriteFile(appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create validate cache with correct hashes
	specHash := computeHash(specPath)
	appendixHash := computeHash(appendixPath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit")
	os.MkdirAll(cacheDir, 0755)

	validateCache := fmt.Sprintf(`---
command: validate
unit: test_unit
result: pass
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
Validate passed.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644); err != nil {
		t.Fatal(err)
	}

	// Create verify cache with result: fail (P0/P1 findings — such a cache is
	// never written by the agent, but promote must reject it if present)
	verifyCache := fmt.Sprintf(`---
command: verify
unit: test_unit
mode: full
result: fail
target: candidate
blocking: true
p0_count: 1
p1_count: 0
timestamp: "2026-06-30T11:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
---
Blocking findings found.
`, specHash, appendixHash)
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(verifyCache), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runPromote([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected promote to fail with blocking verify mismatch")
	}
	output := stdout.String()
	if !strings.Contains(output, "Verify cache check: FAIL") {
		t.Fatalf("expected verify cache check FAIL, got %s", output)
	}

	// Candidate spec must not be promoted
	if _, statErr := os.Stat(specPath); statErr != nil {
		t.Fatalf("candidate spec should still exist, got: %v", statErr)
	}
	stablePath := filepath.Join(repoRoot, "docs/specs/units/stable/unit_test_unit.md")
	if _, statErr := os.Stat(stablePath); !os.IsNotExist(statErr) {
		t.Fatal("stable spec should not exist after rejected promote")
	}
}

func TestValidateCandidateFrontmatterDeprecated(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// Create a valid candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	specContent := `---
id: test_unit
layer: candidate
version: 1.0.0
unit_refs: none
rule_refs: none
---

acceptance_item_set:
  - id: test.check
    description: Test check.
    verification_type: testable
    verification_surface: internal
    implementation_surface: internal
    verification_method: test
    pass_condition: passes
    runnable: yes
`
	specPath := filepath.Join(candidateDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Test deprecated command still works
	if err := runValidate([]string{"candidate-frontmatter", "--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deprecated candidate-frontmatter failed: %v\nstderr=%s", err, stderr.String())
	}

	stderrOutput := stderr.String()
	if !strings.Contains(stderrOutput, "DEPRECATED") {
		t.Fatal("expected DEPRECATED warning on stderr")
	}

	stdoutOutput := stdout.String()
	if !strings.Contains(stdoutOutput, "PASS") {
		t.Fatalf("expected PASS result, got %s", stdoutOutput)
	}
}

func createCLITestRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "specflowctl-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// computeHash computes the SHA-256 hash using the same normalization as validationcache.
func computeHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

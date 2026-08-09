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

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

// cacheDeps renders a deps block for a cache file entry covering the whole
// file (whole-file dependency — the conservative declaration).
func cacheDeps(t *testing.T, path string) string {
	t.Helper()
	fc, err := contenthash.ChunkFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if len(fc.Chunks) > 0 {
		b.WriteString("    deps:\n")
	}
	for _, c := range fc.Chunks {
		fmt.Fprintf(&b, "      - %s\n", c.CID)
	}
	return b.String()
}

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
mode: full
result: pass
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
Validate passed.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644); err != nil {
		t.Fatal(err)
	}

	// Create verify cache
	verifyCache := fmt.Sprintf(`---
command: verify
unit: test_unit
mode: full
result: pass
target: candidate
timestamp: "2026-06-30T11:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
Verify passed.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
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
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
No P0/P1 findings.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
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

	// Verify the stable copy is a verbatim copy of the candidate content —
	// promote performs a pure copy (the layer is encoded by the file path).
	stableContent, err := os.ReadFile(stablePath)
	if err != nil {
		t.Fatalf("failed to read stable spec: %v", err)
	}
	if !strings.Contains(string(stableContent), "id: test_unit") {
		t.Fatalf("stable spec content missing, got:\n%s", string(stableContent))
	}

	stableAppendixContent, err := os.ReadFile(stableAppendixPath)
	if err != nil {
		t.Fatalf("failed to read stable appendix: %v", err)
	}
	if !strings.Contains(string(stableAppendixContent), "unit: test_unit") {
		t.Fatalf("stable appendix content missing, got:\n%s", string(stableAppendixContent))
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
mode: full
result: pass
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
Validate passed.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
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
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
Non-blocking findings found.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
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
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
No P0/P1 findings.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
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
mode: full
result: pass
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:%s
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
Validate passed.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
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
%s  - path: docs/specs/units/candidate/appendix/unit_test_unit_helper.md
    hash: sha256:%s
%s---
Blocking findings found.
`, specHash, cacheDeps(t, specPath), appendixHash, cacheDeps(t, appendixPath))
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

func TestPromoteRetiredUnitEndToEnd(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// Stable layer holds the unit from a previous round.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	stableAppendixDir := filepath.Join(stableDir, "appendix")
	if err := os.MkdirAll(stableAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableSpec := "---\nid: test_unit\nversion: 1.0.0\nunit_refs: none\nrule_refs: none\n---\n\n# test_unit\n"
	if err := os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte(stableSpec), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stableAppendixDir, "unit_test_unit_helper.md"),
		[]byte("---\nunit: test_unit\n---\nOld content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Candidate declares the unit retired (no acceptance item set).
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candidateDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: test_unit\nversion: 1.0.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# test_unit\n\nThe unit is retired.\n"
	specPath := filepath.Join(candidateDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(retiredSpec), 0644); err != nil {
		t.Fatal(err)
	}

	// Write the three required caches (main spec only — no active appendix).
	specHash := computeHash(specPath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit")
	os.MkdirAll(cacheDir, 0755)
	validateCache := fmt.Sprintf("---\ncommand: validate\nunit: test_unit\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test_unit.md\n    hash: sha256:%s\n%s---\n", specHash, cacheDeps(t, specPath))
	verifyCache := fmt.Sprintf("---\ncommand: verify\nunit: test_unit\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test_unit.md\n    hash: sha256:%s\n%s---\n", specHash, cacheDeps(t, specPath))
	reviewCache := fmt.Sprintf("---\ncommand: review\nunit: test_unit\nmode: full\nresult: pass\np0_count: 0\np1_count: 0\np2_count: 0\np3_count: 0\nblocking: false\ntarget: candidate\ntimestamp: \"2026-06-30T12:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test_unit.md\n    hash: sha256:%s\n%s---\n", specHash, cacheDeps(t, specPath))
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(verifyCache), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(reviewCache), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runPromote([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("retire promote failed: %v\nstderr=%s\nstdout=%s", err, stderr.String(), stdout.String())
	}

	// The whole unit is gone from stable and candidate; caches cleared.
	for _, p := range []string{
		"docs/specs/units/stable/unit_test_unit.md",
		"docs/specs/units/stable/appendix/unit_test_unit_helper.md",
		"docs/specs/units/candidate/unit_test_unit.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after retire", p)
		}
	}
	for _, c := range []string{"validate_result.md", "verify_result.md", "review_result.md"} {
		if _, err := os.Stat(filepath.Join(cacheDir, c)); !os.IsNotExist(err) {
			t.Fatalf("cache %s must be cleared after retire", c)
		}
	}
}

func TestPromoteRetiredUnitValidateCacheOnly(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// Stable layer holds the unit from a previous round, including an
	// appendix that must disappear with the unit.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	stableAppendixDir := filepath.Join(stableDir, "appendix")
	if err := os.MkdirAll(stableAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	stableSpec := "---\nid: test_unit\nversion: 1.0.0\nunit_refs: none\nrule_refs: none\n---\n\n# test_unit\n"
	if err := os.WriteFile(filepath.Join(stableDir, "unit_test_unit.md"), []byte(stableSpec), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stableAppendixDir, "unit_test_unit_helper.md"),
		[]byte("---\nunit: test_unit\n---\nOld content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Candidate declares the unit retired; the candidate appendix is NOT
	// marked retired (the documented procedure only requires the main spec).
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candidateAppendixDir := filepath.Join(candidateDir, "appendix")
	if err := os.MkdirAll(candidateAppendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSpec := "---\nid: test_unit\nversion: 1.0.1\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n\n# test_unit\n\nThe unit is retired.\n"
	specPath := filepath.Join(candidateDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(retiredSpec), 0644); err != nil {
		t.Fatal(err)
	}
	appendixPath := filepath.Join(candidateAppendixDir, "unit_test_unit_helper.md")
	if err := os.WriteFile(appendixPath, []byte("---\nunit: test_unit\n---\nNew content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Only the validate cache exists — and it lists the main spec only, not
	// the active candidate appendix. A retiring unit must pass with just this
	// gate (verify, review, and appendix coverage are skipped).
	specHash := computeHash(specPath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit")
	os.MkdirAll(cacheDir, 0755)
	validateCache := fmt.Sprintf("---\ncommand: validate\nunit: test_unit\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test_unit.md\n    hash: sha256:%s\n%s---\n", specHash, cacheDeps(t, specPath))
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runPromote([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("retire promote failed: %v\nstderr=%s\nstdout=%s", err, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "gates skipped") {
		t.Fatalf("expected gate-skip notice, stdout:\n%s", stdout.String())
	}

	// The whole unit is gone from every layer — no stable appendix may
	// survive a retiring unit, and no candidate appendix may be promoted.
	for _, p := range []string{
		"docs/specs/units/stable/unit_test_unit.md",
		"docs/specs/units/stable/appendix/unit_test_unit_helper.md",
		"docs/specs/units/candidate/unit_test_unit.md",
		"docs/specs/units/candidate/appendix/unit_test_unit_helper.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, p)); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist after retire", p)
		}
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "validate_result.md")); !os.IsNotExist(err) {
		t.Fatal("validate cache must be cleared after retire")
	}
}

func TestConsumers_GlobalRuleListsAllUnits(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// The rule file exists (candidate layer) and two units are present — a
	// global rule applies to every current-layer unit by default, so all
	// units are reported.
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "g_rule_naming.md"),
		[]byte("---\nrule_id: g_rule_naming\nrule_scope: global\nrule_version: 0.1.0\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unit_a.md", "unit_b.md"} {
		if err := os.WriteFile(filepath.Join(unitDir, name),
			[]byte("---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runConsumers([]string{"--rule", "g_rule_naming", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("consumers failed: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Consumers of \"g_rule_naming\" (2)") {
		t.Fatalf("expected both units listed, got:\n%s", out)
	}
}

func TestConsumers_GlobalRuleMissingFileErrors(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// A retired (or mistyped) global rule has no rule file — its default
	// applicability no longer exists, so the command reports the rule as
	// not found instead of listing every unit.
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "unit_a.md"),
		[]byte("---\nid: a\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runConsumers([]string{"--rule", "g_rule_naming", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error for missing rule file, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
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

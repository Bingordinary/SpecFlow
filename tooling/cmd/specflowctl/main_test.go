package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// writeMergedVerifyCache writes a merged verify cache for the promote
// fixtures: an alignment check on the main spec file and a quality check on
// the declared code file. Both lens sections must be present for promote.
func writeMergedVerifyCache(t *testing.T, repoRoot, unit, specRel, codeRel, result string, blocking bool, counts [4]int) {
	t.Helper()
	codePath := filepath.Join(repoRoot, filepath.FromSlash(codeRel))
	if err := os.MkdirAll(filepath.Dir(codePath), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(codePath); os.IsNotExist(err) {
		if werr := os.WriteFile(codePath, []byte("package demo\n\nfunc Demo() int { return 1 }\n"), 0644); werr != nil {
			t.Fatal(werr)
		}
	}
	currentVerifyFixture(t, repoRoot, unit, "candidate", fmt.Sprintf("result: %s\nblocking: %t\np0_count: %d\np1_count: %d\np2_count: %d\np3_count: %d\n", result, blocking, counts[0], counts[1], counts[2], counts[3]))

}

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

	if err := runNext([]string{"--repo-root", repoRoot}, &stdout, &stderr); err == nil {
		t.Fatalf("expected usage error when --unit is missing, got nil")
	}
	output := stderr.String()
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("expected usage output on stderr, got %s", output)
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout on usage error, got %s", stdout.String())
	}
}

func TestInitHooksOnlyInstallsHooksWithoutManifest(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteCLITestFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteCLITestFile(t, filepath.Join(repoRoot, "specflow/templates/.codex/hooks.json"), `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {
            "type": "command",
            "command": "run-hook session-start codex"
          }
        ]
      }
    ]
  }
}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runInit([]string{"--hooks-only", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("runInit --hooks-only returned error: %v\nstderr=%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".codex/hooks.json")); err != nil {
		t.Fatalf("Codex hooks were not installed: %v", err)
	}
	if strings.Contains(stdout.String(), "specFlow init completed") {
		t.Fatalf("hooks-only unexpectedly ran framework initialization: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "hooks installed: copied=1") {
		t.Fatalf("hooks-only did not report hook installation: %s", stdout.String())
	}
}

func mustWriteCLITestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
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
	writeUnitSpec(t, repoRoot, "test_unit")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runPromote([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr)
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
    implementation_surface: internal/demo.go
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

	// Create the merged verify cache (alignment + quality).
	writeMergedVerifyCache(t, repoRoot, "test_unit", "docs/specs/units/candidate/unit_test_unit.md", "internal/demo.go", "pass", false, [4]int{})

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

	// Verify caches were rewritten after successful promote (not deleted — each
	// cache becomes a stable-layer confirmation cache).
	cacheFiles, _ := filepath.Glob(filepath.Join(cacheDir, "*"))
	if len(cacheFiles) == 0 {
		t.Fatal("expected caches to survive promote as stable confirmation caches, but dir is empty")
	}
	for _, cf := range cacheFiles {
		data, err := os.ReadFile(cf)
		if err != nil {
			t.Fatalf("failed to read cache %s: %v", cf, err)
		}
		// Physical paths must reference stable layer after rewrite (no "candidate"
		// in any path entry)
		if strings.Contains(strings.SplitN(string(data), "\n---\n", 2)[0], "candidate/") {
			t.Fatalf("cache %s still contains candidate-layer path after promote:\n%s", cf, string(data))
		}
		// Verify caches (which had target: candidate) must get target: stable.
		// Validate cache (no target field in the test fixture) is accepted without it —
		// CheckValidateStable does not require the target field.
		if filepath.Base(cf) != "validate_result.md" {
			if !strings.Contains(string(data), "target: stable") {
				t.Fatalf("cache %s was not rewritten to target: stable, content:\n%s", cf, string(data))
			}
		}
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
	if string(candidateData) != specContent {
		t.Fatalf("expected the candidate to be a verbatim copy of the stable spec, got:\n%s", string(candidateData))
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
	if string(candidateData) != ruleContent {
		t.Fatalf("expected the candidate to be a verbatim copy of the stable rule, got:\n%s", string(candidateData))
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
    implementation_surface: internal/demo.go
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

	// Create the merged verify cache with only P2/P3 findings: non-blocking,
	// promote allowed (both lenses present).
	writeMergedVerifyCache(t, repoRoot, "test_unit", "docs/specs/units/candidate/unit_test_unit.md", "internal/demo.go", "pass", false, [4]int{0, 0, 1, 2})

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
    implementation_surface: internal/demo.go
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

	// Create the merged verify cache with result: fail (a failure record —
	// promote must reject it as blocking).
	writeMergedVerifyCache(t, repoRoot, "test_unit", "docs/specs/units/candidate/unit_test_unit.md", "internal/demo.go", "fail", true, [4]int{1, 0, 0, 0})

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

	// Create a valid candidate spec whose implementation_surface resolves.
	internalDir := filepath.Join(repoRoot, "internal")
	os.MkdirAll(internalDir, 0755)
	if err := os.WriteFile(filepath.Join(internalDir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a valid candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	specContent := `---
id: test_unit
unit_refs: none
rule_refs: none
---

## Testability / Acceptance Criteria

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
		[]byte("---\nrule_id: g_rule_naming\nrule_scope: global\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unit_a.md", "unit_b.md"} {
		if err := os.WriteFile(filepath.Join(unitDir, name),
			[]byte("---\nid: demo\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
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
		[]byte("---\nid: a\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
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
	initCLITestGitRepo(t, dir)
	return dir
}

// initCLITestGitRepo makes dir a git worktree, the deployment layout every
// SpecFlow project has: a directory code surface expands over Git repository
// content, so a test project root must be a worktree.
func initCLITestGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
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

// TestPromoteRejectsSingleLensVerifyCache verifies the merged-cache promote
// requirement: a verify cache that covers only the alignment lens (no quality
// section) cannot promote.
func TestPromoteRejectsSingleLensVerifyCache(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	specContent := `---
id: test_unit
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
    implementation_surface: internal/demo.go
    verification_method: test
    pass_condition: passes
    runnable: yes
`
	specPath := filepath.Join(candidateDir, "unit_test_unit.md")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(repoRoot, "internal"), 0755)
	if err := os.WriteFile(filepath.Join(repoRoot, "internal", "demo.go"), []byte("package demo\n\nfunc Demo() int { return 1 }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test_unit")
	os.MkdirAll(cacheDir, 0755)
	specHash := computeHash(specPath)
	validateCache := fmt.Sprintf("---\ncommand: validate\nunit: test_unit\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test_unit.md\n    hash: sha256:%s\n%s---\n", specHash, cacheDeps(t, specPath))
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify cache with only the alignment check — no quality section.
	specEntry, err := validationcache.BuildEntryFromChecks(repoRoot, "docs/specs/units/candidate/unit_test_unit.md", []validationcache.CheckDeclaration{
		{Check: "test.check", Lens: "alignment", AcceptanceItemIDs: []string{"test.check"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "test_unit", validationcache.CacheWrite{
		Command:   "verify",
		Unit:      "test_unit",
		Mode:      "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-06-30T11:00:00Z",
		Entries:   []validationcache.FileEntry{specEntry},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runPromote([]string{"--unit", "test_unit", "--repo-root", repoRoot}, &stdout, &stderr); err == nil {
		t.Fatalf("expected promote to reject a single-lens verify cache, stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "old or damaged review protocol") {
		t.Fatalf("expected the lens-coverage rejection, got:\n%s", stdout.String())
	}
}

package validationcache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

// chunkDeps computes the dependency chunk CIDs of a file on disk.
func chunkDeps(t *testing.T, path string) []string {
	t.Helper()
	fc, err := contenthash.ChunkFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var deps []string
	for _, c := range fc.Chunks {
		deps = append(deps, c.CID)
	}
	return deps
}

// depsYAML renders a deps block for a cache file's files entry.
func depsYAML(deps []string) string {
	if len(deps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("    deps:\n")
	for _, d := range deps {
		fmt.Fprintf(&b, "      - %s\n", d)
	}
	return b.String()
}

func TestCheckValidate(t *testing.T) {
	repoRoot := t.TempDir()

	// Create minimal candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, err := fileHash(specPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create cache dir
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "---\nAll checks passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh, got: %s", result.Reason)
	}
}

func TestCheckValidateStale(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Write cache with WRONG dependency CID (deliberately stale)
	staleCache := "---\ncommand: validate\nunit: test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n" + depsYAML([]string{"sha256:0000000000000000000000000000000000000000000000000000000000000000"}) + "---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(staleCache), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale cache, got fresh")
	}
}

func TestCheckVerify(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nAll items aligned.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh, got: %s", result.Reason)
	}
}

func TestCheckValidateMissingMode(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Validate cache with no mode field: cannot prove a full run, must fail closed
	cacheContent := "---\ncommand: validate\nunit: test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n---\nCheck passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected validate cache without mode to be rejected, got fresh")
	}
}

func TestCheckVerifyInvalidMode(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Verify cache with an invalid mode value: must fail closed
	cacheContent := "---\ncommand: verify\nunit: test\nmode: partial\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nPartial run.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected verify cache with invalid mode to be rejected, got fresh")
	}
}

func TestCheckVerifyNonBlockingFindings(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Full-mode verify cache with only P2/P3 findings: non-blocking, must pass
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\nblocking: false\np2_count: 1\np3_count: 2\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nNon-blocking findings found.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected non-blocking findings cache to be fresh, got: %s", result.Reason)
	}
}

func TestCheckVerifyFailResultRejected(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// A verify cache written with result: fail (P0/P1 findings) is invalid —
	// P0/P1 findings never write a cache, so any fail/mismatch value is
	// rejected by the result vocabulary check.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: fail\ntarget: candidate\nblocking: true\np0_count: 1\np2_count: 1\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nBlocking findings found.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected fail-result verify cache to be rejected, got fresh")
	}
	if !strings.Contains(result.Reason, "expected one of [pass]") {
		t.Fatalf("expected reason to mention result vocabulary rejection, got: %s", result.Reason)
	}
}

func TestCheckVerifyInvalidBlockingValue(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Verify cache with a malformed blocking value: readCache parsing fails
	// and the gate cannot read the cache.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\nblocking: truee\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nVerified.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected mismatch cache with invalid blocking value to be rejected, got fresh")
	}
	if !strings.Contains(result.Reason, "invalid `blocking`") {
		t.Fatalf("expected reason to mention invalid blocking value, got: %s", result.Reason)
	}
}

func TestDeleteCache(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	vPath := filepath.Join(cacheDir, "validate_result.md")
	os.WriteFile(vPath, []byte("---\ncommand: validate\nresult: pass\n---\n"), 0644)

	// Delete and verify
	if err := DeleteCache(repoRoot, "test", "validate"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(vPath); !os.IsNotExist(err) {
		t.Fatal("cache file should be deleted")
	}
}

func TestCheckRuleValidate(t *testing.T) {
	repoRoot := t.TempDir()

	// Create minimal candidate rule file
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(ruleDir, 0755)

	rulePath := filepath.Join(ruleDir, "b_rule_test.md")
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	ruleHash, err := fileHash(rulePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create cache dir under rule path
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: b_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/b_rule_test.md\n    hash: sha256:" + ruleHash + "\n" + depsYAML(chunkDeps(t, rulePath)) + "---\nAll checks passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckRuleValidate(repoRoot, "b_rule_test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh, got: %s", result.Reason)
	}
}

func TestCheckRuleValidateStale(t *testing.T) {
	repoRoot := t.TempDir()

	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(ruleDir, 0755)

	rulePath := filepath.Join(ruleDir, "b_rule_test.md")
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	// Write cache with WRONG dependency CID (deliberately stale)
	staleCache := "---\ncommand: validate\nunit: b_rule_test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/b_rule_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n" + depsYAML([]string{"sha256:0000000000000000000000000000000000000000000000000000000000000000"}) + "---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(staleCache), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckRuleValidate(repoRoot, "b_rule_test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale cache, got fresh")
	}
}

func TestCheckAppendicesInCache_AllInCachePass(t *testing.T) {
	repoRoot := t.TempDir()

	// Create candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candidateDir, "appendix")
	os.MkdirAll(appendixDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create appendix files on disk
	appendixPath1 := filepath.Join(appendixDir, "unit_test_api.md")
	appendixContent1 := "---\nunit: test\n---\n"
	if err := os.WriteFile(appendixPath1, []byte(appendixContent1), 0644); err != nil {
		t.Fatal(err)
	}
	appendixHash1, _ := fileHash(appendixPath1)

	appendixPath2 := filepath.Join(appendixDir, "unit_test_errors.md")
	appendixContent2 := "---\nunit: test\n---\n"
	if err := os.WriteFile(appendixPath2, []byte(appendixContent2), 0644); err != nil {
		t.Fatal(err)
	}
	appendixHash2, _ := fileHash(appendixPath2)

	// Create validate cache that includes both appendix files
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/appendix/unit_test_api.md\n    hash: sha256:" + appendixHash1 + "\n" +
		"  - path: docs/specs/units/candidate/appendix/unit_test_errors.md\n    hash: sha256:" + appendixHash2 + "\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\nAll checks passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckAppendicesInCache(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh, got: %s", result.Reason)
	}
}

func TestCheckAppendicesInCache_MissingAppendixFails(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candidateDir, "appendix")
	os.MkdirAll(appendixDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create appendix on disk but NOT in cache
	appendixPath := filepath.Join(appendixDir, "unit_test_api.md")
	appendixContent := "---\nunit: test\n---\n"
	if err := os.WriteFile(appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create validate cache WITHOUT the appendix entry
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\nAll checks passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckAppendicesInCache(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected not fresh (appendix missing from cache), got fresh")
	}
	if !strings.Contains(result.Reason, "not included") {
		t.Fatalf("expected reason about missing appendix, got: %s", result.Reason)
	}
}

func TestCheckAppendicesInCache_ExemptAppendixNotInCachePass(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candidateDir, "appendix")
	os.MkdirAll(appendixDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create exempt appendix on disk but NOT in cache — should be allowed
	appendixPath := filepath.Join(appendixDir, "unit_test_legacy.md")
	appendixContent := "---\nunit: test\nstatus: exempt\n---\n"
	if err := os.WriteFile(appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\nAll checks passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckAppendicesInCache(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh (exempt appendix skipped), got: %s", result.Reason)
	}
}

func TestCheckAppendicesInCache_RetiredAppendixNotInCachePass(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	appendixDir := filepath.Join(candidateDir, "appendix")
	os.MkdirAll(appendixDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create retiring appendix on disk but NOT in cache — should be allowed
	appendixPath := filepath.Join(appendixDir, "unit_test_legacy.md")
	appendixContent := "---\nunit: test\nstatus: retired\n---\n"
	if err := os.WriteFile(appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\nAll checks passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckAppendicesInCache(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh (retired appendix skipped), got: %s", result.Reason)
	}
}

func TestCheckAppendicesInCache_ValidateCacheNotPassFails(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Validate cache says "fail" — not a valid pass state
	cacheContent := "---\ncommand: validate\nunit: test\nresult: fail\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles: []\n---\nValidate failed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckAppendicesInCache(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected not fresh (validate cache result is not pass), got fresh")
	}
}

func TestCheckAppendicesInCache_NoValidateCacheFails(t *testing.T) {
	repoRoot := t.TempDir()

	result, err := CheckAppendicesInCache(repoRoot, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected not fresh (no validate cache), got fresh")
	}
	if !strings.Contains(result.Reason, "validate cache not found") {
		t.Fatalf("expected reason about missing cache, got: %s", result.Reason)
	}
}

func TestDeleteRuleCache(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	vPath := filepath.Join(cacheDir, "validate_result.md")
	os.WriteFile(vPath, []byte("---\ncommand: validate\nresult: pass\n---\n"), 0644)

	if err := DeleteRuleCache(repoRoot, "b_rule_test", "validate"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(vPath); !os.IsNotExist(err) {
		t.Fatal("cache file should be deleted")
	}
}

// TestNormalizeConsistency verifies that the hash computed by fileHash
// is deterministic and matches the expected normalization.
func TestNormalizeConsistency(t *testing.T) {
	repoRoot := t.TempDir()
	testFile := filepath.Join(repoRoot, "test.txt")

	// Content CRLF -> should normalize same as LF
	crlfContent := "line1\r\nline2\r\nline3\r\n"
	lFContent := "line1\nline2\nline3\n"

	os.WriteFile(testFile, []byte(crlfContent), 0644)
	hashCRLF, _ := fileHash(testFile)

	os.WriteFile(testFile, []byte(lFContent), 0644)
	hashLF, _ := fileHash(testFile)

	if hashCRLF != hashLF {
		t.Fatalf("CRLF and LF versions produced different hashes: %s vs %s", hashCRLF, hashLF)
	}

	// Content without trailing newline -> should normalize to same
	noNewline := "line1\nline2"
	withNewline := "line1\nline2\n"

	os.WriteFile(testFile, []byte(noNewline), 0644)
	hashNoNewline, _ := fileHash(testFile)

	os.WriteFile(testFile, []byte(withNewline), 0644)
	hashWithNewline, _ := fileHash(testFile)

	if hashNoNewline != hashWithNewline {
		t.Fatalf("missing trailing newline produced different hash: %s vs %s", hashNoNewline, hashWithNewline)
	}
}

func TestCheckReviewNoCache(t *testing.T) {
	repoRoot := t.TempDir()

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected not fresh when no review cache exists, got: %s", result.Reason)
	}
	if !strings.Contains(result.Reason, "Review not completed") {
		t.Fatalf("expected 'Review not completed' message, got: %s", result.Reason)
	}
}

func TestCheckReviewPass(t *testing.T) {
	repoRoot := t.TempDir()

	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: pass\np0_count: 0\np1_count: 0\np2_count: 1\np3_count: 0\nblocking: false\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nNo P0/P1 findings.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh, got: %s", result.Reason)
	}
}

func TestCheckReviewBlocking(t *testing.T) {
	repoRoot := t.TempDir()

	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: fail\np0_count: 1\np1_count: 0\np2_count: 0\np3_count: 0\nblocking: true\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nFound P0: null pointer.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected not fresh (blocking review), got fresh")
	}
}

func TestCheckReviewStale(t *testing.T) {
	repoRoot := t.TempDir()

	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Stale dependency CID: the declared chunk no longer exists in the file
	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: pass\np0_count: 0\np1_count: 1\np2_count: 0\np3_count: 0\nblocking: true\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n" + depsYAML([]string{"sha256:0000000000000000000000000000000000000000000000000000000000000000"}) + "---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected not fresh (stale review cache), got: %s", result.Reason)
	}
	if !strings.Contains(result.Reason, "stale") {
		t.Fatalf("expected stale message, got: %s", result.Reason)
	}
}

func TestCheckReviewMissingBlockingField(t *testing.T) {
	repoRoot := t.TempDir()

	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: fail\np0_count: 1\np1_count: 0\np2_count: 0\np3_count: 0\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nFound P0: null pointer.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected not fresh (missing blocking field), got fresh")
	}
	if !strings.Contains(result.Reason, "blocking") {
		t.Fatalf("expected reason to mention missing blocking field, got: %s", result.Reason)
	}
}

func TestCheckReviewConflictingDeclaration(t *testing.T) {
	repoRoot := t.TempDir()

	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: fail\np0_count: 1\np1_count: 0\np2_count: 0\np3_count: 0\nblocking: false\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nFound P0: null pointer.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected not fresh (conflicting result/blocking declarations), got fresh")
	}
	if !strings.Contains(result.Reason, "conflicting") {
		t.Fatalf("expected reason to mention conflicting declarations, got: %s", result.Reason)
	}
}

func TestDeleteReviewCache(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	rPath := filepath.Join(cacheDir, "review_result.md")
	os.WriteFile(rPath, []byte("---\ncommand: review\nresult: pass\n---\n"), 0644)

	if err := DeleteCache(repoRoot, "test", "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rPath); !os.IsNotExist(err) {
		t.Fatal("review cache file should be deleted")
	}
}

func TestDeleteAllWithReviewCache(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	for _, name := range []string{"validate_result.md", "verify_result.md", "review_result.md"} {
		os.WriteFile(filepath.Join(cacheDir, name), []byte("---\n---\n"), 0644)
	}

	if err := DeleteAll(repoRoot, "test"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"validate_result.md", "verify_result.md", "review_result.md"} {
		path := filepath.Join(cacheDir, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be deleted after DeleteAll", name)
		}
	}
}

func TestCheckValidateMissingMainSpecFails(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Cache lists an appendix path but NOT the main spec
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/appendix/unit_test_api.md\n    hash: sha256:abc\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected cache without the main spec to be rejected, got fresh")
	}
	if !strings.Contains(result.Reason, "main unit file") {
		t.Fatalf("expected reason to mention the main unit file, got: %s", result.Reason)
	}
}

func TestCheckValidateEmptyFilesListFails(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Cache with no files listed at all
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected cache with an empty files list to be rejected, got fresh")
	}
}

func TestCheckVerifyMissingMainSpecFails(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Verify cache lists only a source file, not the main spec
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\nblocking: false\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:abc\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected verify cache without the main spec to be rejected, got fresh")
	}
}

func TestCheckRuleValidateMissingMainRuleFails(t *testing.T) {
	repoRoot := t.TempDir()

	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(ruleDir, 0755)

	rulePath := filepath.Join(ruleDir, "b_rule_test.md")
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	// Cache with a files list that omits the main rule file
	cacheContent := "---\ncommand: validate\nunit: b_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/other_rule.md\n    hash: sha256:abc\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckRuleValidate(repoRoot, "b_rule_test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected rule cache without the main rule file to be rejected, got fresh")
	}
	if !strings.Contains(result.Reason, "main rule file") {
		t.Fatalf("expected reason to mention the main rule file, got: %s", result.Reason)
	}
}

func TestCheckAppendicesInCache_GlobErrorFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:abc\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	// An invalid glob pattern in the unit name makes filepath.Glob fail
	result, err := CheckAppendicesInCache(repoRoot, "test[")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected glob failure to reject the gate, got fresh")
	}
}

func TestCheckVerifyStable(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(stableDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(stableDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// verify@stable records the STABLE spec path in its files list.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: stable\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nAll items aligned.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerifyStable(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh for stable verify cache, got: %s", result.Reason)
	}

	// The candidate-based CheckVerify must NOT accept a stable-path cache:
	// it proves nothing about a candidate round.
	candResult, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if candResult.Fresh {
		t.Fatalf("CheckVerify must reject a stable-path verify cache")
	}
}

func TestCheckVerifyStable_CodeChanged(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(stableDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(stableDir, "unit_test.md")
	os.WriteFile(specPath, []byte("---\nid: test\nversion: 0.1.0\n---\n"), 0644)

	srcPath := filepath.Join(srcDir, "handler.go")
	os.WriteFile(srcPath, []byte("package main\nfunc main() {}\n"), 0644)

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: stable\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644)

	// Code changes after the stable verify -> the dependency chunks change,
	// so the silence no longer applies and baseline drift shows.
	os.WriteFile(srcPath, []byte("package main\nfunc main() { println(\"changed\") }\n"), 0644)

	result, err := CheckVerifyStable(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected stale after code change, got: %s", result.Reason)
	}
}

// rangeDeps computes the dependency chunk CIDs covered by line ranges.
func rangeDeps(t *testing.T, path string, ranges [][2]int) []string {
	t.Helper()
	fc, err := contenthash.ChunkFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contenthash.CIDsForRanges(fc, ranges)
}

// makeSharedFile builds a ~16 KB shared code file with a unique line per row.
func makeSharedFile(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "line %d: some unique content to fill the shared file\n", i)
	}
	path := filepath.Join(dir, "shared.go")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCheckVerifyDepOutsideChangeStaysFresh is the core value of
// content-addressed freshness: a change to a shared file outside unit A's
// declared dependency chunks must not stale unit A's cache.
func TestCheckVerifyDepOutsideChangeStaysFresh(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	os.WriteFile(specPath, []byte(specContent), 0644)

	// Unit A depends only on the first 10 lines of the shared file.
	sharedPath := makeSharedFile(t, srcDir)
	specDeps := chunkDeps(t, specPath)
	sharedDeps := rangeDeps(t, sharedPath, [][2]int{{1, 10}})
	if len(sharedDeps) == 0 {
		t.Fatal("expected at least one dependency chunk for the declared range")
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + mustHash(t, specPath) + "\n" + depsYAML(specDeps) +
		"  - path: src/shared.go\n    hash: sha256:" + mustHash(t, sharedPath) + "\n" + depsYAML(sharedDeps) +
		"---\nAll items aligned.\n"
	os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644)

	// Another unit modifies line 350 of the shared file — far from unit A's
	// declared dependency range.
	data, _ := os.ReadFile(sharedPath)
	modified := strings.Replace(string(data), "line 350:", "line 350 CHANGED:", 1)
	if modified == string(data) {
		t.Fatal("test setup: modification did not apply")
	}
	os.WriteFile(sharedPath, []byte(modified), 0644)

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected cache to stay fresh after an unrelated change to a shared file, got: %s", result.Reason)
	}
	if result.Note == "" {
		t.Fatal("expected an informational note about content changed outside the dependency chunks")
	}
}

// TestCheckVerifyDepChangeStales is the other side: a change inside the
// declared dependency range must stale the cache.
func TestCheckVerifyDepChangeStales(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	os.WriteFile(specPath, []byte(specContent), 0644)

	sharedPath := makeSharedFile(t, srcDir)
	specDeps := chunkDeps(t, specPath)
	sharedDeps := rangeDeps(t, sharedPath, [][2]int{{1, 10}})

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + mustHash(t, specPath) + "\n" + depsYAML(specDeps) +
		"  - path: src/shared.go\n    hash: sha256:" + mustHash(t, sharedPath) + "\n" + depsYAML(sharedDeps) +
		"---\nAll items aligned.\n"
	os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644)

	// Modify line 5 — inside unit A's declared dependency range.
	data, _ := os.ReadFile(sharedPath)
	modified := strings.Replace(string(data), "line 5:", "line 5 CHANGED:", 1)
	if modified == string(data) {
		t.Fatal("test setup: modification did not apply")
	}
	os.WriteFile(sharedPath, []byte(modified), 0644)

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected cache to go stale when a declared dependency chunk changes")
	}
}

// TestCheckNoDepsFailsClosed: a cache written before content-addressed
// freshness (hash-only file entries) cannot prove freshness and must fail
// closed.
func TestCheckNoDepsFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	os.WriteFile(specPath, []byte(specContent), 0644)
	specHash, _ := fileHash(specPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	// Old format: hash only, no deps.
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected hash-only cache to fail closed, got fresh")
	}
	if !strings.Contains(result.Reason, "no dependency chunks declared") {
		t.Fatalf("expected reason about missing dependency chunks, got: %s", result.Reason)
	}
}

// TestCheckEmptyFileNoDepsFresh: an empty file has no content, so an entry
// with no deps is consistent and stays fresh.
func TestCheckEmptyFileNoDepsFresh(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	os.WriteFile(specPath, []byte(""), 0644)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected empty-file cache with no deps to be fresh, got: %s", result.Reason)
	}
}

func mustHash(t *testing.T, path string) string {
	t.Helper()
	h, err := fileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestReadVerifyDeps(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	abs := filepath.Join(repoRoot, "src", "util.go")
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\nfiles:\n  - path: ./src/handler.go\n    hash: sha256:aaa\n    deps:\n      - sha256:cid1\n      - sha256:cid2\n  - path: " + abs + "\n    hash: sha256:bbb\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644)

	deps, err := ReadVerifyDeps(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 file entries, got %d: %v", len(deps), deps)
	}
	if len(deps["src/handler.go"]) != 2 || deps["src/handler.go"][0] != "sha256:cid1" || deps["src/handler.go"][1] != "sha256:cid2" {
		t.Fatalf("unexpected deps for handler.go: %v", deps["src/handler.go"])
	}
	if deps["src/util.go"] != nil {
		t.Fatalf("expected no deps for util.go, got %v", deps["src/util.go"])
	}
}

func TestReadVerifyDeps_MissingCacheFails(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := ReadVerifyDeps(repoRoot, "test"); err == nil {
		t.Fatal("expected error for missing verify cache")
	}
}

func TestReadVerifyDeps_CorruptCacheFails(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte("not a cache"), 0644)
	if _, err := ReadVerifyDeps(repoRoot, "test"); err == nil {
		t.Fatal("expected error for corrupt verify cache")
	}
}

func TestCheckValidateLogicalRef(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	// Self spec
	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	if err := os.WriteFile(selfPath, []byte(selfContent), 0644); err != nil {
		t.Fatal(err)
	}
	selfHash, err := fileHash(selfPath)
	if err != nil {
		t.Fatal(err)
	}

	// Dependency unit (candidate layer)
	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := os.WriteFile(depPath, []byte(depContent), 0644); err != nil {
		t.Fatal(err)
	}
	depHash, err := fileHash(depPath)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep\n    hash: sha256:" + depHash + "\n" + depsYAML(chunkDeps(t, depPath)) + "---\nAll checks passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh with logical ref, got: %s", result.Reason)
	}
}

func TestLogicalRefSurvivesPromote(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(stableDir, 0755)

	// Self spec and dependency unit, both candidate.
	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)

	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: passes.\n    runnable: yes\n"
	os.WriteFile(depPath, []byte(depContent), 0644)
	depHash, err := fileHash(depPath)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	selfHash, _ := fileHash(selfPath)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep\n    hash: sha256:" + depHash + "\n" + depsYAML(chunkDeps(t, depPath)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Simulate promote of the dependency unit: content copied verbatim to
	// stable, candidate deleted (pure copy — no field transforms).
	stableDep := filepath.Join(stableDir, "unit_dep.md")
	os.WriteFile(stableDep, []byte(depContent), 0644)
	os.Remove(depPath)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh after dependency promote (logical ref resolves to stable, content unchanged), got: %s", result.Reason)
	}
}

func TestPhysicalRefStalesAfterPromote(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(stableDir, 0755)

	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	os.WriteFile(depPath, []byte(depContent), 0644)
	depHash, _ := fileHash(depPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// Physical path entry — the pre-logical-reference cache form.
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: docs/specs/units/candidate/unit_dep.md\n    hash: sha256:" + depHash + "\n" + depsYAML(chunkDeps(t, depPath)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Simulate promote of the dependency unit.
	stableDep := filepath.Join(stableDir, "unit_dep.md")
	os.WriteFile(stableDep, []byte(depContent), 0644)
	os.Remove(depPath)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale when a physical path entry points at a promoted-away candidate file")
	}
	if !strings.Contains(result.Reason, "missing") {
		t.Fatalf("expected missing-file staleness reason, got: %s", result.Reason)
	}
}

func TestLogicalRefUnresolvedFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// Logical ref with no candidate or stable file.
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n" + depsYAML([]string{"sha256:0000000000000000000000000000000000000000000000000000000000000000"}) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale for an unresolved logical reference")
	}
	if !strings.Contains(result.Reason, "unit:dep") {
		t.Fatalf("expected the logical ref named in the reason, got: %s", result.Reason)
	}
}

func TestAppendixLogicalRefSurvivesPromote(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(filepath.Join(candidateDir, "appendix"), 0755)
	os.MkdirAll(filepath.Join(stableDir, "appendix"), 0755)

	// Self spec.
	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	// Dependency unit main spec + protocol appendix, both candidate.
	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	os.WriteFile(depPath, []byte(depContent), 0644)
	depHash, _ := fileHash(depPath)

	depAppendix := filepath.Join(candidateDir, "appendix", "unit_dep_api.md")
	appendixContent := "---\nunit: dep\n---\n\n# API\n\nPOST /login with timeout 30s. Response code 201 with {id, email}.\n"
	os.WriteFile(depAppendix, []byte(appendixContent), 0644)
	appendixHash, _ := fileHash(depAppendix)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep\n    hash: sha256:" + depHash + "\n" + depsYAML(chunkDeps(t, depPath)) + "  - path: unit:dep:appendix:unit_dep_api\n    hash: sha256:" + appendixHash + "\n" + depsYAML(chunkDeps(t, depAppendix)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Fresh before promote.
	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh before promote, got: %s", result.Reason)
	}

	// Simulate promote of the dependency unit: main spec and appendix copied
	// verbatim to stable, candidate files deleted (pure copy — no transforms).
	stableDep := filepath.Join(stableDir, "unit_dep.md")
	os.WriteFile(stableDep, []byte(depContent), 0644)
	stableAppendix := filepath.Join(stableDir, "appendix", "unit_dep_api.md")
	os.WriteFile(stableAppendix, []byte(appendixContent), 0644)
	os.Remove(depPath)
	os.Remove(depAppendix)

	result, err = CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh after dependency promote (appendix logical ref resolves to stable, content unchanged), got: %s", result.Reason)
	}
}

func TestAppendixLogicalRefStalesAfterContentChange(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(filepath.Join(candidateDir, "appendix"), 0755)

	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	depAppendix := filepath.Join(candidateDir, "appendix", "unit_dep_api.md")
	appendixContent := "---\nunit: dep\n---\n\n# API\n\nPOST /login with timeout 30s.\n"
	os.WriteFile(depAppendix, []byte(appendixContent), 0644)
	appendixHash, _ := fileHash(depAppendix)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep:appendix:unit_dep_api\n    hash: sha256:" + appendixHash + "\n" + depsYAML(chunkDeps(t, depAppendix)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The dependency appendix content changes — the dependency changed and
	// the cache must go stale.
	os.WriteFile(depAppendix, []byte("---\nunit: dep\n---\n\n# API\n\nPOST /login with timeout 60s.\n"), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale after dependency appendix content change")
	}
}

func TestAppendixLogicalRefUnresolvedFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(filepath.Join(candidateDir, "appendix"), 0755)

	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// Appendix exists in no layer (candidate or stable).
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep:appendix:unit_dep_api\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n" + depsYAML([]string{"sha256:0000000000000000000000000000000000000000000000000000000000000000"}) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale for an unresolved appendix logical reference")
	}
	if !strings.Contains(result.Reason, "unit:dep:appendix:unit_dep_api") {
		t.Fatalf("expected the appendix logical ref named in the reason, got: %s", result.Reason)
	}
}

func TestRegionDepUnaffectedByProseEdit(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n## Description\n\nBackground prose.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	os.WriteFile(depPath, []byte(depContent), 0644)
	depHash, _ := fileHash(depPath)

	// Region CID of the acceptance item set.
	depText, _ := os.ReadFile(depPath)
	region, ok := contenthash.AcceptanceItemsRegion(string(depText))
	if !ok {
		t.Fatal("expected region")
	}
	regionCID := contenthash.RegionCID(region)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep\n    hash: sha256:" + depHash + "\n    deps:\n      - region:acceptance_items:" + regionCID + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Prose edit inside the same content-defined chunk (the file is small —
	// one chunk covers everything). The region dependency must stay fresh.
	edited := strings.Replace(depContent, "Background prose.", "Background prose edited during iteration.", 1)
	os.WriteFile(depPath, []byte(edited), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh after prose edit (region dependency is structural), got: %s", result.Reason)
	}

	// Editing the acceptance item set must stale the cache.
	edited = strings.Replace(depContent, "Core behavior.", "Core behavior changed.", 1)
	os.WriteFile(depPath, []byte(edited), 0644)
	result, err = CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale after acceptance item set edit")
	}
}

func TestRegionDepMissingMarkerFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nversion: 0.1.0\nunit_refs: dep\nrule_refs: none\n---\n"
	os.WriteFile(selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\nNo acceptance items.\n"
	os.WriteFile(depPath, []byte(depContent), 0644)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n" + depsYAML(chunkDeps(t, selfPath)) + "  - path: unit:dep\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    deps:\n      - region:acceptance_items:sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale when the region marker is gone (fail closed)")
	}
}

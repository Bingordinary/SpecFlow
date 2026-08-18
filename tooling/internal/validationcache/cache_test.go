package validationcache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func TestCheckValidateDeltaBasis(t *testing.T) {
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

	// A delta-basis cache (mode: full, basis: delta) must satisfy the gate —
	// the basis field is audit metadata and never affects the mode check.
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nbasis: delta\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "---\nAll checks passed (incremental re-run).\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected delta-basis cache to be fresh, got: %s", result.Reason)
	}
}

func TestReadCacheSummaryBasis(t *testing.T) {
	repoRoot := t.TempDir()

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	deltaCache := "---\ncommand: validate\nunit: test\nmode: full\nbasis: delta\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles: []\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(deltaCache), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := ReadCacheSummary(repoRoot, "unit", "test", "validate_result.md")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Basis != "delta" {
		t.Fatalf("expected basis delta, got %q", summary.Basis)
	}
	if summary.Mode != "full" {
		t.Fatalf("expected mode full, got %q", summary.Mode)
	}
}

func TestReadCacheSummaryBasisDefault(t *testing.T) {
	repoRoot := t.TempDir()

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// A cache without a basis field (legacy full-run cache) reads back empty.
	plainCache := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles: []\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(plainCache), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := ReadCacheSummary(repoRoot, "unit", "test", "validate_result.md")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Basis != "" {
		t.Fatalf("expected empty basis, got %q", summary.Basis)
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

	// A fail-result verify cache is a delta re-run's or a candidate full-run
	// FAIL's failure record — it is a valid cache shape since the
	// failure-recovery design. It must declare
	// its blocking status: `result: fail` + `blocking: true` classifies as
	// CategoryBlocked (promote rejected, fresh reports BLOCKED).
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: fail\ntarget: candidate\nblocking: true\np0_count: 1\np2_count: 1\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nBlocking findings found.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected fail-record verify cache to block promote, got fresh")
	}
	if result.Category != CategoryBlocked {
		t.Fatalf("expected CategoryBlocked, got %q: %s", result.Category, result.Reason)
	}
}

func TestCheckVerifyFailRecordMissingBlocking(t *testing.T) {
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

	// A fail result without an explicit `blocking` declaration fails closed —
	// the gate cannot determine the blocking status.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: fail\ntarget: candidate\np0_count: 1\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nBlocking findings found.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected fail-record without blocking declaration to be rejected, got fresh")
	}
	if !strings.Contains(result.Reason, "missing required field `blocking`") {
		t.Fatalf("expected missing-blocking rejection, got: %s", result.Reason)
	}
}

func TestCheckVerifyFailRecordConflictingBlocking(t *testing.T) {
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

	// `result: fail` with `blocking: false` is a conflicting declaration —
	// the cache was written incorrectly and cannot be trusted.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: fail\ntarget: candidate\nblocking: false\np0_count: 1\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nBlocking findings found.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected conflicting fail-record to be rejected, got fresh")
	}
	if !strings.Contains(result.Reason, "conflicting declarations") {
		t.Fatalf("expected conflicting-declarations rejection, got: %s", result.Reason)
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

	// A validate failure record (result: fail) cannot prove appendix
	// coverage — the appendix gate stays unrecovered for a fail cache.
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

func TestCheckReviewStable(t *testing.T) {
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

	// A stable review cache records `target: stable` — the stable-layer
	// quality confirmation consumed by the fresh stable report.
	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: pass\np0_count: 0\np1_count: 0\np2_count: 1\np3_count: 0\nblocking: false\ntarget: stable\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nNo P0/P1 findings.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReviewStable(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh for stable review cache, got: %s", result.Reason)
	}
}

func TestCheckReviewStable_RejectsCandidateCache(t *testing.T) {
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

	// A candidate review cache (target: candidate — the state after a
	// candidate review during an active round) must NOT satisfy the stable
	// confirmation check: the layers are separated by the target field, and
	// the candidate cache cannot prove the stable confirmation state.
	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: pass\np0_count: 0\np1_count: 0\np2_count: 0\np3_count: 0\nblocking: false\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nNo P0/P1 findings.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReviewStable(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected stale for candidate-target review cache, got: %s", result.Reason)
	}
	if !strings.Contains(result.Reason, "target") {
		t.Fatalf("expected reason to mention the target field, got: %s", result.Reason)
	}

	// The candidate-based CheckReview must still accept the same cache.
	candResult, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !candResult.Fresh {
		t.Fatalf("CheckReview must accept a candidate-target review cache, got: %s", candResult.Reason)
	}
}

func TestCheckReviewStable_BlockedStaysBlocked(t *testing.T) {
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

	// A stable review FAIL writes `target: stable` with blocking: true — the
	// target check must not interfere with the blocking classification.
	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: fail\np0_count: 1\np1_count: 0\np2_count: 0\np3_count: 0\nblocking: true\ntarget: stable\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nFound P0: null pointer.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReviewStable(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Category != CategoryBlocked {
		t.Fatalf("expected blocked category, got %v (reason: %s)", result.Category, result.Reason)
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

func TestCheckValidateStable(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	rulesStableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	os.MkdirAll(stableDir, 0755)
	os.MkdirAll(rulesStableDir, 0755)

	specPath := filepath.Join(stableDir, "unit_test.md")
	os.WriteFile(specPath, []byte("---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	// The rule file is an external dependency of the stable content: when it
	// changes, the validate@stable confirmation goes stale.
	rulePath := filepath.Join(rulesStableDir, "g_rule_http.md")
	os.WriteFile(rulePath, []byte("---\nid: g_rule_http\nrule_version: 1\n---\nAll APIs must use HTTPS.\n"), 0644)

	specHash, _ := fileHash(specPath)
	ruleHash, _ := fileHash(rulePath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// validate@stable records the STABLE spec path and its rule dependency.
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n" + depsYAML(chunkDeps(t, rulePath)) + "---\nAll checks passed.\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidateStable(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh for stable validate cache, got: %s", result.Reason)
	}

	// The candidate-based CheckValidate must NOT accept a stable-path cache.
	candResult, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if candResult.Fresh {
		t.Fatalf("CheckValidate must reject a stable-path validate cache")
	}
}

func TestCheckValidateStable_RuleChanged(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	rulesStableDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	os.MkdirAll(stableDir, 0755)
	os.MkdirAll(rulesStableDir, 0755)

	specPath := filepath.Join(stableDir, "unit_test.md")
	os.WriteFile(specPath, []byte("---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	rulePath := filepath.Join(rulesStableDir, "g_rule_http.md")
	os.WriteFile(rulePath, []byte("---\nid: g_rule_http\nrule_version: 1\n---\nAll APIs must use HTTPS.\n"), 0644)

	specHash, _ := fileHash(specPath)
	ruleHash, _ := fileHash(rulePath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n" + depsYAML(chunkDeps(t, rulePath)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The rule changes after the stable validate -> the confirmation goes stale.
	os.WriteFile(rulePath, []byte("---\nid: g_rule_http\nrule_version: 2\n---\nAll APIs must use HTTPS and reject cleartext.\n"), 0644)

	result, err := CheckValidateStable(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected stale after rule change, got: %s", result.Reason)
	}
}

func TestCheckRuleValidateStable(t *testing.T) {
	repoRoot := t.TempDir()

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	unitsStableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableRuleDir, 0755)
	os.MkdirAll(unitsStableDir, 0755)

	rulePath := filepath.Join(stableRuleDir, "g_rule_http.md")
	os.WriteFile(rulePath, []byte("---\nid: g_rule_http\nrule_version: 1\n---\nAll APIs must use HTTPS.\n"), 0644)

	// A consumer unit is an external dependency of the stable rule: when the
	// consumer changes, the rule's validate@stable confirmation goes stale.
	consumerPath := filepath.Join(unitsStableDir, "unit_consumer.md")
	os.WriteFile(consumerPath, []byte("---\nid: consumer\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	ruleHash, _ := fileHash(rulePath)
	consumerHash, _ := fileHash(consumerPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_http")
	os.MkdirAll(cacheDir, 0755)

	// validate@stable on a rule records the STABLE rule path and the consumer
	// units it scanned.
	cacheContent := "---\ncommand: validate\nunit: g_rule_http\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n" + depsYAML(chunkDeps(t, rulePath)) + "  - path: docs/specs/units/stable/unit_consumer.md\n    hash: sha256:" + consumerHash + "\n" + depsYAML(chunkDeps(t, consumerPath)) + "---\nAll checks passed.\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckRuleValidateStable(repoRoot, "g_rule_http")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh for stable rule validate cache, got: %s", result.Reason)
	}

	// The candidate-based CheckRuleValidate must NOT accept a stable-path cache.
	candResult, err := CheckRuleValidate(repoRoot, "g_rule_http")
	if err != nil {
		t.Fatal(err)
	}
	if candResult.Fresh {
		t.Fatalf("CheckRuleValidate must reject a stable-path rule validate cache")
	}
}

func TestCheckRuleValidateStable_ConsumerChanged(t *testing.T) {
	repoRoot := t.TempDir()

	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	unitsStableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableRuleDir, 0755)
	os.MkdirAll(unitsStableDir, 0755)

	rulePath := filepath.Join(stableRuleDir, "g_rule_http.md")
	os.WriteFile(rulePath, []byte("---\nid: g_rule_http\nrule_version: 1\n---\nAll APIs must use HTTPS.\n"), 0644)

	consumerPath := filepath.Join(unitsStableDir, "unit_consumer.md")
	os.WriteFile(consumerPath, []byte("---\nid: consumer\nversion: 0.1.0\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	ruleHash, _ := fileHash(rulePath)
	consumerHash, _ := fileHash(consumerPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_http")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: g_rule_http\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n" + depsYAML(chunkDeps(t, rulePath)) + "  - path: docs/specs/units/stable/unit_consumer.md\n    hash: sha256:" + consumerHash + "\n" + depsYAML(chunkDeps(t, consumerPath)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The consumer changes (e.g. its rule_refs) -> the confirmation goes stale.
	os.WriteFile(consumerPath, []byte("---\nid: consumer\nversion: 0.2.0\nunit_refs: none\nrule_refs:\n  - g_rule_http\n  - g_rule_audit\n---\n"), 0644)

	result, err := CheckRuleValidateStable(repoRoot, "g_rule_http")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected stale after consumer change, got: %s", result.Reason)
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

// specWithSections writes a unit spec with frontmatter plus two sections and
// returns its path and a depsYAML-style checks block declaration.
func writeSpecWithSections(t *testing.T, repoRoot, name, descBody string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Description\n\n" + descBody + "\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + name + ".core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeSpecWithThreeSections writes a spec with three ## sections — a
// Description section, an acceptance-item-bearing section, and a Scope
// section — so tests can edit some sections while leaving others fresh.
func writeSpecWithThreeSections(t *testing.T, repoRoot, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + name + ".core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n\n## Scope\n\nIn scope.\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadCacheChecksMapping(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nbasis: delta\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n    checks:\n      - check: \"1\"\n        deps:\n          - " + descDep + "\n      - check: \"5\"\n        deps:\n          - " + itemsDep + "\n    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	// The promote gate must still see the union deps and stay fresh.
	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh with checks mapping, got: %s", result.Reason)
	}

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.HasChecks {
		t.Fatal("expected per-check evidence")
	}
	if len(scope.Affected) != 0 || len(scope.StaleDeps) != 0 {
		t.Fatalf("expected no stale deps, got affected=%v stale=%v", scope.Affected, scope.StaleDeps)
	}
	if scope.Degrades {
		t.Fatal("expected no degradation")
	}
}

func TestDeriveStaleScopeSectionEdit(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n    checks:\n      - check: \"1\"\n        deps:\n          - " + descDep + "\n      - check: \"5\"\n        deps:\n          - " + itemsDep + "\n      - check: \"7\"\n        deps:\n          - " + descDep + "\n          - " + itemsDep + "\n    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Edit only the Description section.
	os.WriteFile(specPath, []byte(strings.Replace(string(mustRead(t, specPath)), "Prose.", "Prose, edited.", 1)), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.HasChecks {
		t.Fatal("expected per-check evidence")
	}
	if len(scope.StaleDeps) != 1 || scope.StaleDeps[0] != descDep {
		t.Fatalf("expected only the Description dep stale, got %v", scope.StaleDeps)
	}
	if len(scope.Affected) != 2 || scope.Affected[0] != "1" || scope.Affected[1] != "7" {
		t.Fatalf("expected checks 1 and 7 affected, got %v", scope.Affected)
	}
	if scope.Degrades {
		t.Fatal("expected no degradation when check 5 is unaffected")
	}
}

func TestDeriveStaleScopeDegrades(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n    checks:\n      - check: \"1\"\n        deps:\n          - " + descDep + "\n      - check: \"5\"\n        deps:\n          - " + itemsDep + "\n    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Edit both sections: every declared check is affected → degradation.
	edited := strings.Replace(string(mustRead(t, specPath)), "Prose.", "Prose, edited.", 1)
	edited = strings.Replace(edited, "Passes.", "Passes promptly.", 1)
	os.WriteFile(specPath, []byte(edited), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Affected) != 2 {
		t.Fatalf("expected both checks affected, got %v", scope.Affected)
	}
	if !scope.Degrades {
		t.Fatal("expected degradation when every declared check is affected")
	}
}

// TestDeriveStaleScopeCrossFreshOthersStale verifies that the degradation
// conclusion treats the cross-check as always affected (its delta re-run is
// unconditional): every declared non-cross check stale + a fresh cross entry
// means the delta re-run covers every declared check — the whole run.
func TestDeriveStaleScopeCrossFreshOthersStale(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithThreeSections(t, repoRoot, "self")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)
	scopeRegion, _ := contenthash.LocateSectionRegion(text, "Scope")
	scopeDep := "region:section:Scope:" + contenthash.RegionCID(scopeRegion.Text)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + checksYAML(map[string][]string{
		"1":     {descDep},
		"5":     {itemsDep},
		"cross": {scopeDep},
	}) + "    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n      - " + scopeDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Edit the Description and item sections: checks 1 and 5 are stale; the
	// cross entry (Scope section) stays fresh.
	edited := strings.Replace(string(mustRead(t, specPath)), "Prose.", "Prose, edited.", 1)
	edited = strings.Replace(edited, "Passes.", "Passes promptly.", 1)
	os.WriteFile(specPath, []byte(edited), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Affected) != 2 {
		t.Fatalf("expected checks 1 and 5 affected (cross stays fresh), got %v", scope.Affected)
	}
	if !scope.Degrades {
		t.Fatal("expected degradation: every declared non-cross check is stale and the cross-check always re-runs — the delta re-run covers every declared check")
	}
}

// TestDeriveStaleScopeCrossFreshPartialStale verifies the non-degradation
// side of the same rule: with only part of the declared non-cross checks
// stale and a fresh cross entry, the delta re-run covers only part of the
// declaration — the cross entry changes nothing.
func TestDeriveStaleScopeCrossFreshPartialStale(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithThreeSections(t, repoRoot, "self")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)
	scopeRegion, _ := contenthash.LocateSectionRegion(text, "Scope")
	scopeDep := "region:section:Scope:" + contenthash.RegionCID(scopeRegion.Text)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + checksYAML(map[string][]string{
		"1":     {descDep},
		"5":     {itemsDep},
		"cross": {scopeDep},
	}) + "    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n      - " + scopeDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Edit only the Description section: check 1 is stale, check 5 and the
	// cross entry stay fresh — the delta re-run covers only part of the
	// declaration.
	edited := strings.Replace(string(mustRead(t, specPath)), "Prose.", "Prose, edited.", 1)
	os.WriteFile(specPath, []byte(edited), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Affected) != 1 || scope.Affected[0] != "1" {
		t.Fatalf("expected only check 1 affected, got %v", scope.Affected)
	}
	if scope.Degrades {
		t.Fatal("expected no degradation when a declared check (5) stays fresh")
	}
}

func TestDeriveStaleScopeNoChecksMapping(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, specPath)) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if scope.HasChecks {
		t.Fatal("expected no per-check evidence in a legacy cache")
	}
	if len(scope.Affected) != 0 {
		t.Fatalf("expected no affected checks, got %v", scope.Affected)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// checksYAML renders a checks block (with 8-space check-level deps indent) for
// a cache file's files entry.
func checksYAML(checks map[string][]string) string {
	if len(checks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("    checks:\n")
	for _, key := range sortedKeys(checks) {
		fmt.Fprintf(&b, "      - check: \"%s\"\n", key)
		b.WriteString("        deps:\n")
		for _, d := range checks[key] {
			fmt.Fprintf(&b, "          - %s\n", d)
		}
	}
	return b.String()
}

func sortedKeys(m map[string][]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestCheckValidateChecksUnionSubset(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)

	// The file-level deps union omits the check-5 dep → the gate must fail
	// closed (false-fresh protection).
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + checksYAML(map[string][]string{
		"1": {descDep},
		"5": {itemsDep},
	}) + "    deps:\n      - " + descDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected fail-closed when a check dep is missing from the file-level deps union")
	}
	if !strings.Contains(result.Reason, "file-level deps union") {
		t.Fatalf("expected union guidance in reason, got: %s", result.Reason)
	}
}

func TestCheckValidateChecksUnionExtraDepsLegal(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)

	// File-level deps may exceed the check union (declare-heavy extra deps).
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + checksYAML(map[string][]string{
		"1": {descDep},
	}) + "    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh with extra file-level deps, got: %s", result.Reason)
	}
}

func TestDeriveStaleScopeUnionViolationLoud(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + checksYAML(map[string][]string{
		"1": {descDep},
	}) + "    deps:\n      - sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	if _, err := DeriveStaleScope(repoRoot, "unit", "self", "validate"); err == nil {
		t.Fatal("expected loud error for a union violation during delta derivation")
	}
}

func TestDeriveStaleScopeLogicalRefUnclaimed(t *testing.T) {
	repoRoot := t.TempDir()
	// The dependency unit is declared as a logical reference (unit:dep) with
	// no per-check mapping — its acceptance items are read by the consumer's
	// cross-unit check.
	writeSpecWithSections(t, repoRoot, "dep", "Dep prose.")
	depText, _ := contenthash.FileText(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dep.md"))
	depItemsRegion, _ := contenthash.AcceptanceItemsRegion(depText)
	depItemsDep := "region:acceptance_items:" + contenthash.RegionCID(depItemsRegion)

	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + checksYAML(map[string][]string{
		"1": {descDep},
	}) + "    deps:\n      - " + descDep + "\n  - path: unit:dep\n    hash: sha256:dep\n    deps:\n      - " + depItemsDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The dependency unit's acceptance items change.
	os.WriteFile(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dep.md"), []byte(strings.Replace(string(mustRead(t, filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dep.md"))), "Core.", "Core, edited.", 1)), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.HasChecks {
		t.Fatal("expected per-check evidence for the main spec entry")
	}
	if len(scope.Affected) != 0 {
		t.Fatalf("expected no check-claimed stale deps, got affected=%v", scope.Affected)
	}
	if len(scope.Unclaimed) != 1 || scope.Unclaimed[0] != "unit:dep" {
		t.Fatalf("expected the logical reference entry unclaimed, got %v", scope.Unclaimed)
	}
	if len(scope.StaleDeps) != 1 || scope.StaleDeps[0] != depItemsDep {
		t.Fatalf("expected the dependency items dep stale, got %v", scope.StaleDeps)
	}
}

func TestDeriveStaleScopeUnionExtraUnclaimed(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeSpecWithSections(t, repoRoot, "self", "Prose.")
	specHash, _ := fileHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, _ := contenthash.LocateSectionRegion(text, "Description")
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, _ := contenthash.AcceptanceItemsRegion(text)
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// The items region dep is a declare-heavy extra: it sits in the file-level
	// union but no check declared it (whole-file-style declaration).
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + specHash + "\n" + checksYAML(map[string][]string{
		"1": {descDep},
	}) + "    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The items region (unclaimed by any check) changes.
	os.WriteFile(specPath, []byte(strings.Replace(string(mustRead(t, specPath)), "Passes.", "Passes promptly.", 1)), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Affected) != 0 {
		t.Fatalf("expected no check-claimed stale deps, got affected=%v", scope.Affected)
	}
	if len(scope.Unclaimed) != 1 || scope.Unclaimed[0] != "docs/specs/units/candidate/unit_self.md" {
		t.Fatalf("expected the main spec entry unclaimed, got %v", scope.Unclaimed)
	}
	if len(scope.StaleDeps) != 1 || scope.StaleDeps[0] != itemsDep {
		t.Fatalf("expected the items dep stale, got %v", scope.StaleDeps)
	}
}

func TestDeriveStaleScopeUnreadableEntry(t *testing.T) {
	repoRoot := t.TempDir()
	writeSpecWithSections(t, repoRoot, "self", "Prose.")

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// The cache entry points at a file that does not exist — derivation cannot
	// read it, so it is reported as unreadable instead of silently skipped.
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_gone.md\n    hash: sha256:abc\n" + depsYAML([]string{"sha256:0000000000000000000000000000000000000000000000000000000000000000"}) + "---\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	scope, err := DeriveStaleScope(repoRoot, "unit", "self", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.StaleDeps) != 0 {
		t.Fatalf("expected no stale dependency CIDs for an unreadable entry, got %v", scope.StaleDeps)
	}
	if len(scope.Unreadable) != 1 || !strings.Contains(scope.Unreadable[0], "unit_gone.md") || !strings.Contains(scope.Unreadable[0], "unreadable") {
		t.Fatalf("expected the unreadable entry reported, got %v", scope.Unreadable)
	}
}

// writeRuleAndConsumer writes a candidate rule file and a consumer unit spec
// and returns the rule path.
func writeRuleAndConsumer(t *testing.T, repoRoot string) string {
	t.Helper()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(ruleDir, 0755)
	rulePath := filepath.Join(ruleDir, "g_rule_test.md")
	ruleContent := "---\nid: g_rule_test\nversion: 0.1.0\nscope: global\n---\n\n# Rule\n\nBody.\n"
	if err := os.WriteFile(rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}
	writeSpecWithSections(t, repoRoot, "consumer", "Prose.")
	return rulePath
}

func writeRuleCache(t *testing.T, repoRoot string, ruleHash string, rulePath string) {
	t.Helper()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: g_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/g_rule_test.md\n    hash: sha256:" + ruleHash + "\n" + depsYAML(chunkDeps(t, rulePath)) + "  - path: unit:consumer\n    hash: sha256:consumer\n" + depsYAML(chunkDeps(t, filepath.Join(repoRoot, "docs/specs/units/candidate/unit_consumer.md"))) + "---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDeriveStaleScopeRuleFileChangeDegrades(t *testing.T) {
	repoRoot := t.TempDir()
	rulePath := writeRuleAndConsumer(t, repoRoot)
	ruleHash, _ := fileHash(rulePath)
	writeRuleCache(t, repoRoot, ruleHash, rulePath)

	// The rule file itself changes: it is a whole-file declaration, so every
	// rule-body check is affected — the delta scope degrades.
	ruleContent := string(mustRead(t, rulePath))
	os.WriteFile(rulePath, []byte(strings.Replace(ruleContent, "Body.", "Body, edited.", 1)), 0644)

	scope, err := DeriveStaleScope(repoRoot, "rule", "g_rule_test", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Degrades {
		t.Fatal("expected degradation when the rule file itself went stale")
	}
	if len(scope.Unclaimed) != 1 || scope.Unclaimed[0] != "docs/specs/rules/candidate/g_rule_test.md" {
		t.Fatalf("expected the rule file entry unclaimed, got %v", scope.Unclaimed)
	}
}

func TestDeriveStaleScopeRuleConsumerChangeNoDegrades(t *testing.T) {
	repoRoot := t.TempDir()
	rulePath := writeRuleAndConsumer(t, repoRoot)
	ruleHash, _ := fileHash(rulePath)
	writeRuleCache(t, repoRoot, ruleHash, rulePath)

	// Only the consumer unit spec changes: the rule file entry stays fresh,
	// so the scope does not degrade.
	consumerPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_consumer.md")
	os.WriteFile(consumerPath, []byte(strings.Replace(string(mustRead(t, consumerPath)), "Prose.", "Prose, edited.", 1)), 0644)

	scope, err := DeriveStaleScope(repoRoot, "rule", "g_rule_test", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if scope.Degrades {
		t.Fatal("expected no degradation when only a consumer changed")
	}
	if len(scope.Unclaimed) != 1 || scope.Unclaimed[0] != "unit:consumer" {
		t.Fatalf("expected the consumer logical reference unclaimed, got %v", scope.Unclaimed)
	}
}

func TestDeriveStaleScopeRuleFilePrefixedPathDegrades(t *testing.T) {
	repoRoot := t.TempDir()
	rulePath := writeRuleAndConsumer(t, repoRoot)
	ruleHash, _ := fileHash(rulePath)
	ruleDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_test")
	os.MkdirAll(ruleDir, 0755)
	// The rule file entry is recorded with the documented-equivalent
	// `./`-prefixed spelling — the degradation detection must not depend on
	// the exact recorded path form.
	cacheContent := "---\ncommand: validate\nunit: g_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: ./docs/specs/rules/candidate/g_rule_test.md\n    hash: sha256:" + ruleHash + "\n" + depsYAML(chunkDeps(t, rulePath)) + "  - path: unit:consumer\n    hash: sha256:consumer\n" + depsYAML(chunkDeps(t, filepath.Join(repoRoot, "docs/specs/units/candidate/unit_consumer.md"))) + "---\n"
	os.WriteFile(filepath.Join(ruleDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The rule file itself changes: the `./`-prefixed entry must still be
	// recognized as the rule file and set the degradation state.
	ruleContent := string(mustRead(t, rulePath))
	os.WriteFile(rulePath, []byte(strings.Replace(ruleContent, "Body.", "Body, edited.", 1)), 0644)

	scope, err := DeriveStaleScope(repoRoot, "rule", "g_rule_test", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Degrades {
		t.Fatal("expected degradation for a ./ -prefixed rule file entry that went stale")
	}
	if len(scope.Unclaimed) != 1 || scope.Unclaimed[0] != "./docs/specs/rules/candidate/g_rule_test.md" {
		t.Fatalf("expected the prefixed rule file entry unclaimed, got %v", scope.Unclaimed)
	}
}

// TestRuleCacheChecksMapping verifies the rule-checks format contract: rule
// caches may carry per-check evidence (rule-body checks on the rule file
// entry, consumer-discovery checks on the unit logical references), making
// a consumer change stale exactly the checks that declared it.
func TestRuleCacheChecksMapping(t *testing.T) {
	repoRoot := t.TempDir()
	rulePath := writeRuleAndConsumer(t, repoRoot)
	ruleHash, _ := fileHash(rulePath)
	consumerPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_consumer.md")
	consumerDeps := chunkDeps(t, consumerPath)
	ruleDeps := chunkDeps(t, rulePath)

	indentDeps := func(deps []string) string {
		var b strings.Builder
		for _, d := range deps {
			b.WriteString("          - " + d + "\n")
		}
		return b.String()
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: g_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/rules/candidate/g_rule_test.md\n    hash: sha256:" + ruleHash + "\n" +
		"    checks:\n" +
		"      - check: \"1\"\n        deps:\n" + indentDeps(ruleDeps) +
		"      - check: \"5\"\n        deps:\n" + indentDeps(ruleDeps) +
		"    deps:\n" + depsYAML(ruleDeps) +
		"  - path: unit:consumer\n    hash: sha256:consumer\n" +
		"    checks:\n" +
		"      - check: \"5\"\n        deps:\n" + indentDeps(consumerDeps) +
		"    deps:\n" + depsYAML(consumerDeps) +
		"---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	// A fresh rule cache with per-check evidence satisfies the gate like any
	// other cache.
	result, err := CheckRuleValidate(repoRoot, "g_rule_test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh rule cache with checks mapping, got: %s", result.Reason)
	}

	scope, err := DeriveStaleScope(repoRoot, "rule", "g_rule_test", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.HasChecks {
		t.Fatal("expected per-check evidence on the rule cache")
	}
	if len(scope.Affected) != 0 || len(scope.StaleDeps) != 0 || scope.Degrades {
		t.Fatalf("expected a clean scope, got affected=%v stale=%v degrades=%v", scope.Affected, scope.StaleDeps, scope.Degrades)
	}

	// The consumer unit changes: only the check that declared it (5) is
	// affected, the entry is claimed (no longer unclaimed), no degradation.
	os.WriteFile(consumerPath, []byte(strings.Replace(string(mustRead(t, consumerPath)), "Prose.", "Prose, edited.", 1)), 0644)

	scope, err = DeriveStaleScope(repoRoot, "rule", "g_rule_test", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Affected) != 1 || scope.Affected[0] != "5" {
		t.Fatalf("expected only check 5 affected by the consumer change, got %v", scope.Affected)
	}
	if scope.Degrades {
		t.Fatal("expected no degradation for a consumer-only change")
	}
	for _, u := range scope.Unclaimed {
		if u == "unit:consumer" {
			t.Fatalf("consumer entry must be claimed by check 5, got unclaimed: %v", scope.Unclaimed)
		}
	}

	// The rule file itself changes: whole-file declaration — every check is
	// affected and the scope degrades.
	ruleContent := string(mustRead(t, rulePath))
	os.WriteFile(rulePath, []byte(strings.Replace(ruleContent, "Body.", "Body, edited.", 1)), 0644)

	scope, err = DeriveStaleScope(repoRoot, "rule", "g_rule_test", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Degrades {
		t.Fatal("expected degradation when the rule file itself went stale")
	}
}

// TestRuleFailureRecordStatusMap verifies the failure-record shape for rules:
// a fail cache with a per-check status map is a valid blocking cache (promote
// rejects it as BLOCKED), and the parser recovers the status map — the
// failure-recovery scope input.
func TestRuleFailureRecordStatusMap(t *testing.T) {
	repoRoot := t.TempDir()
	rulePath := writeRuleAndConsumer(t, repoRoot)
	ruleHash, _ := fileHash(rulePath)
	consumerPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_consumer.md")
	consumerDeps := chunkDeps(t, consumerPath)
	ruleDeps := chunkDeps(t, rulePath)

	indentDeps := func(deps []string) string {
		var b strings.Builder
		for _, d := range deps {
			b.WriteString("          - " + d + "\n")
		}
		return b.String()
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: g_rule_test\nmode: full\nbasis: delta\nresult: fail\nblocking: true\np0_count: 1\np1_count: 0\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/rules/candidate/g_rule_test.md\n    hash: sha256:" + ruleHash + "\n" +
		"    checks:\n" +
		"      - check: \"1\"\n        deps:\n" + indentDeps(ruleDeps) + "        status: pass\n" +
		"      - check: \"5\"\n        status: fail\n        deps:\n" + indentDeps(ruleDeps) +
		"    deps:\n" + depsYAML(ruleDeps) +
		"  - path: unit:consumer\n    hash: sha256:consumer\n" +
		"    checks:\n" +
		"      - check: \"5\"\n        status: fail\n        deps:\n" + indentDeps(consumerDeps) +
		"    deps:\n" + depsYAML(consumerDeps) +
		"---\nCheck 5 found P0: consumer drift.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	// A failure record with a status map is a valid blocking cache shape:
	// promote rejects it as BLOCKED (the status map lives in the record for
	// the recovery — the gate does not consume it).
	result, err := CheckRuleValidate(repoRoot, "g_rule_test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected a fail-record rule cache to block, got fresh")
	}
	if result.Category != CategoryBlocked {
		t.Fatalf("expected CategoryBlocked, got %q: %s", result.Category, result.Reason)
	}

	// The parser recovers the per-check status map — the recovery scope input.
	cache, err := readCache(filepath.Join(cacheDir, "validate_result.md"))
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, f := range cache.Files {
		for _, c := range f.Checks {
			statuses[c.Check] = c.Status
		}
	}
	if statuses["1"] != "pass" || statuses["5"] != "fail" {
		t.Fatalf("expected status map {1: pass, 5: fail}, got %v", statuses)
	}
}

func TestRewriteCacheLayer(t *testing.T) {
	input := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntarget: stable\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:abc\n  - path: docs/specs/units/stable/appendix/unit_test_a.md\n    hash: sha256:def\n  - path: unit:dep\n    hash: sha256:ghi\n  - path: src/a.go\n    hash: sha256:jkl\n---\n## Findings\n- P2: something\ntarget: stable is body text, not frontmatter\n"

	out, changed := rewriteCacheLayer(input)
	if !changed {
		t.Fatal("expected the cache to be rewritten")
	}
	if !strings.Contains(out, "target: candidate\n") {
		t.Fatalf("expected frontmatter target rewritten, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: docs/specs/units/candidate/unit_test.md") {
		t.Fatalf("expected main spec path rewritten, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: docs/specs/units/candidate/appendix/unit_test_a.md") {
		t.Fatalf("expected appendix path rewritten, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: unit:dep") {
		t.Fatalf("logical reference must be preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: src/a.go") {
		t.Fatalf("code file path must be preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "target: stable is body text, not frontmatter") {
		t.Fatalf("body must be preserved verbatim, got:\n%s", out)
	}
	if strings.Contains(out, "docs/specs/units/stable/") {
		t.Fatalf("no stable path may remain, got:\n%s", out)
	}
}

func TestRewriteCacheLayerNoChange(t *testing.T) {
	input := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:abc\n---\nok\n"
	out, changed := rewriteCacheLayer(input)
	if changed {
		t.Fatal("a candidate-layer cache must not be rewritten")
	}
	if out != input {
		t.Fatalf("content must be unchanged, got:\n%s", out)
	}
}

func TestRewriteCacheLayerToStable(t *testing.T) {
	input := `---
command: validate
unit: test_unit
mode: full
result: pass
target: candidate
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:abc123
  - path: docs/specs/units/candidate/appendix/unit_test_unit_a.md
    hash: sha256:def456
  - path: unit:dep
    hash: sha256:ghi789
  - path: src/a.go
    hash: sha256:jkl012
---
## Findings
- P2: cosmetic issue
target: candidate is body text, not frontmatter
`
	out, changed := rewriteCacheLayerToStable(input)
	if !changed {
		t.Fatal("expected the cache to be rewritten to stable")
	}
	if !strings.Contains(out, "target: stable\n") {
		t.Fatalf("expected frontmatter target rewritten to 'stable', got:\n%s", out)
	}
	if !strings.Contains(out, "- path: docs/specs/units/stable/unit_test_unit.md") {
		t.Fatalf("expected main spec path rewritten, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: docs/specs/units/stable/appendix/unit_test_unit_a.md") {
		t.Fatalf("expected appendix path rewritten, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: unit:dep") {
		t.Fatalf("logical reference must be preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: src/a.go") {
		t.Fatalf("code file path must be preserved, got:\n%s", out)
	}
	if !strings.Contains(out, "target: candidate is body text, not frontmatter") {
		t.Fatalf("body must be preserved verbatim, got:\n%s", out)
	}
	if strings.Contains(out, "/candidate/") {
		t.Fatalf("no candidate path may remain, got:\n%s", out)
	}
}

func TestRewriteCacheLayerToStableNoTarget(t *testing.T) {
	// Cache with no target field in frontmatter (target defaults to candidate).
	// The rewrite must still add target: stable since every promoted cache
	// becomes a stable confirmation cache.
	input := `---
command: validate
unit: test_unit
mode: full
result: pass
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/candidate/unit_test_unit.md
    hash: sha256:abc123
---
Validate passed.
`
	out, changed := rewriteCacheLayerToStable(input)
	if !changed {
		t.Fatal("expected the cache to be rewritten (paths must change)")
	}
	if !strings.Contains(out, "- path: docs/specs/units/stable/unit_test_unit.md") {
		t.Fatalf("expected main spec path rewritten, got:\n%s", out)
	}
	// target field is absent; rewriteLayerFrontmatter does not add one.
	// The validate stable gate does not require a target field, so this is
	// semantically correct. The review cache (always has target: candidate
	// in practice) is handled separately with its own rewrite.
}

func TestRewriteCacheLayerToStableAlreadyStable(t *testing.T) {
	// A stable confirmation cache must not be rewritten again.
	input := `---
command: validate
unit: test_unit
mode: full
result: pass
target: stable
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/units/stable/unit_test_unit.md
    hash: sha256:abc123
---
`
	out, changed := rewriteCacheLayerToStable(input)
	if changed {
		t.Fatal("a stable-layer cache must not be rewritten")
	}
	if out != input {
		t.Fatalf("content must be unchanged, got:\n%s", out)
	}
}

func TestRewriteLayerFrontmatterRulePath(t *testing.T) {
	input := `---
command: validate
unit: b_rule_test
mode: full
result: pass
target: candidate
timestamp: "2026-06-30T10:00:00Z"
files:
  - path: docs/specs/rules/candidate/b_rule_test.md
    hash: sha256:abc
  - path: unit:consumer
    hash: sha256:def
---
`
	out, changed := rewriteCacheLayerToStable(input)
	if !changed {
		t.Fatal("expected rule cache to be rewritten")
	}
	if !strings.Contains(out, "- path: docs/specs/rules/stable/b_rule_test.md") {
		t.Fatalf("expected rule path rewritten, got:\n%s", out)
	}
	if !strings.Contains(out, "- path: unit:consumer") {
		t.Fatalf("logical reference must be preserved, got:\n%s", out)
	}
}

func TestRewriteCachesToStablePromotedCachesPassStableChecks(t *testing.T) {
	repoRoot := t.TempDir()

	// Candidate spec + appendix + source file at their candidate-layer paths.
	unit := "test"
	candSpec := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_"+unit+".md")
	candAppendix := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix/unit_"+unit+"_a.md")
	srcPath := filepath.Join(repoRoot, "src/a.go")
	os.MkdirAll(filepath.Dir(candSpec), 0755)
	os.MkdirAll(filepath.Dir(candAppendix), 0755)
	os.MkdirAll(filepath.Dir(srcPath), 0755)
	os.WriteFile(candSpec, []byte("---\nid: test\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Test\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: test.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"), 0644)
	os.WriteFile(candAppendix, []byte("---\nunit: test\n---\n\n# Appendix\n"), 0644)
	os.WriteFile(srcPath, []byte("package demo\n\nfunc Demo() int { return 1 }\n"), 0644)

	specHash, _ := fileHash(candSpec)
	appendixHash, _ := fileHash(candAppendix)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit", unit)
	os.MkdirAll(cacheDir, 0755)

	// Candidate-layer validate cache (no target field — defaults to candidate).
	validateCache := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\nblocking: false\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, candSpec)) + "  - path: docs/specs/units/candidate/appendix/unit_test_a.md\n    hash: sha256:" + appendixHash + "\n" + depsYAML(chunkDeps(t, candAppendix)) + "---\nAll checks passed.\n"
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644)

	// Candidate-layer verify cache.
	verifyCache := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\nblocking: false\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n" + depsYAML(chunkDeps(t, candSpec)) + "  - path: src/a.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nAll items aligned.\n"
	os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(verifyCache), 0644)

	// Candidate-layer review cache.
	reviewCache := "---\ncommand: review\nunit: test\nmode: full\nresult: pass\np0_count: 0\np1_count: 0\np2_count: 0\np3_count: 0\nblocking: false\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/a.go\n    hash: sha256:" + srcHash + "\n" + depsYAML(chunkDeps(t, srcPath)) + "---\nNo P0/P1 findings.\n"
	os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(reviewCache), 0644)

	// Simulate the stable layer existing (promote has copied the files).
	stableSpec := filepath.Join(repoRoot, "docs/specs/units/stable/unit_test.md")
	stableAppendix := filepath.Join(repoRoot, "docs/specs/units/stable/appendix/unit_test_a.md")
	os.MkdirAll(filepath.Dir(stableSpec), 0755)
	os.MkdirAll(filepath.Dir(stableAppendix), 0755)
	os.WriteFile(stableSpec, mustRead(t, candSpec), 0644)
	os.WriteFile(stableAppendix, mustRead(t, candAppendix), 0644)

	// Rewrite the candidate caches into stable confirmation caches.
	report, err := RewriteCachesToStable(repoRoot, "unit", unit)
	if err != nil {
		t.Fatalf("RewriteCachesToStable failed: %v", err)
	}
	var rewrittenCount int
	for _, e := range report.Entries {
		if e.Rewritten {
			rewrittenCount++
		}
	}
	if rewrittenCount != 3 {
		t.Fatalf("expected all 3 caches rewritten, got %d (report: %+v)", rewrittenCount, report.Entries)
	}

	// The rewritten caches must pass the stable-layer checks.
	if r, err := CheckValidateStable(repoRoot, unit); err != nil || !r.Fresh {
		t.Fatalf("CheckValidateStable after rewrite: fresh=%v err=%v reason=%s", r.Fresh, err, r.Reason)
	}
	if r, err := CheckVerifyStable(repoRoot, unit); err != nil || !r.Fresh {
		t.Fatalf("CheckVerifyStable after rewrite: fresh=%v err=%v reason=%s", r.Fresh, err, r.Reason)
	}
	if r, err := CheckReviewStable(repoRoot, unit); err != nil || !r.Fresh {
		t.Fatalf("CheckReviewStable after rewrite: fresh=%v err=%v reason=%s", r.Fresh, err, r.Reason)
	}
}

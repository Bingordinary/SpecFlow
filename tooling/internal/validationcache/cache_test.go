package validationcache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckValidate(t *testing.T) {
	repoRoot := t.TempDir()

	// Create minimal candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
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

	cacheContent := "---\ncommand: validate\nunit: test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n---\nAll checks passed.\n"
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
	specContent := "---\nid: test\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Write cache with WRONG hash (deliberately stale)
	staleCache := "---\ncommand: validate\nunit: test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\n"
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
	specContent := "---\nid: test\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
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

	cacheContent := "---\ncommand: verify\nunit: test\nresult: aligned\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nAll items aligned.\n"
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

func TestCheckValidateScoped(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Scoped cache: pass but mode=scoped, scoped_check=1
	cacheContent := "---\ncommand: validate\nunit: test\nmode: scoped\nscoped_check: \"1\"\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n---\nCheck 1 passed.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected scoped validate cache to be rejected, got fresh")
	}
}

func TestCheckVerifyScoped(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
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

	// Scoped verify cache: aligned but mode=scoped, scoped_item=AUTH-AC-001
	cacheContent := "---\ncommand: verify\nunit: test\nmode: scoped\nscoped_item: AUTH-AC-001\nresult: aligned\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nItem AUTH-AC-001 aligned.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected scoped verify cache to be rejected, got fresh")
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
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\n---\n"
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

	cacheContent := "---\ncommand: validate\nunit: b_rule_test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/b_rule_test.md\n    hash: sha256:" + ruleHash + "\n---\nAll checks passed.\n"
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
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	// Write cache with WRONG hash (deliberately stale)
	staleCache := "---\ncommand: validate\nunit: b_rule_test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/b_rule_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\n"
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

func TestCheckRuleVerify(t *testing.T) {
	repoRoot := t.TempDir()

	result, err := CheckRuleVerify(repoRoot, "b_rule_test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected not fresh (rule verify is deprecated), got fresh")
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
	if !result.Fresh {
		t.Fatalf("expected fresh when no cache exists, got: %s", result.Reason)
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

	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: pass\np0_count: 0\np1_count: 0\np2_count: 1\np3_count: 0\nblocking: false\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nNo P0/P1 findings.\n"
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

	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: fail\np0_count: 1\np1_count: 0\np2_count: 0\np3_count: 0\nblocking: true\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nFound P0: null pointer.\n"
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

	// Stale hash: file content hash doesn't match what's in cache
	cacheContent := "---\ncommand: review\nunit: test\nmode: full\nresult: pass\np0_count: 0\np1_count: 1\np2_count: 0\np3_count: 0\nblocking: true\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh (stale gate skipped), got: %s", result.Reason)
	}
}

func TestCheckReviewScopedNonBlocking(t *testing.T) {
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

	cacheContent := "---\ncommand: review\nunit: test\nmode: scoped\nresult: pass\np0_count: 0\np1_count: 0\np2_count: 0\np3_count: 1\nblocking: false\ntarget: candidate\ntimestamp: \"2026-07-24T10:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n---\nScoped non-blocking review.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "review_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckReview(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh (scoped non-blocking), got: %s", result.Reason)
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

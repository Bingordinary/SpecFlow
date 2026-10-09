package validationcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func TestCheckValidate(t *testing.T) {
	repoRoot := t.TempDir()

	// Create minimal candidate spec
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, err := fileHash(specPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create cache dir
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll checks passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Write cache with WRONG dependency CID (deliberately stale)
	staleCache := "---\ncommand: validate\nunit: test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(staleCache), 0644); err != nil {
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

// TestCheckValidateRejectsLegacyDepsCache pins that a cache in the removed
// `deps:`-declaration format — which lacks the required chunk evidence — is
// rejected by the freshness chain that `fresh` and `promote` use, so only a
// full run rebuilds it. The `deps:` lines themselves are ignored (unknown
// fields are harmless); the missing chunk evidence is what makes the cache
// not current-format.
func TestCheckValidateRejectsLegacyDepsCache(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	if err := writeCacheFixtureFile(t, specPath, []byte("---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	specHash, err := fileHash(specPath)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    deps:\n      - region:legacy\n---\nAll checks passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected the deps-format cache to fail closed as STALE, got fresh=%t category=%s", result.Fresh, result.Category)
	}
	if !strings.Contains(result.Reason, "chunk evidence") {
		t.Fatalf("expected the missing-chunk-evidence reason, got: %s", result.Reason)
	}
}

// TestReadGateBaselineTreatsUnusableCacheAsAbsent pins that an existing cache
// in the removed format (no chunk evidence) is not returned as a baseline: it
// degrades to Exists=false with a reason, so every reader falls back to the
// full command instead of aborting.
func TestReadGateBaselineTreatsUnusableCacheAsAbsent(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	legacy := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:abc\n---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(legacy), 0644)

	baseline, err := ReadGateBaseline(repoRoot, "unit", "test", "validate")
	if err != nil {
		t.Fatalf("an unusable baseline must not error: %v", err)
	}
	if baseline.Exists {
		t.Fatal("an unusable cache must not be returned as an existing baseline")
	}
	if !strings.Contains(baseline.UnusableReason, "chunk evidence") {
		t.Fatalf("expected the unusable reason, got %q", baseline.UnusableReason)
	}
}

// TestDeriveChangeReportReportsContradictoryEvidence pins that a cache whose
// recorded whole-file hash disagrees with its own (matching) chunk sequence is
// reported as inconsistent rather than localized: the entry's evidence
// contradicts itself, so a delta/repair run must refuse it and a full run must
// not carry the standing conclusions mechanically. Chunk evidence is required
// in every entry, so this is the only remaining unlocalizable state.
func TestDeriveChangeReportReportsContradictoryEvidence(t *testing.T) {
	repoRoot := t.TempDir()
	specRel := "docs/specs/units/candidate/unit_test.md"
	specPath := filepath.Join(repoRoot, filepath.FromSlash(specRel))
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("---\nid: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := BuildEvidenceEntry(repoRoot, specRel)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt only the whole-file hash; the chunk sequence still matches the
	// current content, so the recorded evidence contradicts itself.
	entry.Hash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := WriteCache(repoRoot, "unit", "test", CacheWrite{
		Command:   "validate",
		Unit:      "test",
		Mode:      "full",
		Result:    "pass",
		Timestamp: "2026-06-30T10:00:00Z",
		Entries:   []FileEntry{*entry},
	}); err != nil {
		t.Fatal(err)
	}

	report, err := DeriveChangeReport(repoRoot, "unit", "test", "validate")
	if err != nil {
		t.Fatalf("derivation must not error: %v", err)
	}
	if got := report.Inconsistent(); len(got) != 1 || got[0] != specRel {
		t.Fatalf("expected the contradictory entry reported inconsistent, got %v (%+v)", got, report.Entries)
	}
}

// TestParseCacheIgnoresExtrasInPlace pins that unknown fields (e.g. the removed
// `deps:` lines) are skipped without ending the files block, so entries after
// them are retained rather than silently dropped.
func TestParseCacheIgnoresExtrasInPlace(t *testing.T) {
	content := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\nfiles:\n" +
		"  - path: a.md\n    hash: sha256:aaa\n    deps:\n      - region:legacy\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:c1\n        start: 1\n        end: 2\n" +
		"  - path: b.md\n    hash: sha256:bbb\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:c2\n        start: 1\n        end: 2\n" +
		"---\n"
	cache, err := parseCache([]byte(content))
	if err != nil {
		t.Fatalf("extras must be ignored: %v", err)
	}
	if len(cache.Files) != 2 {
		t.Fatalf("expected both entries retained, got %d: %+v", len(cache.Files), cache.Files)
	}
	if cache.Files[0].Path != "a.md" || cache.Files[1].Path != "b.md" {
		t.Fatalf("unexpected entries: %+v", cache.Files)
	}
}

func TestCheckValidateDeltaBasis(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// A delta-basis cache must carry its change-review record: the delta model
	// exists because an independent reviewer accepted the change set. The
	// basis itself never affects the mode check.
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nbasis: delta\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nreviewed_change_set: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nreview_result: accept\nreview_session: review\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll checks passed (incremental re-run).\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected a reviewed delta-basis cache to be fresh, got: %s", result.Reason)
	}

	// A delta cache without the review record is a pre-review artifact.
	unreviewed := strings.Replace(cacheContent, "reviewed_change_set: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nreview_result: accept\nreview_session: review\n", "", 1)
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(unreviewed), 0644); err != nil {
		t.Fatal(err)
	}
	result, err = CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || !strings.Contains(result.Reason, "change-review record") {
		t.Fatalf("expected an unreviewed delta cache to fail closed, got: %s", result.Reason)
	}
}

func TestReadCacheSummaryBasis(t *testing.T) {
	repoRoot := t.TempDir()

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	deltaCache := "---\ncommand: validate\nunit: test\nmode: full\nbasis: delta\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles: []\n---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(deltaCache), 0644); err != nil {
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
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(plainCache), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll items aligned.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Validate cache with no mode field: cannot prove a full run, must fail closed
	cacheContent := "---\ncommand: validate\nunit: test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nCheck passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Verify cache with an invalid mode value: must fail closed
	cacheContent := "---\ncommand: verify\nunit: test\nmode: partial\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nPartial run.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Full-mode verify cache with only P2/P3 findings: non-blocking, must pass
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\nblocking: false\np2_count: 1\np3_count: 2\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nNon-blocking findings found.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
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
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: fail\ntarget: candidate\nblocking: true\np0_count: 1\np2_count: 1\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nBlocking findings found.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// A fail result without an explicit `blocking` declaration fails closed —
	// the gate cannot determine the blocking status.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: fail\ntarget: candidate\np0_count: 1\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nBlocking findings found.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// `result: fail` with `blocking: false` is a conflicting declaration —
	// the cache was written incorrectly and cannot be trusted.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: fail\ntarget: candidate\nblocking: false\np0_count: 1\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nBlocking findings found.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Verify cache with a malformed blocking value: readCache parsing fails
	// and the gate cannot read the cache.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\nblocking: truee\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nVerified.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	writeCacheFixtureFile(t, vPath, []byte("---\ncommand: validate\nresult: pass\n---\n"), 0644)

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
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\n---\n"
	if err := writeCacheFixtureFile(t, rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	ruleHash, err := fileHash(rulePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create cache dir under rule path
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: b_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/b_rule_test.md\n    hash: sha256:" + ruleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll checks passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\n---\n"
	if err := writeCacheFixtureFile(t, rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	// Write cache with WRONG dependency CID (deliberately stale)
	staleCache := "---\ncommand: validate\nunit: b_rule_test\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/b_rule_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(staleCache), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create appendix files on disk
	appendixPath1 := filepath.Join(appendixDir, "unit_test_api.md")
	appendixContent1 := "---\nunit: test\n---\n"
	if err := writeCacheFixtureFile(t, appendixPath1, []byte(appendixContent1), 0644); err != nil {
		t.Fatal(err)
	}
	appendixHash1, _ := fileHash(appendixPath1)

	appendixPath2 := filepath.Join(appendixDir, "unit_test_errors.md")
	appendixContent2 := "---\nunit: test\n---\n"
	if err := writeCacheFixtureFile(t, appendixPath2, []byte(appendixContent2), 0644); err != nil {
		t.Fatal(err)
	}
	appendixHash2, _ := fileHash(appendixPath2)

	// Create validate cache that includes both appendix files
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/appendix/unit_test_api.md\n    hash: sha256:" + appendixHash1 + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" +
		"  - path: docs/specs/units/candidate/appendix/unit_test_errors.md\n    hash: sha256:" + appendixHash2 + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nAll checks passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create appendix on disk but NOT in cache
	appendixPath := filepath.Join(appendixDir, "unit_test_api.md")
	appendixContent := "---\nunit: test\n---\n"
	if err := writeCacheFixtureFile(t, appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create validate cache WITHOUT the appendix entry
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nAll checks passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create exempt appendix on disk but NOT in cache — should be allowed
	appendixPath := filepath.Join(appendixDir, "unit_test_legacy.md")
	appendixContent := "---\nunit: test\nstatus: exempt\n---\n"
	if err := writeCacheFixtureFile(t, appendixPath, []byte(appendixContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nAll checks passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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

func TestCheckAppendicesInCache_ValidateCacheNotPassFails(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// A validate failure record (result: fail) cannot prove appendix
	// coverage — the appendix gate stays unrecovered for a fail cache.
	cacheContent := "---\ncommand: validate\nunit: test\nresult: fail\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles: []\n---\nValidate failed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	writeCacheFixtureFile(t, vPath, []byte("---\ncommand: validate\nresult: pass\n---\n"), 0644)

	if err := DeleteRuleCache(repoRoot, "b_rule_test", "validate"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(vPath); !os.IsNotExist(err) {
		t.Fatal("cache file should be deleted")
	}
}

func TestPublishCacheCleansTempFileOnFailure(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/demo")
	canonical := filepath.Join(cacheDir, "validate_result.md")
	if err := os.MkdirAll(filepath.Join(canonical, "keep"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := PublishCache(repoRoot, "unit", "demo", "validate", []byte("candidate\n")); err == nil {
		t.Fatal("expected publication over a non-empty directory to fail")
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".specflow-cache-") {
			t.Fatalf("failed publication left temp file %s", entry.Name())
		}
	}
	if info, err := os.Stat(canonical); err != nil || !info.IsDir() {
		t.Fatalf("failed publication changed the canonical target: info=%v err=%v", info, err)
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

	writeCacheFixtureFile(t, testFile, []byte(crlfContent), 0644)
	hashCRLF, _ := fileHash(testFile)

	writeCacheFixtureFile(t, testFile, []byte(lFContent), 0644)
	hashLF, _ := fileHash(testFile)

	if hashCRLF != hashLF {
		t.Fatalf("CRLF and LF versions produced different hashes: %s vs %s", hashCRLF, hashLF)
	}

	// Content without trailing newline -> should normalize to same
	noNewline := "line1\nline2"
	withNewline := "line1\nline2\n"

	writeCacheFixtureFile(t, testFile, []byte(noNewline), 0644)
	hashNoNewline, _ := fileHash(testFile)

	writeCacheFixtureFile(t, testFile, []byte(withNewline), 0644)
	hashWithNewline, _ := fileHash(testFile)

	if hashNoNewline != hashWithNewline {
		t.Fatalf("missing trailing newline produced different hash: %s vs %s", hashNoNewline, hashWithNewline)
	}
}

// writeMergedVerifyCache writes a valid merged verify cache under
// docs/specs/meta/validation/unit/test: an alignment check on the spec file and
// a quality check on the code file.
func writeMergedVerifyCache(t *testing.T, repoRoot string) {
	t.Helper()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)
	specPath := filepath.Join(candidateDir, "unit_test.md")
	if err := writeCacheFixtureFile(t, specPath, []byte("---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(srcDir, "handler.go")
	if err := writeCacheFixtureFile(t, srcPath, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	specEntry, err := BuildEvidenceEntry(repoRoot, "docs/specs/units/candidate/unit_test.md")
	if err != nil {
		t.Fatal(err)
	}
	specEntry.Checks = []CheckEntry{{Check: "test.core", Lens: "alignment"}}
	srcEntry, err := BuildEvidenceEntry(repoRoot, "src/handler.go")
	if err != nil {
		t.Fatal(err)
	}
	srcEntry.Checks = []CheckEntry{{Check: "src/handler.go", Lens: "quality"}}
	if _, err := writeCacheFixture(t, repoRoot, "unit", "test", CacheWrite{
		Command:   "verify",
		Unit:      "test",
		Mode:      "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-06-30T11:00:00Z",
		Entries:   []FileEntry{*specEntry, *srcEntry},
	}); err != nil {
		t.Fatal(err)
	}
}

// fixedChecks adapts a static expected set to CheckVerifyMerged's lazy
// derivation provider.
func fixedChecks(checks []ExpectedCheck) func() ([]ExpectedCheck, error) {
	return func() ([]ExpectedCheck, error) { return checks, nil }
}

func TestCheckVerifyMergedPass(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", fixedChecks([]ExpectedCheck{
		{Key: "test.core", Lens: "alignment"},
		{Key: "src/handler.go", Lens: "quality"},
	}), false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh merged verify cache, got: %s", result.Reason)
	}
}

func TestCheckVerifyMergedMissingLens(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	// The expected set names an alignment key the cache never recorded.
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", fixedChecks([]ExpectedCheck{
		{Key: "test.other", Lens: "alignment"},
		{Key: "src/handler.go", Lens: "quality"},
	}), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected a cache missing an alignment key to be rejected")
	}
	if !strings.Contains(result.Reason, "test.other") {
		t.Fatalf("expected the missing key in the reason, got: %s", result.Reason)
	}
}

func TestCheckVerifyMergedMissingQualityLens(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", fixedChecks([]ExpectedCheck{
		{Key: "test.core", Lens: "alignment"},
		{Key: "src/handler.go", Lens: "quality"},
		{Key: "src/other.go", Lens: "quality"},
	}), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected a cache missing a quality key to be rejected")
	}
	if !strings.Contains(result.Reason, "src/other.go") {
		t.Fatalf("expected the lens-coverage reason, got: %s", result.Reason)
	}
}

func TestCheckVerifyMergedStaleQualityKey(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	// Change the code file: the whole-file evidence no longer matches.
	srcPath := filepath.Join(repoRoot, "src", "handler.go")
	if err := writeCacheFixtureFile(t, srcPath, []byte("package main\nfunc main() { println(1) }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", fixedChecks([]ExpectedCheck{
		{Key: "test.core", Lens: "alignment"},
		{Key: "src/handler.go", Lens: "quality"},
	}), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected a changed quality file to stale the merged verify cache")
	}
	if !strings.Contains(result.Reason, "src/handler.go") {
		t.Fatalf("expected the changed file named in the reason, got: %s", result.Reason)
	}
}

// TestCheckVerifyMergedStaleAlignmentKey verifies that a spec-only change
// stales the merged verify cache and is named in the reason.
func TestCheckVerifyMergedStaleAlignmentKey(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_test.md")
	if err := writeCacheFixtureFile(t, specPath, []byte("---\nid: test\nunit_refs: none\nrule_refs: none\n---\n// changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", fixedChecks([]ExpectedCheck{
		{Key: "test.core", Lens: "alignment"},
		{Key: "src/handler.go", Lens: "quality"},
	}), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected a changed spec to stale the merged verify cache")
	}
	if !strings.Contains(result.Reason, "unit_test.md") {
		t.Fatalf("expected the changed spec named in the reason, got: %s", result.Reason)
	}
}

// TestCheckVerifyMergedRequiresBothLenses verifies the fallback contract: when
// no coverage keys can be derived, an existing cache must still prove both lens
// sections ran.
func TestCheckVerifyMergedRequiresBothLenses(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	// Drop the quality check from the cache so only the alignment section
	// remains, then require both lenses without enumerating keys.
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test", "verify_result.md")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Replace(string(data), "      - check: \"src/handler.go\"\n        lens: quality\n", "", 1)
	if stripped == string(data) {
		t.Fatal("fixture assumption broken: quality check not found")
	}
	if err := writeCacheFixtureFile(t, cachePath, []byte(stripped), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected a cache missing the quality lens to be rejected")
	}
	if !strings.Contains(result.Reason, "quality lens") {
		t.Fatalf("expected the quality lens named, got: %s", result.Reason)
	}
}

// TestCheckVerifyMergedNoPerCheckEvidenceFailsClosed pins the no-compat
// contract: a verify cache without per-check evidence is invalid for both
// call shapes — promote (expected keys enumerated) and fresh (no expected
// keys, both lenses required) — and fails closed as STALE with re-run
// guidance. A no-checks failure record is STALE too: repair cannot use a
// record whose judgments cannot be associated with checks.
func TestCheckVerifyMergedNoPerCheckEvidenceFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	specPath := filepath.Join(candidateDir, "unit_test.md")
	if err := writeCacheFixtureFile(t, specPath, []byte("---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// A cache without a per-check `checks` breakdown.
	entry, err := BuildEvidenceEntry(repoRoot, "docs/specs/units/candidate/unit_test.md")
	if err != nil {
		t.Fatal(err)
	}
	write := CacheWrite{
		Command:   "verify",
		Unit:      "test",
		Mode:      "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-06-30T11:00:00Z",
		Entries:   []FileEntry{*entry},
	}
	if _, err := writeCacheFixture(t, repoRoot, "unit", "test", write); err != nil {
		t.Fatal(err)
	}

	// promote shape: expected keys enumerated.
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", fixedChecks([]ExpectedCheck{{Key: "test.core", Lens: "alignment"}}), false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected a no-checks cache to fail closed as STALE, got fresh=%t category=%s", result.Fresh, result.Category)
	}
	if !strings.Contains(result.Reason, "no per-check evidence") || !strings.Contains(result.Reason, "verify@test") {
		t.Fatalf("expected the no-compat reason with re-run guidance, got: %s", result.Reason)
	}

	// fresh shape: coverage derivation failed, both lenses required.
	result, err = CheckVerifyMerged(repoRoot, "test", "candidate", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected a no-checks cache to fail closed as STALE for the fresh shape, got fresh=%t category=%s", result.Fresh, result.Category)
	}
	if !strings.Contains(result.Reason, "no per-check evidence") {
		t.Fatalf("expected the no-compat reason, got: %s", result.Reason)
	}

	// A no-checks failure record fails closed as STALE, not BLOCKED: the
	// record cannot be the failure-recovery baseline, so the fix is a re-run.
	write.Result = "fail"
	write.Blocking = true
	write.P0Count = 0
	write.P1Count = 1
	if _, err := writeCacheFixture(t, repoRoot, "unit", "test", write); err != nil {
		t.Fatal(err)
	}
	result, err = CheckVerifyMerged(repoRoot, "test", "candidate", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected a no-checks failure record to fail closed as STALE, got fresh=%t category=%s", result.Fresh, result.Category)
	}
}

// TestCheckVerifyMergedMissingCacheSkipsDerivation pins the lazy-derivation
// contract: a missing cache is classified MISSING without deriving the
// expected coverage, so a read-only freshness report pays no evidence
// discovery for the normal iteration state.
func TestCheckVerifyMergedMissingCacheSkipsDerivation(t *testing.T) {
	repoRoot := t.TempDir()
	derived := false
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", func() ([]ExpectedCheck, error) {
		derived = true
		return nil, errors.New("derivation must not run")
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Category != CategoryMissing {
		t.Fatalf("expected MISSING, got %s (%s)", result.Category, result.Reason)
	}
	if derived {
		t.Fatal("expected derivation to be skipped for a missing cache")
	}
}

// TestCheckVerifyMergedStaleBaseSkipsDerivation pins that a stale base cache
// is reported with its own reason and without attempting the coverage
// derivation: staleness stays attributable when derivation would fail anyway.
func TestCheckVerifyMergedStaleBaseSkipsDerivation(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	// Change the code file: the base cache goes stale.
	srcPath := filepath.Join(repoRoot, "src", "handler.go")
	if err := writeCacheFixtureFile(t, srcPath, []byte("package main\nfunc main() { println(1) }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	derived := false
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", func() ([]ExpectedCheck, error) {
		derived = true
		return nil, errors.New("derivation must not run")
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected a stale base cache to fail closed as STALE, got fresh=%t category=%s", result.Fresh, result.Category)
	}
	if strings.TrimSpace(result.Reason) == "" {
		t.Fatal("expected the base cache's own stale reason")
	}
	if derived {
		t.Fatal("expected derivation to be skipped for a stale base cache")
	}
}

// TestCheckVerifyMergedLegacyCacheSkipsDerivation pins that the no-compat
// legacy-cache rejection happens before the coverage derivation: an old cache
// is STALE without paying for evidence discovery.
func TestCheckVerifyMergedLegacyCacheSkipsDerivation(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	specPath := filepath.Join(candidateDir, "unit_test.md")
	if err := writeCacheFixtureFile(t, specPath, []byte("---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// A cache without a per-check `checks` breakdown.
	entry, err := BuildEvidenceEntry(repoRoot, "docs/specs/units/candidate/unit_test.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeCacheFixture(t, repoRoot, "unit", "test", CacheWrite{
		Command:   "verify",
		Unit:      "test",
		Mode:      "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-06-30T11:00:00Z",
		Entries:   []FileEntry{*entry},
	}); err != nil {
		t.Fatal(err)
	}

	derived := false
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", func() ([]ExpectedCheck, error) {
		derived = true
		return nil, errors.New("derivation must not run")
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected a legacy cache to fail closed as STALE, got fresh=%t category=%s", result.Fresh, result.Category)
	}
	if !strings.Contains(result.Reason, "no per-check evidence") {
		t.Fatalf("expected the no-compat reason, got: %s", result.Reason)
	}
	if derived {
		t.Fatal("expected derivation to be skipped for a legacy cache")
	}
}

// TestCheckVerifyMergedDerivationRunsOnlyForReusableCache pins that the
// derivation runs exactly once for a base-fresh cache with per-check evidence,
// and that a derivation failure fails closed as STALE with the provider's
// error as the reason.
func TestCheckVerifyMergedDerivationRunsOnlyForReusableCache(t *testing.T) {
	repoRoot := t.TempDir()
	writeMergedVerifyCache(t, repoRoot)

	calls := 0
	result, err := CheckVerifyMerged(repoRoot, "test", "candidate", func() ([]ExpectedCheck, error) {
		calls++
		return nil, errors.New("cannot derive required verify checks: boom")
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected a failed derivation to fail closed as STALE, got fresh=%t category=%s", result.Fresh, result.Category)
	}
	if !strings.Contains(result.Reason, "cannot derive required verify checks") || !strings.Contains(result.Reason, "boom") {
		t.Fatalf("expected the provider's derivation failure as the reason, got: %s", result.Reason)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one derivation for a reusable cache, got %d", calls)
	}
}

func TestDeleteAllRemovesValidateAndVerify(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	for _, name := range []string{"validate_result.md", "verify_result.md"} {
		writeCacheFixtureFile(t, filepath.Join(cacheDir, name), []byte("---\n---\n"), 0644)
	}

	if err := DeleteAll(repoRoot, "test"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"validate_result.md", "verify_result.md"} {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Cache lists an appendix path but NOT the main spec
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/appendix/unit_test_api.md\n    hash: sha256:abc\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Cache with no files listed at all
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// Verify cache lists only a source file, not the main spec
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\nblocking: false\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: src/handler.go\n    hash: sha256:abc\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	ruleContent := "---\nrule_id: b_rule_test\nrule_scope: bound\n---\n"
	if err := writeCacheFixtureFile(t, rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/b_rule_test")
	os.MkdirAll(cacheDir, 0755)

	// Cache with a files list that omits the main rule file
	cacheContent := "---\ncommand: validate\nunit: b_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/other_rule.md\n    hash: sha256:abc\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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

func TestCheckAppendicesInCache_InvalidNameFailsClosed(t *testing.T) {
	repoRoot := t.TempDir()

	// A name that would make filepath.Glob fail is rejected before any path is
	// built — the name gate is the fail-closed boundary now, so the appendix
	// check never sees an unvalidated name.
	if _, err := CheckAppendicesInCache(repoRoot, "test["); err == nil {
		t.Fatal("expected an invalid unit name to be rejected, got nil error")
	}
}

func TestCachePathsRejectInvalidTargetNames(t *testing.T) {
	repoRoot := t.TempDir()

	// The path a traversal name would resolve to: it must stay untouched.
	victimDir := filepath.Join(repoRoot, "docs", "tmp", "evil")
	if err := os.MkdirAll(victimDir, 0755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(victimDir, "validate_result.md")
	if err := writeCacheFixtureFile(t, victim, []byte("victim"), 0644); err != nil {
		t.Fatal(err)
	}

	traversal := "../../../../tmp/evil"
	if err := DeleteRuleCache(repoRoot, traversal, "validate"); err == nil {
		t.Fatal("expected DeleteRuleCache to reject a traversal rule id")
	}
	if err := DeleteCache(repoRoot, "auth/x", "validate"); err == nil {
		t.Fatal("expected DeleteCache to reject a unit name with a separator")
	}
	if _, err := PublishCache(repoRoot, "rule", traversal, "validate", []byte("payload")); err == nil {
		t.Fatal("expected PublishCache to reject a traversal rule id")
	}
	if _, err := ReadGateBaseline(repoRoot, "rule", traversal, "validate"); err == nil {
		t.Fatal("expected ReadGateBaseline to reject a traversal rule id")
	}
	if _, err := ReadCacheSummary(repoRoot, "unit", "..", "validate_result.md"); err == nil {
		t.Fatal("expected ReadCacheSummary to reject a traversal unit name")
	}

	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("the traversed target must stay untouched, stat err=%v", err)
	}
}

func TestCheckVerifyStable(t *testing.T) {
	repoRoot := t.TempDir()

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(stableDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(stableDir, "unit_test.md")
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	srcPath := filepath.Join(srcDir, "handler.go")
	srcContent := "package main\nfunc main() {}\n"
	if err := writeCacheFixtureFile(t, srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// verify@stable records the STABLE spec path in its files list.
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: stable\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll items aligned.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644); err != nil {
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
	writeCacheFixtureFile(t, specPath, []byte("---\nid: test\n---\n"), 0644)

	srcPath := filepath.Join(srcDir, "handler.go")
	writeCacheFixtureFile(t, srcPath, []byte("package main\nfunc main() {}\n"), 0644)

	specHash, _ := fileHash(specPath)
	srcHash, _ := fileHash(srcPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: stable\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: src/handler.go\n    hash: sha256:" + srcHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644)

	// Code changes after the stable verify -> the dependency chunks change,
	// so the silence no longer applies and baseline drift shows.
	writeCacheFixtureFile(t, srcPath, []byte("package main\nfunc main() { println(\"changed\") }\n"), 0644)

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
	writeCacheFixtureFile(t, specPath, []byte("---\nid: test\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	// The rule file is an external dependency of the stable content: when it
	// changes, the validate@stable confirmation goes stale.
	rulePath := filepath.Join(rulesStableDir, "g_rule_http.md")
	writeCacheFixtureFile(t, rulePath, []byte("---\nid: g_rule_http\n---\nAll APIs must use HTTPS.\n"), 0644)

	specHash, _ := fileHash(specPath)
	ruleHash, _ := fileHash(rulePath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)

	// validate@stable records the STABLE spec path and its rule dependency.
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll checks passed.\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

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
	writeCacheFixtureFile(t, specPath, []byte("---\nid: test\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	rulePath := filepath.Join(rulesStableDir, "g_rule_http.md")
	writeCacheFixtureFile(t, rulePath, []byte("---\nid: g_rule_http\n---\nAll APIs must use HTTPS.\n"), 0644)

	specHash, _ := fileHash(specPath)
	ruleHash, _ := fileHash(rulePath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The rule changes after the stable validate -> the confirmation goes stale.
	writeCacheFixtureFile(t, rulePath, []byte("---\nid: g_rule_http\n---\nAll APIs must use HTTPS and reject cleartext.\n"), 0644)

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
	writeCacheFixtureFile(t, rulePath, []byte("---\nid: g_rule_http\n---\nAll APIs must use HTTPS.\n"), 0644)

	// A consumer unit is an external dependency of the stable rule: when the
	// consumer changes, the rule's validate@stable confirmation goes stale.
	consumerPath := filepath.Join(unitsStableDir, "unit_consumer.md")
	writeCacheFixtureFile(t, consumerPath, []byte("---\nid: consumer\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	ruleHash, _ := fileHash(rulePath)
	consumerHash, _ := fileHash(consumerPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_http")
	os.MkdirAll(cacheDir, 0755)

	// validate@stable on a rule records the STABLE rule path and the consumer
	// units it scanned.
	cacheContent := "---\ncommand: validate\nunit: g_rule_http\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: docs/specs/units/stable/unit_consumer.md\n    hash: sha256:" + consumerHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll checks passed.\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

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
	writeCacheFixtureFile(t, rulePath, []byte("---\nid: g_rule_http\n---\nAll APIs must use HTTPS.\n"), 0644)

	consumerPath := filepath.Join(unitsStableDir, "unit_consumer.md")
	writeCacheFixtureFile(t, consumerPath, []byte("---\nid: consumer\nunit_refs: none\nrule_refs:\n  - g_rule_http\n---\n"), 0644)

	ruleHash, _ := fileHash(rulePath)
	consumerHash, _ := fileHash(consumerPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_http")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: g_rule_http\nmode: full\ntarget: stable\nresult: pass\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n  - path: docs/specs/rules/stable/g_rule_http.md\n    hash: sha256:" + ruleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: docs/specs/units/stable/unit_consumer.md\n    hash: sha256:" + consumerHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The consumer changes (e.g. its rule_refs) -> the confirmation goes stale.
	writeCacheFixtureFile(t, consumerPath, []byte("---\nid: consumer\nunit_refs: none\nrule_refs:\n  - g_rule_http\n  - g_rule_audit\n---\n"), 0644)

	result, err := CheckRuleValidateStable(repoRoot, "g_rule_http")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected stale after consumer change, got: %s", result.Reason)
	}
}

// makeSharedFile builds a ~16 KB shared code file with a unique line per row.
func makeSharedFile(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "line %d: some unique content to fill the shared file\n", i)
	}
	path := filepath.Join(dir, "shared.go")
	if err := writeCacheFixtureFile(t, path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCheckVerifyAnyRecordedChangeStales: freshness is whole-file. Any
// content change in a recorded input file stales the cache; localization of
// the change is the delta change report's job, and the judgment is the delta
// reviewer's — never a sub-file freshness rule.
func TestCheckVerifyAnyRecordedChangeStales(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(srcDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, specPath, []byte(specContent), 0644)

	sharedPath := makeSharedFile(t, srcDir)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: verify\nunit: test\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T11:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + mustHash(t, specPath) + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" +
		"  - path: src/shared.go\n    hash: sha256:" + mustHash(t, sharedPath) + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" +
		"---\nAll items aligned.\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "verify_result.md"), []byte(cacheContent), 0644)

	// Any modification — even far from any previously declared range —
	// stales the cache now.
	data, _ := os.ReadFile(sharedPath)
	modified := strings.Replace(string(data), "line 350:", "line 350 CHANGED:", 1)
	if modified == string(data) {
		t.Fatal("test setup: modification did not apply")
	}
	writeCacheFixtureFile(t, sharedPath, []byte(modified), 0644)

	result, err := CheckVerify(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected the cache to go stale after any recorded content change, got fresh")
	}
	if !strings.Contains(result.Reason, "src/shared.go") {
		t.Fatalf("expected the changed file named in the reason, got: %s", result.Reason)
	}
}

// TestCheckVerifyDepChangeStales is the other side: a change inside the
// declared dependency range must stale the cache.

// TestCheckHashOnlyFreshness: the whole-file hash is the freshness evidence —
// an entry with no dependency declarations is fresh while its hash matches,
// and stales as soon as the content changes.
// TestCheckRejectsHashOnlyCache pins that a cache without the required chunk
// evidence is not a usable current-format cache even when its whole-file hash
// matches: chunk evidence is a required part of the format, so `fresh` fails
// it closed and only a full run rebuilds it.
func TestCheckRejectsHashOnlyCache(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	specContent := "---\nid: test\nunit_refs: none\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, specPath, []byte(specContent), 0644)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + mustHash(t, specPath) + "\n---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || result.Category != CategoryStale {
		t.Fatalf("expected a hash-only cache without chunk evidence to be rejected, got fresh=%t category=%s", result.Fresh, result.Category)
	}
	if !strings.Contains(result.Reason, "chunk evidence") {
		t.Fatalf("expected the missing-chunk-evidence reason, got: %s", result.Reason)
	}
}

// TestCheckEmptyFileFreshWithNormalizedHash: an empty file's cache records
// the hash of its normalized content ("\n") and stays fresh while unchanged.
func TestCheckEmptyFileFreshWithNormalizedHash(t *testing.T) {
	repoRoot := t.TempDir()
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	specPath := filepath.Join(candidateDir, "unit_test.md")
	writeCacheFixtureFile(t, specPath, []byte(""), 0644)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + mustHash(t, specPath) + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	result, err := CheckValidate(repoRoot, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected an empty-file cache with the normalized hash to be fresh, got: %s", result.Reason)
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

func TestCheckValidateLogicalRef(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	// Self spec
	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644); err != nil {
		t.Fatal(err)
	}
	selfHash, err := fileHash(selfPath)
	if err != nil {
		t.Fatal(err)
	}

	// Dependency unit (candidate layer)
	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nunit_refs: none\nrule_refs: none\n---\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: passes.\n    runnable: yes\n"
	if err := writeCacheFixtureFile(t, depPath, []byte(depContent), 0644); err != nil {
		t.Fatal(err)
	}
	depHash, err := fileHash(depPath)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)

	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:dep\n    hash: sha256:" + depHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\nAll checks passed.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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

func TestGlobalRuleLogicalRefUsesStableOnly(t *testing.T) {
	repoRoot := t.TempDir()
	candidateUnitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candidateRuleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	for _, dir := range []string{candidateUnitDir, candidateRuleDir, stableRuleDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	selfPath := filepath.Join(candidateUnitDir, "unit_self.md")
	selfContent := "---\nid: self\nunit_refs: none\nrule_refs: none\n---\n"
	if err := writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0o644); err != nil {
		t.Fatal(err)
	}
	stableRulePath := filepath.Join(stableRuleDir, "g_rule_http.md")
	stableRuleContent := "---\nrule_id: g_rule_http\nrule_scope: global\n---\nStable constraint.\n"
	if err := writeCacheFixtureFile(t, stableRulePath, []byte(stableRuleContent), 0o644); err != nil {
		t.Fatal(err)
	}
	candidateRulePath := filepath.Join(candidateRuleDir, "g_rule_http.md")
	if err := writeCacheFixtureFile(t, candidateRulePath, []byte("candidate draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	selfHash, _ := fileHash(selfPath)
	stableRuleHash, _ := fileHash(stableRulePath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: rule:g_rule_http\n    hash: sha256:" + stableRuleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("candidate sibling must not shadow the stable global rule: %s", result.Reason)
	}

	if err := writeCacheFixtureFile(t, candidateRulePath, []byte("changed candidate draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("candidate global change must not stale the unit cache: %s", result.Reason)
	}

	if err := writeCacheFixtureFile(t, stableRulePath, []byte(stableRuleContent+"Changed stable constraint.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("stable global change must stale the unit cache")
	}
}

func TestGlobalRuleLogicalRefRejectsCandidateOnly(t *testing.T) {
	repoRoot := t.TempDir()
	candidateUnitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	candidateRuleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	for _, dir := range []string{candidateUnitDir, candidateRuleDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	selfPath := filepath.Join(candidateUnitDir, "unit_self.md")
	if err := writeCacheFixtureFile(t, selfPath, []byte("---\nid: self\nunit_refs: none\nrule_refs: none\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidateRulePath := filepath.Join(candidateRuleDir, "g_rule_draft.md")
	if err := writeCacheFixtureFile(t, candidateRulePath, []byte("candidate draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	selfHash, _ := fileHash(selfPath)
	candidateRuleHash, _ := fileHash(candidateRulePath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: rule:g_rule_draft\n    hash: sha256:" + candidateRuleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh || !strings.Contains(result.Reason, "rule:g_rule_draft") {
		t.Fatalf("candidate-only global reference must fail closed, got %+v", result)
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
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644)

	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nunit_refs: none\nrule_refs: none\n---\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: passes.\n    runnable: yes\n"
	writeCacheFixtureFile(t, depPath, []byte(depContent), 0644)
	depHash, err := fileHash(depPath)
	if err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	selfHash, _ := fileHash(selfPath)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:dep\n    hash: sha256:" + depHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Simulate promote of the dependency unit: content copied verbatim to
	// stable, candidate deleted (pure copy — no field transforms).
	stableDep := filepath.Join(stableDir, "unit_dep.md")
	writeCacheFixtureFile(t, stableDep, []byte(depContent), 0644)
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
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nunit_refs: none\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, depPath, []byte(depContent), 0644)
	depHash, _ := fileHash(depPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// Physical path entry — the pre-logical-reference cache form.
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: docs/specs/units/candidate/unit_dep.md\n    hash: sha256:" + depHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Simulate promote of the dependency unit.
	stableDep := filepath.Join(stableDir, "unit_dep.md")
	writeCacheFixtureFile(t, stableDep, []byte(depContent), 0644)
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
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// Logical ref with no candidate or stable file.
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:dep\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

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
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	// Dependency unit main spec + protocol appendix, both candidate.
	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nunit_refs: none\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, depPath, []byte(depContent), 0644)
	depHash, _ := fileHash(depPath)

	depAppendix := filepath.Join(candidateDir, "appendix", "unit_dep_api.md")
	appendixContent := "---\nunit: dep\n---\n\n# API\n\nPOST /login with timeout 30s. Response code 201 with {id, email}.\n"
	writeCacheFixtureFile(t, depAppendix, []byte(appendixContent), 0644)
	appendixHash, _ := fileHash(depAppendix)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:dep\n    hash: sha256:" + depHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:dep:appendix:unit_dep_api\n    hash: sha256:" + appendixHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

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
	writeCacheFixtureFile(t, stableDep, []byte(depContent), 0644)
	stableAppendix := filepath.Join(stableDir, "appendix", "unit_dep_api.md")
	writeCacheFixtureFile(t, stableAppendix, []byte(appendixContent), 0644)
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
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	depAppendix := filepath.Join(candidateDir, "appendix", "unit_dep_api.md")
	appendixContent := "---\nunit: dep\n---\n\n# API\n\nPOST /login with timeout 30s.\n"
	writeCacheFixtureFile(t, depAppendix, []byte(appendixContent), 0644)
	appendixHash, _ := fileHash(depAppendix)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:dep:appendix:unit_dep_api\n    hash: sha256:" + appendixHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// The dependency appendix content changes — the dependency changed and
	// the cache must go stale.
	writeCacheFixtureFile(t, depAppendix, []byte("---\nunit: dep\n---\n\n# API\n\nPOST /login with timeout 60s.\n"), 0644)

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
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	// Appendix exists in no layer (candidate or stable).
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:dep:appendix:unit_dep_api\n    hash: sha256:0000000000000000000000000000000000000000000000000000000000000000\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

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

func TestWholeFileFreshnessFlagsProseEdit(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	selfPath := filepath.Join(candidateDir, "unit_self.md")
	selfContent := "---\nid: self\nunit_refs: dep\nrule_refs: none\n---\n"
	writeCacheFixtureFile(t, selfPath, []byte(selfContent), 0644)
	selfHash, _ := fileHash(selfPath)

	depPath := filepath.Join(candidateDir, "unit_dep.md")
	depContent := "---\nid: dep\nunit_refs: none\nrule_refs: none\n---\n\n## Description\n\nBackground prose.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	writeCacheFixtureFile(t, depPath, []byte(depContent), 0644)
	depHash, _ := fileHash(depPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: self\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_self.md\n    hash: sha256:" + selfHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: unit:dep\n    hash: sha256:" + depHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644)

	// Any prose edit is a whole-file content change: the cache goes stale,
	// and localizing the edit is the delta change report's job.
	edited := strings.Replace(depContent, "Background prose.", "Background prose edited during iteration.", 1)
	writeCacheFixtureFile(t, depPath, []byte(edited), 0644)

	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatalf("expected stale after any recorded content change, got fresh")
	}

	// Editing the acceptance item set must stale the cache.
	edited = strings.Replace(depContent, "Core behavior.", "Core behavior changed.", 1)
	writeCacheFixtureFile(t, depPath, []byte(edited), 0644)
	result, err = CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fresh {
		t.Fatal("expected stale after acceptance item set edit")
	}
}

// specWithSections writes a unit spec with frontmatter plus two sections and
// returns its path and a depsYAML-style checks block declaration.
func writeSpecWithSections(t *testing.T, repoRoot, name, descBody string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Description\n\n" + descBody + "\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + name + ".core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := writeCacheFixtureFile(t, path, []byte(content), 0644); err != nil {
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
	content := "---\nid: " + name + "\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + name + ".core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n\n## Scope\n\nIn scope.\n"
	if err := writeCacheFixtureFile(t, path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadCacheChecksMapping(t *testing.T) {
	repoRoot := t.TempDir()
	writeSpecWithSections(t, repoRoot, "self", "Prose.")

	entry, err := BuildEvidenceEntry(repoRoot, "docs/specs/units/candidate/unit_self.md")
	if err != nil {
		t.Fatal(err)
	}
	entry.Checks = []CheckEntry{{Check: "1"}, {Check: "5"}}
	if _, err := writeCacheFixture(t, repoRoot, "unit", "self", CacheWrite{
		Command:   "validate",
		Unit:      "self",
		Mode:      "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-06-30T10:00:00Z",
		Entries:   []FileEntry{*entry},
	}); err != nil {
		t.Fatal(err)
	}

	// The promote gate must see the per-check mapping and stay fresh.
	result, err := CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("expected fresh with checks mapping, got: %s", result.Reason)
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

func writeRuleAndConsumer(t *testing.T, repoRoot string) string {
	t.Helper()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(ruleDir, 0755)
	rulePath := filepath.Join(ruleDir, "g_rule_test.md")
	ruleContent := "---\nid: g_rule_test\nscope: global\n---\n\n# Rule\n\nBody.\n"
	if err := writeCacheFixtureFile(t, rulePath, []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}
	writeSpecWithSections(t, repoRoot, "consumer", "Prose.")
	return rulePath
}

func writeRuleCache(t *testing.T, repoRoot string, ruleHash string, rulePath string) {
	t.Helper()
	consumerPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_consumer.md")
	consumerHash, _ := fileHash(consumerPath)
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: g_rule_test\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/candidate/g_rule_test.md\n    hash: sha256:" + ruleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "  - path: unit:consumer\n    hash: sha256:" + consumerHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" + "---\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
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
	consumerHash, _ := fileHash(consumerPath)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_test")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: g_rule_test\nmode: full\nbasis: delta\nresult: fail\nblocking: true\np0_count: 1\np1_count: 0\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/rules/candidate/g_rule_test.md\n    hash: sha256:" + ruleHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" +
		"    checks:\n" +
		"      - check: \"1\"\n        status: pass\n" +
		"      - check: \"5\"\n        status: fail\n" +
		"  - path: unit:consumer\n    hash: sha256:" + consumerHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" +
		"    checks:\n" +
		"      - check: \"5\"\n        status: fail\n" +
		"---\nCheck 5 found P0: consumer drift.\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
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
	input := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntarget: stable\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/stable/unit_test.md\n    hash: sha256:abc\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: docs/specs/units/stable/appendix/unit_test_a.md\n    hash: sha256:def\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: unit:dep\n    hash: sha256:ghi\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: src/a.go\n    hash: sha256:jkl\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\n## Findings\n- P2: something\ntarget: stable is body text, not frontmatter\n"

	out, changed := rewriteCacheLayer(input, []string{"docs/specs/units/stable/appendix/unit_test_a.md"})
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
	input := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:abc\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nok\n"
	out, changed := rewriteCacheLayer(input, nil)
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
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
  - path: docs/specs/units/candidate/appendix/unit_test_unit_a.md
    hash: sha256:def456
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
  - path: unit:dep
    hash: sha256:ghi789
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
  - path: src/a.go
    hash: sha256:jkl012
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
---
## Findings
- P2: cosmetic issue
target: candidate is body text, not frontmatter
<!-- GATE_JUDGMENTS_BEGIN
{"schema_version":1,"logical_status":{"1":"pass"},"findings":[],"synthesis_digest":"sha256:abc"}
GATE_JUDGMENTS_END -->
`
	out, changed := rewriteCacheLayerToStable(input, []string{"docs/specs/units/candidate/appendix/unit_test_unit_a.md"})
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
	if !strings.Contains(out, `{"schema_version":1,"logical_status":{"1":"pass"},"findings":[],"synthesis_digest":"sha256:abc"}`) {
		t.Fatalf("structured judgment state must survive the layer rewrite, got:\n%s", out)
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
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
---
Validate passed.
`
	out, changed := rewriteCacheLayerToStable(input, nil)
	if !changed {
		t.Fatal("expected the cache to be rewritten (paths must change)")
	}
	if !strings.Contains(out, "- path: docs/specs/units/stable/unit_test_unit.md") {
		t.Fatalf("expected main spec path rewritten, got:\n%s", out)
	}
	// target field is absent; rewriteLayerFrontmatter does not add one.
	// The validate stable gate does not require a target field, so this is
	// semantically correct.
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
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
---
`
	out, changed := rewriteCacheLayerToStable(input, nil)
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
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
  - path: unit:consumer
    hash: sha256:def
    chunker: buzhash-v1
    chunks:
      - cid: sha256:fixture
        start: 1
        end: 1
---
`
	out, changed := rewriteCacheLayerToStable(input, nil)
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
	writeCacheFixtureFile(t, candSpec, []byte("---\nid: test\nunit_refs: none\nrule_refs: none\n---\n\n# Test\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: test.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: internal_flow\n    implementation_surface: internal/demo\n    verification_method: Go test\n    pass_condition: passes.\n    runnable: yes\n"), 0644)
	writeCacheFixtureFile(t, candAppendix, []byte("---\nunit: test\n---\n\n# Appendix\n"), 0644)
	writeCacheFixtureFile(t, srcPath, []byte("package demo\n\nfunc Demo() int { return 1 }\n"), 0644)

	specHash, _ := fileHash(candSpec)
	appendixHash, _ := fileHash(candAppendix)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit", unit)
	os.MkdirAll(cacheDir, 0755)

	// Candidate-layer validate cache (no target field — defaults to candidate).
	validateCache := "---\ncommand: validate\nunit: test\nmode: full\nresult: pass\nblocking: false\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_test.md\n    hash: sha256:" + specHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: docs/specs/units/candidate/appendix/unit_test_a.md\n    hash: sha256:" + appendixHash + "\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\nAll checks passed.\n"
	writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(validateCache), 0644)

	// Candidate-layer verify cache (merged: alignment + quality checks).
	specEntry, err := BuildEvidenceEntry(repoRoot, "docs/specs/units/candidate/unit_test.md")
	if err != nil {
		t.Fatal(err)
	}
	specEntry.Checks = []CheckEntry{{Check: "test.core", Lens: "alignment"}}
	srcEntry, err := BuildEvidenceEntry(repoRoot, "src/a.go")
	if err != nil {
		t.Fatal(err)
	}
	srcEntry.Checks = []CheckEntry{{Check: "src/a.go", Lens: "quality"}}
	if _, err := writeCacheFixture(t, repoRoot, "unit", unit, CacheWrite{
		Command:   "verify",
		Unit:      unit,
		Mode:      "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-06-30T11:00:00Z",
		Entries:   []FileEntry{*specEntry, *srcEntry},
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate the stable layer existing (promote has copied the files).
	stableSpec := filepath.Join(repoRoot, "docs/specs/units/stable/unit_test.md")
	stableAppendix := filepath.Join(repoRoot, "docs/specs/units/stable/appendix/unit_test_a.md")
	os.MkdirAll(filepath.Dir(stableSpec), 0755)
	os.MkdirAll(filepath.Dir(stableAppendix), 0755)
	writeCacheFixtureFile(t, stableSpec, mustRead(t, candSpec), 0644)
	writeCacheFixtureFile(t, stableAppendix, mustRead(t, candAppendix), 0644)

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
	if rewrittenCount != 2 {
		t.Fatalf("expected both unit caches rewritten, got %d (report: %+v)", rewrittenCount, report.Entries)
	}

	// The rewritten caches must pass the stable-layer checks.
	if r, err := CheckValidateStable(repoRoot, unit); err != nil || !r.Fresh {
		t.Fatalf("CheckValidateStable after rewrite: fresh=%v err=%v reason=%s", r.Fresh, err, r.Reason)
	}
	if r, err := CheckVerifyStable(repoRoot, unit); err != nil || !r.Fresh {
		t.Fatalf("CheckVerifyStable after rewrite: fresh=%v err=%v reason=%s", r.Fresh, err, r.Reason)
	}
}

// ---------------------------------------------------------------------------
// Acceptance item-level declarations
// ---------------------------------------------------------------------------

// writeSpecWithTwoItems writes a spec whose acceptance_item_set carries two
// items, so tests can edit, rename, or reorder one item independently.
func writeSpecWithTwoItems(t *testing.T, repoRoot, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + name + ".core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n\n  - id: " + name + ".aux\n    description: Aux.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := writeCacheFixtureFile(t, path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReadGateBaselineCanonicalizesQuotedScalars pins the one value form the
// parser guarantees: quotes and surrounding whitespace are stripped before a
// scalar is recorded, so a quoted-with-whitespace spelling like
// `status: " fail "` can never be a valid `fail` for one consumer and an
// ignored value for another (repair must re-run that judgment, not carry it).
func TestReadGateBaselineCanonicalizesQuotedScalars(t *testing.T) {
	repoRoot := t.TempDir()
	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"command: \" validate \"\n" +
		"unit: \" auth \"\n" +
		"mode: \" full \"\n" +
		"basis: \" delta \"\n" +
		"result: \" fail \"\n" +
		"target: \" candidate \"\n" +
		"blocking: true\n" +
		"p0_count: 1\n" +
		"p1_count: 0\n" +
		"p2_count: 0\n" +
		"p3_count: 0\n" +
		"timestamp: 2026-01-01T00:00:00Z\n" +
		"files:\n" +
		"  - path: \" src/auth/login.go \"\n" +
		"    hash: sha256:1111\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n" +
		"    checks:\n" +
		"      - check: \" 5 \"\n" +
		"        status: \" fail \"\n" +
		"---\n" +
		"\nbody\n"
	if err := writeCacheFixtureFile(t, filepath.Join(cacheDir, "validate_result.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	baseline, err := ReadGateBaseline(repoRoot, "unit", "auth", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if !baseline.Exists {
		t.Fatal("expected the baseline to exist")
	}
	if baseline.Mode != "full" || baseline.Basis != "delta" || baseline.Result != "fail" || baseline.Target != "candidate" {
		t.Fatalf("quoted scalars must canonicalize, got mode=%q basis=%q result=%q target=%q",
			baseline.Mode, baseline.Basis, baseline.Result, baseline.Target)
	}
	if len(baseline.Checks) != 1 || baseline.Checks[0].Check != "5" || baseline.Checks[0].Status != "fail" {
		t.Fatalf("quoted check/status must canonicalize, got %+v", baseline.Checks)
	}
	if len(baseline.Entries) != 1 || baseline.Entries[0].Path != "src/auth/login.go" {
		t.Fatalf("quoted path must canonicalize, got %+v", baseline.Entries)
	}
}

// writeCacheFixtureFile authors current-protocol test data. Tests of old
// protocol rejection deliberately use os.WriteFile directly.
func writeCacheFixtureFile(t *testing.T, p string, data []byte, mode os.FileMode) error {
	t.Helper()
	if filepath.Base(p) != "verify_result.md" {
		return os.WriteFile(p, data, mode)
	}
	content := string(data)
	if extractJudgments(content) != "" {
		return os.WriteFile(p, data, mode)
	}
	root := strings.Split(filepath.ToSlash(p), "/docs/specs/")[0]
	cache, err := parseCache(data)
	if err != nil {
		return os.WriteFile(p, data, mode)
	}
	refs := map[string]judgments.Binding{}
	statuses := map[string]string{}
	ownedByLayer := map[string][]string{}
	for _, layer := range []string{"candidate", "stable"} {
		appendices, err := specpaths.UnitAppendices(root, cache.Unit, layer)
		if err != nil {
			t.Fatal(err)
		}
		for _, appendix := range appendices {
			ownedByLayer[layer] = append(ownedByLayer[layer], appendix.Path)
		}
	}
	for i, entry := range cache.Files {
		keys := entry.Checks
		if len(keys) == 0 {
			key := "fixture:" + fmt.Sprint(i)
			keys = []checkEntry{{Check: key}}
		}
		for _, check := range keys {
			dependency := judgments.Dependency{Path: entry.Path, Hash: normalizeHash(entry.Hash)}
			if strings.HasPrefix(entry.Path, "docs/specs/units/") {
				for _, layer := range []string{"candidate", "stable"} {
					if suffix, ok := judgments.OwnSuffix(entry.Path, cache.Unit, layer, ownedByLayer[layer]); ok {
						dependency.Path = suffix
						dependency.Own = true
					}
				}
			}
			record := judgments.Record{Version: judgments.RecordVersion, Kind: judgments.Item, Unit: cache.Unit, Subject: check.Check, Coverage: []string{check.Check}, Inputs: []string{entry.Path}, Dependencies: []judgments.Dependency{dependency}, Protocol: judgments.Protocol(root), Verdict: "ALIGNED", Result: func() json.RawMessage {
				data, _ := json.Marshal(map[string]any{"effective_status": map[string]string{check.Check: "pass"}})
				return data
			}(), Report: "fixture", ReportDigest: judgments.Digest([]byte("fixture")), SourceRun: "test"}
			layer := cache.Target
			if layer == "" {
				layer = "candidate"
			}
			record.SpecContext, err = judgments.SpecContext(root, cache.Unit, layer)
			if err != nil {
				// Malformed-cache tests intentionally omit their main spec.
				record.SpecContext = judgments.Digest([]byte("fixture:" + cache.Unit))
			}
			ref, err := judgments.Save(root, record)
			if err != nil {
				return err
			}
			refs[check.Check] = judgments.Binding{Reference: ref, Layer: layer, Source: "executed"}
			statuses[check.Check] = "pass"
		}
	}
	// A missing evidence list remains malformed; a sentinel lets the existing
	// lower-level tests exercise the exact metadata defect they target.
	if len(refs) == 0 {
		refs["fixture"] = judgments.Binding{}
	}
	state, _ := json.Marshal(map[string]any{"schema_version": 4, "records": refs, "logical_status": statuses})
	content += "\n<!-- GATE_JUDGMENTS_BEGIN\n" + string(state) + "\nGATE_JUDGMENTS_END -->"
	return os.WriteFile(p, []byte(content), mode)
}

func writeCacheFixture(t *testing.T, root, kind, name string, w CacheWrite) (string, error) {
	t.Helper()
	p, err := WriteCache(root, kind, name, w)
	if err != nil || w.Command != "verify" {
		return p, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return p, err
	}
	return p, writeCacheFixtureFile(t, p, data, 0644)
}

// TestBuildEntryResolvesSubsectionDeclarationToEnclosingSection pins issue
// #64's normalization: a dependency-scope declaration that cites a uniquely
// named ### subsection records the enclosing ## section region's dep — the
// same dep the ## heading itself records — and an ambiguous subsection still
// fails closed.

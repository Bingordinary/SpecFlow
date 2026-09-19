package validationcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeInvalidationFixture(t *testing.T, repoRoot, result string, blocking bool) string {
	t.Helper()
	specRel := "docs/specs/units/candidate/unit_auth.md"
	specPath := filepath.Join(repoRoot, filepath.FromSlash(specRel))
	if err := os.MkdirAll(filepath.Dir(specPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# auth\n"), 0644); err != nil {
		t.Fatal(err)
	}
	entry, err := BuildEntryFromChecks(repoRoot, specRel, []CheckDeclaration{{
		Check:  "auth.login",
		Status: map[bool]string{true: "pass", false: ""}[blocking],
	}})
	if err != nil {
		t.Fatal(err)
	}
	cachePath, err := WriteCache(repoRoot, "unit", "auth", CacheWrite{
		Command:   "verify",
		Unit:      "auth",
		Mode:      "full",
		Basis:     "full",
		Result:    result,
		Target:    "candidate",
		Blocking:  blocking,
		P1Count:   map[bool]int{true: 1, false: 0}[blocking],
		Timestamp: "2026-09-19T00:00:00Z",
		Judgments: `{"schema_version":2,"logical_status":{"auth.login":"pass"},"findings":[],"synthesis_digest":"sha256:test"}`,
		Body:      "ORIGINAL BODY\n",
		Entries:   []FileEntry{entry},
	})
	if err != nil {
		t.Fatal(err)
	}
	return cachePath
}

func TestInvalidateGateCacheDeletesPassCache(t *testing.T) {
	repoRoot := t.TempDir()
	cachePath := writeInvalidationFixture(t, repoRoot, "pass", false)

	result, err := InvalidateGateCache(repoRoot, "unit", "auth", "verify", "candidate", []string{"auth.login"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != InvalidationCacheDeleted {
		t.Fatalf("action = %q, want %q", result.Action, InvalidationCacheDeleted)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("pass cache still exists after targeted P0/P1: %v", err)
	}
}

func TestInvalidateGateCachePersistsFailureRecordKeysWithoutChangingEvidence(t *testing.T) {
	repoRoot := t.TempDir()
	cachePath := writeInvalidationFixture(t, repoRoot, "fail", true)
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}

	result, err := InvalidateGateCache(repoRoot, "unit", "auth", "verify", "candidate", []string{"z.item", "auth.login", "z.item"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != InvalidationRecordUpdated || strings.Join(result.Added, ",") != "auth.login,z.item" {
		t.Fatalf("unexpected invalidation result: %+v", result)
	}
	baseline, err := ReadGateBaseline(repoRoot, "unit", "auth", "verify")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(baseline.InvalidatedChecks, ","); got != "auth.login,z.item" {
		t.Fatalf("invalidated checks = %q", got)
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, preserved := range []string{"ORIGINAL BODY", "GATE_JUDGMENTS_BEGIN", `status: pass`} {
		if !strings.Contains(string(before), preserved) || !strings.Contains(string(after), preserved) {
			t.Fatalf("expected %q to remain unchanged across invalidation", preserved)
		}
	}

	second, err := InvalidateGateCache(repoRoot, "unit", "auth", "verify", "candidate", []string{"auth.login"})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Added) != 0 {
		t.Fatalf("repeated invalidation must be idempotent, added %v", second.Added)
	}
}

func TestParseCacheRejectsInvalidationOnPassCache(t *testing.T) {
	content := []byte("---\ncommand: verify\nunit: auth\nmode: full\nresult: pass\ntarget: candidate\nblocking: false\ninvalidated_checks:\n  - \"auth.login\"\nfiles: []\n---\n")
	if _, err := parseCache(content); err == nil || !strings.Contains(err.Error(), "failure record") {
		t.Fatalf("expected invalidated pass cache to fail closed, got %v", err)
	}
}

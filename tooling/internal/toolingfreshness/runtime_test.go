package toolingfreshness

import (
	"strings"
	"testing"
)

func TestCheckProcessFailsClosedWhenBuildFingerprintIsMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeToolingRepo(t, repoRoot)

	original := BuildFingerprint
	BuildFingerprint = ""
	t.Cleanup(func() {
		BuildFingerprint = original
	})

	err := CheckProcess([]string{"review", "collect-default-scope", "--repo-root", repoRoot}, repoRoot)
	if err == nil {
		t.Fatalf("expected missing embedded fingerprint error")
	}
	if !strings.Contains(err.Error(), "missing embedded build fingerprint") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckProcessStillBypassesDoctorWhenBuildFingerprintIsMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeToolingRepo(t, repoRoot)

	original := BuildFingerprint
	BuildFingerprint = ""
	t.Cleanup(func() {
		BuildFingerprint = original
	})

	if err := CheckProcess([]string{"doctor", "--repo-root", repoRoot}, repoRoot); err != nil {
		t.Fatalf("doctor should bypass freshness gate, got %v", err)
	}
}

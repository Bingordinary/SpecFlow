package toolingfreshness

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(err.Error(), "cd ") || !strings.Contains(err.Error(), "go run ./cmd/specflowctl build-release") {
		t.Fatalf("error should include executable build-release recovery command, got: %v", err)
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

func TestCheckProcessFailsClosedForSourceRepoWhenBuildFingerprintIsMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeSourceToolingRepo(t, repoRoot)

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
	if !strings.Contains(err.Error(), "tooling") || strings.Contains(err.Error(), "specflow/tooling") {
		t.Fatalf("source-repo recovery command should use tooling root, got: %v", err)
	}
}

func TestCheckProcessRejectsExplicitRepoRootOutsideLayout(t *testing.T) {
	// An injected --repo-root pointing at a non-specFlow directory must not
	// bypass the freshness gate: the ErrNotFound pass-through is reserved
	// for the implicit working-directory case.
	foreign := t.TempDir()

	err := CheckProcess([]string{"promote", "--unit", "x", "--repo-root", foreign}, t.TempDir())
	if err == nil {
		t.Fatal("expected explicit repo-root outside a specFlow layout to be rejected")
	}
	if !strings.Contains(err.Error(), "does not point to a specFlow project layout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckProcessAllowsCwdOutsideLayout(t *testing.T) {
	// Without an explicit --repo-root, a non-specFlow working directory
	// still passes through so the tool remains usable outside projects.
	if err := CheckProcess([]string{"promote", "--unit", "x"}, t.TempDir()); err != nil {
		t.Fatalf("expected implicit cwd outside a specFlow layout to pass through, got %v", err)
	}
}

func TestCheckProcessAllowsExplicitRepoRootPointingAtCwdOutsideLayout(t *testing.T) {
	// An explicit --repo-root equal to the working directory is equivalent
	// to the implicit case and must keep passing through.
	cwd := t.TempDir()
	if err := CheckProcess([]string{"promote", "--unit", "x", "--repo-root", cwd}, cwd); err != nil {
		t.Fatalf("expected explicit repo-root equal to cwd to pass through, got %v", err)
	}
}

func TestCheckProcessAllowsExplicitRepoRootAliasingCwdViaSymlink(t *testing.T) {
	// An explicit --repo-root that aliases the working directory through a
	// symlink is equivalent to the implicit case and must keep passing
	// through (e.g. /tmp vs /private/tmp on macOS).
	cwd := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(cwd, alias); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := CheckProcess([]string{"promote", "--unit", "x", "--repo-root", alias}, cwd); err != nil {
		t.Fatalf("expected symlink aliasing cwd to pass through, got %v", err)
	}
}

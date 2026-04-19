package install

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/buildrelease"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

func TestDoctorPassesForFreshBinary(t *testing.T) {
	repoRoot := t.TempDir()
	liveFingerprint := setupDoctorRepo(t, repoRoot)
	writeFingerprintProbeBinary(t, repoRoot, liveFingerprint)

	result, err := Doctor(repoRoot)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures, got %v", result.Failures)
	}
}

func TestDoctorFailsForStaleBinary(t *testing.T) {
	repoRoot := t.TempDir()
	_ = setupDoctorRepo(t, repoRoot)
	writeFingerprintProbeBinary(t, repoRoot, "stale-binary-fingerprint")

	result, err := Doctor(repoRoot)
	if err != nil {
		t.Fatalf("Doctor returned unexpected error: %v", err)
	}

	joined := strings.Join(result.Failures, "\n")
	if !strings.Contains(joined, "STALE specflow/tooling/bin/"+buildrelease.CurrentBinaryName()) {
		t.Fatalf("expected stale binary failure, got %v", result.Failures)
	}
}

func setupDoctorRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), "templates/root/.githooks/pre-commit\t.githooks/pre-commit\tframework\n")
	mustWriteFile(t, filepath.Join(repoRoot, ".githooks/pre-commit"), "#!/usr/bin/env bash\nspecflow/tooling/bin/specflowctl-linux-amd64 entry sync --stage\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/cmd/specflowctl/main.go"), "package main\n\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/internal/demo/demo.go"), "package demo\n\nfunc Value() string { return \"demo\" }\n")

	fingerprint, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}
	return fingerprint
}

func writeFingerprintProbeBinary(t *testing.T, repoRoot, fingerprint string) {
	t.Helper()
	path := filepath.Join(repoRoot, "specflow/tooling/bin", buildrelease.CurrentBinaryName())
	script := "#!/usr/bin/env bash\nif [[ \"$1\" == \"" + toolingfreshness.HiddenBuildFingerprintCommand + "\" ]]; then\n  printf '%s\\n' \"" + fingerprint + "\"\n  exit 0\nfi\nexit 0\n"
	if runtime.GOOS == "windows" {
		t.Fatalf("windows test environment is not supported for this script-based probe")
	}
	mustWriteExecutableFile(t, path, script)
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
	}
}

func mustWriteExecutableFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
	}
}

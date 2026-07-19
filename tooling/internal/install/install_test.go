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

func TestDoctorSkipsProjectModeItems(t *testing.T) {
	repoRoot := t.TempDir()
	setupDoctorRepoAt(t, repoRoot, "specflow", "specflow/tooling")
	// Add a project-mode entry to the manifest (doctor should skip it)
	addManifestEntry(t, repoRoot, "specflow/tooling/manifest.tsv", "templates/docs/mapping.md\tdocs/mapping.md\tproject")
	// Don't create the project file — doctor should not report it
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/templates/docs/mapping.md"), "repo mapping\n")

	liveFingerprint := setupDoctorRepoLiveFingerprint(t, repoRoot)
	writeFingerprintProbeBinary(t, repoRoot, liveFingerprint)

	result, err := Doctor(repoRoot)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	for _, failure := range result.Failures {
		if strings.Contains(failure, "docs/mapping.md") {
			t.Fatalf("Doctor reported project-mode file as missing: %s", failure)
		}
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
	return setupDoctorRepoAt(t, repoRoot, "specflow", "specflow/tooling")
}

func setupDoctorRepoAt(t *testing.T, repoRoot, contentRoot, toolingRoot string) string {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "manifest.tsv"), strings.Join([]string{
		"templates/AGENTS.md\tAGENTS.md\tframework",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(contentRoot), "templates/AGENTS.md"), "template\n==SPECFLOW:BEGIN==\nmanaged\n==SPECFLOW:END==\n")
	mustWriteFile(t, filepath.Join(repoRoot, "AGENTS.md"), "host\n==SPECFLOW:BEGIN==\nmanaged\n==SPECFLOW:END==\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "cmd/specflowctl/main.go"), "package main\n\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "internal/demo/demo.go"), "package demo\n\nfunc Value() string { return \"demo\" }\n")

	return setupDoctorRepoLiveFingerprint(t, repoRoot)
}

func setupDoctorRepoLiveFingerprint(t *testing.T, repoRoot string) string {
	t.Helper()
	fingerprint, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}
	return fingerprint
}

func addManifestEntry(t *testing.T, repoRoot, manifestPath, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(repoRoot, filepath.FromSlash(manifestPath)), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(manifest.tsv) failed: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("WriteString(manifest.tsv) failed: %v", err)
	}
}

func writeFingerprintProbeBinary(t *testing.T, repoRoot, fingerprint string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script-based executable probe is not supported on windows")
	}
	script := "#!/usr/bin/env bash\nif [[ \"$1\" == \"" + toolingfreshness.HiddenBuildFingerprintCommand + "\" ]]; then\n  printf '%s\\n' \"" + fingerprint + "\"\n  exit 0\nfi\nexit 0\n"
	mustWriteExecutableFile(t, filepath.Join(repoRoot, "specflow/tooling/bin", buildrelease.CurrentBinaryName()), script)
}

func TestCheckProjectInitPassesWhenProjectFilesPresent(t *testing.T) {
	repoRoot := t.TempDir()
	// Setup a repo as InstalledProject with a project-mode manifest entry
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), strings.Join([]string{
		"templates/docs/guide.md\tdocs/guide.md\tproject",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	// Create both the template source and the destination project file
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/templates/docs/guide.md"), "guide template\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/guide.md"), "guide project file\n")

	result, err := CheckProjectInit(repoRoot)
	if err != nil {
		t.Fatalf("CheckProjectInit returned error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures, got %v", result.Failures)
	}
}

func TestCheckProjectInitFailsWhenProjectFileMissing(t *testing.T) {
	repoRoot := t.TempDir()
	// Setup a repo as InstalledProject with a project-mode manifest entry
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), strings.Join([]string{
		"templates/docs/guide.md\tdocs/guide.md\tproject",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	// Create the template but NOT the destination project file
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/templates/docs/guide.md"), "guide template\n")

	result, err := CheckProjectInit(repoRoot)
	if err != nil {
		t.Fatalf("CheckProjectInit returned error: %v", err)
	}
	if len(result.Failures) == 0 {
		t.Fatal("expected CheckProjectInit to report missing project file, but got no failures")
	}
	joined := strings.Join(result.Failures, "\n")
	if !strings.Contains(joined, "MISSING docs/guide.md") {
		t.Fatalf("expected MISSING docs/guide.md, got %v", result.Failures)
	}
}

func TestCheckProjectInitSkipsFrameworkModeItems(t *testing.T) {
	repoRoot := t.TempDir()
	// Setup a repo as InstalledProject with ONLY framework-mode entries
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), strings.Join([]string{
		"templates/AGENTS.md\tAGENTS.md\tframework",
	}, "\n")+"\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")

	result, err := CheckProjectInit(repoRoot)
	if err != nil {
		t.Fatalf("CheckProjectInit returned error: %v", err)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures for framework-only manifest, got %v", result.Failures)
	}
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

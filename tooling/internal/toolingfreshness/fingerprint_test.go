package toolingfreshness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveFingerprintChangesWhenToolingSourceChanges(t *testing.T) {
	repoRoot := t.TempDir()
	writeToolingRepo(t, repoRoot)

	first, files, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("expected 4 fingerprint input files, got %d", len(files))
	}

	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/internal/demo/demo.go"), "package demo\n\nfunc Value() string { return \"changed\" }\n")

	second, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint after change returned error: %v", err)
	}
	if first == second {
		t.Fatalf("expected fingerprint to change after tooling source change")
	}
}

func TestLiveFingerprintChangesWhenManifestChanges(t *testing.T) {
	repoRoot := t.TempDir()
	writeToolingRepo(t, repoRoot)

	first, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), "templates/GEMINI.md\tGEMINI.md\tframework\n")

	second, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint after manifest change returned error: %v", err)
	}
	if first == second {
		t.Fatalf("expected fingerprint to change after manifest change")
	}
}

func TestLiveFingerprintIgnoresNonToolingFiles(t *testing.T) {
	repoRoot := t.TempDir()
	writeToolingRepo(t, repoRoot)

	first, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/README.md"), "# changed doc\n")

	second, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint after doc change returned error: %v", err)
	}
	if first != second {
		t.Fatalf("expected fingerprint to stay unchanged after non-tooling file change")
	}
}



func TestLiveFingerprintSupportsSourceRepoLayout(t *testing.T) {
	repoRoot := t.TempDir()
	writeSourceToolingRepo(t, repoRoot)

	_, files, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}
	if !containsPath(files, "tooling/go.mod") || !containsPath(files, "tooling/manifest.tsv") {
		t.Fatalf("expected source-repo tooling input paths, got %v", files)
	}
}

func TestLiveFingerprintMatchesAcrossLayouts(t *testing.T) {
	sourceRoot := t.TempDir()
	installedRoot := t.TempDir()
	writeSourceToolingRepo(t, sourceRoot)
	writeToolingRepo(t, installedRoot)

	sourceFingerprint, _, err := LiveFingerprint(sourceRoot)
	if err != nil {
		t.Fatalf("source LiveFingerprint returned error: %v", err)
	}
	installedFingerprint, _, err := LiveFingerprint(installedRoot)
	if err != nil {
		t.Fatalf("installed LiveFingerprint returned error: %v", err)
	}
	if sourceFingerprint != installedFingerprint {
		t.Fatalf("fingerprint differs across layouts: source=%s installed=%s", sourceFingerprint, installedFingerprint)
	}
}

func TestLiveFingerprintNormalizesCRLFLineEndings(t *testing.T) {
	lfRoot := t.TempDir()
	crlfRoot := t.TempDir()
	writeToolingRepo(t, lfRoot)
	writeToolingRepo(t, crlfRoot)
	convertFingerprintInputsToCRLF(t, filepath.Join(crlfRoot, "specflow/tooling"))

	lfFingerprint, _, err := LiveFingerprint(lfRoot)
	if err != nil {
		t.Fatalf("LF LiveFingerprint returned error: %v", err)
	}
	crlfFingerprint, _, err := LiveFingerprint(crlfRoot)
	if err != nil {
		t.Fatalf("CRLF LiveFingerprint returned error: %v", err)
	}
	if lfFingerprint != crlfFingerprint {
		t.Fatalf("fingerprint differs across line endings: lf=%s crlf=%s", lfFingerprint, crlfFingerprint)
	}
}

func TestShellFingerprintScriptMatchesLiveFingerprint(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not available")
	}
	if err := exec.Command("bash", "-lc", "true").Run(); err != nil {
		t.Skipf("bash is not usable in this environment: %v", err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	want, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}

	scriptPath := "tooling/scripts/tooling_fingerprint.sh"
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tooling_fingerprint.sh failed: %v\n%s", err, string(out))
	}
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("script fingerprint mismatch\ngot  %s\nwant %s", got, want)
	}

	shortCmd := exec.Command("bash", scriptPath, "--short")
	shortCmd.Dir = repoRoot
	shortOut, err := shortCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tooling_fingerprint.sh --short failed: %v\n%s", err, string(shortOut))
	}
	if got, want := strings.TrimSpace(string(shortOut)), want[:12]; got != want {
		t.Fatalf("script short fingerprint mismatch\ngot  %s\nwant %s", got, want)
	}
}

func TestPowerShellFingerprintScriptMatchesLiveFingerprint(t *testing.T) {
	powershellPath, ok := findPowerShell()
	if !ok {
		t.Skip("PowerShell is not available")
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	want, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}

	scriptPath := filepath.Join("tooling", "scripts", "tooling_fingerprint.ps1")
	cmd := exec.Command(powershellPath, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tooling_fingerprint.ps1 failed: %v\n%s", err, string(out))
	}
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("PowerShell script fingerprint mismatch\ngot  %s\nwant %s", got, want)
	}

	shortCmd := exec.Command(powershellPath, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-Short")
	shortCmd.Dir = repoRoot
	shortOut, err := shortCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tooling_fingerprint.ps1 -Short failed: %v\n%s", err, string(shortOut))
	}
	if got, want := strings.TrimSpace(string(shortOut)), want[:12]; got != want {
		t.Fatalf("PowerShell script short fingerprint mismatch\ngot  %s\nwant %s", got, want)
	}
}

func writeToolingRepo(t *testing.T, repoRoot string) {
	t.Helper()
	writeToolingRepoAt(t, repoRoot, "specflow/tooling")
}

func writeSourceToolingRepo(t *testing.T, repoRoot string) {
	t.Helper()
	writeToolingRepoAt(t, repoRoot, "tooling")
}

func writeToolingRepoAt(t *testing.T, repoRoot, toolingRoot string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "manifest.tsv"), "templates/AGENTS.md\tAGENTS.md\tframework\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "cmd/specflowctl/main.go"), "package main\n\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "internal/demo/demo.go"), "package demo\n\nfunc Value() string { return \"demo\" }\n")
}

func convertFingerprintInputsToCRLF(t *testing.T, toolingRoot string) {
	t.Helper()
	paths := []string{
		filepath.Join(toolingRoot, "go.mod"),
		filepath.Join(toolingRoot, "manifest.tsv"),
	}
	err := filepath.Walk(filepath.Join(toolingRoot, "cmd"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk cmd files: %v", err)
	}
	err = filepath.Walk(filepath.Join(toolingRoot, "internal"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal files: %v", err)
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) failed: %v", path, err)
		}
		lf := strings.ReplaceAll(string(content), "\r\n", "\n")
		crlf := strings.ReplaceAll(lf, "\n", "\r\n")
		if err := os.WriteFile(path, []byte(crlf), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) failed: %v", path, err)
		}
	}
}

func findPowerShell() (string, bool) {
	for _, name := range []string{"pwsh", "powershell.exe", "powershell"} {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, true
		}
	}
	return "", false
}

func containsPath(paths []string, expected string) bool {
	for _, path := range paths {
		if path == expected {
			return true
		}
	}
	return false
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

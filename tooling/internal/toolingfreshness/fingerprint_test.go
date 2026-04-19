package toolingfreshness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLiveFingerprintChangesWhenToolingSourceChanges(t *testing.T) {
	repoRoot := t.TempDir()
	writeToolingRepo(t, repoRoot)

	first, files, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 fingerprint input files, got %d", len(files))
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

func TestLiveFingerprintIgnoresNonToolingFiles(t *testing.T) {
	repoRoot := t.TempDir()
	writeToolingRepo(t, repoRoot)

	first, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specflow_go_tooling.md"), "# changed doc\n")

	second, _, err := LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint after doc change returned error: %v", err)
	}
	if first != second {
		t.Fatalf("expected fingerprint to stay unchanged after non-tooling file change")
	}
}

func writeToolingRepo(t *testing.T, repoRoot string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/cmd/specflowctl/main.go"), "package main\n\nfunc main() {}\n")
	mustWriteFile(t, filepath.Join(repoRoot, "specflow/tooling/internal/demo/demo.go"), "package demo\n\nfunc Value() string { return \"demo\" }\n")
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

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

func TestToolingFingerprintCommand(t *testing.T) {
	repoRoot := t.TempDir()
	writeFingerprintCLITestRepo(t, repoRoot)

	want, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runToolingFingerprint([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("tooling-fingerprint failed: %v\nstderr=%s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("fingerprint mismatch\ngot  %s\nwant %s", got, want)
	}
}

func TestToolingFingerprintCommandShort(t *testing.T) {
	repoRoot := t.TempDir()
	writeFingerprintCLITestRepo(t, repoRoot)

	want, _, err := toolingfreshness.LiveFingerprint(repoRoot)
	if err != nil {
		t.Fatalf("LiveFingerprint returned error: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runToolingFingerprint([]string{"--short", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("tooling-fingerprint --short failed: %v\nstderr=%s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != want[:12] {
		t.Fatalf("short fingerprint mismatch\ngot  %s\nwant %s", got, want[:12])
	}
}

func writeFingerprintCLITestRepo(t *testing.T, repoRoot string) {
	t.Helper()
	mustWriteCLI(t, filepath.Join(repoRoot, "tooling/go.mod"), "module github.com/Bingordinary/SpecFlow/specflow/tooling\n\ngo 1.22.2\n")
	mustWriteCLI(t, filepath.Join(repoRoot, "tooling/manifest.tsv"), "templates/AGENTS.md\tAGENTS.md\tframework\n")
	mustWriteCLI(t, filepath.Join(repoRoot, "tooling/cmd/specflowctl/main.go"), "package main\n\nfunc main() {}\n")
	mustWriteCLI(t, filepath.Join(repoRoot, "tooling/internal/demo/demo.go"), "package demo\n\nfunc Value() string { return \"demo\" }\n")
}

func mustWriteCLI(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) failed: %v", path, err)
	}
}

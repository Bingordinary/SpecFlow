package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGateEvidenceBasic(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	path := filepath.Join(srcDir, "auth.go")
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "line %d: some unique content\n", i)
	}
	os.WriteFile(path, []byte(b.String()), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "1-20"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "file: src/auth.go") {
		t.Fatalf("expected file line, got:\n%s", out)
	}
	if !strings.Contains(out, "hash: sha256:") {
		t.Fatalf("expected hash line, got:\n%s", out)
	}
	if !strings.Contains(out, "deps:") {
		t.Fatalf("expected deps block, got:\n%s", out)
	}
	if !strings.Contains(out, "- sha256:") {
		t.Fatalf("expected at least one dependency CID, got:\n%s", out)
	}
}

func TestGateEvidenceWholeFile(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	path := filepath.Join(srcDir, "auth.go")
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "line %d: some unique content\n", i)
	}
	os.WriteFile(path, []byte(b.String()), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v", err)
	}
	// No ranges declared: the whole file is the dependency.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	depLines := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "  - sha256:") {
			depLines++
		}
	}
	if depLines == 0 {
		t.Fatalf("expected whole-file dependency CIDs, got:\n%s", stdout.String())
	}
}

func TestGateEvidenceRangeCoversContent(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "auth.go"), []byte("package main\n"), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "1-1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v", err)
	}
	out := stdout.String()
	// Range 1-1 covers line 1 ("package main") — must map to the file's
	// single chunk, so deps must not be empty.
	if !strings.Contains(out, "- sha256:") {
		t.Fatalf("expected dependency CIDs for a content range, got:\n%s", out)
	}
}

func TestGateEvidenceRangeOutOfBounds(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "auth.go"), []byte("package main\n"), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "99-100"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for out-of-bounds range")
	}
	if !strings.Contains(err.Error(), "exceeds the file's line count") {
		t.Fatalf("expected line-count error, got: %v", err)
	}
}

func TestGateEvidenceMissingFileFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing --file")
	}
}

func TestGateEvidenceMissingFile(t *testing.T) {
	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/nope.go"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestGateEvidenceMalformedRanges(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "auth.go"), []byte("package main\n"), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "abc"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for malformed ranges")
	}
}

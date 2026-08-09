package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextDiscoversCandidateSpec(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_demo.md"), []byte("# Demo\n"), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--unit", "demo", "--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "Candidate: true") {
		t.Errorf("expected 'Candidate: true' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "unit_demo.md") {
		t.Errorf("expected candidate filename in output, got:\n%s", output)
	}
}

func TestNextDiscoversNoFiles(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--unit", "nonexistent", "--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "Candidate: false") {
		t.Errorf("expected 'Candidate: false' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Stable: false") {
		t.Errorf("expected 'Stable: false' in output, got:\n%s", output)
	}
}

func TestNextUsageWithoutUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output, got:\n%s", output)
	}
}

func TestNextRelatedUnitsBlockList(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_demo.md"), []byte(`---
id: demo
version: 1.0.0
unit_refs:
  - payment
  - auth
rule_refs: none
---

# Demo
`), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--unit", "demo", "--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "Related units:") {
		t.Errorf("expected 'Related units:' in output for block-list unit_refs, got:\n%s", output)
	}
	if !strings.Contains(output, "- payment") || !strings.Contains(output, "- auth") {
		t.Errorf("expected related units payment and auth in output, got:\n%s", output)
	}
}

func TestNextRelatedUnitsInlineList(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_demo.md"), []byte(`---
id: demo
version: 1.0.0
unit_refs: [payment, auth]
rule_refs: none
---

# Demo
`), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--unit", "demo", "--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "- payment") || !strings.Contains(output, "- auth") {
		t.Errorf("expected related units payment and auth in output, got:\n%s", output)
	}
}

func TestNextRelatedUnitsStableFallback(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	os.WriteFile(filepath.Join(stableDir, "unit_demo.md"), []byte(`---
id: demo
version: 1.0.0
unit_refs:
  - payment
rule_refs: none
---

# Demo
`), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--unit", "demo", "--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "Related units:") {
		t.Errorf("expected 'Related units:' in output from stable fallback, got:\n%s", output)
	}
	if !strings.Contains(output, "- payment") {
		t.Errorf("expected related unit payment from stable fallback, got:\n%s", output)
	}
}

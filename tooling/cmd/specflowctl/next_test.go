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

func TestNextEmitsAcceptanceItemFields(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_demo.md"), []byte(`---
id: demo
version: 1.0.0
unit_refs: none
rule_refs: none
---

# Demo

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: demo.login
    description: Login.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/auth
    verification_method: test
    pass_condition: Returns 200.
    runnable: yes
    affects:
      files:
        - src/auth/login.go
        - src/auth/token.go
  - id: demo.register
    description: Register.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/auth
    verification_method: test
    pass_condition: Returns 201.
    runnable: yes
    affects:
      files:
        - src/auth/register.go
`), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--unit", "demo", "--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "Implementation surface:") {
		t.Errorf("expected 'Implementation surface:' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "  - src/auth") {
		t.Errorf("expected deduplicated implementation surface in output, got:\n%s", output)
	}
	if strings.Count(output, "  - src/auth\n") != 1 {
		t.Errorf("expected implementation surface deduplicated to one entry, got:\n%s", output)
	}
	if !strings.Contains(output, "Affects files:") {
		t.Errorf("expected 'Affects files:' in output, got:\n%s", output)
	}
	for _, f := range []string{"src/auth/login.go", "src/auth/token.go", "src/auth/register.go"} {
		if !strings.Contains(output, "- "+f) {
			t.Errorf("expected affects file %s in output, got:\n%s", f, output)
		}
	}
	if !strings.Contains(output, "Acceptance items:") {
		t.Errorf("expected 'Acceptance items:' in output, got:\n%s", output)
	}
	for _, id := range []string{"demo.login", "demo.register"} {
		if !strings.Contains(output, "- "+id) {
			t.Errorf("expected acceptance item %s in output, got:\n%s", id, output)
		}
	}
}

func TestNextOmitsAcceptanceItemFieldsWhenAbsent(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_demo.md"), []byte(`---
id: demo
version: 1.0.0
unit_refs: none
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

	for _, section := range []string{"Implementation surface:", "Affects files:", "Acceptance items:"} {
		if strings.Contains(output, section) {
			t.Errorf("expected %q absent from output, got:\n%s", section, output)
		}
	}
}

func TestNextAcceptanceItemFieldsStableFallback(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	os.WriteFile(filepath.Join(stableDir, "unit_demo.md"), []byte(`---
id: demo
version: 1.0.0
unit_refs: none
rule_refs: none
---

# Demo

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: demo.core
    description: Core.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/core
    verification_method: test
    pass_condition: Passes.
    runnable: yes
    affects:
      files:
        - src/core.go
`), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--unit", "demo", "--repo-root", repoRoot}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runNext failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()

	if !strings.Contains(output, "Implementation surface:") {
		t.Errorf("expected 'Implementation surface:' from stable fallback, got:\n%s", output)
	}
	if !strings.Contains(output, "  - src/core") {
		t.Errorf("expected implementation surface from stable fallback, got:\n%s", output)
	}
	if !strings.Contains(output, "Acceptance items:") {
		t.Errorf("expected 'Acceptance items:' from stable fallback, got:\n%s", output)
	}
	if !strings.Contains(output, "- demo.core") {
		t.Errorf("expected acceptance item from stable fallback, got:\n%s", output)
	}
}

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func nextDiscoverySpec(layer string) string {
	return fmt.Sprintf(`---
id: demo
unit_refs: [demo, %[1]s_peer, %[1]s_peer]
rule_refs: [b_rule_%[1]s]
---

# Demo

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: demo.%[1]s
    implementation_surface: src/%[1]s.go
    affects:
      files:
        - src/%[1]s.go
`, layer)
}

func TestNextSelectedSpecDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name, selected    string
		candidate, stable bool
		unreadableStable  bool
	}{
		{name: "candidate preferred", selected: "new", candidate: true, stable: true},
		{name: "stable only", selected: "old", stable: true},
		{name: "no files"},
		{name: "unselected stable is not read", selected: "new", candidate: true, stable: true, unreadableStable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := createCLITestRepo(t)
			candidate := "docs/specs/units/candidate/unit_demo.md"
			stable := "docs/specs/units/stable/unit_demo.md"
			if tc.candidate {
				grWriteFile(t, root, candidate, nextDiscoverySpec("new"))
			}
			if tc.stable {
				grWriteFile(t, root, stable, nextDiscoverySpec("old"))
			}
			if tc.unreadableStable {
				denyNextDiscoveryAccess(t, filepath.Join(root, filepath.FromSlash(stable)), false)
			}
			var stdout, stderr bytes.Buffer
			if err := runNext([]string{"--unit", "demo", "--repo-root", root}, &stdout, &stderr); err != nil {
				t.Fatalf("next failed: %v; stderr=%s", err, stderr.String())
			}
			output := stdout.String()
			for _, state := range []string{fmt.Sprintf("Candidate: %v", tc.candidate), fmt.Sprintf("Stable: %v", tc.stable)} {
				if !strings.Contains(output, state) {
					t.Errorf("missing %q in %s", state, output)
				}
			}
			if tc.selected == "" {
				if !strings.Contains(output, "(no design recorded)") {
					t.Errorf("missing empty state: %s", output)
				}
				return
			}
			for _, field := range []string{tc.selected + "_peer", "b_rule_" + tc.selected, "src/" + tc.selected + ".go", "demo." + tc.selected} {
				if !strings.Contains(output, field) {
					t.Errorf("missing selected-spec field %q in %s", field, output)
				}
			}
			other := "old"
			if tc.selected == "old" {
				other = "new"
			}
			for _, field := range []string{other + "_peer", "b_rule_" + other, "src/" + other + ".go", "demo." + other} {
				if strings.Contains(output, field) {
					t.Errorf("unexpected unselected-spec field %q in %s", field, output)
				}
			}
			if strings.Count(output, "  - "+tc.selected+"_peer\n") != 1 || strings.Contains(output, "  - demo\n") {
				t.Errorf("unit refs must exclude self and duplicates: %s", output)
			}
		})
	}
}

func TestNextRejectsFilesystemErrors(t *testing.T) {
	for _, tc := range []struct {
		name, layer, failure string
		candidate, stable    bool
	}{
		{name: "unreadable candidate with stable", layer: "candidate", failure: "read", candidate: true, stable: true},
		{name: "unreadable candidate only", layer: "candidate", failure: "read", candidate: true},
		{name: "unreadable stable only", layer: "stable", failure: "read", stable: true},
		{name: "candidate stat failure", layer: "candidate", failure: "stat", candidate: true, stable: true},
		{name: "stable stat failure", layer: "stable", failure: "stat", candidate: true, stable: true},
		{name: "candidate is directory", layer: "candidate", failure: "directory", candidate: true, stable: true},
		{name: "stable is directory", layer: "stable", failure: "directory", stable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := createCLITestRepo(t)
			if tc.candidate {
				grWriteFile(t, root, "docs/specs/units/candidate/unit_demo.md", nextDiscoverySpec("new"))
			}
			if tc.stable {
				grWriteFile(t, root, "docs/specs/units/stable/unit_demo.md", nextDiscoverySpec("old"))
			}
			ref := "docs/specs/units/" + tc.layer + "/unit_demo.md"
			path := filepath.Join(root, filepath.FromSlash(ref))
			switch tc.failure {
			case "read":
				denyNextDiscoveryAccess(t, path, false)
			case "stat":
				denyNextDiscoveryAccess(t, filepath.Dir(path), true)
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			err := runNext([]string{"--unit", "demo", "--repo-root", root}, &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), ref) {
				t.Fatalf("expected discovery error naming %s, got %v; stdout=%s", ref, err, stdout.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("failed discovery emitted successful metadata: %s", stdout.String())
			}
		})
	}
}

func denyNextDiscoveryAccess(t *testing.T, path string, directory bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permission denial via chmod is not supported on Windows")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, info.Mode().Perm()); err != nil {
			t.Errorf("restore permissions: %v", err)
		}
	})
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatal(err)
	}
	if directory {
		_, err = os.ReadDir(path)
	} else {
		_, err = os.ReadFile(path)
	}
	if !os.IsPermission(err) {
		t.Skipf("filesystem did not enforce permission denial: %v", err)
	}
}

func TestNextRejectsAppendixDirectoryErrors(t *testing.T) {
	for _, layer := range []string{"candidate", "stable"} {
		for _, failure := range []string{"permission", "not a directory"} {
			t.Run(layer+"/"+failure, func(t *testing.T) {
				root := createCLITestRepo(t)
				grWriteFile(t, root, "docs/specs/units/candidate/unit_demo.md", nextDiscoverySpec("new"))
				ref := "docs/specs/units/" + layer + "/appendix"
				if failure == "permission" {
					grWriteFile(t, root, ref+"/unit_demo_extra.md", "---\nunit: demo\n---\nExtra design.\n")
					denyNextDiscoveryAccess(t, filepath.Join(root, filepath.FromSlash(ref)), true)
				} else {
					grWriteFile(t, root, ref, "not a directory\n")
				}
				var stdout, stderr bytes.Buffer
				err := runNext([]string{"--unit", "demo", "--repo-root", root}, &stdout, &stderr)
				if err == nil || !strings.Contains(err.Error(), ref) || stdout.Len() != 0 {
					t.Fatalf("expected appendix discovery failure naming %s without metadata, got %v; stdout=%s", ref, err, stdout.String())
				}
			})
		}
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
	if !strings.Contains(output, "(no design recorded)") {
		t.Errorf("expected '(no design recorded)' in output, got:\n%s", output)
	}
}

func TestNextUsageWithoutUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runNext([]string{"--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected usage error when --unit is missing, got nil")
	}

	output := stderr.String()

	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output on stderr, got:\n%s", output)
	}
	if stdout.String() != "" {
		t.Errorf("expected no stdout output on usage error, got:\n%s", stdout.String())
	}
}

func TestNextRelatedUnitsBlockList(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)
	os.WriteFile(filepath.Join(candidateDir, "unit_demo.md"), []byte(`---
id: demo
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

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

func writeSurfacesUnitSpec(t *testing.T, repoRoot, unitName, impl string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + unitName + "\nunit_refs: none\nrule_refs: none\n---\n\n" +
		"acceptance_item_set:\n" +
		"  - id: item_1\n" +
		"    description: test\n" +
		"    verification_type: testable\n" +
		"    verification_surface: src/\n" +
		"    implementation_surface: " + impl + "\n" +
		"    verification_method: check\n" +
		"    pass_condition: ok\n" +
		"    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unitName+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSurfacesCommandReportsOverlap(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	pkgDir := filepath.Join(repoRoot, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSurfacesUnitSpec(t, repoRoot, "alpha", "pkg/a.go")
	writeSurfacesUnitSpec(t, repoRoot, "beta", "pkg/a.go")

	var stdout, stderr bytes.Buffer
	if err := runSurfaces([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("runSurfaces: %v\n%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "File judgments:") || !strings.Contains(out, "pkg/a.go") {
		t.Fatalf("expected overlap in report:\n%s", out)
	}

	stdout.Reset()
	if err := runSurfaces([]string{"--repo-root", repoRoot, "--json"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var report specvalidation.SurfaceAuditReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if len(report.SharedFiles) != 1 || report.SharedFiles[0].Path != "pkg/a.go" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestSurfacesToleratesUnusableVerifyCache(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	pkgDir := filepath.Join(repoRoot, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSurfacesUnitSpec(t, repoRoot, "alpha", "pkg/a.go")

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/alpha")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// An old-format verify cache (no chunk evidence): the unusable baseline
	// must degrade to "no judgments", not fail the whole command.
	legacy := "---\ncommand: verify\nunit: alpha\nmode: full\nresult: pass\nblocking: false\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/units/candidate/unit_alpha.md\n    hash: sha256:00\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runSurfaces([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("surfaces must not be blocked by an unusable verify cache: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "File judgments:") {
		t.Fatalf("expected the surfaces report, got:\n%s", stdout.String())
	}
}

func TestValidateCandidateFailsOnSharedSurfaceFile(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	pkgDir := filepath.Join(repoRoot, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSurfacesUnitSpec(t, repoRoot, "alpha", "pkg/a.go")
	writeSurfacesUnitSpec(t, repoRoot, "beta", "pkg/a.go")

	var stdout, stderr bytes.Buffer
	err := runValidate([]string{"candidate", "--unit", "alpha", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected validate to fail on a surface overlap:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Surface associations: PASS") {
		t.Fatalf("expected the surface ownership check to fail:\n%s", stdout.String())
	}
}

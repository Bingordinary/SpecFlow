package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

func TestLayoutAccessFailureStopsCommandBoundaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not deny directory traversal on Windows")
	}
	originalFingerprint := toolingfreshness.BuildFingerprint
	t.Cleanup(func() { toolingfreshness.BuildFingerprint = originalFingerprint })
	for _, toolingRoot := range []string{"tooling", "specflow/tooling"} {
		for _, otherPresent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/other=%t", toolingRoot, otherPresent), func(t *testing.T) {
				repoRoot := t.TempDir()
				marker := filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "go.mod")
				mustWriteCLITestFile(t, marker, "marker\n")
				if otherPresent {
					otherRoot := "tooling"
					if toolingRoot == otherRoot {
						otherRoot = "specflow/tooling"
					}
					mustWriteCLITestFile(t, filepath.Join(repoRoot, filepath.FromSlash(otherRoot), "go.mod"), "marker\n")
				}
				candidate := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_demo.md")
				content := "---\nunit: demo\nunit_refs: []\nrule_refs: []\nevidence_appendix_ref: none\n---\n# Demo\n"
				mustWriteCLITestFile(t, candidate, content)
				frameworkPath := filepath.Join(repoRoot, "framework/concepts.md")
				mustWriteCLITestFile(t, frameworkPath, "# Framework\n")
				mustWriteCLITestFile(t, filepath.Join(repoRoot, "specflow/framework/concepts.md"), "# Framework\n")
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(repoRoot); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chdir(cwd) })
				markerDir := filepath.Dir(marker)
				t.Cleanup(func() { os.Chmod(markerDir, 0755) })
				if err := os.Chmod(markerDir, 0000); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(marker); !os.IsPermission(err) {
					t.Skipf("filesystem did not enforce marker permission denial: %v", err)
				}

				// Exercise the ordinary process entry from the project cwd: neither
				// a missing nor an obsolete fingerprint may bypass removal checks.
				for _, fingerprint := range []string{"", "obsolete-build"} {
					toolingfreshness.BuildFingerprint = fingerprint
					var stdout, stderr bytes.Buffer
					err := run([]string{"remove", "--unit", "demo", "--layer", "candidate", "--repo-root", repoRoot}, &stdout, &stderr)
					assertLayoutAccessRejected(t, marker, err, &stdout)
					actual, err := os.ReadFile(candidate)
					if err != nil || string(actual) != content {
						t.Fatalf("failed command changed candidate: %q, %v", actual, err)
					}
					if _, err := os.Stat(filepath.Join(repoRoot, ".specflow")); !os.IsNotExist(err) {
						t.Fatalf("failed command wrote process state: %v", err)
					}
				}

				// Freshness metadata must fail at the process boundary. Test write
				// validation directly as well, independently of process preflight.
				var stdout, stderr bytes.Buffer
				err = run([]string{"fresh", "--unit", "demo", "--repo-root", repoRoot}, &stdout, &stderr)
				assertLayoutAccessRejected(t, marker, err, &stdout)
				for _, path := range []string{frameworkPath, filepath.Join(repoRoot, "specflow/framework/concepts.md")} {
					stdout.Reset()
					stderr.Reset()
					err = runValidate([]string{"write", "--path", path, "--repo-root", repoRoot}, &stdout, &stderr)
					assertLayoutAccessRejected(t, marker, err, &stdout)
				}
			})
		}
	}
}

func assertLayoutAccessRejected(t *testing.T, marker string, err error, stdout *bytes.Buffer) {
	t.Helper()
	if !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), marker) {
		t.Fatalf("expected permission error naming %s, got %v; stdout=%s", marker, err, stdout.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed layout emitted successful metadata or write permission: %s", stdout.String())
	}
}

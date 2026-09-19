package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validateTestRepo(t *testing.T) string {
	t.Helper()
	repoRoot := createCLITestRepo(t)
	if err := os.MkdirAll(filepath.Join(repoRoot, "tooling"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "tooling", "go.mod"), []byte("module example/source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repoRoot
}

func mustValidateWrite(t *testing.T, repoRoot, path string) validateResult {
	t.Helper()
	result, err := validateWrite(repoRoot, path)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})
}

func TestValidateWriteRelativePaths(t *testing.T) {
	repoRoot := validateTestRepo(t)
	chdir(t, repoRoot)

	cases := []struct {
		name    string
		path    string
		allowed bool
	}{
		{"framework denied", "framework/concepts.md", false},
		{"stable spec denied", "docs/specs/units/stable/unit_auth.md", false},
		{"stable rule denied", "docs/specs/rules/stable/g_rule_x.md", false},
		{"candidate spec allowed", "docs/specs/units/candidate/unit_auth.md", true},
		{"candidate rule allowed", "docs/specs/rules/candidate/b_rule_x.md", true},
		{"source code allowed", "src/auth/login.go", true},
		{"clean-folded dotdot denied", "docs/../framework/concepts.md", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := mustValidateWrite(t, repoRoot, tc.path)
			if result.Allowed != tc.allowed {
				t.Fatalf("validateWrite(%q) = allowed %t, want %t (reason: %s)", tc.path, result.Allowed, tc.allowed, result.Reason)
			}
		})
	}
}

func TestValidateWriteAbsolutePathsWithinRepo(t *testing.T) {
	repoRoot := validateTestRepo(t)

	cases := []struct {
		name    string
		rel     string
		allowed bool
	}{
		{"framework denied", "framework/concepts.md", false},
		{"stable spec denied", "docs/specs/units/stable/unit_auth.md", false},
		{"stable rule denied", "docs/specs/rules/stable/g_rule_x.md", false},
		{"candidate spec allowed", "docs/specs/units/candidate/unit_auth.md", true},
		{"candidate rule allowed", "docs/specs/rules/candidate/b_rule_x.md", true},
		{"source code allowed", "src/auth/login.go", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			abs := filepath.Join(repoRoot, filepath.FromSlash(tc.rel))
			result := mustValidateWrite(t, repoRoot, abs)
			if result.Allowed != tc.allowed {
				t.Fatalf("validateWrite(abs %q) = allowed %t, want %t (reason: %s)", abs, result.Allowed, tc.allowed, result.Reason)
			}
		})
	}
}

func TestValidateWriteAbsolutePathOutsideRepo(t *testing.T) {
	repoRoot := validateTestRepo(t)
	tmp := t.TempDir()

	paths := []string{
		filepath.Join(tmp, "docs/specs/units/stable/unit_auth.md"),
		filepath.Join(tmp, "framework/concepts.md"),
		"/etc/passwd",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			result := mustValidateWrite(t, repoRoot, p)
			if !result.Allowed {
				t.Fatalf("validateWrite(%q) = allowed %t, want true (outside repo root)", p, result.Allowed)
			}
			if !strings.Contains(result.Reason, "not governed") {
				t.Fatalf("expected 'not governed' reason, got %q", result.Reason)
			}
		})
	}
}

func TestValidateWriteEscapingRelativePathFromRepoRoot(t *testing.T) {
	repoRoot := validateTestRepo(t)
	chdir(t, repoRoot)

	cases := []struct {
		path    string
		allowed bool
	}{
		{"../outside/stable/unit_auth.md", true},
		{"../../etc/passwd", true},
		{"../framework/concepts.md", true},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			result := mustValidateWrite(t, repoRoot, tc.path)
			if result.Allowed != tc.allowed {
				t.Fatalf("validateWrite(%q) = allowed %t, want %t (reason: %s)", tc.path, result.Allowed, tc.allowed, result.Reason)
			}
			if tc.allowed && !strings.Contains(result.Reason, "not governed") {
				t.Fatalf("expected 'not governed' reason, got %q", result.Reason)
			}
		})
	}
}

func TestValidateWriteEscapingRelativePathFromSubdir(t *testing.T) {
	repoRoot := validateTestRepo(t)
	subdir := filepath.Join(repoRoot, "tooling")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	chdir(t, subdir)

	cases := []struct {
		path    string
		allowed bool
	}{
		// From tooling/, ../framework/... resolves back into the repo's
		// governed framework zone and must be denied.
		{"../framework/concepts.md", false},
		{"../docs/specs/units/stable/unit_auth.md", false},
		{"../docs/specs/units/candidate/unit_auth.md", true},
		{"../src/auth/login.go", true},
		{"../../outside/anything.txt", true},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			result := mustValidateWrite(t, repoRoot, tc.path)
			if result.Allowed != tc.allowed {
				t.Fatalf("validateWrite(%q) = allowed %t, want %t (reason: %s)", tc.path, result.Allowed, tc.allowed, result.Reason)
			}
		})
	}
}

func TestValidateWriteCLIUsesInstalledProjectFrameworkRoot(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	if err := os.MkdirAll(filepath.Join(repoRoot, "specflow", "tooling"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "specflow", "tooling", "go.mod"), []byte("module example/installed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repoRoot, "specflow", "framework", "concepts.md")
	var stdout, stderr bytes.Buffer
	err := runValidate([]string{"write", "--repo-root", repoRoot, "--path", path}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "write denied") {
		t.Fatalf("installed framework path must be denied, err=%v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "allowed: false") || !strings.Contains(stdout.String(), "specflow/framework") {
		t.Fatalf("unexpected installed-layout result:\n%s", stdout.String())
	}
}

func TestValidateWriteCLIFailsClosedWithoutLayout(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	var stdout, stderr bytes.Buffer
	err := runValidate([]string{"write", "--repo-root", repoRoot, "--path", "src/app.go"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "layout not found") {
		t.Fatalf("missing layout must fail closed, got %v", err)
	}
}

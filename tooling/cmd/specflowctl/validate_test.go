package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWriteRelativePaths(t *testing.T) {
	repoRoot := createCLITestRepo(t)

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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validateWrite(repoRoot, tc.path)
			if result.Allowed != tc.allowed {
				t.Fatalf("validateWrite(%q) = allowed %t, want %t (reason: %s)", tc.path, result.Allowed, tc.allowed, result.Reason)
			}
		})
	}
}

func TestValidateWriteAbsolutePathsWithinRepo(t *testing.T) {
	repoRoot := createCLITestRepo(t)

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
			result := validateWrite(repoRoot, abs)
			if result.Allowed != tc.allowed {
				t.Fatalf("validateWrite(abs %q) = allowed %t, want %t (reason: %s)", abs, result.Allowed, tc.allowed, result.Reason)
			}
		})
	}
}

func TestValidateWriteAbsolutePathOutsideRepo(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	tmp := t.TempDir()

	paths := []string{
		filepath.Join(tmp, "docs/specs/units/stable/unit_auth.md"),
		filepath.Join(tmp, "framework/concepts.md"),
		"/etc/passwd",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			result := validateWrite(repoRoot, p)
			if !result.Allowed {
				t.Fatalf("validateWrite(%q) = allowed %t, want true (outside repo root)", p, result.Allowed)
			}
			if !strings.Contains(result.Reason, "not governed") {
				t.Fatalf("expected 'not governed' reason, got %q", result.Reason)
			}
		})
	}
}

func TestValidateWriteEscapingRelativePath(t *testing.T) {
	repoRoot := createCLITestRepo(t)

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
			result := validateWrite(repoRoot, tc.path)
			if result.Allowed != tc.allowed {
				t.Fatalf("validateWrite(%q) = allowed %t, want %t", tc.path, result.Allowed, tc.allowed)
			}
		})
	}
}

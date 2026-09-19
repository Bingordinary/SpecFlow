package writezone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "writezone-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tooling"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tooling", "go.mod"), []byte("module example/source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func mustClassifier(t *testing.T, repoRoot string) *Classifier {
	t.Helper()
	classifier, err := New(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return classifier
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

func TestClassifyRelativePaths(t *testing.T) {
	repoRoot := testRepo(t)
	chdir(t, repoRoot)
	classifier := mustClassifier(t, repoRoot)

	cases := []struct {
		name    string
		path    string
		allowed bool
	}{
		{"framework denied", "framework/concepts.md", false},
		{"framework zone root denied", "framework", false},
		{"stable spec denied", "docs/specs/units/stable/unit_auth.md", false},
		{"stable unit zone root denied", "docs/specs/units/stable", false},
		{"stable rule denied", "docs/specs/rules/stable/g_rule_x.md", false},
		{"stable rule zone root denied", "docs/specs/rules/stable", false},
		{"candidate spec allowed", "docs/specs/units/candidate/unit_auth.md", true},
		{"candidate rule allowed", "docs/specs/rules/candidate/b_rule_x.md", true},
		{"source code allowed", "src/auth/login.go", true},
		{"clean-folded dotdot denied", "docs/../framework/concepts.md", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifier.Classify(tc.path)
			if result.Allowed != tc.allowed {
				t.Fatalf("Classify(%q) = allowed %t, want %t (reason: %s)", tc.path, result.Allowed, tc.allowed, result.Reason)
			}
			if result.Path == "" {
				t.Fatalf("Classify(%q) returned an empty Path", tc.path)
			}
		})
	}
}

func TestClassifyAbsolutePathsWithinRepo(t *testing.T) {
	repoRoot := testRepo(t)
	classifier := mustClassifier(t, repoRoot)

	cases := []struct {
		rel     string
		allowed bool
	}{
		{"framework/concepts.md", false},
		{"framework", false},
		{"docs/specs/units/stable/unit_auth.md", false},
		{"docs/specs/units/stable", false},
		{"docs/specs/units/candidate/unit_auth.md", true},
		{"src/auth/login.go", true},
	}

	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			abs := filepath.Join(repoRoot, filepath.FromSlash(tc.rel))
			result := classifier.Classify(abs)
			if result.Allowed != tc.allowed {
				t.Fatalf("Classify(abs %q) = allowed %t, want %t (reason: %s)", abs, result.Allowed, tc.allowed, result.Reason)
			}
		})
	}
}

func TestClassifyAbsolutePathOutsideRepo(t *testing.T) {
	repoRoot := testRepo(t)
	tmp := t.TempDir()
	classifier := mustClassifier(t, repoRoot)

	paths := []string{
		filepath.Join(tmp, "docs/specs/units/stable/unit_auth.md"),
		filepath.Join(tmp, "framework/concepts.md"),
		"/etc/passwd",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			result := classifier.Classify(p)
			if !result.Allowed {
				t.Fatalf("Classify(%q) = allowed %t, want true (outside repo root)", p, result.Allowed)
			}
			if !strings.Contains(result.Reason, "not governed") {
				t.Fatalf("expected 'not governed' reason, got %q", result.Reason)
			}
		})
	}
}

func TestClassifyEscapingRelativePathFromSubdir(t *testing.T) {
	repoRoot := testRepo(t)
	subdir := filepath.Join(repoRoot, "tooling")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	chdir(t, subdir)
	classifier := mustClassifier(t, repoRoot)

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
			result := classifier.Classify(tc.path)
			if result.Allowed != tc.allowed {
				t.Fatalf("Classify(%q) = allowed %t, want %t (reason: %s)", tc.path, result.Allowed, tc.allowed, result.Reason)
			}
		})
	}
}

func TestClassifyDeniesWhenEitherSymlinkIdentityIsProtected(t *testing.T) {
	repoRoot := testRepo(t)
	classifier := mustClassifier(t, repoRoot)

	for _, dir := range []string{
		"framework",
		"src",
		"docs/specs/units/stable",
		"docs/specs/units/candidate",
	} {
		if err := os.MkdirAll(filepath.Join(repoRoot, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"src/plain.go":                                      "package src\n",
		"framework/real.md":                                 "# Framework\n",
		"docs/specs/units/stable/unit_real.md":              "# Stable\n",
		"docs/specs/units/candidate/unit_real_candidate.md": "# Candidate\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(repoRoot, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name   string
		link   string
		target string
	}{
		{"stable lexical path to candidate target", "docs/specs/units/stable/unit_link.md", "docs/specs/units/candidate/unit_real_candidate.md"},
		{"candidate lexical path to stable target", "docs/specs/units/candidate/unit_link.md", "docs/specs/units/stable/unit_real.md"},
		{"framework lexical path to source target", "framework/source-link.md", "src/plain.go"},
		{"source lexical path to framework target", "src/framework-link.md", "framework/real.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link := filepath.Join(repoRoot, filepath.FromSlash(tc.link))
			target := filepath.Join(repoRoot, filepath.FromSlash(tc.target))
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("cannot create symlink: %v", err)
			}
			if result := classifier.Classify(link); result.Allowed {
				t.Fatalf("expected protected lexical/resolved identity to be denied: %+v", result)
			}
		})
	}
}

func TestClassifyDeniesUnresolvableSymlinkIdentity(t *testing.T) {
	repoRoot := testRepo(t)
	classifier := mustClassifier(t, repoRoot)

	for _, dir := range []string{"src", "docs/specs/units/candidate", "docs/specs/rules/stable"} {
		if err := os.MkdirAll(filepath.Join(repoRoot, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// A dangling link in an allowed lexical zone whose target will appear in a
	// denied zone: classifying by the lexical identity alone would allow a
	// write through the link into the denied location.
	inRepo := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dangle.md")
	if err := os.Symlink(filepath.Join(repoRoot, "docs/specs/rules/stable/g_rule_new.md"), inRepo); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if result := classifier.Classify(inRepo); result.Allowed {
		t.Fatalf("dangling symlink with an in-repo target must be denied: %+v", result)
	}

	// A dangling link pointing outside the repository.
	outside := filepath.Join(repoRoot, "src/outside-link.go")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing.go"), outside); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if result := classifier.Classify(outside); result.Allowed {
		t.Fatalf("dangling symlink with an outside target must be denied: %+v", result)
	}
}

func TestClassifyUsesInstalledProjectFrameworkRoot(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "specflow", "tooling"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "specflow", "tooling", "go.mod"), []byte("module example/installed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	classifier := mustClassifier(t, repoRoot)
	installedFramework := filepath.Join(repoRoot, "specflow", "framework", "concepts.md")
	if result := classifier.Classify(installedFramework); result.Allowed {
		t.Fatalf("installed framework path must be denied: %+v", result)
	}
	projectFramework := filepath.Join(repoRoot, "framework", "project.go")
	if result := classifier.Classify(projectFramework); !result.Allowed {
		t.Fatalf("non-SpecFlow project path must remain allowed in installed layout: %+v", result)
	}
}

func TestNewFailsClosedForMissingOrAmbiguousLayout(t *testing.T) {
	missing := t.TempDir()
	if _, err := New(missing); err == nil || !strings.Contains(err.Error(), "layout not found") {
		t.Fatalf("missing layout must fail closed, got %v", err)
	}

	ambiguous := testRepo(t)
	if err := os.MkdirAll(filepath.Join(ambiguous, "specflow", "tooling"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ambiguous, "specflow", "tooling", "go.mod"), []byte("module example/installed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(ambiguous); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous layout must fail closed, got %v", err)
	}
}

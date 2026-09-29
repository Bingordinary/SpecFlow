package repofiles

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	gitRun(t, repoRoot, "init", "-q")
	return repoRoot
}

func writeFile(t *testing.T, repoRoot, rel, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func expandedPaths(files []File) []string {
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths
}

func TestExpandDir_RepositoryContentOnly(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, ".gitignore", "web/node_modules/\nweb/dist/\n")
	writeFile(t, repoRoot, "web/app.js", "export {};\n")
	writeFile(t, repoRoot, "web/lib/util.js", "export {};\n")
	writeFile(t, repoRoot, "web/node_modules/dep/index.js", "module.exports = {};\n")
	writeFile(t, repoRoot, "web/dist/bundle.js", "bundle\n")
	writeFile(t, repoRoot, "web/dist/tracked.js", "tracked build output\n")
	gitRun(t, repoRoot, "add", "-f", "web/dist/tracked.js")

	files, err := ExpandDir(repoRoot, "web")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(expandedPaths(files), ",")
	want := "web/app.js,web/dist/tracked.js,web/lib/util.js"
	if got != want {
		t.Fatalf("ignored untracked files must not enter the surface, tracked ones must: got %q, want %q", got, want)
	}
	for _, f := range files {
		if f.Hash == "" {
			t.Fatalf("every expanded file must carry a content hash, got %+v", f)
		}
	}
}

func TestExpandDir_OnlyIgnoredFilesIsEmpty(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, ".gitignore", "node_modules/\n")
	writeFile(t, repoRoot, "node_modules/dep/index.js", "module.exports = {};\n")

	files, err := ExpandDir(repoRoot, "node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("a directory of only ignored files must expand to zero files, got %v", expandedPaths(files))
	}
}

func TestExpandDir_RequiresWorkTreeTop(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "web/app.js", "export {};\n")
	writeFile(t, repoRoot, "sub/app.js", "export {};\n")

	if _, err := ExpandDir(repoRoot, "web"); err != nil {
		t.Fatalf("a worktree top must expand: %v", err)
	}
	if _, err := ExpandDir(filepath.Join(repoRoot, "sub"), "sub"); err == nil || !strings.Contains(err.Error(), "not the git worktree top level") {
		t.Fatalf("a non-top repository root must fail closed, got %v", err)
	}
}

// caseVariant returns a spelling of p that differs only in the letter case of
// one path component, and whether that variant names the same directory as p.
// On a case-insensitive filesystem (macOS, Windows) the variant is the same
// directory; on a case-sensitive filesystem it either does not exist or is a
// genuinely different directory.
func caseVariant(t *testing.T, p string) (string, bool) {
	t.Helper()
	parts := strings.Split(filepath.Clean(p), string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "" {
			continue
		}
		component := []byte(parts[i])
		switch {
		case component[0] >= 'a' && component[0] <= 'z':
			component[0] = component[0] - 'a' + 'A'
		case component[0] >= 'A' && component[0] <= 'Z':
			component[0] = component[0] - 'A' + 'a'
		default:
			continue
		}
		parts[i] = string(component)
		variant := strings.Join(parts, string(filepath.Separator))
		info, err := os.Stat(variant)
		if err != nil {
			return variant, false
		}
		orig, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return variant, os.SameFile(info, orig)
	}
	t.Fatalf("cannot derive a case variant for %q", p)
	return "", false
}

func TestRequireWorkTreeTop_AcceptsCaseVariantRoot(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "web/app.js", "export {};\n")
	writeFile(t, repoRoot, "sub/app.js", "export {};\n")

	variant, sameDir := caseVariant(t, repoRoot)
	if !sameDir {
		t.Skip("filesystem is case-sensitive; no case-variant spelling of the root exists")
	}
	if err := RequireWorkTreeTop(variant); err != nil {
		t.Fatalf("a case-variant spelling of the worktree top is the same directory and must be accepted, got %v", err)
	}
	if err := RequireWorkTreeTop(filepath.Join(repoRoot, "sub")); err == nil || !strings.Contains(err.Error(), "not the git worktree top level") {
		t.Fatalf("a non-top repository root must still fail closed, got %v", err)
	}
}

func TestRequireWorkTreeTop_WrongCasePWDEnv(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "web/app.js", "export {};\n")

	variant, sameDir := caseVariant(t, repoRoot)
	if !sameDir {
		t.Skip("filesystem is case-sensitive; a wrong-case PWD cannot name the same directory")
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	oldPWD, hadPWD := os.LookupEnv("PWD")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Fatal(err)
		}
		if hadPWD {
			os.Setenv("PWD", oldPWD)
		} else {
			os.Unsetenv("PWD")
		}
	})
	os.Setenv("PWD", variant)

	// Go's os.Getwd returns the PWD environment variable verbatim when it
	// stats equal to "." — exactly what a host shell with a wrong-case PWD
	// produces (issue #46).
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if cwd != variant {
		t.Skipf("os.Getwd did not adopt the wrong-case PWD spelling (got %q)", cwd)
	}
	if err := RequireWorkTreeTop(cwd); err != nil {
		t.Fatalf("a wrong-case PWD adopted by os.Getwd must be accepted as the worktree top, got %v", err)
	}
}

func TestExpandDir_NonRepositoryFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "web/app.js", "export {};\n")

	if _, err := ExpandDir(dir, "web"); err == nil || !strings.Contains(err.Error(), "resolve git worktree") {
		t.Fatalf("a non-repository root must fail closed, got %v", err)
	}
}

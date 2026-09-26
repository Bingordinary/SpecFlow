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

func TestExpandDir_NonRepositoryFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "web/app.js", "export {};\n")

	if _, err := ExpandDir(dir, "web"); err == nil || !strings.Contains(err.Error(), "resolve git worktree") {
		t.Fatalf("a non-repository root must fail closed, got %v", err)
	}
}

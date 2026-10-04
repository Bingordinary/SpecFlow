package specflowlayout

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveRejectsMarkerAccessErrors(t *testing.T) {
	for _, toolingRoot := range []string{"tooling", "specflow/tooling"} {
		for _, otherPresent := range []bool{false, true} {
			for _, failure := range []string{"permission", "symlink loop"} {
				t.Run(toolingRoot+"/"+fmtBool(otherPresent)+"/"+failure, func(t *testing.T) {
					repoRoot := t.TempDir()
					marker := filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), "go.mod")
					writeMarker(t, marker)
					if otherPresent {
						otherRoot := "tooling"
						if toolingRoot == otherRoot {
							otherRoot = "specflow/tooling"
						}
						writeMarker(t, filepath.Join(repoRoot, filepath.FromSlash(otherRoot), "go.mod"))
					}
					if failure == "permission" {
						if runtime.GOOS == "windows" {
							t.Skip("chmod does not deny directory traversal on Windows")
						}
						dir := filepath.Dir(marker)
						t.Cleanup(func() { os.Chmod(dir, 0755) })
						if err := os.Chmod(dir, 0000); err != nil {
							t.Fatal(err)
						}
					} else {
						if err := os.Remove(marker); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink("go.mod", marker); err != nil {
							t.Skipf("cannot create symlink: %v", err)
						}
					}
					_, probeErr := os.Stat(marker)
					if probeErr == nil || os.IsNotExist(probeErr) {
						t.Skipf("filesystem did not enforce marker access failure: %v", probeErr)
					}
					if failure == "permission" && !os.IsPermission(probeErr) {
						t.Fatalf("expected permission error, got %v", probeErr)
					}
					layout, err := Resolve(repoRoot)
					var pathErr *os.PathError
					if err == nil || errors.Is(err, ErrNotFound) || !errors.As(err, &pathErr) || pathErr.Path != marker {
						t.Fatalf("expected wrapped marker error, got layout=%+v err=%v", layout, err)
					}
				})
			}
		}
	}
}

func fmtBool(value bool) string {
	if value {
		return "both layouts"
	}
	return "single layout"
}

func TestResolveMarkerAlternativesAndAbsence(t *testing.T) {
	for _, toolingRoot := range []string{"tooling", "specflow/tooling"} {
		for _, marker := range []string{"go.mod", "manifest.tsv"} {
			t.Run(toolingRoot+"/"+marker, func(t *testing.T) {
				repoRoot := t.TempDir()
				writeMarker(t, filepath.Join(repoRoot, filepath.FromSlash(toolingRoot), marker))
				layout, err := Resolve(repoRoot)
				if err != nil || layout.ToolingRoot != toolingRoot {
					t.Fatalf("expected %s, got %+v, %v", toolingRoot, layout, err)
				}
			})
		}
	}
	repoRoot := t.TempDir()
	for _, marker := range []string{"go.mod", "manifest.tsv"} {
		if err := os.MkdirAll(filepath.Join(repoRoot, "tooling", marker), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Resolve(repoRoot); !errors.Is(err, ErrNotFound) {
		t.Fatalf("directories must not count as marker files: %v", err)
	}
}

func TestResolveSourceRepo(t *testing.T) {
	repoRoot := t.TempDir()
	writeMarker(t, filepath.Join(repoRoot, "tooling", "manifest.tsv"))

	layout, err := Resolve(repoRoot)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if layout.Kind != SourceRepo || layout.ToolingRoot != "tooling" {
		t.Fatalf("unexpected layout: %+v", layout)
	}
}

func TestResolveInstalledProject(t *testing.T) {
	repoRoot := t.TempDir()
	writeMarker(t, filepath.Join(repoRoot, "specflow", "tooling", "manifest.tsv"))

	layout, err := Resolve(repoRoot)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if layout.Kind != InstalledProject || layout.ToolingRoot != "specflow/tooling" {
		t.Fatalf("unexpected layout: %+v", layout)
	}
}

func TestResolveRejectsAmbiguousLayouts(t *testing.T) {
	repoRoot := t.TempDir()
	writeMarker(t, filepath.Join(repoRoot, "tooling", "manifest.tsv"))
	writeMarker(t, filepath.Join(repoRoot, "specflow", "tooling", "manifest.tsv"))

	_, err := Resolve(repoRoot)
	if err == nil || !strings.Contains(err.Error(), "ambiguous specFlow layout") {
		t.Fatalf("expected ambiguous layout error, got %v", err)
	}
}

func writeMarker(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("marker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

package repopath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalAcceptsMissingAndInternalSymlinkPaths(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repoRoot, "real"), filepath.Join(repoRoot, "alias")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	for input, want := range map[string]string{
		"missing/child.txt": "missing/child.txt",
		"alias/new.txt":     "alias/new.txt",
	} {
		got, err := Canonical(repoRoot, input)
		if err != nil {
			t.Fatalf("Canonical(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("Canonical(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalRejectsLexicalAndSymlinkEscapes(t *testing.T) {
	repoRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "existing.txt"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repoRoot, "external")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	for _, input := range []string{
		"../outside.txt",
		outside,
		"external/existing.txt",
		"external/missing/child.txt",
	} {
		if _, err := Canonical(repoRoot, input); err == nil || !strings.Contains(err.Error(), "outside the repository root") {
			t.Fatalf("Canonical(%q) error = %v, want repository escape rejection", input, err)
		}
	}
}

func TestCanonicalRejectsUnresolvableSymlinks(t *testing.T) {
	repoRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "missing-target"), filepath.Join(repoRoot, "dangling")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	for _, input := range []string{"dangling", "dangling/child.txt"} {
		if _, err := Canonical(repoRoot, input); err == nil || !strings.Contains(err.Error(), "unresolvable symlink") {
			t.Fatalf("Canonical(%q) error = %v, want unresolvable-symlink rejection", input, err)
		}
	}
	// Plain missing paths (no symlink involved) are still accepted: the
	// nearest existing ancestor resolves them.
	if _, err := Canonical(repoRoot, "missing/child.txt"); err != nil {
		t.Fatalf("plain missing paths must stay accepted: %v", err)
	}
}

func TestCanonicalRejectsRepositoryRoot(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := Canonical(repoRoot, "."); err == nil {
		t.Fatal("expected repository root to be rejected")
	}
}

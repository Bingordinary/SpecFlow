package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInputsManifestParsesEntriesVerbatim(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	path := filepath.Join(t.TempDir(), "inputs.txt")
	content := "\xEF\xBB\xBFsrc/a.go\r\n\r\n  spaced.go  \ntests\r\nunit:auth\nsrc/a.go\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadInputsManifest(repoRoot, path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/a.go", "  spaced.go  ", "tests", "unit:auth", "src/a.go"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("manifest entries must be verbatim (BOM and CRLF tolerated, blank lines skipped), got %q want %q", got, want)
	}
}

func TestLoadInputsManifestEmptyFile(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	path := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadInputsManifest(repoRoot, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("an empty manifest lists no extra inputs, got %q", got)
	}
}

func TestLoadInputsManifestMissingFile(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	if _, err := loadInputsManifest(repoRoot, filepath.Join(t.TempDir(), "absent.txt")); err == nil || !strings.Contains(err.Error(), "--inputs-file") {
		t.Fatalf("expected a read error naming the manifest, got %v", err)
	}
}

func TestLoadInputsManifestRejectsRepositoryContent(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	path := grWriteFile(t, repoRoot, "plan_inputs.txt", "src/a.go\n")
	if _, err := loadInputsManifest(repoRoot, path); err == nil || !strings.Contains(err.Error(), "repository-content file") {
		t.Fatalf("expected a repository-content manifest to be rejected, got %v", err)
	}
}

func TestLoadInputsManifestAllowsIgnoredLocalState(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteFile(t, repoRoot, ".gitignore", "/meta/\n")
	path := grWriteFile(t, repoRoot, "meta/plan_inputs/verify-auth-candidate.txt", "src/a.go\n")
	got, err := loadInputsManifest(repoRoot, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "src/a.go" {
		t.Fatalf("expected the ignored local-state manifest to load, got %q", got)
	}
}

func TestGatePlanRejectsRepeatedInputsFile(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	manifest := grInputsManifest(t, "src/a.go")
	_, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate",
		"--inputs-file", manifest, "--inputs-file", manifest)
	if err == nil || !strings.Contains(err.Error(), "given more than once") {
		t.Fatalf("expected a repeated --inputs-file to be rejected, got %v", err)
	}
}

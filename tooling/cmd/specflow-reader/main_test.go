package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseOptionsDefaultsRepoRootToThreeLevelsUp(t *testing.T) {
	repoRoot := t.TempDir()
	binDir := filepath.Join(repoRoot, "specflow", "tooling", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(binDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	options, err := parseOptions(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	want := repoRoot
	if resolved, err := filepath.EvalSymlinks(repoRoot); err == nil {
		want = resolved
	}
	if options.RepoRoot != want {
		t.Fatalf("repo root = %q, want %q", options.RepoRoot, want)
	}
}

func TestArgsForFreshnessAddsDefaultRepoRootForServerRun(t *testing.T) {
	got := argsForFreshness(nil)
	want := []string{"--repo-root", defaultRepoRoot}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("freshness args = %#v, want %#v", got, want)
	}
}

func TestArgsForFreshnessAddsDefaultRepoRootWhenOmitted(t *testing.T) {
	got := argsForFreshness([]string{"--addr", "127.0.0.1:17863"})
	want := []string{"--addr", "127.0.0.1:17863", "--repo-root", defaultRepoRoot}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("freshness args = %#v, want %#v", got, want)
	}
}

func TestArgsForFreshnessPreservesExplicitRepoRoot(t *testing.T) {
	args := []string{"--repo-root", "."}
	got := argsForFreshness(args)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("freshness args = %#v, want %#v", got, args)
	}
}

func TestParseOptionsRejectsUnexpectedArguments(t *testing.T) {
	_, err := parseOptions([]string{"serve"}, io.Discard)
	if err == nil {
		t.Fatal("expected unexpected argument error")
	}
}

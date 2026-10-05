package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanDryRunAndApply(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	spec := grWriteSpec(t, repoRoot, "auth")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	// Orphan the run by deleting the spec outside the CLI, then add a scratch
	// input manifest.
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(repoRoot, "meta", "plan_inputs")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(manifestDir, "validate-auth-candidate.txt")
	if err := os.WriteFile(manifest, []byte("src/auth.go\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runClean([]string{"--repo-root", repoRoot, "--dry-run"}, &stdout, &stderr); err != nil {
		t.Fatalf("clean --dry-run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Delete run: "+runID) || !strings.Contains(out, "Delete plan input: validate-auth-candidate.txt") {
		t.Fatalf("dry run did not report the orphans:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "meta", "gate_runs", runID)); err != nil {
		t.Fatal("dry run deleted the orphaned run")
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal("dry run deleted the plan input")
	}

	stdout.Reset()
	if err := runClean([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "meta", "gate_runs", runID)); !os.IsNotExist(err) {
		t.Fatal("clean did not delete the orphaned run")
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatal("clean did not delete the plan input")
	}
}

func TestRemoveSweepsOrphanedRun(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	var stdout, stderr bytes.Buffer
	if err := runRemove([]string{"--repo-root", repoRoot, "--unit", "auth"}, &stdout, &stderr); err != nil {
		t.Fatalf("remove: %v\n%s", err, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "meta", "gate_runs", runID)); !os.IsNotExist(err) {
		t.Fatal("run of the removed unit survived the removal sweep")
	}
}

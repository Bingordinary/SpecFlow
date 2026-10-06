package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
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

// Issue #61's recovery path, end to end through clean: a verify run reuses an
// accepted public record from an earlier run; the record is then removed, so
// the run can no longer resolve its states — no listing or submission can
// repair it — and clean must reclaim it, the on-demand equivalent of the
// replan it already requires.
func TestCleanReclaimsRunWithDeadEvidence(t *testing.T) {
	root, _, _ := sharedFixture(t)
	first := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	firstRun := mustLoadRun(t, root, first)
	ck := firstRun.CoverageByKey("code:contracts.js")
	if ck == nil {
		t.Fatalf("fixture must produce a public code key: %+v", firstRun.Coverage)
	}

	// Accept the public session; the successor run reuses its record.
	if err := sharedSubmit(t, root, first, ck.Key, grDefaultQualityReport(firstRun, *ck)); err != nil {
		t.Fatal(err)
	}
	second := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	secondRun := mustLoadRun(t, root, second)
	binding, ok := secondRun.Records[ck.Key]
	if !ok {
		t.Fatalf("successor run must reuse the accepted record: %+v", secondRun.Records)
	}
	if _, err := gaterun.LoadSessionStates(root, secondRun); err != nil {
		t.Fatalf("control: the successor must resolve while its record exists: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := runClean([]string{"--repo-root", root, "--dry-run"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "Delete run: "+second) {
		t.Fatalf("a resolvable run must not be reclaimable:\n%s", stdout.String())
	}

	// Remove the bound record like the pre-fix collection could: from here on
	// every resolution path for the run fails.
	p := filepath.Join(root, filepath.FromSlash(judgments.Directory), binding.Reference.ID+".json")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := runClean([]string{"--repo-root", root, "--dry-run"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Delete run: "+second) {
		t.Fatalf("clean did not reclaim the dead-evidence run:\n%s", stdout.String())
	}
	stdout.Reset()
	if err := runClean([]string{"--repo-root", root}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "meta", "gate_runs", second)); !os.IsNotExist(err) {
		t.Fatal("clean did not delete the dead-evidence run")
	}
}

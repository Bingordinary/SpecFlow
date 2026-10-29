package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMissionOutPathGuard: a gate-mission scratch file must not be repository
// content — it belongs outside the repository or inside the run's own state
// directory meta/gate_runs/<run_id>/ (framework/verification_scope.md §Coverage
// Model). The guard is what keeps a mission file out of the snapshot and the
// orphan sweep.
func TestMissionOutPathGuard(t *testing.T) {
	root := t.TempDir()
	runID := "20261029-090000-abc123"
	allowed := "meta/gate_runs/" + runID + "/mission-batch.txt"

	if err := validateMissionOutPath(root, runID, filepath.Join(root, "docs/specs/mission.txt")); err == nil {
		t.Fatal("a repository-content path must be rejected")
	}
	if err := validateMissionOutPath(root, runID, filepath.Join(root, "meta/gate_runs/other-run/mission.txt")); err == nil {
		t.Fatal("another run's state directory must be rejected")
	}
	if err := validateMissionOutPath(root, runID, filepath.Join(root, filepath.FromSlash(allowed))); err != nil {
		t.Fatalf("the run's own state directory must be allowed: %v", err)
	}
	if err := validateMissionOutPath(root, runID, filepath.Join(t.TempDir(), "mission.txt")); err != nil {
		t.Fatalf("a path outside the repository must be allowed: %v", err)
	}
}

// TestGateMissionOutWritesFile: --out writes the mission to the file and keeps
// the mission body out of stdout, so the coordinator never reads it into its
// own context.
func TestGateMissionOutWritesFile(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	out := filepath.Join(t.TempDir(), "mission-batch.txt")

	var stdout, stderr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", "design", "--format", "prompt", "--out", out}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-mission --out: %v (%s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mission written:") {
		t.Fatalf("expected the confirmation line on stdout, got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "You are the independent read-only reviewer") {
		t.Fatalf("the mission body must not be written to stdout when --out is set")
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "You are the independent read-only reviewer") {
		t.Fatalf("the mission file must hold the rendered mission, got %q", string(written))
	}
}

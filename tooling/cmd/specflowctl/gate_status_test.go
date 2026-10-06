package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #61: an open run whose bound state no longer resolves (a collected
// judgment binding, a missing shared task) must not abort the unscoped
// listing — the recovery point stays usable and one poisoned run cannot hide
// the others. Scoped --run stays strict.
func TestGateStatusListingDegradesStaleRun(t *testing.T) {
	root, _, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, root, id)

	// Sever one shared task the run's coverage reaches: LoadSessionStates
	// fails for this run exactly like a dangling judgment binding would.
	task := ""
	for _, ck := range run.Coverage {
		if ck.Task != "" {
			task = ck.Task
			break
		}
	}
	if task == "" {
		t.Fatal("fixture must produce a shared task")
	}
	if err := os.Remove(filepath.Join(root, "meta", "gate_runs", "shared", task+".json")); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", root}, &stdout, &stderr); err != nil {
		t.Fatalf("unscoped gate-status must degrade the poisoned run, not fail: %v", err)
	}
	if !strings.Contains(stdout.String(), "stale run state: "+id) {
		t.Fatalf("expected a stale entry for %s, got:\n%s", id, stdout.String())
	}

	var jsonOut, jsonErr bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", root, "--format", "json"}, &jsonOut, &jsonErr); err != nil {
		t.Fatalf("unscoped JSON gate-status must degrade too: %v", err)
	}
	var views []gateRunView
	if err := json.Unmarshal(jsonOut.Bytes(), &views); err != nil {
		t.Fatalf("parse JSON status: %v\n%s", err, jsonOut.String())
	}
	if len(views) != 1 || views[0].RunID != id || len(views[0].Notices) == 0 || !strings.Contains(views[0].Notices[0], "stale run state") {
		t.Fatalf("expected one stale JSON view for %s, got %+v", id, views)
	}

	var scopedOut, scopedErr bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", root, "--run", id}, &scopedOut, &scopedErr); err == nil {
		t.Fatal("scoped gate-status must stay strict")
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// gateMissionHintRe extracts the key list of every `gate-mission` command the
// next-step hint prints.
var gateMissionHintRe = regexp.MustCompile("gate-mission --run [^ ]+ --keys ([^ `]+)")

// gateStatusHintBatches returns the key batches the hint for one run prints.
func gateStatusHintBatches(t *testing.T, root, runID string) [][]string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", root, "--run", runID}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-status failed: %v (%s)", err, stderr.String())
	}
	var batches [][]string
	for _, m := range gateMissionHintRe.FindAllStringSubmatch(stdout.String(), -1) {
		batches = append(batches, strings.Split(m[1], ","))
	}
	if len(batches) == 0 {
		t.Fatalf("hint carries no gate-mission command:\n%s", stdout.String())
	}
	return batches
}

// grMission runs gate-mission for one key batch.
func grMission(t *testing.T, root, runID, keys string) error {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", runID, "--keys", keys}, &stdout, &stderr); err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

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

// Issue #85: the "Next:" hint must print only commands that run. A delta run's
// uncovered set mixes the delta_review key with check keys, and a verify run
// mixes item, unit, and quality kinds; one joined `gate-mission` command is
// always rejected for mixing kinds, so the hint must emit one command per
// runnable session — including the verify quality co-batch that keeps a design
// key ready without a separately accepted public record.
func TestGateStatusHintPrintsRunnableBatches(t *testing.T) {
	t.Run("verify splits item, unit, and quality kinds", func(t *testing.T) {
		root := createCLITestRepo(t)
		grEnableMissionLayout(t, root)
		grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.core"})
		grWriteFile(t, root, "src/auth.go", "package auth\n\nfunc Core() {}\n")

		verifyID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
		assertHintBatches(t, root, verifyID)
	})

	t.Run("verify delta has no review batch", func(t *testing.T) {
		root := createCLITestRepo(t)
		grEnableMissionLayout(t, root)
		specPath := grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.core"})
		main := "docs/specs/units/candidate/unit_auth.md"
		grWriteFile(t, root, "src/auth.go", "package auth\n\nfunc Core() {}\n")

		validateID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
		grSubmitValidateSessions(t, root, validateID, main)
		grFinalizeOK(t, root, validateID)
		verifyID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
		grSubmitOK(t, root, verifyID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
		grAutoSubmitRemaining(t, root, verifyID)
		grFinalizeOK(t, root, verifyID)

		data, err := os.ReadFile(specPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose. Edited.", 1)), 0644); err != nil {
			t.Fatal(err)
		}

		deltaID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
		batches := assertHintBatches(t, root, deltaID)
		for _, batch := range batches {
			if grContainsString(batch, "review") {
				t.Fatalf("a verify delta plan has no review session, got %v", batches)
			}
		}
	})
}

// assertHintBatches pins the hint's shape and acceptability: every printed
// batch must materialize as a real session through gate-mission, and the single
// joined command the old hint printed must be rejected for mixing report kinds.
func assertHintBatches(t *testing.T, root, runID string) [][]string {
	t.Helper()
	batches := gateStatusHintBatches(t, root, runID)
	if len(batches) < 2 {
		t.Fatalf("hint must split the uncovered keys into runnable sessions, got %v", batches)
	}
	run := mustLoadRun(t, root, runID)
	var all []string
	for _, batch := range batches {
		all = append(all, batch...)
	}
	if _, err := gaterun.BuildSessionSpec(root, run, all); err == nil || !strings.Contains(err.Error(), "mixes report kinds") {
		t.Fatalf("premise: the joined mixed-kind batch must be rejected, got %v", err)
	}
	// gate-mission is the hint's real contract: it rejects a batch that mixes
	// kinds or lenses, or whose design dependency is not yet ready. Materializing
	// every printed batch here proves each printed command actually runs.
	for _, batch := range batches {
		if err := grMission(t, root, runID, strings.Join(batch, ",")); err != nil {
			t.Fatalf("printed batch %v is not runnable: %v", batch, err)
		}
	}
	return batches
}

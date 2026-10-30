package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/operation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
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

	// One closed operation (an audit record sweep may discard) and one open
	// operation (live state that must survive) in a second, operation-managed
	// repository.
	opRoot, specRel := opTestGitRepo(t)
	closedID := opCloseClean(t, opRoot, specRel)
	openID := opOpenClean(t, opRoot)

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

	// Cleaning the first repository must not have touched the second one's
	// operation state.
	if _, err := os.Stat(filepath.Join(opRoot, "meta", "operations", openID+".json")); err != nil {
		t.Fatal("clean touched operation state in another repository")
	}
	if _, err := os.Stat(filepath.Join(opRoot, "meta", "operations", closedID+".json")); !os.IsNotExist(err) {
		t.Fatal("the closed operation state must be removed by its close")
	}
}

// opOpenClean opens a minimal path-only operation and returns its id.
func opOpenClean(t *testing.T, repoRoot string) string {
	t.Helper()
	stdout, stderr, err := opRun("open", "--allow", "internal/demo", "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("operation open failed: %v\nstderr=%s", err, stderr)
	}
	return opParseID(t, stdout)
}

// opCloseClean opens an operation that passes its check and closes it,
// returning the closed operation id.
func opCloseClean(t *testing.T, repoRoot, specRel string) string {
	t.Helper()
	stdout, stderr, err := opRun("open", "--unit", "demo", "--require-spec", specRel, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("operation open failed: %v\nstderr=%s", err, stderr)
	}
	opID := opParseID(t, stdout)
	opWriteFile(t, repoRoot, specRel, "# demo\n\nUpdated design.\n")
	stdout, stderr, err = opRun("close", "--id", opID, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("operation close failed: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	return opID
}

// TestCleanSweepsClosedOperationsPreview verifies the closed-operation sweep:
// a dry run lists a surviving closed state without deleting it, the apply
// deletes it, an open operation is never selected, and an unprovable state
// neither fails the preview nor is selected by it. A surviving closed state
// is a leftover from an older tooling version that persisted closed states,
// so the test stages one back.
func TestCleanSweepsClosedOperationsPreview(t *testing.T) {
	root, specRel := opTestGitRepo(t)
	closedID := opCloseClean(t, root, specRel)
	stageClosedOperationState(t, root, specRel, closedID)
	openID := opOpenClean(t, root)

	// Unprovable state (a file that does not load as a complete, valid closed
	// state) must follow the same rule on both paths: skipped, never listed,
	// never deleted.
	unprovableID := "20260101-000000-aaaaaa"
	unprovable := filepath.Join(root, "meta", "operations", unprovableID+".json")
	if err := os.WriteFile(unprovable, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runClean([]string{"--repo-root", root, "--dry-run"}, &stdout, &stderr); err != nil {
		t.Fatalf("clean --dry-run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Delete operation: "+closedID) {
		t.Fatalf("dry run did not report the closed operation:\n%s", out)
	}
	if strings.Contains(out, openID) || strings.Contains(out, unprovableID) {
		t.Fatalf("dry run selected an open or unprovable operation:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, "meta", "operations", closedID+".json")); err != nil {
		t.Fatal("dry run deleted the closed operation")
	}

	stdout.Reset()
	if err := runClean([]string{"--repo-root", root}, &stdout, &stderr); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "meta", "operations", closedID+".json")); !os.IsNotExist(err) {
		t.Fatal("clean did not delete the closed operation")
	}
	if _, err := os.Stat(filepath.Join(root, "meta", "operations", openID+".json")); err != nil {
		t.Fatal("clean deleted the open operation")
	}
	if _, err := os.Stat(unprovable); err != nil {
		t.Fatal("clean deleted an unprovable operation state")
	}
}

// stageClosedOperationState recreates the leftover shape an older tooling
// version leaves behind: a valid closed operation state under the given id,
// which clean must migrate away.
func stageClosedOperationState(t *testing.T, repoRoot, specRel, operationID string) {
	t.Helper()
	stdout, stderr, err := opRun("open", "--unit", "demo", "--require-spec", specRel, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("operation open failed: %v\nstderr=%s", err, stderr)
	}
	opID := opParseID(t, stdout)
	opWriteFile(t, repoRoot, specRel, "# demo\n\nUpdated design.\n")

	path := filepath.Join(repoRoot, "meta", "operations", opID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read staged operation state: %v", err)
	}
	var op operation.Operation
	if err := json.Unmarshal(data, &op); err != nil {
		t.Fatal(err)
	}
	op.OperationID = operationID
	op.Status = operation.StatusClosed
	op.ClosedAt = op.UpdatedAt
	op.CloseOutcome = operation.CloseOutcomePassed
	staged, err := json.MarshalIndent(&op, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "meta", "operations", operationID+".json"), append(staged, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestPlanSweepClearsConsumedInputManifest verifies that gate-plan is itself a
// cleanup point for meta/plan_inputs/: the manifest is consumed into the run
// snapshot (so a later replan or finalize never needs the file again) and the
// file is cleared in the same plan, exactly like the other orphan sweep
// points.
func TestPlanSweepClearsConsumedInputManifest(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, ".gitignore", "/meta/\n")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	manifestDir := filepath.Join(repoRoot, "meta", "plan_inputs")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(manifestDir, "validate-auth-candidate.txt")
	if err := os.WriteFile(manifest, []byte("src/auth.go\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--inputs-file", manifest)

	// The manifest entry is part of the run's immutable snapshot ...
	run := mustLoadRun(t, repoRoot, runID)
	found := false
	for _, input := range run.ExtraInputs {
		if input == "src/auth.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("planned run lost the manifest input: %+v", run.ExtraInputs)
	}
	// ... and the consumed scratch file is already cleared.
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatal("gate-plan did not clear the consumed input manifest")
	}
}

func TestCleanSweepsOrphanedRun(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	// Deleting the unit's spec files is an ordinary edit; `clean` reclaims the
	// run whose target spec no longer exists.
	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := runClean([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("clean: %v\n%s", err, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "meta", "gate_runs", runID)); !os.IsNotExist(err) {
		t.Fatal("orphaned run survived the clean sweep")
	}
}

// A retired target's durable per-target records are reclaimed mechanically:
// deleting the unit's spec files (an ordinary edit) leaves its validation
// caches, promise state and publication baseline orphaned, and the sweep
// removes them. A unit that still exists in one layer is never reclaimed.
func TestCleanReclaimsRetiredTargetDurableState(t *testing.T) {
	root, _, _ := sharedFixture(t)
	publishUnitThroughGates(t, root, "auth")

	baseline := filepath.Join(root, "docs/specs/meta/baseline/unit/auth.yaml")
	cacheDir := filepath.Join(root, "docs/specs/meta/validation/unit/auth")
	if _, err := os.Stat(baseline); err != nil {
		t.Fatalf("fixture must publish a baseline: %v", err)
	}
	if _, err := os.Stat(cacheDir); err != nil {
		t.Fatalf("fixture must publish a validation cache: %v", err)
	}

	// The unit still exists in the stable layer: a clean must reclaim nothing.
	var stdout, stderr bytes.Buffer
	if err := runClean([]string{"--repo-root", root}, &stdout, &stderr); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if strings.Contains(stdout.String(), "Delete durable state: unit auth") {
		t.Fatalf("clean reclaimed a target that still exists:\n%s", stdout.String())
	}
	if _, err := os.Stat(baseline); err != nil {
		t.Fatalf("a live target's baseline must survive: %v", err)
	}

	// Retire the unit: neither layer carries it any more.
	if err := os.Remove(filepath.Join(root, "docs/specs/units/stable/unit_auth.md")); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	if err := runClean([]string{"--repo-root", root, "--dry-run"}, &stdout, &stderr); err != nil {
		t.Fatalf("clean --dry-run: %v", err)
	}
	if !strings.Contains(stdout.String(), "Delete durable state: unit auth") {
		t.Fatalf("dry run did not report the retired target's durable state:\n%s", stdout.String())
	}
	if _, err := os.Stat(baseline); err != nil {
		t.Fatal("dry run deleted the durable state")
	}

	stdout.Reset()
	if err := runClean([]string{"--repo-root", root}, &stdout, &stderr); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := os.Stat(baseline); !os.IsNotExist(err) {
		t.Fatal("clean left the retired target's baseline")
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatal("clean left the retired target's validation cache")
	}
}

// A deferred finding whose owner unit has been retired can never be disposed
// by the run it is routed to, so the sweep drops it from the ledger. Entries
// owned by a unit that still exists, and the shared judgment records, stay.
func TestCleanDropsOrphanedDeferredFindings(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpec(t, root, "auth")
	grWriteSpec(t, root, "order")

	entry := func(id, owner, source string) validationcache.DeferredEntry {
		return validationcache.DeferredEntry{
			FindingID: id, OwnerUnit: owner, SourceUnit: source,
			SourceRun: "20260101-000000-aaaaaa", Severity: "P2",
			Text: "pending finding", Detail: "detail", SourceKey: "code:contracts.js",
			EvidencePath: "contracts.js", Reason: "ownership evidence",
		}
	}
	ledger := validationcache.DeferredLedger{Entries: []validationcache.DeferredEntry{
		entry("orphan", "auth", "order"),
		entry("live", "order", "auth"),
	}}
	if err := validationcache.WriteDeferredLedger(root, ledger); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := runClean([]string{"--repo-root", root}, &stdout, &stderr); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if !strings.Contains(stdout.String(), "Drop deferred findings owned by: auth") {
		t.Fatalf("clean did not report the orphaned deferral:\n%s", stdout.String())
	}

	after, err := validationcache.ReadDeferredLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.PendingForUnit("auth"); len(got) != 0 {
		t.Fatalf("orphaned deferral survived: %+v", got)
	}
	if got := after.PendingForUnit("order"); len(got) != 1 {
		t.Fatalf("live deferral was dropped: %+v", got)
	}
}

// meta/ is the framework-owned disposable tree: entries outside the declared
// layout are strays, reported by the preview and removed by the sweep, while
// the declared directories and the lock carrier stay.
func TestCleanRemovesUndeclaredMetaEntries(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	strayDir := filepath.Join(repoRoot, "meta", "missions")
	strayFile := filepath.Join(repoRoot, "meta", "notes.txt")
	lockCarrier := filepath.Join(repoRoot, "meta", ".gate_runs.lock")
	if err := os.MkdirAll(strayDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{strayFile, lockCarrier} {
		if err := os.WriteFile(path, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	if err := runClean([]string{"--repo-root", repoRoot, "--dry-run"}, &stdout, &stderr); err != nil {
		t.Fatalf("clean --dry-run: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Delete stray: missions") || !strings.Contains(out, "Delete stray: notes.txt") {
		t.Fatalf("dry run did not report the strays:\n%s", out)
	}
	if _, err := os.Stat(strayDir); err != nil {
		t.Fatal("dry run deleted a stray")
	}

	stdout.Reset()
	if err := runClean([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("clean: %v", err)
	}
	for _, path := range []string{strayDir, strayFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("clean left the stray %s", path)
		}
	}
	if _, err := os.Stat(lockCarrier); err != nil {
		t.Fatal("clean removed the lock carrier")
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
	p, err := judgments.StoredPath(root, binding.Reference)
	if err != nil {
		t.Fatal(err)
	}
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

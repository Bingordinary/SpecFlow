package gaterun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

func planOrFail(t *testing.T, repoRoot, gate, kind, name, target, mode string) *Run {
	t.Helper()
	run, err := Plan(repoRoot, gate, kind, name, target, mode, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("plan %s %s %s: %v", gate, kind, name, err)
	}
	return run
}

func runDirPath(t *testing.T, repoRoot, runID string) string {
	t.Helper()
	path, err := runStatePath(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(path)
}

func taskIDs(run *Run) map[string]bool {
	out := map[string]bool{}
	for _, ck := range run.Coverage {
		if ck.Task != "" {
			out[ck.Task] = true
		}
	}
	return out
}

func sharedTaskIDs(t *testing.T, repoRoot string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, "meta", "gate_runs", "shared"))
	if os.IsNotExist(err) {
		return map[string]bool{}
	}
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		out[strings.TrimSuffix(entry.Name(), ".json")] = true
	}
	return out
}

func sameIDSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if !b[id] {
			return false
		}
	}
	return true
}

func TestSweepRemovesOrphanedRunsAndTasks(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src/a.go", "")
	writeUnit(t, repoRoot, "candidate", "order", "none", "none", "src/b.go", "")
	writeFile(t, repoRoot, "src/a.go", "package a\n")
	writeFile(t, repoRoot, "src/b.go", "package b\n")

	authRun := planOrFail(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull)
	orderRun := planOrFail(t, repoRoot, GateVerify, TargetKindUnit, "order", TargetCandidate, ModeFull)
	authTasks, orderTasks := taskIDs(authRun), taskIDs(orderRun)
	if len(authTasks) == 0 || len(orderTasks) == 0 {
		t.Fatalf("fixture must create shared tasks: auth=%v order=%v", authTasks, orderTasks)
	}

	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	result, err := SweepOrphanedState(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Runs) != 1 || result.Runs[0].ID != authRun.RunID {
		t.Fatalf("expected only the auth run to be swept, got %+v", result.Runs)
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, authRun.RunID)); !os.IsNotExist(err) {
		t.Fatal("orphaned run survived the sweep")
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, orderRun.RunID)); err != nil {
		t.Fatalf("live run was removed: %v", err)
	}
	if got := sharedTaskIDs(t, repoRoot); !sameIDSet(got, orderTasks) {
		t.Fatalf("task sweep mismatch: got %v want the surviving run's tasks %v", got, orderTasks)
	}
}

func TestPlanSweepsOrphanedState(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src/a.go", "")
	writeUnit(t, repoRoot, "candidate", "order", "none", "none", "src/b.go", "")
	writeFile(t, repoRoot, "src/a.go", "package a\n")
	writeFile(t, repoRoot, "src/b.go", "package b\n")

	authRun := planOrFail(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull)
	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	planOrFail(t, repoRoot, GateVerify, TargetKindUnit, "order", TargetCandidate, ModeFull)
	if _, err := os.Stat(runDirPath(t, repoRoot, authRun.RunID)); !os.IsNotExist(err) {
		t.Fatal("a new plan did not sweep the orphaned run")
	}
}

func TestPlanSweepIsNonDestructive(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src/a.go", "")
	writeFile(t, repoRoot, "src/a.go", "package a\n")

	run := planOrFail(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull)
	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	planned, err := PlanSweep(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.Runs) != 1 || planned.Runs[0].ID != run.RunID {
		t.Fatalf("PlanSweep did not report the orphan: %+v", planned)
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, run.RunID)); err != nil {
		t.Fatal("PlanSweep must not delete state")
	}
	applied, err := SweepOrphanedState(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Runs) != len(planned.Runs) || applied.Runs[0].ID != planned.Runs[0].ID {
		t.Fatalf("applied sweep %+v does not match the preview %+v", applied, planned)
	}
	after, err := PlanSweep(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Runs) != 0 || len(after.Tasks) != 0 {
		t.Fatalf("sweep left residue: %+v", after)
	}
}

func TestSweepKeepsStableRunWhileSpecExists(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "stable", "auth", "none", "none", "src/a.go", "")
	writeFile(t, repoRoot, "src/a.go", "package a\n")

	run := planOrFail(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetStable, ModeFull)
	if result, err := SweepOrphanedState(repoRoot); err != nil || len(result.Runs) != 0 {
		t.Fatalf("existing stable spec must keep its run: %+v %v", result, err)
	}
	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/units/stable/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	result, err := SweepOrphanedState(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Runs) != 1 || result.Runs[0].ID != run.RunID {
		t.Fatalf("removed stable spec must orphan its run, got %+v", result.Runs)
	}
}

func TestSweepLeavesUnknownRunState(t *testing.T) {
	repoRoot := newRepo(t)
	dir := filepath.Join(repoRoot, "meta", "gate_runs", "20260101-000000-abcdef")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "run.json", "{not json")

	result, err := SweepOrphanedState(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Runs) != 0 {
		t.Fatalf("unknown run state must not be swept, got %+v", result.Runs)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("unknown run state was deleted")
	}
}

// An open run the strict loader rejects can never resume: it is skipped by
// every listing, never replaced by a replan for the same gate and target, and
// its protocol requires it to be replanned. Leaving it on disk also pins its
// bound records beyond the reach of collection forever. The sweep is the
// reclaim path (issue #61).
func TestSweepReclaimsUnresumableOpenRun(t *testing.T) {
	repoRoot := t.TempDir()
	bound := gcRecord(t, repoRoot, "bound.js", "stale-protocol-fingerprint", nil)

	rejected := "20260101-000000-a11d01"
	writeRetiredRunState(t, repoRoot, rejected, StatusOpen, map[string]judgments.Binding{
		"code:bound.js": {Reference: bound, Layer: TargetCandidate, Source: "executed"},
	})
	// A finished rejected run is audit state, not a resumable plan.
	writeRetiredRunState(t, repoRoot, "20260101-000000-a11d02", StatusConsumed, nil)

	planned, err := PlanSweep(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.Runs) != 1 || planned.Runs[0].ID != rejected {
		t.Fatalf("expected only the unresumable open run to be reclaimable, got %+v", planned.Runs)
	}
	if planned.Runs[0].Gate != GateVerify || planned.Runs[0].TargetKind != TargetKindUnit || planned.Runs[0].TargetName != "auth" || planned.Runs[0].Target != TargetCandidate {
		t.Fatalf("reclaimed run must report its decoded identity, got %+v", planned.Runs[0])
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, rejected)); err != nil {
		t.Fatal("PlanSweep must not delete state")
	}

	result, err := SweepOrphanedState(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Runs) != 1 || result.Runs[0].ID != rejected {
		t.Fatalf("sweep removed %+v, want the single rejected run", result.Runs)
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, rejected)); !os.IsNotExist(err) {
		t.Fatal("rejected open run survived the sweep")
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, "20260101-000000-a11d02")); err != nil {
		t.Fatalf("a finished rejected run was removed: %v", err)
	}

	// With the rejected run gone, nothing protects its stale record anymore:
	// reclaiming the run terminates the GC-protected-forever leak.
	collected, err := CollectJudgments(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if collected.Count != 1 {
		t.Fatalf("collected %+v, want the freed record", collected)
	}
	if _, err := judgments.Load(repoRoot, bound); err == nil {
		t.Fatal("record freed by the reclaim survived collection")
	}
}

// writeLoadableRunState writes a current-shape open run state the strict
// loader accepts. It is hand-written because current tooling can no longer
// produce the dead-evidence states the sweep reclaims: judgment collection
// protects every record an on-disk open run binds, so only state damaged
// before that protection (or by hand) exists in this shape (issue #61).
func writeLoadableRunState(t *testing.T, root, runID, protocol string, coverage []CoverageKey, records map[string]judgments.Binding) {
	t.Helper()
	payload := mustJSON(t, map[string]any{
		"run_id": runID, "schema_version": 2, "protocol": protocol,
		"gate": GateVerify, "target_kind": TargetKindUnit, "target_name": "auth",
		"target": TargetCandidate, "status": StatusOpen, "mode": ModeFull,
		"records": records, "coverage": coverage,
	})
	p := filepath.Join(root, "meta", "gate_runs", runID, "run.json")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, payload, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, runID); err != nil {
		t.Fatalf("fixture must produce a run state the strict loader accepts: %v", err)
	}
}

// evidenceRecord saves a resolvable code record: its only dependency is the
// fixture spec file with no hash, so judgments.Check passes while the file
// exists and fails once the record file is removed.
func evidenceRecord(t *testing.T, root, protocol, specFile string) judgments.Reference {
	t.Helper()
	report := "fixture report"
	ref, err := judgments.Save(root, judgments.Record{
		Version:      judgments.RecordVersion,
		Kind:         judgments.Code,
		Subject:      "reused.js",
		Coverage:     []string{"code:reused.js"},
		Inputs:       []string{specFile},
		Dependencies: []judgments.Dependency{{Path: specFile}},
		Protocol:     protocol,
		Verdict:      "FACTS",
		Result:       json.RawMessage(`{"observations":[]}`),
		Report:       report,
		ReportDigest: judgments.Digest([]byte(report)),
		SourceRun:    "20260101-000000-src001",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func writeSessionFile(t *testing.T, root, runID, name string, content []byte) {
	t.Helper()
	p := filepath.Join(root, "meta", "gate_runs", runID, "sessions", name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0644); err != nil {
		t.Fatal(err)
	}
}

// Issue #61's stale-run class: an open run the strict loader accepts whose
// bound judgment record no longer resolves. Every listing and submission path
// fails on the dead binding and no command can repair it, so the sweep
// reclaims it — the on-demand equivalent of the replan it already requires.
func TestSweepReclaimsOpenRunWithDeadEvidence(t *testing.T) {
	repoRoot := t.TempDir()
	specFile := "docs/specs/units/candidate/unit_auth.md"
	writeFile(t, repoRoot, specFile, "# auth\n")

	protocol := "fixture-protocol"
	runID := "20260101-000000-b11d01"
	record := evidenceRecord(t, repoRoot, protocol, specFile)
	coverage := []CoverageKey{{Key: "code:reused.js", Kind: SessionKindCode, Lens: LensQuality, Task: "task-0001"}}
	writeLoadableRunState(t, repoRoot, runID, protocol, coverage, map[string]judgments.Binding{
		"code:reused.js": {Reference: record, Layer: TargetCandidate, Source: "executed"},
	})

	// Control: while the record resolves, the run must survive the sweep.
	planned, err := PlanSweep(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.Runs) != 0 {
		t.Fatalf("a run with resolvable evidence must not be reclaimable: %+v", planned.Runs)
	}

	if err := os.Remove(filepath.Join(repoRoot, filepath.FromSlash(judgments.Directory), record.ID+".json")); err != nil {
		t.Fatal(err)
	}
	run, err := Load(repoRoot, runID)
	if err != nil {
		t.Fatalf("the strict loader must still accept the run: %v", err)
	}
	if _, err := LoadSessionStates(repoRoot, run); err == nil {
		t.Fatal("fixture must make evidence resolution fail")
	}

	planned, err = PlanSweep(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned.Runs) != 1 || planned.Runs[0].ID != runID {
		t.Fatalf("expected only the dead-evidence run to be reclaimable, got %+v", planned.Runs)
	}
	if planned.Runs[0].Gate != GateVerify || planned.Runs[0].TargetKind != TargetKindUnit || planned.Runs[0].TargetName != "auth" || planned.Runs[0].Target != TargetCandidate {
		t.Fatalf("reclaimed run must report its identity, got %+v", planned.Runs[0])
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, runID)); err != nil {
		t.Fatal("PlanSweep must not delete state")
	}

	result, err := SweepOrphanedState(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Runs) != 1 || result.Runs[0].ID != runID {
		t.Fatalf("sweep removed %+v, want the dead-evidence run", result.Runs)
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, runID)); !os.IsNotExist(err) {
		t.Fatal("dead-evidence run survived the sweep")
	}
}

// A binding whose key an accepted session already covers is never consulted
// by the resolution gate, so a missing record behind it cannot make the run
// stale — a key covered by an accepted session finalizes without its binding.
// The sweep must mirror that skip; reclaiming would destroy a run that can
// still complete.
func TestSweepKeepsCoveredBindingWithMissingRecord(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", "# auth\n")

	runID := "20260101-000000-b11d02"
	missing := judgments.Reference{ID: judgments.Digest([]byte("missing")), Digest: judgments.Digest([]byte("missing"))}
	coverage := []CoverageKey{{Key: "code:reused.js", Kind: SessionKindCode, Lens: LensQuality, Task: "task-0001"}}
	writeLoadableRunState(t, repoRoot, runID, "fixture-protocol", coverage, map[string]judgments.Binding{
		"code:reused.js": {Reference: missing, Layer: TargetCandidate, Source: "executed"},
	})
	key := "code:reused.js"
	writeSessionFile(t, repoRoot, runID, sessionFileBase(key)+".json", mustJSON(t, &SessionState{SessionID: key, Keys: []string{key}, Status: SessionAccepted}))

	run, err := Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSessionStates(repoRoot, run); err != nil {
		t.Fatalf("a covered key must resolve without its binding: %v", err)
	}
	if result, err := SweepOrphanedState(repoRoot); err != nil || len(result.Runs) != 0 {
		t.Fatalf("a covered binding must not make the run reclaimable: %+v %v", result, err)
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, runID)); err != nil {
		t.Fatal("run with a covered binding was removed")
	}
}

// Session state is mutable progress, not immutable evidence: a missing state
// file means the session is still pending, so removing an unusable file
// reopens the session for execution. The sweep must never reclaim a run over
// repairable session damage, even though every listing path fails on it.
func TestSweepKeepsRepairableSessionDamage(t *testing.T) {
	repoRoot := t.TempDir()
	specFile := "docs/specs/units/candidate/unit_auth.md"
	writeFile(t, repoRoot, specFile, "# auth\n")

	protocol := "fixture-protocol"
	runID := "20260101-000000-b11d03"
	record := evidenceRecord(t, repoRoot, protocol, specFile)
	coverage := []CoverageKey{{Key: "code:reused.js", Kind: SessionKindCode, Lens: LensQuality, Task: "task-0001"}}
	writeLoadableRunState(t, repoRoot, runID, protocol, coverage, map[string]judgments.Binding{
		"code:reused.js": {Reference: record, Layer: TargetCandidate, Source: "executed"},
	})

	writeSessionFile(t, repoRoot, runID, "deadbeef.json", []byte("{not json"))
	if result, err := SweepOrphanedState(repoRoot); err != nil || len(result.Runs) != 0 {
		t.Fatalf("repairable session damage must not make the run reclaimable: %+v %v", result, err)
	}
	if _, err := os.Stat(runDirPath(t, repoRoot, runID)); err != nil {
		t.Fatal("run with repairable session damage was removed")
	}

	// Removing the damaged file reopens the session; the run stays a normal
	// resumable run.
	if err := os.Remove(filepath.Join(repoRoot, "meta", "gate_runs", runID, "sessions", "deadbeef.json")); err != nil {
		t.Fatal(err)
	}
	if result, err := SweepOrphanedState(repoRoot); err != nil || len(result.Runs) != 0 {
		t.Fatalf("repaired run must stay: %+v %v", result, err)
	}
}

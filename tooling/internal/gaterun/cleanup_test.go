package gaterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

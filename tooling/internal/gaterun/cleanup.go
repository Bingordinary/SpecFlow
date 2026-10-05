package gaterun

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SweptRun identifies one run directory removed (or removable) by the
// local-state sweep.
type SweptRun struct {
	ID         string
	Gate       string
	TargetKind string
	TargetName string
	Target     string
}

// SweepResult reports one cleanup pass: run directories whose target spec no
// longer exists and shared task files no surviving run references.
type SweepResult struct {
	Runs  []SweptRun
	Tasks []string
}

// PlanSweep reports what SweepOrphanedState would remove without removing
// anything.
func PlanSweep(repoRoot string) (SweepResult, error) {
	return collectSweep(repoRoot)
}

// SweepOrphanedState removes the local state of targets that no longer exist:
// a run directory is removed when its target's spec file is gone from the
// run's own layer, and a shared task file is removed when no surviving run
// references it. The durable truth stays in the committed caches and judgment
// records; a removed task file is recreated on demand by the next plan.
// Callers must hold the repository mutation lock (WithMutation): Plan does,
// and promote/remove/clean wrap the call.
func SweepOrphanedState(repoRoot string) (SweepResult, error) {
	result, err := collectSweep(repoRoot)
	if err != nil {
		return result, err
	}
	for _, run := range result.Runs {
		dir, err := runDirectoryPath(repoRoot, run.ID)
		if err != nil {
			return result, err
		}
		if err := os.RemoveAll(dir); err != nil {
			return result, err
		}
	}
	sharedDir := filepath.Join(repoRoot, filepath.FromSlash(runStateDir), "shared")
	for _, id := range result.Tasks {
		if err := os.Remove(filepath.Join(sharedDir, id+".json")); err != nil && !os.IsNotExist(err) {
			return result, err
		}
	}
	return result, nil
}

// collectSweep computes the sweep without deleting. A run directory whose
// state cannot be loaded by the normal loader is left alone: unknown or
// damaged state is never deleted.
func collectSweep(repoRoot string) (SweepResult, error) {
	var result SweepResult
	dir := filepath.Join(repoRoot, filepath.FromSlash(runStateDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read gate run directory: %w", err)
	}
	var surviving []Run
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		run, err := Load(repoRoot, entry.Name())
		if err != nil {
			continue
		}
		spec := targetLayerSpecRef(run.TargetKind, run.TargetName, run.Target)
		if !fileExists(filepath.Join(repoRoot, filepath.FromSlash(spec))) {
			result.Runs = append(result.Runs, SweptRun{
				ID:         run.RunID,
				Gate:       run.Gate,
				TargetKind: run.TargetKind,
				TargetName: run.TargetName,
				Target:     run.Target,
			})
			continue
		}
		surviving = append(surviving, *run)
	}
	referenced := map[string]bool{}
	for _, run := range surviving {
		for _, ck := range run.Coverage {
			if ck.Task != "" {
				referenced[ck.Task] = true
			}
		}
	}
	shared, err := os.ReadDir(filepath.Join(dir, "shared"))
	if err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("read shared task directory: %w", err)
	}
	for _, entry := range shared {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !referenced[id] {
			result.Tasks = append(result.Tasks, id)
		}
	}
	sort.Slice(result.Runs, func(i, j int) bool { return result.Runs[i].ID < result.Runs[j].ID })
	sort.Strings(result.Tasks)
	return result, nil
}

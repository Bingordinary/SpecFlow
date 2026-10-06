package gaterun

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
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

// SweepResult reports one cleanup pass: run directories that can no longer
// describe a live target or a resumable open run, and shared task files no
// surviving run references.
type SweepResult struct {
	Runs  []SweptRun
	Tasks []string
}

// PlanSweep reports what SweepOrphanedState would remove without removing
// anything.
func PlanSweep(repoRoot string) (SweepResult, error) {
	return collectSweep(repoRoot)
}

// SweepOrphanedState removes the local state of targets that no longer exist
// and of open runs that can no longer resume: a run directory is removed when
// its target's spec file is gone from the run's own layer, when the strict
// loader rejects its open state (a retired protocol, an invalid shape — such
// a run is never listed, never replaced by a replan, and its protocol
// requires it to be replanned), or when its planned coverage can no longer
// resolve its bound evidence (a judgment or shared task record that is
// missing, unreadable, or no longer valid — the stale state the unscoped
// listing exposes and no submission can repair), and a shared task file is
// removed when no surviving run references it. The durable truth stays in the
// committed caches and judgment records; a removed task file is recreated on
// demand by the next plan. Callers must hold the repository mutation lock
// (WithMutation): Plan does, and promote/remove/clean wrap the call.
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
// state cannot be loaded by the normal loader is reclaimed only when its raw
// decode proves an open status: an open run the strict loader rejects can
// never resume, so keeping it would hold its bound records beyond the reach
// of collection forever. A loadable open run is reclaimed when its planned
// coverage can no longer resolve its bound evidence: no listing, submission,
// or finalize can proceed on such a run, so reclaiming it is the on-demand
// equivalent of the replan it already requires (see unresumableEvidence).
// State that cannot even be decoded, finished (non-open) runs, and runs kept
// alive by repairable session-state damage are left alone: unprovable,
// audit, or repairable state is never deleted.
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
			raw, rawErr := readRawRunState(repoRoot, entry.Name())
			if rawErr != nil || !raw.Open() {
				continue
			}
			// A directory whose name cannot address a run is not reclaimable
			// through the run-state path — no command could have created it.
			if validateErr := localstate.ValidateID(entry.Name()); validateErr != nil {
				continue
			}
			result.Runs = append(result.Runs, SweptRun{
				ID:         entry.Name(),
				Gate:       raw.Gate,
				TargetKind: raw.TargetKind,
				TargetName: raw.TargetName,
				Target:     raw.Target,
			})
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
		if run.Status == StatusOpen && unresumableEvidence(repoRoot, run) {
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

// unresumableEvidence reports whether a loadable open run can no longer
// resume because the evidence its planned coverage depends on cannot resolve:
// LoadSessionStates fails on a bound judgment or shared task record that is
// missing, unreadable, or no longer valid (its inputs changed, its protocol
// is retired, it was invalidated or superseded). Every listing and submission
// path fails on such a run, so reclaiming it is the on-demand equivalent of
// the replan it already requires. Session state files are the one exclusion:
// a damaged session state is mutable progress the run repairs by
// re-execution — a missing session state means the session is still pending —
// so it never makes a run reclaimable here.
func unresumableEvidence(repoRoot string, run *Run) bool {
	if !sessionStatesIntact(repoRoot, run) {
		return false
	}
	_, err := LoadSessionStates(repoRoot, run)
	return err != nil
}

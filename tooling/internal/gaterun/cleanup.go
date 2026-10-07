package gaterun

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
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
// describe a live target or a resumable open run, run directories preserved
// under the retired audit policy (the migration path for states written
// before terminal runs were removed at their terminal command), shared task
// files no surviving run references, the consumed input manifests under
// meta/plan_inputs/, and undeclared "stray" entries under meta/ that belong
// to no governed local-state tree.
type SweepResult struct {
	Runs   []SweptRun
	Audits []SweptRun
	Tasks  []string
	Inputs []string
	Strays []string
}

// PlanSweep reports what SweepOrphanedState would remove without removing
// anything: orphaned run directories, retired audit run directories,
// unreferenced shared task files, the consumed input manifests under
// meta/plan_inputs/, and undeclared entries under meta/.
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
// removed when no surviving run references it. The consumed scratch input
// manifests under meta/plan_inputs/ are cleared in the same pass: a manifest
// is read once by gate-plan, its entries are part of the run's immutable
// snapshot, and no later command reads the file, so a manifest that survives
// its plan has no value for any run — a manifest written but not yet consumed
// is re-written by piecemeal planning. Terminal runs (consumed, invalidated)
// are removed by the command that concluded them; a surviving one is a crash
// leftover or a state written before that policy, and the sweep removes it.
// Undeclared entries under meta/ are removed as strays: meta/ is the
// framework-owned disposable tree, an entry outside the declared layout
// belongs to no governed local-state tree, and the sweep is where it is
// reclaimed. The durable truth stays in the committed caches and judgment
// records; a removed task file is recreated on demand by the next plan, and a
// removed manifest is recreated on demand by the next plan that needs one.
// Every deletion target is resolved through the local-state boundary before
// anything is deleted: a run, shared task, plan input directory, or meta/
// entry that resolves outside its declared state root fails closed and the
// sweep deletes nothing. Callers must hold the repository mutation lock
// (WithMutation): Plan does, and promote/remove/clean wrap the call.
func SweepOrphanedState(repoRoot string) (SweepResult, error) {
	result, err := collectSweep(repoRoot)
	if err != nil {
		return result, err
	}
	sharedDir, err := sharedTasksDir(repoRoot)
	if err != nil {
		return result, err
	}
	inputsDir, err := planInputsDir(repoRoot)
	if err != nil {
		return result, err
	}
	metaPath, err := metaDir(repoRoot)
	if err != nil {
		return result, err
	}
	for _, run := range append(append([]SweptRun{}, result.Runs...), result.Audits...) {
		dir, err := runDirectoryPath(repoRoot, run.ID)
		if err != nil {
			return result, err
		}
		if err := os.RemoveAll(dir); err != nil {
			return result, err
		}
	}
	for _, id := range result.Tasks {
		if err := os.Remove(filepath.Join(sharedDir, id+".json")); err != nil && !os.IsNotExist(err) {
			return result, err
		}
	}
	for _, name := range result.Inputs {
		if err := os.Remove(filepath.Join(inputsDir, name)); err != nil && !os.IsNotExist(err) {
			return result, err
		}
	}
	for _, name := range result.Strays {
		if err := os.RemoveAll(filepath.Join(metaPath, name)); err != nil && !os.IsNotExist(err) {
			return result, err
		}
	}
	return result, nil
}

// sharedTasksDir resolves the shared task directory and proves it stays
// inside the gate-run state root, the same boundary run directories are
// proven against: a link that resolves outside fails closed before any task
// file is listed or removed.
func sharedTasksDir(repoRoot string) (string, error) {
	dir, err := localstate.Path(repoRoot, runStateDir, "shared")
	if err != nil {
		return "", fmt.Errorf("resolve shared task directory: %w", err)
	}
	return dir, nil
}

// planInputsDir resolves the scratch input directory and proves it stays
// inside the local-state tree, so a link that resolves outside fails closed
// before any manifest is listed or removed. The sweep deletes through proven
// paths only.
func planInputsDir(repoRoot string) (string, error) {
	dir, err := localstate.Path(repoRoot, "meta", "plan_inputs")
	if err != nil {
		return "", fmt.Errorf("resolve plan input directory: %w", err)
	}
	return dir, nil
}

// metaDir resolves the framework-owned local-state directory and proves it
// stays inside the repository: stray discovery and removal operate on direct
// entries below it, and a meta/ that resolves outside the repository fails
// closed before anything is listed or removed.
func metaDir(repoRoot string) (string, error) {
	dir, err := repopath.Canonical(repoRoot, "meta")
	if err != nil {
		return "", fmt.Errorf("resolve local-state directory: %w", err)
	}
	return filepath.Join(repoRoot, filepath.FromSlash(dir)), nil
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
	if err != nil && !os.IsNotExist(err) {
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
		if run.Status == StatusConsumed || run.Status == StatusInvalidated {
			// Terminal runs are removed by the command that concluded them; a
			// surviving one is a leftover from a crash or from the retired
			// audit-preserving policy — the sweep is its migration path.
			result.Audits = append(result.Audits, SweptRun{
				ID:         run.RunID,
				Gate:       run.Gate,
				TargetKind: run.TargetKind,
				TargetName: run.TargetName,
				Target:     run.Target,
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
	sharedDir, err := sharedTasksDir(repoRoot)
	if err != nil {
		return result, err
	}
	shared, err := os.ReadDir(sharedDir)
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
	sort.Slice(result.Audits, func(i, j int) bool { return result.Audits[i].ID < result.Audits[j].ID })
	sort.Strings(result.Tasks)

	// The scratch input manifests under meta/plan_inputs/ are consumable state
	// of the plan that read them: gate-plan reads a manifest once and stores
	// its entries in the run snapshot, and no later command reads the file. A
	// manifest that survives its plan describes nothing, so every sweep point
	// clears the directory — the same "no longer describes a live target"
	// principle applied to scratch input rather than to runs.
	inputsDir, err := planInputsDir(repoRoot)
	if err != nil {
		return result, err
	}
	inputEntries, readErr := os.ReadDir(inputsDir)
	if readErr != nil && !os.IsNotExist(readErr) {
		return result, fmt.Errorf("read plan input directory: %w", readErr)
	}
	for _, entry := range inputEntries {
		if entry.IsDir() {
			continue
		}
		result.Inputs = append(result.Inputs, entry.Name())
	}
	sort.Strings(result.Inputs)

	// Undeclared entries directly under meta/ belong to no governed
	// local-state tree: the four declared directories, the lock files, and
	// hidden carries are the complete known layout, and meta/ is the
	// framework-owned disposable tree — an entry outside that layout is a
	// stray (a misplaced working file, an editor artifact) and every sweep
	// point removes it. Hidden entries are preserved because the documented
	// lock carriers are hidden; declared content is never hidden.
	strays, err := collectMetaStrays(repoRoot)
	if err != nil {
		return result, err
	}
	result.Strays = strays
	return result, nil
}

// collectMetaStrays lists the undeclared entries directly under meta/ — the
// top-level entries outside the known layout (declared directories, hidden
// carriers). It never descends into the declared directories: their content
// is owned by their own sweep rules.
func collectMetaStrays(repoRoot string) ([]string, error) {
	dir, err := metaDir(repoRoot)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read local-state directory: %w", err)
	}
	var strays []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() && knownMetaDirectory(name) {
			continue
		}
		strays = append(strays, name)
	}
	sort.Strings(strays)
	return strays, nil
}

// knownMetaDirectory reports whether a directory name is one of the four
// declared local-state directories.
func knownMetaDirectory(name string) bool {
	switch name {
	case "gate_runs", "operations", "plan_inputs", "governance_review":
		return true
	}
	return false
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

package gaterun

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// rawRunState is the tolerance-level view of a run state: only the fields the
// judgment collector and the orphan sweep need, decoded without the strict
// loader's resumption-shape checks. The strict loader answers "is this run
// actionable under the current protocol?"; the raw reader answers "what does
// this on-disk state bind right now?" — a run the loader rejects still names
// records that nothing else protects, and skipping such a run in an
// enumeration that means "everything on disk" silently rewrites existence
// into absence.
type rawRunState struct {
	RunID      string
	Status     string
	Gate       string
	TargetKind string
	TargetName string
	Target     string
	Records    map[string]judgments.Binding
	Tasks      []string
}

// Open reports whether the raw state describes an open run.
func (raw *rawRunState) Open() bool {
	return raw.Status == StatusOpen
}

// readRawRunState decodes meta/gate_runs/<dir>/run.json without identity or
// shape validation. Bytes that cannot be read or parsed are an error — the
// callers fail closed: unprovable state is kept, never inferred away.
func readRawRunState(repoRoot, dir string) (*rawRunState, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(runStateDir), dir, "run.json"))
	if err != nil {
		return nil, err
	}
	var payload struct {
		RunID      string                       `json:"run_id"`
		Status     string                       `json:"status"`
		Gate       string                       `json:"gate"`
		TargetKind string                       `json:"target_kind"`
		TargetName string                       `json:"target_name"`
		Target     string                       `json:"target"`
		Records    map[string]judgments.Binding `json:"records"`
		Coverage   []struct {
			Task string `json:"task"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	raw := &rawRunState{
		RunID:      payload.RunID,
		Status:     payload.Status,
		Gate:       payload.Gate,
		TargetKind: payload.TargetKind,
		TargetName: payload.TargetName,
		Target:     payload.Target,
		Records:    payload.Records,
	}
	for _, ck := range payload.Coverage {
		if ck.Task != "" {
			raw.Tasks = append(raw.Tasks, ck.Task)
		}
	}
	return raw, nil
}

// openRunStates raw-reads every run state under meta/gate_runs and returns
// the ones whose status is open, whatever the strict loader would say. The
// "shared" task directory is not a run, and a run directory with no run.json
// (an interrupted initial write) provably binds nothing, so it is skipped. A
// present but unreadable or unparsable run.json fails the enumeration — a
// live-set computation that cannot prove a directory harmless must not
// shrink what it protects.
func openRunStates(repoRoot string) ([]*rawRunState, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(runStateDir)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var runs []*rawRunState
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "shared" {
			continue
		}
		raw, err := readRawRunState(repoRoot, entry.Name())
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if raw.Open() {
			runs = append(runs, raw)
		}
	}
	return runs, nil
}

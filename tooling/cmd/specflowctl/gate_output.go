package main

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

type gateCoverageView struct {
	Key        string   `json:"key"`
	Lens       string   `json:"lens,omitempty"`
	Kind       string   `json:"kind"`
	ReportKeys []string `json:"report_keys"`
	Source     string   `json:"source,omitempty"`
	Task       string   `json:"shared_task,omitempty"`
	CoveredBy  string   `json:"covered_by,omitempty"`
}

type gateSessionView struct {
	SessionID     string   `json:"session_id"`
	Keys          []string `json:"keys"`
	Status        string   `json:"status"`
	Attempts      int      `json:"attempts"`
	LastRejection string   `json:"last_rejection,omitempty"`
}

type gateRunView struct {
	SchemaVersion     int                       `json:"schema_version"`
	RunID             string                    `json:"run_id"`
	Gate              string                    `json:"gate"`
	TargetKind        string                    `json:"target_kind"`
	TargetName        string                    `json:"target_name"`
	Target            string                    `json:"target"`
	Mode              string                    `json:"mode"`
	Status            string                    `json:"status"`
	Escalated         bool                      `json:"escalated,omitempty"`
	Coverage          []gateCoverageView        `json:"coverage"`
	Sessions          []gateSessionView         `json:"sessions"`
	UncoveredKeys     []string                  `json:"uncovered_keys"`
	NextAction        string                    `json:"next_action"`
	ReviewerBatches   [][]string                `json:"reviewer_batches"`
	SplitRequiredKeys []string                  `json:"split_required_keys"`
	CarriedKeys       []string                  `json:"carried_keys"`
	Relationships     []string                  `json:"relationships"`
	DeferredFindings  []gaterun.DeferredFinding `json:"deferred_findings"`
	Notices           []string                  `json:"notices"`
}

func writeGateJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func gateRunSnapshot(root string, run *gaterun.Run) (gateRunView, error) {
	states, err := gaterun.LoadSessionStates(root, run)
	if err != nil {
		return gateRunView{}, err
	}
	covered, uncovered, err := gaterun.CoverageProgress(run, states)
	if err != nil {
		return gateRunView{}, err
	}
	view := gateRunView{
		SchemaVersion: 3, RunID: run.RunID, Gate: run.Gate,
		TargetKind: run.TargetKind, TargetName: run.TargetName, Target: run.Target,
		Mode: run.Mode, Status: run.Status, Escalated: run.Escalated,
		Coverage: []gateCoverageView{}, Sessions: []gateSessionView{}, UncoveredKeys: []string{},
		ReviewerBatches:   [][]string{},
		SplitRequiredKeys: []string{},
		CarriedKeys:       append([]string{}, run.CarriedKeys...),
		Relationships:     append([]string{}, run.Relationships...),
		DeferredFindings:  append([]gaterun.DeferredFinding{}, run.DeferredFindings...),
		Notices:           append([]string{}, run.Notices...),
	}
	for _, ck := range run.Coverage {
		view.Coverage = append(view.Coverage, gateCoverageView{
			Key: ck.Key, Lens: ck.Lens, Kind: ck.Kind, Source: ck.Source, Task: ck.Task,
			ReportKeys: run.ReportKeys(ck),
			CoveredBy:  covered[ck.Key],
		})
	}
	for _, state := range states {
		entry := gateSessionView{
			SessionID: state.SessionID, Keys: append([]string{}, state.Keys...),
			Status: state.Status, Attempts: len(state.Attempts),
		}
		if len(state.Attempts) > 0 {
			entry.LastRejection = state.Attempts[len(state.Attempts)-1].RejectionReason
		}
		view.Sessions = append(view.Sessions, entry)
	}
	view.UncoveredKeys = uncovered
	view.NextAction = gateNextAction(run, states, uncovered)
	// The computed default reviewer batches are part of the execute next step:
	// the coordinator reads them here rather than deriving its own partition,
	// and the split-required keys name the batches a session refused. A
	// non-execute action has no batches to run, so both stay empty.
	if view.NextAction == "execute" {
		view.ReviewerBatches, view.SplitRequiredKeys = nextStepBatches(run, states, uncovered)
	}
	return view, nil
}

// sessionCounts tallies one run's session states. overCapacity counts sessions
// that declared they cannot carry their assigned batch: they are terminal (no
// re-submission), but their keys stay uncovered until the coordinator splits
// the batch, so they are neither pending nor rejected.
func sessionCounts(states []*gaterun.SessionState) (accepted, pending, rejected, overCapacity int) {
	for _, state := range states {
		switch state.Status {
		case gaterun.SessionAccepted, gaterun.SessionNotRequired:
			accepted++
		case gaterun.SessionRejected:
			rejected++
		case gaterun.SessionOverCapacity:
			overCapacity++
		default:
			pending++
		}
	}
	return accepted, pending, rejected, overCapacity
}

// fmtCoverageRow renders one coverage key's progress line.
func fmtCoverageRow(run *gaterun.Run, ck gaterun.CoverageKey, coveredBy string) string {
	line := "  " + ck.Key
	if ck.Source != "" {
		line += " (" + ck.Source + ")"
	}
	if ck.Lens != "" {
		line += " [" + ck.Lens + "]"
	} else {
		line += " [" + ck.Kind + "]"
	}
	reports := run.ReportKeys(ck)
	if len(reports) > 1 || (len(reports) == 1 && reports[0] != ck.Key) {
		line += " — reports: " + strings.Join(reports, ", ")
	}
	if coveredBy != "" {
		line += " — covered by " + coveredBy
	} else {
		line += " — UNCOVERED"
	}
	return line
}

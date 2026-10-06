package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// runGateStatus reports coverage-run progress. Without --run it lists every
// open run; with --run it reports one run's coverage progress, session states,
// attempts, latest rejection reasons, and the next action. It reads run state
// only and is the recovery point after an interrupted run. In the unscoped
// listing a run whose state cannot be read (a bound record that no longer
// resolves, a damaged session file) degrades to a stale entry so one
// poisoned run cannot hide the others; --run stays strict.
func runGateStatus(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id (omit to list open runs)")
	gatePtr := fs.String("gate", "", "filter: gate name")
	unitPtr := fs.String("unit", "", "filter: unit name")
	ruleIDPtr := fs.String("rule", "", "filter: rule id")
	formatPtr := fs.String("format", "text", "output format: text | json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *formatPtr != "text" && *formatPtr != "json" {
		return fmt.Errorf("invalid --format %q: must be text or json", *formatPtr)
	}

	absRoot := mustAbs(*repoRootPtr)
	runID := strings.TrimSpace(*runIDPtr)
	if runID != "" {
		run, err := gaterun.Load(absRoot, runID)
		if err != nil {
			return err
		}
		if *formatPtr == "json" {
			view, err := gateRunSnapshot(absRoot, run)
			if err != nil {
				return err
			}
			return writeGateJSON(stdout, view)
		}
		return writeRunStatus(stdout, absRoot, run)
	}

	gate := strings.TrimSpace(*gatePtr)
	unitName, err := requireTargetName("unit", *unitPtr)
	if err != nil {
		return err
	}
	ruleID, err := requireTargetName("rule", *ruleIDPtr)
	if err != nil {
		return err
	}
	if unitName != "" && ruleID != "" {
		return errors.New("--unit and --rule are mutually exclusive")
	}
	runs, err := gaterun.ListRuns(absRoot)
	if err != nil {
		return err
	}
	if *formatPtr == "json" {
		views := []gateRunView{}
		for _, run := range runs {
			if run.Status != gaterun.StatusOpen || (gate != "" && run.Gate != gate) || (unitName != "" && !(run.TargetKind == gaterun.TargetKindUnit && run.TargetName == unitName)) || (ruleID != "" && !(run.TargetKind == gaterun.TargetKindRule && run.TargetName == ruleID)) {
				continue
			}
			view, err := gateRunSnapshot(absRoot, run)
			if err != nil {
				views = append(views, staleRunView(run, err))
				continue
			}
			views = append(views, view)
		}
		return writeGateJSON(stdout, views)
	}
	var listed int
	for _, run := range runs {
		if run.Status != gaterun.StatusOpen {
			continue
		}
		if gate != "" && run.Gate != gate {
			continue
		}
		if unitName != "" && !(run.TargetKind == gaterun.TargetKindUnit && run.TargetName == unitName) {
			continue
		}
		if ruleID != "" && !(run.TargetKind == gaterun.TargetKindRule && run.TargetName == ruleID) {
			continue
		}
		states, err := gaterun.LoadSessionStates(absRoot, run)
		if err != nil {
			fmt.Fprintf(stdout, "stale run state: %s — %v\n", run.RunID, err)
			listed++
			continue
		}
		accepted, pending, rejected := sessionCounts(states)
		_, uncovered, err := gaterun.CoverageProgress(run, states)
		if err != nil {
			fmt.Fprintf(stdout, "stale run state: %s — %v\n", run.RunID, err)
			listed++
			continue
		}
		fmt.Fprintf(stdout, "%s · %s@%s · %s · mode %s · coverage %d/%d covered · %d session(s) (%d accepted, %d pending, %d rejected) · created %s\n",
			run.RunID, run.Gate, run.TargetName, run.Target, run.Mode, len(run.Coverage)-len(uncovered), len(run.Coverage), len(states), accepted, pending, rejected, run.CreatedAt)
		listed++
	}
	if listed == 0 {
		fmt.Fprintln(stdout, "No open gate runs.")
	} else {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Run `specflowctl gate-status --run <run_id>` for coverage detail and the next action.")
	}
	return nil
}

// staleRunView degrades one unreadable open run to a stub entry instead of
// failing the whole unscoped listing: a run state whose bound records no
// longer resolve is audit debris, and the recovery point must stay able to
// see the runs that do work.
func staleRunView(run *gaterun.Run, err error) gateRunView {
	return gateRunView{
		SchemaVersion: 2, RunID: run.RunID, Gate: run.Gate,
		TargetKind: run.TargetKind, TargetName: run.TargetName, Target: run.Target,
		Mode: run.Mode, Status: run.Status,
		Coverage: []gateCoverageView{}, Sessions: []gateSessionView{}, UncoveredKeys: []string{},
		NextAction:       "none",
		CarriedKeys:      []string{},
		Relationships:    []string{},
		DeferredFindings: []gaterun.DeferredFinding{},
		Notices:          []string{"stale run state: " + err.Error()},
	}
}

// writeRunStatus prints one run's coverage detail and the next action.
func writeRunStatus(stdout io.Writer, absRoot string, run *gaterun.Run) error {
	states, err := gaterun.LoadSessionStates(absRoot, run)
	if err != nil {
		return err
	}
	covered, uncovered, err := gaterun.CoverageProgress(run, states)
	if err != nil {
		return err
	}
	accepted, pending, rejected := sessionCounts(states)
	fmt.Fprintf(stdout, "Run: %s · %s@%s · %s\n", run.RunID, run.Gate, run.TargetName, run.Target)
	fmt.Fprintf(stdout, "Mode: %s · status: %s · created: %s\n", run.Mode, run.Status, run.CreatedAt)
	fmt.Fprintf(stdout, "Input snapshot: %d ref(s), %d surface path(s), %d file(s)\n", len(run.Refs), len(run.Surfaces), run.EntryCount())
	if len(run.CarriedKeys) > 0 {
		fmt.Fprintf(stdout, "Carried over: %s\n", strings.Join(run.CarriedKeys, ", "))
	}
	fmt.Fprintf(stdout, "Relationships to check: %s\n", strings.Join(run.Relationships, ", "))
	for _, notice := range run.Notices {
		fmt.Fprintf(stdout, "Notice: %s\n", notice)
	}
	fmt.Fprintf(stdout, "Coverage: %d/%d covered\n", len(run.Coverage)-len(uncovered), len(run.Coverage))
	for _, ck := range run.Coverage {
		fmt.Fprintln(stdout, fmtCoverageRow(run, ck, covered[ck.Key]))
	}
	fmt.Fprintf(stdout, "Sessions (%d accepted, %d pending, %d rejected):\n", accepted, pending, rejected)
	for _, state := range states {
		line := fmt.Sprintf("  %s | %s — keys: %s", state.Status, state.SessionID, strings.Join(state.Keys, ", "))
		if n := len(state.Attempts); n > 0 {
			last := state.Attempts[n-1]
			line += fmt.Sprintf(" — attempt %d %s", last.Attempt, last.Status)
			if last.RejectionReason != "" {
				line += " — last rejection: " + last.RejectionReason
			}
			if last.ResultDigest != "" {
				line += " — digest " + last.ResultDigest
			}
		}
		fmt.Fprintln(stdout, line)
	}
	if run.Status != gaterun.StatusOpen {
		if run.Status == gaterun.StatusInvalidated {
			fmt.Fprintln(stdout, "This run was invalidated by a targeted P0/P1 — plan a new run; this state is audit-only.")
		} else {
			fmt.Fprintln(stdout, "This run is consumed (finalized) — the report is audit-only.")
		}
		return nil
	}
	fmt.Fprintf(stdout, "Next: %s\n", gateNextStep(run, states, uncovered))
	return nil
}

// gateNextStep renders the next concrete action for an open run.
func gateNextStep(run *gaterun.Run, states []*gaterun.SessionState, uncovered []string) string {
	switch gateNextAction(run, states, uncovered) {
	case "execute":
		keys := strings.Join(uncovered, ",")
		return fmt.Sprintf("batch these uncovered keys into sessions: `specflowctl gate-mission --run %s --keys %s --format prompt`, then submit each report: `specflowctl gate-submit --run %s --session <id> --keys <keys> --report PATH`", run.RunID, keys, run.RunID)
	case "synthesize":
		return fmt.Sprintf("relationships to check or findings to dispose — generate the final synthesis: `specflowctl gate-mission --run %s --final --format prompt`, then submit it: `specflowctl gate-submit --run %s --session cross --keys cross --report PATH`", run.RunID, run.RunID)
	case "none":
		return "none — this run is audit-only"
	}
	return fmt.Sprintf("all coverage keys are covered — `specflowctl gate-finalize --run %s`", run.RunID)
}

// gateNextAction is shared by JSON status and every text next-step hint.
func gateNextAction(run *gaterun.Run, states []*gaterun.SessionState, uncovered []string) string {
	if run.Status != gaterun.StatusOpen {
		return "none"
	}
	if len(uncovered) > 0 {
		return "execute"
	}
	if run.TargetKind != gaterun.TargetKindRule && (len(run.Relationships) > 0 || runHasFindings(run, states)) {
		for _, state := range states {
			if state.SessionID == gaterun.CrossKey && state.Status == gaterun.SessionAccepted {
				return "finalize"
			}
		}
		return "synthesize"
	}
	return "finalize"
}

// runHasFindings reports whether any accepted non-final session produced a
// finding, the run carries a baseline finding in its carried results, or the
// run has pending deferrals.
func runHasFindings(run *gaterun.Run, states []*gaterun.SessionState) bool {
	for i := range run.CarriedResults {
		if len(resultFindings(&run.CarriedResults[i])) > 0 {
			return true
		}
	}
	if len(run.DeferredFindings) > 0 {
		return true
	}
	for _, state := range states {
		if state.Status != gaterun.SessionAccepted || state.Result == nil {
			continue
		}
		if state.SessionID == gaterun.CrossKey {
			continue
		}
		if len(state.Result.Findings) > 0 {
			return true
		}
	}
	return false
}

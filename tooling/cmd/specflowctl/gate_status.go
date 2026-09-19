package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// runGateStatus reports packet-run progress. Without --run it lists every
// open run; with --run it reports one run's packet states, attempts, latest
// rejection reasons, and the next action. It reads run state only and is the
// recovery point after an interrupted run.
func runGateStatus(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id (omit to list open runs)")
	gatePtr := fs.String("gate", "", "filter: gate name")
	unitPtr := fs.String("unit", "", "filter: unit name")
	ruleIDPtr := fs.String("rule", "", "filter: rule id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot := mustAbs(*repoRootPtr)
	runID := strings.TrimSpace(*runIDPtr)
	if runID != "" {
		run, err := gaterun.Load(absRoot, runID)
		if err != nil {
			return err
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
		accepted, pending, rejected, err := packetCounts(absRoot, run)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s · %s@%s · %s · mode %s · %d packet(s) (%d accepted, %d pending, %d rejected) · created %s\n",
			run.RunID, run.Gate, run.TargetName, run.Target, run.Mode, len(run.Packets), accepted, pending, rejected, run.CreatedAt)
		listed++
	}
	if listed == 0 {
		fmt.Fprintln(stdout, "No open gate runs.")
	} else {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Run `specflowctl gate-status --run <run_id>` for packet detail and the next action.")
	}
	return nil
}

// packetCounts tallies one run's packet states.
func packetCounts(absRoot string, run *gaterun.Run) (accepted, pending, rejected int, err error) {
	for _, spec := range run.Packets {
		state, lerr := gaterun.LoadPacketState(absRoot, run, spec.PacketID)
		if lerr != nil {
			return 0, 0, 0, lerr
		}
		switch state.Status {
		case gaterun.PacketAccepted, gaterun.PacketNotRequired:
			accepted++
		case gaterun.PacketRejected:
			rejected++
		default:
			pending++
		}
	}
	return accepted, pending, rejected, nil
}

// writeRunStatus prints one run's packet detail and the next action.
func writeRunStatus(stdout io.Writer, absRoot string, run *gaterun.Run) error {
	accepted, pending, rejected, err := packetCounts(absRoot, run)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Run: %s · %s@%s · %s\n", run.RunID, run.Gate, run.TargetName, run.Target)
	fmt.Fprintf(stdout, "Mode: %s · status: %s · created: %s\n", run.Mode, run.Status, run.CreatedAt)
	fmt.Fprintf(stdout, "Input snapshot: %d ref(s), %d surface path(s), %d file(s)\n", len(run.Refs), len(run.Surfaces), run.EntryCount())
	if len(run.CarriedKeys) > 0 {
		fmt.Fprintf(stdout, "Carried over: %s\n", strings.Join(run.CarriedKeys, ", "))
	}
	for _, notice := range run.Notices {
		fmt.Fprintf(stdout, "Notice: %s\n", notice)
	}
	fmt.Fprintf(stdout, "Packets (%d accepted, %d pending, %d rejected):\n", accepted, pending, rejected)
	for _, spec := range run.Packets {
		state, lerr := gaterun.LoadPacketState(absRoot, run, spec.PacketID)
		if lerr != nil {
			return lerr
		}
		line := fmt.Sprintf("  %s | %s [%s] — checks: %s", state.Status, spec.PacketID, spec.Kind, strings.Join(spec.CheckKeys, ", "))
		if len(spec.DependsOn) > 0 {
			line += " — depends on: " + strings.Join(spec.DependsOn, ", ")
		}
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
	fmt.Fprintf(stdout, "Next: %s\n", gateNextStep(absRoot, run))
	return nil
}

// gateNextStep renders the next concrete action for an open run.
func gateNextStep(absRoot string, run *gaterun.Run) string {
	for _, spec := range run.Packets {
		state, err := gaterun.LoadPacketState(absRoot, run, spec.PacketID)
		if err != nil {
			return "inspect the run state (a packet state file is unreadable)"
		}
		if state.Status == gaterun.PacketAccepted || state.Status == gaterun.PacketNotRequired {
			continue
		}
		ready := true
		for _, dep := range spec.DependsOn {
			depState, derr := gaterun.LoadPacketState(absRoot, run, dep)
			if derr != nil || (depState.Status != gaterun.PacketAccepted && depState.Status != gaterun.PacketNotRequired) {
				ready = false
				break
			}
		}
		if ready {
			verb := "inspect the packet context, execute it, then submit:"
			if state.Status == gaterun.PacketRejected {
				verb = "inspect the packet context again, fix the rejection reason, then re-submit:"
			}
			return fmt.Sprintf("%s `specflowctl gate-packet --run %s --packet %s`; `specflowctl gate-submit --run %s --packet %s --report PATH`", verb, run.RunID, spec.PacketID, run.RunID, spec.PacketID)
		}
	}
	return fmt.Sprintf("all required packets are resolved — `specflowctl gate-finalize --run %s`", run.RunID)
}

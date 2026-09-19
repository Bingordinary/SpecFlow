package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// runGatePacket materializes the immutable context for one packet executor.
// It is read-only. Dependency results are emitted from persisted state so an
// analysis/cross prompt can consume the exact artifacts later bound by
// gate-submit.
func runGatePacket(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-packet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id printed by gate-plan")
	packetIDPtr := fs.String("packet", "", "packet id from the run plan")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID := strings.TrimSpace(*runIDPtr)
	packetID := strings.TrimSpace(*packetIDPtr)
	if runID == "" || packetID == "" {
		return errors.New("--run and --packet are required")
	}
	absRoot := mustAbs(*repoRootPtr)
	run, err := gaterun.Load(absRoot, runID)
	if err != nil {
		return err
	}
	if run.Status != gaterun.StatusOpen {
		return fmt.Errorf("gate run %s is %s — only an open run can materialize packet context; plan a new run", run.RunID, run.Status)
	}
	spec := run.PacketByID(packetID)
	if spec == nil {
		return fmt.Errorf("packet %q is not part of run %s", packetID, runID)
	}
	state, err := gaterun.LoadPacketState(absRoot, run, packetID)
	if err != nil {
		return err
	}
	if state.Status == gaterun.PacketNotRequired {
		return fmt.Errorf("packet %q is not required for this run", packetID)
	}

	fmt.Fprintf(stdout, "Run: %s\nPacket: %s\nKind: %s\nChecks: %s\n", run.RunID, spec.PacketID, spec.Kind, strings.Join(spec.CheckKeys, ", "))
	fmt.Fprintln(stdout, "Read refs:")
	for _, ref := range spec.ReadRefs {
		fmt.Fprintf(stdout, "  - %s\n", ref)
	}
	if len(spec.DependsOn) > 0 {
		fmt.Fprintln(stdout, "Dependency results:")
		for _, dep := range spec.DependsOn {
			depState, derr := gaterun.LoadPacketState(absRoot, run, dep)
			if derr != nil {
				return derr
			}
			if depState.Status == gaterun.PacketNotRequired {
				fmt.Fprintf(stdout, "  - %s: not_required\n", dep)
				continue
			}
			if depState.Status != gaterun.PacketAccepted || depState.Result == nil {
				return fmt.Errorf("packet %q is not ready: dependency %q is %s", packetID, dep, depState.Status)
			}
			data, _ := json.MarshalIndent(depState.Result, "    ", "  ")
			fmt.Fprintf(stdout, "  - %s (%s):\n    %s\n", dep, depState.Result.ReportDigest, data)
			fmt.Fprintln(stdout, "    Accepted report:")
			fmt.Fprintln(stdout, indentPacketContext(depState.Report, "      "))
		}
	}
	if spec.Kind == gaterun.PacketKindCross && len(run.CarriedResults) > 0 {
		fmt.Fprintln(stdout, "Carried judgments:")
		for _, result := range run.CarriedResults {
			data, _ := json.MarshalIndent(result, "    ", "  ")
			fmt.Fprintf(stdout, "  - %s (%s):\n    %s\n", result.PacketID, result.ReportDigest, data)
		}
	}
	return nil
}

func indentPacketContext(value, prefix string) string {
	value = strings.TrimRight(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	if value == "" {
		return prefix
	}
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}

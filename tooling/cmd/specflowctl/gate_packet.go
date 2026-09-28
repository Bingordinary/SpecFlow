package main

import (
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
	formatPtr := fs.String("format", "prompt", "output format: prompt | json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID := strings.TrimSpace(*runIDPtr)
	packetID := strings.TrimSpace(*packetIDPtr)
	if runID == "" || packetID == "" {
		return errors.New("--run and --packet are required")
	}
	if *formatPtr != "prompt" && *formatPtr != "json" {
		return fmt.Errorf("invalid --format %q: must be prompt or json", *formatPtr)
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
	if state.Status != gaterun.PacketPending && state.Status != gaterun.PacketRejected {
		return fmt.Errorf("packet %q is %s — only pending or rejected packets can receive a mission", packetID, state.Status)
	}
	mission, err := buildGateMission(absRoot, run, spec, state)
	if err != nil {
		return err
	}
	if *formatPtr == "json" {
		return writeGateJSON(stdout, mission)
	}
	writeGatePrompt(stdout, mission)
	return nil
}

// deferredFindingsForPacket selects the pending deferrals a packet executor
// must see: the cross packet receives every pending deferral (its synthesis
// disposes them all); a file packet receives the deferrals whose affected keys
// name the file it reviews.
func deferredFindingsForPacket(run *gaterun.Run, spec *gaterun.PacketSpec) []gaterun.DeferredFinding {
	if len(run.DeferredFindings) == 0 {
		return nil
	}
	switch spec.Kind {
	case gaterun.PacketKindCross:
		return run.DeferredFindings
	case gaterun.PacketKindFile:
		var out []gaterun.DeferredFinding
		for _, deferred := range run.DeferredFindings {
			if deferredCoversKeys(deferred, spec.CheckKeys) {
				out = append(out, deferred)
			}
		}
		return out
	default:
		return nil
	}
}

// deferredCoversKeys reports whether a deferred finding affects one of the
// packet's keys.
func deferredCoversKeys(deferred gaterun.DeferredFinding, keys []string) bool {
	for _, key := range keys {
		if key == deferred.Finding.SourceKey {
			return true
		}
		for _, affected := range deferred.Finding.AffectedKeys {
			if affected == key {
				return true
			}
		}
	}
	return false
}

func indentPacketContext(value, prefix string) string {
	value = strings.TrimRight(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	if value == "" {
		return prefix
	}
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}

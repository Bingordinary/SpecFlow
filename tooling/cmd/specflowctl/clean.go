package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/operation"
)

// runClean removes disposable local state on demand: run directories whose
// target spec no longer exists or whose open state can no longer resume (a
// retired protocol, an invalid shape, or planned evidence that no longer
// resolves), run directories a terminal command concluded but did not remove
// (crash leftovers and states written before terminal removal), shared task
// files no surviving run references, consumed scratch input manifests under
// meta/plan_inputs/, undeclared stray entries under meta/, and closed
// operation states. The same sweep runs automatically after gate-plan and
// after a successful promote or remove; clean additionally removes closed
// operation states, whose lifecycle ends in the close command itself and
// whose surviving files are migration leftovers.
func runClean(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	dryRun := fs.Bool("dry-run", false, "report what would be removed without deleting")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	absRoot := mustAbs(*repoRoot)
	if err := printCleanReport(stdout, absRoot, *dryRun); err != nil {
		return err
	}
	if *dryRun {
		fmt.Fprintln(stdout, "Preview only. Nothing deleted.")
	} else {
		fmt.Fprintln(stdout, "Cleanup complete. Disposable local state removed: orphaned runs, terminal leftovers, unreferenced tasks, consumed plan inputs, strays, and closed operations.")
	}
	return nil
}

// printCleanReport performs one cleanup pass (or previews it) for one
// repository root: the gate-run sweep runs under the gate-run mutation lock,
// the closed-operation sweep under the operation mutation lock, and nothing
// is held across both.
func printCleanReport(stdout io.Writer, absRoot string, dryRun bool) error {
	var sweep gaterun.SweepResult
	var sweptOperations []string
	err := gaterun.WithMutation(absRoot, func() error {
		var err error
		if dryRun {
			sweep, err = gaterun.PlanSweep(absRoot)
		} else {
			sweep, err = gaterun.SweepOrphanedState(absRoot)
		}
		return err
	})
	if err != nil {
		return err
	}

	if dryRun {
		// Preview the closed-operation sweep without deleting, using the
		// sweep's own selection rule: open and unprovable states are never
		// listed, exactly as they are never removed.
		sweptOperations, err = operation.PlanClosedSweep(absRoot)
	} else {
		sweptOperations, err = operation.SweepClosed(absRoot)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "LOCAL STATE CLEANUP — preview: %t\n", dryRun)
	for _, run := range sweep.Runs {
		fmt.Fprintf(stdout, "  Delete run: %s (%s %s %s %s)\n", run.ID, run.Gate, run.TargetKind, run.TargetName, run.Target)
	}
	for _, run := range sweep.Audits {
		fmt.Fprintf(stdout, "  Delete run: %s (%s %s %s %s, concluded)\n", run.ID, run.Gate, run.TargetKind, run.TargetName, run.Target)
	}
	for _, id := range sweep.Tasks {
		fmt.Fprintf(stdout, "  Delete task: %s\n", id)
	}
	for _, name := range sweep.Inputs {
		fmt.Fprintf(stdout, "  Delete plan input: %s\n", name)
	}
	for _, name := range sweep.Strays {
		fmt.Fprintf(stdout, "  Delete stray: %s\n", name)
	}
	for _, id := range sweptOperations {
		fmt.Fprintf(stdout, "  Delete operation: %s\n", id)
	}
	return nil
}

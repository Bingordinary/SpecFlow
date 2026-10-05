package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
)

// runClean removes orphaned local gate state on demand: run directories whose
// target spec no longer exists, shared task files no surviving run references,
// and the scratch input manifests under meta/plan_inputs/. The orphan sweep
// (runs and shared tasks) also runs automatically after gate-plan and after a
// successful promote or remove; only this command clears the plan input
// manifests.
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

	var sweep gaterun.SweepResult
	var inputs []string
	err := gaterun.WithMutation(absRoot, func() error {
		var err error
		if *dryRun {
			sweep, err = gaterun.PlanSweep(absRoot)
		} else {
			sweep, err = gaterun.SweepOrphanedState(absRoot)
		}
		if err != nil {
			return err
		}
		inputs, err = cleanPlanInputs(absRoot, *dryRun)
		return err
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "LOCAL STATE CLEANUP — preview: %t\n", *dryRun)
	for _, run := range sweep.Runs {
		fmt.Fprintf(stdout, "  Delete run: %s (%s %s %s %s)\n", run.ID, run.Gate, run.TargetKind, run.TargetName, run.Target)
	}
	for _, id := range sweep.Tasks {
		fmt.Fprintf(stdout, "  Delete task: %s\n", id)
	}
	for _, name := range inputs {
		fmt.Fprintf(stdout, "  Delete plan input: %s\n", name)
	}
	if *dryRun {
		fmt.Fprintln(stdout, "Preview only. Nothing deleted.")
	} else {
		fmt.Fprintln(stdout, "Cleanup complete. Orphaned runs and unreferenced tasks removed; plan inputs cleared.")
	}
	return nil
}

// cleanPlanInputs lists (dry run) or deletes the scratch input manifests under
// meta/plan_inputs/. The directory holds coordinator scratch only — the run's
// snapshot is the evidence record — so clearing it never removes truth.
func cleanPlanInputs(repoRoot string, dryRun bool) ([]string, error) {
	dir, err := repopath.Canonical(repoRoot, "meta/plan_inputs")
	if err != nil {
		return nil, err
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(dir))
	entries, err := os.ReadDir(abs)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !dryRun {
			if err := os.Remove(filepath.Join(abs, entry.Name())); err != nil {
				return removed, err
			}
		}
		removed = append(removed, entry.Name())
	}
	sort.Strings(removed)
	return removed, nil
}

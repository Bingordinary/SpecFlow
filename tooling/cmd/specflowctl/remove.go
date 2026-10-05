package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/removal"
)

func runRemove(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("repo-root", ".", "repository root")
	layer := fs.String("layer", "all", "all or candidate")
	dryRun := fs.Bool("dry-run", false, "preview the same checks without writing")
	var request removal.Request
	fs.Func("unit", "unit to remove (repeatable)", func(v string) error { request.Units = append(request.Units, v); return nil })
	fs.Func("rule", "rule to remove (repeatable)", func(v string) error { request.Rules = append(request.Rules, v); return nil })
	fs.Func("appendix", "<unit>:<complete filename.md> (repeatable)", func(v string) error { request.Appendices = append(request.Appendices, v); return nil })
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	request.Layer, request.DryRun = *layer, *dryRun
	absRoot := mustAbs(*root)
	result, err := removal.Run(absRoot, request)
	if result != nil {
		fmt.Fprintf(stdout, "SPEC REMOVAL — layer: %s · preview: %t\n", *layer, *dryRun)
		for _, path := range result.Files {
			fmt.Fprintf(stdout, "  Delete: %s\n", path)
		}
		for _, path := range result.Updates {
			fmt.Fprintf(stdout, "  Update: %s\n", path)
		}
		for _, blocker := range result.Blockers {
			fmt.Fprintf(stdout, "  Blocked: %s\n", blocker)
		}
	}
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Fprintln(stdout, "Structural checks: PASS. No files changed.")
	} else {
		fmt.Fprintln(stdout, "Removal complete. Remaining structured references resolve; selected artifacts and current records are cleared.")
		sweepAfterLifecycle(absRoot, stderr)
	}
	return nil
}

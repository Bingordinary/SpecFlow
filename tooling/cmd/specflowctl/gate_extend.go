package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// runGateExtend adds supplementary evidence inputs to an OPEN gate run in
// place. The reviewer protocol uses it when a session reports
// `missing read ref`: the coordinator adds the needed path to the run instead
// of replanning, so the coverage set and every accepted session stay valid
// and only the uncovered key re-runs with the extended read refs (see
// framework/verification_scope.md §Verify evidence discovery before planning).
func runGateExtend(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-extend", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runPtr := fs.String("run", "", "open gate run id")
	pathsPtr := fs.String("paths", "", "comma-separated evidence inputs to add (files, directories, or logical references)")
	var inputsFile string
	inputsFileSet := false
	fs.Func("inputs-file", "input manifest with one extra read input per line (may be combined with --paths)", func(v string) error {
		if inputsFileSet {
			return errors.New("given more than once")
		}
		inputsFileSet = true
		inputsFile = v
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID := strings.TrimSpace(*runPtr)
	if runID == "" {
		writeGateExtendUsage(stderr)
		return errors.New("--run is required")
	}
	absRoot := mustAbs(*repoRootPtr)
	var inputs []string
	if strings.TrimSpace(*pathsPtr) != "" {
		inputs = append(inputs, splitKeys(*pathsPtr)...)
	}
	if inputsFileSet {
		loaded, err := loadInputsManifest(absRoot, inputsFile)
		if err != nil {
			return err
		}
		inputs = append(inputs, loaded...)
	}
	if len(inputs) == 0 {
		writeGateExtendUsage(stderr)
		return errors.New("declare at least one input with --paths or --inputs-file")
	}
	run, err := gaterun.Extend(absRoot, runID, inputs, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Gate run extended: %s\n", run.RunID)
	fmt.Fprintf(stdout, "Input snapshot: %d ref(s), %d surface path(s), %d file(s)\n", len(run.Refs), len(run.Surfaces), run.EntryCount())
	for _, notice := range run.Notices {
		fmt.Fprintf(stdout, "Notice: %s\n", notice)
	}
	fmt.Fprintf(stdout, "Next: re-generate the uncovered keys' missions with `specflowctl gate-mission --run %s --keys <k1,k2,...>` and submit them on the same run; accepted sessions keep their verdicts.\n", run.RunID)
	return nil
}

func writeGateExtendUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl gate-extend --run RUN_ID (--paths PATH[,PATH...] | --inputs-file PATH) [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Adds supplementary evidence inputs to an OPEN gate run in place, under the")
	fmt.Fprintln(w, "repository lock: files become snapshot refs, directories become expanded")
	fmt.Fprintln(w, "surfaces, and logical references stay logical. Coverage keys and accepted")
	fmt.Fprintln(w, "sessions stay fixed — a supplement broadens what sessions may read, not what")
	fmt.Fprintln(w, "their plan required — so only uncovered keys re-run with the extended read")
	fmt.Fprintln(w, "refs. A new `gate-plan` would replace the run instead.")
}

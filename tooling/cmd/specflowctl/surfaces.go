package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

func runSurfaces(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("surfaces", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	asJSON := fs.Bool("json", false, "print the audit as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	report, err := specvalidation.SurfaceAudit(mustAbs(*repoRoot))
	if err != nil {
		return err
	}
	views, err := surfaceJudgments(mustAbs(*repoRoot), report)
	if err != nil {
		return err
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(struct {
			*specvalidation.SurfaceAuditReport
			Judgments []surfaceJudgment `json:"judgments"`
		}{report, views})
	}
	fmt.Fprint(stdout, specvalidation.FormatSurfaceAudit(report))
	printSurfaceJudgments(stdout, views)
	return nil
}

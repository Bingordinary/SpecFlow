package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
)

// runToolingFingerprint computes the live tooling source fingerprint from
// disk (not the embedded build fingerprint). Release scripts call this via
// `go run` so the fingerprint always reflects the current working tree.
func runToolingFingerprint(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tooling-fingerprint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	short := fs.Bool("short", false, "print only the first 12 characters")
	if err := fs.Parse(args); err != nil {
		return err
	}

	fingerprint, _, err := toolingfreshness.LiveFingerprint(mustAbs(*repoRoot))
	if err != nil {
		return err
	}
	if *short {
		fingerprint = fingerprint[:12]
	}
	fmt.Fprintln(stdout, fingerprint)
	return nil
}

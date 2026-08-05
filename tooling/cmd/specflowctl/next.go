package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/next"
)

func runNext(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	unitPtr := fs.String("unit", "", "unit name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	unitName := strings.TrimSpace(*unitPtr)
	if unitName == "" {
		fmt.Fprintln(stdout, "Usage: specflowctl next --unit <name>")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Discovers the unit's spec files, appendices, rules, and related units.")
		fmt.Fprintln(stdout, "No lifecycle state is read or advanced — file existence is state.")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Flags:")
		fmt.Fprintln(stdout, "  --unit NAME      Unit name to discover")
		fmt.Fprintln(stdout, "  --repo-root PATH Repository root path (default: .)")
		return nil
	}

	absRoot, err := filepath.Abs(*repoRootPtr)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	info, err := next.DiscoverUnit(absRoot, unitName)
	if err != nil {
		return fmt.Errorf("discover unit: %w", err)
	}

	_, err = fmt.Fprint(stdout, next.FormatInfo(info))
	return err
}

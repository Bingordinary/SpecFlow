package main

import (
	"errors"
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
		fmt.Fprintln(stderr, "Usage: specflowctl next --unit <name>")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Discovers the unit's spec files, appendices, rules, and related units.")
		fmt.Fprintln(stderr, "No lifecycle state is read or advanced — file existence is state.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Flags:")
		fmt.Fprintln(stderr, "  --unit NAME      Unit name to discover")
		fmt.Fprintln(stderr, "  --repo-root PATH Repository root path (default: .)")
		return errors.New("missing --unit flag")
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

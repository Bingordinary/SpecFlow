package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fork"
)

func runFork(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fork", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	unitPtr := fs.String("unit", "", "unit name")
	ruleIDPtr := fs.String("rule", "", "rule id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	unitName := strings.TrimSpace(*unitPtr)
	ruleID := strings.TrimSpace(*ruleIDPtr)

	if unitName == "" && ruleID == "" {
		fmt.Fprintln(stderr, "Usage: specflowctl fork (--unit <name> | --rule <id>) [--repo-root PATH]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Copy a stable spec/rule (and associated appendix files) to the candidate layer.")
		fmt.Fprintln(stderr, "Copies content verbatim (the layer is encoded by the path), increments the version, and reports the complete fork manifest.")
		fmt.Fprintln(stderr, "This is the only allowed way to fork. Manual cp is not permitted.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Flags:")
		fmt.Fprintln(stderr, "  --unit NAME      Unit name to fork")
		fmt.Fprintln(stderr, "  --rule ID        Rule id to fork")
		fmt.Fprintln(stderr, "  --repo-root PATH Repository root path (default: .)")
		return errors.New("missing --unit or --rule flag")
	}

	if unitName != "" && ruleID != "" {
		return errors.New("--unit and --rule are mutually exclusive")
	}

	absRoot, err := filepath.Abs(*repoRootPtr)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	if ruleID != "" {
		result := fork.ForkRule(absRoot, ruleID)
		_, err = fmt.Fprint(stdout, fork.FormatRuleResult(result))
		if err != nil {
			return err
		}
		if !result.Passed {
			return errors.New("fork failed")
		}
		return nil
	}

	result := fork.Fork(absRoot, unitName)
	_, err = fmt.Fprint(stdout, fork.FormatResult(result))
	if err != nil {
		return err
	}
	if !result.Passed {
		return errors.New("fork failed")
	}

	return nil
}

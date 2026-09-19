package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulerefs"
)

func runConsumers(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("consumers", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	ruleID := fs.String("rule", "", "rule id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ruleIDValue, err := requireTargetName("rule", *ruleID)
	if err != nil {
		return err
	}
	if ruleIDValue == "" {
		fmt.Fprintln(stderr, "Usage: specflowctl consumers --rule RULE_ID [--repo-root PATH]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Lists all units that reference the given rule in their rule_refs.")
		fmt.Fprintln(stderr, "For global rules (g_rule_*): returns every current-layer unit.")
		fmt.Fprintln(stderr, "For bound rules (b_rule_*): empty output means no consumers.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Flags:")
		fmt.Fprintln(stderr, "  --rule RULE_ID  Rule id to find consumers for (required)")
		fmt.Fprintln(stderr, "  --repo-root PATH Repository root directory (default: current directory)")
		return errors.New("--rule is required")
	}

	absRoot, err := filepath.Abs(*repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	consumers, err := rulerefs.FindRuleConsumers(absRoot, ruleIDValue)
	if err != nil {
		return fmt.Errorf("find consumers: %w", err)
	}

	if len(consumers) == 0 {
		fmt.Fprintf(stdout, "No consumers found for rule %q.\n", ruleIDValue)
		return nil
	}

	fmt.Fprintf(stdout, "Consumers of %q (%d):\n", ruleIDValue, len(consumers))
	for _, c := range consumers {
		fmt.Fprintf(stdout, "  - %s\n", c)
	}

	return nil
}

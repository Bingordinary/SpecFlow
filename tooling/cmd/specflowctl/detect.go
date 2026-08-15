package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/ruledetect"
)

func runDetect(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("detect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	ruleIDPtr := fs.String("rule", "", "rule id for single-rule detection")
	allPtr := fs.Bool("all", false, "scan every bound rule")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ruleID := strings.TrimSpace(*ruleIDPtr)

	if ruleID != "" && *allPtr {
		writeDetectUsage(stderr)
		return errors.New("--rule and --all are mutually exclusive")
	}
	if ruleID == "" && !*allPtr {
		writeDetectUsage(stderr)
		return errors.New("--rule or --all is required")
	}

	absRoot := mustAbs(*repoRootPtr)

	if ruleID != "" {
		result, err := ruledetect.DetectRule(absRoot, ruleID)
		if err != nil {
			return err
		}
		return writeRuleDetection(stdout, result)
	}

	results, err := ruledetect.DetectAll(absRoot)
	if err != nil {
		return err
	}
	return writeAllDetections(stdout, results)
}

func writeRuleDetection(stdout io.Writer, r *ruledetect.DetectResult) error {
	fmt.Fprintf(stdout, "RULE DETECTION — %s\n", r.RuleID)
	fmt.Fprintln(stdout)
	if len(r.Consumers) == 0 {
		fmt.Fprintln(stdout, "Consumers: none")
	} else {
		fmt.Fprintf(stdout, "Consumers (%d):\n", len(r.Consumers))
		for _, c := range r.Consumers {
			fmt.Fprintf(stdout, "  - %s\n", c)
		}
	}
	fmt.Fprintf(stdout, "Unbound retention: %s\n", yesNo(r.UnboundRetention))
	fmt.Fprintf(stdout, "Removable: %s\n", yesNo(r.Removable))
	return nil
}

func writeAllDetections(stdout io.Writer, results []ruledetect.DetectResult) error {
	if len(results) == 0 {
		fmt.Fprintln(stdout, "No bound rules found.")
		return nil
	}
	fmt.Fprintln(stdout, "UNBOUND RULE DETECTION (b_rule_*)")
	fmt.Fprintln(stdout)
	removable := 0
	for _, r := range results {
		if r.Removable {
			removable++
		}
		consumerLabel := fmt.Sprintf("%d", len(r.Consumers))
		if len(r.Consumers) > 0 {
			consumerLabel += " (" + strings.Join(r.Consumers, ", ") + ")"
		}
		fmt.Fprintf(stdout, "  %-40s consumers: %-24s retention: %-3s removable: %t\n",
			r.RuleID, consumerLabel, yesNo(r.UnboundRetention), r.Removable)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "REMOVABLE: %d of %d\n", removable, len(results))
	fmt.Fprintln(stdout, "Removal is never automatic — run `specflowctl remove --rule <id>` to delete.")
	return nil
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func writeDetectUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl detect --rule RULE_ID [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl detect --all [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Read-only detection of rule removal readiness: reports the current-layer")
	fmt.Fprintln(w, "(effective) consumers of a rule — candidate preferred, stable fallback,")
	fmt.Fprintln(w, "same resolution as `deps` — and whether the rule declares")
	fmt.Fprintln(w, "unbound_retention (intentional retention, exempt from removal). A rule is")
	fmt.Fprintln(w, "removable when it has no consumers and no retention declaration.")
	fmt.Fprintln(w, "`--all` lists every bound rule (b_rule_*) only; global rules (g_rule_*)")
	fmt.Fprintln(w, "are never listed — they apply to every unit by default and are removed")
	fmt.Fprintln(w, "only by an explicit `remove --rule`.")
	fmt.Fprintln(w, "Pure read-only: never writes or deletes files.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --rule RULE_ID  Rule id to detect (single rule; global rules report explicit consumers only)")
	fmt.Fprintln(w, "  --all           Scan every bound rule (candidate and stable layers)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

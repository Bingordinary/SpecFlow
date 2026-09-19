package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// runGateInvalidate persists a targeted P0/P1 contradiction without
// publishing the targeted report as a complete gate result. The gaterun
// module performs the cache transition and invalidates matching open plans
// under one repository mutation lock.
func runGateInvalidate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-invalidate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	gatePtr := fs.String("gate", "", "gate name: validate | verify | review")
	unitPtr := fs.String("unit", "", "unit name")
	ruleIDPtr := fs.String("rule", "", "rule id")
	targetPtr := fs.String("target", "", "layer checked: candidate | stable")
	checks := repeatedString{}
	fs.Var(&checks, "check", "targeted check key contradicted by P0/P1 (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	gate := strings.TrimSpace(*gatePtr)
	unitName := strings.TrimSpace(*unitPtr)
	ruleID := strings.TrimSpace(*ruleIDPtr)
	target := strings.TrimSpace(*targetPtr)
	if err := validateGateInvalidateTarget(gate, unitName, ruleID, target); err != nil {
		writeGateInvalidateUsage(stderr)
		return err
	}
	if len(checks) == 0 {
		writeGateInvalidateUsage(stderr)
		return errors.New("at least one --check is required")
	}

	targetKind := gaterun.TargetKindUnit
	targetName := unitName
	if ruleID != "" {
		targetKind = gaterun.TargetKindRule
		targetName = ruleID
	}
	result, err := gaterun.InvalidateTargeted(mustAbs(*repoRootPtr), gate, targetKind, targetName, target, checks)
	if err != nil {
		return err
	}

	switch result.Cache.Action {
	case validationcache.InvalidationNoCache:
		fmt.Fprintf(stdout, "No cache to invalidate: %s (a complete run is still required)\n", result.Cache.Path)
	case validationcache.InvalidationCacheDeleted:
		fmt.Fprintf(stdout, "Pass cache deleted after targeted P0/P1: %s\n", result.Cache.Path)
	case validationcache.InvalidationRecordUpdated:
		if len(result.Cache.Added) == 0 {
			fmt.Fprintf(stdout, "Failure record already carries the targeted invalidation(s): %s\n", result.Cache.Path)
		} else {
			fmt.Fprintf(stdout, "Failure record invalidated: %s\n", result.Cache.Path)
			fmt.Fprintf(stdout, "Persisted check(s): %s\n", strings.Join(result.Cache.Added, ", "))
		}
	default:
		return fmt.Errorf("unknown targeted invalidation action %q", result.Cache.Action)
	}
	if len(result.InvalidatedRunIDs) > 0 {
		fmt.Fprintf(stdout, "Invalidated open gate run(s): %s\n", strings.Join(result.InvalidatedRunIDs, ", "))
	}
	fmt.Fprintln(stdout, "Next: resolve the findings, then plan a new full or repair run; repair reads persisted invalidations automatically.")
	return nil
}

func validateGateInvalidateTarget(gate, unitName, ruleID, target string) error {
	switch gate {
	case gaterun.GateValidate, gaterun.GateVerify, gaterun.GateReview:
	default:
		return fmt.Errorf("invalid --gate %q: must be validate, verify, or review", gate)
	}
	if (unitName == "") == (ruleID == "") {
		return errors.New("exactly one of --unit or --rule is required")
	}
	if ruleID != "" && gate != gaterun.GateValidate {
		return fmt.Errorf("rule targets support the validate gate only (rule verify and review have been removed) — got %q", gate)
	}
	if target != gaterun.TargetCandidate && target != gaterun.TargetStable {
		return fmt.Errorf("invalid --target %q: must be candidate or stable", target)
	}
	return nil
}

func writeGateInvalidateUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl gate-invalidate --gate validate|verify|review (--unit NAME | --rule ID) --target candidate|stable --check CHECK_KEY [--check CHECK_KEY]... [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Records a targeted P0/P1 without publishing a targeted result cache.")
	fmt.Fprintln(w, "Deletes a pass cache, or persists the contradicted key(s) on a failure")
	fmt.Fprintln(w, "record, and invalidates any matching open gate run.")
}

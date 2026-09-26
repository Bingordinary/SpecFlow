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

// runGatePlan fixes the immutable input snapshot and the deterministic packet
// plan for a quality-gate run before any executor reads input. The tooling
// resolves the gate's protocol input surface for the target (the target's
// spec files, the dependency spec objects, and — for verify/review — the
// declared code surface), adds the agent-declared --input entries, generates
// the packet set for the run mode, prints the plan, and persists the run
// state under meta/gate_runs/. Every packet report is recorded by
// gate-submit; gate-finalize writes the cache only when the whole plan is
// accepted and the inputs are unchanged. See framework/verification_scope.md
// §Gate Work Packets and framework/validation_cache.md §Write Rules → Tooled
// writes for the contract.
func runGatePlan(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	gatePtr := fs.String("gate", "", "gate name: validate | verify | review")
	unitPtr := fs.String("unit", "", "unit name")
	ruleIDPtr := fs.String("rule", "", "rule id")
	targetPtr := fs.String("target", "", "layer checked: candidate | stable")
	modePtr := fs.String("mode", "full", "run mode: full | delta | repair")
	inputPtr := repeatedString{}
	fs.Var(&inputPtr, "input", "extra read input the derived surface cannot see: a path, a directory, or a logical reference (repeatable)")
	rerunPtr := repeatedString{}
	fs.Var(&rerunPtr, "rerun", "delta/repair only: force a check key into the re-run set (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	gate := strings.TrimSpace(*gatePtr)
	unitName := strings.TrimSpace(*unitPtr)
	ruleID := strings.TrimSpace(*ruleIDPtr)
	target := strings.TrimSpace(*targetPtr)
	mode := strings.TrimSpace(*modePtr)

	if err := requireGateTarget(gate, unitName, ruleID, target, stderr); err != nil {
		return err
	}
	switch mode {
	case gaterun.ModeFull, gaterun.ModeDelta, gaterun.ModeRepair:
	default:
		writeGatePlanUsage(stderr)
		return fmt.Errorf("invalid --mode %q: must be full, delta, or repair", mode)
	}

	targetKind := "unit"
	targetName := unitName
	if ruleID != "" {
		targetKind = "rule"
		targetName = ruleID
	}

	absRoot := mustAbs(*repoRootPtr)
	run, err := gaterun.Plan(absRoot, gate, targetKind, targetName, target, mode, inputPtr, rerunPtr, time.Now().UTC())
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Gate run planned: %s\n", run.RunID)
	fmt.Fprintf(stdout, "Gate: %s@%s · %s · mode %s\n", run.Gate, run.TargetName, run.Target, run.Mode)
	fmt.Fprintf(stdout, "Input snapshot: %d ref(s), %d surface path(s), %d file(s)\n", len(run.Refs), len(run.Surfaces), run.EntryCount())
	fmt.Fprintf(stdout, "Packets (%d):\n", len(run.Packets))
	for _, p := range run.Packets {
		line := fmt.Sprintf("  %s [%s] — checks: %s", p.PacketID, p.Kind, strings.Join(p.CheckKeys, ", "))
		if len(p.DependsOn) > 0 {
			line += " — depends on: " + strings.Join(p.DependsOn, ", ")
		}
		fmt.Fprintln(stdout, line)
	}
	if len(run.CarriedKeys) > 0 {
		fmt.Fprintf(stdout, "Carried over: %s\n", strings.Join(run.CarriedKeys, ", "))
	}
	if len(run.DeferredFindings) > 0 {
		fmt.Fprintf(stdout, "Pending deferred findings (%d) — this review must dispose them in the cross synthesis:\n", len(run.DeferredFindings))
		for _, deferred := range run.DeferredFindings {
			fmt.Fprintf(stdout, "  [%s] %s — from %s run %s: %s\n",
				deferred.Finding.Severity, deferred.Finding.ID, deferred.SourceUnit, deferred.SourceRun, deferred.Finding.Text)
		}
	}
	for _, notice := range run.Notices {
		fmt.Fprintf(stdout, "Notice: %s\n", notice)
	}
	fmt.Fprintf(stdout, "Run state: %s\n", gaterun.StateRelPath(run.RunID))
	fmt.Fprintf(stdout, "Next: inspect each ready packet with `specflowctl gate-packet --run %s --packet <id>`, submit its report, then run `specflowctl gate-finalize --run %s` after all required packets resolve\n", run.RunID, run.RunID)
	return nil
}

// requireGateTarget validates the gate/target combination and prints usage
// on invalid input. Rule targets have validate only (rule verify and review
// have been removed).
func requireGateTarget(gate, unitName, ruleID, target string, stderr io.Writer) error {
	switch gate {
	case "validate", "verify", "review":
	default:
		writeGatePlanUsage(stderr)
		return fmt.Errorf("invalid --gate %q: must be validate, verify, or review", gate)
	}
	if unitName != "" && ruleID != "" {
		writeGatePlanUsage(stderr)
		return errors.New("--unit and --rule are mutually exclusive")
	}
	if unitName == "" && ruleID == "" {
		writeGatePlanUsage(stderr)
		return errors.New("--unit or --rule is required")
	}
	if target != "candidate" && target != "stable" {
		writeGatePlanUsage(stderr)
		return fmt.Errorf("invalid --target %q: must be candidate or stable", target)
	}
	if ruleID != "" && gate != "validate" {
		return fmt.Errorf("rule targets support the validate gate only (rule verify and review have been removed) — got %q", gate)
	}
	return nil
}

func writeGatePlanUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl gate-plan --gate validate|verify|review (--unit NAME | --rule ID) --target candidate|stable [--mode full|delta|repair] [--input PATH_OR_REF]... [--rerun CHECK_KEY]... [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Fixes the immutable input snapshot and the deterministic packet plan for a")
	fmt.Fprintln(w, "quality-gate run before any executor reads input. The tooling resolves the")
	fmt.Fprintln(w, "gate's protocol input surface for the target (the target's spec files, the")
	fmt.Fprintln(w, "dependency spec objects, and — for verify/review — the declared code surface),")
	fmt.Fprintln(w, "records each input's path, current-layer resolution, and content hash, generates")
	fmt.Fprintln(w, "the packet set for the run mode, prints the plan, and writes the run state to")
	fmt.Fprintln(w, "meta/gate_runs/{run_id}/ (local process state, not committed). One open run")
	fmt.Fprintln(w, "exists per (gate, target, layer); a new plan replaces the previous run.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Every file a packet report may declare must be inside the snapshot. The derived")
	fmt.Fprintln(w, "surface covers the gate's protocol inputs; --input (repeatable) adds evidence")
	fmt.Fprintln(w, "available to packets but never creates work packets. A declaration outside a")
	fmt.Fprintln(w, "packet's read refs is rejected by gate-submit.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Modes: full (every packet), delta (re-run set derived from a pass baseline's")
	fmt.Fprintln(w, "stale evidence), repair (re-run set derived from a failure record's status map).")
	fmt.Fprintln(w, "--rerun explicitly forces a check key (validate: 1-8; verify: item id; review: file path)")
	fmt.Fprintln(w, "into the delta/repair re-run set. Targeted P0/P1 findings are persisted by")
	fmt.Fprintln(w, "gate-invalidate and included in repair automatically; they do not rely on --rerun.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Verify/review plans validate the declared code surface: every acceptance item's")
	fmt.Fprintln(w, "implementation_surface must be the exact <pending> design-first placeholder or")
	fmt.Fprintln(w, "a single path resolving to at least one real file (a directory expands to its")
	fmt.Fprintln(w, "repository-content files: the files Git tracks plus untracked files that are")
	fmt.Fprintln(w, "not ignored). Semicolon lists, wildcard patterns, nonexistent paths, and")
	fmt.Fprintln(w, "directories with no repository-content files are rejected with the item id and")
	fmt.Fprintln(w, "the reason. A spec with an empty acceptance item set is rejected as well — a")
	fmt.Fprintln(w, "verify/review run has no verifiable object without at least one item.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --gate GATE      validate | verify | review (required)")
	fmt.Fprintln(w, "  --unit NAME      Unit name (mutually exclusive with --rule)")
	fmt.Fprintln(w, "  --rule ID        Rule id (validate only)")
	fmt.Fprintln(w, "  --target T       candidate | stable (required; stable = stable-only target)")
	fmt.Fprintln(w, "  --mode M         full | delta | repair (default: full)")
	fmt.Fprintln(w, "  --input REF      extra read input: path, directory, or logical reference")
	fmt.Fprintln(w, "                   (unit:{name} / unit:{name}:appendix:{file} / rule:{id}); repeatable")
	fmt.Fprintln(w, "  --rerun KEY      delta/repair only: explicit additional re-run override; repeatable")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

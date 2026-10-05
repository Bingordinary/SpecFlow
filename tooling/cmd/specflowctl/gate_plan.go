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

// runGatePlan fixes the immutable input snapshot and computes the coverage set
// for a quality-gate run before any executor reads input. The tooling resolves
// the gate's protocol input surface for the target (the target's spec files,
// the dependency spec objects, and — for verify — the declared code
// surface), adds the entries listed in the agent-declared input manifest
// (--inputs-file), computes the coverage set
// for the run mode, prints it, and persists the run state under meta/gate_runs/
// with no sessions. The agent partitions the coverage set into reviewer
// sessions, generates each mission with gate-mission, and records each report
// with gate-submit; gate-finalize writes the cache only when every coverage key
// is covered and the inputs are unchanged. See framework/verification_scope.md
// §Coverage Model and framework/validation_cache.md §Write Rules → Tooled
// writes for the contract.
func runGatePlan(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	gatePtr := fs.String("gate", "", "gate name: validate | verify")
	unitPtr := fs.String("unit", "", "unit name")
	ruleIDPtr := fs.String("rule", "", "rule id")
	targetPtr := fs.String("target", "", "layer checked: candidate | stable")
	modePtr := fs.String("mode", "full", "run mode: full | delta | repair")
	formatPtr := fs.String("format", "text", "output format: text | json")
	relationshipsPtr := fs.String("relationships", "", "relationships touched by this change: comma-separated names, or none (required for units)")
	var inputsFile string
	inputsFileSet := false
	fs.Func("inputs-file", "input manifest: a plain text file with one extra read input per line — a path, a directory, or a logical reference (single use)", func(v string) error {
		if inputsFileSet {
			return errors.New("given more than once")
		}
		inputsFileSet = true
		inputsFile = v
		return nil
	})
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
	if *formatPtr != "text" && *formatPtr != "json" {
		return fmt.Errorf("invalid --format %q: must be text or json", *formatPtr)
	}

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
	var relationships []string
	if targetKind == gaterun.TargetKindUnit {
		if strings.TrimSpace(*relationshipsPtr) == "" {
			return errors.New("--relationships is required for unit runs: declare the relationships touched by this change, or none")
		}
		if strings.TrimSpace(*relationshipsPtr) != "none" {
			relationships = splitKeys(*relationshipsPtr)
			if len(relationships) == 0 {
				return errors.New("--relationships must name relationships or explicitly declare none")
			}
		}
	} else if *relationshipsPtr != "" {
		return errors.New("--relationships applies to unit runs only")
	}

	absRoot := mustAbs(*repoRootPtr)
	var extraInputs []string
	if inputsFileSet {
		loaded, err := loadInputsManifest(absRoot, inputsFile)
		if err != nil {
			return err
		}
		extraInputs = loaded
	}
	run, err := gaterun.Plan(absRoot, gate, targetKind, targetName, target, mode, extraInputs, rerunPtr, relationships, time.Now().UTC())
	if err != nil {
		return err
	}
	if *formatPtr == "json" {
		view, err := gateRunSnapshot(absRoot, run)
		if err != nil {
			return err
		}
		return writeGateJSON(stdout, view)
	}

	fmt.Fprintf(stdout, "Gate run planned: %s\n", run.RunID)
	fmt.Fprintf(stdout, "Gate: %s@%s · %s · mode %s\n", run.Gate, run.TargetName, run.Target, run.Mode)
	fmt.Fprintf(stdout, "Input snapshot: %d ref(s), %d surface path(s), %d file(s)\n", len(run.Refs), len(run.Surfaces), run.EntryCount())
	fmt.Fprintf(stdout, "Coverage (%d key(s)):\n", len(run.Coverage))
	for _, ck := range run.Coverage {
		line := fmt.Sprintf("  %s", ck.Key)
		if ck.Lens != "" {
			line += " [" + ck.Lens + "]"
		} else {
			line += " [" + ck.Kind + "]"
		}
		if reports := run.ReportKeys(ck); len(reports) > 0 && strings.Join(reports, ",") != ck.Key {
			line += " — reports: " + strings.Join(reports, ", ")
		}
		fmt.Fprintln(stdout, line)
	}
	if len(run.CarriedKeys) > 0 {
		fmt.Fprintf(stdout, "Carried over: %s\n", strings.Join(run.CarriedKeys, ", "))
	}
	fmt.Fprintf(stdout, "Relationships to check: %s\n", strings.Join(run.Relationships, ", "))
	if len(run.DeferredFindings) > 0 {
		fmt.Fprintf(stdout, "Pending deferred findings (%d) — the final synthesis must dispose them:\n", len(run.DeferredFindings))
		for _, deferred := range run.DeferredFindings {
			fmt.Fprintf(stdout, "  [%s] %s — from %s run %s: %s\n",
				deferred.Finding.Severity, deferred.Finding.ID, deferred.SourceUnit, deferred.SourceRun, deferred.Finding.Text)
		}
	}
	for _, notice := range run.Notices {
		fmt.Fprintf(stdout, "Notice: %s\n", notice)
	}
	fmt.Fprintf(stdout, "Run state: %s\n", gaterun.StateRelPath(run.RunID))
	fmt.Fprintf(stdout, "Next: choose the local key batches, generate each session mission with `specflowctl gate-mission --run %s --keys <k1,k2,...> --format prompt`, send it to an independent reviewer, submit its report with `specflowctl gate-submit --run %s --session <id> --keys <k1,k2,...> --report PATH`. Follow gate-status: selected relationships or existing findings require `gate-mission --run %s --final` before finalization.\n", run.RunID, run.RunID, run.RunID)
	return nil
}

// requireGateTarget validates the gate/target combination and prints usage
// on invalid input. Rule targets have validate only (rule verify has been
// removed); unit verify carries the alignment and quality lenses.
func requireGateTarget(gate, unitName, ruleID, target string, stderr io.Writer) error {
	switch gate {
	case "validate", "verify":
	default:
		writeGatePlanUsage(stderr)
		return fmt.Errorf("invalid --gate %q: must be validate or verify", gate)
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
		return fmt.Errorf("rule targets support the validate gate only (rule verify has been removed) — got %q", gate)
	}
	return nil
}

func writeGatePlanUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl gate-plan --gate validate|verify (--unit NAME --relationships NAMES|none | --rule ID) --target candidate|stable [--mode full|delta|repair] [--inputs-file PATH] [--rerun CHECK_KEY]... [--format text|json] [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Fixes the immutable input snapshot and computes the coverage set for a")
	fmt.Fprintln(w, "quality-gate run before any executor reads input. The tooling resolves the")
	fmt.Fprintln(w, "gate's protocol input surface for the target (the target's spec files, the")
	fmt.Fprintln(w, "dependency spec objects, and — for verify — the declared code surface),")
	fmt.Fprintln(w, "records each input's path, current-layer resolution, and content hash, computes")
	fmt.Fprintln(w, "the coverage set for the run mode, prints it, and writes the run state to")
	fmt.Fprintln(w, "meta/gate_runs/{run_id}/ with no sessions (local process state, not committed).")
	fmt.Fprintln(w, "One open run exists per (gate, target, layer); a new plan replaces the previous")
	fmt.Fprintln(w, "run.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Every file a session report may declare must be inside the snapshot. The derived")
	fmt.Fprintln(w, "surface covers the gate's protocol inputs; --inputs-file lists extra evidence that")
	fmt.Fprintln(w, "sessions may read and declare but that never creates coverage keys. The manifest is")
	fmt.Fprintln(w, "a plain text file with one path, directory, or logical reference per line; blank")
	fmt.Fprintln(w, "lines are skipped and entries are used verbatim. It is scratch input, not evidence:")
	fmt.Fprintln(w, "keep it under an ignored local-state path such as meta/plan_inputs/ or outside the")
	fmt.Fprintln(w, "repository — a manifest that is a repository-content file is rejected before the")
	fmt.Fprintln(w, "plan is fixed. A declaration outside a session's read refs is rejected by")
	fmt.Fprintln(w, "gate-submit.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Modes: full (the complete coverage set), delta (coverage keys derived from a pass")
	fmt.Fprintln(w, "baseline's stale evidence), repair (coverage keys derived from a failure record's")
	fmt.Fprintln(w, "status map). --rerun explicitly forces a check key (validate: 1-10; verify:")
	fmt.Fprintln(w, "acceptance item id or code file path) into the delta/repair re-run set. Targeted")
	fmt.Fprintln(w, "P0/P1 findings are persisted by gate-invalidate and included in repair automatically;")
	fmt.Fprintln(w, "they do not rely on --rerun.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "A verify plan's coverage set is the union of the alignment lens (acceptance item ids)")
	fmt.Fprintln(w, "and the quality lens (declared code files). A session's keys must all come from one")
	fmt.Fprintln(w, "lens. Verify plans validate the declared code surface: every acceptance item's")
	fmt.Fprintln(w, "implementation_surface must be the exact <pending> design-first placeholder or")
	fmt.Fprintln(w, "a single path resolving to at least one real file (a directory expands to its")
	fmt.Fprintln(w, "repository-content files: the files Git tracks plus untracked files that are")
	fmt.Fprintln(w, "not ignored). Semicolon lists, wildcard patterns, nonexistent paths, and")
	fmt.Fprintln(w, "directories with no repository-content files are rejected with the item id and")
	fmt.Fprintln(w, "the reason. A spec with an empty acceptance item set is rejected as well — a")
	fmt.Fprintln(w, "verify run has no alignment object without at least one item.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --gate GATE      validate | verify (required)")
	fmt.Fprintln(w, "  --unit NAME      Unit name (mutually exclusive with --rule)")
	fmt.Fprintln(w, "  --rule ID        Rule id (validate only)")
	fmt.Fprintln(w, "  --target T       candidate | stable (required; stable = stable-only target)")
	fmt.Fprintln(w, "  --mode M         full | delta | repair (default: full)")
	fmt.Fprintln(w, "  --inputs-file PATH  input manifest: one extra read input per line — a path, a")
	fmt.Fprintln(w, "                   directory, or a logical reference (unit:{name} /")
	fmt.Fprintln(w, "                   unit:{name}:appendix:{file} / rule:{id}); the file must not be")
	fmt.Fprintln(w, "                   a repository-content file")
	fmt.Fprintln(w, "  --rerun KEY      delta/repair only: explicit additional re-run override; repeatable")
	fmt.Fprintln(w, "  --relationships NAMES|none  required for units: relationships touched by the current change")
	fmt.Fprintln(w, "                   validate: design_constraints, coverage_scope, cross_unit_cohesion")
	fmt.Fprintln(w, "                   verify: contract_consistency, data_definition_drift, state_machine_coherence,")
	fmt.Fprintln(w, "                   error_code_conflict, cross_reference_integrity")
	fmt.Fprintln(w, "  --format F       text | json (default: text)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

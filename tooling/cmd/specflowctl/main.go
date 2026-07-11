package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/buildrelease"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/install"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/promote"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repositorymapping"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewrun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewscope"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulesync"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := toolingfreshness.CheckProcess(args, cwd); err != nil {
		return err
	}

	if len(args) == 0 {
		writeRootUsage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case toolingfreshness.HiddenBuildFingerprintCommand:
		fmt.Fprintln(stdout, toolingfreshness.PrintBuildFingerprint())
		return nil
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "build-release":
		return runBuildRelease(args[1:], stdout, stderr)
	case "migrate":
		return runMigrate(args[1:], stdout, stderr)
	case "next":
		return runNext(args[1:], stdout, stderr)
	case "promote":
		return runPromote(args[1:], stdout, stderr)
	case "review":
		return runReview(args[1:], stdout, stderr)
	case "rule":
		return runRule(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "repository-mapping":
		fmt.Fprintln(stderr, "Warning: 'repository-mapping' is deprecated and may be removed in a future version")
		return runRepositoryMapping(args[1:], stdout, stderr)
	case "command", "evaluation", "process", "snapshot", "status", "check-report", "relation":
		fmt.Fprintf(stderr, "'%s' is no longer supported in this version of specFlow\n", args[0])
		fmt.Fprintln(stderr, "See specflow/framework/concepts.md for the current framework design")
		return errors.New("removed command")
	case "unit":
		fmt.Fprintf(stderr, "'unit' is deprecated. Use 'next --unit <name>' instead\n")
		fmt.Fprintln(stderr, "Usage: specflowctl next --unit <name>")
		return nil
	case "-h", "--help", "help":
		writeRootUsage(stdout)
		return nil
	default:
		writeRootUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runPromote(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
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
		fmt.Fprintln(stderr, "Usage: specflowctl promote (--unit <name> | --rule <id>) [--repo-root PATH]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Validates the candidate spec/rule and archives it to stable.")
		fmt.Fprintln(stderr, "Agent should run review+verify before calling this.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Flags:")
		fmt.Fprintln(stderr, "  --unit NAME      Unit name to promote")
		fmt.Fprintln(stderr, "  --rule ID        Rule id to promote")
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
		return runRulePromote(absRoot, ruleID, stdout)
	}

	// Unit promote path
	// Check validate cache freshness
	validateResult, err := validationcache.CheckValidate(absRoot, unitName)
	if err != nil {
		return fmt.Errorf("validate cache error: %w", err)
	}
	if !validateResult.Fresh {
		fmt.Fprintf(stdout, "Validate cache check: FAIL — %s\n", validateResult.Reason)
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Run spec_validate for this unit first, then retry promote.")
		return errors.New("validate cache check failed")
	}
	fmt.Fprintf(stdout, "Validate cache: %s\n", validateResult.Reason)
	fmt.Fprintln(stdout, "")

	// Check verify cache freshness
	verifyResult, err := validationcache.CheckVerify(absRoot, unitName)
	if err != nil {
		return fmt.Errorf("verify cache error: %w", err)
	}
	if !verifyResult.Fresh {
		fmt.Fprintf(stdout, "Verify cache check: FAIL — %s\n", verifyResult.Reason)
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Run spec_verify for this unit first, then retry promote.")
		return errors.New("verify cache check failed")
	}
	fmt.Fprintf(stdout, "Verify cache: %s\n", verifyResult.Reason)
	fmt.Fprintln(stdout, "")

	result := promote.Promote(absRoot, unitName)
	_, err = fmt.Fprint(stdout, promote.FormatResult(result))
	if err != nil {
		return err
	}
	if !result.Passed {
		return errors.New("promote failed")
	}

	// Clean up cache on successful promote
	if delErr := validationcache.DeleteAll(absRoot, unitName); delErr != nil {
		fmt.Fprintf(stderr, "Warning: failed to delete validation cache: %v\n", delErr)
	} else {
		fmt.Fprintln(stdout, "Validation cache cleared.")
	}

	return nil
}

func runRulePromote(absRoot, ruleID string, stdout io.Writer) error {
	result := promote.PromoteRule(absRoot, ruleID)
	_, err := fmt.Fprint(stdout, promote.FormatRuleResult(result))
	if err != nil {
		return err
	}
	if !result.Passed {
		return errors.New("promote failed")
	}
	return nil
}

func runMigrate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot, err := filepath.Abs(*repoRootPtr)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	fmt.Fprintln(stdout, "=== SpecFlow Migration ===")
	fmt.Fprintln(stdout, "")

	// Step 1: Update hook files
	fmt.Fprintln(stdout, "Step 1: Updating hook files...")
	hooksResult, err := install.InstallHooks(absRoot)
	if err != nil {
		fmt.Fprintf(stderr, "  FAILED: %v\n", err)
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Run 'specflowctl init --force' to retry hook installation.")
		return err
	}
	if hooksResult.Copied > 0 {
		fmt.Fprintf(stdout, "  Updated %d hook file(s). Restart your agent session to load new hooks.\n", hooksResult.Copied)
	} else {
		fmt.Fprintln(stdout, "  Hook files are up to date.")
	}
	fmt.Fprintln(stdout, "")

	// Step 2: Check binary version
	fmt.Fprintln(stdout, "Step 2: Checking specFlow binary...")
	doctorResult, err := install.Doctor(absRoot)
	if err != nil {
		fmt.Fprintf(stdout, "  WARNING: doctor check failed: %v\n", err)
	}
	passed := true
	for _, failure := range doctorResult.Failures {
		if strings.Contains(failure, "MISSING") || strings.Contains(failure, "STALE") {
			fmt.Fprintf(stdout, "  %s\n", failure)
			passed = false
		}
	}
	if passed {
		fmt.Fprintln(stdout, "  specFlow binary is up to date.")
	} else {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Run 'specflowctl build-release' to rebuild binaries, or reinstall specFlow.")
	}
	fmt.Fprintln(stdout, "")

	// Summary
	fmt.Fprintln(stdout, "=== Migration Complete ===")
	if hooksResult.Copied > 0 {
		fmt.Fprintln(stdout, "Hook files were updated. Please restart your agent session.")
	}
	if hooksResult.Copied == 0 && passed {
		fmt.Fprintln(stdout, "All checks passed. No updates needed.")
	}
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "Next step: run spec_flow_update in your agent session to check project document format.")
	return nil
}

func runInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	force := fs.Bool("force", false, "overwrite framework files")
	verify := fs.Bool("verify", false, "check project initialization state only (no copy)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot := mustAbs(*repoRoot)

	if *verify {
		result, err := install.CheckProjectInit(absRoot)
		if err != nil {
			return err
		}
		if len(result.Failures) == 0 {
			fmt.Fprintln(stdout, "Project initialization is complete.")
			return nil
		}
		for _, failure := range result.Failures {
			fmt.Fprintln(stdout, failure)
		}
		return fmt.Errorf("project initialization check failed: %d missing file(s)", len(result.Failures))
	}

	result, err := install.Init(absRoot, *force)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "specFlow init completed. copied=%d skipped=%d\n", result.Copied, result.Skipped)

	// Always install platform hooks for session injection
	hooksResult, err := install.InstallHooks(absRoot)
	if err != nil {
		return fmt.Errorf("install hooks: %w", err)
	}
	if hooksResult.Copied > 0 {
		fmt.Fprintf(stdout, "hooks installed: copied=%d\n", hooksResult.Copied)
	}

	return nil
}

func runDoctor(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot := mustAbs(*repoRoot)

	frameworkResult, err := install.Doctor(absRoot)
	if err != nil {
		return err
	}
	projectResult, err := install.CheckProjectInit(absRoot)
	if err != nil {
		return err
	}

	allFailures := append(frameworkResult.Failures, projectResult.Failures...)

	for _, warning := range frameworkResult.Warnings {
		fmt.Fprintln(stdout, warning)
	}
	if len(allFailures) == 0 {
		fmt.Fprintln(stdout, "specFlow doctor passed")
		return nil
	}
	for _, failure := range allFailures {
		fmt.Fprintln(stdout, failure)
	}
	return fmt.Errorf("specFlow doctor failed: %d issue(s)", len(allFailures))
}

func runBuildRelease(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("build-release", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := buildrelease.BuildAll(mustAbs(*repoRoot), nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Built release binaries:")
	for _, target := range result.Targets {
		fmt.Fprintf(stdout, "- %s\n", target)
	}
	return nil
}

func runRepositoryMapping(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeRepositoryMappingUsage(stderr)
		return errors.New("missing repository-mapping subcommand")
	}

	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("repository-mapping validate", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := repositorymapping.Validate(mustAbs(*repoRoot))
		if err != nil {
			return err
		}

		if result.Valid() {
			fmt.Fprintln(stdout, "Repository mapping is valid.")
			return nil
		}

		fmt.Fprintf(stdout, "Repository mapping is invalid. issues=%d\n", len(result.Diagnostics))
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(stdout, "- %s\n", diagnostic)
		}
		return errors.New("repository mapping validation failed")
	case "-h", "--help", "help":
		writeRepositoryMappingUsage(stdout)
		return nil
	default:
		writeRepositoryMappingUsage(stderr)
		return fmt.Errorf("unknown repository-mapping subcommand %q", args[0])
	}
}

func runReview(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeReviewUsage(stderr)
		return errors.New("missing review subcommand")
	}

	switch args[0] {
	case "collect-default-scope":
		fs := flag.NewFlagSet("review collect-default-scope", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		flow := fs.String("flow", "", "review flow")

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}

		return writeReviewScope(stdout, mustAbs(*repoRoot), *flow)
	case "run-init":
		fs := flag.NewFlagSet("review run-init", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		flow := fs.String("flow", "", "review flow")

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}
		result, err := reviewrun.Init(mustAbs(*repoRoot), *flow, time.Now().UTC())
		if err != nil {
			return err
		}
		if result.Created {
			fmt.Fprintf(stdout, "Review run-state created: %s\n", result.File)
			if len(result.DeletedFiles) > 0 {
				fmt.Fprintf(stdout, "Deleted run-state files (%d):\n", len(result.DeletedFiles))
				for _, deleted := range result.DeletedFiles {
					fmt.Fprintf(stdout, "- %s | reason=%s\n", deleted.File, deleted.Reason)
				}
			}
			return nil
		}
		fmt.Fprintf(stdout, "Review run-state reused: %s\n", result.File)
		return nil
	case "run-validate":
		fs := flag.NewFlagSet("review run-validate", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		flow := fs.String("flow", "", "review flow")

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}
		absRepoRoot := mustAbs(*repoRoot)
		file, err := reviewrun.FixedRunStateFile(absRepoRoot, *flow)
		if err != nil {
			return err
		}
		result := reviewrun.ValidateFile(absRepoRoot, *flow, file, time.Now().UTC())
		if result.Valid {
			fmt.Fprintf(stdout, "Review run-state is valid: %s\n", result.File)
			return nil
		}
		fmt.Fprintf(stdout, "Review run-state is invalid: %s\n", result.File)
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(stdout, "- %s\n", diagnostic)
		}
		return errors.New("review run-state validation failed")
	case "run-refresh":
		fs := flag.NewFlagSet("review run-refresh", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		flow := fs.String("flow", "", "review flow")

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}
		absRepoRoot := mustAbs(*repoRoot)
		file, err := reviewrun.FixedRunStateFile(absRepoRoot, *flow)
		if err != nil {
			return err
		}
		result, err := reviewrun.Refresh(absRepoRoot, *flow, file, time.Now().UTC())
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Review run-state refreshed: %s\n", result.File)
		fmt.Fprintf(stdout, "last_updated_at: %s\n", result.LastUpdatedAtUTC)
		writeList(stdout, "Changed fingerprint slices", result.ChangedSlices)
		writeList(stdout, "Stale slices", result.StaleSlices)
		writeList(stdout, "Missing inputs", result.MissingInputs)
		return nil
	case "run-touch":
		fs := flag.NewFlagSet("review run-touch", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		flow := fs.String("flow", "", "review flow")

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}
		absRepoRoot := mustAbs(*repoRoot)
		file, err := reviewrun.FixedRunStateFile(absRepoRoot, *flow)
		if err != nil {
			return err
		}
		result, err := reviewrun.Touch(absRepoRoot, *flow, file, time.Now().UTC())
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Review run-state touched: %s\n", result.File)
		fmt.Fprintf(stdout, "last_updated_at: %s\n", result.LastUpdatedAtUTC)
		return nil
	case "-h", "--help", "help":
		writeReviewUsage(stdout)
		return nil
	default:
		writeReviewUsage(stderr)
		return fmt.Errorf("unknown review subcommand %q", args[0])
	}
}
func runRule(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeRuleUsage(stderr)
		return errors.New("missing rule subcommand")
	}

	switch args[0] {
	case "sync-impact":
		fs := flag.NewFlagSet("rule sync-impact", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		modules := fs.String("units", "", "comma-separated formal units")
		ruleRefs := fs.String("rule-refs", "", "comma-separated rule version refs")
		ruleIDs := fs.String("rule-ids", "", "comma-separated rule ids")
		deletedRuleRefs := fs.String("deleted-rule-refs", "", "comma-separated terminal deleted rule version refs that must have no current consumers")
		stableLandingUnit := fs.String("stable-landing-unit", "", "formal unit whose same-round stable landing should not invalidate itself")
		stableLandingRuleRefs := fs.String("stable-landing-rule-refs", "", "comma-separated exact rule refs written by the same-round stable landing")
		retargetedUnits := fs.String("retargeted-units", "", "comma-separated candidate units retargeted to same-round stable landing rule refs")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := rulesync.SyncImpact(mustAbs(*repoRoot), rulesync.Options{
			Modules:               parseCSV(*modules),
			RuleRefs:              parseCSV(*ruleRefs),
			RuleIDs:               parseCSV(*ruleIDs),
			DeletedRuleRefs:       parseCSV(*deletedRuleRefs),
			StableLandingModule:   strings.TrimSpace(*stableLandingUnit),
			StableLandingRuleRefs: parseCSV(*stableLandingRuleRefs),
			RetargetedUnits:       parseCSV(*retargetedUnits),
		})
		if err != nil {
			return err
		}

		writeList(stdout, "Scoped units", result.ScopedModules)
		writeList(stdout, "Scoped rule refs", result.ScopedRuleRefs)
		writeList(stdout, "Scoped rule ids", result.ScopedRuleIDs)
		writeList(stdout, "Deleted rule refs verified no-impact", result.DeletedRuleRefs)
		fmt.Fprintf(stdout, "Stable landing unit: %s\n", noneIfEmpty(result.StableLandingModule))
		writeList(stdout, "Stable landing rule refs", result.StableLandingRuleRefs)
		writeList(stdout, "Retargeted units", result.RetargetedUnits)
		fmt.Fprintf(stdout, "Unit results (%d):\n", len(result.ModuleResults))
		if len(result.ModuleResults) == 0 {
			fmt.Fprintln(stdout, "- none")
		}
		for _, item := range result.ModuleResults {
			fmt.Fprintf(stdout, "- %s | layer=%s | outcome=%s | reason=%s\n", item.Module, item.ActiveLayer, item.Outcome, noneIfEmpty(item.FallbackReasonCode))
			for _, diagnostic := range item.Diagnostics {
				fmt.Fprintf(stdout, "  diagnostic: %s\n", diagnostic)
			}
		}
		return nil
	case "consumers":
		fs := flag.NewFlagSet("rule consumers", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		ruleID := fs.String("rule-id", "", "rule id")
		ruleRef := fs.String("rule-ref", "", "exact rule version ref")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		result, err := rulesync.Consumers(mustAbs(*repoRoot), rulesync.ConsumerOptions{
			RuleID:  *ruleID,
			RuleRef: *ruleRef,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Rule id: %s\n", noneIfEmpty(result.RuleID))
		fmt.Fprintf(stdout, "Rule ref: %s\n", noneIfEmpty(result.RuleRef))
		fmt.Fprintf(stdout, "Consumers (%d):\n", len(result.Consumers))
		if len(result.Consumers) == 0 {
			fmt.Fprintln(stdout, "- none")
		}
		for _, consumer := range result.Consumers {
			fmt.Fprintf(stdout, "- %s:%s | layer=%s | file=%s | refs=%s\n", consumer.ObjectType, consumer.Object, consumer.ActiveLayer, consumer.FileRef, strings.Join(defaultListValue(consumer.RuleRefs), ", "))
		}
		return nil
	case "release-version":
		fs := flag.NewFlagSet("rule release-version", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		ruleID := fs.String("rule-id", "", "rule id")
		fromRef := fs.String("from-ref", "", "old stable rule version ref")
		toRef := fs.String("to-ref", "", "new stable rule version ref")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		result, err := rulesync.ReleaseVersion(mustAbs(*repoRoot), rulesync.ReleaseVersionOptions{
			RuleID:  *ruleID,
			FromRef: *fromRef,
			ToRef:   *toRef,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Released rule version: %s from %s to %s\n", result.RuleID, result.FromRef, result.ToRef)
		writeList(stdout, "Candidate units updated", result.CandidateUpdated)
		writeList(stdout, "Synced units", result.Sync.ScopedModules)
		return nil
	case "-h", "--help", "help":
		writeRuleUsage(stdout)
		return nil
	default:
		writeRuleUsage(stderr)
		return fmt.Errorf("unknown rule subcommand %q", args[0])
	}
}
func writeRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl <command> [subcommand] [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init       Install specFlow framework files and platform hooks")
	fmt.Fprintln(w, "  migrate    Update hook files and check tooling version")
	fmt.Fprintln(w, "  doctor     Check installed specFlow structure")
	fmt.Fprintln(w, "  build-release Build platform binaries into <tooling-root>/bin")
	fmt.Fprintln(w, "  next       Discover unit files, specs, rules, and dependencies")
	fmt.Fprintln(w, "  promote    Validate candidate spec and archive to stable")
	fmt.Fprintln(w, "  review     Collect governance review scope or maintain run-state files")
	fmt.Fprintln(w, "  rule       Execute rule-impact reconciliation helpers")
	fmt.Fprintln(w, "  validate   Validate candidate spec structure or file write permissions")
	fmt.Fprintln(w, "  repository-mapping (deprecated) Validate docs/specs/repository_mapping.md")
}

func writeRepositoryMappingUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl repository-mapping validate [--repo-root PATH]")
}
func writeReviewUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl review collect-default-scope --flow spec_flow_review|spec_flow_design_review [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-init --flow spec_flow_review|spec_flow_design_review [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-validate --flow spec_flow_review|spec_flow_design_review [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-refresh --flow spec_flow_review|spec_flow_design_review [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-touch --flow spec_flow_review|spec_flow_design_review [--repo-root PATH]")
}
func writeReviewScope(stdout io.Writer, repoRoot, flow string) error {
	var scope reviewscope.SpecFlowScope
	var err error
	switch flow {
	case reviewrun.FlowSpecFlowReview:
		scope, err = reviewscope.CollectDefaultSpecFlowScope(repoRoot)
	case reviewrun.FlowSpecFlowDesignReview:
		scope, err = reviewscope.CollectDefaultSpecFlowDesignScope(repoRoot)
	default:
		return fmt.Errorf("unsupported review flow %q", flow)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Review flow: %s\n", flow)
	fmt.Fprintf(stdout, "Review profile: %s\n", scope.Profile)
	fmt.Fprintf(stdout, "Review layout: %s\n", scope.Layout)
	fmt.Fprintf(stdout, "Framework root: %s\n", scope.FrameworkRoot)
	fmt.Fprintf(stdout, "Template root: %s\n", scope.TemplateRoot)
	fmt.Fprintf(stdout, "Tooling root: %s\n", scope.ToolingRoot)
	fmt.Fprintf(stdout, "Project-instance compatibility mode: %s\n", scope.ProjectInstanceCompatibilityMode)
	writeList(stdout, "Framework guideline files", scope.FrameworkGuidelineFiles)
	writeList(stdout, "Command files", scope.CommandFiles)
	writeList(stdout, "Candidate intent files", scope.CandidateIntentFiles)
	writeList(stdout, "Guidance skill files", scope.GuidanceSkillFiles)
	writeList(stdout, "Rule-governance minimum files", scope.RuleGovernanceFiles)
	writeList(stdout, "Template governance files", scope.TemplateGovernanceFiles)
	writeList(stdout, "Template project-instance files", scope.TemplateProjectInstanceFiles)
	writeList(stdout, "Template entry files", scope.TemplateEntryFiles)
	writeList(stdout, "Project entry files", scope.ProjectEntryFiles)
	writeList(stdout, "Source repo entry example files", scope.SourceRepoEntryExampleFiles)
	writeList(stdout, "Agent operability files", scope.AgentOperabilityFiles)
	writeList(stdout, "Project-instance compatibility files", scope.ProjectInstanceCompatibilityFiles)
	writeList(stdout, "Tooling contract files", scope.ToolingContractFiles)
	writeList(stdout, "Tooling source files", scope.ToolingSourceFiles)
	if len(scope.ToolingScriptFiles) > 0 {
		writeList(stdout, "Tooling script files", scope.ToolingScriptFiles)
	}
	if len(scope.ToolingRuntimeFiles) > 0 {
		writeList(stdout, "Tooling runtime files", scope.ToolingRuntimeFiles)
	}
	return nil
}

func requireReviewFlow(flow string, stderr io.Writer) error {
	flow = strings.TrimSpace(flow)
	if flow == "" {
		writeReviewUsage(stderr)
		return errors.New("flow is required")
	}
	for _, supported := range reviewrun.ConfiguredFlows() {
		if flow == supported {
			return nil
		}
	}
	return fmt.Errorf("unsupported review flow %q", flow)
}

func writeRuleUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl rule sync-impact (--rule-refs c_b_rule_x@0.1.0 | --rule-ids b_rule_x | --deleted-rule-refs c_b_rule_x@0.1.0) [--units unit_a,unit_b] [--stable-landing-unit unit_a --stable-landing-rule-refs s_b_rule_x@1.0.0] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl rule sync-impact --rule-refs c_b_rule_x@0.1.0,s_b_rule_x@0.1.0 --stable-landing-unit unit_a --stable-landing-rule-refs s_b_rule_x@0.1.0 --retargeted-units unit_b [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl rule consumers (--rule-id b_rule_x | --rule-ref s_b_rule_x@1.0.0) [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl rule release-version --rule-id b_rule_x --from-ref s_b_rule_x@0.3.0 --to-ref s_b_rule_x@0.4.0 [--repo-root PATH]")
}
func writeList(w io.Writer, title string, items []string) {
	fmt.Fprintf(w, "%s (%d):\n", title, len(items))
	if len(items) == 0 {
		fmt.Fprintln(w, "- none")
		return
	}
	for _, item := range items {
		fmt.Fprintf(w, "- %s\n", item)
	}
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func resolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, filepath.FromSlash(path))
}

func parseCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func noneIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}


func defaultListValue(items []string) []string {
	if len(items) == 0 {
		return []string{"none"}
	}
	return items
}

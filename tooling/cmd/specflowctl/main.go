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
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewrun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewscope"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
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
	case "fork":
		return runFork(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "build-release":
		return runBuildRelease(args[1:], stdout, stderr)
	case "tooling-fingerprint":
		return runToolingFingerprint(args[1:], stdout, stderr)
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
	case "consumers":
		return runConsumers(args[1:], stdout, stderr)
	case "fresh":
		return runFresh(args[1:], stdout, stderr)
	case "gate-evidence":
		return runGateEvidence(args[1:], stdout, stderr)
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
		return runRulePromote(absRoot, ruleID, stdout, stderr)
	}

	// Unit promote path
	// Detect retirement: a retiring unit is removed from stable, so the
	// content-alignment gates (verify, review) and appendix coverage have no
	// object — only the validate cache gate remains, matching rule retirement.
	retiring := false
	if data, err := os.ReadFile(filepath.Join(absRoot, "docs/specs/units/candidate", fmt.Sprintf("unit_%s.md", unitName))); err == nil {
		fm := specpaths.ReadFrontmatterStringMap(string(data))
		retiring = strings.TrimSpace(fm["status"]) == "retired"
	}

	// Check validate cache freshness
	validateResult, err := validationcache.CheckValidate(absRoot, unitName)
	if err != nil {
		return fmt.Errorf("validate cache error: %w", err)
	}
	if !validateResult.Fresh {
		fmt.Fprintf(stdout, "Validate cache check: FAIL — %s\n", validateResult.Reason)
		fmt.Fprintln(stdout, "")
		fmt.Fprintf(stdout, "Run `validate@%s` first, then retry promote.\n", unitName)
		return errors.New("validate cache check failed")
	}
	fmt.Fprintf(stdout, "Validate cache: %s\n", validateResult.Reason)
	if validateResult.Note != "" {
		fmt.Fprintf(stdout, "Note: %s\n", validateResult.Note)
	}
	fmt.Fprintln(stdout, "")

	if retiring {
		fmt.Fprintln(stdout, "Retiring unit — verify, review, and appendix coverage gates skipped.")
		fmt.Fprintln(stdout, "")
	} else {
		// Check verify cache freshness
		verifyResult, err := validationcache.CheckVerify(absRoot, unitName)
		if err != nil {
			return fmt.Errorf("verify cache error: %w", err)
		}
		if !verifyResult.Fresh {
			fmt.Fprintf(stdout, "Verify cache check: FAIL — %s\n", verifyResult.Reason)
			fmt.Fprintln(stdout, "")
			fmt.Fprintf(stdout, "Run `verify@%s` first, then retry promote.\n", unitName)
			return errors.New("verify cache check failed")
		}
		fmt.Fprintf(stdout, "Verify cache: %s\n", verifyResult.Reason)
		if verifyResult.Note != "" {
			fmt.Fprintf(stdout, "Note: %s\n", verifyResult.Note)
		}
		fmt.Fprintln(stdout, "")

		// Check review cache (required gate — must exist, be full mode, fresh, and non-blocking)
		reviewResult, err := validationcache.CheckReview(absRoot, unitName)
		if err != nil {
			return fmt.Errorf("review cache error: %w", err)
		}
		if !reviewResult.Fresh {
			fmt.Fprintf(stdout, "Review cache check: FAIL — %s\n", reviewResult.Reason)
			fmt.Fprintln(stdout, "")
			fmt.Fprintf(stdout, "Run `review@%s` first, then retry promote.\n", unitName)
			return errors.New("review cache check failed")
		}
		fmt.Fprintf(stdout, "Review cache: %s\n", reviewResult.Reason)
		if reviewResult.Note != "" {
			fmt.Fprintf(stdout, "Note: %s\n", reviewResult.Note)
		}
		fmt.Fprintln(stdout, "")

		// Check appendix files are included in validate cache
		appendixResult, err := validationcache.CheckAppendicesInCache(absRoot, unitName)
		if err != nil {
			return fmt.Errorf("appendix cache check error: %w", err)
		}
		if !appendixResult.Fresh {
			fmt.Fprintf(stdout, "Appendix cache check: FAIL — %s\n", appendixResult.Reason)
			fmt.Fprintln(stdout, "")
			fmt.Fprintf(stdout, "One or more appendix files were not validated. Run `validate@%s` first.\n", unitName)
			return errors.New("appendix validation check failed")
		}
		fmt.Fprintf(stdout, "Appendix cache: %s\n", appendixResult.Reason)
		fmt.Fprintln(stdout, "")
	}

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
		fmt.Fprintf(stderr, "Warning: failed to delete caches: %v\n", delErr)
	} else {
		fmt.Fprintln(stdout, "Validate, verify, and review caches cleared.")
	}

	return nil
}

func runRulePromote(absRoot, ruleID string, stdout, stderr io.Writer) error {
	// Check validate cache freshness only (rule verify has been removed — see framework/concepts.md)
	validateResult, err := validationcache.CheckRuleValidate(absRoot, ruleID)
	if err != nil {
		return fmt.Errorf("validate cache error: %w", err)
	}
	if !validateResult.Fresh {
		fmt.Fprintf(stdout, "Validate cache check: FAIL — %s\n", validateResult.Reason)
		fmt.Fprintln(stdout, "")
		fmt.Fprintf(stdout, "Run `validate@%s` first, then retry promote.\n", ruleID)
		return errors.New("validate cache check failed")
	}
	fmt.Fprintf(stdout, "Validate cache: %s\n", validateResult.Reason)
	if validateResult.Note != "" {
		fmt.Fprintf(stdout, "Note: %s\n", validateResult.Note)
	}
	fmt.Fprintln(stdout, "")

	result := promote.PromoteRule(absRoot, ruleID)
	_, err = fmt.Fprint(stdout, promote.FormatRuleResult(result))
	if err != nil {
		return err
	}
	if !result.Passed {
		return errors.New("promote failed")
	}

	// Clean up cache on successful promote
	if delErr := validationcache.DeleteRuleCache(absRoot, ruleID, "validate"); delErr != nil {
		fmt.Fprintf(stderr, "Warning: failed to delete validation cache: %v\n", delErr)
	} else {
		fmt.Fprintln(stdout, "Validation cache cleared.")
	}

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

func runReview(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeReviewUsage(stderr)
		return errors.New("missing review subcommand")
	}

	switch args[0] {
	case "collect-default-scope":
		return runReviewCollectDefaultScope(args[1:], stdout, stderr)
	case "run-init":
		return runReviewRunInit(args[1:], stdout, stderr)
	case "run-validate":
		return runReviewRunValidate(args[1:], stdout, stderr)
	case "run-refresh":
		return runReviewRunRefresh(args[1:], stdout, stderr)
	case "run-touch":
		return runReviewRunTouch(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeReviewUsage(stdout)
		return nil
	default:
		writeReviewUsage(stderr)
		return fmt.Errorf("unknown review subcommand %q", args[0])
	}
}

func runReviewCollectDefaultScope(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("review collect-default-scope", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	flow := fs.String("flow", "", "review flow")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireReviewFlow(*flow, stderr); err != nil {
		return err
	}
	return writeReviewScope(stdout, mustAbs(*repoRoot), *flow)
}

func runReviewRunInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("review run-init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	flow := fs.String("flow", "", "review flow")

	if err := fs.Parse(args); err != nil {
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
}

func runReviewRunValidate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("review run-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	flow := fs.String("flow", "", "review flow")

	if err := fs.Parse(args); err != nil {
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
}

func runReviewRunRefresh(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("review run-refresh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	flow := fs.String("flow", "", "review flow")

	if err := fs.Parse(args); err != nil {
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
}

func runReviewRunTouch(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("review run-touch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	flow := fs.String("flow", "", "review flow")

	if err := fs.Parse(args); err != nil {
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
}
func runRule(args []string, stdout, stderr io.Writer) error {
	fmt.Fprintf(stderr, "'rule' subcommands are no longer supported in this version of specFlow\n")
	return errors.New("removed command")
}
func writeRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl <command> [subcommand] [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  fork       Fork a stable spec/rule (and appendices) to candidate layer")
	fmt.Fprintln(w, "  init       Install specFlow framework files and platform hooks")
	fmt.Fprintln(w, "  doctor     Check installed specFlow structure")
	fmt.Fprintln(w, "  build-release Build platform binaries into <tooling-root>/bin")
	fmt.Fprintln(w, "  tooling-fingerprint Print the live tooling source fingerprint")
	fmt.Fprintln(w, "  next       Discover unit files, specs, rules, and dependencies")
	fmt.Fprintln(w, "  promote    Validate candidate spec and archive to stable")
	fmt.Fprintln(w, "  review     Collect governance review scope or maintain run-state files")
	fmt.Fprintln(w, "  consumers  List units that reference a given rule")
	fmt.Fprintln(w, "  fresh      Report cache freshness for all candidates or a single target")
	fmt.Fprintln(w, "  gate-evidence Compute dependency chunk CIDs for a file read during a gate run")
	fmt.Fprintln(w, "  validate   Validate candidate spec/rule structure or file write permissions")
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

func noneIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

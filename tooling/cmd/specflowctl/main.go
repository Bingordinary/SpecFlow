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
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/entrysync"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/install"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/processcleanup"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/projectstandards"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewrun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewscope"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/sharedsync"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/snapshot"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/toolingfreshness"
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
	case "upgrade":
		return runUpgrade(args[1:], stdout, stderr)
	case "build-release":
		return runBuildRelease(args[1:], stdout, stderr)
	case "entry":
		return runEntry(args[1:], stdout, stderr)
	case "registry":
		return runRegistry(args[1:], stdout, stderr)
	case "review":
		return runReview(args[1:], stdout, stderr)
	case "process":
		return runProcess(args[1:], stdout, stderr)
	case "shared":
		return runShared(args[1:], stdout, stderr)
	case "snapshot":
		return runSnapshot(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeRootUsage(stdout)
		return nil
	default:
		writeRootUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	force := fs.Bool("force", false, "overwrite managed files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := install.Init(mustAbs(*repoRoot), *force)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "specFlow init completed. copied=%d skipped=%d\n", result.Copied, result.Skipped)
	return nil
}

func runDoctor(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := install.Doctor(mustAbs(*repoRoot))
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(stdout, warning)
	}
	if len(result.Failures) == 0 {
		fmt.Fprintln(stdout, "specFlow doctor passed")
		return nil
	}
	for _, failure := range result.Failures {
		fmt.Fprintln(stdout, failure)
	}
	return fmt.Errorf("specFlow doctor failed: %d issue(s)", len(result.Failures))
}

func runUpgrade(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := install.Upgrade(mustAbs(*repoRoot))
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "specFlow upgrade completed. updated=%d skipped=%d\n", result.Updated, result.Skipped)
	return nil
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

func runEntry(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeEntryUsage(stderr)
		return errors.New("missing entry subcommand")
	}

	switch args[0] {
	case "check":
		fs := flag.NewFlagSet("entry check", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		inspection, err := entrysync.Inspect(mustAbs(*repoRoot))
		if err != nil {
			return err
		}

		if inspection.Consistent {
			fmt.Fprintln(stdout, "Managed entry blocks are already consistent.")
			return nil
		}

		fmt.Fprintln(stdout, "Managed entry blocks are inconsistent.")
		if inspection.SuggestedSource != "" {
			fmt.Fprintf(stdout, "Suggested source: %s\n", inspection.SuggestedSource)
		}
		if len(inspection.CurrentRoundChanged) > 0 {
			fmt.Fprintln(stdout, "Registered entry files changed in current round:")
			for _, path := range inspection.CurrentRoundChanged {
				fmt.Fprintf(stdout, "- %s\n", path)
			}
		}
		return errors.New("entry managed blocks differ")
	case "sync":
		fs := flag.NewFlagSet("entry sync", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		source := fs.String("source", "", "registered source entry file")
		stage := fs.Bool("stage", false, "stage synced registered entry files")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := entrysync.Sync(mustAbs(*repoRoot), *source, *stage)
		if err != nil {
			return err
		}

		if len(result.UpdatedFiles) == 0 {
			if result.Source != "" {
				fmt.Fprintf(stdout, "Managed entry blocks already matched source: %s\n", result.Source)
			} else {
				fmt.Fprintln(stdout, "Managed entry blocks are already consistent.")
			}
			return nil
		}

		fmt.Fprintf(stdout, "Synced managed entry blocks from %s\n", result.Source)
		for _, path := range result.UpdatedFiles {
			fmt.Fprintf(stdout, "- %s\n", path)
		}
		if result.Staged {
			fmt.Fprintln(stdout, "Registered entry files were staged.")
		}
		return nil
	case "-h", "--help", "help":
		writeEntryUsage(stdout)
		return nil
	default:
		writeEntryUsage(stderr)
		return fmt.Errorf("unknown entry subcommand %q", args[0])
	}
}

func runRegistry(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeRegistryUsage(stderr)
		return errors.New("missing registry subcommand")
	}

	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("registry validate", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := projectstandards.ValidateRegistry(mustAbs(*repoRoot))
		if err != nil {
			return err
		}

		if len(result.Diagnostics) == 0 {
			fmt.Fprintf(stdout, "Project standards registry is valid. active_entries=%d\n", len(result.Entries))
			return nil
		}

		fmt.Fprintf(stdout, "Project standards registry is invalid. issues=%d\n", len(result.Diagnostics))
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(stdout, "- %s\n", diagnostic)
		}
		return errors.New("project standards registry validation failed")
	case "-h", "--help", "help":
		writeRegistryUsage(stdout)
		return nil
	default:
		writeRegistryUsage(stderr)
		return fmt.Errorf("unknown registry subcommand %q", args[0])
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

		var scope reviewscope.SpecFlowScope
		var err error
		switch *flow {
		case reviewrun.FlowSpecFlowReview:
			scope, err = reviewscope.CollectDefaultSpecFlowScope(mustAbs(*repoRoot))
		case reviewrun.FlowSpecFlowDesignReview:
			scope, err = reviewscope.CollectDefaultSpecFlowDesignScope(mustAbs(*repoRoot))
		default:
			return fmt.Errorf("unsupported review flow %q", *flow)
		}
		if err != nil {
			return err
		}

		fmt.Fprintf(stdout, "Review flow: %s\n", *flow)
		fmt.Fprintf(stdout, "Review scenario: %s\n", scope.Scenario)
		writeList(stdout, "Framework guideline files", scope.FrameworkGuidelineFiles)
		writeList(stdout, "Command files", scope.CommandFiles)
		writeList(stdout, "Guidance skill files", scope.GuidanceSkillFiles)
		writeList(stdout, "Shared-governance minimum files", scope.SharedGovernanceFiles)
		writeList(stdout, "Template governance files", scope.TemplateGovernanceFiles)
		writeList(stdout, "Template entry files", scope.TemplateEntryFiles)
		writeList(stdout, "Project entry files", scope.ProjectEntryFiles)
		writeList(stdout, "Agent operability files", scope.AgentOperabilityFiles)
		writeList(stdout, "Project registry files", scope.ProjectRegistryFiles)
		writeList(stdout, "Project registry diagnostics", scope.RegistryDiagnostics)
		writeList(stdout, "Tooling contract files", scope.ToolingContractFiles)
		writeList(stdout, "Tooling source files", scope.ToolingSourceFiles)
		writeList(stdout, "Active project-local governance-input files", scope.ActiveProjectStandardFiles)
		return nil
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
		file := fs.String("file", "", "review run-state file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}
		if strings.TrimSpace(*file) == "" {
			writeReviewUsage(stderr)
			return errors.New("file is required")
		}
		absRepoRoot := mustAbs(*repoRoot)
		result := reviewrun.ValidateFile(absRepoRoot, *flow, resolvePath(absRepoRoot, *file), time.Now().UTC())
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
		file := fs.String("file", "", "review run-state file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}
		if strings.TrimSpace(*file) == "" {
			writeReviewUsage(stderr)
			return errors.New("file is required")
		}
		absRepoRoot := mustAbs(*repoRoot)
		result, err := reviewrun.Refresh(absRepoRoot, *flow, resolvePath(absRepoRoot, *file), time.Now().UTC())
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
		file := fs.String("file", "", "review run-state file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if err := requireReviewFlow(*flow, stderr); err != nil {
			return err
		}
		if strings.TrimSpace(*file) == "" {
			writeReviewUsage(stderr)
			return errors.New("file is required")
		}
		absRepoRoot := mustAbs(*repoRoot)
		result, err := reviewrun.Touch(absRepoRoot, *flow, resolvePath(absRepoRoot, *file), time.Now().UTC())
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

func runProcess(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeProcessUsage(stderr)
		return errors.New("missing process subcommand")
	}

	switch args[0] {
	case "cleanup-fallback":
		fs := flag.NewFlagSet("process cleanup-fallback", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		module := fs.String("unit", "", "formal unit name")
		fromCommand := fs.String("from-command", "", "origin command")
		reason := fs.String("reason", "", "fallback reason code")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*module) == "" || strings.TrimSpace(*fromCommand) == "" || strings.TrimSpace(*reason) == "" {
			writeProcessUsage(stderr)
			return errors.New("unit, from-command, and reason are required")
		}

		result, err := processcleanup.ApplyFallback(mustAbs(*repoRoot), *module, *fromCommand, *reason)
		if err != nil {
			return err
		}

		fmt.Fprintf(stdout, "Applied fallback cleanup for %s\n", result.Module)
		fmt.Fprintf(stdout, "From command: %s\n", result.FromCommand)
		fmt.Fprintf(stdout, "Fallback reason: %s\n", result.Reason)
		fmt.Fprintf(stdout, "Next Command: %s\n", result.NextCommand)
		writeList(stdout, "Deleted files", result.DeletedFiles)
		writeList(stdout, "Missing files", result.MissingFiles)
		fmt.Fprintf(stdout, "Status file updated: %t\n", result.StatusUpdated)
		return nil
	case "cleanup-success":
		fs := flag.NewFlagSet("process cleanup-success", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		module := fs.String("unit", "", "formal unit name")
		mode := fs.String("mode", "", "success cleanup mode")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*module) == "" || strings.TrimSpace(*mode) == "" {
			writeProcessUsage(stderr)
			return errors.New("unit and mode are required")
		}

		result, err := processcleanup.ApplySuccessCleanup(mustAbs(*repoRoot), *module, *mode)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Applied success cleanup for %s\n", result.Module)
		fmt.Fprintf(stdout, "Mode: %s\n", result.Mode)
		writeList(stdout, "Deleted files", result.DeletedFiles)
		writeList(stdout, "Missing files", result.MissingFiles)
		return nil
	case "-h", "--help", "help":
		writeProcessUsage(stdout)
		return nil
	default:
		writeProcessUsage(stderr)
		return fmt.Errorf("unknown process subcommand %q", args[0])
	}
}

func runShared(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeSharedUsage(stderr)
		return errors.New("missing shared subcommand")
	}

	switch args[0] {
	case "sync-impact":
		fs := flag.NewFlagSet("shared sync-impact", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		modules := fs.String("units", "", "comma-separated formal units")
		sharedRefs := fs.String("shared-refs", "", "comma-separated shared version refs")
		sharedIDs := fs.String("shared-ids", "", "comma-separated shared contract ids")
		stableLandingUnit := fs.String("stable-landing-unit", "", "formal unit whose same-round stable landing should not invalidate itself")
		stableLandingSharedRefs := fs.String("stable-landing-shared-refs", "", "comma-separated exact shared refs written by the same-round stable landing")
		boundObjectsOnlySharedFileRefs := fs.String("bound-objects-only-shared-file-refs", "", "comma-separated shared file refs proven to be bound_objects-only deltas")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := sharedsync.SyncImpact(mustAbs(*repoRoot), sharedsync.Options{
			Modules:                        parseCSV(*modules),
			SharedRefs:                     parseCSV(*sharedRefs),
			SharedIDs:                      parseCSV(*sharedIDs),
			StableLandingModule:            strings.TrimSpace(*stableLandingUnit),
			StableLandingSharedRefs:        parseCSV(*stableLandingSharedRefs),
			BoundObjectsOnlySharedFileRefs: parseCSV(*boundObjectsOnlySharedFileRefs),
		})
		if err != nil {
			return err
		}

		writeList(stdout, "Scoped units", result.ScopedModules)
		writeList(stdout, "Scoped scenarios", result.ScopedFlows)
		writeList(stdout, "Scoped shared refs", result.ScopedSharedRefs)
		writeList(stdout, "Scoped shared ids", result.ScopedSharedIDs)
		fmt.Fprintf(stdout, "Stable landing unit: %s\n", noneIfEmpty(result.StableLandingModule))
		writeList(stdout, "Stable landing shared refs", result.StableLandingSharedRefs)
		writeList(stdout, "Bound-objects-only shared file refs", result.BoundObjectsOnlySharedFileRefs)
		fmt.Fprintf(stdout, "Unit results (%d):\n", len(result.ModuleResults))
		if len(result.ModuleResults) == 0 {
			fmt.Fprintln(stdout, "- none")
		}
		for _, item := range result.ModuleResults {
			fmt.Fprintf(stdout, "- %s | layer=%s | outcome=%s | next=%s | reason=%s | status_updated=%t\n", item.Module, item.ActiveLayer, item.Outcome, item.NextCommand, noneIfEmpty(item.FallbackReasonCode), item.StatusUpdated)
			for _, diagnostic := range item.Diagnostics {
				fmt.Fprintf(stdout, "  diagnostic: %s\n", diagnostic)
			}
			for _, path := range item.DeletedFiles {
				fmt.Fprintf(stdout, "  deleted: %s\n", path)
			}
			for _, path := range item.MissingFiles {
				fmt.Fprintf(stdout, "  missing: %s\n", path)
			}
		}
		fmt.Fprintf(stdout, "Bound-object drifts (%d):\n", len(result.BoundObjectDrifts))
		if len(result.BoundObjectDrifts) == 0 {
			fmt.Fprintln(stdout, "- none")
		} else {
			for _, drift := range result.BoundObjectDrifts {
				fmt.Fprintf(stdout, "- %s | file=%s | version=%s | bound_objects_only_delta=%t\n", drift.SharedContractID, drift.FileRef, drift.VersionRef, drift.BoundObjectsOnlyDelta)
				fmt.Fprintf(stdout, "  declared=%s\n", strings.Join(defaultListValue(drift.DeclaredObjects), ", "))
				fmt.Fprintf(stdout, "  actual=%s\n", strings.Join(defaultListValue(drift.ActualObjects), ", "))
			}
		}
		fmt.Fprintf(stdout, "Flow results (%d):\n", len(result.FlowResults))
		if len(result.FlowResults) == 0 {
			fmt.Fprintln(stdout, "- none")
		}
		for _, item := range result.FlowResults {
			fmt.Fprintf(stdout, "- %s | layer=%s | outcome=%s | next=%s | reason=%s | status_updated=%t\n", item.Object, item.ActiveLayer, item.Outcome, item.NextCommand, noneIfEmpty(item.FallbackReasonCode), item.StatusUpdated)
			for _, diagnostic := range item.Diagnostics {
				fmt.Fprintf(stdout, "  diagnostic: %s\n", diagnostic)
			}
			for _, path := range item.DeletedFiles {
				fmt.Fprintf(stdout, "  deleted: %s\n", path)
			}
			for _, path := range item.MissingFiles {
				fmt.Fprintf(stdout, "  missing: %s\n", path)
			}
		}
		return nil
	case "reconcile-bound-objects":
		fs := flag.NewFlagSet("shared reconcile-bound-objects", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		modules := fs.String("units", "", "comma-separated formal units")
		sharedRefs := fs.String("shared-refs", "", "comma-separated shared version refs")
		sharedIDs := fs.String("shared-ids", "", "comma-separated shared contract ids")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		result, err := sharedsync.ReconcileBoundModules(mustAbs(*repoRoot), sharedsync.ReconcileBoundModulesOptions{
			Modules:    parseCSV(*modules),
			SharedRefs: parseCSV(*sharedRefs),
			SharedIDs:  parseCSV(*sharedIDs),
		})
		if err != nil {
			return err
		}

		writeList(stdout, "Scoped units", result.ScopedModules)
		writeList(stdout, "Scoped shared refs", result.ScopedSharedRefs)
		writeList(stdout, "Scoped shared ids", result.ScopedSharedIDs)
		writeList(stdout, "Touched shared files", result.TouchedFiles)
		writeList(stdout, "Updated shared files", result.UpdatedFiles)
		writeList(stdout, "Unchanged shared files", result.UnchangedFiles)
		return nil
	case "-h", "--help", "help":
		writeSharedUsage(stdout)
		return nil
	default:
		writeSharedUsage(stderr)
		return fmt.Errorf("unknown shared subcommand %q", args[0])
	}
}

func runSnapshot(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeSnapshotUsage(stderr)
		return errors.New("missing snapshot subcommand")
	}

	switch args[0] {
	case "rebuild":
		fs := flag.NewFlagSet("snapshot rebuild", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		module := fs.String("unit", "", "formal unit name")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*module) == "" {
			writeSnapshotUsage(stderr)
			return errors.New("unit is required")
		}
		result, err := snapshot.RebuildCurrent(mustAbs(*repoRoot), *module)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, snapshot.Render(result))
		return nil
	case "validate-process":
		fs := flag.NewFlagSet("snapshot validate-process", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		module := fs.String("unit", "", "formal unit name")
		processKind := fs.String("process", "", "check | plan | verify")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*module) == "" || strings.TrimSpace(*processKind) == "" {
			writeSnapshotUsage(stderr)
			return errors.New("unit and process are required")
		}
		result, err := snapshot.ValidateProcessFile(mustAbs(*repoRoot), *module, *processKind)
		if err != nil {
			return err
		}
		if result.Valid {
			fmt.Fprintf(stdout, "Process snapshot is valid. file=%s\n", result.ProcessFile)
			return nil
		}
		fmt.Fprintf(stdout, "Process snapshot is invalid. file=%s\n", result.ProcessFile)
		for _, mismatch := range result.Mismatches {
			fmt.Fprintf(stdout, "- %s\n", mismatch)
		}
		return errors.New("process snapshot mismatch")
	case "-h", "--help", "help":
		writeSnapshotUsage(stdout)
		return nil
	default:
		writeSnapshotUsage(stderr)
		return fmt.Errorf("unknown snapshot subcommand %q", args[0])
	}
}

func runStatus(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeStatusUsage(stderr)
		return errors.New("missing status subcommand")
	}

	switch args[0] {
	case "set-object":
		fs := flag.NewFlagSet("status set-object", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		objectType := fs.String("type", "", "object type")
		object := fs.String("object", "", "formal object id")
		stable := fs.String("stable", "", "yes | no")
		candidate := fs.String("candidate", "", "yes | no")
		activeLayer := fs.String("active-layer", "", "stable | candidate")
		nextCommand := fs.String("next-command", "", "next command")
		notes := fs.String("notes", "", "notes text")
		create := fs.Bool("create", false, "create row when missing")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*objectType) == "" || strings.TrimSpace(*object) == "" || strings.TrimSpace(*stable) == "" || strings.TrimSpace(*candidate) == "" || strings.TrimSpace(*activeLayer) == "" || strings.TrimSpace(*nextCommand) == "" {
			writeStatusUsage(stderr)
			return errors.New("type, object, stable, candidate, active-layer, and next-command are required")
		}
		if *stable != "yes" && *stable != "no" {
			return fmt.Errorf("stable must be yes or no")
		}
		if *candidate != "yes" && *candidate != "no" {
			return fmt.Errorf("candidate must be yes or no")
		}
		if *activeLayer != "stable" && *activeLayer != "candidate" {
			return fmt.Errorf("active-layer must be stable or candidate")
		}

		updated, err := statusfile.UpsertObjectStatus(mustAbs(*repoRoot), statusfile.ObjectStatus{
			ObjectType:  strings.TrimSpace(*objectType),
			Object:      strings.TrimSpace(*object),
			Stable:      strings.TrimSpace(*stable),
			Candidate:   strings.TrimSpace(*candidate),
			ActiveLayer: strings.TrimSpace(*activeLayer),
			NextCommand: strings.TrimSpace(*nextCommand),
			Notes:       strings.TrimSpace(*notes),
		}, *create)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Status row upserted: %t\n", updated)
		fmt.Fprintf(stdout, "Object Type: %s\n", strings.TrimSpace(*objectType))
		fmt.Fprintf(stdout, "Object: %s\n", strings.TrimSpace(*object))
		fmt.Fprintf(stdout, "Stable: %s\n", strings.TrimSpace(*stable))
		fmt.Fprintf(stdout, "Candidate: %s\n", strings.TrimSpace(*candidate))
		fmt.Fprintf(stdout, "Active Layer: %s\n", strings.TrimSpace(*activeLayer))
		fmt.Fprintf(stdout, "Next Command: %s\n", strings.TrimSpace(*nextCommand))
		fmt.Fprintf(stdout, "Notes: %s\n", noneIfEmpty(strings.TrimSpace(*notes)))
		return nil
	case "set-unit":
		fs := flag.NewFlagSet("status set-unit", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		module := fs.String("unit", "", "formal unit name")
		stable := fs.String("stable", "", "yes | no")
		candidate := fs.String("candidate", "", "yes | no")
		activeLayer := fs.String("active-layer", "", "stable | candidate")
		nextCommand := fs.String("next-command", "", "next command")
		notes := fs.String("notes", "", "notes text")
		create := fs.Bool("create", false, "create row when missing")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*module) == "" || strings.TrimSpace(*stable) == "" || strings.TrimSpace(*candidate) == "" || strings.TrimSpace(*activeLayer) == "" || strings.TrimSpace(*nextCommand) == "" {
			writeStatusUsage(stderr)
			return errors.New("unit, stable, candidate, active-layer, and next-command are required")
		}
		if *stable != "yes" && *stable != "no" {
			return fmt.Errorf("stable must be yes or no")
		}
		if *candidate != "yes" && *candidate != "no" {
			return fmt.Errorf("candidate must be yes or no")
		}
		if *activeLayer != "stable" && *activeLayer != "candidate" {
			return fmt.Errorf("active-layer must be stable or candidate")
		}

		updated, err := statusfile.UpsertModuleStatus(mustAbs(*repoRoot), statusfile.ModuleStatus{
			Module:      strings.TrimSpace(*module),
			Stable:      strings.TrimSpace(*stable),
			Candidate:   strings.TrimSpace(*candidate),
			ActiveLayer: strings.TrimSpace(*activeLayer),
			NextCommand: strings.TrimSpace(*nextCommand),
			Notes:       strings.TrimSpace(*notes),
		}, *create)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Status row upserted: %t\n", updated)
		fmt.Fprintf(stdout, "Unit: %s\n", strings.TrimSpace(*module))
		fmt.Fprintf(stdout, "Stable: %s\n", strings.TrimSpace(*stable))
		fmt.Fprintf(stdout, "Candidate: %s\n", strings.TrimSpace(*candidate))
		fmt.Fprintf(stdout, "Active Layer: %s\n", strings.TrimSpace(*activeLayer))
		fmt.Fprintf(stdout, "Next Command: %s\n", strings.TrimSpace(*nextCommand))
		fmt.Fprintf(stdout, "Notes: %s\n", noneIfEmpty(strings.TrimSpace(*notes)))
		return nil
	case "-h", "--help", "help":
		writeStatusUsage(stdout)
		return nil
	default:
		writeStatusUsage(stderr)
		return fmt.Errorf("unknown status subcommand %q", args[0])
	}
}

func writeRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl <command> [subcommand] [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init     Install specFlow files from manifest")
	fmt.Fprintln(w, "  doctor   Check installed specFlow structure")
	fmt.Fprintln(w, "  upgrade  Refresh framework-managed files")
	fmt.Fprintln(w, "  build-release Build platform binaries into specflow/tooling/bin")
	fmt.Fprintln(w, "  entry    Check or sync registered entry-file managed blocks")
	fmt.Fprintln(w, "  registry Validate docs/project_standards/_registry.md")
	fmt.Fprintln(w, "  review   Collect governance review scope or maintain run-state files")
	fmt.Fprintln(w, "  process  Execute deterministic fallback cleanup")
	fmt.Fprintln(w, "  shared   Execute deterministic shared-impact reconciliation helpers")
	fmt.Fprintln(w, "  snapshot Rebuild or compare process snapshot fields")
	fmt.Fprintln(w, "  status   Apply deterministic _status.md row writeback")
}

func writeEntryUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl entry check [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl entry sync [--repo-root PATH] [--source FILE] [--stage]")
}

func writeRegistryUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl registry validate [--repo-root PATH]")
}

func writeReviewUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl review collect-default-scope --flow spec_flow_review|spec_flow_design_review [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-init --flow spec_flow_review|spec_flow_design_review [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-validate --flow spec_flow_review|spec_flow_design_review --file FILE [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-refresh --flow spec_flow_review|spec_flow_design_review --file FILE [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl review run-touch --flow spec_flow_review|spec_flow_design_review --file FILE [--repo-root PATH]")
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
	writeReviewUsage(stderr)
	return fmt.Errorf("unsupported review flow %q", flow)
}

func writeProcessUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl process cleanup-fallback --unit UNIT --from-command COMMAND --reason CODE [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl process cleanup-success --unit UNIT --mode unit_fork|unit_promote [--repo-root PATH]")
}

func writeSharedUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl shared sync-impact (--shared-refs c_shared_x@0.1.0 | --shared-ids shared_x) [--units unit_a,unit_b] [--stable-landing-unit unit_a --stable-landing-shared-refs s_shared_x@1.0.0] [--bound-objects-only-shared-file-refs docs/specs/shared_contracts/stable/s_shared_x.md] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl shared reconcile-bound-objects [--units unit_a,unit_b] [--shared-refs c_shared_x@0.1.0] [--shared-ids shared_x] [--repo-root PATH]")
}

func writeSnapshotUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl snapshot rebuild --unit UNIT [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl snapshot validate-process --unit UNIT --process check|plan|verify [--repo-root PATH]")
}

func writeStatusUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl status set-object --type scenario|unit --object OBJECT --stable yes|no --candidate yes|no --active-layer stable|candidate --next-command COMMAND [--notes TEXT] [--create] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl status set-unit --unit UNIT --stable yes|no --candidate yes|no --active-layer stable|candidate --next-command COMMAND [--notes TEXT] [--create] [--repo-root PATH]")
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

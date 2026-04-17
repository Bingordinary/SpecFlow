package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/entrysync"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/processcleanup"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/projectstandards"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewscope"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeRootUsage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "entry":
		return runEntry(args[1:], stdout, stderr)
	case "registry":
		return runRegistry(args[1:], stdout, stderr)
	case "review":
		return runReview(args[1:], stdout, stderr)
	case "process":
		return runProcess(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeRootUsage(stdout)
		return nil
	default:
		writeRootUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
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
		if len(inspection.StagedChanged) > 0 {
			fmt.Fprintln(stdout, "Registered entry files changed in index:")
			for _, path := range inspection.StagedChanged {
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
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		scope, err := reviewscope.CollectDefaultSpecFlowScope(mustAbs(*repoRoot))
		if err != nil {
			return err
		}

		fmt.Fprintf(stdout, "Review scenario: %s\n", scope.Scenario)
		writeList(stdout, "Framework guideline files", scope.FrameworkGuidelineFiles)
		writeList(stdout, "Command files", scope.CommandFiles)
		writeList(stdout, "Shared-governance minimum files", scope.SharedGovernanceFiles)
		writeList(stdout, "Template governance files", scope.TemplateGovernanceFiles)
		writeList(stdout, "Template entry files", scope.TemplateEntryFiles)
		writeList(stdout, "Project registry files", scope.ProjectRegistryFiles)
		writeList(stdout, "Active project-local governance-input files", scope.ActiveProjectStandardFiles)
		writeList(stdout, "Matched governance overlay files", scope.MatchedOverlayFiles)
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
		module := fs.String("module", "", "formal module name")
		fromCommand := fs.String("from-command", "", "origin command")
		reason := fs.String("reason", "", "fallback reason code")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*module) == "" || strings.TrimSpace(*fromCommand) == "" || strings.TrimSpace(*reason) == "" {
			writeProcessUsage(stderr)
			return errors.New("module, from-command, and reason are required")
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
	case "-h", "--help", "help":
		writeProcessUsage(stdout)
		return nil
	default:
		writeProcessUsage(stderr)
		return fmt.Errorf("unknown process subcommand %q", args[0])
	}
}

func writeRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl <command> <subcommand> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  entry    Check or sync registered entry-file managed blocks")
	fmt.Fprintln(w, "  registry Validate docs/project_standards/_registry.md")
	fmt.Fprintln(w, "  review   Collect deterministic governance review scope")
	fmt.Fprintln(w, "  process  Execute deterministic fallback cleanup")
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
	fmt.Fprintln(w, "  specflowctl review collect-default-scope [--repo-root PATH]")
}

func writeProcessUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl process cleanup-fallback --module MODULE --from-command COMMAND --reason CODE [--repo-root PATH]")
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

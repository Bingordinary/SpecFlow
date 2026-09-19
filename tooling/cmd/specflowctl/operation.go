package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/operation"
)

func runOperation(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeOperationUsage(stderr)
		return errors.New("missing operation subcommand")
	}

	switch args[0] {
	case "open":
		return runOperationOpen(args[1:], stdout, stderr)
	case "check":
		return runOperationCheck(args[1:], stdout, stderr)
	case "close":
		return runOperationClose(args[1:], stdout, stderr)
	case "update":
		return runOperationUpdate(args[1:], stdout, stderr)
	case "status":
		return runOperationStatus(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeOperationUsage(stdout)
		return nil
	default:
		writeOperationUsage(stderr)
		return fmt.Errorf("unknown operation subcommand %q", args[0])
	}
}

func runOperationOpen(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("operation open", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	unit := fs.String("unit", "", "target unit name")
	rule := fs.String("rule", "", "target rule id")
	baseline := fs.String("baseline", "", "git baseline ref (default HEAD)")
	parent := fs.String("parent", "", "parent operation id")
	var allow repeatedString
	var requireSpec repeatedString
	fs.Var(&allow, "allow", "declared allowed path (repeatable)")
	fs.Var(&requireSpec, "require-spec", "spec path required to change (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := operation.Open(mustAbs(*repoRoot), operation.OpenOptions{
		Unit:        *unit,
		Rule:        *rule,
		Allow:       allow,
		RequireSpec: requireSpec,
		Baseline:    *baseline,
		Parent:      *parent,
	}, time.Now().UTC())
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Operation opened: %s\n", result.Operation.OperationID)
	writeOperationSummary(stdout, result.Operation)
	for _, notice := range result.Notices {
		fmt.Fprintf(stdout, "Notice: %s\n", notice)
	}
	return nil
}

func runOperationCheck(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("operation check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	opID := fs.String("id", "", "operation id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*opID) == "" {
		writeOperationUsage(stderr)
		return errors.New("--id is required")
	}

	report, err := operation.Check(mustAbs(*repoRoot), *opID)
	if err != nil {
		return err
	}
	writeOperationReport(stdout, report)
	if report.Result != "PASS" {
		return errors.New("operation check failed")
	}
	return nil
}

func runOperationClose(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("operation close", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	opID := fs.String("id", "", "operation id")
	abandon := fs.Bool("abandon", false, "end a violating operation explicitly, recording the abandoned outcome")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*opID) == "" {
		writeOperationUsage(stderr)
		return errors.New("--id is required")
	}

	report, err := operation.Close(mustAbs(*repoRoot), *opID, *abandon, time.Now().UTC())
	if err != nil {
		return err
	}
	writeOperationReport(stdout, report)
	if report.Result != "PASS" && !*abandon {
		return errors.New("operation close refused: violations found (use --abandon to end the operation explicitly, or resolve the violations)")
	}
	outcome := report.Operation.CloseOutcome
	fmt.Fprintf(stdout, "Operation closed (%s): %s\n", outcome, report.Operation.OperationID)
	return nil
}

func runOperationUpdate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("operation update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	opID := fs.String("id", "", "operation id")
	var allow repeatedString
	var requireSpec repeatedString
	fs.Var(&allow, "allow", "additional declared allowed path (repeatable)")
	fs.Var(&requireSpec, "require-spec", "additional required spec path (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*opID) == "" {
		writeOperationUsage(stderr)
		return errors.New("--id is required")
	}

	hasAllow, hasRequireSpec := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "allow":
			hasAllow = true
		case "require-spec":
			hasRequireSpec = true
		}
	})

	op, err := operation.Update(mustAbs(*repoRoot), *opID, operation.UpdateOptions{
		Allow:          allow,
		HasAllow:       hasAllow,
		RequireSpec:    requireSpec,
		HasRequireSpec: hasRequireSpec,
	}, time.Now().UTC())
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Operation updated: %s\n", op.OperationID)
	writeOperationSummary(stdout, op)
	return nil
}

func runOperationStatus(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("operation status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	opID := fs.String("id", "", "operation id (omit to list open operations)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root := mustAbs(*repoRoot)

	if strings.TrimSpace(*opID) != "" {
		op, err := operation.Load(root, *opID)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Operation: %s\n", op.OperationID)
		writeOperationSummary(stdout, op)
		fmt.Fprintf(stdout, "Opened at: %s\n", op.OpenedAt)
		fmt.Fprintf(stdout, "Updated at: %s\n", op.UpdatedAt)
		if op.ClosedAt != "" {
			fmt.Fprintf(stdout, "Closed at: %s\n", op.ClosedAt)
		}
		if op.CloseOutcome != "" {
			fmt.Fprintf(stdout, "Close outcome: %s\n", op.CloseOutcome)
		}
		fmt.Fprintf(stdout, "Updates: %d\n", len(op.Updates))
		for _, update := range op.Updates {
			fmt.Fprintf(stdout, "  - %s\n", update.At)
			if len(update.DeclaredAllowedPaths) > 0 {
				fmt.Fprintf(stdout, "    declared scope: %s\n", strings.Join(update.DeclaredAllowedPaths, ", "))
			} else {
				fmt.Fprintln(stdout, "    declared scope: (none)")
			}
			if len(update.RequiredSpecPaths) > 0 {
				fmt.Fprintf(stdout, "    required spec paths: %s\n", strings.Join(update.RequiredSpecPaths, ", "))
			}
		}
		return nil
	}

	ops, err := operation.List(root)
	if err != nil {
		return err
	}
	open := operation.OpenOperations(ops)
	if len(open) == 0 {
		fmt.Fprintln(stdout, "No open operations.")
		return nil
	}
	fmt.Fprintf(stdout, "Open operations (%d):\n", len(open))
	for _, op := range open {
		fmt.Fprintf(stdout, "- %s | %s | baseline %s | opened %s\n",
			op.OperationID, operationTargetLabel(op.Target), shortenSHA(op.Baseline.SHA), op.OpenedAt)
	}
	return nil
}

func writeOperationSummary(stdout io.Writer, op *operation.Operation) {
	fmt.Fprintf(stdout, "Status: %s\n", op.Status)
	fmt.Fprintf(stdout, "Target: %s\n", operationTargetLabel(op.Target))
	fmt.Fprintf(stdout, "Baseline: %s (%s)\n", shortenSHA(op.Baseline.SHA), op.Baseline.Ref)
	fmt.Fprintf(stdout, "Allowed scope (%d):\n", len(op.AllowedPaths))
	if len(op.AllowedPaths) == 0 {
		fmt.Fprintln(stdout, "- none")
	}
	for _, entry := range op.AllowedPaths {
		fmt.Fprintf(stdout, "- %s  [%s]\n", entry.Path, entry.Source)
	}
	writeList(stdout, "Required spec paths", op.RequiredSpecPaths)
	if op.ParentOperation != "" {
		fmt.Fprintf(stdout, "Parent operation: %s\n", op.ParentOperation)
	}
}

func writeOperationReport(stdout io.Writer, report *operation.Report) {
	op := report.Operation
	fmt.Fprintf(stdout, "Operation: %s\n", op.OperationID)
	fmt.Fprintf(stdout, "Status: %s\n", op.Status)
	fmt.Fprintf(stdout, "Target: %s\n", operationTargetLabel(op.Target))
	fmt.Fprintf(stdout, "Baseline: %s (%s)\n", shortenSHA(op.Baseline.SHA), op.Baseline.Ref)
	fmt.Fprintf(stdout, "Changed paths: %d\n", len(report.Changed))
	writeList(stdout, "Out of scope", report.OutOfScope)
	writeList(stdout, "Static policy violations", report.StaticViolations)
	writeList(stdout, "Missing required spec paths", report.MissingRequired)
	fmt.Fprintf(stdout, "Result: %s\n", report.Result)
}

func operationTargetLabel(target operation.Target) string {
	switch target.Kind {
	case operation.TargetKindUnit:
		return "unit " + target.Name
	case operation.TargetKindRule:
		return "rule " + target.Name
	default:
		return "paths-only"
	}
}

func shortenSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func writeOperationUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl operation open (--unit NAME | --rule ID)? [--allow PATH]... [--require-spec PATH]... [--baseline REF] [--parent OP_ID] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl operation check --id OP_ID [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl operation close --id OP_ID [--abandon] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl operation update --id OP_ID [--allow PATH]... [--require-spec PATH]... [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl operation status [--id OP_ID] [--repo-root PATH]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Maintains a declared, frozen change scope for one bounded piece of work and")
	fmt.Fprintln(w, "evaluates the final working-tree change against it. State lives under")
	fmt.Fprintln(w, "meta/operations/ (local process state). Requires git; --repo-root must be")
	fmt.Fprintln(w, "the git worktree top level.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --id ID           Operation id (required for check/close/update)")
	fmt.Fprintln(w, "  --abandon         close: end a violating operation explicitly (records outcome abandoned)")
	fmt.Fprintln(w, "  --unit NAME       Target unit (scope derived from its current-layer spec)")
	fmt.Fprintln(w, "  --rule ID         Target rule (scope derived from its current-layer rule file)")
	fmt.Fprintln(w, "  --allow PATH      Explicitly declared allowed path (repeatable)")
	fmt.Fprintln(w, "  --require-spec PATH  Spec path that must appear in the change set (repeatable)")
	fmt.Fprintln(w, "  --baseline REF    Git baseline ref recorded at open (default HEAD)")
	fmt.Fprintln(w, "  --parent OP_ID    Parent operation id (lineage of a re-authorized operation)")
	fmt.Fprintln(w, "  --repo-root PATH  Repository root path (default: .)")
}

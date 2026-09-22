package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulevalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/writezone"
)

func runValidate(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		writeValidateUsage(stderr)
		return errors.New("missing validate subcommand")
	}

	switch args[0] {
	case "write":
		fs := flag.NewFlagSet("validate write", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		path := fs.String("path", "", "file path to validate")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*path) == "" {
			writeValidateUsage(stderr)
			return errors.New("path is required")
		}

		result, err := validateWrite(mustAbs(*repoRoot), *path)
		if err != nil {
			return fmt.Errorf("resolve write-zone policy: %w", err)
		}
		writeValidateWriteResult(stdout, result)
		if !result.Allowed {
			return fmt.Errorf("write denied: %s", result.Reason)
		}
		return nil
	case "candidate-frontmatter":
		fmt.Fprintln(stderr, "DEPRECATED: 'validate candidate-frontmatter' has been replaced by 'validate candidate', which is a superset.")
		// Delegate to candidate logic after deprecation notice.
		fallthrough
	case "candidate":
		fs := flag.NewFlagSet("validate candidate", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		unitNamePtr := fs.String("unit", "", "unit name")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		unitName, err := requireTargetName("unit", *unitNamePtr)
		if err != nil {
			return err
		}
		if unitName == "" {
			writeValidateUsage(stderr)
			return errors.New("unit is required")
		}

		result := specvalidation.ValidateCandidate(mustAbs(*repoRoot), unitName)
		_, err = fmt.Fprint(stdout, specvalidation.FormatResult(result))
		if err != nil {
			return err
		}
		if !result.Passed {
			return fmt.Errorf("validate candidate failed")
		}
		return nil
	case "rule":
		fs := flag.NewFlagSet("validate rule", flag.ContinueOnError)
		fs.SetOutput(stderr)
		repoRoot := fs.String("repo-root", ".", "repository root")
		ruleIDPtr := fs.String("id", "", "rule id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		ruleID, err := requireTargetName("rule", *ruleIDPtr)
		if err != nil {
			return err
		}
		if ruleID == "" {
			writeValidateUsage(stderr)
			return errors.New("--id is required")
		}

		result := rulevalidation.ValidateRule(mustAbs(*repoRoot), ruleID)
		_, err = fmt.Fprint(stdout, rulevalidation.FormatResult(result))
		if err != nil {
			return err
		}
		if !result.Passed {
			return fmt.Errorf("validate rule failed")
		}
		return nil
	case "-h", "--help", "help":
		writeValidateUsage(stdout)
		return nil
	default:
		writeValidateUsage(stderr)
		return fmt.Errorf("unknown validate subcommand %q", args[0])
	}
}

type validateResult = writezone.Result

func validateWrite(repoRoot, path string) (validateResult, error) {
	classifier, err := writezone.New(repoRoot)
	if err != nil {
		return validateResult{}, err
	}
	return classifier.Classify(path), nil
}

func writeValidateWriteResult(stdout io.Writer, result validateResult) {
	fmt.Fprintf(stdout, "allowed: %t\n", result.Allowed)
	fmt.Fprintf(stdout, "reason: %s\n", result.Reason)
	fmt.Fprintf(stdout, "path: %s\n", noneIfEmpty(result.Path))
}

func writeValidateUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl validate write --path PATH [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl validate candidate --unit UNIT [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl validate rule --id RULE_ID [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Validate write checks if a file path is in an allowed write zone.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Validate candidate runs the full validation on a candidate spec:")
	fmt.Fprintln(w, "  1. Frontmatter completeness")
	fmt.Fprintln(w, "  2. Acceptance items format")
	fmt.Fprintln(w, "  3. Anchor integrity (affects.files paths exist; implementation_surface values resolve)")
	fmt.Fprintln(w, "  4. Reference integrity (unit_refs/rule_refs)")
	fmt.Fprintln(w, "  5. Appendix files")
	fmt.Fprintln(w, "  6. Version/ref consistency")
	fmt.Fprintln(w, "  7. Body layer-path check (candidate-layer spec paths)")
	fmt.Fprintln(w, "  8. Dependency cycle check (unit_refs graph — a unit on a cycle FAILs)")
	fmt.Fprintln(w, "  9. Region locatability (section and acceptance item regions resolve unambiguously)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Validate rule runs mechanical checks on a candidate rule:")
	fmt.Fprintln(w, "  1. Frontmatter completeness")
	fmt.Fprintln(w, "  2. ID/Scope consistency")
	fmt.Fprintln(w, "  3. Version semantics")
	fmt.Fprintln(w, "  4. promotion_owner_unit (warning)")
	fmt.Fprintln(w, "  5. Prohibited fields")
	fmt.Fprintln(w, "  6. unbound_retention correctness")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Checks 3 (File Path Consistency) and 8 (Rule Body Quality) are agent-only and not covered by this command.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --path PATH     File path to validate (required for 'write')")
	fmt.Fprintln(w, "  --unit UNIT     Unit name (required for 'candidate')")
	fmt.Fprintln(w, "  --id RULE_ID    Rule id (required for 'rule')")
	fmt.Fprintln(w, "  --repo-root PATH Repository root directory (default: current directory)")
}

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulevalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
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

		result := validateWrite(mustAbs(*repoRoot), *path)
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
		unitName := fs.String("unit", "", "unit name")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*unitName) == "" {
			writeValidateUsage(stderr)
			return errors.New("unit is required")
		}

		result := specvalidation.ValidateCandidate(mustAbs(*repoRoot), *unitName)
		_, err := fmt.Fprint(stdout, specvalidation.FormatResult(result))
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
		ruleID := fs.String("id", "", "rule id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*ruleID) == "" {
			writeValidateUsage(stderr)
			return errors.New("--id is required")
		}

		result := rulevalidation.ValidateRule(mustAbs(*repoRoot), *ruleID)
		_, err := fmt.Fprint(stdout, rulevalidation.FormatResult(result))
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

type validateResult struct {
	Allowed bool
	Reason  string
	Path    string
}

func validateWrite(repoRoot, path string) validateResult {
	// Resolve the path to its real filesystem location before zone matching:
	// relative paths are resolved against the process working directory, so
	// a relative path that resolves into a governed zone cannot fall through
	// to the default allowed branch. If the working directory cannot be
	// determined, fall back to matching the raw relative path.
	if !filepath.IsAbs(path) {
		if cwd, err := os.Getwd(); err == nil {
			path = filepath.Join(cwd, path)
		}
	}
	// Normalize symlinks so relative-path resolution (e.g. /var -> /private/var
	// on macOS) does not break the repo-relative conversion below. Paths that
	// do not exist yet keep their cleaned form.
	resolvedRoot := repoRoot
	if r, err := filepath.EvalSymlinks(repoRoot); err == nil {
		resolvedRoot = r
	}
	resolvedPath := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		resolvedPath = r
	}
	normalizedPath := filepath.ToSlash(filepath.Clean(resolvedPath))
	if filepath.IsAbs(resolvedPath) {
		rel, err := filepath.Rel(resolvedRoot, resolvedPath)
		if err != nil || escapesRoot(rel) {
			// The path could not be symlink-resolved (e.g. it does not exist
			// yet): retry against the un-resolved root so the prefix match
			// stays consistent with the caller-provided repo root.
			rel, err = filepath.Rel(repoRoot, filepath.Clean(path))
		}
		if err == nil && !escapesRoot(rel) {
			normalizedPath = filepath.ToSlash(rel)
		}
	}

	// Deny pattern: framework files are never writable via validate
	if strings.HasPrefix(normalizedPath, "framework/") {
		return validateResult{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q is under framework/ and is not writable", path),
			Path:    path,
		}
	}

	// Deny pattern: stable spec files are not directly writable (use promote)
	if strings.HasPrefix(normalizedPath, "docs/specs/units/stable/") {
		return validateResult{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q is under docs/specs/units/stable/; use promote to write stable specs", path),
			Path:    path,
		}
	}

	// Deny pattern: rule files are not directly writable (use rule governance flows)
	if strings.HasPrefix(normalizedPath, "docs/specs/rules/stable/") {
		return validateResult{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q is under docs/specs/rules/stable/; use rule governance flows", path),
			Path:    path,
		}
	}

	// Candidate spec files are writable
	if strings.HasPrefix(normalizedPath, "docs/specs/units/candidate/") {
		return validateResult{
			Allowed: true,
			Reason:  fmt.Sprintf("path %q is a candidate spec file and is writable", path),
			Path:    path,
		}
	}

	// Candidate rule files are writable
	if strings.HasPrefix(normalizedPath, "docs/specs/rules/candidate/") {
		return validateResult{
			Allowed: true,
			Reason:  fmt.Sprintf("path %q is a candidate rule file and is writable", path),
			Path:    path,
		}
	}

	// Source code files are writable by default
	return validateResult{
		Allowed: true,
		Reason:  fmt.Sprintf("path %q is not governed by specFlow write restrictions", path),
		Path:    path,
	}
}

// escapesRoot reports whether a repository-relative path escapes the
// repository root (e.g. ".." or "../other"). Paths that escape the root
// are outside the specFlow write-zone contract and must keep the default
// "not governed" result instead of being matched against write-zone prefixes.
func escapesRoot(rel string) bool {
	rel = filepath.Clean(rel)
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	fmt.Fprintln(w, "  3. Anchor integrity (affects.files paths)")
	fmt.Fprintln(w, "  4. Reference integrity (unit_refs/rule_refs)")
	fmt.Fprintln(w, "  5. Appendix files")
	fmt.Fprintln(w, "  6. Version/ref consistency")
	fmt.Fprintln(w, "  7. Body layer-path check (candidate-layer spec paths)")
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

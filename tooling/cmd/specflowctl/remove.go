package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/baseline"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/ruledetect"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// runRemove implements `specflowctl remove --rule <id>`: the deletion path
// for a rule whose constraint no longer applies. It reuses the detection
// primitive as final verification — the rule is rejected while any
// current-layer (effective) unit still references it in rule_refs (the
// referrers are listed), and while it declares unbound_retention
// (intentional retention). On success the rule's stable copy (and candidate
// copy if present) is deleted, followed by its baseline and validate cache.
func runRemove(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	ruleIDPtr := fs.String("rule", "", "rule id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ruleID := strings.TrimSpace(*ruleIDPtr)
	if ruleID == "" {
		writeRemoveUsage(stderr)
		return errors.New("--rule is required")
	}

	absRoot := mustAbs(*repoRootPtr)

	result, err := ruledetect.DetectRule(absRoot, ruleID)
	if err != nil {
		// A rule with no file in either layer has nothing left to protect:
		// the consumer and retention checks have no object. Degrade to
		// metadata cleanup so `remove` stays re-entrant as the recovery path
		// after a partial deletion (its own failure hints and the promote
		// Step 8 hints both point here).
		if errors.Is(err, ruledetect.ErrRuleNotFound) {
			fmt.Fprintf(stdout, "Rule %s has no file in docs/specs/rules/ (candidate or stable) — nothing to protect, cleaning up residual metadata.\n", ruleID)
			return removeRuleMetadata(absRoot, ruleID, stdout, stderr)
		}
		return err
	}

	if len(result.Consumers) > 0 {
		fmt.Fprintf(stdout, "Removal rejected: %s is still referenced by %d unit(s):\n", ruleID, len(result.Consumers))
		for _, c := range result.Consumers {
			fmt.Fprintf(stdout, "  - %s\n", c)
		}
		fmt.Fprintln(stdout, "Remove the references (or promote the units that dropped them) before removing the rule.")
		return errors.New("rule still has consumers")
	}

	if result.UnboundRetention {
		fmt.Fprintf(stdout, "Removal rejected: %s declares unbound_retention (intentional retention).\n", ruleID)
		fmt.Fprintln(stdout, "Remove the unbound_retention fields from the rule frontmatter to allow removal.")
		return errors.New("rule is retained by declaration")
	}

	// Delete the stable copy first (the semantic act), then the candidate
	// copy if one exists. A failure after a file removal reports the concrete
	// recovery path instead of claiming success.
	if result.HasStable {
		stablePath := filepath.Join(absRoot, filepath.FromSlash(specpaths.RuleStableFileRef(ruleID)))
		if err := os.Remove(stablePath); err != nil {
			return fmt.Errorf("failed to remove stable rule %s: %w", ruleID, err)
		}
		fmt.Fprintf(stdout, "Removed: %s\n", specpaths.RuleStableFileRef(ruleID))
	}
	if result.HasCandidate {
		candidatePath := filepath.Join(absRoot, filepath.FromSlash(specpaths.RuleCandidateFileRef(ruleID)))
		if err := os.Remove(candidatePath); err != nil {
			fmt.Fprintf(stderr, "Warning: failed to remove candidate rule %s: %v\n", ruleID, err)
			fmt.Fprintln(stderr, "The stable copy is already removed; delete docs/specs/rules/candidate/"+ruleID+".md manually.")
			return err
		}
		fmt.Fprintf(stdout, "Removed: %s\n", specpaths.RuleCandidateFileRef(ruleID))
	}

	return removeRuleMetadata(absRoot, ruleID, stdout, stderr)
}

// removeRuleMetadata deletes the rule's baseline and validate cache — the
// cleanup chain shared by the full removal path and the degraded path taken
// when the rule files no longer exist (a partial deletion or a typo'd id:
// nothing is left to protect, so metadata cleanup is safe and idempotent).
func removeRuleMetadata(absRoot, ruleID string, stdout, stderr io.Writer) error {
	if err := baseline.RemoveBaseline(absRoot, "rule", ruleID); err != nil {
		fmt.Fprintf(stderr, "Warning: failed to remove baseline: %v\n", err)
		fmt.Fprintln(stderr, "Remove docs/specs/meta/baseline/rule/"+ruleID+".yaml manually.")
		return err
	}
	if err := validationcache.DeleteRuleCache(absRoot, ruleID, "validate"); err != nil {
		fmt.Fprintf(stderr, "Warning: failed to remove validate cache: %v\n", err)
		fmt.Fprintln(stderr, "Remove docs/specs/meta/validation/rule/"+ruleID+"/ manually.")
		return err
	}
	fmt.Fprintln(stdout, "Baseline and validate cache removed.")

	fmt.Fprintf(stdout, "Rule %s removed. Git history is the only record.\n", ruleID)
	return nil
}

func writeRemoveUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl remove --rule RULE_ID [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Deletes a rule whose constraint no longer applies. Bound rules (b_rule_*):")
	fmt.Fprintln(w, "the final verification reuses the detection primitive — the rule is rejected")
	fmt.Fprintln(w, "while any current-layer (effective) unit still references it in rule_refs")
	fmt.Fprintln(w, "(the referrers are listed), and while it declares unbound_retention")
	fmt.Fprintln(w, "(intentional retention). On success the stable copy (and candidate copy")
	fmt.Fprintln(w, "if present) is deleted, followed by the rule's baseline and validate cache.")
	fmt.Fprintln(w, "Global rules (g_rule_*) are checked on explicit references only — their")
	fmt.Fprintln(w, "default applicability to every unit lifts automatically with the file.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --rule RULE_ID  Rule id to remove (required)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

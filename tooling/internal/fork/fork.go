package fork

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fileops"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

type Result struct {
	Unit    string
	Passed  bool
	Issues  []string
	Actions []string
}

type RuleResult struct {
	RuleID     string
	Passed     bool
	Issues     []string
	Actions    []string
	StableRule string
}

func Fork(repoRoot, unitName string) *Result {
	r := &Result{Unit: unitName}

	stableSpec := specpaths.StableUnitSpecFileRef(unitName)
	candidateSpec := specpaths.CandidateUnitSpecFileRef(unitName)
	stableSpecPath := filepath.Join(repoRoot, stableSpec)
	candidateSpecPath := filepath.Join(repoRoot, candidateSpec)

	if _, err := os.Stat(stableSpecPath); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Stable spec not found: %s", stableSpec))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found stable spec: %s", stableSpec))

	if _, err := os.Stat(candidateSpecPath); err == nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate already exists: %s — edit the existing candidate directly", candidateSpec))
		r.Passed = false
		return r
	}

	appendixCopies, skipped, err := specpaths.PlanUnitAppendixCopies(repoRoot, unitName, "stable")
	if err != nil {
		r.Issues = append(r.Issues, err.Error())
		return r
	}

	for _, path := range skipped {
		r.Actions = append(r.Actions, fmt.Sprintf("Skipped exempt appendix: %s", path))
	}
	for _, appendix := range appendixCopies {
		r.Actions = append(r.Actions, fmt.Sprintf("Found appendix: %s", appendix.Source))
	}

	if err := fileops.CopyFile(stableSpecPath, candidateSpecPath); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy spec: %v", err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Forked: %s -> %s", stableSpec, candidateSpec))

	for _, appendix := range appendixCopies {
		src := filepath.Join(repoRoot, filepath.FromSlash(appendix.Source))
		dst := filepath.Join(repoRoot, filepath.FromSlash(appendix.Destination))
		if err := fileops.CopyFile(src, dst); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy appendix: %v", err))
			r.Passed = false
			return r
		}
		r.Actions = append(r.Actions, fmt.Sprintf("Forked appendix: %s", appendix.Destination))
	}

	// Inherit the stable confirmation caches: fork copies the stable content
	// verbatim, so pass confirmation conclusions carry over to the candidate
	// round (rewritten to the candidate layer) and stay valid until the round's
	// edits stale their evidence. Skipped gates need their full run;
	// inheritance errors are non-fatal — the fork itself succeeded, and a
	// missing baseline is a safe degradation (the gate re-runs in full).
	inheritReport, err := validationcache.InheritStableCaches(repoRoot, unitName)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to inherit confirmation caches: %v (gates need their full runs)", err))
	} else {
		for _, e := range inheritReport.Entries {
			if e.Inherited {
				r.Actions = append(r.Actions, fmt.Sprintf("Inherited %s confirmation cache (rewritten to candidate layer)", e.Command))
			} else {
				r.Actions = append(r.Actions, fmt.Sprintf("%s: %s", e.Command, e.Reason))
			}
		}
	}

	r.Passed = true
	return r
}

func ForkRule(repoRoot, ruleID string) *RuleResult {
	r := &RuleResult{RuleID: ruleID}

	stableRule := specpaths.RuleStableFileRef(ruleID)
	candidateRule := specpaths.RuleCandidateFileRef(ruleID)
	stableRulePath := filepath.Join(repoRoot, stableRule)
	candidateRulePath := filepath.Join(repoRoot, candidateRule)
	r.StableRule = stableRule

	if _, err := os.Stat(stableRulePath); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Stable rule not found: %s", stableRule))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found stable rule: %s", stableRule))

	if _, err := os.Stat(candidateRulePath); err == nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate already exists: %s — edit the existing candidate directly", candidateRule))
		r.Passed = false
		return r
	}

	if err := fileops.CopyFile(stableRulePath, candidateRulePath); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy rule: %v", err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Forked: %s -> %s", stableRule, candidateRule))

	r.Passed = true
	return r
}

func FormatResult(r *Result) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "Unit: %s\n", r.Unit)
	if r.Passed {
		buf.WriteString("Result: PASSED\n\n")
	} else {
		buf.WriteString("Result: FAILED\n\n")
	}
	if len(r.Issues) > 0 {
		buf.WriteString("Issues:\n")
		for _, i := range r.Issues {
			fmt.Fprintf(&buf, "  - %s\n", i)
		}
		buf.WriteString("\n")
	}
	if len(r.Actions) > 0 {
		buf.WriteString("Actions:\n")
		for _, a := range r.Actions {
			fmt.Fprintf(&buf, "  - %s\n", a)
		}
		buf.WriteString("\n")
	}
	if r.Passed {
		buf.WriteString("Candidate spec created from stable. Edit the candidate to begin working.\n")
	} else {
		buf.WriteString("Fork failed. Fix the issues above and try again.\n")
	}
	return buf.String()
}

func FormatRuleResult(r *RuleResult) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "Rule: %s\n", r.RuleID)
	if r.Passed {
		buf.WriteString("Result: PASSED\n\n")
	} else {
		buf.WriteString("Result: FAILED\n\n")
	}
	if len(r.Issues) > 0 {
		buf.WriteString("Issues:\n")
		for _, i := range r.Issues {
			fmt.Fprintf(&buf, "  - %s\n", i)
		}
		buf.WriteString("\n")
	}
	if len(r.Actions) > 0 {
		buf.WriteString("Actions:\n")
		for _, a := range r.Actions {
			fmt.Fprintf(&buf, "  - %s\n", a)
		}
		buf.WriteString("\n")
	}
	if r.Passed {
		buf.WriteString("Candidate rule created from stable. Edit the candidate to begin working.\n")
	} else {
		buf.WriteString("Fork failed. Fix the issues above and try again.\n")
	}
	return buf.String()
}



package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// gateStatus classifies the freshness of a single gate. The vocabulary is
// shared between the summary and detail output modes:
//
//	FRESH   cache exists and satisfies the gate (hashes match, mode full, etc.)
//	STALE   cache exists but the gate is not satisfied (files changed,
//	        coverage incomplete, mode/result invalid) — re-running fixes it
//	MISSING cache file does not exist (never run, or run failed and cache deleted)
//	BLOCKED cache exists but declares blocking findings (review P0/P1)
//	OK      appendix gate: every appendix is covered by the validate cache
type gateStatus string

const (
	gateFresh   gateStatus = "FRESH"
	gateStale   gateStatus = "STALE"
	gateMissing gateStatus = "MISSING"
	gateBlocked gateStatus = "BLOCKED"
	gateOK      gateStatus = "OK"
)

func runFresh(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fresh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	unitPtr := fs.String("unit", "", "unit name")
	ruleIDPtr := fs.String("rule", "", "rule id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot := mustAbs(*repoRootPtr)
	unitName := strings.TrimSpace(*unitPtr)
	ruleID := strings.TrimSpace(*ruleIDPtr)

	if unitName != "" && ruleID != "" {
		writeFreshUsage(stderr)
		return errors.New("--unit and --rule are mutually exclusive")
	}

	switch {
	case unitName != "":
		return writeUnitFreshDetail(stdout, absRoot, unitName)
	case ruleID != "":
		return writeRuleFreshDetail(stdout, absRoot, ruleID)
	default:
		return writeAllFresh(stdout, absRoot)
	}
}

// ------------------------------------------------------------
// Summary mode (specflowctl fresh — all candidates)
// ------------------------------------------------------------

func writeAllFresh(stdout io.Writer, absRoot string) error {
	unitNames, err := candidateUnitNames(absRoot)
	if err != nil {
		return err
	}
	ruleIDs, err := candidateRuleIDs(absRoot)
	if err != nil {
		return err
	}

	if len(unitNames) == 0 && len(ruleIDs) == 0 {
		fmt.Fprintln(stdout, "No active candidates found.")
		return nil
	}

	fmt.Fprintf(stdout, "FRESHNESS REPORT (%s)\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintln(stdout)

	readyCount := 0
	total := 0

	if len(unitNames) > 0 {
		fmt.Fprintf(stdout, "UNITS (%d):\n", len(unitNames))
		for _, name := range unitNames {
			total++
			line, ready := unitSummaryLine(absRoot, name)
			if ready {
				readyCount++
			}
			fmt.Fprintf(stdout, "  %s\n", line)
		}
		fmt.Fprintln(stdout)
	}

	if len(ruleIDs) > 0 {
		fmt.Fprintf(stdout, "RULES (%d):\n", len(ruleIDs))
		for _, id := range ruleIDs {
			total++
			line, ready := ruleSummaryLine(absRoot, id)
			if ready {
				readyCount++
			}
			fmt.Fprintf(stdout, "  %s\n", line)
		}
		fmt.Fprintln(stdout)
	}

	fmt.Fprintf(stdout, "READY FOR PROMOTE: %d of %d\n", readyCount, total)
	return nil
}

func unitSummaryLine(repoRoot, unitName string) (string, bool) {
	if isRetiringUnit(repoRoot, unitName) {
		vStatus, _ := checkUnitGate(repoRoot, unitName, "validate")
		ready := gatePassed(vStatus)
		return fmt.Sprintf("%-13s (retiring)  validate: %-8s  READY: %t",
			unitName, vStatus, ready), ready
	}

	vStatus, _ := checkUnitGate(repoRoot, unitName, "validate")
	vfStatus, _ := checkUnitGate(repoRoot, unitName, "verify")
	rStatus, _ := checkUnitGate(repoRoot, unitName, "review")
	aStatus, _ := checkAppendixGate(repoRoot, unitName)

	ready := gatePassed(vStatus) && gatePassed(vfStatus) && gatePassed(rStatus) && gatePassed(aStatus)
	return fmt.Sprintf("%-13s  validate: %-8s  verify: %-8s  review: %-8s  appendix: %-4s  READY: %t",
		unitName, vStatus, vfStatus, rStatus, aStatus, ready), ready
}

func ruleSummaryLine(repoRoot, ruleID string) (string, bool) {
	vStatus, _ := checkRuleGate(repoRoot, ruleID)
	ready := gatePassed(vStatus)
	return fmt.Sprintf("%-13s  validate: %-8s  READY: %t",
		ruleID, vStatus, ready), ready
}

// ------------------------------------------------------------
// Detail mode (specflowctl fresh --unit / --rule)
// ------------------------------------------------------------

func writeUnitFreshDetail(stdout io.Writer, absRoot, unitName string) error {
	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (unit)\n\n", unitName)

	nonFresh := 0

	vStatus, vDetail := checkUnitGate(absRoot, unitName, "validate")
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "unit", unitName, "validate_result.md"))
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vStatus, vDetail)

	if isRetiringUnit(absRoot, unitName) {
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Unit is retiring — verify, review, and appendix gates are skipped (matching promote).")
		if gatePassed(vStatus) {
			fmt.Fprintln(stdout, "READY FOR PROMOTE: yes")
		} else {
			fmt.Fprintln(stdout, "READY FOR PROMOTE: no — validate needs attention")
		}
		return nil
	}

	vfStatus, vfDetail := checkUnitGate(absRoot, unitName, "verify")
	if vfStatus == gateFresh {
		vfDetail = freshDetail(readSummary(absRoot, "unit", unitName, "verify_result.md"))
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "verify", vfStatus, vfDetail)

	rStatus, rDetail := checkUnitGate(absRoot, unitName, "review")
	if rStatus == gateFresh {
		rDetail = freshDetail(readSummary(absRoot, "unit", unitName, "review_result.md"))
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "review", rStatus, rDetail)

	aStatus, aDetail := checkAppendixGate(absRoot, unitName)
	if aStatus != gateOK {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "appendix", aStatus, aDetail)

	fmt.Fprintln(stdout)
	if nonFresh == 0 {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: yes\n")
	} else {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: no — %d gate(s) need attention\n", nonFresh)
	}
	return nil
}

func writeRuleFreshDetail(stdout io.Writer, absRoot, ruleID string) error {
	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (rule)\n\n", ruleID)

	vStatus, vDetail := checkRuleGate(absRoot, ruleID)
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "rule", ruleID, "validate_result.md"))
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vStatus, vDetail)
	fmt.Fprintln(stdout, "verify and review do not apply to rules.")

	fmt.Fprintln(stdout)
	if gatePassed(vStatus) {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: yes\n")
	} else {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: no — validate needs attention\n")
	}
	return nil
}

// ------------------------------------------------------------
// Gate checks
// ------------------------------------------------------------

// checkUnitGate classifies one of the unit gates (validate/verify/review)
// and returns the promote-identical reason text.
func checkUnitGate(repoRoot, unitName, command string) (gateStatus, string) {
	var (
		result validationcache.CheckResult
		err    error
	)
	switch command {
	case "validate":
		result, err = validationcache.CheckValidate(repoRoot, unitName)
	case "verify":
		result, err = validationcache.CheckVerify(repoRoot, unitName)
	case "review":
		result, err = validationcache.CheckReview(repoRoot, unitName)
	default:
		return gateStale, fmt.Sprintf("unknown gate %q", command)
	}
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err)
	}
	return classifyGate(result), result.Reason
}

func checkRuleGate(repoRoot, ruleID string) (gateStatus, string) {
	result, err := validationcache.CheckRuleValidate(repoRoot, ruleID)
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err)
	}
	return classifyGate(result), result.Reason
}

func checkAppendixGate(repoRoot, unitName string) (gateStatus, string) {
	result, err := validationcache.CheckAppendicesInCache(repoRoot, unitName)
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err)
	}
	switch result.Category {
	case validationcache.CategoryMissing:
		return gateMissing, result.Reason
	case validationcache.CategoryFresh:
		return gateOK, result.Reason
	default:
		return gateStale, result.Reason
	}
}

// classifyGate maps the category recorded by the cache check chain to a
// gate status. The classification is decided by the same checks promote
// runs, so a fresh report and a promote run never disagree.
func classifyGate(result validationcache.CheckResult) gateStatus {
	switch result.Category {
	case validationcache.CategoryMissing:
		return gateMissing
	case validationcache.CategoryBlocked:
		return gateBlocked
	case validationcache.CategoryStale:
		return gateStale
	case validationcache.CategoryFresh:
		return gateFresh
	default:
		if result.Fresh {
			return gateFresh
		}
		return gateStale
	}
}

func gatePassed(status gateStatus) bool {
	return status == gateFresh || status == gateOK
}

// freshDetail renders the frontmatter summary of a fresh cache.
func freshDetail(summary *validationcache.CacheSummary) string {
	if summary == nil {
		return "cache is fresh"
	}
	return fmt.Sprintf("result: %s · mode: %s · %d file(s) · %s",
		summary.Result, summary.Mode, summary.FileCount, noneIfEmpty(summary.Timestamp))
}

func readSummary(repoRoot, targetKind, targetName, fileName string) *validationcache.CacheSummary {
	summary, err := validationcache.ReadCacheSummary(repoRoot, targetKind, targetName, fileName)
	if err != nil {
		return nil
	}
	return summary
}

// ------------------------------------------------------------
// Candidate discovery
// ------------------------------------------------------------

func candidateUnitNames(repoRoot string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_*.md"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, m := range matches {
		names = append(names, strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "unit_"), ".md"))
	}
	sort.Strings(names)
	return names, nil
}

func candidateRuleIDs(repoRoot string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(repoRoot, "docs/specs/rules/candidate/*.md"))
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, m := range matches {
		ids = append(ids, strings.TrimSuffix(filepath.Base(m), ".md"))
	}
	sort.Strings(ids)
	return ids, nil
}

func isRetiringUnit(repoRoot, unitName string) bool {
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/units/candidate", fmt.Sprintf("unit_%s.md", unitName)))
	if err != nil {
		return false
	}
	fm := specpaths.ReadFrontmatterStringMap(string(data))
	return strings.TrimSpace(fm["status"]) == "retired"
}

func writeFreshUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl fresh [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl fresh --unit UNIT [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl fresh --rule RULE_ID [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Reports cache freshness for all active candidates (no flags),")
	fmt.Fprintln(w, "or for a single unit/rule target. Read-only: never writes or")
	fmt.Fprintln(w, "deletes caches, never runs validate/verify/review.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --unit UNIT      Unit name for single-target report")
	fmt.Fprintln(w, "  --rule RULE_ID   Rule id for single-target report")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

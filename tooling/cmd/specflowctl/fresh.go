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

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/baseline"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/ruledetect"
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
	scopePtr := fs.String("scope", "candidate", "scope: candidate | stable | all")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot := mustAbs(*repoRootPtr)
	unitName := strings.TrimSpace(*unitPtr)
	ruleID := strings.TrimSpace(*ruleIDPtr)
	scope := strings.TrimSpace(strings.ToLower(*scopePtr))

	switch scope {
	case "candidate", "stable", "all":
	default:
		return fmt.Errorf("invalid --scope %q: must be candidate, stable, or all", scope)
	}

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
		return writeAllFresh(stdout, absRoot, scope)
	}
}

// ------------------------------------------------------------
// Summary mode (specflowctl fresh [--scope ...])
// ------------------------------------------------------------

func writeAllFresh(stdout io.Writer, absRoot, scope string) error {
	if scope == "candidate" || scope == "all" {
		if err := writeCandidateFreshSection(stdout, absRoot); err != nil {
			return err
		}
	}
	if scope == "stable" || scope == "all" {
		if err := writeStableFreshSection(stdout, absRoot); err != nil {
			return err
		}
	}
	return writeUnboundRulesSection(stdout, absRoot)
}

// writeCandidateFreshSection reports the cache freshness of every active
// candidate (the promote-readiness view).
func writeCandidateFreshSection(stdout io.Writer, absRoot string) error {
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

// writeStableFreshSection reports the confirmation and drift state of every
// stable target. Stable targets have no promote gate — the report shows the
// three confirmation states (validate: dependencies/rules, verify: code
// alignment, review: code quality) plus the baseline drift comparison
// (OK / CHANGED / MISSING).
func writeStableFreshSection(stdout io.Writer, absRoot string) error {
	unitNames, err := stableUnitNames(absRoot)
	if err != nil {
		return err
	}
	ruleIDs, err := stableRuleIDs(absRoot)
	if err != nil {
		return err
	}

	if len(unitNames) == 0 && len(ruleIDs) == 0 {
		fmt.Fprintln(stdout, "No stable targets found.")
		return nil
	}

	if len(unitNames) > 0 {
		fmt.Fprintf(stdout, "STABLE UNITS (%d):\n", len(unitNames))
		for _, name := range unitNames {
			fmt.Fprintf(stdout, "  %s\n", stableUnitSummaryLine(absRoot, name))
		}
		fmt.Fprintln(stdout)
	}

	if len(ruleIDs) > 0 {
		fmt.Fprintf(stdout, "STABLE RULES (%d):\n", len(ruleIDs))
		for _, id := range ruleIDs {
			fmt.Fprintf(stdout, "  %s\n", stableRuleSummaryLine(absRoot, id))
		}
		fmt.Fprintln(stdout)
	}

	return nil
}

// writeUnboundRulesSection embeds the removal-candidate list — bound rules
// (b_rule_*) with no current-layer (effective) consumers and no
// unbound_retention declaration — at the end of every fresh summary report
// (candidate, stable, and all scopes). The list is layer-independent:
// removability is decided by consumers and the retention declaration alone,
// not by which layer holds the rule file, so every summary shows the same
// complete list exactly once. Read-only: removal always happens through
// `specflowctl remove --rule`, never here.
func writeUnboundRulesSection(stdout io.Writer, absRoot string) error {
	results, err := ruledetect.DetectAll(absRoot)
	if err != nil {
		return err
	}
	var removable []ruledetect.DetectResult
	for _, r := range results {
		if r.Removable {
			removable = append(removable, r)
		}
	}
	if len(removable) == 0 {
		return nil
	}
	fmt.Fprintf(stdout, "RULES WITHOUT CONSUMERS (removable candidates, %d):\n", len(removable))
	for _, r := range removable {
		fmt.Fprintf(stdout, "  %s\n", r.RuleID)
	}
	fmt.Fprintln(stdout, "  Removal is never automatic — run `specflowctl remove --rule <id>` to delete.")
	fmt.Fprintln(stdout)
	return nil
}

// stableUnitSummaryLine reports one stable unit's confirmation and drift
// state. The three confirmation columns (validate/verify/review) come from
// the stable-layer caches written by the corresponding @stable runs; the
// drift column is the mechanical baseline comparison. A fresh verify cache
// means the code was recently confirmed to still conform even when the
// baseline surface differs.
func stableUnitSummaryLine(repoRoot, unitName string) string {
	vaStatus, _, _ := checkStableUnitGate(repoRoot, unitName, "validate")
	vfStatus, _, _ := checkStableUnitGate(repoRoot, unitName, "verify")
	rStatus, _, _ := checkStableUnitGate(repoRoot, unitName, "review")
	return fmt.Sprintf("%-13s  validate: %-8s  verify: %-8s  review: %-8s  drift: %-8s",
		unitName, vaStatus, vfStatus, rStatus, stableDriftLabel(repoRoot, unitName, baseline.CheckUnitBaseline(repoRoot, unitName)))
}

func stableRuleSummaryLine(repoRoot, ruleID string) string {
	vStatus, _, _ := checkStableRuleGate(repoRoot, ruleID)
	return fmt.Sprintf("%-13s  validate: %-8s  drift: %-8s",
		ruleID, vStatus, stableDriftLabel(repoRoot, ruleID, baseline.CheckRuleBaseline(repoRoot, ruleID)))
}

// stableDriftLabel maps a baseline check status to the drift column label.
func stableDriftLabel(repoRoot, name string, result baseline.CheckResult) string {
	switch result.Status {
	case baseline.StatusOK:
		return "OK"
	case baseline.StatusChanged:
		return "CHANGED"
	default:
		return "MISSING"
	}
}

func unitSummaryLine(repoRoot, unitName string) (string, bool) {
	if isRetiringUnit(repoRoot, unitName) {
		vStatus, _, _ := checkUnitGate(repoRoot, unitName, "validate")
		ready := gatePassed(vStatus)
		return fmt.Sprintf("%-13s (retiring)  validate: %-8s  READY: %t",
			unitName, vStatus, ready), ready
	}

	vStatus, _, _ := checkUnitGate(repoRoot, unitName, "validate")
	vfStatus, _, _ := checkUnitGate(repoRoot, unitName, "verify")
	rStatus, _, _ := checkUnitGate(repoRoot, unitName, "review")
	aStatus, _ := checkAppendixGate(repoRoot, unitName)

	ready := gatePassed(vStatus) && gatePassed(vfStatus) && gatePassed(rStatus) && gatePassed(aStatus)
	return fmt.Sprintf("%-13s  validate: %-8s  verify: %-8s  review: %-8s  appendix: %-4s  READY: %t",
		unitName, vStatus, vfStatus, rStatus, aStatus, ready), ready
}

func ruleSummaryLine(repoRoot, ruleID string) (string, bool) {
	vStatus, _, _ := checkRuleGate(repoRoot, ruleID)
	ready := gatePassed(vStatus)
	return fmt.Sprintf("%-13s  validate: %-8s  READY: %t",
		ruleID, vStatus, ready), ready
}

// ------------------------------------------------------------
// Detail mode (specflowctl fresh --unit / --rule)
// ------------------------------------------------------------

func writeUnitFreshDetail(stdout io.Writer, absRoot, unitName string) error {
	// A stable-only unit has no candidate round — report its drift state
	// instead of candidate gate statuses.
	if _, err := os.Stat(filepath.Join(absRoot, "docs/specs/units/stable", "unit_"+unitName+".md")); err == nil {
		if _, err := os.Stat(filepath.Join(absRoot, "docs/specs/units/candidate", "unit_"+unitName+".md")); os.IsNotExist(err) {
			return writeUnitStableFreshDetail(stdout, absRoot, unitName)
		}
	}

	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (unit)\n\n", unitName)

	nonFresh := 0

	vStatus, vDetail, vNote := checkUnitGate(absRoot, unitName, "validate")
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "unit", unitName, "validate_result.md"))
		if vNote != "" {
			vDetail += " | " + vNote
		}
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vStatus, vDetail)
	if advice := gateAdvice("validate", vStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

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

	vfStatus, vfDetail, vfNote := checkUnitGate(absRoot, unitName, "verify")
	if vfStatus == gateFresh {
		vfDetail = freshDetail(readSummary(absRoot, "unit", unitName, "verify_result.md"))
		if vfNote != "" {
			vfDetail += " | " + vfNote
		}
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "verify", vfStatus, vfDetail)
	if advice := gateAdvice("verify", vfStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	rStatus, rDetail, rNote := checkUnitGate(absRoot, unitName, "review")
	if rStatus == gateFresh {
		rDetail = freshDetail(readSummary(absRoot, "unit", unitName, "review_result.md"))
		if rNote != "" {
			rDetail += " | " + rNote
		}
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "review", rStatus, rDetail)
	if advice := gateAdvice("review", rStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	aStatus, aDetail := checkAppendixGate(absRoot, unitName)
	if aStatus != gateOK {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "appendix", aStatus, aDetail)
	if advice := appendixAdvice(aStatus, vStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	writeDeltaScopeSections(stdout, absRoot, "unit", unitName, map[string]gateStatus{
		"validate": vStatus,
		"verify":   vfStatus,
		"review":   rStatus,
	})

	fmt.Fprintln(stdout)
	if nonFresh == 0 {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: yes\n")
	} else {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: no — %d gate(s) need attention\n", nonFresh)
	}
	return nil
}

// writeDeltaScopeSections prints the mechanism-derived delta scope (see
// validationcache.DeriveStaleScope) for every STALE gate. The scope is the
// delta re-run input: which checks declared the stale dependencies, which
// file entries are unclaimed (no check declared their stale deps), which
// entries could not be resolved or read, and whether every declared check
// is affected (degradation). Gates that are not STALE print nothing — the
// fresh report's own gate status already covers them.
func writeDeltaScopeSections(stdout io.Writer, absRoot, targetKind, targetName string, statuses map[string]gateStatus) {
	commands := []string{"validate", "verify", "review"}
	for _, cmd := range commands {
		if statuses[cmd] != gateStale {
			continue
		}
		scope, err := validationcache.DeriveStaleScope(absRoot, targetKind, targetName, cmd)
		if err != nil {
			fmt.Fprintf(stdout, "\nDELTA SCOPE (%s):\n  cache format error: %v — re-run %s@%s to rewrite the cache\n", cmd, err, cmd, targetName)
			continue
		}
		fmt.Fprintf(stdout, "\nDELTA SCOPE (%s):\n", cmd)
		if len(scope.StaleDeps) == 0 {
			if len(scope.Unreadable) > 0 {
				fmt.Fprintln(stdout, "  no stale dependency CIDs — staleness comes from missing/unreadable files or cache metadata")
			} else {
				fmt.Fprintln(stdout, "  no stale dependencies — staleness comes from a non-dependency cause (cache metadata)")
			}
			if len(scope.Unreadable) > 0 {
				fmt.Fprintf(stdout, "  unreadable entries: %s\n", strings.Join(scope.Unreadable, ", "))
			}
			continue
		}
		if scope.HasChecks {
			if len(scope.Affected) > 0 {
				fmt.Fprintf(stdout, "  affected checks: %s\n", strings.Join(scope.Affected, ", "))
			} else {
				fmt.Fprintln(stdout, "  affected checks: none — the stale deps are all unclaimed")
			}
		} else {
			fmt.Fprintln(stdout, "  no per-check evidence — derive the scope from the stale entries semantically (legacy cache or whole-file declarations)")
		}
		if len(scope.Unclaimed) > 0 {
			fmt.Fprintf(stdout, "  unclaimed entries: %s\n", strings.Join(scope.Unclaimed, ", "))
		}
		if len(scope.Unreadable) > 0 {
			fmt.Fprintf(stdout, "  unreadable entries: %s\n", strings.Join(scope.Unreadable, ", "))
		}
		fmt.Fprintf(stdout, "  stale deps: %d\n", len(scope.StaleDeps))
		if scope.Degrades {
			if targetKind == "rule" {
				fmt.Fprintln(stdout, "  degrades: yes — the rule file is a whole-file declaration, any change stales every rule-body check")
			} else {
				fmt.Fprintln(stdout, "  degrades: yes — every declared check is affected, an incremental re-run is a full re-run")
			}
		} else {
			fmt.Fprintln(stdout, "  degrades: no")
		}
	}
}

func writeUnitStableFreshDetail(stdout io.Writer, absRoot, unitName string) error {
	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (unit, stable)\n\n", unitName)

	vaStatus, vaDetail, vaNote := checkStableUnitGate(absRoot, unitName, "validate")
	if vaStatus == gateFresh {
		vaDetail = freshDetail(readSummary(absRoot, "unit", unitName, "validate_result.md"))
		if vaNote != "" {
			vaDetail += " | " + vaNote
		}
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vaStatus, vaDetail)
	if advice := gateAdvice("validate", vaStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	vfStatus, vfDetail, vfNote := checkStableUnitGate(absRoot, unitName, "verify")
	if vfStatus == gateFresh {
		vfDetail = freshDetail(readSummary(absRoot, "unit", unitName, "verify_result.md"))
		if vfNote != "" {
			vfDetail += " | " + vfNote
		}
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "verify", vfStatus, vfDetail)
	if advice := gateAdvice("verify", vfStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	rStatus, rDetail, rNote := checkStableUnitGate(absRoot, unitName, "review")
	if rStatus == gateFresh {
		rDetail = freshDetail(readSummary(absRoot, "unit", unitName, "review_result.md"))
		if rNote != "" {
			rDetail += " | " + rNote
		}
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "review", rStatus, rDetail)
	if advice := gateAdvice("review", rStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	result := baseline.CheckUnitBaseline(absRoot, unitName)
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "drift", result.Status, result.Details)
	fmt.Fprintln(stdout)
	switch result.Status {
	case baseline.StatusOK:
		fmt.Fprintln(stdout, "Drift: none — code surface matches the promote-time baseline.")
		if result.Note != "" {
			fmt.Fprintf(stdout, "Note: %s\n", result.Note)
		}
	case baseline.StatusChanged:
		fmt.Fprintln(stdout, "Drift: possible — code changed since promote. Run verify against stable to confirm.")
	default:
		fmt.Fprintln(stdout, "No baseline recorded for this stable unit (promoted before baseline support).")
	}

	writeDeltaScopeSections(stdout, absRoot, "unit", unitName, map[string]gateStatus{
		"validate": vaStatus,
		"verify":   vfStatus,
		"review":   rStatus,
	})
	return nil
}

func writeRuleFreshDetail(stdout io.Writer, absRoot, ruleID string) error {
	// A stable-only rule has no candidate round — report its drift state.
	if _, err := os.Stat(filepath.Join(absRoot, "docs/specs/rules/stable", ruleID+".md")); err == nil {
		if _, err := os.Stat(filepath.Join(absRoot, "docs/specs/rules/candidate", ruleID+".md")); os.IsNotExist(err) {
			return writeRuleStableFreshDetail(stdout, absRoot, ruleID)
		}
	}

	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (rule)\n\n", ruleID)

	vStatus, vDetail, vNote := checkRuleGate(absRoot, ruleID)
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "rule", ruleID, "validate_result.md"))
		if vNote != "" {
			vDetail += " | " + vNote
		}
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vStatus, vDetail)
	if advice := gateAdvice("validate", vStatus, ruleID); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}
	fmt.Fprintln(stdout, "verify and review do not apply to rules.")

	writeDeltaScopeSections(stdout, absRoot, "rule", ruleID, map[string]gateStatus{"validate": vStatus})

	fmt.Fprintln(stdout)
	if gatePassed(vStatus) {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: yes\n")
	} else {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: no — validate needs attention\n")
	}
	return nil
}

func writeRuleStableFreshDetail(stdout io.Writer, absRoot, ruleID string) error {
	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (rule, stable)\n\n", ruleID)

	vStatus, vDetail, vNote := checkStableRuleGate(absRoot, ruleID)
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "rule", ruleID, "validate_result.md"))
		if vNote != "" {
			vDetail += " | " + vNote
		}
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vStatus, vDetail)
	if advice := gateAdvice("validate", vStatus, ruleID); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	result := baseline.CheckRuleBaseline(absRoot, ruleID)
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "drift", result.Status, result.Details)
	fmt.Fprintln(stdout)
	switch result.Status {
	case baseline.StatusOK:
		fmt.Fprintln(stdout, "Drift: none — rule file matches the promote-time baseline.")
	case baseline.StatusChanged:
		fmt.Fprintln(stdout, "Drift: possible — rule file changed since promote.")
	default:
		fmt.Fprintln(stdout, "No baseline recorded for this stable rule (promoted before baseline support).")
	}

	writeDeltaScopeSections(stdout, absRoot, "rule", ruleID, map[string]gateStatus{
		"validate": vStatus,
	})
	return nil
}

// ------------------------------------------------------------
// Gate checks
// ------------------------------------------------------------

// checkStableUnitGate classifies one of the stable-layer confirmation states
// (validate/verify/review) using the same check chain promote relies on. The
// stable variants separate the layers the way each gate's evidence allows:
// validate/verify point the main-file check at the stable spec path (their
// caches must list the stable main spec), and review requires the cache to be
// recorded with `target: stable` (the review gate has no main-file
// requirement). A candidate-run cache fails the matching stable variant, so
// the stable report never mislabels a candidate cache as a stable
// confirmation.
func checkStableUnitGate(repoRoot, unitName, command string) (gateStatus, string, string) {
	var (
		result validationcache.CheckResult
		err    error
	)
	switch command {
	case "validate":
		result, err = validationcache.CheckValidateStable(repoRoot, unitName)
	case "verify":
		result, err = validationcache.CheckVerifyStable(repoRoot, unitName)
	case "review":
		result, err = validationcache.CheckReviewStable(repoRoot, unitName)
	default:
		return gateStale, fmt.Sprintf("unknown gate %q", command), ""
	}
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err), ""
	}
	return classifyGate(result), result.Reason, result.Note
}

// checkStableRuleGate classifies the stable-layer validate confirmation
// state of a rule.
func checkStableRuleGate(repoRoot, ruleID string) (gateStatus, string, string) {
	result, err := validationcache.CheckRuleValidateStable(repoRoot, ruleID)
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err), ""
	}
	return classifyGate(result), result.Reason, result.Note
}

// checkUnitGate classifies one of the unit gates (validate/verify/review)
// and returns the promote-identical reason text plus the informational note
// (e.g. content changed outside the declared dependency chunks).
func checkUnitGate(repoRoot, unitName, command string) (gateStatus, string, string) {
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
		return gateStale, fmt.Sprintf("unknown gate %q", command), ""
	}
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err), ""
	}
	return classifyGate(result), result.Reason, result.Note
}

func checkRuleGate(repoRoot, ruleID string) (gateStatus, string, string) {
	result, err := validationcache.CheckRuleValidate(repoRoot, ruleID)
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err), ""
	}
	return classifyGate(result), result.Reason, result.Note
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

// gateAdvice renders the recovery suggestion for a non-fresh gate. STALE is
// recoverable by the delta re-run (re* — the default recovery path); MISSING
// and BLOCKED have no usable delta baseline and need the full command.
func gateAdvice(command string, status gateStatus, targetName string) string {
	switch status {
	case gateStale:
		return fmt.Sprintf("-> suggestion: re%s@%s (delta recovery)", command, targetName)
	case gateMissing:
		return fmt.Sprintf("-> required: %s@%s (full run - no delta baseline)", command, targetName)
	case gateBlocked:
		return fmt.Sprintf("-> required: %s@%s (full run - resolve P0/P1 first)", command, targetName)
	default:
		return ""
	}
}

// appendixAdvice renders the recovery suggestion for the appendix gate. The
// full validate run is required only when the validate cache itself is FRESH:
// a delta re-run then stops early ("cache is fresh — no incremental re-run
// needed"), so it cannot pick up newly added appendices. When the validate
// cache is not FRESH, the delta re-run (revalidate@) restores appendix
// coverage instead — its rewrite carries a complete files list including
// every appendix (see framework/verification_scope.md §Delta Runs →
// Execution). An appendix MISSING state always implies validate MISSING,
// whose own advice line already demands the full run.
func appendixAdvice(status, validateStatus gateStatus, unitName string) string {
	if status == gateStale && validateStatus == gateFresh {
		return fmt.Sprintf("-> required: validate@%s (full run - appendix coverage)", unitName)
	}
	return ""
}

// freshDetail renders the frontmatter summary of a fresh cache.
func freshDetail(summary *validationcache.CacheSummary) string {
	if summary == nil {
		return "cache is fresh"
	}
	detail := fmt.Sprintf("result: %s · mode: %s · %d file(s) · %s",
		summary.Result, summary.Mode, summary.FileCount, noneIfEmpty(summary.Timestamp))
	if summary.Basis != "" {
		detail += fmt.Sprintf(" · basis: %s", summary.Basis)
	}
	return detail
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
	return unitNamesInLayer(repoRoot, "candidate")
}

func stableUnitNames(repoRoot string) ([]string, error) {
	return unitNamesInLayer(repoRoot, "stable")
}

func unitNamesInLayer(repoRoot, layer string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(repoRoot, "docs/specs/units/"+layer+"/unit_*.md"))
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
	return ruleIDsInLayer(repoRoot, "candidate")
}

func stableRuleIDs(repoRoot string) ([]string, error) {
	return ruleIDsInLayer(repoRoot, "stable")
}

func ruleIDsInLayer(repoRoot, layer string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(repoRoot, "docs/specs/rules/"+layer+"/*.md"))
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
	fmt.Fprintln(w, "  specflowctl fresh [--scope candidate|stable|all] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl fresh --unit UNIT [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl fresh --rule RULE_ID [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Reports cache freshness for all active candidates (default --scope")
	fmt.Fprintln(w, "candidate), drift state for all stable targets (--scope stable), or")
	fmt.Fprintln(w, "both (--scope all), or for a single unit/rule target. Read-only:")
	fmt.Fprintln(w, "never writes or deletes caches, never runs validate/verify/review.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --scope SCOPE    candidate | stable | all (default: candidate)")
	fmt.Fprintln(w, "  --unit UNIT      Unit name for single-target report")
	fmt.Fprintln(w, "  --rule RULE_ID   Rule id for single-target report")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

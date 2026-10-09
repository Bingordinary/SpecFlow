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
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/promote"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// gateStatus classifies the freshness of a single gate. The vocabulary is
// shared between the summary and detail output modes:
//
//	FRESH   cache exists and satisfies the gate (hashes match, mode full, etc.)
//	STALE   cache exists but the gate is not satisfied (files changed,
//	        coverage incomplete, mode/result invalid) — re-running fixes it
//	MISSING cache file does not exist (never run, or run failed and cache deleted)
//	BLOCKED cache exists but declares blocking findings (a failure record —
//	        validate/verify P0/P1)
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
	unitName, err := requireTargetName("unit", *unitPtr)
	if err != nil {
		return err
	}
	ruleID, err := requireTargetName("rule", *ruleIDPtr)
	if err != nil {
		return err
	}
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
	return nil
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
	globalAdvisories := map[string]promote.RulePrerequisite{}

	if len(unitNames) > 0 {
		// One derivation serves every unit line: the repo-wide surface audit
		// and the directory expansions are invocation constants, so summary
		// mode derives them once instead of once per unit.
		derivation, err := gaterun.NewDerivation(absRoot)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "UNITS (%d):\n", len(unitNames))
		for _, name := range unitNames {
			rulePrerequisites, err := promote.CheckUnitRulePrerequisites(absRoot, name)
			if err != nil {
				return err
			}
			total++
			line, ready := unitSummaryLine(derivation, absRoot, name, rulePrerequisites)
			if ready {
				readyCount++
			}
			fmt.Fprintf(stdout, "  %s\n", line)
			writeRulePrerequisiteItems(stdout, rulePrerequisites.Blockers)
			for _, advisory := range rulePrerequisites.Advisories {
				globalAdvisories[advisory.RuleID] = advisory
			}
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

	advisoryIDs := make([]string, 0, len(globalAdvisories))
	for id := range globalAdvisories {
		advisoryIDs = append(advisoryIDs, id)
	}
	sort.Strings(advisoryIDs)
	var advisories []promote.RulePrerequisite
	for _, id := range advisoryIDs {
		advisories = append(advisories, globalAdvisories[id])
	}
	writeGlobalRuleAdvisories(stdout, advisories)
	fmt.Fprintf(stdout, "READY FOR PROMOTE: %d of %d\n", readyCount, total)
	return nil
}

// writeStableFreshSection reports the confirmation and drift state of every
// stable target. Stable targets have no promote gate — the report shows the
// two confirmation states (validate: dependencies/rules, verify: code
// alignment and quality) plus the baseline drift comparison
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
		derivation, err := gaterun.NewDerivation(absRoot)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "STABLE UNITS (%d):\n", len(unitNames))
		for _, name := range unitNames {
			fmt.Fprintf(stdout, "  %s\n", stableUnitSummaryLine(derivation, absRoot, name))
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

// stableUnitSummaryLine reports one stable unit's confirmation and drift
// state. The two confirmation columns (validate/verify) come from
// the stable-layer caches written by the corresponding @stable runs; the
// drift column is the mechanical baseline comparison. A fresh verify cache
// means the code was recently confirmed to still conform even when the
// baseline surface differs.
func stableUnitSummaryLine(derivation *gaterun.Derivation, repoRoot, unitName string) string {
	vaStatus, _ := checkStableUnitGate(derivation, repoRoot, unitName, "validate")
	vfStatus, _ := checkStableUnitGate(derivation, repoRoot, unitName, "verify")
	return fmt.Sprintf("%-13s  validate: %-8s  verify: %-8s  drift: %-8s",
		unitName, vaStatus, vfStatus, stableDriftLabel(repoRoot, unitName, baseline.CheckUnitBaseline(repoRoot, unitName)))
}

func stableRuleSummaryLine(repoRoot, ruleID string) string {
	vStatus, _ := checkStableRuleGate(repoRoot, ruleID)
	return fmt.Sprintf("%-13s  validate: %-8s  drift: %-8s",
		ruleID, vStatus, stableDriftLabel(repoRoot, ruleID, baseline.CheckRuleBaseline(repoRoot, ruleID)))
}

// writeStaleImpactAfterPromote reports the stable units whose confirmation
// state is not FRESH immediately after a successful promote. Dependency
// freshness is the propagation path for cross-unit impact — a promoted change
// stales the confirmation caches of units whose declared evidence covers the
// changed content — so the sweep makes that impact surface loud at promote
// time instead of leaving it to be rediscovered (see
// framework/shared_judgments.md §Cache, delta and release — missing or
// indeterminate confirmation state blocks a unit's release with the specific
// gap; its own next gate re-confirms the affected surface).
// The promoted unit itself is excluded; its caches were just confirmed by the
// promotion. The sweep is informational.
func writeStaleImpactAfterPromote(stdout io.Writer, absRoot, promotedUnit string) error {
	unitNames, err := stableUnitNames(absRoot)
	if err != nil {
		return err
	}
	derivation, err := gaterun.NewDerivation(absRoot)
	if err != nil {
		return err
	}
	var lines []string
	for _, name := range unitNames {
		if name == promotedUnit {
			continue
		}
		for _, gate := range []string{"validate", "verify"} {
			status, reason := checkStableUnitGate(derivation, absRoot, name, gate)
			if status != gateStale && status != gateBlocked {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %s: %s %s — %s", name, gate, status, reason))
		}
	}
	if len(lines) == 0 {
		fmt.Fprintln(stdout, "Impact: no other stable unit confirmation is stale.")
		return nil
	}
	fmt.Fprintf(stdout, "Impact: %d stable unit confirmation(s) are not FRESH (dependency freshness) — each owner's next gate re-confirms:\n", len(lines))
	for _, line := range lines {
		fmt.Fprintln(stdout, line)
	}
	return nil
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

func unitSummaryLine(derivation *gaterun.Derivation, repoRoot, unitName string, rules promote.RulePrerequisites) (string, bool) {

	vStatus, _ := checkUnitGate(derivation, repoRoot, unitName, "validate")
	vfStatus, _ := checkUnitGate(derivation, repoRoot, unitName, "verify")
	aStatus, _ := checkAppendixGate(repoRoot, unitName)

	ruleStatus := "OK"
	if len(rules.Blockers) > 0 {
		ruleStatus = "BLOCKED"
	}
	ready := gatePassed(vStatus) && gatePassed(vfStatus) && gatePassed(aStatus) && len(rules.Blockers) == 0
	return fmt.Sprintf("%-13s  validate: %-8s  verify: %-8s  appendix: %-4s  rules: %-7s  READY: %t",
		unitName, vStatus, vfStatus, aStatus, ruleStatus, ready), ready
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
	// A stable-only unit has no candidate round — report its drift state
	// instead of candidate gate statuses.
	if _, err := os.Stat(filepath.Join(absRoot, "docs/specs/units/stable", "unit_"+unitName+".md")); err == nil {
		if _, err := os.Stat(filepath.Join(absRoot, "docs/specs/units/candidate", "unit_"+unitName+".md")); os.IsNotExist(err) {
			return writeUnitStableFreshDetail(stdout, absRoot, unitName)
		}
	}

	derivation, err := gaterun.NewDerivation(absRoot)
	if err != nil {
		return err
	}
	rulePrerequisites, err := promote.CheckUnitRulePrerequisites(absRoot, unitName)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (unit)\n\n", unitName)
	writeRulePrerequisites(stdout, rulePrerequisites)
	fmt.Fprintln(stdout)

	nonFresh := 0

	vStatus, vDetail := checkUnitGate(derivation, absRoot, unitName, "validate")
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "unit", unitName, "validate_result.md"))
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vStatus, vDetail)
	if advice := gateAdvice("validate", vStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	vfStatus, vfDetail := checkUnitGate(derivation, absRoot, unitName, "verify")
	if vfStatus == gateFresh {
		vfDetail = freshDetail(readSummary(absRoot, "unit", unitName, "verify_result.md"))
	} else {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "verify", vfStatus, vfDetail)
	if advice := gateAdvice("verify", vfStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}
	writeDeferredFindingsNote(stdout, absRoot, unitName)

	aStatus, aDetail := checkAppendixGate(absRoot, unitName)
	if aStatus != gateOK {
		nonFresh++
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "appendix", aStatus, aDetail)
	if advice := appendixAdvice(aStatus, vStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	writeChangeReportSections(stdout, absRoot, "unit", unitName, map[string]gateStatus{
		"validate": vStatus,
		"verify":   vfStatus,
	})

	fmt.Fprintln(stdout)
	if nonFresh == 0 && len(rulePrerequisites.Blockers) == 0 {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: yes\n")
	} else {
		fmt.Fprintf(stdout, "READY FOR PROMOTE: no — %d gate(s) need attention; %d rule prerequisite(s) blocked\n", nonFresh, len(rulePrerequisites.Blockers))
	}
	return nil
}

// writeChangeReportSections prints the mechanically detected change set for
// every STALE gate — what changed and where, or that the cached evidence
// cannot localize the change (legacy). The report is cache-side and
// read-only; the delta reviewer consumes the same change set when a re* run
// is triggered. Gates that are not STALE print nothing.
func writeChangeReportSections(stdout io.Writer, absRoot, targetKind, targetName string, statuses map[string]gateStatus) {
	for _, cmd := range []string{"validate", "verify"} {
		if statuses[cmd] != gateStale {
			continue
		}
		fmt.Fprintf(stdout, "\nCHANGE REPORT (%s):\n", cmd)
		report, err := validationcache.DeriveChangeReport(absRoot, targetKind, targetName, cmd)
		if err != nil {
			fmt.Fprintf(stdout, "  change report unavailable: %v\n", err)
			continue
		}
		writeChangeReportDetail(stdout, report)
	}
}

// writeChangeReportDetail renders one gate's change report: localized change
// entries per file, removed files, and files whose cached evidence cannot
// localize the change (a complete run is required for those).
func writeChangeReportDetail(stdout io.Writer, report *validationcache.ChangeReport) {
	if report.Empty() {
		fmt.Fprintln(stdout, "  no changes detected in the recorded input surface")
		return
	}
	for _, entry := range report.Entries {
		switch entry.Kind {
		case "changed":
			fmt.Fprintf(stdout, "  %s: changed\n", entry.Path)
			for _, c := range entry.Changes {
				fmt.Fprintf(stdout, "    %-7s lines %d-%d\n", c.Kind, c.StartLine, c.EndLine)
			}
		case "removed":
			fmt.Fprintf(stdout, "  %s: removed\n", entry.Path)
		case "legacy":
			fmt.Fprintf(stdout, "  %s: changed — cached evidence cannot localize it; run the full gate\n", entry.Path)
		}
	}
}

func writeUnitStableFreshDetail(stdout io.Writer, absRoot, unitName string) error {
	fmt.Fprintf(stdout, "FRESHNESS REPORT — %s (unit, stable)\n\n", unitName)

	derivation, err := gaterun.NewDerivation(absRoot)
	if err != nil {
		return err
	}
	vaStatus, vaDetail := checkStableUnitGate(derivation, absRoot, unitName, "validate")
	if vaStatus == gateFresh {
		vaDetail = freshDetail(readSummary(absRoot, "unit", unitName, "validate_result.md"))
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vaStatus, vaDetail)
	if advice := gateAdvice("validate", vaStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}

	vfStatus, vfDetail := checkStableUnitGate(derivation, absRoot, unitName, "verify")
	if vfStatus == gateFresh {
		vfDetail = freshDetail(readSummary(absRoot, "unit", unitName, "verify_result.md"))
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "verify", vfStatus, vfDetail)
	if advice := gateAdvice("verify", vfStatus, unitName); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}
	writeDeferredFindingsNote(stdout, absRoot, unitName)

	result := baseline.CheckUnitBaseline(absRoot, unitName)
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "drift", result.Status, result.Details)
	fmt.Fprintln(stdout)
	switch result.Status {
	case baseline.StatusOK:
		fmt.Fprintln(stdout, "Drift: none — code surface matches the promote-time baseline.")
	case baseline.StatusChanged:
		fmt.Fprintln(stdout, "Drift: possible — code changed since promote. Run verify against stable to confirm.")
	default:
		fmt.Fprintln(stdout, "No baseline recorded for this stable unit (promoted before baseline support).")
	}

	writeChangeReportSections(stdout, absRoot, "unit", unitName, map[string]gateStatus{
		"validate": vaStatus,
		"verify":   vfStatus,
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

	vStatus, vDetail := checkRuleGate(absRoot, ruleID)
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "rule", ruleID, "validate_result.md"))
	}
	fmt.Fprintf(stdout, "%-9s %-8s %s\n", "validate", vStatus, vDetail)
	if advice := gateAdvice("validate", vStatus, ruleID); advice != "" {
		fmt.Fprintf(stdout, "  %s\n", advice)
	}
	fmt.Fprintln(stdout, "verify does not apply to rules.")

	writeChangeReportSections(stdout, absRoot, "rule", ruleID, map[string]gateStatus{"validate": vStatus})

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

	vStatus, vDetail := checkStableRuleGate(absRoot, ruleID)
	if vStatus == gateFresh {
		vDetail = freshDetail(readSummary(absRoot, "rule", ruleID, "validate_result.md"))
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

	writeChangeReportSections(stdout, absRoot, "rule", ruleID, map[string]gateStatus{
		"validate": vStatus,
	})
	return nil
}

// ------------------------------------------------------------
// Gate checks
// ------------------------------------------------------------

// checkStableUnitGate classifies one of the stable-layer confirmation states
// (validate/verify) using the same check chain promote relies on. The
// stable variants separate the layers the way each gate's evidence allows:
// validate/verify point the main-file check at the stable spec path (their
// caches must list the stable main spec). A candidate-run cache fails the
// matching stable variant, so the stable report never mislabels a candidate
// cache as a stable confirmation.
func checkStableUnitGate(derivation *gaterun.Derivation, repoRoot, unitName, command string) (gateStatus, string) {
	var (
		result validationcache.CheckResult
		err    error
	)
	switch command {
	case "validate":
		result, err = validationcache.CheckValidateStable(repoRoot, unitName)
	case "verify":
		// No compatibility: a stable confirmation cache must carry per-check
		// evidence; an old cache without `checks` is invalid. When the stable
		// spec's coverage can be derived, its expected keys must be present.
		result, err = checkStableUnitVerifyMerged(derivation, repoRoot, unitName)
	default:
		return gateStale, fmt.Sprintf("unknown gate %q", command)
	}
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err)
	}
	return classifyGate(result), result.Reason
}

// checkStableRuleGate classifies the stable-layer validate confirmation
// state of a rule.
func checkStableRuleGate(repoRoot, ruleID string) (gateStatus, string) {
	result, err := validationcache.CheckRuleValidateStable(repoRoot, ruleID)
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err)
	}
	return classifyGate(result), result.Reason
}

// writeDeferredFindingsNote reports the unit's pending deferred findings —
// verify findings another unit's final synthesis routed here by recorded
// ownership. The next verify of this unit (either layer) disposes them; a
// malformed ledger is reported instead of silently ignored.
func writeDeferredFindingsNote(stdout io.Writer, repoRoot, unitName string) {
	ledger, err := validationcache.ReadDeferredLedger(repoRoot)
	if err != nil {
		fmt.Fprintf(stdout, "  Note: %v\n", err)
		return
	}
	pending := ledger.PendingForUnit(unitName)
	if len(pending) == 0 {
		return
	}
	fmt.Fprintf(stdout, "  Note: %d deferred finding(s) pending from other units — the next verify@%s disposes them:\n", len(pending), unitName)
	for _, entry := range pending {
		fmt.Fprintf(stdout, "    [%s] %s — from %s run %s: %s\n", entry.Severity, entry.FindingID, entry.SourceUnit, entry.SourceRun, entry.Text)
	}
}

// checkUnitGate classifies one of the unit gates (validate/verify) and
// returns the promote-identical reason text.
func checkUnitGate(derivation *gaterun.Derivation, repoRoot, unitName, command string) (gateStatus, string) {
	var (
		result validationcache.CheckResult
		err    error
	)
	switch command {
	case "validate":
		result, err = validationcache.CheckValidate(repoRoot, unitName)
	case "verify":
		// Use the same merged verify check promote runs, so a fresh report and
		// a promote run never disagree: the verify cache must cover both the
		// alignment and quality lenses.
		result, err = checkUnitVerifyMerged(derivation, repoRoot, unitName, "candidate")
	default:
		return gateStale, fmt.Sprintf("unknown gate %q", command)
	}
	if err != nil {
		return gateStale, fmt.Sprintf("gate check error: %v", err)
	}
	return classifyGate(result), result.Reason
}

// checkUnitVerifyMerged applies the merged-cache promote requirement: the
// verify cache must exist, be full mode, not block, be dependency-fresh, and
// cover every alignment key and every quality key of the current target. The
// expected keys are derived lazily — a missing or stale cache is classified
// without deriving them — and the derivation re-uses gate-plan's resolution
// (ExpectedChecks), so promote, fresh and the planner never disagree.
func checkUnitVerifyMerged(derivation *gaterun.Derivation, repoRoot, unitName, target string) (validationcache.CheckResult, error) {
	return validationcache.CheckVerifyMerged(repoRoot, unitName, target, func() ([]validationcache.ExpectedCheck, error) {
		expected, err := derivation.ExpectedChecks(unitName, target)
		if err != nil {
			// Coverage cannot be derived (e.g. the spec is missing). The
			// merged check fails the cache closed with this reason; when no
			// cache exists the base check has already reported MISSING.
			// promote and fresh share this function, so they still agree.
			return nil, fmt.Errorf("cannot derive required verify checks: %w", err)
		}
		return expected, nil
	}, false)
}

// checkStableUnitVerifyMerged applies the no-compatibility requirement to a
// stable-layer confirmation cache: the cache must carry per-check evidence
// (an old cache without `checks` is invalid), and when the stable spec's
// coverage can be derived its expected keys must be present. The both-lens
// requirement is the promote requirement and does not apply to a stable
// confirmation.
func checkStableUnitVerifyMerged(derivation *gaterun.Derivation, repoRoot, unitName string) (validationcache.CheckResult, error) {
	return validationcache.CheckVerifyMerged(repoRoot, unitName, "stable", func() ([]validationcache.ExpectedCheck, error) {
		expected, err := derivation.ExpectedChecks(unitName, "stable")
		if err != nil {
			return nil, fmt.Errorf("cannot derive stable verify checks: %w", err)
		}
		return expected, nil
	}, false)
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

// gateAdvice renders the recovery suggestion for a non-fresh gate. STALE is
// recoverable by the delta re-run (re* — the pass-baseline recovery path);
// MISSING has no usable baseline and needs the full command; BLOCKED is a
// failure record (validate/verify P0/P1 findings) whose recovery is
// the repair re-run (`gate-plan --mode repair`) after the findings are
// resolved — the failure record is the failure-recovery baseline (see
// framework/verification_scope.md §Delta Runs → Failure recovery).
func gateAdvice(command string, status gateStatus, targetName string) string {
	switch status {
	case gateStale:
		return fmt.Sprintf("-> suggestion: re%s@%s (delta recovery)", command, targetName)
	case gateMissing:
		return fmt.Sprintf("-> required: %s@%s (full run - no delta baseline)", command, targetName)
	case gateBlocked:
		return fmt.Sprintf("-> resolve P0/P1, then re%s@%s (repair recovery from the failure record)", command, targetName)
	default:
		return ""
	}
}

// appendixAdvice renders the recovery suggestion for the appendix gate. The
// full validate run is required only when the validate cache itself is FRESH:
// a delta re-run then stops early ("cache is fresh — no changes to review"),
// so it cannot pick up newly added appendices. When the validate
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
	if summary.ReviewChangeSet != "" {
		detail += fmt.Sprintf(" · change review: %s (%s)", summary.ReviewResult, shortHash(summary.ReviewChangeSet))
	}
	return detail
}

// shortHash abbreviates a content hash for display.
func shortHash(hash string) string {
	hash = strings.TrimPrefix(strings.TrimSpace(hash), "sha256:")
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
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

func writeFreshUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl fresh [--scope candidate|stable|all] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl fresh --unit UNIT [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl fresh --rule RULE_ID [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Reports cache freshness for all active candidates (default --scope")
	fmt.Fprintln(w, "candidate), drift state for all stable targets (--scope stable), or")
	fmt.Fprintln(w, "both (--scope all), or for a single unit/rule target. Read-only:")
	fmt.Fprintln(w, "never writes or deletes caches, never runs validate/verify.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --scope SCOPE    candidate | stable | all (default: candidate)")
	fmt.Fprintln(w, "  --unit UNIT      Unit name for single-target report")
	fmt.Fprintln(w, "  --rule RULE_ID   Rule id for single-target report")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

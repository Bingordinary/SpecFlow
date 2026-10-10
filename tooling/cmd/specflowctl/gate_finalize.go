package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// reportRef is one accepted session report plus its parsed content.
type reportRef struct {
	spec   *gaterun.SessionSpec
	result *gaterun.SessionResult
	text   string
}

type gateOutcome struct {
	Result             string
	Counts             [4]int
	Findings           []gaterun.Finding
	DeferredFindings   []gaterun.Finding
	Ownerships         []gaterun.FindingOwnership
	EffectiveStatus    map[string]string
	QualityConclusions map[string]string
	SynthesisDigest    string
}

// runGateFinalize writes the gate cache from the run's accepted session
// reports, after coverage closure and snapshot checks. Every coverage key must
// be covered by exactly one accepted session (plus carried-over baseline
// evidence for delta/repair); a run with findings must carry an accepted final
// synthesis. The input snapshot must be unchanged since gate-plan. Cache
// evidence is assembled from the accepted declarations, while result,
// blocking, severity counts, and statuses are derived from the accepted
// synthesis artifact. See framework/verification_scope.md §Coverage model and
// framework/validation_cache.md §Write Rules → Tooled writes.
func runGateFinalize(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-finalize", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id printed by gate-plan")
	timestampPtr := fs.String("timestamp", "", "run timestamp (default: now UTC)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID := strings.TrimSpace(*runIDPtr)
	if runID == "" {
		writeGateFinalizeUsage(stderr)
		return errors.New("--run is required (the run id printed by gate-plan)")
	}
	now := strings.TrimSpace(*timestampPtr)
	if now == "" {
		now = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}

	absRoot := mustAbs(*repoRootPtr)
	return gaterun.WithMutation(absRoot, func() error {
		return finalizeGateRun(absRoot, runID, now, stdout)
	})
}

func finalizeGateRun(absRoot, runID, now string, stdout io.Writer) error {
	run, err := gaterun.Load(absRoot, runID)
	if err != nil {
		return err
	}
	if run.Status != gaterun.StatusOpen {
		return fmt.Errorf("gate run %s is %s — only an open run can be finalized; plan a new run", run.RunID, run.Status)
	}
	if run.Escalated {
		return fmt.Errorf("the change review escalated this run to a full run — run the full command instead: `specflowctl gate-plan --gate %s %s --target %s --mode full`", run.Gate, gateRunTargetFlags(run), run.Target)
	}

	states, err := gaterun.LoadSessionStates(absRoot, run)
	if err != nil {
		return err
	}

	// Coverage closure: every coverage key must be covered by exactly one
	// accepted session. A duplicate is a mechanical error (submit prevents
	// it, but a hand-edited state must fail closed).
	_, uncovered, err := gaterun.CoverageProgress(run, states)
	if err != nil {
		return fmt.Errorf("gate-finalize rejected: %w", err)
	}
	if len(uncovered) > 0 {
		var rejected []string
		for _, state := range states {
			if state.Status != gaterun.SessionAccepted {
				rejected = append(rejected, fmt.Sprintf("%s (%s, keys: %s)", state.SessionID, state.Status, strings.Join(state.Keys, ", ")))
			}
		}
		detail := ""
		if len(rejected) > 0 {
			detail = "\nNon-accepted sessions: " + strings.Join(rejected, "; ")
		}
		return fmt.Errorf("gate-finalize rejected: coverage incomplete — uncovered key(s): %s%s\nSubmit them first: `specflowctl gate-mission --run %s --keys <keys> --format prompt`, then `specflowctl gate-submit --run %s --session <id> --keys <keys> --report PATH`",
			strings.Join(uncovered, ", "), detail, run.RunID, run.RunID)
	}
	if run.HasReviewCoverage() && run.Review == nil {
		return fmt.Errorf("gate-finalize rejected: the review session is covered but the run carries no review record — plan a new run")
	}

	var reports []reportRef
	for _, state := range states {
		if state.Status != gaterun.SessionAccepted {
			continue
		}
		spec, serr := gaterun.BuildSessionSpec(absRoot, run, state.Keys)
		if serr != nil {
			return fmt.Errorf("accepted session %s has an invalid key set: %w", state.SessionID, serr)
		}
		if spec.SessionID != state.SessionID {
			return fmt.Errorf("accepted session %s does not match the id derived from its keys %s", state.SessionID, strings.Join(state.Keys, ", "))
		}
		semanticData, _ := json.Marshal(state.Result)
		if run.Gate == gaterun.GateVerify && state.SemanticDigest != judgments.Digest(semanticData) {
			return fmt.Errorf("session %s structured result digest mismatch", state.SessionID)
		}
		if state.Result == nil || state.Result.ReportDigest != reportDigest(state.Report) {
			return fmt.Errorf("session %q is accepted but its structured result is missing or does not match the stored report — plan a new run", state.SessionID)
		}
		if err := validateConsumedDigests(absRoot, run, spec, state, states); err != nil {
			return err
		}
		reports = append(reports, reportRef{spec: spec, result: state.Result, text: state.Report})
	}

	// A run with findings must carry an accepted final synthesis; a clean run
	// finalizes directly. The final synthesis is the optional `cross` session.
	cross := crossReport(reports)
	hasFindings := runHasInputFindings(run, reports)
	if (hasFindings || len(run.Relationships) > 0) && cross == nil && run.TargetKind != gaterun.TargetKindRule {
		return fmt.Errorf("gate-finalize rejected: the run has relationships to check or findings but no accepted final synthesis — generate it with `specflowctl gate-mission --run %s --final --format prompt`, then submit it with `specflowctl gate-submit --run %s --session cross --keys cross --report PATH`", run.RunID, run.RunID)
	}

	// Snapshot check — the closure of the time-of-check/time-of-use gap. Any
	// divergence rejects the write and discards the run.
	divergences, err := gaterun.Compare(absRoot, run)
	if err != nil {
		return err
	}
	if len(divergences) > 0 {
		return rejectSnapshotDivergence(absRoot, run, divergences)
	}

	outcome, err := deriveGateOutcome(run, reports)
	if err != nil {
		return err
	}
	for _, finding := range outcome.Findings {
		if strings.TrimSpace(finding.Detail) == "" {
			return fmt.Errorf("gate-finalize rejected: retained finding %q has no renderable detail", finding.ID)
		}
	}
	result := outcome.Result
	counts := outcome.Counts

	entries, err := assembleEntries(absRoot, run)
	if err != nil {
		return err
	}
	if evidenceDivergences := evidenceSnapshotDivergences(absRoot, run, entries); len(evidenceDivergences) > 0 {
		return rejectSnapshotDivergence(absRoot, run, evidenceDivergences)
	}
	if result == "fail" {
		if err := applyFailureStatuses(run, outcome.EffectiveStatus, entries); err != nil {
			return err
		}
	}

	var body strings.Builder
	for i, rep := range reports {
		if i > 0 {
			body.WriteString("\n\n")
		}
		body.WriteString(strings.TrimRight(rep.text, "\n"))
	}
	retainedFindings := make([]gaterun.Finding, 0, len(outcome.Findings)+len(outcome.DeferredFindings))
	retainedFindings = append(retainedFindings, outcome.Findings...)
	retainedFindings = append(retainedFindings, outcome.DeferredFindings...)
	retainedDetails, err := retainedFindingDetails(reports, retainedFindings)
	if err != nil {
		return err
	}
	if len(retainedDetails) > 0 {
		body.WriteString("\n\nAdditional retained findings:\n")
		body.WriteString(strings.Join(retainedDetails, "\n\n"))
	}
	schema := 3
	if run.Gate == gaterun.GateVerify {
		schema = 4
		if err := publishUnitJudgments(absRoot, run, reports, outcome); err != nil {
			return err
		}
	}
	judgmentData, err := json.Marshal(gaterun.JudgmentBaseline{
		SchemaVersion:    schema,
		Records:          run.Records,
		Relationships:    run.AllRelationships(),
		LogicalStatus:    outcome.EffectiveStatus,
		Findings:         outcome.Findings,
		SynthesisDigest:  outcome.SynthesisDigest,
		DeferredFindings: outcome.DeferredFindings,
	})
	if err != nil {
		return fmt.Errorf("encode judgment baseline: %w", err)
	}

	write := validationcache.CacheWrite{
		Command:   run.Gate,
		Unit:      run.TargetName,
		Mode:      "full",
		Basis:     run.Mode,
		Result:    result,
		Target:    run.Target,
		Blocking:  result == "fail",
		P0Count:   counts[0],
		P1Count:   counts[1],
		P2Count:   counts[2],
		P3Count:   counts[3],
		Timestamp: now,
		GateRun:   run.RunID,
		Judgments: string(judgmentData),
		Body:      body.String(),
		Entries:   entries,
	}
	if run.Review != nil {
		write.ReviewChangeSet = run.Review.ChangeSet
		write.ReviewResult = run.Review.Result
		write.ReviewSession = run.Review.Session
		write.ReviewRecheck = append([]string(nil), run.Review.Recheck...)
		write.ReviewDeclined = declinedReviewCandidates(run.Review)
	}

	rendered, err := validationcache.RenderCache(run.TargetName, write)
	if err != nil {
		return err
	}

	check, err := validationcache.CheckRenderedCache(absRoot, run.TargetKind, run.TargetName, run.Gate, run.Target, rendered)
	if err != nil {
		return fmt.Errorf("self-check failed: %v — the candidate cache was not published", err)
	}
	expectedBlocked := result == "fail"
	accepted := check.Fresh
	if expectedBlocked && check.Category == validationcache.CategoryBlocked {
		accepted = true
	}
	if !accepted {
		return fmt.Errorf("gate-finalize self-check FAILED: %s — the candidate cache is not accepted by the %s gate and was not published", check.Reason, run.Gate)
	}

	if run.Gate == "validate" && run.TargetKind == "unit" && run.Target == "candidate" && result == "pass" {
		appendixResult, aerr := validationcache.CheckAppendicesInRenderedCache(absRoot, run.TargetName, rendered)
		if aerr != nil {
			return fmt.Errorf("appendix coverage check failed: %v — the candidate cache was not published", aerr)
		}
		if !appendixResult.Fresh {
			return fmt.Errorf("gate-finalize rejected: %s — the candidate cache was not published", appendixResult.Reason)
		}
	}

	// Re-resolve the complete input surface after evidence assembly and
	// self-check. A change after this comparison can only make the published
	// cache stale; it cannot combine an old judgment with evidence from new
	// bytes because every entry was already bound to its plan-time hash above.
	divergences, err = gaterun.Compare(absRoot, run)
	if err != nil {
		return err
	}
	if len(divergences) > 0 {
		return rejectSnapshotDivergence(absRoot, run, divergences)
	}

	writtenPath, err := validationcache.PublishCache(absRoot, run.TargetKind, run.TargetName, run.Gate, rendered)
	if err != nil {
		return err
	}
	if err := updateDeferredLedger(absRoot, run, outcome); err != nil {
		return fmt.Errorf("%v — the cache at %s was written and is valid, but the deferred-findings ledger could not be updated; re-run gate-finalize --run %s", err, relToRepo(absRoot, writtenPath), run.RunID)
	}
	// Storage collection rides the same transaction as cache publication:
	// the just-published cache defines the live-reference set this pass
	// collects against, and a failure leaves the run open for a retry.
	collected, err := gaterun.CollectJudgments(absRoot)
	if err != nil {
		return fmt.Errorf("%v — the cache at %s was written and is valid, but judgment collection failed; re-run gate-finalize --run %s", err, relToRepo(absRoot, writtenPath), run.RunID)
	}
	if expectedBlocked {
		fmt.Fprintf(stdout, "Self-check: BLOCKED (result: fail — the failure record blocks promote and is the failure-recovery baseline)\n")
	} else {
		fmt.Fprintf(stdout, "Self-check: FRESH (result: %s, whole-file content of %d file(s) unchanged)\n", result, len(entries))
	}

	if err := gaterun.Consume(absRoot, run); err != nil {
		return fmt.Errorf("%v — the cache at %s was written and is valid, but the run state could not be concluded and removed", err, relToRepo(absRoot, writtenPath))
	}

	fmt.Fprintf(stdout, "Cache written: %s\n", relToRepo(absRoot, writtenPath))
	if collected.Count > 0 {
		fmt.Fprintf(stdout, "Judgment collection: %d stale-protocol record(s) / %.1f MB reclaimed\n", collected.Count, float64(collected.Bytes)/(1<<20))
	}
	return nil
}

// crossReport returns the accepted final-synthesis report, or nil.
func crossReport(reports []reportRef) *reportRef {
	for i := range reports {
		if reports[i].spec.Kind == gaterun.SessionKindCross {
			return &reports[i]
		}
	}
	return nil
}

// runHasInputFindings reports whether the run's accepted non-final reports,
// carried results, or pending deferrals carry any finding — the trigger for
// the optional final synthesis.
func runHasInputFindings(run *gaterun.Run, reports []reportRef) bool {
	for _, rep := range reports {
		if rep.spec.Kind == gaterun.SessionKindCross {
			continue
		}
		if len(resultFindings(rep.result)) > 0 {
			return true
		}
	}
	for i := range run.CarriedResults {
		if len(resultFindings(&run.CarriedResults[i])) > 0 {
			return true
		}
	}
	return len(run.DeferredFindings) > 0
}

// updateDeferredLedger synchronizes the repository's deferred-findings ledger
// with one finalized verify run. It (1) consumes the owner-side entries the run
// disposed — every pending deferral loaded at plan time is an input finding the
// final synthesis must dispose — and (2) supersedes this unit's own older
// deferrals for files the run re-reviewed, then (3) records this run's new
// deferrals. The write is idempotent: a retried finalize re-applies the same
// removals and upserts the same entries by finding id.
func updateDeferredLedger(absRoot string, run *gaterun.Run, outcome *gateOutcome) error {
	if run.Gate != gaterun.GateVerify || run.TargetKind != gaterun.TargetKindUnit {
		return nil
	}
	ledger, err := validationcache.ReadDeferredLedger(absRoot)
	if err != nil {
		return err
	}
	current := map[string]bool{}
	for _, ck := range run.Coverage {
		for _, key := range run.ReportKeys(ck) {
			current[key] = true
		}
	}
	consumed := map[string]bool{}
	for _, deferred := range run.DeferredFindings {
		consumed[deferred.Finding.ID] = true
	}
	var kept []validationcache.DeferredEntry
	for _, entry := range ledger.Entries {
		if entry.OwnerUnit == run.TargetName && consumed[entry.FindingID] {
			continue
		}
		if entry.SourceUnit == run.TargetName && entryCoversCurrent(entry, current) {
			continue
		}
		kept = append(kept, entry)
	}
	ownershipByID := map[string]gaterun.FindingOwnership{}
	for _, ownership := range outcome.Ownerships {
		ownershipByID[ownership.FindingID] = ownership
	}
	for _, finding := range outcome.DeferredFindings {
		ownership, ok := ownershipByID[finding.ID]
		if !ok {
			return fmt.Errorf("deferred finding %q has no ownership record", finding.ID)
		}
		entry := validationcache.DeferredEntry{
			FindingID:    finding.ID,
			OwnerUnit:    ownership.OwnerUnit,
			SourceUnit:   run.TargetName,
			SourceRun:    run.RunID,
			Severity:     finding.Severity,
			Text:         finding.Text,
			Detail:       finding.Detail,
			SourceKey:    finding.SourceKey,
			AffectedKeys: append([]string(nil), finding.AffectedKeys...),
			EvidencePath: ownership.EvidencePath,
			Reason:       ownership.Reason,
		}
		replaced := false
		for i := range kept {
			if kept[i].FindingID == entry.FindingID {
				kept[i] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			kept = append(kept, entry)
		}
	}
	if sameDeferredEntries(kept, ledger.Entries) {
		return nil
	}
	return validationcache.WriteDeferredLedger(absRoot, validationcache.DeferredLedger{Entries: kept})
}

// sameDeferredEntries reports whether two entry slices are equal in order and
// content — a finalize that changes nothing must not rewrite the ledger.
func sameDeferredEntries(a, b []validationcache.DeferredEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// entryCoversCurrent reports whether any of the entry's affected keys was
// re-reviewed by the run (its current coverage report keys).
func entryCoversCurrent(entry validationcache.DeferredEntry, current map[string]bool) bool {
	if current[entry.SourceKey] {
		return true
	}
	for _, key := range entry.AffectedKeys {
		if current[key] {
			return true
		}
	}
	return false
}

// retainedFindingDetails returns the renderable block of every terminal
// retained finding whose canonical detail is not already present in a current
// session report — a finding carried over from the baseline, deferred by this
// run's final synthesis, or routed in from another unit's verify run. Every
// terminal retained finding keeps its complete block in the human-readable
// body exactly once; a finding already rendered by a session report is skipped.
func retainedFindingDetails(reports []reportRef, retained []gaterun.Finding) ([]string, error) {
	current := map[string]bool{}
	for _, report := range reports {
		for _, finding := range resultFindings(report.result) {
			current[finding.ID] = true
		}
	}
	seen := map[string]bool{}
	var details []string
	for _, finding := range retained {
		if seen[finding.ID] || current[finding.ID] {
			continue
		}
		detail := strings.TrimSpace(finding.Detail)
		if detail == "" {
			return nil, fmt.Errorf("retained finding %q has no renderable detail — run the full command", finding.ID)
		}
		seen[finding.ID] = true
		details = append(details, detail)
	}
	return details, nil
}

func validateConsumedDigests(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec, state *gaterun.SessionState, states []*gaterun.SessionState) error {
	if spec.Kind != gaterun.SessionKindCross && spec.Kind != gaterun.SessionKindDesign {
		return nil
	}
	expected := consumedResultDigests(absRoot, run, spec, states)
	if len(expected) != len(state.ConsumedResultDigests) {
		return fmt.Errorf("session %q consumed-result set no longer matches its dependencies — plan a new run", spec.SessionID)
	}
	for id, digest := range expected {
		if state.ConsumedResultDigests[id] != digest {
			return fmt.Errorf("session %q consumed digest for %q does not match the accepted result — plan a new run", spec.SessionID, id)
		}
	}
	return nil
}

func deriveGateOutcome(run *gaterun.Run, reports []reportRef) (*gateOutcome, error) {
	out := &gateOutcome{Result: "pass", EffectiveStatus: map[string]string{}}
	if run.TargetKind == gaterun.TargetKindRule {
		if err := deriveRuleOutcome(run, reports, out); err != nil {
			return nil, err
		}
	} else if cross := crossReport(reports); cross != nil {
		if err := deriveCrossOutcome(run, reports, cross, out); err != nil {
			return nil, err
		}
	} else {
		// A clean run: coverage is closed by alignment/quality sessions and no
		// final synthesis is required. No finding exists, so every status is
		// derived from the sessions' verdicts plus the carried baseline
		// statuses, and the mechanical synthesis digest covers the derived
		// status map.
		for i := range run.CarriedResults {
			for key, status := range run.CarriedResults[i].EffectiveStatus {
				out.EffectiveStatus[key] = status
			}
		}
		for _, rep := range reports {
			for key, verdict := range rep.result.Verdicts {
				out.EffectiveStatus[key] = statusForVerdict(verdict)
			}
		}
		if runHasInputFindings(run, reports) || len(run.Relationships) > 0 {
			return nil, errors.New("gate-finalize rejected: the run has findings but no accepted final synthesis")
		}
		digestState, err := json.Marshal(struct {
			LogicalStatus map[string]string `json:"logical_status"`
			Findings      []gaterun.Finding `json:"findings"`
		}{LogicalStatus: out.EffectiveStatus, Findings: []gaterun.Finding{}})
		if err != nil {
			return nil, fmt.Errorf("encode clean synthesis state: %w", err)
		}
		out.SynthesisDigest = reportDigest(string(digestState))
	}
	// A zero-finding run must emit the documented array shape in the
	// GATE_JUDGMENTS block, not JSON null: the block is the machine-readable
	// delta/repair baseline and its schema example records `"findings":[]`
	// (framework/validation_cache.md §Format).
	if out.Findings == nil {
		out.Findings = []gaterun.Finding{}
	}
	for _, finding := range out.Findings {
		switch finding.Severity {
		case "P0":
			out.Counts[0]++
		case "P1":
			out.Counts[1]++
		case "P2":
			out.Counts[2]++
		case "P3":
			out.Counts[3]++
		default:
			return nil, fmt.Errorf("finding %q has invalid severity %q", finding.ID, finding.Severity)
		}
	}
	if out.Counts[0]+out.Counts[1] > 0 {
		out.Result = "fail"
	}
	if run.Gate == gaterun.GateValidate && out.Counts[2]+out.Counts[3] > 0 {
		return nil, errors.New("validate synthesis contains P2/P3 findings; validate grades P0/P1 only")
	}
	return out, nil
}

// declinedReviewCandidates lists the carry candidates a review left out of
// Recheck — the conclusions the review declined to re-run. The cache records
// them so the scope decision is auditable beyond review_recheck.
func declinedReviewCandidates(rev *gaterun.ReviewRecord) []string {
	rechecked := map[string]bool{}
	for _, key := range rev.Recheck {
		rechecked[key] = true
	}
	var declined []string
	for _, key := range rev.Candidates {
		if !rechecked[key] {
			declined = append(declined, key)
		}
	}
	return declined
}

// statusForVerdict maps a session verdict token to the logical fail/pass
// status the cache records.
func statusForVerdict(verdict string) string {
	switch verdict {
	case "FAIL", "MISMATCH", "unacceptable":
		return "fail"
	default:
		return "pass"
	}
}

func deriveRuleOutcome(run *gaterun.Run, reports []reportRef, out *gateOutcome) error {
	carriedKeys := map[string]bool{}
	for _, key := range run.CarriedKeys {
		if carriedKeys[key] {
			return fmt.Errorf("rule run carries check %q more than once", key)
		}
		carriedKeys[key] = true
	}
	findingIndex := map[string]int{}
	addCarriedFinding := func(finding gaterun.Finding) {
		if index, ok := findingIndex[finding.ID]; ok {
			keys := findingKeySet(out.Findings[index])
			for key := range findingKeySet(finding) {
				keys[key] = true
			}
			out.Findings[index].AffectedKeys = sortedFindingKeys(keys, out.Findings[index].SourceKey)
			return
		}
		findingIndex[finding.ID] = len(out.Findings)
		out.Findings = append(out.Findings, finding)
	}
	for i := range run.CarriedResults {
		result := &run.CarriedResults[i]
		for key, status := range result.EffectiveStatus {
			if !carriedKeys[key] {
				return fmt.Errorf("carried rule result declares non-carried check %q", key)
			}
			if status != "pass" && status != "fail" {
				return fmt.Errorf("carried rule check %q has invalid status %q", key, status)
			}
			if prior, exists := out.EffectiveStatus[key]; exists && prior != status {
				return fmt.Errorf("carried rule check %q has conflicting statuses %q and %q", key, prior, status)
			}
			out.EffectiveStatus[key] = status
		}
		for _, finding := range resultFindings(result) {
			addCarriedFinding(finding)
		}
	}
	for key := range carriedKeys {
		if _, ok := out.EffectiveStatus[key]; !ok {
			return fmt.Errorf("carried rule check %q has no baseline status", key)
		}
	}
	for _, report := range reports {
		if report.spec.Kind == gaterun.SessionKindCross {
			return errors.New("rule validate has no final synthesis session")
		}
		for key, verdict := range report.result.Verdicts {
			if carriedKeys[key] {
				return fmt.Errorf("rule run check %q is both carried and re-run", key)
			}
			out.EffectiveStatus[key] = statusForVerdict(verdict)
		}
		for _, finding := range resultFindings(report.result) {
			if index, ok := findingIndex[finding.ID]; ok {
				out.Findings[index] = finding
			} else {
				findingIndex[finding.ID] = len(out.Findings)
				out.Findings = append(out.Findings, finding)
			}
		}
	}
	for _, key := range gaterun.RuleValidateChecks() {
		if _, ok := out.EffectiveStatus[key]; !ok {
			return fmt.Errorf("rule validate outcome has no logical status for check %q", key)
		}
	}
	if len(out.EffectiveStatus) != len(gaterun.RuleValidateChecks()) {
		return fmt.Errorf("rule validate outcome carries unexpected logical statuses: %v", out.EffectiveStatus)
	}
	digestState, err := json.Marshal(struct {
		LogicalStatus map[string]string `json:"logical_status"`
		Findings      []gaterun.Finding `json:"findings"`
	}{LogicalStatus: out.EffectiveStatus, Findings: out.Findings})
	if err != nil {
		return fmt.Errorf("encode rule synthesis state: %w", err)
	}
	out.SynthesisDigest = reportDigest(string(digestState))
	return nil
}

func deriveCrossOutcome(run *gaterun.Run, reports []reportRef, cross *reportRef, out *gateOutcome) error {
	retained, err := resolveCrossFindings(synthesisInputFindings(run, reports), cross.result.Findings, cross.result.Dispositions)
	if err != nil {
		return fmt.Errorf("gate-finalize rejected invalid cross synthesis: %w", err)
	}
	if len(cross.result.Ownerships) > 0 && run.Gate != gaterun.GateVerify {
		return fmt.Errorf("gate-finalize rejected invalid ownership synthesis: ownership records belong to unit verify — the %s gate has no ownership dimension", run.Gate)
	}
	retained, err = applyOwnerships(retained, cross.result.Ownerships)
	if err != nil {
		return fmt.Errorf("gate-finalize rejected invalid ownership synthesis: %w", err)
	}
	out.Findings, out.DeferredFindings = splitDeferred(retained, run.TargetName)
	out.Ownerships = cross.result.Ownerships
	for key, status := range cross.result.EffectiveStatus {
		out.EffectiveStatus[key] = status
	}
	out.QualityConclusions = cross.result.QualityConclusions
	out.SynthesisDigest = cross.result.ReportDigest
	return nil
}

// evidenceSnapshotDivergences binds every assembled cache entry to the
// immutable plan-time snapshot. assembleEntries reads live files; comparing
// the hash produced from that exact read prevents a file change between the
// plan-time surface comparison and evidence assembly from entering the cache.
func evidenceSnapshotDivergences(absRoot string, run *gaterun.Run, entries []validationcache.FileEntry) []string {
	var divergences []string
	for _, entry := range entries {
		expected, ok := run.SnapshotHash(absRoot, entry.Path)
		switch {
		case !ok:
			divergences = append(divergences, "evidence path absent from snapshot: "+entry.Path)
		case expected == "":
			divergences = append(divergences, "snapshot has no content hash for evidence path: "+entry.Path)
		case normalizeEvidenceHash(entry.Hash) != normalizeEvidenceHash(expected):
			divergences = append(divergences, "evidence differs from snapshot: "+entry.Path)
		}
	}
	sort.Strings(divergences)
	return divergences
}

func normalizeEvidenceHash(hash string) string {
	return strings.TrimPrefix(strings.TrimSpace(hash), "sha256:")
}

func rejectSnapshotDivergence(absRoot string, run *gaterun.Run, divergences []string) error {
	_ = gaterun.Delete(absRoot, run)
	return fmt.Errorf("gate-finalize rejected: input(s) changed since gate-plan —\n  %s\nRun state discarded; plan a new run and execute again: `specflowctl gate-plan --gate %s %s --target %s --mode %s`",
		strings.Join(divergences, "\n  "), run.Gate, gateRunTargetFlags(run), run.Target, run.Mode)
}

// assembleEntries builds the cache files list from the run's input surface.
// Every entry records the whole-file hash and the ordered chunk sequence of
// one surface file, computed from current bytes and bound to the plan-time
// snapshot by the caller. Checks are markers attached to the file that owns
// them — validate/verify judgment keys on the target's own main file, code
// checks on their file, carried checks on the target's own main file — with
// the lens tag and, for failure records, the status. There is no per-check
// dependency attribution: the entry set IS the input surface
// (framework/validation_cache.md §Format).
func assembleEntries(absRoot string, run *gaterun.Run) ([]validationcache.FileEntry, error) {
	surface := append([]string{gaterun.TargetMainFile(run)}, run.OwnSpecFiles...)
	surface = append(surface, gaterun.InputSurface(absRoot, run)...)

	type agg struct {
		entry validationcache.FileEntry
	}
	aggByAbs := map[string]*agg{}
	var order []string
	ensure := func(ref string) (*agg, error) {
		abs := gaterun.CanonicalDeclPath(absRoot, ref)
		if a := aggByAbs[abs]; a != nil {
			return a, nil
		}
		entry, err := validationcache.BuildEvidenceEntry(absRoot, ref)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, nil
		}
		a := &agg{entry: *entry}
		aggByAbs[abs] = a
		order = append(order, abs)
		return a, nil
	}
	seenRef := map[string]bool{}
	for _, ref := range surface {
		if ref == "" || seenRef[ref] {
			continue
		}
		seenRef[ref] = true
		if _, err := ensure(ref); err != nil {
			return nil, err
		}
	}

	seenKey := map[string]bool{}
	attach := func(ref, key, lens string) error {
		a, err := ensure(ref)
		if err != nil {
			return err
		}
		if a == nil {
			return fmt.Errorf("no evidence file for check %q (path %s)", key, ref)
		}
		a.entry.Checks = append(a.entry.Checks, validationcache.CheckEntry{Check: key, Lens: lens})
		seenKey[key] = true
		return nil
	}
	for _, ck := range run.Coverage {
		if ck.Kind == gaterun.SessionKindDeltaReview {
			continue
		}
		for _, key := range run.ReportKeys(ck) {
			ref := gaterun.TargetMainFile(run)
			if ck.Kind == gaterun.SessionKindCode {
				// A public check's artifact is its own file: the check key is
				// `code:<file>` and the coverage key carries the file path.
				ref = ck.File
				if ref == "" {
					ref = strings.TrimPrefix(key, "code:")
				}
			}
			if err := attach(ref, key, run.LensForReportKey(key)); err != nil {
				return nil, err
			}
		}
	}
	for _, key := range run.CarriedKeys {
		// A carried key's lens is part of the run's immutable plan-time
		// snapshot (captured from the baseline at plan time), so finalize
		// never depends on a baseline another run may have rewritten.
		lens, ok := run.CarriedLenses[key]
		if !ok {
			return nil, fmt.Errorf("carried-over check %q has no lens snapshot in the run state — plan a new run", key)
		}
		if lens == "" {
			lens = run.LensForReportKey(key)
		}
		if err := attach(gaterun.TargetMainFile(run), key, lens); err != nil {
			return nil, err
		}
	}
	// Standing relationship conclusions are judgment keys too: their check
	// markers make the published cache carry every logical conclusion the
	// judgment baseline records, so a later repair status map or delta lens
	// snapshot can resolve them.
	for _, name := range run.AllRelationships() {
		key := gaterun.RelationshipKey(name)
		if seenKey[key] {
			continue
		}
		if err := attach(gaterun.TargetMainFile(run), key, run.LensForReportKey(key)); err != nil {
			return nil, err
		}
	}

	sort.Strings(order)
	var entries []validationcache.FileEntry
	for _, abs := range order {
		a := aggByAbs[abs]
		sortChecks(a.entry.Checks)
		entries = append(entries, a.entry)
	}
	for _, ck := range run.Coverage {
		if ck.Kind == gaterun.SessionKindDeltaReview {
			continue
		}
		for _, key := range run.ReportKeys(ck) {
			if !seenKey[key] {
				return nil, fmt.Errorf("coverage incomplete: check %q has no evidence in the assembled cache — plan a new run", key)
			}
		}
	}
	for _, required := range run.RequiredFiles {
		found := false
		for _, entry := range entries {
			if gaterun.CanonicalDeclPath(absRoot, entry.Path) == gaterun.CanonicalDeclPath(absRoot, required) {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("coverage incomplete: required file %s is not covered by the accepted reports — plan a new run", required)
		}
	}
	return entries, nil
}

// applyFailureStatuses copies the accepted synthesis's effective status map;
// carried-over keys are always recorded as `carried` for repair planning — a
// carried judgment did not run in this attempt, whatever its effective status
// is (its retained findings stay in the GATE_JUDGMENTS baseline and are
// resolved by the recovery run, not re-derived from a `fail` status).
func applyFailureStatuses(run *gaterun.Run, status map[string]string, entries []validationcache.FileEntry) error {
	carried := map[string]bool{}
	for _, k := range run.CarriedKeys {
		carried[k] = true
	}
	for i := range entries {
		for j := range entries[i].Checks {
			key := entries[i].Checks[j].Check
			st, ok := status[key]
			if !ok {
				return fmt.Errorf("cannot derive a status for check %q — plan a new run", key)
			}
			if carried[key] {
				entries[i].Checks[j].Status = "carried"
				continue
			}
			entries[i].Checks[j].Status = st
		}
	}
	return nil
}

// sortCheckKeys orders check keys numerically first (validate "1"-"10"), then
// lexically (verify item ids, quality file paths, "cross").
func sortCheckKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		ni, ei := strconv.Atoi(keys[i])
		nj, ej := strconv.Atoi(keys[j])
		switch {
		case ei == nil && ej == nil:
			return ni < nj
		case ei == nil:
			return true
		case ej == nil:
			return false
		default:
			return keys[i] < keys[j]
		}
	})
}

// sortChecks orders a files entry's check list with sortCheckKeys.
func sortChecks(checks []validationcache.CheckEntry) {
	sort.Slice(checks, func(i, j int) bool {
		ki, kj := checks[i].Check, checks[j].Check
		ni, ei := strconv.Atoi(ki)
		nj, ej := strconv.Atoi(kj)
		switch {
		case ei == nil && ej == nil:
			return ni < nj
		case ei == nil:
			return true
		case ej == nil:
			return false
		default:
			return ki < kj
		}
	})
}

func hasCheckKey(checks []validationcache.CheckEntry, key string) bool {
	for _, c := range checks {
		if c.Check == key {
			return true
		}
	}
	return false
}

func containsString(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

// gateRunTargetFlags renders the unit/rule flag combination of a run for
// guidance text.
func gateRunTargetFlags(run *gaterun.Run) string {
	if run.TargetKind == "rule" {
		return "--rule " + run.TargetName
	}
	return "--unit " + run.TargetName
}

func relToRepo(absRoot, path string) string {
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func writeGateFinalizeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl gate-finalize --run RUN_ID [--timestamp T] [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Writes the gate cache from the run's accepted session reports, after coverage")
	fmt.Fprintln(w, "closure and snapshot checks: every coverage key must be covered by exactly one")
	fmt.Fprintln(w, "accepted session (plus carried-over baseline evidence for delta/repair), and no")
	fmt.Fprintln(w, "input may have changed since gate-plan (a divergence discards the run). A run")
	fmt.Fprintln(w, "that produced findings must carry an accepted final synthesis (the optional")
	fmt.Fprintln(w, "`cross` session); a clean run finalizes directly. The tooling assembles one")
	fmt.Fprintln(w, "files entry per run input-surface file (whole-file hash + ordered chunk")
	fmt.Fprintln(w, "sequence), attaches each check's lens and failure status, derives the")
	fmt.Fprintln(w, "failure-record status map mechanically, then re-reads and runs the gate's own")
	fmt.Fprintln(w, "freshness chain (pass → FRESH, failure record → BLOCKED; validate@")
	fmt.Fprintln(w, "additionally checks appendix coverage). Result, blocking, severity counts,")
	fmt.Fprintln(w, "and failure statuses are derived from accepted artifacts.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --run RUN_ID     gate run id printed by gate-plan (required)")
	fmt.Fprintln(w, "  --timestamp T    run timestamp RFC3339 (default: now UTC)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

func publishUnitJudgments(root string, run *gaterun.Run, reports []reportRef, outcome *gateOutcome) error {
	coverage, err := gaterun.RequiredCoverage(root, run)
	if err != nil {
		return err
	}
	byKey := map[string]reportRef{}
	for _, rep := range reports {
		if rep.spec.Kind == gaterun.SessionKindCross {
			continue
		}
		for _, key := range rep.spec.CheckKeys {
			byKey[key] = rep
		}
	}
	for _, ck := range coverage {
		// Public observations are independent facts, published at acceptance.
		// Unit-specific synthesis must not rewrite them.
		if ck.Kind == gaterun.SessionKindCode {
			continue
		}
		binding, bound := run.Records[ck.Key]
		rep, reported := byKey[ck.Key]
		if !reported {
			if !bound {
				return fmt.Errorf("no accepted judgment for %s", ck.Key)
			}
			record, err := judgments.Load(root, binding.Reference)
			if err != nil {
				return err
			}
			result, err := gaterun.BoundJudgmentResult(record, ck, binding.Layer)
			if err != nil {
				return err
			}
			rep = reportRef{result: &result, text: record.Report}
		}
		original := keyJudgmentResult(ck, rep.result)
		canonical := finalizedKeyResult(ck, &original, outcome)
		changed := !judgmentDecisionEqual(&original, &canonical)
		if bound && !changed {
			if !gaterun.IsItemKind(ck.Kind) {
				continue
			}
			context, err := judgments.SpecContext(root, ck.Unit, binding.Layer)
			if err != nil {
				return err
			}
			record, err := judgments.Load(root, binding.Reference)
			if err != nil {
				return err
			}
			if record.SpecContext == context {
				continue
			}
			// Carry the unchanged judgment into this spec context without
			// executing it again or changing another context's current decision.
		}
		report := rep.text
		if cross := crossReport(reports); cross != nil && changed {
			// The decision includes the final review's evidence, even when the
			// original item was ALIGNED or was carried without re-execution.
			report += "\n\nFinal synthesis:\n" + cross.text
		}
		var refs []judgments.Binding
		if ck.Kind == gaterun.SessionKindDesign {
			public, ok := run.Records["code:"+ck.File]
			if !ok {
				return fmt.Errorf("no public record for %s", ck.File)
			}
			refs = append(refs, public)
		}
		ref, err := gaterun.SaveJudgment(root, run, ck, &canonical, report, refs)
		if err != nil {
			return err
		}
		layer, source := run.Target, "executed"
		if bound {
			source = binding.Source
		}
		run.Records[ck.Key] = judgments.Binding{Reference: ref, Layer: layer, Source: source}
	}
	return gaterun.Persist(root, run)
}

func keyJudgmentResult(ck gaterun.CoverageKey, original *gaterun.SessionResult) gaterun.SessionResult {
	result := *original
	verdict := original.Verdicts[ck.Key]
	status, ok := original.EffectiveStatus[ck.Key]
	if !ok {
		status = statusForVerdict(verdict)
	}
	if ck.Kind != gaterun.SessionKindCode {
		// A design record documents dispositions, not the public facts — the
		// facts live in the file's public record (a co-batched session's
		// result carries them, but they must not leak into unit records).
		result.Observations = nil
	}
	result.Verdicts = map[string]string{ck.Key: verdict}
	result.EffectiveStatus = map[string]string{ck.Key: status}
	result.Findings = nil
	for _, finding := range original.Findings {
		if findingKeySet(finding)[ck.Key] {
			finding.SourceKey = ck.Key
			finding.AffectedKeys = nil
			result.Findings = append(result.Findings, finding)
		}
	}
	return result
}

func finalizedKeyResult(ck gaterun.CoverageKey, original *gaterun.SessionResult, outcome *gateOutcome) gaterun.SessionResult {
	result := *original
	status := outcome.EffectiveStatus[ck.Key]
	verdict := original.Verdicts[ck.Key]
	if gaterun.IsItemKind(ck.Kind) {
		// Passing an advisory gate does not prove an indeterminate requirement:
		// keep the original alignment verdict.
		if status == "pass" && verdict != "CANNOT_DETERMINE" {
			verdict = "ALIGNED"
		} else if verdict != "CANNOT_DETERMINE" {
			verdict = "MISMATCH"
		}
	} else if conclusion, finalized := outcome.QualityConclusions[ck.Key]; finalized {
		verdict = conclusion
	}
	result.Verdicts = map[string]string{ck.Key: verdict}
	result.EffectiveStatus = map[string]string{ck.Key: status}
	result.Findings = nil
	for _, finding := range outcome.Findings {
		if findingKeySet(finding)[ck.Key] {
			finding.SourceKey = ck.Key
			finding.AffectedKeys = nil
			result.Findings = append(result.Findings, finding)
		}
	}
	return result
}

func judgmentDecisionEqual(original, canonical *gaterun.SessionResult) bool {
	return reflect.DeepEqual(original.Verdicts, canonical.Verdicts) &&
		reflect.DeepEqual(original.EffectiveStatus, canonical.EffectiveStatus) &&
		reflect.DeepEqual(original.Findings, canonical.Findings)
}

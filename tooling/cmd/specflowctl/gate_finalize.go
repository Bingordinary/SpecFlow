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

	synthesisKeys := synthesisEvidenceKeys(run, reports, outcome)
	entries, err := assembleEntries(absRoot, run, reports, synthesisKeys)
	if err != nil {
		return err
	}
	if evidenceDivergences := evidenceSnapshotDivergences(absRoot, run, entries, freshEvidencePaths(reports)); len(evidenceDivergences) > 0 {
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
		if err := publishUnitJudgments(absRoot, run, reports, outcome, synthesisKeys); err != nil {
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
		fmt.Fprintf(stdout, "Self-check: FRESH (result: %s, dependency chunks of %d file(s) unchanged)\n", result, len(entries))
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

// freshEvidencePaths lists the paths this run's accepted reports declared —
// the entries whose evidence is computed from current bytes at assembly time.
// Paths whose evidence is only carried over from the baseline are excluded:
// they keep their baseline entries by contract (their declared deps were
// verified against current content at plan time), and a whole-file change
// outside those deps is a documented fresh state that must not reject the
// finalize (see framework/verification_scope.md §Delta Runs → Execution and
// framework/validation_cache.md §Staleness Detection).
func freshEvidencePaths(reports []reportRef) map[string]bool {
	fresh := map[string]bool{}
	for _, rep := range reports {
		for _, scope := range rep.result.Scopes {
			fresh[scope.Path] = true
		}
	}
	return fresh
}

// evidenceSnapshotDivergences binds every freshly assembled cache entry to
// the immutable plan-time snapshot. BuildEntryFromChecks reads live files;
// comparing the hash produced from that exact read prevents a file change
// between the initial surface comparison and evidence assembly from entering
// the cache. Carried-only entries are validated for snapshot presence only —
// their evidence comes from the baseline, not from a read of current bytes
// (the plan → finalize content window for them is covered by gaterun.Compare).
func evidenceSnapshotDivergences(absRoot string, run *gaterun.Run, entries []validationcache.FileEntry, fresh map[string]bool) []string {
	var divergences []string
	for _, entry := range entries {
		expected, ok := run.SnapshotHash(absRoot, entry.Path)
		switch {
		case !ok:
			divergences = append(divergences, "evidence path absent from snapshot: "+entry.Path)
		case expected == "":
			divergences = append(divergences, "snapshot has no content hash for evidence path: "+entry.Path)
		case fresh[entry.Path] && normalizeEvidenceHash(entry.Hash) != normalizeEvidenceHash(expected):
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

// assembleEntries builds the cache files list from the accepted reports'
// dependency-scope declarations plus the carried-over baseline evidence
// (delta/repair). It also enforces coverage: the coverage set's report keys and
// the target's required files must all appear.
func assembleEntries(absRoot string, run *gaterun.Run, reports []reportRef, synthesisKeys map[string]map[string]bool) ([]validationcache.FileEntry, error) {
	type pathAgg struct {
		path  string
		keys  []string
		decls map[string]*declParts
	}
	agg := map[string]*pathAgg{}
	var pathOrder []string
	var scopes []gaterun.Scope
	reportedKeys := map[string]bool{}
	for _, rep := range reports {
		for _, s := range rep.result.Scopes {
			if s.Key != gaterun.CrossKey {
				scopes = append(scopes, s)
				reportedKeys[s.Key] = true
			}
		}
	}
	// `cross` has no independent check entry. Its disposition/ownership
	// evidence and each finding group's source evidence belong to the logical
	// judgments that consumed them, including suppressed and carried findings.
	var affectedKeys []string
	for key := range synthesisKeys {
		affectedKeys = append(affectedKeys, key)
	}
	sortCheckKeys(affectedKeys)
	for _, key := range affectedKeys {
		scopes = append(scopes, synthesisScopesForKey(key, reports, synthesisKeys[key])...)
	}
	for _, s := range scopes {
		p := agg[s.Path]
		if p == nil {
			p = &pathAgg{path: s.Path, decls: map[string]*declParts{}}
			agg[s.Path] = p
			pathOrder = append(pathOrder, s.Path)
		}
		if _, ok := p.decls[s.Key]; !ok {
			p.decls[s.Key] = &declParts{}
			p.keys = append(p.keys, s.Key)
		}
		d, derr := parseDecl(s.Declaration)
		if derr != nil {
			return nil, fmt.Errorf("check %q declaration for %s: %w", s.Key, s.Path, derr)
		}
		mergeDecl(p.decls[s.Key], d)
	}

	// Carried-over evidence comes from the run's plan-time snapshot — the
	// immutable input fixed before execution — not from the baseline cache,
	// which another run may have rewritten in the meantime. The snapshot
	// copies the baseline entries' hash + deps + check evidence verbatim
	// (their CIDs are unchanged by construction — they were not stale
	// sources); statuses are dropped (absent means pass on the new cache).
	type carriedEntry struct {
		hash   string
		deps   []string
		checks []validationcache.CheckEntry
	}
	carried := map[string]*carriedEntry{}
	carriedKeys := map[string]bool{}
	for _, key := range run.CarriedKeys {
		carriedKeys[key] = true
	}
	if len(run.CarriedKeys) > 0 {
		if len(run.CarriedEvidence) == 0 {
			return nil, fmt.Errorf("carried-over checks %s have no evidence snapshot in the run state — plan a new run", strings.Join(run.CarriedKeys, ", "))
		}
		for _, entry := range run.CarriedEvidence {
			ce := &carriedEntry{hash: entry.Hash, deps: append([]string(nil), entry.Deps...)}
			for _, c := range entry.Checks {
				ce.checks = append(ce.checks, validationcache.CheckEntry{Check: c.Check, Lens: c.Lens, Deps: append([]string(nil), c.Deps...)})
				for _, key := range affectedKeys {
					if key != c.Check && synthesisKeys[key][c.Check] {
						ce.checks = append(ce.checks, validationcache.CheckEntry{Check: key, Lens: run.LensForReportKey(key), Deps: append([]string(nil), c.Deps...)})
					}
				}
			}
			carried[entry.Path] = ce
			pathOrder = append(pathOrder, entry.Path)
		}
		for _, key := range run.CarriedKeys {
			found := false
			for _, ce := range carried {
				if hasCheckKey(ce.checks, key) {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("carried-over check %q has no evidence snapshot — plan a new run", key)
			}
		}
	}

	seenPath := map[string]bool{}
	for _, p := range pathOrder {
		seenPath[p] = true
	}
	var paths []string
	for p := range seenPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var entries []validationcache.FileEntry
	seenKey := map[string]bool{}
	for _, path := range paths {
		p := agg[path]
		var entry validationcache.FileEntry
		if p != nil && len(p.keys) > 0 {
			keys := append([]string(nil), p.keys...)
			sortCheckKeys(keys)
			var checks []validationcache.CheckDeclaration
			for _, k := range keys {
				m := p.decls[k]
				cd := validationcache.CheckDeclaration{Check: k, Lens: run.LensForReportKey(k)}
				if !m.WholeFile {
					cd.Sections = m.Sections
					cd.Ranges = strings.Join(m.Ranges, ",")
					cd.AcceptanceItems = m.Accepts
					cd.AcceptanceItemIDs = m.Items
				}
				checks = append(checks, cd)
			}
			built, berr := validationcache.BuildEntryFromChecks(absRoot, path, checks)
			if berr != nil {
				return nil, berr
			}
			entry = built
		} else {
			entry.Path = path
		}
		if ce := carried[path]; ce != nil {
			if entry.Hash == "" {
				entry.Hash = ce.hash
			}
			for _, dep := range ce.deps {
				if !containsString(entry.Deps, dep) {
					entry.Deps = append(entry.Deps, dep)
				}
			}
			for _, c := range ce.checks {
				if carriedKeys[c.Check] && reportedKeys[c.Check] {
					return nil, fmt.Errorf("check %q is both re-run and carried over — plan a new run", c.Check)
				}
				index := -1
				for i := range entry.Checks {
					if entry.Checks[i].Check == c.Check {
						index = i
						break
					}
				}
				if index < 0 {
					entry.Checks = append(entry.Checks, c)
				} else {
					for _, dep := range c.Deps {
						if !containsString(entry.Checks[index].Deps, dep) {
							entry.Checks[index].Deps = append(entry.Checks[index].Deps, dep)
						}
					}
				}
				for _, dep := range c.Deps {
					if !containsString(entry.Deps, dep) {
						entry.Deps = append(entry.Deps, dep)
					}
				}
			}
		}
		sortChecks(entry.Checks)
		for _, c := range entry.Checks {
			seenKey[c.Check] = true
		}
		entries = append(entries, entry)
	}

	for _, ck := range run.Coverage {
		for _, key := range run.ReportKeys(ck) {
			if !seenKey[key] {
				return nil, fmt.Errorf("coverage incomplete: check %q has no evidence in the assembled cache — plan a new run", key)
			}
		}
	}
	for _, required := range run.RequiredFiles {
		found := false
		for _, entry := range entries {
			if entry.Path == required {
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
	fmt.Fprintln(w, "`cross` session); a clean run finalizes directly. The tooling assembles each")
	fmt.Fprintln(w, "files entry and the per-check checks mapping from the reports' Dependency scope")
	fmt.Fprintln(w, "declarations, computes hash + deps, enforces union discipline and path-form")
	fmt.Fprintln(w, "rules, derives the failure-record status map mechanically, then re-reads and")
	fmt.Fprintln(w, "runs the gate's own freshness chain (pass → FRESH, failure record → BLOCKED;")
	fmt.Fprintln(w, "validate@ additionally checks appendix coverage). Result, blocking, severity")
	fmt.Fprintln(w, "counts, and failure statuses are derived from accepted artifacts.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --run RUN_ID     gate run id printed by gate-plan (required)")
	fmt.Fprintln(w, "  --timestamp T    run timestamp RFC3339 (default: now UTC)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

func publishUnitJudgments(root string, run *gaterun.Run, reports []reportRef, outcome *gateOutcome, synthesisKeys map[string]map[string]bool) error {
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
			canonical.Scopes = append(canonical.Scopes, synthesisScopesForKey(ck.Key, reports, synthesisKeys[ck.Key])...)
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
	result.Scopes = nil
	for _, scope := range original.Scopes {
		if scope.Key == ck.Key {
			result.Scopes = append(result.Scopes, scope)
		}
	}
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

func synthesisScopesForKey(key string, reports []reportRef, sources map[string]bool) []gaterun.Scope {
	var scopes []gaterun.Scope
	for _, rep := range reports {
		for _, scope := range rep.result.Scopes {
			if sources[scope.Key] {
				scope.Key = key
				scopes = append(scopes, scope)
			}
		}
	}
	return scopes
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

package gaterun

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// SessionKindDeltaReview is the review session kind of a delta run: an
// independent reviewer judges which standing conclusions the mechanically
// detected change set could affect. It produces no check verdicts — it
// accepts the unchanged remainder, names re-run keys, or escalates to a full
// run. The re-run sessions it names are ordinary sessions with the standard
// checklist procedure.
const SessionKindDeltaReview = "delta_review"

// DeltaReviewKey is the coverage key of the review session.
const DeltaReviewKey = "review"

// ReviewRecord is the recorded outcome of one delta review. ChangeSet binds
// the review to the exact mechanically detected change set it judged.
type ReviewRecord struct {
	Result    string   `json:"result"` // accept | recheck | escalate-full
	Recheck   []string `json:"recheck,omitempty"`
	Reason    string   `json:"reason"`
	Session   string   `json:"session"`
	ChangeSet string   `json:"change_set,omitempty"`
}

// HasReviewCoverage reports whether the run carries a review session.
func (r *Run) HasReviewCoverage() bool { return r.CoverageByKey(DeltaReviewKey) != nil }

// buildReviewPlan plans a delta run around the change-review model: the
// mechanical change set (complete for the recorded input surface) plus the
// standing conclusions become one review session. No re-run scope is derived
// mechanically: the reviewer accepts, names re-run keys, or escalates.
func (d *Derivation) buildReviewPlan(run *Run) ([]CoverageKey, []string, []string, []string, error) {
	required := requiredFiles(run)
	full, err := d.computeCoverage(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	baseline, err := validationcache.ReadGateBaseline(d.root, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	switch run.Mode {
	case ModeDelta:
		switch {
		case !baseline.Exists:
			if baseline.UnusableReason != "" {
				return nil, nil, nil, nil, fmt.Errorf("the existing cache is not in the current format and cannot be used as a delta baseline: %s", baseline.UnusableReason)
			}
			if run.Target == TargetStable {
				return nil, nil, nil, nil, noUsableBaselineError(run)
			}
			return nil, nil, nil, nil, fmt.Errorf("no baseline cache for this target — run the full command instead: `specflowctl gate-plan --gate %s %s --target %s`", run.Gate, gateTargetFlags(run), run.Target)
		case baseline.Mode != "full":
			return nil, nil, nil, nil, fmt.Errorf("baseline cache mode is %q, expected full — run the full command instead", baseline.Mode)
		case strings.TrimSpace(baseline.Result) != "" && (baseline.Result == "fail") != baseline.Blocking:
			return nil, nil, nil, nil, fmt.Errorf("baseline cache declarations conflict (result: %q, blocking: %t) — run the full command instead", baseline.Result, baseline.Blocking)
		case baseline.Result == "fail":
			return nil, nil, nil, nil, fmt.Errorf("baseline is a failure record — use `--mode repair` after resolving the findings")
		case baseline.Result != "pass":
			return nil, nil, nil, nil, fmt.Errorf("baseline cache result is %q, expected pass — run the full command instead", baseline.Result)
		}
	case ModeRepair:
		if !baseline.Exists && run.Target == TargetStable {
			return nil, nil, nil, nil, noUsableBaselineError(run)
		}
		if !baseline.Exists || baseline.Result != "fail" || !baseline.Blocking {
			return nil, nil, nil, nil, fmt.Errorf("--mode repair requires a failure-record baseline (result: fail, blocking: true) — none found; run the full command instead")
		}
	default:
		return nil, nil, nil, nil, fmt.Errorf("review planning requires --mode delta or --mode repair")
	}
	state, err := validatedJudgmentState(baseline)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if run.Mode == ModeRepair {
		// A failure record whose per-check status map is absent, invalid, or
		// does not match its judgment baseline cannot say which judgments
		// failed: nothing may be carried over, so the plan is the full
		// coverage set (fail closed — carrying unresolved findings is never
		// allowed).
		if reason, defective := repairStatusMapDefect(baseline, state); defective {
			return full, nil, required, []string{reason + " — the plan covers the full scope"}, nil
		}
	}
	report, err := validationcache.DeriveChangeReport(d.root, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// New physical inputs the baseline never recorded are part of the change
	// set: the reviewer sees them as added files. Logical references resolve
	// through the freshness chain and are not duplicated here.
	var newEvidence []string
	if run.Gate == GateVerify {
		recorded := map[string]bool{}
		for _, entry := range baseline.Entries {
			recorded[filepath.ToSlash(filepath.Clean(entry.Path))] = true
		}
		for _, p := range extraInputPaths(run) {
			if isLogicalRef(p) {
				continue
			}
			clean := filepath.ToSlash(filepath.Clean(p))
			if recorded[clean] {
				continue
			}
			report.Entries = append(report.Entries, validationcache.ChangeReportEntry{Path: p, Kind: "added"})
			newEvidence = append(newEvidence, p)
		}
	}
	inconsistent := report.Inconsistent()
	if len(inconsistent) > 0 {
		return nil, nil, nil, nil, fmt.Errorf("the baseline cache records evidence that contradicts itself for %s — the change cannot be localized; run the full command instead", strings.Join(inconsistent, ", "))
	}
	fingerprint, err := report.Fingerprint()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	current, err := d.currentGateKeys(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	currentSet := map[string]bool{}
	for _, key := range current {
		currentSet[key] = true
	}

	forced := map[string]bool{}
	for _, raw := range run.RerunKeys {
		key, ok := normalizeGateKey(run, raw, currentSet)
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("--rerun key %q is not in the current coverage surface", raw)
		}
		forced[key] = true
	}
	var recordNotices []string
	if run.Gate == GateVerify {
		// A verify judgment record whose own evidence surface no longer holds
		// (changed code, a new caller, a damaged or missing record) cannot be
		// kept: it re-runs unconditionally, and a re-run public record pulls
		// its private design judgment with it.
		currentCoverage, err := d.computeCoverage(run)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		for _, ck := range currentCoverage {
			if ck.Kind == SessionKindDeltaReview {
				continue
			}
			binding, bound := state.Records[ck.Key]
			if !bound {
				if !forced[ck.Key] {
					forced[ck.Key] = true
					recordNotices = append(recordNotices, "verify judgment "+ck.Key+" has no recorded evidence — it re-runs")
				}
				continue
			}
			binding.Layer = run.Target
			inputs, err := normalizeOwnInputs(d.root, coverageReadRefs(d.root, run, ck), ck.Unit, binding.Layer)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			record, rerr := judgments.Load(d.root, binding.Reference)
			if rerr != nil || judgments.Check(d.root, binding.Reference, binding.Layer, run.Protocol) != nil || !judgments.CoversInputs(record.Inputs, inputs) {
				if !forced[ck.Key] {
					forced[ck.Key] = true
					recordNotices = append(recordNotices, "invalidated verify judgment "+ck.Key+" re-runs")
				}
			}
		}
		for _, key := range sortedKeySet(forced) {
			if strings.HasPrefix(key, "code:") {
				designKey := "design:" + run.TargetName + ":" + strings.TrimPrefix(key, "code:")
				if !forced[designKey] {
					forced[designKey] = true
					recordNotices = append(recordNotices, "design judgment "+designKey+" re-runs with its public record")
				}
			}
		}
	}

	var baselineKeys, newKeys, candidates []string
	var retired []string
	if run.Mode == ModeRepair {
		// Failed judgments re-run unconditionally; the repaired remainder is
		// the reviewer's (or, with nothing changed, the mechanical carry's).
		for key, status := range state.LogicalStatus {
			if key == CrossKey {
				continue
			}
			if IsRelationshipKey(key) {
				if status != "fail" {
					continue
				}
				name := relationshipName(key)
				if !stringInSlice(RelationshipNames(run.Gate), name) {
					return nil, nil, nil, nil, fmt.Errorf("baseline declares unknown relationship %q", key)
				}
				run.Relationships = appendUnique(run.Relationships, name)
				continue
			}
			if status == "fail" {
				forced[key] = true
			}
		}
		for _, raw := range baseline.InvalidatedChecks {
			key, ok := normalizeGateKey(run, raw, currentSet)
			if !ok {
				return full, nil, required, []string{fmt.Sprintf("re-run judgment %q is not in the current coverage surface — the plan covers the full scope", raw)}, nil
			}
			forced[key] = true
		}
		if len(baseline.InvalidatedChecks) > 0 {
			recordNotices = append(recordNotices, "persisted targeted invalidations: "+strings.Join(baseline.InvalidatedChecks, ", "))
		}
	}
	for key := range state.LogicalStatus {
		if key == CrossKey {
			continue
		}
		if IsRelationshipKey(key) {
			name := relationshipName(key)
			if !stringInSlice(RelationshipNames(run.Gate), name) {
				return nil, nil, nil, nil, fmt.Errorf("baseline declares unknown relationship %q", key)
			}
			baselineKeys = append(baselineKeys, key)
			if !stringInSlice(run.Relationships, name) {
				candidates = append(candidates, key)
			}
			continue
		}
		if !currentSet[key] {
			retired = append(retired, key)
			continue
		}
		baselineKeys = append(baselineKeys, key)
		if !forced[key] {
			candidates = append(candidates, key)
		}
	}
	for _, key := range current {
		if _, ok := state.LogicalStatus[key]; !ok {
			newKeys = append(newKeys, key)
		}
	}
	sort.Strings(baselineKeys)
	sort.Strings(newKeys)
	sort.Strings(candidates)
	sort.Strings(retired)

	rerunSet := map[string]bool{}
	for key := range forced {
		rerunSet[key] = true
	}
	for _, key := range newKeys {
		rerunSet[key] = true
	}
	if run.Mode == ModeDelta && report.Empty() && len(rerunSet) == 0 && len(run.Relationships) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("cache is fresh — no changes to review")
	}
	// A delta run always reviews its change set; a repair run reviews the
	// change set only when the fix changed content (otherwise the failed
	// judgments re-run against unchanged evidence).
	needReview := run.Mode == ModeDelta || !report.Empty()
	var coverage []CoverageKey
	if needReview {
		coverage = append(coverage, CoverageKey{Key: DeltaReviewKey, Kind: SessionKindDeltaReview})
	}
	selected := coverageKeysForRerun(run, rerunSet)
	rest, err := filterCoverage(full, selected)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// A re-run coverage key owns every report key it re-executes (a validate
	// group re-runs all its checks): those must leave the carry candidates.
	rerunOwned := map[string]bool{}
	for _, ck := range rest {
		for _, key := range run.ReportKeys(ck) {
			rerunOwned[key] = true
		}
	}
	var keptCandidates []string
	for _, key := range candidates {
		if !rerunOwned[key] {
			keptCandidates = append(keptCandidates, key)
		}
	}
	candidates = keptCandidates
	coverage = append(coverage, rest...)

	var notices []string
	if needReview {
		notices = append(notices, fmt.Sprintf("change review: %d changed file(s) — the reviewer accepts, names re-run keys, or escalates", len(report.Entries)))
	} else {
		notices = append(notices, "no content change — the failed judgments re-run and the remaining evidence carries mechanically")
	}
	notices = append(notices, recordNotices...)
	if len(newEvidence) > 0 {
		notices = append(notices, "new verify evidence not in the baseline: "+strings.Join(newEvidence, ", ")+" — it joins the change set as added files")
	}
	if len(newKeys) > 0 {
		notices = append(notices, "new conclusions (not in the baseline) run unconditionally: "+strings.Join(newKeys, ", "))
	}
	if len(retired) > 0 {
		notices = append(notices, "baseline conclusions no longer in the current surface: "+strings.Join(retired, ", "))
	}
	if len(candidates) > 0 {
		notices = append(notices, "standing conclusions pending review: "+strings.Join(candidates, ", "))
	}

	run.ChangeSetFP = fingerprint
	run.BaselineKeys = baselineKeys
	if run.BaselineStatus == nil {
		run.BaselineStatus = map[string]string{}
	}
	for _, key := range baselineKeys {
		run.BaselineStatus[key] = state.LogicalStatus[key]
	}
	run.ChangeReport = report
	return coverage, candidates, required, notices, nil
}

// reviewReadRefs is the review session's read surface: the run's whole input
// surface, so the reviewer can follow a changed file to any conclusion it may
// anchor (framework/verification_scope.md §Delta Runs → The change review).
// Removed paths are not part of the current surface — they survive only as
// change-report entries, which cannot be read.
func reviewReadRefs(repoRoot string, run *Run) []string {
	return InputSurface(repoRoot, run)
}

// normalizeGateKey maps a caller-supplied key spelling to a current coverage
// key. Verify keys accept the short item id or code file path spelling.
func normalizeGateKey(run *Run, key string, currentSet map[string]bool) (string, bool) {
	if currentSet[key] {
		return key, true
	}
	if run.Gate == GateVerify && run.TargetKind == TargetKindUnit {
		for _, candidate := range []string{
			reviewKey(SessionKindItem, run.TargetName, key),
			reviewKey(SessionKindCode, "", key),
			reviewKey(SessionKindDesign, run.TargetName, key),
		} {
			if currentSet[candidate] {
				return candidate, true
			}
		}
	}
	return "", false
}

// ApplyReviewLocked applies one review outcome to an open run and persists
// the run. The caller must hold the repository mutation lock (gate-submit
// runs inside one). The application is idempotent for the same review
// record so a retried submit cannot be rejected by its own earlier effect.
func ApplyReviewLocked(repoRoot string, run *Run, record ReviewRecord) error {
	if !run.HasReviewCoverage() {
		return fmt.Errorf("run %s has no review session", run.RunID)
	}
	if run.Review != nil {
		if run.Review.Session == record.Session && run.Review.Result == record.Result && strings.Join(run.Review.Recheck, ",") == strings.Join(record.Recheck, ",") {
			return nil
		}
		return fmt.Errorf("run %s already carries a review — an accepted review cannot be replaced; plan a new run", run.RunID)
	}
	if run.ChangeSetFP == "" {
		return fmt.Errorf("run %s carries no change-set fingerprint — plan a new run", run.RunID)
	}

	removed := map[string]bool{}
	switch record.Result {
	case "accept":
	case "recheck":
		if len(record.Recheck) == 0 {
			return fmt.Errorf("a recheck review must name at least one key")
		}
		d, err := NewDerivation(repoRoot)
		if err != nil {
			return err
		}
		full, err := d.computeCoverage(run)
		if err != nil {
			return err
		}
		current, err := d.currentGateKeys(run)
		if err != nil {
			return err
		}
		currentSet := map[string]bool{}
		for _, key := range current {
			currentSet[key] = true
		}
		local := map[string]bool{}
		for _, raw := range record.Recheck {
			key := strings.TrimSpace(raw)
			if key == "" {
				return fmt.Errorf("recheck key list carries an empty key")
			}
			if IsRelationshipKey(key) {
				name := relationshipName(key)
				if !stringInSlice(RelationshipNames(run.Gate), name) {
					return fmt.Errorf("recheck key %q is not a known relationship", raw)
				}
				if !stringInSlice(run.BaselineKeys, key) {
					return fmt.Errorf("recheck relationship %q is not a baseline conclusion", raw)
				}
				run.Relationships = appendUnique(run.Relationships, name)
				removed[key] = true
				continue
			}
			norm, ok := normalizeGateKey(run, key, currentSet)
			if !ok {
				return fmt.Errorf("recheck key %q is not in the current coverage surface", raw)
			}
			if !stringInSlice(run.BaselineKeys, norm) {
				return fmt.Errorf("recheck key %q is not a baseline conclusion", raw)
			}
			local[norm] = true
		}
		if len(local) > 0 {
			selected := coverageKeysForRerun(run, local)
			extra, err := filterCoverage(full, selected)
			if err != nil {
				return err
			}
			seen := map[string]bool{}
			for _, ck := range run.Coverage {
				seen[ck.Key] = true
			}
			for _, ck := range extra {
				for _, key := range run.ReportKeys(ck) {
					removed[key] = true
				}
				if seen[ck.Key] {
					continue
				}
				run.Coverage = append(run.Coverage, ck)
			}
		}
	case "escalate-full":
		if run.Mode == ModeFull {
			return fmt.Errorf("this run already covers the full scope — choose accept or recheck")
		}
		run.Escalated = true
	default:
		return fmt.Errorf("unknown review result %q", record.Result)
	}
	if len(removed) > 0 {
		var kept []string
		for _, key := range run.CarriedKeys {
			if !removed[key] {
				kept = append(kept, key)
			}
		}
		run.CarriedKeys = kept
	}
	if run.CarriedLenses != nil {
		carriedSet := map[string]bool{}
		for _, key := range run.CarriedKeys {
			carriedSet[key] = true
		}
		for key := range run.CarriedLenses {
			if !carriedSet[key] {
				delete(run.CarriedLenses, key)
			}
		}
	}

	results, err := loadCarriedResults(repoRoot, run, run.CarriedKeys)
	if err != nil {
		return err
	}
	run.CarriedResults = results
	if run.Gate == GateVerify {
		carriedSet := map[string]bool{}
		for _, key := range run.CarriedKeys {
			carriedSet[key] = true
		}
		for key, ref := range run.Records {
			if ref.Source == "carried" && !carriedSet[key] {
				delete(run.Records, key)
			}
		}
	}
	record.ChangeSet = run.ChangeSetFP
	run.Review = &record
	return Persist(repoRoot, run)
}

// repairStatusMapDefect reports whether a failure record's per-check status
// map is unusable: absent, incomplete, invalid, or inconsistent with the
// record's structured judgment baseline. A defective map cannot say which
// judgments failed, so the repair plan must cover the full scope.
func repairStatusMapDefect(baseline *validationcache.GateBaseline, state JudgmentBaseline) (string, bool) {
	if !baseline.HasChecks || len(baseline.Checks) == 0 {
		return "the failure record carries no per-check status map", true
	}
	declared := map[string]bool{}
	statusByCheck := map[string]string{}
	var missing, invalid []string
	missingSeen := map[string]bool{}
	invalidSeen := map[string]bool{}
	addInvalid := func(reason string) {
		if !invalidSeen[reason] {
			invalidSeen[reason] = true
			invalid = append(invalid, reason)
		}
	}
	fullBasis := baseline.Basis == ModeFull || baseline.Basis == ""
	for _, entry := range baseline.Entries {
		for _, c := range entry.Checks {
			declared[c.Check] = true
			status := strings.TrimSpace(c.Status)
			if prior, ok := statusByCheck[c.Check]; ok && prior != status {
				addInvalid(c.Check + "=conflicting")
			} else {
				statusByCheck[c.Check] = status
			}
			switch {
			case status == "":
				if !missingSeen[c.Check] {
					missingSeen[c.Check] = true
					missing = append(missing, c.Check)
				}
			case status != "pass" && status != "fail" && status != "carried":
				addInvalid(c.Check + "=" + status)
			case status == "carried" && !IsRelationshipKey(c.Check) && fullBasis:
				addInvalid(c.Check + "=carried")
			}
		}
	}
	if len(missing) > 0 {
		return "the failure record carries an incomplete per-check status map (missing: " + strings.Join(missing, ", ") + ")", true
	}
	if len(invalid) > 0 {
		return "the failure record carries an invalid per-check status map (invalid: " + strings.Join(invalid, ", ") + ")", true
	}
	var mismatch []string
	for key := range declared {
		if key == CrossKey {
			continue
		}
		if _, ok := state.LogicalStatus[key]; !ok {
			mismatch = append(mismatch, key)
		}
	}
	for key := range state.LogicalStatus {
		if key == CrossKey {
			continue
		}
		if !declared[key] {
			mismatch = append(mismatch, key)
		}
	}
	if len(mismatch) > 0 {
		sort.Strings(mismatch)
		return "the failure record's per-check status map does not match its judgment baseline (unmatched: " + strings.Join(mismatch, ", ") + ")", true
	}
	return "", false
}

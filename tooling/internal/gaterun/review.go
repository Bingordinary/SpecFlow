package gaterun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// SessionKindDeltaReview is the review session kind of a validate delta run:
// an independent reviewer judges which standing conclusions the mechanically
// detected change set could affect. It produces no check verdicts — it
// accepts the unchanged remainder, names re-run keys, or escalates to a full
// run. The re-run sessions it names are ordinary sessions with the standard
// checklist procedure. A verify delta has no review session: its re-run scope
// is derived from the conclusion records.
const SessionKindDeltaReview = "delta_review"

// DeltaReviewKey is the coverage key of the review session.
const DeltaReviewKey = "review"

// ReviewRecord is the recorded outcome of one delta review. ChangeSet binds
// the review to the exact mechanically detected change set it judged.
// Candidates is the carry-candidate set the review adjudicated: a candidate
// the reviewer left out of Recheck was not re-run by the review, so the
// difference Candidates-minus-Recheck is what the review declined to re-run,
// kept for audit. A declined design judgment may still re-run mechanically to
// stay consistent with a re-run public record.
type ReviewRecord struct {
	Result     string   `json:"result"` // accept | recheck | escalate-full
	Recheck    []string `json:"recheck,omitempty"`
	Reason     string   `json:"reason"`
	Session    string   `json:"session"`
	ChangeSet  string   `json:"change_set,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
}

// HasReviewCoverage reports whether the run carries a review session.
func (r *Run) HasReviewCoverage() bool { return r.CoverageByKey(DeltaReviewKey) != nil }

// standingRelationships returns the relationship names the baseline declares.
// A full-scope plan re-runs every local conclusion, so the final synthesis is
// recomputed over those results and each standing relationship must re-check
// rather than disappear from the rewritten cache.
func standingRelationships(state JudgmentBaseline) []string {
	return append([]string(nil), state.Relationships...)
}

// addRelationships merges names into the run's relationship scope. The caller
// keeps run.Relationships within the gate's vocabulary; validation runs again
// after the plan is built.
func addRelationships(run *Run, names ...string) {
	for _, name := range names {
		run.Relationships = appendUnique(run.Relationships, name)
	}
	run.Relationships = dedupeSorted(run.Relationships)
}

// buildMechanicalVerifyPlan plans a verify delta/repair run without a change
// review. The re-run set is derived mechanically from the conclusion records:
// a conclusion re-runs exactly when its own pinned evidence no longer holds,
// its review protocol moved, it is explicitly invalidated, the evidence its key
// currently requires is no longer inside the evidence the record read, or it
// has no baseline record; every other standing conclusion carries. Supplied
// evidence that only the final synthesis owns re-runs the synthesis, not every
// local conclusion. There is no review session and no "new evidence"
// escalation: coverage is derived, not adjudicated.
func (d *Derivation) buildMechanicalVerifyPlan(run *Run) ([]CoverageKey, []string, []string, []string, error) {
	required := requiredFiles(run)
	full, err := d.computeCoverage(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	baseline, err := validationcache.ReadGateBaseline(d.root, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// salvagedBaseline marks a pass cache whose finalize persisted pending
	// invalidations (mid-run input drift): repair force re-runs exactly that
	// list, so it is a valid repair baseline even though it is not a failure
	// record.
	salvagedBaseline := baseline.Exists && baseline.Result == "pass" && len(baseline.InvalidatedChecks) > 0
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
			return nil, nil, nil, nil, fmt.Errorf("no baseline cache for this target — run the full command instead: %s", PlanCommand(run, ModeFull))
		case baseline.Mode != "full":
			return nil, nil, nil, nil, fmt.Errorf("baseline cache mode is %q, expected full — run the full command instead", baseline.Mode)
		case len(baseline.InvalidatedChecks) > 0:
			return nil, nil, nil, nil, fmt.Errorf("baseline cache carries pending invalidations (%s) — run `--mode repair` to re-run them, or the full command instead", strings.Join(baseline.InvalidatedChecks, ", "))
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
		if !salvagedBaseline && (!baseline.Exists || baseline.Result != "fail" || !baseline.Blocking) {
			return nil, nil, nil, nil, fmt.Errorf("--mode repair requires a failure-record baseline (result: fail, blocking: true) or a salvaged pass cache with pending invalidations — none found; run the full command instead")
		}
	default:
		return nil, nil, nil, nil, fmt.Errorf("verify planning requires --mode delta or --mode repair")
	}
	state, err := validatedJudgmentState(baseline)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	standing := standingRelationships(state)
	if run.Mode == ModeRepair && !salvagedBaseline {
		// A failure record whose per-check status map is absent, invalid, or
		// inconsistent with its judgment baseline cannot say which judgments
		// failed: nothing is carried, so the plan covers the full scope. The
		// final synthesis is a conclusion over the full local result, so the
		// baseline's standing relationships re-check rather than vanish. A
		// salvaged pass cache has no status map by design (nothing failed);
		// its forced set is the persisted invalidation list.
		if reason, defective := repairStatusMapDefect(baseline, state); defective {
			addRelationships(run, standing...)
			return full, nil, required, []string{reason + " — the plan covers the full scope"}, nil
		}
	}

	current, err := d.currentGateKeys(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	currentSet := map[string]bool{}
	for _, key := range current {
		currentSet[key] = true
	}

	var notices []string
	forced := map[string]bool{}
	for _, raw := range run.RerunKeys {
		key, ok := normalizeGateKey(run, raw, currentSet)
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("--rerun key %q is not in the current coverage surface", raw)
		}
		forced[key] = true
	}

	// A conclusion re-runs exactly when its own record no longer holds. Two
	// things decide that, and both must agree with the gate's own freshness
	// check: the record's pinned files, structural regions and review protocol
	// are unchanged (judgments.Check), and the evidence the key currently
	// requires stayed inside the evidence the record read. The second half is
	// the same rule CheckVerifyMerged enforces, so it is taken from the same
	// authority (ExpectedChecks) through the same predicate
	// (judgments.NewlyRequiredEvidence) — the planner can never carry a
	// conclusion the merged freshness check then rejects.
	expected, err := d.ExpectedChecks(run.TargetName, run.Target)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	requiredByKey := make(map[string]validationcache.ExpectedCheck, len(expected))
	for _, want := range expected {
		requiredByKey[want.Key] = want
	}
	for _, ck := range full {
		binding, bound := state.Records[ck.Key]
		if !bound {
			forced[ck.Key] = true
			notices = append(notices, "verify judgment "+ck.Key+" has no recorded evidence — it re-runs")
			continue
		}
		binding.Layer = run.Target
		if err := judgments.Check(d.root, binding.Reference, binding.Layer, run.Protocol); err != nil {
			forced[ck.Key] = true
			notices = append(notices, "verify judgment "+ck.Key+" re-runs: "+err.Error())
			continue
		}
		want, ok := requiredByKey[ck.Key]
		if !ok {
			continue
		}
		record, err := judgments.Load(d.root, binding.Reference)
		if err != nil {
			forced[ck.Key] = true
			notices = append(notices, "verify judgment "+ck.Key+" re-runs: "+err.Error())
			continue
		}
		missing, newlyRequired, err := judgments.NewlyRequiredEvidence(d.root, want.Unit, want.Layer, *record, want.Inputs)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if newlyRequired {
			forced[ck.Key] = true
			notices = append(notices, "verify judgment "+ck.Key+" re-runs: the key now requires evidence the recorded judgment never read: "+missing)
		}
	}
	// A public quality record and its design companion travel together.
	for _, key := range sortedKeySet(forced) {
		if strings.HasPrefix(key, "code:") {
			forced["design:"+run.TargetName+":"+strings.TrimPrefix(key, "code:")] = true
		}
	}

	if run.Mode == ModeRepair {
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
					return nil, nil, nil, nil, fmt.Errorf("baseline declares unknown relationship %q", name)
				}
				run.Relationships = appendUnique(run.Relationships, name)
				continue
			}
			if status == "fail" {
				if !currentSet[key] {
					addRelationships(run, standing...)
					notices = append(notices, fmt.Sprintf("re-run judgment %q is not in the current coverage surface — the plan covers the full scope", key))
					return full, nil, required, notices, nil
				}
				forced[key] = true
			}
		}
		for _, raw := range baseline.InvalidatedChecks {
			key, ok := normalizeGateKey(run, raw, currentSet)
			if !ok {
				addRelationships(run, standing...)
				notices = append(notices, fmt.Sprintf("re-run judgment %q is not in the current coverage surface — the plan covers the full scope", raw))
				return full, nil, required, notices, nil
			}
			forced[key] = true
		}
	}

	var baselineKeys, newKeys, retired []string
	for key := range state.LogicalStatus {
		if key == CrossKey {
			continue
		}
		if IsRelationshipKey(key) {
			if !stringInSlice(RelationshipNames(run.Gate), relationshipName(key)) {
				return nil, nil, nil, nil, fmt.Errorf("baseline declares unknown relationship %q", relationshipName(key))
			}
			baselineKeys = append(baselineKeys, key)
			continue
		}
		if !currentSet[key] {
			retired = append(retired, key)
			continue
		}
		baselineKeys = append(baselineKeys, key)
	}
	for _, key := range current {
		if _, ok := state.LogicalStatus[key]; !ok {
			newKeys = append(newKeys, key)
		}
	}
	sort.Strings(baselineKeys)
	sort.Strings(newKeys)
	sort.Strings(retired)

	// Report recorded evidence that changed but that no conclusion depends on.
	// Conclusion-bound reuse cannot see a semantic dependency the record never
	// declared, so the tooling tells the coordinator when a full run is
	// warranted: as a plan notice, and — when the plan has no work at all — in
	// the fresh-stop message below.
	uncovered, uerr := validationcache.UncoveredChangedFilesForUnit(d.root, run.TargetName)
	if uerr != nil {
		uncovered = nil
	}
	if len(uncovered) > 0 {
		notices = append(notices, fmt.Sprintf("%d recorded file(s) changed that no conclusion depends on — a full run confirms no semantic dependency was missed: %s", len(uncovered), strings.Join(uncovered, ", ")))
	}

	// A physical evidence input the baseline never recorded is background
	// material: it creates no coverage key and re-runs no conclusion. The plan
	// still names the new paths so the coordinator can confirm its manifest
	// (framework/validation_cache.md §Write Rules → gate-plan). A spec-object
	// spelling is already carried as its logical reference by the ingestion
	// boundary, so only physical paths are compared.
	recorded := map[string]bool{}
	for _, entry := range baseline.Entries {
		recorded[filepath.ToSlash(filepath.Clean(entry.Path))] = true
	}
	var newEvidence []string
	for _, p := range extraInputPaths(d.root, run) {
		if isLogicalRef(p) {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(p))
		if recorded[clean] {
			continue
		}
		newEvidence = append(newEvidence, p)
	}
	if len(newEvidence) > 0 {
		notices = append(notices, "new evidence not in the baseline: "+strings.Join(newEvidence, ", ")+" — background material; it re-runs nothing")
	}

	// A changed local conclusion re-runs the whole relationship scope: the
	// synthesis consumes the local results, so a carried relationship would be
	// a verdict over superseded inputs.
	localChanged := len(forced) > 0 || len(newKeys) > 0
	if dead := synthesisEvidenceDead(d.root, state); dead != "" {
		if len(standing) == 0 {
			notices = append(notices, "synthesis evidence changed ("+dead+") and the baseline declares no relationship scope — the plan covers the full scope")
			return full, nil, required, notices, nil
		}
		addRelationships(run, standing...)
		localChanged = true
		notices = append(notices, "synthesis evidence changed: "+dead+" — the standing relationship conclusions re-run")
	}
	if localChanged {
		addRelationships(run, standing...)
	}

	rerunSet := map[string]bool{}
	for key := range forced {
		rerunSet[key] = true
	}
	for _, key := range newKeys {
		rerunSet[key] = true
	}
	if len(rerunSet) == 0 && len(run.Relationships) == 0 {
		msg := "cache is fresh — no conclusion's evidence changed; there is nothing to re-run"
		if len(uncovered) > 0 {
			msg += fmt.Sprintf(". Note: %d recorded file(s) changed that no conclusion depends on — run `verify@%s` (full) to confirm no semantic dependency was missed: %s", len(uncovered), run.TargetName, strings.Join(uncovered, ", "))
		}
		return nil, nil, nil, nil, errors.New(msg)
	}

	selected := coverageKeysForRerun(run, rerunSet)
	rest, err := filterCoverage(full, selected)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// A re-run coverage key owns every report key it re-executes: those must
	// leave the carry candidates.
	rerunOwned := map[string]bool{}
	for _, ck := range rest {
		for _, key := range run.ReportKeys(ck) {
			rerunOwned[key] = true
		}
	}
	var carried []string
	for _, key := range baselineKeys {
		if forced[key] || rerunOwned[key] {
			continue
		}
		// Only a relationship this run actually re-checks leaves the carry
		// set. A standing relationship the coordinator did not select is not
		// re-run, so dropping it here would erase a recorded cross-check
		// conclusion without re-checking or reporting it
		// (framework/verification_scope.md §Incremental relationship work).
		if IsRelationshipKey(key) && stringInSlice(run.Relationships, relationshipName(key)) {
			continue
		}
		carried = append(carried, key)
	}

	run.BaselineKeys = dedupeSorted(append([]string(nil), baselineKeys...))
	if run.BaselineStatus == nil {
		run.BaselineStatus = map[string]string{}
	}
	for _, key := range baselineKeys {
		run.BaselineStatus[key] = state.LogicalStatus[key]
	}

	notices = append(notices, fmt.Sprintf("%d conclusion(s) re-run from their own evidence pins; %d carry mechanically (no change review)", len(rerunSet), len(carried)))
	if len(newKeys) > 0 {
		notices = append(notices, "new conclusions (not in the baseline) run unconditionally: "+strings.Join(newKeys, ", "))
	}
	if len(retired) > 0 {
		notices = append(notices, "baseline conclusions no longer in the current surface: "+strings.Join(retired, ", "))
	}
	return rest, carried, required, notices, nil
}

// synthesisEvidenceDead returns the first final-synthesis evidence pin whose
// content no longer matches, or "" when every pin holds. A cache written
// before the pin existed carries none, and nothing is reported: the gap is
// bounded to that cache and closes when the next run writes the pins.
func synthesisEvidenceDead(root string, state JudgmentBaseline) string {
	for _, pin := range state.SynthesisEvidence {
		canonical, err := judgments.ResolveEvidencePath(root, pin.Path)
		if err != nil {
			return pin.Path
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(canonical)))
		if err != nil {
			return pin.Path
		}
		if pin.Hash == "" || specpaths.HashText(data) != pin.Hash {
			return pin.Path
		}
	}
	return ""
}

// buildReviewPlan plans a validate delta run around the change-review model:
// the mechanical change set (complete for the recorded input surface) plus the
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
	// salvagedBaseline marks a pass cache whose finalize persisted pending
	// invalidations (mid-run input drift): repair force re-runs exactly that
	// list, so it is a valid repair baseline even though it is not a failure
	// record.
	salvagedBaseline := baseline.Exists && baseline.Result == "pass" && len(baseline.InvalidatedChecks) > 0
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
			return nil, nil, nil, nil, fmt.Errorf("no baseline cache for this target — run the full command instead: %s", PlanCommand(run, ModeFull))
		case baseline.Mode != "full":
			return nil, nil, nil, nil, fmt.Errorf("baseline cache mode is %q, expected full — run the full command instead", baseline.Mode)
		case len(baseline.InvalidatedChecks) > 0:
			return nil, nil, nil, nil, fmt.Errorf("baseline cache carries pending invalidations (%s) — run `--mode repair` to re-run them, or the full command instead", strings.Join(baseline.InvalidatedChecks, ", "))
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
		if !salvagedBaseline && (!baseline.Exists || baseline.Result != "fail" || !baseline.Blocking) {
			return nil, nil, nil, nil, fmt.Errorf("--mode repair requires a failure-record baseline (result: fail, blocking: true) or a salvaged pass cache with pending invalidations — none found; run the full command instead")
		}
	default:
		return nil, nil, nil, nil, fmt.Errorf("review planning requires --mode delta or --mode repair")
	}
	state, err := validatedJudgmentState(baseline)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	standing := standingRelationships(state)
	if run.Mode == ModeRepair && !salvagedBaseline {
		// A failure record whose per-check status map is absent, invalid, or
		// does not match its judgment baseline cannot say which judgments
		// failed: nothing may be carried over, so the plan is the full
		// coverage set (fail closed — carrying unresolved findings is never
		// allowed). The full local result re-runs the synthesis, so the
		// standing relationship conclusions re-check too. A salvaged pass
		// cache has no status map by design (nothing failed); its forced set
		// is the persisted invalidation list.
		if reason, defective := repairStatusMapDefect(baseline, state); defective {
			addRelationships(run, standing...)
			return full, nil, required, []string{reason + " — the plan covers the full scope"}, nil
		}
	}
	// A validate conclusion binds the judging standard it was made under on
	// its cache. A conclusion judged under a different checklist cannot be
	// carried, whatever its own evidence says, so nothing carries over and the
	// plan is the full coverage set.
	if reason, moved := validateStandardMoved(run, baseline); moved {
		addRelationships(run, standing...)
		// Nothing carries across a standard change, so this is a full run in
		// substance. Recording it as such makes gate-finalize write
		// `basis: full`; a delta basis would require a change-review record
		// the full-scope plan never schedules.
		run.Mode = ModeFull
		return full, nil, required, []string{reason + " — the plan covers the full scope"}, nil
	}
	report, err := validationcache.DeriveChangeReport(d.root, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// New physical inputs the baseline never recorded are part of the change
	// set: the reviewer sees them as added files
	// (framework/verification_scope.md §Delta Runs → Change detection). The
	// cache's own required inputs — its appendices and the published stable
	// global rules — are added by DeriveChangeReport; this block adds the
	// plan's supplementary inputs and the logical refs the baseline never
	// recorded, without duplicating an entry the report already carries.
	changeKey := func(p string) string {
		if isLogicalRef(p) {
			return p
		}
		return filepath.ToSlash(filepath.Clean(p))
	}
	var newEvidence []string
	recorded := map[string]bool{}
	for _, entry := range baseline.Entries {
		recorded[changeKey(entry.Path)] = true
	}
	added := map[string]bool{}
	for _, entry := range report.Entries {
		if entry.Kind == "added" {
			added[changeKey(entry.Path)] = true
		}
	}
	addAdded := func(path string) bool {
		k := changeKey(path)
		if recorded[k] || added[k] {
			return false
		}
		report.Entries = append(report.Entries, validationcache.ChangeReportEntry{Path: path, Kind: "added"})
		added[k] = true
		return true
	}
	for _, p := range extraInputPaths(d.root, run) {
		if isLogicalRef(p) {
			continue
		}
		if addAdded(p) {
			newEvidence = append(newEvidence, p)
		}
	}
	// The target's own current files and the current logical read refs are
	// required inputs even when the old cache never recorded them: a new
	// appendix or a new stable global rule must reach the change review
	// instead of being reported as "nothing changed".
	for _, p := range run.OwnSpecFiles {
		if recorded[changeKey(p)] {
			continue
		}
		if ref := logicalAppendixRef(run, p); ref != "" && recorded[ref] {
			continue
		}
		addAdded(p)
		newEvidence = append(newEvidence, p)
	}
	for _, ref := range run.Refs {
		if !isLogicalRef(ref.Ref) || recorded[changeKey(ref.Ref)] {
			continue
		}
		addAdded(ref.Ref)
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
				// A failed judgment whose object left the surface cannot
				// re-run and cannot be carried: the record can no longer say
				// which judgments are missing, so nothing is carried over and
				// the plan is the full coverage set (fail closed). This is
				// the same degradation the persisted-invalidation branch
				// below applies to an unmappable key.
				if !currentSet[key] {
					addRelationships(run, standing...)
					return full, nil, required, []string{fmt.Sprintf("re-run judgment %q is not in the current coverage surface — the plan covers the full scope", key)}, nil
				}
				forced[key] = true
			}
		}
		for _, raw := range baseline.InvalidatedChecks {
			key, ok := normalizeGateKey(run, raw, currentSet)
			if !ok {
				addRelationships(run, standing...)
				return full, nil, required, []string{fmt.Sprintf("re-run judgment %q is not in the current coverage surface — the plan covers the full scope", raw)}, nil
			}
			forced[key] = true
		}
		if len(baseline.InvalidatedChecks) > 0 {
			recordNotices = append(recordNotices, "persisted invalidations: "+strings.Join(baseline.InvalidatedChecks, ", "))
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
		notices = append(notices, "new evidence not in the baseline: "+strings.Join(newEvidence, ", ")+" — it joins the change set as added files")
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
// anchor (framework/verification_scope.md §Delta Runs → The change review (validate)).
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

// recheckVocabulary renders the keys a review may name, in the same spelling
// and order the mission prints under "Standing conclusions" (a unit validate
// key is the bare check number). It makes a rejected `Recheck:` line
// self-correcting: the reviewer sees the accepted spellings instead of having
// to read tooling source.
func recheckVocabulary(run *Run) string {
	if len(run.BaselineKeys) == 0 {
		return "(this run has no standing conclusions to name)"
	}
	return strings.Join(run.BaselineKeys, ", ")
}

// normalizeRecheckKeys rewrites a review's named re-run keys to canonical
// spellings and validates each against the run. A verify key may be named by
// its short item id or code-file path, so the recorded review and the derived
// declined set must be normalized to one spelling before either is stored or
// compared. A relationship keeps its `relationship:<name>` key. An unknown
// key, a key outside the current coverage surface, or a non-baseline key fails
// closed. The result is de-duplicated in first-seen order.
func normalizeRecheckKeys(run *Run, d *Derivation, keys []string) ([]string, error) {
	current, err := d.currentGateKeys(run)
	if err != nil {
		return nil, err
	}
	currentSet := map[string]bool{}
	for _, key := range current {
		currentSet[key] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, fmt.Errorf("recheck key list carries an empty key")
		}
		canonical := key
		if IsRelationshipKey(key) {
			name := relationshipName(key)
			if !stringInSlice(RelationshipNames(run.Gate), name) {
				return nil, fmt.Errorf("recheck key %q is not a known relationship", raw)
			}
			if !stringInSlice(run.BaselineKeys, key) {
				return nil, fmt.Errorf("recheck relationship %q is not a baseline conclusion; name one of the standing conclusions: %s", raw, recheckVocabulary(run))
			}
		} else {
			norm, ok := normalizeGateKey(run, key, currentSet)
			if !ok {
				return nil, fmt.Errorf("recheck key %q is not in the current coverage surface; name one of the standing conclusions: %s", raw, recheckVocabulary(run))
			}
			if !stringInSlice(run.BaselineKeys, norm) {
				return nil, fmt.Errorf("recheck key %q is not a baseline conclusion; name one of the standing conclusions: %s", raw, recheckVocabulary(run))
			}
			canonical = norm
		}
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	return out, nil
}

// ApplyReviewLocked applies one review outcome to an open run and persists
// the run. The caller must hold the repository mutation lock (gate-submit
// runs inside one). The application is idempotent for the same review
// record so a retried submit cannot be rejected by its own earlier effect.
func ApplyReviewLocked(repoRoot string, run *Run, record ReviewRecord) error {
	if !run.HasReviewCoverage() {
		return fmt.Errorf("run %s has no review session", run.RunID)
	}
	if run.ChangeSetFP == "" {
		return fmt.Errorf("run %s carries no change-set fingerprint — plan a new run", run.RunID)
	}

	d, err := NewDerivation(repoRoot)
	if err != nil {
		return err
	}
	full, err := d.computeCoverage(run)
	if err != nil {
		return err
	}

	// Normalize the named re-run keys to canonical coverage keys before
	// anything consumes them: the recorded review (review_recheck) and the
	// derived declined set (review_declined) must agree on one spelling, and
	// the stable canonical form keeps the idempotency check below valid across
	// a retried submit.
	if record.Result == "recheck" {
		normalized, err := normalizeRecheckKeys(run, d, record.Recheck)
		if err != nil {
			return err
		}
		record.Recheck = normalized
	}

	if run.Review != nil {
		if run.Review.Session == record.Session && run.Review.Result == record.Result && strings.Join(run.Review.Recheck, ",") == strings.Join(record.Recheck, ",") {
			return nil
		}
		return fmt.Errorf("run %s already carries a review — an accepted review cannot be replaced; plan a new run", run.RunID)
	}

	// The carry-candidate set offered to the review: a candidate the reviewer
	// left out of Recheck is not re-run, so Candidates-minus-Recheck is what
	// the review declined to re-run (kept for audit).
	record.Candidates = append([]string{}, run.CarriedKeys...)

	removed := map[string]bool{}
	switch record.Result {
	case "accept":
	case "recheck":
		if len(record.Recheck) == 0 {
			return fmt.Errorf("a recheck review must name at least one key")
		}
		// record.Recheck is canonical and validated by normalizeRecheckKeys;
		// this branch only applies the side effects.
		local := map[string]bool{}
		for _, key := range record.Recheck {
			if IsRelationshipKey(key) {
				run.Relationships = appendUnique(run.Relationships, relationshipName(key))
				removed[key] = true
				continue
			}
			local[key] = true
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
	record.ChangeSet = run.ChangeSetFP
	run.Review = &record
	return Persist(repoRoot, run)
}

// validateStandardMoved reports whether a validate baseline was judged under a
// standard other than the one this run deploys. A cache that records no
// standard cannot prove it was judged by the current one, so it counts as
// moved.
func validateStandardMoved(run *Run, baseline *validationcache.GateBaseline) (string, bool) {
	if baseline.Standard == run.Protocol {
		return "", false
	}
	if strings.TrimSpace(baseline.Standard) == "" {
		return "the baseline validate cache records no judging standard", true
	}
	return "the validate standard changed since the baseline cache was written", true
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

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

// The validate delta plan is mechanical. It compares the current per-object
// catalog against the baseline catalog and the recorded change set: an object
// re-runs when it is not in the baseline catalog, its read set changed, or a read
// ref's region changed; a coverage group re-runs when any of its objects
// re-runs and the rest carry. Every group changed, a failure record, a salvaged
// cache, a moved standard, an unusable baseline, or an unrecorded or
// unattributable input covers the full scope, and the planner upgrades to full
// when the estimated delta cost is not lower. No reviewer decides the scope
// (framework/verification_scope.md §Delta Runs).

// standingRelationships returns the relationship names the baseline declares.
// A full-scope plan re-runs every local conclusion, so the final synthesis is
// recomputed over those results and each standing relationship must re-check
// rather than disappear from the rewritten cache.
func standingRelationships(state JudgmentBaseline) []string {
	return append([]string(nil), state.Relationships...)
}

// blockingBaselineFindings returns the findings of a baseline judgment state
// that the recovering run's check sessions must dispose: the P0/P1 findings
// bound to at least one local check key. A relationship-only finding (a cross
// finding) is not injected — the full-scope plan re-assigns the baseline's
// standing relationships, so the re-run relationship conclusions re-derive
// their defects. P2/P3 advisories are not injected either: they are re-judged
// naturally when their owning check re-runs
// (framework/validation_cache.md §Advisory carry).
func blockingBaselineFindings(state JudgmentBaseline) []Finding {
	var out []Finding
	for _, finding := range state.Findings {
		if finding.Severity != "P0" && finding.Severity != "P1" {
			continue
		}
		for _, key := range BaselineFindingKeys(finding) {
			if !IsRelationshipKey(key) {
				out = append(out, finding)
				break
			}
		}
	}
	return out
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

// buildMechanicalVerifyPlan plans a verify delta run without a change
// review. The incremental mode is derived inside the planner: a failure-record
// baseline or a salvaged pass cache upgrades the plan to a repair, which
// re-runs the failed and invalidated judgments unconditionally. The re-run set
// is derived mechanically from the conclusion records:
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
	// The incremental mode is a pure function of the baseline state, so the
	// planner derives it instead of taking it from the command line: a clean
	// pass baseline plans a delta (re-run the conclusions whose own pins no
	// longer hold), and a failure record or a salvaged pass cache plans a
	// repair (re-run the failed and invalidated judgments unconditionally).
	// Every other baseline shape is no incremental baseline at all — the plan
	// refuses and directs the full command (framework/verification_scope.md
	// §Delta Runs → Failure recovery).
	switch {
	case !baseline.Exists:
		if baseline.UnusableReason != "" {
			return nil, nil, nil, nil, fmt.Errorf("the existing cache is unusable as an incremental baseline: %s", baseline.UnusableReason)
		}
		if run.Target == TargetStable {
			return nil, nil, nil, nil, noUsableBaselineError(run)
		}
		return nil, nil, nil, nil, fmt.Errorf("no baseline cache for this target — run the full command instead: %s", PlanCommand(run, ModeFull))
	case baseline.Mode != "full":
		return nil, nil, nil, nil, fmt.Errorf("baseline cache mode is %q, expected full — run the full command instead", baseline.Mode)
	case strings.TrimSpace(baseline.Result) != "" && (baseline.Result == "fail") != baseline.Blocking:
		return nil, nil, nil, nil, fmt.Errorf("baseline cache declarations conflict (result: %q, blocking: %t) — run the full command instead", baseline.Result, baseline.Blocking)
	case baseline.Result == "fail":
		run.Mode = ModeRepair
	case salvagedBaseline:
		run.Mode = ModeRepair
	case baseline.Result != "pass":
		return nil, nil, nil, nil, fmt.Errorf("baseline cache result is %q, expected pass — run the full command instead", baseline.Result)
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
		record, err := judgments.Load(d.root, binding.Reference)
		if err != nil {
			forced[ck.Key] = true
			notices = append(notices, "verify judgment "+ck.Key+" re-runs: "+err.Error())
			continue
		}
		if err := judgments.Verify(d.root, record, binding.Reference, binding.Layer, run.Protocol); err != nil {
			forced[ck.Key] = true
			notices = append(notices, "verify judgment "+ck.Key+" re-runs: "+err.Error())
			continue
		}
		want, ok := requiredByKey[ck.Key]
		if !ok {
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

	notices = append(notices, fmt.Sprintf("%d conclusion(s) re-run from their own evidence pins; %d carry mechanically", len(rerunSet), len(carried)))
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

// changeReportKey normalizes a change-report path for comparison with the
// run's own-spec and extra-input spellings.
func changeReportKey(p string) string {
	if isLogicalRef(p) {
		return p
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// buildDeltaPlan plans a validate delta run mechanically from the per-object
// catalog: an object whose read set changed re-runs, its coverage group
// re-runs, and groups whose objects are all unchanged carry. Every group
// changed, a failure record, a salvaged cache, a moved judging standard, an
// unusable baseline, or an unrecorded/unattributable input covers the full
// scope. The coordinator decides nothing; when the plan upgrades, the run is
// recorded as a full run (basis: full).
func (d *Derivation) buildDeltaPlan(run *Run) ([]CoverageKey, []string, []string, []string, error) {
	required := requiredFiles(run)
	full, err := d.computeCoverage(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	baseline, err := validationcache.ReadGateBaseline(d.root, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// upgrade plans the full scope. Nothing about a whole-document judgment
	// carries across a changed spec, an unusable baseline, or a moved
	// standard, so the run becomes a full run in substance.
	upgrade := func(reason string, standing []string) ([]CoverageKey, []string, []string, []string, error) {
		addRelationships(run, standing...)
		run.Mode = ModeFull
		return full, nil, required, []string{reason + " — the plan covers the full scope"}, nil
	}

	if !baseline.Exists {
		if baseline.UnusableReason != "" {
			return upgrade("the existing cache is unusable as a delta baseline: "+baseline.UnusableReason, nil)
		}
		if run.Target == TargetStable {
			return nil, nil, nil, nil, noUsableBaselineError(run)
		}
		return nil, nil, nil, nil, fmt.Errorf("no baseline cache for this target — run the full command instead: %s", PlanCommand(run, ModeFull))
	}
	state, stateErr := validatedJudgmentState(baseline)
	if stateErr != nil {
		return upgrade("the baseline judgment state is unusable: "+stateErr.Error(), nil)
	}
	standing := standingRelationships(state)
	if baseline.Mode != "full" {
		return upgrade(fmt.Sprintf("baseline cache mode is %q, expected full", baseline.Mode), standing)
	}
	if len(baseline.InvalidatedChecks) > 0 {
		return upgrade("the baseline carries pending invalidations ("+strings.Join(baseline.InvalidatedChecks, ", ")+")", standing)
	}
	if strings.TrimSpace(baseline.Result) != "" && (baseline.Result == "fail") != baseline.Blocking {
		return upgrade(fmt.Sprintf("baseline cache declarations conflict (result: %q, blocking: %t)", baseline.Result, baseline.Blocking), standing)
	}
	if baseline.Result != "pass" {
		// The scope rule is unchanged: a failure record is not a usable delta
		// baseline, so the plan covers the full scope. The record's P0/P1
		// findings travel with the plan instead — every re-run check session
		// receives the findings its check keys own and must dispose each one,
		// so the recovery verifies the fixes instead of re-discovering them
		// from zero (framework/verification_scope.md §Delta Runs → Failure
		// recovery).
		run.BaselineFindings = blockingBaselineFindings(state)
		notice := fmt.Sprintf("baseline cache result is %q, expected pass", baseline.Result)
		if len(run.BaselineFindings) > 0 {
			notice += fmt.Sprintf(" — the failure record's %d P0/P1 finding(s) are injected into the re-run sessions for disposition", len(run.BaselineFindings))
		}
		return upgrade(notice+" — the plan covers the full scope", standing)
	}
	if reason, moved := validateStandardMoved(run, baseline); moved {
		return upgrade(reason, standing)
	}
	report, err := validationcache.DeriveChangeReport(d.root, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return upgrade("the change set cannot be derived: "+err.Error(), standing)
	}
	if len(report.Inconsistent()) > 0 {
		return upgrade("the baseline records evidence that contradicts itself for "+strings.Join(report.Inconsistent(), ", "), standing)
	}
	// An input the current plan cannot attribute to the unit's own spec union
	// or to a declared external reference is read by the judgments but is not
	// part of any object's derived read set. Fail safe: cover the full scope
	// instead of carrying an unattributed input.
	ownSet := map[string]bool{}
	for _, path := range run.OwnSpecFiles {
		ownSet[changeReportKey(path)] = true
		if ref := logicalEvidenceRef(run, path); ref != "" {
			ownSet[changeReportKey(ref)] = true
		}
	}
	rulePaths := map[string]bool{}
	for _, ref := range run.Refs {
		if !strings.HasPrefix(ref.Ref, "rule:") {
			continue
		}
		rulePaths[changeReportKey(ref.Ref)] = true
		if ref.Resolved != "" {
			rulePaths[changeReportKey(ref.Resolved)] = true
		}
	}
	extraSet := map[string]bool{}
	for _, path := range extraInputPaths(d.root, run) {
		extraSet[changeReportKey(path)] = true
	}
	for _, entry := range report.Entries {
		if entry.Kind == "promise" {
			continue
		}
		key := changeReportKey(entry.Path)
		if ownSet[key] {
			continue
		}
		if rulePaths[key] {
			continue
		}
		if extraSet[key] {
			// A declared extra input is readable background for every group.
			// The catalog has no object-level read set for it, so a change
			// cannot be localized safely; cover the full scope.
			return upgrade("a declared extra input changed: "+entry.Path, standing)
		}
		return upgrade("the baseline records an input that is not part of the unit's own spec union: "+entry.Path, standing)
	}
	// A current input the baseline never recorded — a new own evidence record,
	// a newly declared extra input, or a main spec entry the baseline dropped —
	// has no recorded judgment to carry. The plan covers the full scope.
	recorded := map[string]bool{}
	for _, entry := range baseline.Entries {
		recorded[changeReportKey(entry.Path)] = true
	}
	for _, path := range run.OwnSpecFiles {
		if recorded[changeReportKey(path)] {
			continue
		}
		if ref := logicalEvidenceRef(run, path); ref != "" && recorded[changeReportKey(ref)] {
			continue
		}
		return upgrade("a current own input was not recorded in the baseline: "+path, standing)
	}
	for _, path := range extraInputPaths(d.root, run) {
		if !recorded[changeReportKey(path)] {
			return upgrade("a current declared input was not recorded in the baseline: "+path, standing)
		}
	}

	selection, err := d.selectValidateObjectScope(run, state, report)
	if err != nil {
		return upgrade("the per-object delta scope cannot be derived: "+err.Error(), standing)
	}
	ruleChanged, providerChanged := selection.RuleChanged, selection.ProviderChanged

	current, err := d.currentGateKeys(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	currentSet := map[string]bool{}
	for _, key := range current {
		currentSet[key] = true
	}
	rerunSet := map[string]bool{}
	for _, raw := range run.RerunKeys {
		key, ok := normalizeGateKey(run, raw, currentSet)
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("--rerun key %q is not in the current coverage surface", raw)
		}
		rerunSet[key] = true
	}
	// An explicit --rerun check forces every object judged by that check into
	// the object batch, even when its own read set is unchanged.
	for _, object := range run.ValidateObjects {
		check := object.Check
		if rerunSet[check] {
			selection.RerunObjects[object.ID] = true
		}
	}
	run.RerunObjects = run.RerunObjects[:0]
	for key := range selection.RerunObjects {
		run.RerunObjects = append(run.RerunObjects, key)
	}
	sort.Strings(run.RerunObjects)
	if rerunSet["8"] {
		ruleChanged = true
	}
	if rerunSet["7"] {
		providerChanged = true
	}
	selected := map[string]bool{}
	for group := range selection.SelectedGroups {
		selected[group] = true
	}
	for _, key := range coverageKeysForRerun(run, rerunSet) {
		selected[key] = true
	}
	if len(selection.RerunObjects) >= len(run.ValidateObjects) {
		return upgrade("every object has changed", standing)
	}
	if len(selected) == 0 && len(run.Relationships) == 0 {
		return nil, nil, nil, nil, errors.New("cache is fresh — no changes")
	}

	// A relationship is a conclusion over the local results. The design and
	// acceptance groups feed every relationship's local side (Checks 2, 3, 5),
	// so when either re-runs every standing relationship re-checks instead of
	// carrying a verdict computed over superseded input. An external-only
	// change keeps the precise mapping: a rule change re-checks
	// design_constraints (design × Check 8), a provider change re-checks
	// cross_unit_cohesion (Check 7 × design intent); the other relationships'
	// local content is unchanged and they carry.
	if selected["design"] || selected["acceptance"] {
		addRelationships(run, standing...)
	}
	if ruleChanged && stringInSlice(standing, "design_constraints") {
		addRelationships(run, "design_constraints")
	}
	if providerChanged && stringInSlice(standing, "cross_unit_cohesion") {
		addRelationships(run, "cross_unit_cohesion")
	}

	rest, err := filterCoverage(full, sortedKeySet(selected))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// Net-cost delta/full decision: a delta is kept whenever its estimated
	// work (plus planning/merge overhead) is lower than the full run's. There
	// is no fixed percentage threshold.
	var notices []string
	rerunObjects := map[string]bool{}
	for key := range selection.RerunObjects {
		rerunObjects[key] = true
	}
	fullCost := validatePlanCost(full, run.ValidateObjects, nil)
	deltaCost := validatePlanCost(rest, run.ValidateObjects, rerunObjects) + validateDeltaOverhead
	if deltaCost >= fullCost {
		return upgrade(fmt.Sprintf("the delta plan's estimated cost (%.2f) is not lower than the full run's (%.2f)", deltaCost, fullCost), standing)
	}
	notices = append(notices, fmt.Sprintf("delta cost %.2f < full cost %.2f", deltaCost, fullCost))
	rerunOwned := map[string]bool{}
	objectCheck := map[string]string{}
	checkHasObject := map[string]bool{}
	for _, object := range run.ValidateObjects {
		check := object.Check
		objectCheck[object.ID] = check
		checkHasObject[check] = true
	}
	rerunChecks := map[string]bool{}
	for key := range selection.RerunObjects {
		if check := objectCheck[key]; check != "" {
			rerunChecks[check] = true
		}
	}
	for _, ck := range rest {
		for _, key := range run.ReportKeys(ck) {
			// A check whose every object carried stays carried even though its
			// group session runs for other objects. A check with no catalog
			// object is judged by the group session directly.
			if checkHasObject[key] && !rerunChecks[key] {
				continue
			}
			rerunOwned[key] = true
		}
	}
	var baselineKeys []string
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
			continue
		}
		baselineKeys = append(baselineKeys, key)
	}
	sort.Strings(baselineKeys)
	var carried []string
	for _, key := range baselineKeys {
		if rerunOwned[key] {
			continue
		}
		// A relationship conclusion this run assigned leaves the carry set: it
		// re-checks in the final synthesis.
		if IsRelationshipKey(key) && stringInSlice(run.Relationships, relationshipName(key)) {
			continue
		}
		carried = append(carried, key)
	}

	rerunCount := len(rerunOwned)
	notices = append(notices, selection.Notices...)
	notices = append(notices, fmt.Sprintf("%d local conclusion(s) re-run; %d carry mechanically (their recorded content is unchanged)", rerunCount, len(carried)))
	if len(run.Relationships) > 0 {
		notices = append(notices, "relationship conclusions re-check: "+strings.Join(run.Relationships, ", "))
	}
	return rest, carried, required, notices, nil
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

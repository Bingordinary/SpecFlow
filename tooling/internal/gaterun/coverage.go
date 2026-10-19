package gaterun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// loadDeferredFindings materializes the unit's pending deferred findings from
// the repository's deferred-findings ledger. A malformed ledger fails the
// plan closed: a corrupted routing state must never silently drop a finding.
func loadDeferredFindings(repoRoot, unitName string) ([]DeferredFinding, error) {
	ledger, err := validationcache.ReadDeferredLedger(repoRoot)
	if err != nil {
		return nil, err
	}
	var out []DeferredFinding
	for _, entry := range ledger.PendingForUnit(unitName) {
		for i, key := range entry.AffectedKeys {
			entry.AffectedKeys[i] = deferredOwnerKey(key, entry.SourceUnit, unitName)
		}
		out = append(out, DeferredFinding{
			SourceUnit:   entry.SourceUnit,
			SourceRun:    entry.SourceRun,
			EvidencePath: entry.EvidencePath,
			Reason:       entry.Reason,
			Finding: Finding{
				ID:           entry.FindingID,
				Severity:     entry.Severity,
				Text:         entry.Text,
				Detail:       entry.Detail,
				SourceKey:    deferredOwnerKey(entry.SourceKey, entry.SourceUnit, unitName),
				AffectedKeys: append([]string(nil), entry.AffectedKeys...),
				OwnedBy:      entry.OwnerUnit,
			},
		})
	}
	return out, nil
}

// Rebind unit-scoped quality keys while retaining the finding's source
// identity and leaving public and relationship keys unchanged
// (see framework/verification_scope.md §Deferred findings).
func deferredOwnerKey(key, source, owner string) string {
	if key == reviewKey(SessionKindArchitecture, source, "") {
		return reviewKey(SessionKindArchitecture, owner, "")
	}
	prefix := SessionKindDesign + ":" + source + ":"
	if strings.HasPrefix(key, prefix) {
		return reviewKey(SessionKindDesign, owner, strings.TrimPrefix(key, prefix))
	}
	return key
}

func loadCarriedResults(repoRoot string, run *Run, carried []string) ([]SessionResult, error) {
	baseline, err := validationcache.ReadGateBaseline(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	state, err := validatedJudgmentState(baseline)
	if err != nil {
		return nil, err
	}
	results := make([]SessionResult, 0, len(carried))
	for _, key := range carried {
		status, ok := state.LogicalStatus[key]
		if !ok {
			return nil, fmt.Errorf("baseline judgment state has no logical status for carried check %q — run the full command", key)
		}
		result := SessionResult{
			SessionID:       "carried:" + key,
			Kind:            "carried",
			EffectiveStatus: map[string]string{key: status},
			ReportDigest:    state.SynthesisDigest,
		}
		for _, finding := range state.Findings {
			if finding.SourceKey == key || stringInSlice(finding.AffectedKeys, key) {
				result.Findings = append(result.Findings, finding)
			}
		}
		if binding, ok := state.Records[key]; ok {
			record, err := judgments.Load(repoRoot, binding.Reference)
			if err != nil {
				return nil, err
			}
			var original SessionResult
			if err := json.Unmarshal(record.Result, &original); err != nil {
				return nil, err
			}
			result.Verdicts = map[string]string{key: record.Verdict}
			result.Observations = original.Observations
		}
		results = append(results, result)
	}
	return results, nil
}

// capturedCarriedLenses snapshots the lens tag of every carried check from
// the baseline cache into the run state. gate-finalize reads the run's
// immutable snapshot instead of a baseline another run may have rewritten
// between plan and finalize (see framework/validation_cache.md §Write Rules).
// A carried key without a recorded check marker fails closed.
func capturedCarriedLenses(repoRoot string, run *Run, carried []string) (map[string]string, error) {
	baseline, err := validationcache.ReadGateBaseline(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	lensByKey := map[string]string{}
	for _, entry := range baseline.Entries {
		for _, c := range entry.Checks {
			if existing, seen := lensByKey[c.Check]; !seen || existing == "" {
				lensByKey[c.Check] = c.Lens
			}
		}
	}
	out := make(map[string]string, len(carried))
	for _, key := range carried {
		lens, ok := lensByKey[key]
		if !ok {
			return nil, fmt.Errorf("carried-over check %q has no check evidence in the baseline cache — plan a new run", key)
		}
		out[key] = lens
	}
	return out, nil
}

// validatedJudgmentState re-reads and validates the baseline cache's
// structured judgment state (the GATE_JUDGMENTS schema-3 block). A cache
// without it cannot supply carried semantic results or a complete findings
// body, so any partial run that must carry something is refused instead of
// guessed from prose (see framework/verification_scope.md §Delta Runs and
// framework/validation_cache.md §Format). Delta and repair share this check
// through the derivation and the carried-result loader, so `fresh@` and
// `gate-plan` never disagree about whether the baseline supports a partial
// run.
func validatedJudgmentState(baseline *validationcache.GateBaseline) (JudgmentBaseline, error) {
	if strings.TrimSpace(baseline.Judgments) == "" {
		return JudgmentBaseline{}, fmt.Errorf("baseline cache has no structured judgment state — run the full command before using delta/repair")
	}
	var state JudgmentBaseline
	if err := json.Unmarshal([]byte(baseline.Judgments), &state); err != nil || state.SchemaVersion != expectedBaselineSchema(baseline) || state.SynthesisDigest == "" {
		return JudgmentBaseline{}, fmt.Errorf("baseline cache has an invalid structured judgment state — run the full command before using delta/repair")
	}
	for _, finding := range state.Findings {
		if strings.TrimSpace(finding.Detail) == "" {
			return JudgmentBaseline{}, fmt.Errorf("baseline finding %q has no renderable detail — run the full command before using delta/repair", finding.ID)
		}
	}
	declared := map[string]bool{}
	for _, name := range state.Relationships {
		key := RelationshipKey(name)
		if _, ok := state.LogicalStatus[key]; !ok || declared[key] {
			return JudgmentBaseline{}, fmt.Errorf("baseline relationship %q has no unique judgment — run the full command", name)
		}
		declared[key] = true
	}
	for key := range state.LogicalStatus {
		if IsRelationshipKey(key) && !declared[key] {
			return JudgmentBaseline{}, fmt.Errorf("baseline relationship judgment %q has no scope declaration — run the full command", key)
		}
	}
	return state, nil
}

// validateCheckGroups is the coverage decomposition of the unit validate
// checklist: every one of the 10 checks belongs to exactly one group. The
// mapping is part of the gate contract
// (framework/verification_scope.md §Coverage model → Coverage keys) — the same
// coverage set is computed for the same target every time. Clarity (Check 10)
// is a normal single-session check: the session reads the unit spec and reports
// what is unclear, underspecified, or internally contradictory.
var validateCheckGroups = []struct {
	GroupID string
	Kind    string
	Checks  []string
}{
	{"structural", SessionKindChecks, []string{"1", "3", "6"}},
	{"design", SessionKindChecks, []string{"2", "4"}},
	{"acceptance", SessionKindChecks, []string{"5"}},
	{"dependencies", SessionKindChecks, []string{"7", "8", "9"}},
	{"clarity", SessionKindChecks, []string{ClarityCheck}},
}

// ruleValidateChecks is the rule validate coverage key set (the 6 rule
// checks).
var ruleValidateChecks = []string{"1", "2", "3", "4", "5", "6"}

// RuleValidateChecks returns the rule validate coverage key set.
func RuleValidateChecks() []string {
	return append([]string(nil), ruleValidateChecks...)
}

// validateGroupForCheck maps a unit validate check key to its coverage group.
func validateGroupForCheck(check string) (string, bool) {
	for _, group := range validateCheckGroups {
		for _, c := range group.Checks {
			if c == check {
				return group.GroupID, true
			}
		}
	}
	return "", false
}

// runClass is the coverage classification of a run: rule validate, unit
// validate, or unit verify. It is the single place the gate/target-kind
// combination is interpreted, so every coverage function selects the same
// branch. The zero value (classUnsupported) is never planned.
type runClass int

const (
	classUnsupported runClass = iota
	classRuleValidate
	classUnitValidate
	classUnitVerify
)

func classifyRun(run *Run) runClass {
	switch {
	case run.TargetKind == TargetKindRule && run.Gate == GateValidate:
		return classRuleValidate
	case run.TargetKind == TargetKindUnit && run.Gate == GateValidate:
		return classUnitValidate
	case run.TargetKind == TargetKindUnit && run.Gate == GateVerify:
		return classUnitVerify
	}
	return classUnsupported
}

// RequiredCoverage is the complete coverage set of a run, derived through a
// fresh derivation. Callers that already hold a derivation should call
// Derivation.computeCoverage so the expansions and audit are shared.
func RequiredCoverage(repoRoot string, run *Run) ([]CoverageKey, error) {
	d, err := NewDerivation(repoRoot)
	if err != nil {
		return nil, err
	}
	return d.computeCoverage(run)
}

// computeCoverage is the complete coverage set for a full run: the judgment
// keys that must each receive exactly one verdict. validate (unit) covers the
// validate check groups; validate (rule) covers the rule check keys; verify
// covers the merged union of the acceptance item ids plus the always-present
// unit-level scan under the alignment lens and the declared code files under
// the quality lens.
// RequiredCoverage includes both executed and carried checks of this run.
func (d *Derivation) computeCoverage(run *Run) ([]CoverageKey, error) {
	switch classifyRun(run) {
	case classRuleValidate:
		var coverage []CoverageKey
		for _, check := range ruleValidateChecks {
			coverage = append(coverage, CoverageKey{Key: check, Kind: SessionKindChecks})
		}
		return coverage, nil
	case classUnitValidate:
		var coverage []CoverageKey
		for _, group := range validateCheckGroups {
			coverage = append(coverage, CoverageKey{Key: group.GroupID, Kind: group.Kind})
		}
		return coverage, nil
	case classUnitVerify:
		return d.verifyCoverage(run)
	}
	return nil, fmt.Errorf("unsupported gate %q for %s target", run.Gate, run.TargetKind)
}

// verifyCoverage derives the merged verify gate's coverage keys: the
// acceptance item ids and the always-present unit-level scan under the
// alignment lens, then the declared code files under the quality lens.
func (d *Derivation) verifyCoverage(run *Run) ([]CoverageKey, error) {
	items, err := verifyItems(d.root, run)
	if err != nil {
		return nil, err
	}
	var coverage []CoverageKey
	for _, item := range items {
		coverage = append(coverage, CoverageKey{Key: reviewKey(SessionKindItem, run.TargetName, item), Kind: SessionKindItem, Lens: LensAlignment, Unit: run.TargetName, Item: item})
	}
	// The unit-level scan (Steps 1 and 5) is one judgment over the whole unit;
	// it replaces the unit-wide work every item session used to repeat.
	coverage = append(coverage, CoverageKey{Key: reviewKey(SessionKindUnit, run.TargetName, ""), Kind: SessionKindUnit, Lens: LensAlignment, Unit: run.TargetName})
	for _, file := range qualityFiles(run) {
		reads := codeReadRefs(run, file)
		coverage = append(coverage, CoverageKey{Key: reviewKey(SessionKindCode, "", file), Kind: SessionKindCode, Lens: LensQuality, File: file, ReadRefs: reads}, CoverageKey{Key: reviewKey(SessionKindDesign, run.TargetName, file), Kind: SessionKindDesign, Lens: LensQuality, File: file, Unit: run.TargetName})
	}
	coverage = append(coverage, CoverageKey{Key: reviewKey(SessionKindArchitecture, run.TargetName, ""), Kind: SessionKindArchitecture, Lens: LensQuality, Unit: run.TargetName})
	return coverage, nil
}

// coverageReportKeys lists the reviewer report check keys one coverage key
// owns: the group's check numbers (validate unit), the check number (rule),
// or the key itself (verify item / quality file).
func coverageReportKeys(run *Run, ck CoverageKey) []string {
	if ck.Kind == SessionKindDeltaReview {
		return []string{DeltaReviewKey}
	}
	switch classifyRun(run) {
	case classUnitValidate:
		return append([]string(nil), groupChecks(ck.Key)...)
	default:
		return []string{ck.Key}
	}
}

// ReportKeys returns the reviewer report check keys a coverage key owns.
func (r *Run) ReportKeys(ck CoverageKey) []string {
	return coverageReportKeys(r, ck)
}

// LensForReportKey returns the lens tag of the coverage key that owns a
// reviewer report check key, or "" when the run's gate has no lens dimension
// (validate). It lets gate-finalize stamp each assembled cache check with its
// lens so the merged verify cache carries an alignment section and a quality
// section.
func (r *Run) LensForReportKey(key string) string {
	for _, ck := range r.Coverage {
		for _, owned := range coverageReportKeys(r, ck) {
			if owned == key {
				return ck.Lens
			}
		}
	}
	return ""
}

// ExpectedVerifyCoverage computes the merged verify coverage set — the
// alignment keys (the acceptance-item keys plus the always-present unit-level
// scan key) and the quality keys (declared code files, their unit design keys
// and the unit architecture key) — for the current target without persisting a
// run. promote uses it to require the published verify cache to cover both
// lenses.
func ExpectedVerifyCoverage(repoRoot, unitName, target string) ([]CoverageKey, error) {
	d, err := NewDerivation(repoRoot)
	if err != nil {
		return nil, err
	}
	run, err := d.resolveRun(GateVerify, TargetKindUnit, unitName, target, ModeFull, nil, nil)
	if err != nil {
		return nil, err
	}
	return d.computeCoverage(run)
}

// buildCoveragePlan computes the coverage set for the run mode, the carried
// check keys (delta/repair only), the target's required files, and plan
// notices (scope derivation and conservative degradations).
func (d *Derivation) buildCoveragePlan(run *Run) ([]CoverageKey, []string, []string, []string, error) {
	required := requiredFiles(run)
	full, err := d.computeCoverage(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if run.Mode == ModeFull {
		carried, notices, err := planFullRunCarry(d.root, run)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if run.ChangeSetFP != "" {
			// Standing relationship conclusions await the validate change
			// review; the full coverage set runs beside it. Verify has no
			// change review: its standing relationships re-run directly (see
			// planFullRunCarry).
			full = append([]CoverageKey{{Key: DeltaReviewKey, Kind: SessionKindDeltaReview}}, full...)
			notices = append(notices, "change review: the standing relationship conclusions await accept-or-recheck against the recorded change set")
		}
		if err := validateCoverage(run, full); err != nil {
			return nil, nil, nil, nil, err
		}
		return full, carried, required, notices, nil
	}
	if run.Mode == ModeDelta || run.Mode == ModeRepair {
		var (
			coverage []CoverageKey
			carried  []string
			notices  []string
		)
		if run.Gate == GateVerify {
			coverage, carried, required, notices, err = d.buildMechanicalVerifyPlan(run)
		} else {
			coverage, carried, required, notices, err = d.buildReviewPlan(run)
		}
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := validateCoverage(run, coverage); err != nil {
			return nil, nil, nil, nil, err
		}
		return coverage, carried, required, notices, nil
	}
	return nil, nil, nil, nil, fmt.Errorf("unsupported run mode %q", run.Mode)
}

// filterCoverage keeps the coverage keys whose key is in want, preserving the
// full coverage order.
func filterCoverage(full []CoverageKey, want []string) ([]CoverageKey, error) {
	wanted := map[string]bool{}
	for _, key := range want {
		wanted[key] = true
	}
	var out []CoverageKey
	for _, ck := range full {
		if wanted[ck.Key] {
			out = append(out, ck)
		}
	}
	if len(out) != len(wanted) {
		// A planned re-run key that is absent from the current coverage
		// surface cannot be covered — fail closed.
		seen := map[string]bool{}
		for _, ck := range out {
			seen[ck.Key] = true
		}
		var missing []string
		for key := range wanted {
			if !seen[key] {
				missing = append(missing, key)
			}
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("re-run coverage key(s) not in the current surface: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// validateCoverage rejects ambiguous or unusable coverage sets before any run
// state is persisted. CoverageByKey is a simple lookup, so its precondition —
// one unique entry for every key — is established here and rechecked whenever
// a run is loaded.
func validateCoverage(run *Run, coverage []CoverageKey) error {
	if len(coverage) == 0 && len(run.Relationships) == 0 {
		return fmt.Errorf("coverage set is empty")
	}
	seen := make(map[string]bool, len(coverage))
	for _, ck := range coverage {
		key := strings.TrimSpace(ck.Key)
		if key == "" {
			return fmt.Errorf("coverage set contains an empty key")
		}
		if key == CrossKey {
			return fmt.Errorf("coverage set declares the reserved final-synthesis key %q", CrossKey)
		}
		if IsRelationshipKey(key) {
			return fmt.Errorf("local coverage key %q uses the reserved relationship namespace", key)
		}
		if seen[key] {
			return fmt.Errorf("coverage set contains duplicate key %q", key)
		}
		seen[key] = true
		switch ck.Kind {
		case SessionKindChecks, SessionKindItem, SessionKindUnit, SessionKindDesign, SessionKindCode, SessionKindArchitecture:
		case SessionKindDeltaReview:
			if key != DeltaReviewKey {
				return fmt.Errorf("delta review coverage key %q must be %q", key, DeltaReviewKey)
			}
		default:
			return fmt.Errorf("coverage key %q has invalid kind %q", key, ck.Kind)
		}
		switch ck.Lens {
		case "":
			if ck.Kind != SessionKindDeltaReview && run.Gate == GateVerify && run.TargetKind == TargetKindUnit {
				return fmt.Errorf("coverage key %q has no lens — a verify key must be tagged alignment or quality", key)
			}
		case LensAlignment:
			if !IsAlignmentKind(ck.Kind) {
				return fmt.Errorf("alignment coverage key %q has kind %q, expected %q or %q", key, ck.Kind, SessionKindItem, SessionKindUnit)
			}
		case LensQuality:
			if ck.Kind != SessionKindDesign && ck.Kind != SessionKindCode && ck.Kind != SessionKindArchitecture {
				return fmt.Errorf("quality coverage key %q has kind %q, expected %q", key, ck.Kind, SessionKindDesign)
			}
		default:
			return fmt.Errorf("coverage key %q has invalid lens %q", key, ck.Lens)
		}
		if ck.Lens != "" && run.Gate != GateVerify {
			return fmt.Errorf("coverage key %q declares a lens outside the verify gate", key)
		}
		if len(coverageReportKeys(run, ck)) == 0 {
			return fmt.Errorf("coverage key %q owns no report keys", key)
		}
	}
	return nil
}

// validatePlannedVerify enforces the resumption shape of a persisted verify
// run: every public coverage key must be backed by its execution machinery —
// a shared task (source "executed"/"waiting") or a bound public record (source
// "reused"). A public key with neither was published before its shared task
// existed — an interrupted plan no submission can complete (issue #80). The
// strict loader rejects it as unresumable, so the orphan sweep reclaims it
// instead of listing it as a run that can resume. The check runs only on load;
// plan-time validation precedes the stamping.
func validatePlannedVerify(run *Run) error {
	if run.Gate != GateVerify {
		return nil
	}
	for _, ck := range run.Coverage {
		if ck.Kind != SessionKindCode || ck.Task != "" {
			continue
		}
		if _, bound := run.Records[ck.Key]; bound {
			continue
		}
		return fmt.Errorf("public coverage key %q has no shared task — the run was not fully planned; replan the run", ck.Key)
	}
	return nil
}

// targetLayerSpecRef resolves a target's own main file — a unit main spec or a
// rule file — in the given layer (candidate or stable). Appendices are
// separate files; callers that need the whole own-spec surface add them.
func targetLayerSpecRef(targetKind, targetName, target string) string {
	if targetKind == TargetKindRule {
		if target == TargetCandidate {
			return specpaths.RuleCandidateFileRef(targetName)
		}
		return specpaths.RuleStableFileRef(targetName)
	}
	if target == TargetCandidate {
		return specpaths.CandidateUnitSpecFileRef(targetName)
	}
	return specpaths.StableUnitSpecFileRef(targetName)
}

// mainSpecRef resolves the target's own main file — the unit main spec or the
// rule file — in the run's target layer (candidate or stable). Appendices are
// separate files; callers that need the whole own-spec surface add them.
func mainSpecRef(run *Run) string {
	return targetLayerSpecRef(run.TargetKind, run.TargetName, run.Target)
}

// requiredFiles lists the target's own main file — the file the assembled
// cache evidence must cover (unit main spec; rule file). Appendices are
// covered by the appendix gate, not by this list.
func requiredFiles(run *Run) []string {
	return []string{mainSpecRef(run)}
}

// SessionID derives the stable id of the reviewer session assigned a key
// batch. A single-key session keeps the key as its id; a batch gets a
// deterministic `batch-<hash>` id so finding ids and session state files are
// stable for the same assigned key set.
func SessionID(keys []string) string {
	sorted := dedupeSorted(keys)
	if len(sorted) == 1 {
		return sorted[0]
	}
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return "batch-" + hex.EncodeToString(sum[:])[:12]
}

// qualityPairClass is the session class of a verify quality file's co-batched
// `code:<file>` and `design:<unit>:<file>` keys. It is a partition sentinel,
// not a coverage kind.
const qualityPairClass = "quality-file"

// sessionBatchKey is the equivalence a reviewer session batch is built on: one
// class and one lens. For every key but the verify quality public/design pair
// the class is the report kind, so a batch holds the run's keys of one kind and
// one lens. For a verify quality `code:<file>` key and its unit's
// `design:<unit>:<file>` key the class is the reviewed file, so the pair
// co-batches — the framework's one-pass public+design composition. This keeps
// every batch ready to materialize: when both keys of a file are uncovered they
// share the co-batch (the design judgment disposes the facts the same session
// collects and needs no separate public record), and when only the design key
// is uncovered its public record is already accepted or carried.
type sessionBatchKey struct {
	class string
	lens  string
	file  string
}

// sessionBatchKeyOf maps a coverage key to its reviewer session batch key.
func sessionBatchKeyOf(ck *CoverageKey) sessionBatchKey {
	if ck.Lens == LensQuality && (ck.Kind == SessionKindCode || ck.Kind == SessionKindDesign) {
		return sessionBatchKey{class: qualityPairClass, lens: ck.Lens, file: ck.File}
	}
	return sessionBatchKey{class: ck.Kind, lens: ck.Lens}
}

// PartitionBatches partitions coverage keys into the run's reviewer units: one
// unit per session class and lens, in the run's coverage order. A unit is the
// smallest batch the tool guarantees to materialize: every returned unit is
// accepted by BuildSessionSpec — the class applies both the "one lens" rule
// submission enforces and the verify quality co-batch the design judgment needs
// — so a hint built from these units never prints a command the tool rejects
// for mixing lenses or for an unsatisfied dependency. A unit is a unit of
// joining, not a mandate: the coordinator joins the units of one lens and kind
// into a session (a verify quality session may hold the code and design keys of
// several files), because joining never widens the read surface, while only
// splitting a session's key set — which needs an over-capacity declaration —
// costs a repeated read. Keys not in the run's coverage set are ignored.
func PartitionBatches(run *Run, keys []string) [][]string {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	var batches [][]string
	index := map[sessionBatchKey]int{}
	for i := range run.Coverage {
		ck := &run.Coverage[i]
		if !wanted[ck.Key] {
			continue
		}
		class := sessionBatchKeyOf(ck)
		j, ok := index[class]
		if !ok {
			batches = append(batches, nil)
			j = len(batches) - 1
			index[class] = j
		}
		batches[j] = append(batches[j], ck.Key)
	}
	return batches
}

// BuildSessionSpec materializes the read-only reviewer session spec for a
// joined key set. Every key must belong to the run's coverage set (or
// be the reserved final-synthesis key `cross`) and all keys must share one
// lens, with one kind exception: a verify quality session may co-batch the
// code and design keys of its reviewed files. Any other kind mix stays
// rejected.
func BuildSessionSpec(repoRoot string, run *Run, keys []string) (*SessionSpec, error) {
	keys = dedupeSorted(keys)
	if len(keys) == 0 {
		return nil, fmt.Errorf("at least one coverage key is required")
	}
	if len(keys) == 1 && keys[0] == CrossKey {
		if run.TargetKind == TargetKindRule {
			return nil, fmt.Errorf("rule validate has no final synthesis session")
		}
		return &SessionSpec{
			SessionID:     CrossKey,
			Keys:          []string{CrossKey},
			Kind:          SessionKindCross,
			CheckKeys:     []string{CrossKey},
			ReadRefs:      crossReadRefs(repoRoot, run),
			Relationships: append([]string(nil), run.Relationships...),
			Context:       []string{"Check only the assigned relationships against current source and accepted/carried judgments. Do not repeat local checks. The tooling derives every effective status from your dispositions and findings. If no relationships are assigned, only dispose existing findings."},
		}, nil
	}
	if len(keys) == 1 && keys[0] == DeltaReviewKey {
		if run.CoverageByKey(DeltaReviewKey) == nil {
			return nil, fmt.Errorf("key %q is not part of run %s's coverage set", DeltaReviewKey, run.RunID)
		}
		return &SessionSpec{
			SessionID: DeltaReviewKey,
			Keys:      []string{DeltaReviewKey},
			Kind:      SessionKindDeltaReview,
			CheckKeys: []string{DeltaReviewKey},
			ReadRefs:  reviewReadRefs(repoRoot, run),
			Context: []string{
				"Judge only whether the standing conclusions are affected by the change set. Produce no check verdicts.",
				"Focus on the changed content, but read any input-surface file you need for context — a change can affect a conclusion anchored elsewhere.",
				"When in doubt, name the re-run keys or escalate; accepting is the consequential call.",
			},
		}, nil
	}
	wanted := map[string]bool{}
	kind := ""
	lens := ""
	mixedPair := false
	for _, key := range keys {
		ck := run.CoverageByKey(key)
		if ck == nil {
			return nil, fmt.Errorf("key %q is not part of run %s's coverage set", key, run.RunID)
		}
		if kind == "" {
			kind = ck.Kind
		} else if ck.Kind != kind {
			// A verify quality session may co-batch the code key of a file
			// with its unit's design key: one reviewer collects the public
			// facts and judges the unit design in one pass. Any other kind
			// mix stays rejected — a session covers one kind only.
			if !(run.Gate == GateVerify && IsQualityKind(kind) && IsQualityKind(ck.Kind)) {
				return nil, fmt.Errorf("session mixes report kinds %q and %q — a session covers one kind only (a code+design quality pairing is the exception)", kind, ck.Kind)
			}
			mixedPair = true
		}
		if ck.Lens != "" {
			if lens == "" {
				lens = ck.Lens
			} else if ck.Lens != lens {
				return nil, fmt.Errorf("session mixes lenses %q and %q — a session covers one lens only", lens, ck.Lens)
			}
		}
		wanted[key] = true
	}
	if mixedPair {
		for _, key := range keys {
			if ck := run.CoverageByKey(key); ck.Kind != SessionKindCode && ck.Kind != SessionKindDesign {
				return nil, fmt.Errorf("co-batched session mixes %q with code/design keys — only code+design keys may pair", ck.Kind)
			}
		}
		kind = SessionKindDesign
	}
	spec := &SessionSpec{SessionID: SessionID(keys), Keys: keys, Kind: kind}
	for _, ck := range run.Coverage {
		if !wanted[ck.Key] {
			continue
		}
		spec.CheckKeys = appendUnique(spec.CheckKeys, coverageReportKeys(run, ck)...)
		spec.ReadRefs = appendUnique(spec.ReadRefs, coverageReadRefs(repoRoot, run, ck)...)
	}
	if kind == SessionKindDesign {
		states, err := LoadSessionStates(repoRoot, run)
		if err != nil {
			return nil, err
		}
		pairedCode := map[string]bool{}
		if mixedPair {
			for _, key := range keys {
				if run.CoverageByKey(key).Kind == SessionKindCode {
					pairedCode[key] = true
				}
			}
		}
		for _, key := range keys {
			ck := run.CoverageByKey(key)
			if ck.Kind != SessionKindDesign {
				continue
			}
			codeKey := DesignPublicKey(*ck)
			if pairedCode[codeKey] {
				// Co-batched: this session collects the public facts itself;
				// it depends on no other session for the file.
				continue
			}
			dep := codeKey
			for _, state := range states {
				if state.Status == SessionAccepted && stringInSlice(state.Keys, codeKey) {
					dep = state.SessionID
				}
			}
			if stringInSlice(run.CarriedKeys, codeKey) {
				spec.Context = append(spec.Context, "Consume the accepted public record for "+codeKey)
			} else {
				spec.DependsOn = appendUnique(spec.DependsOn, dep)
			}
		}
		if mixedPair {
			spec.Context = append(spec.Context, "Co-batched public+design session. For each code:<file> key, read the complete file and collect facts and potential problems without unit rationale — they publish as the immutable public record. For each design:<unit>:<file> key, dispose every observation you collected for that file with a reason — retain it as a finding or exclude it with evidence from this unit's design — and actively check the unit spec, including violations absent from the facts. The facts stay rationale-free; the unit rationale never alters them.")
		} else {
			spec.Context = append(spec.Context, "Consume only the immutable public records for the assigned files, regardless of their execution batch. Actively check the unit spec. Dispose every observation in those records with a reason; retain it as a finding or exclude it with evidence from this unit's design. Never alter the public record.")
		}
	}
	if kind == SessionKindCode {
		spec.Context = append(spec.Context, "Collect code facts and potential problems only. Do not read unit-private designs or suppress problems by a unit's rationale. Read the whole public evidence surface; if a required file is missing from read_refs, report it instead of judging from incomplete context.")
	}
	if kind == SessionKindArchitecture {
		spec.Context = append(spec.Context, "Assess all six Dimension 8 architecture fields once for the whole unit.")
	}
	if IsItemKind(kind) {
		spec.Context = append(spec.Context, "Steps 1 (structural alignment) and 5 (implementation integrity and code design-surface surplus) are executed by the run's unit-level scan session; do not re-derive them here. This session owns Steps 2, 3, 4, 6, 7 for its items: Step 2 acceptance alignment, Step 3 scope accuracy — for each assigned item verify that its declared implementation_surface / affects.files carry the implementation its pass_condition describes and are relevant to it (shared implementation is permitted, and a file that only supplies an input to behavior implemented elsewhere is not under-declaration; a declaration defect is a MISMATCH (scope) finding) — Step 4 retirement verification for replacement candidates, Step 6 stub adjudication, and Step 7 first-principles divergence analysis for every MISMATCH this session retains. If evidence is missing from read_refs, report the missing input rather than judging from incomplete context.")
		for _, notice := range run.Notices {
			if strings.HasPrefix(notice, "stub scan:") {
				spec.Context = append(spec.Context, notice)
			}
		}
	}
	if kind == SessionKindUnit {
		spec.Context = append(spec.Context, "Unit-level alignment scan: execute Step 1 (structural alignment) and Step 5 (implementation integrity and code design-surface surplus) over the whole unit — Step 5 covers both the code design-surface surplus analysis (Part A) and the non-runnable assessment (count the acceptance items marked `runnable: no`, check each carries a substantive `not_runnable_reason` with external evidence, and judge whether the remaining runnable set gives meaningful coverage) — Step 2 acceptance alignment, Step 3 scope accuracy and Step 6 are owned by the item sessions, while this session completes Step 7 first-principles divergence analysis for every structural or surplus MISMATCH it retains. Attribute code structures and designs to the responsibility of this unit before judging surplus: shared-file association is not exclusive ownership, independent behavior outside that responsibility is not surplus, and helpers or shared mechanisms implementing or constraining this unit's requirements remain in scope. If attribution evidence is missing from read_refs, report the missing input rather than infer a mismatch from file co-location. Report one `declarations.<class>:` line per declaration class, one `surplus:` line for the code design-surface scan, and one `non_runnable:` line for the non-runnable assessment, as the report contract requires, so neither an unexamined declaration class nor the Step 5 Part 2 result is silent.")
	}
	return spec, nil
}

// coverageReadRefs derives the read surface of one coverage key. A validate
// group's surface is the existing group read refs; a verify item reads the own
// spec plus the whole declared code surface; a file reads the own spec plus
// that file; all add the agent-declared extra inputs. The surface is the
// spec-declared evidence and the explicitly supplied inputs only — no
// repository-wide name-token closure participates (framework/shared_judgments.md
// §Required tasks).
func coverageReadRefs(repoRoot string, run *Run, ck CoverageKey) []string {
	switch ck.Kind {
	case SessionKindChecks:
		if classifyRun(run) == classRuleValidate {
			return appendUnique(allRefNames(run), extraInputPaths(repoRoot, run)...)
		}
		return unitValidateGroupReadRefs(repoRoot, run, ck.Key)
	case SessionKindItem, SessionKindUnit:
		// An acceptance-item session and the unit-level scan read the own spec
		// plus the whole declared code surface; the item session judges its
		// items (Steps 2, 3, 4, 6, 7) and the unit session scans the unit
		// (Steps 1 and 5).
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, surfacePaths(run)...)
		read = appendUnique(read, affectsEvidencePaths(run)...)
		read = appendUnique(read, ruleRefNames(run)...)
		return appendUnique(read, extraInputPaths(repoRoot, run)...)
	case SessionKindCode:
		return appendUnique(append([]string(nil), ck.ReadRefs...), extraInputPaths(repoRoot, run)...)
	case SessionKindArchitecture:
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, surfacePaths(run)...)
		read = appendUnique(read, ruleRefNames(run)...)
		return appendUnique(read, extraInputPaths(repoRoot, run)...)
	case SessionKindDesign:
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, ck.File)
		read = appendUnique(read, ruleRefNames(run)...)
		return appendUnique(read, extraInputPaths(repoRoot, run)...)
	}
	return nil
}

// crossReadRefs lists the final synthesis's read surface: every snapshot ref
// and every code surface file.
func crossReadRefs(repoRoot string, run *Run) []string {
	read := append([]string(nil), allRefNames(run)...)
	read = append(read, surfacePaths(run)...)
	return appendUnique(read, extraInputPaths(repoRoot, run)...)
}

// InputSurface lists every file a run may read: the union of every coverage
// key's read surface plus the cross-synthesis surface. Cache evidence is
// assembled from exactly this surface — the entry set IS the input surface
// (framework/validation_cache.md §Format), so a change anywhere in it is
// visible to the freshness comparison and the change diff. Because the read
// surface is the declared code surface plus explicitly supplied inputs, an
// unrelated repository file can never make a run stale (issue #73).
func InputSurface(repoRoot string, run *Run) []string {
	var out []string
	for _, ck := range run.Coverage {
		if ck.Kind == SessionKindDeltaReview {
			continue
		}
		out = appendUnique(out, coverageReadRefs(repoRoot, run, ck)...)
	}
	out = appendUnique(out, crossReadRefs(repoRoot, run)...)
	return out
}

// TargetMainFile returns the run target's own main artifact reference in the
// run's layer — the unit main spec or the rule file.
func TargetMainFile(run *Run) string {
	return mainSpecRef(run)
}

// noUsableBaselineError is the documented message for a stable-only target
// with no usable confirmation baseline (see framework/verification_scope.md
// §Delta Runs → Layer applicability): the full confirmation run or a fork.
func noUsableBaselineError(run *Run) error {
	return fmt.Errorf("No usable confirmation baseline. Run the full `%s@%s` (confirmation check) first, or `specflowctl fork %s` to start a new round.", run.Gate, run.TargetName, gateTargetFlags(run))
}

// coverageKeysForRerun maps the effective re-run check set onto the run's
// coverage keys: unit validate selects the groups whose checks intersect the
// effective set; every other gate's check keys are its coverage keys.
func coverageKeysForRerun(run *Run, effective map[string]bool) []string {
	local := map[string]bool{}
	for key := range effective {
		if !IsRelationshipKey(key) {
			local[key] = true
		}
	}
	switch classifyRun(run) {
	case classRuleValidate:
		return sortedKeySet(local)
	case classUnitValidate:
		selected := map[string]bool{}
		for _, group := range validateCheckGroups {
			for _, c := range group.Checks {
				if effective[c] {
					selected[group.GroupID] = true
					break
				}
			}
		}
		return sortedKeySet(selected)
	default:
		return sortedKeySet(local)
	}
}

// currentGateKeys lists the current non-cross judgment keys for the run's
// gate and target: the fixed check numbers (validate and rule targets),
// the acceptance item ids plus the always-present unit-level scan, or quality
// code files (verify).
func (d *Derivation) currentGateKeys(run *Run) ([]string, error) {
	switch classifyRun(run) {
	case classRuleValidate:
		return append([]string(nil), ruleValidateChecks...), nil
	case classUnitValidate:
		seen := map[string]bool{}
		var keys []string
		for _, group := range validateCheckGroups {
			for _, check := range group.Checks {
				if seen[check] {
					continue
				}
				seen[check] = true
				keys = append(keys, check)
			}
		}
		return keys, nil
	case classUnitVerify:
		coverage, err := d.verifyCoverage(run)
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(coverage))
		for _, ck := range coverage {
			keys = append(keys, ck.Key)
		}
		return keys, nil
	}
	return nil, fmt.Errorf("unsupported gate %q for %s target", run.Gate, run.TargetKind)
}

// groupChecks returns one validate coverage group's check keys.
func groupChecks(groupID string) []string {
	for _, group := range validateCheckGroups {
		if group.GroupID == groupID {
			return group.Checks
		}
	}
	return nil
}

func sortedKeySet(keys map[string]bool) []string {
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// unitValidateGroupReadRefs derives the exact group-local read surface for one
// unit validate coverage group. Check 1 (structural) must resolve unit_refs
// and rule_refs to verify that they exist, while Checks 7-8 (dependencies)
// read those logical objects for cross-unit and constraint judgments. The
// logical refs here are the declared unit_refs and rule_refs only — a unit
// validate never carries unrelated peers (Check 9 is the mechanical
// `specflowctl surfaces` audit and needs no peer spec read ref).
// Validate is a spec-quality gate: every group reads only the unit's own spec
// truth, the agent-declared shared inputs, and (structural/dependencies) the
// declared logical refs. No group receives code evidence — affects.files and
// affects.evidence_files are verify inputs, never validate read refs.
func unitValidateGroupReadRefs(repoRoot string, run *Run, groupID string) []string {
	read := ownSpecPaths(repoRoot, run)
	read = appendUnique(read, extraInputPaths(repoRoot, run)...)
	if groupID == "structural" || groupID == "dependencies" {
		read = appendUnique(read, logicalRefNames(run)...)
	}
	return append([]string(nil), read...)
}

func isRuleValidateCheck(check string) bool {
	for _, c := range ruleValidateChecks {
		if c == check {
			return true
		}
	}
	return false
}

// affectsEvidencePaths lists the spec-derived affects.evidence_files evidence
// files (SourceDerivedAffects). They are part of a verify item session's read
// surface; they never enter a validate read surface and never create coverage
// keys.
func affectsEvidencePaths(run *Run) []string {
	var out []string
	for _, ref := range run.Refs {
		if ref.Source == SourceDerivedAffects && !isLogicalRef(ref.Ref) {
			out = append(out, ref.Ref)
		}
	}
	return out
}

// ownSpecPaths lists the target's own spec files (unit main spec + appendices
// in the target layer; the rule file).
func ownSpecPaths(repoRoot string, run *Run) []string {
	return append([]string(nil), run.OwnSpecFiles...)
}

// verifyItems lists the current spec's acceptance item ids in document order.
func verifyItems(repoRoot string, run *Run) ([]string, error) {
	content, err := readSpecContent(repoRoot, mainSpecRef(run))
	if err != nil {
		return nil, err
	}
	return specvalidation.ExtractAcceptanceItemIDs(content), nil
}

// surfaceFiles lists a run's code surface files (deduplicated, sorted). With
// derivedOnly set it keeps only spec-derived surface entries — the quality
// lens's declared code files; otherwise it keeps every surface.
func surfaceFiles(run *Run, derivedOnly bool) []string {
	seen := map[string]bool{}
	var files []string
	for _, surface := range run.Surfaces {
		if derivedOnly && surface.Source != SourceDerived {
			continue
		}
		for _, entry := range surface.Entries {
			if seen[entry.Path] {
				continue
			}
			seen[entry.Path] = true
			files = append(files, entry.Path)
		}
	}
	sort.Strings(files)
	return files
}

// qualityFiles lists the merged verify run's quality-lens code files
// (expanded surface entries, deduplicated, sorted).
func qualityFiles(run *Run) []string {
	return surfaceFiles(run, true)
}

// surfacePaths lists every code file in the run's surfaces.
func surfacePaths(run *Run) []string {
	return surfaceFiles(run, false)
}

// allRefNames lists every snapshot ref spelling.
func allRefNames(run *Run) []string {
	var out []string
	for _, ref := range run.Refs {
		out = append(out, ref.Ref)
	}
	return out
}

// SynthesisEvidencePins pins the supplied evidence the accepted final
// synthesis actually cited. The report is the citation record: a physical
// evidence input or a logical reference it names (rule:{id}, unit:{name},
// unit:{name}:appendix:{file}) is part of the synthesis's own object, so a
// change to its resolved content re-runs the synthesis instead of carrying the
// relationship conclusions silently. The logical spelling is pinned as
// spelled; readers resolve it with judgments.ResolveEvidencePath, so a layer
// move with unchanged content keeps the pin valid. The caller must have
// verified the run snapshot.
func SynthesisEvidencePins(repoRoot string, run *Run, report string) []EvidencePin {
	seen := map[string]bool{}
	var pins []EvidencePin
	for _, p := range extraInputPaths(repoRoot, run) {
		if seen[p] {
			continue
		}
		if !strings.Contains(report, p) {
			continue
		}
		seen[p] = true
		h, ok := run.SnapshotHash(repoRoot, p)
		if !ok || h == "" {
			continue
		}
		pins = append(pins, EvidencePin{Path: p, Hash: h})
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].Path < pins[j].Path })
	return pins
}

// extraInputPaths lists every agent-declared evidence input. These paths are
// readable by sessions but never define coverage keys. A spec object inside an
// expanded directory input is listed as its logical reference — the same
// carried form the ingestion boundary gives a file-form spelling — so a
// directory spelling of a unit spec or rule set behaves exactly like listing
// the objects one by one.
func extraInputPaths(root string, run *Run) []string {
	var out []string
	for _, input := range run.ExtraInputs {
		expanded := false
		for _, surface := range run.Surfaces {
			if surface.Path != input {
				continue
			}
			for _, entry := range surface.Entries {
				out = append(out, extraInputEntry(root, entry.Path))
			}
			expanded = true
			break
		}
		if !expanded {
			out = append(out, input)
		}
	}
	for _, ref := range run.Refs {
		if ref.Source == SourceInput {
			out = append(out, ref.Ref)
		}
	}
	for _, surface := range run.Surfaces {
		if surface.Source != SourceInput {
			continue
		}
		for _, entry := range surface.Entries {
			out = append(out, extraInputEntry(root, entry.Path))
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
}

// extraInputEntry maps one expanded evidence input to its carried form: a
// spec object's physical path becomes its logical reference — a rule file to
// rule:<id>, a unit main spec or appendix to unit:<name> /
// unit:<name>:appendix:<file> — so a layer move does not dangle the evidence
// the run recorded for it.
func extraInputEntry(root, p string) string {
	if ref, ok := LogicalRuleRefForPath(p); ok {
		return ref
	}
	if ref, ok := specpaths.LogicalUnitRefForPath(root, p); ok {
		return ref
	}
	return p
}

func appendUnique(base []string, values ...string) []string {
	seen := map[string]bool{}
	for _, value := range base {
		seen[value] = true
	}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		base = append(base, value)
	}
	return base
}

// logicalRefNames lists the logical (unit:/rule:) snapshot refs.
func logicalRefNames(run *Run) []string {
	var out []string
	for _, ref := range run.Refs {
		if isLogicalRef(ref.Ref) {
			out = append(out, ref.Ref)
		}
	}
	return out
}

// ruleRefNames lists the run's logical rule references. Rules are governance
// constraints named as part of a judgment's basis, whether declared in the
// spec (rule_refs, global rules) or listed explicitly in the input manifest.
func ruleRefNames(run *Run) []string {
	var out []string
	for _, ref := range run.Refs {
		if strings.HasPrefix(ref.Ref, "rule:") {
			out = append(out, ref.Ref)
		}
	}
	return out
}

// codeReadRefs is a public code key's read surface base: the file itself plus
// the run's applicable rule references. It replaces the removed repository-wide
// name-token evidence closure: a code file's evidence is the file and the
// rules that constrain it; related tests, callers and callees are supplied
// explicitly through the input manifest or gate-extend.
func codeReadRefs(run *Run, file string) []string {
	return append([]string{file}, ruleRefNames(run)...)
}

// gateTargetFlags renders the unit/rule flag combination of a run for
// guidance text.
func gateTargetFlags(run *Run) string {
	if run.TargetKind == TargetKindRule {
		return "--rule " + run.TargetName
	}
	return "--unit " + run.TargetName
}

// PlanCommand renders a copy-ready `specflowctl gate-plan` command that replaces
// this run for the given mode. A unit plan requires `--relationships` — a full
// unit plan requires it explicitly — and a rule plan must not carry it, so the
// target kind decides the flag. Every tooling message that sends a coordinator
// to a new plan renders through this, so a printed command is always runnable
// instead of being rejected by gate-plan's own flag contract.
func PlanCommand(run *Run, mode string) string {
	relationships := ""
	if run.TargetKind != TargetKindRule {
		relationships = " --relationships <names|none>"
	}
	return fmt.Sprintf("`specflowctl gate-plan --gate %s %s --target %s%s --mode %s`", run.Gate, gateTargetFlags(run), run.Target, relationships, mode)
}

// CoverageProgress reports which coverage keys are covered by an accepted
// session and which remain open. A key covered by two accepted sessions is a
// mechanical error. The reserved final-synthesis key is excluded.
func CoverageProgress(run *Run, states []*SessionState) (covered map[string]string, uncovered []string, err error) {
	covered = map[string]string{}
	for _, state := range states {
		if state.Status != SessionAccepted {
			continue
		}
		for _, key := range state.Keys {
			if run.CoverageByKey(key) == nil {
				continue
			}
			if prior, ok := covered[key]; ok && prior != state.SessionID {
				return nil, nil, fmt.Errorf("coverage key %q is covered by both session %s and session %s", key, prior, state.SessionID)
			}
			covered[key] = state.SessionID
		}
	}
	for _, ck := range run.Coverage {
		if _, ok := covered[ck.Key]; !ok {
			uncovered = append(uncovered, ck.Key)
		}
	}
	return covered, uncovered, nil
}

func expectedBaselineSchema(b *validationcache.GateBaseline) int {
	if b.Command == GateVerify {
		return 4
	}
	return 3
}

// stubScanPatterns are the deterministic stub/placeholder patterns of verify
// Step 6. A hit is a candidate signal, not a verdict: the reviewer classifies
// each hit RELEVANT (a real stub) or IRRELEVANT (an idiomatic construct such
// as Go's `return nil`).
var stubScanPatterns = []*regexp.Regexp{
	regexp.MustCompile(`return null`),
	regexp.MustCompile(`return \[\]`),
	regexp.MustCompile(`\bplaceholder\b`),
	regexp.MustCompile(`not.*implement`),
	regexp.MustCompile(`return Response\.json\(\{\}\)`),
	regexp.MustCompile(`w\.WriteHeader\(204\)`),
}

// stubScanNotice scans the run's spec-derived code surface with the Step 6
// patterns and renders one plan notice listing the candidate hits, so the
// reviewer classifies a tool-produced list instead of running the greps.
// It returns "" when the surface is clean or unreadable.
func stubScanNotice(repoRoot string, run *Run) string {
	type stubHit struct {
		path string
		line int
		text string
	}
	var hits []stubHit
	for _, file := range qualityFiles(run) {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(file)))
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, pat := range stubScanPatterns {
				if pat.MatchString(line) {
					hits = append(hits, stubHit{path: file, line: i + 1, text: strings.TrimSpace(line)})
					break
				}
			}
		}
	}
	if len(hits) == 0 {
		return ""
	}
	const limit = 20
	rendered := make([]string, 0, limit)
	for _, h := range hits {
		if len(rendered) == limit {
			break
		}
		rendered = append(rendered, fmt.Sprintf("%s:%d: %s", h.path, h.line, h.text))
	}
	more := ""
	if len(hits) > limit {
		more = fmt.Sprintf(" … and %d more", len(hits)-limit)
	}
	return fmt.Sprintf("stub scan: %d candidate hit(s) over the declared code surface — classify each during the sequence review (verify Step 6); a RELEVANT stub is a MISMATCH: %s%s", len(hits), strings.Join(rendered, " | "), more)
}

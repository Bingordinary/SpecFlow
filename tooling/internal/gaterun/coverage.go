package gaterun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
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
// identity and leaving public and relationship keys unchanged. A protected
// stable-record deferral (preserve:<owner>:<item>) rebinds to the owner's own
// acceptance item: that is the logical key owning the requirement's judgment in
// the owner's run (see framework/verification_scope.md §Deferred findings).
func deferredOwnerKey(key, source, owner string) string {
	if key == reviewKey(SessionKindArchitecture, source, "") {
		return reviewKey(SessionKindArchitecture, owner, "")
	}
	prefix := SessionKindDesign + ":" + source + ":"
	if strings.HasPrefix(key, prefix) {
		return reviewKey(SessionKindDesign, owner, strings.TrimPrefix(key, prefix))
	}
	preservePrefix := SessionKindPreserve + ":" + owner + ":"
	if strings.HasPrefix(key, preservePrefix) {
		return reviewKey(SessionKindItem, owner, strings.TrimPrefix(key, preservePrefix))
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

// loadCarriedEvidence snapshots the baseline evidence entries for the
// carried checks into the run at plan time. The snapshot is the run's
// immutable input: gate-finalize merges carried evidence from it instead of
// re-reading the baseline cache, which another run may have rewritten between
// plan and finalize.
func loadCarriedEvidence(repoRoot string, run *Run, carried []string) ([]CarriedEvidenceEntry, error) {
	baseline, err := validationcache.ReadGateBaseline(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	if !baseline.Exists {
		return nil, fmt.Errorf("carried-over checks %s have no baseline cache — plan a new full run", strings.Join(carried, ", "))
	}
	carriedSet := map[string]bool{}
	for _, key := range carried {
		carriedSet[key] = true
	}
	var out []CarriedEvidenceEntry
	for _, entry := range baseline.Entries {
		var checks []CarriedCheckEntry
		for _, c := range entry.Checks {
			if carriedSet[c.Check] {
				checks = append(checks, CarriedCheckEntry{Check: c.Check, Lens: c.Lens, Deps: append([]string(nil), c.Deps...)})
			}
		}
		if len(checks) == 0 {
			continue
		}
		// Snapshot only the file-level remainder no check owns: check deps
		// travel with their check entries, and a re-run check's superseded
		// deps must not be merged back into the new entry (they may name
		// changed content the re-run judgment no longer depends on).
		owned := map[string]bool{}
		for _, c := range entry.Checks {
			for _, dep := range c.Deps {
				owned[dep] = true
			}
		}
		var remainder []string
		for _, dep := range entry.Deps {
			if !owned[dep] {
				remainder = append(remainder, dep)
			}
		}
		out = append(out, CarriedEvidenceEntry{Path: entry.Path, Hash: entry.Hash, Deps: remainder, Checks: checks})
	}
	covered := map[string]bool{}
	for _, entry := range out {
		for _, c := range entry.Checks {
			covered[c.Check] = true
		}
	}
	for _, key := range carried {
		if !covered[key] {
			return nil, fmt.Errorf("carried-over check %q has no baseline evidence — plan a new full run", key)
		}
	}
	return out, nil
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
// covers the merged union of the acceptance item ids under the alignment lens
// and the declared code files under the quality lens.
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
// acceptance item ids under the alignment lens, then the declared code files
// under the quality lens.
func (d *Derivation) verifyCoverage(run *Run) ([]CoverageKey, error) {
	items, err := verifyItems(d.root, run)
	if err != nil {
		return nil, err
	}
	var coverage []CoverageKey
	for _, item := range items {
		coverage = append(coverage, CoverageKey{Key: reviewKey(SessionKindItem, run.TargetName, item), Kind: SessionKindItem, Lens: LensAlignment, Unit: run.TargetName, Item: item})
	}
	for _, file := range qualityFiles(run) {
		reads := run.PublicEvidence[file]
		coverage = append(coverage, CoverageKey{Key: reviewKey(SessionKindCode, "", file), Kind: SessionKindCode, Lens: LensQuality, File: file, ReadRefs: reads}, CoverageKey{Key: reviewKey(SessionKindDesign, run.TargetName, file), Kind: SessionKindDesign, Lens: LensQuality, File: file, Unit: run.TargetName})
	}
	coverage = append(coverage, CoverageKey{Key: reviewKey(SessionKindArchitecture, run.TargetName, ""), Kind: SessionKindArchitecture, Lens: LensQuality, Unit: run.TargetName})
	protected, err := d.protectedCoverage(run)
	if err != nil {
		return nil, err
	}
	return append(coverage, protected...), nil
}

// coverageReportKeys lists the reviewer report check keys one coverage key
// owns: the group's check numbers (validate unit), the check number (rule),
// or the key itself (verify item / quality file).
func coverageReportKeys(run *Run, ck CoverageKey) []string {
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
	for _, entry := range r.CarriedEvidence {
		for _, check := range entry.Checks {
			if check.Check == key {
				return check.Lens
			}
		}
	}
	return ""
}

// ExpectedVerifyCoverage computes the merged verify coverage set — the
// alignment acceptance-item keys and the quality declared-code-file keys — for
// the current target without persisting a run. promote uses it to require the
// published verify cache to cover both lenses.
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

// CoverageKeysForSpec maps a materialized session spec back to the coverage
// keys it covers: the reserved final key for the cross synthesis, else the
// coverage keys whose report keys the spec owns.
func CoverageKeysForSpec(run *Run, spec *SessionSpec) []string {
	if spec.Kind == SessionKindCross {
		return []string{CrossKey}
	}
	owned := map[string]bool{}
	for _, key := range spec.CheckKeys {
		owned[key] = true
	}
	var out []string
	for _, ck := range run.Coverage {
		if ck.Kind != spec.Kind {
			continue
		}
		all := true
		for _, key := range coverageReportKeys(run, ck) {
			if !owned[key] {
				all = false
				break
			}
		}
		if all {
			out = append(out, ck.Key)
		}
	}
	return out
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
		carried, err := fullRelationshipScope(d.root, run)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if err := validateCoverage(run, full); err != nil {
			return nil, nil, nil, nil, err
		}
		return full, carried, required, nil, nil
	}
	derivation, err := d.deriveDeltaRerun(run)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if derivation.degraded {
		// A full rerun must not forget relationships checked by its baseline.
		if baseline, err := validationcache.ReadGateBaseline(d.root, run.TargetKind, run.TargetName, run.Gate); err == nil {
			for _, entry := range baseline.Entries {
				for _, check := range entry.Checks {
					if IsRelationshipKey(check.Check) {
						run.Relationships = appendUnique(run.Relationships, relationshipName(check.Check))
					}
				}
			}
		}
		if err := validateCoverage(run, full); err != nil {
			return nil, nil, nil, nil, err
		}
		return full, nil, required, derivation.notices, nil
	}
	coverage, err := filterCoverage(full, derivation.rerunCoverage)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := validateCoverage(run, coverage); err != nil {
		return nil, nil, nil, nil, err
	}
	return coverage, derivation.carried, required, derivation.notices, nil
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
		case SessionKindChecks, SessionKindItem, SessionKindDesign, SessionKindCode, SessionKindArchitecture, SessionKindPreserve:
		default:
			return fmt.Errorf("coverage key %q has invalid kind %q", key, ck.Kind)
		}
		switch ck.Lens {
		case "":
			if run.Gate == GateVerify && run.TargetKind == TargetKindUnit {
				return fmt.Errorf("coverage key %q has no lens — a verify key must be tagged alignment or quality", key)
			}
		case LensAlignment:
			if ck.Kind != SessionKindItem && ck.Kind != SessionKindPreserve {
				return fmt.Errorf("alignment coverage key %q has kind %q, expected %q", key, ck.Kind, SessionKindItem)
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

// BuildSessionSpec materializes the read-only reviewer session spec for an
// agent-chosen key batch. Every key must belong to the run's coverage set (or
// be the reserved final-synthesis key `cross`) and all keys must share one
// report kind and one lens (a session never mixes shapes).
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
			Kind:          SessionKindCross,
			CheckKeys:     []string{CrossKey},
			ReadRefs:      crossReadRefs(run),
			Relationships: append([]string(nil), run.Relationships...),
			Context:       []string{"Check only the assigned relationships against current source and accepted/carried judgments. Do not repeat local checks. If no relationships are assigned, only dispose existing findings and derive effective statuses."},
		}, nil
	}
	wanted := map[string]bool{}
	kind := ""
	lens := ""
	for _, key := range keys {
		ck := run.CoverageByKey(key)
		if ck == nil {
			return nil, fmt.Errorf("key %q is not part of run %s's coverage set", key, run.RunID)
		}
		if kind == "" {
			kind = ck.Kind
		} else if ck.Kind != kind {
			return nil, fmt.Errorf("session mixes report kinds %q and %q — a session covers one kind only", kind, ck.Kind)
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
	spec := &SessionSpec{SessionID: SessionID(keys), Kind: kind}
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
		for _, key := range keys {
			ck := run.CoverageByKey(key)
			codeKey := reviewKey(SessionKindCode, "", ck.File)
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
		spec.Context = append(spec.Context, "Consume only the immutable public records for the assigned files, regardless of their execution batch. Actively check the unit spec. Dispose every observation in those records with a reason; retain it as a finding or exclude it with evidence from this unit's design. Never alter the public record.")
	}
	if kind == SessionKindCode {
		spec.Context = append(spec.Context, "Collect code facts and potential problems only. Do not read unit-private designs or suppress problems by a unit's rationale. Read the whole public evidence surface; missing evidence requires replanning.")
	}
	if kind == SessionKindArchitecture {
		spec.Context = append(spec.Context, "Assess all six Dimension 8 architecture fields once for the whole unit.")
	}
	if IsItemKind(kind) {
		spec.Context = append(spec.Context, "For Steps 1 and 5, attribute code structures and designs to the responsibility of the unit owning each key before judging surplus; for preserve, assess only the assigned protected stable requirement. Shared-file association is not exclusive ownership. Independent behavior outside that responsibility is not surplus; helpers and shared mechanisms implementing or constraining the assigned requirement remain in scope. If attribution evidence is missing, replan with the needed input rather than infer a mismatch from file co-location.")
	}
	if kind == SessionKindPreserve {
		spec.Context = append(spec.Context, "Use only the protected unit's stable spec. A declared implementation mapping that no longer resolves is MISMATCH — never ALIGNED. When the requirement's behavior is still implemented (the protected unit's own code or its current round declares it), the mismatch is peer-owned record drift: the final synthesis routes it to the protected unit by recorded ownership and it does not block this run. When the behavior is not implemented anywhere, it stays this run's blocking finding.")
	}
	return spec, nil
}

// coverageReadRefs derives the read surface of one coverage key. A validate
// group's surface is the existing group read refs; a verify item reads the own
// spec plus the whole declared code surface; a file reads the own spec plus
// that file; all add the agent-declared extra inputs.
func coverageReadRefs(repoRoot string, run *Run, ck CoverageKey) []string {
	switch ck.Kind {
	case SessionKindChecks:
		if classifyRun(run) == classRuleValidate {
			return appendUnique(allRefNames(run), extraInputPaths(run)...)
		}
		return unitValidateGroupReadRefs(repoRoot, run, ck.Key)
	case SessionKindItem:
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, surfacePaths(run)...)
		for _, ref := range run.Refs {
			if strings.HasPrefix(ref.Ref, "rule:") {
				read = appendUnique(read, ref.Ref)
			}
		}
		return appendUnique(read, extraInputPaths(run)...)
	case SessionKindCode, SessionKindPreserve:
		return ck.ReadRefs
	case SessionKindArchitecture:
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, surfacePaths(run)...)
		for _, ref := range run.Refs {
			if strings.HasPrefix(ref.Ref, "rule:") {
				read = appendUnique(read, ref.Ref)
			}
		}
		return appendUnique(read, extraInputPaths(run)...)
	case SessionKindDesign:
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, ck.File)
		for _, ref := range run.Refs {
			if strings.HasPrefix(ref.Ref, "rule:") {
				read = appendUnique(read, ref.Ref)
			}
		}
		return appendUnique(read, extraInputPaths(run)...)
	}
	return nil
}

// crossReadRefs lists the final synthesis's read surface: every snapshot ref
// and every code surface file.
func crossReadRefs(run *Run) []string {
	read := append([]string(nil), allRefNames(run)...)
	read = append(read, surfacePaths(run)...)
	return appendUnique(read, extraInputPaths(run)...)
}

// scopeDerivation is the mechanism-derived re-run scope of one delta/repair
// plan: the declared judgments that must re-execute, the ones carried over,
// and every disclosure the plan owes the user. gate-plan and the fresh@
// DELTA SCOPE preview share this derivation, so both report one scope (see
// framework/verification_scope.md §Delta Runs).
type scopeDerivation struct {
	scope         *validationcache.StaleScope // raw stale evidence (nil when derivation degraded before reading it)
	current       []string                    // current non-cross judgment keys (items, quality files, or check numbers)
	newKeys       []string                    // current keys absent from the baseline declaration — no evidence to carry
	rerun         []string                    // sorted effective re-run check keys
	rerunCoverage []string                    // sorted coverage keys that must re-execute
	carried       []string                    // sorted declared keys that stay carried over
	coversFull    bool                        // the re-run covers every declared check — nothing is carried over
	degraded      bool                        // the plan is the full coverage set because no scope could be derived
	reason        string                      // degradation reason (degraded only)
	notices       []string
}

// baselineFailureRecord reports whether a baseline cache is a failure record
// whose recovery runs in `--mode repair` (see
// framework/verification_scope.md §Delta Runs → Failure recovery). A failure
// record declares `result: fail` (and `blocking: true`).
func baselineFailureRecord(baseline *validationcache.GateBaseline) bool {
	return baseline.Result == "fail"
}

// noUsableBaselineError is the documented message for a stable-only target
// with no usable confirmation baseline (see framework/verification_scope.md
// §Delta Runs → Layer applicability): the full confirmation run or a fork.
func noUsableBaselineError(run *Run) error {
	return fmt.Errorf("No usable confirmation baseline. Run the full `%s@%s` (confirmation check) first, or `specflowctl fork %s` to start a new round.", run.Gate, run.TargetName, gateTargetFlags(run))
}

// deriveDeltaRerun derives the delta/repair re-run scope from the baseline
// cache's per-check evidence. The re-run set is the stale-judgment set, the
// force-listed keys, and every current key the baseline never declared — a new
// acceptance item or quality file has no evidence to carry over, so it must
// execute exactly like a stale judgment. The derived check set is then
// expressed over the coverage set (rerunCoverage). Where the derivation cannot
// trust its association it degrades conservatively to the full coverage set
// and reports the degradation.
func (d *Derivation) deriveDeltaRerun(run *Run) (*scopeDerivation, error) {
	repoRoot := d.root
	baseline, err := validationcache.ReadGateBaseline(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	switch run.Mode {
	case ModeDelta:
		if !baseline.Exists {
			if run.Target == TargetStable {
				return nil, noUsableBaselineError(run)
			}
			return nil, fmt.Errorf("no baseline cache for this target — run the full command instead: `specflowctl gate-plan --gate %s %s --target %s`", run.Gate, gateTargetFlags(run), run.Target)
		}
		if baseline.Mode != "full" {
			return nil, fmt.Errorf("baseline cache mode is %q, expected full — run the full command instead", baseline.Mode)
		}
		// The cached declarations must agree with each other before either
		// one is trusted as a pass baseline: a cache that says result: fail
		// while claiming not to block (or the reverse) is malformed state and
		// cannot say what passed, so nothing may be carried over (see
		// framework/validation_cache.md §Format: the blocking field).
		if strings.TrimSpace(baseline.Result) != "" && (baseline.Result == "fail") != baseline.Blocking {
			return nil, fmt.Errorf("baseline cache declarations conflict (result: %q, blocking: %t) — run the full command instead", baseline.Result, baseline.Blocking)
		}
		if baselineFailureRecord(baseline) {
			return nil, fmt.Errorf("baseline is a failure record — use `--mode repair` after resolving the findings")
		}
		if baseline.Result != "pass" {
			return nil, fmt.Errorf("baseline cache result is %q, expected pass — run the full command instead", baseline.Result)
		}
	case ModeRepair:
		if !baseline.Exists && run.Target == TargetStable {
			return nil, noUsableBaselineError(run)
		}
		if !baseline.Exists || baseline.Result != "fail" || !baseline.Blocking {
			return nil, fmt.Errorf("--mode repair requires a failure-record baseline (result: fail, blocking: true) — none found; run the full command instead")
		}
	default:
		return nil, fmt.Errorf("delta derivation requires --mode delta or --mode repair")
	}

	degraded := func(reason string, scope *validationcache.StaleScope) *scopeDerivation {
		return &scopeDerivation{
			scope:    scope,
			degraded: true,
			reason:   reason,
			notices:  []string{reason + " — the plan covers the full scope"},
		}
	}

	if run.Mode == ModeRepair {
		// The failure baseline must declare a status for every check: absent
		// status means pass only on a pass baseline, never on a failure one.
		// The values are a closed set and a full-run record never carries a
		// judgment — an unknown value or an illegal carried entry cannot say
		// which judgments failed, so nothing may be carried over (see
		// framework/verification_scope.md §Delta Runs → Failure recovery).
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
		for _, entry := range baseline.Entries {
			for _, c := range entry.Checks {
				declared[c.Check] = true
				status := strings.TrimSpace(c.Status)
				// A check declared with conflicting statuses across entries
				// cannot say which judgment failed; take the fail-closed
				// degradation path instead of trusting the first occurrence.
				if prior, ok := statusByCheck[c.Check]; ok {
					if prior != status {
						addInvalid(c.Check + "=conflicting")
					}
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
				case status == "carried" && !IsRelationshipKey(c.Check) && (baseline.Basis == ModeFull || baseline.Basis == ""):
					// `basis: full` (or absent — legacy records) has no
					// carried judgments (see framework/validation_cache.md
					// §Format: the basis field).
					addInvalid(c.Check + "=carried")
				}
			}
		}
		if len(missing) > 0 {
			return degraded("the failure record carries an incomplete per-check status map (missing: "+strings.Join(missing, ", ")+")", nil), nil
		}
		if len(invalid) > 0 {
			return degraded("the failure record carries an invalid per-check status map (invalid: "+strings.Join(invalid, ", ")+")", nil), nil
		}
		// The status map must describe exactly the judgments the record's
		// structured baseline holds: a status map and a judgment baseline
		// that disagree on the key set cannot say which judgments failed, so
		// nothing may be carried over. An absent or older judgment schema is
		// left to the carry-time check, which refuses a partial plan that
		// must carry a judgment with the "run the full command" guidance.
		if state, err := validatedJudgmentState(baseline); err == nil {
			var mismatch []string
			for key := range declared {
				if _, ok := state.LogicalStatus[key]; !ok {
					mismatch = append(mismatch, key)
				}
			}
			for key := range state.LogicalStatus {
				// The reserved final-synthesis key is a synthesis status, not
				// a per-check status, and has no `checks` entry to match.
				if key == CrossKey {
					continue
				}
				if !declared[key] {
					mismatch = append(mismatch, key)
				}
			}
			if len(mismatch) > 0 {
				sort.Strings(mismatch)
				return degraded("the failure record's per-check status map does not match its judgment baseline (unmatched: "+strings.Join(mismatch, ", ")+")", nil), nil
			}
		}
	}
	if run.Gate == GateVerify {
		recorded := make(map[string]bool, len(baseline.Entries))
		for _, entry := range baseline.Entries {
			recorded[filepath.ToSlash(filepath.Clean(entry.Path))] = true
		}
		var newEvidence []string
		for _, path := range extraInputPaths(run) {
			if !isLogicalRef(path) && !recorded[filepath.ToSlash(filepath.Clean(path))] {
				newEvidence = append(newEvidence, path)
			}
		}
		if len(newEvidence) > 0 {
			return degraded("verify evidence absent from the baseline cache: "+strings.Join(newEvidence, ", "), nil), nil
		}
	}

	scope, err := validationcache.DeriveStaleScope(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	if len(scope.Unreadable) > 0 {
		return degraded("the baseline cache lists entries that cannot be resolved or read: "+strings.Join(scope.Unreadable, ", "), scope), nil
	}
	if !baseline.HasChecks || len(baseline.Checks) == 0 {
		reason := "the baseline cache carries no per-check evidence"
		if run.Mode == ModeRepair {
			reason = "the failure record carries no per-check status map"
		}
		return degraded(reason, scope), nil
	}
	// A baseline entry without dependency chunks over a file with content, or
	// a baseline files list that does not cover the target's main file, is
	// stale for a cause the declared per-check evidence cannot attribute.
	// Degrade here — before any execution — instead of planning a partial run
	// the finalize self-check must reject (see
	// framework/verification_scope.md §Delta Runs → Incremental scope).
	if len(scope.Untrackable) > 0 {
		return degraded("the baseline cache is stale for a cause the declared per-check evidence cannot attribute (no dependency chunks): "+strings.Join(scope.Untrackable, ", "), scope), nil
	}
	for _, required := range requiredFiles(run) {
		listed := false
		for _, entry := range baseline.Entries {
			if filepath.ToSlash(filepath.Clean(entry.Path)) == filepath.ToSlash(filepath.Clean(required)) {
				listed = true
				break
			}
		}
		if !listed {
			return degraded("the baseline cache files list does not include the main file "+required, scope), nil
		}
	}

	declaredNonCross := map[string]bool{}
	for _, c := range baseline.Checks {
		if c.Check != CrossKey {
			declaredNonCross[c.Check] = true
		}
	}

	rerun := map[string]bool{}
	if run.Gate == GateVerify {
		state, err := validatedJudgmentState(baseline)
		if err != nil {
			return nil, err
		}
		current, err := d.computeCoverage(run)
		if err != nil {
			return nil, err
		}
		for _, ck := range current {
			binding, ok := state.Records[ck.Key]
			if !ok {
				scope.Affected = appendUnique(scope.Affected, ck.Key)
				continue
			}
			binding.Layer = run.Target
			if ck.Kind == SessionKindPreserve {
				binding.Layer = TargetStable
			}
			inputs, err := normalizeOwnInputs(repoRoot, coverageReadRefs(repoRoot, run, ck), ck.Unit, binding.Layer)
			if err != nil {
				return nil, err
			}
			record, err := judgments.Load(repoRoot, binding.Reference)
			if err != nil || judgments.Check(repoRoot, binding.Reference, binding.Layer, run.Protocol) != nil || !judgments.CoversInputs(record.Inputs, inputs) {
				scope.Affected = appendUnique(scope.Affected, ck.Key)
			}
		}
	}
	for _, key := range append([]string(nil), scope.Affected...) {
		if strings.HasPrefix(key, "code:") {
			scope.Affected = appendUnique(scope.Affected, "design:"+run.TargetName+":"+strings.TrimPrefix(key, "code:"))
		}
	}
	for _, k := range scope.Affected {
		rerun[k] = true
	}
	for _, k := range run.RerunKeys {
		rerun[k] = true
	}
	for _, name := range run.Relationships {
		rerun[RelationshipKey(name)] = true
	}
	if run.Mode == ModeRepair {
		for _, k := range baseline.InvalidatedChecks {
			rerun[k] = true
		}
	}

	// Map unclaimed stale sources by the command's fixed association; where
	// no association exists, degrade.
	for _, entry := range scope.Unclaimed {
		switch run.Gate {
		case GateValidate:
			switch {
			case strings.HasPrefix(entry, "unit:"):
				if run.TargetKind == TargetKindRule {
					rerun["4"] = true
				} else {
					rerun["7"] = true
				}
			case strings.HasPrefix(entry, "rule:"):
				rerun["8"] = true
			default:
				return degraded(fmt.Sprintf("stale dependency %s has no fixed check association", entry), scope), nil
			}
		default:
			return degraded(fmt.Sprintf("stale dependency %s has no fixed check association", entry), scope), nil
		}
	}

	if run.Mode == ModeRepair {
		for _, c := range baseline.Checks {
			if c.Status == "fail" {
				rerun[c.Check] = true
			}
		}
	}

	current, err := d.currentGateKeys(run)
	if err != nil {
		return nil, err
	}
	for key := range declaredNonCross {
		if IsRelationshipKey(key) {
			name := relationshipName(key)
			if !stringInSlice(RelationshipNames(run.Gate), name) || run.TargetKind == TargetKindRule {
				return nil, fmt.Errorf("baseline declares unknown relationship %q", key)
			}
			current = appendUnique(current, key)
		}
	}
	for _, name := range run.Relationships {
		current = appendUnique(current, RelationshipKey(name))
	}
	currentSet := map[string]bool{}
	for _, key := range current {
		currentSet[key] = true
	}
	var newKeys []string
	for _, key := range current {
		if !declaredNonCross[key] && !rerun[key] {
			newKeys = append(newKeys, key)
			rerun[key] = true
		}
	}
	sort.Strings(newKeys)

	if run.Mode == ModeDelta && len(scope.StaleDeps) == 0 && len(scope.Affected) == 0 && len(scope.Unclaimed) == 0 && len(newKeys) == 0 && len(run.RerunKeys) == 0 && len(run.Relationships) == 0 {
		// Nothing re-executes under the per-check derivation. That means either
		// the cache is genuinely fresh, or it is stale for a cause the declared
		// per-check evidence cannot attribute (e.g. an entry with no dependency
		// chunks, or the main file missing from the files list). The gate's own
		// freshness chain decides — the report and the plan must not disagree.
		check, err := validationcache.CheckWriteResult(repoRoot, run.TargetKind, run.TargetName, run.Gate, run.Target)
		if err != nil {
			return nil, err
		}
		if !check.Fresh {
			return degraded("the baseline cache is stale for a cause the declared per-check evidence cannot attribute: "+check.Reason, scope), nil
		}
		return nil, fmt.Errorf("cache is fresh — no incremental re-run needed")
	}

	// Recorded or forced judgments that no longer exist in the current
	// surface cannot be re-executed or carried — fail closed.
	for _, key := range sortedKeySet(declaredNonCross) {
		if !currentSet[key] {
			return degraded(fmt.Sprintf("recorded judgment %q is no longer in the current coverage surface", key), scope), nil
		}
	}
	for key := range rerun {
		if declaredNonCross[key] {
			continue
		}
		if !currentSet[key] {
			return degraded(fmt.Sprintf("re-run judgment %q is not in the current coverage surface", key), scope), nil
		}
	}

	// Reject keys the gate cannot own, then expand the re-run set to its
	// effective coverage (a unit validate group re-runs every check it owns).
	effective := map[string]bool{}
	if run.TargetKind == TargetKindRule {
		for key := range rerun {
			if !isRuleValidateCheck(key) {
				return degraded(fmt.Sprintf("recorded check %q is not a rule validate check", key), scope), nil
			}
			effective[key] = true
		}
	} else if run.Gate == GateValidate {
		for key := range rerun {
			if IsRelationshipKey(key) {
				effective[key] = true
				continue
			}
			group, ok := validateGroupForCheck(key)
			if !ok {
				return degraded(fmt.Sprintf("recorded check %q is not a unit validate check", key), scope), nil
			}
			for _, c := range groupChecks(group) {
				effective[c] = true
			}
		}
	} else {
		for key := range rerun {
			effective[key] = true
		}
	}

	if len(effective) == 0 {
		// Stale evidence that maps to no coverage key cannot be re-judged by
		// a partial run, so the plan degrades to the full coverage set.
		return degraded("the stale evidence has no coverage key in the coverage model", scope), nil
	}
	rerunCoverage := coverageKeysForRerun(run, effective)
	run.Relationships = nil
	for _, key := range sortedKeySet(effective) {
		if IsRelationshipKey(key) {
			run.Relationships = append(run.Relationships, relationshipName(key))
		}
	}

	carried := make([]string, 0, len(declaredNonCross))
	for key := range declaredNonCross {
		if !effective[key] {
			carried = append(carried, key)
		}
	}
	sort.Strings(carried)

	// A plan that carries anything over needs the baseline's structured
	// judgment state; without it the partial run is refused here, where both
	// `fresh@` (PreviewDeltaScope) and `gate-plan` see the same derivation.
	if len(carried) > 0 {
		if _, err := validatedJudgmentState(baseline); err != nil {
			return nil, err
		}
	}

	derivation := &scopeDerivation{
		scope:         scope,
		current:       current,
		newKeys:       newKeys,
		rerun:         sortedKeySet(effective),
		rerunCoverage: rerunCoverage,
		carried:       carried,
		coversFull:    len(carried) == 0,
	}
	if len(rerunCoverage) == 0 && len(run.Relationships) == 0 {
		return nil, fmt.Errorf("cache is fresh — no incremental re-run needed")
	}
	if len(baseline.InvalidatedChecks) > 0 {
		derivation.notices = append(derivation.notices, "persisted targeted invalidations: "+strings.Join(baseline.InvalidatedChecks, ", "))
	}
	derivation.notices = append(derivation.notices, rerunPlanNotice(derivation))
	if derivation.coversFull {
		derivation.notices = append(derivation.notices, "the re-run covers every declared check — the plan covers the full scope")
	}
	return derivation, nil
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
// acceptance item ids, or quality code files (verify).
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
// and rule_refs to verify that they exist, while Checks 7-9 (dependencies)
// read those logical objects for cross-unit, constraint, and
// surface-ownership judgments. The design, acceptance, and clarity groups stay
// limited to the unit's own truth and shared evidence inputs; they do not
// receive unrelated logical objects, code evidence, or extra read refs beyond
// the agent-declared shared inputs.
func unitValidateGroupReadRefs(repoRoot string, run *Run, groupID string) []string {
	read := ownSpecPaths(repoRoot, run)
	read = appendUnique(read, extraInputPaths(run)...)
	read = appendUnique(read, affectsEvidencePaths(run)...)
	if groupID == "structural" || groupID == "dependencies" {
		read = appendUnique(read, logicalRefNames(run)...)
	}
	return append([]string(nil), read...)
}

// rerunPlanNotice renders the plan's re-run/carry disclosure. The carried
// clause states the trust basis the delta run relies on: carried judgments
// keep their dependency evidence unchanged (see
// framework/verification_scope.md §Delta Runs).
func rerunPlanNotice(derivation *scopeDerivation) string {
	msg := "re-run coverage keys " + strings.Join(derivation.rerunCoverage, ", ")
	if len(derivation.newKeys) > 0 {
		msg += "; new checks not in the baseline: " + strings.Join(derivation.newKeys, ", ")
	}
	if len(derivation.carried) > 0 {
		msg += "; carried over: " + strings.Join(derivation.carried, ", ") + " (their dependency evidence is unchanged)"
	} else {
		msg += "; nothing carried over"
	}
	return msg
}

func isRuleValidateCheck(check string) bool {
	for _, c := range ruleValidateChecks {
		if c == check {
			return true
		}
	}
	return false
}

// affectsEvidencePaths lists the spec-derived affects.files evidence files
// (SourceDerivedAffects). They are part of the validate read surface: local
// validate sessions may read and declare them, but they never create coverage
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

// extraInputPaths lists every agent-declared evidence input. These paths are
// readable by sessions but never define coverage keys.
func extraInputPaths(run *Run) []string {
	var out []string
	for _, input := range run.ExtraInputs {
		expanded := false
		for _, surface := range run.Surfaces {
			if surface.Path != input {
				continue
			}
			for _, entry := range surface.Entries {
				out = append(out, entry.Path)
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
			out = append(out, entry.Path)
		}
	}
	sort.Strings(out)
	return dedupeStrings(out)
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

// gateTargetFlags renders the unit/rule flag combination of a run for
// guidance text.
func gateTargetFlags(run *Run) string {
	if run.TargetKind == TargetKindRule {
		return "--rule " + run.TargetName
	}
	return "--unit " + run.TargetName
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

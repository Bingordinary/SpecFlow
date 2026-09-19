package gaterun

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func loadCarriedResults(repoRoot string, run *Run, carried []string) ([]PacketResult, error) {
	baseline, err := validationcache.ReadGateBaseline(repoRoot, run.TargetKind, run.TargetName, run.Gate)
	if err != nil {
		return nil, err
	}
	state, err := validatedJudgmentState(baseline)
	if err != nil {
		return nil, err
	}
	results := make([]PacketResult, 0, len(carried))
	for _, key := range carried {
		status, ok := state.LogicalStatus[key]
		if !ok {
			return nil, fmt.Errorf("baseline judgment state has no logical status for carried check %q — run the full command", key)
		}
		result := PacketResult{
			PacketID:        "carried:" + key,
			Kind:            "carried",
			EffectiveStatus: map[string]string{key: status},
			ReportDigest:    state.SynthesisDigest,
		}
		for _, finding := range state.Findings {
			if finding.SourceKey == key || stringInSlice(finding.AffectedKeys, key) {
				result.Findings = append(result.Findings, finding)
			}
		}
		results = append(results, result)
	}
	return results, nil
}

// validatedJudgmentState re-reads and validates the baseline cache's
// structured judgment state (the GATE_JUDGMENTS schema-2 block). A cache
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
	if err := json.Unmarshal([]byte(baseline.Judgments), &state); err != nil || state.SchemaVersion != 2 || state.SynthesisDigest == "" {
		return JudgmentBaseline{}, fmt.Errorf("baseline cache has an invalid structured judgment state — run the full command before using delta/repair")
	}
	for _, finding := range state.Findings {
		if strings.TrimSpace(finding.Detail) == "" {
			return JudgmentBaseline{}, fmt.Errorf("baseline finding %q has no renderable detail — run the full command before using delta/repair", finding.ID)
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
	pathIndex := map[string]int{}
	for _, entry := range baseline.Entries {
		var checks []CarriedCheckEntry
		for _, c := range entry.Checks {
			if carriedSet[c.Check] {
				checks = append(checks, CarriedCheckEntry{Check: c.Check, Deps: append([]string(nil), c.Deps...)})
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
		idx, seen := pathIndex[entry.Path]
		if !seen {
			pathIndex[entry.Path] = len(out)
			out = append(out, CarriedEvidenceEntry{Path: entry.Path, Hash: entry.Hash, Deps: remainder})
			idx = len(out) - 1
		}
		out[idx].Checks = append(out[idx].Checks, checks...)
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

// validateCheckGroups is the fixed packet decomposition of the unit validate
// checklist: every one of the 8 checks belongs to exactly one group, and the
// cross-check is its own packet. The mapping is part of the gate contract
// (framework/verification_scope.md §Gate Work Packets → Packet generation
// rules) — the same plan is generated for the same target every time.
var validateCheckGroups = []struct {
	PacketID string
	Checks   []string
}{
	{"structural", []string{"1", "3", "6"}},
	{"design", []string{"2", "4"}},
	{"acceptance", []string{"5"}},
	{"dependencies", []string{"7", "8"}},
}

// ruleValidateChecks is the rule validate packet's check key set (the 8 rule
// checks; rules have no cross-check).
var ruleValidateChecks = []string{"1", "2", "3", "4", "5", "6", "7", "8"}

// validatePacketOwner maps a unit validate check key to its packet id.
func validatePacketOwner(check string) (string, bool) {
	for _, group := range validateCheckGroups {
		for _, c := range group.Checks {
			if c == check {
				return group.PacketID, true
			}
		}
	}
	return "", false
}

// buildPacketPlan generates the deterministic packet plan for a run. It
// returns the packets, the carried-over check keys (delta/repair only), the
// target's required files (the main spec / rule file that the assembled
// evidence must cover), and plan notices (scope derivation and conservative
// degradations).
func buildPacketPlan(repoRoot string, run *Run) ([]PacketSpec, []string, []string, []string, error) {
	required := requiredFiles(run)
	var packets []PacketSpec
	var carried, notices []string
	var err error
	if run.Mode == ModeFull {
		packets, err = fullPacketPlan(repoRoot, run)
	} else {
		packets, carried, notices, err = derivedPacketPlan(repoRoot, run)
	}
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := validatePacketPlan(run, packets); err != nil {
		return nil, nil, nil, nil, err
	}
	return packets, carried, required, notices, err
}

// validatePacketPlan rejects ambiguous or unusable packet graphs before any
// run state is persisted. PacketByID is intentionally a simple lookup, so its
// precondition — one unique packet for every id — is established here and
// rechecked whenever a run is loaded.
func validatePacketPlan(run *Run, packets []PacketSpec) error {
	if len(packets) == 0 {
		return fmt.Errorf("packet plan is empty")
	}
	byID := make(map[string]PacketSpec, len(packets))
	crossCount := 0
	for _, packet := range packets {
		id := strings.TrimSpace(packet.PacketID)
		if id == "" {
			return fmt.Errorf("packet plan contains an empty packet id")
		}
		if _, exists := byID[id]; exists {
			return fmt.Errorf("packet plan contains duplicate packet id %q", id)
		}
		byID[id] = packet
		if id == CrossKey || packet.Kind == PacketKindCross {
			if id != CrossKey || packet.Kind != PacketKindCross {
				return fmt.Errorf("packet id %q and kind %q violate the reserved cross packet contract", id, packet.Kind)
			}
			crossCount++
		}
		seenChecks := map[string]bool{}
		for _, key := range packet.CheckKeys {
			key = strings.TrimSpace(key)
			if key == "" {
				return fmt.Errorf("packet %q contains an empty check key", id)
			}
			if seenChecks[key] {
				return fmt.Errorf("packet %q contains duplicate check key %q", id, key)
			}
			// `cross` is reserved for the single cross packet; a non-cross
			// packet owning it would alias the run's logical status keys
			// (acceptance items and reviewed file paths included).
			if key == CrossKey && packet.Kind != PacketKindCross {
				return fmt.Errorf("packet %q declares the reserved check key %q", id, CrossKey)
			}
			seenChecks[key] = true
		}
	}
	wantCross := 1
	if run.TargetKind == TargetKindRule {
		wantCross = 0
	}
	if crossCount != wantCross {
		return fmt.Errorf("packet plan contains %d cross packets; expected %d", crossCount, wantCross)
	}

	for _, packet := range packets {
		seenDeps := map[string]bool{}
		for _, dependency := range packet.DependsOn {
			if dependency == packet.PacketID {
				return fmt.Errorf("packet %q depends on itself", packet.PacketID)
			}
			if seenDeps[dependency] {
				return fmt.Errorf("packet %q contains duplicate dependency %q", packet.PacketID, dependency)
			}
			seenDeps[dependency] = true
			if _, exists := byID[dependency]; !exists {
				return fmt.Errorf("packet %q depends on missing packet %q", packet.PacketID, dependency)
			}
		}
	}

	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("packet dependency graph contains a cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range byID[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// requiredFiles lists the target's own main file — the file the assembled
// cache evidence must cover (unit main spec; rule file). Appendices are
// covered by the appendix gate, not by this list.
func requiredFiles(run *Run) []string {
	if run.TargetKind == TargetKindUnit {
		if run.Target == TargetCandidate {
			return []string{specpaths.CandidateUnitSpecFileRef(run.TargetName)}
		}
		return []string{specpaths.StableUnitSpecFileRef(run.TargetName)}
	}
	if run.Target == TargetCandidate {
		return []string{specpaths.RuleCandidateFileRef(run.TargetName)}
	}
	return []string{specpaths.RuleStableFileRef(run.TargetName)}
}

// fullPacketPlan generates the complete packet set for the run's gate and
// target.
func fullPacketPlan(repoRoot string, run *Run) ([]PacketSpec, error) {
	switch run.TargetKind {
	case TargetKindRule:
		return []PacketSpec{{
			PacketID:  "checks",
			Kind:      PacketKindChecks,
			CheckKeys: append([]string(nil), ruleValidateChecks...),
			ReadRefs:  appendUnique(allRefNames(run), extraInputPaths(run)...),
		}}, nil
	}
	switch run.Gate {
	case GateValidate:
		var packets []PacketSpec
		for _, group := range validateCheckGroups {
			packets = append(packets, PacketSpec{
				PacketID:  group.PacketID,
				Kind:      PacketKindChecks,
				CheckKeys: append([]string(nil), group.Checks...),
				ReadRefs:  unitValidatePacketReadRefs(repoRoot, run, group.PacketID),
			})
		}
		packets = append(packets, crossPacket(run, packetIDs(packets)))
		return packets, nil
	case GateVerify:
		items, err := verifyItems(repoRoot, run)
		if err != nil {
			return nil, err
		}
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, surfacePaths(run)...)
		read = appendUnique(read, extraInputPaths(run)...)
		var packets []PacketSpec
		for _, item := range items {
			detectID := detectionPacketID(item)
			analysisID := analysisPacketID(item)
			packets = append(packets, PacketSpec{
				PacketID:  detectID,
				Kind:      PacketKindItem,
				CheckKeys: []string{item},
				ReadRefs:  append([]string(nil), read...),
			})
			packets = append(packets, PacketSpec{
				PacketID:  analysisID,
				Kind:      PacketKindAnalysis,
				CheckKeys: []string{item},
				DependsOn: []string{detectID},
				ReadRefs:  append([]string(nil), read...),
			})
		}
		packets = append(packets, crossPacket(run, packetIDs(packets)))
		return packets, nil
	case GateReview:
		files := reviewFiles(run)
		var packets []PacketSpec
		for _, file := range files {
			read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
			read = append(read, file)
			read = appendUnique(read, extraInputPaths(run)...)
			packets = append(packets, PacketSpec{
				PacketID:  file,
				Kind:      PacketKindFile,
				CheckKeys: []string{file},
				ReadRefs:  read,
			})
		}
		packets = append(packets, crossPacket(run, packetIDs(packets)))
		return packets, nil
	}
	return nil, fmt.Errorf("unsupported gate %q for %s target", run.Gate, run.TargetKind)
}

// crossPacket builds the cross-check packet depending on every given packet.
func crossPacket(run *Run, dependsOn []string) PacketSpec {
	read := append([]string(nil), allRefNames(run)...)
	read = append(read, surfacePaths(run)...)
	return PacketSpec{
		PacketID:  CrossKey,
		Kind:      PacketKindCross,
		CheckKeys: []string{CrossKey},
		DependsOn: dependsOn,
		ReadRefs:  read,
	}
}

func detectionPacketID(item string) string { return "detect:" + item }

func analysisPacketID(item string) string { return "analysis:" + item }

func packetIDs(packets []PacketSpec) []string {
	var out []string
	for _, p := range packets {
		out = append(out, p.PacketID)
	}
	return out
}

// scopeDerivation is the mechanism-derived re-run scope of one delta/repair
// plan: the declared judgments that must re-execute, the ones carried over,
// and every disclosure the plan owes the user. gate-plan and the fresh@
// DELTA SCOPE preview share this derivation, so both report one scope (see
// framework/verification_scope.md §Delta Runs).
type scopeDerivation struct {
	scope      *validationcache.StaleScope // raw stale evidence (nil when derivation degraded before reading it)
	current    []string                    // current non-cross judgment keys (items, review files, or check numbers)
	newKeys    []string                    // current keys absent from the baseline declaration — no evidence to carry
	rerun      []string                    // sorted effective re-run set; includes the cross key for unit targets
	carried    []string                    // sorted declared keys that stay carried over
	coversFull bool                        // the re-run covers every declared check — nothing is carried over
	degraded   bool                        // the plan is the full packet set because no scope could be derived
	reason     string                      // degradation reason (degraded only)
	notices    []string
}

// baselineFailureRecord reports whether a baseline cache is a failure record
// whose recovery runs in `--mode repair` (see
// framework/verification_scope.md §Delta Runs → Failure recovery). Review
// records its failure state in `blocking`; validate/verify in `result`.
func baselineFailureRecord(gate string, baseline *validationcache.GateBaseline) bool {
	if gate == GateReview {
		return baseline.Blocking
	}
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
// force-listed keys, the cross-check (unit targets), and every current key
// the baseline never declared — a new acceptance item or review file has no
// evidence to carry over, so it must execute exactly like a stale judgment.
// Where the derivation cannot trust its association it degrades
// conservatively to the full packet set and reports the degradation.
func deriveDeltaRerun(repoRoot string, run *Run) (*scopeDerivation, error) {
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
		if baselineFailureRecord(run.Gate, baseline) {
			reason := "baseline is a failure record"
			if run.Gate == GateReview {
				reason = "baseline review cache is blocking"
			}
			return nil, fmt.Errorf("%s — use `--mode repair` after resolving the findings", reason)
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
				case status == "carried" && (baseline.Basis == ModeFull || baseline.Basis == ""):
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
	if run.TargetKind != TargetKindRule {
		rerun[CrossKey] = true
	}
	for _, k := range scope.Affected {
		rerun[k] = true
	}
	for _, k := range run.RerunKeys {
		rerun[k] = true
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
					rerun["5"] = true
					rerun["7"] = true
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

	current, err := currentGateKeys(repoRoot, run)
	if err != nil {
		return nil, err
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

	if run.Mode == ModeDelta && len(scope.StaleDeps) == 0 && len(scope.Affected) == 0 && len(scope.Unclaimed) == 0 && len(newKeys) == 0 && len(run.RerunKeys) == 0 {
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
			return degraded(fmt.Sprintf("recorded judgment %q is no longer in the %s surface", key, runSurfaceLabel(run)), scope), nil
		}
	}
	for key := range rerun {
		if key == CrossKey || declaredNonCross[key] {
			continue
		}
		if !currentSet[key] {
			return degraded(fmt.Sprintf("re-run judgment %q is not in the %s surface", key, runSurfaceLabel(run)), scope), nil
		}
	}

	// Reject keys the gate cannot own, then expand the re-run set to its
	// effective coverage (a unit validate packet re-runs every check it owns).
	effective := map[string]bool{}
	if run.TargetKind == TargetKindRule {
		for key := range rerun {
			if key == CrossKey {
				continue
			}
			if !isRuleValidateCheck(key) {
				return degraded(fmt.Sprintf("recorded check %q is not a rule validate check", key), scope), nil
			}
			effective[key] = true
		}
	} else if run.Gate == GateValidate {
		effective[CrossKey] = true
		for key := range rerun {
			if key == CrossKey {
				continue
			}
			group, ok := validatePacketOwner(key)
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

	// The re-run set always includes the cross-check, so it covers every
	// declared check exactly when nothing is carried over — including a
	// baseline declaring only the cross-check (see
	// framework/validation_cache.md §Format → Per-check evidence).
	derivation := &scopeDerivation{
		scope:      scope,
		current:    current,
		newKeys:    newKeys,
		rerun:      sortedKeySet(effective),
		carried:    carried,
		coversFull: len(carried) == 0,
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

// derivedPacketPlan derives the delta/repair packet set from the baseline
// cache's per-check evidence. Where the derivation cannot trust its
// association it degrades conservatively to the full packet set and reports
// the degradation ("the plan covers the full scope").
func derivedPacketPlan(repoRoot string, run *Run) ([]PacketSpec, []string, []string, error) {
	derivation, err := deriveDeltaRerun(repoRoot, run)
	if err != nil {
		return nil, nil, nil, err
	}
	if derivation.degraded {
		packets, err := fullPacketPlan(repoRoot, run)
		if err != nil {
			return nil, nil, nil, err
		}
		return packets, nil, derivation.notices, nil
	}
	packets, err := rerunPacketPlan(repoRoot, run, derivation)
	if err != nil {
		return nil, nil, nil, err
	}
	return packets, derivation.carried, derivation.notices, nil
}

// rerunPacketPlan builds the packet set for a derived re-run scope. Carried
// judgments do not get a packet; a cross-only re-run plans the single cross
// packet, which consumes carried judgments and publishes their statuses.
func rerunPacketPlan(repoRoot string, run *Run, derivation *scopeDerivation) ([]PacketSpec, error) {
	rerun := map[string]bool{}
	for _, key := range derivation.rerun {
		rerun[key] = true
	}
	switch run.Gate {
	case GateValidate:
		if run.TargetKind == TargetKindRule {
			// The rule validate packet owns the derived re-run checks; the
			// rule file is a contract file declared whole.
			var rerunChecks []string
			for _, key := range derivation.rerun {
				if key != CrossKey {
					rerunChecks = append(rerunChecks, key)
				}
			}
			return []PacketSpec{{
				PacketID:  "checks",
				Kind:      PacketKindChecks,
				CheckKeys: rerunChecks,
				ReadRefs:  appendUnique(allRefNames(run), extraInputPaths(run)...),
			}}, nil
		}
		selected := map[string]bool{}
		for _, key := range derivation.rerun {
			if key == CrossKey {
				continue
			}
			group, ok := validatePacketOwner(key)
			if !ok {
				return nil, fmt.Errorf("internal: re-run check %q has no unit validate packet", key)
			}
			selected[group] = true
		}
		return validatePacketsFor(repoRoot, selected, run), nil
	case GateVerify:
		read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
		read = append(read, surfacePaths(run)...)
		read = appendUnique(read, extraInputPaths(run)...)
		var packets []PacketSpec
		for _, item := range derivation.current {
			if !rerun[item] {
				continue
			}
			detectID := detectionPacketID(item)
			packets = append(packets, PacketSpec{
				PacketID:  detectID,
				Kind:      PacketKindItem,
				CheckKeys: []string{item},
				ReadRefs:  append([]string(nil), read...),
			})
			packets = append(packets, PacketSpec{
				PacketID:  analysisPacketID(item),
				Kind:      PacketKindAnalysis,
				CheckKeys: []string{item},
				DependsOn: []string{detectID},
				ReadRefs:  append([]string(nil), read...),
			})
		}
		return append(packets, crossPacket(run, packetIDs(packets))), nil
	case GateReview:
		var packets []PacketSpec
		for _, file := range derivation.current {
			if !rerun[file] {
				continue
			}
			read := append([]string(nil), ownSpecPaths(repoRoot, run)...)
			read = append(read, file)
			read = appendUnique(read, extraInputPaths(run)...)
			packets = append(packets, PacketSpec{
				PacketID:  file,
				Kind:      PacketKindFile,
				CheckKeys: []string{file},
				ReadRefs:  read,
			})
		}
		return append(packets, crossPacket(run, packetIDs(packets))), nil
	}
	return nil, fmt.Errorf("unsupported gate %q for %s target", run.Gate, run.TargetKind)
}

// currentGateKeys lists the current non-cross judgment keys for the run's
// gate and target: the fixed check numbers (validate and rule targets),
// acceptance item ids (verify), or reviewed code files (review).
func currentGateKeys(repoRoot string, run *Run) ([]string, error) {
	switch {
	case run.TargetKind == TargetKindRule:
		return append([]string(nil), ruleValidateChecks...), nil
	case run.Gate == GateValidate:
		var keys []string
		for _, group := range validateCheckGroups {
			keys = append(keys, group.Checks...)
		}
		return keys, nil
	case run.Gate == GateVerify:
		return verifyItems(repoRoot, run)
	case run.Gate == GateReview:
		return reviewFiles(run), nil
	}
	return nil, fmt.Errorf("unsupported gate %q for %s target", run.Gate, run.TargetKind)
}

// groupChecks returns one validate packet group's check keys.
func groupChecks(packetID string) []string {
	for _, group := range validateCheckGroups {
		if group.PacketID == packetID {
			return group.Checks
		}
	}
	return nil
}

func runSurfaceLabel(run *Run) string {
	if run.Gate == GateReview {
		return "review"
	}
	return "spec"
}

func sortedKeySet(keys map[string]bool) []string {
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// validatePacketsFor builds the unit validate packets for the selected
// groups, in the fixed group order, plus the cross packet.
func validatePacketsFor(repoRoot string, selected map[string]bool, run *Run) []PacketSpec {
	var packets []PacketSpec
	for _, group := range validateCheckGroups {
		if !selected[group.PacketID] {
			continue
		}
		packets = append(packets, PacketSpec{
			PacketID:  group.PacketID,
			Kind:      PacketKindChecks,
			CheckKeys: append([]string(nil), group.Checks...),
			ReadRefs:  unitValidatePacketReadRefs(repoRoot, run, group.PacketID),
		})
	}
	packets = append(packets, crossPacket(run, packetIDs(packets)))
	return packets
}

// unitValidatePacketReadRefs derives the exact packet-local read surface for
// one unit validate packet. Check 1 (structural) must resolve unit_refs and
// rule_refs to verify that they exist, while Checks 7-8 (dependencies) read
// those same logical objects for cross-unit and constraint judgments. The
// design and acceptance packets stay limited to the unit's own truth and
// shared evidence inputs; they do not receive unrelated logical objects.
func unitValidatePacketReadRefs(repoRoot string, run *Run, packetID string) []string {
	read := ownSpecPaths(repoRoot, run)
	read = appendUnique(read, extraInputPaths(run)...)
	read = appendUnique(read, affectsEvidencePaths(run)...)
	if packetID == "structural" || packetID == "dependencies" {
		read = appendUnique(read, logicalRefNames(run)...)
	}
	return append([]string(nil), read...)
}

// rerunPlanNotice renders the plan's re-run/carry disclosure. The carried
// clause states the trust basis the delta run relies on: carried judgments
// keep their dependency evidence unchanged (see
// framework/verification_scope.md §Delta Runs).
func rerunPlanNotice(derivation *scopeDerivation) string {
	msg := "re-run checks " + strings.Join(derivation.rerun, ", ")
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
// validate packets may read and declare them, but they never create work
// packets.
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
	if run.TargetKind == TargetKindRule {
		if run.Target == TargetCandidate {
			return []string{specpaths.RuleCandidateFileRef(run.TargetName)}
		}
		return []string{specpaths.RuleStableFileRef(run.TargetName)}
	}
	var main string
	if run.Target == TargetCandidate {
		main = specpaths.CandidateUnitSpecFileRef(run.TargetName)
	} else {
		main = specpaths.StableUnitSpecFileRef(run.TargetName)
	}
	paths := []string{main}
	paths = append(paths, unitAppendices(repoRoot, run.TargetName, run.Target)...)
	return paths
}

// verifyItems lists the current spec's acceptance item ids in document order.
func verifyItems(repoRoot string, run *Run) ([]string, error) {
	var main string
	if run.Target == TargetCandidate {
		main = specpaths.CandidateUnitSpecFileRef(run.TargetName)
	} else {
		main = specpaths.StableUnitSpecFileRef(run.TargetName)
	}
	content, err := readSpecContent(repoRoot, main)
	if err != nil {
		return nil, err
	}
	return specvalidation.ExtractAcceptanceItemIDs(content), nil
}

// reviewFiles lists the review surface's code files (expanded surface entries,
// deduplicated, sorted).
func reviewFiles(run *Run) []string {
	seen := map[string]bool{}
	var files []string
	for _, surface := range run.Surfaces {
		if surface.Source != SourceDerived {
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

// surfacePaths lists every code file in the run's surfaces.
func surfacePaths(run *Run) []string {
	seen := map[string]bool{}
	var files []string
	for _, surface := range run.Surfaces {
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

// allRefNames lists every snapshot ref spelling.
func allRefNames(run *Run) []string {
	var out []string
	for _, ref := range run.Refs {
		out = append(out, ref.Ref)
	}
	return out
}

// extraInputPaths lists every agent-declared evidence input. These paths are
// readable by packets but never define the packet set.
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
	// SourceInput remains the compatibility representation for input refs and
	// surfaces that are not already part of the derived surface. ExtraInputs
	// above is authoritative when an explicit input overlaps a derived entry.
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

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
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
// synthesis. The input snapshot must be unchanged since gate-plan: on a full
// run, content drift (a recorded input whose bytes, layer resolution, or
// existence changed mid-run) is salvaged — the result is persisted with the
// coverage keys whose read surface intersects the drift recorded as
// invalidated, and the gate's recovery plan re-runs them; a structural divergence (the surface
// shape itself changed) still discards the run. Cache evidence is assembled
// from the accepted declarations, while result, blocking, severity counts, and
// statuses are derived from the accepted synthesis artifact.
// See framework/verification_scope.md §Coverage model and
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

	// A run that requires a final synthesis must carry one; a clean run
	// finalizes without it. The final synthesis is the optional `cross`
	// session.
	cross := crossReport(reports)
	hasFindings := runHasInputFindings(run, reports)
	if requiresFinalSynthesis(run, hasFindings, runHasBlockingFindings(run, reports)) && cross == nil {
		return fmt.Errorf("gate-finalize rejected: the run requires a final synthesis but none is accepted — generate it with `specflowctl gate-mission --run %s --final --format prompt`, then submit it with `specflowctl gate-submit --run %s --session cross --keys cross --report PATH`", run.RunID, run.RunID)
	}

	// Snapshot check — the closure of the time-of-check/time-of-use gap. On
	// a full run, content drift (a recorded input whose bytes moved) does not
	// discard the run: the affected coverage keys are computed mechanically
	// from the plan-time read sets, and the salvaged cache records them as
	// invalidated. A structural divergence (the surface shape itself changed)
	// and any drift on a delta/repair run discard the run: a delta/repair
	// run's carried keys have no session coverage to re-attribute, so the
	// salvage's re-run set could not be covered by the planned sessions.
	var salvage *runSalvage
	drift, err := gaterun.CompareDetailed(absRoot, run)
	if err != nil {
		return err
	}
	if len(drift.Divergences) > 0 {
		if run.Mode != gaterun.ModeFull || drift.Structural {
			return rejectSnapshotDivergence(absRoot, run, drift.Divergences)
		}
		salvage = newRunSalvage(drift.DriftedSet())
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
		if salvage == nil {
			return rejectSnapshotDivergence(absRoot, run, evidenceDivergences)
		}
		// Salvage: an entry assembled from drifted bytes is the expected
		// divergence — the drift joins the salvage set and the entry is
		// re-bound to its plan-time hash below. Any other class (a path the
		// snapshot never recorded) is structural and discards the run.
		var structural []string
		for _, divergence := range evidenceDivergences {
			if path, ok := evidenceDriftPath(divergence); ok {
				salvage.drift[gaterun.CanonicalDeclPath(absRoot, path)] = true
				continue
			}
			structural = append(structural, divergence)
		}
		if len(structural) > 0 {
			return rejectSnapshotDivergence(absRoot, run, structural)
		}
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
	schema := validationcache.RecordStateSchemaValidate
	if run.Gate == gaterun.GateVerify {
		schema = validationcache.RecordStateSchemaVerify
		// A salvaged run publishes no judgment record for a coverage key the
		// drift invalidated: the record would pin plan-time evidence the key
		// can no longer prove it read cleanly. An unbound key makes the
		// recovery plan force its re-run.
		if err := publishUnitJudgments(absRoot, run, reports, outcome, salvage.affectedCoverageKeys(absRoot, run)); err != nil {
			return err
		}
	}
	// The synthesis's supplied-evidence pins are only load-bearing when the
	// final synthesis is a real conclusion of this run: an accepted cross
	// session produced relationship verdicts, dispositions, findings or a
	// final quality assessment. Without one, the supplied evidence is
	// background reading and pinning it would make a change un-actionable.
	// Only verify reuse is conclusion-bound; a validate cache is judged by its
	// whole-file list, so it writes no synthesis pins.
	var synthesisPins []gaterun.EvidencePin
	if cross != nil && run.Gate == gaterun.GateVerify {
		synthesisPins = gaterun.SynthesisEvidencePins(absRoot, run, cross.text)
	}
	judgmentData, err := json.Marshal(gaterun.JudgmentBaseline{
		SchemaVersion:     schema,
		Records:           run.Records,
		Relationships:     run.AllRelationships(),
		LogicalStatus:     outcome.EffectiveStatus,
		Findings:          outcome.Findings,
		SynthesisDigest:   outcome.SynthesisDigest,
		SynthesisEvidence: synthesisPins,
		DeferredFindings:  outcome.DeferredFindings,
	})
	if err != nil {
		return fmt.Errorf("encode judgment baseline: %w", err)
	}

	drifted := map[string]bool{}
	if salvage != nil {
		drifted = salvage.drift
	}
	promiseState, promiseStatePath, err := preparePromiseState(absRoot, run, result, drifted)
	if err != nil {
		return err
	}
	pinFormat := ""
	pinsSet := false
	var promisePins []validationcache.PromisePin
	if run.TargetKind == gaterun.TargetKindUnit {
		pinFormat = validationcache.PromisePinFormat
		if run.Gate == gaterun.GateValidate && result == "pass" {
			promisePins, err = prepareValidatePromisePins(absRoot, run, drifted)
			if err != nil {
				return err
			}
			pinsSet = true
		}
	}

	write := validationcache.CacheWrite{
		Command:        run.Gate,
		Unit:           run.TargetName,
		Mode:           "full",
		Basis:          run.Mode,
		Result:         result,
		Target:         run.Target,
		Blocking:       result == "fail",
		P0Count:        counts[0],
		P1Count:        counts[1],
		P2Count:        counts[2],
		P3Count:        counts[3],
		Timestamp:      now,
		GateRun:        run.RunID,
		Judgments:      string(judgmentData),
		Body:           body.String(),
		PinFormat:      pinFormat,
		PromisePins:    promisePins,
		PromisePinsSet: pinsSet,
		Entries:        entries,
	}
	// A validate conclusion has no judgment record to bind its judging
	// standard, so the cache itself records it. Verify caches bind it per
	// record instead (see framework/validation_cache.md §Format).
	if run.Gate == gaterun.GateValidate {
		write.Standard = run.Protocol
	}

	// Re-resolve the complete input surface after evidence assembly and
	// before rendering. A change after this comparison can only make the
	// published cache stale — never mix an old judgment with new bytes —
	// because every entry is bound to its plan-time snapshot hash (on a
	// salvaged run, explicitly re-bound below).
	drift, err = gaterun.CompareDetailed(absRoot, run)
	if err != nil {
		return err
	}
	if len(drift.Divergences) > 0 {
		if salvage == nil || drift.Structural {
			return rejectSnapshotDivergence(absRoot, run, drift.Divergences)
		}
		for decl := range drift.DriftedSet() {
			salvage.drift[decl] = true
		}
	}
	if salvage != nil {
		// The drifted entries re-bind to the plan-time snapshot: the
		// judgments were made against those bytes, so the cache records
		// exactly what was judged, and the whole-file freshness comparison
		// reports the drifted files stale until repair rewrites the evidence.
		if err := salvage.bind(absRoot, run, entries); err != nil {
			return err
		}
		write.InvalidatedChecks = salvage.invalidatedChecks(absRoot, run)
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
	if salvage != nil && check.Category == validationcache.CategoryStale {
		// A salvaged write is stale by construction — plan-time hashes on the
		// drifted entries plus the declared pending invalidations. The
		// self-check proves the candidate parses and is classified by the
		// gate's own chain; it is a recovery baseline, not a promotable pass.
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

	writtenPath, err := validationcache.PublishCache(absRoot, run.TargetKind, run.TargetName, run.Gate, rendered)
	if err != nil {
		return err
	}
	if promiseState != nil {
		if err := validationcache.WritePromiseStateFile(promiseStatePath, *promiseState); err != nil {
			return fmt.Errorf("%v — the cache at %s was written and is valid, but the promise state could not be recorded; re-run gate-finalize --run %s", err, relToRepo(absRoot, writtenPath), run.RunID)
		}
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
	if salvage != nil {
		fmt.Fprintf(stdout, "Input drift since gate-plan: %s\n", salvage.noticeList())
		fmt.Fprintf(stdout, "Salvaged: result persisted; invalidated check(s): %s\n", strings.Join(write.InvalidatedChecks, ", "))
		if run.Gate == gaterun.GateValidate {
			fmt.Fprintf(stdout, "Recover with: %s\n", gaterun.PlanCommand(run, gaterun.ModeFull))
		} else {
			fmt.Fprintf(stdout, "Recover with: %s\n", gaterun.PlanCommand(run, gaterun.ModeRepair))
		}
	}
	switch check.Category {
	case validationcache.CategoryStale:
		fmt.Fprintf(stdout, "Self-check: STALE (result: %s — a recovery baseline, not promotable until the invalidated checks re-run)\n", result)
	case validationcache.CategoryBlocked:
		fmt.Fprintf(stdout, "Self-check: BLOCKED (result: fail — the failure record blocks promote and is the failure-recovery baseline)\n")
	default:
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

// salvagedFingerprintMark prefixes a promise fingerprint that could not be
// computed from the bytes the run read: the file drifted mid-run, so the
// plan-time contract is unrecoverable from the snapshot. The value never
// equals a real fingerprint, so every downstream comparison treats the
// contract as changed and the next run recomputes it.
const salvagedFingerprintMark = "salvaged:"

func salvagedFingerprint(snapshotHash string) string {
	return salvagedFingerprintMark + snapshotHash
}

// salvagedPin returns the pin for a provider whose spec drifted mid-run. The
// run read the provider spec's plan-time bytes; the current promise
// fingerprints are computed from post-drift content, so recording them would
// claim a contract the run never read. The pin carries the plan-time
// whole-file hash under the salvage mark instead.
func salvagedPin(absRoot string, run *gaterun.Run, drifted map[string]bool, dep, provider string) (validationcache.PromisePin, bool, error) {
	want := "unit:" + provider
	if run.Target == gaterun.TargetStable {
		want = specpaths.StableUnitSpecFileRef(provider)
	}
	if !driftedDecl(absRoot, drifted, want) {
		return validationcache.PromisePin{}, false, nil
	}
	hash, ok := run.SnapshotHash(absRoot, want)
	if !ok || hash == "" {
		return validationcache.PromisePin{}, false, fmt.Errorf("salvage: drifted provider ref %s has no plan-time snapshot hash", want)
	}
	return validationcache.PromisePin{ID: dep, Fingerprint: salvagedFingerprint(hash)}, true, nil
}

// driftedDecl reports whether a declaration is in the run's drift set.
func driftedDecl(absRoot string, drifted map[string]bool, declared string) bool {
	if len(drifted) == 0 {
		return false
	}
	return drifted[gaterun.CanonicalDeclPath(absRoot, declared)]
}

// preparePromiseState builds the promise state record for a passing unit
// verify run. It returns nil for non-verify runs, rules, failed runs, and
// units whose spec has no `## Promises` section. The record is written only
// after the gate cache is published, so a build failure cannot leave a valid
// cache with a missing state record.
//
// The record is layer-scoped: a candidate verifies against the current-layer
// provider promises it read, a stable confirmation verifies against the
// published stable provider promises. Provider fingerprints and the input
// surface check use the same layer, so a recorded pin is always a contract the
// run actually read — a drifted provider records the salvage mark instead of a
// post-drift fingerprint.
func preparePromiseState(absRoot string, run *gaterun.Run, result string, drifted map[string]bool) (*validationcache.PromiseStateFile, string, error) {
	if run.Gate != gaterun.GateVerify || run.TargetKind != gaterun.TargetKindUnit || result != "pass" {
		return nil, "", nil
	}
	relPath := specpaths.CandidateUnitSpecFileRef(run.TargetName)
	if run.Target == gaterun.TargetStable {
		relPath = specpaths.StableUnitSpecFileRef(run.TargetName)
	}
	data, err := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, "", fmt.Errorf("read promise spec %s: %w", relPath, err)
	}
	content := string(data)
	promises, parseIssues := specvalidation.ParsePromises(content)
	if len(parseIssues) > 0 {
		if strings.Contains(strings.Join(parseIssues, "; "), "section not found") {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("promise state: %s", strings.Join(parseIssues, "; "))
	}
	for _, check := range specvalidation.CheckPromiseSpec(content, run.TargetName) {
		if check.Status == specvalidation.Fail {
			return nil, "", fmt.Errorf("promise state: %s failed: %s", check.Name, check.Details)
		}
	}
	scope := specvalidation.PromiseScopeForLayer(run.Target)
	graph := specvalidation.CheckPromiseGraphScope(absRoot, scope, run.TargetName)
	if graph.Status != specvalidation.Pass {
		return nil, "", fmt.Errorf("promise state: graph failed: %s", graph.Details)
	}
	current, currentIssues := specvalidation.PromiseFingerprintsForScope(absRoot, scope)
	if len(currentIssues) > 0 {
		return nil, "", fmt.Errorf("promise state: %s", strings.Join(currentIssues, "; "))
	}

	own := map[string]string{}
	// A drifted own spec means every own-promise fingerprint would be
	// computed from post-drift content — content the run never judged. The
	// salvage mark records that the contract identity is unrecoverable.
	ownDrifted := driftedDecl(absRoot, drifted, relPath)
	ownSalvageMark := ""
	if ownDrifted {
		hash, ok := run.SnapshotHash(absRoot, relPath)
		if !ok || hash == "" {
			return nil, "", fmt.Errorf("promise state: drifted own spec %s has no plan-time snapshot hash", relPath)
		}
		ownSalvageMark = salvagedFingerprint(hash)
	}
	for _, promise := range promises {
		if ownDrifted {
			own[promise.ID] = ownSalvageMark
			continue
		}
		fingerprint, err := specvalidation.PromiseFingerprint(promise)
		if err != nil {
			return nil, "", err
		}
		own[promise.ID] = fingerprint
	}
	state := &validationcache.PromiseStateFile{Unit: run.TargetName, Layer: run.Target}
	for _, id := range sortedKeys(own) {
		state.Promises = append(state.Promises, validationcache.PromisePin{ID: id, Fingerprint: own[id]})
	}
	normalized := specpaths.NormalizeText(content)
	for _, promise := range promises {
		for _, anchor := range promise.Source {
			cid, ok := contenthash.NarrativeSourceCID(normalized, anchor)
			if !ok {
				return nil, "", fmt.Errorf("promise state: source %q of promise %q does not resolve to a Design Narrative block", anchor, promise.ID)
			}
			if ownDrifted {
				// The run judged the plan-time content, not the post-drift
				// block. Record the same unrecoverable marker used for a
				// drifted own promise fingerprint.
				cid = salvagedFingerprint(cid)
			}
			state.SourcePins = append(state.SourcePins, validationcache.PromiseSourcePin{ID: promise.ID, Anchor: anchor, CID: cid})
		}
	}
	sort.Slice(state.SourcePins, func(i, j int) bool {
		if state.SourcePins[i].ID != state.SourcePins[j].ID {
			return state.SourcePins[i].ID < state.SourcePins[j].ID
		}
		return state.SourcePins[i].Anchor < state.SourcePins[j].Anchor
	})
	pins := map[string]bool{}
	for _, promise := range promises {
		for _, dep := range promise.DependsOn {
			if pins[dep] {
				continue
			}
			provider := specvalidation.PromiseProvider(dep)
			if !runHasProviderRef(run, provider) {
				return nil, "", fmt.Errorf("promise state: dependency %q provider %q is not in the verify input surface — re-plan the verify run so it reads the dependency", dep, provider)
			}
			if pin, salvaged, err := salvagedPin(absRoot, run, drifted, dep, provider); err != nil {
				return nil, "", err
			} else if salvaged {
				state.Pins = append(state.Pins, pin)
				pins[dep] = true
				continue
			}
			fingerprint, ok := current[dep]
			if !ok {
				return nil, "", fmt.Errorf("promise state: dependency %q is missing from the %s layer", dep, run.Target)
			}
			state.Pins = append(state.Pins, validationcache.PromisePin{ID: dep, Fingerprint: fingerprint})
			pins[dep] = true
		}
	}
	sort.Slice(state.Pins, func(i, j int) bool { return state.Pins[i].ID < state.Pins[j].ID })
	path, err := validationcache.PromiseStatePath(absRoot, run.TargetName, run.Target)
	if err != nil {
		return nil, "", err
	}
	return state, path, nil
}

// prepareValidatePromisePins records the provider promise contracts a passing
// unit validate run read. The pin set is the validate cache's contract
// snapshot: freshness compares these fingerprints instead of the provider
// spec's whole-file hash, so a provider narrative or acceptance-item edit no
// longer stales the consumer. A provider that is not in the run's input
// surface cannot be pinned: the run did not read it. A drifted provider
// records the salvage mark instead of a post-drift fingerprint.
func prepareValidatePromisePins(absRoot string, run *gaterun.Run, drifted map[string]bool) ([]validationcache.PromisePin, error) {
	relPath := specpaths.CandidateUnitSpecFileRef(run.TargetName)
	if run.Target == gaterun.TargetStable {
		relPath = specpaths.StableUnitSpecFileRef(run.TargetName)
	}
	data, err := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, fmt.Errorf("read promise spec %s: %w", relPath, err)
	}
	ids := specvalidation.PromiseDependencyIDs(string(data))
	if len(ids) == 0 {
		return []validationcache.PromisePin{}, nil
	}
	scope := specvalidation.PromiseScopeForLayer(run.Target)
	current, issues := specvalidation.PromiseFingerprintsForScope(absRoot, scope)
	if len(issues) > 0 {
		return nil, fmt.Errorf("promise pins: %s", strings.Join(issues, "; "))
	}
	var pins []validationcache.PromisePin
	for _, id := range ids {
		provider := specvalidation.PromiseProvider(id)
		if !runHasProviderRef(run, provider) {
			return nil, fmt.Errorf("promise pins: dependency %q provider %q is not in the validate input surface — re-plan the validate run so it reads the dependency", id, provider)
		}
		if pin, salvaged, err := salvagedPin(absRoot, run, drifted, id, provider); err != nil {
			return nil, err
		} else if salvaged {
			pins = append(pins, pin)
			continue
		}
		fingerprint, ok := current[id]
		if !ok {
			return nil, fmt.Errorf("promise pins: dependency %q is unresolved in the %s layer", id, run.Target)
		}
		pins = append(pins, validationcache.PromisePin{ID: id, Fingerprint: fingerprint})
	}
	return pins, nil
}

// runHasProviderRef reports whether the verify run's input surface includes the
// provider unit's main spec in the run's own layer. A dependency pin is
// recorded only when the verify sessions actually read the promise it pins, so
// the provider must be part of the run: a candidate run reads the current-layer
// provider logical ref, a stable run reads the published stable spec.
func runHasProviderRef(run *gaterun.Run, unit string) bool {
	want := "unit:" + unit
	if run.Target == gaterun.TargetStable {
		want = specpaths.StableUnitSpecFileRef(unit)
	}
	for _, ref := range run.Refs {
		if ref.Ref == want {
			return true
		}
	}
	return false
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

// runHasBlockingFindings reports whether the run carries a blocking P0/P1
// finding (or a pending deferral, which the synthesis must dispose). A
// validate run's non-blocking P2/P3 advisories do not require a final
// synthesis (framework/severity_policy.md §11).
func runHasBlockingFindings(run *gaterun.Run, reports []reportRef) bool {
	blocking := func(findings []gaterun.Finding) bool {
		for _, finding := range findings {
			if finding.Severity == "P0" || finding.Severity == "P1" {
				return true
			}
		}
		return false
	}
	for _, rep := range reports {
		if rep.spec.Kind == gaterun.SessionKindCross {
			continue
		}
		if blocking(resultFindings(rep.result)) {
			return true
		}
	}
	for i := range run.CarriedResults {
		if blocking(resultFindings(&run.CarriedResults[i])) {
			return true
		}
	}
	return len(run.DeferredFindings) > 0
}

// requiresFinalSynthesis reports whether the run must carry an accepted final
// synthesis before it can finalize. Any assigned relationship requires one.
// Beyond that, a validate run (full or delta) requires one for
// blocking P0/P1 findings — its P2/P3 findings are non-blocking advisories
// presented directly (framework/severity_policy.md §11), and the synthesis is
// also where Check 2 Step 1 / Check 5 extraction artifacts are re-verified
// (framework/unit_validate_checklist.md §Extraction re-verification). Verify
// requires one for any finding. Rule validate never synthesizes.
func requiresFinalSynthesis(run *gaterun.Run, hasAnyFindings, hasBlockingFindings bool) bool {
	if run.TargetKind == gaterun.TargetKindRule {
		return false
	}
	if len(run.Relationships) > 0 {
		return true
	}
	if run.Gate == gaterun.GateValidate {
		return hasBlockingFindings
	}
	return hasAnyFindings
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
		// final synthesis is required. Every status is derived from the
		// sessions' verdicts plus the carried baseline statuses, and the
		// mechanical synthesis digest covers the derived status map.
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
		if requiresFinalSynthesis(run, runHasInputFindings(run, reports), runHasBlockingFindings(run, reports)) {
			return nil, errors.New("gate-finalize rejected: the run has findings but no accepted final synthesis")
		}
		findings := []gaterun.Finding{}
		if run.Gate == gaterun.GateValidate {
			// A validate run finalizes without a final synthesis when it has no
			// assigned relationships and no blocking finding: its P2/P3
			// advisories are the accepted sessions' own findings, retained
			// verbatim. The findings feed the failure record, the advisory
			// carry, and the counts.
			for _, rep := range reports {
				if rep.spec.Kind == gaterun.SessionKindCross {
					continue
				}
				findings = append(findings, resultFindings(rep.result)...)
			}
			for i := range run.CarriedResults {
				findings = append(findings, resultFindings(&run.CarriedResults[i])...)
			}
			out.Findings = findings
		}
		digestState, err := json.Marshal(struct {
			LogicalStatus map[string]string `json:"logical_status"`
			Findings      []gaterun.Finding `json:"findings"`
		}{LogicalStatus: out.EffectiveStatus, Findings: findings})
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
	if run.Gate == gaterun.GateValidate && run.TargetKind == gaterun.TargetKindRule && out.Counts[2]+out.Counts[3] > 0 {
		return nil, errors.New("rule validate grades findings P0/P1 only; P2/P3 adjudication belongs to unit validate")
	}
	// P2/P3 findings are non-blocking advisories for unit validate
	// (framework/severity_policy.md §11): they are counted and recorded, and
	// they never drive Result or blocking.
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
	return fmt.Errorf("gate-finalize rejected: input(s) changed since gate-plan —\n  %s\nRun state discarded; plan a new run and execute again: %s",
		strings.Join(divergences, "\n  "), gaterun.PlanCommand(run, run.Mode))
}

// runSalvage carries a full run's mid-run content drift through finalize: the
// run is persisted (its verdicts dictate a pass cache or a failure record)
// with the coverage keys whose plan-time read set intersects the drift
// recorded as invalidated. The intersection is the framework's own trust
// model — a verdict can only be influenced by material its session read, and
// the read set is fixed at plan time and enforced at gate-submit — so the
// attribution needs no judgment, and a verdict whose read surface never
// contained a drifted file stays valid by construction.
type runSalvage struct {
	drift map[string]bool
}

func newRunSalvage(drifted map[string]bool) *runSalvage {
	return &runSalvage{drift: drifted}
}

// noticeList renders the drifted declarations for the finalize output.
func (s *runSalvage) noticeList() string {
	decls := make([]string, 0, len(s.drift))
	for decl := range s.drift {
		decls = append(decls, decl)
	}
	sort.Strings(decls)
	return strings.Join(decls, ", ")
}

// affectedCoverageKeys returns the coverage keys whose read surface
// intersects the drifted declarations — the judgments the drift can have
// influenced. Nil-safe: a non-salvaged run affects nothing.
func (s *runSalvage) affectedCoverageKeys(absRoot string, run *gaterun.Run) map[string]bool {
	affected := map[string]bool{}
	if s == nil {
		return affected
	}
	for _, ck := range run.Coverage {
		for _, ref := range gaterun.CoverageReadRefs(absRoot, run, ck) {
			if s.drift[gaterun.CanonicalDeclPath(absRoot, ref)] {
				affected[ck.Key] = true
				break
			}
		}
	}
	return affected
}

// invalidatedChecks renders the salvage's machine-owned re-run set: the
// report keys of every affected coverage key. The recovery plan forces
// exactly this list (mapped back onto coverage keys) and carries the rest.
func (s *runSalvage) invalidatedChecks(absRoot string, run *gaterun.Run) []string {
	var keys []string
	for key := range s.affectedCoverageKeys(absRoot, run) {
		ck := run.CoverageByKey(key)
		if ck == nil {
			continue
		}
		keys = append(keys, run.ReportKeys(*ck)...)
	}
	sort.Strings(keys)
	deduped := keys[:0]
	for i, key := range keys {
		if i > 0 && keys[i-1] == key {
			continue
		}
		deduped = append(deduped, key)
	}
	return deduped
}

// bind re-binds every drifted evidence entry to its plan-time snapshot hash.
// The judgments were made against the snapshot bytes, so the cache records
// exactly what was judged; the whole-file freshness comparison then reports
// the drifted files stale until repair rewrites the evidence. The chunk
// sequence is recorded empty (a valid recorded block): the plan-time chunking
// is gone with the bytes, so the change cannot be localized — the next change
// report shows the file as changed without a line-level diff.
func (s *runSalvage) bind(absRoot string, run *gaterun.Run, entries []validationcache.FileEntry) error {
	for i := range entries {
		if !s.drift[gaterun.CanonicalDeclPath(absRoot, entries[i].Path)] {
			continue
		}
		hash, ok := run.SnapshotHash(absRoot, entries[i].Path)
		if !ok || hash == "" {
			return fmt.Errorf("salvage: drifted evidence %s has no plan-time snapshot hash", entries[i].Path)
		}
		entries[i].Hash = hash
		entries[i].Chunks = nil
	}
	return nil
}

// evidenceDriftPath reports the declared path of an evidence divergence that
// re-binds to its plan-time snapshot hash (the entry was assembled from
// drifted bytes); ok is false for every other divergence class.
func evidenceDriftPath(divergence string) (string, bool) {
	if rest, found := strings.CutPrefix(divergence, "evidence differs from snapshot: "); found {
		return rest, true
	}
	return "", false
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
			// The declaration's file is gone (a mid-run removal). The cache's
			// files list is the plan-time input surface, so the entry keeps
			// its plan-time identity with an empty chunk sequence: the change
			// cannot be localized, and the salvage invalidations force every
			// judgment that read the file to re-run. A declaration outside the
			// snapshot still fails closed.
			hash, ok := run.SnapshotHash(absRoot, ref)
			if !ok || hash == "" {
				return nil, fmt.Errorf("no evidence record for %q — the path is not in the plan-time input surface", ref)
			}
			entry = &validationcache.FileEntry{Path: ref, Hash: hash, Chunker: contenthash.ChunkerVersion}
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
	fmt.Fprintln(w, "accepted session (plus carried-over baseline evidence for partial runs), and the")
	fmt.Fprintln(w, "input snapshot must still verify against gate-plan. On a full run, content drift")
	fmt.Fprintln(w, "(a recorded input whose bytes changed mid-run) is salvaged: the result is")
	fmt.Fprintln(w, "persisted with the coverage keys whose read set intersects the drift recorded as")
	fmt.Fprintln(w, "invalidated (a recovery baseline; a unit validate recovers with a full-scope")
	fmt.Fprintln(w, "`revalidate`, a rule validate with the full confirmation run, and verify with")
	fmt.Fprintln(w, "`reverify --mode repair`). A structural divergence")
	fmt.Fprintln(w, "(the input surface itself changed) discards the run. A run that produced findings")
	fmt.Fprintln(w, "must carry an accepted final synthesis (the optional")
	fmt.Fprintln(w, "`cross` session); a clean run finalizes directly. The tooling assembles one")
	fmt.Fprintln(w, "files entry per run input-surface file (whole-file hash + ordered chunk")
	fmt.Fprintln(w, "sequence), attaches each check's lens and failure status, derives the")
	fmt.Fprintln(w, "failure-record status map mechanically, then re-reads and runs the gate's own")
	fmt.Fprintln(w, "freshness chain (pass → FRESH, failure record → BLOCKED, salvage → STALE;")
	fmt.Fprintln(w, "validate@ additionally checks appendix coverage). Result, blocking, severity")
	fmt.Fprintln(w, "counts, and failure statuses are derived from accepted artifacts.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --run RUN_ID     gate run id printed by gate-plan (required)")
	fmt.Fprintln(w, "  --timestamp T    run timestamp RFC3339 (default: now UTC)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

func publishUnitJudgments(root string, run *gaterun.Run, reports []reportRef, outcome *gateOutcome, skip map[string]bool) error {
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
		if skip[ck.Key] {
			// A salvaged run publishes no judgment record for a key whose
			// evidence basis the drift invalidated: the record could only pin
			// plan-time evidence the key can no longer prove it read cleanly.
			// An unbound key forces the recovery plan to re-run it.
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
	if gaterun.IsAlignmentKind(ck.Kind) {
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

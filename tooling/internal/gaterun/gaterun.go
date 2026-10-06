// Package gaterun fixes the immutable input snapshot and the coverage set of a
// quality-gate run. A promote-consumable gate cache is written by the sequence
//
//	gate-plan → decide session batches → gate-submit (per session) → gate-finalize
//
// gate-plan resolves the run's input surface — the target's spec files, the
// dependency spec objects, and (for verify) the declared code surface
// — adds the agent-declared extra inputs, computes the coverage set (the
// judgment keys that must each receive exactly one verdict) for the run mode,
// and persists the run under meta/gate_runs/ with no sessions. The agent
// chooses how to batch coverage keys into reviewer sessions; gate-mission
// materializes a session's read-only mission, gate-submit validates and
// records each accepted session report mechanically, and gate-finalize
// re-resolves the surface and re-hashes the stored entries. Any divergence (a
// modified, added, or removed input, or a logical reference whose layer
// resolution moved) rejects the write, so the cache's evidence can only
// describe content that was stable for the whole judgment window (see
// framework/validation_cache.md §Write Rules → Tooled writes).
//
// Coverage closure is mechanical: gate-finalize refuses a run whose coverage
// set is not covered by exactly one accepted session per key (plus carried
// baseline judgments for delta/repair). This replaces the anti-skip guarantee
// the former fixed session plan provided.
//
// The run id correlates state only. It is not executor identity and proves
// nothing about the execution shape.
package gaterun

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

const (
	GateValidate = "validate"
	GateVerify   = "verify"

	TargetKindUnit = "unit"
	TargetKindRule = "rule"

	TargetCandidate = "candidate"
	TargetStable    = "stable"

	// Run modes: full generates every session; delta derives the re-run set
	// from a pass baseline's stale evidence; repair derives it from a
	// failure record's status map.
	ModeFull   = "full"
	ModeDelta  = "delta"
	ModeRepair = "repair"

	// StatusOpen marks a run whose sessions may still be submitted and
	// finalized; StatusConsumed marks a finalized run kept for audit;
	// StatusInvalidated marks an open plan contradicted by a later targeted
	// P0/P1, so it can no longer submit or finalize.
	StatusOpen        = "open"
	StatusConsumed    = "consumed"
	StatusInvalidated = "invalidated"

	// Session statuses. A session is pending (no submission yet), accepted
	// (its report validated and recorded — terminal), or rejected (its last
	// submission failed validation; it can be re-submitted).
	SessionPending     = "pending"
	SessionAccepted    = "accepted"
	SessionRejected    = "rejected"
	SessionNotRequired = "not_required"

	// Report kinds: a group of validate checks, one or more verify items, a
	// reviewed file, or the final cross synthesis.
	SessionKindChecks       = "checks"
	SessionKindItem         = "item"
	SessionKindDesign       = "design"
	SessionKindCode         = "code"
	SessionKindArchitecture = "architecture"
	SessionKindPreserve     = "preserve"
	SessionKindCross        = "cross"

	// Lens tags distinguish the two judgment stances the merged `verify` gate
	// carries: `alignment` treats the spec as authority (acceptance items) and
	// `quality` treats the spec as rationale (declared code files). A reviewer
	// session never mixes lenses.
	LensAlignment = "alignment"
	LensQuality   = "quality"

	// ClarityCheck is the unit validate check key of the clarity check
	// (Check 10): read the unit spec and report what is unclear,
	// underspecified, or internally contradictory.
	ClarityCheck = "10"

	// CrossKey is the reserved check key of the cross-check.
	CrossKey = "cross"

	// SourceDerived marks an entry resolved from the gate's protocol input
	// surface; SourceInput marks an entry declared by the agent through the
	// plan's input manifest.
	SourceDerived = "derived"
	SourceInput   = "input"
	// SourceDerivedAffects marks a spec-derived evidence file that exists
	// because the target spec declares it in an acceptance item's
	// affects.files. Validate's read surface includes these files, so local
	// validate sessions may read and declare them; they never define work
	// sessions.
	SourceDerivedAffects = "derived_affects"

	runStateDir     = "meta/gate_runs"
	timestampLayout = "2006-01-02T15:04:05Z"
	mutationLockDir = "meta"
	mutationLock    = ".gate_runs.lock"
	mutationTimeout = 30 * time.Second
)

// Ref is one snapshot entry resolved by name or by physical path: the logical
// spelling (or physical repo-relative path), the applicable file it resolves
// to ("" when a logical reference resolves to no file), and the resolved
// file's whole-file hash ("" when the file does not exist).
type Ref struct {
	Ref      string `json:"ref"`
	Resolved string `json:"resolved"`
	Hash     string `json:"hash"`
	Source   string `json:"source"`
}

// Entry is one file inside a code surface directory.
type Entry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// Surface is one declared code surface path (a file or a directory expanded
// to its repository-content files at snapshot time; the entries are those
// files).
type Surface struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
	Source  string  `json:"source"`
}

// CoverageKey is one unit of judgment in a gate run: the key that must receive
// exactly one verdict, tagged with its lens and the reviewer report kind it
// maps to. A verify item key maps to the item report kind; a reviewed file key
// to the file report kind; a validate group key (including clarity) to the
// checks report kind. The agent decides how to batch coverage keys into
// reviewer sessions; the planner only fixes the set.
type CoverageKey struct {
	Key      string   `json:"key"`
	Lens     string   `json:"lens,omitempty"`
	Kind     string   `json:"kind"`
	File     string   `json:"file,omitempty"`
	Unit     string   `json:"unit,omitempty"`
	Item     string   `json:"item,omitempty"`
	ReadRefs []string `json:"read_refs,omitempty"`
	Source   string   `json:"source,omitempty"`
	Task     string   `json:"task,omitempty"`
}

// SessionSpec is one materialized reviewer session spec: the immutable read
// surface and report contract for an agent-chosen key batch. It is rebuilt
// deterministically from the run's coverage set at mission and submit time;
// the session's mutable state (status, attempts, accepted report) lives in its
// own state file under the run directory.
type SessionSpec struct {
	SessionID     string   `json:"session_id"`
	Kind          string   `json:"kind"`
	CheckKeys     []string `json:"check_keys"`
	DependsOn     []string `json:"depends_on,omitempty"`
	ReadRefs      []string `json:"read_refs,omitempty"`
	Relationships []string `json:"relationships,omitempty"`
	// Context carries report-kind-specific plan-time facts a mission must
	// state verbatim.
	Context []string `json:"context,omitempty"`
}

// Finding is one mechanically identified finding. Local finding ids are
// assigned at submit time from the immutable session id and report order.
type Finding struct {
	ID           string   `json:"id"`
	Severity     string   `json:"severity"`
	Text         string   `json:"text"`
	Detail       string   `json:"detail"`
	SourceKey    string   `json:"source_key,omitempty"`
	AffectedKeys []string `json:"affected_keys,omitempty"`
	// OwnedBy is the unit whose behavior the finding belongs to, set by the
	// cross synthesis's ownership records. Empty means unassigned: the finding
	// drives this unit's gate. A finding owned by another unit is deferred —
	// recorded and routed to the owner's verify run, not blocking this run (see
	// framework/verification_scope.md §Coverage Model → Deferred findings).
	OwnedBy string `json:"owned_by,omitempty"`
}

// WithMinimumSeverity raises a validated P0-P3 grade without lowering it.
// The canonical detail follows the grade; the original reviewer report stays
// unchanged as evidence of the submitted assessment.
func (finding Finding) WithMinimumSeverity(minimum string) Finding {
	if minimum == "" || minimum >= finding.Severity {
		return finding
	}
	previous := finding.Severity
	finding.Severity = minimum
	lines := strings.Split(finding.Detail, "\n")
	lines[0] = strings.Replace(lines[0], "["+previous+"]", "["+minimum+"]", 1)
	for i, line := range lines {
		label, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(label, "Severity") && strings.EqualFold(strings.TrimSpace(value), previous) {
			lines[i] = line[:strings.IndexByte(line, ':')+1] + " " + minimum
		}
	}
	finding.Detail = strings.Join(lines, "\n")
	return finding
}

// FindingOwnership is the cross synthesis's evidence-backed ownership record
// for one terminal retained finding: the recorded ownership statement that
// routes a finding to another unit. A finding without a record is unassigned.
type FindingOwnership struct {
	FindingID    string `json:"finding_id"`
	OwnerUnit    string `json:"owner_unit"`
	EvidencePath string `json:"evidence_path"`
	Reason       string `json:"reason"`
}

// Scope is one dependency declaration parsed from a session report.
type Scope struct {
	Key         string `json:"key"`
	Path        string `json:"path"`
	Declaration string `json:"declaration"`
}

// FindingDisposition is the cross result's decision for one input finding.
type FindingDisposition struct {
	FindingID string `json:"finding_id"`
	Action    string `json:"action"` // retained | suppressed | merged
	TargetID  string `json:"target_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// SessionResult is the immutable machine-readable interpretation of an
// accepted session report. Later sessions (the final synthesis) and
// gate-finalize consume this structure, never a coordinator-supplied
// restatement of the result.
type SessionResult struct {
	SessionID               string               `json:"session_id"`
	Kind                    string               `json:"kind"`
	Verdicts                map[string]string    `json:"verdicts,omitempty"`
	Scopes                  []Scope              `json:"scopes,omitempty"`
	Findings                []Finding            `json:"findings,omitempty"`
	EffectiveStatus         map[string]string    `json:"effective_status,omitempty"`
	QualityConclusions      map[string]string    `json:"quality_conclusions,omitempty"`
	Dispositions            []FindingDisposition `json:"dispositions,omitempty"`
	Ownerships              []FindingOwnership   `json:"ownerships,omitempty"`
	Analysis                map[string]string    `json:"analysis,omitempty"`
	ReportDigest            string               `json:"report_digest"`
	Observations            []Finding            `json:"observations,omitempty"`
	ObservationDispositions []FindingDisposition `json:"observation_dispositions,omitempty"`
}

// JudgmentBaseline is the cache-embedded semantic state used to materialize
// carried judgments for a delta/repair run.
type JudgmentBaseline struct {
	SchemaVersion   int                          `json:"schema_version"`
	Records         map[string]judgments.Binding `json:"records,omitempty"`
	LogicalStatus   map[string]string            `json:"logical_status"`
	Findings        []Finding                    `json:"findings"`
	SynthesisDigest string                       `json:"synthesis_digest"`
	Relationships   []string                     `json:"relationships"`
	// DeferredFindings records the run's findings whose ownership points at
	// another unit. They are audit state, not carry state: routing lives in the
	// deferred-findings ledger, and a later run of this unit must not re-dispose
	// them (see framework/validation_cache.md §Format → Deferred-findings ledger).
	DeferredFindings []Finding `json:"deferred_findings,omitempty"`
}

// DeferredFinding is one pending verify finding routed to this run's unit by
// another unit's verify synthesis. The plan loads it from the deferred-
// findings ledger, the cross synthesis must dispose it, and gate-finalize
// consumes the ledger entry it resolved (see framework/verification_scope.md
// §Coverage Model → Deferred findings).
type DeferredFinding struct {
	SourceUnit   string  `json:"source_unit"`
	SourceRun    string  `json:"source_run"`
	EvidencePath string  `json:"evidence_path"`
	Reason       string  `json:"reason"`
	Finding      Finding `json:"finding"`
}

// Attempt is one submission of a session report. Every submission is recorded
// — accepted and rejected alike — so retry counts and replacement results are
// traceable.
type Attempt struct {
	Attempt         int    `json:"attempt"`
	SubmittedAt     string `json:"submitted_at"`
	Status          string `json:"status"` // accepted | rejected
	RejectionReason string `json:"rejection_reason,omitempty"`
	ResultDigest    string `json:"result_digest"`
}

// SessionState is the persisted mutable state of one reviewer session: the
// agent-assigned coverage keys and, once accepted, the verbatim report and its
// parsed result. A missing state file means pending.
type SessionState struct {
	SessionID             string            `json:"session_id"`
	Keys                  []string          `json:"keys"`
	Status                string            `json:"status"` // pending | accepted | rejected | not_required
	Attempts              []Attempt         `json:"attempts,omitempty"`
	Report                string            `json:"report,omitempty"`
	SemanticDigest        string            `json:"semantic_digest,omitempty"` // the accepted report, verbatim
	Result                *SessionResult    `json:"session_result,omitempty"`
	ConsumedResultDigests map[string]string `json:"consumed_result_digests,omitempty"`
}

// CarriedEvidenceEntry is the plan-time snapshot of one baseline file entry
// whose checks are carried over by a delta/repair run. The plan fixes it
// before execution so gate-finalize merges carried evidence from the run's
// immutable input instead of re-reading a baseline that another run may have
// rewritten since (see framework/validation_cache.md §Write Rules → Tooled
// writes). Deps carries only the declare-heavy remainder the entry owns
// beyond its checks — file-level deps no check declared — so a re-run check's
// superseded deps can never be resurrected into the new entry; each carried
// check's own deps travel in Checks.
type CarriedEvidenceEntry struct {
	Path   string              `json:"path"`
	Hash   string              `json:"hash"`
	Deps   []string            `json:"deps,omitempty"`
	Checks []CarriedCheckEntry `json:"checks,omitempty"`
}

// CarriedCheckEntry is one carried check's dependency evidence.
type CarriedCheckEntry struct {
	Check string   `json:"check"`
	Lens  string   `json:"lens,omitempty"`
	Deps  []string `json:"deps,omitempty"`
}

// Run is the persisted plan of one gate run: the immutable input snapshot,
// the coverage set that must be judged, and the run mode. It carries no
// sessions: the agent creates them under their chosen key batches.
type Run struct {
	PublicEvidence  map[string][]string          `json:"public_evidence,omitempty"`
	RunID           string                       `json:"run_id"`
	SchemaVersion   int                          `json:"schema_version,omitempty"`
	Protocol        string                       `json:"protocol,omitempty"`
	Records         map[string]judgments.Binding `json:"records,omitempty"`
	Gate            string                       `json:"gate"`
	TargetKind      string                       `json:"target_kind"`
	TargetName      string                       `json:"target_name"`
	Target          string                       `json:"target"`
	CreatedAt       string                       `json:"created_at"`
	Status          string                       `json:"status"`
	Mode            string                       `json:"mode"`
	Refs            []Ref                        `json:"refs"`
	Surfaces        []Surface                    `json:"surfaces"`
	ExtraInputs     []string                     `json:"extra_inputs,omitempty"`
	RequiredFiles   []string                     `json:"required_files,omitempty"`
	OwnSpecFiles    []string                     `json:"own_spec_files,omitempty"`
	CarriedKeys     []string                     `json:"carried_keys,omitempty"`
	CarriedEvidence []CarriedEvidenceEntry       `json:"carried_evidence,omitempty"`
	RerunKeys       []string                     `json:"rerun_keys,omitempty"`
	Coverage        []CoverageKey                `json:"coverage"`
	Relationships   []string                     `json:"relationships"`
	CarriedResults  []SessionResult              `json:"carried_results,omitempty"`

	// ProtectedEvidence holds the stable spec paths selected for requirement
	// protection before execution scope is narrowed by delta or repair.
	ProtectedEvidence []string `json:"protected_evidence,omitempty"`

	// DeferredFindings are the pending deferrals this run's unit must dispose,
	// loaded from the deferred-findings ledger at plan time (verify unit runs;
	// gate-finalize writes the ledger for the same gate/kind).
	DeferredFindings []DeferredFinding `json:"deferred_findings,omitempty"`
	// Notices records plan-time disclosures (delta scope derivation, carried
	// checks, conservative degradations) for gate-status and the plan output.
	Notices []string `json:"notices,omitempty"`
}

// StateRelPath returns the repo-relative path of a run's state file.
func StateRelPath(runID string) string {
	return path.Join(runStateDir, runID, "run.json")
}

func runStatePath(repoRoot, runID string) (string, error) {
	if err := localstate.ValidateID(runID); err != nil {
		return "", err
	}
	return localstate.Path(repoRoot, runStateDir, runID, "run.json")
}

func runDirectoryPath(repoRoot, runID string) (string, error) {
	if err := localstate.ValidateID(runID); err != nil {
		return "", err
	}
	return localstate.Path(repoRoot, runStateDir, runID)
}

func sessionStatePath(repoRoot, runID, sessionID string) (string, error) {
	if err := localstate.ValidateID(runID); err != nil {
		return "", err
	}
	return localstate.Path(repoRoot, runStateDir, runID, "sessions", sessionFileBase(sessionID)+".json")
}

func sessionsDirPath(repoRoot, runID string) (string, error) {
	if err := localstate.ValidateID(runID); err != nil {
		return "", err
	}
	return localstate.Path(repoRoot, runStateDir, runID, "sessions")
}

// sessionFileBase maps the logical session id to a fixed-length filename that
// is valid on every supported filesystem. The original id remains embedded in
// the session state and is validated after loading.
func sessionFileBase(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return fmt.Sprintf("%x", sum)
}

// EntryCount reports how many files the snapshot covers (refs with a resolved
// file plus expanded surface entries).
func (r *Run) EntryCount() int {
	n := 0
	for _, ref := range r.Refs {
		if ref.Resolved != "" {
			n++
		}
	}
	for _, surface := range r.Surfaces {
		n += len(surface.Entries)
	}
	return n
}

// CoverageByKey returns the coverage key with the given key, or nil.
func (r *Run) CoverageByKey(key string) *CoverageKey {
	for i := range r.Coverage {
		if r.Coverage[i].Key == key {
			return &r.Coverage[i]
		}
	}
	return nil
}

// validateGateTarget checks the supported gate/kind/target combinations.
func validateGateTarget(gate, targetKind, target string) error {
	switch gate {
	case GateValidate, GateVerify:
	default:
		return fmt.Errorf("invalid gate %q: must be validate or verify", gate)
	}
	switch targetKind {
	case TargetKindUnit:
	case TargetKindRule:
		if gate != GateValidate {
			return fmt.Errorf("rule targets support the validate gate only (rule verify has been removed) — got %q", gate)
		}
	default:
		return fmt.Errorf("invalid target kind %q: must be unit or rule", targetKind)
	}
	switch target {
	case TargetCandidate, TargetStable:
	default:
		return fmt.Errorf("invalid target %q: must be candidate or stable", target)
	}
	return nil
}

// validateTargetLayer enforces the file-existence layer boundary: a candidate
// target requires the candidate main file; a stable target is a stable-only
// target (the stable file exists and no candidate file exists).
func validateTargetLayer(repoRoot, targetKind, targetName, target string) error {
	candidate := targetLayerSpecRef(targetKind, targetName, TargetCandidate)
	stable := targetLayerSpecRef(targetKind, targetName, TargetStable)
	hasCandidate := fileExists(filepath.Join(repoRoot, filepath.FromSlash(candidate)))
	hasStable := fileExists(filepath.Join(repoRoot, filepath.FromSlash(stable)))
	if target == TargetCandidate {
		if !hasCandidate {
			return fmt.Errorf("no candidate %s file for %q (%s)", targetKind, targetName, candidate)
		}
		return nil
	}
	if !hasStable {
		return fmt.Errorf("no stable %s file for %q (%s)", targetKind, targetName, stable)
	}
	if hasCandidate {
		return fmt.Errorf("%s %q has a candidate file (%s) — a stable target is a stable-only confirmation; use --target candidate", targetKind, targetName, candidate)
	}
	return nil
}

// Plan resolves the run's input surface, applies the agent-declared extra
// inputs, computes the coverage set, and persists the run with no sessions.
// Any previous run for the same (gate, target kind, target name, layer) is
// replaced. The coverage set and the input snapshot are fixed here, before any
// executor reads input.
func Plan(repoRoot, gate, targetKind, targetName, target, mode string, extraInputs, rerunKeys, relationships []string, now time.Time) (*Run, error) {
	// Validate the target name before the mutation lock touches the working
	// tree: an invalid name must fail without creating meta/ or a lock file
	// (tooling/README.md §Target names).
	if err := specpaths.ValidateTargetName(targetKind, targetName); err != nil {
		return nil, err
	}
	var planned *Run
	err := WithMutation(repoRoot, func() error {
		var err error
		planned, err = planUnlocked(repoRoot, gate, targetKind, targetName, target, mode, extraInputs, rerunKeys, relationships, now)
		return err
	})
	return planned, err
}

// WithMutation serializes one complete gate-run state transition across
// processes. Callers must enter before loading mutable run state and hold the
// lock through the final write or cache publication.
func WithMutation(repoRoot string, fn func() error) error {
	return localstate.WithExclusiveLock(repoRoot, mutationLockDir, mutationLock, mutationTimeout, fn)
}

// TargetedInvalidation is the complete, repository-locked result of recording
// a targeted P0/P1: invalidated immutable evidence, the canonical cache
// transition, and matching open runs that cannot overwrite recovery state.
type TargetedInvalidation struct {
	Cache                  *validationcache.TargetedInvalidation
	InvalidatedRunIDs      []string
	InvalidatedJudgmentIDs []string
}

// InvalidateTargeted records targeted P0/P1 recovery state and invalidates
// every matching open plan under the same mutation lock used by plan, submit,
// and finalize. This ordering closes the race where an older plan could write
// a pass cache after the targeted finding was recorded.
func InvalidateTargeted(repoRoot, gate, targetKind, targetName, target string, checkKeys []string) (*TargetedInvalidation, error) {
	if err := validateGateTarget(gate, targetKind, target); err != nil {
		return nil, err
	}
	if err := specpaths.ValidateTargetName(targetKind, targetName); err != nil {
		return nil, err
	}
	if err := validateTargetLayer(repoRoot, targetKind, targetName, target); err != nil {
		return nil, err
	}
	var keys []string
	for _, raw := range checkKeys {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, fmt.Errorf("targeted invalidation check key must not be empty")
		}
		keys = append(keys, key)
	}
	keys = dedupeSorted(keys)
	if len(keys) == 0 {
		return nil, fmt.Errorf("at least one targeted invalidation check key is required")
	}

	result := &TargetedInvalidation{}
	err := WithMutation(repoRoot, func() error {
		if gate == GateVerify {
			items, err := verifyItems(repoRoot, &Run{TargetKind: targetKind, TargetName: targetName, Target: target})
			if err != nil {
				return err
			}
			for i, key := range keys {
				if stringInSlice(items, key) {
					keys[i] = reviewKey(SessionKindItem, targetName, key)
				}
			}
			keys = dedupeSorted(keys)
			refs, err := targetedVerifyReferences(repoRoot, targetName, target, keys)
			if err != nil {
				return err
			}
			reason := fmt.Sprintf("targeted P0/P1: verify@%s (%s), checks: %s", targetName, target, strings.Join(keys, ", "))
			for _, ref := range refs {
				if err := judgments.Invalidate(repoRoot, ref.ID, reason); err != nil {
					return err
				}
				result.InvalidatedJudgmentIDs = append(result.InvalidatedJudgmentIDs, ref.ID)
			}
		}
		ids, err := invalidateOpenRunsFor(repoRoot, gate, targetKind, targetName, target, keys)
		if err != nil {
			return err
		}
		result.InvalidatedRunIDs = ids
		cacheResult, err := validationcache.InvalidateGateCache(repoRoot, targetKind, targetName, gate, target, keys)
		if err != nil {
			return err
		}
		result.Cache = cacheResult
		return nil
	})
	return result, err
}

func planUnlocked(repoRoot, gate, targetKind, targetName, target, mode string, extraInputs, rerunKeys, relationships []string, now time.Time) (*Run, error) {
	// The plan's reads all precede its writes, so one derivation resolves the
	// run and builds the coverage plan with shared expansions and audit.
	derivation, err := NewDerivation(repoRoot)
	if err != nil {
		return nil, err
	}
	run, err := derivation.resolveRun(gate, targetKind, targetName, target, mode, extraInputs, rerunKeys)
	if err != nil {
		return nil, err
	}
	run.Relationships = dedupeSorted(relationships)
	if err := validateRelationships(run); err != nil {
		return nil, err
	}
	coverage, carried, required, notices, err := derivation.buildCoveragePlan(run)
	if err != nil {
		return nil, err
	}
	if err := validateRelationships(run); err != nil {
		return nil, err
	}
	run.Coverage = coverage
	run.CarriedKeys = carried
	if len(carried) > 0 {
		carriedResults, cerr := loadCarriedResults(repoRoot, run, carried)
		if cerr != nil {
			return nil, cerr
		}
		run.CarriedResults = carriedResults
		if gate == GateVerify {
			baseline, err := validationcache.ReadGateBaseline(repoRoot, targetKind, targetName, gate)
			if err != nil {
				return nil, err
			}
			state, err := validatedJudgmentState(baseline)
			if err != nil {
				return nil, err
			}
			for _, key := range carried {
				if ref, ok := state.Records[key]; ok {
					ref.Source = "carried"
					if !strings.HasPrefix(key, "preserve:") {
						ref.Layer = target
					}
					run.Records[key] = ref
				}
			}
		}
		carriedEvidence, cerr := loadCarriedEvidence(repoRoot, run, carried)
		if cerr != nil {
			return nil, cerr
		}
		run.CarriedEvidence = carriedEvidence
	}
	run.RequiredFiles = required
	run.Notices = notices

	// A verify run consumes the unit's pending deferrals: findings another
	// unit's verify synthesis routed here by recorded ownership (quality
	// ownership or this unit's own protected stable-record drift). Loading
	// them at plan time makes them part of the run's immutable input — the
	// final synthesis must dispose every one of them, exactly like a carried
	// judgment.
	if gate == GateVerify && targetKind == TargetKindUnit {
		deferred, derr := loadDeferredFindings(repoRoot, targetName)
		if derr != nil {
			return nil, derr
		}
		run.DeferredFindings = deferred
	}

	if err := removeRunsFor(repoRoot, gate, targetKind, targetName, target); err != nil {
		return nil, err
	}
	runID, err := newRunID(now)
	if err != nil {
		return nil, err
	}
	if err := localstate.ValidateID(runID); err != nil {
		return nil, fmt.Errorf("generated run id: %w", err)
	}
	run.RunID = runID
	run.CreatedAt = now.UTC().Format(timestampLayout)
	if err := writeRun(repoRoot, run); err != nil {
		return nil, err
	}
	if gate == GateVerify {
		if err := prepareSharedChecks(repoRoot, run); err != nil {
			return nil, err
		}
		if err := writeRun(repoRoot, run); err != nil {
			return nil, err
		}
	}
	// Every new plan is a cleanup point: runs of targets that no longer exist
	// and shared task files no surviving run references are discarded. The
	// sweep is best-effort — a plan is not failed by leftover local state.
	if _, err := SweepOrphanedState(repoRoot); err != nil {
		run.Notices = append(run.Notices, "local-state cleanup failed: "+err.Error())
		if werr := writeRun(repoRoot, run); werr != nil {
			return nil, werr
		}
	}
	return run, nil
}

// resolveRun validates the run request and resolves its input surface: the
// gate's derived refs and surfaces plus the agent-declared extra inputs. It
// performs no session planning and writes no state, so Plan and the delta
// scope preview share exactly the same input resolution.
func (d *Derivation) resolveRun(gate, targetKind, targetName, target, mode string, extraInputs, rerunKeys []string) (*Run, error) {
	repoRoot := d.root
	if err := validateGateTarget(gate, targetKind, target); err != nil {
		return nil, err
	}
	if err := specpaths.ValidateTargetName(targetKind, targetName); err != nil {
		return nil, err
	}
	if err := validateTargetLayer(repoRoot, targetKind, targetName, target); err != nil {
		return nil, err
	}
	switch mode {
	case ModeFull, ModeDelta, ModeRepair:
	default:
		return nil, fmt.Errorf("invalid mode %q: must be full, delta, or repair", mode)
	}
	if mode == ModeFull && len(rerunKeys) > 0 {
		return nil, fmt.Errorf("--rerun applies to --mode delta or --mode repair only (a full run already covers every check)")
	}
	refs, surfaces, err := d.derive(gate, targetKind, targetName, target)
	if err != nil {
		return nil, err
	}
	run := &Run{
		SchemaVersion: 2,
		Protocol:      judgments.Protocol(repoRoot),
		Records:       map[string]judgments.Binding{},
		Gate:          gate,
		TargetKind:    targetKind,
		TargetName:    targetName,
		Target:        target,
		Status:        StatusOpen,
		Mode:          mode,
		RerunKeys:     dedupeSorted(rerunKeys),
		Refs:          refs,
		Surfaces:      surfaces,
	}
	run.OwnSpecFiles = []string{mainSpecRef(run)}
	if targetKind == TargetKindUnit {
		appendices, err := unitAppendices(repoRoot, targetName, target)
		if err != nil {
			return nil, err
		}
		run.OwnSpecFiles = append(run.OwnSpecFiles, appendices...)
	}
	seenRefs := map[string]bool{}
	for _, ref := range run.Refs {
		seenRefs[ref.Ref] = true
	}
	seenSurfaces := map[string]bool{}
	for _, surface := range run.Surfaces {
		seenSurfaces[surface.Path] = true
	}
	for _, input := range extraInputs {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		canonical := input
		if !isLogicalRef(canonical) {
			canonical, err = repopath.Canonical(repoRoot, input)
			if err != nil {
				return nil, fmt.Errorf("input %q: %w", input, err)
			}
			if ref, ok := LogicalRuleRefForPath(canonical); ok {
				// A rule file is carried by its logical reference at every
				// boundary downstream, so a manifest's physical spelling is
				// normalized here — at the one point where inputs enter the
				// snapshot — instead of being rejected (or made
				// undeclarable) by each consumer.
				canonical = ref
			}
		}
		if !stringInSlice(run.ExtraInputs, canonical) {
			run.ExtraInputs = append(run.ExtraInputs, canonical)
		}
		if isLogicalRef(canonical) {
			if seenRefs[canonical] {
				continue
			}
			seenRefs[canonical] = true
			run.Refs = append(run.Refs, refreshRef(repoRoot, Ref{Ref: canonical, Source: SourceInput}))
			continue
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(canonical))
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("input %q: %v", input, err)
		}
		if info.IsDir() {
			if seenSurfaces[canonical] {
				continue
			}
			seenSurfaces[canonical] = true
			surface, err := d.resolveSurface(Surface{Path: canonical, Source: SourceInput})
			if err != nil {
				return nil, fmt.Errorf("input %q: %w", input, err)
			}
			run.Surfaces = append(run.Surfaces, surface)
			continue
		}
		if seenRefs[canonical] {
			continue
		}
		seenRefs[canonical] = true
		run.Refs = append(run.Refs, refreshRef(repoRoot, Ref{Ref: canonical, Source: SourceInput}))
	}
	if gate == GateVerify {
		if err := d.addReviewInputs(run); err != nil {
			return nil, err
		}
	}
	// The derived refs and surfaces are validated inside derive; extra inputs
	// are appended afterwards, so re-validate the complete set. A logical
	// reference whose resolution escapes the repository must be rejected here,
	// before any run state is written (see framework/verification_scope.md
	// §Coverage Model → Input roles).
	if err := validateSnapshotPaths(repoRoot, run.Refs, run.Surfaces); err != nil {
		return nil, err
	}
	sortRefs(run.Refs)
	sortSurfaces(run.Surfaces)
	sort.Strings(run.ExtraInputs)
	return run, nil
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// Load reads a run state by id. A malformed or missing run is an error — the
// caller fails closed.
func Load(repoRoot, runID string) (*Run, error) {
	runID = strings.TrimSpace(runID)
	if err := localstate.ValidateID(runID); err != nil {
		return nil, err
	}
	statePath, err := runStatePath(repoRoot, runID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read gate run %s: %w", runID, err)
	}
	run := &Run{}
	if err := json.Unmarshal(data, run); err != nil {
		return nil, fmt.Errorf("cannot parse gate run %s: %w", runID, err)
	}
	if err := localstate.BindID(runID, run.RunID); err != nil {
		return nil, fmt.Errorf("gate run %s has invalid identity: %w", runID, err)
	}
	if run.RunID == "" || run.Gate == "" || run.TargetKind == "" || run.TargetName == "" || run.Target == "" || run.Status == "" || run.Mode == "" {
		return nil, fmt.Errorf("gate run %s is missing required fields", runID)
	}
	if run.Records == nil {
		run.Records = map[string]judgments.Binding{}
	}
	switch run.Status {
	case StatusOpen, StatusConsumed, StatusInvalidated:
	default:
		return nil, fmt.Errorf("gate run %s has invalid status %q", runID, run.Status)
	}
	if err := validateGateTarget(run.Gate, run.TargetKind, run.Target); err != nil {
		return nil, fmt.Errorf("gate run %s: %w", runID, err)
	}
	if len(run.Coverage) == 0 && len(run.Relationships) == 0 {
		return nil, fmt.Errorf("gate run %s has no coverage set", runID)
	}
	if err := validateRelationships(run); err != nil {
		return nil, err
	}
	if err := validateCoverage(run, run.Coverage); err != nil {
		return nil, fmt.Errorf("gate run %s has an invalid coverage set: %w", runID, err)
	}
	if run.Gate == GateVerify && run.SchemaVersion != 2 {
		return nil, fmt.Errorf("old verify run uses a retired protocol; replan a full verify")
	}
	return run, nil
}

// ListRuns reads every run state under meta/gate_runs, newest first. Entries
// that cannot be loaded are skipped — a status listing never fails on a
// stray file.
func ListRuns(repoRoot string) ([]*Run, error) {
	dir := filepath.Join(repoRoot, filepath.FromSlash(runStateDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read gate run directory: %w", err)
	}
	var runs []*Run
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		run, err := Load(repoRoot, entry.Name())
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID > runs[j].RunID })
	return runs, nil
}

// LoadSessionState reads one session's state. A missing state file means the
// session is still pending.
func LoadSessionState(repoRoot string, run *Run, sessionID string) (*SessionState, error) {
	statePath, err := sessionStatePath(repoRoot, run.RunID, sessionID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		states, err := sharedStates(repoRoot, run, nil)
		if err != nil {
			return nil, err
		}
		for _, s := range states {
			if s.SessionID == sessionID {
				return s, nil
			}
		}
		return &SessionState{SessionID: sessionID, Status: SessionPending}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read session %q state: %w", sessionID, err)
	}
	state := &SessionState{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("cannot parse session %q state: %w", sessionID, err)
	}
	if state.SessionID != sessionID || state.Status == "" {
		return nil, fmt.Errorf("session %q state is missing required fields", sessionID)
	}
	return state, nil
}

// LoadSessionStates reads every session state under the run directory. A
// missing directory means no session has been created. Entries that cannot be
// loaded are an error — a corrupt session state must not silently drop a
// judgment.
func LoadSessionStates(repoRoot string, run *Run) ([]*SessionState, error) {
	dir, err := sessionsDirPath(repoRoot, run.RunID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return sharedStates(repoRoot, run, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}
	var states []*SessionState
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if rerr != nil {
			return nil, fmt.Errorf("cannot read session state %s: %w", entry.Name(), rerr)
		}
		state := &SessionState{}
		if uerr := json.Unmarshal(data, state); uerr != nil {
			return nil, fmt.Errorf("cannot parse session state %s: %w", entry.Name(), uerr)
		}
		if state.SessionID == "" || state.Status == "" {
			return nil, fmt.Errorf("session state %s is missing required fields", entry.Name())
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].SessionID < states[j].SessionID })
	return sharedStates(repoRoot, run, states)
}

// sessionStatesIntact reports whether every session state file LoadSessionStates
// would read is readable, parsable, and carries its required fields. The check
// separates repairable session-progress damage from permanent evidence
// failure: an unusable session state file can be removed to reopen its session
// (a missing session state means the session is still pending), while a dead
// evidence binding has no repair inside the run. The orphan sweep reclaims
// only the permanent class (see unresumableEvidence in cleanup.go).
func sessionStatesIntact(repoRoot string, run *Run) bool {
	dir, err := sessionsDirPath(repoRoot, run.RunID)
	if err != nil {
		return false
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return false
		}
		state := &SessionState{}
		if err := json.Unmarshal(data, state); err != nil {
			return false
		}
		if state.SessionID == "" || state.Status == "" {
			return false
		}
	}
	return true
}

// SaveSessionState persists one session's state atomically. Session state
// files are independent, so concurrent submissions of different sessions never
// contend.
func SaveSessionState(repoRoot string, run *Run, state *SessionState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session %s state: %w", state.SessionID, err)
	}
	data = append(data, '\n')
	statePath, err := sessionStatePath(repoRoot, run.RunID, state.SessionID)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(statePath, data, 0644); err != nil {
		return fmt.Errorf("write session %s state: %w", state.SessionID, err)
	}
	return nil
}

// Compare re-resolves the derived input surface and re-hashes the stored
// entries, returning one line per divergence (sorted). An empty result means
// the snapshot is unchanged. An input surface that no longer resolves at all
// (a removed main spec, an unreadable input) is reported as a divergence —
// the snapshot cannot be verified, so the run must not be finalized.
func Compare(repoRoot string, run *Run) ([]string, error) {
	if run.Status != StatusOpen {
		return nil, fmt.Errorf("gate run %s is %s — only an open run can be finalized; plan a new run", run.RunID, run.Status)
	}
	derivation, err := NewDerivation(repoRoot)
	if err != nil {
		return nil, err
	}
	refs, surfaces, err := derivation.derive(run.Gate, run.TargetKind, run.TargetName, run.Target)
	if err != nil {
		return []string{fmt.Sprintf("input surface no longer resolvable: %v", err)}, nil
	}
	if run.Gate == GateVerify {
		current, err := derivation.resolveRun(run.Gate, run.TargetKind, run.TargetName, run.Target, ModeFull, run.ExtraInputs, nil)
		if err != nil {
			return []string{err.Error()}, nil
		}
		if current.Protocol != run.Protocol {
			return []string{"review protocol changed"}, nil
		}
		var derived []Ref
		for _, ref := range current.Refs {
			if ref.Source != SourceInput {
				derived = append(derived, ref)
			}
		}
		refs = derived
	}
	// The derived part is re-derived; the agent-declared part (the input
	// manifest entries) is re-resolved from the stored entries. The two parts are compared
	// separately so a replaced or missing entry on either side is reported.
	var derivedStoredRefs, inputStoredRefs, inputCurrentRefs []Ref
	for _, ref := range run.Refs {
		if ref.Source == SourceInput {
			inputStoredRefs = append(inputStoredRefs, ref)
			if !isLogicalRef(ref.Ref) {
				if _, err := repopath.Canonical(repoRoot, ref.Ref); err != nil {
					return nil, fmt.Errorf("input %q is no longer project-contained: %w", ref.Ref, err)
				}
			}
			inputCurrentRefs = append(inputCurrentRefs, refreshRef(repoRoot, ref))
		} else {
			derivedStoredRefs = append(derivedStoredRefs, ref)
		}
	}
	var derivedStoredSurfaces, inputStoredSurfaces, inputCurrentSurfaces []Surface
	for _, surface := range run.Surfaces {
		if surface.Source == SourceInput {
			inputStoredSurfaces = append(inputStoredSurfaces, surface)
			current, err := derivation.resolveSurface(surface)
			if err != nil {
				return nil, fmt.Errorf("input surface %q is no longer project-contained: %w", surface.Path, err)
			}
			inputCurrentSurfaces = append(inputCurrentSurfaces, current)
		} else {
			derivedStoredSurfaces = append(derivedStoredSurfaces, surface)
		}
	}
	var divergences []string
	divergences = append(divergences, diffRefs(derivedStoredRefs, refs)...)
	divergences = append(divergences, diffRefs(inputStoredRefs, inputCurrentRefs)...)
	divergences = append(divergences, diffSurfaces(derivedStoredSurfaces, surfaces)...)
	divergences = append(divergences, diffSurfaces(inputStoredSurfaces, inputCurrentSurfaces)...)
	sort.Strings(divergences)
	return divergences, nil
}

// Consume marks a run as finalized. The run state is kept for audit and is
// replaced by the next Plan for the same tuple.
func Consume(repoRoot string, run *Run) error {
	run.Status = StatusConsumed
	return writeRun(repoRoot, run)
}

func invalidateOpenRunsFor(repoRoot, gate, targetKind, targetName, target string, checkKeys []string) ([]string, error) {
	runs, err := ListRuns(repoRoot)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, run := range runs {
		if run.Status != StatusOpen || run.Gate != gate || run.TargetKind != targetKind || run.TargetName != targetName || run.Target != target {
			continue
		}
		run.Status = StatusInvalidated
		run.Notices = append(run.Notices, "targeted P0/P1 invalidated checks "+strings.Join(checkKeys, ", ")+" — plan a new run")
		if err := writeRun(repoRoot, run); err != nil {
			return nil, err
		}
		ids = append(ids, run.RunID)
	}
	sort.Strings(ids)
	return ids, nil
}

// Delete removes a run's state directory (a rejected finalize has no usable
// state left).
func Delete(repoRoot string, run *Run) error {
	runDir, err := runDirectoryPath(repoRoot, run.RunID)
	if err != nil {
		return err
	}
	err = os.RemoveAll(runDir)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// AllowsDeclaration reports whether a cache files entry path (a physical repo
// path or a logical reference) belongs to the run's input snapshot.
func (r *Run) AllowsDeclaration(repoRoot, declared string) bool {
	_, ok := r.SnapshotHash(repoRoot, declared)
	return ok
}

// SnapshotHash returns the plan-time whole-file hash for one declaration.
// It is the evidence-assembly boundary: callers that read a file later must
// prove that the bytes they used still have this identity before publishing
// evidence derived from them.
func (r *Run) SnapshotHash(repoRoot, declared string) (string, bool) {
	canonical := canonicalPath(repoRoot, declared)
	if isLogicalRef(canonical) {
		for _, ref := range r.Refs {
			if isLogicalRef(ref.Ref) && ref.Ref == canonical {
				return ref.Hash, true
			}
		}
		return "", false
	}
	for _, ref := range r.Refs {
		if !isLogicalRef(ref.Ref) && ref.Ref == canonical {
			return ref.Hash, true
		}
	}
	for _, surface := range r.Surfaces {
		for _, entry := range surface.Entries {
			if entry.Path == canonical {
				return entry.Hash, true
			}
		}
	}
	return "", false
}

// SessionAllowsDeclaration reports whether a path belongs to one session's
// declared read surface. Run-wide snapshot membership is insufficient: a
// session may not borrow evidence that was assigned only to another session.
func (r *Run) SessionAllowsDeclaration(repoRoot string, session *SessionSpec, declared string) bool {
	canonical := canonicalPath(repoRoot, declared)
	for _, allowed := range session.ReadRefs {
		if isLogicalRef(allowed) {
			if isLogicalRef(canonical) && allowed == canonical {
				return true
			}
			continue
		}
		if canonicalPath(repoRoot, allowed) == canonical {
			return true
		}
	}
	return false
}

// IsProtectedStableInput identifies the physical stable spec evidence selected
// by planning. It remains available when a delta carries the preserve checks.
func (r *Run) IsProtectedStableInput(repoRoot, declared string) bool {
	canonical := canonicalPath(repoRoot, declared)
	return stringInSlice(r.ProtectedEvidence, canonical)
}

// ------------------------------------------------------------
// Surface derivation
// ------------------------------------------------------------

// derive resolves the gate's protocol input surface for one target.
func (d *Derivation) derive(gate, targetKind, targetName, target string) ([]Ref, []Surface, error) {
	repoRoot := d.root
	var refs []Ref
	var surfaces []Surface
	var err error
	switch targetKind {
	case TargetKindUnit:
		switch gate {
		case GateValidate:
			refs, err = deriveUnitValidate(repoRoot, targetName, target)
		case GateVerify:
			refs, surfaces, err = d.deriveUnitCodeGate(targetName, target)
		}
	case TargetKindRule:
		refs, err = deriveRuleValidate(repoRoot, targetName, target)
	}
	if err != nil {
		return nil, nil, err
	}
	if refs == nil && surfaces == nil {
		return nil, nil, fmt.Errorf("unsupported gate %q for %s target", gate, targetKind)
	}
	if err := validateSnapshotPaths(repoRoot, refs, surfaces); err != nil {
		return nil, nil, err
	}
	return refs, surfaces, nil
}

// deriveUnitValidate resolves a validate unit run's inputs: the unit's own
// spec files in the target layer, the unit_refs dependency units (current
// layer) with their protocol appendices, the bound and global rules, the
// spec's affects.files entries, and every peer unit main spec (Check 9's
// surface-ownership audit reads all of them).
func deriveUnitValidate(repoRoot, unitName, target string) ([]Ref, error) {
	unitMain := targetLayerSpecRef(TargetKindUnit, unitName, target)
	content, err := readSpecContent(repoRoot, unitMain)
	if err != nil {
		return nil, err
	}
	var refs []Ref
	refs = append(refs, physicalRef(repoRoot, unitMain, SourceDerived))
	appendices, err := unitAppendices(repoRoot, unitName, target)
	if err != nil {
		return nil, err
	}
	for _, appendix := range appendices {
		refs = append(refs, physicalRef(repoRoot, appendix, SourceDerived))
	}
	depUnits := map[string]bool{}
	for _, dep := range parseRefList(content, "unit_refs", unitName) {
		depUnits[dep] = true
		refs = append(refs, logicalUnitRef(repoRoot, dep, SourceDerived))
		appendixRefs, err := logicalUnitAppendixRefs(repoRoot, dep)
		if err != nil {
			return nil, err
		}
		for _, appendixRef := range appendixRefs {
			refs = append(refs, appendixRef)
		}
	}
	unitNames, err := allUnitNames(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, peer := range unitNames {
		if peer == unitName || depUnits[peer] {
			continue
		}
		refs = append(refs, logicalUnitRef(repoRoot, peer, SourceDerived))
	}
	globalIDs, err := globalRuleIDs(repoRoot)
	if err != nil {
		return nil, err
	}
	ruleIDs := append(parseRefList(content, "rule_refs", ""), globalIDs...)
	for _, ruleID := range dedupeSorted(ruleIDs) {
		refs = append(refs, logicalRuleRef(repoRoot, ruleID, SourceDerived))
	}
	for _, affected := range dedupeStrings(specvalidation.ExtractAffectsFiles(content)) {
		if strings.TrimSpace(affected) == "" || strings.TrimSpace(affected) == "<pending>" {
			continue
		}
		raw := affected
		affected, err = repopath.Canonical(repoRoot, affected)
		if err != nil {
			return nil, fmt.Errorf("affects.files path %q: %w", raw, err)
		}
		if fileExists(filepath.Join(repoRoot, filepath.FromSlash(affected))) {
			refs = append(refs, physicalRef(repoRoot, affected, SourceDerivedAffects))
		}
	}
	sortRefs(refs)
	return refs, nil
}

// deriveUnitCodeGate resolves a verify unit run's inputs: the unit's
// own spec files in the target layer plus the declared code surface
// (implementation_surface directories expanded to their repository-content
// files + affects.files). It fails closed before any run state is written on
// a spec with no acceptance items or an unresolvable declared surface.
func (d *Derivation) deriveUnitCodeGate(unitName, target string) ([]Ref, []Surface, error) {
	repoRoot := d.root
	unitMain := targetLayerSpecRef(TargetKindUnit, unitName, target)
	content, err := readSpecContent(repoRoot, unitMain)
	if err != nil {
		return nil, nil, err
	}
	// Fail closed before any run state is written: a verify run's work
	// set is the acceptance items, so an empty item set has no verifiable
	// object — the session plan would carry only the cross session and could
	// finalize a pass cache with no code evidence at all. The same
	// precondition is enforced by mechanical validate Check 2.
	if len(specvalidation.ExtractAcceptanceItemIDs(content)) == 0 {
		return nil, nil, fmt.Errorf("declared acceptance item set is empty — add at least one acceptance item before planning a verify run")
	}
	var refs []Ref
	refs = append(refs, physicalRef(repoRoot, unitMain, SourceDerived))
	appendices, err := unitAppendices(repoRoot, unitName, target)
	if err != nil {
		return nil, nil, err
	}
	for _, appendix := range appendices {
		refs = append(refs, physicalRef(repoRoot, appendix, SourceDerived))
	}
	surfaces, err := d.codeSurfaces(content)
	if err != nil {
		return nil, nil, err
	}
	sortRefs(refs)
	sortSurfaces(surfaces)
	return refs, surfaces, nil
}

// deriveRuleValidate resolves a validate rule run's inputs: the rule file and
// its stable sibling, plus every unit main spec (consumer discovery reads all
// of them).
func deriveRuleValidate(repoRoot, ruleID, target string) ([]Ref, error) {
	var refs []Ref
	if target == TargetCandidate {
		refs = append(refs, physicalRef(repoRoot, specpaths.RuleCandidateFileRef(ruleID), SourceDerived))
		stable := specpaths.RuleStableFileRef(ruleID)
		if fileExists(filepath.Join(repoRoot, filepath.FromSlash(stable))) {
			refs = append(refs, physicalRef(repoRoot, stable, SourceDerived))
		}
	} else {
		refs = append(refs, physicalRef(repoRoot, specpaths.RuleStableFileRef(ruleID), SourceDerived))
	}
	unitNames, err := allUnitNames(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, unitName := range unitNames {
		refs = append(refs, logicalUnitRef(repoRoot, unitName, SourceDerived))
	}
	sortRefs(refs)
	return refs, nil
}

// codeSurfaces resolves the spec's declared code surface: implementation
// surface paths (directories expand to their repository-content files) and
// affects.files. A missing affects.files path is recorded with no entries — a
// later appearance is a divergence. A declared implementation_surface gets no
// such tolerance: the fail-closed check below rejects an unresolvable value,
// so the no-entries divergence semantics apply to affects.files only.
func (d *Derivation) codeSurfaces(specContent string) ([]Surface, error) {
	repoRoot := d.root
	// Fail closed before expanding anything: a non-<pending>
	// implementation_surface that yields no file — an unresolvable path, or a
	// directory with no repository-content files — would expand to zero
	// entries, producing a plan (and later a pass cache) with no code evidence
	// at all. The same check runs in mechanical validate Check 3.
	if problems := specvalidation.CheckImplementationSurfaces(repoRoot, specContent); len(problems) > 0 {
		return nil, fmt.Errorf("declared implementation_surface cannot resolve to a code file — fix the acceptance items before planning a verify run: %s", specvalidation.FormatSurfaceProblems(problems))
	}
	var paths []string
	paths = append(paths, specvalidation.ExtractImplementationSurfaces(specContent)...)
	paths = append(paths, specvalidation.ExtractAffectsFiles(specContent)...)
	var surfaces []Surface
	seen := map[string]bool{}
	for _, p := range paths {
		if strings.TrimSpace(p) == "" || strings.TrimSpace(p) == "<pending>" {
			continue
		}
		raw := p
		var err error
		p, err = repopath.Canonical(repoRoot, p)
		if err != nil {
			return nil, fmt.Errorf("declared code surface %q: %w", raw, err)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		surface, err := d.resolveSurface(Surface{Path: p, Source: SourceDerived})
		if err != nil {
			return nil, err
		}
		surfaces = append(surfaces, surface)
	}
	return surfaces, nil
}

// resolveSurface fills a surface's entries from the current repository
// content: a directory expands to the files Git tracks or leaves untracked and
// unignored, a file becomes a single entry, and a missing path keeps no
// entries. A path that exists but cannot be expanded is an error — no entry
// can be recorded for it, so the caller must not proceed with a partial
// surface.
func (d *Derivation) resolveSurface(surface Surface) (Surface, error) {
	repoRoot := d.root
	canonical, err := repopath.Canonical(repoRoot, surface.Path)
	if err != nil {
		return Surface{}, err
	}
	surface.Path = canonical
	abs := filepath.Join(repoRoot, filepath.FromSlash(surface.Path))
	info, err := os.Stat(abs)
	if err != nil {
		surface.Entries = nil
		return surface, nil
	}
	if info.IsDir() {
		files, err := d.expander.Expand(surface.Path)
		if err != nil {
			return Surface{}, fmt.Errorf("declared code surface %q: %w", surface.Path, err)
		}
		var entries []Entry
		for _, f := range files {
			entries = append(entries, Entry{Path: f.Path, Hash: f.Hash})
		}
		surface.Entries = entries
		return surface, nil
	}
	hash, err := specpaths.FileHash(abs)
	if err != nil {
		return Surface{}, fmt.Errorf("declared code surface %q: %w", surface.Path, err)
	}
	surface.Entries = []Entry{{Path: surface.Path, Hash: hash}}
	return surface, nil
}

// unitAppendices lists a unit's appendix files in one layer.
func unitAppendices(repoRoot, unitName, layer string) ([]string, error) {
	appendices, err := specpaths.UnitAppendices(repoRoot, unitName, layer)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, appendix := range appendices {
		if appendix.Status != "exempt" {
			out = append(out, appendix.Path)
		}
	}
	return out, nil
}

// allUnitNames lists every unit with a main spec in either layer.
func allUnitNames(repoRoot string) ([]string, error) {
	seen := map[string]bool{}
	for _, dir := range []string{specpaths.CandidateDir, specpaths.StableDir} {
		entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(dir)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read unit directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "unit_") || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			base := strings.TrimSuffix(entry.Name(), ".md")
			name := strings.TrimPrefix(base, "unit_")
			if name != "" {
				seen[name] = true
			}
		}
	}
	var out []string
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// globalRuleIDs lists the active global rule (g_rule_*) ids from the stable
// layer. Candidate global rules are unpublished and do not constrain units.
func globalRuleIDs(repoRoot string) ([]string, error) {
	dir := specpaths.RuleStableDir
	entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(dir)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read global rule directory %s: %w", dir, err)
	}
	var out []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "g_rule_") && strings.HasSuffix(entry.Name(), ".md") {
			out = append(out, strings.TrimSuffix(entry.Name(), ".md"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// ------------------------------------------------------------
// Entry construction and refresh
// ------------------------------------------------------------

// physicalRef builds a physical snapshot ref: the path is canonicalized, the
// resolved value is the path itself, and the hash is the file's current
// whole-file hash ("" when the file does not exist).
func physicalRef(repoRoot, rel, source string) Ref {
	rel = canonicalPath(repoRoot, rel)
	ref := Ref{Ref: rel, Resolved: rel, Source: source}
	if hash, err := specpaths.FileHash(filepath.Join(repoRoot, filepath.FromSlash(rel))); err == nil {
		ref.Hash = hash
	}
	return ref
}

// logicalUnitRef builds a unit main spec logical ref (`unit:{name}`).
func logicalUnitRef(repoRoot, unitName, source string) Ref {
	return refreshRef(repoRoot, Ref{Ref: "unit:" + unitName, Source: source})
}

// logicalUnitAppendixRefs builds the logical refs of a unit's protocol
// appendices: every appendix base name present in either layer, resolved
// current-layer.
func logicalUnitAppendixRefs(repoRoot, unitName string) ([]Ref, error) {
	var bases []string
	for _, layer := range []string{TargetCandidate, TargetStable} {
		appendices, err := unitAppendices(repoRoot, unitName, layer)
		if err != nil {
			return nil, err
		}
		for _, file := range appendices {
			base := strings.TrimSuffix(filepath.Base(file), ".md")
			bases = append(bases, base)
		}
	}
	var refs []Ref
	for _, base := range dedupeSorted(bases) {
		refs = append(refs, refreshRef(repoRoot, Ref{Ref: "unit:" + unitName + ":appendix:" + base, Source: SourceDerived}))
	}
	return refs, nil
}

// logicalRuleRef builds a rule logical ref (`rule:{id}`).
func logicalRuleRef(repoRoot, ruleID, source string) Ref {
	return refreshRef(repoRoot, Ref{Ref: "rule:" + ruleID, Source: source})
}

// LogicalRuleRefForPath maps a rule file's physical path to the logical
// reference that carries it: docs/specs/rules/<layer>/<id>.md is carried as
// rule:<id>. Rule dependencies resolve by name everywhere downstream
// (freshness and promote re-bind by name, not by path), and the declaration
// boundary rejects a rule file's physical path — so a physical spelling that
// entered a run's snapshot would be readable by sessions yet undeclarable, a
// state no report can satisfy. The physical layer is dropped on purpose: the
// logical reference resolves by its documented applicability (global rules
// stable-only, bound rules current-layer), exactly like the same rule
// spelled logically in the input manifest.
func LogicalRuleRefForPath(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, specpaths.RuleModulesRootDir+"/")
	if !ok || !strings.HasSuffix(p, ".md") {
		return "", false
	}
	layer, file, ok := strings.Cut(rest, "/")
	if !ok || strings.Contains(file, "/") || (layer != TargetCandidate && layer != TargetStable) {
		return "", false
	}
	id := strings.TrimSuffix(file, ".md")
	if id == "" {
		return "", false
	}
	return "rule:" + id, true
}

// refreshRef re-resolves a ref against the current filesystem: logical unit
// and bound-rule references resolve current-layer, logical global-rule
// references resolve stable-only, and physical references resolve to their own
// path. Returns the ref with Resolved/Hash updated; Source is preserved.
func refreshRef(repoRoot string, ref Ref) Ref {
	if isLogicalRef(ref.Ref) {
		ref.Resolved = resolveLogical(repoRoot, ref.Ref)
	} else {
		ref.Ref = canonicalPath(repoRoot, ref.Ref)
		ref.Resolved = ref.Ref
	}
	ref.Hash = ""
	if ref.Resolved != "" {
		if hash, err := specpaths.FileHash(filepath.Join(repoRoot, filepath.FromSlash(ref.Resolved))); err == nil {
			ref.Hash = hash
		}
	}
	return ref
}

// resolveLogical resolves a logical reference to its applicable repo-relative
// path, or "" when it resolves to no file. Units and bound rules use
// current-layer semantics; global rules use stable-only semantics. The
// semantics mirror the cache freshness resolution.
func resolveLogical(repoRoot, ref string) string {
	var abs string
	if rest, found := strings.CutPrefix(ref, "unit:"); found {
		if _, appendix, isAppendix := strings.Cut(rest, ":appendix:"); isAppendix {
			abs = specpaths.ResolveUnitAppendix(repoRoot, appendix)
		} else {
			abs = specpaths.ResolveUnitFile(repoRoot, rest)
		}
	} else if rest, found := strings.CutPrefix(ref, "rule:"); found {
		abs = specpaths.ResolveRuleFile(repoRoot, rest)
	}
	if abs == "" {
		return ""
	}
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// ------------------------------------------------------------
// Diff
// ------------------------------------------------------------

func diffRefs(stored, current []Ref) []string {
	currentByRef := make(map[string]Ref, len(current))
	for _, ref := range current {
		currentByRef[ref.Ref] = ref
	}
	storedByRef := make(map[string]Ref, len(stored))
	var out []string
	for _, ref := range stored {
		storedByRef[ref.Ref] = ref
		now, ok := currentByRef[ref.Ref]
		if !ok {
			out = append(out, fmt.Sprintf("input removed: %s", ref.Ref))
			continue
		}
		if now.Resolved != ref.Resolved {
			out = append(out, fmt.Sprintf("layer resolution changed: %s (%s → %s)", ref.Ref, orNone(ref.Resolved), orNone(now.Resolved)))
			continue
		}
		if now.Hash != ref.Hash {
			out = append(out, fmt.Sprintf("modified: %s", ref.Ref))
		}
	}
	for _, ref := range current {
		if _, ok := storedByRef[ref.Ref]; !ok {
			out = append(out, fmt.Sprintf("input added: %s", ref.Ref))
		}
	}
	return out
}

func diffSurfaces(stored, current []Surface) []string {
	currentByPath := make(map[string]Surface, len(current))
	for _, surface := range current {
		currentByPath[surface.Path] = surface
	}
	storedByPath := make(map[string]Surface, len(stored))
	var out []string
	for _, surface := range stored {
		storedByPath[surface.Path] = surface
		now, ok := currentByPath[surface.Path]
		if !ok {
			out = append(out, fmt.Sprintf("input surface removed: %s", surface.Path))
			continue
		}
		out = append(out, diffSurfaceEntries(surface, now)...)
	}
	for _, surface := range current {
		if _, ok := storedByPath[surface.Path]; !ok {
			out = append(out, fmt.Sprintf("input surface added: %s", surface.Path))
		}
	}
	return out
}

func diffSurfaceEntries(stored, current Surface) []string {
	currentByPath := make(map[string]Entry, len(current.Entries))
	for _, entry := range current.Entries {
		currentByPath[entry.Path] = entry
	}
	storedByPath := make(map[string]Entry, len(stored.Entries))
	var out []string
	for _, entry := range stored.Entries {
		storedByPath[entry.Path] = entry
		now, ok := currentByPath[entry.Path]
		if !ok {
			out = append(out, fmt.Sprintf("removed: %s", entry.Path))
			continue
		}
		if now.Hash != entry.Hash {
			out = append(out, fmt.Sprintf("modified: %s", entry.Path))
		}
	}
	for _, entry := range current.Entries {
		if _, ok := storedByPath[entry.Path]; !ok {
			out = append(out, fmt.Sprintf("added: %s", entry.Path))
		}
	}
	return out
}

// ------------------------------------------------------------
// Parsing and path helpers
// ------------------------------------------------------------

// parseRefList reads a frontmatter ref list (unit_refs / rule_refs), drops
// the given self name, dedupes, and sorts.
func parseRefList(content, field, self string) []string {
	fm := specpaths.ReadFrontmatterStringMap(content)
	raw := strings.TrimSpace(fm[field])
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, ref := range specpaths.ParseRefList(raw) {
		if ref == "" || ref == self || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func readSpecContent(repoRoot, rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", rel, err)
	}
	return string(data), nil
}

func isLogicalRef(ref string) bool {
	return strings.HasPrefix(ref, "unit:") || strings.HasPrefix(ref, "rule:")
}

// validateSnapshotPaths proves that every physical path retained in a
// derived snapshot is project-contained. Logical references are validated by
// their resolved physical path when one exists.
func validateSnapshotPaths(repoRoot string, refs []Ref, surfaces []Surface) error {
	for _, ref := range refs {
		if !isLogicalRef(ref.Ref) {
			if _, err := repopath.Canonical(repoRoot, ref.Ref); err != nil {
				return fmt.Errorf("snapshot path %q: %w", ref.Ref, err)
			}
		}
		if ref.Resolved != "" {
			if _, err := repopath.Canonical(repoRoot, ref.Resolved); err != nil {
				return fmt.Errorf("resolved snapshot path %q: %w", ref.Resolved, err)
			}
		}
	}
	for _, surface := range surfaces {
		if _, err := repopath.Canonical(repoRoot, surface.Path); err != nil {
			return fmt.Errorf("snapshot surface %q: %w", surface.Path, err)
		}
		for _, entry := range surface.Entries {
			if _, err := repopath.Canonical(repoRoot, entry.Path); err != nil {
				return fmt.Errorf("snapshot entry %q: %w", entry.Path, err)
			}
		}
	}
	return nil
}

// canonicalPath converts a declared path to the canonical repo-relative
// slash form used throughout the snapshot: absolute paths become relative to
// the repo root, relative paths are cleaned.
// CanonicalDeclPath normalizes a report-declared path to the canonical
// repo-relative spelling the run snapshot uses, so `/`-relative spellings,
// `./` prefixes, absolute paths, and platform separators are equivalent when
// a report declaration is recorded (see framework/validation_cache.md §Format
// and §Dependency Declaration). Membership is still enforced separately
// against the session's read refs.
func CanonicalDeclPath(repoRoot, p string) string {
	return canonicalPath(repoRoot, p)
}

func canonicalPath(repoRoot, p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(repoRoot, p); err == nil {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(p)
	}
	return path.Clean(filepath.ToSlash(p))
}

func fileExists(abs string) bool {
	info, err := os.Stat(abs)
	return err == nil && !info.IsDir()
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func dedupeStrings(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func dedupeSorted(values []string) []string {
	out := dedupeStrings(values)
	sort.Strings(out)
	return out
}

func sortRefs(refs []Ref) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].Ref < refs[j].Ref })
}

func sortSurfaces(surfaces []Surface) {
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i].Path < surfaces[j].Path })
}

// ------------------------------------------------------------
// Run state persistence
// ------------------------------------------------------------

func newRunID(now time.Time) (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return fmt.Sprintf("%s-%x", now.UTC().Format("20060102-150405"), b), nil
}

func writeRun(repoRoot string, run *Run) error {
	if err := localstate.ValidateID(run.RunID); err != nil {
		return fmt.Errorf("write gate run: %w", err)
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gate run %s: %w", run.RunID, err)
	}
	data = append(data, '\n')
	statePath, err := runStatePath(repoRoot, run.RunID)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(statePath, data, 0644); err != nil {
		return fmt.Errorf("write gate run %s: %w", run.RunID, err)
	}
	return nil
}

// writeFileAtomic writes a file through a temp file + rename so a reader
// never observes a half-written state.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// removeRunsFor deletes every existing run state for the same
// (gate, target kind, target name, layer) — at most one run exists per
// tuple.
func removeRunsFor(repoRoot, gate, targetKind, targetName, target string) error {
	dir := filepath.Join(repoRoot, filepath.FromSlash(runStateDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read gate run directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		existing, loadErr := Load(repoRoot, entry.Name())
		if loadErr != nil {
			continue
		}
		if existing.Gate == gate && existing.TargetKind == targetKind && existing.TargetName == targetName && existing.Target == target {
			if err := Delete(repoRoot, existing); err != nil {
				return fmt.Errorf("replace previous gate run %s: %w", existing.RunID, err)
			}
		}
	}
	return nil
}

// Package gaterun fixes the immutable input snapshot and the deterministic
// packet plan of a quality-gate run. A promote-consumable gate cache is
// written by the sequence
//
//	gate-plan → execute packets → gate-submit (per report) → gate-finalize
//
// gate-plan resolves the run's input surface — the target's spec files, the
// dependency spec objects, and (for verify/review) the declared code surface
// — adds the agent-declared extra inputs, generates the packet set for the
// run mode, and persists the run under meta/gate_runs/. gate-submit validates
// and records each packet report mechanically; gate-finalize re-resolves the
// surface and re-hashes the stored entries, and any divergence (a modified,
// added, or removed input, or a logical reference whose layer resolution
// moved) rejects the write, so the cache's evidence can only describe content
// that was stable for the whole judgment window (see
// framework/validation_cache.md §Write Rules → Tooled writes).
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

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

const (
	GateValidate = "validate"
	GateVerify   = "verify"
	GateReview   = "review"

	TargetKindUnit = "unit"
	TargetKindRule = "rule"

	TargetCandidate = "candidate"
	TargetStable    = "stable"

	// Run modes: full generates every packet; delta derives the re-run set
	// from a pass baseline's stale evidence; repair derives it from a
	// failure record's status map.
	ModeFull   = "full"
	ModeDelta  = "delta"
	ModeRepair = "repair"

	// StatusOpen marks a run whose packets may still be submitted and
	// finalized; StatusConsumed marks a finalized run kept for audit;
	// StatusInvalidated marks an open plan contradicted by a later targeted
	// P0/P1, so it can no longer submit or finalize.
	StatusOpen        = "open"
	StatusConsumed    = "consumed"
	StatusInvalidated = "invalidated"

	// Packet statuses. A packet is pending (no submission yet), accepted
	// (its report validated and recorded — terminal), or rejected (its last
	// submission failed validation; it can be re-submitted).
	PacketPending     = "pending"
	PacketAccepted    = "accepted"
	PacketRejected    = "rejected"
	PacketNotRequired = "not_required"

	// Packet kinds: a group of validate checks, one verify detection, one
	// conditional verify analysis, one reviewed file, or the cross-check.
	PacketKindChecks   = "checks"
	PacketKindItem     = "item"
	PacketKindAnalysis = "analysis"
	PacketKindFile     = "file"
	PacketKindCross    = "cross"

	// CrossKey is the reserved check key of the cross-check.
	CrossKey = "cross"

	// SourceDerived marks an entry resolved from the gate's protocol input
	// surface; SourceInput marks an entry declared by the agent via --input.
	SourceDerived = "derived"
	SourceInput   = "input"
	// SourceDerivedAffects marks a spec-derived evidence file that exists
	// because the target spec declares it in an acceptance item's
	// affects.files. Validate's read surface includes these files, so local
	// validate packets may read and declare them; they never define work
	// packets.
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

// PacketSpec is one planned work packet: the immutable plan record. The
// packet's mutable state (status, attempts, accepted report) lives in its own
// state file under the run directory.
type PacketSpec struct {
	PacketID  string   `json:"packet_id"`
	Kind      string   `json:"kind"`
	CheckKeys []string `json:"check_keys"`
	DependsOn []string `json:"depends_on,omitempty"`
	ReadRefs  []string `json:"read_refs,omitempty"`
}

// Finding is one mechanically identified finding. Local finding ids are
// assigned at submit time from the immutable packet id and report order.
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
	// recorded and routed to the owner's review, not blocking this run (see
	// framework/verification_scope.md §Gate Work Packets → Deferred findings).
	OwnedBy string `json:"owned_by,omitempty"`
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

// Scope is one dependency declaration parsed from a packet report.
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

// SeverityConfirmation is the cross or rule-validate executor's evidence-
// backed step in one finding's severity-confirmation sequence.
// OriginalSeverity must match the severity produced by the previous step (or
// the finding before synthesis for the first step). FinalSeverity is equal for
// confirmed records or one adjacent level away for adjusted records.
type SeverityConfirmation struct {
	FindingID        string `json:"finding_id"`
	Outcome          string `json:"outcome"` // confirmed | adjusted
	OriginalSeverity string `json:"original_severity"`
	FinalSeverity    string `json:"final_severity"`
	EvidencePath     string `json:"evidence_path"`
	Reason           string `json:"reason"`
}

// PacketResult is the immutable machine-readable interpretation of an
// accepted report. Later packets and gate-finalize consume this structure,
// never a coordinator-supplied restatement of the result.
type PacketResult struct {
	PacketID        string                 `json:"packet_id"`
	Kind            string                 `json:"kind"`
	Verdicts        map[string]string      `json:"verdicts,omitempty"`
	Scopes          []Scope                `json:"scopes,omitempty"`
	Findings        []Finding              `json:"findings,omitempty"`
	EffectiveStatus map[string]string      `json:"effective_status,omitempty"`
	Dispositions    []FindingDisposition   `json:"dispositions,omitempty"`
	SeverityChecks  []SeverityConfirmation `json:"severity_confirmations,omitempty"`
	Ownerships      []FindingOwnership     `json:"ownerships,omitempty"`
	Analysis        map[string]string      `json:"analysis,omitempty"`
	ReportDigest    string                 `json:"report_digest"`
}

// JudgmentBaseline is the cache-embedded semantic state used to materialize
// carried judgments for a delta/repair run.
type JudgmentBaseline struct {
	SchemaVersion   int               `json:"schema_version"`
	LogicalStatus   map[string]string `json:"logical_status"`
	Findings        []Finding         `json:"findings"`
	SynthesisDigest string            `json:"synthesis_digest"`
	// DeferredFindings records the run's findings whose ownership points at
	// another unit. They are audit state, not carry state: routing lives in the
	// deferred-findings ledger, and a later run of this unit must not re-dispose
	// them (see framework/validation_cache.md §Format → Deferred-findings ledger).
	DeferredFindings []Finding `json:"deferred_findings,omitempty"`
}

// DeferredFinding is one pending review finding routed to this run's unit by
// another unit's review synthesis. The plan loads it from the deferred-
// findings ledger, the cross synthesis must dispose it, and gate-finalize
// consumes the ledger entry it resolved (see framework/verification_scope.md
// §Gate Work Packets → Deferred findings).
type DeferredFinding struct {
	SourceUnit   string  `json:"source_unit"`
	SourceRun    string  `json:"source_run"`
	EvidencePath string  `json:"evidence_path"`
	Reason       string  `json:"reason"`
	Finding      Finding `json:"finding"`
}

// Attempt is one submission of a packet report. Every submission is recorded —
// accepted and rejected alike — so retry counts and replacement results are
// traceable.
type Attempt struct {
	Attempt         int    `json:"attempt"`
	SubmittedAt     string `json:"submitted_at"`
	Status          string `json:"status"` // accepted | rejected
	RejectionReason string `json:"rejection_reason,omitempty"`
	ResultDigest    string `json:"result_digest"`
}

// PacketState is the persisted mutable state of one packet. A missing state
// file means pending.
type PacketState struct {
	PacketID              string            `json:"packet_id"`
	Status                string            `json:"status"` // pending | accepted | rejected | not_required
	Attempts              []Attempt         `json:"attempts,omitempty"`
	Report                string            `json:"report,omitempty"` // the accepted report, verbatim
	Result                *PacketResult     `json:"packet_result,omitempty"`
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
	Deps  []string `json:"deps,omitempty"`
}

// Run is the persisted plan of one gate run: the immutable input snapshot,
// the deterministic packet plan, and the run mode.
type Run struct {
	RunID           string                 `json:"run_id"`
	Gate            string                 `json:"gate"`
	TargetKind      string                 `json:"target_kind"`
	TargetName      string                 `json:"target_name"`
	Target          string                 `json:"target"`
	CreatedAt       string                 `json:"created_at"`
	Status          string                 `json:"status"`
	Mode            string                 `json:"mode"`
	Refs            []Ref                  `json:"refs"`
	Surfaces        []Surface              `json:"surfaces"`
	ExtraInputs     []string               `json:"extra_inputs,omitempty"`
	RequiredFiles   []string               `json:"required_files,omitempty"`
	CarriedKeys     []string               `json:"carried_keys,omitempty"`
	CarriedEvidence []CarriedEvidenceEntry `json:"carried_evidence,omitempty"`
	RerunKeys       []string               `json:"rerun_keys,omitempty"`
	Packets         []PacketSpec           `json:"packets"`
	CarriedResults  []PacketResult         `json:"carried_results,omitempty"`
	// DeferredFindings are the pending deferrals this run's unit must dispose,
	// loaded from the deferred-findings ledger at plan time (review runs only).
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

func packetStatePath(repoRoot, runID, packetID string) (string, error) {
	if err := localstate.ValidateID(runID); err != nil {
		return "", err
	}
	return localstate.Path(repoRoot, runStateDir, runID, "packets", packetFileBase(packetID)+".json")
}

// packetFileBase maps the logical packet id to a fixed-length filename that is
// valid on every supported filesystem. The original id remains embedded in the
// packet state and is validated after loading.
func packetFileBase(packetID string) string {
	sum := sha256.Sum256([]byte(packetID))
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

// PacketByID returns the planned packet with the given id, or nil.
func (r *Run) PacketByID(packetID string) *PacketSpec {
	for i := range r.Packets {
		if r.Packets[i].PacketID == packetID {
			return &r.Packets[i]
		}
	}
	return nil
}

// validateGateTarget checks the supported gate/kind/target combinations.
func validateGateTarget(gate, targetKind, target string) error {
	switch gate {
	case GateValidate, GateVerify, GateReview:
	default:
		return fmt.Errorf("invalid gate %q: must be validate, verify, or review", gate)
	}
	switch targetKind {
	case TargetKindUnit:
	case TargetKindRule:
		if gate != GateValidate {
			return fmt.Errorf("rule targets support the validate gate only (rule verify and review have been removed) — got %q", gate)
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
	var candidate, stable string
	if targetKind == TargetKindUnit {
		candidate = specpaths.CandidateUnitSpecFileRef(targetName)
		stable = specpaths.StableUnitSpecFileRef(targetName)
	} else {
		candidate = specpaths.RuleCandidateFileRef(targetName)
		stable = specpaths.RuleStableFileRef(targetName)
	}
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
// inputs, generates the deterministic packet plan, and persists the run. Any
// previous run for the same (gate, target kind, target name, layer) is
// replaced. The packet plan and the input snapshot are fixed here, before any
// executor reads input.
func Plan(repoRoot, gate, targetKind, targetName, target, mode string, extraInputs, rerunKeys []string, now time.Time) (*Run, error) {
	// Validate the target name before the mutation lock touches the working
	// tree: an invalid name must fail without creating meta/ or a lock file
	// (tooling/README.md §Target names).
	if err := specpaths.ValidateTargetName(targetKind, targetName); err != nil {
		return nil, err
	}
	var planned *Run
	err := WithMutation(repoRoot, func() error {
		var err error
		planned, err = planUnlocked(repoRoot, gate, targetKind, targetName, target, mode, extraInputs, rerunKeys, now)
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
// a targeted P0/P1: the canonical cache transition plus every matching open
// run that was made unusable so it cannot overwrite the new recovery state.
type TargetedInvalidation struct {
	Cache             *validationcache.TargetedInvalidation
	InvalidatedRunIDs []string
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

func planUnlocked(repoRoot, gate, targetKind, targetName, target, mode string, extraInputs, rerunKeys []string, now time.Time) (*Run, error) {
	run, err := resolveRun(repoRoot, gate, targetKind, targetName, target, mode, extraInputs, rerunKeys)
	if err != nil {
		return nil, err
	}
	plan, carried, required, notices, err := buildPacketPlan(repoRoot, run)
	if err != nil {
		return nil, err
	}
	run.Packets = plan
	run.CarriedKeys = carried
	if len(carried) > 0 {
		carriedResults, cerr := loadCarriedResults(repoRoot, run, carried)
		if cerr != nil {
			return nil, cerr
		}
		run.CarriedResults = carriedResults
		carriedEvidence, cerr := loadCarriedEvidence(repoRoot, run, carried)
		if cerr != nil {
			return nil, cerr
		}
		run.CarriedEvidence = carriedEvidence
	}
	run.RequiredFiles = required
	run.Notices = notices

	// A review run consumes the unit's pending deferrals: findings another
	// unit's review routed here by recorded ownership. Loading them at plan
	// time makes them part of the run's immutable input — the cross synthesis
	// must dispose every one of them, exactly like a carried judgment.
	if gate == GateReview && targetKind == TargetKindUnit {
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
	return run, nil
}

// resolveRun validates the run request and resolves its input surface: the
// gate's derived refs and surfaces plus the agent-declared extra inputs. It
// performs no packet planning and writes no state, so Plan and the delta
// scope preview share exactly the same input resolution.
func resolveRun(repoRoot, gate, targetKind, targetName, target, mode string, extraInputs, rerunKeys []string) (*Run, error) {
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
	refs, surfaces, err := derive(repoRoot, gate, targetKind, targetName, target)
	if err != nil {
		return nil, err
	}
	run := &Run{
		Gate:       gate,
		TargetKind: targetKind,
		TargetName: targetName,
		Target:     target,
		Status:     StatusOpen,
		Mode:       mode,
		RerunKeys:  dedupeSorted(rerunKeys),
		Refs:       refs,
		Surfaces:   surfaces,
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
				return nil, fmt.Errorf("--input %q: %w", input, err)
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
			return nil, fmt.Errorf("--input %q: %v", input, err)
		}
		if info.IsDir() {
			if seenSurfaces[canonical] {
				continue
			}
			seenSurfaces[canonical] = true
			surface, err := resolveSurface(repoRoot, Surface{Path: canonical, Source: SourceInput})
			if err != nil {
				return nil, fmt.Errorf("--input %q: %w", input, err)
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
	// The derived refs and surfaces are validated inside derive; extra inputs
	// are appended afterwards, so re-validate the complete set. A logical
	// reference whose resolution escapes the repository must be rejected here,
	// before any run state is written (see framework/verification_scope.md
	// §Gate Work Packets → Input roles).
	if err := validateSnapshotPaths(repoRoot, run.Refs, run.Surfaces); err != nil {
		return nil, err
	}
	sortRefs(run.Refs)
	sortSurfaces(run.Surfaces)
	sort.Strings(run.ExtraInputs)
	return run, nil
}

// DeltaPreview is the read-only preview of a delta plan for a stale pass
// baseline: the raw stale evidence plus the exact re-run/carry split the
// planner would fix. fresh@ prints it so the DELTA SCOPE section reports the
// same scope gate-plan derives (see framework/verification_scope.md
// §Delta Runs).
type DeltaPreview struct {
	Scope      *validationcache.StaleScope // nil when the derivation degraded before reading stale evidence
	NewKeys    []string                    // current keys absent from the baseline declaration
	Rerun      []string                    // sorted effective re-run keys
	Carried    []string                    // sorted carried-over keys
	CoversFull bool                        // the re-run covers every declared check
	Degraded   bool                        // no scope could be derived — the plan is the full packet set
	Reason     string                      // degradation reason (degraded only)
	Notices    []string
	PlanError  string // the derivation refused (fresh cache or an unreadable baseline)
}

// PreviewDeltaScope derives the delta/repair scope for a stale baseline
// without writing run state: a pass baseline uses the delta derivation, a
// failure record uses the repair derivation — the same derivation gate-plan
// applies for that baseline, so the fresh report and the planner never
// disagree (see framework/verification_scope.md §Delta Runs). A refusal to
// derive (a fresh cache, a malformed baseline) is reported through
// DeltaPreview.PlanError instead of an error so the caller can still present
// the stale evidence it did read.
func PreviewDeltaScope(repoRoot, gate, targetKind, targetName, target string) (*DeltaPreview, error) {
	mode := ModeDelta
	if baseline, err := validationcache.ReadGateBaseline(repoRoot, targetKind, targetName, gate); err == nil && baseline.Exists && baselineFailureRecord(gate, baseline) {
		mode = ModeRepair
	}
	run, err := resolveRun(repoRoot, gate, targetKind, targetName, target, mode, nil, nil)
	if err != nil {
		return nil, err
	}
	derivation, err := deriveDeltaRerun(repoRoot, run)
	if err != nil {
		// The derivation refused (a fresh cache or an unreadable baseline).
		// The stale evidence is still useful to present, so re-read it for
		// the report and carry the refusal as PlanError.
		preview := &DeltaPreview{PlanError: err.Error()}
		if scope, scopeErr := validationcache.DeriveStaleScope(repoRoot, targetKind, targetName, gate); scopeErr == nil {
			preview.Scope = scope
		}
		return preview, nil
	}
	preview := &DeltaPreview{
		Scope:      derivation.scope,
		NewKeys:    derivation.newKeys,
		Rerun:      derivation.rerun,
		Carried:    derivation.carried,
		CoversFull: derivation.coversFull,
		Degraded:   derivation.degraded,
		Reason:     derivation.reason,
		Notices:    derivation.notices,
	}
	return preview, nil
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
	switch run.Status {
	case StatusOpen, StatusConsumed, StatusInvalidated:
	default:
		return nil, fmt.Errorf("gate run %s has invalid status %q", runID, run.Status)
	}
	if err := validateGateTarget(run.Gate, run.TargetKind, run.Target); err != nil {
		return nil, fmt.Errorf("gate run %s: %w", runID, err)
	}
	if len(run.Packets) == 0 {
		return nil, fmt.Errorf("gate run %s has no packet plan", runID)
	}
	if err := validatePacketPlan(run, run.Packets); err != nil {
		return nil, fmt.Errorf("gate run %s has an invalid packet plan: %w", runID, err)
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

// LoadPacketState reads one packet's state. A missing state file means the
// packet is still pending.
func LoadPacketState(repoRoot string, run *Run, packetID string) (*PacketState, error) {
	if run.PacketByID(packetID) == nil {
		return nil, fmt.Errorf("packet %q is not part of run %s's plan", packetID, run.RunID)
	}
	statePath, err := packetStatePath(repoRoot, run.RunID, packetID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return &PacketState{PacketID: packetID, Status: PacketPending}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read packet %q state: %w", packetID, err)
	}
	state := &PacketState{}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("cannot parse packet %q state: %w", packetID, err)
	}
	if state.PacketID != packetID || state.Status == "" {
		return nil, fmt.Errorf("packet %q state is missing required fields", packetID)
	}
	return state, nil
}

// SavePacketState persists one packet's state atomically. Packet state files
// are independent, so concurrent submissions of different packets never
// contend.
func SavePacketState(repoRoot string, run *Run, state *PacketState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode packet %s state: %w", state.PacketID, err)
	}
	data = append(data, '\n')
	statePath, err := packetStatePath(repoRoot, run.RunID, state.PacketID)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(statePath, data, 0644); err != nil {
		return fmt.Errorf("write packet %s state: %w", state.PacketID, err)
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
	refs, surfaces, err := derive(repoRoot, run.Gate, run.TargetKind, run.TargetName, run.Target)
	if err != nil {
		return []string{fmt.Sprintf("input surface no longer resolvable: %v", err)}, nil
	}
	// The derived part is re-derived; the agent-declared part (--input) is
	// re-resolved from the stored entries. The two parts are compared
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
			current, err := resolveSurface(repoRoot, surface)
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

// PacketAllowsDeclaration reports whether a path belongs to one packet's
// declared read surface. Run-wide snapshot membership is insufficient: a
// packet may not borrow evidence that was assigned only to another packet.
func (r *Run) PacketAllowsDeclaration(repoRoot string, packet *PacketSpec, declared string) bool {
	canonical := canonicalPath(repoRoot, declared)
	for _, allowed := range packet.ReadRefs {
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

// ------------------------------------------------------------
// Surface derivation
// ------------------------------------------------------------

// derive resolves the gate's protocol input surface for one target.
func derive(repoRoot, gate, targetKind, targetName, target string) ([]Ref, []Surface, error) {
	var refs []Ref
	var surfaces []Surface
	var err error
	switch targetKind {
	case TargetKindUnit:
		switch gate {
		case GateValidate:
			refs, err = deriveUnitValidate(repoRoot, targetName, target)
		case GateVerify, GateReview:
			refs, surfaces, err = deriveUnitCodeGate(repoRoot, targetName, target)
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
// layer) with their protocol appendices, the bound and global rules, and the
// spec's affects.files entries.
func deriveUnitValidate(repoRoot, unitName, target string) ([]Ref, error) {
	var unitMain string
	if target == TargetCandidate {
		unitMain = specpaths.CandidateUnitSpecFileRef(unitName)
	} else {
		unitMain = specpaths.StableUnitSpecFileRef(unitName)
	}
	content, err := readSpecContent(repoRoot, unitMain)
	if err != nil {
		return nil, err
	}
	var refs []Ref
	refs = append(refs, physicalRef(repoRoot, unitMain, SourceDerived))
	for _, appendix := range unitAppendices(repoRoot, unitName, target) {
		refs = append(refs, physicalRef(repoRoot, appendix, SourceDerived))
	}
	for _, dep := range parseRefList(content, "unit_refs", unitName) {
		refs = append(refs, logicalUnitRef(repoRoot, dep, SourceDerived))
		for _, appendixRef := range logicalUnitAppendixRefs(repoRoot, dep) {
			refs = append(refs, appendixRef)
		}
	}
	ruleIDs := append(parseRefList(content, "rule_refs", ""), globalRuleIDs(repoRoot)...)
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

// deriveUnitCodeGate resolves a verify/review unit run's inputs: the unit's
// own spec files in the target layer plus the declared code surface
// (implementation_surface directories expanded to their repository-content
// files + affects.files). It fails closed before any run state is written on
// a spec with no acceptance items or an unresolvable declared surface.
func deriveUnitCodeGate(repoRoot, unitName, target string) ([]Ref, []Surface, error) {
	var unitMain string
	if target == TargetCandidate {
		unitMain = specpaths.CandidateUnitSpecFileRef(unitName)
	} else {
		unitMain = specpaths.StableUnitSpecFileRef(unitName)
	}
	content, err := readSpecContent(repoRoot, unitMain)
	if err != nil {
		return nil, nil, err
	}
	// Fail closed before any run state is written: a verify/review run's work
	// set is the acceptance items, so an empty item set has no verifiable
	// object — the packet plan would carry only the cross packet and could
	// finalize a pass cache with no code evidence at all. The same
	// precondition is enforced by mechanical validate Check 2.
	if len(specvalidation.ExtractAcceptanceItemIDs(content)) == 0 {
		return nil, nil, fmt.Errorf("declared acceptance item set is empty — add at least one acceptance item before planning a verify/review run")
	}
	var refs []Ref
	refs = append(refs, physicalRef(repoRoot, unitMain, SourceDerived))
	for _, appendix := range unitAppendices(repoRoot, unitName, target) {
		refs = append(refs, physicalRef(repoRoot, appendix, SourceDerived))
	}
	surfaces, err := codeSurfaces(repoRoot, content)
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
	for _, unitName := range allUnitNames(repoRoot) {
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
func codeSurfaces(repoRoot, specContent string) ([]Surface, error) {
	// Fail closed before expanding anything: a non-<pending>
	// implementation_surface that yields no file — an unresolvable path, or a
	// directory with no repository-content files — would expand to zero
	// entries, producing a plan (and later a pass cache) with no code evidence
	// at all. The same check runs in mechanical validate Check 3.
	if problems := specvalidation.CheckImplementationSurfaces(repoRoot, specContent); len(problems) > 0 {
		return nil, fmt.Errorf("declared implementation_surface cannot resolve to a code file — fix the acceptance items before planning a verify/review run: %s", specvalidation.FormatSurfaceProblems(problems))
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
		surface, err := resolveSurface(repoRoot, Surface{Path: p, Source: SourceDerived})
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
func resolveSurface(repoRoot string, surface Surface) (Surface, error) {
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
		files, err := repofiles.ExpandDir(repoRoot, surface.Path)
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
func unitAppendices(repoRoot, unitName, layer string) []string {
	pattern := fmt.Sprintf("docs/specs/units/%s/appendix/unit_%s_*.md", layer, unitName)
	matches, err := filepath.Glob(filepath.Join(repoRoot, filepath.FromSlash(pattern)))
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range matches {
		rel, relErr := filepath.Rel(repoRoot, m)
		if relErr != nil {
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out
}

// allUnitNames lists every unit with a main spec in either layer.
func allUnitNames(repoRoot string) []string {
	seen := map[string]bool{}
	for _, dir := range []string{specpaths.CandidateDir, specpaths.StableDir} {
		matches, err := filepath.Glob(filepath.Join(repoRoot, filepath.FromSlash(dir), "unit_*.md"))
		if err != nil {
			continue
		}
		for _, m := range matches {
			base := strings.TrimSuffix(filepath.Base(m), ".md")
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
	return out
}

// globalRuleIDs lists the active global rule (g_rule_*) ids from the stable
// layer. Candidate global rules are unpublished and do not constrain units.
func globalRuleIDs(repoRoot string) []string {
	seen := map[string]bool{}
	matches, err := filepath.Glob(filepath.Join(repoRoot, filepath.FromSlash(specpaths.RuleStableDir), "g_rule_*.md"))
	if err == nil {
		for _, match := range matches {
			id := strings.TrimSuffix(filepath.Base(match), ".md")
			if id != "" {
				seen[id] = true
			}
		}
	}
	var out []string
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
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
func logicalUnitAppendixRefs(repoRoot, unitName string) []Ref {
	var bases []string
	for _, layer := range []string{TargetCandidate, TargetStable} {
		for _, file := range unitAppendices(repoRoot, unitName, layer) {
			base := strings.TrimSuffix(filepath.Base(file), ".md")
			bases = append(bases, base)
		}
	}
	var refs []Ref
	for _, base := range dedupeSorted(bases) {
		refs = append(refs, refreshRef(repoRoot, Ref{Ref: "unit:" + unitName + ":appendix:" + base, Source: SourceDerived}))
	}
	return refs
}

// logicalRuleRef builds a rule logical ref (`rule:{id}`).
func logicalRuleRef(repoRoot, ruleID, source string) Ref {
	return refreshRef(repoRoot, Ref{Ref: "rule:" + ruleID, Source: source})
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

// parseRefList reads a frontmatter ref list (unit_refs / rule_refs), strips
// any @version suffix, drops the given self name, dedupes, and sorts.
func parseRefList(content, field, self string) []string {
	fm := specpaths.ReadFrontmatterStringMap(content)
	raw := strings.TrimSpace(fm[field])
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, ref := range specpaths.ParseRefList(raw) {
		ref = stripVersion(strings.TrimSpace(ref))
		if ref == "" || ref == self || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func stripVersion(ref string) string {
	if at := strings.LastIndex(ref, "@"); at > 0 {
		return ref[:at]
	}
	return ref
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
// against the packet's read refs.
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

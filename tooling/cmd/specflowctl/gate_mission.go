package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
)

// runGateMission materializes the read-only reviewer mission for an
// computed coverage key batch. Public missions claim their fixed batch under the repository lock;
// mission generation never persists an accepted session. With --final it builds the optional final cross synthesis instead.
func runGateMission(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-mission", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id printed by gate-plan")
	keysPtr := fs.String("keys", "", "comma-separated coverage keys assigned to this session")
	finalPtr := fs.Bool("final", false, "build the final cross synthesis mission instead of a coverage batch")
	formatPtr := fs.String("format", "prompt", "output format: prompt | json")
	outPtr := fs.String("out", "", "write the mission to this file instead of stdout (a path outside the repository, or under meta/gate_runs/<run>/)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runID := strings.TrimSpace(*runIDPtr)
	if runID == "" {
		return errors.New("--run is required")
	}
	if *formatPtr != "prompt" && *formatPtr != "json" {
		return fmt.Errorf("invalid --format %q: must be prompt or json", *formatPtr)
	}
	if *finalPtr && strings.TrimSpace(*keysPtr) != "" {
		return errors.New("--final and --keys are mutually exclusive")
	}
	absRoot := mustAbs(*repoRootPtr)
	run, err := gaterun.Load(absRoot, runID)
	if err != nil {
		return err
	}
	if run.Status != gaterun.StatusOpen {
		return fmt.Errorf("gate run %s is %s — only an open run can materialize a mission; plan a new run", run.RunID, run.Status)
	}
	if run.Escalated {
		return escalatedRunError(run)
	}
	keys := []string{gaterun.CrossKey}
	if !*finalPtr {
		keys = splitKeys(*keysPtr)
		if len(keys) == 0 {
			return errors.New("--keys is required (or use --final)")
		}
	}
	if err := gaterun.WithMutation(absRoot, func() error {
		current, err := gaterun.Load(absRoot, runID)
		if err != nil {
			return err
		}
		if current.Status != gaterun.StatusOpen {
			return fmt.Errorf("gate run %s is %s", runID, current.Status)
		}
		// Recheck escalation under the lock: a review submission that returned
		// escalate-full abandons the run, so no mission may be materialized or
		// claimed for it even when the review landed after the pre-lock check.
		if current.Escalated {
			return escalatedRunError(current)
		}
		if _, err := gaterun.BuildSessionSpec(absRoot, current, keys); err != nil {
			return err
		}
		run = current
		return gaterun.ClaimShared(absRoot, run, keys)
	}); err != nil {
		return err
	}
	spec, err := gaterun.BuildSessionSpec(absRoot, run, keys)
	if err != nil {
		return err
	}
	// The mission and the submit side enforce the same coverage facts from
	// the same CoverageProgress source, so a mission can never claim a
	// readiness its run does not have: a coverage batch refuses already
	// accepted keys, and the final synthesis refuses an uncovered run.
	states, cerr := gaterun.LoadSessionStates(absRoot, run)
	if cerr != nil {
		return cerr
	}
	covered, uncovered, cerr := gaterun.CoverageProgress(run, states)
	if cerr != nil {
		return cerr
	}
	if spec.Kind == gaterun.SessionKindCross {
		if len(uncovered) > 0 {
			return fmt.Errorf("final synthesis is not ready — %d coverage key(s) are still uncovered; complete and accept the dependency sessions first", len(uncovered))
		}
	} else {
		for _, key := range keys {
			if owner, ok := covered[key]; ok {
				return fmt.Errorf("coverage key %q is already covered by accepted session %q — its judgment is terminal", key, owner)
			}
		}
	}
	state, err := gaterun.LoadSessionState(absRoot, run, spec.SessionID)
	if err != nil {
		return err
	}
	if state.Status != gaterun.SessionPending && state.Status != gaterun.SessionRejected {
		return fmt.Errorf("session %q is %s — only a pending or rejected session can receive a mission", spec.SessionID, state.Status)
	}
	mission, err := buildGateMission(absRoot, run, spec, state)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if *formatPtr == "json" {
		if err := writeGateJSON(&buf, mission); err != nil {
			return err
		}
	} else {
		writeGatePrompt(&buf, mission)
	}
	// A mission file keeps the mission text out of the coordinator's context:
	// the coordinator passes the reviewer the path, and the reviewer reads the
	// file in full (framework/verification_scope.md §Coverage Model).
	if out := strings.TrimSpace(*outPtr); out != "" {
		absOut := mustAbs(out)
		if err := validateMissionOutPath(absRoot, run.RunID, absOut); err != nil {
			return err
		}
		if err := os.WriteFile(absOut, buf.Bytes(), 0o644); err != nil {
			return fmt.Errorf("write mission file: %w", err)
		}
		fmt.Fprintf(stdout, "mission written: %s (%d bytes, session %s)\n", absOut, buf.Len(), spec.SessionID)
		return nil
	}
	_, err = stdout.Write(buf.Bytes())
	return err
}

// validateMissionOutPath enforces the mission scratch-file location rule
// (framework/verification_scope.md §Coverage Model): a mission file must not be
// repository content, so it lives either outside the repository or inside the
// run's own state directory meta/gate_runs/<run_id>/, which the orphan sweep
// removes with the run.
func validateMissionOutPath(repoRoot, runID, absOut string) error {
	repoAbs := mustAbs(repoRoot)
	rel, err := filepath.Rel(repoAbs, absOut)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil // outside the repository — allowed
	}
	relSlash := filepath.ToSlash(rel)
	want := "meta/gate_runs/" + runID
	if relSlash == want || strings.HasPrefix(relSlash, want+"/") {
		return nil
	}
	return fmt.Errorf("--out inside the repository must stay under the run's own state directory %s/ — a mission file must not be repository content: %s", want, relSlash)
}

// splitKeys parses a comma-separated key list, trimming blanks.
func splitKeys(raw string) []string {
	var keys []string
	for _, tok := range strings.Split(raw, ",") {
		if key := strings.TrimSpace(tok); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// deferredFindingsForSession selects the pending deferrals a session executor
// must see: the final cross session receives every pending deferral (its
// synthesis disposes them all); a file session receives the deferrals whose
// affected keys name the file it reviews.
func deferredFindingsForSession(run *gaterun.Run, spec *gaterun.SessionSpec) []gaterun.DeferredFinding {
	if len(run.DeferredFindings) == 0 {
		return nil
	}
	switch spec.Kind {
	case gaterun.SessionKindCross:
		return run.DeferredFindings
	case gaterun.SessionKindDesign:
		var out []gaterun.DeferredFinding
		for _, deferred := range run.DeferredFindings {
			if deferredCoversKeys(deferred, spec.CheckKeys) {
				out = append(out, deferred)
			}
		}
		return out
	default:
		return nil
	}
}

// deferredCoversKeys reports whether a deferred finding affects one of the
// session's keys.
func deferredCoversKeys(deferred gaterun.DeferredFinding, keys []string) bool {
	for _, key := range keys {
		if key == deferred.Finding.SourceKey {
			return true
		}
		for _, affected := range deferred.Finding.AffectedKeys {
			if affected == key {
				return true
			}
		}
	}
	return false
}

func indentSessionContext(value, prefix string) string {
	value = strings.TrimRight(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	if value == "" {
		return prefix
	}
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}

// missionDependency is one compact judgment record in a session mission: the
// consumer-scoped projection of an accepted session result (dependency
// results) or of a carried baseline judgment (carried results). It carries the
// verdicts, findings with their stored details, and analysis fields the
// consuming session judges over — never the verbatim accepted report, which
// stays in run state and is assembled into the published cache body. Only the
// design consumer also receives the public observations it must dispose; every
// other consumer, including the final cross synthesis, receives judgments only.
type missionDependency struct {
	SessionID       string            `json:"session_id"`
	Kind            string            `json:"kind,omitempty"`
	Status          string            `json:"status"`
	Digest          string            `json:"digest,omitempty"`
	Verdicts        map[string]string `json:"verdicts,omitempty"`
	Findings        []gaterun.Finding `json:"findings,omitempty"`
	Observations    []gaterun.Finding `json:"observations,omitempty"`
	Analysis        map[string]string `json:"analysis,omitempty"`
	EffectiveStatus map[string]string `json:"effective_status,omitempty"`
}

type missionTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Source     string `json:"source"`
}

type missionReadInput struct {
	Ref      string `json:"ref"`
	Resolved string `json:"resolved,omitempty"`
}

type missionSession struct {
	SessionID        string                    `json:"session_id"`
	Kind             string                    `json:"kind"`
	Mission          string                    `json:"mission"`
	LastRejection    string                    `json:"last_rejection,omitempty"`
	CheckKeys        []string                  `json:"check_keys"`
	Relationships    []string                  `json:"relationships"`
	ReadRefs         []string                  `json:"read_refs"`
	ReadInputs       []missionReadInput        `json:"read_inputs"`
	DependsOn        []string                  `json:"depends_on"`
	Context          []string                  `json:"context,omitempty"`
	ChangeSet        []string                  `json:"change_set,omitempty"`
	Standing         []string                  `json:"standing_conclusions,omitempty"`
	Dependencies     []missionDependency       `json:"dependency_results"`
	CarriedResults   []missionDependency       `json:"carried_results"`
	DeferredFindings []gaterun.DeferredFinding `json:"deferred_findings"`
	ProtocolRef      string                    `json:"protocol_ref"`
	ProtocolScope    string                    `json:"protocol_scope"`
	ProtocolInline   []protocolChunk           `json:"protocol_inline"`
	ReportContract   gateReportContract        `json:"report_contract"`
}

type gateMission struct {
	SchemaVersion int              `json:"schema_version"`
	RunID         string           `json:"run_id"`
	Gate          string           `json:"gate"`
	TargetKind    string           `json:"target_kind"`
	TargetName    string           `json:"target_name"`
	Target        string           `json:"target"`
	Mode          string           `json:"mode"`
	SpecSource    string           `json:"spec_source"`
	Sessions      []missionSession `json:"sessions"`
	Constraints   []string         `json:"constraints"`
	Glossary      []missionTerm    `json:"glossary"`
	FailurePath   string           `json:"failure_path"`
	Submission    string           `json:"submission_command"`
}

func buildGateMission(root string, run *gaterun.Run, spec *gaterun.SessionSpec, state *gaterun.SessionState) (gateMission, error) {
	if len(run.RequiredFiles) == 0 {
		return gateMission{}, fmt.Errorf("gate run %s has no spec source", run.RunID)
	}
	layout, err := specflowlayout.Resolve(root)
	if err != nil {
		return gateMission{}, err
	}
	protocol := ""
	switch {
	case spec.Kind == gaterun.SessionKindDeltaReview || spec.Kind == gaterun.SessionKindCross:
		// These sessions do not judge by the gate checklist: their judging
		// standard lives in verification_scope.md (the final synthesis also
		// judges severity_policy.md, named per inlined section below).
		protocol = "verification_scope.md"
	case run.Gate == gaterun.GateValidate && run.TargetKind == gaterun.TargetKindRule:
		protocol = "rule_validate_checklist.md"
	case run.Gate == gaterun.GateValidate:
		protocol = "unit_validate_checklist.md"
	case run.Gate == gaterun.GateVerify:
		protocol = "unit_verify_checklist.md"
	default:
		return gateMission{}, fmt.Errorf("unsupported gate %q", run.Gate)
	}
	session := missionSession{
		SessionID: spec.SessionID, Kind: spec.Kind, Mission: missionTextFor(spec.Kind),
		CheckKeys:        append([]string{}, spec.CheckKeys...),
		Relationships:    append([]string{}, spec.Relationships...),
		ReadRefs:         append([]string{}, spec.ReadRefs...),
		ReadInputs:       missionInputs(run, spec),
		DependsOn:        append([]string{}, spec.DependsOn...),
		Context:          append([]string{}, spec.Context...),
		Dependencies:     []missionDependency{},
		CarriedResults:   []missionDependency{},
		DeferredFindings: append([]gaterun.DeferredFinding{}, deferredFindingsForSession(run, spec)...),
		ProtocolRef:      specflowlayout.Relative(layout.FrameworkRoot, protocol),
		ProtocolScope:    protocolScopeFor(spec.Kind, spec.CheckKeys),
		ReportContract:   reportContractFor(run, spec),
	}
	// The mission is self-contained: the protocol sections this session judges
	// by are inlined from the deployed framework documents. Extraction is
	// fail-closed — a section the documents no longer provide aborts the
	// mission instead of degrading it back to a document pointer.
	sections, err := protocolSectionsFor(run.Gate, run.TargetKind, spec.Kind, spec.CheckKeys)
	if err != nil {
		return gateMission{}, err
	}
	session.ProtocolInline, err = inlineProtocol(root, layout.FrameworkRoot, sections)
	if err != nil {
		return gateMission{}, err
	}
	// The scope line advertises the layer resolution section when the session's
	// inlined standard carries it: the validate checklist inlines it, and a
	// session that judges by another document (delta review, final synthesis)
	// does not. Deriving it from the inlined sections keeps the advertised
	// scope aligned with the text the reviewer actually receives.
	for _, section := range sections {
		if section.Heading == "Target-layer applicability" {
			session.ProtocolScope = "Target-layer applicability and " + session.ProtocolScope
			break
		}
	}
	// A run with a review session (delta/repair, or a full run preserving
	// standing relationship conclusions) shows every session the mechanically
	// detected change set and the standing conclusions it must judge against.
	if run.ChangeReport != nil {
		for _, entry := range run.ChangeReport.Entries {
			switch entry.Kind {
			case "changed":
				for _, c := range entry.Changes {
					session.ChangeSet = append(session.ChangeSet, fmt.Sprintf("%s: %s lines %d-%d", entry.Path, c.Kind, c.StartLine, c.EndLine))
				}
			default:
				session.ChangeSet = append(session.ChangeSet, entry.Path+": "+entry.Kind)
			}
		}
		for _, key := range run.BaselineKeys {
			session.Standing = append(session.Standing, key+": "+run.BaselineStatus[key])
		}
	}
	if state.Status == gaterun.SessionRejected {
		if len(state.Attempts) == 0 || strings.TrimSpace(state.Attempts[len(state.Attempts)-1].RejectionReason) == "" {
			return gateMission{}, fmt.Errorf("rejected session %q has no rejection reason", spec.SessionID)
		}
		session.LastRejection = state.Attempts[len(state.Attempts)-1].RejectionReason
	}
	framework := layout.FrameworkRoot
	glossary := []missionTerm{
		{"session", "one independently reviewed batch of coverage keys in a gate run", framework + "/verification_scope.md §Coverage model"},
		{"check_keys", "the checks this session must report, excluding carried checks", framework + "/verification_scope.md §Session record"},
		{"read_refs", "the exact evidence files this session may read", framework + "/verification_scope.md §Session record"},
		{"input surface", "every file a run records as evidence; each cache entry holds the whole-file hash and ordered chunk sequence", framework + "/validation_cache.md §Format"},
	}
	if spec.Kind == gaterun.SessionKindCross {
		glossary = append(glossary,
			missionTerm{"carried judgments", "baseline judgments preserved without re-execution in a delta or repair run", framework + "/verification_scope.md §Delta Runs"},
			missionTerm{"finding disposition", "cross decision to retain, suppress, or merge each input finding", framework + "/verification_scope.md §Session report contract"},
			missionTerm{"effective status", "cross result for every logical check and cross itself", framework + "/verification_scope.md §Session report contract"},
		)
		if len(crossItemsFor(run)) > 0 {
			glossary = append(glossary, missionTerm{"Cross item finding", "the new retained cross finding that explains one failed fixed cross item", framework + "/verification_scope.md §Session report contract"})
		}
	}
	if len(session.DeferredFindings) > 0 {
		glossary = append(glossary, missionTerm{"deferred finding", "a finding routed from another unit for this verify run to dispose (quality ownership)", framework + "/verification_scope.md §Deferred findings"})
	}
	if spec.Kind == gaterun.SessionKindCross {
		// The optional final synthesis consumes every accepted session's
		// compact judgment record, plus the carried baseline judgments below.
		// It disposes findings, not public observations: per-file facts belong
		// to the design sessions that consume them.
		states, serr := gaterun.LoadSessionStates(root, run)
		if serr != nil {
			return gateMission{}, serr
		}
		for _, dep := range states {
			if dep.Status != gaterun.SessionAccepted || dep.SessionID == gaterun.CrossKey || dep.Result == nil {
				continue
			}
			session.Dependencies = append(session.Dependencies, missionJudgmentFor(dep.SessionID, dep.Status, dep.Result))
		}
	}
	for _, dep := range spec.DependsOn {
		state, err := gaterun.LoadSessionState(root, run, dep)
		if err != nil {
			return gateMission{}, err
		}
		if state.Status != gaterun.SessionAccepted && state.Status != gaterun.SessionNotRequired {
			return gateMission{}, fmt.Errorf("session %q is not ready: dependency %q is %s", spec.SessionID, dep, state.Status)
		}
		if state.Status == gaterun.SessionAccepted && state.Result == nil {
			return gateMission{}, fmt.Errorf("dependency %q has no accepted result", dep)
		}
		if spec.Kind != gaterun.SessionKindDesign {
			entry := missionJudgmentFor(dep, state.Status, state.Result)
			session.Dependencies = append(session.Dependencies, entry)
		}
	}
	if spec.Kind == gaterun.SessionKindDesign {
		for _, key := range spec.CheckKeys {
			ck := run.CoverageByKey(key)
			if ck == nil || ck.Kind != gaterun.SessionKindDesign {
				continue
			}
			if stringInList(spec.CheckKeys, gaterun.DesignPublicKey(*ck)) {
				// Co-batched: the reviewer collects the public facts itself.
				continue
			}
			result, err := gaterun.PublicResultForDesignKey(root, run, *ck)
			if err != nil {
				return gateMission{}, err
			}
			if stringInList(run.CarriedKeys, result.SessionID) {
				session.CarriedResults = append(session.CarriedResults, missionPublicRecordFor(result.SessionID, "", &result))
			} else {
				session.Dependencies = append(session.Dependencies, missionPublicRecordFor(result.SessionID, gaterun.SessionAccepted, &result))
			}
		}
	}
	if spec.Kind == gaterun.SessionKindCross {
		for i := range run.CarriedResults {
			session.CarriedResults = append(session.CarriedResults, missionJudgmentFor(run.CarriedResults[i].SessionID, "", &run.CarriedResults[i]))
		}
	}
	constraints := []string{"independent read-only reviewer session without the author's context", "the read surface is derived deterministically from the assigned keys; judge the same evidence regardless of execution order", "read files, search by pattern, and run read-only commands only (git queries and the framework inspection subcommands the checks instruct: specflowctl deps, specflowctl surfaces, specflowctl validate candidate)", "do not modify files, run state-changing commands, or launch sub-agents", "report evidence only from session read_refs; the inlined protocol is instruction, not evidence", "the main agent collects verdicts verbatim and does not re-litigate them"}
	// The over-capacity declaration is the recovery signal for a batch that can
	// be split. A single-key batch is atomic, so the mission does not offer the
	// escape and the session must carry its report (see
	// framework/verification_scope.md §Session capacity).
	if spec.Splittable() {
		constraints = append(constraints, "if this assigned batch exceeds what one session can carry, submit the single line `Session over capacity: <what exceeded>` instead of a partial report — the batch is then split into smaller batches, so never judge only part of it and never guess")
	}
	if run.Gate == gaterun.GateVerify && (spec.Kind == gaterun.SessionKindItem || spec.Kind == gaterun.SessionKindUnit || spec.Kind == gaterun.SessionKindCode) {
		constraints = append(constraints, "if a required test, caller, callee, or dependency file is missing from read_refs, return `Verification could not complete — missing read ref: <repo-relative path>`; do not judge from incomplete context or submit a verdict")
	}
	schema := 3
	if run.Gate == gaterun.GateVerify {
		schema = 4
	}
	return gateMission{
		SchemaVersion: schema, RunID: run.RunID, Gate: run.Gate,
		TargetKind: run.TargetKind, TargetName: run.TargetName, Target: run.Target, Mode: run.Mode,
		SpecSource:  run.RequiredFiles[0],
		Sessions:    []missionSession{session},
		Constraints: constraints,
		Glossary:    glossary,
		FailurePath: failureLineFor(run.Gate),
		Submission:  fmt.Sprintf("specflowctl gate-submit --run %s --session %s --keys %s --report REPORT_PATH", run.RunID, spec.SessionID, strings.Join(spec.Keys, ",")),
	}, nil
}

func writeGatePrompt(w io.Writer, mission gateMission) {
	p := mission.Sessions[0]
	fmt.Fprintf(w, "You are the independent read-only reviewer for %s@%s (%s), session %s of run %s.\n", mission.Gate, mission.TargetName, mission.Target, p.SessionID, mission.RunID)
	fmt.Fprintf(w, "Mission: %s The main agent will submit your report verbatim; follow the report contract below.\n", p.Mission)
	fmt.Fprintf(w, "Why now: this session is ready in the %s run; every dependency is accepted or not required.\n", mission.Mode)
	if p.LastRejection != "" {
		fmt.Fprintf(w, "Previous submission was rejected: %s. Re-evaluate the session and return a complete corrected report.\n", p.LastRejection)
	}

	fmt.Fprintf(w, "Run: %s\nGate: %s\nTarget kind: %s\nTarget name: %s\nTarget layer: %s\nSession: %s\nKind: %s\nChecks: %s\nMode: %s\n", mission.RunID, mission.Gate, mission.TargetKind, mission.TargetName, mission.Target, p.SessionID, p.Kind, strings.Join(p.CheckKeys, ", "), mission.Mode)
	fmt.Fprintf(w, "Spec source: %s\n", mission.SpecSource)
	if len(p.ChangeSet) > 0 {
		fmt.Fprintln(w, "Change set (mechanically detected; complete for the input surface):")
		for _, line := range p.ChangeSet {
			fmt.Fprintf(w, "  - %s\n", line)
		}
	}
	if len(p.Standing) > 0 {
		if p.Kind == gaterun.SessionKindDeltaReview {
			fmt.Fprintln(w, "Standing conclusions (accepted) — these keys are the accepted `Recheck:` values, written exactly as shown:")
		} else {
			fmt.Fprintln(w, "Standing conclusions (accepted):")
		}
		for _, line := range p.Standing {
			fmt.Fprintf(w, "  - %s\n", line)
		}
	}
	if p.Kind == gaterun.SessionKindCross {
		fmt.Fprintf(w, "Assigned relationships: %s\n", strings.Join(p.Relationships, ", "))
	}
	fmt.Fprintf(w, "Protocol scope: %s. The protocol text below is this session's inlined judging standard — judge from this text; each section's source document is provenance only, do not re-read it.\n", p.ProtocolScope)
	for _, chunk := range p.ProtocolInline {
		fmt.Fprintf(w, "----- Inlined protocol from %s (%s) -----\n", chunk.Source, chunk.Heading)
		fmt.Fprintln(w, chunk.Content)
		fmt.Fprintf(w, "----- End inlined protocol from %s -----\n", chunk.Source)
	}
	fmt.Fprintln(w, "Read refs:")
	for _, input := range p.ReadInputs {
		fmt.Fprintf(w, "  - %s", input.Ref)
		if input.Resolved != "" && input.Resolved != input.Ref {
			fmt.Fprintf(w, " -> %s", input.Resolved)
		}
		fmt.Fprintln(w)
	}
	if len(p.Context) > 0 {
		fmt.Fprintln(w, "Session context (plan-time facts; state these verbatim where the report contract requires):")
		for _, line := range p.Context {
			fmt.Fprintf(w, "  - %s\n", line)
		}
	}
	if len(p.Dependencies) > 0 {
		fmt.Fprintln(w, "Dependency judgments (accepted results this session consumes):")
		for _, dep := range p.Dependencies {
			writeMissionJudgment(w, dep)
		}
	}
	if len(p.CarriedResults) > 0 {
		fmt.Fprintln(w, "Carried judgments:")
		for _, result := range p.CarriedResults {
			writeMissionJudgment(w, result)
		}
	}
	if len(p.DeferredFindings) > 0 {
		fmt.Fprintln(w, "Pending deferred findings (dispose each in cross synthesis):")
		for _, entry := range p.DeferredFindings {
			data, _ := json.MarshalIndent(entry, "    ", "  ")
			fmt.Fprintf(w, "  - %s\n", data)
		}
	}
	fmt.Fprintln(w, "Constraints:")
	for _, constraint := range mission.Constraints {
		fmt.Fprintf(w, "  - %s\n", constraint)
	}
	fmt.Fprintln(w, "Glossary:")
	for _, term := range mission.Glossary {
		fmt.Fprintf(w, "  - %s — %s (%s)\n", term.Term, term.Definition, term.Source)
	}
	fmt.Fprintln(w, "Required report lines and values:")
	for _, v := range p.ReportContract.Verdicts {
		fmt.Fprintf(w, "  - %s: %s; reason required for: %s\n", v.Line, strings.Join(v.Allowed, " | "), strings.Join(v.ReasonRequiredFor, ", "))
	}
	for _, rule := range p.ReportContract.Requirements {
		fmt.Fprintf(w, "  - %s (%s; count %d..%d", rule.ID, rule.When, rule.MinCount, rule.MaxCount)
		if rule.CountBasis != "" {
			fmt.Fprintf(w, " by %s", rule.CountBasis)
		}
		if len(rule.Allowed) > 0 {
			fmt.Fprintf(w, "; allowed %s", strings.Join(rule.Allowed, " | "))
		}
		fmt.Fprintf(w, "): %s\n", rule.Description)
	}
	fmt.Fprintln(w, "Report template (replace placeholders; include conditional lines only when applicable):")
	fmt.Fprintln(w, p.ReportContract.Template)
	fmt.Fprintf(w, "If you cannot complete: %s\n", mission.FailurePath)
	fmt.Fprintln(w, "Return the report text only. The main agent will submit it with:")
	fmt.Fprintln(w, mission.Submission)
}

// missionJudgmentFor projects an accepted session result into the compact
// judgment record a mission carries. A nil result (a not-required conditional
// dependency) yields the identity fields only. The verbatim accepted report is
// never part of the projection — a mission carries judgments, not the
// reviewers' narratives; evidence is read from the session's read refs. Public
// observations are excluded: only the design consumer disposes them.
func missionJudgmentFor(sessionID, status string, result *gaterun.SessionResult) missionDependency {
	return missionJudgment(sessionID, status, result, false)
}

// missionPublicRecordFor projects the public record a design session consumes:
// the same compact judgment record plus the observations that session must
// dispose. Findings still carry their own details, so the cross consumer loses
// no finding evidence when it receives judgments only.
func missionPublicRecordFor(sessionID, status string, result *gaterun.SessionResult) missionDependency {
	return missionJudgment(sessionID, status, result, true)
}

func missionJudgment(sessionID, status string, result *gaterun.SessionResult, observations bool) missionDependency {
	entry := missionDependency{SessionID: sessionID, Status: status}
	if result == nil {
		return entry
	}
	entry.Kind = result.Kind
	entry.Digest = result.ReportDigest
	entry.Verdicts = result.Verdicts
	entry.Findings = result.Findings
	if observations {
		entry.Observations = result.Observations
	}
	entry.Analysis = result.Analysis
	entry.EffectiveStatus = result.EffectiveStatus
	return entry
}

// writeMissionJudgment renders one compact judgment record: the session header
// line, its verdicts or effective statuses, analysis fields, and every finding
// with its stored detail block. Map keys are sorted so the mission text is
// deterministic for a given run state.
func writeMissionJudgment(w io.Writer, dep missionDependency) {
	header := dep.SessionID
	if dep.Status != "" {
		header += ": " + dep.Status
	}
	var meta []string
	if dep.Kind != "" {
		meta = append(meta, "kind "+dep.Kind)
	}
	if dep.Digest != "" {
		meta = append(meta, "digest "+dep.Digest)
	}
	if len(meta) > 0 {
		header += " (" + strings.Join(meta, ", ") + ")"
	}
	fmt.Fprintf(w, "  - %s\n", header)
	for _, f := range dep.Observations {
		fmt.Fprintf(w, "  Observation %s [%s]: %s\n%s\n", f.ID, f.Severity, f.Text, f.Detail)
	}
	if len(dep.Verdicts) > 0 {
		fmt.Fprintf(w, "    verdicts: %s\n", joinSortedPairs(dep.Verdicts))
	}
	if len(dep.EffectiveStatus) > 0 {
		fmt.Fprintf(w, "    effective status: %s\n", joinSortedPairs(dep.EffectiveStatus))
	}
	if len(dep.Analysis) > 0 {
		fmt.Fprintln(w, "    analysis:")
		for _, key := range sortedStringKeys(dep.Analysis) {
			fmt.Fprintf(w, "      %s: %s\n", key, dep.Analysis[key])
		}
	}
	if len(dep.Findings) > 0 {
		fmt.Fprintln(w, "    findings:")
		for _, finding := range dep.Findings {
			fmt.Fprintf(w, "      - %s [%s]:\n", finding.ID, finding.Severity)
			fmt.Fprintf(w, "%s\n", indentSessionContext(finding.Detail, "        "))
		}
	}
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func joinSortedPairs(values map[string]string) string {
	keys := sortedStringKeys(values)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+values[key])
	}
	return strings.Join(pairs, ", ")
}

func missionInputs(run *gaterun.Run, spec *gaterun.SessionSpec) []missionReadInput {
	refs := make(map[string]missionReadInput)
	for _, ref := range run.Refs {
		refs[ref.Ref] = missionReadInput{Ref: ref.Ref, Resolved: ref.Resolved}
	}
	for _, surface := range run.Surfaces {
		for _, entry := range surface.Entries {
			refs[entry.Path] = missionReadInput{Ref: entry.Path, Resolved: entry.Path}
		}
	}
	out := make([]missionReadInput, 0, len(spec.ReadRefs))
	for _, name := range spec.ReadRefs {
		input, ok := refs[name]
		if !ok {
			input = missionReadInput{Ref: name}
		}
		out = append(out, input)
	}
	return out
}

func failureLineFor(gate string) string {
	switch gate {
	case gaterun.GateValidate:
		return "Validation could not complete — <reason>"
	default:
		return "Verification could not complete — <reason>"
	}
}

func missionTextFor(kind string) string {
	switch kind {
	case gaterun.SessionKindItem:
		return "Judge each acceptance item against the implementation: report its alignment verdict and evidence, and for a mismatch author the finding with its root cause, severity, and repair direction."
	case gaterun.SessionKindUnit:
		return "Scan the whole unit once: report the structural-declaration scan (Step 1) and Step 5 (implementation integrity and code design-surface surplus) with their declaration-class lines, and author a finding for every mismatch you retain. Do not repeat per-item judgments."
	case gaterun.SessionKindCode:
		return "Inspect whole-file public code facts without unit-private rationale."
	case gaterun.SessionKindArchitecture:
		return "Assess the whole unit architecture once (Dimension 8)."
	case gaterun.SessionKindDesign:
		return "Review the named implementation file against the unit spec and report its assessment and findings."
	case gaterun.SessionKindCross:
		return "Check only the assigned relationships using current source and accepted/carried judgments. Do not repeat local checks. Dispose existing findings and raise severity conservatively on retain or merge — the tooling derives the effective status map from your dispositions and findings. An empty relationship scope means finding disposition only."
	case gaterun.SessionKindDeltaReview:
		return "Review the change set against the standing conclusions: accept them, name the keys that must re-run, or escalate to a full run. Produce no check verdicts; focus on the changed content but read any input-surface file you need for context."
	}
	return "Judge only the session's check keys and report evidence for each judgment."
}

func protocolScopeFor(kind string, keys []string) string {
	switch kind {
	case gaterun.SessionKindItem:
		return "Steps 2, 3, 4, 6, 7 for acceptance item(s) " + strings.Join(keys, ", ")
	case gaterun.SessionKindUnit:
		return "Steps 1 and 5 over the whole unit"
	case gaterun.SessionKindDesign:
		return "quality assessment of " + strings.Join(keys, ", ")
	case gaterun.SessionKindCross:
		return "cross synthesis"
	case gaterun.SessionKindDeltaReview:
		return "change review"
	default:
		return "checks " + strings.Join(keys, ", ")
	}
}

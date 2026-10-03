package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
)

// runGateMission materializes the read-only reviewer mission for an
// agent-chosen coverage key batch. Public missions claim their fixed batch under the repository lock;
// mission generation never persists an accepted session. With --final it builds the optional final cross synthesis instead.
func runGateMission(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-mission", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id printed by gate-plan")
	keysPtr := fs.String("keys", "", "comma-separated coverage keys assigned to this session")
	finalPtr := fs.Bool("final", false, "build the final cross synthesis mission instead of a coverage batch")
	formatPtr := fs.String("format", "prompt", "output format: prompt | json")
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
	if spec.Kind != gaterun.SessionKindCross {
		states, cerr := gaterun.LoadSessionStates(absRoot, run)
		if cerr != nil {
			return cerr
		}
		covered, _, cerr := gaterun.CoverageProgress(run, states)
		if cerr != nil {
			return cerr
		}
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
	if *formatPtr == "json" {
		return writeGateJSON(stdout, mission)
	}
	writeGatePrompt(stdout, mission)
	return nil
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
// synthesis-relevant projection of an accepted session result (dependency
// results) or of a carried baseline judgment (carried results). It carries the
// verdicts, findings with their stored details, and analysis fields a
// result-consuming session synthesizes over — never the verbatim accepted
// report, which stays in run state and is assembled into the published cache
// body.
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
	Dependencies     []missionDependency       `json:"dependency_results"`
	CarriedResults   []missionDependency       `json:"carried_results"`
	DeferredFindings []gaterun.DeferredFinding `json:"deferred_findings"`
	ProtocolRef      string                    `json:"protocol_ref"`
	ProtocolScope    string                    `json:"protocol_scope"`
	AdditionalRefs   []string                  `json:"additional_protocol_refs"`
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
		AdditionalRefs:   []string{},
		ReportContract:   reportContractFor(run, spec),
	}
	if state.Status == gaterun.SessionRejected {
		if len(state.Attempts) == 0 || strings.TrimSpace(state.Attempts[len(state.Attempts)-1].RejectionReason) == "" {
			return gateMission{}, fmt.Errorf("rejected session %q has no rejection reason", spec.SessionID)
		}
		session.LastRejection = state.Attempts[len(state.Attempts)-1].RejectionReason
	}
	if spec.Kind == gaterun.SessionKindCross {
		session.AdditionalRefs = append(session.AdditionalRefs,
			specflowlayout.Relative(layout.FrameworkRoot, "verification_scope.md")+" §Cross-check",
			specflowlayout.Relative(layout.FrameworkRoot, "severity_policy.md"))
	}
	framework := layout.FrameworkRoot
	glossary := []missionTerm{
		{"session", "one independently reviewed batch of coverage keys in a gate run", framework + "/verification_scope.md §Coverage model"},
		{"check_keys", "the checks this session must report, excluding carried checks", framework + "/verification_scope.md §Session record"},
		{"read_refs", "the exact evidence entries this session may declare", framework + "/verification_scope.md §Session record"},
		{"Dependency scope", "one line per executed check naming the file and region used for its judgment", framework + "/validation_cache.md §Dependency Declaration"},
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
		glossary = append(glossary, missionTerm{"deferred finding", "a finding routed from another unit for this verify run to dispose", framework + "/verification_scope.md §Deferred findings"})
	}
	if spec.Kind == gaterun.SessionKindCross {
		// The optional final synthesis consumes every accepted session's
		// compact judgment record, plus the carried baseline judgments below.
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
		results, err := gaterun.PublicResultsForDesign(root, run, spec)
		if err != nil {
			return gateMission{}, err
		}
		for _, key := range spec.CheckKeys {
			result := results[key]
			if stringInList(run.CarriedKeys, result.SessionID) {
				session.CarriedResults = append(session.CarriedResults, missionJudgmentFor(result.SessionID, "", &result))
			} else {
				session.Dependencies = append(session.Dependencies, missionJudgmentFor(result.SessionID, gaterun.SessionAccepted, &result))
			}
		}
	}
	if spec.Kind == gaterun.SessionKindCross {
		for i := range run.CarriedResults {
			session.CarriedResults = append(session.CarriedResults, missionJudgmentFor(run.CarriedResults[i].SessionID, "", &run.CarriedResults[i]))
		}
	}
	constraints := []string{"independent read-only reviewer session without the author's context", "the read surface is derived deterministically from the assigned keys; judge the same evidence regardless of execution order", "read files, search by pattern, and run read-only git queries only", "do not modify files, run state-changing commands, or launch sub-agents", "report evidence only from session read_refs; protocol_ref is instruction, not evidence", "the main agent collects verdicts verbatim and does not re-litigate them"}
	if run.Gate == gaterun.GateVerify && spec.Kind == gaterun.SessionKindItem {
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
		Submission:  fmt.Sprintf("specflowctl gate-submit --run %s --session %s --keys %s --report REPORT_PATH", run.RunID, spec.SessionID, strings.Join(gaterun.CoverageKeysForSpec(run, spec), ",")),
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
	if p.Kind == gaterun.SessionKindCross {
		fmt.Fprintf(w, "Assigned relationships: %s\n", strings.Join(p.Relationships, ", "))
	}
	fmt.Fprintf(w, "Protocol: %s (%s). Read this scope for semantic judgment; the task and report format are fixed here.\n", p.ProtocolRef, p.ProtocolScope)
	for _, ref := range p.AdditionalRefs {
		fmt.Fprintf(w, "Additional semantic reference: %s\n", ref)
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
		fmt.Fprintln(w, "Dependency judgments (accepted verdicts, findings, observations, and analysis):")
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
// reviewers' narratives; evidence is read from the session's read refs.
func missionJudgmentFor(sessionID, status string, result *gaterun.SessionResult) missionDependency {
	entry := missionDependency{SessionID: sessionID, Status: status}
	if result == nil {
		return entry
	}
	entry.Kind = result.Kind
	entry.Digest = result.ReportDigest
	entry.Verdicts = result.Verdicts
	entry.Findings = result.Findings
	entry.Observations = result.Observations
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
	case gaterun.SessionKindItem, gaterun.SessionKindPreserve:
		return "Judge each acceptance item against the implementation: report its alignment verdict and evidence, and for a mismatch author the finding with its root cause, severity, and repair direction."
	case gaterun.SessionKindCode:
		return "Inspect whole-file public code facts without unit-private rationale."
	case gaterun.SessionKindArchitecture:
		return "Assess the whole unit architecture once (Dimension 8)."
	case gaterun.SessionKindDesign:
		return "Review the named implementation file against the unit spec and report its assessment and findings."
	case gaterun.SessionKindCross:
		return "Check only the assigned relationships using current source and accepted/carried judgments. Do not repeat local checks. Dispose existing findings, raise severity conservatively on retain or merge, and report effective statuses. An empty relationship scope means finding disposition only."
	}
	return "Judge only the session's check keys and report evidence for each judgment."
}

func protocolScopeFor(kind string, keys []string) string {
	switch kind {
	case gaterun.SessionKindItem, gaterun.SessionKindPreserve:
		return "Steps 1-7 for acceptance item(s) " + strings.Join(keys, ", ")
	case gaterun.SessionKindDesign:
		return "quality assessment of " + strings.Join(keys, ", ")
	case gaterun.SessionKindCross:
		return "cross synthesis"
	default:
		return "checks " + strings.Join(keys, ", ")
	}
}

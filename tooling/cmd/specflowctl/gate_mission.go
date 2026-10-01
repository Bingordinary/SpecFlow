package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
)

// missionDependency is one compact judgment record in a packet mission: the
// synthesis-relevant projection of an accepted packet result (dependency
// results) or of a carried baseline judgment (carried results). It carries the
// verdicts, findings with their stored details, and analysis fields a
// result-consuming packet synthesizes over — never the verbatim accepted
// report, which stays in run state and is assembled into the published cache
// body. The one exception is a verify analysis packet's single detection
// dependency, whose report is the documented input of the analysis step.
type missionDependency struct {
	PacketID        string            `json:"packet_id"`
	Kind            string            `json:"kind,omitempty"`
	Status          string            `json:"status"`
	Digest          string            `json:"digest,omitempty"`
	Verdicts        map[string]string `json:"verdicts,omitempty"`
	Findings        []gaterun.Finding `json:"findings,omitempty"`
	Analysis        map[string]string `json:"analysis,omitempty"`
	EffectiveStatus map[string]string `json:"effective_status,omitempty"`
	Report          string            `json:"accepted_report,omitempty"`
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

type missionPacket struct {
	PacketID         string                    `json:"packet_id"`
	Kind             string                    `json:"kind"`
	Mission          string                    `json:"mission"`
	LastRejection    string                    `json:"last_rejection,omitempty"`
	CheckKeys        []string                  `json:"check_keys"`
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
	SchemaVersion int             `json:"schema_version"`
	RunID         string          `json:"run_id"`
	Gate          string          `json:"gate"`
	TargetKind    string          `json:"target_kind"`
	TargetName    string          `json:"target_name"`
	Target        string          `json:"target"`
	Mode          string          `json:"mode"`
	SpecSource    string          `json:"spec_source"`
	Packets       []missionPacket `json:"packets"`
	Constraints   []string        `json:"constraints"`
	Glossary      []missionTerm   `json:"glossary"`
	FailurePath   string          `json:"failure_path"`
	Submission    string          `json:"submission_command"`
}

func buildGateMission(root string, run *gaterun.Run, spec *gaterun.PacketSpec, state *gaterun.PacketState) (gateMission, error) {
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
	case run.Gate == gaterun.GateReview:
		protocol = "spec_review_checklist.md"
	default:
		return gateMission{}, fmt.Errorf("unsupported gate %q", run.Gate)
	}
	packet := missionPacket{
		PacketID: spec.PacketID, Kind: spec.Kind, Mission: missionTextFor(spec.Kind),
		CheckKeys:        append([]string{}, spec.CheckKeys...),
		ReadRefs:         append([]string{}, spec.ReadRefs...),
		ReadInputs:       missionInputs(run, spec),
		DependsOn:        append([]string{}, spec.DependsOn...),
		Context:          append([]string{}, spec.Context...),
		Dependencies:     []missionDependency{},
		CarriedResults:   []missionDependency{},
		DeferredFindings: append([]gaterun.DeferredFinding{}, deferredFindingsForPacket(run, spec)...),
		ProtocolRef:      specflowlayout.Relative(layout.FrameworkRoot, protocol),
		ProtocolScope:    protocolScopeFor(spec.Kind, spec.CheckKeys),
		AdditionalRefs:   []string{},
		ReportContract:   reportContractFor(run, spec),
	}
	if state.Status == gaterun.PacketRejected {
		if len(state.Attempts) == 0 || strings.TrimSpace(state.Attempts[len(state.Attempts)-1].RejectionReason) == "" {
			return gateMission{}, fmt.Errorf("rejected packet %q has no rejection reason", spec.PacketID)
		}
		packet.LastRejection = state.Attempts[len(state.Attempts)-1].RejectionReason
	}
	if spec.Kind == gaterun.PacketKindCross {
		packet.AdditionalRefs = append(packet.AdditionalRefs,
			specflowlayout.Relative(layout.FrameworkRoot, "verification_scope.md")+" §Cross-check",
			specflowlayout.Relative(layout.FrameworkRoot, "severity_policy.md")+" §9")
	}
	framework := layout.FrameworkRoot
	glossary := []missionTerm{
		{"packet", "one planned and independently reviewed part of a gate run", framework + "/verification_scope.md §Gate Work Packets"},
		{"check_keys", "the checks this packet must report, excluding carried checks", framework + "/verification_scope.md §Packet record"},
		{"read_refs", "the exact evidence entries this packet may declare", framework + "/verification_scope.md §Packet record"},
		{"Dependency scope", "one line per executed check naming the file and region used for its judgment", framework + "/validation_cache.md §Dependency Declaration"},
	}
	if spec.Kind == gaterun.PacketKindCross {
		glossary = append(glossary,
			missionTerm{"carried judgments", "baseline judgments preserved without re-execution in a delta or repair run", framework + "/verification_scope.md §Delta Runs"},
			missionTerm{"finding disposition", "cross decision to retain, suppress, or merge each input finding", framework + "/verification_scope.md §Packet report contract"},
			missionTerm{"effective status", "cross result for every logical check and cross itself", framework + "/verification_scope.md §Packet report contract"},
			missionTerm{"severity confirmation", "cross evidence-backed confirmation or adjustment of a retained finding's grade", framework + "/severity_policy.md §9"},
		)
		if len(crossItemsFor(run.Gate)) > 0 {
			glossary = append(glossary, missionTerm{"Cross item finding", "the new retained cross finding that explains one failed fixed cross item", framework + "/verification_scope.md §Packet report contract"})
		}
	}
	if len(packet.DeferredFindings) > 0 {
		glossary = append(glossary, missionTerm{"deferred finding", "a finding routed from another unit for this review to dispose", framework + "/verification_scope.md §Deferred findings"})
	}
	for _, dep := range spec.DependsOn {
		state, err := gaterun.LoadPacketState(root, run, dep)
		if err != nil {
			return gateMission{}, err
		}
		if state.Status != gaterun.PacketAccepted && state.Status != gaterun.PacketNotRequired {
			return gateMission{}, fmt.Errorf("packet %q is not ready: dependency %q is %s", spec.PacketID, dep, state.Status)
		}
		if state.Status == gaterun.PacketAccepted && state.Result == nil {
			return gateMission{}, fmt.Errorf("dependency %q has no accepted result", dep)
		}
		entry := missionJudgmentFor(dep, state.Status, state.Result)
		if spec.Kind == gaterun.PacketKindAnalysis || spec.Kind == gaterun.PacketKindVerifier {
			// The analysis step's documented input is the accepted detection
			// report carrying the detector's evidence lines (see
			// framework/unit_verify_checklist.md Step 7); the verifier's
			// documented input is the accepted reader reconstruction (see
			// framework/unit_validate_checklist.md Check 10). Every other
			// dependency carries its compact judgment record only.
			entry.Report = state.Report
		}
		packet.Dependencies = append(packet.Dependencies, entry)
	}
	if spec.Kind == gaterun.PacketKindCross {
		for i := range run.CarriedResults {
			packet.CarriedResults = append(packet.CarriedResults, missionJudgmentFor(run.CarriedResults[i].PacketID, "", &run.CarriedResults[i]))
		}
	}
	constraints := []string{"independent read-only reviewer session without the author's context", "packet boundaries are deterministic; judge the same evidence regardless of execution order", "read files, search by pattern, and run read-only git queries only", "do not modify files, run state-changing commands, or launch sub-agents", "report evidence only from packet read_refs; protocol_ref is instruction, not evidence", "the main agent collects verdicts verbatim and does not re-litigate them"}
	if spec.Kind == gaterun.PacketKindReader {
		constraints = append(constraints,
			"reconstruct from the human-readable part only: the ## sections of the main spec listed in the packet Context, before the section holding acceptance_item_set; content after that section, appendices, and code are out of scope",
			"you receive no question bank: restate the design as one coherent block so each part connects to the next — a restatement that only answers scattered questions proves nothing about the whole; when something would not connect or had to be guessed, declare it in the Undetermined list instead of filling the gap",
			"do not write a verdict line or finding entries — the check verdict is the verifier packet's, composed mechanically from its classifications")
	}
	if spec.Kind == gaterun.PacketKindVerifier {
		constraints = append(constraints,
			"reconcile the accepted reconstruction against the human-readable part (support) and the formal carrier — the acceptance item set and the protocol appendices (backbone and contradiction); do not repair the reader's gaps from your own knowledge",
			"classify, do not author: Claim lines carry supported, reader-error, unsupported-central, unsupported-minor, or contradicted; Carrier lines carry seen or missing for every acceptance item; Must-close lines carry closed, missing, or not-applicable for every §9 decision; Consistency is coherent or incoherent",
			"central = the unit's declared responsibility + each acceptance item's behavioral subject + every applicable must-close decision; unsupported-minor claims are advisory and reader-error claims are the reader's own invention — neither may be inflated to a blocking class; a declarative unit is reconciled by its declared responsibility and its acceptance items' behavioral subjects, never by a behavior template",
			"do not write finding entries — findings are composed mechanically from the classification lines")
	}
	if run.Gate == gaterun.GateVerify && (spec.Kind == gaterun.PacketKindItem || spec.Kind == gaterun.PacketKindAnalysis) {
		constraints = append(constraints, "if a required test, caller, callee, or dependency file is missing from read_refs, return `Verification could not complete — missing read ref: <repo-relative path>`; do not judge from incomplete context or submit a verdict")
	}
	return gateMission{
		SchemaVersion: 2, RunID: run.RunID, Gate: run.Gate,
		TargetKind: run.TargetKind, TargetName: run.TargetName, Target: run.Target, Mode: run.Mode,
		SpecSource:  run.RequiredFiles[0],
		Packets:     []missionPacket{packet},
		Constraints: constraints,
		Glossary:    glossary,
		FailurePath: failureLineFor(run.Gate),
		Submission:  fmt.Sprintf("specflowctl gate-submit --run %s --packet %s --report REPORT_PATH", run.RunID, spec.PacketID),
	}, nil
}

func writeGatePrompt(w io.Writer, mission gateMission) {
	p := mission.Packets[0]
	fmt.Fprintf(w, "You are the independent read-only reviewer for %s@%s (%s), packet %s of run %s.\n", mission.Gate, mission.TargetName, mission.Target, p.PacketID, mission.RunID)
	fmt.Fprintf(w, "Mission: %s The main agent will submit your report verbatim; follow the report contract below.\n", p.Mission)
	fmt.Fprintf(w, "Why now: this packet is ready in the %s run; every dependency is accepted or not required.\n", mission.Mode)
	if p.LastRejection != "" {
		fmt.Fprintf(w, "Previous submission was rejected: %s. Re-evaluate the packet and return a complete corrected report.\n", p.LastRejection)
	}

	fmt.Fprintf(w, "Run: %s\nGate: %s\nTarget kind: %s\nTarget name: %s\nTarget layer: %s\nPacket: %s\nKind: %s\nChecks: %s\nMode: %s\n", mission.RunID, mission.Gate, mission.TargetKind, mission.TargetName, mission.Target, p.PacketID, p.Kind, strings.Join(p.CheckKeys, ", "), mission.Mode)
	fmt.Fprintf(w, "Spec source: %s\n", mission.SpecSource)
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
		fmt.Fprintln(w, "Packet context (plan-time facts; state these verbatim where the report contract requires):")
		for _, line := range p.Context {
			fmt.Fprintf(w, "  - %s\n", line)
		}
	}
	if len(p.Dependencies) > 0 {
		fmt.Fprintln(w, "Dependency results (each accepted packet's verdicts, findings, and analysis):")
		for _, dep := range p.Dependencies {
			writeMissionJudgment(w, dep)
			if dep.Report != "" {
				fmt.Fprintf(w, "    Accepted report:\n%s\n", indentPacketContext(dep.Report, "      "))
			}
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

// missionJudgmentFor projects an accepted packet result into the compact
// judgment record a mission carries. A nil result (a not-required conditional
// dependency) yields the identity fields only. The verbatim accepted report is
// never part of the projection — a mission carries judgments, not the
// reviewers' narratives; evidence is read from the packet's read refs.
func missionJudgmentFor(packetID, status string, result *gaterun.PacketResult) missionDependency {
	entry := missionDependency{PacketID: packetID, Status: status}
	if result == nil {
		return entry
	}
	entry.Kind = result.Kind
	entry.Digest = result.ReportDigest
	entry.Verdicts = result.Verdicts
	entry.Findings = result.Findings
	entry.Analysis = result.Analysis
	entry.EffectiveStatus = result.EffectiveStatus
	return entry
}

// writeMissionJudgment renders one compact judgment record: the packet header
// line, its verdicts or effective statuses, analysis fields, and every finding
// with its stored detail block. Map keys are sorted so the mission text is
// deterministic for a given run state.
func writeMissionJudgment(w io.Writer, dep missionDependency) {
	header := dep.PacketID
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
			fmt.Fprintf(w, "%s\n", indentPacketContext(finding.Detail, "        "))
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

func missionInputs(run *gaterun.Run, spec *gaterun.PacketSpec) []missionReadInput {
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
	case gaterun.GateVerify:
		return "Verification could not complete — <reason>"
	default:
		return "Review could not complete — <reason>"
	}
}

func missionTextFor(kind string) string {
	switch kind {
	case gaterun.PacketKindItem:
		return "Detect whether the acceptance item matches the implementation; report its type and evidence without assigning severity."
	case gaterun.PacketKindAnalysis:
		return "Analyze the accepted mismatch, determine its root cause, severity, and repair direction."
	case gaterun.PacketKindReader:
		return "Read only the main spec's human-readable part and reconstruct the design closed-book: one coherent restatement plus an honest Undetermined list; no question bank, no citations, no verdicts."
	case gaterun.PacketKindVerifier:
		return "Reconcile the accepted reconstruction against the human-readable part and the formal carrier (acceptance item set and protocol appendices): classify every claim, map every carrier item and §9 must-close decision, and judge the restatement's internal coherence; the verdict is composed mechanically from the classifications."
	case gaterun.PacketKindFile:
		return "Review the named implementation file against the unit spec and report its assessment and findings."
	case gaterun.PacketKindCross:
		return "Synthesize all accepted and carried judgments, dispose every input finding, confirm retained severities, and report effective statuses."
	}
	return "Judge only the packet's check keys and report evidence for each judgment."
}

func protocolScopeFor(kind string, keys []string) string {
	switch kind {
	case gaterun.PacketKindItem:
		return "Steps 1-6 for acceptance item " + strings.Join(keys, ", ")
	case gaterun.PacketKindAnalysis:
		return "Step 7 for acceptance item " + strings.Join(keys, ", ")
	case gaterun.PacketKindReader:
		return "Check 10 reader probe (closed-book reconstruction)"
	case gaterun.PacketKindVerifier:
		return "Check 10 verifier protocol (reconciliation)"
	case gaterun.PacketKindFile:
		return "file review for " + strings.Join(keys, ", ")
	case gaterun.PacketKindCross:
		return "cross synthesis"
	default:
		return "checks " + strings.Join(keys, ", ")
	}
}

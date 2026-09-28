package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
)

type missionDependency struct {
	PacketID string                `json:"packet_id"`
	Status   string                `json:"status"`
	Digest   string                `json:"digest,omitempty"`
	Result   *gaterun.PacketResult `json:"result,omitempty"`
	Report   string                `json:"accepted_report,omitempty"`
}

type missionTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Source     string `json:"source"`
}

type missionReadInput struct {
	Ref      string `json:"ref"`
	Resolved string `json:"resolved,omitempty"`
	Hash     string `json:"hash,omitempty"`
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
	Dependencies     []missionDependency       `json:"dependency_results"`
	CarriedResults   []gaterun.PacketResult    `json:"carried_results"`
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
		Dependencies:     []missionDependency{},
		CarriedResults:   []gaterun.PacketResult{},
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
		entry := missionDependency{PacketID: dep, Status: state.Status}
		if state.Status == gaterun.PacketAccepted {
			if state.Result == nil {
				return gateMission{}, fmt.Errorf("dependency %q has no accepted result", dep)
			}
			entry.Digest, entry.Result, entry.Report = state.Result.ReportDigest, state.Result, state.Report
		}
		packet.Dependencies = append(packet.Dependencies, entry)
	}
	if spec.Kind == gaterun.PacketKindCross {
		packet.CarriedResults = append(packet.CarriedResults, run.CarriedResults...)
	}
	constraints := []string{"independent read-only reviewer session without the author's context", "packet boundaries are deterministic; judge the same evidence regardless of execution order", "read files, search by pattern, and run read-only git queries only", "do not modify files, run state-changing commands, or launch sub-agents", "report evidence only from packet read_refs; protocol_ref is instruction, not evidence", "the main agent collects verdicts verbatim and does not re-litigate them"}
	if run.Gate == gaterun.GateVerify && (spec.Kind == gaterun.PacketKindItem || spec.Kind == gaterun.PacketKindAnalysis) {
		constraints = append(constraints, "if a required test, caller, callee, or dependency file is missing from read_refs, return `Verification could not complete — missing read ref: <repo-relative path>`; do not judge from incomplete context or submit a verdict")
	}
	return gateMission{
		SchemaVersion: 1, RunID: run.RunID, Gate: run.Gate,
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
		if input.Hash != "" {
			fmt.Fprintf(w, "    snapshot hash: %s\n", input.Hash)
		}
	}
	if len(p.Dependencies) > 0 {
		fmt.Fprintln(w, "Dependency results:")
		for _, dep := range p.Dependencies {
			fmt.Fprintf(w, "  - %s: %s", dep.PacketID, dep.Status)
			if dep.Digest != "" {
				fmt.Fprintf(w, " (digest %s)", dep.Digest)
			}
			fmt.Fprintln(w)
			if dep.Result != nil {
				data, _ := json.MarshalIndent(dep.Result, "    ", "  ")
				fmt.Fprintf(w, "    Parsed result: %s\n", data)
			}
			if dep.Report != "" {
				fmt.Fprintf(w, "    Accepted report:\n%s\n", indentPacketContext(dep.Report, "      "))
			}
		}
	}
	if len(p.CarriedResults) > 0 {
		fmt.Fprintln(w, "Carried judgments:")
		for _, result := range p.CarriedResults {
			data, _ := json.MarshalIndent(result, "    ", "  ")
			fmt.Fprintf(w, "  - %s (%s): %s\n", result.PacketID, result.ReportDigest, data)
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

func missionInputs(run *gaterun.Run, spec *gaterun.PacketSpec) []missionReadInput {
	refs := make(map[string]missionReadInput)
	for _, ref := range run.Refs {
		refs[ref.Ref] = missionReadInput{Ref: ref.Ref, Resolved: ref.Resolved, Hash: ref.Hash}
	}
	for _, surface := range run.Surfaces {
		for _, entry := range surface.Entries {
			refs[entry.Path] = missionReadInput{Ref: entry.Path, Resolved: entry.Path, Hash: entry.Hash}
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
	case gaterun.PacketKindFile:
		return "file review for " + strings.Join(keys, ", ")
	case gaterun.PacketKindCross:
		return "cross synthesis"
	default:
		return "checks " + strings.Join(keys, ", ")
	}
}

package main

import (
	"fmt"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// These declarations drive both packet instructions and fixed-field report
// validation. Dynamic synthesis checks remain in gate-submit.
var unitAcceptanceSubchecks = []string{"5a", "5b", "5c", "5d", "5e", "5f", "5g", "5h", "5i"}
var verifyItemFields = []string{"evidence", "deterministic", "Part A", "Part B"}
var reviewDimensions = []string{"module_boundaries", "responsibility_organization", "dependency_clarity", "abstraction_level", "extension_landing_points", "engineering_patterns"}
var analysisFields = []string{"Problem", "Impact", "Root cause", "Suggested direction", "Severity", "Confidence"}
var mismatchTypes = []string{"structural", "acceptance", "scope", "stub", "surplus"}
var analysisRootCauses = []string{"incomplete", "stale", "shadow_spec", "divergence", "accident", "blocked"}
var analysisDirections = []string{"spec_gap", "code_gap", "needs_design", "blocked"}
var analysisConfidences = []string{"high", "medium", "low"}
var severityLevels = []string{"P0", "P1", "P2", "P3"}
var crossDispositions = []string{"retained", "suppressed", "merged"}
var crossStatuses = []string{"pass", "fail"}
var severityOutcomes = []string{"confirmed", "adjusted"}
var verifyCrossItems = []string{"contract_consistency", "data_definition_drift", "state_machine_coherence", "error_code_conflict", "cross_reference_integrity"}
var validateCrossItems = []string{"design_constraints", "coverage_scope", "cross_unit_cohesion"}

func crossItemsFor(gate string) []string {
	switch gate {
	case gaterun.GateVerify:
		return verifyCrossItems
	case gaterun.GateValidate:
		return validateCrossItems
	default:
		return nil
	}
}

func unitAcceptanceAllowed(subcheck string) []string {
	if subcheck == "5a" {
		return []string{"PASS", "WARNING", "FAIL"}
	}
	return []string{"PASS", "FAIL"}
}

type reportVerdict struct {
	Key               string   `json:"key"`
	Line              string   `json:"line"`
	Allowed           []string `json:"allowed"`
	ReasonRequiredFor []string `json:"reason_required_for"`
}

func verdictContractFor(kind, key string) reportVerdict {
	v := reportVerdict{Key: key, Line: key, Allowed: []string{}, ReasonRequiredFor: []string{}}
	switch kind {
	case gaterun.PacketKindChecks:
		v.Line = key + ". <check name>"
		v.Allowed = []string{"PASS", "WARNING", "FAIL"}
		v.ReasonRequiredFor = append([]string{}, v.Allowed...)
	case gaterun.PacketKindItem:
		v.Allowed = []string{"ALIGNED", "MISMATCH", "CANNOT_DETERMINE"}
		v.ReasonRequiredFor = append([]string{}, v.Allowed...)
	case gaterun.PacketKindFile:
		v.Line = "conclusion"
		v.Allowed = []string{"acceptable", "needs_attention", "unacceptable"}
		v.ReasonRequiredFor = []string{"unacceptable"}
	case gaterun.PacketKindCross:
		v.Line = "Cross-check"
		v.Allowed = []string{"PASS", "FAIL"}
		v.ReasonRequiredFor = append([]string{}, v.Allowed...)
	case gaterun.PacketKindVerifier:
		v.Line = gaterun.ReaderContractCheck + ". Reader contract"
		v.Allowed = []string{"PASS", "FAIL"}
		v.ReasonRequiredFor = append([]string{}, v.Allowed...)
	case gaterun.PacketKindReader:
		// The reader authors evidence only — the verdict is computed by
		// gate-submit from the question blocks, so the contract declares no
		// reviewer-authored verdict line.
		v.Line = "(computed from the question blocks — do not write a verdict line)"
	case gaterun.PacketKindAnalysis:
		v.Allowed = []string{"MISMATCH"}
	}
	return v
}

type reportRequirement struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	When        string   `json:"when"`
	MinCount    int      `json:"min_count"`
	MaxCount    int      `json:"max_count"`
	CountBasis  string   `json:"count_basis,omitempty"`
	Allowed     []string `json:"allowed,omitempty"`
}

type gateReportContract struct {
	Format       string              `json:"format"`
	Verdicts     []reportVerdict     `json:"verdicts"`
	Requirements []reportRequirement `json:"requirements"`
	Template     string              `json:"template"`
}

func reportContractFor(run *gaterun.Run, packet *gaterun.PacketSpec) gateReportContract {
	c := gateReportContract{Format: "text", Verdicts: []reportVerdict{}, Requirements: []reportRequirement{}}
	var lines []string
	for _, key := range packet.CheckKeys {
		v := verdictContractFor(packet.Kind, key)
		switch packet.Kind {
		case gaterun.PacketKindChecks, gaterun.PacketKindVerifier:
			if packet.Kind == gaterun.PacketKindVerifier {
				lines = append(lines, fmt.Sprintf("%s. Reader contract: <PASS|FAIL> — <reason>", key))
			} else {
				lines = append(lines, fmt.Sprintf("%s. <check name>: <PASS|WARNING|FAIL> — <reason>", key))
			}
		case gaterun.PacketKindReader:
			for _, q := range readerContractQuestions {
				lines = append(lines,
					"Question: "+q.ID+" — "+q.Title,
					"Status: <answered|no_local_answer>",
					"Answer: <one-sentence answer; answered questions only>",
					"Quote: <verbatim quote, "+fmt.Sprint(readerQuoteMinRunes)+"-"+fmt.Sprint(readerQuoteMaxRunes)+" characters, one contiguous span; answered questions only>",
					"Location: <exact heading text of the ## section containing the quote; answered questions only>",
					"")
			}
		case gaterun.PacketKindItem:
			lines = append(lines, fmt.Sprintf("%s: <ALIGNED|MISMATCH (type)|CANNOT_DETERMINE> — <reason>", key))
		case gaterun.PacketKindFile:
			lines = append(lines, "conclusion: <acceptable|needs_attention|unacceptable> — <reason if unacceptable>")
		case gaterun.PacketKindCross:
			if items := crossItemsFor(run.Gate); len(items) > 0 {
				for _, item := range items {
					addLine := "Cross item: " + item + " = <PASS|FAIL> — <reason>"
					lines = append(lines, addLine)
				}
				lines = append(lines, fmt.Sprintf("Cross-check: <passed>/%d <PASS|FAIL> — <reason>", len(items)))
			} else {
				lines = append(lines, "Cross-check: <PASS|FAIL> — <reason>")
			}
		case gaterun.PacketKindAnalysis:
		}
		if packet.Kind != gaterun.PacketKindAnalysis && packet.Kind != gaterun.PacketKindReader {
			c.Verdicts = append(c.Verdicts, v)
		}
	}
	add := func(id, description, example string) {
		c.Requirements = append(c.Requirements, reportRequirement{ID: id, Description: description, When: "always", MinCount: 1, MaxCount: 1})
		if example != "" {
			lines = append(lines, example)
		}
	}
	if packet.Kind == gaterun.PacketKindReader {
		add("reader-question-blocks", "exactly one block per bank question, in bank order (Q01-Q17); each block carries Status, and answered blocks carry non-empty Answer, Quote, and Location", "Question: Q01 — intended user, actor, or caller\nStatus: <answered|no_local_answer>\nAnswer: <answer>\nQuote: <verbatim quote>\nLocation: <## heading>")
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		add("reader-quote-bounds", "each Quote is a verbatim span of one human-readable section, "+fmt.Sprint(readerQuoteMinRunes)+"-"+fmt.Sprint(readerQuoteMaxRunes)+" characters; a missing answer is reported as Status: no_local_answer, never approximated", "")
		add("reader-narrative-scopes", "one Dependency scope line per human-readable section of the main spec (the packet Context lists them), declared for check "+gaterun.ReaderContractCheck, "Dependency scope:\n  check-"+gaterun.ReaderContractCheck+": <main spec>: <section heading>")
	}
	if packet.Kind == gaterun.PacketKindVerifier {
		add("verifier-judgments", "exactly one Judgment line per bank question, in bank order (Q01-Q17), each with supported, partial, or unsupported and a non-empty basis", "Judgment: Q01 = <supported|partial|unsupported> — <basis>")
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].Allowed = []string{"supported", "partial", "unsupported"}
		add("verifier-verdict-closure", "the verdict is PASS exactly when every judgment is supported and the accepted reader report carries no no_local_answer; any other outcome is FAIL", "")
		add("verifier-narrative-scopes", "one Dependency scope line per human-readable section of the main spec (the packet Context lists them), declared for check "+gaterun.ReaderContractCheck, "Dependency scope:\n  check-"+gaterun.ReaderContractCheck+": <main spec>: <section heading>")
	}
	if packet.Kind == gaterun.PacketKindChecks && run.Gate == gaterun.GateValidate && run.TargetKind == gaterun.TargetKindUnit && stringInList(packet.CheckKeys, "5") {
		for _, sub := range unitAcceptanceSubchecks {
			allowed := strings.Join(unitAcceptanceAllowed(sub), "|")
			add("check-"+sub, "exactly one "+sub+" verdict with reason ("+allowed+")", sub+". <name>: <"+allowed+"> — <reason>")
			c.Requirements[len(c.Requirements)-1].Allowed = unitAcceptanceAllowed(sub)
		}
	}
	if packet.Kind == gaterun.PacketKindItem {
		add("mismatch-type", "MISMATCH requires one of: "+strings.Join(mismatchTypes, ", "), "")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].Allowed = mismatchTypes
		for _, field := range verifyItemFields {
			example := field + ": <value>"
			if field == "deterministic" {
				example = "deterministic: <true|false>"
			}
			add("verify-"+field, "exactly one non-empty "+field+" line", example)
			if field == "deterministic" {
				c.Requirements[len(c.Requirements)-1].Allowed = []string{"true", "false"}
			}
		}
		add("part-b-skipped", "a skipped Part B requires a reason", "")
		c.Requirements[len(c.Requirements)-1].When = "if_part_b_skipped"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
	}
	if packet.Kind == gaterun.PacketKindAnalysis {
		add("analysis-item", "exactly one Item line matching the packet check key", "Item: "+packet.CheckKeys[0])
		for _, field := range analysisFields {
			allowed := analysisAllowed(field)
			example := field + ": <value>"
			if len(allowed) > 0 {
				example = field + ": <" + strings.Join(allowed, "|") + ">"
			}
			add("analysis-"+field, "exactly one non-empty "+field+" line", example)
			c.Requirements[len(c.Requirements)-1].Allowed = allowed
		}
		add("analysis-evidence", "Evidence contains spec and code sub-lines", "Evidence:\n  - spec: <quoted fact or absence and searched scope>\n  - code: <quoted fact or absence and searched scope>")
		add("analysis-resolution", "For spec_gap or code_gap use Fix; for needs_design or blocked use Decision and at least one indented Options entry", "<Fix: repair OR Decision: choice followed by Options: and an indented option>")
		c.Requirements[len(c.Requirements)-1].When = "by_suggested_direction"
	}
	if packet.Kind == gaterun.PacketKindFile {
		for _, field := range reviewDimensions {
			add("review-"+field, "exactly one assessment and non-empty basis", field+": <assessment> — <basis>")
		}
		add("review-gate-findings", "exactly one gate_findings line", "gate_findings: <none or blocking findings>")
		add("review-suppressed", "exactly one Suppressed by spec (N) block", "Suppressed by spec (0):")
		add("review-p3-anchor", "every P3 finding has a fact_anchor line", "")
		c.Requirements[len(c.Requirements)-1].When = "if_p3_finding"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
	}
	if packet.Kind == gaterun.PacketKindCross {
		for _, item := range crossItemsFor(run.Gate) {
			add("cross-item-"+item, "exactly one "+item+" result with PASS or FAIL and a non-empty reason", "")
			c.Requirements[len(c.Requirements)-1].Allowed = []string{"PASS", "FAIL"}
		}
		if len(crossItemsFor(run.Gate)) > 0 {
			add("cross-item-finding", "one link from each failed Cross item to a new retained cross finding; no link for a passing item", "Cross item finding: <failed_item_key> = <new_cross_finding_id>")
			c.Requirements[len(c.Requirements)-1].When = "if_cross_item_fail"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
			c.Requirements[len(c.Requirements)-1].CountBasis = "failed_cross_items"
		}
		add("cross-dispositions", "dispose every input finding once", "Finding disposition: <finding_id> = <retained|suppressed|merged> [reason or merge target]")
		c.Requirements[len(c.Requirements)-1].CountBasis = "input_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].Allowed = crossDispositions
		add("cross-effective-status", "one effective status per logical key and cross", "Effective status: <key> = <pass|fail>")
		c.Requirements[len(c.Requirements)-1].CountBasis = "logical_keys_plus_cross"
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].Allowed = crossStatuses
		add("cross-severity", "complete severity confirmation for every retained finding", "Severity confirmation: <finding_id> = confirmed <Px> — evidence: <read_ref>; reason: <reason>")
		c.Requirements[len(c.Requirements)-1].CountBasis = "retained_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].Allowed = severityOutcomes
		if run.Gate == gaterun.GateReview {
			add("cross-ownership", "ownership record for every finding deferred to another unit", "Finding ownership: <finding_id> = owned_by <unit> — evidence: <read_ref>; reason: <reason>")
			c.Requirements[len(c.Requirements)-1].When = "if_deferred_finding"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
		}
	}
	if run.Gate == gaterun.GateValidate && run.TargetKind == gaterun.TargetKindRule {
		add("rule-severity", "confirm each retained finding's severity per rule checklist", "")
		c.Requirements[len(c.Requirements)-1].When = "if_retained_finding"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
	}
	if packet.Kind != gaterun.PacketKindAnalysis && packet.Kind != gaterun.PacketKindItem && packet.Kind != gaterun.PacketKindReader && packet.Kind != gaterun.PacketKindVerifier {
		finding := "[P1] <location> — <finding> (actionable|needs_decision)\n  problem: <problem>\n  evidence: <evidence>\n  impact: <impact>\n  fix: <repair>"
		if packet.Kind == gaterun.PacketKindCross {
			finding = "[P1] <location> — <new cross finding>\nFinding affects: <new_finding_id> = <check_key>"
		}
		add("finding-block", "if findings are present, use the checklist's contiguous finding detail block", finding)
		c.Requirements[len(c.Requirements)-1].When = "if_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
	}
	add("dependency-scope", "at least one declaration per executed check key; file must be in read_refs", "Dependency scope:\n  <check_key>: <read_ref>: <section|range|acceptance_item:id|acceptance_items|all>")
	c.Requirements[len(c.Requirements)-1].MaxCount = -1
	c.Requirements[len(c.Requirements)-1].CountBasis = "per_check_key"
	c.Template = strings.Join(lines, "\n")
	return c
}

func analysisAllowed(field string) []string {
	switch field {
	case "Root cause":
		return analysisRootCauses
	case "Suggested direction":
		return analysisDirections
	case "Severity":
		return severityLevels
	case "Confidence":
		return analysisConfidences
	default:
		return nil
	}
}

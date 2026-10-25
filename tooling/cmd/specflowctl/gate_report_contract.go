package main

import (
	"fmt"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// These declarations drive both session instructions and fixed-field report
// validation. Dynamic synthesis checks remain in gate-submit.
var unitAcceptanceSubchecks = []string{"5a", "5b", "5e", "5h"}
var verifyItemFields = []string{"evidence", "deterministic", "Part A"}
var qualityDimensions = []string{"module_boundaries", "responsibility_organization", "dependency_clarity", "abstraction_level", "extension_landing_points", "engineering_patterns"}
var analysisFields = []string{"Problem", "Impact", "Root cause", "Suggested direction", "Severity", "Confidence"}

// itemMismatchTypes is the MISMATCH vocabulary of the item sessions: Step 2
// acceptance alignment, Step 3 scope accuracy, Step 4 retirement verification
// (a structural residue) and Step 6 stub adjudication. Surplus detection is
// Step 5, owned by the unit-level scan (unitMismatchTypes), so `surplus` is
// not an item type.
var itemMismatchTypes = []string{"structural", "acceptance", "scope", "stub"}

// unitScanClasses are the structural-declaration classes Step 1 compares against
// the implementation. The unit-level session reports one line per class so a
// class that was never examined is visible as a missing line instead of
// disappearing into prose.
var unitScanClasses = []string{"api_endpoints", "function_signatures", "data_structures", "enums_constants", "error_types", "error_messages", "state_machines", "configuration_keys"}

// unitMismatchTypes is the MISMATCH vocabulary of the unit-level scan: Step 1
// structures and Step 5 Part A surplus. Scope accuracy stays with the item
// sessions (Step 3), so an acceptance/scope/stub mismatch is rejected here.
var unitMismatchTypes = []string{"structural", "surplus"}
var analysisRootCauses = []string{"incomplete", "stale", "shadow_spec", "divergence", "accident", "blocked"}
var analysisDirections = []string{"spec_gap", "code_gap", "needs_design", "blocked"}
var analysisConfidences = []string{"high", "medium", "low"}
var severityLevels = []string{"P0", "P1", "P2", "P3"}
var crossDispositions = []string{"retained", "suppressed", "merged"}
var verifyCrossItems = gaterun.RelationshipNames(gaterun.GateVerify)
var validateCrossItems = gaterun.RelationshipNames(gaterun.GateValidate)

func crossItemsFor(run *gaterun.Run) []string {
	return run.Relationships
}

func unitAcceptanceAllowed(subcheck string) []string {
	return []string{"PASS", "FAIL"}
}

// isUnitValidateCheck5 reports whether the session judges the unit validate
// Check 5 batch (acceptance coverage & correctness), whose report carries the
// fixed 5a/5b/5e/5h sub-check lines in addition to the check verdict.
func isUnitValidateCheck5(run *gaterun.Run, spec *gaterun.SessionSpec) bool {
	return run.Gate == gaterun.GateValidate && run.TargetKind == gaterun.TargetKindUnit &&
		spec.Kind == gaterun.SessionKindChecks && stringInList(spec.CheckKeys, "5")
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
	case gaterun.SessionKindChecks:
		v.Line = key + ". <check name>"
		v.Allowed = []string{"PASS", "WARNING", "FAIL"}
		v.ReasonRequiredFor = append([]string{}, v.Allowed...)
	case gaterun.SessionKindItem, gaterun.SessionKindUnit:
		v.Allowed = []string{"ALIGNED", "MISMATCH", "CANNOT_DETERMINE"}
		v.ReasonRequiredFor = append([]string{}, v.Allowed...)
	case gaterun.SessionKindDesign, gaterun.SessionKindArchitecture:
		v.Line = "conclusion"
		v.Allowed = []string{"acceptable", "needs_attention", "unacceptable"}
		v.ReasonRequiredFor = []string{"unacceptable"}
	case gaterun.SessionKindCode:
		v.Line = "conclusion"
		v.Allowed = []string{"FACTS"}
	case gaterun.SessionKindCross:
		v.Line = "Cross-check"
		v.Allowed = []string{"PASS", "FAIL"}
		v.ReasonRequiredFor = append([]string{}, v.Allowed...)
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

func reportContractFor(run *gaterun.Run, session *gaterun.SessionSpec) gateReportContract {
	c := gateReportContract{Format: "text", Verdicts: []reportVerdict{}, Requirements: []reportRequirement{}}
	if session.Kind == gaterun.SessionKindDeltaReview {
		c.Verdicts = append(c.Verdicts, reportVerdict{
			Line:              "Review result",
			Allowed:           []string{"accept", "recheck", "escalate-full"},
			ReasonRequiredFor: []string{"accept", "recheck", "escalate-full"},
		})
		c.Requirements = append(c.Requirements, reportRequirement{
			ID:          "review-result",
			Description: "exactly one `Review result: accept|recheck|escalate-full` line with a reason",
			When:        "always", MinCount: 1, MaxCount: 1,
			Allowed: []string{"accept", "recheck", "escalate-full"},
		})
		c.Requirements = append(c.Requirements, reportRequirement{
			ID:          "recheck-keys",
			Description: "each Recheck key is written exactly as the mission prints it under \"Standing conclusions\" (a relationship key is `relationship:<name>`); the coverage-group id and the report's `check-{n}` label are not accepted",
			When:        "if_recheck", MinCount: 0, MaxCount: -1,
		})
		c.Template = "Review result: <accept|recheck|escalate-full> — <reason>\nRecheck: <key>[, <key>...]   # required when the result is recheck; each key exactly as printed under \"Standing conclusions\"\n"
		return c
	}
	var lines []string
	codeKeys := map[string]bool{}
	for _, key := range session.CheckKeys {
		if ck := run.CoverageByKey(key); ck != nil && ck.Kind == gaterun.SessionKindCode {
			codeKeys[key] = true
		}
	}
	for _, key := range session.CheckKeys {
		// A co-batched code+design session carries both kinds: each block
		// follows its own coverage kind's report contract.
		kind := session.Kind
		if ck := run.CoverageByKey(key); ck != nil {
			kind = ck.Kind
		}
		v := verdictContractFor(kind, key)
		switch kind {
		case gaterun.SessionKindChecks:
			lines = append(lines, fmt.Sprintf("%s. <check name>: <PASS|WARNING|FAIL> — <reason>", key))
		case gaterun.SessionKindUnit:
			lines = append(lines,
				fmt.Sprintf("%s: <ALIGNED|MISMATCH ("+strings.Join(unitMismatchTypes, "|")+")|CANNOT_DETERMINE> — <reason>", key),
				"  # for a MISMATCH, append one finding block per retained mismatch (each Step 1 declaration-class or Step 5 surplus mismatch), in report order:",
				"  Problem: <one line>",
				"  Evidence:",
				"    - spec: <quoted fact or absence>",
				"    - code: <quoted fact or absence>",
				"  Impact: <one line>",
				"  Fix: <repair>",
				"  Root cause: <incomplete|stale|shadow_spec|divergence|accident|blocked>",
				"  Suggested direction: <spec_gap|code_gap|needs_design|blocked>",
				"  Severity: <P0|P1|P2|P3>",
				"  Confidence: <high|medium|low>",
				"  # repeat the whole finding block (Problem: onward) once per retained mismatch; each block carries its own root cause, direction, and severity")
		case gaterun.SessionKindItem:
			lines = append(lines,
				fmt.Sprintf("%s: <ALIGNED|MISMATCH (type)|CANNOT_DETERMINE> — <reason>", key),
				"  evidence: <file:line or quoted fact>",
				"  deterministic: <true|false>",
				"  Part A: <concerns or No concerns>",
				"  # for a MISMATCH, append one finding block per retained mismatch, in report order:",
				"  Problem: <one line>",
				"  Evidence:",
				"    - spec: <quoted fact or absence>",
				"    - code: <quoted fact or absence>",
				"  Impact: <one line>",
				"  Fix: <repair>",
				"  Root cause: <incomplete|stale|shadow_spec|divergence|accident|blocked>",
				"  Suggested direction: <spec_gap|code_gap|needs_design|blocked>",
				"  Severity: <P0|P1|P2|P3>",
				"  Confidence: <high|medium|low>",
				"  # repeat the whole finding block (Problem: onward) once per retained mismatch; each block carries its own root cause, direction, and severity")
		case gaterun.SessionKindCode:
			lines = append(lines,
				"File: "+key,
				"conclusion: FACTS",
				"facts: <code facts and potential problems; no unit design rationale>",
				"  # a public potential finding is a finding block written inside this code block,",
				"  # after facts: and before the next File: line; its id is the finding's report-order id:",
				"  # [P2] <location> — <potential problem> (actionable)",
				"  #   problem: <one-sentence statement naming the concrete subject>",
				"  #   evidence: <quoted source content — file:line>",
				"  #   impact: <consequence if unaddressed>",
				"  #   fix: <concrete repair action>   # or decision:/options: for needs_decision",
				"  #   fact_anchor: <required for a P3 observation>",
				"")
		case gaterun.SessionKindDesign, gaterun.SessionKindArchitecture:
			prefix := "File: "
			if kind == gaterun.SessionKindArchitecture {
				prefix = "Unit: "
			}
			lines = append(lines, prefix+key, "conclusion: <acceptable|needs_attention|unacceptable> — <reason>")
			if kind == gaterun.SessionKindArchitecture {
				for _, field := range qualityDimensions {
					lines = append(lines, field+": <assessment> — <basis>")
				}
			} else {
				lines = append(lines, "spec_requirements: <active check of the unit requirements> — <basis>", "Observation disposition: <public observation id> = <retained|suppressed> — <unit-specific evidence and reason>")
			}
			lines = append(lines,
				"gate_findings: none | [P0|P1] {finding};",
				"  # gate_findings accepts only `none` or one or more [P0|P1] {finding} entries separated by `;`.",
				"  # A retained P2/P3 observation is recorded by its own finding block plus its Observation disposition, never here.")
			if kind == gaterun.SessionKindArchitecture {
				lines = append(lines, "Suppressed by spec (0):")
			}
			lines = append(lines,
				"  # a finding block is written inside this block, after the lines above and before the",
				"  # next "+prefix+"line; its id is the finding's report-order id:",
				"  #   [P2] <location> — <retained observation or new finding> (actionable)",
				"  #     problem: <problem>",
				"  #     evidence: <evidence>",
				"  #     impact: <impact>",
				"  #     fix: <repair>   # or decision:/options: for needs_decision",
				"")

		case gaterun.SessionKindCross:
			if items := crossItemsFor(run); len(items) > 0 {
				for _, item := range items {
					addLine := "Cross item: " + item + " = <PASS|FAIL> — <reason>"
					lines = append(lines, addLine)
				}
				lines = append(lines, fmt.Sprintf("Cross-check: <passed>/%d <PASS|FAIL> — <reason>", len(items)))
				if run.Gate == gaterun.GateValidate {
					lines = append(lines,
						"  # a Cross-check FAIL requires a new linked cross finding at P0/P1, and every",
						"  # validate relationship finding must be P0/P1, so a failed item also fails the summary")
				} else {
					lines = append(lines,
						"  # a Cross-check FAIL requires a new linked cross finding at P0/P1; an item FAIL",
						"  # carrying only P2/P3 findings keeps the summary as <passed>/N PASS, and still",
						"  # needs its own new finding at any severity")
				}
			} else {
				lines = append(lines, "Cross-check: <PASS|FAIL> — <reason>")
			}
		}
		c.Verdicts = append(c.Verdicts, v)
	}
	add := func(id, description, example string) {
		c.Requirements = append(c.Requirements, reportRequirement{ID: id, Description: description, When: "always", MinCount: 1, MaxCount: 1})
		if example != "" {
			lines = append(lines, example)
		}
	}
	if isUnitValidateCheck5(run, session) {
		for _, sub := range unitAcceptanceSubchecks {
			allowed := strings.Join(unitAcceptanceAllowed(sub), "|")
			add("check-"+sub, "exactly one "+sub+" verdict with reason ("+allowed+")", sub+". <name>: <"+allowed+"> — <reason>")
			c.Requirements[len(c.Requirements)-1].Allowed = unitAcceptanceAllowed(sub)
		}
	}
	if gaterun.IsItemKind(session.Kind) {
		add("mismatch-type", "MISMATCH requires one of: "+strings.Join(itemMismatchTypes, ", ")+"; when several are retained the verdict-line type names the primary one and each finding block carries its own root cause and severity", "")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].Allowed = itemMismatchTypes
		for _, field := range verifyItemFields {
			example := field + ": <value>"
			if field == "deterministic" {
				example = "deterministic: <true|false>"
			}
			add("verify-"+field, "exactly one non-empty "+field+" line per item", example)
			if field == "deterministic" {
				c.Requirements[len(c.Requirements)-1].Allowed = []string{"true", "false"}
			}
		}
		for _, field := range analysisFields {
			allowed := analysisAllowed(field)
			example := "  " + field + ": <value>"
			if len(allowed) > 0 {
				example = "  " + field + ": <" + strings.Join(allowed, "|") + ">"
			}
			add("item-"+field, "for a MISMATCH, one non-empty "+field+" line per retained mismatch finding block", example)
			c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
			c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
			c.Requirements[len(c.Requirements)-1].Allowed = allowed
		}
		add("item-finding-blocks", "for a MISMATCH, one finding block per retained mismatch, each introduced by a `Problem:` line, in report order — a single item may retain several independent mismatches", "")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
		add("item-evidence", "for a MISMATCH, each retained mismatch finding block's Evidence contains spec and code sub-lines", "  Evidence:\n    - spec: <quoted fact or absence and searched scope>\n    - code: <quoted fact or absence and searched scope>")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
		add("item-resolution", "for each MISMATCH finding block with spec_gap or code_gap use Fix; with needs_design or blocked use Decision and at least one indented Options entry", "<Fix: repair OR Decision: choice followed by Options: and an indented option>")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
	}
	if session.Kind == gaterun.SessionKindUnit {
		add("mismatch-type", "MISMATCH requires one of: "+strings.Join(unitMismatchTypes, ", ")+"; when several are retained the verdict-line type names the primary one and each finding block carries its own root cause and severity", "")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].Allowed = unitMismatchTypes
		for _, class := range unitScanClasses {
			add("unit-scan-"+class, "exactly one `declarations."+class+": checked=<n> — <result>` line; n is how many declarations of this class were located and compared — a class you did not examine is reported as checked=0 with the reason, so a skipped class is visible", "declarations."+class+": checked=<n> — <result>")
		}
		add("unit-scan-surplus", "exactly one `surplus: found=<n> — <result>` line; n is the number of surplus candidates that survived the design-decision filter", "surplus: found=<n> — <result>")
		add("unit-scan-non-runnable", "exactly one `non_runnable: count=<n> — <result>` line; n is the number of acceptance items marked `runnable: no`, and the result states whether each carries a substantive not_runnable_reason with external evidence and whether the remaining runnable set gives meaningful coverage (Step 5 Part 2)", "non_runnable: count=<n> — <result>")
		for _, field := range analysisFields {
			allowed := analysisAllowed(field)
			example := "  " + field + ": <value>"
			if len(allowed) > 0 {
				example = "  " + field + ": <" + strings.Join(allowed, "|") + ">"
			}
			add("unit-"+field, "for a MISMATCH, one non-empty "+field+" line per retained mismatch finding block", example)
			c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
			c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
			c.Requirements[len(c.Requirements)-1].Allowed = allowed
		}
		add("unit-finding-blocks", "for a MISMATCH, one finding block per retained mismatch, each introduced by a `Problem:` line, in report order — one Step 1 declaration class or Step 5 surplus construct can each yield several independent mismatches", "")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
		add("unit-evidence", "for a MISMATCH, each retained mismatch finding block's Evidence contains spec and code sub-lines", "  Evidence:\n    - spec: <quoted fact or absence and searched scope>\n    - code: <quoted fact or absence and searched scope>")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
		add("unit-resolution", "for each MISMATCH finding block with spec_gap or code_gap use Fix; with needs_design or blocked use Decision and at least one indented Options entry", "<Fix: repair OR Decision: choice followed by Options: and an indented option>")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_mismatch_finding"
	}
	if session.Kind == gaterun.SessionKindDesign {
		add("quality-file-block", "exactly one File: <assigned path> block per file; verdicts, assessments, and findings belong to that file", "")
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_file_key"
		for _, field := range []string{"spec_requirements"} {
			add("quality-"+field, "exactly one assessment and non-empty basis per file block", "")
			c.Requirements[len(c.Requirements)-1].CountBasis = "per_file_key"
		}
		add("quality-gate-findings", "exactly one gate_findings line per file block: `none` or one or more `[P0|P1] {finding}` entries separated by `;` — a retained P2/P3 observation is never listed here", "")
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_file_key"
		add("quality-finding-id-order", "finding ids are {run}/{session}/F{n} assigned in report order across the whole report (the k-th finding block in report order is F{k}), not restarted per file; a design Observation disposition copies the id of the code block that reported it", "")
		c.Requirements[len(c.Requirements)-1].When = "if_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		add("quality-p3-anchor", "every P3 finding has a fact_anchor line", "")
		c.Requirements[len(c.Requirements)-1].When = "if_p3_finding"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		add("design-finding-placement", "a design finding block is written inside the File: design:<unit>:<file> block of the file it judges, after that block's gate_findings: line and before the next File: line; a finding whose location names another file this session judges is rejected, because a finding binds to the File: block it appears in — a finding listed after the last file block binds to the last file's key", "")
		c.Requirements[len(c.Requirements)-1].When = "if_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
	}
	if len(codeKeys) > 0 {
		add("public-facts", "one non-empty facts assessment per assigned file; the tooling records each check's own public evidence surface (its coverage key's read refs) as its dependency", "")
		add("public-finding-placement", "a public potential finding is a finding block written inside its own File: code:<file> block, after facts: and before the next File: line; observations never appear in a design block", "")
		c.Requirements[len(c.Requirements)-1].When = "if_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
	}
	if session.Kind == gaterun.SessionKindArchitecture {
		for _, field := range qualityDimensions {
			add("architecture-"+field, "one non-empty architecture assessment and basis for "+field, "")
		}
		add("architecture-gate-findings", "one gate_findings line", "")
		add("architecture-suppressed", "one Suppressed by spec (N) block", "")
	}

	if session.Kind == gaterun.SessionKindCross {
		for _, item := range crossItemsFor(run) {
			add("cross-item-"+item, "exactly one "+item+" result with PASS or FAIL and a non-empty reason", "")
			c.Requirements[len(c.Requirements)-1].Allowed = []string{"PASS", "FAIL"}
		}
		if len(crossItemsFor(run)) > 0 {
			desc := "one link from each failed Cross item to a new retained cross finding, at any severity; no link for a passing item"
			if run.Gate == gaterun.GateValidate {
				desc = "one link from each failed Cross item to a new retained cross finding at P0/P1; no link for a passing item"
			}
			add("cross-item-finding", desc, "Cross item finding: <failed_item_key> = <new_cross_finding_id>")
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
		add("cross-finding-affects", "each new cross finding names the non-cross logical keys it makes fail, written as the logical keys the dependency judgments show (validate@unit check numbers; verify full coverage keys — item:<unit>:<item>, unit:<unit>, code:<file>, design:<unit>:<file>, architecture:<unit>; or a relationship:<name> key); the finding id is the run-scoped {run_id}/{session_id}/F{n}", "")
		c.Requirements[len(c.Requirements)-1].When = "if_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		if run.Gate == gaterun.GateVerify {
			add("cross-quality-conclusions", "one author-declared final quality conclusion with reason per design and architecture key, including carried judgments; unacceptable iff a finalized gate-driving P0/P1 finding affects that key; do not infer quality from pass/fail", "Quality conclusion: <design_or_architecture_key> = <acceptable|needs_attention|unacceptable> — <reason>")
			c.Requirements[len(c.Requirements)-1].CountBasis = "unit_quality_keys"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
			c.Requirements[len(c.Requirements)-1].Allowed = []string{"acceptable", "needs_attention", "unacceptable"}
			add("cross-ownership", "ownership record for every finding deferred to another unit", "Finding ownership: <finding_id> = owned_by <unit> — evidence: <read_ref>; reason: <reason>")
			c.Requirements[len(c.Requirements)-1].When = "if_deferred_finding"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
		}
	}
	if !gaterun.IsAlignmentKind(session.Kind) {
		finding := "[P1] <location> — <finding> (actionable|needs_decision)\n  problem: <problem>\n  evidence: <evidence>\n  impact: <impact>\n  fix: <repair>"
		if session.Kind == gaterun.SessionKindCross {
			finding = "[P1] <location> — <new cross finding>\n  problem: <problem>\n  evidence: <evidence>\n  impact: <impact>\n  fix: <repair>\nFinding affects: <new_finding_id> = <check_key>"
		}
		if gaterun.IsQualityKind(session.Kind) {
			finding = ""
		}
		add("finding-block", "if findings are present, use the checklist's contiguous finding detail block", finding)
		c.Requirements[len(c.Requirements)-1].When = "if_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		if session.Kind == gaterun.SessionKindChecks {
			add("finding-id-order", "finding ids are {run_id}/{session_id}/F{n} assigned in report order across the whole report (the k-th finding block is F{k}); the run id and session id are printed in the mission's Run: and Session: lines", "")
			c.Requirements[len(c.Requirements)-1].When = "if_findings"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
		}
		if session.Kind == gaterun.SessionKindChecks && len(session.CheckKeys) > 1 {
			add("finding-affects", "each finding declares exactly the assigned report check keys it affects, written as the mission prints them in its Checks: line; every FAIL key has its own P0/P1 finding", "Finding affects: <finding_id> = <check_key>[,<check_key>...]")
			c.Requirements[len(c.Requirements)-1].When = "if_findings"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
		}
	}
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

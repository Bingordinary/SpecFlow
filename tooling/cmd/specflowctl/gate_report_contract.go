package main

import (
	"fmt"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// These declarations drive both session instructions and fixed-field report
// validation. Dynamic synthesis checks remain in gate-submit.
var unitAcceptanceSubchecks = []string{"5a", "5b", "5c", "5d", "5e", "5f", "5g", "5h", "5i"}
var verifyItemFields = []string{"evidence", "deterministic", "Part A", "Part B"}
var qualityDimensions = []string{"module_boundaries", "responsibility_organization", "dependency_clarity", "abstraction_level", "extension_landing_points", "engineering_patterns"}
var analysisFields = []string{"Problem", "Impact", "Root cause", "Suggested direction", "Severity", "Confidence"}
var mismatchTypes = []string{"structural", "acceptance", "scope", "stub", "surplus"}
var analysisRootCauses = []string{"incomplete", "stale", "shadow_spec", "divergence", "accident", "blocked"}
var analysisDirections = []string{"spec_gap", "code_gap", "needs_design", "blocked"}
var analysisConfidences = []string{"high", "medium", "low"}
var severityLevels = []string{"P0", "P1", "P2", "P3"}
var crossDispositions = []string{"retained", "suppressed", "merged"}
var crossStatuses = []string{"pass", "fail"}
var verifyCrossItems = gaterun.RelationshipNames(gaterun.GateVerify)
var validateCrossItems = gaterun.RelationshipNames(gaterun.GateValidate)

func crossItemsFor(run *gaterun.Run) []string {
	return run.Relationships
}

func unitAcceptanceAllowed(subcheck string) []string {
	if subcheck == "5a" {
		return []string{"PASS", "WARNING", "FAIL"}
	}
	return []string{"PASS", "FAIL"}
}

// isUnitValidateCheck5 reports whether the session judges the unit validate
// Check 5 batch (acceptance coverage & correctness), whose report carries the
// fixed 5a-5i sub-check lines in addition to the check verdict.
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
	case gaterun.SessionKindItem, gaterun.SessionKindPreserve:
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
		case gaterun.SessionKindItem, gaterun.SessionKindPreserve:
			lines = append(lines,
				fmt.Sprintf("%s: <ALIGNED|MISMATCH (type)|CANNOT_DETERMINE> — <reason>", key),
				"  evidence: <file:line or quoted fact>",
				"  deterministic: <true|false>",
				"  Part A: <concerns or No concerns>",
				"  Part B: <test-quality assessment or skipped — <reason>>",
				"  # for a MISMATCH, append the finding fields inside the same item block:",
				"  Problem: <one line>",
				"  Evidence:",
				"    - spec: <quoted fact or absence>",
				"    - code: <quoted fact or absence>",
				"  Impact: <one line>",
				"  Fix: <repair>",
				"  Root cause: <incomplete|stale|shadow_spec|divergence|accident|blocked>",
				"  Suggested direction: <spec_gap|code_gap|needs_design|blocked>",
				"  Severity: <P0|P1|P2|P3>",
				"  Confidence: <high|medium|low>")
		case gaterun.SessionKindCode:
			lines = append(lines, "File: "+key, "conclusion: FACTS", "facts: <code facts and potential problems; no unit design rationale>")
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
			lines = append(lines, "gate_findings: <none or blocking findings>")
			if kind == gaterun.SessionKindArchitecture {
				lines = append(lines, "Suppressed by spec (0):")
			}
			lines = append(lines, "Dependency scope:", key+": <read_ref>: <section|range|all>", "")

		case gaterun.SessionKindCross:
			if items := crossItemsFor(run); len(items) > 0 {
				for _, item := range items {
					addLine := "Cross item: " + item + " = <PASS|FAIL> — <reason>"
					lines = append(lines, addLine)
				}
				lines = append(lines, fmt.Sprintf("Cross-check: <passed>/%d <PASS|FAIL> — <reason>", len(items)))
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
		add("mismatch-type", "MISMATCH requires one of: "+strings.Join(mismatchTypes, ", "), "")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].Allowed = mismatchTypes
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
		add("part-b-skipped", "a skipped Part B requires a reason", "")
		c.Requirements[len(c.Requirements)-1].When = "if_part_b_skipped"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		for _, field := range analysisFields {
			allowed := analysisAllowed(field)
			example := "  " + field + ": <value>"
			if len(allowed) > 0 {
				example = "  " + field + ": <" + strings.Join(allowed, "|") + ">"
			}
			add("item-"+field, "for a MISMATCH, exactly one non-empty "+field+" line in the item block", example)
			c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].Allowed = allowed
		}
		add("item-evidence", "for a MISMATCH, the item block's Evidence contains spec and code sub-lines", "  Evidence:\n    - spec: <quoted fact or absence and searched scope>\n    - code: <quoted fact or absence and searched scope>")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		add("item-resolution", "for a MISMATCH with spec_gap or code_gap use Fix; with needs_design or blocked use Decision and at least one indented Options entry", "<Fix: repair OR Decision: choice followed by Options: and an indented option>")
		c.Requirements[len(c.Requirements)-1].When = "if_mismatch"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
	}
	if session.Kind == gaterun.SessionKindDesign {
		add("quality-file-block", "exactly one File: <assigned path> block per file; verdicts, assessments, findings and dependency scopes belong to that file", "")
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_file_key"
		for _, field := range []string{"spec_requirements"} {
			add("quality-"+field, "exactly one assessment and non-empty basis per file block", "")
			c.Requirements[len(c.Requirements)-1].CountBasis = "per_file_key"
		}
		add("quality-gate-findings", "exactly one gate_findings line per file block", "")
		c.Requirements[len(c.Requirements)-1].CountBasis = "per_file_key"
		add("quality-p3-anchor", "every P3 finding has a fact_anchor line", "")
		c.Requirements[len(c.Requirements)-1].When = "if_p3_finding"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
	}
	if len(codeKeys) > 0 {
		add("public-facts", "one non-empty facts assessment per assigned file; the tooling records each check's own public evidence surface (its coverage key's read refs) as its dependency", "")
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
			add("cross-item-finding", "one link from each failed Cross item to a new retained cross finding; no link for a passing item", "Cross item finding: <failed_item_key> = <new_cross_finding_id>")
			c.Requirements[len(c.Requirements)-1].When = "if_cross_item_fail"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
			c.Requirements[len(c.Requirements)-1].CountBasis = "failed_cross_items"
		}
		for _, item := range run.Relationships {
			lines = append(lines, gaterun.RelationshipKey(item)+": <read_ref>: <section|range|all>")
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
		if run.Gate == gaterun.GateVerify {
			add("cross-quality-conclusions", "one author-declared final quality conclusion with reason per design and architecture key, including carried judgments; unacceptable iff finalized gate-driving P0/P1 findings exist; do not infer quality from pass/fail", "Quality conclusion: <design_or_architecture_key> = <acceptable|needs_attention|unacceptable> — <reason>")
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
	if !gaterun.IsItemKind(session.Kind) {
		finding := "[P1] <location> — <finding> (actionable|needs_decision)\n  problem: <problem>\n  evidence: <evidence>\n  impact: <impact>\n  fix: <repair>"
		if session.Kind == gaterun.SessionKindCross {
			finding = "[P1] <location> — <new cross finding>\nFinding affects: <new_finding_id> = <check_key>"
		}
		if gaterun.IsQualityKind(session.Kind) {
			finding = ""
		}
		add("finding-block", "if findings are present, use the checklist's contiguous finding detail block", finding)
		c.Requirements[len(c.Requirements)-1].When = "if_findings"
		c.Requirements[len(c.Requirements)-1].MinCount = 0
		c.Requirements[len(c.Requirements)-1].MaxCount = -1
		if session.Kind == gaterun.SessionKindChecks && len(session.CheckKeys) > 1 {
			add("finding-affects", "each finding declares exactly the assigned report check keys it affects; every FAIL key has its own P0/P1 finding", "Finding affects: <finding_id> = <check_key>[,<check_key>...]")
			c.Requirements[len(c.Requirements)-1].When = "if_findings"
			c.Requirements[len(c.Requirements)-1].MinCount = 0
			c.Requirements[len(c.Requirements)-1].MaxCount = -1
		}
	}
	scopeExample := "Dependency scope:\n  <check_key>: <read_ref>: <section|range|acceptance_item:id|acceptance_items|all>"
	scopeDescription := "at least one declaration per executed check key; file must be in read_refs"
	switch {
	case len(codeKeys) == len(session.CheckKeys):
		scopeDescription = "no Dependency scope lines: the tooling records each check's own public evidence surface (its coverage key's read refs) as its whole-file dependency"
		scopeExample = "# No Dependency scope lines — the tooling records each public code check's own public evidence surface (its coverage key's read refs)."
	case len(codeKeys) > 0:
		scopeDescription = "at least one declaration per executed design or architecture check key; file must be in read_refs (public code checks need none — the tooling records their whole public evidence surface)"
	}
	if session.Kind == gaterun.SessionKindCross {
		scopeExample = "# Declare each assigned relationship's read evidence under relationship:<name>; use cross: for finding-disposition evidence."
		if len(session.Relationships) == 0 {
			scopeExample += "\nDependency scope:\n  cross: <read_ref>: <section|range|all>"
		}
	}
	if session.Kind == gaterun.SessionKindDesign {
		scopeExample = ""
	}
	add("dependency-scope", scopeDescription, scopeExample)
	c.Requirements[len(c.Requirements)-1].MaxCount = -1
	c.Requirements[len(c.Requirements)-1].CountBasis = "per_check_key"
	if len(codeKeys) == len(session.CheckKeys) {
		c.Requirements[len(c.Requirements)-1].MinCount = 0
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

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// parsedReport is the mechanically extracted content of one packet report:
// verdicts, dependency declarations, findings, synthesis records, and
// evidence-backed severity confirmations.
type parsedReport struct {
	Verdicts        map[string]string // check key -> verdict token
	Scopes          []parsedScope
	Findings        []gaterun.Finding
	EffectiveStatus map[string]string
	Dispositions    []gaterun.FindingDisposition
	SeverityChecks  []gaterun.SeverityConfirmation
	Ownerships      []gaterun.FindingOwnership
	Analysis        map[string]string
	GateFindings    string // review file packets: the gate_findings line content (`none` or [P0|P1] entries)
}

// parsedScope is one `{check key}: {file}: {declaration}` line.
type parsedScope struct {
	Key         string
	Path        string
	Declaration string // all | line ranges | acceptance_items | section heading
}

// verifyMismatchTypes is the fixed MISMATCH type vocabulary of the verify
// detection verdict (`MISMATCH (type)`; see framework/verification_scope.md
// §Sub-agent Prompt Assembly).
var verifyMismatchTypes = map[string]bool{
	"structural": true,
	"acceptance": true,
	"scope":      true,
	"stub":       true,
	"surplus":    true,
}

var (
	rangeDeclRe         = regexp.MustCompile(`^\d+-\d+(,\d+-\d+)*$`)
	findingRe           = regexp.MustCompile(`^[ \t]*-?[ \t]*\[(P0|P1|P2|P3)\][ \t]*(.+)$`)
	resolutionLabelRe   = regexp.MustCompile(`\((?:actionable|needs_decision)\)\s*$`)
	factAnchorRe        = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*fact_anchor:\s*\S`)
	gateFindingsRe      = regexp.MustCompile(`(?mi)^[ \t]*gate_findings:\s*(\S[^\n]*)$`)
	gateFindingsEntryRe = regexp.MustCompile(`^\[(?:P0|P1)\][ \t]+\S`)
	alnumRe             = regexp.MustCompile(`[A-Za-z0-9]`)
	effectiveStatusRe   = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Effective status:\s*(.+?)\s*=\s*(pass|fail)\s*$`)
	dispositionRe       = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding disposition:\s*(\S+)\s*=\s*(retained|suppressed|merged)(?:\s*->\s*(\S+))?(?:\s+[—-]\s+(.+))?\s*$`)
	findingAffectsRe    = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding affects:\s*(\S+)\s*=\s*(.+?)\s*$`)
	severityCheckRe     = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Severity confirmation:\s*(\S+)\s*=\s*(confirmed|adjusted)\s+(P0|P1|P2|P3)(?:\s*->\s*(P0|P1|P2|P3))?\s+[—-]\s+evidence:\s*(.+?)\s*;\s*reason:\s*(.+?)\s*$`)
	severityCheckLineRe = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Severity confirmation:`)
	ownershipRe         = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding ownership:\s*(\S+)\s*=\s*owned_by\s+(\S+)\s+[—-]\s+evidence:\s*(.+?)\s*;\s*reason:\s*(.+?)\s*$`)
	ownershipLineRe     = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding ownership:`)
)

// itemDeclPrefix is the reserved declaration prefix for acceptance item
// regions: `acceptance_item:<id>[,<id>...]`.
const itemDeclPrefix = "acceptance_item:"

// declParts is one `Dependency scope:` declaration parsed into its forms.
// The zero value declares nothing; WholeFile ("all") covers every other form.
// Accepts (the whole item set) and Items (specific item regions) may coexist
// — the deps are the union (declare-heavy conservatism).
type declParts struct {
	WholeFile bool
	Ranges    []string
	Sections  []string
	Accepts   bool
	Items     []string
}

// parseDecl classifies one dependency-scope declaration string. The grammar
// is shared by the packet-report validation (gate-submit) and the cache
// assembly (gate-finalize) so the two can never drift.
func parseDecl(decl string) (declParts, error) {
	decl = strings.TrimSpace(decl)
	switch {
	case decl == "" || decl == "all":
		return declParts{WholeFile: true}, nil
	case rangeDeclRe.MatchString(decl):
		return declParts{Ranges: []string{decl}}, nil
	case decl == "acceptance_items":
		return declParts{Accepts: true}, nil
	case strings.HasPrefix(decl, itemDeclPrefix):
		var items []string
		for _, tok := range strings.Split(strings.TrimPrefix(decl, itemDeclPrefix), ",") {
			id := strings.TrimSpace(tok)
			if id == "" {
				return declParts{}, fmt.Errorf("declaration %q carries an empty acceptance item id", decl)
			}
			items = append(items, id)
		}
		return declParts{Items: items}, nil
	default:
		return declParts{Sections: []string{decl}}, nil
	}
}

// mergeDecl merges one parsed declaration into the accumulator of one
// (check, path) pair. A whole-file declaration ("all") covers every other
// form, so it replaces them regardless of line order.
func mergeDecl(m *declParts, d declParts) {
	if d.WholeFile {
		*m = declParts{WholeFile: true}
		return
	}
	if m.WholeFile {
		return
	}
	m.Ranges = append(m.Ranges, d.Ranges...)
	m.Sections = append(m.Sections, d.Sections...)
	m.Accepts = m.Accepts || d.Accepts
	m.Items = append(m.Items, d.Items...)
}

// parsePacketReport validates one packet report against its planned packet:
// every check key has exactly one verdict line with an allowed token, blocking
// verdicts carry a reason, and every check key declares at least one
// dependency-scope line whose file belongs to the run snapshot. See
// framework/verification_scope.md §Gate Work Packets → Packet report contract.
func parsePacketReport(run *gaterun.Run, spec *gaterun.PacketSpec, report string) (*parsedReport, error) {
	if strings.TrimSpace(report) == "" {
		return nil, fmt.Errorf("empty report")
	}
	report = strings.ReplaceAll(report, "\r\n", "\n")
	out := &parsedReport{Verdicts: map[string]string{}, EffectiveStatus: map[string]string{}, Analysis: map[string]string{}}
	verdictLines := map[int]bool{}
	if spec.Kind == gaterun.PacketKindAnalysis {
		if err := parseAnalysisReport(run, spec, report, out); err != nil {
			return nil, err
		}
	} else {
		for _, key := range spec.CheckKeys {
			token, line, lineIdx, err := extractVerdict(spec, key, report)
			if err != nil {
				return nil, err
			}
			if needsReason(spec, token) && !hasReason(line, token) {
				return nil, fmt.Errorf("check %q verdict %s carries no reason", key, token)
			}
			out.Verdicts[key] = token
			verdictLines[lineIdx] = true
		}
		if err := validatePacketBodyStructure(run, spec, report); err != nil {
			return nil, err
		}
	}
	scopes, err := extractScopes(run, spec, report, verdictLines)
	if err != nil {
		return nil, err
	}
	for _, key := range spec.CheckKeys {
		found := false
		for _, s := range scopes {
			if s.Key == key {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("check %q declares no Dependency scope line (`{check key}: {file}: {declaration}`)", key)
		}
	}
	out.Scopes = scopes
	if spec.Kind != gaterun.PacketKindAnalysis {
		// Finding ids are run-scoped — {run_id}/{packet_id}/F{n} — so they
		// are unique by construction across runs: a finding authored by this
		// report can never collide with a carried finding from an earlier
		// run (see framework/verification_scope.md §Gate Work Packets →
		// Packet report contract).
		for i, extracted := range extractFindings(report) {
			// The unified finding format carries a resolution label on the
			// entry line (framework/_atoms/misc/report_skeleton.md); cross
			// reports author new findings in the cross synthesis format,
			// which has no label.
			if spec.Kind != gaterun.PacketKindCross && !resolutionLabelRe.MatchString(extracted.text) {
				return nil, fmt.Errorf("finding [%s] %s carries no resolution label (`(actionable)` or `(needs_decision)`)", extracted.severity, extracted.text)
			}
			// Every review P3 finding must carry the fact anchor required by
			// the review checklist's P3 reportability gate.
			if spec.Kind == gaterun.PacketKindFile && extracted.severity == "P3" && !factAnchorRe.MatchString(extracted.detail) {
				return nil, fmt.Errorf("review P3 finding [%s] %s carries no non-empty fact_anchor detail line", extracted.severity, extracted.text)
			}
			sourceKey := spec.PacketID
			if len(spec.CheckKeys) == 1 {
				sourceKey = spec.CheckKeys[0]
			}
			out.Findings = append(out.Findings, gaterun.Finding{
				ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.PacketID, i+1),
				Severity:  extracted.severity,
				Text:      extracted.text,
				Detail:    extracted.detail,
				SourceKey: sourceKey,
			})
		}
	}
	if spec.Kind == gaterun.PacketKindFile {
		gateFindings, err := extractGateFindings(report)
		if err != nil {
			return nil, err
		}
		out.GateFindings = gateFindings
	}
	if spec.Kind == gaterun.PacketKindCross {
		if err := parseCrossSynthesis(report, out); err != nil {
			return nil, err
		}
	}
	if err := parseSeverityConfirmations(report, out); err != nil {
		return nil, err
	}
	return out, nil
}

func validatePacketBodyStructure(run *gaterun.Run, spec *gaterun.PacketSpec, report string) error {
	switch {
	case run.Gate == gaterun.GateValidate && run.TargetKind == gaterun.TargetKindUnit && spec.Kind == gaterun.PacketKindChecks && stringInList(spec.CheckKeys, "5"):
		return validateUnitAcceptanceBody(report)
	case run.Gate == gaterun.GateVerify && spec.Kind == gaterun.PacketKindItem:
		return validateVerifyItemBody(report)
	case run.Gate == gaterun.GateReview && spec.Kind == gaterun.PacketKindFile:
		return validateReviewFileBody(report)
	default:
		return nil
	}
}

func validateUnitAcceptanceBody(report string) error {
	for _, subcheck := range []string{"5a", "5b", "5c", "5d", "5e", "5f", "5g", "5h", "5i"} {
		allowed := "PASS|FAIL"
		if subcheck == "5a" {
			allowed = "PASS|WARNING|FAIL"
		}
		re := regexp.MustCompile(`(?m)^[ \t]*` + subcheck + `\.[^\n:]*:\s*(` + allowed + `)\b[^\n]*$`)
		matches := re.FindAllStringSubmatch(report, -1)
		if len(matches) != 1 {
			return fmt.Errorf("unit validate Check 5 report must declare exactly one %s verdict line", subcheck)
		}
		if !hasReason(matches[0][0], matches[0][1]) {
			return fmt.Errorf("unit validate Check 5 sub-check %s carries no reason", subcheck)
		}
	}
	return nil
}

func validateVerifyItemBody(report string) error {
	required := []struct {
		name string
		pat  string
	}{
		{"evidence", `(?mi)^[ \t]*-?[ \t]*evidence:\s*(\S[^\n]*)$`},
		{"deterministic", `(?mi)^[ \t]*-?[ \t]*deterministic:\s*(true|false)\s*$`},
		{"Part A", `(?mi)^[ \t]*Part A:\s*(\S[^\n]*)$`},
		{"Part B", `(?mi)^[ \t]*Part B:\s*(\S[^\n]*)$`},
	}
	for _, field := range required {
		if count := len(regexp.MustCompile(field.pat).FindAllStringSubmatch(report, -1)); count != 1 {
			return fmt.Errorf("verify item report must declare exactly one non-empty %s field", field.name)
		}
	}
	// When Part B does not apply, the report must use the explicit
	// `skipped — {reason}` form; a bare `skipped` carries no reason.
	if partB := regexp.MustCompile(`(?mi)^[ \t]*Part B:\s*(\S[^\n]*)$`).FindStringSubmatch(report); partB != nil {
		content := strings.TrimSpace(partB[1])
		if strings.HasPrefix(strings.ToLower(content), "skipped") {
			reason := strings.TrimSpace(content[len("skipped"):])
			reason = strings.Trim(reason, " \t—–-:;,.")
			if reason == "" {
				return fmt.Errorf("verify item report Part B `skipped` must carry a reason (`skipped — {reason}`)")
			}
		}
	}
	return nil
}

func validateReviewFileBody(report string) error {
	for _, field := range []string{"module_boundaries", "responsibility_organization", "dependency_clarity", "abstraction_level", "extension_landing_points", "engineering_patterns"} {
		re := regexp.MustCompile(`(?mi)^[ \t]*` + regexp.QuoteMeta(field) + `:\s*(\S[^\n]*?)\s+[—-]\s+(\S[^\n]*)$`)
		if count := len(re.FindAllStringSubmatch(report, -1)); count != 1 {
			return fmt.Errorf("review file report must declare exactly one %s assessment with a non-empty basis", field)
		}
	}
	required := []struct {
		name string
		pat  string
	}{
		{"gate_findings", `(?mi)^[ \t]*gate_findings:\s*(\S[^\n]*)$`},
		{"Suppressed by spec", `(?mi)^[ \t]*Suppressed by spec\s*\(\d+\):\s*$`},
	}
	for _, field := range required {
		if count := len(regexp.MustCompile(field.pat).FindAllStringSubmatch(report, -1)); count != 1 {
			return fmt.Errorf("review file report must declare exactly one %s field", field.name)
		}
	}
	return nil
}

func stringInList(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func parseSeverityConfirmations(report string, out *parsedReport) error {
	matches := severityCheckRe.FindAllStringSubmatch(report, -1)
	if len(matches) != len(severityCheckLineRe.FindAllString(report, -1)) {
		return fmt.Errorf("malformed Severity confirmation line; use `{finding_id} = confirmed {Px} — evidence: {path}; reason: {text}` or `{finding_id} = adjusted {Px} -> {Py} — evidence: {path}; reason: {text}`")
	}
	for _, m := range matches {
		id := strings.TrimSpace(m[1])
		outcome := m[2]
		original := m[3]
		final := m[4]
		if outcome == "confirmed" {
			if final != "" {
				return fmt.Errorf("confirmed severity for finding %q must not declare an adjusted level", id)
			}
			final = original
		} else if final == "" {
			return fmt.Errorf("adjusted severity for finding %q must declare the final level", id)
		}
		out.SeverityChecks = append(out.SeverityChecks, gaterun.SeverityConfirmation{
			FindingID:        id,
			Outcome:          outcome,
			OriginalSeverity: original,
			FinalSeverity:    final,
			EvidencePath:     strings.TrimSpace(m[5]),
			Reason:           strings.TrimSpace(m[6]),
		})
	}
	return nil
}

type extractedFinding struct {
	severity string
	text     string
	detail   string
}

// extractFindings captures the canonical finding line and its contiguous
// indented detail lines. Dependency declarations and later report sections
// are deliberately excluded, so the stored detail can be rendered on its own
// when a delta/repair run carries the finding forward. A finding entry at the
// same or shallower indentation than the current entry starts a new finding —
// an indented sibling entry must never be absorbed as detail. A deeper-indented
// `[Px]` line is a quoted detail line and stays part of the block.
func extractFindings(report string) []extractedFinding {
	lines := strings.Split(report, "\n")
	var out []extractedFinding
	for i := 0; i < len(lines); i++ {
		match := findingRe.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		entryIndent := indentWidth(lines[i])
		block := []string{strings.TrimSpace(lines[i])}
		for j := i + 1; j < len(lines); j++ {
			line := strings.TrimRight(lines[j], " \t")
			if strings.TrimSpace(line) == "" || (line[0] != ' ' && line[0] != '\t') {
				break
			}
			if findingRe.MatchString(line) && indentWidth(line) <= entryIndent {
				break
			}
			block = append(block, line)
			i = j
		}
		out = append(out, extractedFinding{
			severity: match[1],
			text:     strings.TrimSpace(match[2]),
			detail:   strings.Join(block, "\n"),
		})
	}
	return out
}

// indentWidth returns the number of leading space or tab characters on a
// line. It is the indentation scale compared when deciding whether an
// indented `[Px]` line opens a new finding entry or continues the current
// entry's detail.
func indentWidth(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func parseAnalysisReport(run *gaterun.Run, spec *gaterun.PacketSpec, report string, out *parsedReport) error {
	if len(spec.CheckKeys) != 1 {
		return fmt.Errorf("analysis packet %q must own exactly one item", spec.PacketID)
	}
	item := spec.CheckKeys[0]
	fields := []string{"Problem", "Impact", "Root cause", "Suggested direction", "Severity", "Confidence"}
	for _, field := range fields {
		re := regexp.MustCompile(`(?mi)^[ \t]*` + regexp.QuoteMeta(field) + `:\s*([^\n]+)$`)
		matches := re.FindAllStringSubmatch(report, -1)
		if len(matches) != 1 || strings.TrimSpace(matches[0][1]) == "" {
			return fmt.Errorf("analysis %q must declare exactly one non-empty %s field", item, field)
		}
		out.Analysis[field] = strings.TrimSpace(matches[0][1])
	}
	itemRe := regexp.MustCompile(`(?mi)^[ \t]*Item:\s*` + regexp.QuoteMeta(item) + `\s*$`)
	if len(itemRe.FindAllString(report, -1)) != 1 {
		return fmt.Errorf("analysis report must declare exactly one `Item: %s` line", item)
	}
	evidence, err := analysisEvidenceBlock(report)
	if err != nil {
		return fmt.Errorf("analysis %q: %w", item, err)
	}
	severity := strings.ToUpper(out.Analysis["Severity"])
	if severity != "P0" && severity != "P1" && severity != "P2" && severity != "P3" {
		return fmt.Errorf("analysis %q has invalid Severity %q", item, out.Analysis["Severity"])
	}
	if !oneOf(out.Analysis["Root cause"], "incomplete", "stale", "shadow_spec", "divergence", "accident", "blocked") {
		return fmt.Errorf("analysis %q has invalid Root cause %q", item, out.Analysis["Root cause"])
	}
	direction := out.Analysis["Suggested direction"]
	if !oneOf(direction, "spec_gap", "code_gap", "needs_design", "blocked") {
		return fmt.Errorf("analysis %q has invalid Suggested direction %q", item, direction)
	}
	if !oneOf(strings.ToLower(out.Analysis["Confidence"]), "high", "medium", "low") {
		return fmt.Errorf("analysis %q has invalid Confidence %q", item, out.Analysis["Confidence"])
	}
	out.Verdicts[item] = "MISMATCH"
	resolution := "actionable"
	if direction == "needs_design" || direction == "blocked" {
		resolution = "needs_decision"
	}
	fixBlock, err := analysisFixBlock(report, resolution)
	if err != nil {
		return fmt.Errorf("analysis %q: %w", item, err)
	}
	var evidenceRendered strings.Builder
	for _, line := range evidence {
		evidenceRendered.WriteString("\n    " + line)
	}
	detail := fmt.Sprintf("[%s] %s — verify mismatch analysis (%s)\n  problem: %s\n  evidence:%s\n  impact: %s\n%s\n  root_cause: %s\n  direction: %s (confidence: %s)",
		severity, item, resolution, out.Analysis["Problem"], evidenceRendered.String(), out.Analysis["Impact"], fixBlock, out.Analysis["Root cause"], direction, strings.ToLower(out.Analysis["Confidence"]))
	out.Findings = []gaterun.Finding{{
		ID:        fmt.Sprintf("%s/%s/F1", run.RunID, spec.PacketID),
		Severity:  severity,
		Text:      "verify mismatch analysis for " + item,
		Detail:    detail,
		SourceKey: item,
	}}
	return nil
}

// analysisBlockLines returns the contiguous indented sub-lines that follow the
// `{name}:` header line of an analysis report. The block ends at the first
// blank line or line that is not indented; a missing header yields no lines.
func analysisBlockLines(report, name string) []string {
	headerRe := regexp.MustCompile(`(?mi)^[ \t]*` + regexp.QuoteMeta(name) + `:\s*$`)
	loc := headerRe.FindStringIndex(report)
	if loc == nil {
		return nil
	}
	lines := strings.Split(report[loc[1]:], "\n")
	var out []string
	for _, raw := range lines[1:] {
		if strings.TrimSpace(raw) == "" || (raw[0] != ' ' && raw[0] != '\t') {
			break
		}
		out = append(out, strings.TrimSpace(raw))
	}
	return out
}

// analysisEvidenceBlock requires the finding block's Evidence section: at least
// one spec-side and one code-side sub-line (see
// framework/verification_scope.md §Gate Work Packets → Packet report contract).
func analysisEvidenceBlock(report string) ([]string, error) {
	lines := analysisBlockLines(report, "Evidence")
	hasSpec, hasCode := false, false
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "- spec:") {
			hasSpec = true
		}
		if strings.HasPrefix(lower, "- code:") {
			hasCode = true
		}
	}
	if !hasSpec || !hasCode {
		return nil, fmt.Errorf("Evidence must carry at least one `- spec:` and one `- code:` sub-line")
	}
	return lines, nil
}

// analysisFixBlock requires exactly one of Fix (actionable) or Decision with at
// least one Options entry (needs_decision), matching the suggested direction.
func analysisFixBlock(report, resolution string) (string, error) {
	fixRe := regexp.MustCompile(`(?mi)^[ \t]*Fix:\s*([^\n]+)$`)
	decisionRe := regexp.MustCompile(`(?mi)^[ \t]*Decision:\s*([^\n]+)$`)
	fixes := fixRe.FindAllStringSubmatch(report, -1)
	decisions := decisionRe.FindAllStringSubmatch(report, -1)
	if resolution == "actionable" {
		if len(fixes) != 1 || strings.TrimSpace(fixes[0][1]) == "" {
			return "", fmt.Errorf("actionable analysis must declare exactly one non-empty Fix field")
		}
		if len(decisions) != 0 {
			return "", fmt.Errorf("actionable analysis must not declare a Decision field")
		}
		return "  fix: " + strings.TrimSpace(fixes[0][1]), nil
	}
	if len(decisions) != 1 || strings.TrimSpace(decisions[0][1]) == "" {
		return "", fmt.Errorf("needs_decision analysis must declare exactly one non-empty Decision field")
	}
	if len(fixes) != 0 {
		return "", fmt.Errorf("needs_decision analysis must not declare a Fix field")
	}
	options := analysisBlockLines(report, "Options")
	if len(options) == 0 {
		return "", fmt.Errorf("needs_decision analysis must declare at least one Options entry")
	}
	block := "  decision: " + strings.TrimSpace(decisions[0][1]) + "\n  options:"
	for _, option := range options {
		block += "\n    " + option
	}
	return block, nil
}

func oneOf(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func parseCrossSynthesis(report string, out *parsedReport) error {
	for _, m := range effectiveStatusRe.FindAllStringSubmatch(report, -1) {
		key := strings.TrimSpace(m[1])
		if key == "" {
			return fmt.Errorf("cross report carries an empty Effective status key")
		}
		if _, exists := out.EffectiveStatus[key]; exists {
			return fmt.Errorf("cross report declares Effective status for %q more than once", key)
		}
		out.EffectiveStatus[key] = m[2]
	}
	seen := map[string]bool{}
	for _, m := range dispositionRe.FindAllStringSubmatch(report, -1) {
		id, action := m[1], m[2]
		if seen[id] {
			return fmt.Errorf("cross report disposes finding %q more than once", id)
		}
		seen[id] = true
		d := gaterun.FindingDisposition{FindingID: id, Action: action, TargetID: strings.TrimSpace(m[3]), Reason: strings.TrimSpace(m[4])}
		if (action == "suppressed" || action == "merged") && d.Reason == "" {
			return fmt.Errorf("finding disposition %q (%s) requires a reason", id, action)
		}
		if action == "merged" && d.TargetID == "" {
			return fmt.Errorf("finding disposition %q (merged) requires a target id", id)
		}
		out.Dispositions = append(out.Dispositions, d)
	}
	findingByID := map[string]*gaterun.Finding{}
	for i := range out.Findings {
		findingByID[out.Findings[i].ID] = &out.Findings[i]
	}
	affectedSeen := map[string]bool{}
	for _, m := range findingAffectsRe.FindAllStringSubmatch(report, -1) {
		id := strings.TrimSpace(m[1])
		finding := findingByID[id]
		if finding == nil {
			return fmt.Errorf("cross report declares affected keys for unknown new finding %q", id)
		}
		if affectedSeen[id] {
			return fmt.Errorf("cross report declares affected keys for finding %q more than once", id)
		}
		affectedSeen[id] = true
		seenKeys := map[string]bool{}
		for _, raw := range strings.Split(m[2], ",") {
			key := strings.TrimSpace(raw)
			if key == "" {
				return fmt.Errorf("cross finding %q carries an empty affected key", id)
			}
			if !seenKeys[key] {
				seenKeys[key] = true
				finding.AffectedKeys = append(finding.AffectedKeys, key)
			}
		}
	}
	ownershipMatches := ownershipRe.FindAllStringSubmatch(report, -1)
	if len(ownershipMatches) != len(ownershipLineRe.FindAllString(report, -1)) {
		return fmt.Errorf("cross report carries a malformed Finding ownership line — expected `Finding ownership: {finding_id} = owned_by {unit} — evidence: {read_ref}; reason: {one line}`")
	}
	ownershipSeen := map[string]bool{}
	for _, m := range ownershipMatches {
		id := strings.TrimSpace(m[1])
		if ownershipSeen[id] {
			return fmt.Errorf("cross report declares ownership for finding %q more than once", id)
		}
		ownershipSeen[id] = true
		out.Ownerships = append(out.Ownerships, gaterun.FindingOwnership{
			FindingID:    id,
			OwnerUnit:    strings.TrimSpace(m[2]),
			EvidencePath: strings.TrimSpace(m[3]),
			Reason:       strings.TrimSpace(m[4]),
		})
	}
	return nil
}

// extractGateFindings parses the review packet's `gate_findings:` line. The
// documented forms are `none` or one or more `[P0|P1] {finding}` entries; any
// other content is rejected so the conclusion mapping can be checked
// mechanically (see framework/spec_review_checklist.md §Output Format).
func extractGateFindings(report string) (string, error) {
	matches := gateFindingsRe.FindAllStringSubmatch(report, -1)
	if len(matches) != 1 {
		return "", fmt.Errorf("review file report must declare exactly one gate_findings field")
	}
	content, _, err := parseGateFindings(matches[0][1])
	return content, err
}

// parseGateFindings validates the complete `gate_findings` grammar — `none` or
// one or more `[P0|P1] {finding}` entries separated by `;` — and reports
// whether at least one P0/P1 gate finding is declared. Whole-line validation
// (not a substring probe) keeps a line such as `none [P1] x` or
// `[P1] x; [P3] y` from satisfying the conclusion mapping.
func parseGateFindings(content string) (string, bool, error) {
	content = strings.TrimSpace(content)
	if strings.EqualFold(content, "none") {
		return "none", false, nil
	}
	for _, segment := range strings.Split(content, ";") {
		segment = strings.TrimSpace(segment)
		if segment == "" || !gateFindingsEntryRe.MatchString(segment) {
			return "", false, fmt.Errorf(
				"gate_findings must be `none` or one or more `[P0|P1] {finding}` entries separated by `;`, got %q",
				content)
		}
	}
	return content, true, nil
}

// gateFindingsDeclareBlocking reports whether the gate_findings content names
// at least one P0/P1 gate finding.
func gateFindingsDeclareBlocking(content string) bool {
	_, blocking, err := parseGateFindings(content)
	return err == nil && blocking
}

// extractVerdict finds the single verdict line of one check key and returns
// the token, the line text, and its index.
func extractVerdict(spec *gaterun.PacketSpec, key, report string) (string, string, int, error) {
	var pat string
	switch spec.Kind {
	case gaterun.PacketKindChecks:
		pat = `(?m)^[ \t]*-?[ \t]*` + regexp.QuoteMeta(key) + `\.[^\n]*?:\s*(PASS|WARNING|FAIL)\b`
	case gaterun.PacketKindItem:
		pat = `(?m)^[ \t]*-?[ \t]*` + regexp.QuoteMeta(key) + `:\s*(ALIGNED|MISMATCH|CANNOT_DETERMINE)\b(?:\s*\(([^)\n]*)\))?`
	case gaterun.PacketKindFile:
		pat = `(?m)^[ \t]*-?[ \t]*conclusion:\s*((?i:acceptable|needs_attention|unacceptable))\b`
	case gaterun.PacketKindCross:
		pat = `(?m)^[ \t]*-?[ \t]*Cross-check:\s*(?:\d+\s*/\s*\d+\s+)?(PASS|FAIL)\b`
	default:
		return "", "", 0, fmt.Errorf("unknown packet kind %q", spec.Kind)
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return "", "", 0, err
	}
	locs := re.FindAllStringSubmatchIndex(report, -1)
	if len(locs) == 0 {
		return "", "", 0, fmt.Errorf("check %q has no verdict line", key)
	}
	if len(locs) > 1 {
		return "", "", 0, fmt.Errorf("check %q has %d verdict lines — exactly one is required", key, len(locs))
	}
	match := re.FindStringSubmatch(report[locs[0][0]:locs[0][1]])
	token := match[1]
	if spec.Kind == gaterun.PacketKindItem && token == "MISMATCH" {
		// The detection verdict is `MISMATCH (type)` with a fixed type
		// vocabulary; a missing or unknown type is rejected so the analysis
		// packet always receives a classified mismatch.
		verdictType := ""
		if len(match) > 2 {
			verdictType = strings.TrimSpace(match[2])
		}
		if !verifyMismatchTypes[verdictType] {
			return "", "", 0, fmt.Errorf("check %q MISMATCH verdict requires a type suffix from `structural | acceptance | scope | stub | surplus`, got %q", key, verdictType)
		}
	}
	if spec.Kind == gaterun.PacketKindFile {
		token = strings.ToLower(token)
	}
	lineIdx := strings.Count(report[:locs[0][0]], "\n")
	line := report[lineStart(report, locs[0][0]):lineEnd(report, locs[0][0])]
	return token, line, lineIdx, nil
}

func lineStart(s string, pos int) int {
	if i := strings.LastIndexByte(s[:pos], '\n'); i >= 0 {
		return i + 1
	}
	return 0
}

func lineEnd(s string, pos int) int {
	if i := strings.IndexByte(s[pos:], '\n'); i >= 0 {
		return pos + i
	}
	return len(s)
}

// needsReason reports whether a verdict token requires a non-empty reason.
func needsReason(spec *gaterun.PacketSpec, token string) bool {
	switch spec.Kind {
	case gaterun.PacketKindChecks, gaterun.PacketKindCross:
		return true
	case gaterun.PacketKindItem:
		return true
	case gaterun.PacketKindFile:
		return token == "unacceptable"
	}
	return false
}

// hasReason reports whether text follows a verdict token: at least one
// alphanumeric character beyond a parenthesized type suffix and punctuation.
func hasReason(line, token string) bool {
	idx := strings.Index(line, token)
	if idx < 0 {
		return false
	}
	rest := strings.TrimSpace(line[idx+len(token):])
	if strings.HasPrefix(rest, "(") {
		if end := strings.Index(rest, ")"); end >= 0 {
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	rest = strings.Trim(rest, " \t—–-:()[]`,;")
	return alnumRe.MatchString(rest)
}

// extractScopes parses every `{check key}: {file}: {declaration}` line for
// the packet's check keys, skipping lines already consumed as verdicts.
func extractScopes(_ *gaterun.Run, spec *gaterun.PacketSpec, report string, verdictLines map[int]bool) ([]parsedScope, error) {
	paths := declarationPaths(spec)
	lines := strings.Split(report, "\n")
	var out []parsedScope
	for i, raw := range lines {
		if verdictLines[i] {
			continue
		}
		t := strings.TrimSpace(raw)
		t = strings.TrimSpace(strings.TrimPrefix(t, "- "))
		if t == "" {
			continue
		}
		matched := false
		for _, key := range spec.CheckKeys {
			prefixes := []string{key + ":"}
			if spec.Kind == gaterun.PacketKindChecks {
				prefixes = append(prefixes, "check-"+key+":")
			}
			for _, p := range prefixes {
				if !strings.HasPrefix(t, p) {
					continue
				}
				rest := strings.TrimSpace(t[len(p):])
				file, decl, ok := splitPathDecl(rest, paths)
				if !ok {
					return nil, fmt.Errorf("check %q dependency scope line %q: its file is not part of this packet's read refs", key, strings.TrimSpace(raw))
				}
				out = append(out, parsedScope{Key: key, Path: file, Declaration: decl})
				matched = true
				break
			}
			if matched {
				break
			}
		}
	}
	return out, nil
}

// declarationPaths lists every path spelling a report may name: physical
// snapshot refs, resolved logical refs, and surface entry files. Sorted by
// length descending so the longest path wins a prefix match.
func declarationPaths(spec *gaterun.PacketSpec) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}
	for _, ref := range spec.ReadRefs {
		add(ref)
	}
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) > len(paths[j])
		}
		return paths[i] < paths[j]
	})
	return paths
}

// splitPathDecl splits `{file}: {declaration}` against the known paths. A
// known snapshot path (longest match) wins; otherwise the file is the part
// before the first ": " — downstream validation reports the precise reason
// (outside the snapshot / wrong path form).
func splitPathDecl(rest string, paths []string) (string, string, bool) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", false
	}
	for _, p := range paths {
		if rest == p {
			return p, "all", true
		}
		if strings.HasPrefix(rest, p+":") {
			return p, declOf(rest[len(p)+1:]), true
		}
	}
	if idx := strings.Index(rest, ": "); idx >= 0 {
		return strings.TrimSpace(rest[:idx]), declOf(rest[idx+1:]), true
	}
	return rest, "all", true
}

func declOf(raw string) string {
	decl := strings.Trim(strings.TrimSpace(raw), "`\"'")
	if decl == "" {
		return "all"
	}
	return decl
}

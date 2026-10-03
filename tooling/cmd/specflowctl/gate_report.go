package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// parsedReport is the mechanically extracted content of one session report:
// verdicts, dependency declarations, findings, and synthesis records.
type parsedReport struct {
	Observations            []gaterun.Finding
	ObservationDispositions []gaterun.FindingDisposition
	Verdicts                map[string]string // check key -> verdict token
	CrossItems              map[string]string // fixed cross item -> PASS | FAIL
	CrossItemFindings       map[string]string // failed cross item -> new cross finding id
	Scopes                  []parsedScope
	Findings                []gaterun.Finding
	EffectiveStatus         map[string]string
	QualityConclusions      map[string]string
	Dispositions            []gaterun.FindingDisposition
	Ownerships              []gaterun.FindingOwnership
	Analysis                map[string]string
	FileGateFindings        map[string]string // file key -> gate_findings content
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
var verifyMismatchTypes = func() map[string]bool {
	out := make(map[string]bool, len(mismatchTypes))
	for _, value := range mismatchTypes {
		out[value] = true
	}
	return out
}()

var (
	rangeDeclRe         = regexp.MustCompile(`^\d+-\d+(,\d+-\d+)*$`)
	findingRe           = regexp.MustCompile(`^[ \t]*-?[ \t]*\[(P0|P1|P2|P3)\][ \t]*(.+)$`)
	resolutionLabelRe   = regexp.MustCompile(`\((?:actionable|needs_decision)\)\s*$`)
	factAnchorRe        = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*fact_anchor:\s*\S`)
	gateFindingsRe      = regexp.MustCompile(`(?mi)^[ \t]*gate_findings:\s*(\S[^\n]*)$`)
	gateFindingsEntryRe = regexp.MustCompile(`^\[(?:P0|P1)\][ \t]+\S`)
	alnumRe             = regexp.MustCompile(`[A-Za-z0-9]`)
	effectiveStatusRe   = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Effective status:\s*(.+?)\s*=\s*(` + strings.Join(crossStatuses, "|") + `)\s*$`)
	dispositionRe       = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding disposition:\s*(\S+)\s*=\s*(` + strings.Join(crossDispositions, "|") + `)(?:\s*->\s*(\S+))?(?:\s+[—-]\s+(.+))?\s*$`)
	findingAffectsRe    = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding affects:\s*(\S+)\s*=\s*(.+?)\s*$`)
	ownershipRe         = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding ownership:\s*(\S+)\s*=\s*owned_by\s+(\S+)\s+[—-]\s+evidence:\s*(.+?)\s*;\s*reason:\s*(.+?)\s*$`)
	ownershipLineRe     = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Finding ownership:`)
	crossItemLineRe     = regexp.MustCompile(`^Cross item:\s*([a-z][a-z0-9_]*)\s*=\s*(PASS|FAIL)\s+[—-]\s+(\S.*)$`)
	crossItemFindingRe  = regexp.MustCompile(`^Cross item finding:\s*([a-z][a-z0-9_]*)\s*=\s*(\S+)\s*$`)
	crossSummaryRe      = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*Cross-check:[ \t]*(\d+)[ \t]*/[ \t]*(\d+)[ \t]+(PASS|FAIL)[ \t]+[—-][ \t]+(\S[^\n]*)$`)
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
// is shared by the session-report validation (gate-submit) and the cache
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

// parseSessionReport validates one session report against its planned session:
// every check key has exactly one verdict line with an allowed token, blocking
// verdicts carry a reason, and every check key declares at least one
// dependency-scope line whose file belongs to the run snapshot. See
// framework/verification_scope.md §Coverage Model → Session report contract.
func parseSessionReport(run *gaterun.Run, spec *gaterun.SessionSpec, report string) (*parsedReport, error) {
	if strings.TrimSpace(report) == "" {
		return nil, fmt.Errorf("empty report")
	}
	report = strings.ReplaceAll(report, "\r\n", "\n")
	if gaterun.IsQualityKind(spec.Kind) {
		return parseQualitySessionReport(run, spec, report)
	}
	out := &parsedReport{Verdicts: map[string]string{}, EffectiveStatus: map[string]string{}, Analysis: map[string]string{}}
	verdictLines := map[int]bool{}
	if gaterun.IsItemKind(spec.Kind) {
		// A verify session judges its acceptance items directly: each item
		// carries an alignment verdict and, for a mismatch, the finding with
		// its root cause, severity, and repair direction (there is no separate
		// analysis session in the coverage model).
		lines, err := parseVerifyItemReport(run, spec, report, out)
		if err != nil {
			return nil, err
		}
		verdictLines = lines
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
		if err := validateSessionBodyStructure(run, spec, report, out); err != nil {
			return nil, err
		}
	}
	scopes, err := extractScopes(run, spec, report, verdictLines)
	if err != nil {
		return nil, err
	}
	for _, key := range spec.RequiredScopeKeys() {
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
	if !gaterun.IsItemKind(spec.Kind) {
		// Finding ids are run-scoped — {run_id}/{session_id}/F{n} — so they
		// are unique by construction across runs: a finding authored by this
		// report can never collide with a carried finding from an earlier
		// run (see framework/verification_scope.md §Coverage Model →
		// Session report contract).
		for i, extracted := range extractFindings(report) {
			// The unified finding format carries a resolution label on the
			// entry line (framework/_atoms/misc/report_skeleton.md); cross
			// reports author new findings in the cross synthesis format,
			// which has no label.
			if spec.Kind != gaterun.SessionKindCross && !resolutionLabelRe.MatchString(extracted.text) {
				return nil, fmt.Errorf("finding [%s] %s carries no resolution label (`(actionable)` or `(needs_decision)`)", extracted.severity, extracted.text)
			}
			sourceKey := ""
			if spec.Kind == gaterun.SessionKindCross {
				sourceKey = gaterun.CrossKey
			}
			if len(spec.CheckKeys) == 1 {
				sourceKey = spec.CheckKeys[0]
			}
			out.Findings = append(out.Findings, gaterun.Finding{
				ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.SessionID, i+1),
				Severity:  extracted.severity,
				Text:      extracted.text,
				Detail:    extracted.detail,
				SourceKey: sourceKey,
			})
		}
	}
	if spec.Kind == gaterun.SessionKindChecks {
		if err := parseFindingAffects(report, out); err != nil {
			return nil, err
		}
		for i := range out.Findings {
			finding := &out.Findings[i]
			keys := findingKeySet(*finding)
			if len(keys) == 0 {
				return nil, fmt.Errorf("finding %q must declare Finding affects with its check keys", finding.ID)
			}
			for key := range keys {
				if !stringInList(spec.CheckKeys, key) {
					return nil, fmt.Errorf("finding %q affects unassigned check %q", finding.ID, key)
				}
			}
			if finding.SourceKey == "" {
				finding.SourceKey = sortedFindingKeys(keys, "")[0]
			}
			finding.AffectedKeys = sortedFindingKeys(keys, finding.SourceKey)
		}
	}
	if spec.Kind == gaterun.SessionKindCross {
		if err := parseCrossSynthesis(report, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func validateSessionBodyStructure(run *gaterun.Run, spec *gaterun.SessionSpec, report string, out *parsedReport) error {
	switch {
	case isUnitValidateCheck5(run, spec):
		return validateUnitAcceptanceBody(report)
	case spec.Kind == gaterun.SessionKindCross:
		return validateCrossItemBody(run, report, out)
	default:
		return nil
	}
}

func validateCrossItemBody(run *gaterun.Run, report string, out *parsedReport) error {
	gate := run.Gate
	items := crossItemsFor(run)
	seen := make(map[string]bool, len(items))
	links := make(map[string]string, len(items))
	out.CrossItems = make(map[string]string, len(items))
	if out.Analysis == nil {
		out.Analysis = map[string]string{}
	}
	passed := 0
	summaryLines := 0
	for _, line := range strings.Split(report, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Cross-check:") || strings.HasPrefix(line, "- Cross-check:") {
			summaryLines++
		}
		if strings.HasPrefix(line, "Cross item finding:") {
			match := crossItemFindingRe.FindStringSubmatch(line)
			if match == nil {
				return fmt.Errorf("malformed Cross item finding line %q: expected `Cross item finding: <key> = <finding_id>`", line)
			}
			if !stringInList(items, match[1]) {
				return fmt.Errorf("unknown Cross item finding key %q for %s", match[1], gate)
			}
			if _, exists := links[match[1]]; exists {
				return fmt.Errorf("duplicate Cross item finding for %q", match[1])
			}
			links[match[1]] = match[2]
			continue
		}
		if !strings.HasPrefix(line, "Cross item:") {
			continue
		}
		match := crossItemLineRe.FindStringSubmatch(line)
		if match == nil {
			return fmt.Errorf("malformed Cross item line %q: expected `Cross item: <key> = PASS|FAIL — <reason>`", line)
		}
		key := match[1]
		if !stringInList(items, key) {
			return fmt.Errorf("unknown Cross item %q for %s", key, gate)
		}
		if seen[key] {
			return fmt.Errorf("duplicate Cross item %q", key)
		}
		seen[key] = true
		out.CrossItems[key] = match[2]
		out.Analysis[gaterun.RelationshipKey(key)] = match[2]
		if match[2] == "PASS" {
			passed++
		}
	}
	for _, item := range items {
		if !seen[item] {
			return fmt.Errorf("missing Cross item %q", item)
		}
		if out.CrossItems[item] == "FAIL" && links[item] == "" {
			return fmt.Errorf("failed Cross item %q has no Cross item finding", item)
		}
		if out.CrossItems[item] == "PASS" && links[item] != "" {
			return fmt.Errorf("passing Cross item %q must not declare a Cross item finding", item)
		}
	}
	if len(items) == 0 {
		if summaryLines != 1 || crossSummaryRe.MatchString(report) {
			return fmt.Errorf("finding-only synthesis requires exactly one `Cross-check: PASS|FAIL — reason` summary without relationship counts")
		}
		return nil
	}
	summaries := crossSummaryRe.FindAllStringSubmatch(report, -1)
	if summaryLines != 1 || len(summaries) != 1 {
		return fmt.Errorf("%s cross report requires exactly one `Cross-check: N/%d PASS|FAIL — <reason>` summary", gate, len(items))
	}
	gotPassed, err := strconv.Atoi(summaries[0][1])
	if err != nil {
		return fmt.Errorf("invalid Cross-check passed count %q: %w", summaries[0][1], err)
	}
	gotTotal, err := strconv.Atoi(summaries[0][2])
	if err != nil {
		return fmt.Errorf("invalid Cross-check total count %q: %w", summaries[0][2], err)
	}
	if gotPassed != passed || gotTotal != len(items) {
		return fmt.Errorf("Cross-check count %d/%d contradicts Cross item results: expected %d/%d", gotPassed, gotTotal, passed, len(items))
	}
	out.CrossItemFindings = links
	return nil
}

func validateUnitAcceptanceBody(report string) error {
	for _, subcheck := range unitAcceptanceSubchecks {
		allowed := strings.Join(unitAcceptanceAllowed(subcheck), "|")
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

// itemContinuationBlock returns the verdict line at idx plus its contiguous
// indented continuation lines — the per-item block a verify session report
// must carry. The block ends at the first non-empty, unindented line (the next
// item's verdict or the Dependency scope section).
func itemContinuationBlock(lines []string, idx int) []string {
	block := []string{lines[idx]}
	for i := idx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			break
		}
		block = append(block, line)
	}
	return block
}

// validateVerifyItemBlock validates one acceptance item's field block: one
// non-empty evidence, deterministic, Part A, and Part B line.
func validateVerifyItemBlock(block []string) error {
	text := strings.Join(block, "\n")
	for _, field := range verifyItemFields {
		pat := `(?mi)^[ \t]*-?[ \t]*` + regexp.QuoteMeta(field) + `:[ \t]*(\S[^\n]*)$`
		if field == "deterministic" {
			pat = `(?mi)^[ \t]*-?[ \t]*deterministic:[ \t]*(true|false)[ \t]*$`
		}
		if field == "Part A" || field == "Part B" {
			pat = `(?mi)^[ \t]*` + regexp.QuoteMeta(field) + `:[ \t]*(\S[^\n]*)$`
		}
		if count := len(regexp.MustCompile(pat).FindAllStringSubmatch(text, -1)); count != 1 {
			return fmt.Errorf("verify item block must declare exactly one non-empty %s field", field)
		}
	}
	// When Part B does not apply, the report must use the explicit
	// `skipped — {reason}` form; a bare `skipped` carries no reason.
	if partB := regexp.MustCompile(`(?mi)^[ \t]*Part B:[ \t]*(\S[^\n]*)$`).FindStringSubmatch(text); partB != nil {
		content := strings.TrimSpace(partB[1])
		if strings.HasPrefix(strings.ToLower(content), "skipped") {
			reason := strings.TrimSpace(content[len("skipped"):])
			reason = strings.Trim(reason, " \t—–-:;,.")
			if reason == "" {
				return fmt.Errorf("verify item Part B `skipped` must carry a reason (`skipped — {reason}`)")
			}
		}
	}
	return nil
}

func validateQualityFileBody(report string) error {
	for _, field := range qualityDimensions {
		re := regexp.MustCompile(`(?mi)^[ \t]*` + regexp.QuoteMeta(field) + `:\s*(\S[^\n]*?)\s+[—-]\s+(\S[^\n]*)$`)
		if count := len(re.FindAllStringSubmatch(report, -1)); count != 1 {
			return fmt.Errorf("quality file report must declare exactly one %s assessment with a non-empty basis", field)
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
			return fmt.Errorf("quality file report must declare exactly one %s field", field.name)
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

// parseVerifyItemReport parses a verify session report: one alignment verdict
// per assigned acceptance item, each with its evidence block, and — when the
// verdict is MISMATCH — the inline finding fields (problem, evidence, impact,
// root cause, suggested direction, severity, confidence). It returns the
// verdict line indexes so the generic scope parser skips them.
func parseVerifyItemReport(run *gaterun.Run, spec *gaterun.SessionSpec, report string, out *parsedReport) (map[int]bool, error) {
	verdictLines := map[int]bool{}
	lines := strings.Split(report, "\n")
	for _, item := range spec.CheckKeys {
		token, line, lineIdx, err := extractVerdict(spec, item, report)
		if err != nil {
			return nil, err
		}
		if needsReason(spec, token) && !hasReason(line, token) {
			return nil, fmt.Errorf("check %q verdict %s carries no reason", item, token)
		}
		out.Verdicts[item] = token
		verdictLines[lineIdx] = true
		block := itemContinuationBlock(lines, lineIdx)
		if err := validateVerifyItemBlock(block); err != nil {
			return nil, fmt.Errorf("check %q: %w", item, err)
		}
		if token == "CANNOT_DETERMINE" && spec.Kind == gaterun.SessionKindPreserve {
			out.Findings = append(out.Findings, gaterun.Finding{ID: fmt.Sprintf("%s/%s/F%d", run.RunID, spec.SessionID, len(out.Findings)+1), Severity: "P1", Text: item + " cannot establish preservation", Detail: "Problem: " + line + "\nSuggested direction: blocked", SourceKey: item})
		}
		if token == "MISMATCH" {
			finding, err := parseItemMismatch(run, spec, item, block, len(out.Findings)+1)
			if err != nil {
				return nil, err
			}
			out.Findings = append(out.Findings, *finding)
		}
	}
	return verdictLines, nil
}

// parseItemMismatch parses and validates the inline finding fields of one
// MISMATCH item block and composes the finding.
func parseItemMismatch(run *gaterun.Run, spec *gaterun.SessionSpec, item string, block []string, index int) (*gaterun.Finding, error) {
	text := strings.Join(block, "\n")
	analysis := map[string]string{}
	for _, field := range analysisFields {
		re := regexp.MustCompile(`(?mi)^[ \t]*` + regexp.QuoteMeta(field) + `:\s*([^\n]+)$`)
		matches := re.FindAllStringSubmatch(text, -1)
		if len(matches) != 1 || strings.TrimSpace(matches[0][1]) == "" {
			return nil, fmt.Errorf("check %q MISMATCH must declare exactly one non-empty %s field", item, field)
		}
		analysis[field] = strings.TrimSpace(matches[0][1])
	}
	evidence, err := analysisEvidenceBlock(text)
	if err != nil {
		return nil, fmt.Errorf("check %q: %w", item, err)
	}
	severity := strings.ToUpper(analysis["Severity"])
	if !oneOf(severity, analysisAllowed("Severity")...) {
		return nil, fmt.Errorf("check %q has invalid Severity %q", item, analysis["Severity"])
	}
	if !oneOf(analysis["Root cause"], analysisAllowed("Root cause")...) {
		return nil, fmt.Errorf("check %q has invalid Root cause %q", item, analysis["Root cause"])
	}
	direction := analysis["Suggested direction"]
	if !oneOf(direction, analysisAllowed("Suggested direction")...) {
		return nil, fmt.Errorf("check %q has invalid Suggested direction %q", item, direction)
	}
	if !oneOf(strings.ToLower(analysis["Confidence"]), analysisAllowed("Confidence")...) {
		return nil, fmt.Errorf("check %q has invalid Confidence %q", item, analysis["Confidence"])
	}
	resolution := "actionable"
	if direction == "needs_design" || direction == "blocked" {
		resolution = "needs_decision"
	}
	fixBlock, err := analysisFixBlock(text, resolution)
	if err != nil {
		return nil, fmt.Errorf("check %q: %w", item, err)
	}
	var evidenceRendered strings.Builder
	for _, line := range evidence {
		evidenceRendered.WriteString("\n    " + line)
	}
	detail := fmt.Sprintf("[%s] %s — verify mismatch analysis (%s)\n  problem: %s\n  evidence:%s\n  impact: %s\n%s\n  root_cause: %s\n  direction: %s (confidence: %s)",
		severity, item, resolution, analysis["Problem"], evidenceRendered.String(), analysis["Impact"], fixBlock, analysis["Root cause"], direction, strings.ToLower(analysis["Confidence"]))
	return &gaterun.Finding{
		ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.SessionID, index),
		Severity:  severity,
		Text:      "verify mismatch analysis for " + item,
		Detail:    detail,
		SourceKey: item,
	}, nil
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
// framework/verification_scope.md §Coverage Model → Session report contract).
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
	out.QualityConclusions = map[string]string{}
	for _, line := range strings.Split(report, "\n") {
		rest, declared := strings.CutPrefix(strings.TrimSpace(line), "Quality conclusion:")
		if !declared {
			continue
		}
		key, value, ok := strings.Cut(rest, "=")
		key = strings.TrimSpace(key)
		verdict, reason, hasReason := strings.Cut(value, "—")
		verdict = strings.TrimSpace(verdict)
		if !ok || key == "" || !oneOf(verdict, "acceptable", "needs_attention", "unacceptable") || !hasReason || strings.TrimSpace(reason) == "" {
			return fmt.Errorf("invalid Quality conclusion: expected <key> = <acceptable|needs_attention|unacceptable> — <reason>")
		}
		if _, exists := out.QualityConclusions[key]; exists {
			return fmt.Errorf("cross report declares Quality conclusion for %q more than once", key)
		}
		out.QualityConclusions[key] = verdict
	}
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
	if err := parseFindingAffects(report, out); err != nil {
		return err
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

func parseFindingAffects(report string, out *parsedReport) error {
	findingByID := map[string]*gaterun.Finding{}
	for i := range out.Findings {
		findingByID[out.Findings[i].ID] = &out.Findings[i]
	}
	affectedSeen := map[string]bool{}
	for _, m := range findingAffectsRe.FindAllStringSubmatch(report, -1) {
		id := strings.TrimSpace(m[1])
		finding := findingByID[id]
		if finding == nil {
			return fmt.Errorf("report declares affected keys for unknown new finding %q", id)
		}
		if affectedSeen[id] {
			return fmt.Errorf("report declares affected keys for finding %q more than once", id)
		}
		affectedSeen[id] = true
		seenKeys := map[string]bool{}
		for _, raw := range strings.Split(m[2], ",") {
			key := strings.TrimSpace(raw)
			if key == "" {
				return fmt.Errorf("finding %q carries an empty affected key", id)
			}
			if !seenKeys[key] {
				seenKeys[key] = true
				finding.AffectedKeys = append(finding.AffectedKeys, key)
			}
		}
	}
	return nil
}

// extractGateFindings parses the quality file session's `gate_findings:` line.
// The documented forms are `none` or one or more `[P0|P1] {finding}` entries;
// any other content is rejected so the conclusion mapping can be checked
// mechanically (see framework/unit_verify_checklist.md §Output Format).
func extractGateFindings(report string) (string, error) {
	matches := gateFindingsRe.FindAllStringSubmatch(report, -1)
	if len(matches) != 1 {
		return "", fmt.Errorf("quality file report must declare exactly one gate_findings field")
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
func extractVerdict(spec *gaterun.SessionSpec, key, report string) (string, string, int, error) {
	var pat string
	allowed := strings.Join(verdictContractFor(spec.Kind, key).Allowed, "|")
	switch spec.Kind {
	case gaterun.SessionKindChecks:
		pat = `(?m)^[ \t]*-?[ \t]*` + regexp.QuoteMeta(key) + `\.[^\n]*?:\s*(` + allowed + `)\b`
	case gaterun.SessionKindItem, gaterun.SessionKindPreserve:
		pat = `(?m)^[ \t]*-?[ \t]*` + regexp.QuoteMeta(key) + `:\s*(` + allowed + `)\b(?:\s*\(([^)\n]*)\))?`
	case gaterun.SessionKindDesign, gaterun.SessionKindCode, gaterun.SessionKindArchitecture:
		pat = `(?m)^[ \t]*-?[ \t]*conclusion:\s*((?i:` + allowed + `))\b`
	case gaterun.SessionKindCross:
		pat = `(?m)^[ \t]*-?[ \t]*Cross-check:\s*(?:\d+\s*/\s*\d+\s+)?(` + allowed + `)\b`
	default:
		return "", "", 0, fmt.Errorf("unknown session kind %q", spec.Kind)
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
	if gaterun.IsItemKind(spec.Kind) && token == "MISMATCH" {
		// The detection verdict is `MISMATCH (type)` with a fixed type
		// vocabulary; a missing or unknown type is rejected so the analysis
		// session always receives a classified mismatch.
		verdictType := ""
		if len(match) > 2 {
			verdictType = strings.TrimSpace(match[2])
		}
		if !verifyMismatchTypes[verdictType] {
			return "", "", 0, fmt.Errorf("check %q MISMATCH verdict requires a type suffix from `structural | acceptance | scope | stub | surplus`, got %q", key, verdictType)
		}
	}
	if gaterun.IsQualityKind(spec.Kind) {
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
func needsReason(spec *gaterun.SessionSpec, token string) bool {
	return stringInList(verdictContractFor(spec.Kind, "").ReasonRequiredFor, token)
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
// the session's check keys, skipping lines already consumed as verdicts.
func extractScopes(_ *gaterun.Run, spec *gaterun.SessionSpec, report string, verdictLines map[int]bool) ([]parsedScope, error) {
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
		for _, key := range spec.ScopeKeys() {
			prefixes := []string{key + ":"}
			if spec.Kind == gaterun.SessionKindChecks {
				prefixes = append(prefixes, "check-"+key+":")
			}
			for _, p := range prefixes {
				if !strings.HasPrefix(t, p) {
					continue
				}
				rest := strings.TrimSpace(t[len(p):])
				file, decl, ok := splitPathDecl(rest, paths)
				if !ok {
					return nil, fmt.Errorf("check %q dependency scope line %q: its file is not part of this session's read refs", key, strings.TrimSpace(raw))
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
func declarationPaths(spec *gaterun.SessionSpec) []string {
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

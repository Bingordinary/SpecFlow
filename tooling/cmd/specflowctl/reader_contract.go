package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// The reader contract probe (unit validate Check 10) tests the framework's
// Reader Contract under acquisition conditions: an independent reader session
// answers a fixed 17-question bank using only the unit main spec's
// human-readable part — every ## section before the section holding the
// acceptance_item_set marker — citing a verbatim quote and its section for
// each answer. gate-submit validates the evidence mechanically (quotes exist
// verbatim at the declared section, inside the human-readable part, one
// contiguous span per answer, bounded length); the verifier packet then
// judges each (question, answer, quote) triple's sufficiency without
// re-reading the spec for answers. Protocol source:
// framework/unit_validate_checklist.md Check 10.

// readerContractQuestions is the fixed question bank: the ten §9 expression
// points (Q01-Q10) and the seven must-close decisions (Q11-Q17). The bank is
// the same for every unit and every round; its authoritative definition is
// the checklist's Check 10 question table, and these IDs and titles must
// match it exactly.
var readerContractQuestions = []struct {
	ID    string
	Title string
}{
	{"Q01", "intended user, actor, or caller"},
	{"Q02", "unit responsibility and why the unit owns it"},
	{"Q03", "entry point or trigger"},
	{"Q04", "normal path from input to result"},
	{"Q05", "boundaries crossed on the path"},
	{"Q06", "data, state, or durable truth each step reads or writes"},
	{"Q07", "owner of each read/write responsibility"},
	{"Q08", "output artifact or observable result"},
	{"Q09", "failure and dependency-unavailability exposure"},
	{"Q10", "verification surface and success condition"},
	{"Q11", "which object owns a responsibility"},
	{"Q12", "which entry point starts the behavior"},
	{"Q13", "where state or durable truth lives"},
	{"Q14", "how ordered steps connect"},
	{"Q15", "how boundary failures are reported"},
	{"Q16", "what the result shape means"},
	{"Q17", "how acceptance proves the stated responsibility"},
}

// Reader contract quote bounds (runes). The lower bound only rejects
// degenerate noise; whether a quote really answers its question is the
// verifier packet's judgment, not a length rule.
const (
	readerQuoteMinRunes = 4
	readerQuoteMaxRunes = 400
)

var (
	readerQuestionRe = regexp.MustCompile(`^Question:\s*(Q\d{2})\s*[—-]\s*(.+?)\s*$`)
	readerStatusRe   = regexp.MustCompile(`^Status:\s*(answered|no_local_answer)\s*$`)
	readerAnswerRe   = regexp.MustCompile(`^Answer:\s*(\S[^\n]*)$`)
	readerQuoteRe    = regexp.MustCompile(`^Quote:\s*(\S[^\n]*)$`)
	readerLocationRe = regexp.MustCompile(`^Location:\s*(\S[^\n]*)$`)
	verifierJudgRe   = regexp.MustCompile(`(?m)^[ \t]*Judgment:\s*(Q\d{2})\s*=\s*(supported|partial|unsupported)\s*[—-]\s*(\S[^\n]*)$`)
)

// readerEvidence is one parsed question block of a reader evidence report.
type readerEvidence struct {
	ID       string
	Status   string // answered | no_local_answer
	Answer   string
	Quote    string
	Location string
}

// parseReaderEvidenceReport parses the 17 question blocks of a reader
// evidence report. It enforces the bank order and the per-status field
// rules; the mechanical citation validation against the spec runs in
// validateReaderEvidence. The computed check verdict ("10") is set here:
// the reader authors evidence, never a verdict.
func parseReaderEvidenceReport(spec *gaterun.PacketSpec, report string, out *parsedReport) error {
	lines := strings.Split(report, "\n")
	var blocks []readerEvidence
	var current *readerEvidence
	inScope := false
	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if inScope {
			continue
		}
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "Dependency scope:") {
			inScope = true
			continue
		}
		if m := readerQuestionRe.FindStringSubmatch(trimmed); m != nil {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &readerEvidence{ID: m[1]}
			// The title is part of the bank identity: a mismatched title
			// means the executor paraphrased the bank.
			want := readerQuestionByID(m[1])
			if want == "" || m[2] != want {
				return fmt.Errorf("question block %q does not match the fixed bank (expected `Question: %s — %s`)", m[1], m[1], want)
			}
			continue
		}
		if current == nil {
			return fmt.Errorf("report carries content before the first Question block: %q", trimmed)
		}
		switch {
		case readerStatusRe.MatchString(trimmed):
			if current.Status != "" {
				return fmt.Errorf("question %q declares Status twice", current.ID)
			}
			current.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "Status:"))
		case strings.HasPrefix(trimmed, "Answer:"):
			if current.Answer != "" {
				return fmt.Errorf("question %q declares Answer twice", current.ID)
			}
			current.Answer = strings.TrimSpace(strings.TrimPrefix(trimmed, "Answer:"))
		case strings.HasPrefix(trimmed, "Quote:"):
			if current.Quote != "" {
				return fmt.Errorf("question %q declares Quote twice", current.ID)
			}
			current.Quote = strings.TrimSpace(strings.TrimPrefix(trimmed, "Quote:"))
		case strings.HasPrefix(trimmed, "Location:"):
			if current.Location != "" {
				return fmt.Errorf("question %q declares Location twice", current.ID)
			}
			current.Location = strings.TrimSpace(strings.TrimPrefix(trimmed, "Location:"))
		default:
			return fmt.Errorf("question %q carries an unrecognized line %q — only Status, Answer, Quote, and Location are allowed", current.ID, trimmed)
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}
	if inScope && len(blocks) == 0 {
		return fmt.Errorf("reader evidence report carries no question blocks")
	}
	if len(blocks) != len(readerContractQuestions) {
		return fmt.Errorf("reader evidence report must carry exactly %d question blocks in bank order, got %d", len(readerContractQuestions), len(blocks))
	}
	failures := 0
	for i, block := range blocks {
		want := readerContractQuestions[i]
		if block.ID != want.ID {
			return fmt.Errorf("question block %d is %q, expected %q — blocks must follow the fixed bank order", i+1, block.ID, want.ID)
		}
		switch block.Status {
		case "answered":
			if block.Answer == "" || block.Quote == "" || block.Location == "" {
				return fmt.Errorf("question %q is answered but carries an empty Answer, Quote, or Location", block.ID)
			}
		case "no_local_answer":
			if block.Answer != "" || block.Quote != "" || block.Location != "" {
				return fmt.Errorf("question %q declares no_local_answer but also carries Answer, Quote, or Location — report the absence, do not approximate", block.ID)
			}
			failures++
		default:
			return fmt.Errorf("question %q carries no Status line", block.ID)
		}
	}
	if failures > 0 {
		out.Verdicts[gaterun.ReaderContractCheck] = "FAIL"
	} else {
		out.Verdicts[gaterun.ReaderContractCheck] = "PASS"
	}
	return nil
}

func readerQuestionByID(id string) string {
	for _, q := range readerContractQuestions {
		if q.ID == id {
			return q.Title
		}
	}
	return ""
}

// narrativeSectionsFor resolves the target's unit main spec, its content, and
// its human-readable section list (the Check 10 citation source). Both Check
// 10 packets share this computation so their plan- and submit-time boundaries
// can never drift.
func narrativeSectionsFor(absRoot string, run *gaterun.Run) (string, string, []string, error) {
	main, err := unitMainSpecPath(run)
	if err != nil {
		return "", "", nil, err
	}
	contentBytes, err := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(main)))
	if err != nil {
		return "", "", nil, fmt.Errorf("read the unit main spec %s: %w", main, err)
	}
	content := string(contentBytes)
	headings, found := gaterun.NarrativeSectionHeadings(content)
	if !found {
		return "", "", nil, fmt.Errorf("the human-readable part of %s cannot be located: the acceptance_item_set marker is missing, or it sits outside every ## section", main)
	}
	if len(headings) == 0 {
		return "", "", nil, fmt.Errorf("the human-readable part of %s is empty: every ## section sits at or after the acceptance section", main)
	}
	return main, content, headings, nil
}

// declaredNarrativeScopes collects a Check 10 packet's declared scope
// sections from the parsed report: every check-10 scope line must name the
// unit main spec. The returned declarations feed the exact-set comparison
// against the human-readable section list.
func declaredNarrativeScopes(scopes []parsedScope, main string) ([]string, error) {
	var declared []string
	for _, s := range scopes {
		if s.Key != gaterun.ReaderContractCheck {
			continue
		}
		if s.Path != main {
			return nil, fmt.Errorf("check %q scope line declares %s — citations are read from the unit main spec %s only", gaterun.ReaderContractCheck, s.Path, main)
		}
		declared = append(declared, s.Declaration)
	}
	return declared, nil
}

// validateNarrativeScopeSet enforces that a Check 10 packet's declared scopes
// are exactly the human-readable section list — count, membership, and
// uniqueness (framework/verification_scope.md §Gate Work Packets → Check 10
// packet contracts).
func validateNarrativeScopeSet(declared []string, main string, headings []string) error {
	if len(declared) != len(headings) {
		return fmt.Errorf("check %q must declare exactly the human-readable sections (%d declared, %d required) — one Dependency scope line per section", gaterun.ReaderContractCheck, len(declared), len(headings))
	}
	narrative := map[string]bool{}
	for _, h := range headings {
		narrative[h] = true
	}
	seen := map[string]bool{}
	for _, decl := range declared {
		if !narrative[decl] {
			return fmt.Errorf("check %q declares %q, which is not a human-readable section of %s", gaterun.ReaderContractCheck, decl, main)
		}
		if seen[decl] {
			return fmt.Errorf("check %q declares %q more than once", gaterun.ReaderContractCheck, decl)
		}
		seen[decl] = true
	}
	return nil
}

// validateReaderEvidence performs the mechanical citation validation of an
// accepted-form reader report against the target's unit main spec: every
// quote must exist verbatim inside its declared section, the section must
// belong to the human-readable part, the quote must be a single bounded
// span, and the declared Dependency scope sections must equal the
// human-readable part. It appends one mechanical finding per
// no_local_answer question. The question blocks are re-parsed here because
// the citation facts are not part of parsedReport.
func validateReaderEvidence(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, report string, parsed *parsedReport) error {
	main, content, headings, err := narrativeSectionsFor(absRoot, run)
	if err != nil {
		return err
	}
	narrative := map[string]bool{}
	for _, h := range headings {
		narrative[h] = true
	}
	declared, err := declaredNarrativeScopes(parsed.Scopes, main)
	if err != nil {
		return err
	}
	if err := validateNarrativeScopeSet(declared, main, headings); err != nil {
		return err
	}

	var blocks []readerEvidence
	inScope := false
	var current *readerEvidence
	for _, raw := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "Dependency scope:") {
			inScope = true
			current = nil
			continue
		}
		if inScope {
			continue
		}
		if m := readerQuestionRe.FindStringSubmatch(trimmed); m != nil {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &readerEvidence{ID: m[1], Status: "answered"}
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "Status:"):
			current.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, "Status:"))
		case strings.HasPrefix(trimmed, "Quote:"):
			current.Quote = strings.TrimSpace(strings.TrimPrefix(trimmed, "Quote:"))
		case strings.HasPrefix(trimmed, "Location:"):
			current.Location = strings.TrimSpace(strings.TrimPrefix(trimmed, "Location:"))
		}
	}
	if current != nil {
		blocks = append(blocks, *current)
	}

	regionText := map[string]string{}
	for _, region := range contenthash.SectionRegions(content) {
		regionText[region.Heading] = region.Text
	}
	failureIdx := 0
	for _, block := range blocks {
		if block.Status != "answered" {
			parsed.Findings = append(parsed.Findings, readerNoAnswerFinding(run, spec, block.ID, failureIdx))
			failureIdx++
			continue
		}
		if !narrative[block.Location] {
			return fmt.Errorf("question %s cites %q, which is not a human-readable section — the answer must live before the acceptance section", block.ID, block.Location)
		}
		if n := utf8.RuneCountInString(block.Quote); n < readerQuoteMinRunes || n > readerQuoteMaxRunes {
			return fmt.Errorf("question %s quote is %d runes; the bound is %d-%d", block.ID, n, readerQuoteMinRunes, readerQuoteMaxRunes)
		}
		if !strings.Contains(regionText[block.Location], block.Quote) {
			return fmt.Errorf("question %s quote does not exist verbatim inside section %q of %s — cite the exact text", block.ID, block.Location, main)
		}
	}
	return nil
}

// readerNoAnswerFinding composes the mechanical finding for one
// no_local_answer question: the reader reported the absence, so the §9
// point is not locally expressible in the human-readable part.
func readerNoAnswerFinding(run *gaterun.Run, spec *gaterun.PacketSpec, questionID string, index int) gaterun.Finding {
	title := readerQuestionByID(questionID)
	detail := fmt.Sprintf("[P1] %s (%s) — reader contract: no local answer in the human-readable part (actionable)\n  problem: the independent reader could not find the answer to %s (%s) anywhere in the human-readable part of the unit main spec\n  evidence:\n    - Status: no_local_answer — reader evidence report for %s\n  impact: a human reader cannot reconstruct this §9 point from the human-readable part alone, so the design must be reconstructed from code or from the author\n  fix: express the %s point in the narrative before the acceptance section, quotable in one contiguous span", questionID, title, questionID, title, questionID, title)
	return gaterun.Finding{
		ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.PacketID, index+1),
		Severity:  "P1",
		Text:      fmt.Sprintf("reader contract: %s (%s) has no local answer", questionID, title),
		Detail:    detail,
		SourceKey: gaterun.ReaderContractCheck,
	}
}

// verifierUnsupportedFinding composes the mechanical finding for one
// partial or unsupported triple: the quote exists but does not answer the
// question.
func verifierUnsupportedFinding(run *gaterun.Run, spec *gaterun.PacketSpec, questionID, status, basis string, index int) gaterun.Finding {
	title := readerQuestionByID(questionID)
	detail := fmt.Sprintf("[P1] %s (%s) — reader contract: the cited quote does not answer the question (%s) (actionable)\n  problem: the independent verifier judged the reader's citation for %s (%s) %s\n  evidence:\n    - Judgment: %s = %s — %s\n  impact: the quoted text exists in the human-readable part but does not convey the §9 point, so the answer is not locally reconstructable\n  fix: revise the cited section so it directly answers %s (%s)", questionID, title, status, questionID, title, status, questionID, status, basis, questionID, title)
	return gaterun.Finding{
		ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.PacketID, index+1),
		Severity:  "P1",
		Text:      fmt.Sprintf("reader contract: %s (%s) citation judged %s", questionID, title, status),
		Detail:    detail,
		SourceKey: gaterun.ReaderContractCheck,
	}
}

// parseVerifierJudgments parses the 17 Judgment lines of a verifier report.
func parseVerifierJudgments(report string, out *parsedReport) error {
	matches := verifierJudgRe.FindAllStringSubmatch(report, -1)
	if len(matches) != len(readerContractQuestions) {
		return fmt.Errorf("verifier report must declare exactly %d Judgment lines (`Judgment: <question id> = <supported|partial|unsupported> — <basis>`), got %d", len(readerContractQuestions), len(matches))
	}
	for i, m := range matches {
		want := readerContractQuestions[i]
		if m[1] != want.ID {
			return fmt.Errorf("Judgment line %d is %q, expected %q — judgments must follow the fixed bank order", i+1, m[1], want.ID)
		}
	}
	return nil
}

// validateVerifierPacket checks the verifier report against the accepted
// reader result: the verifier depends on exactly the reader packet, its
// declared scopes are exactly the human-readable section list, every
// no_local_answer question must be judged unsupported, the verdict must
// equal the mechanical outcome of the judgments, and every partial or
// unsupported judgment composes a mechanical finding. The verifier authors
// no findings of its own.
func validateVerifierPacket(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport, report string) error {
	if len(extractFindings(report)) > 0 {
		return errors.New("verifier packets report judgments only — finding entries are composed mechanically by gate-submit")
	}
	if len(spec.DependsOn) != 1 || spec.DependsOn[0] != "reader" {
		return fmt.Errorf("verifier packet %q must depend on exactly the reader packet", spec.PacketID)
	}
	readerState, err := gaterun.LoadPacketState(absRoot, run, "reader")
	if err != nil {
		return err
	}
	if readerState.Status != gaterun.PacketAccepted || readerState.Result == nil {
		return fmt.Errorf("verifier packet %q requires the accepted reader evidence report", spec.PacketID)
	}
	main, _, headings, err := narrativeSectionsFor(absRoot, run)
	if err != nil {
		return err
	}
	declared, err := declaredNarrativeScopes(parsed.Scopes, main)
	if err != nil {
		return err
	}
	if err := validateNarrativeScopeSet(declared, main, headings); err != nil {
		return err
	}
	noAnswer, err := readerNoAnswerQuestions(readerState.Report)
	if err != nil {
		return err
	}
	type judgment struct {
		id     string
		status string
		basis  string
	}
	var judgments []judgment
	for _, m := range verifierJudgRe.FindAllStringSubmatch(report, -1) {
		judgments = append(judgments, judgment{id: m[1], status: m[2], basis: m[3]})
	}
	anyFailure := len(noAnswer) > 0
	index := 0
	for _, j := range judgments {
		if _, missing := noAnswer[j.id]; missing {
			if j.status != "unsupported" {
				return fmt.Errorf("question %s has no local answer in the reader report; the verifier must judge it unsupported, got %s", j.id, j.status)
			}
			anyFailure = true
			continue
		}
		if j.status == "partial" || j.status == "unsupported" {
			anyFailure = true
			parsed.Findings = append(parsed.Findings, verifierUnsupportedFinding(run, spec, j.id, j.status, j.basis, index))
			index++
		}
	}
	verdict := parsed.Verdicts[gaterun.ReaderContractCheck]
	want := "PASS"
	if anyFailure {
		want = "FAIL"
	}
	if verdict != want {
		return fmt.Errorf("verifier verdict %s contradicts its judgments and the reader report; expected %s", verdict, want)
	}
	return nil
}

// readerNoAnswerQuestions extracts the no_local_answer question ids from an
// accepted reader evidence report.
func readerNoAnswerQuestions(report string) (map[string]bool, error) {
	out := map[string]bool{}
	var current string
	for _, raw := range strings.Split(report, "\n") {
		trimmed := strings.TrimSpace(raw)
		if m := readerQuestionRe.FindStringSubmatch(trimmed); m != nil {
			current = m[1]
			continue
		}
		if current == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "Status:") {
			if strings.TrimSpace(strings.TrimPrefix(trimmed, "Status:")) == "no_local_answer" {
				out[current] = true
			}
			current = ""
		}
	}
	return out, nil
}

// unitMainSpecPath resolves the target's unit main spec path for the run's
// layer.
func unitMainSpecPath(run *gaterun.Run) (string, error) {
	if run.TargetKind != gaterun.TargetKindUnit {
		return "", fmt.Errorf("the reader contract check targets unit specs only, got target kind %q", run.TargetKind)
	}
	if run.Target == gaterun.TargetCandidate {
		return specpaths.CandidateUnitSpecFileRef(run.TargetName), nil
	}
	return specpaths.StableUnitSpecFileRef(run.TargetName), nil
}

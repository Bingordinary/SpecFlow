package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

// The reader contract probe (unit validate Check 10) tests the framework's
// Reader Contract under reconstruction conditions. An independent reader
// session reads only the unit main spec's human-readable part — every ##
// section before the section holding the acceptance_item_set marker — and
// reconstructs the design closed-book: one contiguous restatement plus an
// honest Undetermined list. No question bank exists: per-question
// answerability cannot distinguish a design from a collage of locally valid
// statements. A second independent verifier session reconciles the accepted
// reconstruction against the human-readable part (support) and the formal
// carrier — the acceptance item set and the protocol appendices (backbone
// and contradiction): it classifies every claim of the reconstruction, maps
// every carrier item and §9 must-close decision into it, and judges the
// restatement's internal coherence. gate-submit composes the check verdict
// and the findings mechanically from the classifications; judgment enters
// the gate only at the classification step. Protocol source:
// framework/unit_validate_checklist.md Check 10.

// mustCloseDecisions is the fixed set of §9 must-close decisions the
// verifier must map into the reconstruction. The ids and titles must match
// the checklist's Check 10 decision table exactly.
var mustCloseDecisions = []struct {
	ID    string
	Title string
}{
	{"Q11", "which object owns a responsibility"},
	{"Q12", "which entry point starts the behavior"},
	{"Q13", "where state or durable truth lives"},
	{"Q14", "how ordered steps connect"},
	{"Q15", "how boundary failures are reported"},
	{"Q16", "what the result shape means"},
	{"Q17", "how acceptance proves the stated responsibility"},
}

func mustCloseDecisionByID(id string) string {
	for _, d := range mustCloseDecisions {
		if d.ID == id {
			return d.Title
		}
	}
	return ""
}

// readerRestatementMinRunes is the degenerate-noise bound for the whole
// restatement block. Whether the restatement really carries the design is
// the verifier packet's reconciliation, not a length rule.
const readerRestatementMinRunes = 4

var (
	readerReconstructionRe    = regexp.MustCompile(`^Reconstruction:\s*$`)
	readerUndeterminedRe      = regexp.MustCompile(`^Undetermined:\s*$`)
	readerUndeterminedItemRe  = regexp.MustCompile(`^- \S`)
	verifierClaimRe           = regexp.MustCompile(`(?m)^[ \t]*Claim:\s*(C\d{2,})\s*=\s*(supported|reader-error|unsupported-central|unsupported-minor|contradicted)\s*[—-]\s*(\S[^\n]*)$`)
	verifierClaimLineRe       = regexp.MustCompile(`(?m)^[ \t]*Claim:`)
	verifierCarrierRe         = regexp.MustCompile(`(?m)^[ \t]*Carrier:\s*(\S+)\s*=\s*(seen|missing)\s*[—-]\s*(\S[^\n]*)$`)
	verifierCarrierLineRe     = regexp.MustCompile(`(?m)^[ \t]*Carrier:`)
	verifierMustCloseRe       = regexp.MustCompile(`(?m)^[ \t]*Must-close:\s*(Q\d{2})\s*=\s*(closed|missing|not-applicable)\s*[—-]\s*(\S[^\n]*)$`)
	verifierMustCloseLineRe   = regexp.MustCompile(`(?m)^[ \t]*Must-close:`)
	verifierConsistencyRe     = regexp.MustCompile(`(?m)^[ \t]*Consistency:\s*(coherent|incoherent)\s*[—-]\s*(\S[^\n]*)$`)
	verifierConsistencyLineRe = regexp.MustCompile(`(?m)^[ \t]*Consistency:`)
)

// parseReaderReconstructionReport parses the closed-book reconstruction
// report: the Reconstruction block and the Undetermined list, in that
// order, followed by the Dependency scope section (parsed generically).
// The reader authors evidence only — it computes no verdict.
func parseReaderReconstructionReport(spec *gaterun.PacketSpec, report string, out *parsedReport) error {
	lines := strings.Split(report, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || !readerReconstructionRe.MatchString(strings.TrimSpace(lines[i])) {
		return fmt.Errorf("reader report must start with the `Reconstruction:` header line")
	}
	i++
	var restatement []string
	blockClosed := false
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if readerUndeterminedRe.MatchString(trimmed) {
			break
		}
		if strings.HasPrefix(trimmed, "Dependency scope:") || strings.HasPrefix(trimmed, "check-10:") {
			return fmt.Errorf("reader report is missing the `Undetermined:` list")
		}
		if trimmed == "" {
			if len(restatement) > 0 {
				blockClosed = true
			}
			i++
			continue
		}
		if blockClosed {
			return fmt.Errorf("the design restatement must be one contiguous block — %q belongs in the restatement or the Undetermined list", trimmed)
		}
		restatement = append(restatement, trimmed)
		i++
	}
	if i >= len(lines) {
		return fmt.Errorf("reader report is missing the `Undetermined:` list")
	}
	i++
	var undetermined []string
	none := false
	listClosed := false
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "Dependency scope:") {
			break
		}
		if trimmed == "" {
			if len(undetermined) > 0 || none {
				listClosed = true
			}
			i++
			continue
		}
		if listClosed {
			return fmt.Errorf("the Undetermined list must directly precede the Dependency scope section — unexpected line %q", trimmed)
		}
		if !readerUndeterminedItemRe.MatchString(trimmed) {
			return fmt.Errorf("Undetermined entries must be `- {point}` lines, got %q", trimmed)
		}
		entry := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		if strings.EqualFold(entry, "none") {
			none = true
		} else {
			undetermined = append(undetermined, entry)
		}
		i++
	}
	if none && len(undetermined) > 0 {
		return fmt.Errorf("`- none` cannot be combined with other Undetermined entries")
	}
	if !none && len(undetermined) == 0 {
		return fmt.Errorf("the Undetermined list is empty — declare exactly `- none` when the reconstruction left nothing undetermined")
	}
	total := 0
	for _, line := range restatement {
		total += utf8.RuneCountInString(line)
	}
	if total < readerRestatementMinRunes {
		return fmt.Errorf("the design restatement is %d runes; the bound is at least %d", total, readerRestatementMinRunes)
	}
	return nil
}

// narrativeSectionsFor resolves the target's unit main spec, its content, and
// its human-readable section list (the Check 10 reading scope). Both Check
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

// unitCarrierItemIDs extracts the acceptance item ids of the target's unit
// main spec — the Check 10 verifier's carrier backbone. A unit without a
// parseable acceptance item set has no carrier to reconcile against and
// fails closed.
func unitCarrierItemIDs(absRoot string, run *gaterun.Run) ([]string, error) {
	main, err := unitMainSpecPath(run)
	if err != nil {
		return nil, err
	}
	contentBytes, err := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(main)))
	if err != nil {
		return nil, fmt.Errorf("read the unit main spec %s: %w", main, err)
	}
	items := specvalidation.ExtractAcceptanceItemIDs(string(contentBytes))
	if len(items) == 0 {
		return nil, fmt.Errorf("the acceptance item set of %s is empty — the Check 10 verifier reconciles against the acceptance item set as the formal carrier; declare acceptance items per framework/spec_writing_guide.md §7", main)
	}
	return items, nil
}

// declaredNarrativeScopes collects a reader packet's declared scope
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
			return nil, fmt.Errorf("check %q scope line declares %s — the reader reads the unit main spec %s only", gaterun.ReaderContractCheck, s.Path, main)
		}
		declared = append(declared, s.Declaration)
	}
	return declared, nil
}

// validateNarrativeScopeSet enforces that a reader packet's declared scopes
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

// validateReaderReconstruction performs the mechanical validation of an
// accepted-form reader report: the declared Dependency scope sections must
// be exactly the human-readable section list of the target's unit main
// spec. The declared reading scope is the check's locality guarantee; the
// global-coherence judgment belongs to the verifier packet's
// reconciliation.
func validateReaderReconstruction(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport) error {
	main, _, headings, err := narrativeSectionsFor(absRoot, run)
	if err != nil {
		return err
	}
	declared, err := declaredNarrativeScopes(parsed.Scopes, main)
	if err != nil {
		return err
	}
	return validateNarrativeScopeSet(declared, main, headings)
}

// verifierClaim is one parsed Claim line: the verifier's classification of
// one statement of the reconstruction.
type verifierClaim struct {
	ID     string
	Status string // supported | reader-error | unsupported-central | unsupported-minor | contradicted
	Basis  string
}

// verifierCarrierLine is one parsed Carrier line: the mapping of one
// acceptance item's behavioral subject into the reconstruction.
type verifierCarrierLine struct {
	ItemID string
	Status string // seen | missing
	Basis  string
}

// verifierMustCloseLine is one parsed Must-close line: the mapping of one
// §9 must-close decision into the reconstruction.
type verifierMustCloseLine struct {
	DecisionID string
	Status     string // closed | missing | not-applicable
	Basis      string
}

// parseVerifierReconciliation validates the claim lines of a verifier
// report: at least one well-formed Claim line with sequential ids from
// C01. The carrier, must-close, and consistency lines are validated in
// validateVerifierPacket, which knows the target's carrier sets.
func parseVerifierReconciliation(report string) error {
	matches := verifierClaimRe.FindAllStringSubmatch(report, -1)
	if len(matches) != len(verifierClaimLineRe.FindAllString(report, -1)) {
		return errors.New("malformed Claim line; use `Claim: C{nn} = {supported|reader-error|unsupported-central|unsupported-minor|contradicted} — {basis}`")
	}
	if len(matches) == 0 {
		return errors.New("verifier report must classify at least one claim of the reconstruction (`Claim: C01 = {status} — {basis}`)")
	}
	for i, m := range matches {
		if want := fmt.Sprintf("C%02d", i+1); m[1] != want {
			return fmt.Errorf("Claim line %d is %q, expected %q — claim ids are sequential from C01", i+1, m[1], want)
		}
	}
	return nil
}

// validateVerifierPacket checks the verifier report against the accepted
// reader result: the verifier depends on exactly the reader packet, its
// declared scopes cover exactly the human-readable section list plus the
// formal carrier (one `acceptance_items` line on the main spec and one
// whole-file line per protocol appendix), every acceptance item and every
// §9 must-close decision is mapped exactly once, the consistency line is
// present, the verdict must equal the mechanical outcome of the
// classifications, and every blocking classification composes one
// mechanical finding. The verifier authors no findings of its own.
func validateVerifierPacket(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport, report string) error {
	if len(extractFindings(report)) > 0 {
		return errors.New("verifier packets report classifications only — finding entries are composed mechanically by gate-submit")
	}
	if len(spec.DependsOn) != 1 || spec.DependsOn[0] != "reader" {
		return fmt.Errorf("verifier packet %q must depend on exactly the reader packet", spec.PacketID)
	}
	readerState, err := gaterun.LoadPacketState(absRoot, run, "reader")
	if err != nil {
		return err
	}
	if readerState.Status != gaterun.PacketAccepted || readerState.Result == nil {
		return fmt.Errorf("verifier packet %q requires the accepted reader reconstruction report", spec.PacketID)
	}
	main, _, headings, err := narrativeSectionsFor(absRoot, run)
	if err != nil {
		return err
	}
	items, err := unitCarrierItemIDs(absRoot, run)
	if err != nil {
		return err
	}
	appendices := gaterun.UnitAppendices(absRoot, run.TargetName, run.Target)
	if err := validateVerifierScopes(run, parsed.Scopes, main, headings, appendices); err != nil {
		return err
	}

	var claims []verifierClaim
	for _, m := range verifierClaimRe.FindAllStringSubmatch(report, -1) {
		claims = append(claims, verifierClaim{ID: m[1], Status: m[2], Basis: m[3]})
	}
	carrierMatches := verifierCarrierRe.FindAllStringSubmatch(report, -1)
	if len(carrierMatches) != len(verifierCarrierLineRe.FindAllString(report, -1)) {
		return errors.New("malformed Carrier line; use `Carrier: {acceptance item id} = {seen|missing} — {basis}`")
	}
	if len(carrierMatches) != len(items) {
		return fmt.Errorf("verifier report must declare exactly one Carrier line per acceptance item (%d items), got %d", len(items), len(carrierMatches))
	}
	var carriers []verifierCarrierLine
	for i, m := range carrierMatches {
		if m[1] != items[i] {
			return fmt.Errorf("Carrier line %d is %q, expected %q — carriers follow the acceptance item set in document order", i+1, m[1], items[i])
		}
		carriers = append(carriers, verifierCarrierLine{ItemID: m[1], Status: m[2], Basis: m[3]})
	}
	mustCloseMatches := verifierMustCloseRe.FindAllStringSubmatch(report, -1)
	if len(mustCloseMatches) != len(verifierMustCloseLineRe.FindAllString(report, -1)) {
		return errors.New("malformed Must-close line; use `Must-close: {decision id} = {closed|missing|not-applicable} — {basis}`")
	}
	if len(mustCloseMatches) != len(mustCloseDecisions) {
		return fmt.Errorf("verifier report must declare exactly one Must-close line per §9 decision (%d decisions), got %d", len(mustCloseDecisions), len(mustCloseMatches))
	}
	var mustCloses []verifierMustCloseLine
	for i, m := range mustCloseMatches {
		if m[1] != mustCloseDecisions[i].ID {
			return fmt.Errorf("Must-close line %d is %q, expected %q — decisions follow the fixed §9 order", i+1, m[1], mustCloseDecisions[i].ID)
		}
		mustCloses = append(mustCloses, verifierMustCloseLine{DecisionID: m[1], Status: m[2], Basis: m[3]})
	}
	consistencyMatches := verifierConsistencyRe.FindAllStringSubmatch(report, -1)
	if len(consistencyMatches) != 1 || len(verifierConsistencyLineRe.FindAllString(report, -1)) != 1 {
		return errors.New("verifier report must declare exactly one `Consistency: {coherent|incoherent} — {basis}` line")
	}

	blocking := false
	index := 0
	for _, claim := range claims {
		switch claim.Status {
		case "contradicted", "unsupported-central":
			blocking = true
			parsed.Findings = append(parsed.Findings, verifierClaimFinding(run, spec, claim, index))
			index++
		}
	}
	for _, carrier := range carriers {
		if carrier.Status != "missing" {
			continue
		}
		blocking = true
		parsed.Findings = append(parsed.Findings, verifierCarrierFinding(run, spec, carrier, index))
		index++
	}
	for _, mustClose := range mustCloses {
		if mustClose.Status != "missing" {
			continue
		}
		blocking = true
		parsed.Findings = append(parsed.Findings, verifierMustCloseFinding(run, spec, mustClose, index))
		index++
	}
	if consistencyMatches[0][1] == "incoherent" {
		blocking = true
		parsed.Findings = append(parsed.Findings, verifierConsistencyFinding(run, spec, consistencyMatches[0][2], index))
		index++
	}
	verdict := parsed.Verdicts[gaterun.ReaderContractCheck]
	want := "PASS"
	if blocking {
		want = "FAIL"
	}
	if verdict != want {
		return fmt.Errorf("verifier verdict %s contradicts its classifications; expected %s", verdict, want)
	}
	return nil
}

// validateVerifierScopes enforces the verifier's declared evidence: exactly
// the human-readable section list of the main spec, exactly one
// `acceptance_items` line on the main spec, and exactly one whole-file line
// per protocol appendix — the support source and the formal carrier the
// reconciliation depends on.
func validateVerifierScopes(run *gaterun.Run, scopes []parsedScope, main string, headings, appendices []string) error {
	var narrative []string
	acceptanceItems := 0
	declaredAppendices := map[string]bool{}
	for _, s := range scopes {
		if s.Key != gaterun.ReaderContractCheck {
			continue
		}
		decl, err := parseDecl(s.Declaration)
		if err != nil {
			return fmt.Errorf("check %q declaration for %s: %w", gaterun.ReaderContractCheck, s.Path, err)
		}
		if s.Path != main {
			if !decl.WholeFile {
				return fmt.Errorf("check %q scope line for protocol appendix %s must declare the whole file (`all`), got %q", gaterun.ReaderContractCheck, s.Path, s.Declaration)
			}
			declaredAppendices[s.Path] = true
			continue
		}
		switch {
		case decl.Accepts:
			acceptanceItems++
		case decl.WholeFile || len(decl.Ranges) > 0 || len(decl.Items) > 0:
			return fmt.Errorf("check %q must declare each human-readable section of %s by heading plus one `acceptance_items` line — got %q", gaterun.ReaderContractCheck, main, s.Declaration)
		default:
			narrative = append(narrative, s.Declaration)
		}
	}
	if acceptanceItems != 1 {
		return fmt.Errorf("check %q must declare exactly one `check-10: %s: acceptance_items` line — the acceptance item set is the formal carrier, got %d", gaterun.ReaderContractCheck, main, acceptanceItems)
	}
	if err := validateNarrativeScopeSet(narrative, main, headings); err != nil {
		return err
	}
	declaredAppendixList := make([]string, 0, len(declaredAppendices))
	for path := range declaredAppendices {
		declaredAppendixList = append(declaredAppendixList, path)
	}
	sort.Strings(declaredAppendixList)
	if len(declaredAppendixList) != len(appendices) {
		return fmt.Errorf("check %q must declare exactly the protocol appendices (%d declared, %d required) — one whole-file Dependency scope line per appendix", gaterun.ReaderContractCheck, len(declaredAppendixList), len(appendices))
	}
	for i, appendix := range appendices {
		if declaredAppendixList[i] != appendix {
			return fmt.Errorf("check %q declares protocol appendix %q, which is not an appendix of unit %s", gaterun.ReaderContractCheck, declaredAppendixList[i], run.TargetName)
		}
	}
	return nil
}

// verifierClaimFinding composes the mechanical finding for one blocked
// claim classification: a claim the formal carrier contradicts, or a
// central claim whose substance the human-readable part never states.
func verifierClaimFinding(run *gaterun.Run, spec *gaterun.PacketSpec, claim verifierClaim, index int) gaterun.Finding {
	var text, problem, impact, fix string
	switch claim.Status {
	case "contradicted":
		text = fmt.Sprintf("reader contract: claim %s is contradicted by the formal carrier", claim.ID)
		problem = "the reconstruction states something the acceptance item set or a protocol appendix contradicts"
		impact = "the human-readable part and the formal carrier disagree; the design cannot be reconstructed faithfully, so user adjudication is required"
		fix = "reconcile the narrative with the carrier — correct whichever side is stale so the reconstruction and the acceptance items agree"
	case "unsupported-central":
		text = fmt.Sprintf("reader contract: claim %s is unsupported by the human-readable part (central mechanism)", claim.ID)
		problem = "the claim's substance exists in the formal carrier but the human-readable part never states it"
		impact = "a reader cannot reconstruct this central mechanism from the human-readable part alone; the design must be reconstructed from the carrier or the author"
		fix = "state the substance in the narrative before the acceptance section"
	}
	detail := fmt.Sprintf("[P1] %s — %s (actionable)\n  problem: %s\n  evidence:\n    - Claim: %s = %s — %s\n  impact: %s\n  fix: %s",
		gaterun.ReaderContractCheck, text, problem, claim.ID, claim.Status, claim.Basis, impact, fix)
	return gaterun.Finding{
		ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.PacketID, index+1),
		Severity:  "P1",
		Text:      text,
		Detail:    detail,
		SourceKey: gaterun.ReaderContractCheck,
	}
}

// verifierCarrierFinding composes the mechanical finding for one missing
// carrier mapping: neither the restatement nor the Undetermined list lets
// the verifier see the acceptance item's behavioral subject.
func verifierCarrierFinding(run *gaterun.Run, spec *gaterun.PacketSpec, carrier verifierCarrierLine, index int) gaterun.Finding {
	text := fmt.Sprintf("reader contract: acceptance item %s's behavioral subject is missing from the reconstruction", carrier.ItemID)
	detail := fmt.Sprintf("[P1] %s — reader contract: acceptance item %s's behavioral subject is missing from the reconstruction (actionable)\n  problem: neither the restatement nor the Undetermined list lets the verifier see the behavioral subject of acceptance item %s — the document did not let the reader see it\n  evidence:\n    - Carrier: %s = missing — %s\n  impact: the verification contract's subject cannot be reconstructed from the human-readable part; acceptance exercises a design the narrative never presents\n  fix: let the narrative carry the item's behavioral subject before the acceptance section",
		gaterun.ReaderContractCheck, carrier.ItemID, carrier.ItemID, carrier.ItemID, carrier.Basis)
	return gaterun.Finding{
		ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.PacketID, index+1),
		Severity:  "P1",
		Text:      text,
		Detail:    detail,
		SourceKey: gaterun.ReaderContractCheck,
	}
}

// verifierMustCloseFinding composes the mechanical finding for one missing
// must-close decision: the reconstruction neither closes it nor carries it
// as a declared boundary.
func verifierMustCloseFinding(run *gaterun.Run, spec *gaterun.PacketSpec, mustClose verifierMustCloseLine, index int) gaterun.Finding {
	title := mustCloseDecisionByID(mustClose.DecisionID)
	text := fmt.Sprintf("reader contract: must-close decision %s (%s) is missing from the reconstruction", mustClose.DecisionID, title)
	detail := fmt.Sprintf("[P1] %s — reader contract: must-close decision %s (%s) is missing from the reconstruction (actionable)\n  problem: the reconstruction neither closes %s nor lets the verifier see it as a declared boundary\n  evidence:\n    - Must-close: %s = missing — %s\n  impact: the downstream executor is forced to choose on this decision — §9's closure promise is broken\n  fix: close the decision in the narrative, or state its open boundary and why (framework/spec_writing_guide.md §9)",
		gaterun.ReaderContractCheck, mustClose.DecisionID, title, mustClose.DecisionID, mustClose.DecisionID, mustClose.Basis)
	return gaterun.Finding{
		ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.PacketID, index+1),
		Severity:  "P1",
		Text:      text,
		Detail:    detail,
		SourceKey: gaterun.ReaderContractCheck,
	}
}

// verifierConsistencyFinding composes the mechanical finding for an
// internally incoherent reconstruction: the restatement contradicts itself,
// so different readings produce different designs.
func verifierConsistencyFinding(run *gaterun.Run, spec *gaterun.PacketSpec, basis string, index int) gaterun.Finding {
	text := "reader contract: the reconstruction is internally incoherent"
	detail := fmt.Sprintf("[P1] %s — reader contract: the reconstruction is internally incoherent (actionable)\n  problem: the restatement contradicts itself\n  evidence:\n    - Consistency: incoherent — %s\n  impact: a self-contradicting reconstruction means the narrative is ambiguous — different readings produce different designs\n  fix: resolve the contradiction in the narrative so one reading survives",
		gaterun.ReaderContractCheck, basis)
	return gaterun.Finding{
		ID:        fmt.Sprintf("%s/%s/F%d", run.RunID, spec.PacketID, index+1),
		Severity:  "P1",
		Text:      text,
		Detail:    detail,
		SourceKey: gaterun.ReaderContractCheck,
	}
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

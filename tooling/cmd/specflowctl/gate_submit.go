package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// runGateSubmit records one packet report and its parsed result after
// mechanical validation. Declarations are checked against packet-local read
// refs; analysis/cross bind the exact dependency-result digests they consume;
// verify detection resolves the conditional analysis packet.
func runGateSubmit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-submit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id printed by gate-plan")
	packetIDPtr := fs.String("packet", "", "packet id from the run's plan")
	reportPtr := fs.String("report", "", "path to the packet report file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID := strings.TrimSpace(*runIDPtr)
	packetID := strings.TrimSpace(*packetIDPtr)
	reportPath := strings.TrimSpace(*reportPtr)
	if runID == "" || packetID == "" || reportPath == "" {
		writeGateSubmitUsage(stderr)
		return errors.New("--run, --packet, and --report are required")
	}

	absRoot := mustAbs(*repoRootPtr)
	return gaterun.WithMutation(absRoot, func() error {
		return submitGatePacket(absRoot, runID, packetID, reportPath, stdout)
	})
}

func submitGatePacket(absRoot, runID, packetID, reportPath string, stdout io.Writer) error {
	run, err := gaterun.Load(absRoot, runID)
	if err != nil {
		return err
	}
	if run.Status != gaterun.StatusOpen {
		return fmt.Errorf("gate run %s is %s — only an open run accepts submissions; plan a new run", run.RunID, run.Status)
	}
	spec := run.PacketByID(packetID)
	if spec == nil {
		return fmt.Errorf("packet %q is not part of run %s's plan (packet ids: %s)", packetID, run.RunID, strings.Join(packetPlanIDs(run), ", "))
	}
	state, err := gaterun.LoadPacketState(absRoot, run, packetID)
	if err != nil {
		return err
	}
	if state.Status == gaterun.PacketAccepted {
		return fmt.Errorf("packet %q is already accepted (terminal) — an accepted packet cannot be replaced; plan a new run if the result must change", packetID)
	}
	if state.Status == gaterun.PacketNotRequired {
		return fmt.Errorf("packet %q is not_required (terminal) — a not-required packet accepts no submissions; plan a new run if the result must change", packetID)
	}
	for _, dep := range spec.DependsOn {
		depState, derr := gaterun.LoadPacketState(absRoot, run, dep)
		if derr != nil {
			return derr
		}
		if depState.Status != gaterun.PacketAccepted && depState.Status != gaterun.PacketNotRequired {
			return fmt.Errorf("gate-submit rejected: dependency packet %q is %s — submit it first: `specflowctl gate-submit --run %s --packet %s --report PATH`", dep, depState.Status, run.RunID, dep)
		}
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("read --report %s: %w", reportPath, err)
	}
	report := string(data)
	digest := reportDigest(report)
	attempt := len(state.Attempts) + 1
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	reject := func(reason string) error {
		state.Status = gaterun.PacketRejected
		state.Attempts = append(state.Attempts, gaterun.Attempt{
			Attempt:         attempt,
			SubmittedAt:     now,
			Status:          gaterun.PacketRejected,
			RejectionReason: reason,
			ResultDigest:    digest,
		})
		if saveErr := gaterun.SavePacketState(absRoot, run, state); saveErr != nil {
			return saveErr
		}
		return fmt.Errorf("gate-submit rejected: %s (attempt %d recorded — fix the report and re-submit)", reason, attempt)
	}

	if strings.TrimSpace(report) == "" {
		return reject("empty report")
	}
	parsed, perr := parsePacketReport(run, spec, report)
	if perr != nil {
		return reject(perr.Error())
	}
	normalizeDeclaredPaths(absRoot, parsed)
	if derr := validatePacketDeclarations(absRoot, run, spec, parsed); derr != nil {
		return reject(derr.Error())
	}
	if derr := validatePacketSemantics(absRoot, run, spec, parsed); derr != nil {
		return reject(derr.Error())
	}

	state.Status = gaterun.PacketAccepted
	state.Report = report
	state.Result = packetResult(spec, parsed, digest)
	state.ConsumedResultDigests = consumedResultDigests(absRoot, run, spec)
	state.Attempts = append(state.Attempts, gaterun.Attempt{
		Attempt:      attempt,
		SubmittedAt:  now,
		Status:       gaterun.PacketAccepted,
		ResultDigest: digest,
	})
	if err := gaterun.SavePacketState(absRoot, run, state); err != nil {
		return err
	}
	if spec.Kind == gaterun.PacketKindItem {
		if err := resolveVerifyAnalysis(absRoot, run, spec, parsed); err != nil {
			return err
		}
	}

	accepted, pending, rejected, err := packetCounts(absRoot, run)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Packet accepted: %s (attempt %d, digest %s)\n", packetID, attempt, digest)
	fmt.Fprintf(stdout, "Progress: %d accepted, %d pending, %d rejected of %d packet(s)\n", accepted, pending, rejected, len(run.Packets))
	fmt.Fprintf(stdout, "Next: %s\n", gateNextStep(absRoot, run))
	return nil
}

// normalizeDeclaredPaths rewrites every parsed declaration path — dependency
// scopes and severity-confirmation evidence — to its canonical repo-relative
// spelling before validation and persistence, so the run snapshot, the stored
// packet result, and the finalize-time cache assembly all speak one path form
// (see framework/validation_cache.md §Format: `./` prefixes, absolute paths,
// and platform separators in a recorded path are equivalent).
func normalizeDeclaredPaths(absRoot string, parsed *parsedReport) {
	for i := range parsed.Scopes {
		parsed.Scopes[i].Path = gaterun.CanonicalDeclPath(absRoot, parsed.Scopes[i].Path)
	}
	for i := range parsed.SeverityChecks {
		parsed.SeverityChecks[i].EvidencePath = gaterun.CanonicalDeclPath(absRoot, parsed.SeverityChecks[i].EvidencePath)
	}
	for i := range parsed.Ownerships {
		parsed.Ownerships[i].EvidencePath = gaterun.CanonicalDeclPath(absRoot, parsed.Ownerships[i].EvidencePath)
	}
}

// validatePacketDeclarations validates every dependency-scope line's path
// form, snapshot membership, and declaration parseability.
func validatePacketDeclarations(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport) error {
	// Merge the declarations of one check key onto one path: multiple scope
	// lines for the same (key, path) declare the union of their scopes; a
	// single "all" line makes the declaration whole-file.
	type keyPath struct {
		key  string
		path string
	}
	merged := map[keyPath]*declParts{}
	var order []keyPath
	for _, s := range parsed.Scopes {
		kp := keyPath{s.Key, s.Path}
		m := merged[kp]
		if m == nil {
			m = &declParts{}
			merged[kp] = m
			order = append(order, kp)
		}
		d, derr := parseDecl(s.Declaration)
		if derr != nil {
			return fmt.Errorf("check %q declaration for %s: %w", kp.key, kp.path, derr)
		}
		mergeDecl(m, d)
	}
	for _, kp := range order {
		m := merged[kp]
		if err := validationcache.ValidateEntryPathForm(run.TargetKind, run.TargetName, kp.path); err != nil {
			return fmt.Errorf("check %q declaration %s: %w", kp.key, kp.path, err)
		}
		if !run.PacketAllowsDeclaration(absRoot, spec, kp.path) {
			return fmt.Errorf("check %q declaration %q is not part of packet %q's read refs — add evidence with --input at gate-plan time or use the packet that owns this input", kp.key, kp.path, spec.PacketID)
		}
		decl := validationcache.CheckDeclaration{Check: kp.key}
		if !m.WholeFile {
			decl.Sections = m.Sections
			decl.Ranges = strings.Join(m.Ranges, ",")
			decl.AcceptanceItems = m.Accepts
			decl.AcceptanceItemIDs = m.Items
		}
		if _, err := validationcache.BuildEntryFromChecks(absRoot, kp.path, []validationcache.CheckDeclaration{decl}); err != nil {
			return fmt.Errorf("check %q declaration for %s is invalid: %w", kp.key, kp.path, err)
		}
	}
	return nil
}

func validatePacketSemantics(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport) error {
	if len(parsed.SeverityChecks) > 0 && spec.Kind != gaterun.PacketKindCross && !(spec.Kind == gaterun.PacketKindChecks && run.TargetKind == gaterun.TargetKindRule) {
		return errors.New("severity confirmations belong to the unit cross packet or the single rule-validate checks packet")
	}
	if len(parsed.Ownerships) > 0 && spec.Kind != gaterun.PacketKindCross {
		return errors.New("ownership records belong to the cross packet")
	}
	switch spec.Kind {
	case gaterun.PacketKindItem:
		if len(parsed.Findings) > 0 {
			return errors.New("verify detection packets report mismatch type only; severity belongs to the analysis packet")
		}
	case gaterun.PacketKindAnalysis:
		if len(spec.DependsOn) != 1 {
			return errors.New("analysis packet must depend on exactly one detection packet")
		}
		detection, err := gaterun.LoadPacketState(absRoot, run, spec.DependsOn[0])
		if err != nil {
			return err
		}
		if detection.Result == nil || detection.Result.Verdicts[spec.CheckKeys[0]] != "MISMATCH" {
			return fmt.Errorf("analysis packet %q is not required because its detection result is not MISMATCH", spec.PacketID)
		}
	case gaterun.PacketKindChecks:
		if run.TargetKind == gaterun.TargetKindRule {
			var required []gaterun.Finding
			for _, finding := range parsed.Findings {
				if finding.Severity == "P0" {
					required = append(required, finding)
				}
			}
			canonical, err := validateSeverityConfirmations(absRoot, run, spec, parsed, required)
			if err != nil {
				return err
			}
			// The canonical finding carries both the adjusted severity and the
			// detail prefix rewritten to the final level (see
			// applySeverityConfirmations); persist the whole value so the
			// judgment baseline and any carried rendering stay consistent.
			canonicalByID := map[string]gaterun.Finding{}
			for _, finding := range canonical {
				canonicalByID[finding.ID] = finding
			}
			for i := range parsed.Findings {
				if canonicalFinding, ok := canonicalByID[parsed.Findings[i].ID]; ok {
					parsed.Findings[i] = canonicalFinding
				}
			}
		}
		fails := verdictCount(parsed.Verdicts, "FAIL")
		blocking := blockingFindingCount(parsed.Findings)
		if fails > 0 && blocking == 0 {
			return errors.New("a validate FAIL verdict requires at least one P0/P1 finding in the same packet")
		}
		if fails == 0 && blocking > 0 {
			return errors.New("a validate packet with P0/P1 findings must report at least one FAIL verdict")
		}
	case gaterun.PacketKindFile:
		// The conclusion mapping is mechanical for its P0/P1 half: the
		// conclusion, the gate_findings entries, and the report's P0/P1
		// findings must agree (see framework/spec_review_checklist.md
		// §Output Format). The P2/P3/none distinction stays
		// author-declared — it has no machine carrier.
		unacceptable := verdictCount(parsed.Verdicts, "unacceptable") > 0
		gateFindingsBlocking := gateFindingsDeclareBlocking(parsed.GateFindings)
		if unacceptable && blockingFindingCount(parsed.Findings) == 0 {
			return errors.New("an unacceptable review conclusion requires at least one P0/P1 finding")
		}
		if unacceptable && !gateFindingsBlocking {
			return errors.New("an unacceptable review conclusion requires a P0/P1 gate_findings entry")
		}
		if gateFindingsBlocking && !unacceptable {
			return errors.New("a P0/P1 gate_findings entry requires the unacceptable conclusion")
		}
	case gaterun.PacketKindCross:
		return validateCrossSynthesis(absRoot, run, parsed)
	}
	return nil
}

func validateCrossSynthesis(absRoot string, run *gaterun.Run, parsed *parsedReport) error {
	expectedStatus := map[string]bool{gaterun.CrossKey: true}
	var inputFindings []gaterun.Finding
	for _, packet := range run.Packets {
		if packet.Kind == gaterun.PacketKindCross || packet.Kind == gaterun.PacketKindAnalysis {
			continue
		}
		for _, key := range packet.CheckKeys {
			expectedStatus[key] = true
		}
	}
	for _, key := range run.CarriedKeys {
		expectedStatus[key] = true
	}
	for _, packet := range run.Packets {
		if packet.Kind == gaterun.PacketKindCross {
			continue
		}
		state, err := gaterun.LoadPacketState(absRoot, run, packet.PacketID)
		if err != nil {
			return err
		}
		if state.Status != gaterun.PacketAccepted || state.Result == nil {
			continue
		}
		for _, finding := range resultFindings(state.Result) {
			inputFindings = append(inputFindings, finding)
		}
	}
	for _, result := range run.CarriedResults {
		for _, finding := range resultFindings(&result) {
			inputFindings = append(inputFindings, finding)
		}
	}
	for _, deferred := range run.DeferredFindings {
		inputFindings = append(inputFindings, deferred.Finding)
	}

	for key := range expectedStatus {
		if _, ok := parsed.EffectiveStatus[key]; !ok {
			return fmt.Errorf("cross report is missing Effective status for %q", key)
		}
	}
	for key := range parsed.EffectiveStatus {
		if !expectedStatus[key] {
			return fmt.Errorf("cross report declares unexpected Effective status for %q", key)
		}
	}

	retained, err := resolveCrossFindings(inputFindings, parsed.Findings, parsed.Dispositions)
	if err != nil {
		return err
	}
	for _, finding := range parsed.Findings {
		if len(findingKeySet(finding)) == 0 {
			return fmt.Errorf("new cross finding %q must name at least one affected non-cross logical key", finding.ID)
		}
	}
	retained, err = validateSeverityConfirmations(absRoot, run, run.PacketByID(gaterun.CrossKey), parsed, retained)
	if err != nil {
		return err
	}
	retained, err = validateOwnerships(absRoot, run, run.PacketByID(gaterun.CrossKey), parsed, retained)
	if err != nil {
		return err
	}
	wantStatus := make(map[string]string, len(expectedStatus))
	for key := range expectedStatus {
		if key != gaterun.CrossKey {
			wantStatus[key] = "pass"
		}
	}
	for _, finding := range retained {
		for key := range findingKeySet(finding) {
			if !expectedStatus[key] || key == gaterun.CrossKey {
				return fmt.Errorf("retained finding %q affects invalid logical key %q", finding.ID, key)
			}
			if gateDriving(finding, run.TargetName) {
				wantStatus[key] = "fail"
			}
		}
	}
	for key, want := range wantStatus {
		if got := parsed.EffectiveStatus[key]; got != want {
			return fmt.Errorf("retained findings require `Effective status: %s = %s` (got %s)", key, want, got)
		}
	}

	crossVerdict := parsed.Verdicts[gaterun.CrossKey]
	wantCrossStatus := "pass"
	if crossVerdict == "FAIL" {
		wantCrossStatus = "fail"
		newIDs := map[string]bool{}
		for _, finding := range parsed.Findings {
			newIDs[finding.ID] = true
		}
		var canonicalCrossFindings []gaterun.Finding
		for _, finding := range retained {
			if newIDs[finding.ID] && gateDriving(finding, run.TargetName) {
				canonicalCrossFindings = append(canonicalCrossFindings, finding)
			}
		}
		if blockingFindingCount(canonicalCrossFindings) == 0 {
			return errors.New("Cross-check FAIL requires at least one new P0/P1 cross finding owned by this unit or unassigned")
		}
	}
	if parsed.EffectiveStatus[gaterun.CrossKey] != wantCrossStatus {
		return fmt.Errorf("cross verdict %s requires `Effective status: cross = %s`", crossVerdict, wantCrossStatus)
	}

	return nil
}

func validateSeverityConfirmations(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport, findings []gaterun.Finding) ([]gaterun.Finding, error) {
	canonical, err := applySeverityConfirmations(findings, parsed.SeverityChecks)
	if err != nil {
		return nil, err
	}
	for _, confirmation := range parsed.SeverityChecks {
		if strings.TrimSpace(confirmation.EvidencePath) == "" || strings.TrimSpace(confirmation.Reason) == "" {
			return nil, fmt.Errorf("severity confirmation for finding %q requires evidence and reason", confirmation.FindingID)
		}
		if !run.PacketAllowsDeclaration(absRoot, spec, confirmation.EvidencePath) {
			return nil, fmt.Errorf("severity confirmation for finding %q cites %q outside packet %q's read refs", confirmation.FindingID, confirmation.EvidencePath, spec.PacketID)
		}
		if !packetDeclaresPath(spec, parsed, confirmation.EvidencePath) {
			return nil, fmt.Errorf("severity confirmation for finding %q cites %q without a matching Dependency scope declaration", confirmation.FindingID, confirmation.EvidencePath)
		}
	}
	return canonical, nil
}

// packetDeclaresPath reports whether the report carries a Dependency scope
// declaration for path. A cross packet's evidence must be covered by a `cross`
// scope line; other packet kinds match any of their own declarations.
func packetDeclaresPath(spec *gaterun.PacketSpec, parsed *parsedReport, path string) bool {
	for _, scope := range parsed.Scopes {
		if scope.Path != path {
			continue
		}
		if spec.Kind != gaterun.PacketKindCross || scope.Key == gaterun.CrossKey {
			return true
		}
	}
	return false
}

// validateOwnerships applies the cross report's ownership records to the
// terminal retained findings. Each record must cite evidence inside the cross
// packet's read refs, covered by a `cross` dependency-scope declaration, and
// name a unit that exists in the repository — a deferral to a nonexistent
// unit would route the finding nowhere, so it fails closed here.
func validateOwnerships(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport, findings []gaterun.Finding) ([]gaterun.Finding, error) {
	if len(parsed.Ownerships) > 0 && run.Gate != gaterun.GateReview {
		return nil, fmt.Errorf("ownership records are review-only — the %s gate has no ownership dimension", run.Gate)
	}
	canonical, err := applyOwnerships(findings, parsed.Ownerships)
	if err != nil {
		return nil, err
	}
	for _, ownership := range parsed.Ownerships {
		if !run.PacketAllowsDeclaration(absRoot, spec, ownership.EvidencePath) {
			return nil, fmt.Errorf("ownership record for finding %q cites %q outside packet %q's read refs", ownership.FindingID, ownership.EvidencePath, spec.PacketID)
		}
		if !packetDeclaresPath(spec, parsed, ownership.EvidencePath) {
			return nil, fmt.Errorf("ownership record for finding %q cites %q without a matching Dependency scope declaration", ownership.FindingID, ownership.EvidencePath)
		}
		if err := specpaths.ValidateTargetName("unit", ownership.OwnerUnit); err != nil {
			return nil, fmt.Errorf("ownership record for finding %q: %w", ownership.FindingID, err)
		}
		if specpaths.ResolveUnitFile(absRoot, ownership.OwnerUnit) == "" {
			return nil, fmt.Errorf("ownership record for finding %q names unit %q, which exists in no layer — a deferral must route to a real unit", ownership.FindingID, ownership.OwnerUnit)
		}
	}
	return canonical, nil
}

func verdictCount(verdicts map[string]string, token string) int {
	count := 0
	for _, verdict := range verdicts {
		if verdict == token {
			count++
		}
	}
	return count
}

func blockingFindingCount(findings []gaterun.Finding) int {
	count := 0
	for _, finding := range findings {
		if finding.Severity == "P0" || finding.Severity == "P1" {
			count++
		}
	}
	return count
}

func packetResult(spec *gaterun.PacketSpec, parsed *parsedReport, digest string) *gaterun.PacketResult {
	scopes := make([]gaterun.Scope, 0, len(parsed.Scopes))
	for _, scope := range parsed.Scopes {
		scopes = append(scopes, gaterun.Scope{Key: scope.Key, Path: scope.Path, Declaration: scope.Declaration})
	}
	return &gaterun.PacketResult{
		PacketID:        spec.PacketID,
		Kind:            spec.Kind,
		Verdicts:        parsed.Verdicts,
		Scopes:          scopes,
		Findings:        parsed.Findings,
		EffectiveStatus: parsed.EffectiveStatus,
		Dispositions:    parsed.Dispositions,
		SeverityChecks:  parsed.SeverityChecks,
		Ownerships:      parsed.Ownerships,
		Analysis:        parsed.Analysis,
		ReportDigest:    digest,
	}
}

func consumedResultDigests(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec) map[string]string {
	if spec.Kind != gaterun.PacketKindAnalysis && spec.Kind != gaterun.PacketKindCross {
		return nil
	}
	out := map[string]string{}
	for _, dep := range spec.DependsOn {
		state, err := gaterun.LoadPacketState(absRoot, run, dep)
		if err != nil || state.Status == gaterun.PacketNotRequired || state.Result == nil {
			continue
		}
		out[dep] = state.Result.ReportDigest
	}
	if spec.Kind == gaterun.PacketKindCross {
		for _, result := range run.CarriedResults {
			out[result.PacketID] = result.ReportDigest
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveVerifyAnalysis(absRoot string, run *gaterun.Run, spec *gaterun.PacketSpec, parsed *parsedReport) error {
	if len(spec.CheckKeys) != 1 {
		return nil
	}
	analysisID := "analysis:" + spec.CheckKeys[0]
	analysis := run.PacketByID(analysisID)
	if analysis == nil {
		return nil
	}
	if parsed.Verdicts[spec.CheckKeys[0]] == "MISMATCH" {
		return nil
	}
	state, err := gaterun.LoadPacketState(absRoot, run, analysisID)
	if err != nil {
		return err
	}
	if state.Status == gaterun.PacketPending {
		state.Status = gaterun.PacketNotRequired
		return gaterun.SavePacketState(absRoot, run, state)
	}
	return nil
}

// reportDigest is the normalized-text sha256 of a packet report.
func reportDigest(report string) string {
	sum := sha256.Sum256([]byte(specpaths.NormalizeText(report)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func packetPlanIDs(run *gaterun.Run) []string {
	var ids []string
	for _, p := range run.Packets {
		ids = append(ids, p.PacketID)
	}
	return ids
}

func writeGateSubmitUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl gate-submit --run RUN_ID --packet PACKET_ID --report PATH [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Records one packet report after mechanical validation: the run is open, the")
	fmt.Fprintln(w, "packet exists and is not already accepted, its dependencies are resolved,")
	fmt.Fprintln(w, "and the report is structurally complete — every check key has exactly one")
	fmt.Fprintln(w, "verdict line with an allowed token and evidence basis, gate-specific required")
	fmt.Fprintln(w, "fields are present, and every check key declares at least one Dependency scope")
	fmt.Fprintln(w, "line inside packet read refs.")
	fmt.Fprintln(w, "Analysis and cross packets bind accepted dependency result digests; cross also")
	fmt.Fprintln(w, "must dispose every input finding, publish every effective logical status, and")
	fmt.Fprintln(w, "map each new cross finding to the logical keys it makes fail. Every terminal")
	fmt.Fprintln(w, "retained finding needs a complete evidence-backed severity-confirmation sequence;")
	fmt.Fprintln(w, "rule validate carries the required sequence for each P0 in its checks packet.")
	fmt.Fprintln(w, "A valid report is recorded as accepted (terminal); an invalid one as rejected")
	fmt.Fprintln(w, "with the reason — it can be re-submitted, and every attempt is kept.")
	fmt.Fprintln(w, "See framework/verification_scope.md §Gate Work Packets.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --run RUN_ID     gate run id printed by gate-plan (required)")
	fmt.Fprintln(w, "  --packet ID      packet id from the run's plan (required)")
	fmt.Fprintln(w, "  --report PATH    file holding the packet report (required)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

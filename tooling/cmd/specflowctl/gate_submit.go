package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// runGateSubmit records one reviewer session's report for an agent-assigned
// coverage key batch. Declarations are checked against the session's read
// refs; the final cross synthesis binds the accepted dependency result digests
// it consumes.
func runGateSubmit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-submit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	runIDPtr := fs.String("run", "", "gate run id printed by gate-plan")
	sessionIDPtr := fs.String("session", "", "session id (single key, or the derived batch id from gate-mission)")
	keysPtr := fs.String("keys", "", "comma-separated coverage keys assigned to this session")
	reportPtr := fs.String("report", "", "path to the session report file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	runID := strings.TrimSpace(*runIDPtr)
	sessionID := strings.TrimSpace(*sessionIDPtr)
	keys := splitKeys(*keysPtr)
	reportPath := strings.TrimSpace(*reportPtr)
	if runID == "" || sessionID == "" || reportPath == "" {
		writeGateSubmitUsage(stderr)
		return errors.New("--run, --session, and --report are required")
	}
	if len(keys) == 0 {
		writeGateSubmitUsage(stderr)
		return errors.New("--keys is required (a single coverage key, or `cross` for the final synthesis)")
	}

	absRoot := mustAbs(*repoRootPtr)
	return gaterun.WithMutation(absRoot, func() error {
		return submitGateSession(absRoot, runID, sessionID, keys, reportPath, stdout)
	})
}

func submitGateSession(absRoot, runID, sessionID string, keys []string, reportPath string, stdout io.Writer) error {
	run, err := gaterun.Load(absRoot, runID)
	if err != nil {
		return err
	}
	if run.Status != gaterun.StatusOpen {
		return fmt.Errorf("gate run %s is %s — only an open run accepts submissions; plan a new run", run.RunID, run.Status)
	}
	spec, err := gaterun.BuildSessionSpec(absRoot, run, keys)
	if err != nil {
		return err
	}
	if err := gaterun.ClaimShared(absRoot, run, keys); err != nil {
		return err
	}
	if spec.SessionID != sessionID {
		return fmt.Errorf("--session %q does not match the id derived from --keys %s (%q) — use the session id printed by gate-mission", sessionID, strings.Join(keys, ","), spec.SessionID)
	}

	states, err := gaterun.LoadSessionStates(absRoot, run)
	if err != nil {
		return err
	}
	if spec.Kind != gaterun.SessionKindCross {
		covered, _, cerr := gaterun.CoverageProgress(run, states)
		if cerr != nil {
			return cerr
		}
		for _, key := range keys {
			if owner, ok := covered[key]; ok {
				return fmt.Errorf("coverage key %q is already covered by accepted session %q — an accepted judgment cannot be replaced; plan a new run if it must change", key, owner)
			}
		}
	}

	state, err := gaterun.LoadSessionState(absRoot, run, sessionID)
	if err != nil {
		return err
	}
	if state.Status == gaterun.SessionAccepted {
		return fmt.Errorf("session %q is already accepted (terminal) — it cannot be replaced; plan a new run if the result must change", sessionID)
	}
	if state.Status == gaterun.SessionNotRequired {
		return fmt.Errorf("session %q is not_required (terminal) — it accepts no submissions; plan a new run if the result must change", sessionID)
	}
	for _, dep := range spec.DependsOn {
		depState, derr := gaterun.LoadSessionState(absRoot, run, dep)
		if derr != nil {
			return derr
		}
		if depState.Status != gaterun.SessionAccepted && depState.Status != gaterun.SessionNotRequired {
			return fmt.Errorf("gate-submit rejected: dependency session %q is %s — submit it first: `specflowctl gate-submit --run %s --session %s --keys <keys> --report PATH`", dep, depState.Status, run.RunID, dep)
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
		state.SessionID = sessionID
		state.Keys = append([]string(nil), keys...)
		state.Status = gaterun.SessionRejected
		state.Attempts = append(state.Attempts, gaterun.Attempt{
			Attempt:         attempt,
			SubmittedAt:     now,
			Status:          gaterun.SessionRejected,
			RejectionReason: reason,
			ResultDigest:    digest,
		})
		if saveErr := gaterun.SaveSessionState(absRoot, run, state); saveErr != nil {
			return saveErr
		}
		return fmt.Errorf("gate-submit rejected: %s (attempt %d recorded — fix the report and re-submit)", reason, attempt)
	}

	if strings.TrimSpace(report) == "" {
		return reject("empty report")
	}
	parsed, perr := parseSessionReport(run, spec, report)
	if perr != nil {
		return reject(perr.Error())
	}
	normalizeDeclaredPaths(absRoot, parsed)
	if derr := validateSessionDeclarations(absRoot, run, spec, parsed); derr != nil {
		return reject(derr.Error())
	}
	if derr := validateReviewDependencies(absRoot, run, spec, parsed); derr != nil {
		return reject(derr.Error())
	}
	if derr := validateSessionSemantics(absRoot, run, spec, parsed, report); derr != nil {
		return reject(derr.Error())
	}

	state.SessionID = sessionID
	state.Keys = append([]string{}, keys...)
	state.Status = gaterun.SessionAccepted
	state.Report = report
	state.Result = sessionResult(spec, parsed, digest)
	semanticData, _ := json.Marshal(state.Result)
	state.SemanticDigest = judgments.Digest(semanticData)
	state.ConsumedResultDigests = consumedResultDigests(absRoot, run, spec, states)
	state.Attempts = append(state.Attempts, gaterun.Attempt{
		Attempt:      attempt,
		SubmittedAt:  now,
		Status:       gaterun.SessionAccepted,
		ResultDigest: digest,
	})
	if err := gaterun.PublishPublic(absRoot, run, spec, state); err != nil {
		return reject(err.Error())
	}
	if err := gaterun.SaveSessionState(absRoot, run, state); err != nil {
		return err
	}

	states = append(states, state)
	_, uncovered, cerr := gaterun.CoverageProgress(run, states)
	if cerr != nil {
		return cerr
	}
	accepted, pending, rejected := sessionCounts(states)
	fmt.Fprintf(stdout, "Session accepted: %s (keys %s, attempt %d, digest %s)\n", sessionID, strings.Join(keys, ", "), attempt, digest)
	fmt.Fprintf(stdout, "Coverage: %d/%d covered · sessions: %d accepted, %d pending, %d rejected\n", len(run.Coverage)-len(uncovered), len(run.Coverage), accepted, pending, rejected)
	fmt.Fprintf(stdout, "Next: %s\n", gateNextStep(run, states, uncovered))
	return nil
}

// normalizeDeclaredPaths rewrites every parsed declaration path — dependency
// scopes and ownership evidence — to its canonical repo-relative spelling
// before validation and persistence, so the run snapshot, the stored session
// result, and the finalize-time cache assembly all speak one path form
// (see framework/validation_cache.md §Format: `./` prefixes, absolute paths,
// and platform separators in a recorded path are equivalent).
func normalizeDeclaredPaths(absRoot string, parsed *parsedReport) {
	for i := range parsed.Scopes {
		parsed.Scopes[i].Path = gaterun.CanonicalDeclPath(absRoot, parsed.Scopes[i].Path)
	}
	for i := range parsed.Ownerships {
		parsed.Ownerships[i].EvidencePath = gaterun.CanonicalDeclPath(absRoot, parsed.Ownerships[i].EvidencePath)
	}
}

// validateSessionDeclarations validates every dependency-scope line's path
// form, snapshot membership, and declaration parseability.
func validateSessionDeclarations(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec, parsed *parsedReport) error {
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
		protected := (spec.Kind == gaterun.SessionKindPreserve || spec.Kind == gaterun.SessionKindCross) && run.IsProtectedStableInput(absRoot, kp.path)
		if err := validationcache.ValidateEntryPathForm(absRoot, run.TargetKind, run.TargetName, kp.path); err != nil && !protected {
			return fmt.Errorf("check %q declaration %s: %w", kp.key, kp.path, err)
		}
		if !run.SessionAllowsDeclaration(absRoot, spec, kp.path) {
			return fmt.Errorf("check %q declaration %q is not part of session %q's read refs — add evidence with --input at gate-plan time or use the session that owns this input", kp.key, kp.path, spec.SessionID)
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

func validateSessionSemantics(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec, parsed *parsedReport, report string) error {
	if len(parsed.Ownerships) > 0 && spec.Kind != gaterun.SessionKindCross {
		return errors.New("ownership records belong to the final synthesis")
	}
	switch spec.Kind {
	case gaterun.SessionKindItem, gaterun.SessionKindPreserve:
		// A verify session authors its own alignment verdicts and, for each
		// mismatch, the finding (with severity and evidence). The parser
		// composes those findings mechanically; nothing extra is required
		// here.
	case gaterun.SessionKindChecks:
		fails := verdictCount(parsed.Verdicts, "FAIL")
		blocking := blockingFindingCount(parsed.Findings)
		if fails > 0 && blocking == 0 {
			return errors.New("a validate FAIL verdict requires at least one P0/P1 finding in the same session")
		}
		if fails == 0 && blocking > 0 {
			return errors.New("a validate session with P0/P1 findings must report at least one FAIL verdict")
		}
		for key, verdict := range parsed.Verdicts {
			covered := false
			for _, finding := range parsed.Findings {
				if (finding.Severity == "P0" || finding.Severity == "P1") && findingKeySet(finding)[key] {
					covered = true
				}
			}
			if (verdict == "FAIL") != covered {
				return fmt.Errorf("validate check %q verdict must match its own P0/P1 findings", key)
			}
		}
	case gaterun.SessionKindDesign, gaterun.SessionKindArchitecture:
		// The conclusion mapping is mechanical for its P0/P1 half: the
		// conclusion, the gate_findings entries, and the report's P0/P1
		// findings must agree (see framework/unit_verify_checklist.md
		// §Output Format). The P2/P3/none distinction stays
		// author-declared — it has no machine carrier.
		for key, verdict := range parsed.Verdicts {
			var findings []gaterun.Finding
			for _, finding := range parsed.Findings {
				if finding.SourceKey == key {
					findings = append(findings, finding)
				}
			}
			unacceptable := verdict == "unacceptable"
			blocking := blockingFindingCount(findings) > 0
			gateBlocking := gateFindingsDeclareBlocking(parsed.FileGateFindings[key])
			if unacceptable != blocking || unacceptable != gateBlocking {
				return fmt.Errorf("quality file %q conclusion, gate_findings, and its own P0/P1 findings must agree", key)
			}
		}
	case gaterun.SessionKindCross:
		return validateCrossSynthesis(absRoot, run, spec, parsed)
	}
	return nil
}

func validateCrossSynthesis(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec, parsed *parsedReport) error {
	expectedStatus := map[string]bool{gaterun.CrossKey: true}
	for _, name := range run.Relationships {
		expectedStatus[gaterun.RelationshipKey(name)] = true
	}
	for _, ck := range run.Coverage {
		for _, key := range run.ReportKeys(ck) {
			expectedStatus[key] = true
		}
	}
	for _, key := range run.CarriedKeys {
		expectedStatus[key] = true
	}
	states, err := gaterun.LoadSessionStates(absRoot, run)
	if err != nil {
		return err
	}
	var inputFindings []gaterun.Finding
	var protectedMismatches, protectedIndeterminate []string
	for _, state := range states {
		if state.Result == nil {
			continue
		}
		for key, verdict := range state.Result.Verdicts {
			if !strings.HasPrefix(key, "preserve:") || verdict == "ALIGNED" {
				continue
			}
			if verdict == "MISMATCH" {
				protectedMismatches = append(protectedMismatches, key)
				continue
			}
			protectedIndeterminate = append(protectedIndeterminate, key)
		}
	}
	for _, state := range states {
		if state.SessionID == gaterun.CrossKey || state.Status != gaterun.SessionAccepted || state.Result == nil {
			continue
		}
		inputFindings = append(inputFindings, resultFindings(state.Result)...)
	}
	for _, result := range run.CarriedResults {
		inputFindings = append(inputFindings, resultFindings(&result)...)
	}
	for _, deferred := range run.DeferredFindings {
		inputFindings = append(inputFindings, deferred.Finding)
	}

	for _, disposition := range parsed.Dispositions {
		if disposition.Action != "retained" {
			for _, f := range inputFindings {
				if f.ID == disposition.FindingID && strings.HasPrefix(f.SourceKey, "preserve:") {
					return fmt.Errorf("protected requirement finding %s cannot be suppressed or merged", f.ID)
				}
			}
		}
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
	retained, err = validateOwnerships(absRoot, run, spec, parsed, retained)
	if err != nil {
		return err
	}
	// A non-ALIGNED protected requirement must either fail its key or — for a
	// MISMATCH only — be carried by a finding deferred to the protected unit
	// (peer-owned stable-record drift). CANNOT_DETERMINE is never routable: an
	// indeterminate requirement stays blocking.
	deferredKeys := map[string]bool{}
	for _, finding := range retained {
		if gateDriving(finding, run.TargetName) {
			continue
		}
		for key := range findingKeySet(finding) {
			deferredKeys[key] = true
		}
	}
	for _, key := range protectedIndeterminate {
		if parsed.EffectiveStatus[key] != "fail" {
			return fmt.Errorf("protected requirement %s cannot be cleared by synthesis", key)
		}
	}
	for _, key := range protectedMismatches {
		if parsed.EffectiveStatus[key] != "fail" && !deferredKeys[key] {
			return fmt.Errorf("protected requirement %s cannot be cleared by synthesis", key)
		}
	}
	if err := validateCrossItemFindingLinks(run.Gate, parsed, retained); err != nil {
		return err
	}
	qualityKeys := map[string]bool{}
	for key := range expectedStatus {
		if run.Gate == gaterun.GateVerify && (strings.HasPrefix(key, "design:") || strings.HasPrefix(key, "architecture:")) {
			qualityKeys[key] = true
			conclusion, ok := parsed.QualityConclusions[key]
			if !ok {
				return fmt.Errorf("cross report is missing Quality conclusion for %q", key)
			}
			var findings []gaterun.Finding
			for _, finding := range retained {
				if findingKeySet(finding)[key] && gateDriving(finding, run.TargetName) {
					findings = append(findings, finding)
				}
			}
			if (conclusion == "unacceptable") != (blockingFindingCount(findings) > 0) {
				return fmt.Errorf("Quality conclusion for %q must agree with its finalized P0/P1 findings", key)
			}
		}
	}
	for key := range parsed.QualityConclusions {
		if !qualityKeys[key] {
			return fmt.Errorf("cross report declares unexpected Quality conclusion for %q", key)
		}
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
	for name, verdict := range parsed.CrossItems {
		key := gaterun.RelationshipKey(name)
		if (verdict == "FAIL") != (wantStatus[key] == "fail") {
			return fmt.Errorf("relationship %q verdict must match its retained findings", name)
		}
	}

	crossVerdict := parsed.Verdicts[gaterun.CrossKey]
	wantCrossStatus := "pass"
	newIDs := map[string]bool{}
	for _, id := range parsed.CrossItemFindings {
		newIDs[id] = true
	}
	var canonicalCrossFindings []gaterun.Finding
	for _, finding := range retained {
		if newIDs[finding.ID] && gateDriving(finding, run.TargetName) {
			canonicalCrossFindings = append(canonicalCrossFindings, finding)
		}
	}
	blockingCross := blockingFindingCount(canonicalCrossFindings) > 0
	if crossVerdict == "FAIL" {
		wantCrossStatus = "fail"
		if !blockingCross {
			return errors.New("Cross-check FAIL requires at least one new P0/P1 cross finding owned by this unit or unassigned")
		}
	}
	if crossVerdict == "PASS" && blockingCross {
		return errors.New("Cross-check PASS contradicts a retained gate-driving P0/P1 cross finding")
	}
	if parsed.EffectiveStatus[gaterun.CrossKey] != wantCrossStatus {
		return fmt.Errorf("cross verdict %s requires `Effective status: cross = %s`", crossVerdict, wantCrossStatus)
	}

	return nil
}

func validateCrossItemFindingLinks(gate string, parsed *parsedReport, retained []gaterun.Finding) error {
	newIDs := make(map[string]bool, len(parsed.Findings))
	for _, finding := range parsed.Findings {
		newIDs[finding.ID] = true
	}
	retainedNew := make(map[string]gaterun.Finding)
	linked := map[string]bool{}
	for _, disposition := range parsed.Dispositions {
		if disposition.Action == "merged" {
			linked[disposition.TargetID] = true
		}
	}
	for _, finding := range retained {
		if newIDs[finding.ID] {
			retainedNew[finding.ID] = finding
		}
	}
	for item, id := range parsed.CrossItemFindings {
		if !newIDs[id] {
			return fmt.Errorf("Cross item %q refers to %q, which is not a finding created by this cross report", item, id)
		}
		finding, ok := retainedNew[id]
		if !ok {
			return fmt.Errorf("Cross item %q refers to cross finding %q, which is not retained", item, id)
		}
		if !findingKeySet(finding)[gaterun.RelationshipKey(item)] {
			return fmt.Errorf("Cross item %q finding must affect %s", item, gaterun.RelationshipKey(item))
		}
		linked[id] = true
		if gate == gaterun.GateValidate && finding.Severity != "P0" && finding.Severity != "P1" {
			return fmt.Errorf("validate Cross item %q refers to %s finding %q; validate findings must be P0 or P1", item, finding.Severity, id)
		}
	}
	for id := range newIDs {
		if !linked[id] {
			return fmt.Errorf("new synthesis finding %q must explain a failed assigned relationship; local checks must not be repeated", id)
		}
	}
	return nil
}

// sessionDeclaresPath reports whether the report carries a Dependency scope
// declaration for path. A cross session's evidence must be covered by a `cross`
// scope line; other kinds match any of their own declarations.
func sessionDeclaresPath(spec *gaterun.SessionSpec, parsed *parsedReport, path string) bool {
	for _, scope := range parsed.Scopes {
		if scope.Path != path {
			continue
		}
		if spec.Kind != gaterun.SessionKindCross || scope.Key == gaterun.CrossKey {
			return true
		}
	}
	return false
}

// validateOwnerships applies the final synthesis's ownership records to the
// terminal retained findings. Two classes are deferrable: quality-lens
// findings whose recorded ownership belongs to another unit, and protected
// stable-record drift — a preserve finding whose every key is a preserve key
// of the finding's protected unit, routed back to that unit. Each record must
// cite evidence inside the cross session's read refs, covered by a `cross`
// dependency-scope declaration, and name a unit that exists in the repository
// — a deferral to a nonexistent unit would route the finding nowhere, so it
// fails closed here.
func validateOwnerships(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec, parsed *parsedReport, findings []gaterun.Finding) ([]gaterun.Finding, error) {
	if len(parsed.Ownerships) > 0 && run.Gate != gaterun.GateVerify {
		return nil, fmt.Errorf("ownership records belong to unit verify — the %s gate has no ownership dimension", run.Gate)
	}
	canonical, err := applyOwnerships(findings, parsed.Ownerships)
	if err != nil {
		return nil, err
	}
	findingByID := make(map[string]gaterun.Finding, len(canonical))
	for _, finding := range canonical {
		findingByID[finding.ID] = finding
	}
	for _, ownership := range parsed.Ownerships {
		finding := findingByID[ownership.FindingID]
		keys := findingKeySet(finding)
		if len(keys) == 0 {
			return nil, fmt.Errorf("ownership record for finding %q has no coverage key", finding.ID)
		}
		protectedUnit, err := ownershipRoutingTarget(run, finding.ID, keys)
		if err != nil {
			return nil, err
		}
		if protectedUnit != "" && protectedUnit != ownership.OwnerUnit {
			return nil, fmt.Errorf("ownership record for finding %q routes a protected requirement of %q; stable-record drift must route to the protected unit (got owner %q)", ownership.FindingID, protectedUnit, ownership.OwnerUnit)
		}
		if !run.SessionAllowsDeclaration(absRoot, spec, ownership.EvidencePath) {
			return nil, fmt.Errorf("ownership record for finding %q cites %q outside session %q's read refs", ownership.FindingID, ownership.EvidencePath, spec.SessionID)
		}
		if !sessionDeclaresPath(spec, parsed, ownership.EvidencePath) {
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

// ownershipKeyScope is the shared explanation for findings whose keys cannot
// be deferred: only quality findings and one protected unit's own stable-record
// drift are routable.
const ownershipKeyScope = "ownership records are quality-lens-only, except a protected unit's own stable-record drift"

// ownershipRoutingTarget classifies a finding's logical keys for ownership
// routing. Quality findings (every key a quality-lens key) return ""; a
// protected stable-record drift returns the protected unit — every key is a
// preserve key of that one unit. Any other combination (item keys, mixed
// quality and protected keys, several protected units) is not deferrable.
func ownershipRoutingTarget(run *gaterun.Run, findingID string, keys map[string]bool) (string, error) {
	qualityCount, preserveCount := 0, 0
	protectedUnit := ""
	for key := range keys {
		if run.LensForReportKey(key) == gaterun.LensQuality {
			qualityCount++
			continue
		}
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 || parts[0] != gaterun.SessionKindPreserve || parts[1] == "" || parts[2] == "" {
			return "", fmt.Errorf("%s — finding %q affects non-quality key %q", ownershipKeyScope, findingID, key)
		}
		preserveCount++
		if protectedUnit == "" {
			protectedUnit = parts[1]
			continue
		}
		if protectedUnit != parts[1] {
			return "", fmt.Errorf("ownership record for finding %q affects protected requirements of different units %q and %q — a stable-record deferral names one protected unit", findingID, protectedUnit, parts[1])
		}
	}
	switch {
	case qualityCount > 0 && preserveCount > 0:
		return "", fmt.Errorf("%s — finding %q mixes quality and protected keys", ownershipKeyScope, findingID)
	case preserveCount > 0:
		return protectedUnit, nil
	default:
		return "", nil
	}
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

func sessionResult(spec *gaterun.SessionSpec, parsed *parsedReport, digest string) *gaterun.SessionResult {
	scopes := make([]gaterun.Scope, 0, len(parsed.Scopes))
	for _, scope := range parsed.Scopes {
		scopes = append(scopes, gaterun.Scope{Key: scope.Key, Path: scope.Path, Declaration: scope.Declaration})
	}
	return &gaterun.SessionResult{
		SessionID:               spec.SessionID,
		Kind:                    spec.Kind,
		Verdicts:                parsed.Verdicts,
		Scopes:                  scopes,
		Findings:                parsed.Findings,
		Observations:            parsed.Observations,
		ObservationDispositions: parsed.ObservationDispositions,
		EffectiveStatus:         parsed.EffectiveStatus,
		QualityConclusions:      parsed.QualityConclusions,
		Dispositions:            parsed.Dispositions,
		Ownerships:              parsed.Ownerships,
		Analysis:                parsed.Analysis,
		ReportDigest:            digest,
	}
}

// consumedResultDigests records the accepted dependency result digests a
// session consumes: the final synthesis binds every accepted non-final
// session's digest plus the carried results.
func consumedResultDigests(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec, states []*gaterun.SessionState) map[string]string {
	if spec.Kind != gaterun.SessionKindCross {
		out := map[string]string{}
		for _, dep := range spec.DependsOn {
			state, err := gaterun.LoadSessionState(absRoot, run, dep)
			if err == nil && state.Result != nil {
				out[dep] = state.Result.ReportDigest
			}
		}
		return out
	}
	out := map[string]string{}
	for _, dep := range spec.DependsOn {
		depState, err := gaterun.LoadSessionState(absRoot, run, dep)
		if err != nil || depState.Status == gaterun.SessionNotRequired || depState.Result == nil {
			continue
		}
		out[dep] = depState.Result.ReportDigest
	}
	for _, state := range states {
		if state.SessionID == gaterun.CrossKey || state.Status != gaterun.SessionAccepted || state.Result == nil {
			continue
		}
		out[state.SessionID] = state.Result.ReportDigest
	}
	for i := range run.CarriedResults {
		out[run.CarriedResults[i].SessionID] = run.CarriedResults[i].ReportDigest
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// reportDigest is the normalized-text sha256 of a session report.
func reportDigest(report string) string {
	sum := sha256.Sum256([]byte(specpaths.NormalizeText(report)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeGateSubmitUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl gate-submit --run RUN_ID --session SESSION_ID --keys K1,K2 --report PATH [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Records one reviewer session's report after mechanical validation: the run is")
	fmt.Fprintln(w, "open, the assigned keys belong to the run's coverage set and are not already")
	fmt.Fprintln(w, "covered by an accepted session, the session id matches the keys, its")
	fmt.Fprintln(w, "dependencies are resolved, and the report is structurally complete — every")
	fmt.Fprintln(w, "assigned check key has exactly one verdict line with an allowed token and")
	fmt.Fprintln(w, "evidence basis, gate-specific required fields are present, and every check key")
	fmt.Fprintln(w, "declares at least one Dependency scope line inside the session read refs.")
	fmt.Fprintln(w, "The optional final synthesis is submitted with --session cross --keys cross; it")
	fmt.Fprintln(w, "must dispose every input finding, publish every effective logical status, and map")
	fmt.Fprintln(w, "each new cross finding to the logical keys it makes fail. A valid report is")
	fmt.Fprintln(w, "recorded as accepted (terminal); an invalid one as rejected with the reason — it")
	fmt.Fprintln(w, "can be re-submitted, and every attempt is kept.")
	fmt.Fprintln(w, "See framework/verification_scope.md §Coverage model.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --run RUN_ID     gate run id printed by gate-plan (required)")
	fmt.Fprintln(w, "  --session ID     session id printed by gate-mission (required)")
	fmt.Fprintln(w, "  --keys K1,K2     the coverage keys assigned to this session (required)")
	fmt.Fprintln(w, "  --report PATH    file holding the session report (required)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

func validateReviewDependencies(root string, run *gaterun.Run, spec *gaterun.SessionSpec, parsed *parsedReport) error {
	if run.Gate != gaterun.GateVerify {
		return nil
	}
	if spec.Kind != gaterun.SessionKindCode && spec.Kind != gaterun.SessionKindCross {
		for _, key := range spec.CheckKeys {
			ck := run.CoverageByKey(key)
			layer := run.Target
			if ck.Kind == gaterun.SessionKindPreserve {
				layer = gaterun.TargetStable
			}
			main := "docs/specs/units/" + layer + "/unit_" + ck.Unit + ".md"
			found := false
			for _, scope := range parsed.Scopes {
				if scope.Key == key && scope.Path == main {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("check %s must declare its unit spec evidence from %s", key, main)
			}
		}
	}
	if spec.Kind == gaterun.SessionKindCode {
		for _, key := range spec.CheckKeys {
			ck := run.CoverageByKey(key)
			for _, p := range ck.ReadRefs {
				found := false
				for _, scope := range parsed.Scopes {
					if scope.Key == key && scope.Path == p && scope.Declaration == "all" {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("public check %s must declare whole-file evidence for %s", key, p)
				}
			}
		}
	}
	if spec.Kind == gaterun.SessionKindPreserve {
		for i := range parsed.Findings {
			parsed.Findings[i] = parsed.Findings[i].WithMinimumSeverity("P1")
		}
	}
	if spec.Kind == gaterun.SessionKindDesign {
		observations := map[string]gaterun.Finding{}
		results, err := gaterun.PublicResultsForDesign(root, run, spec)
		if err != nil {
			return err
		}
		for key, result := range results {
			for _, f := range result.Observations {
				f.SourceKey = key
				observations[f.ID] = f
			}
		}
		seen := map[string]bool{}
		for _, d := range parsed.ObservationDispositions {
			f, ok := observations[d.FindingID]
			if !ok || seen[d.FindingID] || strings.TrimSpace(d.Reason) == "" {
				return fmt.Errorf("invalid or duplicate public observation disposition %s", d.FindingID)
			}
			seen[d.FindingID] = true
			if d.Action == "retained" {
				f.ID = fmt.Sprintf("%s/%s/F%d", run.RunID, spec.SessionID, len(parsed.Findings)+1)
				f.Detail += "\nPublic observation: " + d.FindingID + "\nUnit design reason: " + d.Reason
				parsed.Findings = append(parsed.Findings, f)
			}
		}
		if len(seen) != len(observations) {
			return fmt.Errorf("design report must dispose every public observation with unit-specific evidence")
		}
	}
	return nil
}

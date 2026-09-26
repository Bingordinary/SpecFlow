package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// ------------------------------------------------------------
// Fixtures and helpers
// ------------------------------------------------------------

// grWriteSpec writes a candidate unit spec with frontmatter and one
// acceptance-bearing section.
func grWriteSpec(t *testing.T, repoRoot, name string) string {
	t.Helper()
	return grWriteSpecWithRefs(t, repoRoot, name, "none", "none")
}

// grWriteSpecWithRefs writes a candidate unit spec with the given
// unit_refs / rule_refs.
func grWriteSpecWithRefs(t *testing.T, repoRoot, name, unitRefs, ruleRefs string) string {
	t.Helper()
	return grWriteSpecItems(t, repoRoot, name, unitRefs, ruleRefs, []string{name + ".core"})
}

// grWriteSpecItems writes a candidate unit spec with the given acceptance
// item ids (each with implementation_surface src).
func grWriteSpecItems(t *testing.T, repoRoot, name, unitRefs, ruleRefs string, items []string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "unit_"+name+".md")
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\nversion: 0.1.0\nunit_refs: %s\nrule_refs: %s\n---\n\n# %s\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n", name, unitRefs, ruleRefs, name)
	for _, item := range items {
		fmt.Fprintf(&b, "  - id: %s\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n", item)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// grWriteSpecSurface writes a candidate unit spec whose single acceptance
// item declares the given implementation_surface (plus optional extra item
// content).
func grWriteSpecSurface(t *testing.T, repoRoot, name, surface, extra string) string {
	t.Helper()
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n" +
		"  - id: " + name + ".core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: " + surface + "\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n" + extra
	return grWriteFile(t, repoRoot, "docs/specs/units/candidate/unit_"+name+".md", content)
}

// grWriteFile writes one repo file.
func grWriteFile(t *testing.T, repoRoot, rel, content string) string {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// grPlan runs gate-plan and returns the run id.
func grPlan(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	runID, err := grPlanRaw(repoRoot, args...)
	if err != nil {
		t.Fatalf("gate-plan failed: %v", err)
	}
	return runID
}

// grPlanRaw runs gate-plan and returns the run id or an error.
func grPlanRaw(repoRoot string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	full := append([]string{"--repo-root", repoRoot}, args...)
	if err := runGatePlan(full, &stdout, &stderr); err != nil {
		return "", fmt.Errorf("%w (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if after, ok := strings.CutPrefix(line, "Gate run planned: "); ok {
			return strings.TrimSpace(after), nil
		}
	}
	return "", fmt.Errorf("no run id in gate-plan output:\n%s", stdout.String())
}

// grSubmit writes a report to a temp file and runs gate-submit.
func grSubmit(t *testing.T, repoRoot, runID, packetID, report string) (string, error) {
	t.Helper()
	run, loadErr := gaterun.Load(repoRoot, runID)
	if loadErr == nil {
		if run.PacketByID(packetID) == nil && run.PacketByID("detect:"+packetID) != nil {
			packetID = "detect:" + packetID
		}
		if packetID == "cross" && !strings.Contains(report, "Effective status:") {
			report = grCompleteCrossReport(t, repoRoot, run, report)
		}
	}
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID, "--packet", packetID, "--report", path}, &stdout, &stderr)
	if err != nil && stderr.Len() > 0 {
		return stdout.String(), fmt.Errorf("%w (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), err
}

// grSubmitOK fails the test when gate-submit errors.
func grSubmitOK(t *testing.T, repoRoot, runID, packetID, report string) string {
	t.Helper()
	out, err := grSubmit(t, repoRoot, runID, packetID, report)
	if err != nil {
		t.Fatalf("gate-submit %s failed: %v", packetID, err)
	}
	return out
}

// grFinalize runs gate-finalize and returns its stdout and error.
func grFinalize(t *testing.T, repoRoot, runID string, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	var supported []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--result", "--p0-count", "--p1-count", "--p2-count", "--p3-count":
			i++
		default:
			supported = append(supported, args[i])
		}
	}
	full := append([]string{"--repo-root", repoRoot, "--run", runID}, supported...)
	err := runGateFinalize(full, &stdout, &stderr)
	if err != nil && stderr.Len() > 0 {
		return stdout.String(), fmt.Errorf("%w (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), err
}

// grFinalizeOK fails the test when gate-finalize errors.
func grFinalizeOK(t *testing.T, repoRoot, runID string, args ...string) string {
	t.Helper()
	out, err := grFinalize(t, repoRoot, runID, args...)
	if err != nil {
		t.Fatalf("gate-finalize failed: %v", err)
	}
	return out
}

var grCheckNames = map[string]string{
	"1": "Structural integrity",
	"2": "Design soundness",
	"3": "Scope integrity",
	"4": "Evidence-driven vs design-driven consistency",
	"5": "Acceptance coverage & correctness",
	"6": "Affects-source validity",
	"7": "Cross-unit consistency",
	"8": "Constraint alignment",
}

// grValidateReport builds a validate packet report: PASS verdicts for the
// given checks plus one scope line per entry of scopes (check key -> raw
// "{file}: {declaration}" strings).
func grValidateReport(checks []string, scopes map[string][]string) string {
	var b strings.Builder
	for _, c := range checks {
		fmt.Fprintf(&b, "%s. %s: PASS — checked\n", c, grCheckNames[c])
		if c == "5" {
			for _, subcheck := range []string{"5a", "5b", "5c", "5d", "5e", "5f", "5g", "5h", "5i"} {
				fmt.Fprintf(&b, "  %s. Required acceptance sub-check: PASS — checked\n", subcheck)
			}
		}
	}
	b.WriteString("\n")
	for _, c := range checks {
		for _, line := range scopes[c] {
			fmt.Fprintf(&b, "check-%s: %s\n", c, line)
		}
	}
	return b.String()
}

// grCrossReport builds a cross-check packet report.
func grCrossReport(scopeFile, scopeDecl string) string {
	return fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: %s\n", scopeFile, scopeDecl)
}

// grRunFindingID renders the run-scoped finding id the parser assigns to the
// n-th finding of a packet report: {run_id}/{packet_id}/F{n}. The prefix is
// the run id printed in the packet context (gate-packet's `Run:` line), so
// report authors copy it instead of inventing ids.
func grRunFindingID(runID, packetID string, n int) string {
	return fmt.Sprintf("%s/%s/F%d", runID, packetID, n)
}

func grCompleteCrossReport(t *testing.T, repoRoot string, run *gaterun.Run, report string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(strings.TrimRight(report, "\n"))
	b.WriteString("\n")
	failedKeys := map[string]bool{}
	var retainedFindings []gaterun.Finding
	for _, packet := range run.Packets {
		if packet.Kind == gaterun.PacketKindCross {
			continue
		}
		state, err := gaterun.LoadPacketState(repoRoot, run, packet.PacketID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Result == nil {
			continue
		}
		for _, finding := range resultFindings(state.Result) {
			retainedFindings = append(retainedFindings, finding)
			for key := range findingKeySet(finding) {
				failedKeys[key] = true
			}
		}
	}
	for _, result := range run.CarriedResults {
		for _, finding := range resultFindings(&result) {
			retainedFindings = append(retainedFindings, finding)
			for key := range findingKeySet(finding) {
				failedKeys[key] = true
			}
		}
	}
	seen := map[string]bool{}
	for _, packet := range run.Packets {
		if packet.Kind == gaterun.PacketKindCross {
			continue
		}
		state, err := gaterun.LoadPacketState(repoRoot, run, packet.PacketID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Result != nil {
			for _, finding := range state.Result.Findings {
				fmt.Fprintf(&b, "Finding disposition: %s = retained\n", finding.ID)
			}
		}
		if packet.Kind == gaterun.PacketKindAnalysis {
			continue
		}
		for _, key := range packet.CheckKeys {
			if seen[key] {
				continue
			}
			seen[key] = true
			status := "pass"
			if failedKeys[key] {
				status = "fail"
			}
			fmt.Fprintf(&b, "Effective status: %s = %s\n", key, status)
		}
	}
	for _, result := range run.CarriedResults {
		for key, status := range result.EffectiveStatus {
			if !seen[key] {
				fmt.Fprintf(&b, "Effective status: %s = %s\n", key, status)
				seen[key] = true
			}
		}
		for _, finding := range result.Findings {
			fmt.Fprintf(&b, "Finding disposition: %s = retained\n", finding.ID)
		}
	}
	crossScopes, err := extractScopes(run, run.PacketByID(gaterun.CrossKey), report, map[int]bool{})
	if err != nil || len(crossScopes) == 0 {
		t.Fatalf("cross fixture has no usable evidence scope: %v", err)
	}
	for _, finding := range retainedFindings {
		fmt.Fprintf(&b, "Severity confirmation: %s = confirmed %s — evidence: %s; reason: fixture confirms the impact boundary\n", finding.ID, finding.Severity, crossScopes[0].Path)
	}
	crossStatus := "pass"
	if strings.Contains(report, "Cross-check: FAIL") {
		crossStatus = "fail"
	}
	fmt.Fprintf(&b, "Effective status: cross = %s\n", crossStatus)
	return b.String()
}

// grSubmitValidatePackets submits the full 5-packet validate plan for a
// spec. extraScopes are appended to the structural packet's scope lines
// (e.g. appendix declarations).
func grSubmitValidatePackets(t *testing.T, repoRoot, runID, specPath string, extraScopes ...string) {
	t.Helper()
	desc := specPath + ": Description"
	accept := specPath + ": Testability / Acceptance Criteria"
	front := specPath + ": frontmatter"
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": append([]string{front, desc}, extraScopes...),
		"3": {accept},
		"6": {accept},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {desc},
		"4": {desc},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {accept},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {desc},
		"8": {desc},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(specPath, "Description"))
}

// grVerifyItemReport builds a verify item packet report with an ALIGNED
// verdict and spec + code scope lines.
func grVerifyItemReport(item, specPath, codeFile string) string {
	return fmt.Sprintf("- %s: ALIGNED — %s:1\n  evidence: %s:1 implements the declared behavior\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped — no test files in this fixture\n\n%s: %s: Testability / Acceptance Criteria\n%s: %s: all\n", item, codeFile, codeFile, item, specPath, item, codeFile)
}

func grVerifyItemBody(item, verdict, evidence string) string {
	return fmt.Sprintf("- %s: %s — %s\n  evidence: %s\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped — no test files in this fixture\n\n", item, verdict, evidence, evidence)
}

func grVerifyAnalysisReport(item, specPath, codeFile, severity string) string {
	return fmt.Sprintf("Item: %s\nProblem: the declared behavior disagrees with the implementation for %s\nEvidence:\n  - spec: declared behavior — %s: acceptance item %s\n  - code: implemented behavior — %s:1\nImpact: the acceptance surface cannot be satisfied as declared\nFix: align the implementation with the declared behavior\nRoot cause: incomplete\nSuggested direction: code_gap\nSeverity: %s\nConfidence: high\n\n%s: %s: acceptance_item:%s\n%s: %s: all\n",
		item, item, specPath, item, codeFile, severity, item, specPath, item, item, codeFile)
}

func grReviewArchitecture(conclusion string) string {
	gateFindings := "none"
	if strings.HasPrefix(conclusion, "unacceptable") {
		gateFindings = "[P1] gate-level architectural boundary defect"
	}
	return fmt.Sprintf("Architecture assessment:\n  conclusion: %s\n  module_boundaries: sound — reviewed module boundary\n  responsibility_organization: sound — reviewed responsibility placement\n  dependency_clarity: sound — reviewed dependency direction\n  abstraction_level: sound — reviewed abstraction level\n  extension_landing_points: sound — reviewed extension seams\n  engineering_patterns: sound — reviewed engineering patterns\n  gate_findings: %s\n\nSuppressed by spec (0):\n", conclusion, gateFindings)
}

// grReviewReport builds a complete review file packet report with an
// acceptable conclusion and spec + file scope lines.
func grReviewReport(file, specPath string) string {
	return fmt.Sprintf("%s\n%s: %s: Description\n%s: %s: all\n", grReviewArchitecture("acceptable"), file, specPath, file, file)
}

func grReadCache(t *testing.T, repoRoot, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func grReadJudgmentBaseline(t *testing.T, repoRoot, targetKind, targetName, gate string) gaterun.JudgmentBaseline {
	t.Helper()
	baseline, err := validationcache.ReadGateBaseline(repoRoot, targetKind, targetName, gate)
	if err != nil {
		t.Fatal(err)
	}
	var state gaterun.JudgmentBaseline
	if err := json.Unmarshal([]byte(baseline.Judgments), &state); err != nil {
		t.Fatalf("decode judgment baseline: %v", err)
	}
	return state
}

func grContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func grRefByName(run *gaterun.Run, name string) (gaterun.Ref, bool) {
	for _, ref := range run.Refs {
		if ref.Ref == name {
			return ref, true
		}
	}
	return gaterun.Ref{}, false
}

// ------------------------------------------------------------
// Plan generation
// ------------------------------------------------------------

func TestGatePlanValidateUnitPlan(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != "full" || len(run.Packets) != 5 {
		t.Fatalf("expected a 5-packet full plan, got mode %q with %d packets", run.Mode, len(run.Packets))
	}
	wantIDs := []string{"structural", "design", "acceptance", "dependencies", "cross"}
	for i, id := range wantIDs {
		if run.Packets[i].PacketID != id {
			t.Fatalf("packet %d: expected %q, got %q", i, id, run.Packets[i].PacketID)
		}
	}
	if got := strings.Join(run.Packets[0].CheckKeys, ","); got != "1,3,6" {
		t.Fatalf("expected structural checks 1,3,6, got %s", got)
	}
	cross := run.Packets[4]
	if strings.Join(cross.DependsOn, ",") != "structural,design,acceptance,dependencies" {
		t.Fatalf("cross-check must depend on all judgment packets, got %v", cross.DependsOn)
	}
	if len(run.RequiredFiles) != 1 || run.RequiredFiles[0] != "docs/specs/units/candidate/unit_auth.md" {
		t.Fatalf("expected the main spec as required file, got %v", run.RequiredFiles)
	}
}

func TestGatePacketStructuralContextIncludesUnresolvedLogicalReference(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecWithRefs(t, repoRoot, "auth", "missing", "none")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	structural := run.PacketByID("structural")
	if structural == nil || !grContainsString(structural.ReadRefs, "unit:missing") {
		t.Fatalf("structural packet must retain the unresolved Check 1 reference, got %+v", structural)
	}
	if design := run.PacketByID("design"); design == nil || grContainsString(design.ReadRefs, "unit:missing") {
		t.Fatalf("design packet must not receive the unrelated logical reference, got %+v", design)
	}

	var stdout, stderr bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "structural"}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-packet failed: %v (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	if !strings.Contains(stdout.String(), "  - unit:missing\n") {
		t.Fatalf("structural execution context must expose the unresolved reference, got:\n%s", stdout.String())
	}
}

func TestGatePlanVerifyAndReviewPlans(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	verifyRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run, err := gaterun.Load(repoRoot, verifyRun)
	if err != nil {
		t.Fatal(err)
	}
	wantVerify := "detect:auth.login,analysis:auth.login,detect:auth.logout,analysis:auth.logout,cross"
	if got := strings.Join(packetIDsOf(run), ","); got != wantVerify {
		t.Fatalf("expected detection/analysis pairs + cross, got %s", got)
	}

	reviewRun := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	run, err = gaterun.Load(repoRoot, reviewRun)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 2 || run.Packets[0].PacketID != "src/auth.go" || run.Packets[1].PacketID != "cross" {
		t.Fatalf("expected per-file packets + cross, got %+v", packetIDsOf(run))
	}
}

// TestGatePlanRejectsUnresolvedImplementationSurface verifies the end-to-end
// fail-closed path: a non-<pending> implementation_surface that cannot
// resolve to a code file rejects verify/review planning with the item id and
// the reason, and leaves no run state behind. The declared files exist — the
// value is rejected because a semicolon list is not a path.
func TestGatePlanRejectsUnresolvedImplementationSurface(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	grWriteFile(t, repoRoot, "internal/demo/b.go", "package demo\n")
	grWriteSpecSurface(t, repoRoot, "demo", "internal/demo/a.go; internal/demo/b.go", "")

	_, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "demo", "--target", "candidate")
	if err == nil || !strings.Contains(err.Error(), "demo.core") || !strings.Contains(err.Error(), "path does not exist") {
		t.Fatalf("expected the item-granular surface rejection, got %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(repoRoot, "meta/gate_runs"))
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("a rejected verify plan must not persist run state, found %d entries", len(entries))
	}
}

// TestGatePlanRejectsEmptyAcceptanceItemSet verifies the end-to-end work-set
// precondition: a verify plan for a spec whose acceptance_item_set has no
// items is rejected before any run state is written, while the validate gate
// still plans the same spec so its Check 2 can report the empty set.
func TestGatePlanRejectsEmptyAcceptanceItemSet(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "demo", "none", "none", nil)

	_, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "demo", "--target", "candidate")
	if err == nil || !strings.Contains(err.Error(), "at least one acceptance item") {
		t.Fatalf("expected the empty-item-set rejection, got %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(repoRoot, "meta/gate_runs"))
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("a rejected verify plan must not persist run state, found %d entries", len(entries))
	}
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "demo", "--target", "candidate"); err != nil {
		t.Fatalf("validate planning must still plan an empty item set, got %v", err)
	}
}

// TestGatePlanVerifyAcceptsLiteralMetacharacterSurface verifies the
// end-to-end positive path for a real path containing glob metacharacters:
// the value is matched literally, so a bracketed route file is planned and
// reaches the packet read refs instead of being misread as a wildcard
// pattern.
func TestGatePlanVerifyAcceptsLiteralMetacharacterSurface(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteFile(t, repoRoot, "app/[id]/route.ts", "export {}\n")
	grWriteSpecSurface(t, repoRoot, "demo", "app/[id]/route.ts", "")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "demo", "--target", "candidate")

	var stdout, stderr bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "detect:demo.core"}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-packet failed: %v (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	if out := stdout.String(); !strings.Contains(out, "  - app/[id]/route.ts\n") {
		t.Fatalf("expected the literal bracketed path in the packet read refs, got:\n%s", out)
	}
}

// TestGatePlanVerifySurfaceExpansionIncludesCodeFiles verifies the positive
// path: a directory surface plus affects.files expands to real code files in
// the packet read refs.
func TestGatePlanVerifySurfaceExpansionIncludesCodeFiles(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	grWriteFile(t, repoRoot, "internal/demo/b.go", "package demo\n")
	extra := "    affects:\n      files:\n        - internal/demo/a.go\n        - internal/demo/b.go\n"
	grWriteSpecSurface(t, repoRoot, "demo", "internal/demo", extra)

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "demo", "--target", "candidate")

	var stdout, stderr bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "detect:demo.core"}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-packet failed: %v (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	out := stdout.String()
	for _, want := range []string{"internal/demo/a.go", "internal/demo/b.go"} {
		if !strings.Contains(out, "  - "+want+"\n") {
			t.Fatalf("expected %s in the packet read refs, got:\n%s", want, out)
		}
	}

	reviewID := grPlan(t, repoRoot, "--gate", "review", "--unit", "demo", "--target", "candidate")
	reviewRun, err := gaterun.Load(repoRoot, reviewID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviewRun.Packets) != 3 {
		t.Fatalf("expected two file packets + cross, got %+v", packetIDsOf(reviewRun))
	}
}

func TestGatePlanRejectsDuplicateAcceptanceItemIDsBeforePersistingRun(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.login"})
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	if _, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate"); err == nil || !strings.Contains(err.Error(), `duplicate packet id "detect:auth.login"`) {
		t.Fatalf("expected duplicate acceptance item ids to reject the packet plan, got %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(repoRoot, "meta/gate_runs"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid plan must not persist run state, found %d entries", len(entries))
	}
}

func TestGatePlanRejectsPhysicalInputsOutsideProject(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	external := grWriteFile(t, t.TempDir(), "outside.txt", "outside\n")

	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--input", external); err == nil || !strings.Contains(err.Error(), "outside the repository root") {
		t.Fatalf("expected an absolute external input to be rejected, got %v", err)
	}

	link := filepath.Join(repoRoot, "external-link")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--input", "external-link"); err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected an escaping symlink input to be rejected, got %v", err)
	}
}

func packetIDsOf(run *gaterun.Run) []string {
	var ids []string
	for _, p := range run.Packets {
		ids = append(ids, p.PacketID)
	}
	return ids
}

func TestGatePlanRulePlan(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteFile(t, repoRoot, "docs/specs/rules/candidate/b_rule_http.md", "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Packets) != 1 || run.Packets[0].PacketID != "checks" || len(run.Packets[0].CheckKeys) != 8 {
		t.Fatalf("expected one 8-check rule packet, got %+v", run.Packets)
	}
	if _, err := grPlanRaw(repoRoot, "--gate", "verify", "--rule", "b_rule_http", "--target", "candidate"); err == nil || !strings.Contains(err.Error(), "validate gate only") {
		t.Fatalf("expected rule verify to be rejected, got %v", err)
	}
}

// ------------------------------------------------------------
// Full runs
// ------------------------------------------------------------

func TestGateRunValidatePassBasic(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
	out := grFinalizeOK(t, repoRoot, runID, "--result", "pass")
	if !strings.Contains(out, "Cache written:") || !strings.Contains(out, "Self-check: FRESH") {
		t.Fatalf("unexpected output:\n%s", out)
	}

	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected written cache to be fresh, got: %s", res.Reason)
	}

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "mode: full") || !strings.Contains(cache, "basis: full") {
		t.Fatalf("expected full-mode cache, got:\n%s", cache)
	}
	_ = specPath
}

func TestGateRunComputesHashAndDeps(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	main := "docs/specs/units/candidate/unit_auth.md"
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": all"},
		"3": {main + ": all"},
		"6": {main + ": all"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": all"},
		"4": {main + ": all"},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": all"}}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {main + ": all"},
		"8": {main + ": all"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "all"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	text, err := contenthash.FileText(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cache, "hash: "+contenthash.FileHashText(text)) {
		t.Fatalf("expected computed hash in cache, got:\n%s", cache)
	}
	for _, c := range contenthash.ChunkText(text).Chunks {
		if !strings.Contains(cache, "- "+c.CID) {
			t.Fatalf("expected chunk CID %s in deps, got:\n%s", c.CID, cache)
		}
	}
}

func TestGateRunSectionAndRangeDeclarations(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	text, err := contenthash.FileText(specPath)
	if err != nil {
		t.Fatal(err)
	}
	descRegion, ok := contenthash.LocateSectionRegion(text, "Description")
	if !ok {
		t.Fatal("expected Description section")
	}
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	main := "docs/specs/units/candidate/unit_auth.md"
	// The structural packet merges a section declaration and a range
	// declaration for check 1; check 5 declares the acceptance-item region.
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description", main + ": 1-20"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {main + ": acceptance_items"},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, descDep) {
		t.Fatalf("expected the section region dep %s, got:\n%s", descDep, cache)
	}
	if !strings.Contains(cache, "region:acceptance_items:") {
		t.Fatalf("expected the acceptance-items region dep, got:\n%s", cache)
	}
	if !strings.Contains(cache, "- sha256:") {
		t.Fatalf("expected chunk CIDs from the range declaration, got:\n%s", cache)
	}
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected FRESH, got: %s", res.Reason)
	}
}

func TestGateRunLogicalReference(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth") // dependency unit
	grWriteSpecWithRefs(t, repoRoot, "self", "auth", "none")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "self", "--target", "candidate")
	main := "docs/specs/units/candidate/unit_self.md"
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": frontmatter", main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {"unit:auth: Testability / Acceptance Criteria"},
		"8": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/self/validate_result.md")
	if !strings.Contains(cache, "- path: unit:auth") || !strings.Contains(cache, "hash: sha256:") {
		t.Fatalf("expected the logical reference entry with computed hash, got:\n%s", cache)
	}
	res, err := validationcache.CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected FRESH, got: %s", res.Reason)
	}
}

func TestGateRunGlobalRuleUsesStableTruth(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	mainPath := grWriteSpec(t, repoRoot, "self")
	stableRulePath := grWriteFile(t, repoRoot, "docs/specs/rules/stable/g_rule_http.md", "---\nrule_id: g_rule_http\nrule_scope: global\nrule_version: 1.0.0\n---\nStable constraint.\n")
	grWriteFile(t, repoRoot, "docs/specs/rules/candidate/g_rule_http.md", "---\nrule_id: g_rule_http\nrule_scope: global\nrule_version: 2.0.0\n---\nCandidate draft.\n")
	grWriteFile(t, repoRoot, "docs/specs/rules/candidate/g_rule_draft.md", "---\nrule_id: g_rule_draft\nrule_scope: global\nrule_version: 0.1.0\n---\nCandidate-only draft.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "self", "--target", "candidate")
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	global, ok := grRefByName(run, "rule:g_rule_http")
	if !ok || global.Resolved != "docs/specs/rules/stable/g_rule_http.md" {
		t.Fatalf("planned global rule must resolve to stable truth, got %+v", global)
	}
	if _, ok := grRefByName(run, "rule:g_rule_draft"); ok {
		t.Fatalf("candidate-only global rule must not enter the plan: %+v", run.Refs)
	}

	main := filepath.ToSlash(strings.TrimPrefix(mainPath, repoRoot+string(filepath.Separator)))
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": frontmatter", main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {main + ": frontmatter"},
		"8": {"rule:g_rule_http: all"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID)

	stableText, err := contenthash.FileText(stableRulePath)
	if err != nil {
		t.Fatal(err)
	}
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/self/validate_result.md")
	if !strings.Contains(cache, "- path: rule:g_rule_http") || !strings.Contains(cache, "hash: "+contenthash.FileHashText(stableText)) {
		t.Fatalf("cache must retain the logical global ref with the stable hash, got:\n%s", cache)
	}
	result, err := validationcache.CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fresh {
		t.Fatalf("finalized cache must be fresh against stable global truth: %s", result.Reason)
	}
}

func TestGateRunVerifyFlow(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Login() {}\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", "docs/specs/units/candidate/unit_auth.md", "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport("docs/specs/units/candidate/unit_auth.md", "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass", "--p2-count", "1")

	res, err := validationcache.CheckVerify(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected verify cache fresh, got: %s", res.Reason)
	}
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, `check: "auth.core"`) || !strings.Contains(cache, "- path: src/auth.go") {
		t.Fatalf("expected the per-item and code declarations, got:\n%s", cache)
	}
}

func TestGateRunReviewFlow(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "src/auth.go", grReviewReport("src/auth.go", "docs/specs/units/candidate/unit_auth.md"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport("docs/specs/units/candidate/unit_auth.md", "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	res, err := validationcache.CheckReview(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected review cache fresh, got: %s", res.Reason)
	}
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
	if !strings.Contains(cache, "- path: src/auth.go") {
		t.Fatalf("expected the reviewed file entry, got:\n%s", cache)
	}
}

func TestGateSubmitRejectsIncompletePacketBodies(t *testing.T) {
	t.Run("review dimensions", func(t *testing.T) {
		repoRoot := createCLITestRepo(t)
		grWriteSpec(t, repoRoot, "auth")
		main := "docs/specs/units/candidate/unit_auth.md"
		grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
		runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
		incomplete := "Architecture assessment:\n  conclusion: acceptable\n  module_boundaries: sound — basis\n\nsrc/auth.go: " + main + ": Description\nsrc/auth.go: src/auth.go: all\n"
		if _, err := grSubmit(t, repoRoot, runID, "src/auth.go", incomplete); err == nil || !strings.Contains(err.Error(), "responsibility_organization") {
			t.Fatalf("expected incomplete review report to be rejected, got %v", err)
		}
		if _, err := grFinalize(t, repoRoot, runID); err == nil {
			t.Fatal("finalize must refuse a run with a rejected packet")
		}
		cache := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
		if _, err := os.Stat(cache); !os.IsNotExist(err) {
			t.Fatalf("incomplete review must not write a cache, stat error=%v", err)
		}
	})

	t.Run("verify evidence blocks", func(t *testing.T) {
		repoRoot := createCLITestRepo(t)
		grWriteSpec(t, repoRoot, "auth")
		main := "docs/specs/units/candidate/unit_auth.md"
		grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
		runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
		incomplete := "- auth.core: ALIGNED — src/auth.go:1\n  evidence: src/auth.go:1\n  deterministic: true\n  Part A: No concerns\n\nauth.core: " + main + ": acceptance_item:auth.core\nauth.core: src/auth.go: all\n"
		if _, err := grSubmit(t, repoRoot, runID, "auth.core", incomplete); err == nil || !strings.Contains(err.Error(), "Part B") {
			t.Fatalf("expected incomplete verify report to be rejected, got %v", err)
		}
	})

	t.Run("validate Check 5 sub-checks", func(t *testing.T) {
		repoRoot := createCLITestRepo(t)
		grWriteSpec(t, repoRoot, "auth")
		main := "docs/specs/units/candidate/unit_auth.md"
		runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
		complete := grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": acceptance_items"}})
		incomplete := strings.Replace(complete, "  5i. Required acceptance sub-check: PASS — checked\n", "", 1)
		if _, err := grSubmit(t, repoRoot, runID, "acceptance", incomplete); err == nil || !strings.Contains(err.Error(), "5i verdict") {
			t.Fatalf("expected incomplete validate report to be rejected, got %v", err)
		}
	})
}

func TestGateRunRuleTarget(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var b strings.Builder
	for c := 1; c <= 8; c++ {
		fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
	}
	for c := 1; c <= 8; c++ {
		fmt.Fprintf(&b, "check-%d: %s: Constraint\n", c, rulePath)
	}
	grSubmitOK(t, repoRoot, runID, "checks", b.String())
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	res, err := validationcache.CheckRuleValidate(repoRoot, "b_rule_http")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected rule validate cache fresh, got: %s", res.Reason)
	}
}

func TestGateRunRuleWithConsumerRef(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")
	grWriteSpecWithRefs(t, repoRoot, "auth", "none", "b_rule_http")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var b strings.Builder
	for c := 1; c <= 8; c++ {
		fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
	}
	for c := 1; c <= 8; c++ {
		scope, decl := rulePath, "Constraint"
		if c == 5 || c == 7 {
			scope, decl = "unit:auth", "Testability / Acceptance Criteria"
		}
		fmt.Fprintf(&b, "check-%d: %s: %s\n", c, scope, decl)
	}
	grSubmitOK(t, repoRoot, runID, "checks", b.String())
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/rule/b_rule_http/validate_result.md")
	if !strings.Contains(cache, "- path: unit:auth") {
		t.Fatalf("expected the consumer logical reference, got:\n%s", cache)
	}
	if judgments := grGateJudgments(t, cache); !strings.Contains(judgments, `"findings":[]`) {
		t.Fatalf("expected the zero-finding rule judgment baseline to record an empty findings array, got:\n%s", judgments)
	}
}

// ------------------------------------------------------------
// Failure records and result consistency
// ------------------------------------------------------------

func TestGateRunFailureRecordDerivesStatus(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemBody("auth.core", "MISMATCH (acceptance)", "pass_condition not met at src/auth.go:1")+"auth.core: "+main+": Testability / Acceptance Criteria\n")
	grSubmitOK(t, repoRoot, runID, "analysis:auth.core", grVerifyAnalysisReport("auth.core", main, "src/auth.go", "P0"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "fail", "--p0-count", "1")
	res, err := validationcache.CheckVerify(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if res.Category != validationcache.CategoryBlocked {
		t.Fatalf("expected BLOCKED failure record, got %q: %s", res.Category, res.Reason)
	}
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, `check: "auth.core"`) || !strings.Contains(cache, "status: fail") {
		t.Fatalf("expected the derived per-item status map, got:\n%s", cache)
	}
	if !strings.Contains(cache, "blocking: true") || !strings.Contains(cache, "result: fail") {
		t.Fatalf("expected a failure record, got:\n%s", cache)
	}
}

func TestGateFinalizeDerivesResultAndRejectsJudgmentFlags(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	var stdout, stderr bytes.Buffer
	if err := runGateFinalize([]string{"--repo-root", repoRoot, "--run", runID, "--result", "fail"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("expected coordinator judgment flags to be rejected, got %v", err)
	}
	grFinalizeOK(t, repoRoot, runID)
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "p0_count: 0") {
		t.Fatalf("expected the result and counts to be derived from the accepted synthesis, got:\n%s", cache)
	}
	if judgments := grGateJudgments(t, cache); !strings.Contains(judgments, `"findings":[]`) {
		t.Fatalf("expected the zero-finding judgment baseline to record an empty findings array, got:\n%s", judgments)
	}
}

func TestGateCrossFailureForcesDerivedFailure(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.login", grVerifyItemReport("auth.login", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "auth.logout", grVerifyItemReport("auth.logout", main, "src/auth.go"))
	missingAffected := "Cross-check: 4/5 FAIL — combined contract contradiction\n\n[P1] cross — individually valid claims conflict when combined (actionable)\n\ncross: " + main + ": Description\nSeverity confirmation: " + grRunFindingID(runID, "cross", 1) + " = confirmed P1 — evidence: " + main + "; reason: combined contract is blocking\nEffective status: auth.login = fail\nEffective status: auth.logout = pass\nEffective status: cross = fail\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", missingAffected); err == nil || !strings.Contains(err.Error(), "at least one affected non-cross logical key") {
		t.Fatalf("expected a new cross finding without affected keys to be rejected, got %v", err)
	}
	cross := "Cross-check: 4/5 FAIL — combined contract contradiction\n\n[P1] cross — individually valid claims conflict when combined (actionable)\nFinding affects: " + grRunFindingID(runID, "cross", 1) + " = auth.login\n\ncross: " + main + ": Description\nSeverity confirmation: " + grRunFindingID(runID, "cross", 1) + " = confirmed P1 — evidence: " + main + "; reason: combined contract is blocking\nEffective status: auth.login = fail\nEffective status: auth.logout = pass\nEffective status: cross = fail\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "blocking: true") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("expected cross failure to force a blocking derived result, got:\n%s", cache)
	}
	repairID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	repair := mustLoadRun(t, repoRoot, repairID)
	if got := strings.Join(packetIDsOf(repair), ","); got != "detect:auth.login,analysis:auth.login,cross" {
		t.Fatalf("expected the cross finding's affected key to drive repair scope, got %s", got)
	}
	if got := strings.Join(repair.CarriedKeys, ","); got != "auth.logout" {
		t.Fatalf("expected unaffected auth.logout to be carried, got %s", got)
	}
}

func TestGateCrossMustDisposeEveryInputFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	report := grReviewArchitecture("unacceptable — broken boundary") + "\n[P1] src/auth.go:1 — broken boundary (actionable)\n\nsrc/auth.go: " + main + ": Description\nsrc/auth.go: src/auth.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/auth.go", report)
	cross := "Cross-check: 4/4 PASS — consistent\n\ncross: " + main + ": Description\nEffective status: src/auth.go = fail\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", cross); err == nil || !strings.Contains(err.Error(), "does not dispose input finding") {
		t.Fatalf("expected omitted finding disposition to be rejected, got %v", err)
	}
}

func TestGateCrossRejectsFailStatusWithoutRetainedFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "src/auth.go", grReviewReport("src/auth.go", main))
	cross := "Cross-check: 4/4 PASS — consistent\n\ncross: " + main + ": Description\nEffective status: src/auth.go = fail\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", cross); err == nil || !strings.Contains(err.Error(), "src/auth.go = pass") {
		t.Fatalf("expected a finding-free fail status to be rejected, got %v", err)
	}
}

func TestGateCrossRequiresNonBlockingRetainedFindingToFailItsKey(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	report := grReviewReport("src/auth.go", main) + "\n[P2] src/auth.go:1 — local maintainability concern (actionable)\n"
	grSubmitOK(t, repoRoot, runID, "src/auth.go", report)
	crossPrefix := "Cross-check: 4/4 PASS — consistent\n\nFinding disposition: " + grRunFindingID(runID, "src/auth.go", 1) + " = retained\ncross: " + main + ": Description\n"
	confirmation := "Severity confirmation: " + grRunFindingID(runID, "src/auth.go", 1) + " = confirmed P2 — evidence: " + main + "; reason: impact remains local\n"
	invalid := crossPrefix + confirmation + "Effective status: src/auth.go = pass\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", invalid); err == nil || !strings.Contains(err.Error(), "src/auth.go = fail") {
		t.Fatalf("expected a retained P2 finding to require fail status for its key, got %v", err)
	}
	valid := crossPrefix + confirmation + "Effective status: src/auth.go = fail\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, runID, "cross", valid)
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "blocking: false") || !strings.Contains(cache, "p2_count: 1") {
		t.Fatalf("expected a non-blocking P2 result with a failed logical key, got:\n%s", cache)
	}
}

func TestGateCrossRequiresOneSeverityConfirmationPerTerminalFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	report := grReviewReport("src/auth.go", main) + "\n[P2] src/auth.go:1 — local maintainability concern (actionable)\n"
	grSubmitOK(t, repoRoot, runID, "src/auth.go", report)
	base := "Cross-check: 4/4 PASS — consistent\n\nFinding disposition: " + grRunFindingID(runID, "src/auth.go", 1) + " = retained\ncross: " + main + ": Description\n"
	statuses := "Effective status: src/auth.go = fail\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", base+statuses); err == nil || !strings.Contains(err.Error(), "has no severity confirmation") {
		t.Fatalf("expected missing severity confirmation to be rejected, got %v", err)
	}
	duplicate := base +
		"Severity confirmation: " + grRunFindingID(runID, "src/auth.go", 1) + " = confirmed P2 — evidence: " + main + "; reason: first\n" +
		"Severity confirmation: " + grRunFindingID(runID, "src/auth.go", 1) + " = confirmed P2 — evidence: " + main + "; reason: second\n" + statuses
	if _, err := grSubmit(t, repoRoot, runID, "cross", duplicate); err == nil || !strings.Contains(err.Error(), "second severity confirmation after a final confirmed result") {
		t.Fatalf("expected duplicate severity confirmation to be rejected, got %v", err)
	}
	missingScope := base +
		"Severity confirmation: " + grRunFindingID(runID, "src/auth.go", 1) + " = confirmed P2 — evidence: src/auth.go; reason: impact remains local\n" + statuses
	if _, err := grSubmit(t, repoRoot, runID, "cross", missingScope); err == nil || !strings.Contains(err.Error(), "without a matching Dependency scope") {
		t.Fatalf("expected undeclared severity evidence to be rejected, got %v", err)
	}
}

func TestGateReviewIndentedFindingsAreNotSwallowedAsDetail(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	report := grReviewArchitecture("needs_attention — local concerns") +
		"\nFindings:\n" +
		"  [P2] src/auth.go:1 — local maintainability concern (actionable)\n" +
		"    spec_context: the design accepts this shape\n" +
		"  [P0] src/auth.go:2 — boundary defect reaching consumers (actionable)\n" +
		"    spec_context: the design requires the boundary\n" +
		"\n" +
		"src/auth.go: " + main + ": Description\n" +
		"src/auth.go: src/auth.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/auth.go", report)

	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := gaterun.LoadPacketState(repoRoot, run, "src/auth.go")
	if err != nil {
		t.Fatal(err)
	}
	if state.Result == nil || len(state.Result.Findings) != 2 {
		t.Fatalf("expected both indented findings to be parsed, got %#v", state.Result)
	}

	crossBase := "Cross-check: 4/4 PASS — consistent\n\ncross: " + main + ": Description\n"
	grSubmitOK(t, repoRoot, runID, "cross", grCompleteCrossReport(t, repoRoot, run, crossBase))
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "blocking: true") || !strings.Contains(cache, "p0_count: 1") {
		t.Fatalf("expected the indented P0 to block the gate, got:\n%s", cache)
	}
}

func TestGateVerifyItemIDWithRegexMetacharacterSubmits(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core", "auth.co(re"})
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "detect:auth.co(re", grVerifyItemReport("auth.co(re", main, "src/auth.go"))
}

func TestGateReviewConclusionMappingIsMechanical(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	scopes := "\nsrc/auth.go: " + main + ": Description\nsrc/auth.go: src/auth.go: all\n"

	// A P0/P1 gate_findings entry requires the unacceptable conclusion.
	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	contradicting := strings.Replace(grReviewArchitecture("acceptable"), "gate_findings: none", "gate_findings: [P1] boundary defect", 1) +
		"\n[P1] src/auth.go:1 — broken boundary (actionable)\n" + scopes
	if _, err := grSubmit(t, repoRoot, runID, "src/auth.go", contradicting); err == nil || !strings.Contains(err.Error(), "requires the unacceptable conclusion") {
		t.Fatalf("expected the conclusion/finding mismatch rejection, got %v", err)
	}

	// The unacceptable conclusion requires a P0/P1 gate_findings entry.
	runID = grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	noEntry := strings.Replace(grReviewArchitecture("unacceptable — broken boundary"),
		"gate_findings: [P1] gate-level architectural boundary defect", "gate_findings: none", 1) +
		"\n[P1] src/auth.go:1 — broken boundary (actionable)\n" + scopes
	if _, err := grSubmit(t, repoRoot, runID, "src/auth.go", noEntry); err == nil || !strings.Contains(err.Error(), "requires a P0/P1 gate_findings entry") {
		t.Fatalf("expected the missing-gate_findings rejection, got %v", err)
	}

	// An unusable gate_findings value is rejected at parse time.
	runID = grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	badContent := strings.Replace(grReviewArchitecture("acceptable"), "gate_findings: none", "gate_findings: reported below", 1) + scopes
	if _, err := grSubmit(t, repoRoot, runID, "src/auth.go", badContent); err == nil || !strings.Contains(err.Error(), "gate_findings must be") {
		t.Fatalf("expected the unusable gate_findings rejection, got %v", err)
	}

	// The consistent combination is accepted.
	runID = grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	valid := grReviewArchitecture("unacceptable — broken boundary") + "\n[P1] src/auth.go:1 — broken boundary (actionable)\n" + scopes
	grSubmitOK(t, repoRoot, runID, "src/auth.go", valid)
}

func TestGateVerifyMismatchRequiresStructuredType(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	noType := grVerifyItemBody("auth.core", "MISMATCH", "broken at src/auth.go:1") + "auth.core: " + main + ": Testability / Acceptance Criteria\n"
	if _, err := grSubmit(t, repoRoot, runID, "auth.core", noType); err == nil || !strings.Contains(err.Error(), "requires a type suffix") {
		t.Fatalf("expected the missing MISMATCH type rejection, got %v", err)
	}
	badType := grVerifyItemBody("auth.core", "MISMATCH (banana)", "broken at src/auth.go:1") + "auth.core: " + main + ": Testability / Acceptance Criteria\n"
	if _, err := grSubmit(t, repoRoot, runID, "auth.core", badType); err == nil || !strings.Contains(err.Error(), "requires a type suffix") {
		t.Fatalf("expected the unknown MISMATCH type rejection, got %v", err)
	}
	good := grVerifyItemBody("auth.core", "MISMATCH (structural)", "broken at src/auth.go:1") + "auth.core: " + main + ": Testability / Acceptance Criteria\n"
	grSubmitOK(t, repoRoot, runID, "auth.core", good)
}

func TestGateVerifyPartBSkippedRequiresReason(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	noReason := "- auth.core: ALIGNED — src/auth.go:1\n  evidence: src/auth.go:1 implements it\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped\n\nauth.core: " + main + ": Testability / Acceptance Criteria\nauth.core: src/auth.go: all\n"
	if _, err := grSubmit(t, repoRoot, runID, "auth.core", noReason); err == nil || !strings.Contains(err.Error(), "must carry a reason") {
		t.Fatalf("expected the bare skipped rejection, got %v", err)
	}
	withReason := strings.Replace(noReason, "Part B: skipped\n", "Part B: skipped — no test files in this fixture\n", 1)
	grSubmitOK(t, repoRoot, runID, "auth.core", withReason)
}

func TestGateFindingsRequireResolutionLabel(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	noLabel := "2. Design soundness: FAIL — contradiction\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n[P1] design — contradiction\n\ncheck-2: " + main + ": Description\ncheck-4: " + main + ": Description\n"
	if _, err := grSubmit(t, repoRoot, runID, "design", noLabel); err == nil || !strings.Contains(err.Error(), "carries no resolution label") {
		t.Fatalf("expected the missing-label rejection, got %v", err)
	}
	labeled := strings.Replace(noLabel, "[P1] design — contradiction\n", "[P1] design — contradiction (actionable)\n", 1)
	grSubmitOK(t, repoRoot, runID, "design", labeled)
}

func TestGateReviewP3RequiresFactAnchor(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	scopes := "src/auth.go: " + main + ": Description\nsrc/auth.go: src/auth.go: all\n"

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	noAnchor := grReviewArchitecture("acceptable") + "\n[P3] src/auth.go:1 — stale comment (actionable)\n\n" + scopes
	if _, err := grSubmit(t, repoRoot, runID, "src/auth.go", noAnchor); err == nil || !strings.Contains(err.Error(), "no non-empty fact_anchor") {
		t.Fatalf("expected the missing fact_anchor rejection, got %v", err)
	}
	withAnchor := grReviewArchitecture("acceptable") + "\n[P3] src/auth.go:1 — stale comment (actionable)\n  fact_anchor: src/auth.go:1 declares behavior the code at src/auth.go:2 contradicts\n\n" + scopes
	grSubmitOK(t, repoRoot, runID, "src/auth.go", withAnchor)
}

func TestApplySeverityConfirmationsRejectsInvalidRecords(t *testing.T) {
	findings := []gaterun.Finding{{ID: "packet/F1", Severity: "P2"}}
	tests := []struct {
		name   string
		checks []gaterun.SeverityConfirmation
		want   string
	}{
		{
			name:   "unknown finding",
			checks: []gaterun.SeverityConfirmation{{FindingID: "packet/F2", Outcome: "confirmed", OriginalSeverity: "P2", FinalSeverity: "P2"}},
			want:   "non-terminal finding",
		},
		{
			name:   "original mismatch",
			checks: []gaterun.SeverityConfirmation{{FindingID: "packet/F1", Outcome: "confirmed", OriginalSeverity: "P1", FinalSeverity: "P1"}},
			want:   "starts at P1, want P2",
		},
		{
			name:   "multi-level adjustment",
			checks: []gaterun.SeverityConfirmation{{FindingID: "packet/F1", Outcome: "adjusted", OriginalSeverity: "P2", FinalSeverity: "P0"}},
			want:   "move exactly one level",
		},
		{
			name:   "adjustment missing final check",
			checks: []gaterun.SeverityConfirmation{{FindingID: "packet/F1", Outcome: "adjusted", OriginalSeverity: "P2", FinalSeverity: "P1"}},
			want:   "requires exactly one final second confirmation",
		},
		{
			name: "broken second-step chain",
			checks: []gaterun.SeverityConfirmation{
				{FindingID: "packet/F1", Outcome: "adjusted", OriginalSeverity: "P2", FinalSeverity: "P1"},
				{FindingID: "packet/F1", Outcome: "confirmed", OriginalSeverity: "P2", FinalSeverity: "P2"},
			},
			want: "starts at P2, want P1",
		},
		{
			name: "third record",
			checks: []gaterun.SeverityConfirmation{
				{FindingID: "packet/F1", Outcome: "adjusted", OriginalSeverity: "P2", FinalSeverity: "P1"},
				{FindingID: "packet/F1", Outcome: "adjusted", OriginalSeverity: "P1", FinalSeverity: "P0"},
				{FindingID: "packet/F1", Outcome: "confirmed", OriginalSeverity: "P0", FinalSeverity: "P0"},
			},
			want: "more than two severity confirmation records",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := applySeverityConfirmations(findings, tc.checks); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestApplySeverityConfirmationsAllowsTwoAdjacentAdjustments(t *testing.T) {
	findings := []gaterun.Finding{{ID: "packet/F1", Severity: "P0", Detail: "[P0] boundary — issue"}}
	checks := []gaterun.SeverityConfirmation{
		{FindingID: "packet/F1", Outcome: "adjusted", OriginalSeverity: "P0", FinalSeverity: "P1"},
		{FindingID: "packet/F1", Outcome: "adjusted", OriginalSeverity: "P1", FinalSeverity: "P2"},
	}
	got, err := applySeverityConfirmations(findings, checks)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Severity != "P2" || !strings.HasPrefix(got[0].Detail, "[P2]") {
		t.Fatalf("two-step adjustment did not produce canonical P2 finding: %+v", got)
	}
}

func TestGateCrossSeverityAdjustmentControlsFinalOutcome(t *testing.T) {
	tests := []struct {
		name       string
		local      string
		from       string
		to         string
		wantResult string
		wantCount  string
	}{
		{
			name:       "upgrade P2 to P1 blocks",
			local:      grReviewArchitecture("needs_attention") + "\n[P2] src/auth.go:1 — impact reaches public behavior (actionable)\n",
			from:       "P2",
			to:         "P1",
			wantResult: "fail",
			wantCount:  "p1_count: 1",
		},
		{
			name:       "downgrade P1 to P2 unblocks",
			local:      grReviewArchitecture("unacceptable — suspected boundary issue") + "\n[P1] src/auth.go:1 — impact is contained locally (actionable)\n",
			from:       "P1",
			to:         "P2",
			wantResult: "pass",
			wantCount:  "p2_count: 1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := createCLITestRepo(t)
			grWriteSpec(t, repoRoot, "auth")
			main := "docs/specs/units/candidate/unit_auth.md"
			grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
			runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
			local := tc.local + "\nsrc/auth.go: " + main + ": Description\nsrc/auth.go: src/auth.go: all\n"
			grSubmitOK(t, repoRoot, runID, "src/auth.go", local)
			cross := "Cross-check: 4/4 PASS — consistent\n\nFinding disposition: " + grRunFindingID(runID, "src/auth.go", 1) + " = retained\ncross: " + main + ": Description\n" +
				"Severity confirmation: " + grRunFindingID(runID, "src/auth.go", 1) + " = adjusted " + tc.from + " -> " + tc.to + " — evidence: " + main + "; reason: boundary evidence changes the grade\n" +
				"Severity confirmation: " + grRunFindingID(runID, "src/auth.go", 1) + " = confirmed " + tc.to + " — evidence: " + main + "; reason: second boundary check confirms the adjusted grade\n" +
				"Effective status: src/auth.go = fail\nEffective status: cross = pass\n"
			grSubmitOK(t, repoRoot, runID, "cross", cross)
			grFinalizeOK(t, repoRoot, runID)
			cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
			if !strings.Contains(cache, "result: "+tc.wantResult) || !strings.Contains(cache, tc.wantCount) {
				t.Fatalf("adjusted severity did not control final outcome:\n%s", cache)
			}
			baseline := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateReview)
			if len(baseline.Findings) != 1 || baseline.Findings[0].Severity != tc.to || !strings.HasPrefix(baseline.Findings[0].Detail, "["+tc.to+"]") {
				t.Fatalf("baseline did not retain canonical severity %s: %+v", tc.to, baseline.Findings)
			}
		})
	}
}

func TestGateCrossTwoSeverityAdjustmentsUseFinalSeverity(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	local := grReviewArchitecture("unacceptable — suspected main-chain break") +
		"\n[P0] src/auth.go:1 — impact is ultimately local maintainability debt (actionable)\n\n" +
		"src/auth.go: " + main + ": Description\nsrc/auth.go: src/auth.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/auth.go", local)
	findingID := grRunFindingID(runID, "src/auth.go", 1)
	cross := "Cross-check: 4/4 PASS — consistent\n\nFinding disposition: " + findingID + " = retained\ncross: " + main + ": Description\n" +
		"Severity confirmation: " + findingID + " = adjusted P0 -> P1 — evidence: " + main + "; reason: impact depends on downstream use\n" +
		"Severity confirmation: " + findingID + " = adjusted P1 -> P2 — evidence: " + main + "; reason: completed consumer read confines the impact to maintainability\n" +
		"Effective status: src/auth.go = fail\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "blocking: false") || !strings.Contains(cache, "p2_count: 1") {
		t.Fatalf("two-step severity sequence did not use final P2 result:\n%s", cache)
	}
}

func TestRuleValidateRequiresConfirmationForP0(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var b strings.Builder
	for c := 1; c <= 8; c++ {
		status := "PASS — checked"
		if c == 1 {
			status = "FAIL — rule cannot be parsed"
		}
		fmt.Fprintf(&b, "%d. %s: %s\n", c, grCheckNames[fmt.Sprint(c)], status)
	}
	b.WriteString("\n[P0] rule — validation mechanism is unusable (actionable)\n")
	for c := 1; c <= 8; c++ {
		fmt.Fprintf(&b, "check-%d: %s: Constraint\n", c, rulePath)
	}
	if _, err := grSubmit(t, repoRoot, runID, "checks", b.String()); err == nil || !strings.Contains(err.Error(), "has no severity confirmation") {
		t.Fatalf("expected unconfirmed rule P0 to be rejected, got %v", err)
	}
	b.WriteString("Severity confirmation: " + grRunFindingID(runID, "checks", 1) + " = adjusted P0 -> P1 — evidence: " + rulePath + "; reason: impact blocks this rule but not the whole framework\n")
	b.WriteString("Severity confirmation: " + grRunFindingID(runID, "checks", 1) + " = confirmed P1 — evidence: " + rulePath + "; reason: second boundary check confirms the adjusted grade\n")
	grSubmitOK(t, repoRoot, runID, "checks", b.String())
	grFinalizeOK(t, repoRoot, runID)
}

func TestReviewDeltaRendersRetainedCarriedFindingExactlyOnce(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/a.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/b.go", "package auth\n")

	fullID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	findingLine := "[P2] src/a.go:1 — local maintainability concern (actionable)"
	detailLines := "  spec_context: the boundary remains correct\n  recommendation: simplify the helper"
	aReport := grReviewArchitecture("needs_attention") + "\n" + findingLine + "\n" + detailLines + "\n\nsrc/a.go: " + main + ": Description\nsrc/a.go: src/a.go: all\n"
	grSubmitOK(t, repoRoot, fullID, "src/a.go", aReport)
	grSubmitOK(t, repoRoot, fullID, "src/b.go", grReviewReport("src/b.go", main))
	fullCross := "Cross-check: 4/4 PASS — consistent\n\nFinding disposition: " + grRunFindingID(fullID, "src/a.go", 1) + " = retained\ncross: " + main + ": Description\nSeverity confirmation: " + grRunFindingID(fullID, "src/a.go", 1) + " = confirmed P2 — evidence: " + main + "; reason: impact remains local\nEffective status: src/a.go = fail\nEffective status: src/b.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, fullID, "cross", fullCross)
	grFinalizeOK(t, repoRoot, fullID)

	baseline := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateReview)
	if baseline.SchemaVersion != 2 || len(baseline.Findings) != 1 || baseline.Findings[0].Detail != findingLine+"\n"+detailLines {
		t.Fatalf("expected schema v2 with complete finding detail, got %+v", baseline)
	}

	grWriteFile(t, repoRoot, "src/b.go", "package auth\n// changed\n")
	deltaID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if got := strings.Join(deltaRun.CarriedKeys, ","); got != "src/a.go" {
		t.Fatalf("expected src/a.go to be carried, got %s", got)
	}
	grSubmitOK(t, repoRoot, deltaID, "src/b.go", grReviewReport("src/b.go", main))
	deltaCross := "Cross-check: 4/4 PASS — consistent\n\nFinding disposition: " + grRunFindingID(fullID, "src/a.go", 1) + " = retained\ncross: " + main + ": Description\nSeverity confirmation: " + grRunFindingID(fullID, "src/a.go", 1) + " = confirmed P2 — evidence: " + main + "; reason: impact remains local\nEffective status: src/a.go = fail\nEffective status: src/b.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, deltaID, "cross", deltaCross)
	grFinalizeOK(t, repoRoot, deltaID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
	parts := strings.SplitN(cache, "GATE_JUDGMENTS_END -->", 2)
	if len(parts) != 2 {
		t.Fatalf("cache has no judgment marker: %s", cache)
	}
	body := parts[1]
	if strings.Count(body, findingLine) != 1 || !strings.Contains(body, detailLines) || !strings.Contains(body, "Additional retained findings:") {
		t.Fatalf("expected the complete carried finding exactly once in the human-readable body, got:\n%s", body)
	}
}

func TestReviewCrossSuppressionRemovesFindingFromDerivedCounts(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	report := grReviewArchitecture("unacceptable — suspected boundary issue") + "\n[P1] src/auth.go:1 — suspected boundary issue (actionable)\n\nsrc/auth.go: " + main + ": Description\nsrc/auth.go: src/auth.go: all\n"
	grSubmitOK(t, repoRoot, runID, "src/auth.go", report)
	cross := "Cross-check: 4/4 PASS — full context resolves the concern\n\ncross: " + main + ": Description\nFinding disposition: " + grRunFindingID(runID, "src/auth.go", 1) + " = suppressed — full-file context proves the boundary is preserved\nEffective status: src/auth.go = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "p1_count: 0") {
		t.Fatalf("expected suppressed finding to be excluded from the gate result, got:\n%s", cache)
	}
}

// TestDeltaFailRecordMarksCarriedKeysCarried pins the failure-record status
// contract: a judgment carried over from the pass baseline is recorded
// `carried` even when its retained advisory finding gives it the effective
// status `fail` — it did not run in this attempt, and the repair plan must
// carry it (not re-execute it) after the findings are resolved.
func TestDeltaFailRecordMarksCarriedKeysCarried(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/a.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/b.go", "package auth\n")

	fullID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	carriedFinding := "[P2] src/a.go:1 — local maintainability concern (actionable)"
	grSubmitOK(t, repoRoot, fullID, "src/a.go", grReviewReport("src/a.go", main)+"\n"+carriedFinding+"\n")
	grSubmitOK(t, repoRoot, fullID, "src/b.go", grReviewReport("src/b.go", main))
	grSubmitOK(t, repoRoot, fullID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullID)

	// Changing src/b.go re-runs src/b.go only; src/a.go (with its retained P2)
	// is carried over. The delta run then fails on src/b.go's P1: the failure
	// record must keep src/a.go `carried`.
	grWriteFile(t, repoRoot, "src/b.go", "package auth\n// changed\n")
	deltaID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if got := strings.Join(deltaRun.CarriedKeys, ","); got != "src/a.go" {
		t.Fatalf("expected src/a.go to be carried, got %s", got)
	}
	bReport := grReviewReport("src/b.go", main) + "\n[P1] src/b.go:1 — broken error path (actionable)\n"
	grSubmitOK(t, repoRoot, deltaID, "src/b.go", bReport)
	grSubmitOK(t, repoRoot, deltaID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, deltaID)

	baseline, err := validationcache.ReadGateBaseline(repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateReview)
	if err != nil {
		t.Fatal(err)
	}
	if !baseline.Blocking || baseline.Result != "fail" {
		t.Fatalf("expected the delta run to write a failure record, got result=%q blocking=%t", baseline.Result, baseline.Blocking)
	}
	statuses := map[string]string{}
	for _, c := range baseline.Checks {
		statuses[c.Check] = c.Status
	}
	if statuses["src/a.go"] != "carried" {
		t.Fatalf("expected the carried key recorded as carried, got %q (all: %v)", statuses["src/a.go"], statuses)
	}
	if statuses["src/b.go"] != "fail" {
		t.Fatalf("expected the re-run key recorded as fail, got %q (all: %v)", statuses["src/b.go"], statuses)
	}

	// The failure-recovery plan must therefore carry src/a.go and re-run only
	// src/b.go plus the cross packet.
	repairID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	repair := mustLoadRun(t, repoRoot, repairID)
	if got := strings.Join(packetIDsOf(repair), ","); got != "src/b.go,cross" {
		t.Fatalf("expected only the failed file plus cross to re-run, got %s", got)
	}
	if got := strings.Join(repair.CarriedKeys, ","); got != "src/a.go" {
		t.Fatalf("expected src/a.go carried into the repair run, got %s", got)
	}
}

func TestReviewCrossMergeMustTerminateAtRetainedFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/a.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/b.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	for _, file := range []string{"src/a.go", "src/b.go"} {
		report := grReviewArchitecture("unacceptable — duplicated boundary failure") + "\n[P1] " + file + ":1 — duplicated boundary failure (actionable)\n\n" + file + ": " + main + ": Description\n" + file + ": " + file + ": all\n"
		grSubmitOK(t, repoRoot, runID, file, report)
	}
	base := "Cross-check: 4/4 PASS — duplicate findings synthesized\n\ncross: " + main + ": Description\n"
	statuses := "Effective status: src/a.go = fail\nEffective status: src/b.go = fail\nEffective status: cross = pass\n"

	toSuppressed := base +
		"Finding disposition: " + grRunFindingID(runID, "src/a.go", 1) + " = merged -> " + grRunFindingID(runID, "src/b.go", 1) + " — duplicate\n" +
		"Finding disposition: " + grRunFindingID(runID, "src/b.go", 1) + " = suppressed — false positive\n" + statuses
	if _, err := grSubmit(t, repoRoot, runID, "cross", toSuppressed); err == nil || !strings.Contains(err.Error(), "suppressed") {
		t.Fatalf("expected a merge chain ending at a suppressed finding to be rejected, got %v", err)
	}

	cycle := base +
		"Finding disposition: " + grRunFindingID(runID, "src/a.go", 1) + " = merged -> " + grRunFindingID(runID, "src/b.go", 1) + " — duplicate\n" +
		"Finding disposition: " + grRunFindingID(runID, "src/b.go", 1) + " = merged -> " + grRunFindingID(runID, "src/a.go", 1) + " — duplicate\n" + statuses
	if _, err := grSubmit(t, repoRoot, runID, "cross", cycle); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected a merge cycle to be rejected, got %v", err)
	}

	missingSourceFailure := base +
		"Finding disposition: " + grRunFindingID(runID, "src/a.go", 1) + " = merged -> " + grRunFindingID(runID, "src/b.go", 1) + " — duplicate\n" +
		"Finding disposition: " + grRunFindingID(runID, "src/b.go", 1) + " = retained\n" +
		"Severity confirmation: " + grRunFindingID(runID, "src/b.go", 1) + " = confirmed P1 — evidence: " + main + "; reason: merged boundary failure remains blocking\n" +
		"Effective status: src/a.go = pass\nEffective status: src/b.go = fail\nEffective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", missingSourceFailure); err == nil || !strings.Contains(err.Error(), "src/a.go = fail") {
		t.Fatalf("expected a blocking merge group to fail every source key, got %v", err)
	}

	valid := base +
		"Finding disposition: " + grRunFindingID(runID, "src/a.go", 1) + " = merged -> " + grRunFindingID(runID, "src/b.go", 1) + " — duplicate\n" +
		"Finding disposition: " + grRunFindingID(runID, "src/b.go", 1) + " = retained\n" +
		"Severity confirmation: " + grRunFindingID(runID, "src/b.go", 1) + " = confirmed P1 — evidence: " + main + "; reason: merged boundary failure remains blocking\n" + statuses
	grSubmitOK(t, repoRoot, runID, "cross", valid)
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/review_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("expected the retained merge target to be counted once, got:\n%s", cache)
	}
	state := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateReview)
	if len(state.Findings) != 1 || state.Findings[0].ID != grRunFindingID(runID, "src/b.go", 1) || !grContainsString(state.Findings[0].AffectedKeys, "src/a.go") {
		t.Fatalf("expected one canonical retained finding carrying both source keys, got %+v", state.Findings)
	}
}

func TestGateVerifyAnalysisIsFormalDependency(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemBody("auth.core", "MISMATCH (acceptance)", "broken at src/auth.go:1")+"auth.core: "+main+": acceptance_item:auth.core\nauth.core: src/auth.go: all\n")
	if _, err := grSubmit(t, repoRoot, runID, "cross", grCrossReport(main, "Description")); err == nil || !strings.Contains(err.Error(), "analysis:auth.core") {
		t.Fatalf("expected cross to wait for formal analysis, got %v", err)
	}
	grSubmitOK(t, repoRoot, runID, "analysis:auth.core", grVerifyAnalysisReport("auth.core", main, "src/auth.go", "P1"))
	analysisState, err := gaterun.LoadPacketState(repoRoot, mustLoadRun(t, repoRoot, runID), "analysis:auth.core")
	if err != nil {
		t.Fatal(err)
	}
	if len(analysisState.Result.Findings) != 1 || analysisState.Result.Findings[0].ID != grRunFindingID(runID, "analysis:auth.core", 1) {
		t.Fatalf("expected the synthesized analysis finding to carry the run-scoped id, got %+v", analysisState.Result.Findings)
	}
	// The finding must be self-contained (issue #40): the detail carries the
	// shared finding block, not only the analysis enums.
	detail := analysisState.Result.Findings[0].Detail
	for _, want := range []string{"problem:", "evidence:", "- spec:", "- code:", "impact:", "fix:", "root_cause:", "direction:"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("expected the analysis finding detail to carry the self-contained finding block (%q), got:\n%s", want, detail)
		}
	}

	var contextOut, contextErr bytes.Buffer
	if err := runGatePacket([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "cross"}, &contextOut, &contextErr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(contextOut.String(), "detect:auth.core") || !strings.Contains(contextOut.String(), "analysis:auth.core") ||
		!strings.Contains(contextOut.String(), "MISMATCH (acceptance) — broken") || !strings.Contains(contextOut.String(), "Root cause: incomplete") {
		t.Fatalf("expected cross context to include both structured results and verbatim accepted reports, got:\n%s", contextOut.String())
	}
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	state, err := gaterun.LoadPacketState(repoRoot, mustLoadRun(t, repoRoot, runID), "cross")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ConsumedResultDigests) != 2 {
		t.Fatalf("expected cross to bind both result digests, got %v", state.ConsumedResultDigests)
	}
}

// TestVerifyAnalysisBindsOnlyDetectionDigestInDelta pins the result-binding
// contract: an analysis packet consumes exactly its detection result, so its
// consumed digests must not include the run's carried judgments (only cross
// consumes those).
func TestVerifyAnalysisBindsOnlyDetectionDigestInDelta(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Login() {}\n")

	fullID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullID, "auth.login", grVerifyItemRegionReport("auth.login", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, fullID, "auth.logout", grVerifyItemRegionReport("auth.logout", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, fullID, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, fullID)

	data, _ := os.ReadFile(specPath)
	edited := strings.Replace(string(data), "  - id: auth.login\n    description: Behavior.", "  - id: auth.login\n    description: Behavior, edited.", 1)
	if edited == string(data) {
		t.Fatal("item edit did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if strings.Join(deltaRun.CarriedKeys, ",") != "auth.logout" {
		t.Fatalf("expected auth.logout carried over, got %v", deltaRun.CarriedKeys)
	}
	grSubmitOK(t, repoRoot, deltaID, "auth.login", grVerifyItemBody("auth.login", "MISMATCH (acceptance)", "broken at src/auth.go:1")+"auth.login: "+main+": acceptance_item:auth.login\nauth.login: src/auth.go: all\n")
	grSubmitOK(t, repoRoot, deltaID, "analysis:auth.login", grVerifyAnalysisReport("auth.login", main, "src/auth.go", "P1"))

	state, err := gaterun.LoadPacketState(repoRoot, deltaRun, "analysis:auth.login")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ConsumedResultDigests) != 1 {
		t.Fatalf("expected analysis to bind only its detection result, got %v", state.ConsumedResultDigests)
	}
	if _, ok := state.ConsumedResultDigests["detect:auth.login"]; !ok {
		t.Fatalf("expected the detection digest, got %v", state.ConsumedResultDigests)
	}
}

// TestDeltaFinalizePreservesCarriedFileLevelDeps verifies that a path the
// delta run both re-declares and carries keeps the carried entry's file-level
// deps union — including the declare-heavy remainder no check owns, which the
// promote gate judges freshness on.
func TestDeltaFinalizePreservesCarriedFileLevelDeps(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	extended := string(data) + "\n## Scope\n\nScope prose.\n\n## Notes\n\nUndeclared extra prose.\n"
	if err := os.WriteFile(specPath, []byte(extended), 0644); err != nil {
		t.Fatal(err)
	}

	// Check 1 reads Description; checks 2-8 and cross read Scope. Editing
	// Description later stales only check 1, so the re-run structural group
	// and the carried checks share the main spec path.
	var decls []validationcache.CheckDeclaration
	decls = append(decls, validationcache.CheckDeclaration{Check: "1", Sections: []string{"Description"}})
	for _, key := range []string{"2", "3", "4", "5", "6", "7", "8", gaterun.CrossKey} {
		decls = append(decls, validationcache.CheckDeclaration{Check: key, Sections: []string{"Scope"}})
	}
	entry, err := validationcache.BuildEntryFromChecks(repoRoot, main, decls)
	if err != nil {
		t.Fatal(err)
	}
	// The Notes region is no check's dependency: append its CID as the
	// declare-heavy remainder of the file-level deps union.
	notesEntry, err := validationcache.BuildEntryFromChecks(repoRoot, main,
		[]validationcache.CheckDeclaration{{Check: "notes", Sections: []string{"Notes"}}})
	if err != nil {
		t.Fatal(err)
	}
	var extraDep string
	for _, dep := range notesEntry.Deps {
		if !containsString(entry.Deps, dep) {
			extraDep = dep
			break
		}
	}
	if extraDep == "" {
		t.Fatal("fixture assumption broken: no undeclared dependency in the Notes region")
	}
	entry.Deps = append(entry.Deps, extraDep)

	statuses := map[string]string{}
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", gaterun.CrossKey} {
		statuses[key] = "pass"
	}
	judgments, err := json.Marshal(gaterun.JudgmentBaseline{SchemaVersion: 2, LogicalStatus: statuses, SynthesisDigest: "sha256:test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   "validate",
		Unit:      "auth",
		Mode:      "full",
		Basis:     "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-01-01T00:00:00Z",
		Judgments: string(judgments),
		Entries:   []validationcache.FileEntry{entry},
	}); err != nil {
		t.Fatal(err)
	}

	// Edit Description only: check 1 goes stale; the Scope region (carried
	// checks 2-8) and the Notes remainder stay fresh.
	current, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(current), "Prose.", "Prose, edited.", 1)
	if edited == string(current) {
		t.Fatal("fixture assumption broken: Description edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if got := strings.Join(packetIDsOf(deltaRun), ","); got != "structural,cross" {
		t.Fatalf("expected the structural group plus cross to re-run, got %s", got)
	}
	if got := strings.Join(deltaRun.CarriedKeys, ","); got != "2,4,5,7,8" {
		t.Fatalf("expected the Scope checks carried over, got %v", deltaRun.CarriedKeys)
	}

	grSubmitOK(t, repoRoot, deltaID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Description"},
		"6": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, deltaID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, deltaID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, extraDep) {
		t.Fatalf("expected the carried file-level dep %s to survive the delta merge:\n%s", extraDep, cache)
	}
}

func TestGateFinalizeRejectsCrossDigestMismatch(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	run := mustLoadRun(t, repoRoot, runID)
	state, err := gaterun.LoadPacketState(repoRoot, run, "cross")
	if err != nil {
		t.Fatal(err)
	}
	for key := range state.ConsumedResultDigests {
		state.ConsumedResultDigests[key] = "sha256:tampered"
		break
	}
	if err := gaterun.SavePacketState(repoRoot, run, state); err != nil {
		t.Fatal(err)
	}
	if _, err := grFinalize(t, repoRoot, runID); err == nil || !strings.Contains(err.Error(), "consumed digest") {
		t.Fatalf("expected consumed-result digest mismatch to reject finalize, got %v", err)
	}
}

func TestGateRunValidateCandidateFailDeletesCache(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	// Seed an existing pass cache that the full-run failure must delete.
	seedRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, seedRun, main)
	grFinalizeOK(t, repoRoot, seedRun, "--result", "pass")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", "2. Design soundness: FAIL — contradiction\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n[P1] design — contradiction (actionable)\n\ncheck-2: "+main+": Description\ncheck-4: "+main+": Description\n")
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Testability / Acceptance Criteria"}}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	out := grFinalizeOK(t, repoRoot, runID, "--result", "fail", "--p0-count", "1")
	if !strings.Contains(out, "no failure record was written") {
		t.Fatalf("expected the delete-on-fail disclosure, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")); !os.IsNotExist(err) {
		t.Fatal("expected the candidate validate cache to be deleted on a full-run FAIL")
	}
}

// ------------------------------------------------------------
// Submit-level validation
// ------------------------------------------------------------

func TestGateSubmitValidation(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	// Empty report.
	if _, err := grSubmit(t, repoRoot, runID, "structural", ""); err == nil || !strings.Contains(err.Error(), "empty report") {
		t.Fatalf("expected an empty-report rejection, got %v", err)
	}
	// Missing verdict line.
	if _, err := grSubmit(t, repoRoot, runID, "structural", "check-1: "+main+": Description\n"); err == nil || !strings.Contains(err.Error(), "no verdict line") {
		t.Fatalf("expected a missing-verdict rejection, got %v", err)
	}
	// Missing scope line.
	if _, err := grSubmit(t, repoRoot, runID, "structural", "1. Structural integrity: PASS — ok\n3. Scope integrity: PASS — ok\n6. Affects-source validity: PASS — ok\n"); err == nil || !strings.Contains(err.Error(), "no Dependency scope line") {
		t.Fatalf("expected a missing-scope rejection, got %v", err)
	}
	// Declaration outside the snapshot.
	grWriteFile(t, repoRoot, "src/helper.go", "package helper\n")
	report := "1. Structural integrity: PASS — ok\n3. Scope integrity: PASS — ok\n6. Affects-source validity: PASS — ok\n\ncheck-1: src/helper.go: all\ncheck-3: " + main + ": Description\ncheck-6: " + main + ": Description\n"
	if _, err := grSubmit(t, repoRoot, runID, "structural", report); err == nil || !strings.Contains(err.Error(), "read refs") {
		t.Fatalf("expected a snapshot-membership rejection, got %v", err)
	}
	// Unresolvable section heading.
	report = "1. Structural integrity: PASS — ok\n3. Scope integrity: PASS — ok\n6. Affects-source validity: PASS — ok\n\ncheck-1: " + main + ": No Such Section\ncheck-3: " + main + ": Description\ncheck-6: " + main + ": Description\n"
	if _, err := grSubmit(t, repoRoot, runID, "structural", report); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a section-lookup rejection, got %v", err)
	}
	// Cross-check before its dependencies.
	if _, err := grSubmit(t, repoRoot, runID, "cross", grCrossReport(main, "Description")); err == nil || !strings.Contains(err.Error(), "dependency packet") {
		t.Fatalf("expected a dependency-order rejection, got %v", err)
	}

	// The rejected attempts are recorded; a valid submission is accepted and
	// terminal.
	grSubmitValidatePackets(t, repoRoot, runID, main)
	state, err := gaterun.LoadPacketState(repoRoot, mustLoadRun(t, repoRoot, runID), "structural")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Attempts) != 6 {
		t.Fatalf("expected 6 recorded attempts (5 rejected + 1 accepted), got %d", len(state.Attempts))
	}
	if state.Status != gaterun.PacketAccepted {
		t.Fatalf("expected the structural packet accepted, got %q", state.Status)
	}
	if _, err := grSubmit(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	})); err == nil || !strings.Contains(err.Error(), "already accepted") {
		t.Fatalf("expected the accepted packet to be terminal, got %v", err)
	}
	for i, a := range state.Attempts {
		if i < 5 && (a.Status != "rejected" || a.RejectionReason == "") {
			t.Fatalf("expected attempt %d to carry a rejection reason, got %+v", i+1, a)
		}
	}
}

func TestConcurrentGateSubmitKeepsFirstTerminalResult(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	report := grValidateReport([]string{"5"}, map[string][]string{
		"5": {main + ": Testability / Acceptance Criteria"},
	})

	reportPaths := []string{
		grWriteFile(t, repoRoot, "reports/first.md", report+"\nfirst-marker\n"),
		grWriteFile(t, repoRoot, "reports/second.md", report+"\nsecond-marker\n"),
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, reportPath := range reportPaths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			<-start
			var stdout, stderr bytes.Buffer
			errs <- runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID, "--packet", "acceptance", "--report", path}, &stdout, &stderr)
		}(reportPath)
	}
	close(start)
	wg.Wait()
	close(errs)

	successes, terminalFailures := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case strings.Contains(err.Error(), "already accepted"):
			terminalFailures++
		default:
			t.Fatalf("unexpected concurrent submit result: %v", err)
		}
	}
	if successes != 1 || terminalFailures != 1 {
		t.Fatalf("expected one accepted submission and one terminal rejection, got success=%d terminal=%d", successes, terminalFailures)
	}

	run := mustLoadRun(t, repoRoot, runID)
	state, err := gaterun.LoadPacketState(repoRoot, run, "acceptance")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != gaterun.PacketAccepted || len(state.Attempts) != 1 {
		t.Fatalf("expected one immutable accepted attempt, got status=%s attempts=%d", state.Status, len(state.Attempts))
	}
	first := strings.Contains(state.Report, "first-marker")
	second := strings.Contains(state.Report, "second-marker")
	if first == second {
		t.Fatalf("expected exactly one submitted report to persist, got report:\n%s", state.Report)
	}
}

func TestGateFinalizeLoadsRunInsideMutationLock(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")

	lockHeld := make(chan struct{})
	deleteRun := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- gaterun.WithMutation(repoRoot, func() error {
			close(lockHeld)
			<-deleteRun
			run, err := gaterun.Load(repoRoot, runID)
			if err != nil {
				return err
			}
			return gaterun.Delete(repoRoot, run)
		})
	}()
	<-lockHeld

	finalizeStarted := make(chan struct{})
	finalizeDone := make(chan error, 1)
	go func() {
		close(finalizeStarted)
		var stdout, stderr bytes.Buffer
		finalizeDone <- runGateFinalize([]string{"--repo-root", repoRoot, "--run", runID}, &stdout, &stderr)
	}()
	<-finalizeStarted
	close(deleteRun)
	if err := <-holderDone; err != nil {
		t.Fatalf("replace run while holding mutation lock: %v", err)
	}
	if err := <-finalizeDone; err == nil || !strings.Contains(err.Error(), "cannot read gate run") {
		t.Fatalf("expected finalize to reload after replacement and reject the deleted run, got %v", err)
	}
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("replaced run must not publish a cache, stat err=%v", err)
	}
}

func mustLoadRun(t *testing.T, repoRoot, runID string) *gaterun.Run {
	t.Helper()
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

// ------------------------------------------------------------
// Coverage and publication
// ------------------------------------------------------------

func TestGateRunAppendixCoverage(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nPOST /login.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, main)
	if _, err := grFinalize(t, repoRoot, runID, "--result", "pass"); err == nil || !strings.Contains(err.Error(), "appendix") {
		t.Fatalf("expected the appendix gate to reject the write, got %v", err)
	}
	if _, serr := os.Stat(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")); !os.IsNotExist(serr) {
		t.Fatal("expected no cache to survive the appendix rejection")
	}

	// A new plan whose structural packet declares the appendix passes.
	runID = grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, main, appendix+": all")
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected FRESH after declaring the appendix, got: %s", res.Reason)
	}

	// A later rejected finalize must not destroy or rewrite the last valid
	// canonical cache. Validation happens against the rendered candidate and
	// publication is the final step.
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	runID = grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, main)
	if _, err := grFinalize(t, repoRoot, runID, "--result", "pass"); err == nil || !strings.Contains(err.Error(), "appendix") {
		t.Fatalf("expected the later appendix gate to reject the candidate, got %v", err)
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("rejected finalize changed the prior canonical cache")
	}
	res, err = validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected the preserved cache to remain FRESH, got: %s", res.Reason)
	}

	// A valid later candidate replaces the canonical cache.
	runID = grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, main, appendix+": all")
	grFinalizeOK(t, repoRoot, runID, "--result", "pass", "--timestamp", "2026-09-17T12:34:56Z")
	replaced, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(replaced, before) || !bytes.Contains(replaced, []byte(`timestamp: "2026-09-17T12:34:56Z"`)) {
		t.Fatal("successful finalize did not replace the canonical cache")
	}
}

func TestGateRunRequiresMainFileCoverage(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "self")
	main := "docs/specs/units/candidate/unit_self.md"
	appendix := "docs/specs/units/candidate/appendix/unit_self_protocol.md"
	grWriteFile(t, repoRoot, appendix, "---\nunit: self\nstatus: active\n---\n\n# Protocol\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "self", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {appendix + ": all"},
		"3": {appendix + ": all"},
		"6": {appendix + ": all"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {appendix + ": all"},
		"4": {appendix + ": all"},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {appendix + ": all"}}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {appendix + ": all"},
		"8": {appendix + ": all"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(appendix, "all"))
	if _, err := grFinalize(t, repoRoot, runID, "--result", "pass"); err == nil || !strings.Contains(err.Error(), "required file") {
		t.Fatalf("expected the required-file coverage check to reject, got %v", err)
	}
	if _, serr := os.Stat(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self/validate_result.md")); !os.IsNotExist(serr) {
		t.Fatal("expected no cache file to be left behind")
	}
	_ = main
}

// ------------------------------------------------------------
// Snapshot divergence
// ------------------------------------------------------------

func TestEvidenceSnapshotDivergencesBindsEntryHash(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	expected, ok := run.SnapshotHash(repoRoot, "src/auth.go")
	if !ok || expected == "" {
		t.Fatalf("planned surface has no snapshot hash: hash=%q ok=%t", expected, ok)
	}
	fresh := map[string]bool{"src/auth.go": true}
	matching := []validationcache.FileEntry{{Path: "src/auth.go", Hash: "sha256:" + expected}}
	if got := evidenceSnapshotDivergences(repoRoot, run, matching, fresh); len(got) != 0 {
		t.Fatalf("matching evidence unexpectedly diverged: %v", got)
	}
	mismatched := []validationcache.FileEntry{{Path: "src/auth.go", Hash: "sha256:different"}}
	if got := evidenceSnapshotDivergences(repoRoot, run, mismatched, fresh); !reflect.DeepEqual(got, []string{"evidence differs from snapshot: src/auth.go"}) {
		t.Fatalf("unexpected mismatch result: %v", got)
	}
	// A carried-only entry keeps its baseline hash: a whole-file change
	// outside the declared deps is a documented fresh state, so the entry
	// must be bound by snapshot presence, not by hash equality.
	carriedOnly := []validationcache.FileEntry{{Path: "src/auth.go", Hash: "sha256:baseline-hash"}}
	if got := evidenceSnapshotDivergences(repoRoot, run, carriedOnly, map[string]bool{}); len(got) != 0 {
		t.Fatalf("carried-only evidence must not be hash-bound to the snapshot: %v", got)
	}
	missing := []validationcache.FileEntry{{Path: "src/not-planned.go", Hash: expected}}
	if got := evidenceSnapshotDivergences(repoRoot, run, missing, fresh); !reflect.DeepEqual(got, []string{"evidence path absent from snapshot: src/not-planned.go"}) {
		t.Fatalf("unexpected missing-path result: %v", got)
	}
}

func TestGateRunSnapshotDivergences(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	cases := []struct {
		name    string
		plan    []string
		packets func(runID string)
		mutate  func()
		want    string
	}{
		{
			name: "modified spec",
			plan: []string{"--gate", "validate", "--unit", "auth", "--target", "candidate"},
			packets: func(runID string) {
				grSubmitValidatePackets(t, repoRoot, runID, main, appendix+": all")
			},
			mutate: func() {
				data, _ := os.ReadFile(specPath)
				os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose, edited.", 1)), 0644)
			},
			want: "modified: " + main,
		},
		{
			name: "modified appendix",
			plan: []string{"--gate", "validate", "--unit", "auth", "--target", "candidate"},
			packets: func(runID string) {
				grSubmitValidatePackets(t, repoRoot, runID, main, appendix+": all")
			},
			mutate: func() {
				grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nPOST /login.\n")
			},
			want: "modified: " + appendix,
		},
		{
			name: "modified code",
			plan: []string{"--gate", "verify", "--unit", "auth", "--target", "candidate"},
			packets: func(runID string) {
				grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
				grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
			},
			mutate: func() { grWriteFile(t, repoRoot, "src/auth.go", "package auth // edited\n") },
			want:   "modified: src/auth.go",
		},
		{
			name: "added surface file",
			plan: []string{"--gate", "verify", "--unit", "auth", "--target", "candidate"},
			packets: func(runID string) {
				grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
				grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
			},
			mutate: func() { grWriteFile(t, repoRoot, "src/extra.go", "package auth\n") },
			want:   "added: src/extra.go",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, tc.plan...)
			tc.packets(runID)
			tc.mutate()
			_, err := grFinalize(t, repoRoot, runID, "--result", "pass")
			if err == nil {
				t.Fatal("expected the finalize to be rejected after the input changed")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q in the rejection, got: %v", tc.want, err)
			}
			if _, serr := os.Stat(filepath.Join(repoRoot, "meta/gate_runs", runID)); !os.IsNotExist(serr) {
				t.Fatal("expected the discarded run state to be removed")
			}
		})
	}
}

func TestGateRunRejectsLogicalRefLayerMove(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteSpecWithRefs(t, repoRoot, "self", "auth", "none")
	main := "docs/specs/units/candidate/unit_self.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "self", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {"unit:auth: Description"},
		"8": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))

	// Simulate a concurrent promote of the dependency unit.
	depCandidate := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	depStable := filepath.Join(repoRoot, "docs/specs/units/stable/unit_auth.md")
	data, _ := os.ReadFile(depCandidate)
	os.MkdirAll(filepath.Dir(depStable), 0755)
	os.WriteFile(depStable, data, 0644)
	os.Remove(depCandidate)

	_, err := grFinalize(t, repoRoot, runID, "--result", "pass")
	if err == nil || !strings.Contains(err.Error(), "layer resolution changed: unit:auth") {
		t.Fatalf("expected a layer-resolution divergence, got %v", err)
	}
}

// ------------------------------------------------------------
// Delta and repair
// ------------------------------------------------------------

func TestGateRunDeltaFlow(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, fullRun, main)
	grFinalizeOK(t, repoRoot, fullRun, "--result", "pass")

	// Editing the Description section stales the checks that declared it.
	data, _ := os.ReadFile(specPath)
	os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose, edited.", 1)), 0644)

	deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if got := packetIDsOf(run); strings.Join(got, ",") != "structural,design,dependencies,cross" {
		t.Fatalf("expected the affected groups + cross, got %v", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "5" {
		t.Fatalf("expected check 5 carried over, got %v", run.CarriedKeys)
	}

	// Only the planned packets may be submitted.
	if _, err := grSubmit(t, repoRoot, deltaRun, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Testability / Acceptance Criteria"}})); err == nil || !strings.Contains(err.Error(), "not part of") {
		t.Fatalf("expected the carried packet to be absent from the plan, got %v", err)
	}
	grSubmitOK(t, repoRoot, deltaRun, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, deltaRun, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, deltaRun, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, deltaRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, deltaRun, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected basis: delta, got:\n%s", cache)
	}
	if !strings.Contains(cache, `check: "5"`) {
		t.Fatalf("expected the carried check 5 evidence, got:\n%s", cache)
	}
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected the delta cache to be fresh, got: %s", res.Reason)
	}

	// A delta plan with nothing stale is refused.
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta"); err == nil || !strings.Contains(err.Error(), "cache is fresh") {
		t.Fatalf("expected the freshness refusal, got %v", err)
	}
}

func TestGateRunRuleConsecutivePartialDeltasKeepCompleteJudgments(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")
	consumerPath := grWriteSpecWithRefs(t, repoRoot, "auth", "none", "b_rule_http")

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	fullScopes := map[string][]string{}
	for c := 1; c <= 8; c++ {
		key := fmt.Sprint(c)
		fullScopes[key] = []string{rulePath + ": Constraint"}
	}
	fullScopes["5"] = []string{"unit:auth: Testability / Acceptance Criteria"}
	fullScopes["7"] = []string{"unit:auth: Testability / Acceptance Criteria"}
	grSubmitOK(t, repoRoot, fullRun, "checks", grValidateReport([]string{"1", "2", "3", "4", "5", "6", "7", "8"}, fullScopes))
	grFinalizeOK(t, repoRoot, fullRun)

	runDelta := func(oldText, newText string) {
		t.Helper()
		data, err := os.ReadFile(consumerPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(consumerPath, []byte(strings.Replace(string(data), oldText, newText, 1)), 0644); err != nil {
			t.Fatal(err)
		}

		deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate", "--mode", "delta")
		run := mustLoadRun(t, repoRoot, deltaRun)
		if got := strings.Join(run.Packets[0].CheckKeys, ","); got != "5,7" {
			t.Fatalf("expected only consumer checks 5 and 7 to re-run, got %s", got)
		}
		if got := strings.Join(run.CarriedKeys, ","); got != "1,2,3,4,6,8" {
			t.Fatalf("unexpected carried checks: %s", got)
		}
		grSubmitOK(t, repoRoot, deltaRun, "checks", grValidateReport([]string{"5", "7"}, map[string][]string{
			"5": {"unit:auth: Testability / Acceptance Criteria"},
			"7": {"unit:auth: Testability / Acceptance Criteria"},
		}))
		grFinalizeOK(t, repoRoot, deltaRun)

		state := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindRule, "b_rule_http", gaterun.GateValidate)
		if len(state.LogicalStatus) != 8 {
			t.Fatalf("expected a complete 8-check judgment baseline after delta, got %v", state.LogicalStatus)
		}
		for c := 1; c <= 8; c++ {
			if state.LogicalStatus[fmt.Sprint(c)] != "pass" {
				t.Fatalf("expected check %d status pass after delta, got %q", c, state.LogicalStatus[fmt.Sprint(c)])
			}
		}
	}

	runDelta("Behavior.", "Behavior changed.")
	runDelta("Behavior changed.", "Behavior changed again.")
}

func TestRuleOutcomeRejectsCarriedRerunOverlap(t *testing.T) {
	run := &gaterun.Run{
		TargetKind:  gaterun.TargetKindRule,
		CarriedKeys: []string{"1"},
		CarriedResults: []gaterun.PacketResult{{
			PacketID:        "carried:1",
			EffectiveStatus: map[string]string{"1": "pass"},
		}},
	}
	report := reportRef{
		spec: &gaterun.PacketSpec{PacketID: "checks", Kind: gaterun.PacketKindChecks},
		result: &gaterun.PacketResult{
			PacketID: "checks",
			Verdicts: map[string]string{"1": "PASS"},
		},
	}
	if _, err := deriveGateOutcome("", run, []reportRef{report}); err == nil || !strings.Contains(err.Error(), "both carried and re-run") {
		t.Fatalf("expected carried/re-run overlap to fail closed, got %v", err)
	}
}

func TestRuleOutcomeDeduplicatesCarriedFindings(t *testing.T) {
	shared := gaterun.Finding{ID: "checks/F1", Severity: "P1", Text: "shared", SourceKey: "1", AffectedKeys: []string{"2"}}
	run := &gaterun.Run{TargetKind: gaterun.TargetKindRule}
	for i := 1; i <= 7; i++ {
		key := fmt.Sprint(i)
		run.CarriedKeys = append(run.CarriedKeys, key)
		status := "pass"
		if i == 1 || i == 2 {
			status = "fail"
		}
		result := gaterun.PacketResult{PacketID: "carried:" + key, EffectiveStatus: map[string]string{key: status}}
		if i == 1 || i == 2 {
			result.Findings = []gaterun.Finding{shared}
		}
		run.CarriedResults = append(run.CarriedResults, result)
	}
	report := reportRef{
		spec: &gaterun.PacketSpec{PacketID: "checks", Kind: gaterun.PacketKindChecks},
		result: &gaterun.PacketResult{
			PacketID: "checks",
			Verdicts: map[string]string{"8": "PASS"},
		},
	}
	outcome, err := deriveGateOutcome("", run, []reportRef{report})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Findings) != 1 || outcome.Counts[1] != 1 || len(outcome.EffectiveStatus) != 8 || outcome.SynthesisDigest == "" {
		t.Fatalf("expected one canonical carried finding and a complete outcome, got %+v", outcome)
	}
}

func TestCandidateValidateDeltaFailPreservesRecoveryRecord(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, fullRun, main)
	grFinalizeOK(t, repoRoot, fullRun)

	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "## Description\n\nProse.", "## Description\n\nProse changed.", 1)
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	for _, packet := range run.Packets {
		if packet.Kind == gaterun.PacketKindCross {
			continue
		}
		scopes := map[string][]string{}
		for _, key := range packet.CheckKeys {
			scopes[key] = []string{main + ": Description"}
		}
		report := grValidateReport(packet.CheckKeys, scopes)
		if packet.PacketID == "design" {
			report = strings.Replace(report, "2. Design soundness: PASS — checked", "2. Design soundness: FAIL — combined design contradiction", 1)
			report += "\n[P1] design — combined design contradiction (actionable)\n"
		}
		grSubmitOK(t, repoRoot, deltaRun, packet.PacketID, report)
	}
	grSubmitOK(t, repoRoot, deltaRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, deltaRun)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "basis: delta") || !strings.Contains(cache, "blocking: true") {
		t.Fatalf("expected candidate validate delta FAIL to write a recovery record, got:\n%s", cache)
	}
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair"); err != nil {
		t.Fatalf("expected the preserved record to support repair planning, got %v", err)
	}
}

func TestGateRunRepairFlow(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	fullRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "auth.login", grVerifyItemBody("auth.login", "MISMATCH (acceptance)", "broken at src/auth.go:1")+"auth.login: "+main+": Testability / Acceptance Criteria\nauth.login: src/auth.go: all\n")
	grSubmitOK(t, repoRoot, fullRun, "analysis:auth.login", grVerifyAnalysisReport("auth.login", main, "src/auth.go", "P0"))
	grSubmitOK(t, repoRoot, fullRun, "auth.logout", grVerifyItemReport("auth.logout", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun, "--result", "fail", "--p0-count", "1")

	repairRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, repoRoot, repairRun)
	if got := packetIDsOf(run); strings.Join(got, ",") != "detect:auth.login,analysis:auth.login,cross" {
		t.Fatalf("expected the failed item + cross, got %v", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "auth.logout" {
		t.Fatalf("expected auth.logout carried over, got %v", run.CarriedKeys)
	}
	grSubmitOK(t, repoRoot, repairRun, "auth.login", grVerifyItemReport("auth.login", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, repairRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, repairRun, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "basis: repair") || !strings.Contains(cache, `check: "auth.logout"`) {
		t.Fatalf("expected a repair cache carrying auth.logout, got:\n%s", cache)
	}
	res, err := validationcache.CheckVerify(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected the repair cache to be fresh, got: %s", res.Reason)
	}
}

// ------------------------------------------------------------
// Status, usage, lifecycle
// ------------------------------------------------------------

func TestGateStatusReportsProgress(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))

	var stdout, stderr bytes.Buffer
	if err := runGateStatus([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), runID) || !strings.Contains(stdout.String(), "1 accepted, 4 pending") {
		t.Fatalf("unexpected status listing:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := runGateStatus([]string{"--repo-root", repoRoot, "--run", runID}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "accepted") || !strings.Contains(out, "gate-submit") {
		t.Fatalf("expected packet detail and the next submit action, got:\n%s", out)
	}
	if !strings.Contains(out, "next rejection") && !strings.Contains(out, "pending") {
		t.Fatalf("expected pending packet detail, got:\n%s", out)
	}
}

func TestGateRunUsageErrors(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")

	planCases := []struct {
		name string
		args []string
		want string
	}{
		{"missing gate", []string{"--unit", "auth", "--target", "candidate"}, "must be validate, verify, or review"},
		{"missing target", []string{"--gate", "validate", "--unit", "auth"}, "candidate or stable"},
		{"mutually exclusive", []string{"--gate", "validate", "--unit", "auth", "--rule", "b_rule_http", "--target", "candidate"}, "mutually exclusive"},
		{"invalid mode", []string{"--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "nope"}, "full, delta, or repair"},
		{"traversal rule name", []string{"--gate", "validate", "--rule", "../../../tmp/evil", "--target", "candidate"}, "is invalid"},
		{"traversal unit name", []string{"--gate", "validate", "--unit", "../../auth", "--target", "candidate"}, "is invalid"},
	}
	for _, tc := range planCases {
		t.Run("plan: "+tc.name, func(t *testing.T) {
			if _, err := grPlanRaw(repoRoot, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	finalizeCases := []struct {
		name string
		args []string
		want string
	}{
		{"missing run", nil, "--run is required"},
		{"removed judgment flag", []string{"--run", runID, "--result", "maybe"}, "flag provided but not defined"},
	}
	for _, tc := range finalizeCases {
		t.Run("finalize: "+tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"--repo-root", repoRoot}, tc.args...)
			if err := runGateFinalize(args, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}

	// gate-submit requires its flags.
	var stdout, stderr bytes.Buffer
	if err := runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "--packet") {
		t.Fatalf("expected the gate-submit usage error, got %v", err)
	}
}

func TestGateRunConsumedRunCloses(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, main)
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	if _, err := grFinalize(t, repoRoot, runID, "--result", "pass"); err == nil || !strings.Contains(err.Error(), "consumed") {
		t.Fatalf("expected a consumed run to reject a second finalize, got %v", err)
	}
	if _, err := grSubmit(t, repoRoot, runID, "cross", grCrossReport(main, "Description")); err == nil || !strings.Contains(err.Error(), "consumed") {
		t.Fatalf("expected a consumed run to reject submissions, got %v", err)
	}
}

func TestGateRunReplanReplacesPreviousRun(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")

	first := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	second := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	if first == second {
		t.Fatal("expected distinct run ids")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "meta/gate_runs", first)); !os.IsNotExist(err) {
		t.Fatal("expected the replaced run state directory to be deleted")
	}
	grSubmitValidatePackets(t, repoRoot, second, "docs/specs/units/candidate/unit_auth.md")
	grFinalizeOK(t, repoRoot, second, "--result", "pass")
}

func TestGateRunRecordsAuditRunID(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "gate_run: "+runID) {
		t.Fatalf("expected the audit run id, got:\n%s", cache)
	}
}

func TestGateRunRoundTripWithFresh(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	var stdout, stderr bytes.Buffer
	if err := runFresh([]string{"--repo-root", repoRoot, "--unit", "auth"}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "validate  FRESH") {
		t.Fatalf("expected validate FRESH in fresh output, got:\n%s", out)
	}
}

func TestGateRunContentEditStales(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidatePackets(t, repoRoot, runID, main)
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	data, _ := os.ReadFile(specPath)
	os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose, edited.", 1)), 0644)

	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if res.Fresh {
		t.Fatal("expected the cache to go stale after editing a declared section")
	}
	scope, err := validationcache.DeriveStaleScope(repoRoot, "unit", "auth", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Affected) == 0 || !containsStr(scope.Affected, "1") {
		t.Fatalf("expected check 1 among the affected checks, got %v", scope.Affected)
	}
}

func containsStr(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------
// Stable-only targets
// ------------------------------------------------------------

func TestGateRunStableReview(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := "docs/specs/units/stable/unit_auth.md"
	grWriteFile(t, repoRoot, main, "---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Auth\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/auth.go\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "stable")
	run := mustLoadRun(t, repoRoot, runID)
	if len(run.Packets) != 2 || run.Packets[0].PacketID != "src/auth.go" || run.Packets[1].PacketID != "cross" {
		t.Fatalf("expected a file packet + cross for a stable review, got %v", packetIDsOf(run))
	}
	grSubmitOK(t, repoRoot, runID, "src/auth.go", grReviewReport("src/auth.go", main))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	res, err := validationcache.CheckReviewStable(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected the stable review cache fresh, got: %s", res.Reason)
	}
}

func TestGatePlanStableRequiresStableOnly(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "docs/specs/units/stable/unit_auth.md", "---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Auth\n")
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "stable"); err == nil || !strings.Contains(err.Error(), "stable-only") {
		t.Fatalf("expected a stable-only error, got %v", err)
	}
}

// ------------------------------------------------------------
// Path-form validation
// ------------------------------------------------------------

func TestGateSubmitRejectsPhysicalNameResolvedPaths(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteSpec(t, repoRoot, "self")
	grWriteFile(t, repoRoot, "docs/specs/rules/candidate/g_rule_repo.md", "---\nid: g_rule_repo\nversion: 0.1.0\nscope: global\n---\n\n# Rule\n")

	cases := []struct {
		name    string
		gate    string
		kind    string
		target  string
		packet  string
		report  string
		wantErr string
	}{
		{
			name: "cross-unit main spec in unit cache", gate: "validate", kind: "unit", target: "self",
			packet:  "dependencies",
			report:  grValidateReport([]string{"7", "8"}, map[string][]string{"7": {"docs/specs/units/candidate/unit_auth.md: Description"}, "8": {"docs/specs/units/candidate/unit_self.md: Description"}}),
			wantErr: "unit:auth",
		},
		{
			name: "rule file in unit cache", gate: "validate", kind: "unit", target: "self",
			packet:  "dependencies",
			report:  grValidateReport([]string{"7", "8"}, map[string][]string{"7": {"docs/specs/rules/candidate/g_rule_repo.md: all"}, "8": {"docs/specs/units/candidate/unit_self.md: Description"}}),
			wantErr: "rule:g_rule_repo",
		},
		{
			name: "unit spec in rule cache", gate: "validate", kind: "rule", target: "g_rule_repo",
			packet:  "checks",
			report:  "1. Structural integrity: PASS — ok\n2. Design soundness: PASS — ok\n3. Scope integrity: PASS — ok\n4. Evidence-driven vs design-driven consistency: PASS — ok\n5. Acceptance coverage & correctness: PASS — ok\n6. Affects-source validity: PASS — ok\n7. Cross-unit consistency: PASS — ok\n8. Constraint alignment: PASS — ok\n\ncheck-1: docs/specs/units/candidate/unit_auth.md: all\ncheck-2: docs/specs/rules/candidate/g_rule_repo.md: Constraint\ncheck-3: docs/specs/rules/candidate/g_rule_repo.md: Constraint\ncheck-4: docs/specs/rules/candidate/g_rule_repo.md: Constraint\ncheck-5: docs/specs/rules/candidate/g_rule_repo.md: Constraint\ncheck-6: docs/specs/rules/candidate/g_rule_repo.md: Constraint\ncheck-7: docs/specs/rules/candidate/g_rule_repo.md: Constraint\ncheck-8: docs/specs/rules/candidate/g_rule_repo.md: Constraint\n",
			wantErr: "unit:auth",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, "--gate", tc.gate, "--"+tc.kind, tc.target, "--target", "candidate")
			_, err := grSubmit(t, repoRoot, runID, tc.packet, tc.report)
			if err == nil {
				t.Fatal("expected the submission to be rejected")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error mentioning %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestGateRunExtraInputRequiredForOutsideReads(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	grWriteFile(t, repoRoot, "tests/auth_test.go", "package tests\n")

	report := grVerifyItemBody("auth.core", "ALIGNED", "src/auth.go:1") + "auth.core: " + main + ": Testability / Acceptance Criteria\nauth.core: tests/auth_test.go: all\n"

	// A test file outside the derived code surface is rejected unless the
	// plan declared it with --input.
	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	if _, err := grSubmit(t, repoRoot, runID, "auth.core", report); err == nil || !strings.Contains(err.Error(), "read refs") {
		t.Fatalf("expected the outside read to be rejected, got %v", err)
	}

	runID = grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--input", "tests")
	grSubmitOK(t, repoRoot, runID, "auth.core", report)
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")
}

func TestGateRunExtraInputsAreEvidenceNotReviewTargets(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	grWriteFile(t, repoRoot, "tests/auth_test.go", "package tests\n")
	grWriteFile(t, repoRoot, "notes/evidence.txt", "evidence\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate", "--input", "tests", "--input", "notes/evidence.txt")
	run := mustLoadRun(t, repoRoot, runID)
	if got := strings.Join(packetIDsOf(run), ","); got != "src/auth.go,cross" {
		t.Fatalf("extra inputs must not create review packets, got %s", got)
	}
	filePacket := run.PacketByID("src/auth.go")
	for _, want := range []string{"tests/auth_test.go", "notes/evidence.txt"} {
		if !containsString(filePacket.ReadRefs, want) {
			t.Fatalf("expected extra evidence %s in packet read refs, got %v", want, filePacket.ReadRefs)
		}
	}
}

func TestGateRunOverlappingExtraInputIsAvailableToEveryPacket(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/helper.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate", "--input", "src")
	run := mustLoadRun(t, repoRoot, runID)
	for _, packetID := range []string{"src/auth.go", "src/helper.go"} {
		packet := run.PacketByID(packetID)
		if packet == nil {
			t.Fatalf("missing review packet %s", packetID)
		}
		for _, want := range []string{"src/auth.go", "src/helper.go"} {
			if !containsString(packet.ReadRefs, want) {
				t.Fatalf("explicit --input src must make %s available to %s, got %v", want, packetID, packet.ReadRefs)
			}
		}
	}
}

func TestGateRunOverlappingLogicalInputIsAvailableToEveryPacket(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteSpecWithRefs(t, repoRoot, "self", "auth", "none")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "self", "--target", "candidate", "--input", "unit:auth")
	run := mustLoadRun(t, repoRoot, runID)
	for _, packetID := range []string{"structural", "design", "acceptance", "dependencies", "cross"} {
		packet := run.PacketByID(packetID)
		if packet == nil || !containsString(packet.ReadRefs, "unit:auth") {
			t.Fatalf("explicit logical input must be available to %s, got %+v", packetID, packet)
		}
	}
}

func TestGateSubmitEnforcesPacketLocalReadRefs(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/helper.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	main := "docs/specs/units/candidate/unit_auth.md"
	report := grReviewArchitecture("acceptable") + "\nsrc/auth.go: " + main + ": Description\nsrc/auth.go: src/helper.go: all\n"
	if _, err := grSubmit(t, repoRoot, runID, "src/auth.go", report); err == nil || !strings.Contains(err.Error(), "packet \"src/auth.go\"'s read refs") {
		t.Fatalf("expected a run-wide but packet-local out-of-scope declaration to be rejected, got %v", err)
	}
}

// grVerifyItemRegionReport builds a verify item packet report whose own-spec
// declaration is the item region (`acceptance_item:<id>`).
func grVerifyItemRegionReport(item, specPath, codeFile string) string {
	return grVerifyItemBody(item, "ALIGNED", codeFile+":1") + fmt.Sprintf("%s: %s: acceptance_item:%s\n%s: %s: all\n", item, specPath, item, item, codeFile)
}

func TestGateRunVerifyItemLevelDelta(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Login() {}\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.login", grVerifyItemRegionReport("auth.login", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "auth.logout", grVerifyItemRegionReport("auth.logout", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	for _, want := range []string{"region:acceptance_item:auth.login:", "region:acceptance_item:auth.logout:", "region:acceptance_items:"} {
		if !strings.Contains(cache, want) {
			t.Fatalf("expected %q in the verify cache, got:\n%s", want, cache)
		}
	}

	// Reorder the complete item blocks without changing their content. Both
	// item-level evidence and the cross-check's semantic whole-set evidence
	// must stay fresh.
	text, err := contenthash.FileText(specPath)
	if err != nil {
		t.Fatal(err)
	}
	loginRegion, ok := contenthash.LocateAcceptanceItemRegion(text, "auth.login")
	if !ok {
		t.Fatal("auth.login region not found")
	}
	logoutRegion, ok := contenthash.LocateAcceptanceItemRegion(text, "auth.logout")
	if !ok {
		t.Fatal("auth.logout region not found")
	}
	swapped := strings.Replace(text, loginRegion.Text+"\n"+logoutRegion.Text, logoutRegion.Text+"\n"+loginRegion.Text, 1)
	if swapped == text {
		swapped = strings.Replace(text, loginRegion.Text+"\n\n"+logoutRegion.Text, logoutRegion.Text+"\n\n"+loginRegion.Text, 1)
	}
	if swapped == text {
		t.Fatal("item reorder did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(swapped), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := validationcache.CheckVerify(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("verify cache must remain fresh after item reorder: %s", res.Reason)
	}

	// Edit only the auth.login item — auth.logout's evidence is unchanged.
	data, _ := os.ReadFile(specPath)
	edited := strings.Replace(string(data), "  - id: auth.login\n    description: Behavior.", "  - id: auth.login\n    description: Behavior, edited.", 1)
	if edited == string(data) {
		t.Fatal("item edit did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if got := packetIDsOf(run); strings.Join(got, ",") != "detect:auth.login,analysis:auth.login,cross" {
		t.Fatalf("expected only auth.login + cross re-run, got %v", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "auth.logout" {
		t.Fatalf("expected auth.logout carried over, got %v", run.CarriedKeys)
	}

	grSubmitOK(t, repoRoot, deltaRun, "auth.login", grVerifyItemRegionReport("auth.login", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, deltaRun, "cross", fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: acceptance_items\n", main))
	grFinalizeOK(t, repoRoot, deltaRun, "--result", "pass")

	cache = grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected basis: delta, got:\n%s", cache)
	}
	res, err = validationcache.CheckVerify(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected the delta cache fresh, got: %s", res.Reason)
	}
}

func TestGateRunLogicalRefItemRegionLayerMove(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	depPath := grWriteSpec(t, repoRoot, "auth") // dependency unit
	grWriteSpecWithRefs(t, repoRoot, "self", "auth", "none")
	main := "docs/specs/units/candidate/unit_self.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "self", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": frontmatter", main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {"unit:auth: acceptance_item:auth.core"},
		"8": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/self/validate_result.md")
	if !strings.Contains(cache, "region:acceptance_item:auth.core:") {
		t.Fatalf("expected the item-region dep on the logical reference, got:\n%s", cache)
	}

	// Promote the dependency unit (the candidate file disappears; the stable
	// file holds identical content) — the item region must re-locate against
	// the current layer and stay fresh.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	if err := os.Rename(depPath, filepath.Join(stableDir, "unit_auth.md")); err != nil {
		t.Fatal(err)
	}

	res, err := validationcache.CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected FRESH after the dependency layer move, got: %s", res.Reason)
	}
}

func TestParseDecl(t *testing.T) {
	cases := []struct {
		decl    string
		want    declParts
		wantErr bool
	}{
		{decl: "", want: declParts{WholeFile: true}},
		{decl: "all", want: declParts{WholeFile: true}},
		{decl: "120-180,300-320", want: declParts{Ranges: []string{"120-180,300-320"}}},
		{decl: "acceptance_items", want: declParts{Accepts: true}},
		{decl: "acceptance_item:auth.login", want: declParts{Items: []string{"auth.login"}}},
		{decl: "acceptance_item:auth.login, auth.logout", want: declParts{Items: []string{"auth.login", "auth.logout"}}},
		{decl: "Testability / Acceptance Criteria", want: declParts{Sections: []string{"Testability / Acceptance Criteria"}}},
		{decl: "acceptance_item:", wantErr: true},
		{decl: "acceptance_item:auth.login,,auth.logout", wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseDecl(tc.decl)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseDecl(%q): expected an error", tc.decl)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseDecl(%q): %v", tc.decl, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("parseDecl(%q) = %+v, want %+v", tc.decl, got, tc.want)
		}
	}

	m := &declParts{}
	mergeDecl(m, declParts{Items: []string{"a"}})
	mergeDecl(m, declParts{Accepts: true})
	mergeDecl(m, declParts{Items: []string{"b"}})
	if !m.Accepts || strings.Join(m.Items, ",") != "a,b" {
		t.Fatalf("union merge failed: %+v", m)
	}
	mergeDecl(m, declParts{WholeFile: true})
	if !m.WholeFile || m.Accepts || len(m.Items) != 0 {
		t.Fatalf("whole-file must cover every other form: %+v", m)
	}
	mergeDecl(m, declParts{Sections: []string{"Description"}})
	if !m.WholeFile || len(m.Sections) != 0 {
		t.Fatalf("a later declaration must not clear whole-file: %+v", m)
	}
}

// TestCrossNewFindingCoexistsWithCarriedFinding verifies the run-scoped
// finding id namespace: a carried finding from an earlier run and a new
// finding created by the current cross synthesis can coexist — the new id is
// {current_run}/..., the carried id is {earlier_run}/..., so the submission
// cannot collide with itself.
func TestCrossNewFindingCoexistsWithCarriedFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.alpha", "auth.beta"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/beta.go", "package auth\n")

	// Baseline run: the cross synthesis creates a retained P2 finding
	// affecting alpha (non-blocking pass cache).
	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.alpha", grVerifyItemReport("auth.alpha", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "auth.beta", grVerifyItemReport("auth.beta", main, "src/beta.go"))
	cross := "Cross-check: 4/5 PASS — combined context clarified\n\n" +
		"[P2] cross — combined claim is imprecise (actionable)\n" +
		"Finding affects: " + grRunFindingID(runID, "cross", 1) + " = auth.alpha\n\n" +
		"cross: " + main + ": Description\n" +
		"Severity confirmation: " + grRunFindingID(runID, "cross", 1) + " = confirmed P2 — evidence: " + main + "; reason: wording only\n" +
		"Effective status: auth.alpha = fail\nEffective status: auth.beta = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)

	baseline := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateVerify)
	if len(baseline.Findings) != 1 || baseline.Findings[0].ID != grRunFindingID(runID, "cross", 1) {
		t.Fatalf("expected the baseline finding to carry the run-scoped id, got %+v", baseline.Findings)
	}

	// Delta run: only beta's evidence went stale — alpha's finding is carried
	// into the new cross synthesis, which also creates a new finding.
	grWriteFile(t, repoRoot, "src/beta.go", "package auth\n// changed\n")
	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if got := strings.Join(deltaRun.CarriedKeys, ","); got != "auth.alpha" {
		t.Fatalf("expected auth.alpha carried over, got %s", got)
	}
	grSubmitOK(t, repoRoot, deltaID, "auth.beta", grVerifyItemReport("auth.beta", main, "src/beta.go"))
	deltaCross := "Cross-check: 4/5 PASS — combined context clarified\n\n" +
		"[P2] cross — combined claim is imprecise (actionable)\n" +
		"Finding affects: " + grRunFindingID(deltaID, "cross", 1) + " = auth.beta\n\n" +
		"Finding disposition: " + grRunFindingID(runID, "cross", 1) + " = retained\n" +
		"cross: " + main + ": Description\n" +
		"Severity confirmation: " + grRunFindingID(runID, "cross", 1) + " = confirmed P2 — evidence: " + main + "; reason: wording only\n" +
		"Severity confirmation: " + grRunFindingID(deltaID, "cross", 1) + " = confirmed P2 — evidence: " + main + "; reason: wording only\n" +
		"Effective status: auth.alpha = fail\nEffective status: auth.beta = fail\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, deltaID, "cross", deltaCross)
	grFinalizeOK(t, repoRoot, deltaID)

	final := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateVerify)
	if len(final.Findings) != 2 {
		t.Fatalf("expected the carried and the new finding to coexist, got %+v", final.Findings)
	}
}

// TestDeltaFinalizeUsesCarriedEvidenceSnapshot verifies that the carried
// evidence is fixed at plan time: finalize merges the snapshot and still
// succeeds after the baseline cache file is gone.
func TestDeltaFinalizeUsesCarriedEvidenceSnapshot(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.alpha", "auth.beta"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/beta.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.alpha", grVerifyItemReport("auth.alpha", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "auth.beta", grVerifyItemReport("auth.beta", main, "src/beta.go"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID)

	// Only beta's evidence goes stale; alpha is carried with its snapshot.
	grWriteFile(t, repoRoot, "src/beta.go", "package auth\n// changed\n")
	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if got := strings.Join(deltaRun.CarriedKeys, ","); got != "auth.alpha" {
		t.Fatalf("expected auth.alpha carried over, got %s", got)
	}
	if len(deltaRun.CarriedEvidence) == 0 {
		t.Fatal("expected the plan to snapshot the carried evidence")
	}

	// Remove the baseline cache before submitting the delta reports: the
	// finalize must merge the plan-time snapshot, not re-read the baseline.
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}

	grSubmitOK(t, repoRoot, deltaID, "auth.beta", grVerifyItemReport("auth.beta", main, "src/beta.go"))
	deltaCross := "Cross-check: 4/5 PASS — combined context clarified\n\ncross: " + main + ": Description\n" +
		"Effective status: auth.alpha = pass\nEffective status: auth.beta = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, deltaID, "cross", deltaCross)
	grFinalizeOK(t, repoRoot, deltaID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected the delta cache written from the snapshot, got:\n%s", cache)
	}
}

// TestValidatePacketDeclaresAffectsEvidence verifies that a spec-derived
// affects.files evidence file is part of the local validate packets' read
// refs: a check may read it and declare it in its Dependency scope.
func TestValidatePacketDeclaresAffectsEvidence(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := "docs/specs/units/candidate/unit_auth.md"
	spec := "---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n" +
		"  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n    affects:\n      files:\n        - docs/notes/auth_contract.md\n"
	grWriteFile(t, repoRoot, specPath, spec)
	grWriteFile(t, repoRoot, "docs/notes/auth_contract.md", "# Auth contract\n")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, repoRoot, runID)
	var structural *gaterun.PacketSpec
	for i := range run.Packets {
		if run.Packets[i].PacketID == "structural" {
			structural = &run.Packets[i]
		}
	}
	if structural == nil {
		t.Fatal("expected a structural packet")
	}
	found := false
	for _, ref := range structural.ReadRefs {
		if ref == "docs/notes/auth_contract.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the affects evidence file in the structural packet's read refs, got %v", structural.ReadRefs)
	}

	report := "1. Structural integrity: PASS — ok\n3. Scope integrity: PASS — ok\n6. Affects-source validity: PASS — ok\n\n" +
		"check-1: " + specPath + ": Description\n" +
		"check-3: " + specPath + ": Description\n" +
		"check-6: docs/notes/auth_contract.md: all\n"
	if _, err := grSubmit(t, repoRoot, runID, "structural", report); err != nil {
		t.Fatalf("expected the affects evidence declaration to be accepted, got %v", err)
	}
}

// TestFinalizeDiscardsRunWhenInputSurfaceUnresolvable verifies the divergence
// class where the input surface no longer resolves at all (e.g. the main spec
// was removed after plan): finalize must reject the write and discard the run.
func TestFinalizeDiscardsRunWhenInputSurfaceUnresolvable(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))

	// Remove the main spec: the derived surface no longer resolves.
	if err := os.Remove(filepath.Join(repoRoot, main)); err != nil {
		t.Fatal(err)
	}
	if _, err := grFinalize(t, repoRoot, runID); err == nil || !strings.Contains(err.Error(), "no longer resolvable") {
		t.Fatalf("expected the unresolvable input surface to reject finalize, got %v", err)
	}
	if _, err := gaterun.Load(repoRoot, runID); err == nil {
		t.Fatal("expected the run state to be discarded with the divergence")
	}
}

// TestGateSubmitNormalizesDeclarationPaths verifies that `./`-prefixed and
// mixed-spelling declarations are recorded in canonical repo-relative form, so
// gate-finalize coverage and the cache entries stay consistent (see
// framework/validation_cache.md §Format path equivalence).
func TestGateSubmitNormalizesDeclarationPaths(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	desc := "./" + main + ": Description"
	accept := "./" + main + ": Testability / Acceptance Criteria"
	front := "./" + main + ": frontmatter"
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {front, desc, main + ": Description"},
		"3": {accept},
		"6": {accept},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {desc},
		"4": {desc},
	}))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {accept},
	}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grValidateReport([]string{"7", "8"}, map[string][]string{
		"7": {desc},
		"8": {desc},
	}))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport("./"+main, "Description"))
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if got := strings.Count(cache, "- path: "+main+"\n"); got != 1 {
		t.Fatalf("expected exactly one canonical entry for the main spec, got %d:\n%s", got, cache)
	}
	if strings.Contains(cache, "- path: ./") {
		t.Fatalf("cache entries must not record a `./` spelling:\n%s", cache)
	}
}

// TestGateCrossNewFindingRequiresNonCrossAffectedKey verifies the documented
// cross-report requirement that a new cross finding names at least one
// affected non-cross logical key, so the effective-status closure stays
// derivable (see framework/verification_scope.md §Gate Work Packets).
func TestGateCrossNewFindingRequiresNonCrossAffectedKey(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "review", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "src/auth.go", grReviewReport("src/auth.go", main))
	findingID := grRunFindingID(runID, "cross", 1)
	report := "Cross-check: 4/4 PASS — consistent\n\n" +
		"[P2] cross — cross-level maintainability concern (actionable)\n" +
		"Finding affects: " + findingID + " = cross\n\n" +
		"cross: " + main + ": Description\n" +
		"Severity confirmation: " + findingID + " = confirmed P2 — evidence: " + main + "; reason: impact stays cross-level\n" +
		"Effective status: src/auth.go = pass\n" +
		"Effective status: cross = pass\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", report); err == nil || !strings.Contains(err.Error(), "at least one affected non-cross logical key") {
		t.Fatalf("expected a cross-only affected key to be rejected, got %v", err)
	}
}

// TestGateSubmitRejectsNotRequiredPacket verifies that a mechanically
// not_required analysis packet is terminal: a manual submission is rejected
// and the state stays not_required, so the run remains finalizable.
func TestGateSubmitRejectsNotRequiredPacket(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	run := mustLoadRun(t, repoRoot, runID)
	state, err := gaterun.LoadPacketState(repoRoot, run, "analysis:auth.core")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != gaterun.PacketNotRequired {
		t.Fatalf("expected the ALIGNED detection to mark the analysis packet not_required, got %s", state.Status)
	}
	if _, err := grSubmit(t, repoRoot, runID, "analysis:auth.core", grVerifyAnalysisReport("auth.core", main, "src/auth.go", "P1")); err == nil || !strings.Contains(err.Error(), "not-required") {
		t.Fatalf("expected a not_required packet to reject submissions, got %v", err)
	}
	state, err = gaterun.LoadPacketState(repoRoot, run, "analysis:auth.core")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != gaterun.PacketNotRequired {
		t.Fatalf("expected the not_required state to stay terminal, got %s", state.Status)
	}
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID)
}

// ------------------------------------------------------------
// Review-driven regression tests
// ------------------------------------------------------------

// grEntryHash extracts the recorded whole-file hash for one cache files entry.
func grEntryHash(t *testing.T, cache, path string) string {
	t.Helper()
	lines := strings.Split(cache, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "- path: "+path {
			continue
		}
		for j := i + 1; j < len(lines) && j <= i+6; j++ {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "- path: ") {
				break
			}
			if after, ok := strings.CutPrefix(trimmed, "hash: "); ok {
				return strings.TrimSpace(after)
			}
		}
	}
	t.Fatalf("cache has no hash for entry %s", path)
	return ""
}

// grGateJudgments extracts the GATE_JUDGMENTS JSON block from a cache file.
func grGateJudgments(t *testing.T, cache string) string {
	t.Helper()
	const begin = "<!-- GATE_JUDGMENTS_BEGIN\n"
	const end = "\nGATE_JUDGMENTS_END -->"
	start := strings.Index(cache, begin)
	if start < 0 {
		t.Fatalf("cache has no GATE_JUDGMENTS block:\n%s", cache)
	}
	start += len(begin)
	finish := strings.Index(cache[start:], end)
	if finish < 0 {
		t.Fatalf("cache has an unterminated GATE_JUDGMENTS block:\n%s", cache)
	}
	return cache[start : start+finish]
}

func grNoticeContains(notices []string, want string) bool {
	for _, notice := range notices {
		if strings.Contains(notice, want) {
			return true
		}
	}
	return false
}

// TestGateSubmitAcceptsLeadingWhitespaceBeforeVerdicts guards the verdict line
// index against multiline whitespace matching: a strictly valid packet report
// that begins with blank lines (or has a blank line before the verdict) must
// be accepted, not misread as a missing reason.
func TestGateSubmitAcceptsLeadingWhitespaceBeforeVerdicts(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", "\n\n"+grVerifyItemReport("auth.core", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "cross", "\n"+grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID)
}

// TestParseGateFindingsGrammar verifies the whole-line `gate_findings`
// grammar: only `none` or one or more bracketed P0/P1 entries may satisfy the
// conclusion mapping. Mixed content such as `none [P1] x` or entries carrying
// P2/P3 are rejected.
func TestParseGateFindingsGrammar(t *testing.T) {
	cases := []struct {
		content  string
		blocking bool
		wantErr  bool
	}{
		{"none", false, false},
		{"[P1] src/auth.go:1 — boundary defect", true, false},
		{"[P0] a — first; [P1] b — second", true, false},
		{"none [P1] x", false, true},
		{"[P1] x; [P3] y", false, true},
		{"[P1] x;", false, true},
		{"[P1]", false, true},
		{"", false, true},
	}
	for _, tc := range cases {
		got, blocking, err := parseGateFindings(tc.content)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("parseGateFindings(%q): expected an error", tc.content)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseGateFindings(%q): %v", tc.content, err)
		}
		if blocking != tc.blocking {
			t.Fatalf("parseGateFindings(%q): blocking=%t want %t", tc.content, blocking, tc.blocking)
		}
		if strings.TrimSpace(got) == "" {
			t.Fatalf("parseGateFindings(%q): empty normalized content", tc.content)
		}
	}
	if _, err := extractGateFindings("  gate_findings: none [P1] x\n"); err == nil {
		t.Fatal("expected a mixed gate_findings line to be rejected")
	}
}

// TestGateDeltaFinalizesWithCarriedEvidenceOutsideDeclaredDeps guards the
// carried-entry snapshot binding: a file whose content changed outside its
// declared dependency chunks keeps its baseline evidence (a documented fresh
// state) and must not block a delta finalize.
func TestGateDeltaFinalizesWithCarriedEvidenceOutsideDeclaredDeps(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.one", "auth.two"})
	main := "docs/specs/units/candidate/unit_auth.md"
	specPath := filepath.Join(repoRoot, filepath.FromSlash(main))

	var big strings.Builder
	for i := 1; i <= 600; i++ {
		fmt.Fprintf(&big, "func padding%03d() int { // marker %03d value %d }\n", i, i, i*7919%104729)
	}
	grWriteFile(t, repoRoot, "src/big.go", big.String())
	grWriteFile(t, repoRoot, "src/two.go", "package two\n\nvar Two = 2\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	oneReport := fmt.Sprintf("- auth.one: ALIGNED — src/big.go:300\n  evidence: src/big.go:300 implements the declared behavior\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped — no test files in this fixture\n\nauth.one: %s: acceptance_item:auth.one\nauth.one: src/big.go: 300-310\n", main)
	grSubmitOK(t, repoRoot, runID, "auth.one", oneReport)
	grSubmitOK(t, repoRoot, runID, "auth.two", grVerifyItemReport("auth.two", main, "src/two.go"))
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID)

	baselineHash := grEntryHash(t, grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md"), "src/big.go")

	// Change content outside the declared 300-310 range, and stale a
	// different check by editing item auth.two's spec region.
	bigPath := filepath.Join(repoRoot, "src/big.go")
	data, err := os.ReadFile(bigPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	lines[0] = "func padding001() int { // changed outside the declared range"
	if err := os.WriteFile(bigPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(spec), "  - id: auth.two\n    description: Behavior.", "  - id: auth.two\n    description: Behavior, revised.", 1)
	if edited == string(spec) {
		t.Fatal("failed to edit the auth.two spec region")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(run.CarriedKeys, "auth.one") {
		t.Fatalf("expected auth.one to be carried over, got %v", run.CarriedKeys)
	}
	grSubmitOK(t, repoRoot, deltaID, "auth.two", grVerifyItemReport("auth.two", main, "src/two.go"))
	grSubmitOK(t, repoRoot, deltaID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, deltaID)

	cacheAfter := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if got := grEntryHash(t, cacheAfter, "src/big.go"); got != baselineHash {
		t.Fatalf("expected the carried entry to keep its baseline hash %s, got %s", baselineHash, got)
	}
}

// TestGatePlanRejectsEscapingLogicalInput verifies that a logical `--input`
// reference whose resolution escapes the repository root is rejected before
// run state is written (the same containment rule physical inputs obey).
func TestGatePlanRejectsEscapingLogicalInput(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	external := filepath.Join(filepath.Dir(repoRoot), "tmp")
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(external) })
	if err := os.WriteFile(filepath.Join(external, "evil.md"), []byte("external\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// docs/specs/units/candidate/unit_ is five levels below the repo root;
	// six parent segments land in the repo root's parent, where the crafted
	// `tmp/evil.md` exists so only the containment check can reject it.
	escape := "unit:"
	for i := 0; i < 6; i++ {
		escape += "/.."
	}
	escape += "/tmp/evil"
	if _, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--input", escape); err == nil || !strings.Contains(err.Error(), "outside the repository root") {
		t.Fatalf("expected the escaping logical input to be rejected, got %v", err)
	}
}

// TestGateRuleSeverityAdjustmentPersistsCanonicalDetail verifies that a rule
// validate severity adjustment (P0 -> P1) persists the whole canonical finding
// — severity and the rewritten detail prefix — into the judgment baseline.
func TestGateRuleSeverityAdjustmentPersistsCanonicalDetail(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")

	passReport := func() string {
		var b strings.Builder
		for c := 1; c <= 8; c++ {
			fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
		}
		for c := 1; c <= 8; c++ {
			fmt.Fprintf(&b, "check-%d: %s: Constraint\n", c, rulePath)
		}
		return b.String()
	}
	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "checks", passReport())
	grFinalizeOK(t, repoRoot, fullRun)

	// Change the rule body so the delta run re-executes the checks, then fail
	// check 8 with a P0 finding that the confirmation sequence adjusts to P1.
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS everywhere.\n")
	deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate", "--mode", "delta")
	findingID := grRunFindingID(deltaRun, "checks", 1)

	var b strings.Builder
	for c := 1; c <= 8; c++ {
		if c == 8 {
			fmt.Fprintf(&b, "8. %s: FAIL — rule body contradicts the declared constraint\n", grCheckNames["8"])
			continue
		}
		fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
	}
	for c := 1; c <= 8; c++ {
		fmt.Fprintf(&b, "check-%d: %s: Constraint\n", c, rulePath)
	}
	fmt.Fprintf(&b, "[P0] %s:10 — rule body contradicts the declared constraint (needs_decision)\n", rulePath)
	fmt.Fprintf(&b, "Severity confirmation: %s = adjusted P0 -> P1 — evidence: %s; reason: impact is local to one consumer\n", findingID, rulePath)
	fmt.Fprintf(&b, "Severity confirmation: %s = confirmed P1 — evidence: %s; reason: local impact confirmed\n", findingID, rulePath)
	grSubmitOK(t, repoRoot, deltaRun, "checks", b.String())
	grFinalizeOK(t, repoRoot, deltaRun)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/rule/b_rule_http/validate_result.md")
	judgments := grGateJudgments(t, cache)
	if !strings.Contains(judgments, `"severity":"P1"`) || !strings.Contains(judgments, "[P1] ") {
		t.Fatalf("expected the canonical P1 severity and rewritten detail, got:\n%s", judgments)
	}
	if strings.Contains(judgments, "[P0]") {
		t.Fatalf("expected no stale [P0] prefix in the judgment baseline, got:\n%s", judgments)
	}
	if !strings.Contains(cache, "p1_count: 1") || !strings.Contains(cache, "result: fail") {
		t.Fatalf("expected the adjusted severity to drive the derived result and counts, got:\n%s", cache)
	}
}

// TestGateRepairDegradesOnConflictingStatusMap verifies that a failure
// record recording two different statuses for the same check degrades the
// repair plan to the full packet set instead of trusting the first occurrence
// and possibly carrying a failed judgment over.
func TestGateRepairDegradesOnConflictingStatusMap(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.one", "auth.two"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	fullRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "auth.one", grVerifyItemBody("auth.one", "MISMATCH (acceptance)", "broken at src/auth.go:1")+"auth.one: "+main+": Testability / Acceptance Criteria\nauth.one: src/auth.go: all\n")
	grSubmitOK(t, repoRoot, fullRun, "analysis:auth.one", grVerifyAnalysisReport("auth.one", main, "src/auth.go", "P0"))
	grSubmitOK(t, repoRoot, fullRun, "auth.two", grVerifyItemReport("auth.two", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun)

	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	entryIdx := strings.Index(cache, "- path: src/auth.go")
	if entryIdx < 0 {
		t.Fatalf("cache has no src/auth.go entry:\n%s", cache)
	}
	checkIdx := strings.Index(cache[entryIdx:], `check: "auth.two"`)
	if checkIdx < 0 {
		t.Fatalf("cache has no auth.two check in the src/auth.go entry:\n%s", cache)
	}
	statusRel := strings.Index(cache[entryIdx+checkIdx:], "status: pass")
	if statusRel < 0 {
		t.Fatalf("cache has no pass status for auth.two in the src/auth.go entry:\n%s", cache)
	}
	statusAbs := entryIdx + checkIdx + statusRel
	mutated := cache[:statusAbs] + "status: fail" + cache[statusAbs+len("status: pass"):]
	if err := os.WriteFile(cachePath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	repairID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, repoRoot, repairID)
	if !grNoticeContains(run.Notices, "invalid per-check status map") || !grNoticeContains(run.Notices, "auth.two=conflicting") {
		t.Fatalf("expected a conflicting status map to degrade the plan, got notices %v", run.Notices)
	}
	want := "detect:auth.one,analysis:auth.one,detect:auth.two,analysis:auth.two,cross"
	if got := strings.Join(packetIDsOf(run), ","); got != want {
		t.Fatalf("expected the degraded full packet set %q, got %q", want, got)
	}
}

// TestGateRepairDegradesOnCarriedStatusInFullRecord verifies that a failure
// record with an absent basis (contract-defined as a full-run record) carrying
// a `carried` status degrades the repair plan to the full packet set: a full
// record has no carried judgments.
func TestGateRepairDegradesOnCarriedStatusInFullRecord(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.one", "auth.two"})
	main := "docs/specs/units/candidate/unit_auth.md"
	specPath := filepath.Join(repoRoot, filepath.FromSlash(main))
	grWriteFile(t, repoRoot, "src/one.go", "package one\n")
	grWriteFile(t, repoRoot, "src/two.go", "package two\n")

	itemReport := func(item, codeFile string) string {
		return fmt.Sprintf("- %s: ALIGNED — %s:1\n  evidence: %s:1 implements the declared behavior\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped — no test files in this fixture\n\n%s: %s: acceptance_item:%s\n%s: %s: all\n", item, codeFile, codeFile, item, main, item, item, codeFile)
	}
	fullRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "auth.one", itemReport("auth.one", "src/one.go"))
	grSubmitOK(t, repoRoot, fullRun, "auth.two", itemReport("auth.two", "src/two.go"))
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun)

	// Stale auth.two and break its code so the delta run fails and records
	// auth.one as carried.
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(spec), "  - id: auth.two\n    description: Behavior.", "  - id: auth.two\n    description: Behavior, revised.", 1)
	if edited == string(spec) {
		t.Fatal("failed to edit the auth.two spec region")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, repoRoot, "src/two.go", "package two\n\nvar Two = 3\n")

	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(deltaRun.CarriedKeys, "auth.one") {
		t.Fatalf("expected auth.one to be carried over, got %v", deltaRun.CarriedKeys)
	}
	grSubmitOK(t, repoRoot, deltaID, "auth.two", grVerifyItemBody("auth.two", "MISMATCH (structural)", "declaration changed at src/two.go:3")+"auth.two: "+main+": acceptance_item:auth.two\nauth.two: src/two.go: all\n")
	grSubmitOK(t, repoRoot, deltaID, "analysis:auth.two", grVerifyAnalysisReport("auth.two", main, "src/two.go", "P0"))
	grSubmitOK(t, repoRoot, deltaID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, deltaID)

	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "status: carried") || !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected a delta failure record carrying auth.one, got:\n%s", cache)
	}
	stripped := strings.Replace(cache, "basis: delta\n", "", 1)
	if stripped == cache {
		t.Fatalf("failed to strip the basis line:\n%s", cache)
	}
	if err := os.WriteFile(cachePath, []byte(stripped), 0644); err != nil {
		t.Fatal(err)
	}

	repairID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, repoRoot, repairID)
	if !grNoticeContains(run.Notices, "invalid per-check status map") || !grNoticeContains(run.Notices, "auth.one=carried") {
		t.Fatalf("expected a carried status in a full record to degrade the plan, got notices %v", run.Notices)
	}
	want := "detect:auth.one,analysis:auth.one,detect:auth.two,analysis:auth.two,cross"
	if got := strings.Join(packetIDsOf(run), ","); got != want {
		t.Fatalf("expected the degraded full packet set %q, got %q", want, got)
	}
}

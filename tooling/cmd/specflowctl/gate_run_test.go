package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
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
	fmt.Fprintf(&b, "---\nid: %s\nunit_refs: %s\nrule_refs: %s\n---\n\n# %s\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n", name, unitRefs, ruleRefs, name)
	for _, item := range items {
		fmt.Fprintf(&b, "  - id: %s\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n", item)
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
	content := "---\nid: " + name + "\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n" +
		"  - id: " + name + ".core\n    description: Given a caller, When the behavior runs, Then it is accepted.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: " + surface + "\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n" + extra
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

// grInputsManifest writes an input manifest outside the repository (scratch
// input, not evidence) and returns its path for --inputs-file.
func grInputsManifest(t *testing.T, values ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan_inputs.txt")
	content := strings.Join(values, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func grEnableMissionLayout(t *testing.T, repoRoot string) {
	t.Helper()
	grWriteFile(t, repoRoot, "tooling/manifest.tsv", "tooling fixture\n")
}

// grPlan runs gate-plan and returns the run id.
func grPlan(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	runID, err := grPlanRaw(repoRoot, args...)
	if err != nil {
		t.Fatalf("gate-plan failed: %v", err)
	}
	grFixtureUnits.Store(runID, mustLoadRun(t, repoRoot, runID).TargetName)
	return runID
}

var grFixtureUnits sync.Map

// grPlanRaw runs gate-plan and returns the run id or an error.
func grPlanRaw(repoRoot string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	full := append([]string{"--repo-root", repoRoot}, args...)
	if stringInList(args, "--unit") && !stringInList(args, "--relationships") {
		full = append(full, "--relationships", "none")
	}
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

// grSessionKeys maps a legacy single-session id used by the fixtures to the
// coverage key set the session now submits for: the reserved final key for the
// cross synthesis, the rule check keys for a rule "checks" session, the item
// id stripped from a legacy detect:/analysis: id, or the key itself.
func grSessionKeys(run *gaterun.Run, sessionID string) []string {
	if sessionID == "cross" {
		return []string{"cross"}
	}
	if run.TargetKind == gaterun.TargetKindRule && sessionID == "checks" {
		return []string{"1", "2", "3", "4", "5", "6"}
	}
	sessionID = strings.TrimPrefix(strings.TrimPrefix(sessionID, "detect:"), "analysis:")
	if run.Gate == gaterun.GateVerify {
		for _, ck := range run.Coverage {
			if ck.Kind == gaterun.SessionKindItem && ck.Item == sessionID || ck.Kind == gaterun.SessionKindDesign && ck.File == sessionID {
				return []string{ck.Key}
			}
		}
		for _, key := range run.CarriedKeys {
			if key == "item:"+run.TargetName+":"+sessionID || key == "design:"+run.TargetName+":"+sessionID {
				return []string{key}
			}
		}
	}
	return []string{sessionID}
}

func grAdaptReport(run *gaterun.Run, report string) string {
	if run.Gate != gaterun.GateVerify {
		return report
	}
	keys := append([]string(nil), run.CarriedKeys...)
	for _, ck := range run.Coverage {
		keys = append(keys, ck.Key)
	}
	for _, key := range keys {
		old := ""
		if strings.HasPrefix(key, "item:"+run.TargetName+":") {
			old = strings.TrimPrefix(key, "item:"+run.TargetName+":")
		}
		if strings.HasPrefix(key, "design:"+run.TargetName+":") {
			old = strings.TrimPrefix(key, "design:"+run.TargetName+":")
		}
		if old == "" {
			continue
		}
		report = regexp.MustCompile(`(?m)^([ \t]*-?[ \t]*)`+regexp.QuoteMeta(old)+`:`).ReplaceAllString(report, "${1}"+key+":")
		report = strings.ReplaceAll(report, "File: "+old+"\n", "File: "+key+"\n")
		report = strings.ReplaceAll(report, "Effective status: "+old+" =", "Effective status: "+key+" =")
		report = strings.ReplaceAll(report, run.RunID+"/"+old+"/F", run.RunID+"/"+key+"/F")
		report = strings.ReplaceAll(report, "= "+old+"\n", "= "+key+"\n")
	}
	lines := strings.Split(report, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "Finding affects:") {
			prefix, rhs, ok := strings.Cut(line, " = ")
			if ok {
				parts := strings.Split(rhs, ",")
				for j, p := range parts {
					parts[j] = grSessionKeys(run, strings.TrimSpace(p))[0]
				}
				lines[i] = prefix + " = " + strings.Join(parts, ", ")
			}
		}
	}
	return strings.Join(lines, "\n")
}

// grSubmit writes a report to a temp file and runs gate-submit.
func grSubmit(t *testing.T, repoRoot, runID, sessionID, report string) (string, error) {
	t.Helper()
	run, loadErr := gaterun.Load(repoRoot, runID)
	if loadErr == nil {
		if sessionID == "cross" {
			grAutoSubmitQuality(t, repoRoot, runID)
			report = grWithCrossItems(run, report)
			report = grWithRelationshipEvidence(run, report)
			report = grCompleteCrossReport(t, repoRoot, run, grAdaptReport(run, report))
		}
	}
	return grSubmitRaw(t, repoRoot, runID, sessionID, report)
}

// grAutoSubmitQuality covers every uncovered quality-lens coverage key of a
// verify run with a minimal acceptable file report. The merged verify gate's
// coverage set is the union of the alignment items and the declared code files;
// most fixtures exercise the alignment lens, so the quality lens is filled in
// mechanically here. It is a no-op for non-verify runs.
func grAutoSubmitQuality(t *testing.T, repoRoot, runID string) {
	t.Helper()
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Gate != gaterun.GateVerify || len(run.RequiredFiles) == 0 {
		return
	}
	for _, ck := range run.Coverage {
		if ck.Lens != gaterun.LensQuality {
			continue
		}
		states, err := gaterun.LoadSessionStates(repoRoot, run)
		if err != nil {
			t.Fatal(err)
		}
		covered, _, err := gaterun.CoverageProgress(run, states)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := covered[ck.Key]; ok {
			continue
		}
		report := grDefaultQualityReport(run, ck)
		path := grWriteFile(t, t.TempDir(), "quality-report.md", report)
		var stdout, stderr bytes.Buffer
		if err := runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID, "--session", gaterun.SessionID([]string{ck.Key}), "--keys", ck.Key, "--report", path}, &stdout, &stderr); err != nil {
			t.Fatalf("auto quality %s: %v", ck.Key, err)
		}
	}
}
func grDefaultQualityReport(run *gaterun.Run, ck gaterun.CoverageKey) string {
	if ck.Kind == gaterun.SessionKindCode {
		// Public code checks carry no Dependency scope lines: their whole
		// public evidence surface is recorded by the tooling.
		return "File: " + ck.Key + "\nconclusion: FACTS\nfacts: no potential problems in fixture\n"
	}
	if ck.Kind == gaterun.SessionKindArchitecture {
		report := strings.Replace(grQualityArchitecture(ck.Key, "acceptable"), "File:", "Unit:", 1)
		report += ck.Key + ": " + run.RequiredFiles[0] + ": Description\n"
		for _, surface := range run.Surfaces {
			for _, file := range surface.Entries {
				report += ck.Key + ": " + file.Path + ": all\n"
			}
		}
		return report
	}
	return grQualityArchitecture(ck.Key, "acceptable") + ck.Key + ": " + run.RequiredFiles[0] + ": Description\n" + ck.Key + ": " + ck.File + ": all\n"
}
func grPreparePublic(t *testing.T, root string, run *gaterun.Run, keys []string) {
	inBatch := map[string]bool{}
	for _, key := range keys {
		inBatch[key] = true
	}
	for _, key := range keys {
		ck := run.CoverageByKey(key)
		if ck == nil || ck.Kind != gaterun.SessionKindDesign {
			continue
		}
		public := run.CoverageByKey("code:" + ck.File)
		if public == nil || inBatch[public.Key] {
			// A co-batched pair supplies its own public record.
			continue
		}
		states, err := gaterun.LoadSessionStates(root, run)
		if err != nil {
			t.Fatal(err)
		}
		covered, _, err := gaterun.CoverageProgress(run, states)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := covered[public.Key]; ok {
			continue
		}
		path := grWriteFile(t, t.TempDir(), "public-report.md", grDefaultQualityReport(run, *public))
		var out, errOut bytes.Buffer
		if err := runGateSubmit([]string{"--repo-root", root, "--run", run.RunID, "--session", public.Key, "--keys", public.Key, "--report", path}, &out, &errOut); err != nil {
			t.Fatal(err)
		}
	}
}

// grSubmitRaw submits the exact report supplied by a test without fixture
// completion. Contract-rejection tests use it to exercise missing fields.
func grSubmitRaw(t *testing.T, repoRoot, runID, sessionID, report string) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		return "", err
	}
	keys := grSessionKeys(run, sessionID)
	grPreparePublic(t, repoRoot, run, keys)
	report = grAdaptReport(run, report)
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	derivedID := gaterun.SessionID(keys)
	var stdout, stderr bytes.Buffer
	err = runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID, "--session", derivedID, "--keys", strings.Join(keys, ","), "--report", path}, &stdout, &stderr)
	if err != nil && stderr.Len() > 0 {
		return stdout.String(), fmt.Errorf("%w (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), err
}

// grSubmitOK fails the test when gate-submit errors.
func grSubmitOK(t *testing.T, repoRoot, runID, sessionID, report string) string {
	t.Helper()
	out, err := grSubmit(t, repoRoot, runID, sessionID, report)
	if err != nil {
		t.Fatalf("gate-submit %s failed: %v", sessionID, err)
	}
	return out
}

// grReviewAccept submits an accepting delta review for an open review run.
func grReviewAccept(t *testing.T, repoRoot, runID string) {
	t.Helper()
	grSubmitOK(t, repoRoot, runID, "review", "Review result: accept — test review\n")
}

// grReviewRecheck submits a delta review that names the given re-run keys.
func grReviewRecheck(t *testing.T, repoRoot, runID string, keys ...string) {
	t.Helper()
	grSubmitOK(t, repoRoot, runID, "review", "Review result: recheck — test review\nRecheck: "+strings.Join(keys, ", ")+"\n")
}

// grSubmitKeys submits one session report for an explicit coverage key batch.
func grSubmitKeys(t *testing.T, repoRoot, runID string, keys []string, report string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range keys {
		keys[i] = grSessionKeys(run, key)[0]
	}
	grPreparePublic(t, repoRoot, run, keys)
	report = grAdaptReport(run, report)
	if err := os.WriteFile(path, []byte(report), 0644); err != nil {
		t.Fatal(err)
	}
	sessionID := gaterun.SessionID(keys)
	var stdout, stderr bytes.Buffer
	err = runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID, "--session", sessionID, "--keys", strings.Join(keys, ","), "--report", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-submit %v failed: %v (stderr=%s)", keys, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String()
}

// grFinalize runs gate-finalize and returns its stdout and error.
func grFinalize(t *testing.T, repoRoot, runID string, args ...string) (string, error) {
	t.Helper()
	// A missing run is returned as the helper's error, not a test abort: a
	// concluded run is a legitimate expectation under test.
	if _, err := gaterun.Load(repoRoot, runID); err != nil {
		return "", err
	}
	grAutoSubmitQuality(t, repoRoot, runID)
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
	"1":  "Structural integrity",
	"2":  "Design soundness",
	"3":  "Scope integrity",
	"4":  "Evidence-driven vs design-driven consistency",
	"5":  "Acceptance coverage & correctness",
	"6":  "Affects-source validity",
	"7":  "Cross-unit consistency",
	"8":  "Constraint alignment",
	"9":  "File associations & shared agreements",
	"10": "Clarity",
}

// grValidateReport builds a validate session report: PASS verdicts for the
// given checks plus one scope line per entry of scopes (check key -> raw
// "{file}: {declaration}" strings).
func grValidateReport(checks []string, scopes map[string][]string) string {
	var b strings.Builder
	for _, c := range checks {
		fmt.Fprintf(&b, "%s. %s: PASS — checked\n", c, grCheckNames[c])
		if c == "5" {
			for _, subcheck := range []string{"5a", "5b", "5e", "5h"} {
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

// grDependenciesReport builds a dependency-group validate report. Check 9
// (file associations) joined the group; when the caller provides no legacy
// scope line for it, check 7's line is reused — scope lines are inert
// fixtures now (reports carry no dependency declarations) and these tests
// exercise the gate mechanics.
func grDependenciesReport(scopes map[string][]string) string {
	if _, ok := scopes["9"]; !ok {
		if lines, ok := scopes["7"]; ok {
			scopes["9"] = lines
		} else if lines, ok := scopes["8"]; ok {
			scopes["9"] = lines
		}
	}
	return grValidateReport([]string{"7", "8", "9"}, scopes)
}

// grCrossReport builds a cross-check session report.
func grCrossReport(scopeFile, scopeDecl string) string {
	return fmt.Sprintf("Cross-check: 3/3 PASS — consistent\n\ncross: %s: %s\n", scopeFile, scopeDecl)
}

// grWithCrossItems upgrades shared fixture reports to the complete unit cross
// contract. Tests of malformed cross-item reports submit their text directly.
func grWithCrossItems(run *gaterun.Run, report string) string {
	items := crossItemsFor(run)
	if len(items) == 0 {
		if match := crossSummaryRe.FindStringSubmatch(report); match != nil {
			report = strings.Replace(report, match[0], "Cross-check: "+match[3]+" — "+match[4], 1)
		}
		return report
	}
	if strings.Contains(report, "Cross item:") {
		return report
	}
	match := crossSummaryRe.FindStringSubmatch(report)
	if match == nil {
		return report
	}
	passed, _ := strconv.Atoi(match[1])
	total, _ := strconv.Atoi(match[2])
	passed = len(items) - (total - passed)
	if passed < 0 {
		passed = 0
	}
	var lines strings.Builder
	hasCrossFinding := len(extractFindings(report)) > 0
	for index, item := range items {
		verdict := "FAIL"
		if index < passed {
			verdict = "PASS"
		}
		fmt.Fprintf(&lines, "Cross item: %s = %s — fixture judgment\n", item, verdict)
		if verdict == "FAIL" && hasCrossFinding {
			fmt.Fprintf(&lines, "Cross item finding: %s = %s/cross/F1\n", item, run.RunID)
		}
	}
	report = strings.Replace(report, match[0], fmt.Sprintf("Cross-check: %d/%d %s — %s", passed, len(items), match[3], match[4]), 1)
	index := strings.Index(report, "Cross-check:")
	return report[:index] + lines.String() + report[index:]
}

// Complete evidence/status fields for the relationship scope explicitly
// selected by fixtures. Raw parser tests still submit reports unchanged.
func grWithRelationshipEvidence(run *gaterun.Run, report string) string {
	var scope string
	for _, line := range strings.Split(report, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "cross:"); ok {
			scope = rest
			break
		}
	}
	for _, name := range run.Relationships {
		key := gaterun.RelationshipKey(name)
		status := "pass"
		if strings.Contains(report, "Cross item: "+name+" = FAIL") {
			status = "fail"
			prefix := "Cross item finding: " + name + " = "
			for _, line := range strings.Split(report, "\n") {
				if id, ok := strings.CutPrefix(line, prefix); ok {
					for _, affects := range strings.Split(report, "\n") {
						if strings.HasPrefix(affects, "Finding affects: "+id+" = ") && !strings.Contains(affects, key) {
							report = strings.Replace(report, affects, affects+", "+key, 1)
						}
					}
				}
			}
		}
		if !strings.Contains(report, key+": ") {
			report += key + ":" + scope + "\n"
		}
		if !strings.Contains(report, "Effective status: "+key+" = ") {
			report += "Effective status: " + key + " = " + status + "\n"
		}
	}
	return report
}

// grRunFindingID renders the run-scoped finding id the parser assigns to the
// n-th finding of a session report: {run_id}/{session_id}/F{n}. The prefix is
// the run id printed in the session context (gate-mission's `Run:` line), so
// report authors copy it instead of inventing ids.
func grRunFindingID(runID, sessionID string, n int) string {
	if unit, ok := grFixtureUnits.Load(runID); ok && strings.Contains(sessionID, "/") && !strings.Contains(sessionID, ":") {
		sessionID = "design:" + unit.(string) + ":" + sessionID
	}
	if !strings.Contains(sessionID, ":") && strings.Contains(sessionID, ".") && !strings.Contains(sessionID, "/") {
		sessionID = "item:" + strings.Split(sessionID, ".")[0] + ":" + sessionID
	}
	return fmt.Sprintf("%s/%s/F%d", runID, sessionID, n)
}

func grCompleteCrossReport(t *testing.T, repoRoot string, run *gaterun.Run, report string) string {
	report = grAdaptReport(run, report)
	t.Helper()
	states, err := gaterun.LoadSessionStates(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(report, "\n"))
	b.WriteString("\n")
	dispositionSeen := map[string]bool{}
	for _, m := range dispositionRe.FindAllStringSubmatch(report, -1) {
		dispositionSeen[strings.TrimSpace(m[1])] = true
	}
	for _, state := range states {
		if state.Status != gaterun.SessionAccepted || state.Result == nil || state.SessionID == gaterun.CrossKey {
			continue
		}
		for _, finding := range state.Result.Findings {
			if !dispositionSeen[finding.ID] {
				fmt.Fprintf(&b, "Finding disposition: %s = retained\n", finding.ID)
				dispositionSeen[finding.ID] = true
			}
		}
	}
	for _, result := range run.CarriedResults {
		for _, finding := range result.Findings {
			if !dispositionSeen[finding.ID] {
				fmt.Fprintf(&b, "Finding disposition: %s = retained\n", finding.ID)
				dispositionSeen[finding.ID] = true
			}
		}
	}
	grCompleteQualityConclusions(run, states, &b)
	return b.String()
}

// grCompleteQualityConclusions authors the default final quality assessments
// for fixtures. Tests may supply explicit conclusions to exercise a different
// reviewer decision or an invalid contract.
func grCompleteQualityConclusions(run *gaterun.Run, states []*gaterun.SessionState, b *strings.Builder) {
	if run.Gate != gaterun.GateVerify {
		return
	}
	conclusions := map[string]string{}
	var input []gaterun.Finding
	consume := func(result *gaterun.SessionResult) {
		for key, verdict := range result.Verdicts {
			if strings.HasPrefix(key, "design:") || strings.HasPrefix(key, "architecture:") {
				conclusions[key] = verdict
			}
		}
		input = append(input, resultFindings(result)...)
	}
	for _, state := range states {
		if state.Status == gaterun.SessionAccepted && state.Result != nil && state.SessionID != gaterun.CrossKey {
			consume(state.Result)
		}
	}
	for i := range run.CarriedResults {
		consume(&run.CarriedResults[i])
	}
	for _, deferred := range run.DeferredFindings {
		input = append(input, deferred.Finding)
	}
	parsed := &parsedReport{EffectiveStatus: map[string]string{}}
	for i, finding := range extractFindings(b.String()) {
		parsed.Findings = append(parsed.Findings, gaterun.Finding{ID: fmt.Sprintf("%s/cross/F%d", run.RunID, i+1), Severity: finding.severity, SourceKey: gaterun.CrossKey})
	}
	retained := input
	if err := parseCrossSynthesis(b.String(), parsed); err == nil {
		if resolved, err := resolveCrossFindings(input, parsed.Findings, parsed.Dispositions); err == nil {
			if owned, err := applyOwnerships(resolved, parsed.Ownerships); err == nil {
				retained = owned
			}
		}
	}
	var keys []string
	for key := range conclusions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.Contains(b.String(), "Quality conclusion: "+key+" =") {
			continue
		}
		blocking, attention := false, false
		for _, finding := range retained {
			if findingKeySet(finding)[key] && gateDriving(finding, run.TargetName) {
				attention = true
				blocking = blocking || finding.Severity == "P0" || finding.Severity == "P1"
			}
		}
		verdict := conclusions[key]
		if blocking {
			verdict = "unacceptable"
		} else if attention && verdict == "acceptable" {
			verdict = "needs_attention"
		} else if verdict == "unacceptable" {
			verdict = "acceptable"
		}
		fmt.Fprintf(b, "Quality conclusion: %s = %s — fixture final assessment\n", key, verdict)
	}
}

// grClarityReport builds the clarity group's standard checks report for check
// 10, declaring the unit spec's Description section.
func grClarityReport(specPath, verdict string) string {
	return fmt.Sprintf("10. Clarity: %s — %s\n\ncheck-10: %s: Description\n", verdict, "clarity fixture judgment", specPath)
}

// grSubmitClarity submits the clarity group's single check-10 session when the
// run carries the coverage key and it is not already accepted. Runs without it
// (verify, rule validate) are skipped, so the helper is safe to call before any
// cross submission.
func grSubmitClarity(t *testing.T, repoRoot, runID, specPath string) {
	t.Helper()
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.CoverageByKey("clarity") == nil {
		return
	}
	state, err := grLoadSessionState(repoRoot, run, "clarity")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status == gaterun.SessionAccepted {
		return
	}
	grSubmitOK(t, repoRoot, runID, "clarity", grClarityReport(specPath, "PASS"))
}

// grSubmitValidateSessions submits the full 7-session validate plan for a
// spec. extraScopes are appended to the structural session's scope lines
// (e.g. appendix declarations).
func grSubmitValidateSessions(t *testing.T, repoRoot, runID, specPath string, extraScopes ...string) {
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
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {desc},
		"8": {desc},
	}))
	grSubmitClarity(t, repoRoot, runID, specPath)
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(specPath, "Description"))
}

// grSubmitPlannedValidateSessions submits exactly the coverage keys the run's
// plan carries — used for delta/repair runs whose coverage set is a subset of
// the full validate plan.
func grSubmitPlannedValidateSessions(t *testing.T, repoRoot, runID, specPath string) {
	t.Helper()
	run := mustLoadRun(t, repoRoot, runID)
	desc := specPath + ": Description"
	accept := specPath + ": Testability / Acceptance Criteria"
	front := specPath + ": frontmatter"
	reports := map[string]string{
		"structural": grValidateReport([]string{"1", "3", "6"}, map[string][]string{
			"1": {front, desc},
			"3": {accept},
			"6": {accept},
		}),
		"design": grValidateReport([]string{"2", "4"}, map[string][]string{
			"2": {desc},
			"4": {desc},
		}),
		"acceptance": grValidateReport([]string{"5"}, map[string][]string{
			"5": {accept},
		}),
		"dependencies": grDependenciesReport(map[string][]string{
			"7": {desc},
			"8": {desc},
		}),
	}
	for _, ck := range run.Coverage {
		switch ck.Key {
		case "clarity":
			grSubmitClarity(t, repoRoot, runID, specPath)
		default:
			if report, ok := reports[ck.Key]; ok {
				grSubmitOK(t, repoRoot, runID, ck.Key, report)
			}
		}
	}
}

// grVerifyItemReport builds a verify item session report with an ALIGNED
// verdict and spec + code scope lines.
func grVerifyItemReport(item, specPath, codeFile string) string {
	return fmt.Sprintf("- %s: ALIGNED — %s:1\n  evidence: %s:1 implements the declared behavior\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped — no test files in this fixture\n\n%s: %s: Testability / Acceptance Criteria\n%s: %s: all\n", item, codeFile, codeFile, item, specPath, item, codeFile)
}

func grVerifyItemBody(item, verdict, evidence string) string {
	return fmt.Sprintf("- %s: %s — %s\n  evidence: %s\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped — no test files in this fixture\n\n", item, verdict, evidence, evidence)
}

// grVerifyAnalysisFields renders the inline finding fields of a MISMATCH item
// block: the finding's root cause, evidence, impact, fix, severity, and
// confidence.
func grVerifyAnalysisFields(item string, severity ...string) string {
	sev := "P1"
	if len(severity) > 0 && severity[0] != "" {
		sev = severity[0]
	}
	return fmt.Sprintf("  Problem: the declared behavior disagrees with the implementation for %s\n  Evidence:\n    - spec: declared behavior — acceptance item %s\n    - code: implemented behavior\n  Impact: the acceptance surface cannot be satisfied as declared\n  Fix: align the implementation with the declared behavior\n  Root cause: incomplete\n  Suggested direction: code_gap\n  Severity: %s\n  Confidence: high\n", item, item, sev)
}

// grVerifyMismatchReport builds a complete verify item session report for a
// MISMATCH: the item verdict and evidence fields, the inline finding fields,
// and the spec + code scope lines.
func grVerifyMismatchReport(item, specPath, codeFile, severity string) string {
	return grVerifyItemBody(item, "MISMATCH (acceptance)", codeFile+":1") + grVerifyAnalysisFields(item, severity) +
		fmt.Sprintf("%s: %s: acceptance_item:%s\n%s: %s: all\n", item, specPath, item, item, codeFile)
}

func grQualityArchitecture(file, conclusion string) string {
	gateFindings := "none"
	if strings.HasPrefix(conclusion, "unacceptable") {
		gateFindings = "[P1] gate-level architectural boundary defect"
	}
	return fmt.Sprintf("File: %s\nspec_requirements: requirements checked — fixture design evidence\nArchitecture assessment:\n  conclusion: %s\n  module_boundaries: sound — reviewed module boundary\n  responsibility_organization: sound — reviewed responsibility placement\n  dependency_clarity: sound — reviewed dependency direction\n  abstraction_level: sound — reviewed abstraction level\n  extension_landing_points: sound — reviewed extension seams\n  engineering_patterns: sound — reviewed engineering patterns\n  gate_findings: %s\n\nSuppressed by spec (0):\n", file, conclusion, gateFindings)
}

// grQualityReport builds a complete quality file session report with an
// acceptable conclusion and spec + file scope lines.
func grQualityReport(file, specPath string) string {
	return fmt.Sprintf("%s\n%s: %s: Description\n%s: %s: all\n", grQualityArchitecture(file, "acceptable"), file, specPath, file, file)
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
		if value == want || strings.HasPrefix(value, "item:") && strings.HasSuffix(value, ":"+want) || strings.HasPrefix(value, "design:") && strings.HasSuffix(value, ":"+want) {
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
	if run.Mode != "full" || len(run.Coverage) != 5 {
		t.Fatalf("expected a 5-key full coverage set, got mode %q with %d keys", run.Mode, len(run.Coverage))
	}
	wantIDs := []string{"structural", "design", "acceptance", "dependencies", "clarity"}
	if got := strings.Join(coverageKeysOf(run), ","); got != strings.Join(wantIDs, ",") {
		t.Fatalf("expected coverage keys %v, got %s", wantIDs, got)
	}
	if got := strings.Join(run.ReportKeys(*run.CoverageByKey("structural")), ","); got != "1,3,6" {
		t.Fatalf("expected structural checks 1,3,6, got %s", got)
	}
	clarity, err := grBuildSessionSpec(repoRoot, run, []string{"clarity"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(clarity.CheckKeys, ",") != "10" {
		t.Fatalf("the clarity session must own check 10, got %+v", clarity)
	}
	if run.CoverageByKey("cross") != nil {
		t.Fatal("the final synthesis must not be a coverage key")
	}
	if len(run.RequiredFiles) != 1 || run.RequiredFiles[0] != "docs/specs/units/candidate/unit_auth.md" {
		t.Fatalf("expected the main spec as required file, got %v", run.RequiredFiles)
	}
}

// TestValidateClarityGroupIsNormalCheck pins D8: the clarity group is a single
// normal checks session that owns check 10, reads the unit spec, and obeys the
// standard verdict/finding contract (a FAIL needs a P0/P1 finding).
func TestValidateClarityGroupIsNormalCheck(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	spec := grRunSpec(t, repoRoot, runID, "clarity")
	if strings.Join(spec.CheckKeys, ",") != "10" {
		t.Fatalf("clarity session checks = %v, want 10", spec.CheckKeys)
	}
	if !grContainsString(spec.ReadRefs, main) {
		t.Fatalf("clarity session must read the unit main spec, got %v", spec.ReadRefs)
	}

	noFinding := "10. Clarity: FAIL — ambiguous\n\ncheck-10: " + main + ": Description\n"
	if _, err := grSubmit(t, repoRoot, runID, "clarity", noFinding); err == nil || !strings.Contains(err.Error(), "requires at least one P0/P1 finding") {
		t.Fatalf("expected a clarity FAIL without a finding to be rejected, got %v", err)
	}
	withFinding := "10. Clarity: FAIL — ambiguous\n\n[P1] clarity — the spec is ambiguous (actionable)\n\ncheck-10: " + main + ": Description\n"
	grSubmitOK(t, repoRoot, runID, "clarity", withFinding)
}

func TestGateMissionStructuralContextIncludesUnresolvedLogicalReference(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	grWriteSpecWithRefs(t, repoRoot, "auth", "missing", "none")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	structural := grRunSpec(t, repoRoot, runID, "structural")
	if !grContainsString(structural.ReadRefs, "unit:missing") {
		t.Fatalf("structural session must retain the unresolved Check 1 reference, got %+v", structural)
	}
	if design := grRunSpec(t, repoRoot, runID, "design"); grContainsString(design.ReadRefs, "unit:missing") {
		t.Fatalf("design session must not receive the unrelated logical reference, got %+v", design)
	}

	var stdout, stderr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", repoRoot, "--run", runID, "--keys", "structural"}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-mission failed: %v (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	if !strings.Contains(stdout.String(), "  - unit:missing\n") {
		t.Fatalf("structural execution context must expose the unresolved reference, got:\n%s", stdout.String())
	}
}

// TestGatePlanRejectsUnresolvedImplementationSurface verifies the end-to-end
// fail-closed path: a non-<pending> implementation_surface that cannot
// resolve to a code file rejects verify planning with the item id and
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
// reaches the session read refs instead of being misread as a wildcard
// pattern.
func TestGatePlanVerifyAcceptsLiteralMetacharacterSurface(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	grWriteFile(t, repoRoot, "app/[id]/route.ts", "export {}\n")
	grWriteSpecSurface(t, repoRoot, "demo", "app/[id]/route.ts", "")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "demo", "--target", "candidate")

	var stdout, stderr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", repoRoot, "--run", runID, "--keys", "item:demo:demo.core"}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-mission failed: %v (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}
	if out := stdout.String(); !strings.Contains(out, "  - app/[id]/route.ts\n") {
		t.Fatalf("expected the literal bracketed path in the session read refs, got:\n%s", out)
	}
}

// TestGatePlanRejectsDuplicateAcceptanceItemIDsBeforePersistingRun rejects a
// plan with duplicate acceptance-item ids before any run state is written.
func TestGatePlanRejectsDuplicateAcceptanceItemIDsBeforePersistingRun(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.login"})
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	if _, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate"); err == nil || !strings.Contains(err.Error(), `duplicate key "item:auth:auth.login"`) {
		t.Fatalf("expected duplicate acceptance item ids to reject the coverage set, got %v", err)
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

	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, external)); err == nil || !strings.Contains(err.Error(), "outside the repository root") {
		t.Fatalf("expected an absolute external input to be rejected, got %v", err)
	}

	link := filepath.Join(repoRoot, "external-link")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, "external-link")); err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected an escaping symlink input to be rejected, got %v", err)
	}
}

func coverageKeysOf(run *gaterun.Run) []string {
	var ids []string
	for _, ck := range run.Coverage {
		ids = append(ids, ck.Key)
	}
	return ids
}

// grRunSpec materializes a session spec from the run for one coverage key.
func grRunSpec(t *testing.T, repoRoot, runID, key string) *gaterun.SessionSpec {
	t.Helper()
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := grBuildSessionSpec(repoRoot, run, grSessionKeys(run, key))
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestGatePlanRulePlan(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteFile(t, repoRoot, "docs/specs/rules/candidate/b_rule_http.md", "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Coverage) != 6 {
		t.Fatalf("expected the six rule check keys, got %+v", coverageKeysOf(run))
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
	grSubmitValidateSessions(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
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

// Finalize rides judgment store collection in the same transaction:
// stale-protocol records that no live reference reaches are collected as a
// side effect of publishing the cache, with no extra step in the flow.
func TestGateFinalizeCollectsStaleProtocolOrphans(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")

	plant := func(subject string) judgments.Reference {
		t.Helper()
		report := "stale report " + subject
		ref, err := judgments.Save(repoRoot, judgments.Record{
			Version: judgments.RecordVersion, Kind: judgments.Code, Subject: subject,
			Coverage: []string{"code:" + subject}, Inputs: []string{subject},
			Dependencies: []judgments.Dependency{{Path: subject, Hash: "deadbeef"}},
			Protocol:     "pre-update-protocol-fingerprint", Verdict: "FACTS",
			Result: json.RawMessage(`{"observations":[]}`), Report: report,
			ReportDigest: judgments.Digest([]byte(report)), SourceRun: "20250101-000000-aaaaaa",
		})
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	orphanA := plant("legacy-a.js")
	orphanB := plant("legacy-b.js")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
	out := grFinalizeOK(t, repoRoot, runID, "--result", "pass")
	if !strings.Contains(out, "Judgment collection: 2 stale-protocol record(s)") {
		t.Fatalf("expected collection report in finalize output:\n%s", out)
	}
	listed, err := judgments.List(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range listed {
		if ref.ID == orphanA.ID || ref.ID == orphanB.ID {
			t.Fatal("stale-protocol orphan survived finalize")
		}
	}
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil || !res.Fresh {
		t.Fatalf("expected the published cache to stay fresh after collection: %v %s", err, res.Reason)
	}
}

func TestGateRunRecordsWholeFileEvidence(t *testing.T) {
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
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {main + ": all"},
		"8": {main + ": all"},
	}))
	grSubmitClarity(t, repoRoot, runID, main)
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
	if !strings.Contains(cache, "chunker: "+contenthash.ChunkerVersion) {
		t.Fatalf("expected the chunker version in cache, got:\n%s", cache)
	}
	for _, c := range contenthash.ChunkRecords(text) {
		if !strings.Contains(cache, "cid: "+c.CID) {
			t.Fatalf("expected chunk CID %s in the recorded chunk sequence, got:\n%s", c.CID, cache)
		}
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
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {"unit:auth: Testability / Acceptance Criteria"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, runID, main)
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
	stableRulePath := grWriteFile(t, repoRoot, "docs/specs/rules/stable/g_rule_http.md", "---\nrule_id: g_rule_http\nrule_scope: global\n---\nStable constraint.\n")
	grWriteFile(t, repoRoot, "docs/specs/rules/candidate/g_rule_http.md", "---\nrule_id: g_rule_http\nrule_scope: global\n---\nCandidate draft.\n")
	grWriteFile(t, repoRoot, "docs/specs/rules/candidate/g_rule_draft.md", "---\nrule_id: g_rule_draft\nrule_scope: global\n---\nCandidate-only draft.\n")

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
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {main + ": frontmatter"},
		"8": {"rule:g_rule_http: all"},
	}))
	grSubmitClarity(t, repoRoot, runID, main)
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
	grSubmitClarity(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
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
	if !strings.Contains(cache, `check: "item:auth:auth.core"`) || !strings.Contains(cache, "- path: src/auth.go") {
		t.Fatalf("expected the per-item and code declarations, got:\n%s", cache)
	}
}
func TestGateRunRuleTarget(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var b strings.Builder
	for c := 1; c <= 6; c++ {
		fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
	}
	for c := 1; c <= 6; c++ {
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
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")
	grWriteSpecWithRefs(t, repoRoot, "auth", "none", "b_rule_http")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var b strings.Builder
	for c := 1; c <= 6; c++ {
		fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
	}
	for c := 1; c <= 6; c++ {
		scope, decl := rulePath, "Constraint"
		if c == 4 {
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
// Co-batched code+design sessions
// ------------------------------------------------------------

// TestGateRunCoBatchedCodeAndDesign submits one paired report carrying a
// file's public facts block and its unit design block; the public record
// publishes at acceptance and finalize binds the design record to it.
func TestGateRunCoBatchedCodeAndDesign(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))

	keys := []string{"code:src/auth.go", "design:auth:src/auth.go"}
	paired := "File: code:src/auth.go\n" +
		"conclusion: FACTS\n" +
		"facts: no potential problems in fixture\n" +
		"\n" +
		"File: design:auth:src/auth.go\n" +
		"spec_requirements: fixture design evidence\n" +
		"Architecture assessment:\n" +
		"  conclusion: acceptable\n" +
		"  module_boundaries: sound — reviewed module boundary\n" +
		"  responsibility_organization: sound — reviewed responsibility placement\n" +
		"  dependency_clarity: sound — reviewed dependency direction\n" +
		"  abstraction_level: sound — reviewed abstraction level\n" +
		"  extension_landing_points: sound — reviewed extension seams\n" +
		"  engineering_patterns: sound — reviewed engineering patterns\n" +
		"  gate_findings: none\n" +
		"\n" +
		"Suppressed by spec (0):\n"
	grSubmitKeys(t, repoRoot, runID, keys, paired)

	run, err := gaterun.Load(repoRoot, runID)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := run.Records["code:src/auth.go"]
	if !ok || binding.Source != "executed" {
		t.Fatalf("paired submit must publish the public record, got %+v", binding)
	}
	designCk := run.CoverageByKey("design:auth:src/auth.go")
	if designCk == nil {
		t.Fatal("design coverage key missing")
	}
	public, err := gaterun.PublicResultForDesignKey(repoRoot, run, *designCk)
	if err != nil {
		t.Fatal(err)
	}
	if public.Verdicts["code:src/auth.go"] != "facts" {
		t.Fatalf("expected the published public record, got %+v", public.Verdicts)
	}
	record, err := judgments.Load(repoRoot, binding.Reference)
	if err != nil {
		t.Fatal(err)
	}
	pinned := false
	for _, dep := range record.Dependencies {
		if dep.Path == "src/auth.go" && dep.Hash != "" {
			pinned = true
		}
	}
	if !pinned {
		t.Fatalf("expected the tool-recorded whole-file evidence for the public code check, got %+v", record.Dependencies)
	}

	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	res, err := validationcache.CheckVerify(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected verify cache fresh, got: %s", res.Reason)
	}
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, `check: "code:src/auth.go"`) {
		t.Fatalf("expected the public code check in the cache checks mapping:\n%s", cache)
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
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyMismatchReport("auth.core", main, "src/auth.go", "P0"))
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
	if !strings.Contains(cache, `check: "item:auth:auth.core"`) || !strings.Contains(cache, "status: fail") {
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
	grSubmitClarity(t, repoRoot, runID, main)
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

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", strings.Join(verifyCrossItems, ","))
	grSubmitOK(t, repoRoot, runID, "auth.login", grVerifyItemReport("auth.login", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, runID, "auth.logout", grVerifyItemReport("auth.logout", main, "src/auth.go"))
	missingAffected := "Cross-check: 4/5 FAIL — combined contract contradiction\n\n[P1] cross — individually valid claims conflict when combined (actionable)\n\ncross: " + main + ": Description\nEffective status: auth.login = fail\nEffective status: auth.logout = pass\nEffective status: cross = fail\n"
	if _, err := grSubmit(t, repoRoot, runID, "cross", missingAffected); err == nil || !strings.Contains(err.Error(), "at least one affected non-cross logical key") {
		t.Fatalf("expected a new cross finding without affected keys to be rejected, got %v", err)
	}
	cross := "Cross-check: 4/5 FAIL — combined contract contradiction\n\n[P1] cross — individually valid claims conflict when combined (actionable)\nFinding affects: " + grRunFindingID(runID, "cross", 1) + " = auth.login\n\ncross: " + main + ": Description\nEffective status: auth.login = fail\nEffective status: auth.logout = pass\nEffective status: cross = fail\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "blocking: true") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("expected cross failure to force a blocking derived result, got:\n%s", cache)
	}
	repairID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	repair := mustLoadRun(t, repoRoot, repairID)
	if got := strings.Join(coverageKeysOf(repair), ","); got != "item:auth:auth.login" {
		t.Fatalf("expected the cross finding's affected key to drive repair scope, got %s", got)
	}
	if got := strings.Join(repair.CarriedKeys, ","); got != "architecture:auth,code:src/auth.go,design:auth:src/auth.go,item:auth:auth.logout,relationship:contract_consistency,relationship:cross_reference_integrity,relationship:data_definition_drift,relationship:error_code_conflict" {
		t.Fatalf("expected unaffected auth.logout and the quality key to be carried, got %s", got)
	}
}

func TestGateVerifyItemIDWithRegexMetacharacterSubmits(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core", "auth.co(re"})
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "detect:auth.co(re", grVerifyItemReport("auth.co(re", main, "src/auth.go"))
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
	good := grVerifyItemBody("auth.core", "MISMATCH (structural)", "broken at src/auth.go:1") + grVerifyAnalysisFields("auth.core") + "auth.core: " + main + ": Testability / Acceptance Criteria\n"
	grSubmitOK(t, repoRoot, runID, "auth.core", good)
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
	labeled += fmt.Sprintf("Finding affects: %s = 2\n", grRunFindingID(runID, "design", 1))
	grSubmitOK(t, repoRoot, runID, "design", labeled)
}

// TestResolveCrossFindingsRaisesSeverityOnMerge pins D9's conservative
// severity handling: a finding retained or merged into a terminal group takes
// the most severe grade among the group — its detail prefix is rewritten to
// match — with no severity-confirmation chain required.
func TestResolveCrossFindingsRaisesSeverityOnMerge(t *testing.T) {
	input := []gaterun.Finding{
		{ID: "p/F1", Severity: "P1", Text: "high", Detail: "[P1] high — issue", SourceKey: "1"},
		{ID: "p/F2", Severity: "P2", Text: "low", Detail: "[P2] low — issue", SourceKey: "2"},
	}
	dispositions := []gaterun.FindingDisposition{
		{FindingID: "p/F1", Action: "merged", TargetID: "p/F2"},
		{FindingID: "p/F2", Action: "retained"},
	}
	got, err := resolveCrossFindings(input, nil, dispositions)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "p/F2" || got[0].Severity != "P1" || !strings.HasPrefix(got[0].Detail, "[P1]") {
		t.Fatalf("expected the merged group raised to its most severe grade P1, got %+v", got)
	}
	if len(got[0].AffectedKeys) != 1 || got[0].AffectedKeys[0] != "1" {
		t.Fatalf("expected the merged key to flow to the terminal finding, got %+v", got[0])
	}
}

// TestGateVerifyMismatchFindingIsSelfContained pins the merged verify-item
// contract: a MISMATCH session composes one run-scoped finding whose detail
// carries the shared finding block (problem, evidence, impact, fix, root cause,
// direction), and the final synthesis mission carries that finding as a compact
// judgment record without embedding the report prose.
func TestGateVerifyMismatchFindingIsSelfContained(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyMismatchReport("auth.core", main, "src/auth.go", "P1"))
	state, err := grLoadSessionState(repoRoot, mustLoadRun(t, repoRoot, runID), "auth.core")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Result.Findings) != 1 || state.Result.Findings[0].ID != grRunFindingID(runID, "auth.core", 1) {
		t.Fatalf("expected the mismatch finding to carry the run-scoped id, got %+v", state.Result.Findings)
	}
	detail := state.Result.Findings[0].Detail
	for _, want := range []string{"problem:", "evidence:", "- spec:", "- code:", "impact:", "fix:", "root_cause:", "direction:"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("expected the finding detail to carry the self-contained finding block (%q), got:\n%s", want, detail)
		}
	}

	var contextOut, contextErr bytes.Buffer
	if err := runGateMission([]string{"--repo-root", repoRoot, "--run", runID, "--final"}, &contextOut, &contextErr); err != nil {
		t.Fatal(err)
	}
	contextText := contextOut.String()
	for _, want := range []string{
		"auth.core",
		"verdicts: item:auth:auth.core=MISMATCH",
		"root_cause: incomplete",
		grRunFindingID(runID, "auth.core", 1),
	} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("expected the final synthesis context to carry the compact judgment record (%q), got:\n%s", want, contextText)
		}
	}
	if strings.Contains(contextText, "broken at src/auth.go:1") {
		t.Fatalf("the final synthesis context must not embed the accepted report, got:\n%s", contextText)
	}
}

// TestVerifyItemBindsOnlyItsOwnResultInDelta pins the result-binding contract:
// a verify item session consumes no other session's result, so its consumed
// digests stay empty (only the final synthesis binds every accepted result
// plus the carried judgments).
func TestVerifyItemBindsOnlyItsOwnResultInDelta(t *testing.T) {
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
	edited := strings.Replace(string(data), "  - id: auth.login\n    description: Given a caller, When the behavior runs, Then it is accepted.", "  - id: auth.login\n    description: Given a caller, When the behavior runs, Then it is accepted. With an edit.", 1)
	if edited == string(data) {
		t.Fatal("item edit did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(coverageKeysOf(deltaRun), "review") || !grContainsString(coverageKeysOf(deltaRun), "item:auth:auth.login") {
		t.Fatalf("expected the review and the invalidated item in coverage, got %v", coverageKeysOf(deltaRun))
	}
	// The reviewer re-runs only auth.login; the rest is carried. The design
	// and architecture judgments pinned the acceptance item set, so the item
	// edit forced them to re-run mechanically.
	grReviewRecheck(t, repoRoot, deltaID, "auth.login")
	deltaRun = mustLoadRun(t, repoRoot, deltaID)
	for _, want := range []string{"code:src/auth.go", "item:auth:auth.logout"} {
		if !grContainsString(deltaRun.CarriedKeys, want) {
			t.Fatalf("expected %s carried over, got %v", want, deltaRun.CarriedKeys)
		}
	}
	if grContainsString(deltaRun.CarriedKeys, "item:auth:auth.login") {
		t.Fatalf("rechecked item must leave the carry set: %v", deltaRun.CarriedKeys)
	}
	grSubmitOK(t, repoRoot, deltaID, "auth.login", grVerifyMismatchReport("auth.login", main, "src/auth.go", "P1"))

	state, err := grLoadSessionState(repoRoot, deltaRun, "auth.login")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ConsumedResultDigests) != 0 {
		t.Fatalf("expected a verify item to bind no other result, got %v", state.ConsumedResultDigests)
	}
}

func TestGateFinalizeRejectsCrossDigestMismatch(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grSubmitClarity(t, repoRoot, runID, main)
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	run := mustLoadRun(t, repoRoot, runID)
	state, err := grLoadSessionState(repoRoot, run, "cross")
	if err != nil {
		t.Fatal(err)
	}
	for key := range state.ConsumedResultDigests {
		state.ConsumedResultDigests[key] = "sha256:tampered"
		break
	}
	if err := gaterun.SaveSessionState(repoRoot, run, state); err != nil {
		t.Fatal(err)
	}
	if _, err := grFinalize(t, repoRoot, runID); err == nil || !strings.Contains(err.Error(), "consumed digest") {
		t.Fatalf("expected consumed-result digest mismatch to reject finalize, got %v", err)
	}
}

func TestGateRunValidateCandidateFailWritesFailureRecord(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	// Seed an existing pass cache that the full-run failure must replace.
	seedRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, seedRun, main)
	grFinalizeOK(t, repoRoot, seedRun, "--result", "pass")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", "2. Design soundness: FAIL — contradiction\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n[P1] design — contradiction (actionable)\n\ncheck-2: "+main+": Description\ncheck-4: "+main+": Description\n"+fmt.Sprintf("Finding affects: %s = 2\n", grRunFindingID(runID, "design", 1)))
	grSubmitOK(t, repoRoot, runID, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Testability / Acceptance Criteria"}}))
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, runID, main)
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	out := grFinalizeOK(t, repoRoot, runID, "--result", "fail", "--p0-count", "1")
	if !strings.Contains(out, "Self-check: BLOCKED") || !strings.Contains(out, "Cache written:") {
		t.Fatalf("expected a published failure record, got:\n%s", out)
	}

	// The failure record replaces the prior pass cache — same shape as a
	// verify full-run FAIL: basis: full, complete pass/fail status map,
	// no carried entries.
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	for _, want := range []string{"result: fail", "blocking: true", "basis: full", `check: "2"`, "status: fail"} {
		if !strings.Contains(cache, want) {
			t.Fatalf("expected %q in the failure record, got:\n%s", want, cache)
		}
	}
	if strings.Contains(cache, "status: carried") {
		t.Fatalf("a full-run record must not carry judgments, got:\n%s", cache)
	}
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if res.Category != validationcache.CategoryBlocked {
		t.Fatalf("expected BLOCKED, got %q: %s", res.Category, res.Reason)
	}

	// fresh shows BLOCKED with the repair recovery advice.
	freshOut, err := freshRun(t, repoRoot, "--unit", "auth")
	if err != nil {
		t.Fatal(err)
	}
	assertGateStatus(t, freshOut, "validate", "BLOCKED")
	if !strings.Contains(freshOut, "revalidate@auth (repair recovery from the failure record)") {
		t.Fatalf("expected the repair advice, got:\n%s", freshOut)
	}
}

// TestGateRunValidateFullFailRepairFlow: a candidate validate full FAIL is the
// failure-recovery baseline — the delta mode refuses it, the repair plan
// re-runs the failed session plus cross and carries the passed checks over, and
// the repaired cache is promote-consumable with basis: repair.
func TestGateRunValidateFullFailRepairFlow(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, fullRun, "design", "2. Design soundness: FAIL — contradiction\n4. Evidence-driven vs design-driven consistency: PASS — ok\n\n[P1] design — contradiction (actionable)\n\ncheck-2: "+main+": Description\ncheck-4: "+main+": Description\n"+fmt.Sprintf("Finding affects: %s = 2\n", grRunFindingID(fullRun, "design", 1)))
	grSubmitOK(t, repoRoot, fullRun, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Testability / Acceptance Criteria"}}))
	grSubmitOK(t, repoRoot, fullRun, "dependencies", grDependenciesReport(map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, fullRun, main)
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun, "--result", "fail", "--p0-count", "1")

	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta"); err == nil || !strings.Contains(err.Error(), "--mode repair") {
		t.Fatalf("expected the delta-mode refusal on a failure baseline, got %v", err)
	}
	repairRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, repoRoot, repairRun)
	if got := strings.Join(coverageKeysOf(run), ","); got != "design" {
		t.Fatalf("expected the failed design group, got %v", got)
	}
	if got := strings.Join(run.CarriedKeys, ","); got != "1,10,3,5,6,7,8,9" {
		t.Fatalf("expected the passed checks carried over, got %v", got)
	}

	grSubmitOK(t, repoRoot, repairRun, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, repairRun, main)
	grSubmitOK(t, repoRoot, repairRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, repairRun, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "basis: repair") || !strings.Contains(cache, `check: "2"`) {
		t.Fatalf("expected a repair cache carrying the passed checks, got:\n%s", cache)
	}
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected the repair cache to be fresh, got: %s", res.Reason)
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
	// A FAIL verdict with no P0/P1 finding is inconsistent.
	failWithoutFinding := "1. Structural integrity: FAIL — broken\n3. Scope integrity: PASS — ok\n6. Affects-source validity: PASS — ok\n"
	if _, err := grSubmit(t, repoRoot, runID, "structural", failWithoutFinding); err == nil || !strings.Contains(err.Error(), "P0/P1 finding") {
		t.Fatalf("expected a FAIL-without-finding rejection, got %v", err)
	}
	// A report with no dependency declarations is valid: reports carry no
	// scope lines — the tooling records the run's input surface.

	// The rejected attempts are recorded; a valid submission is accepted and
	// terminal.
	grSubmitValidateSessions(t, repoRoot, runID, main)
	state, err := grLoadSessionState(repoRoot, mustLoadRun(t, repoRoot, runID), "structural")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Attempts) != 4 {
		t.Fatalf("expected 4 recorded attempts (3 rejected + 1 accepted), got %d", len(state.Attempts))
	}
	if state.Status != gaterun.SessionAccepted {
		t.Fatalf("expected the structural session accepted, got %q", state.Status)
	}
	if _, err := grSubmit(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	})); err == nil || !strings.Contains(err.Error(), "already covered") {
		t.Fatalf("expected the accepted session to be terminal, got %v", err)
	}
	for i, a := range state.Attempts {
		if i < 3 && (a.Status != "rejected" || a.RejectionReason == "") {
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
			errs <- runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID, "--session", "acceptance", "--keys", "acceptance", "--report", path}, &stdout, &stderr)
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
		case strings.Contains(err.Error(), "already covered"):
			terminalFailures++
		default:
			t.Fatalf("unexpected concurrent submit result: %v", err)
		}
	}
	if successes != 1 || terminalFailures != 1 {
		t.Fatalf("expected one accepted submission and one terminal rejection, got success=%d terminal=%d", successes, terminalFailures)
	}

	run := mustLoadRun(t, repoRoot, runID)
	state, err := grLoadSessionState(repoRoot, run, "acceptance")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != gaterun.SessionAccepted || len(state.Attempts) != 1 {
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
	grSubmitValidateSessions(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")

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

	// The structural session declares the protocol appendix as evidence, so
	// the appendix reaches the validated surface.
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, runID, main, appendix+": all")
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected FRESH with the carrier appendix, got: %s", res.Reason)
	}

	// A rejected finalize must not destroy or rewrite the last valid
	// canonical cache. Re-validation and publication are ordered: an error
	// leaves the cache untouched.
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	runID = grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	if _, err := grFinalize(t, repoRoot, runID, "--result", "pass"); err == nil {
		t.Fatal("expected finalize with pending sessions to reject")
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
	grSubmitValidateSessions(t, repoRoot, runID, main, appendix+": all")
	grFinalizeOK(t, repoRoot, runID, "--result", "pass", "--timestamp", "2026-09-17T12:34:56Z")
	replaced, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(replaced, before) || !bytes.Contains(replaced, []byte(`timestamp: "2026-09-17T12:34:56Z"`)) {
		t.Fatal("successful finalize did not replace the canonical cache")
	}
}

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
	matching := []validationcache.FileEntry{{Path: "src/auth.go", Hash: "sha256:" + expected}}
	if got := evidenceSnapshotDivergences(repoRoot, run, matching); len(got) != 0 {
		t.Fatalf("matching evidence unexpectedly diverged: %v", got)
	}
	// Every entry is bound to the plan snapshot: a whole-file change between
	// plan and finalize must reject the finalize, whoever assembled it.
	mismatched := []validationcache.FileEntry{{Path: "src/auth.go", Hash: "sha256:different"}}
	if got := evidenceSnapshotDivergences(repoRoot, run, mismatched); !reflect.DeepEqual(got, []string{"evidence differs from snapshot: src/auth.go"}) {
		t.Fatalf("unexpected mismatch result: %v", got)
	}
	missing := []validationcache.FileEntry{{Path: "src/not-planned.go", Hash: expected}}
	if got := evidenceSnapshotDivergences(repoRoot, run, missing); !reflect.DeepEqual(got, []string{"evidence path absent from snapshot: src/not-planned.go"}) {
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
	grWriteFile(t, repoRoot, "tests/auth_test.go", "package tests\n")

	cases := []struct {
		name     string
		plan     []string
		sessions func(runID string)
		mutate   func()
		want     string
	}{
		{
			name: "modified spec",
			plan: []string{"--gate", "validate", "--unit", "auth", "--target", "candidate"},
			sessions: func(runID string) {
				grSubmitValidateSessions(t, repoRoot, runID, main, appendix+": all")
			},
			mutate: func() {
				data, _ := os.ReadFile(specPath)
				os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose. Edited.", 1)), 0644)
			},
			want: "modified: " + main,
		},
		{
			name: "modified appendix",
			plan: []string{"--gate", "validate", "--unit", "auth", "--target", "candidate"},
			sessions: func(runID string) {
				grSubmitValidateSessions(t, repoRoot, runID, main, appendix+": all")
			},
			mutate: func() {
				grWriteFile(t, repoRoot, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nPOST /login.\n")
			},
			want: "modified: " + appendix,
		},
		{
			name: "modified code",
			plan: []string{"--gate", "verify", "--unit", "auth", "--target", "candidate"},
			sessions: func(runID string) {
				grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
				grSubmitClarity(t, repoRoot, runID, main)
				grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
			},
			mutate: func() { grWriteFile(t, repoRoot, "src/auth.go", "package auth // edited\n") },
			want:   "modified: src/auth.go",
		},
		{
			name: "modified discovered test input",
			plan: []string{"--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, "tests/auth_test.go")},
			sessions: func(runID string) {
				grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
				grSubmitClarity(t, repoRoot, runID, main)
				grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
			},
			mutate: func() { grWriteFile(t, repoRoot, "tests/auth_test.go", "package tests // edited\n") },
			want:   "modified: tests/auth_test.go",
		},
		{
			name: "added surface file",
			plan: []string{"--gate", "verify", "--unit", "auth", "--target", "candidate"},
			sessions: func(runID string) {
				grSubmitOK(t, repoRoot, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
				grSubmitClarity(t, repoRoot, runID, main)
				grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
			},
			mutate: func() { grWriteFile(t, repoRoot, "src/extra.go", "package auth\n") },
			want:   "added: src/extra.go",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := grPlan(t, repoRoot, tc.plan...)
			tc.sessions(runID)
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
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {"unit:auth: Description"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, runID, main)
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
	grEnableMissionLayout(t, repoRoot)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, fullRun, main)
	grFinalizeOK(t, repoRoot, fullRun, "--result", "pass")

	// Editing the Description section stales the checks that declared it.
	data, _ := os.ReadFile(specPath)
	os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose. Edited.", 1)), 0644)

	deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if got := coverageKeysOf(run); strings.Join(got, ",") != "review" {
		t.Fatalf("expected the review coverage only, got %v", got)
	}
	if len(run.CarriedKeys) != 10 || !containsString(run.CarriedKeys, "5") {
		t.Fatalf("expected every standing check as a carry candidate, got %v", run.CarriedKeys)
	}
	if run.ChangeSetFP == "" {
		t.Fatal("expected the change-set fingerprint on the run")
	}
	reviewMission := missionJSON(t, repoRoot, deltaRun, "review")
	if reviewMission.Mode != "delta" || strings.Join(reviewMission.Sessions[0].CheckKeys, ",") != "review" {
		t.Fatalf("expected a delta review mission, got %+v", reviewMission)
	}
	if len(reviewMission.Sessions[0].ChangeSet) == 0 || len(reviewMission.Sessions[0].Standing) == 0 {
		t.Fatalf("review mission must carry the change set and standing conclusions: %+v", reviewMission.Sessions[0])
	}

	// Carried sessions cannot be submitted: they are not part of the plan.
	if _, err := grSubmit(t, repoRoot, deltaRun, "acceptance", grValidateReport([]string{"5"}, map[string][]string{"5": {main + ": Testability / Acceptance Criteria"}})); err == nil || !strings.Contains(err.Error(), "not part of") {
		t.Fatalf("expected the carried session to be absent from the plan, got %v", err)
	}

	// The reviewer names one check per affected group; each group re-runs.
	grReviewRecheck(t, repoRoot, deltaRun, "1", "2", "7", "10")
	grSubmitOK(t, repoRoot, deltaRun, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Testability / Acceptance Criteria"},
		"6": {main + ": Testability / Acceptance Criteria"},
	}))
	grSubmitOK(t, repoRoot, deltaRun, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, deltaRun, "dependencies", grDependenciesReport(map[string][]string{
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, deltaRun, main)
	grFinalizeOK(t, repoRoot, deltaRun, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected basis: delta, got:\n%s", cache)
	}
	if !strings.Contains(cache, "reviewed_change_set:") || !strings.Contains(cache, "review_result: recheck") {
		t.Fatalf("expected the review record in the cache, got:\n%s", cache)
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

	// A delta plan with nothing changed is refused.
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta"); err == nil || !strings.Contains(err.Error(), "cache is fresh") {
		t.Fatalf("expected the freshness refusal, got %v", err)
	}
}

// TestGateRunPeerEditDoesNotStaleValidateCache pins the one-way dependency
// direction (issue #68): a unit validate reads only its declared unit_refs and
// rule_refs, never unrelated peers — Check 9 is the mechanical
// `specflowctl surfaces` audit. A peer's surface-declaration edit must not
// stale the target's validate cache or force a delta re-run.
func TestGateRunPeerEditDoesNotStaleValidateCache(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grEnableMissionLayout(t, repoRoot)
	specPath := grWriteSpec(t, repoRoot, "auth")
	peerPath := grWriteSpec(t, repoRoot, "beta")
	main := specPath
	desc := main + ": Description"
	accept := main + ": Testability / Acceptance Criteria"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": frontmatter", desc},
		"3": {accept},
		"6": {accept},
	}))
	grSubmitOK(t, repoRoot, fullRun, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {desc},
		"4": {desc},
	}))
	grSubmitOK(t, repoRoot, fullRun, "acceptance", grValidateReport([]string{"5"}, map[string][]string{
		"5": {accept},
	}))
	grSubmitOK(t, repoRoot, fullRun, "dependencies", grDependenciesReport(map[string][]string{
		"7": {desc},
		"8": {desc},
		"9": {accept},
	}))
	grSubmitClarity(t, repoRoot, fullRun, main)
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun, "--result", "pass")

	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected the fresh cache, got: %s", res.Reason)
	}

	// An unrelated peer's edit must leave auth's validate cache fresh: beta is
	// not in auth's unit_refs, so it is not part of auth's input surface.
	data, err := os.ReadFile(peerPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(peerPath, []byte(strings.Replace(string(data), "Given a caller, When the behavior runs, Then it is accepted.", "Given a caller, When the behavior runs, Then it is accepted. Changed.", 1)), 0644); err != nil {
		t.Fatal(err)
	}

	res, err = validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("peer edit must not stale the validate cache, got stale: %s", res.Reason)
	}

	// Nothing in auth's own surface changed, so a delta plan is refused.
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta"); err == nil || !strings.Contains(err.Error(), "cache is fresh") {
		t.Fatalf("expected the freshness refusal after an unrelated peer edit, got %v", err)
	}
}

func TestGateRunRuleConsecutivePartialDeltasKeepCompleteJudgments(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")
	consumerPath := grWriteSpecWithRefs(t, repoRoot, "auth", "none", "b_rule_http")

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	fullScopes := map[string][]string{}
	for c := 1; c <= 6; c++ {
		key := fmt.Sprint(c)
		fullScopes[key] = []string{rulePath + ": Constraint"}
	}
	fullScopes["4"] = []string{"unit:auth: Testability / Acceptance Criteria"}
	grSubmitOK(t, repoRoot, fullRun, "checks", grValidateReport([]string{"1", "2", "3", "4", "5", "6"}, fullScopes))
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
		if got := strings.Join(coverageKeysOf(run), ","); got != "review" {
			t.Fatalf("expected the review coverage only, got %s", got)
		}
		grReviewRecheck(t, repoRoot, deltaRun, "4")
		run = mustLoadRun(t, repoRoot, deltaRun)
		if got := strings.Join(run.CarriedKeys, ","); got != "1,2,3,5,6" {
			t.Fatalf("unexpected carried checks: %s", got)
		}
		grSubmitKeys(t, repoRoot, deltaRun, []string{"4"}, grValidateReport([]string{"4"}, map[string][]string{
			"4": {"unit:auth: Testability / Acceptance Criteria"},
		}))
		grFinalizeOK(t, repoRoot, deltaRun)

		state := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindRule, "b_rule_http", gaterun.GateValidate)
		if len(state.LogicalStatus) != 6 {
			t.Fatalf("expected a complete 6-check judgment baseline after delta, got %v", state.LogicalStatus)
		}
		for c := 1; c <= 6; c++ {
			if state.LogicalStatus[fmt.Sprint(c)] != "pass" {
				t.Fatalf("expected check %d status pass after delta, got %q", c, state.LogicalStatus[fmt.Sprint(c)])
			}
		}
	}

	runDelta("Given a caller, When the behavior runs, Then it is accepted.", "Given a caller, When the behavior runs, Then it is accepted. Changed.")
	runDelta("Given a caller, When the behavior runs, Then it is accepted. Changed.", "Given a caller, When the behavior runs, Then it is accepted. Changed again.")
}

func TestRuleOutcomeRejectsCarriedRerunOverlap(t *testing.T) {
	run := &gaterun.Run{
		TargetKind:  gaterun.TargetKindRule,
		CarriedKeys: []string{"1"},
		CarriedResults: []gaterun.SessionResult{{
			SessionID:       "carried:1",
			EffectiveStatus: map[string]string{"1": "pass"},
		}},
	}
	report := reportRef{
		spec: &gaterun.SessionSpec{SessionID: "checks", Kind: gaterun.SessionKindChecks},
		result: &gaterun.SessionResult{
			SessionID: "checks",
			Verdicts:  map[string]string{"1": "PASS"},
		},
	}
	if _, err := deriveGateOutcome(run, []reportRef{report}); err == nil || !strings.Contains(err.Error(), "both carried and re-run") {
		t.Fatalf("expected carried/re-run overlap to fail closed, got %v", err)
	}
}

func TestRuleOutcomeDeduplicatesCarriedFindings(t *testing.T) {
	shared := gaterun.Finding{ID: "checks/F1", Severity: "P1", Text: "shared", SourceKey: "1", AffectedKeys: []string{"2"}}
	run := &gaterun.Run{TargetKind: gaterun.TargetKindRule}
	for i := 1; i <= 5; i++ {
		key := fmt.Sprint(i)
		run.CarriedKeys = append(run.CarriedKeys, key)
		status := "pass"
		if i == 1 || i == 2 {
			status = "fail"
		}
		result := gaterun.SessionResult{SessionID: "carried:" + key, EffectiveStatus: map[string]string{key: status}}
		if i == 1 || i == 2 {
			result.Findings = []gaterun.Finding{shared}
		}
		run.CarriedResults = append(run.CarriedResults, result)
	}
	report := reportRef{
		spec: &gaterun.SessionSpec{SessionID: "checks", Kind: gaterun.SessionKindChecks},
		result: &gaterun.SessionResult{
			SessionID: "checks",
			Verdicts:  map[string]string{"6": "PASS"},
		},
	}
	outcome, err := deriveGateOutcome(run, []reportRef{report})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Findings) != 1 || outcome.Counts[1] != 1 || len(outcome.EffectiveStatus) != 6 || outcome.SynthesisDigest == "" {
		t.Fatalf("expected one canonical carried finding and a complete outcome, got %+v", outcome)
	}
}

func TestCandidateValidateDeltaFailPreservesRecoveryRecord(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, fullRun, main)
	grFinalizeOK(t, repoRoot, fullRun)

	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "## Description\n\nProse.", "## Description\n\nProse. Changed.", 1)
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	// The reviewer re-runs the design group; its check 2 will fail.
	grReviewRecheck(t, repoRoot, deltaRun, "2")
	run := mustLoadRun(t, repoRoot, deltaRun)
	for _, ck := range run.Coverage {
		if ck.Kind == gaterun.SessionKindDeltaReview {
			continue
		}
		checks := run.ReportKeys(ck)
		scopes := map[string][]string{}
		for _, key := range checks {
			scopes[key] = []string{main + ": Description"}
		}
		report := grValidateReport(checks, scopes)
		if ck.Key == "design" {
			report = strings.Replace(report, "2. Design soundness: PASS — checked", "2. Design soundness: FAIL — combined design contradiction", 1)
			report += "\n[P1] design — combined design contradiction (actionable)\n"
			report += fmt.Sprintf("Finding affects: %s = 2\n", grRunFindingID(deltaRun, "design", 1))
		}
		grSubmitOK(t, repoRoot, deltaRun, ck.Key, report)
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
	grEnableMissionLayout(t, repoRoot)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.login", "auth.logout"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	fullRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "auth.login", grVerifyMismatchReport("auth.login", main, "src/auth.go", "P0"))
	grSubmitOK(t, repoRoot, fullRun, "auth.logout", grVerifyItemReport("auth.logout", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun, "--result", "fail", "--p0-count", "1")

	repairRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, repoRoot, repairRun)
	if got := coverageKeysOf(run); strings.Join(got, ",") != "item:auth:auth.login" {
		t.Fatalf("expected the failed item, got %v", got)
	}
	if strings.Join(run.CarriedKeys, ",") != "architecture:auth,code:src/auth.go,design:auth:src/auth.go,item:auth:auth.logout" {
		t.Fatalf("expected auth.logout and the quality key carried over, got %v", run.CarriedKeys)
	}
	repairMission := missionJSON(t, repoRoot, repairRun, "auth.login")
	if repairMission.Mode != "repair" || strings.Join(repairMission.Sessions[0].CheckKeys, ",") != "item:auth:auth.login" {
		t.Fatalf("repair mission repeats a carried check: %+v", repairMission)
	}
	grSubmitOK(t, repoRoot, repairRun, "auth.login", grVerifyItemReport("auth.login", main, "src/auth.go"))
	repairCross := missionJSON(t, repoRoot, repairRun, "cross")
	if len(repairCross.Sessions[0].CarriedResults) == 0 || len(repairCross.Sessions[0].Dependencies) != 1 {
		t.Fatalf("repair cross mission lost carried or dependency results: %+v", repairCross.Sessions[0])
	}
	grSubmitClarity(t, repoRoot, repairRun, main)
	grSubmitOK(t, repoRoot, repairRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, repairRun, "--result", "pass")

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "basis: repair") || !strings.Contains(cache, `check: "item:auth:auth.logout"`) {
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
	if !strings.Contains(stdout.String(), runID) || !strings.Contains(stdout.String(), "coverage 1/5 covered") || !strings.Contains(stdout.String(), "1 session(s) (1 accepted, 0 pending, 0 rejected)") {
		t.Fatalf("unexpected status listing:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := runGateStatus([]string{"--repo-root", repoRoot, "--run", runID}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "accepted") || !strings.Contains(out, "gate-submit") {
		t.Fatalf("expected session detail and the next submit action, got:\n%s", out)
	}
	if !strings.Contains(out, "next rejection") && !strings.Contains(out, "pending") {
		t.Fatalf("expected pending session detail, got:\n%s", out)
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
		{"missing gate", []string{"--unit", "auth", "--target", "candidate"}, "must be validate or verify"},
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
	if err := runGateSubmit([]string{"--repo-root", repoRoot, "--run", runID}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "--session") {
		t.Fatalf("expected the gate-submit usage error, got %v", err)
	}
}

func TestGateRunConsumedRunCloses(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, runID, main)
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	// The finalize concluded the run by removing its state, so the run can no
	// longer accept a second finalize or any submission.
	if _, err := grFinalize(t, repoRoot, runID, "--result", "pass"); err == nil || !strings.Contains(err.Error(), "cannot read gate run") {
		t.Fatalf("expected a concluded run to reject a second finalize, got %v", err)
	}
	if _, err := grSubmit(t, repoRoot, runID, "cross", grCrossReport(main, "Description")); err == nil || !strings.Contains(err.Error(), "cannot read gate run") {
		t.Fatalf("expected a concluded run to reject submissions, got %v", err)
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
	grSubmitValidateSessions(t, repoRoot, second, "docs/specs/units/candidate/unit_auth.md")
	grFinalizeOK(t, repoRoot, second, "--result", "pass")
}

func TestGateRunRecordsAuditRunID(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
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
	grSubmitValidateSessions(t, repoRoot, runID, "docs/specs/units/candidate/unit_auth.md")
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
	grSubmitValidateSessions(t, repoRoot, runID, main)
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

	data, _ := os.ReadFile(specPath)
	os.WriteFile(specPath, []byte(strings.Replace(string(data), "Prose.", "Prose. Edited.", 1)), 0644)

	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if res.Fresh {
		t.Fatal("expected the cache to go stale after editing a recorded file")
	}
	report, err := validationcache.DeriveChangeReport(repoRoot, "unit", "auth", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if report.Empty() {
		t.Fatal("expected the recorded content change in the change report")
	}
}

// ------------------------------------------------------------
// Stable-only targets
// ------------------------------------------------------------
func TestGatePlanStableRequiresStableOnly(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteFile(t, repoRoot, "docs/specs/units/stable/unit_auth.md", "---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# Auth\n")
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--unit", "auth", "--target", "stable"); err == nil || !strings.Contains(err.Error(), "stable-only") {
		t.Fatalf("expected a stable-only error, got %v", err)
	}
}

// ------------------------------------------------------------
// Path-form validation
// ------------------------------------------------------------

func TestGateRunOverlappingLogicalInputIsAvailableToEverySession(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	grWriteSpecWithRefs(t, repoRoot, "self", "auth", "none")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "self", "--target", "candidate", "--inputs-file", grInputsManifest(t, "unit:auth"))
	run := mustLoadRun(t, repoRoot, runID)
	for _, key := range []string{"structural", "design", "acceptance", "dependencies"} {
		spec := grRunSpec(t, repoRoot, runID, key)
		if !containsString(spec.ReadRefs, "unit:auth") {
			t.Fatalf("explicit logical input must be available to %s, got %+v", key, spec.ReadRefs)
		}
	}
	crossSpec, err := grBuildSessionSpec(repoRoot, run, []string{gaterun.CrossKey})
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(crossSpec.ReadRefs, "unit:auth") {
		t.Fatalf("explicit logical input must be available to the final synthesis, got %+v", crossSpec.ReadRefs)
	}
}

// grVerifyItemRegionReport builds a verify item session report whose own-spec
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
	if !strings.Contains(cache, "chunker: ") || !strings.Contains(cache, "chunks:") {
		t.Fatalf("expected the whole-file chunk evidence in the verify cache, got:\n%s", cache)
	}
	original := grReadJudgmentBaseline(t, repoRoot, "unit", "auth", "verify")
	// Each item judgment pins its own item region (located by id), so
	// reordering items never invalidates the recorded conclusion.
	for _, item := range []string{"auth.login", "auth.logout"} {
		binding, ok := original.Records["item:auth:"+item]
		if !ok {
			t.Fatalf("no judgment record for item %s", item)
		}
		record, err := judgments.Load(repoRoot, binding.Reference)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, dep := range record.Dependencies {
			for _, d := range dep.Deps {
				if strings.HasPrefix(d, "region:acceptance_item:"+item+":") {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("item %s judgment does not pin its item region: %+v", item, record.Dependencies)
		}
	}

	// Reorder the complete item blocks without changing their content. Both
	// item-level evidence and the final synthesis's semantic whole-set
	// judgment must stay fresh (item regions are order-insensitive).
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
	// Whole-file cache freshness flags the reorder as a content change: the
	// delta review judges what it means (the item records above survive it).
	if res.Fresh {
		t.Fatalf("expected the reorder to be detected as a whole-file content change")
	}

	// Edit only the auth.login item — auth.logout's evidence is unchanged.
	data, _ := os.ReadFile(specPath)
	edited := strings.Replace(string(data), "  - id: auth.login\n    description: Given a caller, When the behavior runs, Then it is accepted.", "  - id: auth.login\n    description: Given a caller, When the behavior runs, Then it is accepted. With an edit.", 1)
	if edited == string(data) {
		t.Fatal("item edit did not apply — fixture assumption broken")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	// The item edit changes the item's own region (auth.login re-runs) and the
	// acceptance item set the design and architecture judgments pin (they
	// re-run too); the other item's region is untouched.
	if !grContainsString(coverageKeysOf(run), "review") || !grContainsString(coverageKeysOf(run), "item:auth:auth.login") {
		t.Fatalf("expected the review and the invalidated item in coverage, got %v", coverageKeysOf(run))
	}
	if !grContainsString(run.CarriedKeys, "item:auth:auth.logout") {
		t.Fatalf("expected auth.logout carried over, got %v", run.CarriedKeys)
	}
	grReviewRecheck(t, repoRoot, deltaRun, "auth.login")
	run = mustLoadRun(t, repoRoot, deltaRun)
	if grContainsString(run.CarriedKeys, "item:auth:auth.login") {
		t.Fatalf("rechecked item must leave the carry set: %v", run.CarriedKeys)
	}

	grSubmitOK(t, repoRoot, deltaRun, "auth.login", grVerifyItemRegionReport("auth.login", main, "src/auth.go"))
	grAutoSubmitQuality(t, repoRoot, deltaRun)
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

	// The unchanged item keeps its reviewed decision but must be accepted in
	// the new spec context so stable protection can reuse it after promotion.
	state := grReadJudgmentBaseline(t, repoRoot, "unit", "auth", "verify")
	binding := state.Records["item:auth:auth.logout"]
	if binding.Source != "carried" || binding.Reference == original.Records["item:auth:auth.logout"].Reference {
		t.Fatalf("unchanged item was not carried into the new context: %+v", binding)
	}
	record, err := judgments.Load(repoRoot, binding.Reference)
	if err != nil {
		t.Fatal(err)
	}
	context, err := judgments.SpecContext(repoRoot, "auth", "candidate")
	if err != nil || record.SpecContext != context {
		t.Fatalf("carried item retained the previous spec context: %s %v", record.SpecContext, err)
	}
	synthesisPromote(t, repoRoot, "auth")
	latest, accepted, err := judgments.LatestItem(repoRoot, "auth", "auth.logout", "stable")
	if err != nil || !accepted || latest != binding.Reference {
		t.Fatalf("promotion lost the carried current decision: %+v %v %v", latest, accepted, err)
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
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {"unit:auth: acceptance_item:auth.core"},
		"8": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, runID, main)
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID, "--result", "pass")

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

func TestCrossNewFindingCoexistsWithCarriedFinding(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.alpha", "auth.beta"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")
	grWriteFile(t, repoRoot, "src/beta.go", "package auth\n")

	// Baseline run: the cross synthesis creates a retained P2 finding
	// affecting alpha (non-blocking pass cache).
	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", strings.Join(verifyCrossItems, ","))
	grSubmitOK(t, repoRoot, runID, "auth.alpha", strings.Replace(grVerifyItemReport("auth.alpha", main, "src/auth.go"), "Testability / Acceptance Criteria", "acceptance_item:auth.alpha", 1))
	grSubmitOK(t, repoRoot, runID, "auth.beta", strings.Replace(grVerifyItemReport("auth.beta", main, "src/beta.go"), "Testability / Acceptance Criteria", "acceptance_item:auth.beta", 1))
	cross := "Cross-check: 4/5 PASS — combined context clarified\n\n" +
		"[P2] cross — combined claim is imprecise (actionable)\n" +
		"Finding affects: " + grRunFindingID(runID, "cross", 1) + " = auth.alpha\n\n" +
		"cross: " + main + ": Description\n" +
		"Effective status: auth.alpha = fail\nEffective status: auth.beta = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, runID, "cross", cross)
	grFinalizeOK(t, repoRoot, runID)

	baseline := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateVerify)
	if len(baseline.Findings) != 1 || baseline.Findings[0].ID != grRunFindingID(runID, "cross", 1) {
		t.Fatalf("expected the baseline finding to carry the run-scoped id, got %+v", baseline.Findings)
	}

	// Delta run: only beta's evidence went stale — alpha's finding is carried
	// into the new cross synthesis, which also creates a new finding.
	spec, err := os.ReadFile(filepath.Join(repoRoot, main))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(spec), "  - id: auth.beta\n    description: Given a caller, When the behavior runs, Then it is accepted.", "  - id: auth.beta\n    description: Given a caller, When the behavior runs, Then it is accepted. With a revision.", 1)
	if edited == string(spec) {
		t.Fatal("failed to edit the auth.beta requirement")
	}
	grWriteFile(t, repoRoot, main, edited)
	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--relationships", "cross_reference_integrity")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(coverageKeysOf(deltaRun), "review") || !grContainsString(coverageKeysOf(deltaRun), "item:auth:auth.beta") {
		t.Fatalf("expected the review and the invalidated item in coverage, got %v", coverageKeysOf(deltaRun))
	}
	// The reviewer re-runs auth.beta; alpha and its finding are carried.
	grReviewRecheck(t, repoRoot, deltaID, "auth.beta")
	deltaRun = mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(deltaRun.CarriedKeys, "auth.alpha") || !grContainsString(coverageKeysOf(deltaRun), "item:auth:auth.beta") {
		t.Fatalf("expected only auth.beta to re-execute and auth.alpha to carry: coverage %v, carried %v", coverageKeysOf(deltaRun), deltaRun.CarriedKeys)
	}
	grSubmitOK(t, repoRoot, deltaID, "auth.beta", grVerifyItemReport("auth.beta", main, "src/beta.go"))
	deltaCross := "Cross-check: 4/5 PASS — combined context clarified\n\n" +
		"[P2] cross — combined claim is imprecise (actionable)\n" +
		"Finding affects: " + grRunFindingID(deltaID, "cross", 1) + " = auth.beta\n\n" +
		"Finding disposition: " + grRunFindingID(runID, "cross", 1) + " = retained\n" +
		"cross: " + main + ": Description\n" +
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
	grSubmitOK(t, repoRoot, runID, "auth.alpha", strings.Replace(grVerifyItemReport("auth.alpha", main, "src/auth.go"), "Testability / Acceptance Criteria", "acceptance_item:auth.alpha", 1))
	grSubmitOK(t, repoRoot, runID, "auth.beta", strings.Replace(grVerifyItemReport("auth.beta", main, "src/beta.go"), "Testability / Acceptance Criteria", "acceptance_item:auth.beta", 1))
	grSubmitClarity(t, repoRoot, runID, main)
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID)

	// Only beta's evidence goes stale; alpha is carried with its snapshot.
	spec, err := os.ReadFile(filepath.Join(repoRoot, main))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(spec), "  - id: auth.beta\n    description: Given a caller, When the behavior runs, Then it is accepted.", "  - id: auth.beta\n    description: Given a caller, When the behavior runs, Then it is accepted. With a revision.", 1)
	if edited == string(spec) {
		t.Fatal("failed to edit the auth.beta requirement")
	}
	grWriteFile(t, repoRoot, main, edited)
	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(coverageKeysOf(deltaRun), "review") || !grContainsString(coverageKeysOf(deltaRun), "item:auth:auth.beta") {
		t.Fatalf("expected the review and the invalidated item in coverage, got %v", coverageKeysOf(deltaRun))
	}
	// The reviewer re-runs only auth.beta; alpha is carried with its snapshot.
	grReviewRecheck(t, repoRoot, deltaID, "auth.beta")
	deltaRun = mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(deltaRun.CarriedKeys, "auth.alpha") || !grContainsString(coverageKeysOf(deltaRun), "item:auth:auth.beta") {
		t.Fatalf("expected only auth.beta to re-execute and auth.alpha to carry: coverage %v, carried %v", coverageKeysOf(deltaRun), deltaRun.CarriedKeys)
	}
	if len(deltaRun.CarriedLenses) == 0 {
		t.Fatal("expected the plan to snapshot the carried check markers")
	}

	// Remove the baseline cache before submitting the delta reports: the
	// finalize must use the plan-time snapshot, not re-read the baseline.
	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}

	grSubmitOK(t, repoRoot, deltaID, "auth.beta", grVerifyItemReport("auth.beta", main, "src/beta.go"))
	deltaCross := "Cross-check: 5/5 PASS — combined context clarified\n\ncross: " + main + ": Description\n" +
		"Effective status: auth.alpha = pass\nEffective status: auth.beta = pass\nEffective status: cross = pass\n"
	grSubmitOK(t, repoRoot, deltaID, "cross", deltaCross)
	grFinalizeOK(t, repoRoot, deltaID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected the delta cache written from the snapshot, got:\n%s", cache)
	}
}

// TestValidateSessionDeclaresAffectsEvidence verifies that a spec-derived
// affects.files evidence file is part of the local validate sessions' read
// refs: a check may read it.
func TestValidateSessionDeclaresAffectsEvidence(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := "docs/specs/units/candidate/unit_auth.md"
	spec := "---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n" +
		"  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n    affects:\n      files:\n        - docs/notes/auth_contract.md\n"
	grWriteFile(t, repoRoot, specPath, spec)
	grWriteFile(t, repoRoot, "docs/notes/auth_contract.md", "# Auth contract\n")
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	structural := grRunSpec(t, repoRoot, runID, "structural")
	found := false
	for _, ref := range structural.ReadRefs {
		if ref == "docs/notes/auth_contract.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the affects evidence file in the structural session's read refs, got %v", structural.ReadRefs)
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
	grSubmitClarity(t, repoRoot, runID, main)
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
	grSubmitOK(t, repoRoot, runID, "dependencies", grDependenciesReport(map[string][]string{
		"7": {desc},
		"8": {desc},
	}))
	grSubmitClarity(t, repoRoot, runID, "./"+main)
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
// derivable (see framework/verification_scope.md §Coverage Model).
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
// index against multiline whitespace matching: a strictly valid session report
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

// TestGateDeltaFinalizesWithCarriedItemRegion guards the item-region pin: an
// acceptance-item record's own object is its item region, so a change to a
// code file the judgment read does not invalidate the record — the item key
// stays a review candidate and must not block a delta finalize when the
// reviewer re-runs it.
func TestGateDeltaFinalizesWithCarriedItemRegion(t *testing.T) {
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

	// Declare no implementation surface: the two evidence files enter the
	// snapshot as input-manifest evidence only, so the quality lens has no
	// coverage keys and the test isolates the alignment-lens carried evidence.
	specContent, rerr := os.ReadFile(specPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if werr := os.WriteFile(specPath, []byte(strings.ReplaceAll(string(specContent), "implementation_surface: src", "implementation_surface: <pending>")), 0644); werr != nil {
		t.Fatal(werr)
	}

	runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, "src/big.go", "src/two.go"))
	oneReport := fmt.Sprintf("- auth.one: ALIGNED — src/big.go:300\n  evidence: src/big.go:300 implements the declared behavior\n  deterministic: true\n  Part A: No concerns\n  Part B: skipped — no test files in this fixture\n\nauth.one: %s: acceptance_item:auth.one\nauth.one: src/big.go: 300-310\n", main)
	grSubmitOK(t, repoRoot, runID, "auth.one", oneReport)
	grSubmitOK(t, repoRoot, runID, "auth.two", grVerifyItemReport("auth.two", main, "src/two.go"))
	grSubmitClarity(t, repoRoot, runID, main)
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
	edited := strings.Replace(string(spec), "  - id: auth.two\n    description: Given a caller, When the behavior runs, Then it is accepted.", "  - id: auth.two\n    description: Given a caller, When the behavior runs, Then it is accepted. With a revision.", 1)
	if edited == string(spec) {
		t.Fatal("failed to edit the auth.two spec region")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--inputs-file", grInputsManifest(t, "src/big.go", "src/two.go"))
	run := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(coverageKeysOf(run), "review") || !grContainsString(coverageKeysOf(run), "item:auth:auth.two") {
		t.Fatalf("expected the review and the region-invalidated item in coverage, got %v", coverageKeysOf(run))
	}
	if grContainsString(coverageKeysOf(run), "item:auth:auth.one") {
		t.Fatalf("auth.one's own object is unchanged — it must await the review, got %v", coverageKeysOf(run))
	}
	if !grContainsString(run.CarriedKeys, "item:auth:auth.one") {
		t.Fatalf("expected auth.one among the review candidates, got %v", run.CarriedKeys)
	}
	// The reviewer re-runs both items and the architecture judgment: auth.one's
	// evidence file changed (whole-file freshness), auth.two's spec region
	// changed, and the architecture judgment covers the changed file.
	grReviewRecheck(t, repoRoot, deltaID, "auth.one", "auth.two", "architecture:auth")
	grSubmitOK(t, repoRoot, deltaID, "auth.one", oneReport)
	grSubmitOK(t, repoRoot, deltaID, "auth.two", grVerifyItemReport("auth.two", main, "src/two.go"))
	grSubmitClarity(t, repoRoot, deltaID, main)
	grAutoSubmitQuality(t, repoRoot, deltaID)
	grSubmitOK(t, repoRoot, deltaID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, deltaID)

	cacheAfter := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if got := grEntryHash(t, cacheAfter, "src/big.go"); got == baselineHash {
		t.Fatalf("expected rechecked code to receive a new hash, got %s", got)
	}
}

// TestGateDeltaNewEvidenceIsDisclosedAndNotCarried pins the new-evidence rule
// of framework/validation_cache.md §Write Rules: a physical manifest entry
// absent from the baseline cache's recorded evidence makes the plan cover the
// full verify scope — nothing is carried from that baseline — and the plan
// notice names the new paths.
func TestGateDeltaNewEvidenceIsDisclosedAndNotCarried(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")
	main := "docs/specs/units/candidate/unit_auth.md"

	validateID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, validateID, main)
	grFinalizeOK(t, repoRoot, validateID)

	verifyID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, verifyID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grAutoSubmitQuality(t, repoRoot, verifyID)
	grFinalizeOK(t, repoRoot, verifyID)

	// The delta plan discovers evidence the baseline never recorded: the plan
	// must disclose it and carry no judgment over.
	grWriteFile(t, repoRoot, "context/notes.md", "extra context\n")
	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta",
		"--inputs-file", grInputsManifest(t, "context/notes.md"))
	run := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(coverageKeysOf(run), "review") {
		t.Fatalf("expected the review session in the delta plan, got %v", coverageKeysOf(run))
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("new verify evidence must not be carried over, got %v", run.CarriedKeys)
	}
	notices := strings.Join(run.Notices, "\n")
	if !strings.Contains(notices, "new verify evidence not in the baseline: context/notes.md") {
		t.Fatalf("plan notices must name the new evidence, got:\n%s", notices)
	}
}

// TestGatePlanRejectsEscapingLogicalInput verifies that a logical input-
// manifest entry whose resolution escapes the repository root is rejected
// before run state is written (the same containment rule physical inputs obey).
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
	if _, err := grPlanRaw(repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, escape)); err == nil || !strings.Contains(err.Error(), "outside the repository root") {
		t.Fatalf("expected the escaping logical input to be rejected, got %v", err)
	}
}

// TestGateRuleValidateSessionSeverityDrivesResult pins D9 for rule validate:
// the single checks session sets each finding's severity directly (no
// severity-confirmation chain), and the derived result and counts come from
// that canonical severity.
func TestGateRuleValidateSessionSeverityDrivesResult(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var b strings.Builder
	for c := 1; c <= 6; c++ {
		if c == 6 {
			fmt.Fprintf(&b, "6. %s: FAIL — rule body contradicts the declared constraint\n", grCheckNames["6"])
			continue
		}
		fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
	}
	for c := 1; c <= 6; c++ {
		fmt.Fprintf(&b, "check-%d: %s: Constraint\n", c, rulePath)
	}
	fmt.Fprintf(&b, "[P0] %s:10 — rule body contradicts the declared constraint (needs_decision)\n", rulePath)
	fmt.Fprintf(&b, "Finding affects: %s = 6\n", grRunFindingID(runID, gaterun.SessionID(grSessionKeys(mustLoadRun(t, repoRoot, runID), "checks")), 1))
	grSubmitOK(t, repoRoot, runID, "checks", b.String())
	grFinalizeOK(t, repoRoot, runID)

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/rule/b_rule_http/validate_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "p0_count: 1") {
		t.Fatalf("expected the session-assigned P0 severity to drive the derived result, got:\n%s", cache)
	}
	judgments := grGateJudgments(t, cache)
	if !strings.Contains(judgments, `"severity":"P0"`) {
		t.Fatalf("expected the canonical session severity in the judgment baseline, got:\n%s", judgments)
	}
}

// TestGateRunRuleValidateCandidateFailWritesFailureRecord: a rule candidate
// full FAIL writes a failure record with all six check statuses and the
// repair plan derives from it (same recovery shape as unit validate).
func TestGateRunRuleValidateCandidateFailWritesFailureRecord(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := "docs/specs/rules/candidate/b_rule_http.md"
	grWriteFile(t, repoRoot, rulePath, "---\nid: b_rule_http\nscope: unit\n---\n\n# Rule\n\n## Constraint\n\nMust use TLS.\n")

	runID := grPlan(t, repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate")
	var b strings.Builder
	for c := 1; c <= 6; c++ {
		if c == 6 {
			fmt.Fprintf(&b, "6. %s: FAIL — rule body contradicts the declared constraint\n", grCheckNames["6"])
			continue
		}
		fmt.Fprintf(&b, "%d. %s: PASS — checked\n", c, grCheckNames[fmt.Sprint(c)])
	}
	for c := 1; c <= 6; c++ {
		fmt.Fprintf(&b, "check-%d: %s: Constraint\n", c, rulePath)
	}
	fmt.Fprintf(&b, "[P0] %s:5 — rule body contradicts the declared constraint (needs_decision)\n", rulePath)
	fmt.Fprintf(&b, "Finding affects: %s = 6\n", grRunFindingID(runID, gaterun.SessionID(grSessionKeys(mustLoadRun(t, repoRoot, runID), "checks")), 1))
	grSubmitOK(t, repoRoot, runID, "checks", b.String())
	out := grFinalizeOK(t, repoRoot, runID, "--result", "fail", "--p0-count", "0", "--p1-count", "1")
	if !strings.Contains(out, "Self-check: BLOCKED") {
		t.Fatalf("expected a published failure record, got:\n%s", out)
	}

	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/rule/b_rule_http/validate_result.md")
	for _, want := range []string{"result: fail", "blocking: true", "basis: full", "status: fail"} {
		if !strings.Contains(cache, want) {
			t.Fatalf("expected %q in the failure record, got:\n%s", want, cache)
		}
	}
	if _, err := grPlanRaw(repoRoot, "--gate", "validate", "--rule", "b_rule_http", "--target", "candidate", "--mode", "repair"); err != nil {
		t.Fatalf("expected the failure record to support repair planning, got %v", err)
	}
}

// TestGateRepairDegradesOnConflictingStatusMap verifies that a failure
// record recording two different statuses for the same check degrades the
// repair plan to the full coverage set instead of trusting the first occurrence
// and possibly carrying a failed judgment over.
func TestGateRepairDegradesOnConflictingStatusMap(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.one", "auth.two"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, repoRoot, "src/auth.go", "package auth\n")

	fullRun := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	grSubmitOK(t, repoRoot, fullRun, "auth.one", grVerifyMismatchReport("auth.one", main, "src/auth.go", "P0"))
	grSubmitOK(t, repoRoot, fullRun, "auth.two", grVerifyItemReport("auth.two", main, "src/auth.go"))
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun)

	cachePath := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md")
	entryIdx := strings.Index(cache, "- path: docs/specs/units/candidate/unit_auth.md")
	if entryIdx < 0 {
		t.Fatalf("cache has no main-spec entry:\n%s", cache)
	}
	checkIdx := strings.Index(cache[entryIdx:], `check: "item:auth:auth.two"`)
	if checkIdx < 0 {
		t.Fatalf("cache has no auth.two check in the main-spec entry:\n%s", cache)
	}
	statusRel := strings.Index(cache[entryIdx+checkIdx:], "status: pass")
	if statusRel < 0 {
		t.Fatalf("cache has no pass status for auth.two in the main-spec entry:\n%s", cache)
	}
	// Duplicate the check marker with a conflicting status: the status map
	// then contradicts itself and cannot say which judgments failed.
	insertAt := entryIdx + checkIdx + statusRel + len("status: pass")
	dup := "\n      - check: \"item:auth:auth.two\"\n        status: fail"
	mutated := cache[:insertAt] + dup + cache[insertAt:]
	if err := os.WriteFile(cachePath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}

	repairID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, repoRoot, repairID)
	if !grNoticeContains(run.Notices, "invalid per-check status map") || !grNoticeContains(run.Notices, "item:auth:auth.two=conflicting") {
		t.Fatalf("expected a conflicting status map to degrade the plan, got notices %v", run.Notices)
	}
	want := "item:auth:auth.one,item:auth:auth.two,code:src/auth.go,design:auth:src/auth.go,architecture:auth"
	if got := strings.Join(coverageKeysOf(run), ","); got != want {
		t.Fatalf("expected the degraded full coverage set %q, got %q", want, got)
	}
}

// TestGateRepairDegradesOnCarriedStatusInFullRecord verifies that a failure
// record with an absent basis (contract-defined as a full-run record) carrying
// a `carried` status degrades the repair plan to the full coverage set: a full
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
	grSubmitClarity(t, repoRoot, fullRun, main)
	grSubmitOK(t, repoRoot, fullRun, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, fullRun)

	// Change only auth.two's requirement so the delta run can fail while
	// retaining auth.one's unchanged code and requirement evidence.
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(spec), "  - id: auth.two\n    description: Given a caller, When the behavior runs, Then it is accepted.", "  - id: auth.two\n    description: Given a caller, When the behavior runs, Then it is accepted. With a revision.", 1)
	if edited == string(spec) {
		t.Fatal("failed to edit the auth.two spec region")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	deltaID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(coverageKeysOf(deltaRun), "review") || !grContainsString(coverageKeysOf(deltaRun), "item:auth:auth.two") {
		t.Fatalf("expected the review and the invalidated item in coverage, got %v", coverageKeysOf(deltaRun))
	}
	// The reviewer re-runs auth.two; auth.one keeps its evidence.
	grReviewRecheck(t, repoRoot, deltaID, "auth.two")
	deltaRun = mustLoadRun(t, repoRoot, deltaID)
	if !grContainsString(deltaRun.CarriedKeys, "auth.one") {
		t.Fatalf("expected auth.one to be carried over, got %v", deltaRun.CarriedKeys)
	}
	grSubmitOK(t, repoRoot, deltaID, "auth.two", grVerifyMismatchReport("auth.two", main, "src/two.go", "P0"))
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
	want := "item:auth:auth.one,item:auth:auth.two,code:src/one.go,design:auth:src/one.go,code:src/two.go,design:auth:src/two.go,architecture:auth"
	if got := strings.Join(coverageKeysOf(run), ","); got != want {
		t.Fatalf("expected the degraded full coverage set %q, got %q", want, got)
	}
}

// TestCleanDeltaKeepsCarriedJudgmentStatus pins the clean-run synthesis: a
// delta run that carries a baseline judgment and produces no finding finalizes
// without a final synthesis, and its judgment baseline must still record the
// carried key's status so a later partial delta can carry it forward.
func TestCleanDeltaKeepsCarriedJudgmentStatus(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := grWriteSpec(t, repoRoot, "auth")
	main := "docs/specs/units/candidate/unit_auth.md"

	fullRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, repoRoot, fullRun, main)
	grFinalizeOK(t, repoRoot, fullRun, "--result", "pass")

	edit, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(edit), "Prose.", "Prose. Edited.", 1)
	if edited == string(edit) {
		t.Fatal("fixture assumption broken: Description edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	deltaRun := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, repoRoot, deltaRun)
	if !grContainsString(run.CarriedKeys, "5") {
		t.Fatalf("expected check 5 as a carry candidate, got %v", run.CarriedKeys)
	}
	// The reviewer re-runs every group except acceptance.
	grReviewRecheck(t, repoRoot, deltaRun, "1", "2", "7", "10")
	// No finding is produced, so no final synthesis is required.
	grSubmitPlannedValidateSessions(t, repoRoot, deltaRun, main)
	grFinalizeOK(t, repoRoot, deltaRun, "--result", "pass")

	state := grReadJudgmentBaseline(t, repoRoot, gaterun.TargetKindUnit, "auth", gaterun.GateValidate)
	if state.LogicalStatus["5"] != "pass" {
		t.Fatalf("clean delta must record the carried key status, got %v", state.LogicalStatus)
	}
}

func grReadyVerifyCross(t *testing.T) (string, string, string) {
	t.Helper()
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpec(t, root, "auth")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	main := "docs/specs/units/candidate/unit_auth.md"
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", strings.Join(verifyCrossItems, ","))
	grSubmitOK(t, root, runID, "auth.core", grVerifyItemReport("auth.core", main, "src/auth.go"))
	grAutoSubmitQuality(t, root, runID)
	return root, runID, main
}

func grVerifyCrossItems(runID string, passed int, withFinding bool) string {
	var b strings.Builder
	for i, item := range verifyCrossItems {
		verdict := "FAIL"
		if i < passed {
			verdict = "PASS"
		}
		fmt.Fprintf(&b, "Cross item: %s = %s — checked\n", item, verdict)
		if verdict == "FAIL" && withFinding {
			fmt.Fprintf(&b, "Cross item finding: %s = %s/cross/F1\n", item, runID)
		}
	}
	return b.String()
}

func grVerifyCrossReport(runID, main string, passed int, severity, verdict string) string {
	var b strings.Builder
	b.WriteString(grVerifyCrossItems(runID, passed, severity != ""))
	fmt.Fprintf(&b, "Cross-check: %d/5 %s — synthesis complete\n", passed, verdict)
	if severity != "" {
		fmt.Fprintf(&b, "[%s] cross — combined claim needs repair (actionable)\n", severity)
		fmt.Fprintf(&b, "Finding affects: %s/cross/F1 = auth.core\n", runID)
	}
	fmt.Fprintf(&b, "cross: %s: Description\n", main)
	fmt.Fprintf(&b, "Effective status: src/auth.go = pass\nEffective status: code:src/auth.go = pass\nEffective status: architecture:auth = pass\n")
	b.WriteString("Quality conclusion: design:auth:src/auth.go = acceptable — fixture final assessment\nQuality conclusion: architecture:auth = acceptable — fixture final assessment\n")
	if severity != "" {
		fmt.Fprintf(&b, "Effective status: auth.core = fail\n")
	} else {
		b.WriteString("Effective status: auth.core = pass\n")
	}
	fmt.Fprintf(&b, "Effective status: cross = %s\n", strings.ToLower(verdict))
	return grWithRelationshipEvidence(&gaterun.Run{Relationships: verifyCrossItems}, b.String())
}

func TestVerifyCrossContractRejectsIncompleteOrContradictoryReports(t *testing.T) {
	first := "Cross item: contract_consistency = PASS — checked\n"
	cases := []struct {
		name   string
		change func(string) string
		want   string
	}{
		{"missing item", func(s string) string { return strings.Replace(s, first, "", 1) }, "missing Cross item"},
		{"duplicate item", func(s string) string { return first + s }, "duplicate Cross item"},
		{"unknown item", func(s string) string { return strings.Replace(s, "contract_consistency", "unknown_check", 1) }, "unknown Cross item"},
		{"empty reason", func(s string) string {
			return strings.Replace(s, first, "Cross item: contract_consistency = PASS — \n", 1)
		}, "malformed Cross item"},
		{"invalid verdict", func(s string) string {
			return strings.Replace(s, first, "Cross item: contract_consistency = WARNING — checked\n", 1)
		}, "malformed Cross item"},
		{"wrong passed count", func(s string) string { return strings.Replace(s, "Cross-check: 5/5", "Cross-check: 4/5", 1) }, "contradicts Cross item results"},
		{"overflowed passed count", func(s string) string {
			return strings.Replace(s, "Cross-check: 5/5", "Cross-check: 999999999999999999999999999999/5", 1)
		}, "invalid Cross-check passed count"},
		{"wrong total count", func(s string) string { return strings.Replace(s, "Cross-check: 5/5", "Cross-check: 5/3", 1) }, "contradicts Cross item results"},
		{"missing count", func(s string) string { return strings.Replace(s, "Cross-check: 5/5", "Cross-check:", 1) }, "requires exactly one"},
		{"duplicate summary", func(s string) string { return "Cross-check: 5/5 PASS — duplicate\n" + s }, "verdict lines"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, runID, main := grReadyVerifyCross(t)
			report := tc.change(grVerifyCrossReport(runID, main, 5, "", "PASS"))
			if _, err := grSubmitRaw(t, root, runID, "cross", report); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestVerifyCrossContractKeepsNonblockingFourOfFivePass(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	mission := missionJSON(t, root, runID, "cross")
	for _, item := range verifyCrossItems {
		if !strings.Contains(mission.Sessions[0].ReportContract.Template, "Cross item: "+item) {
			t.Fatalf("mission omitted %s", item)
		}
	}
	if !strings.Contains(mission.Sessions[0].ReportContract.Template, "Cross item finding: <failed_item_key> = <new_cross_finding_id>") {
		t.Fatalf("mission omitted failed-item finding links: %s", mission.Sessions[0].ReportContract.Template)
	}
	if _, err := grSubmitRaw(t, root, runID, "cross", grVerifyCrossReport(runID, main, 4, "P2", "PASS")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "p2_count: 1") {
		t.Fatalf("nonblocking cross result was lost:\n%s", cache)
	}
}

func TestVerifyCrossPassRejectsBlockingNewFinding(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	if _, err := grSubmitRaw(t, root, runID, "cross", grVerifyCrossReport(runID, main, 4, "P1", "PASS")); err == nil || !strings.Contains(err.Error(), "Cross-check PASS contradicts") {
		t.Fatalf("expected blocking cross finding to contradict PASS, got %v", err)
	}
}

func TestVerifyCrossItemFindingLinks(t *testing.T) {
	cases := []struct {
		name   string
		change func(string, string) string
		want   string
	}{
		{"missing link", func(report, runID string) string {
			return strings.Replace(report, "Cross item finding: cross_reference_integrity = "+runID+"/cross/F1\n", "", 1)
		}, "has no Cross item finding"},
		{"duplicate link", func(report, runID string) string {
			link := "Cross item finding: cross_reference_integrity = " + runID + "/cross/F1\n"
			return strings.Replace(report, link, link+link, 1)
		}, "duplicate Cross item finding"},
		{"unknown item", func(report, runID string) string {
			return strings.Replace(report, "Cross item finding: cross_reference_integrity", "Cross item finding: unknown_check", 1)
		}, "unknown Cross item finding key"},
		{"unknown finding", func(report, runID string) string {
			return strings.Replace(report, runID+"/cross/F1", runID+"/cross/F9", 1)
		}, "not a finding created"},
		{"passing item link", func(report, runID string) string {
			return "Cross item finding: contract_consistency = " + runID + "/cross/F1\n" + report
		}, "must not declare a Cross item finding"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, runID, main := grReadyVerifyCross(t)
			report := tc.change(grVerifyCrossReport(runID, main, 4, "P2", "PASS"), runID)
			if _, err := grSubmitRaw(t, root, runID, "cross", report); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestVerifyCrossOneFindingSupportsTwoFailedItems(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	report := grVerifyCrossReport(runID, main, 3, "P3", "PASS")
	if _, err := grSubmitRaw(t, root, runID, "cross", report); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: pass") || !strings.Contains(cache, "p3_count: 1") || !strings.Contains(cache, "Cross item finding: error_code_conflict = "+runID+"/cross/F1") || !strings.Contains(cache, "Cross item finding: cross_reference_integrity = "+runID+"/cross/F1") {
		t.Fatalf("shared nonblocking cross finding was lost:\n%s", cache)
	}
}

func TestVerifyCrossBlockingFindingFailsFinalGate(t *testing.T) {
	root, runID, main := grReadyVerifyCross(t)
	if _, err := grSubmitRaw(t, root, runID, "cross", grVerifyCrossReport(runID, main, 4, "P1", "FAIL")); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "blocking: true") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("blocking cross finding did not fail the gate:\n%s", cache)
	}
}

func TestCrossItemFindingLinkMustBeRetained(t *testing.T) {
	id := "run/cross/F1"
	parsed := &parsedReport{Findings: []gaterun.Finding{{ID: id, Severity: "P1", AffectedKeys: []string{gaterun.RelationshipKey("contract_consistency")}}}, CrossItemFindings: map[string]string{"contract_consistency": id}}
	if err := validateCrossItemFindingLinks(gaterun.GateVerify, parsed, nil); err == nil || !strings.Contains(err.Error(), "not retained") {
		t.Fatalf("non-retained cross finding was accepted: %v", err)
	}
	parsed.Findings[0].Severity = "P2"
	if err := validateCrossItemFindingLinks(gaterun.GateValidate, parsed, parsed.Findings); err == nil || !strings.Contains(err.Error(), "must be P0 or P1") {
		t.Fatalf("validate accepted a P2 cross finding: %v", err)
	}
}

func TestValidateCrossFailedItemHasBlockingFinding(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	main := grWriteSpec(t, root, "auth")
	runID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--relationships", strings.Join(validateCrossItems, ","))
	for _, session := range []struct {
		id     string
		checks []string
	}{
		{"structural", []string{"1", "3", "6"}},
		{"design", []string{"2", "4"}},
		{"acceptance", []string{"5"}},
		{"dependencies", []string{"7", "8", "9"}},
	} {
		scopes := make(map[string][]string)
		for _, check := range session.checks {
			scopes[check] = []string{main + ": Description"}
		}
		grSubmitOK(t, root, runID, session.id, grValidateReport(session.checks, scopes))
	}
	grSubmitClarity(t, root, runID, main)
	id := runID + "/cross/F1"
	report := "Cross item: design_constraints = FAIL — combined design violates a constraint\n" +
		"Cross item finding: design_constraints = " + id + "\n" +
		"Cross item: coverage_scope = PASS — checked\n" +
		"Cross item: cross_unit_cohesion = PASS — checked\n" +
		"Cross-check: 2/3 FAIL — blocking cross finding\n" +
		"[P1] cross — combined design violates a constraint\n" +
		"Finding affects: " + id + " = 2\n" +
		"cross: " + main + ": Description\n"
	for _, check := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"} {
		status := "pass"
		if check == "2" {
			status = "fail"
		}
		report += "Effective status: " + check + " = " + status + "\n"
	}
	report += "Effective status: cross = fail\n"
	if _, err := grSubmitRaw(t, root, runID, "cross", grWithRelationshipEvidence(mustLoadRun(t, root, runID), report)); err != nil {
		t.Fatal(err)
	}
	out := grFinalizeOK(t, root, runID)
	if !strings.Contains(out, "Self-check: BLOCKED") {
		t.Fatalf("blocking validate cross finding did not fail the gate: %s", out)
	}
	cache, err := os.ReadFile(filepath.Join(root, "docs/specs/meta/validation/unit/auth/validate_result.md"))
	if err != nil {
		t.Fatalf("failed candidate validate must write a failure record: %v", err)
	}
	if !strings.Contains(string(cache), "result: fail") || !strings.Contains(string(cache), "blocking: true") {
		t.Fatalf("expected a blocking failure record, got:\n%s", cache)
	}
}

func TestValidateCrossContractUsesThreeFixedItems(t *testing.T) {
	run := &gaterun.Run{Gate: gaterun.GateValidate, Relationships: validateCrossItems}
	session := &gaterun.SessionSpec{Kind: gaterun.SessionKindCross, CheckKeys: []string{gaterun.CrossKey}}
	contract := reportContractFor(run, session)
	for _, item := range validateCrossItems {
		if !strings.Contains(contract.Template, "Cross item: "+item) {
			t.Fatalf("validate template omitted %s", item)
		}
	}
	if !strings.Contains(contract.Template, "Cross-check: <passed>/3") {
		t.Fatalf("validate summary has wrong denominator: %s", contract.Template)
	}
	var report strings.Builder
	for _, item := range validateCrossItems {
		fmt.Fprintf(&report, "Cross item: %s = PASS — checked\n", item)
	}
	report.WriteString("Cross-check: 3/3 PASS — consistent\n")
	if err := validateCrossItemBody(&gaterun.Run{Gate: gaterun.GateValidate, Relationships: validateCrossItems}, report.String(), &parsedReport{}); err != nil {
		t.Fatalf("complete validate cross report rejected: %v", err)
	}
	missing := strings.Replace(report.String(), "Cross item: design_constraints = PASS — checked\n", "", 1)
	if err := validateCrossItemBody(&gaterun.Run{Gate: gaterun.GateValidate, Relationships: validateCrossItems}, missing, &parsedReport{}); err == nil || !strings.Contains(err.Error(), "missing Cross item") {
		t.Fatalf("incomplete validate cross report accepted: %v", err)
	}
}

func grLoadSessionState(root string, run *gaterun.Run, id string) (*gaterun.SessionState, error) {
	return gaterun.LoadSessionState(root, run, gaterun.SessionID(grSessionKeys(run, id)))
}
func grBuildSessionSpec(root string, run *gaterun.Run, keys []string) (*gaterun.SessionSpec, error) {
	mapped := make([]string, len(keys))
	for i, key := range keys {
		mapped[i] = grSessionKeys(run, key)[0]
	}
	return gaterun.BuildSessionSpec(root, run, mapped)
}

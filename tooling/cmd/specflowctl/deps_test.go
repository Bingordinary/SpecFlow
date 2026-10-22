package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUnitSpecRefs(t *testing.T, repoRoot, layer, name, ruleRefs string) {
	t.Helper()
	if ruleRefs == "" {
		ruleRefs = "none"
	}
	content := "---\nid: " + name + "\nrule_refs: " + ruleRefs + "\n---\n\n# " + name + "\n"
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_"+name+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDepsUsageWithoutFlags(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none")
	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"DEPENDENCY GRAPH", "Rule refs", "CYCLES (PUBLICATION)", "Promotion order"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestDepsReportsCycleAndOrder(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseDepsUnit(t, repoRoot, "candidate", "auth", "auth.api", "payment.api", "none")
	writePromiseDepsUnit(t, repoRoot, "candidate", "payment", "payment.api", "auth.api", "none")
	writePromiseDepsUnit(t, repoRoot, "candidate", "session", "session.api", "auth.api", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "auth -> payment") {
		t.Fatalf("expected cycle listing, got:\n%s", output)
	}
	if !strings.Contains(output, "CYCLES (PUBLICATION) (1)") {
		t.Fatalf("expected one publication cycle, got:\n%s", output)
	}
	// session depends on the cycle member auth, so nothing can be ordered.
	if !strings.Contains(output, "Promotion order (0 of 3") {
		t.Fatalf("expected empty promotion order, got:\n%s", output)
	}
	if !strings.Contains(output, "unpublished dependency cycle") {
		t.Fatalf("expected the publication-cycle note in promotion order, got:\n%s", output)
	}
}

func TestDepsSingleUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseDepsUnit(t, repoRoot, "candidate", "auth", "auth.api", "payment.api", "[b_rule_1]")
	writePromiseDepsUnit(t, repoRoot, "candidate", "payment", "payment.api", "auth.api", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--unit", "auth", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --unit failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"DEPENDENCIES — auth", "payment", "b_rule_1", "Referenced by (promise depends_on):", "ON A CYCLE"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestDepsPublishedProviderDoesNotBlockOrder(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseDepsUnit(t, repoRoot, "stable", "provider", "provider.api", "", "none")
	writePromiseDepsUnit(t, repoRoot, "candidate", "consumer", "consumer.view", "provider.api", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--unit", "consumer", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --unit failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "Publication blockers: none") {
		t.Fatalf("a provider already published at the same contract must not block, got:\n%s", output)
	}
	if !strings.Contains(output, "Publication cycle: none") {
		t.Fatalf("expected no publication cycle, got:\n%s", output)
	}
}

func TestDepsSingleRule(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "[b_rule_1]")
	writeUnitSpecRefs(t, repoRoot, "candidate", "payment", "[b_rule_1]")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--rule", "b_rule_1", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --rule failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"DEPENDENCIES — b_rule_1 (rule)", "auth", "payment"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

// writeRuleFile writes a rule file at the given layer so rule-file existence
// checks resolve. Content is minimal — deps only checks that the file exists.
func writeRuleFile(t *testing.T, repoRoot, layer, id string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/rules", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nrule_id: " + id + "\nrule_scope: global\n---\n"
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDepsGlobalRule(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none")
	writeUnitSpecRefs(t, repoRoot, "candidate", "payment", "none")
	writeRuleFile(t, repoRoot, "candidate", "g_rule_baseline")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--rule", "g_rule_baseline", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --rule (global) failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	// A global rule is not repeated in rule_refs but applies to every
	// current-layer unit by default — all units are reported.
	for _, want := range []string{"DEPENDENCIES — g_rule_baseline (rule)", "auth", "payment"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestDepsGlobalRuleFileMissing(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none")

	var stdout, stderr bytes.Buffer
	err := runDeps([]string{"--rule", "g_rule_ghost", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for global rule without a rule file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

func TestDepsUnitAndRuleMutuallyExclusive(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	var stdout, stderr bytes.Buffer
	err := runDeps([]string{"--unit", "auth", "--rule", "b_rule_1", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for --unit and --rule together")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion message, got: %v", err)
	}
}

func TestDepsUnitNotFound(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none")

	var stdout, stderr bytes.Buffer
	err := runDeps([]string{"--unit", "ghost", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown unit")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got: %v", err)
	}
}

func TestDepsScopeCandidate(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseDepsUnit(t, repoRoot, "candidate", "auth", "auth.api", "payment.api", "none")
	writePromiseDepsUnit(t, repoRoot, "stable", "payment", "payment.api", "", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--scope", "candidate", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --scope candidate failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "scope: candidate, 1 unit(s)") {
		t.Fatalf("expected candidate scope with 1 unit, got:\n%s", output)
	}
}

func TestDepsReportsPromiseEdges(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseStateUnit(t, repoRoot, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, repoRoot, "web", "web.session_api", "auth.login_api", "session")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "Promise edges") || !strings.Contains(output, "web.session_api") || !strings.Contains(output, "auth.login_api") {
		t.Fatalf("expected promise edge in output:\n%s", output)
	}
}

func writePromiseDepsUnit(t *testing.T, repoRoot, layer, unit, promiseID, dependsOn, ruleRefs string) {
	t.Helper()
	depends := ""
	if dependsOn != "" {
		depends = "    depends_on: [" + dependsOn + "]\n"
	}
	content := "---\nid: " + unit + "\nrule_refs: " + ruleRefs + "\n---\n\n" +
		"## Design Narrative\n\nNarrative.\n\n" +
		"## Promises\n\npromises:\n  - id: " + promiseID + "\n    summary: Contract.\n    contract: {invariants: [contract]}\n" + depends + "\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    promise: " + promiseID + "\n    description: Given X, When Y, Then Z.\n    verification_type: inspectable\n    verification_surface: api\n    implementation_surface: src/" + unit + ".go\n    verification_method: inspect.\n    pass_condition: Passes.\n    runnable: yes\n"
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unit+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDepsPromiseEdgesOrderCyclesAndScope(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseDepsUnit(t, repoRoot, "candidate", "aaa", "aaa.view", "zzz.model", "none")
	writePromiseDepsUnit(t, repoRoot, "candidate", "zzz", "zzz.model", "", "none")
	writePromiseDepsUnit(t, repoRoot, "stable", "stable_consumer", "stable_consumer.view", "stable_provider.model", "none")
	writePromiseDepsUnit(t, repoRoot, "stable", "stable_provider", "stable_provider.model", "", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	orderStart := strings.Index(output, "Promotion order")
	if orderStart < 0 {
		t.Fatalf("missing promotion order:\n%s", output)
	}
	order := output[orderStart:]
	if strings.Index(order, "zzz") > strings.Index(order, "aaa") {
		t.Fatalf("promise provider must be ordered before its consumer:\n%s", output)
	}

	// Mutating the candidate edge into a cycle must be reported by the same
	// cycle section that owns dependency cycles.
	writePromiseDepsUnit(t, repoRoot, "candidate", "zzz", "zzz.model", "aaa.view", "none")
	stdout.Reset()
	stderr.Reset()
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "CYCLES") {
		t.Fatalf("promise cycle must be reported in CYCLES:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runDeps([]string{"--scope", "stable", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --scope stable failed: %v\nstderr=%s", err, stderr.String())
	}
	stableOutput := stdout.String()
	if !strings.Contains(stableOutput, "stable_consumer.view") || strings.Contains(stableOutput, "aaa.view") {
		t.Fatalf("promise edge report must honor --scope stable:\n%s", stableOutput)
	}
	_ = stderr
}

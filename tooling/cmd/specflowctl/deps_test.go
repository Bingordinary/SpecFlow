package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
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
	for _, want := range []string{"DEPENDENCY GRAPH", "Rule refs", "UNIT DEPENDENCY CYCLES", "CYCLES (PUBLICATION)", "Dependency order"} {
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
	if !strings.Contains(output, "UNIT DEPENDENCY CYCLES (INVALID) (1)") {
		t.Fatalf("expected one unit dependency cycle, got:\n%s", output)
	}
	if !strings.Contains(output, "auth, payment") {
		t.Fatalf("expected cycle member listing, got:\n%s", output)
	}
	if !strings.Contains(output, "CYCLES (PUBLICATION) (1)") {
		t.Fatalf("expected one publication cycle, got:\n%s", output)
	}
	// auth and payment are mutually dependent, so they form one invalid group;
	// session follows them. No unit is dropped from the order.
	if !strings.Contains(output, "Dependency order (structural, dependencies first; 3 unit(s), 2 group(s), 3 candidate unit(s))") {
		t.Fatalf("expected a complete grouped dependency order, got:\n%s", output)
	}
	if !strings.Contains(output, "1. auth + payment  (unit dependency cycle — invalid; see above)") {
		t.Fatalf("expected the mutually dependent units as one invalid group, got:\n%s", output)
	}
	if !strings.Contains(output, "2. session") {
		t.Fatalf("expected session after the invalid group, got:\n%s", output)
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
	for _, want := range []string{"DEPENDENCIES — auth", "payment", "b_rule_1", "Referenced by (promise depends_on):", "ON A CYCLE", "Unit dependency cycle (INVALID — validate fails these units):", "auth, payment", "publishing a provider does not clear the unit cycle"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"validate may still pass", "Publish a provider first"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("publication cycle must not report %q:\n%s", unwanted, output)
		}
	}
}

func TestDepsPublishedProviderStillConstrainsStructuralOrder(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseDepsUnit(t, repoRoot, "stable", "provider", "provider.api", "", "none")
	writePromiseDepsUnit(t, repoRoot, "candidate", "consumer", "consumer.view", "provider.api", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if strings.Contains(output, "provider.api  [wait]") {
		t.Fatalf("a provider published at the same contract must not be marked [wait], got:\n%s", output)
	}
	// Publication state is an annotation: the published provider still comes
	// first in the structural dependency order.
	for _, want := range []string{
		"Dependency order (structural, dependencies first; 2 unit(s), 2 group(s), 1 candidate unit(s))",
		"1. provider (stable)",
		"2. consumer",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in the dependency order:\n%s", want, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if err := runDeps([]string{"--unit", "consumer", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --unit failed: %v\nstderr=%s", err, stderr.String())
	}
	unitOutput := stdout.String()
	if !strings.Contains(unitOutput, "Publication blockers: none") {
		t.Fatalf("a provider already published at the same contract must not block, got:\n%s", unitOutput)
	}
	if !strings.Contains(unitOutput, "Publication cycle: none") {
		t.Fatalf("expected no publication cycle, got:\n%s", unitOutput)
	}
	if !strings.Contains(unitOutput, "Unit dependency cycle: none") {
		t.Fatalf("expected no unit dependency cycle, got:\n%s", unitOutput)
	}
}

func TestDepsUnitCycleIsReportedAsViolation(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	// a and b are mutually dependent through different promises (a.p1 ->
	// b.q1 while b.q2 -> a.p2), so the promise graph stays acyclic while the
	// unit aggregation has a cycle: a violation. No promise of a unit reaches
	// another promise of the same unit. a's contracts are unchanged from
	// stable, so b's dependency on a.p2 is already satisfied; b.q1's contract
	// changed, so a's dependency on b.q1 is the only promotion blocker.
	writeTwoPromiseLayerUnit(t, repoRoot, "stable", "a", "a.p1", "b.q1", "c1", "a.p2", "", "c2")
	writeTwoPromiseLayerUnit(t, repoRoot, "candidate", "a", "a.p1", "b.q1", "c1", "a.p2", "", "c2")
	writeTwoPromiseLayerUnit(t, repoRoot, "stable", "b", "b.q1", "", "c3", "b.q2", "a.p2", "c4")
	writeTwoPromiseLayerUnit(t, repoRoot, "candidate", "b", "b.q1", "", "c3-changed", "b.q2", "a.p2", "c4")

	// The promise graph itself stays acyclic; the unit aggregation is what
	// fails, and the mechanical check must report it.
	res := specvalidation.CheckPromiseGraphScope(repoRoot, specvalidation.PromiseScopeAll, "a")
	if res.Status != specvalidation.Fail || !strings.Contains(res.Details, "unit dependency cycle") {
		t.Fatalf("fixture must fail on the unit dependency cycle, got %v: %s", res.Status, res.Details)
	}
	if strings.Contains(res.Details, "promise dependency cycle") {
		t.Fatalf("fixture must not contain a promise cycle, got %s", res.Details)
	}

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"-> b.q1  [wait]",
		"-> a.p2\n",
		"UNIT DEPENDENCY CYCLES (INVALID) (1)",
		"a, b",
		"CYCLES (PUBLICATION): none",
		"Dependency order (structural, dependencies first; 2 unit(s), 1 group(s), 2 candidate unit(s))",
		"1. a + b  (unit dependency cycle — invalid; see above)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if err := runDeps([]string{"--unit", "a", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --unit failed: %v\nstderr=%s", err, stderr.String())
	}
	unitOutput := stdout.String()
	for _, want := range []string{
		"Publication blockers (provider contract not published stable):",
		"  - b",
		"Unit dependency cycle (INVALID — validate fails these units):",
		"  - a, b",
		"Publication cycle: none",
	} {
		if !strings.Contains(unitOutput, want) {
			t.Fatalf("expected %q in unit output:\n%s", want, unitOutput)
		}
	}
}

func TestDepsCycleMemberSetIncludesCrossEdgeClosure(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	// a -> b -> c -> a is one unit cycle; b -> d -> c closes a second cycle
	// through a cross edge. The member set is all four units. There is no
	// stable layer, so every edge is also an unpublished publication wait and
	// the publication deadlock covers the same four units.
	writeTwoPromiseLayerUnit(t, repoRoot, "candidate", "a", "a.p1", "b.p1", "c1", "a.p2", "", "c2")
	writeTwoPromiseLayerUnit(t, repoRoot, "candidate", "b", "b.p1", "c.p1", "c1", "b.p2", "d.p1", "c2")
	writeTwoPromiseLayerUnit(t, repoRoot, "candidate", "c", "c.p1", "a.p2", "c1", "c.p2", "", "c2")
	writeTwoPromiseLayerUnit(t, repoRoot, "candidate", "d", "d.p1", "c.p2", "c1", "d.p2", "", "c2")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"UNIT DEPENDENCY CYCLES (INVALID) (1)",
		"a, b, c, d",
		"CYCLES (PUBLICATION) (1)",
		"1. a + b + c + d  (unit dependency cycle — invalid; see above)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if err := runDeps([]string{"--unit", "d", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --unit failed: %v\nstderr=%s", err, stderr.String())
	}
	unitOutput := stdout.String()
	for _, want := range []string{
		"Publication blockers (provider contract not published stable):",
		"  - c",
		"Unit dependency cycle (INVALID — validate fails these units):",
		"  - a, b, c, d",
		"Publication cycle: ON A CYCLE",
		"publishing a provider does not clear the unit cycle",
	} {
		if !strings.Contains(unitOutput, want) {
			t.Fatalf("expected %q in unit output:\n%s", want, unitOutput)
		}
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

// writeTwoPromiseLayerUnit writes a unit with two promises, each with an
// optional dependency on another unit's promise. It lets a test build a
// unit-level cycle through different promises while the promise graph stays
// acyclic.
func writeTwoPromiseLayerUnit(t *testing.T, repoRoot, layer, unit, firstID, firstDep, firstContract, secondID, secondDep, secondContract string) {
	t.Helper()
	firstDepends := ""
	if firstDep != "" {
		firstDepends = "    depends_on: [" + firstDep + "]\n"
	}
	secondDepends := ""
	if secondDep != "" {
		secondDepends = "    depends_on: [" + secondDep + "]\n"
	}
	content := "---\nid: " + unit + "\nrule_refs: none\n---\n\n# " + unit + "\n\n" +
		"## Design Narrative\n\nNarrative.\n\n" +
		"## Promises\n\npromises:\n" +
		"  - id: " + firstID + "\n    summary: Contract.\n    contract: {invariants: [" + firstContract + "]}\n" + firstDepends +
		"  - id: " + secondID + "\n    summary: Contract.\n    contract: {invariants: [" + secondContract + "]}\n" + secondDepends + "\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n" +
		"  - id: " + unit + ".core\n    promise: " + firstID + "\n    description: Given X, When Y, Then Z.\n    verification_type: inspectable\n    verification_surface: api\n    implementation_surface: src/" + unit + ".go\n    verification_method: inspect.\n    pass_condition: Passes.\n    runnable: yes\n" +
		"  - id: " + unit + ".model\n    promise: " + secondID + "\n    description: Given X, When Y, Then Z.\n    verification_type: inspectable\n    verification_surface: api\n    implementation_surface: src/" + unit + ".go\n    verification_method: inspect.\n    pass_condition: Passes.\n    runnable: yes\n"
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unit+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDepsStableScopeOrdersByDependency(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writePromiseDepsUnit(t, repoRoot, "stable", "aaa", "aaa.view", "zzz.model", "none")
	writePromiseDepsUnit(t, repoRoot, "stable", "zzz", "zzz.model", "", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--scope", "stable", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --scope stable failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	// In the stable scope every edge is already published relative to the
	// stable layer; ordering must still follow the dependency, not the
	// alphabet.
	for _, want := range []string{
		"Dependency order (structural, dependencies first; 2 unit(s), 2 group(s), 0 candidate unit(s))",
		"1. zzz (stable)",
		"2. aaa (stable)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in stable-scope output:\n%s", want, output)
		}
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
	orderStart := strings.Index(output, "Dependency order")
	if orderStart < 0 {
		t.Fatalf("missing dependency order:\n%s", output)
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

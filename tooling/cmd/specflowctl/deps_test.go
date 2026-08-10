package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUnitSpecRefs(t *testing.T, repoRoot, layer, name, unitRefs, ruleRefs string) {
	t.Helper()
	if unitRefs == "" {
		unitRefs = "none"
	}
	if ruleRefs == "" {
		ruleRefs = "none"
	}
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: " + unitRefs + "\nrule_refs: " + ruleRefs + "\n---\n\n# " + name + "\n"
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
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none", "none")
	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"DEPENDENCY GRAPH", "Edges", "Cycles", "Promotion order"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestDepsReportsCycleAndOrder(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "[payment]", "none")
	writeUnitSpecRefs(t, repoRoot, "candidate", "payment", "[auth]", "none")
	writeUnitSpecRefs(t, repoRoot, "candidate", "session", "[auth]", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "auth -> payment") {
		t.Fatalf("expected cycle listing, got:\n%s", output)
	}
	if !strings.Contains(output, "CYCLES (1)") {
		t.Fatalf("expected one cycle, got:\n%s", output)
	}
	// session depends on the cycle member auth, so nothing can be ordered.
	if !strings.Contains(output, "Promotion order (0 of 3") {
		t.Fatalf("expected empty promotion order, got:\n%s", output)
	}
	if !strings.Contains(output, "units without a promotion order are blocked by a cycle") {
		t.Fatalf("expected cycle note in promotion order, got:\n%s", output)
	}
}

func TestDepsSingleUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "[payment]", "[b_rule_1]")
	writeUnitSpecRefs(t, repoRoot, "candidate", "payment", "[auth]", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--unit", "auth", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --unit failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"DEPENDENCIES — auth", "payment", "b_rule_1", "Referenced by:", "ON A CYCLE"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output:\n%s", want, output)
		}
	}
}

func TestDepsSingleRule(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none", "[b_rule_1]")
	writeUnitSpecRefs(t, repoRoot, "candidate", "payment", "none", "[b_rule_1]")

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
	content := "---\nrule_id: " + id + "\nrule_scope: global\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDepsGlobalRule(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none", "none")
	writeUnitSpecRefs(t, repoRoot, "candidate", "payment", "none", "none")
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
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none", "none")

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
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "none", "none")

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
	writeUnitSpecRefs(t, repoRoot, "candidate", "auth", "[payment]", "none")
	writeUnitSpecRefs(t, repoRoot, "stable", "payment", "none", "none")

	var stdout, stderr bytes.Buffer
	if err := runDeps([]string{"--scope", "candidate", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("deps --scope candidate failed: %v\nstderr=%s", err, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "scope: candidate, 1 unit(s)") {
		t.Fatalf("expected candidate scope with 1 unit, got:\n%s", output)
	}
}

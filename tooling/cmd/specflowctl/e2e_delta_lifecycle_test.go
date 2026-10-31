package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func e2eRuleContent(constraint string) string {
	return "---\nid: b_rule_auth\nscope: unit\n---\n\n# b_rule_auth\n\n## Constraint\n\n" + constraint + "\n"
}

// TestE2EMechanicalDeltaLifecycle walks the validate recovery path with the
// real command entry points: a full run, an external-only change that re-runs
// the external group, and an own-content change that covers the full scope.
func TestE2EMechanicalDeltaLifecycle(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	rulePath := "docs/specs/rules/candidate/b_rule_auth.md"
	grWriteFile(t, root, rulePath, e2eRuleContent("Must use TLS."))
	grWriteSpecItems(t, root, "auth", "none", "b_rule_auth", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"

	fullID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, root, fullID, main)
	grFinalizeOK(t, root, fullID)

	if out, err := freshRun(t, root, "--unit", "auth"); err != nil || !strings.Contains(out, "validate  FRESH") {
		t.Fatalf("fresh after the full run: %v\n%s", err, out)
	}

	// External-only change: the bound rule moves, the unit's own spec does not.
	grWriteFile(t, root, rulePath, e2eRuleContent("Must use mutual TLS."))
	deltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	deltaRun := mustLoadRun(t, root, deltaID)
	if deltaRun.Mode != gaterun.ModeDelta {
		t.Fatalf("an external-only change must stay a delta run, got %q", deltaRun.Mode)
	}
	if got := strings.Join(coverageKeysOf(deltaRun), ","); got != "external" {
		t.Fatalf("external-only coverage = %s, want external", got)
	}
	if len(deltaRun.CarriedKeys) == 0 || !grContainsString(deltaRun.CarriedKeys, "5") {
		t.Fatalf("the unaffected checks must carry, got %v", deltaRun.CarriedKeys)
	}
	grSubmitOK(t, root, deltaID, "external", grDependenciesReport(map[string][]string{
		"6": {main + ": Acceptance Items"},
		"7": {main + ": Description"},
		"8": {main + ": Description"},
	}))
	grFinalizeOK(t, root, deltaID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "basis: delta") {
		t.Fatalf("expected a delta-basis cache, got:\n%s", cache)
	}
	if out, err := freshRun(t, root, "--unit", "auth"); err != nil || !strings.Contains(out, "validate  FRESH") {
		t.Fatalf("fresh after the external delta: %v\n%s", err, out)
	}

	// Own-content change: the spec moves, so the tool covers the full scope
	// and records the run as full.
	data, err := os.ReadFile(filepath.Join(root, main))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, main, strings.Replace(string(data), "Prose.", "Prose. Edited.", 1))
	fullDeltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	fullDelta := mustLoadRun(t, root, fullDeltaID)
	if fullDelta.Mode != gaterun.ModeFull {
		t.Fatalf("an own-content change must cover the full scope, got mode %q", fullDelta.Mode)
	}
	if got := strings.Join(coverageKeysOf(fullDelta), ","); got != "document,design,acceptance,external" {
		t.Fatalf("own-content coverage = %s, want the four groups", got)
	}
	if len(fullDelta.CarriedKeys) != 0 {
		t.Fatalf("a full-scope plan carries nothing, got %v", fullDelta.CarriedKeys)
	}
	grSubmitValidateSessions(t, root, fullDeltaID, main)
	grFinalizeOK(t, root, fullDeltaID)
	cache = grReadCache(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if !strings.Contains(cache, "basis: full") {
		t.Fatalf("expected a full-basis cache, got:\n%s", cache)
	}
}

// TestE2EDeletedOwnAppendixForcesFullScope pins the own-content boundary:
// deleting an appendix removes it from run.OwnSpecFiles, but its baseline
// entry still belongs to the unit's own spec union. The delta plan must cover
// the full scope instead of classifying the removal as an external ref.
func TestE2EDeletedOwnAppendixForcesFullScope(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	grWriteFile(t, root, appendix, "---\nunit: auth\n---\n\n# Protocol\n\nExtra protocol.\n")

	fullID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, root, fullID, main, appendix+": all")
	grFinalizeOK(t, root, fullID)

	if err := os.Remove(filepath.Join(root, appendix)); err != nil {
		t.Fatal(err)
	}

	deltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, root, deltaID)
	if run.Mode != gaterun.ModeFull {
		t.Fatalf("a deleted own appendix must force the full scope, got mode %q", run.Mode)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "document,design,acceptance,external" {
		t.Fatalf("deleted-appendix coverage = %s, want the four groups", got)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("a full-scope plan carries nothing, got %v", run.CarriedKeys)
	}
}

// TestE2EValidateNewDeclaredInputForcesFullScope pins the own-content boundary:
// a delta plan that declares an extra input the baseline never recorded must
// cover the full scope, because every validate group reads the declared input
// surface.
func TestE2EValidateNewDeclaredInputForcesFullScope(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteFile(t, root, "helper1.go", "package auth\n\nfunc helper1() {}\n")
	grWriteFile(t, root, "helper2.go", "package auth\n\nfunc helper2() {}\n")
	manifest1 := grInputsManifest(t, "helper1.go")

	fullID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--inputs-file", manifest1)
	grSubmitValidateSessions(t, root, fullID, main)
	grFinalizeOK(t, root, fullID)

	// Only the declared input surface grows. The plan must not stop as fresh,
	// and it must not carry the four groups over an input they never read.
	manifest2 := grInputsManifest(t, "helper1.go", "helper2.go")
	deltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--inputs-file", manifest2)
	run := mustLoadRun(t, root, deltaID)
	if run.Mode != gaterun.ModeFull {
		t.Fatalf("a newly declared extra input must cover the full scope, got mode %q", run.Mode)
	}
	if got := strings.Join(coverageKeysOf(run), ","); got != "document,design,acceptance,external" {
		t.Fatalf("new-input coverage = %s, want the four groups", got)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("a full-scope plan carries nothing, got %v", run.CarriedKeys)
	}
}

// TestE2EValidateMissingMainSpecEntryForcesFullScope pins the other half of the
// same rule: a baseline that never recorded the unit's own main spec cannot
// prove the own union unchanged. The plan must cover the full scope instead of
// reporting the cache fresh.
func TestE2EValidateMissingMainSpecEntryForcesFullScope(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecItems(t, root, "auth", "none", "none", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"

	fullID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, root, fullID, main)
	grFinalizeOK(t, root, fullID)

	dropCacheFileEntry(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md", main)

	data, err := os.ReadFile(filepath.Join(root, main))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, main, strings.Replace(string(data), "Prose.", "Prose. Edited.", 1))

	deltaID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, root, deltaID)
	if run.Mode != gaterun.ModeFull {
		t.Fatalf("a baseline missing its own main spec must cover the full scope, got mode %q", run.Mode)
	}
	if len(run.CarriedKeys) != 0 {
		t.Fatalf("a full-scope plan carries nothing, got %v", run.CarriedKeys)
	}
}

// dropCacheFileEntry removes one file entry block from a cache's files list.
func dropCacheFileEntry(t *testing.T, repoRoot, rel, path string) {
	t.Helper()
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "- path: "+path {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("cache entry %s not found in %s", path, rel)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "  - path:") || lines[i] == "---" {
			end = i
			break
		}
	}
	edited := strings.Join(append(append([]string{}, lines[:start]...), lines[end:]...), "\n")
	if err := os.WriteFile(abs, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestE2EValidateRepairAndRuleDeltaRejected pins the command surface: validate
// has no repair mode, and a rule target has no partial run.
func TestE2EValidateRepairAndRuleDeltaRejected(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteFile(t, root, "docs/specs/rules/candidate/b_rule_auth.md", e2eRuleContent("Must use TLS."))
	grWriteSpecItems(t, root, "auth", "none", "b_rule_auth", []string{"auth.core"})
	main := "docs/specs/units/candidate/unit_auth.md"

	fullID := grPlan(t, root, "--gate", "validate", "--unit", "auth", "--target", "candidate")
	grSubmitValidateSessions(t, root, fullID, main)
	grFinalizeOK(t, root, fullID)

	if _, err := grPlanRaw(root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair"); err == nil || !strings.Contains(err.Error(), "no repair mode") {
		t.Fatalf("expected validate repair to be rejected, got %v", err)
	}
	if _, err := grPlanRaw(root, "--gate", "validate", "--rule", "b_rule_auth", "--target", "candidate", "--mode", "delta"); err == nil || !strings.Contains(err.Error(), "full validate run only") {
		t.Fatalf("expected a rule delta to be rejected, got %v", err)
	}
}

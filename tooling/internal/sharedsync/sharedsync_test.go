package sharedsync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/snapshot"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func TestSyncImpactKeepsCandidateWhenOnlyBoundModulesChanged(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
  - module_other
---

# Shared

Body stays the same.
`)

	result, err := SyncImpact(repoRoot, Options{
		SharedRefs:                     []string{sharedRef},
		BoundModulesOnlySharedFileRefs: []string{"docs/specs/shared_contracts/candidate/c_shared_demo.md"},
	})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "unchanged" {
		t.Fatalf("expected unchanged outcome, got %+v", moduleResult)
	}
	if moduleResult.NextCommand != "cand_plan" {
		t.Fatalf("expected next command cand_plan, got %s", moduleResult.NextCommand)
	}
	if len(result.BoundModuleDrifts) != 1 {
		t.Fatalf("expected one bound_modules drift, got %d", len(result.BoundModuleDrifts))
	}
	if !result.BoundModuleDrifts[0].BoundModulesOnlyDelta {
		t.Fatalf("expected bound_modules-only drift, got %+v", result.BoundModuleDrifts[0])
	}
	checkPath := filepath.Join(repoRoot, "docs/specs/_check_result/module_demo.md")
	if _, err := os.Stat(checkPath); err != nil {
		t.Fatalf("expected process file to remain, stat err=%v", err)
	}
}

func TestSyncImpactInvalidatesCandidateWhenBoundModulesChangedWithoutExplicitDeclaration(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
  - module_other
---

# Shared

Body stays the same.
`)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "invalidated" {
		t.Fatalf("expected invalidated outcome, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("expected shared_contract_drift, got %s", moduleResult.FallbackReasonCode)
	}
	if result.BoundModuleDrifts[0].BoundModulesOnlyDelta {
		t.Fatalf("expected drift to remain unproven without explicit declaration, got %+v", result.BoundModuleDrifts[0])
	}
}

func TestSyncImpactInvalidatesCandidateOnSharedTruthDrift(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
---

# Shared

Body changed.
`)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "invalidated" {
		t.Fatalf("expected invalidated outcome, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("expected shared_contract_drift, got %s", moduleResult.FallbackReasonCode)
	}
	if moduleResult.NextCommand != "cand_check" {
		t.Fatalf("expected next command cand_check, got %s", moduleResult.NextCommand)
	}
	if !moduleResult.StatusUpdated {
		t.Fatalf("expected status update")
	}
	checkPath := filepath.Join(repoRoot, "docs/specs/_check_result/module_demo.md")
	if _, err := os.Stat(checkPath); !os.IsNotExist(err) {
		t.Fatalf("expected process file to be deleted, stat err=%v", err)
	}
	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), "| `module_demo` | `no` | `yes` | `candidate` | `cand_check` | current round |") {
		t.Fatalf("status row not updated:\n%s", string(statusData))
	}
}

func TestSyncImpactIncludesModulesStillBoundToDeletedSharedRef(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md")); err != nil {
		t.Fatalf("remove shared file: %v", err)
	}

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Module != "module_demo" {
		t.Fatalf("expected module_demo, got %+v", moduleResult)
	}
	if moduleResult.Outcome != "invalidated" {
		t.Fatalf("expected invalidated outcome, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "binding_drift" {
		t.Fatalf("expected binding_drift, got %s", moduleResult.FallbackReasonCode)
	}
}

func TestSyncImpactFailsClosedForSharedIDWhenBindingsPointToDeletedSharedRef(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	if err := os.Remove(filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md")); err != nil {
		t.Fatalf("remove shared file: %v", err)
	}

	_, err := SyncImpact(repoRoot, Options{SharedIDs: []string{"shared_demo"}})
	if err == nil {
		t.Fatalf("expected shared-id sync to fail closed when shared ref is unresolved")
	}
	if !strings.Contains(err.Error(), "cannot determine affected modules safely") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncImpactReroutesStableModuleToStableVerify(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupStableSharedRepo(t, repoRoot)

	writeStableSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: stable
shared_version: 1.0.0
bound_modules:
  - module_demo
---

# Shared

Body changed.
`)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "rerouted" {
		t.Fatalf("expected rerouted outcome, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("expected shared_contract_drift, got %s", moduleResult.FallbackReasonCode)
	}
	if moduleResult.NextCommand != "stable_verify" {
		t.Fatalf("expected next command stable_verify, got %s", moduleResult.NextCommand)
	}
	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), "| `module_demo` | `yes` | `no` | `stable` | `stable_verify` | stable round |") {
		t.Fatalf("status row not updated:\n%s", string(statusData))
	}
}

func TestSyncImpactRejectsStableModuleBindingCandidateShared(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.StableDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_demo` | `yes` | `no` | `stable` | `spec_fork` | stable round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.MainSpecFileRef("stable", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: module_demo",
		"layer: stable",
		"version: 1.0.0",
		"---",
		"",
		"# Demo",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_stable_ref: none",
		"2. shared_contract_refs:",
		"   - c_shared_demo@0.1.0",
		"",
	}, "\n"))

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
---

# Shared

Body stays the same.
`)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{"c_shared_demo@0.1.0"}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "rerouted" {
		t.Fatalf("expected rerouted outcome, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "binding_drift" {
		t.Fatalf("expected binding_drift, got %s", moduleResult.FallbackReasonCode)
	}
	if moduleResult.NextCommand != "stable_verify" {
		t.Fatalf("expected next command stable_verify, got %s", moduleResult.NextCommand)
	}
	if len(moduleResult.Diagnostics) == 0 || !strings.Contains(moduleResult.Diagnostics[0], "stable-layer module binding must use an s_ shared ref") {
		t.Fatalf("expected stable binding diagnostic, got %+v", moduleResult.Diagnostics)
	}
}

func TestSyncImpactKeepsStableLandingModule(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupStableSharedRepo(t, repoRoot)

	writeStableSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: stable
shared_version: 1.0.0
bound_modules:
  - module_demo
---

# Shared

Body changed.
`)

	result, err := SyncImpact(repoRoot, Options{
		Modules:             []string{"module_demo"},
		SharedRefs:          []string{sharedRef},
		StableLandingModule: "module_demo",
	})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "unchanged" {
		t.Fatalf("expected unchanged outcome for stable landing module, got %+v", moduleResult)
	}
	if moduleResult.NextCommand != "spec_fork" {
		t.Fatalf("expected next command spec_fork, got %s", moduleResult.NextCommand)
	}
}

func TestSyncImpactReroutesStableModuleWhenOnlyModuleScopeMarksChangedBinding(t *testing.T) {
	repoRoot := t.TempDir()
	setupStableSharedRepo(t, repoRoot)

	result, err := SyncImpact(repoRoot, Options{
		Modules: []string{"module_demo"},
	})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "rerouted" {
		t.Fatalf("expected rerouted outcome, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "binding_drift" {
		t.Fatalf("expected binding_drift, got %s", moduleResult.FallbackReasonCode)
	}
	if moduleResult.NextCommand != "stable_verify" {
		t.Fatalf("expected next command stable_verify, got %s", moduleResult.NextCommand)
	}
}

func TestSyncImpactRejectsEmptySharedContractRefsList(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: module_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_stable_ref: none",
		"2. shared_contract_refs:",
		"",
	}, "\n"))

	_, err = SyncImpact(repoRoot, Options{Modules: []string{"module_demo"}})
	if err == nil || !strings.Contains(err.Error(), "must not be an empty list") {
		t.Fatalf("expected empty-list error, got %v", err)
	}
}

func TestSyncImpactRejectsDuplicateSharedContractRefs(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: module_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_stable_ref: none",
		"2. shared_contract_refs:",
		"   - c_shared_demo@0.1.0",
		"   - c_shared_demo@0.1.0",
		"",
	}, "\n"))

	_, err = SyncImpact(repoRoot, Options{Modules: []string{"module_demo"}})
	if err == nil || !strings.Contains(err.Error(), "duplicate item") {
		t.Fatalf("expected duplicate-item error, got %v", err)
	}
}

func TestReconcileBoundModulesUpdatesTouchedSharedFiles(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_other
---

# Shared

Body stays the same.
`)

	result, err := ReconcileBoundModules(repoRoot, ReconcileBoundModulesOptions{
		SharedRefs: []string{sharedRef},
	})
	if err != nil {
		t.Fatalf("ReconcileBoundModules: %v", err)
	}
	if len(result.UpdatedFiles) != 1 {
		t.Fatalf("expected one updated file, got %+v", result)
	}

	updatedContent, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"))
	if err != nil {
		t.Fatalf("read updated shared file: %v", err)
	}
	if !strings.Contains(string(updatedContent), "bound_modules:\n  - module_demo\n") {
		t.Fatalf("expected bound_modules to be rewritten, got:\n%s", string(updatedContent))
	}
}

func setupCandidateSharedRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_demo` | `no` | `yes` | `candidate` | `cand_plan` | current round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: module_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_stable_ref: none",
		"2. shared_contract_refs:",
		"   - c_shared_demo@0.1.0",
		"",
	}, "\n"))

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
---

# Shared

Body stays the same.
`)

	snap, err := snapshot.RebuildCurrent(repoRoot, "module_demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	writeProcessFile(t, repoRoot, "check", snapshot.Render(snap))
	initGitRepo(t, repoRoot)
	return "c_shared_demo@0.1.0"
}

func setupStableSharedRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.StableDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/stable"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_demo` | `yes` | `no` | `stable` | `spec_fork` | stable round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.MainSpecFileRef("stable", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: module_demo",
		"layer: stable",
		"version: 1.0.0",
		"---",
		"",
		"# Demo",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_stable_ref: none",
		"2. shared_contract_refs:",
		"   - s_shared_demo@1.0.0",
		"",
	}, "\n"))

	writeStableSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: stable
shared_version: 1.0.0
bound_modules:
  - module_demo
---

# Shared

Body stays the same.
`)

	initGitRepo(t, repoRoot)
	return "s_shared_demo@1.0.0"
}

func writeProcessFile(t *testing.T, repoRoot, processKind, snapshotBody string) {
	t.Helper()
	dir := map[string]string{
		"check":  "docs/specs/_check_result",
		"plan":   "docs/specs/_plans",
		"verify": "docs/specs/_verify_result",
	}[processKind]
	mustMkdirAll(t, filepath.Join(repoRoot, dir))
	content := fmt.Sprintf("# %s\n\n```yaml\n%s\n```\n", processKind, snapshotBody)
	mustWriteFile(t, filepath.Join(repoRoot, dir, "module_demo.md"), content)
}

func writeSharedFile(t *testing.T, repoRoot, content string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"), content)
}

func writeStableSharedFile(t *testing.T, repoRoot, content string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/stable/s_shared_demo.md"), content)
}

func initGitRepo(t *testing.T, repoRoot string) {
	t.Helper()
	runGit(t, repoRoot, "init")
	runGit(t, repoRoot, "config", "user.name", "SpecFlow Test")
	runGit(t, repoRoot, "config", "user.email", "specflow@example.com")
	runGit(t, repoRoot, "add", ".")
	runGit(t, repoRoot, "commit", "-m", "init")
}

func runGit(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

package sharedsync

import (
	"crypto/sha256"
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
	if moduleResult.NextCommand != "module_plan" {
		t.Fatalf("expected next command module_plan, got %s", moduleResult.NextCommand)
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

func TestSyncImpactKeepsExplicitModuleScopeWhenOnlyBoundModulesChanged(t *testing.T) {
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
		Modules:                        []string{"module_demo"},
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
	if moduleResult.FallbackReasonCode != "" {
		t.Fatalf("expected no fallback reason, got %+v", moduleResult)
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

func TestSyncImpactRejectsMissingExplicitScope(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	_, err := SyncImpact(repoRoot, Options{})
	if err == nil || !strings.Contains(err.Error(), "at least one of shared refs or shared ids is required") {
		t.Fatalf("expected missing-scope error, got %v", err)
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
	if moduleResult.NextCommand != "module_check" {
		t.Fatalf("expected next command module_check, got %s", moduleResult.NextCommand)
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
	if !strings.Contains(string(statusData), "| `module_demo` | `no` | `yes` | `candidate` | `module_check` | current round |") {
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
	if !strings.Contains(err.Error(), "cannot determine affected downstream objects safely") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncImpactKeepsCandidateFlowWhenOnlyBoundModulesChanged(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)

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

	result, err := SyncImpact(repoRoot, Options{
		SharedRefs:                     []string{sharedRef},
		BoundModulesOnlySharedFileRefs: []string{"docs/specs/shared_contracts/candidate/c_shared_demo.md"},
	})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.FlowResults) != 1 {
		t.Fatalf("expected one flow result, got %d", len(result.FlowResults))
	}
	flowResult := result.FlowResults[0]
	if flowResult.Outcome != "unchanged" {
		t.Fatalf("expected unchanged outcome, got %+v", flowResult)
	}
	if flowResult.NextCommand != "flow_verify" {
		t.Fatalf("expected next command flow_verify, got %s", flowResult.NextCommand)
	}
	for _, relPath := range []string{
		"docs/specs/_check_result/flow_demo.md",
		"docs/specs/_verify_result/flow_demo.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, relPath)); err != nil {
			t.Fatalf("expected %s to remain, stat err=%v", relPath, err)
		}
	}
}

func TestSyncImpactInvalidatesCandidateFlowOnSharedTruthDrift(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules: none
---

# Shared

Body changed.
`)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.FlowResults) != 1 {
		t.Fatalf("expected one flow result, got %d", len(result.FlowResults))
	}
	flowResult := result.FlowResults[0]
	if flowResult.Object != "flow_demo" {
		t.Fatalf("expected flow_demo, got %+v", flowResult)
	}
	if flowResult.Outcome != "invalidated" {
		t.Fatalf("expected invalidated outcome, got %+v", flowResult)
	}
	if flowResult.FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("expected shared_contract_drift, got %s", flowResult.FallbackReasonCode)
	}
	if flowResult.NextCommand != "flow_check" {
		t.Fatalf("expected next command flow_check, got %s", flowResult.NextCommand)
	}
	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), "| `flow` | `flow_demo` | `no` | `yes` | `candidate` | `flow_check` | current round |") {
		t.Fatalf("status row not updated:\n%s", string(statusData))
	}
	for _, relPath := range []string{
		"docs/specs/_check_result/flow_demo.md",
		"docs/specs/_verify_result/flow_demo.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, relPath)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be deleted, stat err=%v", relPath, err)
		}
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
	if moduleResult.NextCommand != "module_stable_verify" {
		t.Fatalf("expected next command module_stable_verify, got %s", moduleResult.NextCommand)
	}
	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), "| `module_demo` | `yes` | `no` | `stable` | `module_stable_verify` | stable round |") {
		t.Fatalf("status row not updated:\n%s", string(statusData))
	}
}

func TestSyncImpactReroutesStableProjectToProjectStableVerify(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupStableProjectSharedRepo(t, repoRoot)

	writeStableSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: stable
shared_version: 1.0.0
bound_modules: none
---

# Shared

Body changed.
`)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ProjectResults) != 1 {
		t.Fatalf("expected one project result, got %d", len(result.ProjectResults))
	}
	projectResult := result.ProjectResults[0]
	if projectResult.Object != "project" {
		t.Fatalf("expected project, got %+v", projectResult)
	}
	if projectResult.Outcome != "rerouted" {
		t.Fatalf("expected rerouted outcome, got %+v", projectResult)
	}
	if projectResult.FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("expected shared_contract_drift, got %s", projectResult.FallbackReasonCode)
	}
	if projectResult.NextCommand != "project_stable_verify" {
		t.Fatalf("expected next command project_stable_verify, got %s", projectResult.NextCommand)
	}
	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(statusData), "| `project` | `project` | `yes` | `no` | `stable` | `project_stable_verify` | stable round |") {
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
		"| `module_demo` | `yes` | `no` | `stable` | `module_fork` | stable round |",
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
	if moduleResult.NextCommand != "module_stable_verify" {
		t.Fatalf("expected next command module_stable_verify, got %s", moduleResult.NextCommand)
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
		Modules:                 []string{"module_demo"},
		SharedRefs:              []string{sharedRef},
		StableLandingModule:     "module_demo",
		StableLandingSharedRefs: []string{sharedRef},
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
	if moduleResult.NextCommand != "module_fork" {
		t.Fatalf("expected next command module_fork, got %s", moduleResult.NextCommand)
	}
}

func TestSyncImpactStableLandingModuleStillReroutesOnUnrelatedSharedDrift(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupStableSharedRepo(t, repoRoot)

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
		"   - s_shared_extra@1.1.0",
		"",
	}, "\n"))

	writeSharedFileAtPath(t, repoRoot, "docs/specs/shared_contracts/stable/s_shared_extra.md", `---
shared_contract_id: shared_extra
layer: stable
shared_version: 1.1.0
bound_modules:
  - module_demo
---

# Shared

Body changed.
`)

	result, err := SyncImpact(repoRoot, Options{
		Modules:                 []string{"module_demo"},
		SharedRefs:              []string{sharedRef, "s_shared_extra@1.1.0"},
		StableLandingModule:     "module_demo",
		StableLandingSharedRefs: []string{sharedRef},
	})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %d", len(result.ModuleResults))
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "rerouted" {
		t.Fatalf("expected rerouted outcome for unrelated shared drift, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("expected shared_contract_drift, got %+v", moduleResult)
	}
	if moduleResult.NextCommand != "module_stable_verify" {
		t.Fatalf("expected next command module_stable_verify, got %s", moduleResult.NextCommand)
	}
}

func TestSyncImpactMixedSharedRefsStillInvalidateOnNonExemptRef(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

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
		"   - c_shared_extra@0.2.0",
		"",
	}, "\n"))

	writeSharedFileAtPath(t, repoRoot, "docs/specs/shared_contracts/candidate/c_shared_extra.md", `---
shared_contract_id: shared_extra
layer: candidate
shared_version: 0.2.0
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
	writeProcessFile(t, repoRoot, "check", renderModuleProcessSnapshotForTest(
		t,
		repoRoot,
		"check",
		"module_demo",
		snap.ModuleAppendixSnapshot,
		snap.SharedContractSnapshot,
	))

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
	writeSharedFileAtPath(t, repoRoot, "docs/specs/shared_contracts/candidate/c_shared_extra.md", `---
shared_contract_id: shared_extra
layer: candidate
shared_version: 0.2.0
bound_modules:
  - module_demo
---

# Shared

Body changed.
`)

	result, err := SyncImpact(repoRoot, Options{
		SharedRefs:                     []string{sharedRef, "c_shared_extra@0.2.0"},
		BoundModulesOnlySharedFileRefs: []string{"docs/specs/shared_contracts/candidate/c_shared_demo.md"},
	})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %+v", result.ModuleResults)
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "invalidated" {
		t.Fatalf("expected invalidated outcome, got %+v", moduleResult)
	}
	if moduleResult.FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("expected shared_contract_drift, got %+v", moduleResult)
	}
}

func TestSyncImpactDoesNotExpandScopeWithExplicitModuleSelector(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

	statusPath := filepath.Join(repoRoot, "docs/specs/_status.md")
	mustWriteFile(t, statusPath, strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_demo` | `no` | `yes` | `candidate` | `module_plan` | current round |",
		"| `module_other` | `no` | `yes` | `candidate` | `module_plan` | current round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "module_other")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: module_other",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_stable_ref: none",
		"2. shared_contract_refs: none",
		"",
	}, "\n"))

	moduleDemoRef, err := specpaths.MainSpecFileRef("candidate", "module_demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(moduleDemoRef)), strings.Join([]string{
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
		"2. shared_contract_refs: none",
		"",
	}, "\n"))

	result, err := SyncImpact(repoRoot, Options{
		Modules:    []string{"module_other"},
		SharedRefs: []string{sharedRef},
	})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedModules) != 0 {
		t.Fatalf("expected explicit module selector not to widen scope, got %+v", result.ScopedModules)
	}
	if len(result.ModuleResults) != 0 {
		t.Fatalf("expected no module results, got %+v", result.ModuleResults)
	}
}

func TestSyncImpactIncludesCandidateModuleWhenSelectedBindingWasRemovedFromCurrentTruth(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)
	processSnapshot, err := snapshot.LoadProcessSnapshot(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("LoadProcessSnapshot: %v", err)
	}
	storedProcess := renderModuleProcessSnapshotForTest(
		t,
		repoRoot,
		"check",
		"module_demo",
		processSnapshot.ModuleAppendixSnapshot,
		processSnapshot.SharedContractSnapshot,
	)

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
		"2. shared_contract_refs: none",
		"",
	}, "\n"))
	writeProcessFile(t, repoRoot, "check", storedProcess)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedModules) != 1 || result.ScopedModules[0] != "module_demo" {
		t.Fatalf("expected removed-binding module to remain in scope, got %+v", result.ScopedModules)
	}
	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %+v", result.ModuleResults)
	}
	if result.ModuleResults[0].Outcome != "invalidated" || result.ModuleResults[0].NextCommand != "module_check" {
		t.Fatalf("expected invalidated module fallback, got %+v", result.ModuleResults[0])
	}
}

func TestSyncImpactIgnoresIncompleteRemovedBindingEvidenceForModule(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

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
		"2. shared_contract_refs: none",
		"",
	}, "\n"))
	writeProcessFile(t, repoRoot, "check", strings.Join([]string{
		"shared_contract_snapshot:",
		"  - shared_contract_id: shared_demo",
		"    layer: candidate",
		"    file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"    version_ref: c_shared_demo@0.1.0",
		"    fingerprint: " + fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md")),
	}, "\n"))

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedModules) != 0 {
		t.Fatalf("expected incomplete module evidence to be ignored, got %+v", result.ScopedModules)
	}
	if len(result.ModuleResults) != 0 {
		t.Fatalf("expected no module fallback from incomplete evidence, got %+v", result.ModuleResults)
	}
}

func TestSyncImpactIgnoresModuleEvidenceThatDoesNotMatchCurrentModuleIdentity(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)
	processSnapshot, err := snapshot.LoadProcessSnapshot(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("LoadProcessSnapshot: %v", err)
	}

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
		"2. shared_contract_refs: none",
		"",
	}, "\n"))

	processPath := filepath.Join(repoRoot, "docs/specs/_check_result/module_demo.md")
	validProcess := renderModuleProcessSnapshotForTest(
		t,
		repoRoot,
		"check",
		"module_demo",
		processSnapshot.ModuleAppendixSnapshot,
		processSnapshot.SharedContractSnapshot,
	)
	rewritten := strings.Replace(validProcess, "truth_file_ref: docs/specs/modules/candidate/c_module_demo.md", "truth_file_ref: docs/specs/modules/candidate/c_module_other.md", 1)
	rewritten = strings.Replace(rewritten, "truth_fingerprint: ", "truth_fingerprint: wrong-", 1)
	mustWriteFile(t, processPath, "# check\n\n```yaml\n"+rewritten+"\n```\n")

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedModules) != 0 {
		t.Fatalf("expected mismatched module evidence to be rejected, got %+v", result.ScopedModules)
	}
	if len(result.ModuleResults) != 0 {
		t.Fatalf("expected no module fallback from mismatched module evidence, got %+v", result.ModuleResults)
	}
}

func TestSyncImpactIgnoresModuleEvidenceWhenCurrentTruthChangedBeyondRemovedBinding(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)
	processSnapshot, err := snapshot.LoadProcessSnapshot(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("LoadProcessSnapshot: %v", err)
	}
	storedProcess := renderModuleProcessSnapshotForTest(
		t,
		repoRoot,
		"check",
		"module_demo",
		processSnapshot.ModuleAppendixSnapshot,
		processSnapshot.SharedContractSnapshot,
	)

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
		"# Demo Updated",
		"",
		"Body changed outside shared bindings.",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_stable_ref: none",
		"2. shared_contract_refs: none",
		"",
	}, "\n"))
	writeProcessFile(t, repoRoot, "check", storedProcess)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedModules) != 0 {
		t.Fatalf("expected stale module evidence to be rejected, got %+v", result.ScopedModules)
	}
	if len(result.ModuleResults) != 0 {
		t.Fatalf("expected no module fallback from stale module evidence, got %+v", result.ModuleResults)
	}
}

func TestSyncImpactRejectsAmbiguousRemovedBindingSharedID(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/stable"))
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/stable/s_shared_demo.md"), strings.Join([]string{
		"---",
		"shared_contract_id: shared_demo",
		"layer: stable",
		"shared_version: 1.0.0",
		"bound_modules:",
		"  - module_demo",
		"---",
		"",
		"# Shared",
		"",
		"Stable body.",
		"",
	}, "\n"))

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
		"   - s_shared_demo@1.0.0",
		"",
	}, "\n"))
	storedProcess := renderModuleProcessSnapshotForTest(
		t,
		repoRoot,
		"check",
		"module_demo",
		nil,
		[]snapshot.SharedContractEntry{{
			SharedContractID: "shared_demo",
			Layer:            "stable",
			FileRef:          "docs/specs/shared_contracts/stable/s_shared_demo.md",
			VersionRef:       "s_shared_demo@1.0.0",
			Fingerprint:      fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/stable/s_shared_demo.md")),
		}},
	)
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
		"2. shared_contract_refs: none",
		"",
	}, "\n"))
	writeProcessFile(t, repoRoot, "check", storedProcess)

	_, err = SyncImpact(repoRoot, Options{SharedIDs: []string{"shared_demo"}})
	if err == nil || !strings.Contains(err.Error(), "removed-binding scope is ambiguous") {
		t.Fatalf("expected ambiguous shared-id removed-binding error, got %v", err)
	}
}

func TestSyncImpactRejectsAmbiguousCurrentBindingSharedID(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/stable"))
	writeSharedFileAtPath(t, repoRoot, "docs/specs/shared_contracts/stable/s_shared_demo.md", `---
shared_contract_id: shared_demo
layer: stable
shared_version: 1.0.0
bound_modules:
  - module_demo
---

# Shared

Stable body.
`)

	_, err := SyncImpact(repoRoot, Options{SharedIDs: []string{"shared_demo"}})
	if err == nil || !strings.Contains(err.Error(), "multiple current shared layers exist") {
		t.Fatalf("expected ambiguous current-binding shared-id error, got %v", err)
	}
}

func TestSyncImpactRejectsUnsortedSharedContractRefsInCurrentTruth(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	writeSharedFileAtPath(t, repoRoot, "docs/specs/shared_contracts/candidate/c_shared_alpha.md", `---
shared_contract_id: shared_alpha
layer: candidate
shared_version: 0.1.0
bound_modules:
  - module_demo
---

# Shared Alpha

Body stays the same.
`)

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
		"   - c_shared_alpha@0.1.0",
		"",
	}, "\n"))

	_, err = SyncImpact(repoRoot, Options{SharedRefs: []string{"c_shared_demo@0.1.0"}})
	if err == nil || !strings.Contains(err.Error(), "shared_contract_refs must be sorted") {
		t.Fatalf("expected unsorted shared_contract_refs error, got %v", err)
	}
}

func TestSyncImpactIncludesRemovedBindingWhenSharedIDIsUnambiguous(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	processSnapshot, err := snapshot.LoadProcessSnapshot(repoRoot, "module_demo", "check")
	if err != nil {
		t.Fatalf("LoadProcessSnapshot: %v", err)
	}
	storedProcess := renderModuleProcessSnapshotForTest(
		t,
		repoRoot,
		"check",
		"module_demo",
		processSnapshot.ModuleAppendixSnapshot,
		processSnapshot.SharedContractSnapshot,
	)

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
		"2. shared_contract_refs: none",
		"",
	}, "\n"))
	writeProcessFile(t, repoRoot, "check", storedProcess)

	result, err := SyncImpact(repoRoot, Options{SharedIDs: []string{"shared_demo"}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedModules) != 1 || result.ScopedModules[0] != "module_demo" {
		t.Fatalf("expected unambiguous shared-id removed binding to remain in scope, got %+v", result.ScopedModules)
	}
	if len(result.ModuleResults) != 1 || result.ModuleResults[0].Outcome != "invalidated" || result.ModuleResults[0].NextCommand != "module_check" {
		t.Fatalf("expected removed-binding shared-id path to invalidate module, got %+v", result.ModuleResults)
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

	_, err = SyncImpact(repoRoot, Options{
		Modules:    []string{"module_demo"},
		SharedRefs: []string{"c_shared_demo@0.1.0"},
	})
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

	_, err = SyncImpact(repoRoot, Options{
		Modules:    []string{"module_demo"},
		SharedRefs: []string{"c_shared_demo@0.1.0"},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate item") {
		t.Fatalf("expected duplicate-item error, got %v", err)
	}
}

func TestSyncImpactRejectsModulesOnlyScope(t *testing.T) {
	repoRoot := t.TempDir()
	setupStableSharedRepo(t, repoRoot)

	_, err := SyncImpact(repoRoot, Options{
		Modules: []string{"module_demo"},
	})
	if err == nil || !strings.Contains(err.Error(), "at least one of shared refs or shared ids is required") {
		t.Fatalf("expected modules-only scope to be rejected, got %v", err)
	}
}

func TestSyncImpactRejectsUnknownSharedRefWithoutCurrentBindingReference(t *testing.T) {
	repoRoot := t.TempDir()
	setupCandidateSharedRepo(t, repoRoot)

	_, err := SyncImpact(repoRoot, Options{
		SharedRefs: []string{"c_shared_missing@9.9.9"},
	})
	if err == nil || !strings.Contains(err.Error(), "is not present under docs/specs/shared_contracts/ and is not referenced by current downstream bindings") {
		t.Fatalf("expected unknown shared ref error, got %v", err)
	}
}

func TestSyncImpactIncludesCandidateFlowWhenSelectedBindingWasRemovedFromCurrentTruth(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)
	sharedFingerprint := fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"))
	storedProcess := renderFlowProcessSnapshotForTest(t, repoRoot, "check", "flow_demo", false, []string{
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"version_ref: c_shared_demo@0.1.0",
		"fingerprint: " + sharedFingerprint,
	}, nil)

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "flow_demo", storedProcess)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedFlows) != 1 || result.ScopedFlows[0] != "flow_demo" {
		t.Fatalf("expected removed-binding flow to remain in scope, got %+v", result.ScopedFlows)
	}
	if len(result.FlowResults) != 1 {
		t.Fatalf("expected one flow result, got %+v", result.FlowResults)
	}
	if result.FlowResults[0].FallbackReasonCode != "binding_drift" {
		t.Fatalf("expected binding_drift, got %+v", result.FlowResults[0])
	}
}

func TestSyncImpactIgnoresFlowEvidenceWithUnrelatedSharedSnapshotDelta(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "flow_demo", renderFlowProcessSnapshotForTest(t, repoRoot, "check", "flow_demo", false, []string{
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"version_ref: c_shared_demo@0.1.0",
		"fingerprint: " + fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md")),
		"shared_contract_id: shared_extra",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_extra.md",
		"version_ref: c_shared_extra@0.1.0",
		"fingerprint: extra",
	}, nil))

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedFlows) != 0 {
		t.Fatalf("expected unrelated shared snapshot delta to be rejected, got %+v", result.ScopedFlows)
	}
	if len(result.FlowResults) != 0 {
		t.Fatalf("expected no flow fallback from unrelated shared snapshot delta, got %+v", result.FlowResults)
	}
}

func TestSyncImpactIncludesCandidateProjectWhenSelectedBindingWasRemovedFromCurrentTruth(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateProjectSharedRepo(t, repoRoot)
	sharedFingerprint := fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"))
	storedProcess := renderProjectProcessSnapshotForTest(t, repoRoot, "check", "project", false, []string{
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"version_ref: c_shared_demo@0.1.0",
		"fingerprint: " + sharedFingerprint,
	}, nil, nil)

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("project", "candidate", "project")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: project",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Project",
		"",
		"## Bindings",
		"",
		"1. flow_refs: none",
		"2. module_refs: none",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "project", storedProcess)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedProjects) != 1 || result.ScopedProjects[0] != "project" {
		t.Fatalf("expected removed-binding project to remain in scope, got %+v", result.ScopedProjects)
	}
	if len(result.ProjectResults) != 1 {
		t.Fatalf("expected one project result, got %+v", result.ProjectResults)
	}
	if result.ProjectResults[0].FallbackReasonCode != "binding_drift" {
		t.Fatalf("expected binding_drift, got %+v", result.ProjectResults[0])
	}
}

func TestSyncImpactFailsClosedWhenCurrentFlowTruthCannotBeRebuilt(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: invalid",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "flow_demo", renderFlowProcessSnapshotForTest(t, repoRoot, "check", "flow_demo", false, []string{
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"version_ref: c_shared_demo@0.1.0",
		"fingerprint: " + fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md")),
	}, nil))

	_, err = SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err == nil || !strings.Contains(err.Error(), "module_refs must use literal none or a markdown list") {
		t.Fatalf("expected current truth rebuild error, got %v", err)
	}
}

func TestSyncImpactRejectsStableLandingModuleWithoutStableLandingSharedRefs(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupStableSharedRepo(t, repoRoot)

	_, err := SyncImpact(repoRoot, Options{
		SharedRefs:          []string{sharedRef},
		StableLandingModule: "module_demo",
	})
	if err == nil || !strings.Contains(err.Error(), "stable landing shared refs are required") {
		t.Fatalf("expected missing stable landing shared refs error, got %v", err)
	}
}

func TestSyncImpactIgnoresIncompleteRemovedBindingEvidenceForFlow(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "flow_demo", strings.Join([]string{
		"shared_contract_snapshot:",
		"  - shared_contract_id: shared_demo",
		"    layer: candidate",
		"    file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"    version_ref: c_shared_demo@0.1.0",
		"    fingerprint: demo",
	}, "\n"))

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedFlows) != 0 {
		t.Fatalf("expected incomplete flow evidence to be ignored, got %+v", result.ScopedFlows)
	}
	if len(result.FlowResults) != 0 {
		t.Fatalf("expected no flow fallback from incomplete evidence, got %+v", result.FlowResults)
	}
}

func TestSyncImpactIgnoresFlowEvidenceWithMismatchedModuleSnapshot(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "flow_demo", renderFlowProcessSnapshotForTest(t, repoRoot, "check", "flow_demo", false, []string{
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"version_ref: c_shared_demo@0.1.0",
		"fingerprint: " + fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md")),
	}, []string{
		"module: module_wrong",
		"layer: candidate",
		"file_ref: docs/specs/modules/candidate/c_module_wrong.md",
		"version_ref: c_module_wrong@0.1.0",
		"fingerprint: wrong",
	}))

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedFlows) != 0 {
		t.Fatalf("expected mismatched module snapshot to be rejected, got %+v", result.ScopedFlows)
	}
	if len(result.FlowResults) != 0 {
		t.Fatalf("expected no flow fallback from mismatched module snapshot, got %+v", result.FlowResults)
	}
}

func TestSyncImpactAcceptsMarkdownBulletRemovedBindingEvidenceForFlow(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)
	sharedFingerprint := fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"))
	storedProcess := renderFlowProcessSnapshotForTest(t, repoRoot, "check", "flow_demo", true, []string{
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"version_ref: c_shared_demo@0.1.0",
		"fingerprint: " + sharedFingerprint,
	}, nil)

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "flow_demo", storedProcess)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedFlows) != 1 || result.ScopedFlows[0] != "flow_demo" {
		t.Fatalf("expected markdown bullet flow evidence to remain valid, got %+v", result.ScopedFlows)
	}
	if len(result.FlowResults) != 1 || result.FlowResults[0].FallbackReasonCode != "binding_drift" {
		t.Fatalf("expected markdown bullet flow evidence to trigger fallback, got %+v", result.FlowResults)
	}
}

func TestSyncImpactAcceptsRemovedBindingEvidenceWhenTruthUsesBacktickedSharedRefs(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateFlowSharedRepo(t, repoRoot)
	sharedFingerprint := fingerprintForTest(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md"))

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs:",
		"   - `c_shared_demo@0.1.0`",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	storedProcess := renderFlowProcessSnapshotForTest(t, repoRoot, "check", "flow_demo", false, []string{
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"file_ref: docs/specs/shared_contracts/candidate/c_shared_demo.md",
		"version_ref: c_shared_demo@0.1.0",
		"fingerprint: " + sharedFingerprint,
	}, nil)
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs: none",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))
	writeNamedProcessFile(t, repoRoot, "check", "flow_demo", storedProcess)

	result, err := SyncImpact(repoRoot, Options{SharedRefs: []string{sharedRef}})
	if err != nil {
		t.Fatalf("SyncImpact: %v", err)
	}
	if len(result.ScopedFlows) != 1 || result.ScopedFlows[0] != "flow_demo" {
		t.Fatalf("expected backticked old truth evidence to remain valid, got %+v", result.ScopedFlows)
	}
	if len(result.FlowResults) != 1 || result.FlowResults[0].FallbackReasonCode != "binding_drift" {
		t.Fatalf("expected backticked old truth evidence to trigger fallback, got %+v", result.FlowResults)
	}
}

func TestSyncImpactRejectsUnknownStableLandingSharedRef(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupStableSharedRepo(t, repoRoot)

	_, err := SyncImpact(repoRoot, Options{
		SharedRefs:              []string{sharedRef},
		StableLandingModule:     "module_demo",
		StableLandingSharedRefs: []string{"s_shared_missing@9.9.9"},
	})
	if err == nil || !strings.Contains(err.Error(), "stable landing shared ref") {
		t.Fatalf("expected unknown stable landing shared ref error, got %v", err)
	}
}

func TestSyncImpactRejectsNonStableStableLandingModule(t *testing.T) {
	repoRoot := t.TempDir()
	sharedRef := setupCandidateSharedRepo(t, repoRoot)

	_, err := SyncImpact(repoRoot, Options{
		SharedRefs:              []string{sharedRef},
		StableLandingModule:     "module_demo",
		StableLandingSharedRefs: []string{sharedRef},
	})
	if err == nil || !strings.Contains(err.Error(), "must currently be at active layer stable") {
		t.Fatalf("expected non-stable stable landing module error, got %v", err)
	}
}

func TestSyncImpactRejectsStableLandingSharedRefOutsideSelectedScope(t *testing.T) {
	repoRoot := t.TempDir()
	setupStableSharedRepo(t, repoRoot)

	writeSharedFileAtPath(t, repoRoot, "docs/specs/shared_contracts/stable/s_shared_extra.md", `---
shared_contract_id: shared_extra
layer: stable
shared_version: 1.1.0
bound_modules:
  - module_demo
---

# Shared

Body stays the same.
`)

	_, err := SyncImpact(repoRoot, Options{
		SharedRefs:              []string{"s_shared_demo@1.0.0"},
		StableLandingModule:     "module_demo",
		StableLandingSharedRefs: []string{"s_shared_extra@1.1.0"},
	})
	if err == nil || !strings.Contains(err.Error(), "is not selected for stable landing module") {
		t.Fatalf("expected stable landing shared ref outside scope error, got %v", err)
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
		"| `module_demo` | `no` | `yes` | `candidate` | `module_plan` | current round |",
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
	writeProcessFile(t, repoRoot, "check", renderModuleProcessSnapshotForTest(t, repoRoot, "check", "module_demo", snap.ModuleAppendixSnapshot, snap.SharedContractSnapshot))
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
		"| `module_demo` | `yes` | `no` | `stable` | `module_fork` | stable round |",
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

func writeSharedFileAtPath(t *testing.T, repoRoot, relPath, content string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repoRoot, relPath), content)
}

func setupCandidateFlowSharedRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateFlowDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|",
		"| `flow` | `flow_demo` | `no` | `yes` | `candidate` | `flow_verify` | current round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", "flow_demo")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: flow_demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Flow",
		"",
		"## Bindings",
		"",
		"1. project_ref: c_project@0.1.0",
		"2. module_refs: none",
		"3. shared_contract_refs:",
		"   - c_shared_demo@0.1.0",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules: none
---

# Shared

Body stays the same.
`)

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/flow_demo.md"), "check")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_verify_result/flow_demo.md"), "verify")
	return "c_shared_demo@0.1.0"
}

func setupStableProjectSharedRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.StableProjectDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/stable"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|",
		"| `project` | `project` | `yes` | `no` | `stable` | `project_fork` | stable round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("project", "stable", "project")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: project",
		"layer: stable",
		"version: 1.0.0",
		"---",
		"",
		"# Demo Project",
		"",
		"## Bindings",
		"",
		"1. flow_refs: none",
		"2. module_refs: none",
		"3. shared_contract_refs:",
		"   - s_shared_demo@1.0.0",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))

	writeStableSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: stable
shared_version: 1.0.0
bound_modules: none
---

# Shared

Body stays the same.
`)

	return "s_shared_demo@1.0.0"
}

func setupCandidateProjectSharedRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateProjectDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|",
		"| `project` | `project` | `no` | `yes` | `candidate` | `project_verify` | current round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("project", "candidate", "project")
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: project",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo Project",
		"",
		"## Bindings",
		"",
		"1. flow_refs: none",
		"2. module_refs: none",
		"3. shared_contract_refs:",
		"   - c_shared_demo@0.1.0",
		"4. system_constraints_stable_ref: none",
		"",
	}, "\n"))

	writeSharedFile(t, repoRoot, `---
shared_contract_id: shared_demo
layer: candidate
shared_version: 0.1.0
bound_modules: none
---

# Shared

Body stays the same.
`)

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/project.md"), "check")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_verify_result/project.md"), "verify")
	return "c_shared_demo@0.1.0"
}

func writeProcessFile(t *testing.T, repoRoot, processKind, snapshotBody string) {
	t.Helper()
	writeNamedProcessFile(t, repoRoot, processKind, "module_demo", snapshotBody)
}

func writeNamedProcessFile(t *testing.T, repoRoot, processKind, object, snapshotBody string) {
	t.Helper()
	dir := map[string]string{
		"check":  "docs/specs/_check_result",
		"plan":   "docs/specs/_plans",
		"verify": "docs/specs/_verify_result",
	}[processKind]
	mustMkdirAll(t, filepath.Join(repoRoot, dir))
	content := fmt.Sprintf("# %s\n\n```yaml\n%s\n```\n", processKind, snapshotBody)
	mustWriteFile(t, filepath.Join(repoRoot, dir, object+".md"), content)
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

func renderModuleProcessSnapshotForTest(t *testing.T, repoRoot, processKind, module string, appendixEntries []snapshot.AppendixEntry, sharedEntries []snapshot.SharedContractEntry) string {
	t.Helper()
	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", module)
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	truthFingerprint := fingerprintForTest(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	scalars := []string{
		"object_type: module",
		"object_ref: " + module,
		"gate: " + map[string]string{"check": "module_check", "verify": "module_verify"}[processKind],
		"decision: pass",
		"allow_next: true",
		"next_command: " + map[string]string{"check": "module_plan", "verify": "module_promote"}[processKind],
		"blocking_summary: none",
		"coverage_summary: current candidate",
		"truth_layer_ref: candidate",
		"truth_file_ref: " + mainSpecRef,
		"truth_version_ref: c_" + module + "@0.1.0",
		"truth_fingerprint: " + truthFingerprint,
		"system_constraints_stable_file_ref: none",
		"system_constraints_stable_version_ref: none",
		"system_constraints_stable_fingerprint: none",
	}
	if processKind == "verify" {
		scalars = append(scalars, "verification_scope_ref: current candidate")
	}
	appendixLines := []string{}
	for _, entry := range appendixEntries {
		appendixLines = append(appendixLines,
			"file_ref: "+entry.FileRef,
			"appendix_ref: "+entry.AppendixRef,
			"fingerprint: "+entry.Fingerprint,
		)
	}
	sharedLines := []string{}
	for _, entry := range sharedEntries {
		sharedLines = append(sharedLines,
			"shared_contract_id: "+entry.SharedContractID,
			"layer: "+entry.Layer,
			"file_ref: "+entry.FileRef,
			"version_ref: "+entry.VersionRef,
			"fingerprint: "+entry.Fingerprint,
		)
	}
	lists := [][]string{
		append([]string{"module_appendix_snapshot: " + noneOrBlank(appendixLines)}, prefixNestedList(appendixLines)...),
		append([]string{"shared_contract_snapshot: " + noneOrBlank(sharedLines)}, prefixNestedList(sharedLines)...),
	}
	return renderSnapshotBodyForTest(scalars, lists, false)
}

func renderFlowProcessSnapshotForTest(t *testing.T, repoRoot, processKind, object string, bulletFormat bool, sharedLines, moduleLines []string) string {
	t.Helper()
	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("flow", "candidate", object)
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	truthFingerprint := fingerprintForTest(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	scalars := []string{
		"object_type: flow",
		"object_ref: " + object,
		"gate: " + map[string]string{"check": "flow_check", "verify": "flow_verify"}[processKind],
		"decision: pass",
		"allow_next: true",
		"next_command: " + map[string]string{"check": "flow_verify", "verify": "flow_promote"}[processKind],
		"blocking_summary: none",
		"coverage_summary: current candidate",
		"truth_layer_ref: candidate",
		"truth_file_ref: " + mainSpecRef,
		"truth_version_ref: c_flow_" + object + "@0.1.0",
		"truth_fingerprint: " + truthFingerprint,
		"system_constraints_stable_file_ref: none",
		"system_constraints_stable_version_ref: none",
		"system_constraints_stable_fingerprint: none",
	}
	if processKind == "verify" {
		scalars = append(scalars, "verification_scope_ref: current candidate")
	}
	lists := [][]string{
		append([]string{"module_snapshot: " + noneOrBlank(moduleLines)}, prefixNestedList(moduleLines)...),
		append([]string{"shared_contract_snapshot: " + noneOrBlank(sharedLines)}, prefixNestedList(sharedLines)...),
	}
	return renderSnapshotBodyForTest(scalars, lists, bulletFormat)
}

func renderProjectProcessSnapshotForTest(t *testing.T, repoRoot, processKind, object string, bulletFormat bool, sharedLines, moduleLines, flowLines []string) string {
	t.Helper()
	mainSpecRef, err := specpaths.ObjectMainSpecFileRef("project", "candidate", object)
	if err != nil {
		t.Fatalf("ObjectMainSpecFileRef: %v", err)
	}
	truthFingerprint := fingerprintForTest(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	scalars := []string{
		"object_type: project",
		"object_ref: " + object,
		"gate: " + map[string]string{"check": "project_check", "verify": "project_verify"}[processKind],
		"decision: pass",
		"allow_next: true",
		"next_command: " + map[string]string{"check": "project_verify", "verify": "project_promote"}[processKind],
		"blocking_summary: none",
		"coverage_summary: current candidate",
		"truth_layer_ref: candidate",
		"truth_file_ref: " + mainSpecRef,
		"truth_version_ref: c_project@0.1.0",
		"truth_fingerprint: " + truthFingerprint,
		"system_constraints_stable_file_ref: none",
		"system_constraints_stable_version_ref: none",
		"system_constraints_stable_fingerprint: none",
	}
	if processKind == "verify" {
		scalars = append(scalars, "verification_scope_ref: current candidate")
	}
	lists := [][]string{
		append([]string{"flow_snapshot: " + noneOrBlank(flowLines)}, prefixNestedList(flowLines)...),
		append([]string{"module_snapshot: " + noneOrBlank(moduleLines)}, prefixNestedList(moduleLines)...),
		append([]string{"shared_contract_snapshot: " + noneOrBlank(sharedLines)}, prefixNestedList(sharedLines)...),
	}
	return renderSnapshotBodyForTest(scalars, lists, bulletFormat)
}

func renderSnapshotBodyForTest(scalars []string, lists [][]string, bulletFormat bool) string {
	lines := []string{}
	for _, scalar := range scalars {
		if bulletFormat {
			key, value, _ := strings.Cut(scalar, ": ")
			lines = append(lines, fmt.Sprintf("- `%s`: `%s`", key, value))
			continue
		}
		lines = append(lines, scalar)
	}
	for _, list := range lists {
		header := list[0]
		items := list[1:]
		if bulletFormat {
			key, value, _ := strings.Cut(header, ": ")
			if value == "none" {
				lines = append(lines, fmt.Sprintf("- `%s`: `none`", key))
				continue
			}
			lines = append(lines, fmt.Sprintf("- `%s`:", key))
			for _, item := range items {
				trimmed := strings.TrimSpace(item)
				trimmed = strings.TrimPrefix(trimmed, "- ")
				key, value, _ := strings.Cut(trimmed, ": ")
				lines = append(lines, fmt.Sprintf("  - `%s`: `%s`", key, value))
			}
			continue
		}
		lines = append(lines, header)
		lines = append(lines, items...)
	}
	return strings.Join(lines, "\n")
}

func noneOrBlank(lines []string) string {
	if len(lines) == 0 {
		return "none"
	}
	return ""
}

func prefixNestedList(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, "  - "+line)
	}
	return result
}

func fingerprintForTest(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	text += "\n"
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

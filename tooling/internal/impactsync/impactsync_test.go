package impactsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/snapshot"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func TestApplyInvalidatesCandidateObjectsAndCleansProcessFiles(t *testing.T) {
	repoRoot := t.TempDir()
	setupImpactRepo(t, repoRoot, strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|---|",
		"| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_plan` | current round |",
		"| `scenario` | `demo` | `no` | `yes` | `candidate` | `scenario_verify` | current round |",
	}, "\n")+"\n")
	for _, relPath := range []string{
		"docs/specs/_check_result/unit/demo.md",
		"docs/specs/_plans/active/demo.md",
		"docs/specs/_plans/draft/demo.md",
		"docs/specs/_verify_result/unit/demo.md",
		"docs/specs/_check_result/scenario/demo.md",
		"docs/specs/_verify_result/scenario/demo.md",
	} {
		mustWriteImpactFile(t, filepath.Join(repoRoot, relPath), "# process\n")
	}

	result, err := Apply(repoRoot, Input{
		Modules: []ScopedModule{{
			Binding: ModuleBinding{
				Module:        "demo",
				ActiveLayer:   "candidate",
				NextCommand:   "unit_plan",
				BindingIssues: []string{"binding drift"},
			},
		}},
		Flows: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:    "scenario",
				Object:        "demo",
				ActiveLayer:   "candidate",
				NextCommand:   "scenario_verify",
				BindingIssues: []string{"binding drift"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.ModuleResults) != 1 || result.ModuleResults[0].NextCommand != "unit_check" || result.ModuleResults[0].Outcome != "invalidated" {
		t.Fatalf("unexpected module result: %+v", result.ModuleResults)
	}
	if len(result.FlowResults) != 1 || result.FlowResults[0].NextCommand != "scenario_check" || result.FlowResults[0].Outcome != "invalidated" {
		t.Fatalf("unexpected flow result: %+v", result.FlowResults)
	}
	for _, relPath := range []string{
		"docs/specs/_check_result/unit/demo.md",
		"docs/specs/_plans/active/demo.md",
		"docs/specs/_plans/draft/demo.md",
		"docs/specs/_verify_result/unit/demo.md",
		"docs/specs/_check_result/scenario/demo.md",
		"docs/specs/_verify_result/scenario/demo.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, relPath)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be deleted, stat err=%v", relPath, err)
		}
	}

	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	statusText := string(statusData)
	for _, expected := range []string{
		"| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_check` | current round |",
		"| `scenario` | `demo` | `no` | `yes` | `candidate` | `scenario_check` | current round |",
	} {
		if !strings.Contains(statusText, expected) {
			t.Fatalf("status row %q not updated:\n%s", expected, statusText)
		}
	}
}

func TestApplyReroutesStableObjectsToVerifyCommands(t *testing.T) {
	repoRoot := t.TempDir()
	setupImpactRepo(t, repoRoot, strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|---|",
		"| `unit` | `demo` | `yes` | `no` | `stable` | `unit_fork` | stable round |",
		"| `scenario` | `demo` | `yes` | `no` | `stable` | `scenario_fork` | stable round |",
	}, "\n")+"\n")

	result, err := Apply(repoRoot, Input{
		Modules: []ScopedModule{{
			Binding: ModuleBinding{
				Module:        "demo",
				ActiveLayer:   "stable",
				NextCommand:   "unit_fork",
				BindingIssues: []string{"binding drift"},
			},
		}},
		Flows: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:    "scenario",
				Object:        "demo",
				ActiveLayer:   "stable",
				NextCommand:   "scenario_fork",
				BindingIssues: []string{"binding drift"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.ModuleResults) != 1 || result.ModuleResults[0].NextCommand != "unit_stable_verify" || result.ModuleResults[0].Outcome != "rerouted" {
		t.Fatalf("unexpected module result: %+v", result.ModuleResults)
	}
	if len(result.FlowResults) != 1 || result.FlowResults[0].NextCommand != "scenario_stable_verify" || result.FlowResults[0].Outcome != "rerouted" {
		t.Fatalf("unexpected flow result: %+v", result.FlowResults)
	}
	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	statusText := string(statusData)
	for _, expected := range []string{
		"| `unit` | `demo` | `yes` | `no` | `stable` | `unit_stable_verify` | stable round |",
		"| `scenario` | `demo` | `yes` | `no` | `stable` | `scenario_stable_verify` | stable round |",
	} {
		if !strings.Contains(statusText, expected) {
			t.Fatalf("status row %q not updated:\n%s", expected, statusText)
		}
	}
}

func TestApplyUsesResolvedSharedInvalidationForStableObjects(t *testing.T) {
	repoRoot := t.TempDir()
	setupImpactRepo(t, repoRoot, strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|---|",
		"| `unit` | `demo` | `yes` | `no` | `stable` | `unit_fork` | stable round |",
		"| `scenario` | `demo` | `yes` | `no` | `stable` | `scenario_fork` | stable round |",
	}, "\n")+"\n")

	result, err := Apply(repoRoot, Input{
		Modules: []ScopedModule{{
			Binding: ModuleBinding{
				Module:      "demo",
				ActiveLayer: "stable",
				NextCommand: "unit_fork",
			},
			InvalidatingSharedRefs: []string{"s_shared_demo@1.0.0"},
		}},
		Flows: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:  "scenario",
				Object:      "demo",
				ActiveLayer: "stable",
				NextCommand: "scenario_fork",
			},
			InvalidatingSharedRefs: []string{"s_shared_demo@1.0.0"},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.ModuleResults) != 1 || result.ModuleResults[0].FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("unexpected module result: %+v", result.ModuleResults)
	}
	if len(result.FlowResults) != 1 || result.FlowResults[0].FallbackReasonCode != "shared_contract_drift" {
		t.Fatalf("unexpected flow result: %+v", result.FlowResults)
	}
}

func TestApplyUsesExplicitFallbackScopeForObjects(t *testing.T) {
	repoRoot := t.TempDir()
	setupImpactRepo(t, repoRoot, strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|---|",
		"| `scenario` | `demo` | `no` | `yes` | `candidate` | `scenario_verify` | current round |",
	}, "\n")+"\n")
	for _, relPath := range []string{
		"docs/specs/_check_result/scenario/demo.md",
		"docs/specs/_verify_result/scenario/demo.md",
	} {
		mustWriteImpactFile(t, filepath.Join(repoRoot, relPath), "# process\n")
	}

	result, err := Apply(repoRoot, Input{
		Flows: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:  "scenario",
				Object:      "demo",
				ActiveLayer: "candidate",
				NextCommand: "scenario_verify",
			},
			ExplicitFallbackScope: true,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.FlowResults) != 1 || result.FlowResults[0].FallbackReasonCode != "binding_drift" || result.FlowResults[0].NextCommand != "scenario_check" {
		t.Fatalf("unexpected flow result: %+v", result.FlowResults)
	}
}

func TestApplyKeepsObjectsUnchangedWithoutFallbackInputs(t *testing.T) {
	repoRoot := t.TempDir()
	setupImpactRepo(t, repoRoot, strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|---|",
		"| `scenario` | `demo` | `no` | `yes` | `candidate` | `scenario_verify` | current round |",
	}, "\n")+"\n")

	result, err := Apply(repoRoot, Input{
		Flows: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:  "scenario",
				Object:      "demo",
				ActiveLayer: "candidate",
				NextCommand: "scenario_verify",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.FlowResults) != 1 || result.FlowResults[0].Outcome != "unchanged" {
		t.Fatalf("unexpected flow result: %+v", result.FlowResults)
	}
}

func TestApplyKeepsCandidateModuleWhenCallerAllowsSharedSnapshotMismatch(t *testing.T) {
	repoRoot := t.TempDir()
	allowedFileRef := setupImpactModuleSharedRepo(t, repoRoot)

	mustWriteImpactFile(t, filepath.Join(repoRoot, allowedFileRef), strings.Join([]string{
		"---",
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"shared_version: 0.1.0",
		"bound_objects:",
		"  - unit:demo",
		"---",
		"",
		"# Shared",
		"",
		"Body changed.",
		"",
	}, "\n"))

	result, err := Apply(repoRoot, Input{
		Modules: []ScopedModule{{
			Binding: ModuleBinding{
				Module:      "demo",
				ActiveLayer: "candidate",
				NextCommand: "unit_plan",
			},
			AllowedSharedSnapshotMismatchFileRefs: []string{"docs/specs/shared_contracts/candidate/c_shared_demo.md"},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %+v", result.ModuleResults)
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "unchanged" {
		t.Fatalf("expected unchanged outcome, got %+v", moduleResult)
	}
	if moduleResult.NextCommand != "unit_plan" {
		t.Fatalf("expected next command unit_plan, got %+v", moduleResult)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/_check_result/unit/demo.md")); err != nil {
		t.Fatalf("expected process file to remain, stat err=%v", err)
	}
}

func TestApplyKeepsCandidateModuleWhenPlanUsesPlanContract(t *testing.T) {
	repoRoot := t.TempDir()
	setupImpactModuleSharedRepo(t, repoRoot)

	snap, err := snapshot.RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	mustWriteImpactFile(t, filepath.Join(repoRoot, "docs/specs/_plans/active/demo.md"), renderImpactPlanProcessSnapshot(snap))

	result, err := Apply(repoRoot, Input{
		Modules: []ScopedModule{{
			Binding: ModuleBinding{
				Module:      "demo",
				ActiveLayer: "candidate",
				NextCommand: "unit_verify",
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.ModuleResults) != 1 {
		t.Fatalf("expected one module result, got %+v", result.ModuleResults)
	}
	moduleResult := result.ModuleResults[0]
	if moduleResult.Outcome != "unchanged" || moduleResult.NextCommand != "unit_verify" {
		t.Fatalf("expected unchanged module with valid plan contract, got %+v", moduleResult)
	}
}

func setupImpactRepo(t *testing.T, repoRoot, statusContent string) {
	t.Helper()
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_check_result/unit"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_check_result/scenario"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_plans/active"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_plans/draft"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result/unit"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result/scenario"))
	mustWriteImpactFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), statusContent)
}

func setupImpactModuleSharedRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	mustMkdirImpactAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateDir)))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_check_result/unit"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_plans/active"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_plans/draft"))
	mustWriteImpactFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|",
		"| `unit` | `demo` | `no` | `yes` | `candidate` | `unit_plan` | current round |",
	}, "\n")+"\n")

	mainSpecRef, err := specpaths.MainSpecFileRef("candidate", "demo")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteImpactFile(t, filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)), strings.Join([]string{
		"---",
		"id: demo",
		"layer: candidate",
		"version: 0.1.0",
		"---",
		"",
		"# Demo",
		"",
		"## Global Constraint Alignment",
		"",
		"1. system_constraints_ref: none",
		"2. shared_contract_refs:",
		"   - c_shared_demo@0.1.0",
		"",
	}, "\n"))

	sharedPath := filepath.Join(repoRoot, "docs/specs/shared_contracts/candidate/c_shared_demo.md")
	mustWriteImpactFile(t, sharedPath, strings.Join([]string{
		"---",
		"shared_contract_id: shared_demo",
		"layer: candidate",
		"shared_version: 0.1.0",
		"bound_objects:",
		"  - unit:demo",
		"---",
		"",
		"# Shared",
		"",
		"Body stays the same.",
		"",
	}, "\n"))

	snap, err := snapshot.RebuildCurrent(repoRoot, "demo")
	if err != nil {
		t.Fatalf("RebuildCurrent: %v", err)
	}
	mustWriteImpactFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/unit/demo.md"), renderImpactCheckProcessSnapshot(snap))
	return "docs/specs/shared_contracts/candidate/c_shared_demo.md"
}

func mustMkdirImpactAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteImpactFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func renderImpactCheckProcessSnapshot(snap snapshot.Snapshot) string {
	lines := []string{
		"# check",
		"",
		"```yaml",
		"object_type: unit",
		"object_ref: " + snap.Module,
		"gate: unit_check",
		"decision: pass",
		"allow_next: true",
		"next_command: unit_plan",
		"blocking_summary: none",
		"coverage_summary: current candidate",
		"truth_layer_ref: " + snap.TruthLayerRef,
		"truth_file_ref: " + snap.SpecFileRef,
		"truth_version_ref: " + snap.SpecVersionRef,
		"truth_fingerprint: " + snap.SpecFingerprint,
		"system_constraints_file_ref: " + snap.SystemConstraintsFileRef,
		"system_constraints_version_ref: " + snap.SystemConstraintsVersionRef,
		"system_constraints_fingerprint: " + snap.SystemConstraintsFingerprint,
		"unit_appendix_snapshot: none",
		"shared_contract_snapshot:",
	}
	for _, entry := range snap.SharedContractSnapshot {
		lines = append(lines,
			"  - shared_contract_id: "+entry.SharedContractID,
			"    layer: "+entry.Layer,
			"    file_ref: "+entry.FileRef,
			"    version_ref: "+entry.VersionRef,
			"    fingerprint: "+entry.Fingerprint,
		)
	}
	if len(snap.SharedContractSnapshot) == 0 {
		lines[len(lines)-1] = "shared_contract_snapshot: none"
	}
	lines = append(lines, "```", "")
	return strings.Join(lines, "\n")
}

func renderImpactPlanProcessSnapshot(snap snapshot.Snapshot) string {
	return strings.Join([]string{
		"# plan",
		"",
		"```yaml",
		"spec_file_ref: " + snap.SpecFileRef,
		"spec_version_ref: " + snap.SpecVersionRef,
		"spec_fingerprint: " + snap.SpecFingerprint,
		"unit_appendix_snapshot: none",
		"system_constraints_file_ref: " + snap.SystemConstraintsFileRef,
		"system_constraints_version_ref: " + snap.SystemConstraintsVersionRef,
		"system_constraints_fingerprint: " + snap.SystemConstraintsFingerprint,
		"shared_contract_snapshot:",
		"  - shared_contract_id: " + snap.SharedContractSnapshot[0].SharedContractID,
		"    layer: " + snap.SharedContractSnapshot[0].Layer,
		"    file_ref: " + snap.SharedContractSnapshot[0].FileRef,
		"    version_ref: " + snap.SharedContractSnapshot[0].VersionRef,
		"    fingerprint: " + snap.SharedContractSnapshot[0].Fingerprint,
		"```",
		"",
	}, "\n")
}

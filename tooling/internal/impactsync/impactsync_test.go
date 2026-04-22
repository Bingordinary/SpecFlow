package impactsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyInvalidatesCandidateObjectsAndCleansProcessFiles(t *testing.T) {
	repoRoot := t.TempDir()
	setupImpactRepo(t, repoRoot, strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|",
		"| `module` | `module_demo` | `no` | `yes` | `candidate` | `cand_plan` | current round |",
		"| `flow` | `flow_demo` | `no` | `yes` | `candidate` | `flow_verify` | current round |",
		"| `project` | `project` | `no` | `yes` | `candidate` | `project_verify` | current round |",
	}, "\n")+"\n")
	for _, relPath := range []string{
		"docs/specs/_check_result/module_demo.md",
		"docs/specs/_plans/module_demo.md",
		"docs/specs/_verify_result/module_demo.md",
		"docs/specs/_check_result/flow_demo.md",
		"docs/specs/_verify_result/flow_demo.md",
		"docs/specs/_check_result/project.md",
		"docs/specs/_verify_result/project.md",
	} {
		mustWriteImpactFile(t, filepath.Join(repoRoot, relPath), "# process\n")
	}

	result, err := Apply(repoRoot, Input{
		Modules: []ScopedModule{{
			Binding: ModuleBinding{
				Module:        "module_demo",
				ActiveLayer:   "candidate",
				NextCommand:   "cand_plan",
				BindingIssues: []string{"binding drift"},
			},
		}},
		Flows: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:    "flow",
				Object:        "flow_demo",
				ActiveLayer:   "candidate",
				NextCommand:   "flow_verify",
				BindingIssues: []string{"binding drift"},
			},
		}},
		Projects: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:    "project",
				Object:        "project",
				ActiveLayer:   "candidate",
				NextCommand:   "project_verify",
				BindingIssues: []string{"binding drift"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.ModuleResults) != 1 || result.ModuleResults[0].NextCommand != "cand_check" || result.ModuleResults[0].Outcome != "invalidated" {
		t.Fatalf("unexpected module result: %+v", result.ModuleResults)
	}
	if len(result.FlowResults) != 1 || result.FlowResults[0].NextCommand != "flow_check" || result.FlowResults[0].Outcome != "invalidated" {
		t.Fatalf("unexpected flow result: %+v", result.FlowResults)
	}
	if len(result.ProjectResults) != 1 || result.ProjectResults[0].NextCommand != "project_check" || result.ProjectResults[0].Outcome != "invalidated" {
		t.Fatalf("unexpected project result: %+v", result.ProjectResults)
	}

	for _, relPath := range []string{
		"docs/specs/_check_result/module_demo.md",
		"docs/specs/_plans/module_demo.md",
		"docs/specs/_verify_result/module_demo.md",
		"docs/specs/_check_result/flow_demo.md",
		"docs/specs/_verify_result/flow_demo.md",
		"docs/specs/_check_result/project.md",
		"docs/specs/_verify_result/project.md",
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
		"| `module` | `module_demo` | `no` | `yes` | `candidate` | `cand_check` | current round |",
		"| `flow` | `flow_demo` | `no` | `yes` | `candidate` | `flow_check` | current round |",
		"| `project` | `project` | `no` | `yes` | `candidate` | `project_check` | current round |",
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
		"|---|---|---|---|---|---|---|",
		"| `module` | `module_demo` | `yes` | `no` | `stable` | `spec_fork` | stable round |",
		"| `flow` | `flow_demo` | `yes` | `no` | `stable` | `flow_fork` | stable round |",
		"| `project` | `project` | `yes` | `no` | `stable` | `project_fork` | stable round |",
	}, "\n")+"\n")

	result, err := Apply(repoRoot, Input{
		Modules: []ScopedModule{{
			Binding: ModuleBinding{
				Module:        "module_demo",
				ActiveLayer:   "stable",
				NextCommand:   "spec_fork",
				BindingIssues: []string{"binding drift"},
			},
		}},
		Flows: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:    "flow",
				Object:        "flow_demo",
				ActiveLayer:   "stable",
				NextCommand:   "flow_fork",
				BindingIssues: []string{"binding drift"},
			},
		}},
		Projects: []ScopedObject{{
			Binding: ObjectBinding{
				ObjectType:    "project",
				Object:        "project",
				ActiveLayer:   "stable",
				NextCommand:   "project_fork",
				BindingIssues: []string{"binding drift"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(result.ModuleResults) != 1 || result.ModuleResults[0].NextCommand != "stable_verify" || result.ModuleResults[0].Outcome != "rerouted" {
		t.Fatalf("unexpected module result: %+v", result.ModuleResults)
	}
	if len(result.FlowResults) != 1 || result.FlowResults[0].NextCommand != "flow_stable_verify" || result.FlowResults[0].Outcome != "rerouted" {
		t.Fatalf("unexpected flow result: %+v", result.FlowResults)
	}
	if len(result.ProjectResults) != 1 || result.ProjectResults[0].NextCommand != "project_stable_verify" || result.ProjectResults[0].Outcome != "rerouted" {
		t.Fatalf("unexpected project result: %+v", result.ProjectResults)
	}

	statusData, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	statusText := string(statusData)
	for _, expected := range []string{
		"| `module` | `module_demo` | `yes` | `no` | `stable` | `stable_verify` | stable round |",
		"| `flow` | `flow_demo` | `yes` | `no` | `stable` | `flow_stable_verify` | stable round |",
		"| `project` | `project` | `yes` | `no` | `stable` | `project_stable_verify` | stable round |",
	} {
		if !strings.Contains(statusText, expected) {
			t.Fatalf("status row %q not updated:\n%s", expected, statusText)
		}
	}
}

func setupImpactRepo(t *testing.T, repoRoot, statusContent string) {
	t.Helper()
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_plans"))
	mustMkdirImpactAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result"))
	mustWriteImpactFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), statusContent)
}

func mustMkdirImpactAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteImpactFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

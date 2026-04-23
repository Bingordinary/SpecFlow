package statusfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateNextCommand(t *testing.T) {
	repoRoot := t.TempDir()
	statusPath := filepath.Join(repoRoot, "docs/specs")
	if err := os.MkdirAll(statusPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `yes` | `candidate` | `module_check` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(statusPath, "_status.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	updated, err := UpdateNextCommand(repoRoot, "module_ai", "module_plan")
	if err != nil {
		t.Fatalf("UpdateNextCommand: %v", err)
	}
	if !updated {
		t.Fatalf("expected update to be true")
	}

	data, err := os.ReadFile(filepath.Join(statusPath, "_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(data), "| `module_ai` | `yes` | `yes` | `candidate` | `module_plan` | note |") {
		t.Fatalf("updated status row not found:\n%s", string(data))
	}
}

func TestUpsertModuleStatusCreatesNewRow(t *testing.T) {
	repoRoot := t.TempDir()
	statusPath := filepath.Join(repoRoot, "docs/specs")
	if err := os.MkdirAll(statusPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `no` | `stable` | `module_fork` | stable note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(statusPath, "_status.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	updated, err := UpsertModuleStatus(repoRoot, ModuleStatus{
		Module:      "module_new",
		Stable:      "no",
		Candidate:   "yes",
		ActiveLayer: "candidate",
		NextCommand: "module_check",
		Notes:       "new note",
	}, true)
	if err != nil {
		t.Fatalf("UpsertModuleStatus: %v", err)
	}
	if !updated {
		t.Fatalf("expected update to be true")
	}

	data, err := os.ReadFile(filepath.Join(statusPath, "_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(data), "| `module_new` | `no` | `yes` | `candidate` | `module_check` | new note |") {
		t.Fatalf("created status row not found:\n%s", string(data))
	}
}

func TestUpsertModuleStatusRejectsUnsupportedNextCommand(t *testing.T) {
	repoRoot := t.TempDir()
	statusPath := filepath.Join(repoRoot, "docs/specs")
	if err := os.MkdirAll(statusPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `no` | `stable` | `module_fork` | stable note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(statusPath, "_status.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	_, err := UpsertModuleStatus(repoRoot, ModuleStatus{
		Module:      "module_ai",
		Stable:      "yes",
		Candidate:   "no",
		ActiveLayer: "stable",
		NextCommand: "typo_command",
		Notes:       "stable note",
	}, false)
	if err == nil || !strings.Contains(err.Error(), "not a supported status value") {
		t.Fatalf("expected unsupported next command error, got %v", err)
	}
}

func TestUpdateObjectNextCommand(t *testing.T) {
	repoRoot := t.TempDir()
	statusPath := filepath.Join(repoRoot, "docs/specs")
	if err := os.MkdirAll(statusPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|",
		"| `flow` | `flow_demo` | `no` | `yes` | `candidate` | `flow_verify` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(statusPath, "_status.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	updated, err := UpdateObjectNextCommand(repoRoot, "flow", "flow_demo", "flow_check")
	if err != nil {
		t.Fatalf("UpdateObjectNextCommand: %v", err)
	}
	if !updated {
		t.Fatalf("expected update to be true")
	}

	data, err := os.ReadFile(filepath.Join(statusPath, "_status.md"))
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(string(data), "| `flow` | `flow_demo` | `no` | `yes` | `candidate` | `flow_check` | note |") {
		t.Fatalf("updated status row not found:\n%s", string(data))
	}
}

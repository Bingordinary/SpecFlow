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
		"| `module_ai` | `yes` | `yes` | `candidate` | `cand_check` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(statusPath, "_status.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	updated, err := UpdateNextCommand(repoRoot, "module_ai", "cand_plan")
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
	if !strings.Contains(string(data), "| `module_ai` | `yes` | `yes` | `candidate` | `cand_plan` | note |") {
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
		"| `module_ai` | `yes` | `no` | `stable` | `spec_fork` | stable note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(statusPath, "_status.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	updated, err := UpsertModuleStatus(repoRoot, ModuleStatus{
		Module:      "module_new",
		Stable:      "no",
		Candidate:   "yes",
		ActiveLayer: "candidate",
		NextCommand: "cand_check",
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
	if !strings.Contains(string(data), "| `module_new` | `no` | `yes` | `candidate` | `cand_check` | new note |") {
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
		"| `module_ai` | `yes` | `no` | `stable` | `spec_fork` | stable note |",
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

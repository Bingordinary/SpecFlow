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

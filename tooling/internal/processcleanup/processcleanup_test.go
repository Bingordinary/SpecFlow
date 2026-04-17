package processcleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyFallbackForPromoteEvidenceIncomplete(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/specs/_verify_result"), 0o755); err != nil {
		t.Fatalf("mkdir verify_result: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}

	status := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `yes` | `candidate` | `cand_promote` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/specs/_status.md"), []byte(status), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}
	verifyPath := filepath.Join(repoRoot, "docs/specs/_verify_result/module_ai.md")
	if err := os.WriteFile(verifyPath, []byte("verify"), 0o644); err != nil {
		t.Fatalf("write verify file: %v", err)
	}

	result, err := ApplyFallback(repoRoot, "module_ai", "cand_promote", "evidence_incomplete")
	if err != nil {
		t.Fatalf("ApplyFallback: %v", err)
	}
	if result.NextCommand != "cand_verify" {
		t.Fatalf("expected next command cand_verify, got %s", result.NextCommand)
	}
	if len(result.DeletedFiles) != 1 || result.DeletedFiles[0] != "docs/specs/_verify_result/module_ai.md" {
		t.Fatalf("unexpected deleted files: %v", result.DeletedFiles)
	}
	if _, err := os.Stat(verifyPath); !os.IsNotExist(err) {
		t.Fatalf("expected verify file to be deleted, stat err=%v", err)
	}
}

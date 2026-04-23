package processcleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
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
		"| `module_ai` | `yes` | `yes` | `candidate` | `module_promote` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/specs/_status.md"), []byte(status), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}
	verifyPath := filepath.Join(repoRoot, "docs/specs/_verify_result/module_ai.md")
	if err := os.WriteFile(verifyPath, []byte("verify"), 0o644); err != nil {
		t.Fatalf("write verify file: %v", err)
	}

	result, err := ApplyFallback(repoRoot, "module_ai", "module_promote", "evidence_incomplete")
	if err != nil {
		t.Fatalf("ApplyFallback: %v", err)
	}
	if result.NextCommand != "module_verify" {
		t.Fatalf("expected next command module_verify, got %s", result.NextCommand)
	}
	if len(result.DeletedFiles) != 1 || result.DeletedFiles[0] != "docs/specs/_verify_result/module_ai.md" {
		t.Fatalf("unexpected deleted files: %v", result.DeletedFiles)
	}
	if _, err := os.Stat(verifyPath); !os.IsNotExist(err) {
		t.Fatalf("expected verify file to be deleted, stat err=%v", err)
	}
}

func TestApplyFallbackForVerifyImplementationDeviation(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_plans/active"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	status := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `yes` | `candidate` | `module_verify` | note |",
	}, "\n") + "\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/module_ai.md"), "check")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_plans/active/module_ai.md"), "plan")
	verifyPath := filepath.Join(repoRoot, "docs/specs/_verify_result/module_ai.md")
	mustWriteFile(t, verifyPath, "verify")

	result, err := ApplyFallback(repoRoot, "module_ai", "module_verify", "implementation_deviation")
	if err != nil {
		t.Fatalf("ApplyFallback: %v", err)
	}
	if result.NextCommand != "module_impl" {
		t.Fatalf("expected next command module_impl, got %s", result.NextCommand)
	}
	if len(result.DeletedFiles) != 1 || result.DeletedFiles[0] != "docs/specs/_verify_result/module_ai.md" {
		t.Fatalf("unexpected deleted files: %v", result.DeletedFiles)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/_check_result/module_ai.md")); err != nil {
		t.Fatalf("expected check file to remain, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/_plans/active/module_ai.md")); err != nil {
		t.Fatalf("expected plan file to remain, stat err=%v", err)
	}
	if _, err := os.Stat(verifyPath); !os.IsNotExist(err) {
		t.Fatalf("expected verify file to be deleted, stat err=%v", err)
	}
}

func TestApplyFallbackForVerifyTruthIncomplete(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_plans/active"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_plans/draft"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	status := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `yes` | `candidate` | `module_verify` | note |",
	}, "\n") + "\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)
	for _, relPath := range []string{
		"docs/specs/_check_result/module_ai.md",
		"docs/specs/_plans/active/module_ai.md",
		"docs/specs/_plans/draft/module_ai.md",
		"docs/specs/_verify_result/module_ai.md",
	} {
		mustWriteFile(t, filepath.Join(repoRoot, relPath), relPath)
	}

	result, err := ApplyFallback(repoRoot, "module_ai", "module_verify", "truth_incomplete")
	if err != nil {
		t.Fatalf("ApplyFallback: %v", err)
	}
	if result.NextCommand != "module_check" {
		t.Fatalf("expected next command module_check, got %s", result.NextCommand)
	}
	if len(result.DeletedFiles) != 4 {
		t.Fatalf("expected 4 deleted files, got %d: %v", len(result.DeletedFiles), result.DeletedFiles)
	}
	for _, relPath := range []string{
		"docs/specs/_check_result/module_ai.md",
		"docs/specs/_plans/active/module_ai.md",
		"docs/specs/_plans/draft/module_ai.md",
		"docs/specs/_verify_result/module_ai.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, relPath)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be deleted, stat err=%v", relPath, err)
		}
	}
}

func TestApplySuccessCleanupForPromote(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir)))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_check_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_plans/active"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_plans/draft"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/_verify_result"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs"))

	status := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `no` | `stable` | `module_fork` | promoted |",
	}, "\n") + "\n"
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_status.md"), status)
	candidateMainRef, err := specpaths.MainSpecFileRef("candidate", "module_ai")
	if err != nil {
		t.Fatalf("MainSpecFileRef: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(candidateMainRef)), "candidate")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixDir), "c_module_ai_prompt.md"), "appendix")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_check_result/module_ai.md"), "check")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_plans/active/module_ai.md"), "plan")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_plans/draft/module_ai.md"), "draft plan")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/_verify_result/module_ai.md"), "verify")

	result, err := ApplySuccessCleanup(repoRoot, "module_ai", "module_promote")
	if err != nil {
		t.Fatalf("ApplySuccessCleanup: %v", err)
	}
	if len(result.DeletedFiles) != 6 {
		t.Fatalf("expected 6 deleted files, got %d: %v", len(result.DeletedFiles), result.DeletedFiles)
	}
	for _, relPath := range []string{
		candidateMainRef,
		specpaths.CandidateAppendixDir + "/c_module_ai_prompt.md",
		"docs/specs/_check_result/module_ai.md",
		"docs/specs/_plans/active/module_ai.md",
		"docs/specs/_plans/draft/module_ai.md",
		"docs/specs/_verify_result/module_ai.md",
	} {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relPath))); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be deleted, stat err=%v", relPath, err)
		}
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

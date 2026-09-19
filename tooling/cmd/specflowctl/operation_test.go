package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ------------------------------------------------------------
// Fixtures
// ------------------------------------------------------------

func opRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// opTestGitRepo creates a temp git repository with a committed candidate unit
// spec and returns the repo root and the spec's repo-relative path.
func opTestGitRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	opRunGit(t, dir, "init", "-q")
	opRunGit(t, dir, "config", "user.email", "test@example.com")
	opRunGit(t, dir, "config", "user.name", "Test")

	specRel := "docs/specs/units/candidate/unit_demo.md"
	specPath := filepath.Join(dir, filepath.FromSlash(specRel))
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: internal/demo\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	opWriteFile(t, dir, "tooling/go.mod", "module example/source\n")
	opRunGit(t, dir, "add", "-A")
	opRunGit(t, dir, "commit", "-q", "-m", "fixture")
	return dir, specRel
}

func opWriteFile(t *testing.T, repoRoot, rel, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// opRun executes one operation subcommand and returns stdout, stderr, error.
func opRun(args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	err := runOperation(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func opParseID(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Operation opened: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Operation opened: "))
		}
	}
	t.Fatalf("operation id not found in output:\n%s", stdout)
	return ""
}

// ------------------------------------------------------------
// End-to-end
// ------------------------------------------------------------

func TestOperationCLIEndToEnd(t *testing.T) {
	repoRoot, specRel := opTestGitRepo(t)

	stdout, stderr, err := opRun("open", "--unit", "demo",
		"--require-spec", specRel, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("operation open failed: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Allowed scope") || !strings.Contains(stdout, "internal/demo") {
		t.Fatalf("expected the frozen scope in the open output:\n%s", stdout)
	}
	opID := opParseID(t, stdout)

	// In-scope changes (the required spec plus the implementation): check passes.
	opWriteFile(t, repoRoot, specRel, "# demo\n\nUpdated design.\n")
	opWriteFile(t, repoRoot, "internal/demo/new.go", "package demo\n")
	stdout, stderr, err = opRun("check", "--id", opID, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("check on in-scope change failed: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "Result: PASS") {
		t.Fatalf("expected PASS, got:\n%s", stdout)
	}

	// Out-of-scope change: check fails closed and close refuses.
	opWriteFile(t, repoRoot, "core/runtime/x.go", "package runtime\n")
	stdout, _, err = opRun("check", "--id", opID, "--repo-root", repoRoot)
	if err == nil {
		t.Fatalf("expected check failure for the out-of-scope change:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Result: FAIL") || !strings.Contains(stdout, "core/runtime/x.go") {
		t.Fatalf("expected FAIL listing the out-of-scope path, got:\n%s", stdout)
	}
	stdout, _, err = opRun("close", "--id", opID, "--repo-root", repoRoot)
	if err == nil {
		t.Fatalf("expected close refusal for the out-of-scope change:\n%s", stdout)
	}

	// Revert: check passes and close succeeds.
	if err := os.Remove(filepath.Join(repoRoot, "core/runtime/x.go")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err = opRun("close", "--id", opID, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("close failed: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "Operation closed (passed):") {
		t.Fatalf("expected close confirmation, got:\n%s", stdout)
	}

	stdout, _, err = opRun("status", "--id", opID, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !strings.Contains(stdout, "Status: closed") {
		t.Fatalf("expected closed status, got:\n%s", stdout)
	}
}

func TestOperationCLIAbandon(t *testing.T) {
	repoRoot, _ := opTestGitRepo(t)

	stdout, stderr, err := opRun("open", "--unit", "demo", "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("open failed: %v\nstderr=%s", err, stderr)
	}
	opID := opParseID(t, stdout)

	opWriteFile(t, repoRoot, "core/runtime/x.go", "package runtime\n")
	stdout, _, err = opRun("close", "--id", opID, "--repo-root", repoRoot)
	if err == nil {
		t.Fatalf("expected the normal close to refuse:\n%s", stdout)
	}

	stdout, stderr, err = opRun("close", "--id", opID, "--abandon", "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("abandon close failed: %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "Result: FAIL") || !strings.Contains(stdout, "Operation closed (abandoned):") {
		t.Fatalf("expected an abandoned close with the violation report, got:\n%s", stdout)
	}

	stdout, _, err = opRun("status", "--id", opID, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !strings.Contains(stdout, "Close outcome: abandoned") {
		t.Fatalf("expected the abandoned outcome in status, got:\n%s", stdout)
	}
}

func TestOperationCLIPathOnlyAndUpdate(t *testing.T) {
	repoRoot, _ := opTestGitRepo(t)

	stdout, stderr, err := opRun("open", "--allow", "scripts/eval", "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("open failed: %v\nstderr=%s", err, stderr)
	}
	opID := opParseID(t, stdout)
	if !strings.Contains(stdout, "Target: paths-only") {
		t.Fatalf("expected paths-only target, got:\n%s", stdout)
	}

	stdout, stderr, err = opRun("update", "--id", opID, "--allow", "scripts/other", "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("update failed: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "scripts/other") || !strings.Contains(stdout, "scripts/eval") {
		t.Fatalf("expected the declared scope to be widened, got:\n%s", stdout)
	}

	_, _, err = opRun("update", "--id", opID, "--repo-root", repoRoot)
	if err == nil {
		t.Fatalf("expected an error when updating without --allow/--require-spec")
	}

	stdout, _, err = opRun("status", "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !strings.Contains(stdout, opID) || !strings.Contains(stdout, "Open operations (1)") {
		t.Fatalf("expected the open operation in the list, got:\n%s", stdout)
	}

	stdout, _, err = opRun("status", "--id", opID, "--repo-root", repoRoot)
	if err != nil {
		t.Fatalf("status --id failed: %v", err)
	}
	if !strings.Contains(stdout, "Updates: 1") || !strings.Contains(stdout, "declared scope: scripts/eval, scripts/other") {
		t.Fatalf("expected the update history detail, got:\n%s", stdout)
	}

	opWriteFile(t, repoRoot, "scripts/other/x.sh", "echo ok\n")
	if _, _, err := opRun("check", "--id", opID, "--repo-root", repoRoot); err != nil {
		t.Fatalf("check on the updated scope failed: %v", err)
	}
}

func TestOperationCLIFailClosedInputs(t *testing.T) {
	repoRoot, _ := opTestGitRepo(t)

	// Usage surfaces.
	stdout, stderr, err := opRun()
	if err == nil || !strings.Contains(stderr, "Usage:") || stdout != "" {
		t.Fatalf("expected usage error, err=%v stderr=%s", err, stderr)
	}
	_, stderr, err = opRun("check", "--repo-root", repoRoot)
	if err == nil || !strings.Contains(stderr, "Usage:") {
		t.Fatalf("expected --id requirement, err=%v stderr=%s", err, stderr)
	}
	_, stderr, err = opRun("bogus")
	if err == nil || !strings.Contains(stderr, "Usage:") {
		t.Fatalf("expected unknown subcommand error, err=%v stderr=%s", err, stderr)
	}

	// Unknown id and a non-repository root fail closed.
	if _, _, err := opRun("check", "--id", "missing", "--repo-root", repoRoot); err == nil {
		t.Fatalf("expected not-found error for an unknown operation")
	}
	tmp := t.TempDir()
	if _, _, err := opRun("open", "--allow", "scripts/x", "--repo-root", tmp); err == nil {
		t.Fatalf("expected git worktree error for a non-repository root")
	}

	// Denied zone declaration.
	_, _, err = opRun("open", "--allow", "framework/concepts.md", "--repo-root", repoRoot)
	if err == nil || !strings.Contains(fmt.Sprint(err), "denied write zone") {
		t.Fatalf("expected denied write zone error, got %v", err)
	}
}

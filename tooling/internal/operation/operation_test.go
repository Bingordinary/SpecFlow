package operation

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
)

// ------------------------------------------------------------
// Fixtures and helpers
// ------------------------------------------------------------

func testNow() time.Time {
	return time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// newGitRepo creates a temp git repository with one initial commit.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitTest(t, dir, "init", "-q")
	runGitTest(t, dir, "config", "user.email", "test@example.com")
	runGitTest(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "README.md", "init\n")
	writeFile(t, dir, "tooling/go.mod", "module example/source\n")
	commitAll(t, dir, "init")
	return dir
}

// newGitRepoWith creates a repo whose baseline commit contains the given
// tracked files.
func newGitRepoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	runGitTest(t, dir, "init", "-q")
	runGitTest(t, dir, "config", "user.email", "test@example.com")
	runGitTest(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "tooling/go.mod", "module example/source\n")
	for rel, content := range files {
		writeFile(t, dir, rel, content)
	}
	commitAll(t, dir, "init")
	return dir
}

func newInstalledGitRepoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	runGitTest(t, dir, "init", "-q")
	runGitTest(t, dir, "config", "user.email", "test@example.com")
	runGitTest(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "specflow/tooling/go.mod", "module example/installed\n")
	for rel, content := range files {
		writeFile(t, dir, rel, content)
	}
	commitAll(t, dir, "init")
	return dir
}

func writeFile(t *testing.T, repoRoot, rel, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// unitSpecContent builds a minimal unit spec whose acceptance item declares
// the given implementation surface and optional affects.files entries.
func unitSpecContent(unit, implSurface string, affects []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# %s\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n", unit, unit)
	fmt.Fprintf(&b, "  - id: %s.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: %s\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n", unit, implSurface)
	if len(affects) > 0 {
		b.WriteString("    affects:\n      files:\n")
		for _, f := range affects {
			fmt.Fprintf(&b, "        - %s\n", f)
		}
	}
	return b.String()
}

// writeUnitSpec writes a candidate unit spec and returns its repo-relative
// path.
func writeUnitSpec(t *testing.T, repoRoot, unit, implSurface string, affects []string) string {
	t.Helper()
	rel := "docs/specs/units/candidate/unit_" + unit + ".md"
	writeFile(t, repoRoot, rel, unitSpecContent(unit, implSurface, affects))
	return rel
}

func commitAll(t *testing.T, repoRoot, msg string) {
	t.Helper()
	runGitTest(t, repoRoot, "add", "-A")
	runGitTest(t, repoRoot, "commit", "-q", "-m", msg)
}

func headSHA(t *testing.T, repoRoot string) string {
	t.Helper()
	return strings.TrimSpace(runGitTest(t, repoRoot, "rev-parse", "HEAD"))
}

func mustOpen(t *testing.T, repoRoot string, opts OpenOptions) *OpenResult {
	t.Helper()
	result, err := Open(repoRoot, opts, testNow())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return result
}

func allowedEntry(t *testing.T, op *Operation, path string) AllowedPath {
	t.Helper()
	for _, entry := range op.AllowedPaths {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("allowed path %q not found in %v", path, op.AllowedPaths)
	return AllowedPath{}
}

func containsAllowedPath(entries []AllowedPath, path string) bool {
	for _, entry := range entries {
		if entry.Path == path {
			return true
		}
	}
	return false
}

func mustCheck(t *testing.T, repoRoot, opID string) *Report {
	t.Helper()
	report, err := Check(repoRoot, opID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return report
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------
// Open
// ------------------------------------------------------------

func TestOpenDerivesUnitScope(t *testing.T) {
	repoRoot := newGitRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "demo", "internal/demo", []string{"docs/design.md"})
	writeFile(t, repoRoot, "docs/specs/units/candidate/appendix/unit_demo_flows.md", "---\nstatus: active\n---\n# Flows\n")
	commitAll(t, repoRoot, "fixture")

	result := mustOpen(t, repoRoot, OpenOptions{
		Unit:        "demo",
		RequireSpec: []string{specPath},
	})
	op := result.Operation

	if op.Target.Kind != TargetKindUnit || op.Target.Name != "demo" {
		t.Fatalf("target = %+v, want unit demo", op.Target)
	}
	if op.Baseline.SHA != headSHA(t, repoRoot) {
		t.Fatalf("baseline sha = %q, want HEAD", op.Baseline.SHA)
	}
	if len(op.Baseline.SHA) != 40 {
		t.Fatalf("baseline sha %q is not a full commit sha", op.Baseline.SHA)
	}
	if op.Baseline.Ref != "HEAD" {
		t.Fatalf("baseline ref = %q, want HEAD", op.Baseline.Ref)
	}

	if entry := allowedEntry(t, op, specPath); entry.Source != SourceSpecFile {
		t.Fatalf("spec source = %q, want %q", entry.Source, SourceSpecFile)
	}
	if entry := allowedEntry(t, op, "docs/specs/units/candidate/appendix/unit_demo_flows.md"); entry.Source != SourceSpecFile {
		t.Fatalf("appendix source = %q, want %q", entry.Source, SourceSpecFile)
	}
	if entry := allowedEntry(t, op, "internal/demo"); entry.Source != SourceImplSurface {
		t.Fatalf("surface source = %q, want %q", entry.Source, SourceImplSurface)
	}
	if entry := allowedEntry(t, op, "docs/design.md"); entry.Source != SourceAffectsFiles {
		t.Fatalf("affects source = %q, want %q", entry.Source, SourceAffectsFiles)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(StateRelPath(op.OperationID)))); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}

func TestOpenIgnoresFencedAcceptanceSetWhenDerivingScope(t *testing.T) {
	repoRoot := newGitRepo(t)
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# demo\n\n~~~yaml\nacceptance_item_set:\n  - id: fake.item\n    implementation_surface: fake\n    affects:\n      files:\n        - fake/file.go\n~~~\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    implementation_surface: internal/demo\n    affects:\n      files:\n        - docs/design.md\n"
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_demo.md", spec)
	commitAll(t, repoRoot, "fixture")

	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	allowedEntry(t, op, "internal/demo")
	allowedEntry(t, op, "docs/design.md")
	for _, unexpected := range []string{"fake", "fake/file.go"} {
		if containsAllowedPath(op.AllowedPaths, unexpected) {
			t.Fatalf("fenced example path %q leaked into operation scope: %+v", unexpected, op.AllowedPaths)
		}
	}
}

func TestOpenStableOnlyTargetScopesCandidateForkPaths(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeFile(t, repoRoot, "docs/specs/units/stable/unit_demo.md", unitSpecContent("demo", "internal/demo", nil))
	writeFile(t, repoRoot, "docs/specs/units/stable/appendix/unit_demo_flows.md", "---\nstatus: active\n---\n# Flows\n")
	commitAll(t, repoRoot, "fixture")

	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	allowedEntry(t, op, "docs/specs/units/candidate/unit_demo.md")
	allowedEntry(t, op, "docs/specs/units/candidate/appendix/unit_demo_flows.md")
	allowedEntry(t, op, "internal/demo")
	for _, entry := range op.AllowedPaths {
		if strings.Contains(entry.Path, "/stable/") {
			t.Fatalf("stable files are never writable and must not be scope entries: %v", op.AllowedPaths)
		}
	}

	// The standard flow: fork (candidate files appear), edit the candidate
	// spec, change code — everything is already in scope.
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_demo.md", "# demo\n\nForked.\n")
	writeFile(t, repoRoot, "docs/specs/units/candidate/appendix/unit_demo_flows.md", "# Flows\n")
	writeFile(t, repoRoot, "internal/demo/new.go", "package demo\n")
	report := mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "PASS" {
		t.Fatalf("result = %s, want PASS (out of scope: %v)", report.Result, report.OutOfScope)
	}
}

func TestOpenRuleTarget(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeFile(t, repoRoot, "docs/specs/rules/candidate/b_rule_x.md", "---\nrule_id: b_rule_x\n---\n")
	commitAll(t, repoRoot, "fixture")

	result := mustOpen(t, repoRoot, OpenOptions{Rule: "b_rule_x"})
	if result.Operation.Target.Kind != TargetKindRule {
		t.Fatalf("target kind = %q, want rule", result.Operation.Target.Kind)
	}
	if entry := allowedEntry(t, result.Operation, "docs/specs/rules/candidate/b_rule_x.md"); entry.Source != SourceSpecFile {
		t.Fatalf("rule source = %q, want %q", entry.Source, SourceSpecFile)
	}
}

func TestOpenValidatesTargetNameBeforeWorkTreeCheck(t *testing.T) {
	dir := t.TempDir() // not a git worktree
	_, err := Open(dir, OpenOptions{Unit: "../../escape"}, testNow())
	if err == nil || !strings.Contains(err.Error(), "is invalid") {
		t.Fatalf("an invalid target name must fail before the git worktree check, got %v", err)
	}
}

func TestOpenRejectsInvalidDeclarations(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")

	cases := []struct {
		name    string
		opts    OpenOptions
		wantErr string
	}{
		{
			name:    "path-only without allow",
			opts:    OpenOptions{},
			wantErr: "requires at least one --allow path",
		},
		{
			name:    "unit and rule together",
			opts:    OpenOptions{Unit: "demo", Rule: "b_rule_x"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "traversal unit name",
			opts:    OpenOptions{Unit: "../../escape"},
			wantErr: "is invalid",
		},
		{
			name:    "invalid rule name",
			opts:    OpenOptions{Rule: "a/b"},
			wantErr: "is invalid",
		},
		{
			name:    "unknown unit",
			opts:    OpenOptions{Unit: "missing"},
			wantErr: "no spec in either layer",
		},
		{
			name:    "unknown rule",
			opts:    OpenOptions{Rule: "b_rule_missing"},
			wantErr: "no file in either layer",
		},
		{
			name:    "declared framework path",
			opts:    OpenOptions{Unit: "demo", Allow: []string{"framework/concepts.md"}},
			wantErr: "denied write zone",
		},
		{
			name:    "declared framework zone root",
			opts:    OpenOptions{Unit: "demo", Allow: []string{"framework"}},
			wantErr: "denied write zone",
		},
		{
			name:    "declared stable spec path",
			opts:    OpenOptions{Unit: "demo", Allow: []string{"docs/specs/units/stable/unit_demo.md"}},
			wantErr: "denied write zone",
		},
		{
			name:    "declared stable spec zone root",
			opts:    OpenOptions{Unit: "demo", Allow: []string{"docs/specs/units/stable"}},
			wantErr: "denied write zone",
		},
		{
			name:    "declared local state path",
			opts:    OpenOptions{Unit: "demo", Allow: []string{"meta/anything"}},
			wantErr: "local state or derived-cache area",
		},
		{
			name:    "declared derived cache path",
			opts:    OpenOptions{Unit: "demo", Allow: []string{"docs/specs/meta/validation/x.md"}},
			wantErr: "local state or derived-cache area",
		},
		{
			name:    "declared outside path",
			opts:    OpenOptions{Unit: "demo", Allow: []string{"../outside"}},
			wantErr: "outside the repository root",
		},
		{
			name:    "required spec in denied zone",
			opts:    OpenOptions{Unit: "demo", RequireSpec: []string{"docs/specs/units/stable/unit_demo.md"}},
			wantErr: "denied write zone",
		},
		{
			name:    "required spec is ordinary code",
			opts:    OpenOptions{Unit: "demo", RequireSpec: []string{"internal/demo/service.go"}},
			wantErr: "not a candidate unit spec, unit appendix, or rule spec",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Open(repoRoot, tc.opts, testNow())
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestOpenRejectsDeclaredPathThroughExternalSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repoRoot, "external")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	commitAll(t, repoRoot, "fixture")

	_, err := Open(repoRoot, OpenOptions{Allow: []string{"external/missing/new.txt"}}, testNow())
	if err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected external symlink declaration to be rejected, got %v", err)
	}
}

func TestOpenRejectsDeclaredPathThroughDanglingSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	outside := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "missing-target"), filepath.Join(repoRoot, "dangling")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	commitAll(t, repoRoot, "fixture")

	_, err := Open(repoRoot, OpenOptions{Allow: []string{"dangling/new.txt"}}, testNow())
	if err == nil || !strings.Contains(err.Error(), "unresolvable symlink") {
		t.Fatalf("expected a dangling symlink declaration to be rejected, got %v", err)
	}
}

func TestOpenAcceptsDeclaredPathThroughInternalSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeFile(t, repoRoot, "real/keep.txt", "keep\n")
	if err := os.Symlink(filepath.Join(repoRoot, "real"), filepath.Join(repoRoot, "alias")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	commitAll(t, repoRoot, "fixture")

	result, err := Open(repoRoot, OpenOptions{Allow: []string{"alias/new.txt"}}, testNow())
	if err != nil {
		t.Fatalf("expected internal symlink scope to be accepted: %v", err)
	}
	if entry := allowedEntry(t, result.Operation, "alias/new.txt"); entry.Source != SourceDeclared {
		t.Fatalf("internal symlink scope source = %q, want %q", entry.Source, SourceDeclared)
	}
}

func TestOpenRejectsSpecDerivedPathThroughExternalSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	outside := t.TempDir()
	writeUnitSpec(t, repoRoot, "demo", "external/missing", nil)
	if err := os.Symlink(outside, filepath.Join(repoRoot, "external")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	commitAll(t, repoRoot, "fixture")

	_, err := Open(repoRoot, OpenOptions{Unit: "demo"}, testNow())
	if err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected spec-derived external symlink to be rejected, got %v", err)
	}
}

func TestOpenRejectsTargetSpecThroughExternalSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	outside := t.TempDir()
	externalSpec := filepath.Join(outside, "unit_demo.md")
	if err := os.WriteFile(externalSpec, []byte("---\nid: demo\n---\n\n# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(candidateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalSpec, filepath.Join(candidateDir, "unit_demo.md")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	commitAll(t, repoRoot, "fixture")

	_, err := Open(repoRoot, OpenOptions{Unit: "demo"}, testNow())
	if err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected symlinked target spec to be rejected, got %v", err)
	}
}

func TestOpenRejectsRequiredSpecThroughExternalParentSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeFile(t, repoRoot, "src/keep.go", "package keep\n")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "unit_demo.md"), []byte("# outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unitsDir := filepath.Join(repoRoot, "docs/specs/units")
	if err := os.MkdirAll(unitsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(unitsDir, "candidate")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	commitAll(t, repoRoot, "fixture")

	_, err := Open(repoRoot, OpenOptions{Allow: []string{"src"}, RequireSpec: []string{"docs/specs/units/candidate/unit_demo.md"}}, testNow())
	if err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected required spec through external parent symlink to be rejected, got %v", err)
	}
}

func TestLoadRejectsAllowedPathReplacedByExternalSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeFile(t, repoRoot, "scope/keep.txt", "keep\n")
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Allow: []string{"scope"}}).Operation

	if err := os.RemoveAll(filepath.Join(repoRoot, "scope")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(repoRoot, "scope")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := Load(repoRoot, op.OperationID); err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected persisted scope to fail after symlink escape, got %v", err)
	}
}

func TestLoadRejectsSpecDerivedPathReplacedByExternalSymlink(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	writeFile(t, repoRoot, "internal/demo/keep.go", "package demo\n")
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	if err := os.RemoveAll(filepath.Join(repoRoot, "internal/demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(repoRoot, "internal/demo")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := Load(repoRoot, op.OperationID); err == nil || !strings.Contains(err.Error(), "resolves outside the repository root") {
		t.Fatalf("expected persisted spec-derived scope to fail after symlink escape, got %v", err)
	}
}

func TestOpenRejectsUnknownParent(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")

	_, err := Open(repoRoot, OpenOptions{Unit: "demo", Parent: "nope"}, testNow())
	if err == nil || !strings.Contains(err.Error(), "parent operation") {
		t.Fatalf("expected parent operation error, got %v", err)
	}
}

func TestOpenRecordsParent(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")

	parent := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"})
	child := mustOpen(t, repoRoot, OpenOptions{Unit: "demo", Allow: []string{"scripts/eval"}, Parent: parent.Operation.OperationID})
	if child.Operation.ParentOperation != parent.Operation.OperationID {
		t.Fatalf("parent = %q, want %q", child.Operation.ParentOperation, parent.Operation.OperationID)
	}
}

func TestOpenNoticesPreExistingOutOfScopeChanges(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	writeFile(t, repoRoot, "core/runtime/x.go", "package runtime\n")

	result := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"})
	if len(result.Notices) == 0 {
		t.Fatalf("expected an open-time notice for the pre-existing out-of-scope change")
	}
}

// ------------------------------------------------------------
// Check
// ------------------------------------------------------------

func TestCheckContainmentAndPrefixBoundary(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	writeFile(t, repoRoot, "internal/demo/new.go", "package demo\n")
	writeFile(t, repoRoot, "internal/demo_old/legacy.go", "package legacy\n")
	writeFile(t, repoRoot, "core/runtime/x.go", "package runtime\n")

	report := mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "FAIL" {
		t.Fatalf("result = %s, want FAIL", report.Result)
	}
	if !contains(report.OutOfScope, "core/runtime/x.go") {
		t.Fatalf("out of scope = %v, want core/runtime/x.go", report.OutOfScope)
	}
	if !contains(report.OutOfScope, "internal/demo_old/legacy.go") {
		t.Fatalf("out of scope = %v, want internal/demo_old/legacy.go (prefix boundary)", report.OutOfScope)
	}
	if contains(report.OutOfScope, "internal/demo/new.go") {
		t.Fatalf("internal/demo/new.go must be in scope (new file under the surface), out of scope = %v", report.OutOfScope)
	}

	// The conservative shared-working-tree behavior: every out-of-scope path
	// is reported without any attribution guessing.
	if len(report.OutOfScope) != 2 {
		t.Fatalf("out of scope = %v, want exactly the two violating paths", report.OutOfScope)
	}

	os.Remove(filepath.Join(repoRoot, "core/runtime/x.go"))
	os.RemoveAll(filepath.Join(repoRoot, "internal/demo_old"))
	report = mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "PASS" {
		t.Fatalf("result = %s, want PASS (out of scope: %v)", report.Result, report.OutOfScope)
	}
}

func TestCheckStaticPolicyViolations(t *testing.T) {
	repoRoot := newGitRepoWith(t, map[string]string{
		"framework/concepts.md":                   "# concepts\n",
		"docs/specs/units/stable/unit_other.md":   "stable\n",
		"docs/specs/units/candidate/unit_demo.md": "spec\n",
	})
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	writeFile(t, repoRoot, "framework/concepts.md", "# changed\n")
	writeFile(t, repoRoot, "docs/specs/units/stable/unit_other.md", "changed\n")
	writeFile(t, repoRoot, "src/app.go", "package app\n")

	report := mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "FAIL" {
		t.Fatalf("result = %s, want FAIL", report.Result)
	}
	if !contains(report.StaticViolations, "framework/concepts.md") {
		t.Fatalf("static violations = %v, want framework/concepts.md", report.StaticViolations)
	}
	if !contains(report.StaticViolations, "docs/specs/units/stable/unit_other.md") {
		t.Fatalf("static violations = %v, want the stable spec path", report.StaticViolations)
	}
	if contains(report.OutOfScope, "framework/concepts.md") {
		t.Fatalf("static violations must not be double-reported as out of scope: %v", report.OutOfScope)
	}
}

func TestCheckFailsClosedOnSymlinkResolvedStaticPolicyViolation(t *testing.T) {
	repoRoot := newGitRepoWith(t, map[string]string{
		"docs/specs/units/stable/unit_real.md": "stable\n",
	})
	linkPath := "docs/specs/units/candidate/unit_link.md"
	op := mustOpen(t, repoRoot, OpenOptions{Allow: []string{linkPath}}).Operation
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repoRoot, linkPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repoRoot, "docs/specs/units/stable/unit_real.md"), filepath.Join(repoRoot, linkPath)); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	if _, err := Check(repoRoot, op.OperationID); err == nil || !strings.Contains(err.Error(), "denied write zone") {
		t.Fatalf("resolved stable target must invalidate the persisted scope, got %v", err)
	}
}

func TestInstalledProjectFrameworkIsDeniedAtDeclarationAndCheck(t *testing.T) {
	repoRoot := newInstalledGitRepoWith(t, map[string]string{
		"docs/specs/units/candidate/unit_demo.md": unitSpecContent("demo", "src/demo.go", nil),
		"specflow/framework/concepts.md":          "# concepts\n",
	})
	if _, err := Open(repoRoot, OpenOptions{Unit: "demo", Allow: []string{"specflow/framework/concepts.md"}}, testNow()); err == nil || !strings.Contains(err.Error(), "denied write zone") {
		t.Fatalf("installed framework declaration must be rejected, got %v", err)
	}

	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	writeFile(t, repoRoot, "specflow/framework/concepts.md", "# changed\n")
	report := mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "FAIL" || !contains(report.StaticViolations, "specflow/framework/concepts.md") {
		t.Fatalf("installed framework change must be a static violation: %+v", report)
	}
}

func TestCheckRequiredSpecPaths(t *testing.T) {
	repoRoot := newGitRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{
		Unit:        "demo",
		RequireSpec: []string{specPath},
	}).Operation

	report := mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "FAIL" || !contains(report.MissingRequired, specPath) {
		t.Fatalf("missing required = %v, result = %s; want the untouched spec reported", report.MissingRequired, report.Result)
	}

	writeFile(t, repoRoot, specPath, "updated spec\n")
	report = mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "PASS" {
		t.Fatalf("result = %s, want PASS (missing required: %v)", report.Result, report.MissingRequired)
	}

	// A deletion appears in the change set but leaves no spec behind: it must
	// not satisfy the spec-first obligation.
	if err := os.Remove(filepath.Join(repoRoot, filepath.FromSlash(specPath))); err != nil {
		t.Fatal(err)
	}
	report = mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "FAIL" || !contains(report.MissingRequired, specPath) {
		t.Fatalf("a deleted required spec must fail the check, got result %s / missing %v", report.Result, report.MissingRequired)
	}
}

func TestCheckExcludesLocalStateAndDerivedCaches(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	writeFile(t, repoRoot, "docs/specs/meta/validation/unit/demo/validate_result.md", "cache\n")

	report := mustCheck(t, repoRoot, op.OperationID)
	for _, p := range report.Changed {
		if strings.HasPrefix(p, "meta/") || strings.HasPrefix(p, "docs/specs/meta/") {
			t.Fatalf("changed set must exclude local state and derived caches, got %q", p)
		}
	}
	if report.Result != "PASS" {
		t.Fatalf("result = %s, want PASS (changed: %v)", report.Result, report.Changed)
	}
}

func TestCheckSeesCommittedChangesAndRenames(t *testing.T) {
	repoRoot := newGitRepoWith(t, map[string]string{
		"docs/specs/units/candidate/unit_demo.md": "spec\n",
		"internal/demo/old.go":                    "package demo\n",
		"core/a.go":                               "package core\n",
	})
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	writeFile(t, repoRoot, "internal/demo/new.go", "package demo\n")
	commitAll(t, repoRoot, "in-scope progress")

	runGitTest(t, repoRoot, "mv", "core/a.go", "core/b.go")

	report := mustCheck(t, repoRoot, op.OperationID)
	if !contains(report.Changed, "internal/demo/new.go") {
		t.Fatalf("changed = %v, want committed in-scope change", report.Changed)
	}
	if report.Result != "FAIL" {
		t.Fatalf("result = %s, want FAIL (renamed out-of-scope file)", report.Result)
	}
	if !contains(report.OutOfScope, "core/a.go") || !contains(report.OutOfScope, "core/b.go") {
		t.Fatalf("out of scope = %v, want both sides of the rename", report.OutOfScope)
	}
}

func TestCheckMissingBaselineFailsClosed(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	op.Baseline.SHA = strings.Repeat("f", 40)
	if err := Save(repoRoot, op); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(repoRoot, op.OperationID); err == nil {
		t.Fatalf("expected a fail-closed error for the missing baseline commit")
	}
}

func TestCheckUnknownOperation(t *testing.T) {
	repoRoot := newGitRepo(t)
	if _, err := Check(repoRoot, "20260101-000000-ffffff"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestLoadRejectsInvalidAndMismatchedOperationIDs(t *testing.T) {
	repoRoot := newGitRepo(t)
	if _, err := Load(repoRoot, "../../outside"); err == nil || !strings.Contains(err.Error(), "invalid local-state id") {
		t.Fatalf("expected traversal-shaped id to be rejected, got %v", err)
	}

	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	path, err := statePath(repoRoot, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored Operation
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	stored.OperationID = "20260101-000000-aaaaaa"
	data, err = json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(repoRoot, op.OperationID); err == nil || !strings.Contains(err.Error(), "does not match requested id") {
		t.Fatalf("expected embedded operation id mismatch to be rejected, got %v", err)
	}
}

func TestLoadRejectsSemanticallyMalformedState(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	stateFile, err := statePath(repoRoot, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "unknown top-level field", mutate: func(state map[string]any) { state["surprise"] = true }},
		{name: "unknown status", mutate: func(state map[string]any) { state["status"] = "pending" }},
		{name: "unknown target kind", mutate: func(state map[string]any) {
			state["target"].(map[string]any)["kind"] = "nonsense"
		}},
		{name: "invalid allowed source", mutate: func(state map[string]any) {
			state["allowed_paths"].([]any)[0].(map[string]any)["source"] = "nonsense"
		}},
		{name: "non-canonical allowed path", mutate: func(state map[string]any) {
			state["allowed_paths"].([]any)[0].(map[string]any)["path"] = "src/../src/a.go"
		}},
		{name: "nested spec:file appendix path", want: "does not match target", mutate: func(state map[string]any) {
			// A spec:file entry must be the target's own spec file: a nested
			// path under the appendix directory is not the target's spec
			// surface and fails closed at load time.
			for _, raw := range state["allowed_paths"].([]any) {
				entry := raw.(map[string]any)
				if entry["source"] != "spec:file" {
					continue
				}
				entry["path"] = "docs/specs/units/candidate/appendix/unit_demo_sub/deep/x.md"
				return
			}
		}},
		{name: "ordinary required spec path", mutate: func(state map[string]any) {
			state["required_spec_paths"] = []any{"src/a.go"}
		}},
		{name: "open state with close fields", mutate: func(state map[string]any) {
			state["closed_at"] = "2026-09-16T12:00:00Z"
			state["close_outcome"] = "passed"
		}},
		{name: "malformed timestamp", mutate: func(state map[string]any) { state["updated_at"] = "yesterday" }},
		{name: "invalid parent id", mutate: func(state map[string]any) { state["parent_operation"] = "../parent" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var state map[string]any
			if err := json.Unmarshal(original, &state); err != nil {
				t.Fatal(err)
			}
			tc.mutate(state)
			data, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stateFile, append(data, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			want := tc.want
			if want == "" {
				want = "state is malformed"
			}
			if _, err := Load(repoRoot, op.OperationID); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("expected malformed state to be rejected with %q, got %v", want, err)
			}
		})
	}
}

func TestSaveRejectsInvalidState(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	op.AllowedPaths[0].Source = "nonsense"
	if err := Save(repoRoot, op); err == nil || !strings.Contains(err.Error(), "invalid source") {
		t.Fatalf("expected Save to reject invalid state, got %v", err)
	}
}

func TestCheckRejectsMalformedOperationState(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	stateFile, err := statePath(repoRoot, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	op.AllowedPaths[0].Source = "nonsense"
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	if report, err := Check(repoRoot, op.OperationID); err == nil {
		t.Fatalf("malformed operation state produced report %+v instead of failing closed", report)
	}
}

// ------------------------------------------------------------
// Update and close
// ------------------------------------------------------------

func TestUpdateWidensDeclaredScopeAndRequiredSpecsAndRecordsEvent(t *testing.T) {
	repoRoot := newGitRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{
		Unit:        "demo",
		Allow:       []string{"scripts/eval"},
		RequireSpec: []string{specPath},
	}).Operation
	additionalSpec := "docs/specs/rules/candidate/b_rule_extra.md"

	updated, err := Update(repoRoot, op.OperationID, UpdateOptions{
		Allow:          []string{"scripts/other", additionalSpec},
		HasAllow:       true,
		RequireSpec:    []string{additionalSpec},
		HasRequireSpec: true,
	}, testNow())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	allowedEntry(t, updated, "scripts/other")
	allowedEntry(t, updated, "scripts/eval")
	allowedEntry(t, updated, additionalSpec)
	allowedEntry(t, updated, "internal/demo")
	if entry := allowedEntry(t, updated, "docs/specs/units/candidate/unit_demo.md"); entry.Source != SourceSpecFile {
		t.Fatalf("spec-derived entry changed source: %q", entry.Source)
	}
	if got := strings.Join(updated.RequiredSpecPaths, ","); got != additionalSpec+","+specPath {
		t.Fatalf("required spec paths = %v, want both old and new paths", updated.RequiredSpecPaths)
	}
	if len(updated.Updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(updated.Updates))
	}
	event := updated.Updates[0]
	if got := strings.Join(event.DeclaredAllowedPaths, ","); got != additionalSpec+",scripts/eval,scripts/other" {
		t.Fatalf("event declared paths = %v, want the complete widened scope", event.DeclaredAllowedPaths)
	}
	if got := strings.Join(event.RequiredSpecPaths, ","); got != additionalSpec+","+specPath {
		t.Fatalf("event required spec paths = %v, want the complete widened requirement set", event.RequiredSpecPaths)
	}

	if _, err := Update(repoRoot, op.OperationID, UpdateOptions{}, testNow()); err == nil {
		t.Fatalf("expected an error when neither --allow nor --require-spec is present")
	}

	updated, err = Update(repoRoot, op.OperationID, UpdateOptions{
		Allow:          []string{"scripts/other", additionalSpec},
		HasAllow:       true,
		RequireSpec:    []string{additionalSpec},
		HasRequireSpec: true,
	}, testNow())
	if err != nil {
		t.Fatalf("idempotent Update: %v", err)
	}
	if got := strings.Join(declaredPaths(updated.AllowedPaths), ","); got != additionalSpec+",scripts/eval,scripts/other" {
		t.Fatalf("idempotent update changed declared paths: %v", declaredPaths(updated.AllowedPaths))
	}
	if got := strings.Join(updated.RequiredSpecPaths, ","); got != additionalSpec+","+specPath {
		t.Fatalf("idempotent update changed required spec paths: %v", updated.RequiredSpecPaths)
	}

	report, err := Check(repoRoot, op.OperationID)
	if err != nil {
		t.Fatalf("Check before required specs change: %v", err)
	}
	if report.Result != "FAIL" || !contains(report.MissingRequired, specPath) || !contains(report.MissingRequired, additionalSpec) {
		t.Fatalf("expected both old and new spec obligations to remain required, got %+v", report)
	}

	writeFile(t, repoRoot, specPath, "# demo\n\nUpdated design.\n")
	writeFile(t, repoRoot, additionalSpec, "# Rule\n")
	report, err = Check(repoRoot, op.OperationID)
	if err != nil {
		t.Fatalf("Check after required specs change: %v", err)
	}
	if report.Result != "PASS" {
		t.Fatalf("expected widened operation to pass after every required spec changed, got %+v", report)
	}
}

func TestLoadRejectsNonMonotonicUpdateHistory(t *testing.T) {
	repoRoot := newGitRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{
		Unit:        "demo",
		Allow:       []string{"scripts/one"},
		RequireSpec: []string{specPath},
	}).Operation

	firstTime := testNow().Add(time.Minute)
	updated, err := Update(repoRoot, op.OperationID, UpdateOptions{
		Allow:          []string{"scripts/two"},
		HasAllow:       true,
		RequireSpec:    []string{"docs/specs/rules/candidate/b_rule_extra.md"},
		HasRequireSpec: true,
	}, firstTime)
	if err != nil {
		t.Fatal(err)
	}
	updated.Updates = append(updated.Updates, UpdateEvent{
		At:                   firstTime.Add(time.Minute).UTC().Format(timestampLayout),
		DeclaredAllowedPaths: []string{"scripts/two"},
		RequiredSpecPaths:    []string{"docs/specs/rules/candidate/b_rule_extra.md"},
	})
	updated.UpdatedAt = updated.Updates[1].At
	updated.AllowedPaths = []AllowedPath{
		{Path: "docs/specs/units/candidate/unit_demo.md", Source: SourceSpecFile},
		{Path: "internal/demo", Source: SourceImplSurface},
		{Path: "scripts/two", Source: SourceDeclared},
	}
	updated.RequiredSpecPaths = append([]string(nil), updated.Updates[1].RequiredSpecPaths...)
	stateFile, err := statePath(repoRoot, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(repoRoot, op.OperationID); err == nil || !strings.Contains(err.Error(), "removes a path recorded by the previous update") {
		t.Fatalf("expected non-monotonic update history to fail closed, got %v", err)
	}
}

func TestUpdateRejectsOrdinaryCodeAsRequiredSpec(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	_, err := Update(repoRoot, op.OperationID, UpdateOptions{
		RequireSpec:    []string{"internal/demo/service.go"},
		HasRequireSpec: true,
	}, testNow())
	if err == nil || !strings.Contains(err.Error(), "not a candidate unit spec, unit appendix, or rule spec") {
		t.Fatalf("expected ordinary code to be rejected as --require-spec, got %v", err)
	}
}

func TestCloseFailClosedThenPass(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	writeFile(t, repoRoot, "core/runtime/x.go", "package runtime\n")
	report, err := Close(repoRoot, op.OperationID, false, testNow())
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if report.Result != "FAIL" {
		t.Fatalf("result = %s, want FAIL", report.Result)
	}
	reloaded, err := Load(repoRoot, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusOpen {
		t.Fatalf("status = %s, want open after a refused close", reloaded.Status)
	}

	os.Remove(filepath.Join(repoRoot, "core/runtime/x.go"))
	report, err = Close(repoRoot, op.OperationID, false, testNow())
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if report.Result != "PASS" {
		t.Fatalf("result = %s, want PASS", report.Result)
	}
	reloaded, err = Load(repoRoot, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusClosed || reloaded.ClosedAt == "" {
		t.Fatalf("status = %s closed_at = %q, want closed with timestamp", reloaded.Status, reloaded.ClosedAt)
	}
	if reloaded.CloseOutcome != CloseOutcomePassed {
		t.Fatalf("close outcome = %q, want %q", reloaded.CloseOutcome, CloseOutcomePassed)
	}

	if _, err := Update(repoRoot, op.OperationID, UpdateOptions{Allow: []string{"scripts/x"}, HasAllow: true}, testNow()); err == nil {
		t.Fatalf("expected an error when updating a closed operation")
	}
	if _, err := Close(repoRoot, op.OperationID, false, testNow()); err == nil {
		t.Fatalf("expected an error when closing an already closed operation")
	}
}

func TestCloseAbandonEndsViolatingOperation(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	writeFile(t, repoRoot, "core/runtime/x.go", "package runtime\n")
	report, err := Close(repoRoot, op.OperationID, true, testNow())
	if err != nil {
		t.Fatalf("Close(abandon): %v", err)
	}
	if report.Result != "FAIL" || !contains(report.OutOfScope, "core/runtime/x.go") {
		t.Fatalf("abandon must still surface the violation report: %+v", report)
	}

	reloaded, err := Load(repoRoot, op.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusClosed || reloaded.CloseOutcome != CloseOutcomeAbandoned {
		t.Fatalf("status = %s outcome = %q, want closed/abandoned", reloaded.Status, reloaded.CloseOutcome)
	}
}

func TestCheckOnClosedOperationStillEvaluates(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	if _, err := Close(repoRoot, op.OperationID, false, testNow()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	writeFile(t, repoRoot, "core/x.go", "package core\n")
	report := mustCheck(t, repoRoot, op.OperationID)
	if report.Result != "FAIL" {
		t.Fatalf("result = %s, want FAIL (read-only historical evaluation)", report.Result)
	}
}

// ------------------------------------------------------------
// Concurrency boundary (worktrees)
// ------------------------------------------------------------

func TestWorktreeIsolation(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")

	worktree := filepath.Join(t.TempDir(), "wt")
	runGitTest(t, repoRoot, "worktree", "add", "--detach", worktree, "HEAD")

	op := mustOpen(t, worktree, OpenOptions{Unit: "demo"}).Operation
	writeFile(t, worktree, "core/runtime/x.go", "package runtime\n")

	report := mustCheck(t, worktree, op.OperationID)
	if report.Result != "FAIL" || !contains(report.OutOfScope, "core/runtime/x.go") {
		t.Fatalf("worktree check must report the worktree's own out-of-scope change: %+v", report)
	}

	// The operation state is local to the worktree: the main working tree
	// does not see it, and its own check does not see the worktree's change.
	if _, err := Check(repoRoot, op.OperationID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("main tree must not see the worktree's operation state, got %v", err)
	}
	mainOp := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	mainReport := mustCheck(t, repoRoot, mainOp.OperationID)
	if mainReport.Result != "PASS" {
		t.Fatalf("main tree check = %+v, want PASS (worktree changes are not visible)", mainReport)
	}
}

// ------------------------------------------------------------
// Listing
// ------------------------------------------------------------

func TestListAndOpenOperations(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")

	first := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	second := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation
	if _, err := Close(repoRoot, second.OperationID, false, testNow()); err != nil {
		t.Fatal(err)
	}

	all, err := List(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("list = %d operations, want 2", len(all))
	}
	open := OpenOperations(all)
	if len(open) != 1 || open[0].OperationID != first.OperationID {
		t.Fatalf("open operations = %v, want only %s", open, first.OperationID)
	}
}

// ------------------------------------------------------------
// Mutation locking
// ------------------------------------------------------------

func TestMutationsSerializeOnRepositoryLock(t *testing.T) {
	repoRoot := newGitRepo(t)
	writeUnitSpec(t, repoRoot, "demo", "internal/demo", nil)
	commitAll(t, repoRoot, "fixture")
	op := mustOpen(t, repoRoot, OpenOptions{Unit: "demo"}).Operation

	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "update",
			run: func() error {
				_, err := Update(repoRoot, op.OperationID, UpdateOptions{
					Allow:    []string{"scripts/other"},
					HasAllow: true,
				}, testNow())
				return err
			},
		},
		{
			name: "close",
			run: func() error {
				_, err := Close(repoRoot, op.OperationID, false, testNow())
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acquired := make(chan struct{})
			release := make(chan struct{})
			lockErr := make(chan error, 1)
			go func() {
				lockErr <- localstate.WithExclusiveLock(repoRoot, mutationLockDir, mutationLock, time.Second, func() error {
					close(acquired)
					<-release
					return nil
				})
			}()
			<-acquired

			done := make(chan error, 1)
			go func() { done <- tc.run() }()
			select {
			case err := <-done:
				t.Fatalf("mutation completed while the state lock was held: %v", err)
			case <-time.After(200 * time.Millisecond):
			}

			close(release)
			if err := <-lockErr; err != nil {
				t.Fatalf("lock holder: %v", err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("mutation after lock release: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("mutation did not complete after the state lock was released")
			}
		})
	}
}

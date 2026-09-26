package gaterun

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newRepo returns a temporary directory that is a git worktree. Directory code
// surfaces expand over Git repository content, so a test project root must be
// a worktree; files written into it are untracked and unignored, which is
// repository content.
func newRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	cmd := exec.Command("git", "-C", repoRoot, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return repoRoot
}

func writeFile(t *testing.T, repoRoot, rel, content string) string {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// unitSpec builds a minimal candidate/stable unit spec. The acceptance item
// carries the given implementation_surface and optional extra body content.
func unitSpec(name, unitRefs, ruleRefs, implSurface, extra string) string {
	return "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: " + unitRefs + "\nrule_refs: " + ruleRefs + "\n---\n\n# " + name + "\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + name + ".core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: " + implSurface + "\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n" + extra
}

func writeUnit(t *testing.T, repoRoot, layer, name, unitRefs, ruleRefs, implSurface, extra string) {
	t.Helper()
	writeFile(t, repoRoot, "docs/specs/units/"+layer+"/unit_"+name+".md", unitSpec(name, unitRefs, ruleRefs, implSurface, extra))
}

func writeRule(t *testing.T, repoRoot, layer, id string) {
	t.Helper()
	writeFile(t, repoRoot, "docs/specs/rules/"+layer+"/"+id+".md", "---\nid: "+id+"\nversion: 0.1.0\nscope: unit\n---\n\n# "+id+"\n\n## Constraint\n\nMust use TLS.\n")
}

func refByRef(run *Run, ref string) (Ref, bool) {
	for _, r := range run.Refs {
		if r.Ref == ref {
			return r, true
		}
	}
	return Ref{}, false
}

func surfaceByPath(run *Run, path string) (Surface, bool) {
	for _, s := range run.Surfaces {
		if s.Path == path {
			return s, true
		}
	}
	return Surface{}, false
}

func mustContain(t *testing.T, lines []string, want string) {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, want) {
			return
		}
	}
	t.Fatalf("expected a divergence mentioning %q, got %v", want, lines)
}

func TestOpenDerivesUnitValidateSurface(t *testing.T) {
	repoRoot := newRepo(t)
	extra := "    affects:\n      files:\n        - docs/notes.md\n"
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "b_rule_x", "src", extra)
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "")
	writeFile(t, repoRoot, "docs/specs/units/candidate/appendix/unit_dep_protocol.md", "---\nunit: dep\nstatus: active\n---\n\n# Protocol\n")
	writeRule(t, repoRoot, "candidate", "b_rule_x")
	writeRule(t, repoRoot, "candidate", "g_rule_repo")
	writeRule(t, repoRoot, "stable", "g_rule_repo")
	writeRule(t, repoRoot, "candidate", "g_rule_draft")
	writeFile(t, repoRoot, "docs/notes.md", "notes\n")

	run, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"docs/specs/units/candidate/unit_auth.md",
		"unit:dep",
		"unit:dep:appendix:unit_dep_protocol",
		"rule:b_rule_x",
		"rule:g_rule_repo",
		"docs/notes.md",
	} {
		ref, ok := refByRef(run, want)
		if !ok {
			t.Fatalf("expected derived ref %q, got %+v", want, run.Refs)
		}
		if want != "unit:dep:appendix:unit_dep_protocol" && ref.Hash == "" {
			t.Fatalf("expected a hash for %q", want)
		}
		if ref.Hash == "" {
			t.Fatalf("expected a hash for %q", want)
		}
	}
	global, ok := refByRef(run, "rule:g_rule_repo")
	if !ok || global.Resolved != "docs/specs/rules/stable/g_rule_repo.md" {
		t.Fatalf("global rule must resolve to stable, got %+v", global)
	}
	if _, ok := refByRef(run, "rule:g_rule_draft"); ok {
		t.Fatalf("candidate-only global rule must not enter the unit validate snapshot: %+v", run.Refs)
	}
	if len(run.Surfaces) != 0 {
		t.Fatalf("validate runs declare no code surface, got %+v", run.Surfaces)
	}
	// The run state is persisted under meta/gate_runs.
	if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(StateRelPath(run.RunID)))); err != nil {
		t.Fatalf("expected run state on disk: %v", err)
	}
}

func TestOpenDerivesUnitCodeGateSurface(t *testing.T) {
	repoRoot := newRepo(t)
	extra := "    affects:\n      files:\n        - docs/design.md\n  - id: auth.pending\n    description: Later.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: <pending>\n    verification_method: test\n    pass_condition: Later.\n    runnable: no\n"
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", extra)
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	writeFile(t, repoRoot, "src/sub/util.go", "package sub\n")
	writeFile(t, repoRoot, "docs/design.md", "design\n")

	run, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	surface, ok := surfaceByPath(run, "src")
	if !ok {
		t.Fatalf("expected the src surface, got %+v", run.Surfaces)
	}
	entries := map[string]bool{}
	for _, e := range surface.Entries {
		entries[e.Path] = true
	}
	if !entries["src/main.go"] || !entries["src/sub/util.go"] {
		t.Fatalf("expected recursive directory expansion, got %+v", surface.Entries)
	}
	design, ok := surfaceByPath(run, "docs/design.md")
	if !ok || len(design.Entries) != 1 || design.Entries[0].Path != "docs/design.md" {
		t.Fatalf("expected the affects.files surface, got %+v", run.Surfaces)
	}
	if _, ok := surfaceByPath(run, "<pending>"); ok {
		t.Fatalf("the <pending> placeholder must be skipped, got %+v", run.Surfaces)
	}
}

func TestOpenIgnoresFencedAcceptanceSetWhenDerivingCodeSurface(t *testing.T) {
	repoRoot := newRepo(t)
	spec := "---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n~~~yaml\nacceptance_item_set:\n  - id: fake.item\n    implementation_surface: fake\n~~~\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", spec)
	writeFile(t, repoRoot, "src/main.go", "package main\n")

	run, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := surfaceByPath(run, "fake"); ok {
		t.Fatalf("fenced example leaked into the code surface: %+v", run.Surfaces)
	}
	if _, ok := surfaceByPath(run, "src"); !ok {
		t.Fatalf("real implementation surface is missing: %+v", run.Surfaces)
	}
	if run.PacketByID("detect:fake.item") != nil || run.PacketByID("detect:auth.core") == nil {
		t.Fatalf("packet ids did not use the real acceptance set: %+v", run.Packets)
	}
}

// TestPlanVerifyRejectsUnresolvableImplementationSurface verifies the
// fail-closed planning precondition: a non-<pending> implementation_surface
// that cannot resolve to a code file rejects the plan before any run state is
// written, with the item id and the reason.
func TestPlanVerifyRejectsUnresolvableImplementationSurface(t *testing.T) {
	repoRoot := newRepo(t)
	pending := "  - id: auth.pending\n    description: Later.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: <pending>\n    verification_method: test\n    pass_condition: Later.\n    runnable: no\n"
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "internal/demo/a.go; internal/demo/b.go", pending)
	writeFile(t, repoRoot, "internal/demo/a.go", "package demo\n")
	writeFile(t, repoRoot, "internal/demo/b.go", "package demo\n")

	_, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "auth.core") || !strings.Contains(err.Error(), "path does not exist") {
		t.Fatalf("expected the item-granular surface rejection, got %v", err)
	}
	if strings.Contains(err.Error(), "auth.pending") {
		t.Fatalf("the <pending> placeholder must not be reported, got %v", err)
	}
	runs, listErr := ListRuns(repoRoot)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(runs) != 0 {
		t.Fatalf("a rejected plan must not leave run state, got %v", runs)
	}
}

// TestPlanReviewRejectsUnresolvableImplementationSurface verifies the review
// gate applies the same precondition as verify.
func TestPlanReviewRejectsUnresolvableImplementationSurface(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "internal/tool/**", "")
	writeFile(t, repoRoot, "internal/tool/main.go", "package tool\n")

	_, err := Plan(repoRoot, GateReview, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "auth.core") || !strings.Contains(err.Error(), "path does not exist") {
		t.Fatalf("expected the unresolvable-pattern rejection, got %v", err)
	}
}

// TestPlanVerifyReadsItemRelativeIndentSurface verifies a consistently nested
// item block contributes its declared surface to the plan: the reader follows
// the item's own indent instead of assuming the canonical column.
func TestPlanVerifyReadsItemRelativeIndentSurface(t *testing.T) {
	repoRoot := newRepo(t)
	spec := "---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n    - id: auth.core\n      description: Core.\n      verification_type: testable\n      verification_surface: api\n      implementation_surface: internal/demo/a.go\n      verification_method: test\n      pass_condition: Passes.\n      runnable: yes\n"
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", spec)
	writeFile(t, repoRoot, "internal/demo/a.go", "package demo\n")

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := surfaceByPath(run, "internal/demo/a.go"); !ok {
		t.Fatalf("expected the item-relative surface in the plan, got %+v", run.Surfaces)
	}
}

// TestPlanVerifyRejectsEmptyDirectorySurface verifies a directory that
// expands to zero files is rejected rather than silently producing an empty
// code surface.
func TestPlanVerifyRejectsEmptyDirectorySurface(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "internal/empty", "")
	if err := os.MkdirAll(filepath.Join(repoRoot, "internal/empty"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "directory contains no files") {
		t.Fatalf("expected the empty-directory rejection, got %v", err)
	}
}

// TestPlanVerifyExcludesIgnoredFilesFromSurface verifies a declared directory
// expands over Git repository content: ignored dependencies and build output
// stay out of the snapshot, its read refs, and the packet plan, while
// untracked non-ignored files remain part of the surface.
func TestPlanVerifyExcludesIgnoredFilesFromSurface(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, ".gitignore", "web/node_modules/\nweb/dist/\n")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "web", "")
	writeFile(t, repoRoot, "web/app.js", "export {};\n")
	writeFile(t, repoRoot, "web/lib/util.js", "export {};\n")
	writeFile(t, repoRoot, "web/node_modules/dep/index.js", "module.exports = {};\n")
	writeFile(t, repoRoot, "web/dist/bundle.js", "bundle\n")

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	surface, ok := surfaceByPath(run, "web")
	if !ok {
		t.Fatalf("expected the declared directory surface, got %+v", run.Surfaces)
	}
	var got []string
	for _, entry := range surface.Entries {
		got = append(got, entry.Path)
	}
	if strings.Join(got, ",") != "web/app.js,web/lib/util.js" {
		t.Fatalf("ignored dependencies and build output must not enter the surface, got %v", got)
	}
	if len(run.Packets) == 0 {
		t.Fatal("expected the verify packet plan")
	}
	for _, packet := range run.Packets {
		for _, ref := range packet.ReadRefs {
			if strings.Contains(ref, "node_modules") || strings.Contains(ref, "dist/") {
				t.Fatalf("packet %s must not list ignored files in its read refs: %s", packet.PacketID, ref)
			}
		}
	}
}

// TestPlanVerifyRejectsDirectoryWithOnlyIgnoredFiles verifies the fail-closed
// precondition applies to repository content: a directory whose files are all
// ignored expands to zero files and rejects the plan.
func TestPlanVerifyRejectsDirectoryWithOnlyIgnoredFiles(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, ".gitignore", "dist/\n")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "dist", "")
	writeFile(t, repoRoot, "dist/bundle.js", "bundle\n")

	_, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "directory contains no files") {
		t.Fatalf("expected the only-ignored-files directory rejection, got %v", err)
	}
}

// TestPlanValidateIgnoresCodeSurfaceResolution verifies the precondition is
// verify/review-only: the validate gate derives no code surface and plans
// normally for a spec whose surface is still the <pending> placeholder.
func TestPlanValidateIgnoresCodeSurfaceResolution(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "<pending>", "")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("validate planning must not require a resolvable code surface, got %v", err)
	}
	if len(run.Surfaces) != 0 {
		t.Fatalf("validate runs declare no code surface, got %+v", run.Surfaces)
	}
}

// TestPlanCodeGatesRejectEmptyAcceptanceItemSet verifies the work-set
// precondition: a verify/review plan for a spec with an empty
// acceptance_item_set is rejected before any run state is written — an empty
// item set has no verifiable object and would plan only the cross packet.
// The validate gate still plans it, because reporting the empty set as a
// check failure is the validate gate's job.
func TestPlanCodeGatesRejectEmptyAcceptanceItemSet(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md",
		"---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# auth\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n")

	for _, gate := range []string{GateVerify, GateReview} {
		if _, err := Plan(repoRoot, gate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "at least one acceptance item") {
			t.Fatalf("expected the empty-item-set rejection for %s, got %v", gate, err)
		}
	}
	runs, err := ListRuns(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("a rejected plan must not leave run state, got %v", runs)
	}
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now()); err != nil {
		t.Fatalf("validate planning must still plan an empty item set so Check 2 can report it, got %v", err)
	}
}

func TestOpenDerivesRuleValidateSurface(t *testing.T) {
	repoRoot := newRepo(t)
	writeRule(t, repoRoot, "candidate", "b_rule_http")
	writeRule(t, repoRoot, "stable", "b_rule_http")
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeUnit(t, repoRoot, "stable", "dep", "none", "none", "src", "")

	run, err := open(t, repoRoot, GateValidate, TargetKindRule, "b_rule_http", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"docs/specs/rules/candidate/b_rule_http.md",
		"docs/specs/rules/stable/b_rule_http.md",
		"unit:auth",
		"unit:dep",
	} {
		if _, ok := refByRef(run, want); !ok {
			t.Fatalf("expected derived ref %q, got %+v", want, run.Refs)
		}
	}
}

func TestOpenTargetLayerRules(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeUnit(t, repoRoot, "stable", "auth", "none", "none", "src", "")

	if _, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetStable, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "stable-only") {
		t.Fatalf("expected a stable-only error when a candidate exists, got %v", err)
	}
	if _, err := open(t, repoRoot, GateValidate, TargetKindUnit, "ghost", TargetCandidate, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "no candidate") {
		t.Fatalf("expected a missing-candidate error, got %v", err)
	}
	if _, err := open(t, repoRoot, GateVerify, TargetKindRule, "b_rule_http", TargetCandidate, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "validate gate only") {
		t.Fatalf("expected rule verify to be rejected, got %v", err)
	}
}

func TestOpenReplacesPreviousRunForSameTuple(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	first, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID == second.RunID {
		t.Fatal("expected a fresh run id")
	}
	if _, err := Load(repoRoot, first.RunID); err == nil {
		t.Fatal("expected the replaced run to be gone")
	}
	if _, err := Load(repoRoot, second.RunID); err != nil {
		t.Fatalf("expected the new run to load: %v", err)
	}
}

func TestLoadRejectsInvalidAndMismatchedRunIDs(t *testing.T) {
	repoRoot := newRepo(t)
	if _, err := Load(repoRoot, "../../outside"); err == nil || !strings.Contains(err.Error(), "invalid local-state id") {
		t.Fatalf("expected traversal-shaped id to be rejected, got %v", err)
	}

	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	run, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	statePath, err := runStatePath(repoRoot, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var stored Run
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	stored.RunID = "20260101-000000-aaaaaa"
	data, err = json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(repoRoot, run.RunID); err == nil || !strings.Contains(err.Error(), "does not match requested id") {
		t.Fatalf("expected embedded run id mismatch to be rejected, got %v", err)
	}
}

func TestPacketStateFilenameIsFixedLengthAndFilesystemSafe(t *testing.T) {
	ids := []string{
		"detect:auth.core",
		"analysis:auth.core",
		`src\\windows:name/handler.go`,
		"src/用户/处理器.go",
		strings.Repeat("very/long/review/path/", 40) + "service.go",
	}
	seen := map[string]string{}
	for _, id := range ids {
		base := packetFileBase(id)
		if len(base) != 64 {
			t.Fatalf("packetFileBase(%q) length = %d, want 64", id, len(base))
		}
		if strings.Trim(base, "0123456789abcdef") != "" {
			t.Fatalf("packetFileBase(%q) = %q, want lowercase hexadecimal", id, base)
		}
		if prior, ok := seen[base]; ok {
			t.Fatalf("packet ids %q and %q mapped to the same filename %q", prior, id, base)
		}
		seen[base] = id
		if again := packetFileBase(id); again != base {
			t.Fatalf("packetFileBase(%q) is not stable: %q then %q", id, base, again)
		}
	}
}

func TestPacketStateRoundTripsReservedAndLongIDs(t *testing.T) {
	repoRoot := newRepo(t)
	run := &Run{RunID: "20260917-123456-a0b1c2"}
	ids := []string{
		"detect:auth.core",
		"analysis:auth.core",
		`src\\windows:name/handler.go`,
		strings.Repeat("very/long/review/path/", 40) + "service.go",
	}
	for _, id := range ids {
		run.Packets = append(run.Packets, PacketSpec{PacketID: id})
		want := &PacketState{PacketID: id, Status: PacketAccepted}
		if err := SavePacketState(repoRoot, run, want); err != nil {
			t.Fatalf("SavePacketState(%q): %v", id, err)
		}
		got, err := LoadPacketState(repoRoot, run, id)
		if err != nil {
			t.Fatalf("LoadPacketState(%q): %v", id, err)
		}
		if got.PacketID != id || got.Status != PacketAccepted {
			t.Fatalf("round trip for %q = %+v", id, got)
		}
	}
}

func TestDeleteRejectsInvalidRunIDWithoutRemovingOutsideState(t *testing.T) {
	repoRoot := newRepo(t)
	sentinel := writeFile(t, repoRoot, "meta/sentinel.txt", "keep\n")
	if err := Delete(repoRoot, &Run{RunID: "../.."}); err == nil || !strings.Contains(err.Error(), "invalid local-state id") {
		t.Fatalf("expected invalid run id to be rejected, got %v", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("outside sentinel was modified: %v", err)
	}
}

func TestCompareDetectsRefDivergences(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "b_rule_x", "src", "")
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "")
	writeRule(t, repoRoot, "candidate", "b_rule_x")

	run, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	divergences, err := Compare(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(divergences) != 0 {
		t.Fatalf("expected no divergences, got %v", divergences)
	}

	// A modified input is reported.
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", unitSpec("auth", "dep", "b_rule_x", "src", "")+"\n<!-- edited -->\n")
	divergences, err = Compare(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "modified: docs/specs/units/candidate/unit_auth.md")

	// A candidate-only global rule is unpublished and is not an input change.
	run2, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	writeRule(t, repoRoot, "candidate", "g_rule_draft")
	divergences, err = Compare(repoRoot, run2)
	if err != nil {
		t.Fatal(err)
	}
	if len(divergences) != 0 {
		t.Fatalf("candidate-only global rule must not change the snapshot, got %v", divergences)
	}

	// A new stable global rule appears — an added derived input.
	writeRule(t, repoRoot, "stable", "g_rule_new")
	divergences, err = Compare(repoRoot, run2)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "input added: rule:g_rule_new")

	// A dependency unit's layer resolution moves.
	run3, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dep.md"))
	writeUnit(t, repoRoot, "stable", "dep", "none", "none", "src", "")
	divergences, err = Compare(repoRoot, run3)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "layer resolution changed: unit:dep")
}

func TestCompareTracksOnlyStableGlobalRules(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeRule(t, repoRoot, "stable", "g_rule_active")
	writeRule(t, repoRoot, "candidate", "g_rule_active")

	run, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, repoRoot, "docs/specs/rules/candidate/g_rule_active.md", "changed candidate draft\n")
	writeRule(t, repoRoot, "candidate", "g_rule_draft")
	divergences, err := Compare(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	if len(divergences) != 0 {
		t.Fatalf("candidate global changes must not change a unit snapshot, got %v", divergences)
	}

	writeFile(t, repoRoot, "docs/specs/rules/stable/g_rule_active.md", "changed stable constraint\n")
	divergences, err = Compare(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "modified: rule:g_rule_active")
}

func TestCompareDetectsSurfaceDivergences(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	writeFile(t, repoRoot, "src/sub/util.go", "package sub\n")

	run, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, repoRoot, "src/main.go", "package main // edited\n")
	divergences, err := Compare(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "modified: src/main.go")

	run2, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, repoRoot, "src/new.go", "package main\n")
	divergences, err = Compare(repoRoot, run2)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "added: src/new.go")

	run3, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(repoRoot, "src/sub/util.go"))
	divergences, err = Compare(repoRoot, run3)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "removed: src/sub/util.go")
}

func TestCompareDetectsMissingSpec(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")

	run, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md"))
	divergences, err := Compare(repoRoot, run)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "input surface no longer resolvable")
}

func TestAllowsDeclaration(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "dep", "none", "src", "")
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")

	validateRun, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !validateRun.AllowsDeclaration(repoRoot, "docs/specs/units/candidate/unit_auth.md") {
		t.Fatal("expected the unit's own main spec to be allowed")
	}
	if !validateRun.AllowsDeclaration(repoRoot, "unit:dep") {
		t.Fatal("expected the dependency logical reference to be allowed")
	}
	if validateRun.AllowsDeclaration(repoRoot, "src/main.go") {
		t.Fatal("expected a code file to be outside a validate snapshot")
	}
	if validateRun.AllowsDeclaration(repoRoot, "unit:other") {
		t.Fatal("expected an unlisted logical reference to be rejected")
	}

	verifyRun, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !verifyRun.AllowsDeclaration(repoRoot, "src/main.go") {
		t.Fatal("expected the code surface file to be allowed")
	}
}

func TestExtraInputs(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	writeFile(t, repoRoot, "tests/unit_test.go", "package tests\n")
	writeFile(t, repoRoot, "extra.txt", "extra\n")

	run, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, []string{"tests", "extra.txt"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	surface, ok := surfaceByPath(run, "tests")
	if !ok || len(surface.Entries) != 1 || surface.Entries[0].Path != "tests/unit_test.go" {
		t.Fatalf("expected the tests surface, got %+v", run.Surfaces)
	}
	if _, ok := refByRef(run, "extra.txt"); !ok {
		t.Fatalf("expected the extra file ref, got %+v", run.Refs)
	}
	if !run.AllowsDeclaration(repoRoot, "tests/unit_test.go") {
		t.Fatal("expected the extra surface file to be allowed")
	}

	run2, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, []string{"tests"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, repoRoot, "tests/new_test.go", "package tests\n")
	divergences, err := Compare(repoRoot, run2)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, divergences, "added: tests/new_test.go")

	if _, err := open(t, repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, []string{"missing-dir"}, time.Now()); err == nil {
		t.Fatal("expected a missing --input path to be rejected")
	}
}

func TestConsumeDeleteAndLoad(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	run, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := Consume(repoRoot, run); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(repoRoot, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != StatusConsumed {
		t.Fatalf("expected consumed status, got %q", loaded.Status)
	}
	if _, err := Compare(repoRoot, loaded); err == nil {
		t.Fatal("expected a consumed run to be rejected by Compare")
	}
	if err := Delete(repoRoot, run); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(repoRoot, run.RunID); err == nil {
		t.Fatal("expected the deleted run to be gone")
	}
	if err := Delete(repoRoot, run); err != nil {
		t.Fatalf("deleting a missing run must be a no-op: %v", err)
	}

	if _, err := Load(repoRoot, "../escape"); err == nil {
		t.Fatal("expected a path-traversal run id to be rejected")
	}
	if _, err := Load(repoRoot, "does-not-exist"); err == nil {
		t.Fatal("expected a missing run to be an error")
	}
}

func TestOpenLogicalInputRef(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src", "")

	run, err := open(t, repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, []string{"unit:dep"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := refByRef(run, "unit:dep")
	if !ok || ref.Resolved != "docs/specs/units/candidate/unit_dep.md" || ref.Hash == "" {
		t.Fatalf("expected the logical input ref resolved, got %+v", run.Refs)
	}
}

// open is the test helper for a full-mode plan.
func open(t *testing.T, repoRoot, gate, targetKind, targetName, target string, extraInputs []string, now time.Time) (*Run, error) {
	t.Helper()
	return Plan(repoRoot, gate, targetKind, targetName, target, ModeFull, extraInputs, nil, now)
}

func TestPlanRejectsInvalidTargetName(t *testing.T) {
	repoRoot := newRepo(t)
	// The traversed rule file exists, so only the name gate can reject the
	// traversal that previously reached validateTargetLayer.
	writeFile(t, repoRoot, "tmp/evil.md", "---\nid: evil\nversion: 0.1.0\nscope: unit\n---\n\n# evil\n")
	writeRule(t, repoRoot, "candidate", "b_rule_http")

	ruleNames := []string{"../../../../tmp/evil", "a/b", "..", "with space", "trailing "}
	for _, name := range ruleNames {
		if _, err := Plan(repoRoot, GateValidate, TargetKindRule, name, TargetCandidate, ModeFull, nil, nil, time.Now()); err == nil {
			t.Fatalf("expected rule id %q to be rejected", name)
		}
	}
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth/x", TargetCandidate, ModeFull, nil, nil, time.Now()); err == nil {
		t.Fatal("expected a unit name with a separator to be rejected")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "meta")); !os.IsNotExist(err) {
		t.Fatalf("a rejected target name must not touch the working tree, stat err=%v", err)
	}
}

func TestConcurrentPlansLeaveOneOpenRun(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src", "")

	start := make(chan struct{})
	runs := make(chan *Run, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(offset time.Duration) {
			defer wg.Done()
			<-start
			run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, time.Now().Add(offset))
			runs <- run
			errs <- err
		}(time.Duration(i) * time.Second)
	}
	close(start)
	wg.Wait()
	close(runs)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent plan failed: %v", err)
		}
	}
	planned := map[string]bool{}
	for run := range runs {
		if run == nil {
			t.Fatal("concurrent plan returned a nil run")
		}
		planned[run.RunID] = true
	}
	if len(planned) != 2 {
		t.Fatalf("expected two distinct plan results, got %v", planned)
	}

	open, err := ListRuns(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].Status != StatusOpen {
		t.Fatalf("expected exactly one persisted open run, got %+v", open)
	}
}

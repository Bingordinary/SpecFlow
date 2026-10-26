package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func TestStableFreshDone(t *testing.T) {
	root := createCLITestRepo(t)
	unit := "auth"
	writePromiseStateUnit(t, root, unit, "auth.login_api", "", "login")
	candidate := filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")
	stableDir := filepath.Join(root, "docs/specs/units/stable")
	if err := os.MkdirAll(stableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stableSpec := filepath.Join(stableDir, "unit_auth.md")
	if err := os.Rename(candidate, stableSpec); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(root, "src", "auth.go")
	if err := os.MkdirAll(filepath.Dir(code), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code, []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeUnitCache(t, root, unit, "validate", "target: stable\n", []cacheFileSpec{{
		path: "docs/specs/units/stable/unit_auth.md",
		hash: computeHash(stableSpec),
	}})
	currentVerifyFixture(t, root, unit, "stable", "target: stable\n")

	var stdout, stderr bytes.Buffer
	if err := runFresh([]string{"--repo-root", root, "--scope", "stable"}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh: %v\n%s", err, stderr.String())
	}
	for _, want := range []string{"done: YES", "DONE: 1 of 1"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stable fresh output missing %q:\n%s", want, stdout.String())
		}
	}
}

// TestPromotedConsumerVerifyStaysFresh pins the documented promote outcome for
// a unit whose promises depend on a provider: the promoted cache is an
// immediately usable stable confirmation baseline, so its own verify reports
// FRESH and done YES right after the promote (issue #97). The candidate record
// reads the provider as the logical reference unit:logger, while the stable
// requirement names the published stable file — the same evidence in two
// spellings.
func TestPromotedConsumerVerifyStaysFresh(t *testing.T) {
	root := createCLITestRepo(t)
	writeLayerPromiseUnit(t, root, "stable", "logger", "logger.interface", "", "log")
	writeLayerPromiseUnit(t, root, "candidate", "trace", "trace.span_api", "logger.interface", "span")
	for _, name := range []string{"logger", "trace"} {
		path := filepath.Join(root, "src", name+".go")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package src\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	specPath := filepath.Join(root, "docs/specs/units/candidate/unit_trace.md")
	writeUnitCache(t, root, "trace", "validate", "target: candidate\n", []cacheFileSpec{{
		path: "docs/specs/units/candidate/unit_trace.md",
		hash: computeHash(specPath),
	}})
	currentVerifyFixture(t, root, "trace", "candidate", "target: candidate\n")

	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "trace"}, &out, &errOut); err != nil {
		t.Fatalf("promote: %v\n%s", err, out.String())
	}

	var stdout, stderr bytes.Buffer
	if err := runFresh([]string{"--repo-root", root, "--scope", "stable"}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh: %v\n%s", err, stderr.String())
	}
	report := stdout.String()
	traceLine := ""
	for _, line := range strings.Split(report, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "trace" {
			traceLine = line
			break
		}
	}
	if !strings.Contains(traceLine, "verify: FRESH") || !strings.Contains(traceLine, "done: YES") {
		t.Fatalf("promoted consumer is not an immediate stable confirmation baseline:\n%s", report)
	}
	if !strings.Contains(report, "DONE: 1 of 2") {
		t.Fatalf("only the promoted consumer has confirmation caches:\n%s", report)
	}
}

func TestPromiseStateDependencyPinAfterGates(t *testing.T) {
	root := createCLITestRepo(t)
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")

	// The gate planner derives the provider unit from the promise depends_on
	// edge; no frontmatter declaration exists.
	webSpec := filepath.Join(root, "docs/specs/units/candidate/unit_web.md")
	for _, rel := range []string{"src/auth.go", "src/web.go"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package web\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeUnitCache(t, root, "web", "validate", "", []cacheFileSpec{{
		path: "docs/specs/units/candidate/unit_web.md",
		hash: computeHash(webSpec),
	}})
	currentVerifyFixture(t, root, "web", "candidate", "target: candidate\n")

	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	state, err := validationcache.ReadPromiseStateFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || len(state.Pins) != 1 || state.Pins[0].ID != "auth.login_api" {
		t.Fatalf("expected auth.login_api pin, got %+v", state)
	}

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "done    YES") || !strings.Contains(out, "auth.login_api  FRESH") {
		t.Fatalf("expected DONE with FRESH pin, got:\n%s", out)
	}
}

func TestPromiseStateDONEAfterGates(t *testing.T) {
	root := createCLITestRepo(t)
	unit := "auth"
	writePromiseStateUnit(t, root, unit, "auth.login_api", "", "login")
	codePath := filepath.Join(root, "src", "auth.go")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codePath, []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")
	writeUnitCache(t, root, unit, "validate", "", []cacheFileSpec{{
		path: "docs/specs/units/candidate/unit_auth.md",
		hash: computeHash(specPath),
	}})
	currentVerifyFixture(t, root, unit, "candidate", "target: candidate\n")

	statePath, err := validationcache.PromiseStatePath(root, unit, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	state, err := validationcache.ReadPromiseStateFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil {
		t.Fatalf("verify finalize did not record promise state at %s", statePath)
	}
	if len(state.Promises) != 1 || state.Promises[0].ID != "auth.login_api" {
		t.Fatalf("unexpected own promise state: %+v", state.Promises)
	}

	detail := promiseStateText(t, root, unit, "candidate")
	if !strings.Contains(detail, "done    YES") {
		t.Fatalf("expected done YES, got:\n%s", detail)
	}

	var freshOut, freshErr bytes.Buffer
	if err := runFresh([]string{"--repo-root", root, "--scope", "candidate"}, &freshOut, &freshErr); err != nil {
		t.Fatalf("fresh: %v\n%s", err, freshErr.String())
	}
	for _, want := range []string{"done: YES", "DONE: 1 of 1 unit(s)"} {
		if !strings.Contains(freshOut.String(), want) {
			t.Fatalf("fresh output missing %q:\n%s", want, freshOut.String())
		}
	}
}

func TestFreshNotReadyWithoutPromiseState(t *testing.T) {
	root := createCLITestRepo(t)
	unit := "auth"
	writePromiseStateUnit(t, root, unit, "auth.login_api", "", "login")
	codePath := filepath.Join(root, "src", "auth.go")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codePath, []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")
	writeUnitCache(t, root, unit, "validate", "", []cacheFileSpec{{path: "docs/specs/units/candidate/unit_auth.md", hash: computeHash(specPath)}})
	currentVerifyFixture(t, root, unit, "candidate", "target: candidate\n")

	statePath, err := validationcache.PromiseStatePath(root, unit, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runFresh([]string{"--repo-root", root, "--unit", unit}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh: %v\n%s", err, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"done    NO", "READY FOR PROMOTE: no", "promise state not recorded"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in fresh detail:\n%s", want, output)
		}
	}
}

func TestFreshNotReadyForMislayeredPromiseState(t *testing.T) {
	root := createCLITestRepo(t)
	unit := "auth"
	writePromiseStateUnit(t, root, unit, "auth.login_api", "", "login")
	codePath := filepath.Join(root, "src", "auth.go")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codePath, []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")
	writeUnitCache(t, root, unit, "validate", "", []cacheFileSpec{{path: "docs/specs/units/candidate/unit_auth.md", hash: computeHash(specPath)}})
	currentVerifyFixture(t, root, unit, "candidate", "target: candidate\n")

	statePath, err := validationcache.PromiseStatePath(root, unit, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(state), "layer: candidate", "layer: stable", 1)
	if edited == string(state) {
		t.Fatal("fixture edit did not apply to the promise state")
	}
	if err := os.WriteFile(statePath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runFresh([]string{"--repo-root", root, "--unit", unit}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh: %v\n%s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "READY FOR PROMOTE: yes") {
		t.Fatalf("fresh must not report ready for a mislayered record:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `promise state layer is "stable", expected "candidate"`) {
		t.Fatalf("fresh must name the layer defect:\n%s", stdout.String())
	}

	var out, errOut bytes.Buffer
	err = runPromote([]string{"--repo-root", root, "--unit", unit}, &out, &errOut)
	if err == nil || !strings.Contains(out.String(), "promise state layer") {
		t.Fatalf("promote must reject the same record, err=%v\n%s", err, out.String())
	}
}

func TestFreshReadinessRequiresPublishedProvider(t *testing.T) {
	report := &promiseStateReport{
		StateRecorded: true,
		Closure:       "CLOSED",
		Publication: []validationcache.PromiseState{{
			ID:     "auth.login_api",
			Status: validationcache.PromiseReopened,
			Reason: "provider promise fingerprint changed",
		}},
	}
	ready, reason := promiseReadiness(report, "")
	if ready {
		t.Fatal("a candidate provider draft must not make the consumer READY FOR PROMOTE")
	}
	if !strings.Contains(reason, "auth.login_api") || !strings.Contains(reason, "promote the provider first") {
		t.Fatalf("unexpected readiness reason: %s", reason)
	}

	report.Publication = []validationcache.PromiseState{{
		ID:     "auth.login_api",
		Status: validationcache.PromiseFresh,
	}}
	if ready, reason := promiseReadiness(report, ""); !ready {
		t.Fatalf("published provider must make the consumer ready: %s", reason)
	}
}

func TestMalformedPromiseSectionDiagnostics(t *testing.T) {
	root := createCLITestRepo(t)
	dir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: broken\nrule_refs: none\n---\n\n# Broken\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises: [\n  - id: broken.api\n\n## Acceptance Items\n\nacceptance_item_set:\n  - id: broken.check\n    promise: broken.api\n    description: Given X, When Y, Then Z.\n    verification_type: inspectable\n    verification_surface: api\n    implementation_surface: src/broken.go\n    verification_method: inspect.\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_broken.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runNext([]string{"--unit", "broken", "--repo-root", root}, &stdout, &stderr); err != nil {
		t.Fatalf("next: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Promise model: INVALID") {
		t.Fatalf("next must distinguish a malformed promise section:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := runFresh([]string{"--repo-root", root, "--scope", "candidate"}, &stdout, &stderr); err != nil {
		t.Fatalf("fresh: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "done: INVALID") {
		t.Fatalf("fresh must report a malformed promise section:\n%s", stdout.String())
	}
}

func TestValidateRejectsSpecWithoutPromises(t *testing.T) {
	root := createCLITestRepo(t)
	dir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := "---\nid: legacy\nunit_refs: [auth]\nrule_refs: none\n---\n\n# Legacy\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set: []\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_legacy.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := runValidate([]string{"candidate", "--unit", "legacy", "--repo-root", root}, &stdout, &stderr)
	if err == nil || !strings.Contains(stdout.String(), "no `## Promises` section") {
		t.Fatalf("a spec without ## Promises must fail validate, err=%v\n%s", err, stdout.String())
	}
}

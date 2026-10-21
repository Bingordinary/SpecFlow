package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func promiseVerifyRun(unit string) *gaterun.Run {
	return &gaterun.Run{
		Gate:       gaterun.GateVerify,
		TargetKind: gaterun.TargetKindUnit,
		TargetName: unit,
		Target:     "candidate",
	}
}

func TestPreparePromiseStateRecordsOwnAndPins(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")

	run := promiseVerifyRun("web")
	run.Refs = []gaterun.Ref{{Ref: "unit:auth"}}
	state, path, err := preparePromiseState(root, run, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if state == nil {
		t.Fatal("expected promise state, got nil")
	}
	if !strings.HasSuffix(filepath.ToSlash(path), "docs/specs/meta/validation/unit/web/promise_state_candidate.yaml") {
		t.Fatalf("unexpected path: %s", path)
	}
	if len(state.Promises) != 1 || state.Promises[0].ID != "web.session_api" {
		t.Fatalf("own promises = %+v", state.Promises)
	}
	if len(state.Pins) != 1 || state.Pins[0].ID != "auth.login_api" {
		t.Fatalf("pins = %+v", state.Pins)
	}
}

func TestPreparePromiseStateSkipsFailedAndOldFormat(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "web", "web.session_api", "", "session")

	state, _, err := preparePromiseState(root, promiseVerifyRun("web"), "fail")
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatalf("failed run must not record promise state: %+v", state)
	}

	oldSpec := "---\nid: web\nrule_refs: none\n---\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: web.core\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/web.go\n    verification_method: test.\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(filepath.Join(root, "docs/specs/units/candidate/unit_web.md"), []byte(oldSpec), 0o644); err != nil {
		t.Fatal(err)
	}
	state, _, err = preparePromiseState(root, promiseVerifyRun("web"), "pass")
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatalf("old-format unit must not record promise state: %+v", state)
	}
}

func TestPreparePromiseStateRejectsMissingDependency(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")

	_, _, err := preparePromiseState(root, promiseVerifyRun("web"), "pass")
	if err == nil || !strings.Contains(err.Error(), "unresolved depends_on") {
		t.Fatalf("expected missing-dependency error, got %v", err)
	}
}

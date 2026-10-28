package next

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNextUnit(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_auth.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverUnitPromiseModel(t *testing.T) {
	root := t.TempDir()
	writeNextUnit(t, root, `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative
### Narrative

Narrative.

## Promises

promises:
  - id: auth.login_api
    summary: Login.
    source: [Narrative]
    contract:
      operations:
        login:
          outputs:
            token: {type: string}
  - id: auth.session_api
    summary: Session.
    source: [Narrative]
    contract: {invariants: [session]}
    depends_on: [web.login_api]

## Acceptance Items

acceptance_item_set:
  - id: auth.core
    promise: auth.login_api
    source: [Narrative]
    description: Given X, When Y, Then Z.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/auth.go
    verification_method: test.
    pass_condition: Passes.
    runnable: yes
`)
	writeNextProvider(t, root, "web", "web.login_api")
	info, err := DiscoverUnit(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !info.PromiseModel || len(info.Promises) != 2 {
		t.Fatalf("promise model = %v, promises = %+v", info.PromiseModel, info.Promises)
	}
	if info.Promises[1].ID != "auth.session_api" || strings.Join(info.Promises[1].DependsOn, ",") != "web.login_api" {
		t.Fatalf("unexpected promise info: %+v", info.Promises)
	}
	out := FormatInfo(info)
	if !strings.Contains(out, "Promise model: 2 promise(s)") || !strings.Contains(out, "auth.session_api (invariants) depends_on: web.login_api") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func writeNextProvider(t *testing.T, root, unit, promiseID string) {
	t.Helper()
	dir := filepath.Join(root, "docs/specs/units/candidate")
	content := "---\nid: " + unit + "\nrule_refs: none\n---\n\n## Design Narrative\n### Narrative\n\nProse.\n\n## Promises\n\npromises:\n  - id: " + promiseID + "\n    summary: Provider.\n    source: [Narrative]\n    contract: {invariants: [provider]}\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unit+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverUnitWithoutPromisesReportsInvalid(t *testing.T) {
	root := t.TempDir()
	writeNextUnit(t, root, "---\nid: auth\nrule_refs: none\n---\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n")
	info, err := DiscoverUnit(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if info.PromiseModel || info.PromiseModelError == "" {
		t.Fatalf("expected an invalid promise model, got %+v", info)
	}
	if out := FormatInfo(info); !strings.Contains(out, "Promise model: INVALID — ## Promises section not found") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestDiscoverUnitSchemaInvalidPromiseReportsInvalid(t *testing.T) {
	root := t.TempDir()
	// The section parses but one promise declares a contract block outside the
	// allowed vocabulary (`response`), so the mechanical promise schema — not
	// just the YAML parse — decides validity.
	writeNextUnit(t, root, `---
id: auth
rule_refs: none
---

## Design Narrative
### Narrative

Narrative.

## Promises

promises:
  - id: auth.login_api
    summary: Login.
    contract:
      response:
        token: {type: string}

## Acceptance Items

acceptance_item_set:
  - id: auth.core
    promise: auth.login_api
    description: Given X, When Y, Then Z.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/auth.go
    verification_method: test.
    pass_condition: Passes.
    runnable: yes
`)
	info, err := DiscoverUnit(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if info.PromiseModel || info.PromiseModelError == "" {
		t.Fatalf("expected an invalid promise model, got %+v", info)
	}
	if out := FormatInfo(info); !strings.Contains(out, "Promise model: INVALID") || !strings.Contains(out, "unknown contract block") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

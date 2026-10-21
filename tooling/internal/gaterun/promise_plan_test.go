package gaterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePromisePlanSpec(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_auth.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func promisePlanSpec(promiseLine string) string {
	return `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

Narrative.

## Promises

promises:
  - id: auth.login_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Login.
    contract: {text: login}

## Acceptance Items

acceptance_item_set:
  - id: auth.core
` + promiseLine + `    description: Given X, When Y, Then Z.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/auth.go
    verification_method: test.
    pass_condition: Passes.
    runnable: yes
`
}

func TestCheckPromisePlanInputsRejectsInvalidCoverage(t *testing.T) {
	root := t.TempDir()
	writePromisePlanSpec(t, root, promisePlanSpec(""))
	err := checkPromisePlanInputs(root, GateVerify, TargetKindUnit, "auth", TargetCandidate)
	if err == nil || !strings.Contains(err.Error(), "Promise coverage") {
		t.Fatalf("expected promise coverage rejection, got %v", err)
	}
}

func TestCheckPromisePlanInputsAcceptsValidSpec(t *testing.T) {
	root := t.TempDir()
	writePromisePlanSpec(t, root, promisePlanSpec("    promise: auth.login_api\n"))
	if err := checkPromisePlanInputs(root, GateVerify, TargetKindUnit, "auth", TargetCandidate); err != nil {
		t.Fatalf("expected valid promise spec, got %v", err)
	}
}

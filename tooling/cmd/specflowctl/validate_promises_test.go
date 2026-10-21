package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const promiseCLISpec = `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

Login uses token-based authentication.

## Promises

promises:
  - id: auth.login_api
    kind: request_response
    summary: Login returns a token on success and 401 on invalid credentials.
    contract:
      request:
        username: {type: string, required: true}
      response:
        token: {type: string, required: true}
      errors:
        401: INVALID_CREDENTIALS

## Acceptance Items

acceptance_item_set:
  - id: auth.login_success
    promise: auth.login_api
    description: Given valid credentials, when login is called, then 200 + token is returned.
    verification_type: testable
    verification_surface: api
    implementation_surface: auth/login.go
    verification_method: Go integration test for login success.
    pass_condition: The login success scenario passes.
    runnable: yes
`

func writePromiseCLISpec(t *testing.T, repoRoot, content string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_auth.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePromiseModelCLI(t *testing.T) {
	repoRoot := t.TempDir()
	writePromiseCLISpec(t, repoRoot, promiseCLISpec)

	var stdout, stderr bytes.Buffer
	if err := runValidate([]string{"promise-model", "--unit", "auth", "--repo-root", repoRoot}, &stdout, &stderr); err != nil {
		t.Fatalf("expected PASS, got error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Validate result: PASS") {
		t.Fatalf("expected PASS output, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Unit spec shape: PASS") {
		t.Fatalf("expected shape check in output, got:\n%s", stdout.String())
	}
}

func TestValidatePromiseModelCLIRejectsExtraSection(t *testing.T) {
	repoRoot := t.TempDir()
	content := strings.Replace(promiseCLISpec, "## Acceptance Items", "## Notes\n\nText.\n\n## Acceptance Items", 1)
	writePromiseCLISpec(t, repoRoot, content)

	var stdout, stderr bytes.Buffer
	if err := runValidate([]string{"promise-model", "--unit", "auth", "--repo-root", repoRoot}, &stdout, &stderr); err == nil {
		t.Fatalf("expected failure, got nil\nstdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Unit spec shape: FAIL") {
		t.Fatalf("expected shape failure, got:\n%s", stdout.String())
	}
}

func TestValidatePromiseModelCLIMissingSpec(t *testing.T) {
	repoRoot := t.TempDir()
	var stdout, stderr bytes.Buffer
	if err := runValidate([]string{"promise-model", "--unit", "auth", "--repo-root", repoRoot}, &stdout, &stderr); err == nil {
		t.Fatal("expected missing-spec error, got nil")
	}
}

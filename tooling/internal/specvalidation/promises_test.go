package specvalidation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPromiseSpec = `---
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
        password: {type: string, required: true}
      response:
        token: {type: string, required: true}
      errors:
        401: INVALID_CREDENTIALS
      timing:
        mode: sync

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

func writePromiseCandidateRepo(t *testing.T, content string) string {
	t.Helper()
	root := newRepo(t)
	writeCandidate(t, root, "auth", content)
	for _, path := range []string{"auth/login.go", "src/auth.go"} {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("package auth\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestValidateCandidateRunsPromiseModel(t *testing.T) {
	root := writePromiseCandidateRepo(t, validPromiseSpec)
	result := ValidateCandidate(root, "auth")
	if !result.Passed {
		t.Fatalf("expected valid promise spec to pass, got: %s", FormatResult(result))
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "Promise model" {
			found = true
			if check.Status != Pass {
				t.Fatalf("promise model = %s: %s", check.Status, check.Details)
			}
		}
	}
	if !found {
		t.Fatal("ValidateCandidate did not run the promise model check")
	}
}

func TestValidateCandidateRejectsInvalidPromiseCoverage(t *testing.T) {
	root := writePromiseCandidateRepo(t, strings.Replace(validPromiseSpec, "    promise: auth.login_api\n", "", 1))
	result := ValidateCandidate(root, "auth")
	if result.Passed {
		t.Fatalf("expected invalid promise coverage to fail: %s", FormatResult(result))
	}
	var details string
	for _, check := range result.Checks {
		if check.Name == "Promise model" {
			details = check.Details
		}
	}
	if !strings.Contains(details, "promise coverage") && !strings.Contains(details, "Promise coverage") {
		t.Fatalf("expected promise coverage failure, got %q", details)
	}
}

func TestValidPromiseSpec(t *testing.T) {
	for _, check := range CheckPromiseSpec(validPromiseSpec, "auth") {
		if check.Status != Pass {
			t.Fatalf("%s: %s — %s", check.Name, check.Status, check.Details)
		}
	}
}

func TestUnitSpecShapeRequiresFixedSections(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing promises",
			content: strings.Replace(validPromiseSpec, "## Promises", "## Contracts", 1),
			want:    "section 2 must be",
		},
		{
			name:    "extra section",
			content: strings.Replace(validPromiseSpec, "## Acceptance Items", "## Notes\n\nText.\n\n## Acceptance Items", 1),
			want:    "exactly 3",
		},
		{
			name:    "wrong order",
			content: strings.Replace(validPromiseSpec, "## Design Narrative", "## Acceptance Items", 1),
			want:    "section 1 must be",
		},
		{
			name:    "deeper heading in promises",
			content: strings.Replace(validPromiseSpec, "promises:", "### Details\n\npromises:", 1),
			want:    "structured content only",
		},
		{
			name:    "missing promise marker",
			content: strings.Replace(validPromiseSpec, "promises:", "contracts:", 1),
			want:    "promises:",
		},
		{
			name:    "empty promise marker",
			content: strings.Replace(validPromiseSpec, "promises:", "promises: []", 1),
			want:    "promises:",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := CheckUnitSpecShape(tc.content)
			if result.Status != Fail {
				t.Fatalf("expected FAIL, got %s — %s", result.Status, result.Details)
			}
			if !strings.Contains(result.Details, tc.want) {
				t.Fatalf("expected details to contain %q, got %q", tc.want, result.Details)
			}
		})
	}
}

func TestPromiseSchemaRejectsCodeAndVerificationFields(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    summary:", "    implementation_surface: auth/login.go\n    summary:", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail {
		t.Fatalf("expected FAIL, got %s — %s", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "implementation_surface") {
		t.Fatalf("expected forbidden-field detail, got %q", result.Details)
	}
}

func TestPromiseSchemaRejectsUnknownTopLevelField(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    contract:\n", "    contract_v2: {response: {token: {type: int}}}\n    contract:\n", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "unknown top-level field") {
		t.Fatalf("expected an unknown-top-level-field failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseSchemaRejectsUnknownContractField(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    contract:\n      request:", "    contract:\n      implementation_surface: auth/login.go\n      request:", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "unknown request_response contract field") {
		t.Fatalf("expected an unknown-contract-field failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseSchemaRejectsForeignOrDuplicateIDs(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "id: auth.login_api", "id: web.login_api", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "must start with") {
		t.Fatalf("expected foreign-id failure, got %s — %s", result.Status, result.Details)
	}

	duplicate := strings.Replace(validPromiseSpec, "promises:\n", "promises:\n  - id: auth.login_api\n    kind: opaque\n    opaque_reason: no structured kind applies\n    summary: duplicate\n    contract: {text: duplicate}\n", 1)
	result = CheckPromiseSchema(duplicate, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "duplicate") {
		t.Fatalf("expected duplicate-id failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseSchemaRejectsSameUnitDependency(t *testing.T) {
	content := `## Promises

promises:
  - id: solo.base
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Base contract.
    contract: {text: base}
  - id: solo.derived
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Derived contract.
    contract: {text: derived}
    depends_on: [solo.base]
`
	result := CheckPromiseSchema(content, "solo")
	if result.Status != Fail || !strings.Contains(result.Details, "owned by this unit") {
		t.Fatalf("expected same-unit dependency failure, got %s — %s", result.Status, result.Details)
	}
}

// TestPromiseSchemaRejectsEmptyPromiseList pins the promise model's minimum:
// a valid unit declares at least one promise. An empty list built from the
// marker line plus an empty YAML list (the only representation that keeps the
// exact `promises:` marker line) previously passed as "no promises declared",
// which made promise coverage vacuous and let an item-less spec plan a verify
// run with no spec-to-code evidence.
func TestPromiseSchemaRejectsEmptyPromiseList(t *testing.T) {
	content := `## Promises

promises:
  []

## Acceptance Items

acceptance_item_set:
`
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "at least one promise") {
		t.Fatalf("expected an empty-promise-list failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseCoverageRequiresPromiseOrNonPromise(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    promise: auth.login_api\n", "", 1)
	result := CheckPromiseCoverage(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "must declare promise or non_promise") {
		t.Fatalf("expected coverage failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseCoverageRequiresEveryPromiseCovered(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    promise: auth.login_api\n", "    non_promise: true\n    non_promise_reason: temporary cleanup\n", 1)
	result := CheckPromiseCoverage(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "not covered") {
		t.Fatalf("expected uncovered promise failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseCoverageAllowsNonPromiseItem(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    runnable: yes\n", "    runnable: yes\n  - id: auth.cleanup\n    non_promise: true\n    non_promise_reason: one-off cleanup\n", 1)
	result := CheckPromiseCoverage(content, "auth")
	if result.Status != Pass {
		t.Fatalf("expected PASS with a justified non_promise item, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseCoverageIgnoresRetiredDependencyField(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    runnable: yes\n", "    runnable: yes\n    affects:\n      dependencies: [common]\n", 1)
	result := CheckPromiseCoverage(content, "auth")
	if result.Status != Pass {
		t.Fatalf("expected PASS — a field outside the deployed format is inert, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseCoverageRejectsForeignPromise(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    promise: auth.login_api", "    promise: web.login_api", 1)
	result := CheckPromiseCoverage(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "references unknown promise") {
		t.Fatalf("expected foreign promise failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseDependenciesForItemScopesToItsPromise(t *testing.T) {
	spec := `---
id: web
rule_refs: none
---

# Web

## Design Narrative

Sessions and billing.

## Promises

promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: request_response was closest; no status codes are established.
    summary: Session.
    contract: {text: session}
    depends_on: [auth.login_api]
  - id: web.billing_api
    kind: opaque
    opaque_reason: request_response was closest; no status codes are established.
    summary: Billing.
    contract: {text: billing}
    depends_on: [billing.charge_api]

## Acceptance Items

acceptance_item_set:
  - id: web.session
    promise: web.session_api
    description: Given X, When Y, Then Z.
    verification_type: inspectable
    verification_surface: api
    implementation_surface: src/web.go
    verification_method: inspect.
    pass_condition: Session works.
    runnable: yes
  - id: web.billing
    promise: web.billing_api
    description: Given X, When Y, Then Z.
    verification_type: inspectable
    verification_surface: api
    implementation_surface: src/billing.go
    verification_method: inspect.
    pass_condition: Billing works.
    runnable: yes
  - id: web.cleanup
    non_promise: true
    non_promise_reason: One-off cleanup.
    description: Remove the old handler.
    verification_type: inspectable
    verification_surface: internal_flow
    implementation_surface: src/old.go
    verification_method: inspect.
    pass_condition: Old handler removed.
    runnable: no
    not_runnable_reason: One-off inspection.
`
	cases := map[string][]string{
		"web.session": {"auth.login_api"},
		"web.billing": {"billing.charge_api"},
		"web.cleanup": nil,
		"web.unknown": nil,
	}
	for item, want := range cases {
		got := PromiseDependenciesForItem(spec, item)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s dependencies = %v, want %v", item, got, want)
		}
	}
}

func TestPromiseFingerprintIgnoresIdentityAndSummary(t *testing.T) {
	promises, issues := ParsePromises(validPromiseSpec)
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	if len(promises) != 1 {
		t.Fatalf("expected 1 promise, got %d", len(promises))
	}
	first, err := PromiseFingerprint(promises[0])
	if err != nil {
		t.Fatal(err)
	}

	renamed := strings.Replace(validPromiseSpec, "id: auth.login_api", "id: auth.login_api_v2", 1)
	renamed = strings.Replace(renamed, "summary: Login returns a token on success and 401 on invalid credentials.", "summary: A different summary.", 1)
	promises, issues = ParsePromises(renamed)
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	second, err := PromiseFingerprint(promises[0])
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identity or summary must not change the fingerprint:\n%s\n%s", first, second)
	}
}

func TestPromiseFingerprintChangesWithContract(t *testing.T) {
	promises, issues := ParsePromises(validPromiseSpec)
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	first, err := PromiseFingerprint(promises[0])
	if err != nil {
		t.Fatal(err)
	}

	changed := strings.Replace(validPromiseSpec, "token: {type: string, required: true}", "access_token: {type: string, required: true}", 1)
	promises, issues = ParsePromises(changed)
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	second, err := PromiseFingerprint(promises[0])
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("a contract change must change the fingerprint")
	}
}

func TestValidatePromiseContractKinds(t *testing.T) {
	field := map[string]any{"type": "string", "required": true}
	tests := []struct {
		name     string
		kind     string
		contract map[string]any
		want     string
	}{
		{
			name: "request response valid",
			kind: "request_response",
			contract: map[string]any{
				"request":  map[string]any{"username": field},
				"response": map[string]any{"token": field},
				"errors":   map[string]any{"401": "INVALID_CREDENTIALS"},
				"timing":   map[string]any{"mode": "sync"},
			},
		},
		{
			name:     "request response empty response",
			kind:     "request_response",
			contract: map[string]any{"response": map[string]any{}},
			want:     "response must be a non-empty field map",
		},
		{
			name: "event valid",
			kind: "event",
			contract: map[string]any{
				"name":    "order.created",
				"payload": map[string]any{"id": field},
			},
		},
		{
			name:     "event missing name",
			kind:     "event",
			contract: map[string]any{"payload": map[string]any{"id": field}},
			want:     "event requires contract.name",
		},
		{
			name: "data schema valid",
			kind: "data_schema",
			contract: map[string]any{
				"fields": map[string]any{"id": field},
			},
		},
		{
			name:     "data schema empty fields",
			kind:     "data_schema",
			contract: map[string]any{"fields": map[string]any{}},
			want:     "data_schema requires a non-empty contract.fields map",
		},
		{
			name: "state machine valid",
			kind: "state_machine",
			contract: map[string]any{
				"states":  []any{"pending", "paid"},
				"initial": "pending",
				"transitions": []any{
					map[string]any{"from": "pending", "event": "pay", "to": "paid"},
				},
			},
		},
		{
			name: "state machine initial not in states",
			kind: "state_machine",
			contract: map[string]any{
				"states":  []any{"pending", "paid"},
				"initial": "missing",
				"transitions": []any{
					map[string]any{"from": "pending", "event": "pay", "to": "paid"},
				},
			},
			want: "initial state is not in states",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issues := validatePromiseContract("promise", tc.kind, tc.contract)
			if tc.want == "" {
				if len(issues) > 0 {
					t.Fatalf("unexpected issues: %v", issues)
				}
				return
			}
			if !strings.Contains(strings.Join(issues, "; "), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, issues)
			}
		})
	}
}

func TestOpaquePromiseRequiresText(t *testing.T) {
	content := strings.Replace(validPromiseSpec, `    kind: request_response
    summary: Login returns a token on success and 401 on invalid credentials.
    contract:
      request:
        username: {type: string, required: true}
        password: {type: string, required: true}
      response:
        token: {type: string, required: true}
      errors:
        401: INVALID_CREDENTIALS
      timing:
        mode: sync`, `    kind: opaque
    opaque_reason: no structured kind applies
    summary: Passwords are never stored in plaintext.
    contract: {}`, 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "opaque promise requires contract.text") {
		t.Fatalf("expected opaque text failure, got %s — %s", result.Status, result.Details)
	}
}

func opaqueSpec(reason, text string) string {
	return `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

Passwords are hashed before storage.

## Promises

promises:
  - id: auth.secret_policy
    kind: opaque
    summary: Passwords are never stored in plaintext.
` + reason + `    contract: {text: ` + text + `}

## Acceptance Items

acceptance_item_set:
  - id: auth.secret
    promise: auth.secret_policy
    description: Given a stored password, when read, then it is hashed.
    verification_type: inspectable
    verification_surface: api
    implementation_surface: auth/password.go
    verification_method: inspect.
    pass_condition: Hashed.
    runnable: yes
`
}

func TestOpaquePromiseRequiresReason(t *testing.T) {
	result := CheckPromiseSchema(opaqueSpec("", "hashed"), "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "opaque_reason") {
		t.Fatalf("expected a missing opaque_reason failure, got %s — %s", result.Status, result.Details)
	}
}

func TestOpaqueReasonRejectedOnStructuredKind(t *testing.T) {
	content := strings.Replace(validPromiseSpec,
		"    kind: request_response\n",
		"    kind: request_response\n    opaque_reason: misplaced\n", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "only allowed on kind: opaque") {
		t.Fatalf("expected a misplaced opaque_reason failure, got %s — %s", result.Status, result.Details)
	}
}

func TestOpaqueReasonIsNotFingerprinted(t *testing.T) {
	first, issues := ParsePromises(opaqueSpec("    opaque_reason: first reason\n", "hashed"))
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	second, issues := ParsePromises(opaqueSpec("    opaque_reason: rewritten reason\n", "hashed"))
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	third, issues := ParsePromises(opaqueSpec("    opaque_reason: first reason\n", "hashed with salt"))
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	fp := func(promises []Promise) string {
		fingerprint, err := PromiseFingerprint(promises[0])
		if err != nil {
			t.Fatal(err)
		}
		return fingerprint
	}
	if fp(first) != fp(second) {
		t.Fatal("changing opaque_reason must not change the fingerprint")
	}
	if fp(first) == fp(third) {
		t.Fatal("changing opaque contract text must change the fingerprint")
	}
}

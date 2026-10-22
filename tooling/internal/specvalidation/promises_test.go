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
    summary: Login returns a token on success and 401 on invalid credentials.
    contract:
      operations:
        login:
          inputs:
            username: {type: string, required: true}
            password: {type: string, required: true}
          outputs:
            token: {type: string, required: true}
          errors:
            401: INVALID_CREDENTIALS
          timing: sync

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
	content := strings.Replace(validPromiseSpec, "    contract:\n", "    contract_v2: {operations: {login: {signature: login()}}}\n    contract:\n", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "unknown top-level field") {
		t.Fatalf("expected an unknown-top-level-field failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseSchemaRejectsRemovedKindField(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    summary:", "    kind: request_response\n    summary:", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "unknown top-level field") {
		t.Fatalf("expected the removed kind field to fail, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseSchemaRejectsUnknownContractBlock(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "    contract:\n      operations:", "    contract:\n      implementation_surface: auth/login.go\n      operations:", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "unknown contract block") {
		t.Fatalf("expected an unknown-contract-block failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseSchemaRejectsForeignOrDuplicateIDs(t *testing.T) {
	content := strings.Replace(validPromiseSpec, "id: auth.login_api", "id: web.login_api", 1)
	result := CheckPromiseSchema(content, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "must start with") {
		t.Fatalf("expected foreign-id failure, got %s — %s", result.Status, result.Details)
	}

	duplicate := strings.Replace(validPromiseSpec, "promises:\n", "promises:\n  - id: auth.login_api\n    summary: duplicate\n    contract: {invariants: [duplicate]}\n", 1)
	result = CheckPromiseSchema(duplicate, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "duplicate") {
		t.Fatalf("expected duplicate-id failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseSchemaRejectsSameUnitDependency(t *testing.T) {
	content := `## Promises

promises:
  - id: solo.base
    summary: Base contract.
    contract: {invariants: [base]}
  - id: solo.derived
    summary: Derived contract.
    contract: {invariants: [derived]}
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
    summary: Session.
    contract: {invariants: [session]}
    depends_on: [auth.login_api]
  - id: web.billing_api
    summary: Billing.
    contract: {invariants: [billing]}
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

const callableSurfaceSpec = `---
id: logger
rule_refs: none
---

# Logger

## Design Narrative

Logger is the logging seam every unit writes through.

## Promises

promises:
  - id: logger.interface
    summary: Logger is the logging seam.
    contract:
      operations:
        Logger.Debug: {signature: "Debug(msg string, args ...any)"}
        Logger.Info: {signature: "Info(msg string, args ...any)"}
      invariants:
        - safe for concurrent use

## Acceptance Items

acceptance_item_set:
  - id: logger.debug
    promise: logger.interface
    description: Given a logger, when Debug is called, then the record is written.
    verification_type: testable
    verification_surface: api
    implementation_surface: logger/logger.go
    verification_method: Go unit test for Debug.
    pass_condition: The record is written.
    runnable: yes
`

func TestCallableSurfaceFingerprint(t *testing.T) {
	for _, check := range CheckPromiseSpec(callableSurfaceSpec, "logger") {
		if check.Status != Pass {
			t.Fatalf("%s: %s — %s", check.Name, check.Status, check.Details)
		}
	}
	fingerprint := func(spec string) string {
		t.Helper()
		promises, issues := ParsePromises(spec)
		if len(issues) > 0 {
			t.Fatal(strings.Join(issues, "; "))
		}
		value, err := PromiseFingerprint(promises[0])
		if err != nil {
			t.Fatal(err)
		}
		return value
	}

	base := fingerprint(callableSurfaceSpec)
	summaryChanged := fingerprint(strings.Replace(callableSurfaceSpec, "summary: Logger is the logging seam.", "summary: Reworded summary.", 1))
	if base != summaryChanged {
		t.Fatal("a summary edit must not change a callable-surface fingerprint")
	}
	signatureChanged := fingerprint(strings.Replace(callableSurfaceSpec, `signature: "Debug(msg string, args ...any)"`, `signature: "Debug(msg string)"`, 1))
	if base == signatureChanged {
		t.Fatal("a signature change must change a callable-surface fingerprint")
	}
	invariantChanged := fingerprint(strings.Replace(callableSurfaceSpec, "safe for concurrent use", "safe for concurrent use and safe to copy", 1))
	if base == invariantChanged {
		t.Fatal("an invariant change must change a callable-surface fingerprint")
	}
}

func TestValidateContractBlocks(t *testing.T) {
	field := map[string]any{"type": "string", "required": true}
	tests := []struct {
		name     string
		contract map[string]any
		want     string
	}{
		{
			name: "operations valid",
			contract: map[string]any{
				"operations": map[string]any{
					"login": map[string]any{
						"inputs":  map[string]any{"username": field},
						"outputs": map[string]any{"token": field},
						"errors":  map[string]any{"401": "INVALID_CREDENTIALS"},
						"timing":  "sync",
					},
				},
			},
		},
		{
			name:     "operations empty",
			contract: map[string]any{"operations": map[string]any{}},
			want:     "operations must be a non-empty map",
		},
		{
			name:     "operation not a map",
			contract: map[string]any{"operations": map[string]any{"login": "login()"}},
			want:     `operation "login" must be a map`,
		},
		{
			name: "operation missing shape",
			contract: map[string]any{
				"operations": map[string]any{"login": map[string]any{"errors": map[string]any{"401": "INVALID_CREDENTIALS"}}},
			},
			want: `operation "login" requires at least one of signature, inputs, or outputs`,
		},
		{
			name: "operation unknown field",
			contract: map[string]any{
				"operations": map[string]any{"login": map[string]any{"signature": "login()", "returns": "error"}},
			},
			want: `operation "login" has unknown field "returns"`,
		},
		{
			name: "operation inputs not a map",
			contract: map[string]any{
				"operations": map[string]any{"login": map[string]any{"signature": "login()", "inputs": "username"}},
			},
			want: `operation "login" inputs must be a field map`,
		},
		{
			name: "data valid",
			contract: map[string]any{
				"data": map[string]any{"LoginRequest": map[string]any{"fields": map[string]any{"username": field}}},
			},
		},
		{
			name:     "data empty",
			contract: map[string]any{"data": map[string]any{}},
			want:     "data must be a non-empty map",
		},
		{
			name:     "data missing fields",
			contract: map[string]any{"data": map[string]any{"LoginRequest": map[string]any{}}},
			want:     `data "LoginRequest" requires a non-empty fields map`,
		},
		{
			name: "data unknown field",
			contract: map[string]any{
				"data": map[string]any{"LoginRequest": map[string]any{"fields": map[string]any{"id": field}, "encoding": "json"}},
			},
			want: `data "LoginRequest" has unknown field "encoding"`,
		},
		{
			name: "occurrences valid",
			contract: map[string]any{
				"occurrences": map[string]any{
					"order.created": map[string]any{
						"payload":  map[string]any{"id": field},
						"delivery": "at_least_once",
						"ordering": "per_key",
					},
				},
			},
		},
		{
			name: "occurrence missing payload",
			contract: map[string]any{
				"occurrences": map[string]any{"order.created": map[string]any{"delivery": "at_least_once"}},
			},
			want: `occurrence "order.created" requires a payload field map`,
		},
		{
			name: "occurrence unknown field",
			contract: map[string]any{
				"occurrences": map[string]any{"order.created": map[string]any{"payload": map[string]any{}, "retries": 3}},
			},
			want: `occurrence "order.created" has unknown field "retries"`,
		},
		{
			name: "state machine valid",
			contract: map[string]any{
				"state_machine": map[string]any{
					"states":  []any{"pending", "paid"},
					"initial": "pending",
					"transitions": []any{
						map[string]any{"from": "pending", "event": "pay", "to": "paid"},
					},
				},
			},
		},
		{
			name: "state machine unknown field",
			contract: map[string]any{
				"state_machine": map[string]any{
					"states":  []any{"pending", "paid"},
					"initial": "pending",
					"transitions": []any{
						map[string]any{"from": "pending", "event": "pay", "to": "paid"},
					},
					"timeout": "30s",
				},
			},
			want: `state_machine has unknown field "timeout"`,
		},
		{
			name: "state machine initial not in states",
			contract: map[string]any{
				"state_machine": map[string]any{
					"states":  []any{"pending", "paid"},
					"initial": "missing",
					"transitions": []any{
						map[string]any{"from": "pending", "event": "pay", "to": "paid"},
					},
				},
			},
			want: "initial state is not in states",
		},
		{
			name:     "invariants valid",
			contract: map[string]any{"invariants": []any{"safe for concurrent use"}},
		},
		{
			name:     "invariants not strings",
			contract: map[string]any{"invariants": []any{42}},
			want:     "invariants must be a non-empty string list",
		},
		{
			name:     "invariants empty",
			contract: map[string]any{"invariants": []any{}},
			want:     "invariants must be a non-empty string list",
		},
		{
			name:     "unknown block",
			contract: map[string]any{"request": map[string]any{"username": field}},
			want:     `unknown contract block "request"`,
		},
		{
			name: "combination of blocks",
			contract: map[string]any{
				"operations": map[string]any{
					"login": map[string]any{"signature": "login(username, password) (Token, error)"},
				},
				"invariants": []any{"safe for concurrent use"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issues := validateContractBlocks("promise", tc.contract)
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

func TestInvariantOnlyContractIsValid(t *testing.T) {
	result := CheckPromiseSchema(invariantSpec("hashed"), "auth")
	if result.Status != Pass {
		t.Fatalf("expected an invariant-only contract to pass, got %s — %s", result.Status, result.Details)
	}
}

func invariantSpec(text string) string {
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
    summary: Passwords are never stored in plaintext.
    contract:
      invariants:
        - ` + text + `

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

func TestInvariantChangeIsFingerprinted(t *testing.T) {
	fingerprint := func(spec string) string {
		t.Helper()
		promises, issues := ParsePromises(spec)
		if len(issues) > 0 {
			t.Fatal(strings.Join(issues, "; "))
		}
		value, err := PromiseFingerprint(promises[0])
		if err != nil {
			t.Fatal(err)
		}
		return value
	}

	base := fingerprint(invariantSpec("hashed"))
	summaryChanged := fingerprint(strings.Replace(invariantSpec("hashed"), "summary: Passwords are never stored in plaintext.", "summary: Reworded summary.", 1))
	if base != summaryChanged {
		t.Fatal("a summary edit must not change an invariant-only fingerprint")
	}
	invariantChanged := fingerprint(invariantSpec("hashed with salt"))
	if base == invariantChanged {
		t.Fatal("an invariant change must change the fingerprint")
	}
}

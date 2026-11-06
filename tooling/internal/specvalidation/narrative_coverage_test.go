package specvalidation

import (
	"strings"
	"testing"
)

const narrativeCoverageSpec = `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

### Authentication model
Email/password login issues a token.

### Background
This block explains why the module exists.

## Promises

promises:
- id: auth.login_api
  summary: Login returns a token.
  source: [Authentication model]
  contract:
    operations:
      login:
        outputs:
          token: {type: string}

## Acceptance Items

acceptance_item_set:
- id: AUTH-AC-001
  promise: auth.login_api
  source: [Authentication model]
  description: Given a user, When login, Then a token.
  pass_condition: 200 with a token.
`

func TestNarrativeCoverageFailsOnUnsourcedBlock(t *testing.T) {
	result := CheckNarrativeCoverage(narrativeCoverageSpec)
	if result.Status != Fail {
		t.Fatalf("status = %s, want FAIL (%s)", result.Status, result.Details)
	}
	if !strings.Contains(result.Details, "Background") {
		t.Fatalf("details should name the unsourced block: %s", result.Details)
	}
}

func TestNarrativeCoverageAcceptsNonBehaviorBlock(t *testing.T) {
	content := strings.Replace(narrativeCoverageSpec, "rule_refs: none\n", "rule_refs: none\nnon_behavior_blocks: [Background]\n", 1)
	result := CheckNarrativeCoverage(content)
	if result.Status != Pass {
		t.Fatalf("status = %s, want PASS (%s)", result.Status, result.Details)
	}
}

func TestNarrativeCoverageAcceptsSourcedBlock(t *testing.T) {
	content := strings.Replace(narrativeCoverageSpec, "source: [Authentication model]\n  contract:", "source: [Authentication model, Background]\n  contract:", 1)
	result := CheckNarrativeCoverage(content)
	if result.Status != Pass {
		t.Fatalf("status = %s, want PASS (%s)", result.Status, result.Details)
	}
}

func TestNarrativeCoverageIgnoresContainer(t *testing.T) {
	content := `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

### Group

#### Authentication model
Email/password login issues a token.

## Promises

promises:
- id: auth.login_api
  summary: Login returns a token.
  source: [Authentication model]
  contract:
    operations:
      login:
        outputs:
          token: {type: string}

## Acceptance Items

acceptance_item_set:
- id: AUTH-AC-001
  promise: auth.login_api
  source: [Authentication model]
  description: Given a user, When login, Then a token.
  pass_condition: 200 with a token.
`
	result := CheckNarrativeCoverage(content)
	if result.Status != Pass {
		t.Fatalf("container block should not be required to carry a source: %s", result.Details)
	}
}

func TestNarrativeCoverageInheritsAncestorSource(t *testing.T) {
	content := `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

### Group
Group overview.

#### Child
Child behavior details.

## Promises

promises:
- id: auth.login_api
  summary: Login returns a token.
  source: [Group]
  contract:
    operations:
      login:
        outputs:
          token: {type: string}

## Acceptance Items

acceptance_item_set:
- id: AUTH-AC-001
  promise: auth.login_api
  source: [Group]
  description: Given a user, When login, Then a token.
  pass_condition: 200 with a token.
`
	result := CheckNarrativeCoverage(content)
	if result.Status != Pass {
		t.Fatalf("a child block is covered by its sourced ancestor: %s", result.Details)
	}
}

func TestNarrativeCoverageDoesNotInheritNonBehavior(t *testing.T) {
	content := `---
id: auth
rule_refs: none
non_behavior_blocks: [Group]
---

# Auth

## Design Narrative

### Group
Group overview.

#### Child
Child behavior details.

## Promises

promises:
- id: auth.login_api
  summary: Login returns a token.
  source: [Elsewhere]
  contract:
    operations:
      login:
        outputs:
          token: {type: string}

## Acceptance Items

acceptance_item_set:
- id: AUTH-AC-001
  promise: auth.login_api
  source: [Elsewhere]
  description: Given a user, When login, Then a token.
  pass_condition: 200 with a token.
`
	result := CheckNarrativeCoverage(content)
	if result.Status != Fail || !strings.Contains(result.Details, "Child") {
		t.Fatalf("non_behavior must be exact; expected a failure naming Child, got %s: %s", result.Status, result.Details)
	}
}

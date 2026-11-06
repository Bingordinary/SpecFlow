package validatekeys

import (
	"strings"
	"testing"
)

const fixtureSpec = `---
id: auth
rule_refs: [b_rule_x]
non_behavior_blocks:
  - "Why this design"
---

# Auth

## Design Narrative

### Authentication model
Email/password login issues a token.

### Why this design
Background rationale.

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
- id: auth.session
  summary: Session refresh.
  source: [Authentication model]
  contract:
    operations:
      refresh:
        outputs:
          token: {type: string}

## Acceptance Items

acceptance_item_set:
- id: AUTH-AC-001
  promise: auth.login_api
  source: [Authentication model]
  affects:
    files:
    - src/auth/login.go
    rules:
    - b_rule_x
- id: AUTH-AC-002
  promise: auth.session
  source: [Authentication model]
  affects:
    files:
    - src/auth/session.go
`

func TestParseGraph(t *testing.T) {
	g, err := Parse("auth", fixtureSpec)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(g.Promises) != 2 {
		t.Fatalf("promises = %d, want 2", len(g.Promises))
	}
	if len(g.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(g.Items))
	}
	if !g.NonBehavior["Why this design"] {
		t.Fatalf("non_behavior_blocks was not parsed: %+v", g.NonBehavior)
	}
	if got := g.Items[0].Sources; len(got) != 1 || got[0] != "Authentication model" {
		t.Fatalf("item sources = %v", got)
	}
	if got := g.Items[0].Files; len(got) != 1 || got[0] != "src/auth/login.go" {
		t.Fatalf("item files = %v", got)
	}
	if got := g.Items[0].Rules; len(got) != 1 || got[0] != "b_rule_x" {
		t.Fatalf("item rules = %v", got)
	}
	if len(g.UnresolvedRefs) != 0 {
		t.Fatalf("unresolved refs = %v", g.UnresolvedRefs)
	}
}

func TestCatalogKeysAndReads(t *testing.T) {
	g, err := Parse("auth", fixtureSpec)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	catalog := BuildCatalog(g, CatalogInput{Unit: "auth"}, contract)
	byID := map[string]Key{}
	for _, k := range catalog.Keys {
		byID[k.ID] = k
	}

	want := []string{
		"item:AUTH-AC-001:substance",
		"item:AUTH-AC-001:promise",
		"item:AUTH-AC-001:source",
		"item:AUTH-AC-001:affects",
		"item:AUTH-AC-002:substance",
		"contradiction:AUTH-AC-001:AUTH-AC-002",
		"promise:auth.login_api:contract",
		"promise:auth.login_api:source",
		"promise:auth.login_api:coverage",
		"section:Authentication model:design",
		"section:Authentication model:clarity",
		"section:Authentication model:structure",
		"rule:b_rule_x:section:Authentication model",
		"file:src/auth/login.go",
		"global:graph",
		"global:scope",
		"global:goal-means",
		"global:narrative-coherence",
		"global:cross-behavior-interactions",
	}
	for _, id := range want {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing key %q", id)
		}
	}

	checkRead := func(id string, reads ...string) {
		t.Helper()
		k, ok := byID[id]
		if !ok {
			t.Fatalf("missing key %q", id)
		}
		for _, read := range reads {
			if !contains(k.Reads, read) {
				t.Fatalf("key %q reads %v, want %q", id, k.Reads, read)
			}
		}
	}
	checkRead("item:AUTH-AC-001:promise", "item:AUTH-AC-001", "promise:auth.login_api")
	checkRead("item:AUTH-AC-001:source", "item:AUTH-AC-001", "section:Authentication model")
	checkRead("item:AUTH-AC-001:affects", "item:AUTH-AC-001", "file:src/auth/login.go", "rule:b_rule_x")
	checkRead("section:Authentication model:design", "section:Authentication model", "item:AUTH-AC-001", "promise:auth.login_api")
	checkRead("contradiction:AUTH-AC-001:AUTH-AC-002", "item:AUTH-AC-001", "item:AUTH-AC-002")
}

func TestParseRejectsUnresolvedSource(t *testing.T) {
	content := strings.Replace(fixtureSpec, "source: [Authentication model]", "source: [No Such Block]", 1)
	g, err := Parse("auth", content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(g.UnresolvedRefs) != 1 || g.UnresolvedRefs[0] != "No Such Block" {
		t.Fatalf("unresolved refs = %v", g.UnresolvedRefs)
	}
}

func TestParseRejectsDuplicateItemID(t *testing.T) {
	content := strings.Replace(fixtureSpec, "- id: AUTH-AC-002", "- id: AUTH-AC-001", 1)
	if _, err := Parse("auth", content); err == nil {
		t.Fatal("expected duplicate item id error")
	}
}

func TestParseNonBehaviorInlineList(t *testing.T) {
	content := strings.Replace(fixtureSpec, "non_behavior_blocks:\n  - \"Why this design\"", "non_behavior_blocks: [\"Why this design\"]", 1)
	g, err := Parse("auth", content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !g.NonBehavior["Why this design"] {
		t.Fatalf("inline non_behavior_blocks was not parsed: %+v", g.NonBehavior)
	}
}

// TestCatalogWholeUnitJudgmentsReadEvidence pins the evidence-record input of
// the whole-unit judgments that the checklist says must include it: goal-means,
// cross-behavior interactions, scope, and acceptance coverage.
func TestCatalogWholeUnitJudgmentsReadEvidence(t *testing.T) {
	g, err := Parse("auth", fixtureSpec)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	catalog := BuildCatalog(g, CatalogInput{Unit: "auth", EvidenceBase: "unit_auth"}, contract)
	byID := map[string]Key{}
	for _, key := range catalog.Keys {
		byID[key.ID] = key
	}
	for _, id := range []string{"global:goal-means", "global:cross-behavior-interactions", "global:scope", "global:acceptance-coverage"} {
		key, ok := byID[id]
		if !ok {
			t.Fatalf("missing key %q", id)
		}
		if !contains(key.Reads, "evidence:unit_auth") {
			t.Fatalf("%s reads %v, want evidence:unit_auth", id, key.Reads)
		}
	}
}

// TestCatalogPairsItemsBySurfaceAndFile pins the Check 5b relation sources:
// items that share only a verification surface or an affects.files entry are
// still compared for contradictions, because the checklist requires that
// comparison even when they share no promise or source.
func TestCatalogPairsItemsBySurfaceAndFile(t *testing.T) {
	content := `---
id: auth
---
# Auth

## Design Narrative
### Login
Login behavior.

## Promises
promises:
- id: auth.login_api
  summary: Login returns a token.
  source: [Login]
  contract:
    operations:
      login:
        outputs:
          token: {type: string}

## Acceptance Items
acceptance_item_set:
- id: AUTH-AC-001
  promise: auth.login_api
  source: [Login]
  verification_surface: api
  affects:
    files:
    - src/auth/login.go
- id: AUTH-AC-002
  source: [Login]
  verification_surface: api
  affects:
    files:
    - src/auth/login.go
`
	g, err := Parse("auth", content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if g.Items[0].VerificationSurface != "api" || g.Items[1].VerificationSurface != "api" {
		t.Fatalf("verification surfaces = %q, %q", g.Items[0].VerificationSurface, g.Items[1].VerificationSurface)
	}
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, key := range BuildCatalog(g, CatalogInput{Unit: "auth"}, contract).Keys {
		if key.ID == "contradiction:AUTH-AC-001:AUTH-AC-002" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("items sharing a verification surface and an affects.files entry have no contradiction object")
	}
}

// TestCatalogHolisticReadsWholeUnit pins the dependency surfaces of the
// decomposed rule check and the still-holistic scope check: a rule × section
// object reads only that section, while scope integrity reads the whole unit.
func TestCatalogHolisticReadsWholeUnit(t *testing.T) {
	g, err := Parse("auth", fixtureSpec)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Key{}
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range BuildCatalog(g, CatalogInput{Unit: "auth"}, contract).Keys {
		byID[key.ID] = key
	}
	rule, ok := byID["rule:b_rule_x:section:Authentication model"]
	if !ok {
		t.Fatal("rule:b_rule_x:section:Authentication model missing")
	}
	if contains(rule.Reads, "unit:auth") {
		t.Fatalf("a section-local rule object must not read the whole unit, got %v", rule.Reads)
	}
	if !contains(rule.Reads, "section:Authentication model") {
		t.Fatalf("rule object must read its section, got %v", rule.Reads)
	}
	scope, ok := byID["global:scope"]
	if !ok {
		t.Fatal("global:scope (Check 3) missing")
	}
	if !contains(scope.Reads, "unit:auth") {
		t.Fatalf("scope object must read the whole unit design, got %v", scope.Reads)
	}
}

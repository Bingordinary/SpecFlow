package specvalidation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePromiseGraphUnit(t *testing.T, repoRoot, layer, unit, promises string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + unit + "\n---\n\n## Promises\n\n" + promises + "\n"
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unit+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPromiseGraphResolvesDependencies(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "candidate", "auth", `promises:
  - id: auth.login_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Login.
    contract: {text: login contract}`)
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Session.
    contract: {text: session contract}
    depends_on: [auth.login_api]`)

	result := CheckPromiseGraph(root, "web")
	if result.Status != Pass {
		t.Fatalf("expected PASS, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseGraphScopeSeparatesLayers(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "stable", "auth", `promises:
  - id: auth.login_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Stable login.
    contract: {text: stable login}`)
	writePromiseGraphUnit(t, root, "candidate", "auth", `promises:
  - id: auth.login_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Candidate login.
    contract: {text: candidate login}`)
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Session.
    contract: {text: session}
    depends_on: [auth.login_api]`)

	candidateEdges, issues := PromiseEdges(root, PromiseScopeCandidate)
	if len(issues) > 0 {
		t.Fatalf("candidate issues: %v", issues)
	}
	if len(candidateEdges) != 1 || candidateEdges[0].Unit != "web" || candidateEdges[0].To != "auth.login_api" {
		t.Fatalf("candidate edges = %+v", candidateEdges)
	}
	stableEdges, issues := PromiseEdges(root, PromiseScopeStable)
	if len(issues) > 0 {
		t.Fatalf("stable issues: %v", issues)
	}
	if len(stableEdges) != 0 {
		t.Fatalf("stable scope must not include candidate promise edges: %+v", stableEdges)
	}

	candidateFP, _ := PromiseFingerprintsForScope(root, PromiseScopeCandidate)
	stableFP, _ := PromiseFingerprintsForScope(root, PromiseScopeStable)
	if candidateFP["auth.login_api"] == stableFP["auth.login_api"] {
		t.Fatalf("candidate and stable fingerprints should differ: %v", candidateFP)
	}
}

func TestPromiseProviderUnits(t *testing.T) {
	content := "## Promises\n\npromises:\n  - id: web.a\n    kind: opaque\n    opaque_reason: no structured kind applies\n    summary: A.\n    contract: {text: a}\n    depends_on: [auth.login_api, common.error_schema]\n  - id: web.b\n    kind: opaque\n    opaque_reason: no structured kind applies\n    summary: B.\n    contract: {text: b}\n    depends_on: [auth.login_api]\n"
	got := PromiseProviderUnits(content)
	want := []string{"auth", "common"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("provider units = %v, want %v", got, want)
	}
	if provider := PromiseProvider("auth.login_api"); provider != "auth" {
		t.Fatalf("provider = %q", provider)
	}
}

func TestPromiseGraphRejectsUnresolvedDependency(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Session.
    contract: {text: session contract}
    depends_on: [auth.missing]`)

	result := CheckPromiseGraph(root, "web")
	if result.Status != Fail || !strings.Contains(result.Details, "unresolved depends_on") {
		t.Fatalf("expected unresolved failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseGraphRejectsCycle(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "candidate", "auth", `promises:
  - id: auth.a
    kind: opaque
    opaque_reason: no structured kind applies
    summary: A.
    contract: {text: a}
    depends_on: [web.b]`)
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: web.b
    kind: opaque
    opaque_reason: no structured kind applies
    summary: B.
    contract: {text: b}
    depends_on: [auth.a]`)
	writePromiseGraphUnit(t, root, "candidate", "other", `promises:
  - id: other.c
    kind: opaque
    opaque_reason: no structured kind applies
    summary: C.
    contract: {text: c}`)

	result := CheckPromiseGraph(root, "auth")
	if result.Status != Fail || !strings.Contains(result.Details, "cycle") {
		t.Fatalf("expected cycle failure, got %s — %s", result.Status, result.Details)
	}

	// A cycle blocks only the units that own a promise on it. An unrelated
	// unit's contract does not depend on the cycle.
	unrelated := CheckPromiseGraph(root, "other")
	if unrelated.Status != Pass {
		t.Fatalf("unrelated unit must not fail on another unit's cycle, got %s — %s", unrelated.Status, unrelated.Details)
	}
}

func TestPromiseGraphUnrelatedMissingSectionDoesNotBlock(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Session.
    contract: {text: session}`)
	legacyDir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.WriteFile(filepath.Join(legacyDir, "unit_legacy.md"), []byte("---\nid: legacy\nunit_refs: none\n---\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if result := CheckPromiseGraph(root, "web"); result.Status != Pass {
		t.Fatalf("an unrelated unit without ## Promises must not fail web, got %s — %s", result.Status, result.Details)
	}
	if result := CheckPromiseGraph(root, "legacy"); result.Status != Fail || !strings.Contains(result.Details, "## Promises section not found") {
		t.Fatalf("the legacy unit itself must fail, got %s — %s", result.Status, result.Details)
	}
	if _, issues := PromiseFingerprintsForScope(root, PromiseScopeAll); len(issues) != 0 {
		t.Fatalf("unit-scoped parse issues must not fail repository fingerprints, got %v", issues)
	}
}

func TestPromiseGraphConsumerOfMissingSectionFails(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Session.
    contract: {text: session}
    depends_on: [legacy.api]`)
	writePromiseGraphUnit(t, root, "candidate", "other", `promises:
  - id: other.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Other.
    contract: {text: other}`)
	dir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.WriteFile(filepath.Join(dir, "unit_legacy.md"), []byte("---\nid: legacy\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if result := CheckPromiseGraph(root, "web"); result.Status != Fail || !strings.Contains(result.Details, "unresolved depends_on") {
		t.Fatalf("a consumer of a unit with no promise section must fail unresolved, got %s — %s", result.Status, result.Details)
	}
	if result := CheckPromiseGraph(root, "other"); result.Status != Pass {
		t.Fatalf("an unrelated unit must not fail on the consumer's unresolved dependency, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseGraphRejectsDuplicateIDAcrossUnits(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "candidate", "auth", `promises:
  - id: shared.contract
    kind: opaque
    opaque_reason: no structured kind applies
    summary: A.
    contract: {text: a}`)
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: shared.contract
    kind: opaque
    opaque_reason: no structured kind applies
    summary: B.
    contract: {text: b}`)

	result := CheckPromiseGraph(root, "web")
	if result.Status != Fail || !strings.Contains(result.Details, "duplicate promise id") {
		t.Fatalf("expected duplicate failure, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseGraphCandidateOverridesStable(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "stable", "auth", `promises:
  - id: auth.login_v1
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Old.
    contract: {text: old}`)
	writePromiseGraphUnit(t, root, "candidate", "auth", `promises:
  - id: auth.login_v2
    kind: opaque
    opaque_reason: no structured kind applies
    summary: New.
    contract: {text: new}`)
	writePromiseGraphUnit(t, root, "candidate", "web", `promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Session.
    contract: {text: session}
    depends_on: [auth.login_v2]`)

	result := CheckPromiseGraph(root, "web")
	if result.Status != Pass {
		t.Fatalf("expected candidate promise to resolve, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseGraphRejectsEveryCycle(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "candidate", "a", `promises:
  - id: a.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: A.
    contract: {text: a}
    depends_on: [b.api]`)
	writePromiseGraphUnit(t, root, "candidate", "b", `promises:
  - id: b.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: B.
    contract: {text: b}
    depends_on: [a.api]`)
	writePromiseGraphUnit(t, root, "candidate", "c", `promises:
  - id: c.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: C.
    contract: {text: c}
    depends_on: [d.api]`)
	writePromiseGraphUnit(t, root, "candidate", "d", `promises:
  - id: d.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: D.
    contract: {text: d}
    depends_on: [c.api]`)
	writePromiseGraphUnit(t, root, "candidate", "solo", `promises:
  - id: solo.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Solo.
    contract: {text: solo}`)

	// Every unit on either disjoint cycle fails, not just the members of the
	// first cycle the scan happens to reach.
	for _, unit := range []string{"a", "b", "c", "d"} {
		result := CheckPromiseGraph(root, unit)
		if result.Status != Fail || !strings.Contains(result.Details, "cycle") {
			t.Fatalf("unit %s: expected cycle failure, got %s — %s", unit, result.Status, result.Details)
		}
	}
	if result := CheckPromiseGraph(root, "solo"); result.Status != Pass {
		t.Fatalf("unrelated unit must not fail, got %s — %s", result.Status, result.Details)
	}
}

func TestPromiseGraphScopeForLayer(t *testing.T) {
	root := t.TempDir()
	writePromiseGraphUnit(t, root, "stable", "auth", `promises:
  - id: auth.login_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Stable login.
    contract: {text: stable login}`)
	writePromiseGraphUnit(t, root, "candidate", "auth", `promises:
  - id: auth.login_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Candidate login.
    contract: {text: candidate login}`)
	writePromiseGraphUnit(t, root, "stable", "web", `promises:
  - id: web.session_api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Session.
    contract: {text: session}
    depends_on: [auth.login_api]`)

	// A candidate resolves providers in the current layer; a stable unit
	// resolves them in the published stable layer. Both resolve here because
	// auth exists in both layers.
	if result := CheckPromiseGraphScope(root, PromiseScopeForLayer("candidate"), "web"); result.Status != Pass {
		t.Fatalf("candidate scope: %s — %s", result.Status, result.Details)
	}
	if result := CheckPromiseGraphScope(root, PromiseScopeForLayer("stable"), "web"); result.Status != Pass {
		t.Fatalf("stable scope: %s — %s", result.Status, result.Details)
	}

	// A provider that exists only as a candidate must not satisfy a stable
	// unit's declared dependency.
	writePromiseGraphUnit(t, root, "stable", "solo", `promises:
  - id: solo.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Solo.
    contract: {text: solo}
    depends_on: [draft.api]`)
	writePromiseGraphUnit(t, root, "candidate", "draft", `promises:
  - id: draft.api
    kind: opaque
    opaque_reason: no structured kind applies
    summary: Draft.
    contract: {text: draft}`)

	if result := CheckPromiseGraphScope(root, PromiseScopeForLayer("stable"), "solo"); result.Status != Fail {
		t.Fatalf("stable scope must not resolve a candidate-only provider, got %s — %s", result.Status, result.Details)
	}
	if result := CheckPromiseGraphScope(root, PromiseScopeForLayer("candidate"), "solo"); result.Status != Pass {
		t.Fatalf("candidate scope resolves the candidate provider, got %s — %s", result.Status, result.Details)
	}
}

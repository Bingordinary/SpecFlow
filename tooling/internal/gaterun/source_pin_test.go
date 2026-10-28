package gaterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// A verify item judgment pins the item's own region and the Design Narrative
// blocks the item declares as its source. An edit to a source block must
// invalidate the judgment even when the item text itself is unchanged; an edit
// to an unrelated narrative block must not.
func TestVerifyItemJudgmentPinsSourceBlock(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "src/web.go", "package web\n")
	specPath := "docs/specs/units/candidate/unit_web.md"
	spec := `---
id: web
rule_refs: none
---

# web

## Design Narrative

### Narrative

Narrative behavior.

### Other

Unrelated rationale.

## Promises

promises:
  - id: web.core
    summary: Core contract.
    source: [Narrative]
    contract: {invariants: [core]}

## Acceptance Items

acceptance_item_set:
  - id: web.core
    promise: web.core
    source: [Narrative]
    description: Given X, When Y, Then Z.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/web.go
    verification_method: test.
    pass_condition: Passes.
    runnable: yes
`
	writeFile(t, root, specPath, spec)

	run, err := Plan(root, GateVerify, TargetKindUnit, "web", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var item CoverageKey
	for _, ck := range run.Coverage {
		if ck.Kind == SessionKindItem {
			item = ck
			break
		}
	}
	if item.Key == "" {
		t.Fatalf("expected an item coverage key: %+v", run.Coverage)
	}
	result := SessionResult{
		Kind:            item.Kind,
		Verdicts:        map[string]string{item.Key: "ALIGNED"},
		EffectiveStatus: map[string]string{item.Key: "pass"},
	}
	ref, err := SaveJudgment(root, run, item, &result, "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := judgments.Load(root, ref)
	if err != nil {
		t.Fatal(err)
	}
	foundSource := false
	for _, dep := range record.Dependencies {
		for _, d := range dep.Deps {
			if strings.HasPrefix(d, "region:subsection:Narrative:") {
				foundSource = true
			}
		}
	}
	if !foundSource {
		t.Fatalf("item judgment does not pin its source block: %+v", record.Dependencies)
	}
	if err := judgments.Check(root, ref, TargetCandidate, run.Protocol); err != nil {
		t.Fatalf("fresh item judgment rejected: %v", err)
	}

	// Editing a non-source block must not invalidate the item.
	absSpec := filepath.Join(root, filepath.FromSlash(specPath))
	data, err := os.ReadFile(absSpec)
	if err != nil {
		t.Fatal(err)
	}
	otherEdited := strings.Replace(string(data), "Unrelated rationale.", "Unrelated rationale, edited.", 1)
	if otherEdited == string(data) {
		t.Fatal("non-source edit did not apply")
	}
	if err := os.WriteFile(absSpec, []byte(otherEdited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := judgments.Check(root, ref, TargetCandidate, run.Protocol); err != nil {
		t.Fatalf("an edit outside the item source must not invalidate the item: %v", err)
	}

	// Editing the declared source block must invalidate the item.
	sourceEdited := strings.Replace(otherEdited, "Narrative behavior.", "Narrative behavior, edited.", 1)
	if sourceEdited == otherEdited {
		t.Fatal("source edit did not apply")
	}
	if err := os.WriteFile(absSpec, []byte(sourceEdited), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := judgments.Check(root, ref, TargetCandidate, run.Protocol); err == nil {
		t.Fatal("an edit inside the item source block must invalidate the item")
	}
}

func TestPlanRejectsContractItemWithoutSource(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "src/web.go", "package web\n")
	writeFile(t, root, "docs/specs/units/candidate/unit_web.md", `---
id: web
rule_refs: none
---

# web

## Design Narrative

### Narrative

Narrative behavior.

## Promises

promises:
  - id: web.core
    summary: Core contract.
    source: [Narrative]
    contract: {invariants: [core]}

## Acceptance Items

acceptance_item_set:
  - id: web.core
    promise: web.core
    description: Given X, When Y, Then Z.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/web.go
    verification_method: test.
    pass_condition: Passes.
    runnable: yes
`)
	_, err := Plan(root, GateVerify, TargetKindUnit, "web", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err == nil || !strings.Contains(err.Error(), "Acceptance item sources") {
		t.Fatalf("a contract item without source must reject the plan, got %v", err)
	}
}

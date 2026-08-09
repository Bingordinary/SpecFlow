package contenthash

import (
	"strings"
	"testing"
)

const specWithItems = `---
id: dep
version: 0.1.0
unit_refs: none
rule_refs: none
---

# Dep Unit

## Description

Background prose about the dependency unit.

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: dep.core
    description: The core behavior is provided.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/dep
    verification_method: test
    pass_condition: Core behavior passes.
    runnable: yes

## Dependencies

This unit has no external dependencies.
`

func TestAcceptanceItemsRegion(t *testing.T) {
	region, ok := AcceptanceItemsRegion(specWithItems)
	if !ok {
		t.Fatal("expected region to be found")
	}
	if !strings.HasPrefix(region, "acceptance_item_set:") {
		t.Fatalf("region must start at the marker, got:\n%s", region)
	}
	if strings.Contains(region, "## Dependencies") {
		t.Fatalf("region must end before the next heading, got:\n%s", region)
	}
	if !strings.Contains(region, "pass_condition: Core behavior passes.") {
		t.Fatalf("region must contain the item content, got:\n%s", region)
	}
}

func TestAcceptanceItemsRegionPositionIndependent(t *testing.T) {
	region1, ok := AcceptanceItemsRegion(specWithItems)
	if !ok {
		t.Fatal("expected region in original")
	}
	cid1 := RegionCID(region1)

	// Insert unrelated prose BEFORE the region: the region content is
	// unchanged and its CID must be identical.
	prefixed := strings.Replace(specWithItems, "## Description", "## Description\n\nA long paragraph added during editing, unrelated to the formal behavior.\n", 1)
	region2, ok := AcceptanceItemsRegion(prefixed)
	if !ok {
		t.Fatal("expected region after insertion")
	}
	if RegionCID(region2) != cid1 {
		t.Fatal("region CID must be position-independent")
	}

	// Editing inside the region changes the CID.
	edited := strings.Replace(specWithItems, "Core behavior passes.", "Core behavior passes faster.", 1)
	region3, _ := AcceptanceItemsRegion(edited)
	if RegionCID(region3) == cid1 {
		t.Fatal("region CID must change when region content changes")
	}
}

func TestAcceptanceItemsRegionMissing(t *testing.T) {
	if _, ok := AcceptanceItemsRegion("---\nid: x\n---\nno items here\n"); ok {
		t.Fatal("expected no region for content without the marker")
	}
}

func TestAcceptanceItemsRegionProseMentionNotMarker(t *testing.T) {
	// A prose sentence mentioning the marker before the real section must
	// not start the region — the region begins at the real marker line.
	spec := `---
id: dep
version: 0.1.0
unit_refs: none
rule_refs: none
---

## Description

This spec declares an acceptance_item_set: field in its Testability section.

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: dep.core
    description: Core behavior.
    pass_condition: Passes.
`
	region, ok := AcceptanceItemsRegion(spec)
	if !ok {
		t.Fatal("expected region to be found")
	}
	if !strings.HasPrefix(region, "acceptance_item_set:") {
		t.Fatalf("region must start at the marker line, not at the prose mention, got:\n%s", region)
	}
	if strings.Contains(region, "acceptance_item_set: field") {
		t.Fatalf("region must not include the prose mention, got:\n%s", region)
	}
}

func TestAcceptanceItemsRegionProseMentionAtLineStartNotMarker(t *testing.T) {
	// A prose line that STARTS with the marker text at line start (not in
	// the middle of a sentence) is still not the marker: the marker line is
	// exactly `acceptance_item_set:` and nothing else.
	spec := `---
id: dep
version: 0.1.0
unit_refs: none
rule_refs: none
---

## Description

acceptance_item_set: is declared under Testability

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: dep.core
    description: Core behavior.
    pass_condition: Passes.
`
	region, ok := AcceptanceItemsRegion(spec)
	if !ok {
		t.Fatal("expected region to be found")
	}
	if !strings.HasPrefix(region, "acceptance_item_set:") {
		t.Fatalf("region must start at the marker line, not at the prose mention, got:\n%s", region)
	}
	if strings.Contains(region, "acceptance_item_set: is declared") {
		t.Fatalf("region must not include the line-start prose mention, got:\n%s", region)
	}
	if !strings.Contains(region, "dep.core") {
		t.Fatalf("region must contain the real marker's item content, got:\n%s", region)
	}
}

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

func TestSectionRegionsSplit(t *testing.T) {
	regions := SectionRegions(specWithItems)
	if len(regions) != 4 {
		t.Fatalf("expected 4 regions (frontmatter + 3 sections), got %d:\n%+v", len(regions), regions)
	}
	if regions[0].Heading != "" {
		t.Fatalf("expected frontmatter region first, got heading %q", regions[0].Heading)
	}
	if regions[0].Start != 1 || regions[0].End != 9 {
		t.Fatalf("expected frontmatter region 1-9, got %d-%d", regions[0].Start, regions[0].End)
	}
	if regions[1].Heading != "Description" {
		t.Fatalf("expected Description section, got %q", regions[1].Heading)
	}
	if !strings.HasPrefix(regions[1].Text, "## Description") {
		t.Fatalf("section region must start at its heading line, got:\n%s", regions[1].Text)
	}
	if regions[2].Heading != "Testability / Acceptance Criteria" {
		t.Fatalf("expected Testability section, got %q", regions[2].Heading)
	}
	if !strings.Contains(regions[2].Text, "acceptance_item_set:") {
		t.Fatalf("Testability section must contain the item set, got:\n%s", regions[2].Text)
	}
	if regions[3].Heading != "Dependencies" {
		t.Fatalf("expected Dependencies section, got %q", regions[3].Heading)
	}
}

func TestSectionRegionsNoHeadings(t *testing.T) {
	regions := SectionRegions("just prose\nwith no headings\n")
	if len(regions) != 1 {
		t.Fatalf("expected a single frontmatter region, got %d", len(regions))
	}
	if regions[0].Heading != "" {
		t.Fatalf("expected empty heading, got %q", regions[0].Heading)
	}
}

func TestSectionRegionsDeeperHeadingsBelongToSection(t *testing.T) {
	spec := "## One\n\n### Sub one\n\ncontent\n\n## Two\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d", len(regions))
	}
	if !strings.Contains(regions[1].Text, "### Sub one") {
		t.Fatalf("### must belong to its ## section, got:\n%s", regions[1].Text)
	}
	if strings.Contains(regions[1].Text, "## Two") {
		t.Fatalf("section must end before the next ## heading, got:\n%s", regions[1].Text)
	}
}

func TestSectionRegionsHeadingRenamingChangesCID(t *testing.T) {
	region1, ok := LocateSectionRegion(specWithItems, "Description")
	if !ok {
		t.Fatal("expected Description section")
	}
	cid1 := RegionCID(region1.Text)

	renamed := strings.Replace(specWithItems, "## Description", "## Overview", 1)
	region2, ok := LocateSectionRegion(renamed, "Overview")
	if !ok {
		t.Fatal("expected Overview section after rename")
	}
	if RegionCID(region2.Text) == cid1 {
		t.Fatal("renaming a heading must change the section region's CID")
	}
}

func TestSectionRegionsPositionIndependent(t *testing.T) {
	region1, ok := LocateSectionRegion(specWithItems, "Testability / Acceptance Criteria")
	if !ok {
		t.Fatal("expected section")
	}
	cid1 := RegionCID(region1.Text)

	// Inserting prose in another section must not change this section's CID.
	edited := strings.Replace(specWithItems, "Background prose about the dependency unit.", "Background prose about the dependency unit, with a long addition that pushes content around.", 1)
	region2, ok := LocateSectionRegion(edited, "Testability / Acceptance Criteria")
	if !ok {
		t.Fatal("expected section after edit")
	}
	if RegionCID(region2.Text) != cid1 {
		t.Fatal("section CID must be independent of edits in other sections")
	}

	// Editing inside the section changes the CID.
	editedInside := strings.Replace(specWithItems, "The core behavior is provided.", "The core behavior is provided promptly.", 1)
	region3, _ := LocateSectionRegion(editedInside, "Testability / Acceptance Criteria")
	if RegionCID(region3.Text) == cid1 {
		t.Fatal("section CID must change when section content changes")
	}
}

func TestSectionRegionMissingHeading(t *testing.T) {
	if _, ok := LocateSectionRegion(specWithItems, "No Such Section"); ok {
		t.Fatal("expected no section for a missing heading")
	}
}

func TestSectionRegionDuplicatedHeadingFailsClosed(t *testing.T) {
	duplicated := "## Notes\n\nFirst.\n\n## Notes\n\nSecond.\n"
	if _, ok := LocateSectionRegion(duplicated, "Notes"); ok {
		t.Fatal("duplicated headings must fail closed")
	}
}

func TestListMissingDeps(t *testing.T) {
	region, ok := LocateSectionRegion(specWithItems, "Description")
	if !ok {
		t.Fatal("expected section")
	}
	sectionDep := "region:section:Description:" + RegionCID(region.Text)
	itemRegion, ok := AcceptanceItemsRegion(specWithItems)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	itemDep := "region:acceptance_items:" + RegionCID(itemRegion)

	freshDeps := []string{sectionDep, itemDep}
	if missing := ListMissingDeps(specWithItems, freshDeps); len(missing) != 0 {
		t.Fatalf("expected no missing deps, got %v", missing)
	}

	// Editing the Description section stales only its own region dep.
	edited := strings.Replace(specWithItems, "Background prose about the dependency unit.", "Background prose, edited.", 1)
	missing := ListMissingDeps(edited, freshDeps)
	if len(missing) != 1 || missing[0] != sectionDep {
		t.Fatalf("expected only the Description section dep missing, got %v", missing)
	}

	// A renamed heading stales the section dep even when the body is unchanged.
	renamed := strings.Replace(specWithItems, "## Description", "## Overview", 1)
	missing = ListMissingDeps(renamed, freshDeps)
	if len(missing) != 1 || missing[0] != sectionDep {
		t.Fatalf("expected the section dep missing after heading rename, got %v", missing)
	}

	// Unknown region types fail closed.
	unknown := []string{"region:unknown_type:sha256:abc"}
	if missing := ListMissingDeps(specWithItems, unknown); len(missing) != 1 {
		t.Fatalf("expected unknown region type reported missing, got %v", missing)
	}
}

func TestSectionRegionsFencedCodeBlock(t *testing.T) {
	spec := "## Usage\n\nExample:\n\n```\n## Not a real heading\ninside a code block\n```\n\n## Real Section\n\nProse.\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions (frontmatter + 2 sections), got %d:\n%+v", len(regions), regions)
	}
	if regions[1].Heading != "Usage" {
		t.Fatalf("expected Usage section, got %q", regions[1].Heading)
	}
	if !strings.Contains(regions[1].Text, "## Not a real heading") {
		t.Fatalf("fenced heading must stay inside its section, got:\n%s", regions[1].Text)
	}
	if strings.Contains(regions[1].Text, "## Real Section") {
		t.Fatalf("section must end at the real heading, got:\n%s", regions[1].Text)
	}
}

func TestSectionRegionsFenceWithLanguageTag(t *testing.T) {
	spec := "## Examples\n\n```go\n// ## inside go code\nfunc main() {}\n```\n\n## Next\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d", len(regions))
	}
	if regions[2].Heading != "Next" {
		t.Fatalf("expected Next section, got %q", regions[2].Heading)
	}
}

func TestSectionRegionsTildeFence(t *testing.T) {
	spec := "## Examples\n\n~~~\n## not a heading\n~~~\n\n## Next\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d", len(regions))
	}
	if regions[2].Heading != "Next" {
		t.Fatalf("expected Next section after tilde fence, got %q", regions[2].Heading)
	}
}

func TestSectionRegionsTildeFenceLongerClosing(t *testing.T) {
	// CommonMark closing semantics: a longer run of the same character
	// closes a shorter fence (~~~~ closes ~~~). The fence must stay closed
	// so the real heading after it splits a new section.
	spec := "## Examples\n\n~~~\n## not a heading\n~~~~\n\n## Next\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d:\n%+v", len(regions), regions)
	}
	if !strings.Contains(regions[1].Text, "## not a heading") {
		t.Fatalf("fenced heading must stay inside its section, got:\n%s", regions[1].Text)
	}
	if regions[2].Heading != "Next" {
		t.Fatalf("expected Next section after longer closing fence, got %q", regions[2].Heading)
	}
}

func TestSectionRegionsBacktickFenceLongerClosing(t *testing.T) {
	// A closing run longer than the opening run (```` closes ```) is valid
	// CommonMark — the fence must close and the real heading after it must
	// split a new section.
	spec := "## Examples\n\n```\n## not a heading\n````\n\n## Next\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d:\n%+v", len(regions), regions)
	}
	if !strings.Contains(regions[1].Text, "## not a heading") {
		t.Fatalf("fenced heading must stay inside its section, got:\n%s", regions[1].Text)
	}
	if regions[2].Heading != "Next" {
		t.Fatalf("expected Next section after longer backtick closing fence, got %q", regions[2].Heading)
	}
}

func TestSectionRegionsUnclosedFence(t *testing.T) {
	spec := "## Examples\n\n```\n## not a heading\nno closing fence\n"
	regions := SectionRegions(spec)
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions (no heading after unclosed fence), got %d", len(regions))
	}
	if !strings.Contains(regions[1].Text, "no closing fence") {
		t.Fatalf("unclosed fence content must stay in its section, got:\n%s", regions[1].Text)
	}
}

func TestSectionRegionsFenceHeadingOutsideStillSplits(t *testing.T) {
	spec := "## A\n\n```\ncode\n```\n\n## B\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d", len(regions))
	}
	if regions[2].Heading != "B" {
		t.Fatalf("real headings outside fences must still split, got %q", regions[2].Heading)
	}
}

func TestSectionRegionsItemExampleInsideFence(t *testing.T) {
	// A spec body showing an acceptance_item_set example inside a code fence
	// must not confuse region splitting.
	spec := "## Notes\n\n```yaml\nacceptance_item_set:\n  - id: example.only\n    description: Example.\n```\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: real.core\n    description: Real.\n"
	regions := SectionRegions(spec)
	if len(regions) != 3 {
		t.Fatalf("expected 3 regions, got %d", len(regions))
	}
	if regions[2].Heading != "Testability / Acceptance Criteria" {
		t.Fatalf("expected Testability section, got %q", regions[2].Heading)
	}
	// The real marker is the exact line — the fence example is indented.
	region, ok := AcceptanceItemsRegion(spec)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	if !strings.Contains(region, "real.core") {
		t.Fatalf("region must contain the real item, got:\n%s", region)
	}
	if strings.Contains(region, "example.only") {
		t.Fatalf("region must not contain the fenced example, got:\n%s", region)
	}
}

func TestAcceptanceItemsRegionFenceTopLevelMarkerIgnored(t *testing.T) {
	// A code-fenced example with a top-level `acceptance_item_set:` marker
	// must not start the region — only the real marker outside a fence does.
	spec := "## Notes\n\n```\nacceptance_item_set:\n  - id: example.only\n    description: Example.\n```\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: real.core\n    description: Real.\n"
	region, ok := AcceptanceItemsRegion(spec)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	if !strings.HasPrefix(region, "acceptance_item_set:") {
		t.Fatalf("region must start at the real marker, got:\n%s", region)
	}
	if !strings.Contains(region, "real.core") {
		t.Fatalf("region must contain the real item, got:\n%s", region)
	}
	if strings.Contains(region, "example.only") {
		t.Fatalf("region must not contain the fenced example, got:\n%s", region)
	}
}

func TestAcceptanceItemsRegionFenceHeadingDoesNotTruncate(t *testing.T) {
	// A fenced top-level `#` line must not end the region — region content
	// inside a fence is not a heading boundary.
	spec := "## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: real.core\n    description: Real.\n\n```\n# not a real heading\n```\n\n## Dependencies\n\nNone.\n"
	region, ok := AcceptanceItemsRegion(spec)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	if !strings.Contains(region, "real.core") {
		t.Fatalf("region must contain the item, got:\n%s", region)
	}
	if !strings.Contains(region, "# not a real heading") {
		t.Fatalf("fenced heading must stay inside the region, got:\n%s", region)
	}
	if strings.Contains(region, "## Dependencies") {
		t.Fatalf("region must end at the real heading, got:\n%s", region)
	}
}

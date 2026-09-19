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
	if regions[3].Start != 26 || regions[3].End != 28 {
		t.Fatalf("expected the Dependencies region 26-28 (the artificial trailing newline is not a line), got %d-%d", regions[3].Start, regions[3].End)
	}
	if strings.HasSuffix(regions[3].Text, "\n") {
		t.Fatalf("the final region must not include the artificial trailing newline, got %q", regions[3].Text)
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
	if regions[0].Start != 1 || regions[0].End != 2 {
		t.Fatalf("expected the region 1-2 (the artificial trailing newline is not a line), got %d-%d", regions[0].Start, regions[0].End)
	}
	if regions[0].Text != "just prose\nwith no headings" {
		t.Fatalf("expected the region text without the artificial trailing newline, got %q", regions[0].Text)
	}
}

func TestSectionPresenceAndSplittability(t *testing.T) {
	if IsSectionSplittable("just prose\nwith no headings\n") {
		t.Fatal("a text with no ## heading must not be section-splittable")
	}
	if !IsSectionSplittable("## One\n\ncontent\n") {
		t.Fatal("a text with a ## heading must be section-splittable")
	}
	if !HasSectionHeading("## One\n\ncontent\n", "One") {
		t.Fatal("expected the heading to be reported present")
	}
	if HasSectionHeading("## One\n\ncontent\n", "Two") {
		t.Fatal("an absent heading must not be reported present")
	}
	dup := "## frontmatter\n\nFirst.\n\n## frontmatter\n\nSecond.\n"
	if !HasSectionHeading(dup, "frontmatter") {
		t.Fatal("a duplicated real heading must count as present")
	}
	if _, ok := LocateSectionRegion(dup, "frontmatter"); ok {
		t.Fatal("the locator must fail closed on a duplicated heading")
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
	itemSetCID, err := AcceptanceItemSetCID(specWithItems)
	if err != nil {
		t.Fatalf("expected acceptance item set CID: %v", err)
	}
	itemDep := "region:acceptance_items:" + itemSetCID

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

func TestAcceptanceItemsRegionEndsAtSectionHeading(t *testing.T) {
	// `###` and `#` lines belong to their enclosing `##` section and must not
	// terminate the item set; only the next `##` heading does.
	spec := "# Dep Unit\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n\n### Extra structure\n\n  - id: dep.aux\n    description: Aux.\n\n# Not a section\n\n  - id: dep.third\n    description: Third.\n\n## Dependencies\n\nNone.\n"
	region, ok := AcceptanceItemsRegion(spec)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	for _, want := range []string{"dep.core", "### Extra structure", "dep.aux", "# Not a section", "dep.third"} {
		if !strings.Contains(region, want) {
			t.Fatalf("region must contain %q, got:\n%s", want, region)
		}
	}
	if strings.Contains(region, "## Dependencies") {
		t.Fatalf("region must end at the next ## heading, got:\n%s", region)
	}
	want := []string{"dep.core", "dep.aux", "dep.third"}
	ids := AcceptanceItemIDs(spec)
	if len(ids) != len(want) {
		t.Fatalf("expected ids %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected ids %v, got %v", want, ids)
		}
	}
}

func TestAcceptanceItemRegionsAcrossSubheading(t *testing.T) {
	spec := "## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n\n### Extra structure\n\n  - id: dep.aux\n    description: Aux.\n\n## Dependencies\n\nNone.\n"
	regions := AcceptanceItemRegions(spec)
	if len(regions) != 2 || regions[0].ID != "dep.core" || regions[1].ID != "dep.aux" {
		t.Fatalf("expected both items across the ### subheading, got %+v", regions)
	}
	if !strings.Contains(regions[1].Text, "description: Aux.") {
		t.Fatalf("second item region must carry its fields, got %q", regions[1].Text)
	}
}

func TestAcceptanceItemsRegionIndentedSectionHeading(t *testing.T) {
	// An indented `##` heading is a section boundary for SectionRegions, so it
	// terminates the item set the same way — one boundary definition.
	spec := "## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n\n  ## Sub\n\n  - id: dep.late\n    description: Late.\n"
	found := false
	for _, r := range SectionRegions(spec) {
		if r.Heading == "Sub" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SectionRegions must treat the indented heading as a section, got %+v", SectionRegions(spec))
	}
	ids := AcceptanceItemIDs(spec)
	if len(ids) != 1 || ids[0] != "dep.core" {
		t.Fatalf("the item set must end at the indented ## heading, got %v", ids)
	}
}

func TestAcceptanceItemsRegionFinalNewlineNotContent(t *testing.T) {
	// A set that runs to the end of the file and the same set followed
	// immediately by a `##` heading have identical region content: the
	// synthetic file-final newline is not a line.
	eof := "## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: a\n    description: A.\n"
	adjacent := "## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: a\n    description: A.\n## Dependencies\n\nNone.\n"
	regionEOF, okEOF := AcceptanceItemsRegion(eof)
	regionAdjacent, okAdjacent := AcceptanceItemsRegion(adjacent)
	if !okEOF || !okAdjacent {
		t.Fatal("expected both regions to be found")
	}
	if strings.HasSuffix(regionEOF, "\n") {
		t.Fatalf("the file-final region must not claim the synthetic newline, got %q", regionEOF)
	}
	if RegionCID(regionEOF) != RegionCID(regionAdjacent) {
		t.Fatalf("region content changed without an inside edit:\n%q\nvs\n%q", regionEOF, regionAdjacent)
	}
}

// specWithTwoItems carries two acceptance items; dep.aux's description
// contains a fenced `- id:` example (content, never an item boundary).
var specWithTwoItems = "# Dep Unit\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: The core behavior is provided.\n    pass_condition: Core behavior passes.\n\n  - id: dep.aux\n    description: |\n      Aux behavior.\n\n      ```\n      - id: example.only\n        description: A fenced example, not an item.\n      ```\n    pass_condition: Aux behavior passes.\n\n## Dependencies\n\nThis unit has no external dependencies.\n"

func TestAcceptanceItemRegionsList(t *testing.T) {
	regions := AcceptanceItemRegions(specWithTwoItems)
	if len(regions) != 2 {
		t.Fatalf("expected 2 item regions, got %d: %+v", len(regions), regions)
	}
	if regions[0].ID != "dep.core" || regions[1].ID != "dep.aux" {
		t.Fatalf("unexpected ids: %q, %q", regions[0].ID, regions[1].ID)
	}
	if !strings.HasPrefix(regions[0].Text, "  - id: dep.core") {
		t.Fatalf("region must start at the id line, got %q", regions[0].Text)
	}
	if strings.Contains(regions[0].Text, "dep.aux") {
		t.Fatalf("region must end before the next item, got %q", regions[0].Text)
	}
	if regions[0].Start != 6 || regions[0].End != 8 {
		t.Fatalf("unexpected dep.core line range: %d-%d", regions[0].Start, regions[0].End)
	}
	if !strings.Contains(regions[1].Text, "example.only") {
		t.Fatalf("fenced example stays inside the item content, got %q", regions[1].Text)
	}
	ids := AcceptanceItemIDs(specWithTwoItems)
	if len(ids) != 2 || ids[0] != "dep.core" || ids[1] != "dep.aux" {
		t.Fatalf("unexpected extraction result: %v", ids)
	}
}

func TestAcceptanceItemRegionReorderInvariance(t *testing.T) {
	blockCore := "  - id: dep.core\n    description: Core behavior.\n    pass_condition: Core passes.\n"
	blockAux := "  - id: dep.aux\n    description: Aux behavior.\n    pass_condition: Aux passes.\n"
	header := "# U\n\nacceptance_item_set:\n"
	plain := header + blockCore + "\n" + blockAux + "\n## Dependencies\n\nNone.\n"
	swapped := header + blockAux + "\n" + blockCore + "\n## Dependencies\n\nNone.\n"

	for _, id := range []string{"dep.core", "dep.aux"} {
		a, okA := LocateAcceptanceItemRegion(plain, id)
		b, okB := LocateAcceptanceItemRegion(swapped, id)
		if !okA || !okB {
			t.Fatalf("item %s must locate in both orders", id)
		}
		if RegionCID(a.Text) != RegionCID(b.Text) {
			t.Fatalf("item %s region changed on reorder:\n%q\nvs\n%q", id, a.Text, b.Text)
		}
	}

	setPlain, err := AcceptanceItemSetCID(plain)
	if err != nil {
		t.Fatal(err)
	}
	setSwapped, err := AcceptanceItemSetCID(swapped)
	if err != nil {
		t.Fatal(err)
	}
	if setPlain != setSwapped {
		t.Fatalf("semantic whole-set CID changed on reorder: %s != %s", setPlain, setSwapped)
	}
}

func TestAcceptanceItemRegionSpacingInvariance(t *testing.T) {
	blockCore := "  - id: dep.core\n    description: Core behavior.\n    pass_condition: Core passes.\n"
	blockAux := "  - id: dep.aux\n    description: Aux behavior.\n    pass_condition: Aux passes.\n"
	header := "# U\n\nacceptance_item_set:\n"
	normal := header + blockCore + "\n" + blockAux + "\n## Dependencies\n\nNone.\n"
	spaced := header + blockCore + "\n\n\n" + blockAux + "\n\n\n## Dependencies\n\nNone.\n"

	coreA, _ := LocateAcceptanceItemRegion(normal, "dep.core")
	auxA, _ := LocateAcceptanceItemRegion(normal, "dep.aux")
	coreB, okCore := LocateAcceptanceItemRegion(spaced, "dep.core")
	auxB, okAux := LocateAcceptanceItemRegion(spaced, "dep.aux")
	if !okCore || !okAux {
		t.Fatal("items must locate in the spaced variant")
	}
	if RegionCID(coreA.Text) != RegionCID(coreB.Text) || RegionCID(auxA.Text) != RegionCID(auxB.Text) {
		t.Fatal("blank-line spacing between items must not change item region CIDs")
	}
	setA, err := AcceptanceItemSetCID(normal)
	if err != nil {
		t.Fatal(err)
	}
	setB, err := AcceptanceItemSetCID(spaced)
	if err != nil {
		t.Fatal(err)
	}
	if setA != setB {
		t.Fatalf("blank-line spacing changed semantic whole-set CID: %s != %s", setA, setB)
	}
}

func TestAcceptanceItemSetCIDTracksSemanticChanges(t *testing.T) {
	base, err := AcceptanceItemSetCID(specWithTwoItems)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		text string
	}{
		{"item content", strings.Replace(specWithTwoItems, "The core behavior is provided.", "The core behavior changed.", 1)},
		{"item addition", strings.Replace(specWithTwoItems, "\n## Dependencies", "\n  - id: dep.extra\n    description: Extra.\n\n## Dependencies", 1)},
		{"item removal", strings.Replace(specWithTwoItems, "  - id: dep.core\n    description: The core behavior is provided.\n    pass_condition: Core behavior passes.\n\n", "", 1)},
		{"item rename", strings.Replace(specWithTwoItems, "- id: dep.core", "- id: dep.renamed", 1)},
		{"set preamble", strings.Replace(specWithTwoItems, "acceptance_item_set:\n", "acceptance_item_set:\n  policy: exhaustive\n", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AcceptanceItemSetCID(tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if got == base {
				t.Fatalf("semantic change %q did not change the whole-set CID", tc.name)
			}
		})
	}
}

func TestAcceptanceItemSetCIDFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{"missing marker", "# U\n\nNo set.\n"},
		{"empty set", "# U\n\nacceptance_item_set:\n\n## Next\n"},
		{"empty id", "# U\n\nacceptance_item_set:\n  - id:\n    description: Missing.\n"},
		{"blank quoted id", "# U\n\nacceptance_item_set:\n  - id: \" \"\n    description: Missing.\n"},
		{"duplicate id", strings.Replace(specWithTwoItems, "- id: dep.aux", "- id: dep.core", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AcceptanceItemSetCID(tc.text); err == nil {
				t.Fatal("expected fail-closed error")
			}
		})
	}
}

func TestEmptyAcceptanceItemIDLine(t *testing.T) {
	valid := "# U\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n"
	if line, found := EmptyAcceptanceItemIDLine(valid); found {
		t.Fatalf("valid ids must not report an empty id, got line %d", line)
	}
	if _, found := EmptyAcceptanceItemIDLine("# U\n\nNo set.\n"); found {
		t.Fatal("a missing marker must not report an empty id")
	}

	empty := "# U\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n\n  - id:\n    description: Missing.\n"
	line, found := EmptyAcceptanceItemIDLine(empty)
	if !found {
		t.Fatal("expected the empty id line to be reported")
	}
	if line != 7 {
		t.Fatalf("unexpected empty id line: %d", line)
	}

	fenced := "# U\n\nacceptance_item_set:\n  - id: dep.core\n    description: |\n      ```\n      - id:\n      ```\n"
	if _, found := EmptyAcceptanceItemIDLine(fenced); found {
		t.Fatal("an empty id inside a fenced block is content, not an item")
	}

	quoted := "# U\n\nacceptance_item_set:\n  - id: \" \"\n    description: Missing.\n"
	if _, found := EmptyAcceptanceItemIDLine(quoted); !found {
		t.Fatal("a blank quoted id must be reported as empty")
	}
}

func TestLegacyRawAcceptanceItemsCIDStales(t *testing.T) {
	region, ok := AcceptanceItemsRegion(specWithTwoItems)
	if !ok {
		t.Fatal("expected raw acceptance item region")
	}
	legacyDep := "region:acceptance_items:" + RegionCID(region)
	missing := ListMissingDeps(specWithTwoItems, []string{legacyDep})
	if len(missing) != 1 || missing[0] != legacyDep {
		t.Fatalf("legacy raw whole-set CID must stale once, got %v", missing)
	}
}

func TestAcceptanceItemRegionDuplicateFailsClosed(t *testing.T) {
	dup := "# U\n\nacceptance_item_set:\n  - id: dep.core\n    description: First.\n\n  - id: dep.core\n    description: Second.\n"
	if _, ok := LocateAcceptanceItemRegion(dup, "dep.core"); ok {
		t.Fatal("a duplicated id must fail closed")
	}
	if ids := AcceptanceItemIDs(dup); len(ids) != 2 {
		t.Fatalf("listing must still show both duplicated ids, got %v", ids)
	}
}

func TestAcceptanceItemRegionMissingFailsClosed(t *testing.T) {
	if _, ok := LocateAcceptanceItemRegion(specWithTwoItems, "dep.none"); ok {
		t.Fatal("an unknown id must fail closed")
	}
	if _, ok := LocateAcceptanceItemRegion("# U\n\nNo item set here.\n", "dep.core"); ok {
		t.Fatal("an absent marker must fail closed")
	}
	// An empty id line is not an item and never a boundary.
	empty := "# U\n\nacceptance_item_set:\n  - id:\n    description: No id.\n\n  - id: dep.core\n    description: Real.\n"
	ids := AcceptanceItemIDs(empty)
	if len(ids) != 1 || ids[0] != "dep.core" {
		t.Fatalf("empty id must be skipped, got %v", ids)
	}
}

func TestListMissingDepsAcceptanceItem(t *testing.T) {
	core, ok := LocateAcceptanceItemRegion(specWithTwoItems, "dep.core")
	if !ok {
		t.Fatal("expected dep.core region")
	}
	aux, ok := LocateAcceptanceItemRegion(specWithTwoItems, "dep.aux")
	if !ok {
		t.Fatal("expected dep.aux region")
	}
	coreDep := "region:acceptance_item:dep.core:" + RegionCID(core.Text)
	auxDep := "region:acceptance_item:dep.aux:" + RegionCID(aux.Text)
	deps := []string{coreDep, auxDep}

	if missing := ListMissingDeps(specWithTwoItems, deps); len(missing) != 0 {
		t.Fatalf("expected no missing deps, got %v", missing)
	}

	// Editing dep.core stales only its own item dep.
	edited := strings.Replace(specWithTwoItems, "The core behavior is provided.", "The core behavior is provided (edited).", 1)
	missing := ListMissingDeps(edited, deps)
	if len(missing) != 1 || missing[0] != coreDep {
		t.Fatalf("expected only the dep.core dep missing, got %v", missing)
	}

	// Renaming an id makes the old declaration unlocatable (fail closed).
	renamed := strings.Replace(specWithTwoItems, "- id: dep.core", "- id: dep.renamed", 1)
	missing = ListMissingDeps(renamed, deps)
	if len(missing) != 1 || missing[0] != coreDep {
		t.Fatalf("expected the dep.core dep missing after rename, got %v", missing)
	}

	// A duplicated id fails closed even when the content is unchanged.
	dup := strings.Replace(specWithTwoItems, "- id: dep.aux", "- id: dep.core", 1)
	if missing := ListMissingDeps(dup, []string{auxDep}); len(missing) != 1 {
		t.Fatalf("duplicate id must be reported missing, got %v", missing)
	}

	unknown := []string{"region:acceptance_item:dep.none:sha256:abc"}
	if missing := ListMissingDeps(specWithTwoItems, unknown); len(missing) != 1 {
		t.Fatalf("unknown item must be reported missing, got %v", missing)
	}
}

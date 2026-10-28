package contenthash

import (
	"strings"
	"testing"
)

const narrativeSpec = `---
id: dep
---

# Dep Unit

## Design Narrative

Intro prose that must be flagged as stray.

### 3.1H Memory

Memory persists across runs.

#### 3.1H.1 Budget

The budget guards the render.

### 3.2 Events

Events are emitted on the run span.

## Promises

promises:
  - id: dep.events
    summary: Run span events.
    contract:
      operations:
        emit:
          outputs:
            name: {type: string}

  - id: dep.core
    summary: The core behavior.
    contract:
      operations:
        run:
          outputs:
            status: {type: string}

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: dep.core
    description: Core behavior passes.
    verification_type: testable
    verification_surface: api
    implementation_surface: src/dep
    verification_method: test
    pass_condition: Core passes.
    runnable: yes
`

func TestNarrativeBlocksExtractsLevelThreeAndDeeper(t *testing.T) {
	blocks := NarrativeBlocks(narrativeSpec)
	var got []string
	for _, b := range blocks {
		got = append(got, b.Heading)
	}
	want := []string{"3.1H Memory", "3.1H.1 Budget", "3.2 Events"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("narrative blocks = %v, want %v", got, want)
	}
	for _, b := range blocks {
		if !strings.Contains(b.Text, b.Heading) {
			t.Fatalf("block %q must include its own heading line", b.Heading)
		}
		if strings.HasSuffix(b.Text, "\n") {
			t.Fatalf("block %q must not end with a blank line", b.Heading)
		}
	}
	// A `##` section heading ends the block it is nested in.
	for _, b := range blocks {
		if strings.Contains(b.Text, "## Promises") {
			t.Fatalf("a narrative block must not cross into the next ## section: %q", b.Heading)
		}
	}
	// A deeper heading does not end its parent block.
	var memory, budget bool
	for _, b := range blocks {
		if b.Heading == "3.1H Memory" {
			memory = true
			if !strings.Contains(b.Text, "The budget guards the render.") {
				t.Fatal("a #### block must stay inside its ### block")
			}
		}
		if b.Heading == "3.1H.1 Budget" {
			budget = true
		}
	}
	if !memory || !budget {
		t.Fatal("expected both the ### and the nested #### block")
	}
}

func TestNarrativeBlocksIgnoresFencedHeadings(t *testing.T) {
	text := "## Design Narrative\n\n### Real\n\nBody.\n\n```markdown\n### Fenced\n\nNot a block.\n```\n"
	blocks := NarrativeBlocks(text)
	if len(blocks) != 1 || blocks[0].Heading != "Real" {
		t.Fatalf("a fenced heading must not start a block, got %v", blocks)
	}
	if !strings.Contains(blocks[0].Text, "### Fenced") {
		t.Fatal("fenced content belongs to the enclosing block")
	}
}

func TestLocateNarrativeBlock(t *testing.T) {
	if b, ok := LocateNarrativeBlock(narrativeSpec, "3.2 Events"); !ok || b.Level != 3 || !strings.Contains(b.Text, "run span") {
		t.Fatalf("expected the 3.2 block, got %+v ok=%v", b, ok)
	}
	if _, ok := LocateNarrativeBlock(narrativeSpec, "Missing"); ok {
		t.Fatal("a missing heading must fail closed")
	}
	dup := "## Design Narrative\n\n### Same\n\nA.\n\n### Same\n\nB.\n"
	if _, ok := LocateNarrativeBlock(dup, "Same"); ok {
		t.Fatal("a duplicated heading text must fail closed")
	}
}

func TestPromiseRegionsAndLocation(t *testing.T) {
	regions := PromiseRegions(narrativeSpec)
	if len(regions) != 2 {
		t.Fatalf("expected two promise regions, got %d", len(regions))
	}
	if regions[0].ID != "dep.events" || regions[1].ID != "dep.core" {
		t.Fatalf("promise ids = %q, %q", regions[0].ID, regions[1].ID)
	}
	for _, r := range regions {
		if !strings.Contains(r.Text, "summary:") {
			t.Fatalf("promise %q region must carry its content", r.ID)
		}
		if strings.Contains(r.Text, "## Testability") {
			t.Fatalf("promise %q region must not cross into the next ## section", r.ID)
		}
	}
	// The first promise must not swallow the second one.
	if strings.Contains(regions[0].Text, "dep.core") {
		t.Fatal("the first promise region must end before the second - id: line")
	}
	if r, ok := LocatePromiseRegion(narrativeSpec, "dep.core"); !ok || !strings.Contains(r.Text, "The core behavior.") {
		t.Fatalf("expected to locate dep.core, got %+v ok=%v", r, ok)
	}
	if _, ok := LocatePromiseRegion(narrativeSpec, "dep.missing"); ok {
		t.Fatal("a missing promise id must fail closed")
	}
}

func TestSpecAnchorDepResolvesAndHolds(t *testing.T) {
	cases := []struct {
		anchor string
		prefix string
	}{
		{"dep.events", "region:promise:dep.events:"},
		{"Promises", "region:section:Promises:"},
		{"3.2 Events", "region:subsection:3.2 Events:"},
	}
	for _, tc := range cases {
		dep, ok := SpecAnchorDep(narrativeSpec, tc.anchor)
		if !ok {
			t.Fatalf("anchor %q must resolve", tc.anchor)
		}
		if !strings.HasPrefix(dep, tc.prefix) {
			t.Fatalf("anchor %q resolved to %q, want prefix %q", tc.anchor, dep, tc.prefix)
		}
		if !DepsPresent(narrativeSpec, []string{dep}) {
			t.Fatalf("anchor %q pin must hold for the same text: %q", tc.anchor, dep)
		}
	}
	if _, ok := SpecAnchorDep(narrativeSpec, "no such anchor"); ok {
		t.Fatal("an unresolvable anchor must fail closed")
	}
}

func TestSpecAnchorDepSurvivesUnrelatedEdits(t *testing.T) {
	dep, ok := SpecAnchorDep(narrativeSpec, "3.2 Events")
	if !ok {
		t.Fatal("anchor must resolve")
	}
	// Editing a different narrative block must not invalidate this pin.
	edited := strings.Replace(narrativeSpec, "Memory persists across runs.", "Memory persists across runs, edited.", 1)
	if !DepsPresent(edited, []string{dep}) {
		t.Fatal("an edit outside the pinned block must not invalidate it")
	}
	// Editing the pinned block must invalidate it.
	edited = strings.Replace(narrativeSpec, "Events are emitted on the run span.", "Events are emitted on the run span, edited.", 1)
	if DepsPresent(edited, []string{dep}) {
		t.Fatal("an edit inside the pinned block must invalidate it")
	}
}

func TestNarrativeSourceDepOnlyAcceptsDesignNarrativeBlocks(t *testing.T) {
	cases := []string{"3.1H Memory", "3.1H.1 Budget", "3.2 Events"}
	for _, anchor := range cases {
		dep, ok := NarrativeSourceDep(narrativeSpec, anchor)
		if !ok {
			t.Fatalf("source anchor %q must resolve", anchor)
		}
		if !strings.HasPrefix(dep, "region:subsection:"+anchor+":") {
			t.Fatalf("source anchor %q resolved to %q, want a subsection pin", anchor, dep)
		}
	}
	for _, anchor := range []string{"Promises", "dep.events", "Testability / Acceptance Criteria", "no such anchor"} {
		if _, ok := NarrativeSourceDep(narrativeSpec, anchor); ok {
			t.Fatalf("source anchor %q must not resolve: a source is a Design Narrative block, not a section or promise id", anchor)
		}
	}
}

func TestNarrativeSourceDepHoldsUntilTheSourceBlockChanges(t *testing.T) {
	dep, ok := NarrativeSourceDep(narrativeSpec, "3.2 Events")
	if !ok {
		t.Fatal("source anchor must resolve")
	}
	edited := strings.Replace(narrativeSpec, "Memory persists across runs.", "Memory persists across runs, edited.", 1)
	if !DepsPresent(edited, []string{dep}) {
		t.Fatal("an edit outside the source block must not invalidate it")
	}
	edited = strings.Replace(narrativeSpec, "Events are emitted on the run span.", "Events are emitted on the run span, edited.", 1)
	if DepsPresent(edited, []string{dep}) {
		t.Fatal("an edit inside the source block must invalidate it")
	}
}

func TestPromiseRegionPinSurvivesReorderAndBlocksOnContractChange(t *testing.T) {
	dep, ok := SpecAnchorDep(narrativeSpec, "dep.core")
	if !ok {
		t.Fatal("anchor must resolve")
	}
	// Reordering the two promises moves the block but keeps its content.
	reordered := strings.Replace(narrativeSpec,
		"  - id: dep.events\n    summary: Run span events.\n    contract:\n      operations:\n        emit:\n          outputs:\n            name: {type: string}\n\n  - id: dep.core",
		"  - id: dep.core\n    summary: The core behavior.\n    contract:\n      operations:\n        run:\n          outputs:\n            status: {type: string}\n\n  - id: dep.events", 1)
	if !DepsPresent(reordered, []string{dep}) {
		t.Fatal("reordering promises must not invalidate a promise-entry pin")
	}
	changed := strings.Replace(narrativeSpec, "The core behavior.", "The core behavior, restated.", 1)
	if DepsPresent(changed, []string{dep}) {
		t.Fatal("a contract edit must invalidate the promise-entry pin")
	}
}

func TestDuplicateHeadings(t *testing.T) {
	dups := DuplicateHeadings(narrativeSpec)
	if len(dups) != 0 {
		t.Fatalf("the fixture has unique headings, got %v", dups)
	}
	dup := "## Design Narrative\n\n### Same\n\nA.\n\n#### Same\n\nB.\n\n## Promises\n\npromises:\n  - id: x.y\n"
	if got := DuplicateHeadings(dup); len(got) != 1 || got[0] != "Same" {
		t.Fatalf("a heading text duplicated across levels must be reported once, got %v", got)
	}
	fenced := "## Design Narrative\n\n### Real\n\n```markdown\n### Same\n```\n\n### Other\n"
	if got := DuplicateHeadings(fenced); len(got) != 0 {
		t.Fatalf("a fenced heading is content, got %v", got)
	}
}

func TestNarrativeStrayProseLines(t *testing.T) {
	lines, ok := NarrativeStrayProseLines(narrativeSpec)
	if !ok {
		t.Fatal("expected the Design Narrative section to be present")
	}
	if len(lines) != 1 || !strings.Contains(narrativeSpec, "Intro prose that must be flagged as stray.") {
		t.Fatalf("expected exactly the intro prose line to be stray, got %v", lines)
	}
	inside, ok := NarrativeStrayProseLines("## Design Narrative\n\n### Only\n\nBody.\n")
	if !ok || len(inside) != 0 {
		t.Fatalf("block-internal prose is not stray, got %v", inside)
	}
	if _, ok := NarrativeStrayProseLines("## Promises\n\npromises:\n  - id: x.y\n"); ok {
		t.Fatal("a spec without Design Narrative must report not-found")
	}
}

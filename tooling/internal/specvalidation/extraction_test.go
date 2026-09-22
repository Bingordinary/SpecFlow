package specvalidation

import (
	"reflect"
	"testing"
)

const extractionSpec = `---
id: demo
version: 0.1.0
unit_refs: none
rule_refs: none
---
acceptance_item_set:
  - id: demo.core
    description: Demo behavior.
    verification_type: auto
    verification_surface: internal_flow
    implementation_surface: internal/demo
    verification_method: check
    pass_condition: ok
    runnable: yes
    affects:
      files:
        - internal/demo/handler.go
      appendices:
        - unit_demo_evidence.md
  - id: demo.aux
    description: Aux behavior.
    verification_type: auto
    verification_surface: internal_flow
    implementation_surface: <pending>
    verification_method: check
    pass_condition: ok
    runnable: yes
    affects:
      files:
        - src/a.go
`

func TestExtractAffectsFiles_MiddlePosition(t *testing.T) {
	got := ExtractAffectsFiles(extractionSpec)
	want := []string{"internal/demo/handler.go", "src/a.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestExtractAffectsFiles_NoSet(t *testing.T) {
	if got := ExtractAffectsFiles("no acceptance set here"); len(got) != 0 {
		t.Fatalf("expected no files, got %v", got)
	}
}

func TestExtractImplementationSurfaces(t *testing.T) {
	got := ExtractImplementationSurfaces(extractionSpec)
	want := []string{"internal/demo", "<pending>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestExtractImplementationSurfaces_NoSet(t *testing.T) {
	if got := ExtractImplementationSurfaces("no acceptance set here"); len(got) != 0 {
		t.Fatalf("expected no surfaces, got %v", got)
	}
}

// A later top-level section must terminate the scan: none of its
// lookalike content (- id:, implementation_surface:, files:) may leak
// into the extraction.
const extractionSpecWithLaterSection = extractionSpec + `## Definition of Done

Unrelated documentation list

- id: unrelated.example
  description: not part of the acceptance set

implementation_surface: docs/specs/units/candidate/unit_demo.md

        - unrelated/path.go
`

func TestExtractAcceptanceItemIDs_StopsAtLaterSection(t *testing.T) {
	got := ExtractAcceptanceItemIDs(extractionSpecWithLaterSection)
	want := []string{"demo.core", "demo.aux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestExtractAffectsFiles_StopsAtLaterSection(t *testing.T) {
	got := ExtractAffectsFiles(extractionSpecWithLaterSection)
	want := []string{"internal/demo/handler.go", "src/a.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestExtractImplementationSurfaces_StopsAtLaterSection(t *testing.T) {
	got := ExtractImplementationSurfaces(extractionSpecWithLaterSection)
	want := []string{"internal/demo", "<pending>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestExtractAcceptanceItemIDs_FencedExampleIgnored(t *testing.T) {
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: |\n      Real item.\n\n      ```\n      - id: example.only\n        description: A fenced example, not an item.\n      ```\n    verification_type: auto\n    implementation_surface: internal/demo\n  - id: demo.aux\n    description: Real item.\n    verification_type: auto\n    implementation_surface: internal/demo\n"
	got := ExtractAcceptanceItemIDs(spec)
	want := []string{"demo.core", "demo.aux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestExtractAcceptanceItemIDs_ProseMentionNotMarker(t *testing.T) {
	spec := "---\nid: demo\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Demo\n\n## Notes\n\nThe acceptance_item_set: marker starts the structured item list.\n\n  - id: not.an.item\n    description: Documentation example, no marker.\n"
	if got := ExtractAcceptanceItemIDs(spec); len(got) != 0 {
		t.Fatalf("expected no ids from a prose marker mention, got %v", got)
	}
}

func TestExtractAcceptanceFields_FencedSetBeforeRealSetIgnored(t *testing.T) {
	spec := `# Demo

~~~yaml
acceptance_item_set:
  - id: example.only
    implementation_surface: fake/example
    affects:
      files:
        - fake/example.go
~~~

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: demo.core
    implementation_surface: internal/demo
    affects:
      files:
        - internal/demo/handler.go
`
	if got, want := ExtractAcceptanceItemIDs(spec), []string{"demo.core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids: expected %v, got %v", want, got)
	}
	if got, want := ExtractImplementationSurfaces(spec), []string{"internal/demo"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("surfaces: expected %v, got %v", want, got)
	}
	if got, want := ExtractAffectsFiles(spec), []string{"internal/demo/handler.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files: expected %v, got %v", want, got)
	}
}

func TestExtractAcceptanceFields_AcrossSubheading(t *testing.T) {
	// A `###` subheading inside the enclosing `##` section is set content —
	// items on both sides of it remain part of the item set.
	spec := `# Demo

## Testability / Acceptance Criteria

acceptance_item_set:
  - id: demo.core
    implementation_surface: internal/demo
    affects:
      files:
        - internal/demo/handler.go

### Extra structure

  - id: demo.aux
    implementation_surface: internal/aux
    affects:
      files:
        - internal/aux/aux.go

## Dependencies

None.
`
	if got, want := ExtractAcceptanceItemIDs(spec), []string{"demo.core", "demo.aux"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids: expected %v, got %v", want, got)
	}
	if got, want := ExtractImplementationSurfaces(spec), []string{"internal/demo", "internal/aux"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("surfaces: expected %v, got %v", want, got)
	}
	if got, want := ExtractAffectsFiles(spec), []string{"internal/demo/handler.go", "internal/aux/aux.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files: expected %v, got %v", want, got)
	}
}

func TestExtractAcceptanceFields_FencedItemContentIgnored(t *testing.T) {
	spec := `# Demo

acceptance_item_set:
  - id: demo.core
    description: |
      The following is only an example:
      ~~~yaml
    implementation_surface: fake/example
    affects:
      files:
        - fake/example.go
      ~~~
    implementation_surface: internal/demo
    affects:
      files:
        - internal/demo/handler.go
`
	if got, want := ExtractAcceptanceItemIDs(spec), []string{"demo.core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids: expected %v, got %v", want, got)
	}
	if got, want := ExtractImplementationSurfaces(spec), []string{"internal/demo"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("surfaces: expected %v, got %v", want, got)
	}
	if got, want := ExtractAffectsFiles(spec), []string{"internal/demo/handler.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files: expected %v, got %v", want, got)
	}
}

// TestExtractAcceptanceFields_ItemRelativeIndent verifies item fields are read
// at the item's own nesting: the block-sequence form (dash at the set indent)
// and a consistently deeper item block both read the surface and the
// affects.files list.
func TestExtractAcceptanceFields_ItemRelativeIndent(t *testing.T) {
	cases := []struct {
		name  string
		item  string
		field string
	}{
		{"block sequence at the set indent", "- id: demo.core", "  "},
		{"item block nested deeper", "    - id: demo.core", "      "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := "# Demo\n\nacceptance_item_set:\n" +
				tc.item + "\n" +
				tc.field + "description: Demo behavior.\n" +
				tc.field + "implementation_surface: internal/demo\n" +
				tc.field + "affects:\n" +
				tc.field + "  files:\n" +
				tc.field + "    - internal/demo/handler.go\n"
			if got, want := ExtractImplementationSurfaces(spec), []string{"internal/demo"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("surfaces: expected %v, got %v", want, got)
			}
			if got, want := ExtractAffectsFiles(spec), []string{"internal/demo/handler.go"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("files: expected %v, got %v", want, got)
			}
		})
	}
}

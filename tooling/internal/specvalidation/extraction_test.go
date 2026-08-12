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

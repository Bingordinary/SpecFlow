package specvalidation

import (
	"reflect"
	"strings"
	"testing"
)

const extractionSpec = `---
id: demo
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
	spec := "---\nid: demo\nrule_refs: none\n---\n\n# Demo\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: demo.core\n    description: |\n      Real item.\n\n      ```\n      - id: example.only\n        description: A fenced example, not an item.\n      ```\n    verification_type: auto\n    implementation_surface: internal/demo\n  - id: demo.aux\n    description: Real item.\n    verification_type: auto\n    implementation_surface: internal/demo\n"
	got := ExtractAcceptanceItemIDs(spec)
	want := []string{"demo.core", "demo.aux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestExtractAcceptanceItemIDs_ProseMentionNotMarker(t *testing.T) {
	spec := "---\nid: demo\nrule_refs: none\n---\n\n# Demo\n\n## Notes\n\nThe acceptance_item_set: marker starts the structured item list.\n\n  - id: not.an.item\n    description: Documentation example, no marker.\n"
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

// TestExtractAcceptanceFields_AffectsSequenceAtFieldColumn verifies that a
// block-sequence entry indented at its affects field's own column (the
// indentless sequence form) is read like the deeper form — both are legal YAML
// (issue #96).
func TestExtractAcceptanceFields_AffectsSequenceAtFieldColumn(t *testing.T) {
	spec := "# Demo\n\nacceptance_item_set:\n" +
		"- id: demo.core\n" +
		"  description: Demo behavior.\n" +
		"  implementation_surface: internal/demo\n" +
		"  affects:\n" +
		"    files:\n" +
		"    - internal/demo/handler.go\n" +
		"    evidence_files:\n" +
		"    - internal/demo/handler_test.go\n"
	if got, want := ExtractAffectsFiles(spec), []string{"internal/demo/handler.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files: expected %v, got %v", want, got)
	}
	if got, want := ExtractAffectsEvidenceFiles(spec), []string{"internal/demo/handler_test.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("evidence files: expected %v, got %v", want, got)
	}
	if got, want := ItemCodeSurfaces(spec)["demo.core"], []string{"internal/demo", "internal/demo/handler.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("item code surfaces: expected %v, got %v", want, got)
	}
}

// TestParseAcceptanceItems_UnattributableAffectsLinesReported verifies the
// parser fails closed instead of silently dropping an affects declaration it
// cannot attribute (issue #96).
func TestParseAcceptanceItems_UnattributableAffectsLinesReported(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want string
	}{
		{
			name: "entry before any affects field",
			spec: "- id: demo.core\n  affects:\n    - internal/demo/handler.go\n",
			want: "not under a recognised affects field",
		},
		{
			name: "unknown affects field",
			spec: "- id: demo.core\n  affects:\n    file:\n      - internal/demo/handler.go\n",
			want: "unknown affects field",
		},
		{
			name: "unrecognised line in the affects mapping",
			spec: "- id: demo.core\n  affects:\n    files:\n      - internal/demo/handler.go\n    stray\n",
			want: "not a recognised affects field",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := parseAcceptanceItems("# Demo\n\nacceptance_item_set:\n" + tc.spec)
			if len(items) != 1 {
				t.Fatalf("expected one item, got %d", len(items))
			}
			if joined := strings.Join(items[0].affectsProblems, "; "); !strings.Contains(joined, tc.want) {
				t.Fatalf("expected a problem containing %q, got %v", tc.want, items[0].affectsProblems)
			}
		})
	}
}

// TestParseAcceptanceItems_H3SubheadingBetweenItemsNoProblem verifies a `###`
// subheading at column 0 between items (legal per
// framework/spec_writing_guide.md §7) is not mistaken for an unreadable affects
// line (issue #96).
func TestParseAcceptanceItems_H3SubheadingBetweenItemsNoProblem(t *testing.T) {
	spec := "# Demo\n\nacceptance_item_set:\n" +
		"- id: demo.a\n" +
		"  affects:\n" +
		"    files:\n" +
		"    - internal/demo/a.go\n" +
		"### Related items\n" +
		"- id: demo.b\n" +
		"  affects:\n" +
		"    files:\n" +
		"    - internal/demo/b.go\n"
	if got, want := ExtractAffectsFiles(spec), []string{"internal/demo/a.go", "internal/demo/b.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files: expected %v, got %v", want, got)
	}
	for _, item := range parseAcceptanceItems(spec) {
		if len(item.affectsProblems) != 0 {
			t.Fatalf("item %s: expected no affects problems, got %v", item.id, item.affectsProblems)
		}
	}
}

// TestParseAcceptanceItems_CommentInsideAffectsIsIgnored verifies that a YAML
// comment line inside the affects mapping — including one indented at the
// item-field level — never terminates the mapping: the subfields after it are
// still read and no unattributable-line problem is recorded (issue #96
// fail-closed).
func TestParseAcceptanceItems_CommentInsideAffectsIsIgnored(t *testing.T) {
	cases := []struct {
		name  string
		spec  string
		files []string
		rules []string
	}{
		{
			name:  "comment at the item-field indent before the first subfield",
			spec:  "- id: demo.a\n  affects:\n  # verified files\n    files:\n    - internal/demo/a.go\n",
			files: []string{"internal/demo/a.go"},
		},
		{
			name:  "comment at the item-field indent between subfields",
			spec:  "- id: demo.a\n  affects:\n    files:\n    - internal/demo/a.go\n  # provisional\n    rules:\n    - g_rule_example\n",
			files: []string{"internal/demo/a.go"},
			rules: []string{"g_rule_example"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := parseAcceptanceItems("# Demo\n\nacceptance_item_set:\n" + tc.spec)
			if len(items) != 1 {
				t.Fatalf("expected one item, got %d", len(items))
			}
			if got := items[0].affectsFiles; !reflect.DeepEqual(got, tc.files) {
				t.Fatalf("files: expected %v, got %v", tc.files, got)
			}
			if got := items[0].affectsRules; !reflect.DeepEqual(got, tc.rules) {
				t.Fatalf("rules: expected %v, got %v", tc.rules, got)
			}
			if len(items[0].affectsProblems) != 0 {
				t.Fatalf("expected no affects problems, got %v", items[0].affectsProblems)
			}
		})
	}
}

const extractionEvidenceSpec = `---
id: demo
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
      evidence_files:
        - internal/demo/handler_test.go
        - internal/peer/peer_test.go
  - id: demo.aux
    description: Aux behavior.
    verification_type: auto
    verification_surface: internal_flow
    implementation_surface: <pending>
    verification_method: check
    pass_condition: ok
    runnable: yes
`

func TestExtractAffectsEvidenceFiles(t *testing.T) {
	got := ExtractAffectsEvidenceFiles(extractionEvidenceSpec)
	want := []string{"internal/demo/handler_test.go", "internal/peer/peer_test.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	// Evidence files never leak into the implementation declaration list.
	if got, want := ExtractAffectsFiles(extractionEvidenceSpec), []string{"internal/demo/handler.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected affects.files %v, got %v", want, got)
	}
}

func TestItemCodeSurfaces(t *testing.T) {
	got := ItemCodeSurfaces(extractionSpec)
	want := map[string][]string{
		"demo.core": {"internal/demo", "internal/demo/handler.go"},
		"demo.aux":  {"src/a.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestItemCodeSurfaces_NoSet(t *testing.T) {
	if got := ItemCodeSurfaces("no acceptance set here"); len(got) != 0 {
		t.Fatalf("expected no surfaces, got %v", got)
	}
}

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// sourceRepoRoot locates this repository's root (the tooling package lives at
// tooling/cmd/specflowctl); its framework documents live under framework/.
func sourceRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestProtocolSectionsResolveAgainstSourceDocs locks the session-to-section
// mapping onto the real framework documents: every mission shape this tool
// renders must resolve exactly against the deployed documents, so a heading
// rename or section removal breaks the build here instead of degrading
// missions back to document pointers at run time.
func TestProtocolSectionsResolveAgainstSourceDocs(t *testing.T) {
	root := sourceRepoRoot(t)
	cases := []struct {
		name       string
		gate       string
		targetKind string
		kind       string
		keys       []string
		wantChunks []protocolChunk
	}{
		{
			name: "validate unit checks", gate: gaterun.GateValidate, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindChecks, keys: []string{"2", "4", "5", "7", "8", "9", "10"},
			wantChunks: []protocolChunk{
				{Source: "framework/unit_validate_checklist.md", Heading: "Target-layer applicability"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Prerequisite"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Execution Rules"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Severity handling"},
				{Source: "framework/spec_writing_guide.md", Heading: "15.2 Externally observable only"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Check 2"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Check 4"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Check 5"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Check 7"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Check 8"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Check 9"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Check 10"},
			},
		},
		{
			name: "validate rule checks", gate: gaterun.GateValidate, targetKind: gaterun.TargetKindRule,
			kind: gaterun.SessionKindChecks, keys: []string{"3", "4"},
			wantChunks: []protocolChunk{
				{Source: "framework/rule_validate_checklist.md", Heading: "Execution Rules"},
				{Source: "framework/rule_validate_checklist.md", Heading: "Target-layer applicability"},
				{Source: "framework/rule_validate_checklist.md", Heading: "Severity handling"},
				{Source: "framework/spec_writing_guide.md", Heading: "15.2 Externally observable only"},
				{Source: "framework/rule_validate_checklist.md", Heading: "Check 3"},
				{Source: "framework/rule_validate_checklist.md", Heading: "Check 4"},
			},
		},
		{
			name: "delta review", gate: gaterun.GateValidate, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindDeltaReview, keys: []string{gaterun.DeltaReviewKey},
			wantChunks: []protocolChunk{
				{Source: "framework/verification_scope.md", Heading: "The change review (validate)"},
			},
		},
		{
			name: "verify item session", gate: gaterun.GateVerify, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindItem, keys: []string{"item:auth:auth.core"},
			wantChunks: []protocolChunk{
				{Source: "framework/unit_verify_checklist.md", Heading: "Target-layer applicability"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Prerequisite"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 2"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 3"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 4"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 6"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 7"},
			},
		},
		{
			name: "verify unit scan session", gate: gaterun.GateVerify, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindUnit, keys: []string{"unit:auth"},
			wantChunks: []protocolChunk{
				{Source: "framework/unit_verify_checklist.md", Heading: "Target-layer applicability"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Prerequisite"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 1"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 5"},
				{Source: "framework/unit_verify_checklist.md", Heading: "Step 7"},
			},
		},
		{
			name: "verify quality sessions", gate: gaterun.GateVerify, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindDesign, keys: []string{"design:auth:internal/a.go"},
			wantChunks: []protocolChunk{
				{Source: "framework/unit_verify_checklist.md", Heading: "Quality Lens Standard"},
			},
		},
		{
			name: "verify architecture session", gate: gaterun.GateVerify, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindArchitecture, keys: []string{"architecture:auth"},
			wantChunks: []protocolChunk{
				{Source: "framework/unit_verify_checklist.md", Heading: "Quality Lens Standard"},
			},
		},
		{
			name: "final synthesis", gate: gaterun.GateVerify, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindCross, keys: []string{gaterun.CrossKey},
			wantChunks: []protocolChunk{
				{Source: "framework/verification_scope.md", Heading: "Final synthesis"},
				{Source: "framework/verification_scope.md", Heading: "Relationship vocabulary"},
				{Source: "framework/severity_policy.md", Heading: "4. Severity Levels"},
				{Source: "framework/severity_policy.md", Heading: "5. Blocking Relationship"},
				{Source: "framework/severity_policy.md", Heading: "9. Severity Consistency Check"},
			},
		},
		{
			name: "validate final synthesis adds the extraction re-verification standard", gate: gaterun.GateValidate, targetKind: gaterun.TargetKindUnit,
			kind: gaterun.SessionKindCross, keys: []string{gaterun.CrossKey},
			wantChunks: []protocolChunk{
				{Source: "framework/verification_scope.md", Heading: "Final synthesis"},
				{Source: "framework/verification_scope.md", Heading: "Relationship vocabulary"},
				{Source: "framework/severity_policy.md", Heading: "4. Severity Levels"},
				{Source: "framework/severity_policy.md", Heading: "5. Blocking Relationship"},
				{Source: "framework/severity_policy.md", Heading: "9. Severity Consistency Check"},
				{Source: "framework/unit_validate_checklist.md", Heading: "Extraction re-verification (Check 2 Step 1, Check 5)"},
				{Source: "framework/spec_writing_guide.md", Heading: "15.2 Externally observable only"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sections, err := protocolSectionsFor(tc.gate, tc.targetKind, tc.kind, tc.keys)
			if err != nil {
				t.Fatalf("map session to sections: %v", err)
			}
			chunks, err := inlineProtocol(root, "framework", sections)
			if err != nil {
				t.Fatalf("inline protocol: %v", err)
			}
			if len(chunks) != len(tc.wantChunks) {
				t.Fatalf("got %d chunks, want %d: %+v", len(chunks), len(tc.wantChunks), chunkSummaries(chunks))
			}
			for i, want := range tc.wantChunks {
				got := chunks[i]
				if got.Source != want.Source || judgments.SectionSlug(got.Heading) != want.Heading {
					t.Fatalf("chunk %d = %s/%s, want %s/%s", i, got.Source, got.Heading, want.Source, want.Heading)
				}
				if !strings.Contains(got.Content, "\n") || strings.TrimSpace(got.Content) == "" {
					t.Fatalf("chunk %d (%s/%s) has an empty body", i, got.Source, got.Heading)
				}
				if !strings.HasPrefix(got.Content, "#") {
					t.Fatalf("chunk %d (%s/%s) must start with its heading line, got: %q", i, got.Source, got.Heading, firstLine(got.Content))
				}
				if strings.Contains(got.Content, "==ATOM_") {
					t.Fatalf("chunk %d (%s/%s) leaks an atom delivery marker", i, got.Source, got.Heading)
				}
			}
		})
	}
}

// TestFinalSynthesisCarriesExtractionStandard proves the validate final
// synthesis hands the reviewer the extraction re-verification standard the
// checklist assigns to it, while the verify synthesis stays without it (a
// verify synthesis re-verifies no extraction artifacts).
func TestFinalSynthesisCarriesExtractionStandard(t *testing.T) {
	root := sourceRepoRoot(t)
	chunksByHeading := func(gate string) map[string]string {
		sections, err := protocolSectionsFor(gate, gaterun.TargetKindUnit, gaterun.SessionKindCross, []string{gaterun.CrossKey})
		if err != nil {
			t.Fatalf("%s cross sections: %v", gate, err)
		}
		chunks, err := inlineProtocol(root, "framework", sections)
		if err != nil {
			t.Fatalf("%s cross protocol inline: %v", gate, err)
		}
		out := map[string]string{}
		for _, chunk := range chunks {
			out[judgments.SectionSlug(chunk.Heading)] = chunk.Content
		}
		return out
	}

	const extraction = "Extraction re-verification (Check 2 Step 1, Check 5)"
	const boundary = "15.2 Externally observable only"

	validate := chunksByHeading(gaterun.GateValidate)
	body, ok := validate[extraction]
	if !ok {
		t.Fatalf("validate final synthesis omits %q", extraction)
	}
	if !strings.Contains(body, "Re-verification failure") {
		t.Fatalf("%q omits its suppression rule", extraction)
	}
	if _, ok := validate[boundary]; !ok {
		t.Fatalf("validate final synthesis omits %q, the boundary its classification step depends on", boundary)
	}

	if _, ok := chunksByHeading(gaterun.GateVerify)[extraction]; ok {
		t.Fatalf("verify final synthesis carries %q; verify has no extraction artifacts to re-verify", extraction)
	}
}

// TestValidateCheckSectionCoversSubChecks proves a validate check section
// carries its sub-checks: Check 5 owns sub-checks 5a/5b/5e/5h, and the
// mission must include them.
func TestValidateCheckSectionCoversSubChecks(t *testing.T) {
	root := sourceRepoRoot(t)
	chunks, err := inlineProtocol(root, "framework", []protocolSectionRef{{"unit_validate_checklist.md", 2, "Check 5"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	for _, sub := range []string{"Sub-check 5a", "Sub-check 5b", "Sub-check 5e", "Sub-check 5h"} {
		if !strings.Contains(chunks[0].Content, sub) {
			t.Fatalf("Check 5 section omits %s", sub)
		}
	}
}

// TestUnitValidateChecklistIsLayerAware locks the stable-only unit
// confirmation fix onto the real checklist: the inlined Prerequisite resolves
// the main spec and appendices from the planned layer, Check 1's mechanical
// pre-pass is candidate-only, and Check 8's promise-model command is
// layer-parameterized, so a stable-only reviewer is not told to read a
// candidate path or run a command that cannot succeed.
func TestUnitValidateChecklistIsLayerAware(t *testing.T) {
	root := sourceRepoRoot(t)
	chunks, err := inlineProtocol(root, "framework", []protocolSectionRef{
		{"unit_validate_checklist.md", 2, "Target-layer applicability"},
		{"unit_validate_checklist.md", 2, "Prerequisite"},
		{"unit_validate_checklist.md", 2, "Check 1"},
		{"unit_validate_checklist.md", 2, "Check 8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	byHeading := map[string]string{}
	for _, c := range chunks {
		byHeading[judgments.SectionSlug(c.Heading)] = c.Content
	}
	prereq, ok := byHeading["Prerequisite"]
	if !ok || !strings.Contains(prereq, "docs/specs/units/{layer}/unit_{unit}.md") || strings.Contains(prereq, "docs/specs/units/candidate/unit_{unit}.md") {
		t.Fatalf("Prerequisite is not layer-aware:\n%s", prereq)
	}
	check1, ok := byHeading["Check 1"]
	if !ok || !strings.Contains(check1, "candidate targets only") || !strings.Contains(check1, "A stable-only target runs no mechanical pre-pass") {
		t.Fatalf("Check 1 is not layer-aware:\n%s", check1)
	}
	check8, ok := byHeading["Check 8"]
	if !ok || !strings.Contains(check8, "specflowctl validate promise-model --unit <name> --layer <candidate|stable>") || strings.Contains(check8, "`validate candidate`") {
		t.Fatalf("Check 8 is not layer-aware:\n%s", check8)
	}
}

// TestUnitVerifyChecklistIsLayerAware locks the stable-only verify fix onto
// the real checklist: the inlined Prerequisite resolves the main spec and
// appendices from the planned layer, and the layer section tells a stable-only
// reviewer this is a drift confirmation that does not enter divergence
// resolution.
func TestUnitVerifyChecklistIsLayerAware(t *testing.T) {
	root := sourceRepoRoot(t)
	chunks, err := inlineProtocol(root, "framework", []protocolSectionRef{
		{"unit_verify_checklist.md", 2, "Target-layer applicability"},
		{"unit_verify_checklist.md", 2, "Prerequisite"},
	})
	if err != nil {
		t.Fatal(err)
	}
	byHeading := map[string]string{}
	for _, c := range chunks {
		byHeading[judgments.SectionSlug(c.Heading)] = c.Content
	}
	prereq, ok := byHeading["Prerequisite"]
	if !ok || !strings.Contains(prereq, "docs/specs/units/{layer}/unit_{unit}.md") || strings.Contains(prereq, "docs/specs/units/candidate/unit_{unit}.md") {
		t.Fatalf("Prerequisite is not layer-aware:\n%s", prereq)
	}
	layer, ok := byHeading["Target-layer applicability"]
	if !ok || !strings.Contains(layer, "drift confirmation") || !strings.Contains(layer, "Step 7") {
		t.Fatalf("Target-layer applicability does not state the stable-only drift rule:\n%s", layer)
	}
}

// TestQualityLensStandardCoversDimension8 proves the quality sessions'
// inlined standard carries the architecture dimensions.
func TestQualityLensStandardCoversDimension8(t *testing.T) {
	root := sourceRepoRoot(t)
	chunks, err := inlineProtocol(root, "framework", []protocolSectionRef{{"unit_verify_checklist.md", 2, "Quality Lens Standard"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if !strings.Contains(chunks[0].Content, "Dimension 8") {
		t.Fatal("Quality Lens Standard section omits Dimension 8")
	}
}

// TestUnmappedSessionKindIsRejected pins the fail-closed contract of the
// mapping itself.
func TestUnmappedSessionKindIsRejected(t *testing.T) {
	if _, err := protocolSectionsFor(gaterun.GateVerify, gaterun.TargetKindUnit, "unknown-kind", nil); err == nil {
		t.Fatal("unknown session kind must be rejected")
	}
}

func TestExtractSection(t *testing.T) {
	doc := "# Doc\n" +
		"\n" +
		"## Alpha — First section\n" +
		"\n" +
		"alpha body\n" +
		"```text\n" +
		"# not a heading inside a fence\n" +
		"## also not a heading\n" +
		"```\n" +
		"==ATOM_BEGIN:demo==\n" +
		"alpha atom body\n" +
		"==ATOM_END:demo==\n" +
		"### Alpha nested\n" +
		"\n" +
		"nested body\n" +
		"## Beta — Second section\n" +
		"\n" +
		"beta body\n"

	t.Run("section spans until the next same-level heading and keeps nested content", func(t *testing.T) {
		body, err := judgments.ExtractSection(doc, 2, "Alpha")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"## Alpha — First section", "alpha body", "alpha atom body", "### Alpha nested", "nested body"} {
			if !strings.Contains(body, want) {
				t.Fatalf("section omits %q:\n%s", want, body)
			}
		}
		if strings.Contains(body, "beta body") {
			t.Fatal("section leaks the following section")
		}
	})
	t.Run("fenced lines never open or close a section", func(t *testing.T) {
		body, err := judgments.ExtractSection(doc, 2, "Alpha")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "## also not a heading") {
			t.Fatal("fenced content must stay inside the section")
		}
	})
	t.Run("atom markers are stripped", func(t *testing.T) {
		body, err := judgments.ExtractSection(doc, 2, "Alpha")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "==ATOM_") {
			t.Fatal("ExtractSection must return the raw section; stripping happens at mission rendering")
		}
		stripped := stripAtomMarkers(body)
		if strings.Contains(stripped, "==ATOM_") {
			t.Fatalf("atom markers must be stripped:\n%s", stripped)
		}
		if !strings.Contains(stripped, "alpha atom body") {
			t.Fatal("stripping must keep the section content")
		}
	})
	t.Run("slug match ignores the heading description", func(t *testing.T) {
		if _, err := judgments.ExtractSection(doc, 2, "Beta"); err != nil {
			t.Fatalf("slug match failed: %v", err)
		}
	})
	t.Run("missing section is an error", func(t *testing.T) {
		if _, err := judgments.ExtractSection(doc, 2, "Gamma"); err == nil {
			t.Fatal("missing section must be an error")
		}
	})
	t.Run("ambiguous section is an error", func(t *testing.T) {
		if _, err := judgments.ExtractSection(doc+"\n## Beta — Third section\n\nmore\n", 2, "Beta"); err == nil {
			t.Fatal("ambiguous section must be an error")
		}
	})
}

func chunkSummaries(chunks []protocolChunk) []string {
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, chunk.Source+"/"+chunk.Heading)
	}
	return out
}

func firstLine(s string) string {
	if idx := strings.Index(s, "\n"); idx >= 0 {
		return s[:idx]
	}
	return s
}

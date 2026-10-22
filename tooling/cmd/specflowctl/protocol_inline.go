package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// protocolSectionRef names one framework document section a session mission
// inlines: the document relative to the framework root, the heading level,
// and the heading slug (the title text before " — ", or the full title when
// no em dash follows).
type protocolSectionRef struct {
	File    string
	Level   int
	Heading string
}

// protocolChunk is one inlined framework section inside a rendered mission.
type protocolChunk struct {
	Source  string `json:"source"`
	Heading string `json:"heading"`
	Content string `json:"content"`
}

// protocolSectionsFor maps a session (gate, target kind, session kind,
// conclusion keys) to the framework sections its mission must carry inline.
// The mission is self-contained: the reviewer judges from this text instead
// of opening the framework documents, so the per-session loading cost is one
// mission read rather than several full-document reads.
func protocolSectionsFor(gate, targetKind, kind string, keys []string) ([]protocolSectionRef, error) {
	switch {
	case kind == gaterun.SessionKindDeltaReview:
		return []protocolSectionRef{
			{"verification_scope.md", 3, "The change review (validate)"},
		}, nil
	case kind == gaterun.SessionKindCross:
		// The final synthesis runs for every gate that can produce findings
		// or relationships (only a rule target never synthesizes), so its
		// judging standard is gate-independent.
		return finalSynthesisSections(), nil
	case gate == gaterun.GateValidate && targetKind == gaterun.TargetKindRule:
		sections := []protocolSectionRef{
			{"rule_validate_checklist.md", 2, "Execution Rules"},
			{"rule_validate_checklist.md", 2, "Target-layer applicability"},
			{"rule_validate_checklist.md", 3, "Severity handling"},
			{"spec_writing_guide.md", 3, "15.2 Externally observable only"},
		}
		for _, key := range keys {
			sections = append(sections, protocolSectionRef{"rule_validate_checklist.md", 3, "Check " + key})
		}
		return sections, nil
	case gate == gaterun.GateValidate:
		sections := []protocolSectionRef{
			{"unit_validate_checklist.md", 2, "Prerequisite"},
			{"unit_validate_checklist.md", 2, "Execution Rules"},
			{"unit_validate_checklist.md", 3, "Severity handling"},
			{"spec_writing_guide.md", 3, "15.2 Externally observable only"},
		}
		for _, key := range keys {
			sections = append(sections, protocolSectionRef{"unit_validate_checklist.md", 2, "Check " + key})
		}
		return sections, nil
	case gate == gaterun.GateVerify:
		switch kind {
		case gaterun.SessionKindItem:
			return append(verifyPrerequisiteSections(),
				verifyStepSection("Step 2"), verifyStepSection("Step 3"), verifyStepSection("Step 4"),
				verifyStepSection("Step 6"), verifyStepSection("Step 7")), nil
		case gaterun.SessionKindUnit:
			return append(verifyPrerequisiteSections(),
				verifyStepSection("Step 1"), verifyStepSection("Step 5"), verifyStepSection("Step 7")), nil
		case gaterun.SessionKindCode, gaterun.SessionKindDesign, gaterun.SessionKindArchitecture:
			return []protocolSectionRef{
				{"unit_verify_checklist.md", 2, "Quality Lens Standard"},
			}, nil
		}
	}
	return nil, fmt.Errorf("no protocol sections mapped for gate %q target %q session kind %q", gate, targetKind, kind)
}

// finalSynthesisSections lists the framework sections the final synthesis
// judges by: the synthesis rules plus the severity standard it disposes
// findings under.
func finalSynthesisSections() []protocolSectionRef {
	return []protocolSectionRef{
		{"verification_scope.md", 3, "Final synthesis"},
		{"verification_scope.md", 3, "Relationship vocabulary"},
		{"severity_policy.md", 2, "4. Severity Levels"},
		{"severity_policy.md", 2, "5. Blocking Relationship"},
		{"severity_policy.md", 2, "9. Severity Consistency Check"},
	}
}

func verifyPrerequisiteSections() []protocolSectionRef {
	return []protocolSectionRef{{"unit_verify_checklist.md", 2, "Prerequisite"}}
}

func verifyStepSection(step string) protocolSectionRef {
	return protocolSectionRef{"unit_verify_checklist.md", 2, step}
}

// inlineProtocol renders the requested framework sections as mission chunks,
// in mapping order. It reads the deployed framework documents — the same
// files the former protocol pointer named — so the inlined text cannot drift
// from the judging standard the run is bound to, and it reuses the framework
// document section parser the protocol fingerprint uses. A missing or
// ambiguous section is an error: the framework documents and this mapping
// must stay aligned, and a silent fallback to a pointer would hide the drift.
// Source names the deployed document path relative to the repository root,
// the same provenance the mission's protocol_ref carries.
func inlineProtocol(root, frameworkRoot string, sections []protocolSectionRef) ([]protocolChunk, error) {
	documents := map[string]string{}
	var chunks []protocolChunk
	for _, ref := range sections {
		content, ok := documents[ref.File]
		if !ok {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(frameworkRoot), filepath.FromSlash(ref.File)))
			if err != nil {
				return nil, fmt.Errorf("read protocol document %s: %w", ref.File, err)
			}
			content = string(raw)
			documents[ref.File] = content
		}
		body, err := judgments.ExtractSection(content, ref.Level, ref.Heading)
		if err != nil {
			return nil, fmt.Errorf("framework/%s: %w", ref.File, err)
		}
		source := filepath.ToSlash(filepath.Join(filepath.FromSlash(frameworkRoot), filepath.FromSlash(ref.File)))
		chunks = append(chunks, protocolChunk{Source: source, Heading: sectionTitle(body, ref), Content: stripAtomMarkers(body)})
	}
	return chunks, nil
}

// sectionTitle returns the heading title of the extracted body's first line.
func sectionTitle(body string, ref protocolSectionRef) string {
	line := body
	if idx := strings.Index(body, "\n"); idx >= 0 {
		line = body[:idx]
	}
	if _, title, ok := judgments.HeadingTitle(line); ok {
		return title
	}
	return ref.Heading
}

// stripAtomMarkers removes atom delivery marker lines
// (==ATOM_BEGIN:id== / ==ATOM_END:id==). Markers are repository machinery,
// not protocol content, and never belong in a rendered mission.
func stripAtomMarkers(body string) string {
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "==ATOM_BEGIN:") || strings.HasPrefix(trimmed, "==ATOM_END:") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

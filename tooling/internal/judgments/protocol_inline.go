package judgments

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
)

// ProtocolSessionKind values name the session class a protocol mapping is
// computed for. They mirror the gate-run session kinds (`gaterun.SessionKind*`)
// and exist here so the protocol mapping and every standard fingerprint can be
// computed without importing the gate-run package.
const (
	ProtocolSessionChecks       = "checks"
	ProtocolSessionItem         = "item"
	ProtocolSessionUnit         = "unit"
	ProtocolSessionDesign       = "design"
	ProtocolSessionCode         = "code"
	ProtocolSessionArchitecture = "architecture"
	ProtocolSessionCross        = "cross"
)

// ProtocolSection names one framework document section a session mission
// inlines: the document relative to the framework root, the heading level, and
// the heading slug (the title text before " — ", or the full title when no em
// dash follows).
type ProtocolSection struct {
	File    string
	Level   int
	Heading string
}

// ProtocolChunk is one unit of protocol text: an inlined mission section or a
// shared-standard document chunk. A standard fingerprint is computed from
// these fields, so an inlined check section and its binding share one shape.
type ProtocolChunk struct {
	Source  string `json:"source"`
	Heading string `json:"heading"`
	Content string `json:"content"`
}

// ProtocolSectionsFor maps a session (gate, target kind, session kind,
// conclusion keys) to the framework sections its mission must carry inline.
// The mission also names the shared protocol documents; a session's applied
// standard is the inlined sections plus that shared text.
func ProtocolSectionsFor(gate, targetKind, kind string, keys []string) ([]ProtocolSection, error) {
	switch {
	case kind == ProtocolSessionCross:
		// The final synthesis runs for every gate that can produce findings
		// or relationships (only a rule target never synthesizes), so its
		// core judging standard is gate-independent. Validate additionally
		// re-verifies the extraction artifacts of its Check 2 Step 1 and
		// Check 5 findings (unit_validate_checklist.md Step 9), so its mission
		// carries that standard and the §15.2 boundary it classifies against.
		sections := FinalSynthesisSections()
		if gate == "validate" {
			sections = append(sections,
				ProtocolSection{"unit_validate_checklist.md", 2, "Extraction re-verification (Check 2 Step 1, Check 5)"},
				ProtocolSection{"spec_writing_guide.md", 3, "15.2 Externally observable only"},
			)
		}
		return sections, nil
	case gate == "validate" && targetKind == "rule":
		sections := []ProtocolSection{
			{"rule_validate_checklist.md", 2, "Execution Rules"},
			{"rule_validate_checklist.md", 2, "Target-layer applicability"},
			{"rule_validate_checklist.md", 3, "Severity handling"},
			{"spec_writing_guide.md", 3, "15.2 Externally observable only"},
		}
		for _, key := range keys {
			sections = append(sections, ProtocolSection{"rule_validate_checklist.md", 3, "Check " + key})
		}
		return sections, nil
	case gate == "validate":
		sections := []ProtocolSection{
			{"unit_validate_checklist.md", 2, "Target-layer applicability"},
			{"unit_validate_checklist.md", 2, "Prerequisite"},
			{"unit_validate_checklist.md", 2, "Execution Rules"},
			{"unit_validate_checklist.md", 3, "Severity handling"},
			{"spec_writing_guide.md", 3, "15.2 Externally observable only"},
		}
		for _, key := range keys {
			sections = append(sections, ProtocolSection{"unit_validate_checklist.md", 2, "Check " + key})
		}
		return sections, nil
	case gate == "verify":
		switch kind {
		case ProtocolSessionItem:
			return append(verifyPrerequisiteSections(),
				verifyStepSection("Step 2"), verifyStepSection("Step 3"), verifyStepSection("Step 4"),
				verifyStepSection("Step 6"), verifyStepSection("Step 7")), nil
		case ProtocolSessionUnit:
			return append(verifyPrerequisiteSections(),
				verifyStepSection("Step 1"), verifyStepSection("Step 5"), verifyStepSection("Step 7")), nil
		case ProtocolSessionCode, ProtocolSessionDesign, ProtocolSessionArchitecture:
			return []ProtocolSection{
				{"unit_verify_checklist.md", 2, "Quality Lens Standard"},
			}, nil
		}
	}
	return nil, fmt.Errorf("no protocol sections mapped for gate %q target %q session kind %q", gate, targetKind, kind)
}

// FinalSynthesisSections lists the framework sections the final synthesis
// judges by: the synthesis rules plus the severity standard it disposes
// findings under.
func FinalSynthesisSections() []ProtocolSection {
	return []ProtocolSection{
		{"verification_scope.md", 3, "Final synthesis"},
		{"verification_scope.md", 3, "Relationship vocabulary"},
		{"severity_policy.md", 2, "4. Severity Levels"},
		{"severity_policy.md", 2, "5. Blocking Relationship"},
		{"severity_policy.md", 2, "9. Severity Consistency Check"},
	}
}

func verifyPrerequisiteSections() []ProtocolSection {
	return []ProtocolSection{
		{"unit_verify_checklist.md", 2, "Target-layer applicability"},
		{"unit_verify_checklist.md", 2, "Prerequisite"},
	}
}

func verifyStepSection(step string) ProtocolSection {
	return ProtocolSection{"unit_verify_checklist.md", 2, step}
}

// InlineProtocol renders the requested framework sections as mission chunks,
// in mapping order. It reads the deployed framework documents — the same
// files the former protocol pointer named — so the inlined text cannot drift
// from the judging standard the run is bound to, and it reuses the framework
// document section parser the protocol fingerprint uses. A missing or
// ambiguous section is an error: the framework documents and this mapping
// must stay aligned, and a silent fallback to a pointer would hide the drift.
// Source names the deployed document path relative to the repository root,
// the same provenance the mission's protocol_ref carries.
func InlineProtocol(root, frameworkRoot string, sections []ProtocolSection) ([]ProtocolChunk, error) {
	documents := map[string]string{}
	var chunks []ProtocolChunk
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
		body, err := ExtractSection(content, ref.Level, ref.Heading)
		if err != nil {
			return nil, fmt.Errorf("framework/%s: %w", ref.File, err)
		}
		source := filepath.ToSlash(filepath.Join(filepath.FromSlash(frameworkRoot), filepath.FromSlash(ref.File)))
		chunks = append(chunks, ProtocolChunk{Source: source, Heading: sectionTitle(body, ref), Content: StripAtomMarkers(body)})
	}
	return chunks, nil
}

// ProtocolSectionFingerprint hashes the exact text the given protocol sections
// contribute to a standard. Gate-finalize uses it for the unit validate final
// synthesis; a coverage group adds its check sections to the shared baseline
// before fingerprinting.
func ProtocolSectionFingerprint(root string, sections []ProtocolSection) (string, error) {
	layout, err := specflowlayout.Resolve(root)
	if err != nil {
		// A root with no SpecFlow layout is not a deployed project: there is
		// no judging text to bind, and the version-only digest keeps
		// non-project tooling deterministic. Once a layout resolves, a
		// missing section is a broken install and fails closed.
		return Digest([]byte(ProtocolSectionVersion)), nil
	}
	chunks, err := InlineProtocol(root, layout.FrameworkRoot, sections)
	if err != nil {
		return "", err
	}
	return FingerprintChunks(chunks), nil
}

// FingerprintChunks hashes a rendered protocol text in mapping order. A group
// standard and the final-synthesis standard both reduce to a chunk sequence,
// so the mission text and the recorded binding use one shape.
func FingerprintChunks(chunks []ProtocolChunk) string {
	content := []byte(ProtocolSectionVersion)
	for _, chunk := range chunks {
		content = append(content, []byte("\n"+chunk.Source+"\n"+chunk.Heading+"\n")...)
		content = append(content, []byte(chunk.Content)...)
	}
	return Digest(content)
}

// UnitValidateGroupFingerprint hashes one unit validate coverage group's
// judging standard: the shared judging baseline plus the group's own check
// sections. The shared baseline is every unit validate protocol document's
// judging text minus the mechanism sections, the per-check sections (each
// group owns exactly one set), and the final-synthesis-only sections (bound by
// the cross standard). A shared-document edit therefore invalidates every
// group that applies it, while a check-section edit invalidates only its own
// group.
func UnitValidateGroupFingerprint(root string, checks []string) (string, error) {
	layout, err := specflowlayout.Resolve(root)
	if err != nil {
		return Digest([]byte(ProtocolSectionVersion)), nil
	}
	shared, err := unitValidateSharedChunks(root, layout.FrameworkRoot)
	if err != nil {
		return "", err
	}
	checkChunks, err := unitValidateCheckChunks(root, layout.FrameworkRoot, checks)
	if err != nil {
		return "", err
	}
	return FingerprintChunks(append(shared, checkChunks...)), nil
}

// unitValidateSharedChunks materializes the shared judging baseline. One chunk
// per document keeps the source provenance a reviewer needs while the digest
// stays independent of section order inside a document.
func unitValidateSharedChunks(root, frameworkRoot string) ([]ProtocolChunk, error) {
	var chunks []ProtocolChunk
	for _, name := range ProtocolDocuments("validate", "unit") {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(frameworkRoot), filepath.FromSlash(name)))
		if err != nil {
			return nil, fmt.Errorf("read protocol document %s: %w", name, err)
		}
		criteria, err := withoutMechanismSections(string(raw), mechanismSections[name])
		if err != nil {
			return nil, fmt.Errorf("framework/%s: %w", name, err)
		}
		if excluded := unitValidateSharedExclusions(name, criteria); len(excluded) > 0 {
			criteria, err = withoutSections(criteria, excluded)
			if err != nil {
				return nil, fmt.Errorf("framework/%s: %w", name, err)
			}
		}
		source := filepath.ToSlash(filepath.Join(filepath.FromSlash(frameworkRoot), filepath.FromSlash(name)))
		chunks = append(chunks, ProtocolChunk{Source: source, Heading: name, Content: StripAtomMarkers(criteria)})
	}
	return chunks, nil
}

// unitValidateSharedExclusions lists the sections that are not part of the
// shared baseline for one protocol document: the per-check sections (each
// group binds its own) and the synthesis-only sections no group applies (the
// cross standard binds those).
func unitValidateSharedExclusions(document, text string) []ProtocolSection {
	var refs []ProtocolSection
	switch document {
	case "unit_validate_checklist.md":
		refs = append(refs, ProtocolSection{
			File: document, Level: 2,
			Heading: "Extraction re-verification (Check 2 Step 1, Check 5)",
		})
		for _, section := range documentSections(strings.Split(text, "\n")) {
			if section.level != 2 {
				continue
			}
			slug := sectionSlug(section.text)
			if !strings.HasPrefix(slug, "Check ") {
				continue
			}
			if _, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(slug, "Check "))); err != nil {
				continue
			}
			refs = append(refs, ProtocolSection{File: document, Level: 2, Heading: slug})
		}
	case "verification_scope.md":
		refs = append(refs,
			ProtocolSection{File: document, Level: 3, Heading: "Final synthesis"},
			ProtocolSection{File: document, Level: 3, Heading: "Relationship vocabulary"},
		)
	}
	return refs
}

// unitValidateCheckChunks extracts the check sections a group binds. The
// caller passes the group's contract check ids.
func unitValidateCheckChunks(root, frameworkRoot string, checks []string) ([]ProtocolChunk, error) {
	sections := make([]ProtocolSection, 0, len(checks))
	for _, check := range checks {
		sections = append(sections, ProtocolSection{
			File: "unit_validate_checklist.md", Level: 2, Heading: "Check " + check,
		})
	}
	return InlineProtocol(root, frameworkRoot, sections)
}

// ProtocolSectionVersion names the fingerprint format. Bump it when the
// fingerprint's input shape changes: every cache written under the old shape
// then fails closed and re-runs instead of being silently reused.
const ProtocolSectionVersion = "protocol-section-v2"

// sectionTitle returns the heading title of the extracted body's first line.
func sectionTitle(body string, ref ProtocolSection) string {
	line := body
	if idx := strings.Index(body, "\n"); idx >= 0 {
		line = body[:idx]
	}
	if _, title, ok := HeadingTitle(line); ok {
		return title
	}
	return ref.Heading
}

// StripAtomMarkers removes atom delivery marker lines
// (==ATOM_BEGIN:id== / ==ATOM_END:id==). Markers are repository machinery,
// not protocol content, and never belong in a rendered mission.
func StripAtomMarkers(body string) string {
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

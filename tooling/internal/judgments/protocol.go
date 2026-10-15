package judgments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
)

// standardKey identifies one gate's judging standard: the gate and the target
// kind whose reviewer reads that standard.
type standardKey struct{ gate, targetKind string }

// protocolStandards maps each gate to the framework documents whose judging
// text its reviewer reads as the standard of judgment. A recorded conclusion is
// reusable only while the standard it was judged under is unchanged, so every
// gate that records a conclusion binds it to the digest of its own standard:
// changing the validate checklist must not invalidate verify conclusions, and
// the two validate targets must not invalidate each other.
//
// spec_writing_guide.md belongs to every set because every gate that judges a
// spec judges it by that guide: unit validate Check 10 applies its §9 Reader
// Contract, Check 2 its §9 decision closure, Check 5 its §4 external-visibility
// boundary and §Acceptance Item Granularity, Check 7 its §14 Truth Ownership;
// rule validate Checks 2-3 apply its §6 and §10; verify Step 2 and Step 7 apply
// its §4 and §14. Omitting it would let a guide edit reuse a conclusion the
// current standard no longer supports (framework/spec_flow_review.md §2.17).
//
// framework/validation_cache.md is deliberately absent from every standard:
// no mission assembles a prompt from it. That omission is the only one, and it
// is justified here rather than assumed.
var protocolStandards = map[standardKey][]string{
	{gate: "verify", targetKind: "unit"}: {
		"unit_verify_checklist.md",
		"shared_judgments.md",
		"verification_scope.md",
		"severity_policy.md",
		"spec_writing_guide.md",
	},
	{gate: "validate", targetKind: "unit"}: {
		"unit_validate_checklist.md",
		"verification_scope.md",
		"severity_policy.md",
		"spec_writing_guide.md",
	},
	{gate: "validate", targetKind: "rule"}: {
		"rule_validate_checklist.md",
		"verification_scope.md",
		"severity_policy.md",
		"spec_writing_guide.md",
	},
}

// mechanismSections lists, per protocol document, the sections that describe
// how a run is planned, executed, recorded, parsed or terminated rather than
// what counts as a pass, a problem, or how severe it is. A conclusion may be
// reused while those sections change: they do not carry the standard it was
// made under, so changing them must not invalidate it.
//
// A named section covers its heading and every line up to the next heading of
// the same or a higher level, so a named section must not contain judging text.
// A section that mixes both stays in the fingerprint: excluding it would drop
// judging text, and the tolerable direction is keeping too much (a re-run),
// never reusing a conclusion the current standard would not support. Everything
// not named here stays part of the standard. Naming a section that does not
// resolve is an error: a renamed heading must fail loudly rather than silently
// widen the fingerprint.
var mechanismSections = map[string][]string{
	"unit_verify_checklist.md": {
		"Run Orientation and Session Plumbing",
		"Coverage Keys and Session Partitioning",
		"The quality lens",
		"Session composition and batch inputs",
		"Session inputs and record reuse",
		"Promote gate and cache requirement",
		"7. Finding Output Format",
		"Mode Selection",
		"Target Selection",
		"Subagent Launch and Permissions",
		"Output Format",
		"Report skeleton and field definitions",
		"Completion — Persist Gate Cache",
		"Lens Body Skeletons",
		"Per-Item Report Format and Test Discovery",
		"Annotation Output Format",
		"Gate-Extend and Input Manifest",
		"Plan-Time Scan and Report Format",
		"Batch classification (main agent)",
		"Full and Delta Coverage Sets",
		"Mission Submission and Report Contract",
		"Final Synthesis Mission and Tool Contract",
		"Summary format",
		"Fix execution rules",
		"Re-check after fixes",
		"Stable-only mode",
		"Step 8 — Write verify cache (tooling finalize)",
		"Delta re-run (reverify@{unit})",
	},
	"unit_validate_checklist.md": {
		"Overview",
		"Mode Selection",
		"Stable-only mode",
		"Output Format",
		"Report skeleton and field definitions",
		"Completion — Persist Gate Cache",
		"Scope and lifecycle",
		"Step 9 — Write validate cache (tooling finalize)",
		"Delta re-run (revalidate@{unit})",
		"Batch classification (validate)",
		"Summary format",
		"Validate-specific notes",
	},
	"rule_validate_checklist.md": {
		"Overview",
		"Mode Selection",
		"Output Format",
		"Report skeleton and field definitions",
		"Completion — Persist Gate Cache",
		"Write cache (tooling finalize)",
		"Stable-only mode",
		"Delta re-run (revalidate@{rule})",
	},
	"shared_judgments.md": {
		"Audience",
		"Evidence-file roles and association display",
		"Task batching and record inputs",
		"Immutable storage and task allocation",
		"Cache, delta and release",
		"Previous protocol",
	},
	"verification_scope.md": {
		"Problem",
		"Solution",
		"Command Mode Principles",
		"Syntax",
		"Validate",
		"Verify",
		"Rule (validate only, verify removed)",
		"Delta re-runs (re* instruction family)",
		"Freshness check (read-only)",
		"Dependency Analysis (read-only)",
		"Keyword Resolution",
		"Parsing order",
		"Target domains",
		"Public Reuse and Allocation",
		"Coverage Run Mechanics",
		"Session record",
		"Coverage keys",
		"Input Surface Mechanics",
		"Report Field and Parse Contract",
		"Final Synthesis Mechanics",
		"Deferral Routing Mechanics",
		"Execution topology",
		"Guarantee Boundary",
		"Check / session scope",
		"Plan-Selected Names and Disposal",
		"Verify Synthesis Report Keys",
		"Validate Synthesis Report Keys",
		"Change detection",
		"Review Session and Outcome Vocabulary",
		"Mechanical floor",
		"Execution",
		"Failure recovery",
		"Trust statement",
		"Layer applicability",
		"Stable-only Targets",
		"Targeted Runs",
		"Output Format",
		"Full result (verify)",
		"Targeted result (verify)",
		"Validate full",
		"Full result with a finding (validate)",
		"Validate targeted",
		"Delta result (validate)",
		"Check Communication to Users",
		"Present Findings",
		"Cache Interaction",
		"Write rules",
		"Cache semantics",
		"Relationship with Promote",
		"State Transition Disclosure",
	},
	"severity_policy.md": {
		"1. Purpose",
		"2. Scope",
		"Finding Block Delivery",
		"7. Relationship To Other Files",
		"8. Non-Goals",
		"10. Code Review Severity Extension (Quality Lens)",
		"Current Execution Positions",
		"Execution position and ordering",
		"Session composition and batch inputs",
		"Session inputs and record reuse",
		"Promote gate and cache requirement",
		"7. Finding Output Format",
	},
	// spec_writing_guide.md names no section here: it is the spec-format
	// contract every validate and verify check judges by, and its sections mix
	// judging rules with mechanism references (for example §13 Section Regions
	// defines the structural checks Check 1/8 apply and also explains cache
	// freshness). A mixed section stays in the standard, so nothing is excluded
	// and the whole document is fingerprint input. The empty list is declared so
	// the document-set check proves that decision was made, not overlooked.
	"spec_writing_guide.md": {},
}

// Protocol fingerprints the judging standard one gate deploys in this project:
// the judging text of that gate's protocol documents, with the mechanism
// sections removed. It errors on an unknown gate/target pair instead of
// guessing a standard.
func Protocol(root, gate, targetKind string) (string, error) {
	documents, known := protocolStandards[standardKey{gate: gate, targetKind: targetKind}]
	if !known {
		return "", fmt.Errorf("no judging standard is defined for gate %q on a %s target", gate, targetKind)
	}
	layout, err := specflowlayout.Resolve(root)
	if err != nil {
		return Digest([]byte(ProtocolVersion)), nil
	}
	content := []byte(ProtocolVersion)
	for _, name := range documents {
		content = append(content, []byte("\n"+name+"\n")...)
		path := filepath.Join(root, filepath.FromSlash(layout.FrameworkRoot), name)
		data, err := os.ReadFile(path)
		if err != nil {
			content = append(content, []byte("missing")...)
			continue
		}
		criteria, err := withoutMechanismSections(string(data), mechanismSections[name])
		if err != nil {
			return "", fmt.Errorf("framework/%s: %w", name, err)
		}
		content = append(content, criteria...)
	}
	return Digest(content), nil
}

// MustProtocol is Protocol for callers that cannot propagate an error. It
// panics when the gate has no standard or a protocol document does not carry
// its declared mechanism sections.
func MustProtocol(root, gate, targetKind string) string {
	protocol, err := Protocol(root, gate, targetKind)
	if err != nil {
		panic(err)
	}
	return protocol
}

type documentSection struct {
	line  int
	end   int
	level int
	text  string
}

func withoutMechanismSections(text string, names []string) (string, error) {
	lines := strings.Split(text, "\n")
	sections := documentSections(lines)
	skip := make([]bool, len(lines))
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			return "", fmt.Errorf("mechanism section %q is listed twice", name)
		}
		seen[name] = true
		match := -1
		for i, s := range sections {
			if s.text != name {
				continue
			}
			if match >= 0 {
				return "", fmt.Errorf("mechanism section %q matches more than one heading", name)
			}
			match = i
		}
		if match < 0 {
			return "", fmt.Errorf("mechanism section %q not found", name)
		}
		for i := sections[match].line; i < sections[match].end; i++ {
			skip[i] = true
		}
	}
	var kept []string
	for i, line := range lines {
		if !skip[i] {
			kept = append(kept, line)
		}
	}
	criteria := strings.Join(kept, "\n")
	if strings.TrimSpace(criteria) == "" {
		return "", fmt.Errorf("every line is inside a mechanism section")
	}
	return criteria, nil
}

// documentSections maps each heading to its subtree: the heading line and every
// line up to the next heading of the same or a higher level. Headings inside
// fenced code blocks are not headings.
func documentSections(lines []string) []documentSection {
	var sections []documentSection
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		level, text, ok := heading(line)
		if !ok {
			continue
		}
		sections = append(sections, documentSection{line: i, level: level, text: text})
	}
	for i := range sections {
		sections[i].end = len(lines)
		for j := i + 1; j < len(sections); j++ {
			if sections[j].level <= sections[i].level {
				sections[i].end = sections[j].line
				break
			}
		}
	}
	return sections
}

func heading(line string) (int, string, bool) {
	if line == "" || line[0] != '#' {
		return 0, "", false
	}
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level > 6 || level == len(line) || line[level] != ' ' {
		return 0, "", false
	}
	text := strings.TrimSpace(line[level:])
	if text == "" {
		return 0, "", false
	}
	return level, text, true
}

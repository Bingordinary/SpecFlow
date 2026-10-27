package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

type qualityReportBlock struct {
	Key  string
	Text string
}

// qualityReportBlocks binds each complete assessment to an assigned file.
// File boundaries are mandatory for both single-file and batched reports.
func qualityReportBlocks(spec *gaterun.SessionSpec, report string) ([]qualityReportBlock, error) {
	var blocks []qualityReportBlock
	seen := map[string]bool{}
	for _, line := range strings.Split(report, "\n") {
		prefix := "File:"
		if spec.Kind == gaterun.SessionKindArchitecture {
			prefix = "Unit:"
		}
		if key, ok := strings.CutPrefix(line, prefix); ok {
			key = strings.TrimSpace(key)
			if !stringInList(spec.CheckKeys, key) {
				return nil, fmt.Errorf("quality report declares unassigned file %q", key)
			}
			if seen[key] {
				return nil, fmt.Errorf("quality report declares File: %s more than once", key)
			}
			seen[key] = true
			blocks = append(blocks, qualityReportBlock{Key: key})
			continue
		}
		if len(blocks) == 0 {
			if strings.TrimSpace(line) != "" {
				return nil, fmt.Errorf("quality report content must belong to a File: <assigned path> block")
			}
			continue
		}
		blocks[len(blocks)-1].Text += line + "\n"
	}
	for _, key := range spec.CheckKeys {
		if !seen[key] {
			return nil, fmt.Errorf("quality report is missing File: %s", key)
		}
	}
	return blocks, nil
}

// findingLocationLineRe captures a finding's `{location}` prefix with its
// optional `:{line}` or `:{line}-{line}` suffix removed: `src/a.go:12` →
// `src/a.go`. A location that carries no line suffix is returned unchanged.
var findingLocationLineRe = regexp.MustCompile(`^(.*?):\d+(?:-\d+)?$`)

// specBasisLineRe captures one design `Spec basis:` declaration: the anchor —
// a promise entry id, or a heading text (the heading line without its leading
// `#` markers) — that the judgment relied on. One line per anchor, so a heading
// text may contain any character except a newline.
var specBasisLineRe = regexp.MustCompile(`(?m)^Spec basis: (\S[^\n]*)$`)

// findingLocationFile extracts the file a finding's own `{location}` names —
// the text before the subject separator, with its `:{line}` suffix removed. A
// location that names no file (a bare subject such as `token shape`, or a
// rule/relationship/review subject) returns "".
func findingLocationFile(findingText string) string {
	loc := findingText
	for _, sep := range []string{" — ", " - "} {
		if before, _, found := strings.Cut(loc, sep); found {
			loc = before
			break
		}
	}
	loc = strings.Trim(strings.TrimSpace(loc), "`\"'")
	if loc == "" {
		return ""
	}
	if m := findingLocationLineRe.FindStringSubmatch(loc); m != nil {
		loc = m[1]
	}
	return strings.TrimSpace(loc)
}

// qualityFileKeysByFile indexes the assigned keys a quality session judges by
// file (design and code keys) by canonical file path. Architecture keys judge
// a unit and carry no file, so they are absent.
func qualityFileKeysByFile(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec) map[string]map[string]bool {
	byFile := map[string]map[string]bool{}
	for _, key := range spec.CheckKeys {
		ck := run.CoverageByKey(key)
		if ck == nil || ck.File == "" {
			continue
		}
		file := gaterun.CanonicalDeclPath(absRoot, ck.File)
		if byFile[file] == nil {
			byFile[file] = map[string]bool{}
		}
		byFile[file][key] = true
	}
	return byFile
}

// checkFindingPlacement rejects a finding whose own `location` names a file
// this session also judges but that is written in a different file's block.
// The key binding is positional — the File: block a finding appears in — and
// the location is the finding's own statement of its subject, so the two must
// agree whenever both name files of this session. Without this check a report
// that lists finding blocks after the last file block silently binds them to
// the last file's key (issue #102).
func checkFindingPlacement(absRoot, blockKey string, extracted extractedFinding, fileKeys map[string]map[string]bool) error {
	file := findingLocationFile(extracted.text)
	if file == "" {
		return nil
	}
	keys := fileKeys[gaterun.CanonicalDeclPath(absRoot, file)]
	if len(keys) == 0 || keys[blockKey] {
		return nil
	}
	owner := ""
	for key := range keys {
		owner = key
		break
	}
	return fmt.Errorf("finding [%s] %s is written in the block of %s but its location names %s, which this session judges as %s; write the finding inside that file's block (a finding binds to the File: block it appears in)", extracted.severity, extracted.text, blockKey, file, owner)
}

func parseQualitySessionReport(absRoot string, run *gaterun.Run, spec *gaterun.SessionSpec, report string) (*parsedReport, error) {
	blocks, err := qualityReportBlocks(spec, report)
	if err != nil {
		return nil, err
	}
	// A finding binds to the File: block it is written in (see
	// checkFindingPlacement): index this session's file-scoped keys so a
	// finding whose own location names a different judged file is rejected
	// instead of silently recorded under the wrong key.
	fileKeys := qualityFileKeysByFile(absRoot, run, spec)
	out := &parsedReport{Verdicts: map[string]string{}, FileGateFindings: map[string]string{}}
	findingNo := 0
	for _, block := range blocks {
		// Each block is judged by its own coverage kind, so a co-batched
		// session can carry code blocks (public facts) and design blocks
		// (unit judgments) in one report.
		ck := run.CoverageByKey(block.Key)
		if ck == nil {
			return nil, fmt.Errorf("check %q is not part of the run's coverage set", block.Key)
		}
		fileSpec := *spec
		fileSpec.Kind = ck.Kind
		fileSpec.CheckKeys = []string{block.Key}
		token, line, _, err := extractVerdict(&fileSpec, block.Key, block.Text)
		if err != nil {
			return nil, err
		}
		if needsReason(&fileSpec, token) && !hasReason(line, token) {
			return nil, fmt.Errorf("check %q verdict %s carries no reason", block.Key, token)
		}
		if err := validateReviewBody(ck.Kind, block.Text); err != nil {
			return nil, fmt.Errorf("file %q: %w", block.Key, err)
		}
		gateFindings := "none"
		if ck.Kind != gaterun.SessionKindCode {
			gateFindings, err = extractGateFindings(block.Text)
		}
		if err != nil {
			return nil, fmt.Errorf("file %q: %w", block.Key, err)
		}
		// A design judgment declares the spec anchors its conclusion relied
		// on — narrative-block headings and promise entry ids — and the
		// record's spec pins are derived from them. A block without the
		// declaration is rejected here rather than under-pinned silently; the
		// anchors themselves are resolved against the unit spec when the
		// record is saved (gaterun.SaveJudgment), where an unresolvable anchor
		// is rejected too.
		if ck.Kind == gaterun.SessionKindDesign {
			matches := specBasisLineRe.FindAllStringSubmatch(block.Text, -1)
			if len(matches) == 0 {
				return nil, fmt.Errorf("file %q: a design block must declare the spec anchors its judgment relied on — add at least one `Spec basis: <promise id or heading text>` line, where heading text is the heading line without its leading '#' markers", block.Key)
			}
			if out.SpecBasis == nil {
				out.SpecBasis = map[string][]string{}
			}
			for _, m := range matches {
				out.SpecBasis[block.Key] = append(out.SpecBasis[block.Key], strings.TrimSpace(m[1]))
			}
			// Resolve the anchors here so an anchor that names nothing is
			// rejected where the report is submitted; SaveJudgment resolves
			// the same anchors again when it derives the record's pins.
			if err := gaterun.ValidateSpecBasis(absRoot, run, out.SpecBasis[block.Key]); err != nil {
				return nil, fmt.Errorf("file %q: %w", block.Key, err)
			}
		}
		out.Verdicts[block.Key] = token
		out.FileGateFindings[block.Key] = gateFindings
		for _, extracted := range extractFindings(block.Text) {
			if err := checkFindingPlacement(absRoot, block.Key, extracted, fileKeys); err != nil {
				return nil, err
			}
			if !resolutionLabelRe.MatchString(extracted.text) {
				return nil, fmt.Errorf("finding [%s] %s carries no resolution label (`(actionable)` or `(needs_decision)`)", extracted.severity, extracted.text)
			}
			if extracted.severity == "P3" && !factAnchorRe.MatchString(extracted.detail) {
				return nil, fmt.Errorf("quality P3 finding [%s] %s carries no non-empty fact_anchor detail line", extracted.severity, extracted.text)
			}
			findingNo++
			finding := gaterun.Finding{
				ID:       fmt.Sprintf("%s/%s/F%d", run.RunID, spec.SessionID, findingNo),
				Severity: extracted.severity, Text: extracted.text, Detail: extracted.detail, SourceKey: block.Key,
			}
			// Code-block observations become the file's public record;
			// design and architecture findings drive the unit's gate.
			if ck.Kind == gaterun.SessionKindCode {
				out.Observations = append(out.Observations, finding)
			} else {
				out.Findings = append(out.Findings, finding)
			}
		}
	}
	if spec.Kind == gaterun.SessionKindDesign {
		for _, match := range regexp.MustCompile(`(?m)^Observation disposition: (\S+) = (retained|suppressed) [—-] (\S[^\n]*)$`).FindAllStringSubmatch(report, -1) {
			out.ObservationDispositions = append(out.ObservationDispositions, gaterun.FindingDisposition{FindingID: match[1], Action: match[2], Reason: match[3]})
		}
	}
	// Record the shared report-order counter so the design retain path can
	// continue it (code observations and design findings share one numbering).
	out.FindingNumber = findingNo

	return out, nil
}

func validateReviewBody(kind, report string) error {
	if kind == gaterun.SessionKindArchitecture {
		return validateQualityFileBody(report)
	}
	field := "spec_requirements"
	if kind == gaterun.SessionKindCode {
		field = "facts"
		if strings.Contains(report, "Suppressed by spec") || strings.Contains(report, "Observation disposition:") {
			return fmt.Errorf("public observations cannot be suppressed by unit design")
		}
	}
	if len(regexp.MustCompile(`(?m)^[ \t]*`+field+`:\s*\S[^\n]*$`).FindAllString(report, -1)) != 1 {
		return fmt.Errorf("review requires exactly one non-empty %s assessment", field)
	}
	return nil
}

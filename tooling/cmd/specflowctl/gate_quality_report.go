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

func parseQualitySessionReport(run *gaterun.Run, spec *gaterun.SessionSpec, report string) (*parsedReport, error) {
	blocks, err := qualityReportBlocks(spec, report)
	if err != nil {
		return nil, err
	}
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
		token, line, index, err := extractVerdict(&fileSpec, block.Key, block.Text)
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
		out.Verdicts[block.Key] = token
		out.FileGateFindings[block.Key] = gateFindings
		scopes, err := extractScopes(run, &fileSpec, block.Text, map[int]bool{index: true})
		if err != nil {
			return nil, err
		}
		if len(scopes) == 0 {
			return nil, fmt.Errorf("check %q declares no Dependency scope line in its file block", block.Key)
		}
		out.Scopes = append(out.Scopes, scopes...)
		for _, extracted := range extractFindings(block.Text) {
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

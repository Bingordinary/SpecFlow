package specvalidation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// CheckAcceptanceItemSchema checks the required shape of every real item.
// Candidate validation and publication share this check. Path resolution and
// semantic quality remain the responsibility of their respective checks.
func CheckAcceptanceItemSchema(content string) CheckResult {
	content = specpaths.NormalizeText(content)
	result := CheckResult{Name: "Acceptance items", Status: Fail}
	if _, ok := contenthash.AcceptanceItemsRegion(content); !ok {
		result.Details = "acceptance_item_set not found"
		return result
	}
	var issues []string
	if line, empty := contenthash.EmptyAcceptanceItemIDLine(content); empty {
		issues = append(issues, fmt.Sprintf("acceptance item at line %d: missing or empty id", line))
	}
	items := parseAcceptanceItems(content)
	if len(items) == 0 {
		issues = append(issues, "acceptance_item_set exists but no items found with - id:")
	}
	required := []string{"description", "verification_type", "verification_surface", "implementation_surface", "verification_method", "pass_condition", "runnable"}
	seen := map[string]bool{}
	for _, item := range items {
		issue := func(message string) {
			issues = append(issues, fmt.Sprintf("acceptance item %q: %s", item.id, message))
		}
		if seen[item.id] {
			issue("duplicate id")
		}
		seen[item.id] = true
		for _, key := range item.duplicateFields {
			issue("duplicate field " + key)
		}
		for _, key := range required {
			if strings.TrimSpace(item.fields[key]) == "" {
				issue("missing or empty required field " + key)
			}
		}
		if value := item.fields["verification_type"]; value != "" && value != "testable" && value != "inspectable" && value != "reviewable" {
			issue(fmt.Sprintf("invalid verification_type %q; must be testable, inspectable or reviewable", value))
		}
		if value := item.fields["runnable"]; value != "" && value != "yes" && value != "no" {
			issue(fmt.Sprintf("invalid runnable %q; must be yes or no", value))
		}
		if item.fields["runnable"] == "no" && strings.TrimSpace(item.fields["not_runnable_reason"]) == "" {
			issue("missing or empty required field not_runnable_reason when runnable is no")
		}
		// Gherkin-style description convention (framework/spec_writing_guide.md
		// §Gherkin-style Description Convention): this is the mechanical half
		// of the retired Check 5d — testable items must carry a
		// Given…When…Then scenario sequence, and .feature file syntax is
		// rejected.
		if item.fields["verification_type"] == "testable" {
			description := item.fields["description"]
			if keyword := featureSyntaxLine(description); keyword != "" {
				issue(fmt.Sprintf("description uses .feature syntax (%s); use Gherkin-style Given/When/Then scenarios (framework/spec_writing_guide.md §Gherkin-style Description Convention)", keyword))
			} else if !gherkinSequenceRe.MatchString(description) {
				issue("testable item description must contain a Given…When…Then scenario sequence (framework/spec_writing_guide.md §Gherkin-style Description Convention)")
			}
		}
	}
	if len(issues) != 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	result.Status = Pass
	result.Details = fmt.Sprintf("%d item(s) found with required fields", len(items))
	return result
}

// featureSyntaxLine reports the first `.feature` keyword a description line
// starts with, or "" when the description uses no `.feature` syntax.
func featureSyntaxLine(description string) string {
	keywords := []string{"Feature:", "Scenario:", "Scenario Outline:", "Examples:", "Background:"}
	for _, line := range strings.Split(description, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, keyword := range keywords {
			if strings.HasPrefix(trimmed, keyword) {
				return keyword
			}
		}
	}
	return ""
}

// gherkinSequenceRe matches a Given…When…Then sequence in order anywhere in
// the description — the mechanical form of the Gherkin-style convention
// (line breaks are presentation; the sequence is the content).
var gherkinSequenceRe = regexp.MustCompile(`(?is)\bgiven\b.*\bwhen\b.*\bthen\b`)

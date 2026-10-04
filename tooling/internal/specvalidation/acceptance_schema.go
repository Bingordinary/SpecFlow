package specvalidation

import (
	"fmt"
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
	}
	if len(issues) != 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	result.Status = Pass
	result.Details = fmt.Sprintf("%d item(s) found with required fields", len(items))
	return result
}

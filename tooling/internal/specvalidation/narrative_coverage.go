package specvalidation

import (
	"fmt"
	"os"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
)

// CheckNarrativeCoverage fails a direct-content narrative block that is
// neither referenced by a promise/item source nor declared non-behavior.
func CheckNarrativeCoverage(content string) CheckResult {
	const name = "Narrative coverage"
	graph, err := validatekeys.Parse("", content)
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: "cannot derive the narrative graph: " + err.Error()}
	}
	start, end, ok := designNarrativeRange(graph.Sections)
	if !ok {
		// The promise-model shape check owns the missing-section failure.
		return CheckResult{Name: name, Status: Pass}
	}
	sourced := map[string]bool{}
	for _, promise := range graph.Promises {
		for _, source := range promise.Sources {
			sourced[source] = true
		}
	}
	for _, item := range graph.Items {
		for _, source := range item.Sources {
			sourced[source] = true
		}
	}
	byHeading := make(map[string]validatekeys.Section, len(graph.Sections))
	for _, section := range graph.Sections {
		byHeading[section.Heading] = section
	}
	// A source anchor names a heading region; the region includes its deeper
	// subsections. A child block is therefore covered when an ancestor is
	// sourced. non_behavior_blocks stays exact: a parent declaration must not
	// silently classify a child behavior as non-behavior.
	isSourced := func(section validatekeys.Section) bool {
		for current := section; ; {
			if sourced[current.Heading] {
				return true
			}
			if current.Parent == "" {
				return false
			}
			parent, ok := byHeading[current.Parent]
			if !ok {
				return false
			}
			current = parent
		}
	}
	var missing []string
	for _, section := range graph.Sections {
		if section.Level < 3 || section.Start < start || section.Start > end || !section.Direct {
			continue
		}
		if graph.NonBehavior[section.Heading] || isSourced(section) {
			continue
		}
		missing = append(missing, fmt.Sprintf("%q (line %d)", section.Heading, section.Start))
	}
	if len(missing) == 0 {
		return CheckResult{Name: name, Status: Pass, Details: "every direct-content narrative block is sourced or declared non-behavior"}
	}
	shown := missing
	if len(shown) > 12 {
		shown = shown[:12]
	}
	details := "direct-content narrative blocks are not referenced by any promise/item source and are not declared non_behavior_blocks: " + strings.Join(shown, ", ")
	if len(missing) > len(shown) {
		details += fmt.Sprintf(", and %d more", len(missing)-len(shown))
	}
	details += "; add a source reference or declare the heading in non_behavior_blocks"
	return CheckResult{Name: name, Status: Fail, Details: details}
}

func checkNarrativeCoverage(repoRoot, unitName string) CheckResult {
	data, err := os.ReadFile(specPath(repoRoot, unitName))
	if err != nil {
		return CheckResult{Name: "Narrative coverage", Status: Fail, Details: fmt.Sprintf("cannot read candidate spec: %v", err)}
	}
	return CheckNarrativeCoverage(string(data))
}

// designNarrativeRange returns the line range of the `## Design Narrative`
// section. ok is false when the section is absent.
func designNarrativeRange(sections []validatekeys.Section) (start, end int, ok bool) {
	for _, section := range sections {
		if section.Level == 2 && section.Heading == "Design Narrative" {
			return section.Start, section.End, true
		}
	}
	return 0, 0, false
}

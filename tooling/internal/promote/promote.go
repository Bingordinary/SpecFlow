// Package promote validates candidate specs and archives them to stable.
// The tooling validates only deterministic format constraints (frontmatter fields,
// acceptance_item_set presence, appendix file paths). Semantic validation
// (reference integrity, cross-unit consistency, acceptance completeness) is
// delegated to the validate subagent and is outside the promote tooling scope.
package promote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result describes the outcome of a promote operation.
type Result struct {
	Unit    string
	Passed  bool
	Issues  []string
	Actions []string
}

// Promote runs the promote flow for the given unit.
// Steps:
//  1. Check candidate spec exists
//  2. Validate frontmatter fields
//  3. Validate acceptance items
//  4. Check reference integrity (appendix files, rule files)
//  5. Check repository_mapping.md entry
//  6. Copy candidate files to stable
func Promote(repoRoot, unitName string) *Result {
	r := &Result{Unit: unitName}

	candidateSpec := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/c_unit_%s.md", unitName))
	stableSpec := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/s_unit_%s.md", unitName))

	// Step 1: Check candidate spec exists
	if _, err := os.Stat(candidateSpec); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate spec not found: docs/specs/units/candidate/c_unit_%s.md", unitName))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found candidate spec: docs/specs/units/candidate/c_unit_%s.md", unitName))

	// Step 2: Read and validate frontmatter
	data, err := os.ReadFile(candidateSpec)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Cannot read candidate spec: %v", err))
		r.Passed = false
		return r
	}
	content := string(data)

	fm := parseFrontmatter(content)
	checks := []struct {
		field string
		value string
	}{
		{"id", fm["id"]},
		{"layer", fm["layer"]},
		{"version", fm["version"]},
	}

	for _, c := range checks {
		if c.value == "" {
			r.Issues = append(r.Issues, fmt.Sprintf("Missing required field: %s", c.field))
		}
	}

	if v := fm["layer"]; v != "" && !strings.EqualFold(v, "candidate") {
		r.Issues = append(r.Issues, fmt.Sprintf("Layer must be 'candidate', got '%s'", v))
	}

	// Step 3: Check acceptance items exist
	if !strings.Contains(content, "acceptance_item_set:") && !strings.Contains(content, "acceptance_item_set") {
		r.Issues = append(r.Issues, "No acceptance items found (acceptance_item_set is required)")
	}

	// Step 4: Check appendix files
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	pattern := fmt.Sprintf("c_unit_%s_*.md", unitName)
	matches, _ := filepath.Glob(filepath.Join(appendixDir, pattern))
	for _, m := range matches {
		rel, _ := filepath.Rel(repoRoot, m)
		r.Actions = append(r.Actions, fmt.Sprintf("Found appendix: %s", rel))
	}

	// Step 5: Check repository_mapping.md
	mappingPath := filepath.Join(repoRoot, "docs/specs/repository_mapping.md")
	if _, err := os.Stat(mappingPath); os.IsNotExist(err) {
		r.Issues = append(r.Issues, "repository_mapping.md not found")
	} else {
		mappingData, _ := os.ReadFile(mappingPath)
		if !strings.Contains(string(mappingData), unitName) {
			r.Issues = append(r.Issues, fmt.Sprintf("Unit '%s' not found in repository_mapping.md", unitName))
		}
	}

	if len(r.Issues) > 0 {
		r.Passed = false
		return r
	}

	// Step 6: Copy candidate files to stable
	// Copy appendices first so that a failure leaves the main spec untouched.
	stableAppendixDir := filepath.Join(repoRoot, "docs/specs/units/stable/appendix")
	_ = os.MkdirAll(stableAppendixDir, 0755)

	for _, m := range matches {
		stableName := strings.Replace(filepath.Base(m), "c_unit_", "s_unit_", 1)
		dest := filepath.Join(stableAppendixDir, stableName)
		if err := copyWithLayerTransform(m, dest); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy appendix: %v", err))
			r.Passed = false
			return r
		}
		rel, _ := filepath.Rel(repoRoot, dest)
		r.Actions = append(r.Actions, fmt.Sprintf("Promoted appendix: %s", rel))
	}

	// Copy main spec last so it acts as the commit point.
	if err := copyWithLayerTransform(candidateSpec, stableSpec); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy spec: %v", err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Promoted: docs/specs/units/candidate/c_unit_%s.md -> docs/specs/units/stable/s_unit_%s.md", unitName, unitName))

	// Step 7: Remove candidate files so file existence remains an unambiguous state signal.
	// "Candidate file exists = being edited" — after promote, no editing is in progress.
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			r.Actions = append(r.Actions, fmt.Sprintf("Warning: could not remove candidate appendix: %s", filepath.Base(m)))
		} else {
			r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate appendix: docs/specs/units/candidate/appendix/%s", filepath.Base(m)))
		}
	}
	if err := os.Remove(candidateSpec); err != nil {
		r.Actions = append(r.Actions, fmt.Sprintf("Warning: could not remove candidate spec: c_unit_%s.md", unitName))
	} else {
		r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate spec: docs/specs/units/candidate/c_unit_%s.md", unitName))
	}

	r.Passed = true
	return r
}

// FormatResult formats the promote result as readable output.
func FormatResult(r *Result) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Unit: %s\n", r.Unit)

	if r.Passed {
		buf.WriteString("Result: PASSED\n\n")
	} else {
		buf.WriteString("Result: FAILED\n\n")
	}

	if len(r.Issues) > 0 {
		buf.WriteString("Issues:\n")
		for _, i := range r.Issues {
			fmt.Fprintf(&buf, "  - %s\n", i)
		}
		buf.WriteString("\n")
	}

	if len(r.Actions) > 0 {
		buf.WriteString("Actions:\n")
		for _, a := range r.Actions {
			fmt.Fprintf(&buf, "  - %s\n", a)
		}
		buf.WriteString("\n")
	}

	if r.Passed {
		buf.WriteString("Candidate spec has been promoted to stable.\n")
		buf.WriteString("Git handles version history.\n")
	} else {
		buf.WriteString("Promote failed. Fix the issues above and try again.\n")
	}

	return buf.String()
}

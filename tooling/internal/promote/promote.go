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

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulesync"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// VersionChangeType describes the kind of version change between candidate and stable.
type VersionChangeType int

const (
	ChangeNone  VersionChangeType = iota // No stable version exists yet
	ChangePatch                          // 0.0.x — wording clarification
	ChangeMinor                          // 0.x.0 — compatible extension
	ChangeMajor                          // x.0.0 — breaking constraint change
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
//  4. Find candidate appendix files
//  5. Copy candidate files to stable
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

	// Step 3b: Check unit_refs don't point to unpromoted candidate-only files
	if fm["unit_refs"] != "" && !strings.EqualFold(fm["unit_refs"], "none") {
		refs := specpaths.ParseRefList(fm["unit_refs"])
		for _, ref := range refs {
			ref = strings.TrimSpace(strings.Split(ref, "@")[0])
			if ref == "" || ref == unitName {
				continue
			}
			stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/s_unit_%s.md", ref))
			candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/c_unit_%s.md", ref))
			if _, err := os.Stat(stablePath); os.IsNotExist(err) {
				if _, err := os.Stat(candidatePath); err == nil {
					r.Issues = append(r.Issues, fmt.Sprintf("unit_refs target '%s' exists only in candidate layer — promote it first", ref))
				} else {
					r.Issues = append(r.Issues, fmt.Sprintf("unit_refs target '%s' does not exist in stable or candidate", ref))
				}
			}
		}
	}

	// Step 3c: Check rule_refs don't point to unpromoted candidate-only files
	if fm["rule_refs"] != "" && !strings.EqualFold(fm["rule_refs"], "none") {
		refs := specpaths.ParseRefList(fm["rule_refs"])
		for _, ref := range refs {
			ref = strings.TrimSpace(strings.Split(ref, "@")[0])
			if ref == "" {
				continue
			}
			stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/stable/%s.md", ref))
			candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ref))
			if _, err := os.Stat(stablePath); os.IsNotExist(err) {
				if _, err := os.Stat(candidatePath); err == nil {
					r.Issues = append(r.Issues, fmt.Sprintf("rule_refs target '%s' exists only in candidate layer — promote it first", ref))
				} else {
					r.Issues = append(r.Issues, fmt.Sprintf("rule_refs target '%s' does not exist in stable or candidate", ref))
				}
			}
		}
	}

	// Step 3d: Scan body for candidate-layer path references
	_, body, _ := specpaths.ParseFrontmatterFields(content)
	if body != "" && strings.Contains(body, "docs/specs/units/candidate/") {
		r.Actions = append(r.Actions, "WARNING: body contains candidate-layer path references (docs/specs/units/candidate/) — verify they are correct after promote")
	}

	// Step 4: Check appendix files
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	pattern := fmt.Sprintf("c_unit_%s_*.md", unitName)
	matches, _ := filepath.Glob(filepath.Join(appendixDir, pattern))
	for _, m := range matches {
		rel, _ := filepath.Rel(repoRoot, m)
		r.Actions = append(r.Actions, fmt.Sprintf("Found appendix: %s", rel))
	}

	if len(r.Issues) > 0 {
		r.Passed = false
		return r
	}

	// Step 5: Copy candidate files to stable
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

// parseVersion extracts major, minor, patch from a semver string.
func parseVersion(v string) (major, minor, patch int, ok bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	_, err := fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &patch)
	return major, minor, patch, err == nil
}

// versionChangeType determines the type of version change.
// If stableVersion is empty (brand new rule), returns ChangeNone.
func versionChangeType(candidateVersion, stableVersion string) VersionChangeType {
	if stableVersion == "" {
		return ChangeNone
	}
	cMaj, cMin, cPat, cOk := parseVersion(candidateVersion)
	sMaj, sMin, sPat, sOk := parseVersion(stableVersion)
	if !cOk || !sOk {
		return ChangeNone
	}
	if cMaj != sMaj {
		return ChangeMajor
	}
	if cMin != sMin {
		return ChangeMinor
	}
	if cPat != sPat {
		return ChangePatch
	}
	return ChangeNone
}

// RuleResult describes the outcome of a rule promote operation.
type RuleResult struct {
	RuleID           string
	Passed           bool
	Issues           []string
	Actions          []string
	CandidateUpdated []string
	ForkedUnits      []string
	ChangeType       VersionChangeType
}

// PromoteRule runs the promote flow for the given rule.
// Steps:
//  1. Check candidate rule file exists
//  2. Validate frontmatter fields (rule_id, rule_scope, layer, rule_version)
//  3. Detect current stable version
//  4. Version sanity check (candidate version > stable version)
//  5. Determine version change type (MAJOR/MINOR/PATCH/none)
//  6. Copy candidate to stable with layer transform
//  7. If MAJOR: fork all stable consumers to candidate, update all candidate consumer rule_refs
//  8. Delete candidate rule file
func PromoteRule(repoRoot, ruleID string) *RuleResult {
	r := &RuleResult{RuleID: ruleID}

	candidateRule := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ruleID))
	stableRule := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/stable/%s.md", ruleID))

	// Step 1: Check candidate rule exists
	if _, err := os.Stat(candidateRule); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate rule not found: docs/specs/rules/candidate/%s.md", ruleID))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found candidate rule: docs/specs/rules/candidate/%s.md", ruleID))

	// Step 2: Read and validate frontmatter
	data, err := os.ReadFile(candidateRule)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Cannot read candidate rule: %v", err))
		r.Passed = false
		return r
	}

	fm := parseFrontmatter(string(data))
	requiredFields := []struct {
		field string
		value string
	}{
		{"rule_id", fm["rule_id"]},
		{"rule_scope", fm["rule_scope"]},
		{"layer", fm["layer"]},
		{"rule_version", fm["rule_version"]},
	}

	for _, f := range requiredFields {
		if f.value == "" {
			r.Issues = append(r.Issues, fmt.Sprintf("Missing required field: %s", f.field))
		}
	}

	if v := fm["layer"]; v != "" && !strings.EqualFold(v, "candidate") {
		r.Issues = append(r.Issues, fmt.Sprintf("Layer must be 'candidate', got '%s'", v))
	}

	if len(r.Issues) > 0 {
		r.Passed = false
		return r
	}

	candidateVersion := fm["rule_version"]

	// Step 3: Detect current stable version
	stableVersion := ""
	if _, err := os.Stat(stableRule); err == nil {
		stableData, err := os.ReadFile(stableRule)
		if err == nil {
			stableFM := parseFrontmatter(string(stableData))
			stableVersion = stableFM["rule_version"]
		}
	}

	if stableVersion != "" {
		r.Actions = append(r.Actions, fmt.Sprintf("Current stable version: %s", stableVersion))
		r.Actions = append(r.Actions, fmt.Sprintf("Candidate version: %s", candidateVersion))
	}

	// Step 4: Version sanity check
	if stableVersion != "" && candidateVersion == stableVersion {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate version %s is same as stable version — bump the version", candidateVersion))
		r.Passed = false
		return r
	}
	if stableVersion != "" && !isVersionGreater(candidateVersion, stableVersion) {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate version %s must be greater than stable version %s", candidateVersion, stableVersion))
		r.Passed = false
		return r
	}

	// Step 5: Determine version change type
	r.ChangeType = versionChangeType(candidateVersion, stableVersion)
	switch r.ChangeType {
	case ChangeMajor:
		r.Actions = append(r.Actions, "MAJOR change detected — breaking constraint change, will cascade to consumers")
	case ChangeMinor:
		r.Actions = append(r.Actions, "MINOR change detected — compatible extension, no consumer cascade needed")
	case ChangePatch:
		r.Actions = append(r.Actions, "PATCH change detected — wording clarification, no consumer cascade needed")
	case ChangeNone:
		r.Actions = append(r.Actions, "New rule promoted (no previous stable version)")
	}

	// Step 6: Copy candidate to stable with layer transform
	if err := copyWithLayerTransform(candidateRule, stableRule); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy rule: %v", err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Promoted: docs/specs/rules/candidate/%s.md -> docs/specs/rules/stable/%s.md", ruleID, ruleID))

	// Step 7: Handle cascade for MAJOR changes
	if r.ChangeType == ChangeMajor && stableVersion != "" {
		// 7a: Discover consumers
		consumerResult, err := rulesync.Consumers(repoRoot, rulesync.ConsumerOptions{RuleID: ruleID})
		if err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to discover consumers: %v", err))
			r.Passed = false
			return r
		}

		// 7b: Fork stable consumers back to candidate
		for _, consumer := range consumerResult.Consumers {
			if consumer.ActiveLayer == "stable" {
				forked, forkErr := forkUnitFromStable(repoRoot, consumer.Object)
				if forkErr != nil {
					r.Actions = append(r.Actions, fmt.Sprintf("Warning: could not fork consumer %s: %v", consumer.Object, forkErr))
					continue
				}
				r.ForkedUnits = append(r.ForkedUnits, consumer.Object)
				for _, action := range forked {
					r.Actions = append(r.Actions, action)
				}
			}
		}

		// 7c: Update all candidate consumer rule_refs
		fromRef := fmt.Sprintf("%s@%s", ruleID, stableVersion)
		toRef := fmt.Sprintf("%s@%s", ruleID, candidateVersion)

		releaseResult, err := rulesync.ReleaseVersion(repoRoot, rulesync.ReleaseVersionOptions{
			RuleID:  ruleID,
			FromRef: fromRef,
			ToRef:   toRef,
		})
		if err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Release version failed: %v", err))
			r.Passed = false
			return r
		}
		r.CandidateUpdated = releaseResult.CandidateUpdated
		for _, updated := range releaseResult.CandidateUpdated {
			r.Actions = append(r.Actions, fmt.Sprintf("Updated consumer: %s", updated))
		}
	}

	// Step 8: Delete candidate rule file
	if err := os.Remove(candidateRule); err != nil {
		r.Actions = append(r.Actions, fmt.Sprintf("Warning: could not remove candidate rule: %s.md", ruleID))
	} else {
		r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate rule: docs/specs/rules/candidate/%s.md", ruleID))
	}

	r.Passed = true
	return r
}

// forkUnitFromStable copies a stable unit spec (and its appendices) back to the
// candidate layer, transforming layer from stable to candidate. This is the
// reverse of promote — it creates a candidate from a stable spec.
// It returns a list of action descriptions. If the candidate already exists,
// it is not overwritten.
func forkUnitFromStable(repoRoot, unitName string) ([]string, error) {
	var actions []string

	stableSpec := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/s_unit_%s.md", unitName))
	candidateSpec := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/c_unit_%s.md", unitName))

	// Check stable spec exists
	if _, err := os.Stat(stableSpec); os.IsNotExist(err) {
		return actions, fmt.Errorf("stable spec not found: docs/specs/units/stable/s_unit_%s.md", unitName)
	}

	// Don't overwrite existing candidate
	if _, err := os.Stat(candidateSpec); err == nil {
		actions = append(actions, fmt.Sprintf("Candidate already exists — skipped fork: c_unit_%s.md", unitName))
		return actions, nil
	}

	// Fork appendices
	stableAppendixDir := filepath.Join(repoRoot, "docs/specs/units/stable/appendix")
	appendixPattern := fmt.Sprintf("s_unit_%s_*.md", unitName)
	appendixMatches, _ := filepath.Glob(filepath.Join(stableAppendixDir, appendixPattern))

	candidateAppendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	_ = os.MkdirAll(candidateAppendixDir, 0755)

	for _, m := range appendixMatches {
		candidateName := strings.Replace(filepath.Base(m), "s_unit_", "c_unit_", 1)
		dest := filepath.Join(candidateAppendixDir, candidateName)
		if err := copyWithLayerTransformReverse(m, dest); err != nil {
			return actions, fmt.Errorf("failed to fork appendix %s: %w", filepath.Base(m), err)
		}
		actions = append(actions, fmt.Sprintf("Forked appendix: docs/specs/units/stable/appendix/%s -> docs/specs/units/candidate/appendix/%s", filepath.Base(m), candidateName))
	}

	// Fork main spec
	if err := copyWithLayerTransformReverse(stableSpec, candidateSpec); err != nil {
		return actions, fmt.Errorf("failed to fork spec: %w", err)
	}
	actions = append(actions, fmt.Sprintf("Forked: docs/specs/units/stable/s_unit_%s.md -> docs/specs/units/candidate/c_unit_%s.md", unitName, unitName))

	return actions, nil
}

// FormatRuleResult formats the rule promote result as readable output.
func FormatRuleResult(r *RuleResult) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Rule: %s\n", r.RuleID)

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

	if len(r.ForkedUnits) > 0 {
		buf.WriteString("Forked consumers (MAJOR change — forked back to candidate):\n")
		for _, u := range r.ForkedUnits {
			fmt.Fprintf(&buf, "  - %s\n", u)
		}
		buf.WriteString("\n")
	}

	if len(r.CandidateUpdated) > 0 {
		buf.WriteString("Updated consumer rule_refs:\n")
		for _, u := range r.CandidateUpdated {
			fmt.Fprintf(&buf, "  - %s\n", u)
		}
		buf.WriteString("\n")
	}

	if r.Passed {
		switch r.ChangeType {
		case ChangeMajor:
			buf.WriteString("MAJOR: Rule promoted. Stable consumers forked to candidate. Re-validate and re-promote them.\n")
		case ChangeMinor, ChangePatch:
			buf.WriteString("Compatible change: Rule promoted. No consumer impact.\n")
		default:
			buf.WriteString("New rule promoted to stable.\n")
		}
	} else {
		buf.WriteString("Promote failed. Fix the issues above and try again.\n")
	}

	return buf.String()
}

// isVersionGreater checks if v1 > v2 using MAJOR.MINOR.PATCH comparison.
func isVersionGreater(v1, v2 string) bool {
	m1, n1, p1, ok1 := parseVersion(v1)
	m2, n2, p2, ok2 := parseVersion(v2)
	if !ok1 || !ok2 {
		return false
	}

	if m1 != m2 {
		return m1 > m2
	}
	if n1 != n2 {
		return n1 > n2
	}
	return p1 > p2
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

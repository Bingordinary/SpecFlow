package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitgraph"
)

// specPath is a shorthand for specpaths.CandidateUnitSpecFileRef(unitName).
// It produces docs/specs/units/candidate/unit_<unitName>.md.
func specPath(repoRoot, unitName string) string {
	return filepath.Join(repoRoot, specpaths.CandidateUnitSpecFileRef(unitName))
}

// ------------------------------------------------------------
// Check 1: Frontmatter completeness
// ------------------------------------------------------------
func checkFrontmatter(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Frontmatter completeness",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))

	required := []struct {
		field string
		label string
	}{
		{"id", "id"},
		{"version", "version"},
		{"unit_refs", "unit_refs"},
		{"rule_refs", "rule_refs"},
	}

	var missing []string
	for _, r := range required {
		if strings.TrimSpace(fm[r.field]) == "" {
			missing = append(missing, r.label)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Name:   "Frontmatter completeness",
			Status: Fail,
			Details: fmt.Sprintf("missing required fields: %s", strings.Join(missing, ", ")),
		}
	}

	if fm["id"] != unitName {
		return CheckResult{
			Name:    "Frontmatter completeness",
			Status:  Fail,
			Details: fmt.Sprintf("frontmatter id %q does not match unit name %q", fm["id"], unitName),
		}
	}

	return CheckResult{Name: "Frontmatter completeness", Status: Pass}
}

// ------------------------------------------------------------
// Check 2: Acceptance items format
// ------------------------------------------------------------
func checkAcceptanceItems(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	content := string(data)

	fm := specpaths.ReadFrontmatterStringMap(content)
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Pass,
			Details: "spec is marked retired — acceptance item set not required",
		}
	}

	if !strings.Contains(content, "acceptance_item_set:") {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: "acceptance_item_set not found",
		}
	}

	requiredItemFields := []string{
		"id:",
		"description:",
		"verification_type:",
		"verification_surface:",
		"implementation_surface:",
		"verification_method:",
		"pass_condition:",
		"runnable:",
	}

	itemBlocks := strings.Count(content, "\n  - id:")
	if itemBlocks == 0 {
		itemBlocks = strings.Count(content, "- id:")
	}
	if itemBlocks == 0 {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: "acceptance_item_set exists but no items found with - id:",
		}
	}

	itemSection := content[strings.Index(content, "acceptance_item_set"):]
	if strings.Contains(itemSection, "\n---") {
		itemSection = itemSection[:strings.Index(itemSection, "\n---")]
	}

	var missingFields []string
	for _, field := range requiredItemFields {
		if !strings.Contains(itemSection, field) {
			missingFields = append(missingFields, strings.TrimSuffix(field, ":"))
		}
	}

	if len(missingFields) > 0 {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: fmt.Sprintf("%d item(s) found, but missing fields in item section: %s", itemBlocks, strings.Join(missingFields, ", ")),
		}
	}

	// Validate runnable values (must be "yes" or "no" per spec_writing_guide.md)
	itemLines := strings.Split(itemSection, "\n")
	var invalidValues []string
	for _, line := range itemLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "runnable:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				value := strings.TrimSpace(parts[1])
				if value != "yes" && value != "no" {
					invalidValues = append(invalidValues, fmt.Sprintf("%q", value))
				}
			}
		}
	}
	if len(invalidValues) > 0 {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: fmt.Sprintf("invalid runnable value(s): %s; must be 'yes' or 'no'", strings.Join(invalidValues, ", ")),
		}
	}

	// Validate implementation_surface values (must be non-empty; <pending>
	// is the legal design-first placeholder — verify blocks on any leftover
	// <pending>, see framework/unit_verify_checklist.md Step 6)
	var emptySurface int
	for _, line := range itemLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "implementation_surface:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[1]) == "" {
				emptySurface++
			}
		}
	}
	if emptySurface > 0 {
		return CheckResult{
			Name:    "Acceptance items",
			Status:  Fail,
			Details: fmt.Sprintf("%d item(s) have an empty implementation_surface; use the <pending> placeholder during design-first rounds", emptySurface),
		}
	}

	return CheckResult{
		Name:    "Acceptance items",
		Status:  Pass,
		Details: fmt.Sprintf("%d item(s) found with required fields", itemBlocks),
	}
}

// ------------------------------------------------------------
// Check 3: Anchor integrity (affects.files paths exist)
// ------------------------------------------------------------
func checkAnchors(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	content := string(data)

	// A retiring spec is removed from stable — its affects.files anchors
	// describe implementation that is going away and are not required.
	fm := specpaths.ReadFrontmatterStringMap(content)
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Pass,
			Details: "spec is marked retired — affects.files paths not required",
		}
	}

	anchorFiles := ExtractAffectsFiles(content)

	if len(anchorFiles) == 0 {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Pass,
			Details: "no affects.files entries to check",
		}
	}

	var missingFiles []string
	for _, af := range anchorFiles {
		fullPath := filepath.Join(repoRoot, filepath.FromSlash(af))
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, af)
		}
	}

	if len(missingFiles) > 0 {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Fail,
			Details: fmt.Sprintf("affects.files paths not found: %s", strings.Join(missingFiles, ", ")),
		}
	}

	return CheckResult{
		Name:    "Anchor integrity",
		Status:  Pass,
		Details: fmt.Sprintf("%d affects.files path(s) exist", len(anchorFiles)),
	}
}

// ------------------------------------------------------------
// Check 4: Reference integrity (unit_refs/rule_refs files exist)
// ------------------------------------------------------------
func checkReferences(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Reference integrity",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))

	// A retiring spec's own references (unit_refs, rule_refs, appendix and
	// evidence references) disappear with it — reference checks apply only to
	// content that survives promote. Protection of OTHER units' references to
	// a retiring target lives in those units' own reference checks.
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Reference integrity",
			Status:  Pass,
			Details: "spec is marked retired — own references not checked",
		}
	}

	unitRefs := fm["unit_refs"]
	var missingRefs []string

	if unitRefs != "" && !strings.EqualFold(unitRefs, "none") {
		refs := specpaths.ParseRefList(unitRefs)
		for _, ref := range refs {
			refName := ref
			if atIdx := strings.LastIndex(ref, "@"); atIdx > 0 {
				refName = ref[:atIdx]
			}

			candidatePath := filepath.Join(repoRoot, "docs/specs/units/candidate", fmt.Sprintf("unit_%s.md", refName))
			if _, err := os.Stat(candidatePath); err == nil {
				// A referenced unit that is being retired loses its stable copy
				// on promote — the reference cannot survive the retirement.
				if cdata, rerr := os.ReadFile(candidatePath); rerr == nil {
					cfm := specpaths.ReadFrontmatterStringMap(string(cdata))
					if strings.TrimSpace(cfm["status"]) == "retired" {
						missingRefs = append(missingRefs, fmt.Sprintf("%s (being retired)", ref))
					}
				}
				continue
			}

			stablePath := filepath.Join(repoRoot, "docs/specs/units/stable", fmt.Sprintf("unit_%s.md", refName))
			if _, err := os.Stat(stablePath); err == nil {
				continue
			}

			missingRefs = append(missingRefs, ref)
		}
	}

	ruleRefs := fm["rule_refs"]
	if ruleRefs != "" && !strings.EqualFold(ruleRefs, "none") {
		refs := specpaths.ParseRefList(ruleRefs)
		for _, ref := range refs {
			refName := ref
			if atIdx := strings.LastIndex(ref, "@"); atIdx > 0 {
				refName = ref[:atIdx]
			}

			candidatePath := filepath.Join(repoRoot, "docs/specs/rules/candidate", fmt.Sprintf("%s.md", refName))
			if _, err := os.Stat(candidatePath); err == nil {
				continue
			}

			stablePath := filepath.Join(repoRoot, "docs/specs/rules/stable", fmt.Sprintf("%s.md", refName))
			if _, err := os.Stat(stablePath); err == nil {
				continue
			}

			missingRefs = append(missingRefs, ref)
		}
	}

	// Appendix references: a candidate appendix marked retired is removed on
	// promote, so neither affects.appendices entries nor evidence_appendix_ref
	// may reference it. Only refs that resolve to an existing candidate
	// appendix are judged mechanically; unresolvable refs are left to the
	// agent-side Check 6.
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	for _, ref := range ExtractAffectsAppendices(string(data)) {
		if AppendixMarkedRetired(appendixDir, ref) {
			missingRefs = append(missingRefs, fmt.Sprintf("%s (appendix being retired)", ref))
		}
	}
	if v := fm["evidence_appendix_ref"]; v != "" && !strings.EqualFold(v, "none") {
		if AppendixMarkedRetired(appendixDir, v) {
			missingRefs = append(missingRefs, fmt.Sprintf("%s (evidence appendix being retired)", v))
		}
	}

	if len(missingRefs) > 0 {
		return CheckResult{
			Name:    "Reference integrity",
			Status:  Fail,
			Details: fmt.Sprintf("referenced files not found: %s", strings.Join(missingRefs, ", ")),
		}
	}

	return CheckResult{Name: "Reference integrity", Status: Pass}
}

// ExtractAffectsAppendices extracts appendix file names referenced by
// affects.appendices entries inside the acceptance_item_set block. Both YAML
// list forms are recognized: the block form (`appendices:` followed by
// indented `- name` lines) and the inline flow form (`appendices: [a.md]`).
func ExtractAffectsAppendices(content string) []string {
	var refs []string
	lines := strings.Split(content, "\n")
	inAcceptance := false
	inAppendices := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "acceptance_item_set:") {
			inAcceptance = true
			continue
		}
		if !inAcceptance {
			continue
		}
		// The acceptance block ends at a top-level line (no indent) that is
		// not an item continuation.
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
			break
		}
		if trimmed == "appendices:" || strings.HasPrefix(trimmed, "appendices: ") {
			value := strings.TrimSpace(trimmed[len("appendices:"):])
			if value == "" {
				inAppendices = true
				continue
			}
			refs = append(refs, parseInlineRefList(value)...)
			continue
		}
		if inAppendices {
			if strings.HasPrefix(trimmed, "- ") {
				refs = append(refs, strings.TrimSpace(trimmed[2:]))
				continue
			}
			if trimmed != "" {
				inAppendices = false
			}
		}
	}
	return refs
}

// parseInlineRefList splits an inline YAML flow list value (`[a.md, "b.md"]`)
// into its items. A value without brackets is treated as a single item so
// that unexpected forms fail towards the mechanical check rather than
// silently passing.
func parseInlineRefList(value string) []string {
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		inner := strings.TrimSpace(value[1 : len(value)-1])
		if inner == "" {
			return nil
		}
		var refs []string
		for _, item := range strings.Split(inner, ",") {
			item = strings.Trim(strings.TrimSpace(item), `"'`)
			if item != "" {
				refs = append(refs, item)
			}
		}
		return refs
	}
	return []string{value}
}

// AppendixMarkedRetired reports whether the named appendix file exists in dir
// and is marked for retirement. A file that cannot be read is not judged.
func AppendixMarkedRetired(dir, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "/\\") {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return false
	}
	fm := specpaths.ReadFrontmatterStringMap(string(data))
	return strings.TrimSpace(fm["status"]) == "retired"
}

// ------------------------------------------------------------
// Check 5: Appendix files exist
// ------------------------------------------------------------
func checkAppendices(repoRoot, unitName string) CheckResult {
	appendixGlob := specpaths.CandidateAppendixGlob(unitName)
	fullGlob := filepath.Join(repoRoot, filepath.FromSlash(appendixGlob))
	matches, err := filepath.Glob(fullGlob)
	if err != nil {
		return CheckResult{
			Name:    "Appendix files",
			Status:  Pass,
			Details: fmt.Sprintf("error globbing appendices: %v", err),
		}
	}

	if len(matches) == 0 {
		return CheckResult{
			Name:    "Appendix files",
			Status:  Pass,
			Details: "no appendix files (optional)",
		}
	}

	var relPaths []string
	var errs []string
	for _, m := range matches {
		rel, _ := filepath.Rel(repoRoot, m)
		relPaths = append(relPaths, rel)

		data, readErr := os.ReadFile(m)
		if readErr != nil {
			errs = append(errs, fmt.Sprintf("%s: cannot read (%v)", rel, readErr))
			continue
		}
		fm := specpaths.ReadFrontmatterStringMap(string(data))
		status := strings.TrimSpace(fm["status"])
		if status == "exempt" || status == "retired" {
			continue
		}
		if strings.TrimSpace(fm["unit"]) != unitName {
			errs = append(errs, fmt.Sprintf("%s: frontmatter unit=%q, expected %q", rel, fm["unit"], unitName))
		}
	}

	if len(errs) > 0 {
		return CheckResult{
			Name:    "Appendix files",
			Status:  Fail,
			Details: fmt.Sprintf("%d appendix file(s) found with errors: %s", len(matches), strings.Join(errs, "; ")),
		}
	}

	return CheckResult{
		Name:    "Appendix files",
		Status:  Pass,
		Details: fmt.Sprintf("%d appendix file(s): %s", len(matches), strings.Join(relPaths, ", ")),
	}
}

// ------------------------------------------------------------
// Check 6: Version/ref consistency
// ------------------------------------------------------------
func checkVersionConsistency(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Version consistency",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))

	// A retiring spec's own version-pinned references disappear with it —
	// same exemption as the reference-integrity check (Check 4).
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Version consistency",
			Status:  Pass,
			Details: "spec is marked retired — own version refs not checked",
		}
	}

	unitRefs := fm["unit_refs"]
	var versionMismatches []string

	if unitRefs != "" && !strings.EqualFold(unitRefs, "none") {
		refs := specpaths.ParseRefList(unitRefs)
		for _, ref := range refs {
			refName := ref
			expectedVersion := ""
			if atIdx := strings.LastIndex(ref, "@"); atIdx > 0 {
				refName = ref[:atIdx]
				expectedVersion = ref[atIdx+1:]
			}
			if expectedVersion == "" {
				continue
			}

			targetFile := filepath.Join(repoRoot, "docs/specs/units/candidate", fmt.Sprintf("unit_%s.md", refName))
			targetData, err := os.ReadFile(targetFile)
			if err != nil {
				targetFile = filepath.Join(repoRoot, "docs/specs/units/stable", fmt.Sprintf("unit_%s.md", refName))
				targetData, err = os.ReadFile(targetFile)
				if err != nil {
					versionMismatches = append(versionMismatches, fmt.Sprintf("%s: cannot read target spec", ref))
					continue
				}
			}
			targetFM := specpaths.ReadFrontmatterStringMap(string(targetData))
			actualVersion := strings.TrimSpace(targetFM["version"])
			if actualVersion != expectedVersion {
				versionMismatches = append(versionMismatches, fmt.Sprintf("%s: expected version %q, target has %q", refName, expectedVersion, actualVersion))
			}
		}
	}

	if len(versionMismatches) > 0 {
		return CheckResult{
			Name:    "Version consistency",
			Status:  Fail,
			Details: strings.Join(versionMismatches, "; "),
		}
	}

	return CheckResult{Name: "Version consistency", Status: Pass}
}

// ------------------------------------------------------------
// Check 7: Body layer-path check
// ------------------------------------------------------------
//
// Candidate-layer spec paths are invalid anywhere in the spec body:
// the candidate layer is deleted on promote, so every such reference
// breaks. Relative-form matches are restricted to spec file naming
// (candidate/(appendix/)?(unit_|g_rule_|b_rule_)) so code paths like
// src/candidate/ are not misreported. Stable-layer paths are not
// checked here: they are legal in structured fields (spec document
// references must point to stable), which a string-level check
// cannot distinguish from prose.
var (
	layerAbsPatterns = []string{
		"docs/specs/units/candidate/",
		"docs/specs/rules/candidate/",
	}
	layerRelPattern = regexp.MustCompile(`candidate/(?:appendix/)?(?:unit_|g_rule_|b_rule_)[A-Za-z0-9_]+\.md`)
)

// FindCandidateLayerPathRefs returns the candidate-layer spec path
// references found in content (absolute and relative forms). Used by
// the promote workflow as a last-resort warning gate.
func FindCandidateLayerPathRefs(content string) []string {
	var refs []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		matched := false
		for _, p := range layerAbsPatterns {
			if strings.Contains(trimmed, p) {
				refs = append(refs, p)
				matched = true
			}
		}
		if !matched {
			if loc := layerRelPattern.FindStringIndex(trimmed); loc != nil {
				refs = append(refs, trimmed[loc[0]:loc[1]])
			}
		}
	}
	return refs
}

func checkLayerPaths(repoRoot, unitName string) CheckResult {
	var hits []string

	scanContent := func(label, content string) {
		for i, line := range strings.Split(content, "\n") {
			for _, ref := range FindCandidateLayerPathRefs(line) {
				hits = append(hits, fmt.Sprintf("%s line %d: contains %s", label, i+1, ref))
			}
		}
	}

	path := specPath(repoRoot, unitName)
	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Body layer-path check",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}
	// A retiring spec is removed from stable — layer-prefix references in its
	// body and in its appendices have no post-promote target and are not
	// checked (matching unit_validate_checklist.md: a retiring spec skips
	// Check 7 entirely, including its appendices).
	fm := specpaths.ReadFrontmatterStringMap(string(data))
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Body layer-path check",
			Status:  Pass,
			Details: "spec is marked retired — layer-path check skipped",
		}
	}
	scanContent(fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName), string(data))

	appendixGlob := specpaths.CandidateAppendixGlob(unitName)
	fullGlob := filepath.Join(repoRoot, filepath.FromSlash(appendixGlob))
	if matches, err := filepath.Glob(fullGlob); err == nil {
		for _, m := range matches {
			appendixData, readErr := os.ReadFile(m)
			if readErr != nil {
				continue
			}
			fm := specpaths.ReadFrontmatterStringMap(string(appendixData))
			status := strings.TrimSpace(fm["status"])
			if status == "exempt" || status == "retired" {
				continue
			}
			rel, _ := filepath.Rel(repoRoot, m)
			scanContent(rel, string(appendixData))
		}
	}

	if len(hits) > 0 {
		return CheckResult{
			Name:    "Body layer-path check",
			Status:  Fail,
			Details: fmt.Sprintf("candidate-layer spec path references in body (use concept names instead): %s", strings.Join(hits, "; ")),
		}
	}

	return CheckResult{Name: "Body layer-path check", Status: Pass}
}

// ------------------------------------------------------------
// Check 8: Dependency cycles (unit_refs graph)
// ------------------------------------------------------------

// cycleGuidance is the standard resolution guidance attached to every cycle
// finding. It lists the two structural resolutions without judging which one
// applies — the tool reports and blocks, the user decides how to unbind.
const cycleGuidance = "Resolve by extracting the shared contract into a rule (star-shaped dependencies), or re-drawing unit boundaries. See g_rule_repository_baseline.md §6.1 item 4; run `deps@all` for the dependency graph."

// cycleBuildGuidance is attached when the graph cannot be built at all. The
// failure cause is unrelated to the validated unit — any unreadable unit
// spec blocks the whole graph — so the guidance must point at the reported
// file rather than at cycle resolutions.
const cycleBuildGuidance = "Check 8 reads every current-layer unit spec to build the graph — an unreadable spec (permission, corruption) blocks all units, not just this one. Repair the reported file and re-run validate; run `deps@all` to reproduce the failure."

// checkDependencyCycles derives the dependency graph from all current-layer
// units' unit_refs and FAILS when the validated unit participates in a cycle.
// A cycle means no member can ever reach stable without drifting (the unit is
// blocked on its own dependency's unstable acceptance items), so every
// in-cycle unit FAILs — blocking promote. Units outside the cycle are not
// affected by it. Only explicit unit_refs edges count; prose is never
// inferred. A retiring spec is skipped — its references disappear with it.
func checkDependencyCycles(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)
	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Dependency cycles",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}
	fm := specpaths.ReadFrontmatterStringMap(string(data))
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Dependency cycles",
			Status:  Pass,
			Details: "spec is marked retired — cycle check skipped",
		}
	}

	graph, err := unitgraph.Build(repoRoot, "all")
	if err != nil {
		return CheckResult{
			Name:    "Dependency cycles",
			Status:  Fail,
			Details: fmt.Sprintf("cannot build dependency graph: %v. %s", err, cycleBuildGuidance),
		}
	}

	var involved []string
	for _, cycle := range graph.Cycles() {
		for _, member := range cycle {
			if member == unitName {
				involved = append(involved, strings.Join(cycle, " -> "))
				break
			}
		}
	}
	if len(involved) == 0 {
		return CheckResult{Name: "Dependency cycles", Status: Pass}
	}

	return CheckResult{
		Name:    "Dependency cycles",
		Status:  Fail,
		Details: fmt.Sprintf("circular dependency: %s. %s", strings.Join(involved, "; "), cycleGuidance),
	}
}

// ------------------------------------------------------------
// Check 9: Region locatability
// ------------------------------------------------------------
func checkRegionLocatability(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)

	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Region locatability",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
		}
	}
	content := string(data)

	fm := specpaths.ReadFrontmatterStringMap(content)
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{
			Name:    "Region locatability",
			Status:  Pass,
			Details: "spec is marked retired — region locatability not required",
		}
	}

	// 1. When the content mentions acceptance_item_set at all, the marker
	// must be locatable — the structural region locator
	// (contenthash.AcceptanceItemsRegion) matches only on a line whose
	// trimmed form is exactly `acceptance_item_set:` and never inside a
	// fenced code block, so inline, trailing-text, and fenced-only
	// variants all fail the locator. The locator is the single judge of
	// locatability — a scan that duplicated its rules would drift.
	if strings.Contains(content, "acceptance_item_set:") {
		if _, ok := contenthash.AcceptanceItemsRegion(content); !ok {
			return CheckResult{
				Name:    "Region locatability",
				Status:  Fail,
				Details: "acceptance_item_set: must appear as an exact standalone line outside a code fence — the structural region locator matches only the exact line outside fences; inline, trailing-text, or fenced variants cannot be located",
			}
		}
	}

	// 2. At least one ## heading: the frontmatter + section region model
	// needs a section boundary.
	regions := contenthash.SectionRegions(content)
	if len(regions) < 2 {
		return CheckResult{
			Name:    "Region locatability",
			Status:  Fail,
			Details: "spec has no ## heading — section regions cannot be located; restructure per framework/spec_writing_guide.md §13",
		}
	}

	// 3. Unique headings: a duplicated heading cannot be located
	// unambiguously and fails closed in the region locator.
	seen := make(map[string]bool)
	for _, r := range regions {
		if r.Heading == "" {
			continue
		}
		if r.Heading == "frontmatter" {
			return CheckResult{
				Name:    "Region locatability",
				Status:  Fail,
				Details: "reserved heading name \"frontmatter\" — the --section declaration spells the pre-heading region with \"frontmatter\", so a real ## frontmatter section cannot be declared by section regions; rename it",
			}
		}
		if seen[r.Heading] {
			return CheckResult{
				Name:    "Region locatability",
				Status:  Fail,
				Details: fmt.Sprintf("duplicated ## heading %q — section regions cannot be located unambiguously; rename one of them", r.Heading),
			}
		}
		seen[r.Heading] = true
	}

	// 4. Heading format: `## ` must be followed by heading text. A
	// near-miss line (`##x`, or a bare `##`) is content to the splitter but
	// almost always means the author intended a heading.
	if malformed := contenthash.MalformedHeadingLines(content); len(malformed) > 0 {
		return CheckResult{
			Name:    "Region locatability",
			Status:  Fail,
			Details: fmt.Sprintf("malformed heading line(s): %s — must be `## ` followed by heading text", strings.Join(malformed, ", ")),
		}
	}

	return CheckResult{
		Name:    "Region locatability",
		Status:  Pass,
		Details: fmt.Sprintf("%d section regions located (%d ## headings)", len(regions), len(seen)),
	}
}

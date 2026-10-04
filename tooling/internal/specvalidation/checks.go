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

	return CheckUnitFrontmatter(string(data), unitName)
}

// CheckUnitFrontmatter validates the required unit fields, requested identity
// and status. Candidate checking and publication share this content check.
func CheckUnitFrontmatter(content, unitName string) CheckResult {
	fm := specpaths.ReadFrontmatterStringMap(content)
	if status := fm["status"]; status != "" && status != "active" {
		return CheckResult{Name: "Frontmatter completeness", Status: Fail, Details: "unit status must be active or absent; deletion uses specflowctl remove"}
	}

	required := []struct {
		field string
		label string
	}{
		{"id", "id"},
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
			Name:    "Frontmatter completeness",
			Status:  Fail,
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

	return CheckAcceptanceItemSchema(string(data))
}

// ------------------------------------------------------------
// Check 3: Anchor integrity (affects.files paths exist and
// implementation_surface values resolve to real code)
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

	// describe implementation that is going away and are not required.

	var problems []string

	// implementation_surface values must be the exact <pending> design-first
	// placeholder or a single path that yields at least one file — a declared
	// file, or a directory's repository-content files (the files Git tracks
	// plus untracked files that are not ignored). A non-pending value that
	// yields none would silently derive an empty code surface, so it fails
	// here (same check gate-plan applies before planning a verify run).
	for _, surfaceProblem := range CheckImplementationSurfaces(repoRoot, content) {
		problems = append(problems, surfaceProblem.String())
	}

	anchorFiles := ExtractAffectsFiles(content)

	var missingFiles []string
	for _, af := range anchorFiles {
		fullPath := filepath.Join(repoRoot, filepath.FromSlash(af))
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, af)
		}
	}

	if len(missingFiles) > 0 {
		problems = append(problems, fmt.Sprintf("affects.files paths not found: %s", strings.Join(missingFiles, ", ")))
	}

	if len(problems) > 0 {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Fail,
			Details: strings.Join(problems, "; "),
		}
	}

	if len(anchorFiles) == 0 {
		return CheckResult{
			Name:    "Anchor integrity",
			Status:  Pass,
			Details: "no affects.files entries to check; implementation_surface values resolve",
		}
	}

	return CheckResult{
		Name:    "Anchor integrity",
		Status:  Pass,
		Details: fmt.Sprintf("%d affects.files path(s) exist; implementation_surface values resolve", len(anchorFiles)),
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

	unitRefs := fm["unit_refs"]
	var missingRefs []string

	if unitRefs != "" && !strings.EqualFold(unitRefs, "none") {
		refs := specpaths.ParseRefList(unitRefs)
		for _, ref := range refs {
			candidatePath := filepath.Join(repoRoot, "docs/specs/units/candidate", fmt.Sprintf("unit_%s.md", ref))
			if _, err := os.Stat(candidatePath); err == nil {

				continue
			}

			stablePath := filepath.Join(repoRoot, "docs/specs/units/stable", fmt.Sprintf("unit_%s.md", ref))
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
			candidatePath := filepath.Join(repoRoot, "docs/specs/rules/candidate", fmt.Sprintf("%s.md", ref))
			if _, err := os.Stat(candidatePath); err == nil {
				continue
			}

			stablePath := filepath.Join(repoRoot, "docs/specs/rules/stable", fmt.Sprintf("%s.md", ref))
			if _, err := os.Stat(stablePath); err == nil {
				continue
			}

			missingRefs = append(missingRefs, ref)
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
	for _, item := range parseAcceptanceItems(content) {
		refs = append(refs, item.affectsAppendices...)
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

// ------------------------------------------------------------
// Check 5: Appendix files exist
// ------------------------------------------------------------
func checkAppendices(repoRoot, unitName string) CheckResult {
	appendices, err := specpaths.UnitAppendices(repoRoot, unitName, "candidate")
	if err != nil {
		return CheckResult{
			Name:    "Appendix files",
			Status:  Fail,
			Details: err.Error(),
		}
	}

	if len(appendices) == 0 {
		return CheckResult{
			Name:    "Appendix files",
			Status:  Pass,
			Details: "no appendix files (optional)",
		}
	}

	var relPaths []string
	for _, appendix := range appendices {
		relPaths = append(relPaths, appendix.Path)
	}

	return CheckResult{
		Name:    "Appendix files",
		Status:  Pass,
		Details: fmt.Sprintf("%d appendix file(s): %s", len(appendices), strings.Join(relPaths, ", ")),
	}
}

// ------------------------------------------------------------
// Check 6: Body layer-path check
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
	// body and in its appendices have no post-promote target and are not
	// Check 6 entirely, including its appendices).

	scanContent(fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName), string(data))

	appendices, err := specpaths.UnitAppendices(repoRoot, unitName, "candidate")
	if err != nil {
		return CheckResult{Name: "Body layer-path check", Status: Fail, Details: err.Error()}
	}
	for _, appendix := range appendices {
		if appendix.Status == "exempt" {
			continue
		}
		appendixData, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(appendix.Path)))
		if err != nil {
			return CheckResult{Name: "Body layer-path check", Status: Fail, Details: err.Error()}
		}
		scanContent(appendix.Path, string(appendixData))
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
// Check 7: Dependency cycles (unit_refs graph)
// ------------------------------------------------------------

// cycleGuidance is the standard resolution guidance attached to every cycle
// finding. It lists the two structural resolutions without judging which one
// applies — the tool reports and blocks, the user decides how to unbind.
const cycleGuidance = "Resolve by extracting the shared contract into a rule (star-shaped dependencies), or re-drawing unit boundaries. See g_rule_repository_baseline.md §6.1 item 4; run `deps@all` for the dependency graph."

// cycleBuildGuidance is attached when the graph cannot be built at all. The
// failure cause is unrelated to the validated unit — any unreadable unit
// spec blocks the whole graph — so the guidance must point at the reported
// file rather than at cycle resolutions.
const cycleBuildGuidance = "Check 7 reads every current-layer unit spec to build the graph — an unreadable spec (permission, corruption) blocks all units, not just this one. Repair the reported file and re-run validate; run `deps@all` to reproduce the failure."

// checkDependencyCycles derives the dependency graph from all current-layer
// units' unit_refs and FAILS when the validated unit participates in a cycle.
// A cycle means no member can ever reach stable without drifting (the unit is
// blocked on its own dependency's unstable acceptance items), so every
// in-cycle unit FAILs — blocking promote. Units outside the cycle are not
// affected by it. Only explicit unit_refs edges count; prose is never
func checkDependencyCycles(repoRoot, unitName string) CheckResult {
	path := specPath(repoRoot, unitName)
	_, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:    "Dependency cycles",
			Status:  Fail,
			Details: fmt.Sprintf("cannot read candidate spec: %v", err),
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
// Check 8: Region locatability
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

	// 4. Non-empty acceptance item ids: an empty id cannot be located as an
	// item region and makes the whole-set semantic identity uncomputable
	// (contenthash.AcceptanceItemSetCID fails closed on it), so it is the
	// same locatability defect as a duplicated id. The scan is shared with
	// that computation — a duplicated rule would drift.
	if line, found := contenthash.EmptyAcceptanceItemIDLine(content); found {
		return CheckResult{
			Name:    "Region locatability",
			Status:  Fail,
			Details: fmt.Sprintf("acceptance item at line %d has an empty id — item regions cannot be located and whole-set declarations fail closed; give every item a non-empty id (framework/spec_writing_guide.md §7)", line),
		}
	}

	// 5. Unique acceptance item ids: a duplicated id cannot be located
	// unambiguously by the item-region locator, so item-region declarations
	// naming it fail closed. Item ids are required to be unique by
	// framework/spec_writing_guide.md §7.
	seenItem := make(map[string]bool)
	for _, id := range contenthash.AcceptanceItemIDs(content) {
		if seenItem[id] {
			return CheckResult{
				Name:    "Region locatability",
				Status:  Fail,
				Details: fmt.Sprintf("duplicated acceptance item id %q — item regions cannot be located unambiguously; make the ids unique (framework/spec_writing_guide.md §7)", id),
			}
		}
		seenItem[id] = true
	}

	// 6. Heading format: `## ` must be followed by heading text. A
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

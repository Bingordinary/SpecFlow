package rulesync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/impactsync"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulebinding"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulerefs"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitdiscovery"
)

type Options struct {
	Modules                      []string
	RuleRefs                     []string
	RuleIDs                      []string
	DeletedRuleRefs              []string
	StableLandingModule          string
	StableLandingRuleRefs        []string
	RetargetedUnits              []string
	BoundObjectsOnlyRuleFileRefs []string
}

type Result struct {
	ScopedModules                []string
	ScopedRuleRefs               []string
	ScopedRuleIDs                []string
	DeletedRuleRefs              []string
	StableLandingModule          string
	StableLandingRuleRefs        []string
	RetargetedUnits              []string
	BoundObjectsOnlyRuleFileRefs []string
	ModuleResults                []ModuleResult
}

type ModuleResult = impactsync.ModuleResult

type moduleBinding struct {
	ID            string
	ActiveLayer   string
	RuleRefs      []string
	BindingIssues []string
}

type sharedFile struct {
	RuleID             string
	RuleScope          string
	Layer              string
	FileRef            string
	VersionRef         string
	RuleVersion        string
	PromotionOwnerUnit string
}

func SyncImpact(repoRoot string, options Options) (Result, error) {
	normalized := Options{
		Modules:                      normalizeStrings(options.Modules),
		RuleRefs:                     normalizeStrings(options.RuleRefs),
		RuleIDs:                      normalizeStrings(options.RuleIDs),
		DeletedRuleRefs:              normalizeStrings(options.DeletedRuleRefs),
		StableLandingModule:          strings.TrimSpace(options.StableLandingModule),
		StableLandingRuleRefs:        normalizeStrings(options.StableLandingRuleRefs),
		RetargetedUnits:              normalizeStrings(options.RetargetedUnits),
		BoundObjectsOnlyRuleFileRefs: normalizeStrings(options.BoundObjectsOnlyRuleFileRefs),
	}
	if len(normalized.RuleRefs) == 0 && len(normalized.RuleIDs) == 0 && len(normalized.DeletedRuleRefs) == 0 {
		return Result{}, fmt.Errorf("at least one of rule refs, rule ids, or deleted rule refs is required")
	}
	if len(normalized.BoundObjectsOnlyRuleFileRefs) > 0 {
		return Result{}, fmt.Errorf("bound_objects-only sync is no longer supported; derive consumers from current-layer rule_refs")
	}
	if _, err := rulerefs.NormalizeRuleRefs(normalized.DeletedRuleRefs); err != nil {
		return Result{}, fmt.Errorf("deleted rule refs: %w", err)
	}

	sharedFilesByRef, err := loadSharedFiles(repoRoot)
	if err != nil {
		return Result{}, err
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)

	moduleBindings, _, _, unresolvedRuleRefs, err := loadModuleBindings(repoRoot)
	if err != nil {
		return Result{}, err
	}
	allUnresolvedRefs := normalizeStrings(unresolvedRuleRefs)
	referencedRuleRefs := allReferencedRuleRefs(moduleBindings)
	if err := validateDeletedRuleRefs(normalized.DeletedRuleRefs, sharedFilesByRef, referencedRuleRefs); err != nil {
		return Result{}, err
	}
	for _, sharedID := range normalized.RuleIDs {
		if len(allUnresolvedRefs) > 0 {
			return Result{}, fmt.Errorf(
				"cannot determine affected downstream objects safely for rule id %q because unresolved rule refs remain in downstream bindings: %s",
				sharedID,
				strings.Join(allUnresolvedRefs, ", "),
			)
		}
		if _, ok := sharedFilesByID[sharedID]; !ok {
			return Result{}, fmt.Errorf("rule id %q is not present under docs/specs/rules/", sharedID)
		}
	}
	for _, ref := range normalized.RuleRefs {
		if _, ok := sharedFilesByRef[ref]; ok {
			continue
		}
		if referencedRuleRefs[ref] {
			continue
		}
		return Result{}, fmt.Errorf("rule ref %q is not present under docs/specs/rules/ and is not referenced by current downstream bindings", ref)
	}
	for _, module := range normalized.Modules {
		if _, ok := moduleBindings[module]; !ok {
			return Result{}, fmt.Errorf("module %q is not registered in docs/specs/_status.md", module)
		}
	}
	if normalized.StableLandingModule != "" {
		binding, ok := moduleBindings[normalized.StableLandingModule]
		if !ok {
			return Result{}, fmt.Errorf("stable landing unit %q is not registered in docs/specs/_status.md", normalized.StableLandingModule)
		}
		if len(normalized.StableLandingRuleRefs) == 0 {
			return Result{}, fmt.Errorf("stable landing rule refs are required when stable landing unit %q is set", normalized.StableLandingModule)
		}
		if binding.ActiveLayer != "stable" {
			return Result{}, fmt.Errorf("stable landing unit %q must currently be at active layer stable", normalized.StableLandingModule)
		}
		landingSelectedRefs, err := selectedRuleRefsForObject(binding.RuleRefs, normalized.RuleRefs, normalized.RuleIDs, sharedFilesByRef, sharedFilesByID)
		if err != nil {
			return Result{}, err
		}
		landingSelectedRefSet := makeStringSet(landingSelectedRefs)
		for _, ref := range normalized.StableLandingRuleRefs {
			if _, ok := sharedFilesByRef[ref]; !ok {
				return Result{}, fmt.Errorf("stable landing rule ref %q is not present under docs/specs/rules/", ref)
			}
			if !landingSelectedRefSet[ref] {
				return Result{}, fmt.Errorf("stable landing rule ref %q is not selected for stable landing unit %q in this shared sync scope", ref, normalized.StableLandingModule)
			}
		}
	} else if len(normalized.StableLandingRuleRefs) > 0 {
		return Result{}, fmt.Errorf("stable landing rule refs require stable landing unit")
	}
	retargetedUnitSet, err := validateRetargetedUnits(moduleBindings, sharedFilesByRef, normalized)
	if err != nil {
		return Result{}, err
	}

	boundObjectsOnlyFileRefs := map[string]bool{}

	removedBindingModules, err := candidateModulesWithRemovedSelectedBinding(repoRoot, moduleBindings, normalized.RuleRefs, normalized.RuleIDs, sharedFilesByRef, sharedFilesByID)
	if err != nil {
		return Result{}, err
	}
	scopeModules, err := buildScopeModules(moduleBindings, sharedFilesByRef, sharedFilesByID, normalized, removedBindingModules)
	if err != nil {
		return Result{}, err
	}
	scopeModules = unionSortedStrings(scopeModules, normalized.RetargetedUnits)

	moduleImpactScope, err := scopedModulesForImpact(scopeModules, moduleBindings, normalized, sharedFilesByRef, sharedFilesByID, boundObjectsOnlyFileRefs, removedBindingModules, retargetedUnitSet)
	if err != nil {
		return Result{}, err
	}

	impactResult, err := impactsync.Apply(repoRoot, impactsync.Input{
		Modules: moduleImpactScope,
	})
	if err != nil {
		return Result{}, err
	}

	return Result{
		ScopedModules:         scopeModules,
		ScopedRuleRefs:        normalized.RuleRefs,
		ScopedRuleIDs:         normalized.RuleIDs,
		DeletedRuleRefs:       normalized.DeletedRuleRefs,
		StableLandingModule:   normalized.StableLandingModule,
		StableLandingRuleRefs: normalized.StableLandingRuleRefs,
		RetargetedUnits:       normalized.RetargetedUnits,
		ModuleResults:         impactResult.ModuleResults,
	}, nil
}

func validateDeletedRuleRefs(deletedRuleRefs []string, sharedFilesByRef map[string]sharedFile, referencedRuleRefs map[string]bool) error {
	for _, ref := range deletedRuleRefs {
		if _, exists := sharedFilesByRef[ref]; exists {
			return fmt.Errorf("deleted rule ref %q is still present under docs/specs/rules/", ref)
		}
		if referencedRuleRefs[ref] {
			return fmt.Errorf("deleted rule ref %q is still referenced by current-layer unit rule_refs", ref)
		}
	}
	return nil
}

func loadSharedFiles(repoRoot string) (map[string]sharedFile, error) {
	result := map[string]sharedFile{}
	for _, root := range []struct {
		layer string
		dir   string
	}{
		{layer: "candidate", dir: "docs/specs/rules/candidate"},
		{layer: "stable", dir: "docs/specs/rules/stable"},
	} {
		pattern := filepath.Join(repoRoot, filepath.FromSlash(root.dir), "*.md")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			relPath, err := filepath.Rel(repoRoot, match)
			if err != nil {
				return nil, err
			}
			relPath = filepath.ToSlash(relPath)
			content, err := os.ReadFile(match)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", relPath, err)
			}
			shared, err := parseSharedFile(relPath, string(content))
			if err != nil {
				return nil, err
			}
			if err := rulebinding.ValidatePromotionOwnerUnit(repoRoot, shared.FileRef, shared.Layer, shared.PromotionOwnerUnit); err != nil {
				return nil, err
			}
			if shared.Layer != root.layer {
				return nil, fmt.Errorf("%s: frontmatter.layer=%s does not match path layer %s", relPath, shared.Layer, root.layer)
			}
			if _, exists := result[shared.VersionRef]; exists {
				return nil, fmt.Errorf("duplicate shared version ref %s", shared.VersionRef)
			}
			result[shared.VersionRef] = shared
		}
	}
	return result, nil
}

func buildSharedFilesByID(sharedFilesByRef map[string]sharedFile) map[string][]sharedFile {
	result := map[string][]sharedFile{}
	for _, shared := range sharedFilesByRef {
		result[shared.RuleID] = append(result[shared.RuleID], shared)
	}
	for sharedID := range result {
		sort.Slice(result[sharedID], func(i, j int) bool {
			return result[sharedID][i].VersionRef < result[sharedID][j].VersionRef
		})
	}
	return result
}

func loadModuleBindings(repoRoot string) (map[string]moduleBinding, map[string][]string, map[string][]string, []string, error) {
	units, err := unitdiscovery.DiscoverUnits(repoRoot)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	bindings := make(map[string]moduleBinding, len(units))
	actualByRef := map[string][]string{}
	actualByID := map[string][]string{}
	unresolvedRefs := []string{}
	for _, unit := range units {
		refs, err := readUnitRuleRefs(repoRoot, unit)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		bindingIssues := []string{}
		bindings[unit.ID] = moduleBinding{
			ID:          unit.ID,
			ActiveLayer: unit.Layer(),
			RuleRefs:    refs,
		}
		for _, ref := range refs {
			resolved, err := rulebinding.ResolveRef(repoRoot, unit.Layer(), ref)
			if err != nil {
				bindingIssues = append(bindingIssues, err.Error())
				unresolvedRefs = append(unresolvedRefs, ref)
				continue
			}
			actualByRef[resolved.VersionRef] = append(actualByRef[resolved.VersionRef], unit.ID)
			actualByID[resolved.RuleID] = append(actualByID[resolved.RuleID], unit.ID)
		}
		bindings[unit.ID] = moduleBinding{
			ID:          unit.ID,
			ActiveLayer: unit.Layer(),
			RuleRefs:    refs,
			BindingIssues: normalizeStrings(bindingIssues),
		}
	}
	for ref := range actualByRef {
		actualByRef[ref] = normalizeStrings(actualByRef[ref])
	}
	for sharedID := range actualByID {
		actualByID[sharedID] = normalizeStrings(actualByID[sharedID])
	}
	return bindings, actualByRef, actualByID, normalizeStrings(unresolvedRefs), nil
}

func buildScopeModules(moduleBindings map[string]moduleBinding, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile, options Options, removedBindingScope map[string]bool) ([]string, error) {
	affected := map[string]bool{}
	if len(stableGlobalRuleRefsForScope(options.RuleRefs, options.RuleIDs, sharedFilesByRef, sharedFilesByID)) > 0 {
		for module := range moduleBindings {
			affected[module] = true
		}
	}
	for module, binding := range moduleBindings {
		selectedRefs, err := selectedRuleRefsForObject(binding.RuleRefs, options.RuleRefs, options.RuleIDs, sharedFilesByRef, sharedFilesByID)
		if err != nil {
			return nil, err
		}
		if len(selectedRefs) > 0 {
			affected[module] = true
		}
	}
	for module := range removedBindingScope {
		affected[module] = true
	}
	if len(options.Modules) == 0 {
		return sortedKeys(affected), nil
	}
	narrowed := map[string]bool{}
	for _, module := range options.Modules {
		if affected[module] {
			narrowed[module] = true
		}
	}
	return sortedKeys(narrowed), nil
}

func scopedModulesForImpact(scopeModules []string, moduleBindings map[string]moduleBinding, options Options, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile, boundModulesOnlyFileRefs map[string]bool, removedBindingScope map[string]bool, retargetedUnitSet map[string]bool) ([]impactsync.ScopedModule, error) {
	result := make([]impactsync.ScopedModule, 0, len(scopeModules))
	stableLandingSharedRefSet := makeStringSet(options.StableLandingRuleRefs)
	stableGlobalRuleRefs := stableGlobalRuleRefsForScope(options.RuleRefs, options.RuleIDs, sharedFilesByRef, sharedFilesByID)
	for _, module := range scopeModules {
		binding := moduleBindings[module]
		selectedRuleRefs, err := selectedRuleRefsForObject(binding.RuleRefs, options.RuleRefs, options.RuleIDs, sharedFilesByRef, sharedFilesByID)
		if err != nil {
			return nil, err
		}
		invalidatingRuleRefs := unionSortedStrings(
			filterInvalidatingRuleRefs(selectedRuleRefs, sharedFilesByRef, boundModulesOnlyFileRefs),
			stableGlobalRuleRefs,
		)
		if binding.ActiveLayer == "stable" && options.StableLandingModule == module {
			invalidatingRuleRefs = subtractStringSet(invalidatingRuleRefs, stableLandingSharedRefSet)
		}
		result = append(result, impactsync.ScopedModule{
			Binding: impactsync.ModuleBinding{
				Module:        binding.ID,
				ActiveLayer:   binding.ActiveLayer,
				
				
			},
			InvalidatingRuleRefs:                  invalidatingRuleRefs,
			ExplicitFallbackScope: removedBindingScope[module] || retargetedUnitSet[module],
		})
	}
	return result, nil
}

func stableGlobalRuleRefsForScope(scopedRefs, scopedIDs []string, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile) []string {
	refs := []string{}
	for _, ref := range scopedRefs {
		shared, ok := sharedFilesByRef[ref]
		if ok && isStableGlobalRule(shared) {
			refs = append(refs, shared.VersionRef)
		}
	}
	for _, sharedID := range scopedIDs {
		for _, shared := range sharedFilesByID[sharedID] {
			if isStableGlobalRule(shared) {
				refs = append(refs, shared.VersionRef)
			}
		}
	}
	return normalizeStrings(refs)
}

func isStableGlobalRule(shared sharedFile) bool {
	if shared.Layer != "stable" {
		return false
	}
	if shared.RuleScope == "global" {
		return true
	}
	return strings.HasPrefix(shared.RuleID, "g_rule_")
}

func validateRetargetedUnits(moduleBindings map[string]moduleBinding, sharedFilesByRef map[string]sharedFile, options Options) (map[string]bool, error) {
	retargetedUnitSet := makeStringSet(options.RetargetedUnits)
	if len(retargetedUnitSet) == 0 {
		return retargetedUnitSet, nil
	}
	if options.StableLandingModule == "" || len(options.StableLandingRuleRefs) == 0 {
		return nil, fmt.Errorf("retargeted units require stable landing unit and stable landing rule refs")
	}
	stableLandingRuleRefSet := makeStringSet(options.StableLandingRuleRefs)
	for _, ref := range options.StableLandingRuleRefs {
		shared, ok := sharedFilesByRef[ref]
		if !ok {
			return nil, fmt.Errorf("stable landing rule ref %q is not present under docs/specs/rules/", ref)
		}
		if shared.Layer != "stable" {
			return nil, fmt.Errorf("stable landing rule ref %q must point to a stable-layer rule file", ref)
		}
		if !hasSameVersionCandidateRuleRef(shared, options.RuleRefs, sharedFilesByRef) {
			return nil, fmt.Errorf("stable landing rule ref %q requires a candidate-layer rule ref with the same rule_id and rule_version in --rule-refs", ref)
		}
	}
	for _, unit := range options.RetargetedUnits {
		binding, ok := moduleBindings[unit]
		if !ok {
			return nil, fmt.Errorf("retargeted unit %q is not registered in docs/specs/_status.md", unit)
		}
		if binding.ActiveLayer != "candidate" {
			return nil, fmt.Errorf("retargeted unit %q must currently be at active layer candidate", unit)
		}
		selectedStableLandingRefs := intersectStrings(binding.RuleRefs, stableLandingRuleRefSet)
		if len(selectedStableLandingRefs) == 0 {
			return nil, fmt.Errorf("retargeted unit %q must currently bind at least one stable landing rule ref", unit)
		}
	}
	return retargetedUnitSet, nil
}

func hasSameVersionCandidateRuleRef(stable sharedFile, ruleRefs []string, sharedFilesByRef map[string]sharedFile) bool {
	for _, ref := range ruleRefs {
		candidate, ok := sharedFilesByRef[ref]
		if !ok {
			continue
		}
		if candidate.Layer == "candidate" && candidate.RuleID == stable.RuleID && candidate.RuleVersion == stable.RuleVersion {
			return true
		}
	}
	return false
}

func candidateModulesWithRemovedSelectedBinding(repoRoot string, moduleBindings map[string]moduleBinding, scopedRefs, scopedIDs []string, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile) (map[string]bool, error) {
	// Process snapshots no longer exist in the simplified model (file existence is state).
	// Return empty — removed bindings are tracked through the standard binding scope.
	return map[string]bool{}, nil
}

func allReferencedRuleRefs(moduleBindings map[string]moduleBinding) map[string]bool {
	result := map[string]bool{}
	for _, binding := range moduleBindings {
		for _, ref := range binding.RuleRefs {
			result[ref] = true
		}
	}
	return result
}

func sortedKeys(scope map[string]bool) []string {
	result := make([]string, 0, len(scope))
	for item := range scope {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func selectedRuleRefsForObject(objectRefs, scopedRefs, scopedIDs []string, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile) ([]string, error) {
	refSet := map[string]bool{}
	for _, ref := range scopedRefs {
		refSet[bareRuleName(ref)] = true
	}
	idSet := map[string]bool{}
	for _, sharedID := range scopedIDs {
		idSet[sharedID] = true
	}

	result := []string{}
	for _, ref := range objectRefs {
		if refSet[bareRuleName(ref)] {
			result = append(result, ref)
			continue
		}
		shared, ok := sharedFilesByRef[ref]
		if !ok {
			for key, s := range sharedFilesByRef {
				if bareRuleName(key) == bareRuleName(ref) {
					shared = s
					ok = true
					break
				}
			}
		}
		if ok && idSet[shared.RuleID] {
			if len(sharedFilesByID[shared.RuleID]) > 1 {
				return nil, fmt.Errorf("cannot determine affected downstream objects safely for rule id %q because multiple current shared layers exist", shared.RuleID)
			}
			result = append(result, ref)
		}
	}
	return normalizeStrings(result), nil
}

func filterInvalidatingRuleRefs(selectedRefs []string, sharedFilesByRef map[string]sharedFile, boundObjectsOnlyFileRefs map[string]bool) []string {
	return normalizeStrings(selectedRefs)
}

func subtractStringSet(values []string, excluded map[string]bool) []string {
	if len(values) == 0 || len(excluded) == 0 {
		return normalizeStrings(values)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !excluded[value] {
			result = append(result, value)
		}
	}
	return normalizeStrings(result)
}

func intersectStrings(values []string, included map[string]bool) []string {
	if len(values) == 0 || len(included) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if included[value] {
			result = append(result, value)
		}
	}
	return normalizeStrings(result)
}

func unionSortedStrings(left, right []string) []string {
	return normalizeStrings(append(append([]string{}, left...), right...))
}

func makeStringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = true
		}
	}
	return result
}

func parseSharedFile(relPath, content string) (sharedFile, error) {
	hasBoundObjects, err := rulerefs.HasRuleBoundObjects(relPath, content)
	if err != nil {
		return sharedFile{}, err
	}
	if hasBoundObjects {
		return sharedFile{}, fmt.Errorf("%s: bound_objects is forbidden; derive consumers from current-layer rule_refs", relPath)
	}

	fm, _, err := specpaths.ParseFrontmatterFields(content)
	if err != nil {
		return sharedFile{}, fmt.Errorf("%s: %w", relPath, err)
	}

	shared := sharedFile{FileRef: relPath}
	shared.RuleID = strings.TrimSpace(fm["rule_id"])
	shared.RuleScope = strings.TrimSpace(fm["rule_scope"])
	shared.Layer = strings.TrimSpace(fm["layer"])
	shared.RuleVersion = strings.TrimSpace(fm["rule_version"])
	if shared.RuleVersion != "" {
		shared.VersionRef = fmt.Sprintf("%s@%s", strings.TrimSuffix(filepath.Base(relPath), ".md"), shared.RuleVersion)
	}
	shared.PromotionOwnerUnit = strings.TrimSpace(fm["promotion_owner_unit"])

	if shared.RuleID == "" || shared.Layer == "" || shared.VersionRef == "" {
		return sharedFile{}, fmt.Errorf("%s: missing rule_id, layer, or rule_version", relPath)
	}
	return shared, nil
}

func readUnitRuleRefs(repoRoot string, unit unitdiscovery.UnitInfo) ([]string, error) {
	fileRef := fmt.Sprintf("docs/specs/units/%s/c_unit_%s.md", "candidate", unit.ID)
	if !unit.HasCandidate {
		fileRef = fmt.Sprintf("docs/specs/units/%s/s_unit_%s.md", "stable", unit.ID)
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", fileRef, err)
	}
	refs, err := rulerefs.ParseObjectRuleRefs(fileRef, string(content))
	if err != nil {
		return nil, err
	}
	return normalizeStrings(refs), nil
}

func parseRuleRefs(body string) ([]string, bool, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		right, matched, err := parseNamedFieldLine(trimmed, "rule_refs")
		if err != nil {
			return nil, false, err
		}
		if !matched {
			continue
		}
		if right == "`none`" || right == "none" {
			return nil, true, nil
		}
		if right != "" {
			return nil, false, fmt.Errorf("rule_refs must use literal none or a markdown list")
		}
		refs := []string{}
		seen := map[string]bool{}
		for next := idx + 1; next < len(lines); next++ {
			nextTrimmed := strings.TrimSpace(lines[next])
			if nextTrimmed == "" {
				continue
			}
			if strings.HasPrefix(nextTrimmed, "## ") || isNumberedListLine(nextTrimmed) {
				break
			}
			if !strings.HasPrefix(nextTrimmed, "- ") {
				return nil, false, fmt.Errorf("rule_refs must be a markdown list of rule refs")
			}
			ref := strings.TrimSpace(strings.TrimPrefix(nextTrimmed, "- "))
			ref = strings.Trim(ref, "`")
			if ref == "" {
				return nil, false, fmt.Errorf("rule_refs contains an empty item")
			}
			if seen[ref] {
				return nil, false, fmt.Errorf("rule_refs contains duplicate item %q", ref)
			}
			seen[ref] = true
			refs = append(refs, ref)
		}
		if len(refs) == 0 {
			return nil, false, fmt.Errorf("rule_refs must not be an empty list")
		}
		if err := validateOrderedRuleRefs(refs); err != nil {
			return nil, false, err
		}
		return refs, true, nil
	}
	return nil, false, nil
}

func validateOrderedRuleRefs(refs []string) error {
	if len(refs) < 2 {
		return nil
	}
	expected := append([]string(nil), refs...)
	sort.Strings(expected)
	for idx := range refs {
		if refs[idx] != expected[idx] {
			return fmt.Errorf("rule_refs must be sorted by exact rule ref string in ascending lexical order")
		}
	}
	return nil
}

func parseNamedFieldLine(trimmed, fieldName string) (string, bool, error) {
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return "", false, nil
	}
	left := normalizeFieldKey(strings.TrimSpace(parts[0]))
	if left != fieldName {
		return "", false, nil
	}
	return strings.TrimSpace(parts[1]), true, nil
}

func normalizeFieldKey(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "`", "")
	if idx := strings.Index(value, ". "); idx > 0 {
		allDigits := true
		for _, ch := range value[:idx] {
			if ch < '0' || ch > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			value = value[idx+2:]
		}
	}
	return strings.TrimSpace(value)
}

func isNumberedListLine(line string) bool {
	if line == "" {
		return false
	}
	digits := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}
	return digits > 0 && digits < len(line) && line[digits] == '.'
}

func normalizeStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

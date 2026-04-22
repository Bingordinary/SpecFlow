package sharedsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/impactsync"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/sharedbinding"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

type Options struct {
	Modules                        []string
	SharedRefs                     []string
	SharedIDs                      []string
	StableLandingModule            string
	BoundModulesOnlySharedFileRefs []string
}

type Result struct {
	ScopedModules                  []string
	ScopedFlows                    []string
	ScopedProjects                 []string
	ScopedSharedRefs               []string
	ScopedSharedIDs                []string
	StableLandingModule            string
	BoundModulesOnlySharedFileRefs []string
	ModuleResults                  []ModuleResult
	FlowResults                    []ObjectResult
	ProjectResults                 []ObjectResult
	BoundModuleDrifts              []BoundModuleDrift
}

type ModuleResult = impactsync.ModuleResult
type ObjectResult = impactsync.ObjectResult

type BoundModuleDrift struct {
	SharedContractID      string
	FileRef               string
	VersionRef            string
	DeclaredModules       []string
	ActualModules         []string
	BoundModulesOnlyDelta bool
}

type moduleBinding struct {
	Status        statusfile.ModuleStatus
	SharedRefs    []string
	BindingIssues []string
}

type sharedFile struct {
	SharedContractID string
	Layer            string
	FileRef          string
	VersionRef       string
	BoundModules     []string
}

type ReconcileBoundModulesOptions struct {
	Modules    []string
	SharedRefs []string
	SharedIDs  []string
}

type ReconcileBoundModulesResult struct {
	ScopedModules    []string
	ScopedSharedRefs []string
	ScopedSharedIDs  []string
	TouchedFiles     []string
	UpdatedFiles     []string
	UnchangedFiles   []string
}

func SyncImpact(repoRoot string, options Options) (Result, error) {
	normalized := Options{
		Modules:                        normalizeStrings(options.Modules),
		SharedRefs:                     normalizeStrings(options.SharedRefs),
		SharedIDs:                      normalizeStrings(options.SharedIDs),
		StableLandingModule:            strings.TrimSpace(options.StableLandingModule),
		BoundModulesOnlySharedFileRefs: normalizeStrings(options.BoundModulesOnlySharedFileRefs),
	}
	if len(normalized.SharedRefs) == 0 && len(normalized.SharedIDs) == 0 {
		return Result{}, fmt.Errorf("at least one of shared refs or shared ids is required")
	}

	sharedFilesByRef, err := loadSharedFiles(repoRoot)
	if err != nil {
		return Result{}, err
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)
	sharedFilesByFileRef := buildSharedFilesByFileRef(sharedFilesByRef)

	moduleBindings, actualModulesByRef, _, unresolvedSharedRefs, err := loadModuleBindings(repoRoot)
	if err != nil {
		return Result{}, err
	}
	flowBindings, unresolvedFlowRefs, err := loadObjectBindings(repoRoot, "flow")
	if err != nil {
		return Result{}, err
	}
	projectBindings, unresolvedProjectRefs, err := loadObjectBindings(repoRoot, "project")
	if err != nil {
		return Result{}, err
	}
	allUnresolvedRefs := normalizeStrings(append(append(unresolvedSharedRefs, unresolvedFlowRefs...), unresolvedProjectRefs...))
	for _, sharedID := range normalized.SharedIDs {
		if len(allUnresolvedRefs) > 0 {
			return Result{}, fmt.Errorf(
				"cannot determine affected downstream objects safely for shared id %q because unresolved shared refs remain in downstream bindings: %s",
				sharedID,
				strings.Join(allUnresolvedRefs, ", "),
			)
		}
		if _, ok := sharedFilesByID[sharedID]; !ok {
			return Result{}, fmt.Errorf("shared id %q is not present under docs/specs/shared_contracts/", sharedID)
		}
	}
	for _, module := range normalized.Modules {
		if _, ok := moduleBindings[module]; !ok {
			return Result{}, fmt.Errorf("module %q is not registered in docs/specs/_status.md", module)
		}
	}
	if normalized.StableLandingModule != "" {
		if _, ok := moduleBindings[normalized.StableLandingModule]; !ok {
			return Result{}, fmt.Errorf("stable landing module %q is not registered in docs/specs/_status.md", normalized.StableLandingModule)
		}
	}

	boundModulesOnlyFileRefs := map[string]bool{}
	for _, fileRef := range normalized.BoundModulesOnlySharedFileRefs {
		shared, ok := sharedFilesByFileRef[fileRef]
		if !ok {
			return Result{}, fmt.Errorf("bound_modules-only shared file ref %q is not present under docs/specs/shared_contracts/", fileRef)
		}
		boundModulesOnlyFileRefs[shared.FileRef] = true
	}

	drifts, err := collectBoundModuleDrifts(sharedFilesByRef, actualModulesByRef, boundModulesOnlyFileRefs)
	if err != nil {
		return Result{}, err
	}

	scopeModules := buildScopeModules(moduleBindings, sharedFilesByRef, normalized)
	scopeFlows := buildScopeObjects(flowBindings, sharedFilesByRef, normalized)
	scopeProjects := buildScopeObjects(projectBindings, sharedFilesByRef, normalized)

	impactResult, err := impactsync.Apply(repoRoot, impactsync.Input{
		Modules:                  scopedModulesForImpact(scopeModules, moduleBindings, normalized, sharedFilesByRef),
		Flows:                    scopedObjectsForImpact("flow", scopeFlows, flowBindings, normalized, sharedFilesByRef),
		Projects:                 scopedObjectsForImpact("project", scopeProjects, projectBindings, normalized, sharedFilesByRef),
		SharedFilesByRef:         impactSharedFiles(sharedFilesByRef),
		BoundModulesOnlyFileRefs: normalized.BoundModulesOnlySharedFileRefs,
		StableLandingModule:      normalized.StableLandingModule,
	})
	if err != nil {
		return Result{}, err
	}

	return Result{
		ScopedModules:                  scopeModules,
		ScopedFlows:                    scopeFlows,
		ScopedProjects:                 scopeProjects,
		ScopedSharedRefs:               normalized.SharedRefs,
		ScopedSharedIDs:                normalized.SharedIDs,
		StableLandingModule:            normalized.StableLandingModule,
		BoundModulesOnlySharedFileRefs: normalized.BoundModulesOnlySharedFileRefs,
		ModuleResults:                  impactResult.ModuleResults,
		FlowResults:                    impactResult.FlowResults,
		ProjectResults:                 impactResult.ProjectResults,
		BoundModuleDrifts:              drifts,
	}, nil
}

func ReconcileBoundModules(repoRoot string, options ReconcileBoundModulesOptions) (ReconcileBoundModulesResult, error) {
	normalized := ReconcileBoundModulesOptions{
		Modules:    normalizeStrings(options.Modules),
		SharedRefs: normalizeStrings(options.SharedRefs),
		SharedIDs:  normalizeStrings(options.SharedIDs),
	}
	if len(normalized.Modules) == 0 && len(normalized.SharedRefs) == 0 && len(normalized.SharedIDs) == 0 {
		return ReconcileBoundModulesResult{}, fmt.Errorf("at least one of modules, shared refs, or shared ids is required")
	}

	sharedFilesByRef, err := loadSharedFiles(repoRoot)
	if err != nil {
		return ReconcileBoundModulesResult{}, err
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)
	moduleBindings, actualModulesByRef, _, _, err := loadModuleBindings(repoRoot)
	if err != nil {
		return ReconcileBoundModulesResult{}, err
	}
	for _, module := range normalized.Modules {
		if _, ok := moduleBindings[module]; !ok {
			return ReconcileBoundModulesResult{}, fmt.Errorf("module %q is not registered in docs/specs/_status.md", module)
		}
	}
	for _, ref := range normalized.SharedRefs {
		if _, ok := sharedFilesByRef[ref]; !ok {
			return ReconcileBoundModulesResult{}, fmt.Errorf("shared ref %q is not present under docs/specs/shared_contracts/", ref)
		}
	}
	for _, sharedID := range normalized.SharedIDs {
		if _, ok := sharedFilesByID[sharedID]; !ok {
			return ReconcileBoundModulesResult{}, fmt.Errorf("shared id %q is not present under docs/specs/shared_contracts/", sharedID)
		}
	}

	touchedFiles := buildScopeSharedFiles(moduleBindings, sharedFilesByRef, sharedFilesByID, normalized)
	result := ReconcileBoundModulesResult{
		ScopedModules:    normalized.Modules,
		ScopedSharedRefs: normalized.SharedRefs,
		ScopedSharedIDs:  normalized.SharedIDs,
		TouchedFiles:     touchedFiles,
	}
	sharedFilesByFileRef := buildSharedFilesByFileRef(sharedFilesByRef)
	for _, fileRef := range touchedFiles {
		shared := sharedFilesByFileRef[fileRef]
		actualModules := normalizeStrings(actualModulesByRef[shared.VersionRef])
		if sameStringSlice(shared.BoundModules, actualModules) {
			result.UnchangedFiles = append(result.UnchangedFiles, shared.FileRef)
			continue
		}
		if err := rewriteSharedBoundModules(repoRoot, shared.FileRef, actualModules); err != nil {
			return ReconcileBoundModulesResult{}, err
		}
		result.UpdatedFiles = append(result.UpdatedFiles, shared.FileRef)
	}
	result.UpdatedFiles = normalizeStrings(result.UpdatedFiles)
	result.UnchangedFiles = normalizeStrings(result.UnchangedFiles)
	return result, nil
}

func collectBoundModuleDrifts(sharedFilesByRef map[string]sharedFile, actualModulesByRef map[string][]string, boundModulesOnlyFileRefs map[string]bool) ([]BoundModuleDrift, error) {
	refs := make([]string, 0, len(sharedFilesByRef))
	for ref := range sharedFilesByRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	drifts := []BoundModuleDrift{}
	for _, ref := range refs {
		shared := sharedFilesByRef[ref]
		actual := normalizeStrings(actualModulesByRef[ref])
		declared := normalizeStrings(shared.BoundModules)
		if sameStringSlice(actual, declared) {
			continue
		}
		drifts = append(drifts, BoundModuleDrift{
			SharedContractID:      shared.SharedContractID,
			FileRef:               shared.FileRef,
			VersionRef:            shared.VersionRef,
			DeclaredModules:       declared,
			ActualModules:         actual,
			BoundModulesOnlyDelta: boundModulesOnlyFileRefs[shared.FileRef],
		})
	}
	return drifts, nil
}

func loadSharedFiles(repoRoot string) (map[string]sharedFile, error) {
	result := map[string]sharedFile{}
	for _, root := range []struct {
		layer string
		dir   string
	}{
		{layer: "candidate", dir: "docs/specs/shared_contracts/candidate"},
		{layer: "stable", dir: "docs/specs/shared_contracts/stable"},
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
		result[shared.SharedContractID] = append(result[shared.SharedContractID], shared)
	}
	for sharedID := range result {
		sort.Slice(result[sharedID], func(i, j int) bool {
			return result[sharedID][i].VersionRef < result[sharedID][j].VersionRef
		})
	}
	return result
}

func buildSharedFilesByFileRef(sharedFilesByRef map[string]sharedFile) map[string]sharedFile {
	result := make(map[string]sharedFile, len(sharedFilesByRef))
	for _, shared := range sharedFilesByRef {
		result[shared.FileRef] = shared
	}
	return result
}

func loadModuleBindings(repoRoot string) (map[string]moduleBinding, map[string][]string, map[string][]string, []string, error) {
	statuses, err := statusfile.LoadModuleStatuses(repoRoot)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	bindings := make(map[string]moduleBinding, len(statuses))
	actualByRef := map[string][]string{}
	actualByID := map[string][]string{}
	unresolvedRefs := []string{}
	for _, status := range statuses {
		refs, err := readModuleSharedRefs(repoRoot, status)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		bindingIssues := []string{}
		bindings[status.Module] = moduleBinding{
			Status:     status,
			SharedRefs: refs,
		}
		for _, ref := range refs {
			resolved, err := sharedbinding.ResolveRef(repoRoot, status.ActiveLayer, ref)
			if err != nil {
				bindingIssues = append(bindingIssues, err.Error())
				unresolvedRefs = append(unresolvedRefs, ref)
				continue
			}
			actualByRef[resolved.VersionRef] = append(actualByRef[resolved.VersionRef], status.Module)
			actualByID[resolved.SharedContractID] = append(actualByID[resolved.SharedContractID], status.Module)
		}
		bindings[status.Module] = moduleBinding{
			Status:        status,
			SharedRefs:    refs,
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

func buildScopeModules(moduleBindings map[string]moduleBinding, sharedFilesByRef map[string]sharedFile, options Options) []string {
	affected := map[string]bool{}
	for module, binding := range moduleBindings {
		if len(selectedSharedRefsForObject(binding.SharedRefs, options.SharedRefs, options.SharedIDs, sharedFilesByRef)) > 0 {
			affected[module] = true
		}
	}
	if len(options.Modules) == 0 {
		return sortedKeys(affected)
	}

	scope := map[string]bool{}
	for _, module := range options.Modules {
		if affected[module] {
			scope[module] = true
		}
	}
	return sortedKeys(scope)
}

func scopedModulesForImpact(scopeModules []string, moduleBindings map[string]moduleBinding, options Options, sharedFilesByRef map[string]sharedFile) []impactsync.ScopedModule {
	result := make([]impactsync.ScopedModule, 0, len(scopeModules))
	for _, module := range scopeModules {
		binding := moduleBindings[module]
		result = append(result, impactsync.ScopedModule{
			Binding: impactsync.ModuleBinding{
				Module:        binding.Status.Module,
				ActiveLayer:   binding.Status.ActiveLayer,
				NextCommand:   binding.Status.NextCommand,
				SharedRefs:    append([]string{}, binding.SharedRefs...),
				BindingIssues: append([]string{}, binding.BindingIssues...),
			},
			RelevantSelectedRefs: selectedSharedRefsForObject(binding.SharedRefs, options.SharedRefs, options.SharedIDs, sharedFilesByRef),
			ExplicitlyScoped:     contains(options.Modules, module),
		})
	}
	return result
}

func scopedObjectsForImpact(objectType string, scopeObjects []string, bindings map[string]objectBinding, options Options, sharedFilesByRef map[string]sharedFile) []impactsync.ScopedObject {
	result := make([]impactsync.ScopedObject, 0, len(scopeObjects))
	for _, object := range scopeObjects {
		binding := bindings[object]
		result = append(result, impactsync.ScopedObject{
			Binding: impactsync.ObjectBinding{
				ObjectType:    objectType,
				Object:        binding.Status.Object,
				ActiveLayer:   binding.Status.ActiveLayer,
				NextCommand:   binding.Status.NextCommand,
				SharedRefs:    append([]string{}, binding.SharedRefs...),
				BindingIssues: append([]string{}, binding.BindingIssues...),
			},
			RelevantSelectedRefs: selectedSharedRefsForObject(binding.SharedRefs, options.SharedRefs, options.SharedIDs, sharedFilesByRef),
		})
	}
	return result
}

func impactSharedFiles(sharedFilesByRef map[string]sharedFile) map[string]impactsync.SharedFile {
	result := make(map[string]impactsync.SharedFile, len(sharedFilesByRef))
	for ref, shared := range sharedFilesByRef {
		result[ref] = impactsync.SharedFile{
			SharedContractID: shared.SharedContractID,
			Layer:            shared.Layer,
			FileRef:          shared.FileRef,
			VersionRef:       shared.VersionRef,
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

func buildScopeSharedFiles(moduleBindings map[string]moduleBinding, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile, options ReconcileBoundModulesOptions) []string {
	scope := map[string]bool{}
	for _, module := range options.Modules {
		for _, ref := range moduleBindings[module].SharedRefs {
			if shared, ok := sharedFilesByRef[ref]; ok {
				scope[shared.FileRef] = true
			}
		}
	}
	for _, ref := range options.SharedRefs {
		if shared, ok := sharedFilesByRef[ref]; ok {
			scope[shared.FileRef] = true
		}
	}
	for _, sharedID := range options.SharedIDs {
		for _, shared := range sharedFilesByID[sharedID] {
			scope[shared.FileRef] = true
		}
	}
	result := make([]string, 0, len(scope))
	for fileRef := range scope {
		result = append(result, fileRef)
	}
	sort.Strings(result)
	return result
}

func selectedSharedRefsForObject(objectRefs, scopedRefs, scopedIDs []string, sharedFilesByRef map[string]sharedFile) []string {
	refSet := map[string]bool{}
	for _, ref := range scopedRefs {
		refSet[ref] = true
	}
	idSet := map[string]bool{}
	for _, sharedID := range scopedIDs {
		idSet[sharedID] = true
	}

	result := []string{}
	for _, ref := range objectRefs {
		if refSet[ref] {
			result = append(result, ref)
			continue
		}
		shared, ok := sharedFilesByRef[ref]
		if ok && idSet[shared.SharedContractID] {
			result = append(result, ref)
		}
	}
	return normalizeStrings(result)
}

func parseSharedFile(relPath, content string) (sharedFile, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return sharedFile{}, fmt.Errorf("%s: missing frontmatter start marker", relPath)
	}
	endIdx := -1
	for idx := 1; idx < len(lines); idx++ {
		if strings.TrimSpace(lines[idx]) == "---" {
			endIdx = idx
			break
		}
	}
	if endIdx == -1 {
		return sharedFile{}, fmt.Errorf("%s: missing frontmatter end marker", relPath)
	}

	shared := sharedFile{FileRef: relPath}
	inBoundModules := false
	for _, line := range lines[1:endIdx] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "bound_modules:") {
			inBoundModules = true
			if strings.HasSuffix(trimmed, "none") {
				inBoundModules = false
			}
			continue
		}
		if inBoundModules {
			if strings.HasPrefix(trimmed, "- ") {
				module := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				module = strings.Trim(module, "`")
				if module != "" {
					shared.BoundModules = append(shared.BoundModules, module)
				}
				continue
			}
			inBoundModules = false
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "shared_contract_id":
			shared.SharedContractID = value
		case "layer":
			shared.Layer = value
		case "shared_version":
			shared.VersionRef = fmt.Sprintf("%s@%s", strings.TrimSuffix(filepath.Base(relPath), ".md"), value)
		}
	}
	if shared.SharedContractID == "" || shared.Layer == "" || shared.VersionRef == "" {
		return sharedFile{}, fmt.Errorf("%s: missing shared_contract_id, layer, or shared_version", relPath)
	}
	shared.BoundModules = normalizeStrings(shared.BoundModules)
	return shared, nil
}

func rewriteSharedBoundModules(repoRoot, fileRef string, modules []string) error {
	path := filepath.Join(repoRoot, filepath.FromSlash(fileRef))
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", fileRef, err)
	}
	content := strings.ReplaceAll(string(contentBytes), "\r\n", "\n")
	hadTrailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fmt.Errorf("%s: missing frontmatter start marker", fileRef)
	}
	endIdx := -1
	for idx := 1; idx < len(lines); idx++ {
		if strings.TrimSpace(lines[idx]) == "---" {
			endIdx = idx
			break
		}
	}
	if endIdx == -1 {
		return fmt.Errorf("%s: missing frontmatter end marker", fileRef)
	}

	boundLines := []string{"bound_modules: none"}
	if len(modules) > 0 {
		boundLines = []string{"bound_modules:"}
		for _, module := range modules {
			boundLines = append(boundLines, fmt.Sprintf("  - %s", module))
		}
	}

	frontmatter := []string{"---"}
	inserted := false
	skipping := false
	for _, line := range lines[1:endIdx] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "bound_modules:") {
			frontmatter = append(frontmatter, boundLines...)
			inserted = true
			skipping = true
			continue
		}
		if skipping {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || trimmed == "" {
				continue
			}
			skipping = false
		}
		frontmatter = append(frontmatter, line)
	}
	if !inserted {
		frontmatter = append(frontmatter, boundLines...)
	}
	frontmatter = append(frontmatter, "---")
	rewritten := strings.Join(append(frontmatter, lines[endIdx+1:]...), "\n")
	if hadTrailingNewline {
		rewritten += "\n"
	}
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", fileRef, err)
	}
	return nil
}

func readModuleSharedRefs(repoRoot string, status statusfile.ModuleStatus) ([]string, error) {
	return readObjectSharedRefs(repoRoot, statusfile.ObjectStatus{
		ObjectType:  "module",
		Object:      status.Module,
		ActiveLayer: status.ActiveLayer,
	})
}

func parseFrontmatter(content string) (map[string]string, string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("missing frontmatter start marker")
	}
	endIdx := -1
	for idx := 1; idx < len(lines); idx++ {
		if strings.TrimSpace(lines[idx]) == "---" {
			endIdx = idx
			break
		}
	}
	if endIdx == -1 {
		return nil, "", fmt.Errorf("missing frontmatter end marker")
	}

	values := map[string]string{}
	for _, line := range lines[1:endIdx] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return values, strings.Join(lines[endIdx+1:], "\n"), nil
}

func parseSharedContractRefs(body string) ([]string, bool, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		right, matched, err := parseNamedFieldLine(trimmed, "shared_contract_refs")
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
			return nil, false, fmt.Errorf("shared_contract_refs must use literal none or a markdown list")
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
				return nil, false, fmt.Errorf("shared_contract_refs must be a markdown list of shared refs")
			}
			ref := strings.TrimSpace(strings.TrimPrefix(nextTrimmed, "- "))
			ref = strings.Trim(ref, "`")
			if ref == "" {
				return nil, false, fmt.Errorf("shared_contract_refs contains an empty item")
			}
			if seen[ref] {
				return nil, false, fmt.Errorf("shared_contract_refs contains duplicate item %q", ref)
			}
			seen[ref] = true
			refs = append(refs, ref)
		}
		if len(refs) == 0 {
			return nil, false, fmt.Errorf("shared_contract_refs must not be an empty list")
		}
		return refs, true, nil
	}
	return nil, false, nil
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

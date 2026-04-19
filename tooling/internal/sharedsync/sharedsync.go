package sharedsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/snapshot"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

type Options struct {
	Modules                        []string
	SharedRefs                     []string
	SharedIDs                      []string
	PromotionOwnerModule           string
	BoundModulesOnlySharedFileRefs []string
}

type Result struct {
	ScopedModules                  []string
	ScopedSharedRefs               []string
	ScopedSharedIDs                []string
	PromotionOwnerModule           string
	BoundModulesOnlySharedFileRefs []string
	ModuleResults                  []ModuleResult
	BoundModuleDrifts              []BoundModuleDrift
}

type ModuleResult struct {
	Module             string
	ActiveLayer        string
	Outcome            string
	FallbackReasonCode string
	NextCommand        string
	DeletedFiles       []string
	MissingFiles       []string
	StatusUpdated      bool
	Diagnostics        []string
}

type BoundModuleDrift struct {
	SharedContractID      string
	FileRef               string
	VersionRef            string
	DeclaredModules       []string
	ActualModules         []string
	BoundModulesOnlyDelta bool
}

type moduleBinding struct {
	Status     statusfile.ModuleStatus
	SharedRefs []string
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
		PromotionOwnerModule:           strings.TrimSpace(options.PromotionOwnerModule),
		BoundModulesOnlySharedFileRefs: normalizeStrings(options.BoundModulesOnlySharedFileRefs),
	}

	sharedFilesByRef, err := loadSharedFiles(repoRoot)
	if err != nil {
		return Result{}, err
	}
	sharedFilesByID := buildSharedFilesByID(sharedFilesByRef)
	sharedFilesByFileRef := buildSharedFilesByFileRef(sharedFilesByRef)

	moduleBindings, actualModulesByRef, actualModulesByID, unresolvedSharedRefs, err := loadModuleBindings(repoRoot, sharedFilesByRef)
	if err != nil {
		return Result{}, err
	}
	for _, sharedID := range normalized.SharedIDs {
		if len(unresolvedSharedRefs) > 0 {
			return Result{}, fmt.Errorf(
				"cannot determine affected modules safely for shared id %q because unresolved shared refs remain in module bindings: %s",
				sharedID,
				strings.Join(unresolvedSharedRefs, ", "),
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
	if normalized.PromotionOwnerModule != "" {
		if _, ok := moduleBindings[normalized.PromotionOwnerModule]; !ok {
			return Result{}, fmt.Errorf("promotion owner module %q is not registered in docs/specs/_status.md", normalized.PromotionOwnerModule)
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

	scopeModules := buildScopeModules(moduleBindings, actualModulesByRef, actualModulesByID, normalized)
	results := make([]ModuleResult, 0, len(scopeModules))
	for _, module := range scopeModules {
		binding := moduleBindings[module]
		moduleResult, err := reconcileModule(repoRoot, binding, normalized, sharedFilesByRef, boundModulesOnlyFileRefs)
		if err != nil {
			return Result{}, err
		}
		results = append(results, moduleResult)
	}

	return Result{
		ScopedModules:                  scopeModules,
		ScopedSharedRefs:               normalized.SharedRefs,
		ScopedSharedIDs:                normalized.SharedIDs,
		PromotionOwnerModule:           normalized.PromotionOwnerModule,
		BoundModulesOnlySharedFileRefs: normalized.BoundModulesOnlySharedFileRefs,
		ModuleResults:                  results,
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
	moduleBindings, actualModulesByRef, _, _, err := loadModuleBindings(repoRoot, sharedFilesByRef)
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

func reconcileModule(repoRoot string, binding moduleBinding, options Options, sharedFilesByRef map[string]sharedFile, boundModulesOnlyFileRefs map[string]bool) (ModuleResult, error) {
	result := ModuleResult{
		Module:      binding.Status.Module,
		ActiveLayer: binding.Status.ActiveLayer,
		Outcome:     "unchanged",
		NextCommand: binding.Status.NextCommand,
	}

	moduleExplicit := contains(options.Modules, binding.Status.Module)
	relevantSelectedRefs := selectedSharedRefsForModule(binding.SharedRefs, options.SharedRefs, options.SharedIDs, sharedFilesByRef)

	bindingDiagnostics := []string{}
	bindingIssue := false
	for _, ref := range binding.SharedRefs {
		shared, ok := sharedFilesByRef[ref]
		if !ok {
			bindingIssue = true
			bindingDiagnostics = append(bindingDiagnostics, fmt.Sprintf("missing shared file for ref %s", ref))
			continue
		}
		expectedPrefix := "s_"
		if shared.Layer == "candidate" {
			expectedPrefix = "c_"
		}
		if !strings.HasPrefix(filepath.Base(shared.FileRef), expectedPrefix) {
			bindingIssue = true
			bindingDiagnostics = append(bindingDiagnostics, fmt.Sprintf("shared file %s has layer %s but file prefix does not match", shared.FileRef, shared.Layer))
		}
	}
	result.Diagnostics = append(result.Diagnostics, bindingDiagnostics...)

	switch binding.Status.ActiveLayer {
	case "candidate":
		return reconcileCandidate(repoRoot, binding, result, relevantSelectedRefs, moduleExplicit, bindingIssue, sharedFilesByRef, boundModulesOnlyFileRefs)
	case "stable":
		return reconcileStable(repoRoot, binding, result, relevantSelectedRefs, moduleExplicit, bindingIssue, sharedFilesByRef, boundModulesOnlyFileRefs, options.PromotionOwnerModule)
	default:
		return ModuleResult{}, fmt.Errorf("unsupported active layer %q for module %s", binding.Status.ActiveLayer, binding.Status.Module)
	}
}

func reconcileCandidate(repoRoot string, binding moduleBinding, result ModuleResult, relevantSelectedRefs []string, moduleExplicit, bindingIssue bool, sharedFilesByRef map[string]sharedFile, boundModulesOnlyFileRefs map[string]bool) (ModuleResult, error) {
	fallbackReason := ""
	if bindingIssue {
		fallbackReason = "binding_drift"
	}

	expectedSnapshot, err := snapshot.RebuildCurrent(repoRoot, binding.Status.Module)
	if err != nil {
		if fallbackReason != "" {
			return applyCandidateFallback(repoRoot, result, fallbackReason)
		}
		return ModuleResult{}, err
	}

	processFound := false
	sharedMismatch := false
	nonSharedMismatch := false
	for _, processKind := range []string{"check", "plan", "verify"} {
		processPath, err := snapshot.ProcessFilePath(binding.Status.Module, processKind)
		if err != nil {
			return ModuleResult{}, err
		}
		processAbs := filepath.Join(repoRoot, filepath.FromSlash(processPath))
		if _, err := os.Stat(processAbs); err != nil {
			if os.IsNotExist(err) {
				result.MissingFiles = append(result.MissingFiles, processPath)
				continue
			}
			return ModuleResult{}, fmt.Errorf("stat %s: %w", processPath, err)
		}
		processFound = true

		validation, err := snapshot.ValidateProcessFile(repoRoot, binding.Status.Module, processKind)
		if err != nil {
			return ModuleResult{}, err
		}
		if validation.Valid {
			continue
		}

		processSnapshot, err := snapshot.LoadProcessSnapshot(repoRoot, binding.Status.Module, processKind)
		if err != nil {
			return ModuleResult{}, err
		}
		if hasNonSharedMismatch(validation.Mismatches) {
			nonSharedMismatch = true
			result.Diagnostics = append(result.Diagnostics, prefixItems(validation.Mismatches, processKind)...)
			continue
		}

		equivalent, err := sharedSnapshotsEquivalentIgnoringBoundModules(processSnapshot.SharedContractSnapshot, expectedSnapshot.SharedContractSnapshot, boundModulesOnlyFileRefs)
		if err != nil {
			return ModuleResult{}, err
		}
		if equivalent {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s snapshot differs only on bound_modules metadata", processKind))
			continue
		}
		sharedMismatch = true
		result.Diagnostics = append(result.Diagnostics, prefixItems(validation.Mismatches, processKind)...)
	}

	switch {
	case fallbackReason != "":
	case nonSharedMismatch:
		fallbackReason = "truth_drift"
	case sharedMismatch:
		fallbackReason = "shared_contract_drift"
	case !processFound && len(relevantSelectedRefs) > 0 && hasNonBoundModulesSelectedChange(relevantSelectedRefs, sharedFilesByRef, boundModulesOnlyFileRefs):
		fallbackReason = "shared_contract_drift"
	case !processFound && moduleExplicit && binding.Status.NextCommand != "cand_check":
		fallbackReason = "binding_drift"
	}

	if fallbackReason == "" {
		return result, nil
	}
	return applyCandidateFallback(repoRoot, result, fallbackReason)
}

func reconcileStable(repoRoot string, binding moduleBinding, result ModuleResult, relevantSelectedRefs []string, moduleExplicit, bindingIssue bool, sharedFilesByRef map[string]sharedFile, boundModulesOnlyFileRefs map[string]bool, promotionOwnerModule string) (ModuleResult, error) {
	fallbackReason := ""
	isPromotionOwner := promotionOwnerModule != "" && binding.Status.Module == promotionOwnerModule
	switch {
	case bindingIssue:
		fallbackReason = "binding_drift"
	case moduleExplicit && !isPromotionOwner:
		fallbackReason = "binding_drift"
	case len(relevantSelectedRefs) > 0 && !isPromotionOwner && hasNonBoundModulesSelectedChange(relevantSelectedRefs, sharedFilesByRef, boundModulesOnlyFileRefs):
		fallbackReason = "shared_contract_drift"
	}

	if fallbackReason == "" {
		return result, nil
	}
	result.FallbackReasonCode = fallbackReason
	result.Outcome = "rerouted"
	result.NextCommand = "stable_verify"
	updated, err := statusfile.UpdateNextCommand(repoRoot, binding.Status.Module, result.NextCommand)
	if err != nil {
		return ModuleResult{}, err
	}
	result.StatusUpdated = updated
	return result, nil
}

func applyCandidateFallback(repoRoot string, result ModuleResult, fallbackReason string) (ModuleResult, error) {
	result.FallbackReasonCode = fallbackReason
	result.Outcome = "invalidated"
	result.NextCommand = "cand_check"
	for _, processKind := range []string{"check", "plan", "verify"} {
		processPath, err := snapshot.ProcessFilePath(result.Module, processKind)
		if err != nil {
			return ModuleResult{}, err
		}
		processAbs := filepath.Join(repoRoot, filepath.FromSlash(processPath))
		if _, err := os.Stat(processAbs); err != nil {
			if os.IsNotExist(err) {
				if !contains(result.MissingFiles, processPath) {
					result.MissingFiles = append(result.MissingFiles, processPath)
				}
				continue
			}
			return ModuleResult{}, fmt.Errorf("stat %s: %w", processPath, err)
		}
		if err := os.Remove(processAbs); err != nil {
			return ModuleResult{}, fmt.Errorf("delete %s: %w", processPath, err)
		}
		result.DeletedFiles = append(result.DeletedFiles, processPath)
	}
	updated, err := statusfile.UpdateNextCommand(repoRoot, result.Module, result.NextCommand)
	if err != nil {
		return ModuleResult{}, err
	}
	result.StatusUpdated = updated
	result.DeletedFiles = normalizeStrings(result.DeletedFiles)
	result.MissingFiles = normalizeStrings(result.MissingFiles)
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

func loadModuleBindings(repoRoot string, sharedFilesByRef map[string]sharedFile) (map[string]moduleBinding, map[string][]string, map[string][]string, []string, error) {
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
		bindings[status.Module] = moduleBinding{
			Status:     status,
			SharedRefs: refs,
		}
		for _, ref := range refs {
			actualByRef[ref] = append(actualByRef[ref], status.Module)
			if shared, ok := sharedFilesByRef[ref]; ok {
				actualByID[shared.SharedContractID] = append(actualByID[shared.SharedContractID], status.Module)
				continue
			}
			unresolvedRefs = append(unresolvedRefs, ref)
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

func buildScopeModules(moduleBindings map[string]moduleBinding, actualModulesByRef map[string][]string, actualModulesByID map[string][]string, options Options) []string {
	scope := map[string]bool{}
	hasExplicitScope := len(options.Modules) > 0 || len(options.SharedRefs) > 0 || len(options.SharedIDs) > 0
	for _, module := range options.Modules {
		scope[module] = true
	}
	for _, ref := range options.SharedRefs {
		for _, module := range actualModulesByRef[ref] {
			scope[module] = true
		}
	}
	for _, sharedID := range options.SharedIDs {
		for _, module := range actualModulesByID[sharedID] {
			scope[module] = true
		}
	}
	if hasExplicitScope && len(scope) == 0 {
		return nil
	}
	if len(scope) == 0 {
		for module, binding := range moduleBindings {
			if binding.Status.ActiveLayer == "candidate" && len(binding.SharedRefs) > 0 {
				scope[module] = true
			}
		}
	}
	result := make([]string, 0, len(scope))
	for module := range scope {
		result = append(result, module)
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

func selectedSharedRefsForModule(moduleRefs, scopedRefs, scopedIDs []string, sharedFilesByRef map[string]sharedFile) []string {
	refSet := map[string]bool{}
	for _, ref := range scopedRefs {
		refSet[ref] = true
	}
	idSet := map[string]bool{}
	for _, sharedID := range scopedIDs {
		idSet[sharedID] = true
	}

	result := []string{}
	for _, ref := range moduleRefs {
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

func hasNonSharedMismatch(mismatches []string) bool {
	for _, mismatch := range mismatches {
		if !strings.HasPrefix(mismatch, "shared_contract_snapshot mismatch") {
			return true
		}
	}
	return false
}

func sharedSnapshotsEquivalentIgnoringBoundModules(actual, expected []snapshot.SharedContractEntry, boundModulesOnlyFileRefs map[string]bool) (bool, error) {
	actual = normalizeSharedEntries(actual)
	expected = normalizeSharedEntries(expected)
	if len(actual) != len(expected) {
		return false, nil
	}
	for idx := range actual {
		if actual[idx].SharedContractID != expected[idx].SharedContractID ||
			actual[idx].Layer != expected[idx].Layer ||
			actual[idx].FileRef != expected[idx].FileRef ||
			actual[idx].VersionRef != expected[idx].VersionRef {
			return false, nil
		}
		if actual[idx].Fingerprint == expected[idx].Fingerprint {
			continue
		}
		if !boundModulesOnlyFileRefs[expected[idx].FileRef] {
			return false, nil
		}
	}
	return true, nil
}

func hasNonBoundModulesSelectedChange(refs []string, sharedFilesByRef map[string]sharedFile, boundModulesOnlyFileRefs map[string]bool) bool {
	for _, ref := range refs {
		shared, ok := sharedFilesByRef[ref]
		if !ok {
			return true
		}
		if !boundModulesOnlyFileRefs[shared.FileRef] {
			return true
		}
	}
	return false
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
	mainSpecRef, err := specpaths.MainSpecFileRef(status.ActiveLayer, status.Module)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", mainSpecRef, err)
	}
	_, body, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainSpecRef, err)
	}
	refs, _, err := parseSharedContractRefs(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainSpecRef, err)
	}
	return normalizeStrings(refs), nil
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
		if !strings.Contains(trimmed, "`shared_contract_refs`") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, false, fmt.Errorf("shared_contract_refs line is malformed")
		}
		right := strings.TrimSpace(parts[1])
		if right == "`none`" || right == "none" {
			return nil, true, nil
		}
		refs := []string{}
		for next := idx + 1; next < len(lines); next++ {
			nextTrimmed := strings.TrimSpace(lines[next])
			if strings.HasPrefix(nextTrimmed, "## ") || isNumberedListLine(nextTrimmed) {
				break
			}
			if strings.HasPrefix(nextTrimmed, "- ") {
				ref := strings.TrimSpace(strings.TrimPrefix(nextTrimmed, "- "))
				ref = strings.Trim(ref, "`")
				if ref != "" {
					refs = append(refs, ref)
				}
			}
		}
		return refs, true, nil
	}
	return nil, false, nil
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

func normalizeSharedEntries(values []snapshot.SharedContractEntry) []snapshot.SharedContractEntry {
	result := append([]snapshot.SharedContractEntry(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].SharedContractID != result[j].SharedContractID {
			return result[i].SharedContractID < result[j].SharedContractID
		}
		if result[i].Layer != result[j].Layer {
			return result[i].Layer < result[j].Layer
		}
		return result[i].FileRef < result[j].FileRef
	})
	return result
}

func prefixItems(items []string, prefix string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, fmt.Sprintf("%s: %s", prefix, item))
	}
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

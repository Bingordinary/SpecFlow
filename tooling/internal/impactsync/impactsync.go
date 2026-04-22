package impactsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/snapshot"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

type SharedFile struct {
	SharedContractID string
	Layer            string
	FileRef          string
	VersionRef       string
}

type ModuleBinding struct {
	Module        string
	ActiveLayer   string
	NextCommand   string
	SharedRefs    []string
	BindingIssues []string
}

type ScopedModule struct {
	Binding              ModuleBinding
	RelevantSelectedRefs []string
	ExplicitlyScoped     bool
}

type ObjectBinding struct {
	ObjectType    string
	Object        string
	ActiveLayer   string
	NextCommand   string
	SharedRefs    []string
	BindingIssues []string
}

type ScopedObject struct {
	Binding              ObjectBinding
	RelevantSelectedRefs []string
}

type Input struct {
	Modules                  []ScopedModule
	Flows                    []ScopedObject
	Projects                 []ScopedObject
	SharedFilesByRef         map[string]SharedFile
	BoundModulesOnlyFileRefs []string
	StableLandingModule      string
}

type Result struct {
	ModuleResults  []ModuleResult
	FlowResults    []ObjectResult
	ProjectResults []ObjectResult
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

type ObjectResult struct {
	Object             string
	ActiveLayer        string
	Outcome            string
	FallbackReasonCode string
	NextCommand        string
	DeletedFiles       []string
	MissingFiles       []string
	StatusUpdated      bool
	Diagnostics        []string
}

type objectFamilyConfig struct {
	CandidateNextCommand  string
	StableNextCommand     string
	CandidateProcessKinds []string
}

func Apply(repoRoot string, input Input) (Result, error) {
	boundModulesOnlyFileRefs := make(map[string]bool, len(input.BoundModulesOnlyFileRefs))
	for _, fileRef := range input.BoundModulesOnlyFileRefs {
		if strings.TrimSpace(fileRef) != "" {
			boundModulesOnlyFileRefs[strings.TrimSpace(fileRef)] = true
		}
	}

	moduleResults := make([]ModuleResult, 0, len(input.Modules))
	for _, scoped := range input.Modules {
		result, err := reconcileModule(repoRoot, scoped, input.SharedFilesByRef, boundModulesOnlyFileRefs, strings.TrimSpace(input.StableLandingModule))
		if err != nil {
			return Result{}, err
		}
		moduleResults = append(moduleResults, result)
	}

	flowResults := make([]ObjectResult, 0, len(input.Flows))
	for _, scoped := range input.Flows {
		result, err := reconcileObject(repoRoot, scoped, input.SharedFilesByRef, boundModulesOnlyFileRefs)
		if err != nil {
			return Result{}, err
		}
		flowResults = append(flowResults, result)
	}

	projectResults := make([]ObjectResult, 0, len(input.Projects))
	for _, scoped := range input.Projects {
		result, err := reconcileObject(repoRoot, scoped, input.SharedFilesByRef, boundModulesOnlyFileRefs)
		if err != nil {
			return Result{}, err
		}
		projectResults = append(projectResults, result)
	}

	return Result{
		ModuleResults:  moduleResults,
		FlowResults:    flowResults,
		ProjectResults: projectResults,
	}, nil
}

func reconcileModule(repoRoot string, scoped ScopedModule, sharedFilesByRef map[string]SharedFile, boundModulesOnlyFileRefs map[string]bool, stableLandingModule string) (ModuleResult, error) {
	binding := scoped.Binding
	result := ModuleResult{
		Module:      binding.Module,
		ActiveLayer: binding.ActiveLayer,
		Outcome:     "unchanged",
		NextCommand: binding.NextCommand,
		Diagnostics: append([]string{}, binding.BindingIssues...),
	}

	bindingIssue := len(binding.BindingIssues) > 0

	switch binding.ActiveLayer {
	case "candidate":
		return reconcileCandidate(repoRoot, binding, result, scoped.RelevantSelectedRefs, scoped.ExplicitlyScoped, bindingIssue, sharedFilesByRef, boundModulesOnlyFileRefs)
	case "stable":
		return reconcileStable(repoRoot, binding, result, scoped.RelevantSelectedRefs, scoped.ExplicitlyScoped, bindingIssue, sharedFilesByRef, boundModulesOnlyFileRefs, stableLandingModule)
	default:
		return ModuleResult{}, fmt.Errorf("unsupported active layer %q for module %s", binding.ActiveLayer, binding.Module)
	}
}

func reconcileCandidate(repoRoot string, binding ModuleBinding, result ModuleResult, relevantSelectedRefs []string, moduleExplicit, bindingIssue bool, sharedFilesByRef map[string]SharedFile, boundModulesOnlyFileRefs map[string]bool) (ModuleResult, error) {
	fallbackReason := ""
	if bindingIssue {
		fallbackReason = "binding_drift"
	}

	expectedSnapshot, err := snapshot.RebuildCurrent(repoRoot, binding.Module)
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
		processPath, err := snapshot.ProcessFilePath(binding.Module, processKind)
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

		validation, err := snapshot.ValidateProcessFile(repoRoot, binding.Module, processKind)
		if err != nil {
			return ModuleResult{}, err
		}
		if validation.Valid {
			continue
		}

		processSnapshot, err := snapshot.LoadProcessSnapshot(repoRoot, binding.Module, processKind)
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
	case !processFound && moduleExplicit && binding.NextCommand != "cand_check":
		fallbackReason = "binding_drift"
	}

	if fallbackReason == "" {
		return result, nil
	}
	return applyCandidateFallback(repoRoot, result, fallbackReason)
}

func reconcileStable(repoRoot string, binding ModuleBinding, result ModuleResult, relevantSelectedRefs []string, moduleExplicit, bindingIssue bool, sharedFilesByRef map[string]SharedFile, boundModulesOnlyFileRefs map[string]bool, stableLandingModule string) (ModuleResult, error) {
	fallbackReason := ""
	isStableLandingModule := stableLandingModule != "" && binding.Module == stableLandingModule
	switch {
	case bindingIssue:
		fallbackReason = "binding_drift"
	case moduleExplicit && len(relevantSelectedRefs) == 0 && !isStableLandingModule:
		fallbackReason = "binding_drift"
	case len(relevantSelectedRefs) > 0 && !isStableLandingModule && hasNonBoundModulesSelectedChange(relevantSelectedRefs, sharedFilesByRef, boundModulesOnlyFileRefs):
		fallbackReason = "shared_contract_drift"
	case moduleExplicit && len(relevantSelectedRefs) == 0:
		result.Diagnostics = append(result.Diagnostics, "explicit module scope alone does not prove stable drift")
	}

	if fallbackReason == "" {
		return result, nil
	}
	result.FallbackReasonCode = fallbackReason
	result.Outcome = "rerouted"
	result.NextCommand = "stable_verify"
	updated, err := statusfile.UpdateNextCommand(repoRoot, binding.Module, result.NextCommand)
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

func reconcileObject(repoRoot string, scoped ScopedObject, sharedFilesByRef map[string]SharedFile, boundModulesOnlyFileRefs map[string]bool) (ObjectResult, error) {
	binding := scoped.Binding
	result := ObjectResult{
		Object:      binding.Object,
		ActiveLayer: binding.ActiveLayer,
		Outcome:     "unchanged",
		NextCommand: binding.NextCommand,
		Diagnostics: append([]string{}, binding.BindingIssues...),
	}

	fallbackReason := ""
	switch {
	case len(binding.BindingIssues) > 0:
		fallbackReason = "binding_drift"
	case len(scoped.RelevantSelectedRefs) > 0 && hasNonBoundModulesSelectedChange(scoped.RelevantSelectedRefs, sharedFilesByRef, boundModulesOnlyFileRefs):
		fallbackReason = "shared_contract_drift"
	default:
		return result, nil
	}

	config, err := objectFamilyConfigFor(binding.ObjectType)
	if err != nil {
		return ObjectResult{}, err
	}
	if binding.ActiveLayer == "candidate" {
		return applyObjectCandidateFallback(repoRoot, result, binding.ObjectType, config, fallbackReason)
	}
	return applyObjectStableReroute(repoRoot, result, binding.ObjectType, config, fallbackReason)
}

func objectFamilyConfigFor(objectType string) (objectFamilyConfig, error) {
	switch objectType {
	case "flow":
		return objectFamilyConfig{
			CandidateNextCommand:  "flow_check",
			StableNextCommand:     "flow_stable_verify",
			CandidateProcessKinds: []string{"check", "verify"},
		}, nil
	case "project":
		return objectFamilyConfig{
			CandidateNextCommand:  "project_check",
			StableNextCommand:     "project_stable_verify",
			CandidateProcessKinds: []string{"check", "verify"},
		}, nil
	default:
		return objectFamilyConfig{}, fmt.Errorf("unsupported object type %q", objectType)
	}
}

func applyObjectCandidateFallback(repoRoot string, result ObjectResult, objectType string, config objectFamilyConfig, fallbackReason string) (ObjectResult, error) {
	result.FallbackReasonCode = fallbackReason
	result.Outcome = "invalidated"
	result.NextCommand = config.CandidateNextCommand

	for _, processPath := range objectProcessPaths(result.Object, config.CandidateProcessKinds) {
		processAbs := filepath.Join(repoRoot, filepath.FromSlash(processPath))
		if _, err := os.Stat(processAbs); err != nil {
			if os.IsNotExist(err) {
				result.MissingFiles = append(result.MissingFiles, processPath)
				continue
			}
			return ObjectResult{}, fmt.Errorf("stat %s: %w", processPath, err)
		}
		if err := os.Remove(processAbs); err != nil {
			return ObjectResult{}, fmt.Errorf("delete %s: %w", processPath, err)
		}
		result.DeletedFiles = append(result.DeletedFiles, processPath)
	}

	updated, err := statusfile.UpdateObjectNextCommand(repoRoot, objectType, result.Object, result.NextCommand)
	if err != nil {
		return ObjectResult{}, err
	}
	result.StatusUpdated = updated
	result.DeletedFiles = normalizeStrings(result.DeletedFiles)
	result.MissingFiles = normalizeStrings(result.MissingFiles)
	return result, nil
}

func applyObjectStableReroute(repoRoot string, result ObjectResult, objectType string, config objectFamilyConfig, fallbackReason string) (ObjectResult, error) {
	result.FallbackReasonCode = fallbackReason
	result.Outcome = "rerouted"
	result.NextCommand = config.StableNextCommand

	updated, err := statusfile.UpdateObjectNextCommand(repoRoot, objectType, result.Object, result.NextCommand)
	if err != nil {
		return ObjectResult{}, err
	}
	result.StatusUpdated = updated
	return result, nil
}

func objectProcessPaths(object string, processKinds []string) []string {
	paths := make([]string, 0, len(processKinds))
	for _, processKind := range processKinds {
		switch processKind {
		case "check":
			paths = append(paths, fmt.Sprintf("docs/specs/_check_result/%s.md", object))
		case "plan":
			paths = append(paths, fmt.Sprintf("docs/specs/_plans/%s.md", object))
		case "verify":
			paths = append(paths, fmt.Sprintf("docs/specs/_verify_result/%s.md", object))
		}
	}
	return normalizeStrings(paths)
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

func hasNonBoundModulesSelectedChange(refs []string, sharedFilesByRef map[string]SharedFile, boundModulesOnlyFileRefs map[string]bool) bool {
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

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

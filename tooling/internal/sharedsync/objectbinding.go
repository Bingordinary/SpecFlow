package sharedsync

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/sharedbinding"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/snapshot"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

type objectBinding struct {
	Status        statusfile.ObjectStatus
	SharedRefs    []string
	BindingIssues []string
}

func loadObjectBindings(repoRoot, objectType string) (map[string]objectBinding, []string, error) {
	statuses, err := statusfile.LoadObjectStatuses(repoRoot)
	if err != nil {
		return nil, nil, err
	}

	bindings := map[string]objectBinding{}
	unresolvedRefs := []string{}
	for _, status := range statuses {
		if status.ObjectType != objectType {
			continue
		}

		refs, err := readObjectSharedRefs(repoRoot, status)
		if err != nil {
			return nil, nil, err
		}

		bindingIssues := []string{}
		for _, ref := range refs {
			if _, err := sharedbinding.ResolveRef(repoRoot, status.ActiveLayer, ref); err != nil {
				bindingIssues = append(bindingIssues, err.Error())
				unresolvedRefs = append(unresolvedRefs, ref)
			}
		}

		bindings[status.Object] = objectBinding{
			Status:        status,
			SharedRefs:    refs,
			BindingIssues: normalizeStrings(bindingIssues),
		}
	}

	return bindings, normalizeStrings(unresolvedRefs), nil
}

func readObjectSharedRefs(repoRoot string, status statusfile.ObjectStatus) ([]string, error) {
	mainSpecRef, err := specpaths.ObjectMainSpecFileRef(status.ObjectType, status.ActiveLayer, status.Object)
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

func buildScopeObjects(bindings map[string]objectBinding, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile, scopedRefs, scopedIDs []string, removedBindingScope map[string]bool) ([]string, error) {
	if len(scopedRefs) == 0 && len(scopedIDs) == 0 {
		return nil, nil
	}

	scope := map[string]bool{}
	for object, binding := range bindings {
		selectedRefs, err := selectedSharedRefsForObject(binding.SharedRefs, scopedRefs, scopedIDs, sharedFilesByRef, sharedFilesByID)
		if err != nil {
			return nil, err
		}
		if len(selectedRefs) > 0 {
			scope[object] = true
		}
	}
	for object := range removedBindingScope {
		scope[object] = true
	}
	return sortedKeys(scope), nil
}

func candidateObjectsWithRemovedSelectedBinding(repoRoot string, bindings map[string]objectBinding, scopedRefs, scopedIDs []string, sharedFilesByRef map[string]sharedFile, sharedFilesByID map[string][]sharedFile) (map[string]bool, error) {
	result := map[string]bool{}
	for object, binding := range bindings {
		if binding.Status.ActiveLayer != "candidate" {
			continue
		}
		selectedRefs, err := selectedSharedRefsForObject(binding.SharedRefs, scopedRefs, scopedIDs, sharedFilesByRef, sharedFilesByID)
		if err != nil {
			return nil, err
		}
		if len(selectedRefs) > 0 {
			continue
		}
		matched, err := processSnapshotContainsSelectedShared(
			repoRoot,
			binding.Status.ObjectType,
			binding.Status.Object,
			binding.Status.ActiveLayer,
			[]string{"check", "verify"},
			scopedRefs,
			scopedIDs,
			sharedFilesByID,
		)
		if err != nil {
			return nil, err
		}
		if matched {
			result[object] = true
		}
	}
	return result, nil
}

func processSnapshotContainsSelectedShared(repoRoot, objectType, object, activeLayer string, processKinds, scopedRefs, scopedIDs []string, sharedFilesByID map[string][]sharedFile) (bool, error) {
	refSet := makeStringSet(scopedRefs)
	idSet := makeStringSet(scopedIDs)
	for _, processKind := range processKinds {
		processPath, err := snapshot.ProcessFilePath(object, processKind)
		if err != nil {
			return false, err
		}
		processAbs := filepath.Join(repoRoot, filepath.FromSlash(processPath))
		if _, err := os.Stat(processAbs); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, fmt.Errorf("stat %s: %w", processPath, err)
		}
		processSnapshot, err := snapshot.LoadProcessSnapshot(repoRoot, object, processKind)
		if err != nil {
			return false, err
		}
		validEvidence := false
		if objectType == "module" {
			validEvidence, err = isValidModuleRemovedBindingEvidence(repoRoot, object, activeLayer, processKind, processSnapshot, scopedRefs, scopedIDs, sharedFilesByID)
		} else {
			validEvidence, err = isValidRemovedBindingEvidence(repoRoot, processSnapshot, objectType, object, activeLayer, processKind, scopedRefs, scopedIDs, sharedFilesByID)
		}
		if err != nil {
			return false, err
		}
		if !validEvidence {
			continue
		}
		for _, entry := range processSnapshot.SharedContractSnapshot {
			matched, err := matchesSelectedSharedEntry(entry, refSet, idSet, sharedFilesByID)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
	}
	return false, nil
}

func isValidModuleRemovedBindingEvidence(repoRoot, module, activeLayer, processKind string, processSnapshot snapshot.ProcessSnapshotData, scopedRefs, scopedIDs []string, sharedFilesByID map[string][]sharedFile) (bool, error) {
	requiredScalars := []string{
		"object_type",
		"object_ref",
		"gate",
		"decision",
		"allow_next",
		"next_command",
		"blocking_summary",
		"coverage_summary",
		"truth_layer_ref",
		"truth_file_ref",
		"truth_version_ref",
		"truth_fingerprint",
		"system_constraints_stable_file_ref",
		"system_constraints_stable_version_ref",
		"system_constraints_stable_fingerprint",
	}
	if processKind == "verify" {
		requiredScalars = append(requiredScalars, "verification_scope_ref")
	}
	for _, field := range requiredScalars {
		if !processSnapshot.PresentFields[field] {
			return false, nil
		}
		if strings.TrimSpace(processSnapshot.Scalars[field]) == "" {
			return false, nil
		}
	}
	if !processSnapshot.PresentFields["module_appendix_snapshot"] || !processSnapshot.PresentFields["shared_contract_snapshot"] {
		return false, nil
	}
	if !allSharedSnapshotEntriesComplete(processSnapshot.SharedContractSnapshot) {
		return false, nil
	}

	expectedGate, expectedNextCommand, ok := expectedModuleProcessRouting(processKind)
	if !ok {
		return false, nil
	}
	currentSnapshot, err := snapshot.RebuildCurrent(repoRoot, module)
	if err != nil {
		return false, err
	}
	currentTruthContent, err := readCurrentObjectTruthContent(repoRoot, "module", module, activeLayer)
	if err != nil {
		return false, err
	}
	if processSnapshot.Scalars["object_type"] != "module" {
		return false, nil
	}
	if processSnapshot.Scalars["object_ref"] != module {
		return false, nil
	}
	if processSnapshot.Scalars["gate"] != expectedGate {
		return false, nil
	}
	if processSnapshot.Scalars["decision"] != "pass" {
		return false, nil
	}
	if processSnapshot.Scalars["allow_next"] != "true" {
		return false, nil
	}
	if processSnapshot.Scalars["next_command"] != expectedNextCommand {
		return false, nil
	}
	if processSnapshot.Scalars["truth_layer_ref"] != activeLayer {
		return false, nil
	}
	if processKind == "verify" && strings.TrimSpace(processSnapshot.Scalars["verification_scope_ref"]) == "" {
		return false, nil
	}
	truthMatches, err := matchesRemovedBindingTruth(processSnapshot, currentSnapshot.SpecFileRef, currentTruthContent, processSnapshot.SharedContractSnapshot)
	if err != nil {
		return false, err
	}
	if !truthMatches {
		return false, nil
	}
	if processSnapshot.Scalars["system_constraints_stable_file_ref"] != currentSnapshot.SystemConstraintsStableFileRef ||
		processSnapshot.Scalars["system_constraints_stable_version_ref"] != currentSnapshot.SystemConstraintsStableVersionRef ||
		processSnapshot.Scalars["system_constraints_stable_fingerprint"] != currentSnapshot.SystemConstraintsStableFingerprint {
		return false, nil
	}
	if !equalAppendixEntries(processSnapshot.ModuleAppendixSnapshot, currentSnapshot.ModuleAppendixSnapshot) {
		return false, nil
	}

	return sharedSnapshotMatchesRemovedBindingEvidence(
		processSnapshot.SharedContractSnapshot,
		currentSnapshot.SharedContractSnapshot,
		scopedRefs,
		scopedIDs,
		sharedFilesByID,
	)
}

func isValidRemovedBindingEvidence(repoRoot string, processSnapshot snapshot.ProcessSnapshotData, objectType, object, activeLayer, processKind string, scopedRefs, scopedIDs []string, sharedFilesByID map[string][]sharedFile) (bool, error) {
	requiredScalars := []string{
		"object_type",
		"object_ref",
		"gate",
		"decision",
		"allow_next",
		"next_command",
		"blocking_summary",
		"coverage_summary",
		"truth_layer_ref",
		"truth_file_ref",
		"truth_version_ref",
		"truth_fingerprint",
		"system_constraints_stable_file_ref",
		"system_constraints_stable_version_ref",
		"system_constraints_stable_fingerprint",
	}
	if processKind == "verify" {
		requiredScalars = append(requiredScalars, "verification_scope_ref")
	}
	for _, field := range requiredScalars {
		if !processSnapshot.PresentFields[field] {
			return false, nil
		}
		if strings.TrimSpace(processSnapshot.Scalars[field]) == "" {
			return false, nil
		}
	}

	requiredListFields := []string{"shared_contract_snapshot", "module_snapshot"}
	if objectType == "project" {
		requiredListFields = append(requiredListFields, "flow_snapshot")
	}
	for _, field := range requiredListFields {
		if !processSnapshot.PresentFields[field] {
			return false, nil
		}
	}
	if !allSharedSnapshotEntriesComplete(processSnapshot.SharedContractSnapshot) {
		return false, nil
	}

	expectedGate, expectedNextCommand, ok := expectedObjectProcessRouting(objectType, processKind)
	if !ok {
		return false, nil
	}
	currentSnapshot, err := rebuildCurrentObjectSnapshot(repoRoot, objectType, object, activeLayer)
	if err != nil {
		return false, err
	}
	currentTruthContent, err := readCurrentObjectTruthContent(repoRoot, objectType, object, activeLayer)
	if err != nil {
		return false, err
	}
	if processSnapshot.Scalars["object_type"] != objectType {
		return false, nil
	}
	if processSnapshot.Scalars["object_ref"] != object {
		return false, nil
	}
	if processSnapshot.Scalars["truth_layer_ref"] != activeLayer {
		return false, nil
	}
	truthMatches, err := matchesRemovedBindingTruth(processSnapshot, currentSnapshot.TruthFileRef, currentTruthContent, processSnapshot.SharedContractSnapshot)
	if err != nil {
		return false, err
	}
	if !truthMatches {
		return false, nil
	}
	if processSnapshot.Scalars["gate"] != expectedGate {
		return false, nil
	}
	if processSnapshot.Scalars["decision"] != "pass" {
		return false, nil
	}
	if processSnapshot.Scalars["allow_next"] != "true" {
		return false, nil
	}
	if processSnapshot.Scalars["next_command"] != expectedNextCommand {
		return false, nil
	}
	if processSnapshot.Scalars["system_constraints_stable_file_ref"] != currentSnapshot.SystemConstraintsStableFileRef ||
		processSnapshot.Scalars["system_constraints_stable_version_ref"] != currentSnapshot.SystemConstraintsStableVersionRef ||
		processSnapshot.Scalars["system_constraints_stable_fingerprint"] != currentSnapshot.SystemConstraintsStableFingerprint {
		return false, nil
	}
	if !equalObjectSnapshotEntries(processSnapshot.ModuleSnapshot, currentSnapshot.ModuleSnapshot) {
		return false, nil
	}
	if objectType == "project" && !equalObjectSnapshotEntries(processSnapshot.FlowSnapshot, currentSnapshot.FlowSnapshot) {
		return false, nil
	}
	return sharedSnapshotMatchesRemovedBindingEvidence(
		processSnapshot.SharedContractSnapshot,
		currentSnapshot.SharedContractSnapshot,
		scopedRefs,
		scopedIDs,
		sharedFilesByID,
	)
}

func allSharedSnapshotEntriesComplete(entries []snapshot.SharedContractEntry) bool {
	if len(entries) == 0 {
		return true
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.SharedContractID) == "" ||
			strings.TrimSpace(entry.Layer) == "" ||
			strings.TrimSpace(entry.FileRef) == "" ||
			strings.TrimSpace(entry.VersionRef) == "" ||
			strings.TrimSpace(entry.Fingerprint) == "" {
			return false
		}
	}
	return true
}

func equalObjectSnapshotEntries(actual, expected []snapshot.ObjectSnapshotEntry) bool {
	actual = normalizeObjectSnapshotEntries(actual)
	expected = normalizeObjectSnapshotEntries(expected)
	if len(actual) != len(expected) {
		return false
	}
	for idx := range actual {
		if actual[idx] != expected[idx] {
			return false
		}
	}
	return true
}

func equalAppendixEntries(actual, expected []snapshot.AppendixEntry) bool {
	actual = normalizeAppendixEntries(actual)
	expected = normalizeAppendixEntries(expected)
	if len(actual) != len(expected) {
		return false
	}
	for idx := range actual {
		if actual[idx] != expected[idx] {
			return false
		}
	}
	return true
}

func equalSharedSnapshotEntries(actual, expected []snapshot.SharedContractEntry) bool {
	actual = normalizeSharedSnapshotEntries(actual)
	expected = normalizeSharedSnapshotEntries(expected)
	if len(actual) != len(expected) {
		return false
	}
	for idx := range actual {
		if actual[idx] != expected[idx] {
			return false
		}
	}
	return true
}

func normalizeAppendixEntries(entries []snapshot.AppendixEntry) []snapshot.AppendixEntry {
	if len(entries) == 0 {
		return nil
	}
	items := append([]snapshot.AppendixEntry(nil), entries...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].FileRef != items[j].FileRef {
			return items[i].FileRef < items[j].FileRef
		}
		return items[i].AppendixRef < items[j].AppendixRef
	})
	return items
}

func normalizeObjectSnapshotEntries(entries []snapshot.ObjectSnapshotEntry) []snapshot.ObjectSnapshotEntry {
	if len(entries) == 0 {
		return nil
	}
	items := append([]snapshot.ObjectSnapshotEntry(nil), entries...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].ObjectRef != items[j].ObjectRef {
			return items[i].ObjectRef < items[j].ObjectRef
		}
		if items[i].Layer != items[j].Layer {
			return items[i].Layer < items[j].Layer
		}
		return items[i].FileRef < items[j].FileRef
	})
	return items
}

func normalizeSharedSnapshotEntries(entries []snapshot.SharedContractEntry) []snapshot.SharedContractEntry {
	if len(entries) == 0 {
		return nil
	}
	items := append([]snapshot.SharedContractEntry(nil), entries...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].SharedContractID != items[j].SharedContractID {
			return items[i].SharedContractID < items[j].SharedContractID
		}
		if items[i].Layer != items[j].Layer {
			return items[i].Layer < items[j].Layer
		}
		return items[i].FileRef < items[j].FileRef
	})
	return items
}

type currentObjectSnapshot struct {
	TruthFileRef                       string
	TruthVersionRef                    string
	TruthFingerprint                   string
	SystemConstraintsStableFileRef     string
	SystemConstraintsStableVersionRef  string
	SystemConstraintsStableFingerprint string
	ModuleSnapshot                     []snapshot.ObjectSnapshotEntry
	FlowSnapshot                       []snapshot.ObjectSnapshotEntry
	SharedContractSnapshot             []snapshot.SharedContractEntry
}

func rebuildCurrentObjectSnapshot(repoRoot, objectType, object, activeLayer string) (currentObjectSnapshot, error) {
	mainSpecRef, err := specpaths.ObjectMainSpecFileRef(objectType, activeLayer, object)
	if err != nil {
		return currentObjectSnapshot{}, err
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	if err != nil {
		return currentObjectSnapshot{}, fmt.Errorf("read %s: %w", mainSpecRef, err)
	}
	frontmatter, body, err := parseFrontmatter(string(content))
	if err != nil {
		return currentObjectSnapshot{}, fmt.Errorf("%s: %w", mainSpecRef, err)
	}
	version := strings.TrimSpace(frontmatter["version"])
	if version == "" {
		return currentObjectSnapshot{}, fmt.Errorf("%s: missing frontmatter.version", mainSpecRef)
	}
	systemFileRef, systemVersionRef, systemFingerprint, err := buildSystemConstraintsSnapshot(repoRoot, body)
	if err != nil {
		return currentObjectSnapshot{}, err
	}

	result := currentObjectSnapshot{
		TruthFileRef:                       mainSpecRef,
		TruthVersionRef:                    fmt.Sprintf("%s@%s", strings.TrimSuffix(filepath.Base(mainSpecRef), ".md"), version),
		TruthFingerprint:                   hashNormalizedText(string(content)),
		SystemConstraintsStableFileRef:     systemFileRef,
		SystemConstraintsStableVersionRef:  systemVersionRef,
		SystemConstraintsStableFingerprint: systemFingerprint,
	}

	if objectType == "flow" || objectType == "project" {
		moduleRefs, hasField, err := parseNamedRefList(body, "module_refs")
		if err != nil {
			return currentObjectSnapshot{}, err
		}
		if hasField {
			result.ModuleSnapshot, err = buildObjectDependencySnapshot(repoRoot, "module", moduleRefs)
			if err != nil {
				return currentObjectSnapshot{}, err
			}
		}
	}
	if objectType == "project" {
		flowRefs, hasField, err := parseNamedRefList(body, "flow_refs")
		if err != nil {
			return currentObjectSnapshot{}, err
		}
		if hasField {
			result.FlowSnapshot, err = buildObjectDependencySnapshot(repoRoot, "flow", flowRefs)
			if err != nil {
				return currentObjectSnapshot{}, err
			}
		}
	}
	sharedRefs, _, err := parseSharedContractRefs(body)
	if err != nil {
		return currentObjectSnapshot{}, err
	}
	result.SharedContractSnapshot, err = buildSharedContractSnapshot(repoRoot, activeLayer, sharedRefs)
	if err != nil {
		return currentObjectSnapshot{}, err
	}

	return result, nil
}

func buildSharedContractSnapshot(repoRoot, activeLayer string, refs []string) ([]snapshot.SharedContractEntry, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	entries := make([]snapshot.SharedContractEntry, 0, len(refs))
	for _, ref := range refs {
		resolved, err := sharedbinding.ResolveRef(repoRoot, activeLayer, ref)
		if err != nil {
			return nil, err
		}
		entries = append(entries, snapshot.SharedContractEntry{
			SharedContractID: resolved.SharedContractID,
			Layer:            resolved.Layer,
			FileRef:          resolved.FileRef,
			VersionRef:       resolved.VersionRef,
			Fingerprint:      hashNormalizedText(resolved.Content),
		})
	}
	return normalizeSharedSnapshotEntries(entries), nil
}

func buildObjectDependencySnapshot(repoRoot, objectType string, refs []string) ([]snapshot.ObjectSnapshotEntry, error) {
	entries := make([]snapshot.ObjectSnapshotEntry, 0, len(refs))
	for _, ref := range refs {
		entry, err := resolveObjectVersionRef(repoRoot, objectType, ref)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return normalizeObjectSnapshotEntries(entries), nil
}

func resolveObjectVersionRef(repoRoot, expectedObjectType, ref string) (snapshot.ObjectSnapshotEntry, error) {
	prefix, _, found := strings.Cut(strings.TrimSpace(ref), "@")
	if !found {
		return snapshot.ObjectSnapshotEntry{}, fmt.Errorf("invalid %s ref %q", expectedObjectType, ref)
	}
	objectType, layer, object, err := parseObjectVersionRefPrefix(prefix)
	if err != nil {
		return snapshot.ObjectSnapshotEntry{}, err
	}
	if objectType != expectedObjectType {
		return snapshot.ObjectSnapshotEntry{}, fmt.Errorf("%s ref %q resolves to object type %q", expectedObjectType, ref, objectType)
	}
	fileRef, err := specpaths.ObjectMainSpecFileRef(objectType, layer, object)
	if err != nil {
		return snapshot.ObjectSnapshotEntry{}, err
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
	if err != nil {
		return snapshot.ObjectSnapshotEntry{}, fmt.Errorf("read %s: %w", fileRef, err)
	}
	frontmatter, _, err := parseFrontmatter(string(content))
	if err != nil {
		return snapshot.ObjectSnapshotEntry{}, fmt.Errorf("%s: %w", fileRef, err)
	}
	version := strings.TrimSpace(frontmatter["version"])
	if version == "" {
		return snapshot.ObjectSnapshotEntry{}, fmt.Errorf("%s: missing frontmatter.version", fileRef)
	}
	expectedVersionRef := fmt.Sprintf("%s@%s", prefix, version)
	if expectedVersionRef != ref {
		return snapshot.ObjectSnapshotEntry{}, fmt.Errorf("%s ref %q does not match current version %q", expectedObjectType, ref, expectedVersionRef)
	}
	return snapshot.ObjectSnapshotEntry{
		ObjectRef:   object,
		Layer:       layer,
		FileRef:     fileRef,
		VersionRef:  expectedVersionRef,
		Fingerprint: hashNormalizedText(string(content)),
	}, nil
}

func parseObjectVersionRefPrefix(prefix string) (string, string, string, error) {
	switch {
	case strings.HasPrefix(prefix, "c_module_"):
		return "module", "candidate", strings.TrimPrefix(prefix, "c_module_"), nil
	case strings.HasPrefix(prefix, "s_module_"):
		return "module", "stable", strings.TrimPrefix(prefix, "s_module_"), nil
	case strings.HasPrefix(prefix, "c_flow_"):
		return "flow", "candidate", strings.TrimPrefix(prefix, "c_flow_"), nil
	case strings.HasPrefix(prefix, "s_flow_"):
		return "flow", "stable", strings.TrimPrefix(prefix, "s_flow_"), nil
	case prefix == "c_project":
		return "project", "candidate", "project", nil
	case prefix == "s_project":
		return "project", "stable", "project", nil
	default:
		return "", "", "", fmt.Errorf("unsupported object version ref prefix %q", prefix)
	}
}

func buildSystemConstraintsSnapshot(repoRoot, body string) (string, string, string, error) {
	ref, _, err := parseSystemConstraintsStableRef(body)
	if err != nil {
		return "", "", "", err
	}
	if ref == "" || ref == "none" {
		return "none", "none", "none", nil
	}
	if !strings.HasPrefix(ref, "s_system_constraints@") {
		return "", "", "", fmt.Errorf("unsupported system_constraints_stable_ref %q", ref)
	}

	systemFileRef := specpaths.SystemConstraintsStableFileRef
	systemContent, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(systemFileRef)))
	if err != nil {
		return "", "", "", fmt.Errorf("read %s: %w", systemFileRef, err)
	}
	systemFrontmatter, _, err := parseFrontmatter(string(systemContent))
	if err != nil {
		return "", "", "", fmt.Errorf("%s: %w", systemFileRef, err)
	}
	systemVersion := strings.TrimSpace(systemFrontmatter["version"])
	if systemVersion == "" {
		return "", "", "", fmt.Errorf("%s: missing frontmatter.version", systemFileRef)
	}
	return systemFileRef, fmt.Sprintf("s_system_constraints@%s", systemVersion), hashNormalizedText(string(systemContent)), nil
}

func parseNamedRefList(body, fieldName string) ([]string, bool, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		right, matched, err := parseObjectNamedFieldLine(trimmed, fieldName)
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
			return nil, false, fmt.Errorf("%s must use literal none or a markdown list", fieldName)
		}
		refs := []string{}
		seen := map[string]bool{}
		for next := idx + 1; next < len(lines); next++ {
			nextTrimmed := strings.TrimSpace(lines[next])
			if nextTrimmed == "" {
				continue
			}
			if strings.HasPrefix(nextTrimmed, "## ") || regexp.MustCompile(`^\d+\.`).MatchString(nextTrimmed) {
				break
			}
			if !strings.HasPrefix(nextTrimmed, "- ") {
				return nil, false, fmt.Errorf("%s must be a markdown list", fieldName)
			}
			ref := strings.TrimSpace(strings.TrimPrefix(nextTrimmed, "- "))
			ref = strings.Trim(ref, "`")
			if ref == "" {
				return nil, false, fmt.Errorf("%s contains an empty item", fieldName)
			}
			if seen[ref] {
				return nil, false, fmt.Errorf("%s contains duplicate item %q", fieldName, ref)
			}
			seen[ref] = true
			refs = append(refs, ref)
		}
		if len(refs) == 0 {
			return nil, false, fmt.Errorf("%s must not be an empty list", fieldName)
		}
		return refs, true, nil
	}
	return nil, false, nil
}

func parseSystemConstraintsStableRef(body string) (string, bool, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		right, matched, err := parseObjectNamedFieldLine(trimmed, "system_constraints_stable_ref")
		if err != nil {
			return "", false, err
		}
		if !matched {
			continue
		}
		value := strings.Trim(right, "`")
		if value == "" {
			return "", false, fmt.Errorf("system_constraints_stable_ref is empty")
		}
		return value, true, nil
	}
	return "", false, nil
}

func parseObjectNamedFieldLine(trimmed, fieldName string) (string, bool, error) {
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return "", false, nil
	}
	left := normalizeObjectFieldKey(strings.TrimSpace(parts[0]))
	if left != fieldName {
		return "", false, nil
	}
	return strings.TrimSpace(parts[1]), true, nil
}

func normalizeObjectFieldKey(value string) string {
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

func hashNormalizedText(content string) string {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	text += "\n"
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

func sharedSnapshotMatchesRemovedBindingEvidence(stored, current []snapshot.SharedContractEntry, scopedRefs []string, scopedIDs []string, sharedFilesByID map[string][]sharedFile) (bool, error) {
	stored = normalizeSharedSnapshotEntries(stored)
	current = normalizeSharedSnapshotEntries(current)
	if equalSharedSnapshotEntries(stored, current) {
		return true, nil
	}
	refSet := makeStringSet(scopedRefs)
	idSet := makeStringSet(scopedIDs)

	currentSet := map[string]bool{}
	for _, entry := range current {
		currentSet[sharedSnapshotEntryKey(entry)] = true
	}
	storedSet := map[string]bool{}
	for _, entry := range stored {
		storedSet[sharedSnapshotEntryKey(entry)] = true
	}

	for _, entry := range current {
		if !storedSet[sharedSnapshotEntryKey(entry)] {
			return false, nil
		}
	}
	for _, entry := range stored {
		if currentSet[sharedSnapshotEntryKey(entry)] {
			continue
		}
		matched, err := matchesSelectedSharedEntry(entry, refSet, idSet, sharedFilesByID)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func matchesSelectedSharedEntry(entry snapshot.SharedContractEntry, scopedRefSet, scopedIDSet map[string]bool, sharedFilesByID map[string][]sharedFile) (bool, error) {
	if scopedRefSet[entry.VersionRef] {
		return true, nil
	}
	return matchesSelectedSharedIDEntry(entry, scopedIDSet, sharedFilesByID)
}

func matchesSelectedSharedIDEntry(entry snapshot.SharedContractEntry, scopedIDSet map[string]bool, sharedFilesByID map[string][]sharedFile) (bool, error) {
	if !scopedIDSet[entry.SharedContractID] {
		return false, nil
	}
	candidates := sharedFilesByID[entry.SharedContractID]
	if len(candidates) > 1 {
		return false, fmt.Errorf("shared_id %q resolves to multiple current shared files; removed-binding scope is ambiguous", entry.SharedContractID)
	}
	if len(candidates) != 1 {
		return false, nil
	}
	current := candidates[0]
	if current.FileRef == entry.FileRef && current.Layer == entry.Layer {
		return true, nil
	}
	return false, nil
}

func sharedSnapshotEntryKey(entry snapshot.SharedContractEntry) string {
	return strings.Join([]string{
		entry.SharedContractID,
		entry.Layer,
		entry.FileRef,
		entry.VersionRef,
		entry.Fingerprint,
	}, "\x00")
}

func hasOnlySharedSnapshotMismatch(mismatches []string) bool {
	if len(mismatches) == 0 {
		return false
	}
	for _, mismatch := range mismatches {
		if !strings.HasPrefix(mismatch, "shared_contract_snapshot mismatch") {
			return false
		}
	}
	return true
}

func readCurrentObjectTruthContent(repoRoot, objectType, object, activeLayer string) (string, error) {
	mainSpecRef, err := specpaths.ObjectMainSpecFileRef(objectType, activeLayer, object)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", mainSpecRef, err)
	}
	return string(content), nil
}

func matchesRemovedBindingTruth(processSnapshot snapshot.ProcessSnapshotData, currentTruthFileRef, currentTruthContent string, storedShared []snapshot.SharedContractEntry) (bool, error) {
	if processSnapshot.Scalars["truth_file_ref"] != currentTruthFileRef {
		return false, nil
	}
	reconstructedContents, err := reconstructTruthWithStoredSharedSnapshot(currentTruthContent, storedShared)
	if err != nil {
		return false, err
	}
	for _, reconstructedContent := range reconstructedContents {
		frontmatter, _, err := parseFrontmatter(reconstructedContent)
		if err != nil {
			return false, err
		}
		version := strings.TrimSpace(frontmatter["version"])
		if version == "" {
			return false, fmt.Errorf("%s: missing frontmatter.version", currentTruthFileRef)
		}
		reconstructedVersionRef := fmt.Sprintf("%s@%s", strings.TrimSuffix(filepath.Base(currentTruthFileRef), ".md"), version)
		if processSnapshot.Scalars["truth_version_ref"] != reconstructedVersionRef {
			continue
		}
		if processSnapshot.Scalars["truth_fingerprint"] == hashNormalizedText(reconstructedContent) {
			return true, nil
		}
	}
	return false, nil
}

func reconstructTruthWithStoredSharedSnapshot(currentTruthContent string, storedShared []snapshot.SharedContractEntry) ([]string, error) {
	_, body, err := parseFrontmatter(currentTruthContent)
	if err != nil {
		return nil, err
	}
	refs := make([]string, 0, len(storedShared))
	for _, entry := range storedShared {
		refs = append(refs, strings.TrimSpace(entry.VersionRef))
	}
	sort.Strings(refs)
	normalized := strings.ReplaceAll(currentTruthContent, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	endIdx := -1
	for idx := 1; idx < len(lines); idx++ {
		if strings.TrimSpace(lines[idx]) == "---" {
			endIdx = idx
			break
		}
	}
	if endIdx == -1 {
		return nil, fmt.Errorf("missing frontmatter end marker")
	}
	variants := []sharedRefRenderStyle{
		{wrapWithBackticks: false},
		{wrapWithBackticks: true},
	}
	rebuiltContents := []string{}
	seen := map[string]bool{}
	for _, style := range variants {
		rewrittenBody, err := rewriteSharedContractRefsInBody(body, refs, style)
		if err != nil {
			return nil, err
		}
		rebuilt := append([]string{}, lines[:endIdx+1]...)
		rebuilt = append(rebuilt, strings.Split(rewrittenBody, "\n")...)
		content := strings.Join(rebuilt, "\n")
		if !seen[content] {
			seen[content] = true
			rebuiltContents = append(rebuiltContents, content)
		}
	}
	return rebuiltContents, nil
}

type sharedRefRenderStyle struct {
	wrapWithBackticks bool
}

func rewriteSharedContractRefsInBody(body string, refs []string, style sharedRefRenderStyle) (string, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		_, matched, err := parseObjectNamedFieldLine(trimmed, "shared_contract_refs")
		if err != nil {
			return "", err
		}
		if !matched {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			return "", fmt.Errorf("shared_contract_refs line missing colon")
		}
		left := line[:colonIdx]
		end := idx + 1
		for end < len(lines) {
			nextTrimmed := strings.TrimSpace(lines[end])
			if nextTrimmed == "" {
				end++
				continue
			}
			if strings.HasPrefix(nextTrimmed, "## ") || regexp.MustCompile(`^\d+\.`).MatchString(nextTrimmed) {
				break
			}
			if !strings.HasPrefix(nextTrimmed, "- ") {
				return "", fmt.Errorf("shared_contract_refs must be a markdown list of shared refs")
			}
			end++
		}

		replacement := []string{}
		if len(refs) == 0 {
			replacement = append(replacement, left+": none")
		} else {
			replacement = append(replacement, left+":")
			indent := detectSharedContractRefIndent(line, lines[idx+1:end])
			for _, ref := range refs {
				renderedRef := ref
				if style.wrapWithBackticks {
					renderedRef = "`" + ref + "`"
				}
				replacement = append(replacement, indent+"- "+renderedRef)
			}
		}
		updated := append([]string{}, lines[:idx]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[end:]...)
		return strings.Join(updated, "\n"), nil
	}
	return "", fmt.Errorf("shared_contract_refs field not found")
}

func detectSharedContractRefIndent(fieldLine string, existingListLines []string) string {
	for _, line := range existingListLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			if idx := strings.Index(line, "- "); idx >= 0 {
				return line[:idx]
			}
		}
	}
	leading := fieldLine[:len(fieldLine)-len(strings.TrimLeft(fieldLine, " \t"))]
	if regexp.MustCompile(`^\s*\d+\.`).MatchString(fieldLine) {
		return leading + "   "
	}
	return leading + "  "
}

func expectedModuleProcessRouting(processKind string) (string, string, bool) {
	switch processKind {
	case "check":
		return "module_check", "module_plan", true
	case "verify":
		return "module_verify", "module_promote", true
	default:
		return "", "", false
	}
}

func expectedObjectProcessRouting(objectType, processKind string) (string, string, bool) {
	switch objectType {
	case "flow":
		switch processKind {
		case "check":
			return "flow_check", "flow_verify", true
		case "verify":
			return "flow_verify", "flow_promote", true
		}
	case "project":
		switch processKind {
		case "check":
			return "project_check", "project_verify", true
		case "verify":
			return "project_verify", "project_promote", true
		}
	}
	return "", "", false
}

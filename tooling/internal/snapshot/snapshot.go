package snapshot

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/sharedbinding"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

type AppendixEntry struct {
	FileRef     string
	AppendixRef string
	Fingerprint string
}

type SharedContractEntry struct {
	SharedContractID string
	Layer            string
	FileRef          string
	VersionRef       string
	Fingerprint      string
}

type ObjectSnapshotEntry struct {
	ObjectRef   string
	Layer       string
	FileRef     string
	VersionRef  string
	Fingerprint string
}

type Snapshot struct {
	Module                       string
	TruthLayerRef                string
	SpecFileRef                  string
	SpecVersionRef               string
	SpecFingerprint              string
	ModuleAppendixSnapshot       []AppendixEntry
	SystemConstraintsFileRef     string
	SystemConstraintsVersionRef  string
	SystemConstraintsFingerprint string
	SharedContractSnapshot       []SharedContractEntry
}

type ValidationResult struct {
	ProcessKind string
	ProcessFile string
	Valid       bool
	Mismatches  []string
	Expected    Snapshot
}

type ProcessSnapshotData struct {
	ProcessKind            string
	ProcessFile            string
	PresentFields          map[string]bool
	Scalars                map[string]string
	ModuleAppendixSnapshot []AppendixEntry
	ModuleSnapshot         []ObjectSnapshotEntry
	FlowSnapshot           []ObjectSnapshotEntry
	SharedContractSnapshot []SharedContractEntry
}

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

var requiredProcessSnapshotFields = map[string][]string{
	"check": {
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
		"unit_appendix_snapshot",
		"system_constraints_file_ref",
		"system_constraints_version_ref",
		"system_constraints_fingerprint",
		"shared_contract_snapshot",
	},
	"plan": {
		"spec_file_ref",
		"spec_version_ref",
		"spec_fingerprint",
		"unit_appendix_snapshot",
		"system_constraints_file_ref",
		"system_constraints_version_ref",
		"system_constraints_fingerprint",
		"shared_contract_snapshot",
	},
	"verify": {
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
		"unit_appendix_snapshot",
		"verification_scope_ref",
		"system_constraints_file_ref",
		"system_constraints_version_ref",
		"system_constraints_fingerprint",
		"shared_contract_snapshot",
	},
}

func RebuildCurrent(repoRoot, module string) (Snapshot, error) {
	moduleStatus, err := statusfile.LookupModuleStatus(repoRoot, module)
	if err != nil {
		return Snapshot{}, err
	}

	mainSpecRef, err := specpaths.MainSpecFileRef(moduleStatus.ActiveLayer, moduleStatus.Module)
	if err != nil {
		return Snapshot{}, err
	}
	mainSpecAbs := filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef))
	mainSpecContent, err := os.ReadFile(mainSpecAbs)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read %s: %w", mainSpecRef, err)
	}
	frontmatter, body, err := parseFrontmatter(string(mainSpecContent))
	if err != nil {
		return Snapshot{}, fmt.Errorf("%s: %w", mainSpecRef, err)
	}

	result := Snapshot{
		Module:          module,
		TruthLayerRef:   moduleStatus.ActiveLayer,
		SpecFileRef:     mainSpecRef,
		SpecFingerprint: hashNormalizedText(string(mainSpecContent)),
	}
	version := strings.TrimSpace(frontmatter["version"])
	if version == "" {
		return Snapshot{}, fmt.Errorf("%s: missing frontmatter.version", mainSpecRef)
	}
	result.SpecVersionRef = fmt.Sprintf("%s@%s", strings.TrimSuffix(filepath.Base(mainSpecRef), ".md"), version)

	appendixEntries, err := buildAppendixSnapshot(repoRoot, mainSpecRef, body)
	if err != nil {
		return Snapshot{}, err
	}
	result.ModuleAppendixSnapshot = appendixEntries

	systemFileRef, systemVersionRef, systemFingerprint, err := buildSystemConstraintsSnapshot(repoRoot, body)
	if err != nil {
		return Snapshot{}, err
	}
	result.SystemConstraintsFileRef = systemFileRef
	result.SystemConstraintsVersionRef = systemVersionRef
	result.SystemConstraintsFingerprint = systemFingerprint

	sharedEntries, err := buildSharedContractSnapshot(repoRoot, moduleStatus.ActiveLayer, body)
	if err != nil {
		return Snapshot{}, err
	}
	result.SharedContractSnapshot = sharedEntries
	return result, nil
}

func ValidateProcessFile(repoRoot, module, processKind string) (ValidationResult, error) {
	expected, err := RebuildCurrent(repoRoot, module)
	if err != nil {
		return ValidationResult{}, err
	}
	requiredFields, ok := requiredProcessSnapshotFields[processKind]
	if !ok {
		return ValidationResult{}, fmt.Errorf("unsupported process kind %q", processKind)
	}

	processFile, err := ProcessFilePath("unit", module, processKind)
	if err != nil {
		return ValidationResult{}, err
	}
	processAbs := filepath.Join(repoRoot, filepath.FromSlash(processFile))
	content, err := os.ReadFile(processAbs)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("read %s: %w", processFile, err)
	}

	actual, err := parseProcessSnapshot(string(content))
	if err != nil {
		return ValidationResult{}, fmt.Errorf("%s: %w", processFile, err)
	}

	result := ValidationResult{
		ProcessKind: processKind,
		ProcessFile: processFile,
		Expected:    expected,
		Valid:       true,
	}

	for _, field := range requiredFields {
		if !actual.presentFields[field] {
			result.Valid = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("missing required field: %s", field))
			continue
		}
		if actualValue, ok := actual.scalars[field]; ok && strings.TrimSpace(actualValue) == "" {
			result.Valid = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("%s must not be empty", field))
		}
	}

	if processKind == "plan" {
		compareScalar(&result, "spec_file_ref", actual.scalars["spec_file_ref"], expected.SpecFileRef)
		compareScalar(&result, "spec_version_ref", actual.scalars["spec_version_ref"], expected.SpecVersionRef)
		compareScalar(&result, "spec_fingerprint", actual.scalars["spec_fingerprint"], expected.SpecFingerprint)

		if _, ok := actual.scalars["unit_appendix_snapshot"]; ok || actual.appendixPresent {
			actualAppendix := normalizeAppendixList(actual.appendixEntries)
			expectedAppendix := normalizeAppendixList(expected.ModuleAppendixSnapshot)
			if actualAppendix != expectedAppendix {
				result.Valid = false
				result.Mismatches = append(result.Mismatches, fmt.Sprintf("unit_appendix_snapshot mismatch: actual=%s expected=%s", actualAppendix, expectedAppendix))
			}
		}
		if _, ok := actual.scalars["shared_contract_snapshot"]; ok || actual.sharedPresent {
			actualShared := normalizeSharedList(actual.sharedEntries)
			expectedShared := normalizeSharedList(expected.SharedContractSnapshot)
			if actualShared != expectedShared {
				result.Valid = false
				result.Mismatches = append(result.Mismatches, fmt.Sprintf("shared_contract_snapshot mismatch: actual=%s expected=%s", actualShared, expectedShared))
			}
		}
		compareScalar(&result, "system_constraints_file_ref", actual.scalars["system_constraints_file_ref"], expected.SystemConstraintsFileRef)
		compareScalar(&result, "system_constraints_version_ref", actual.scalars["system_constraints_version_ref"], expected.SystemConstraintsVersionRef)
		compareScalar(&result, "system_constraints_fingerprint", actual.scalars["system_constraints_fingerprint"], expected.SystemConstraintsFingerprint)
		return result, nil
	}

	expectedGate, expectedNextCommand, err := expectedModuleProcessRouting(processKind)
	if err != nil {
		return ValidationResult{}, err
	}
	compareScalar(&result, "object_type", actual.scalars["object_type"], "unit")
	compareScalar(&result, "object_ref", actual.scalars["object_ref"], expected.Module)
	compareScalar(&result, "gate", actual.scalars["gate"], expectedGate)
	compareScalar(&result, "decision", actual.scalars["decision"], "pass")
	compareScalar(&result, "allow_next", actual.scalars["allow_next"], "true")
	compareScalar(&result, "next_command", actual.scalars["next_command"], expectedNextCommand)
	compareScalar(&result, "truth_layer_ref", actual.scalars["truth_layer_ref"], expected.TruthLayerRef)
	compareScalar(&result, "truth_file_ref", actual.scalars["truth_file_ref"], expected.SpecFileRef)
	compareScalar(&result, "truth_version_ref", actual.scalars["truth_version_ref"], expected.SpecVersionRef)
	compareScalar(&result, "truth_fingerprint", actual.scalars["truth_fingerprint"], expected.SpecFingerprint)
	compareScalar(&result, "system_constraints_file_ref", actual.scalars["system_constraints_file_ref"], expected.SystemConstraintsFileRef)
	compareScalar(&result, "system_constraints_version_ref", actual.scalars["system_constraints_version_ref"], expected.SystemConstraintsVersionRef)
	compareScalar(&result, "system_constraints_fingerprint", actual.scalars["system_constraints_fingerprint"], expected.SystemConstraintsFingerprint)

	if _, ok := actual.scalars["unit_appendix_snapshot"]; ok || actual.appendixPresent {
		actualAppendix := normalizeAppendixList(actual.appendixEntries)
		expectedAppendix := normalizeAppendixList(expected.ModuleAppendixSnapshot)
		if actualAppendix != expectedAppendix {
			result.Valid = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("unit_appendix_snapshot mismatch: actual=%s expected=%s", actualAppendix, expectedAppendix))
		}
	}
	if _, ok := actual.scalars["shared_contract_snapshot"]; ok || actual.sharedPresent {
		actualShared := normalizeSharedList(actual.sharedEntries)
		expectedShared := normalizeSharedList(expected.SharedContractSnapshot)
		if actualShared != expectedShared {
			result.Valid = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("shared_contract_snapshot mismatch: actual=%s expected=%s", actualShared, expectedShared))
		}
	}

	return result, nil
}

func expectedModuleProcessRouting(processKind string) (string, string, error) {
	switch processKind {
	case "check":
		return "unit_check", "unit_plan", nil
	case "plan":
		return "unit_plan", "unit_impl", nil
	case "verify":
		return "unit_verify", "unit_promote", nil
	default:
		return "", "", fmt.Errorf("unsupported process kind %q", processKind)
	}
}

func LoadProcessSnapshot(repoRoot, objectType, object, processKind string) (ProcessSnapshotData, error) {
	processFile, err := ProcessFilePath(objectType, object, processKind)
	if err != nil {
		return ProcessSnapshotData{}, err
	}
	processAbs := filepath.Join(repoRoot, filepath.FromSlash(processFile))
	content, err := os.ReadFile(processAbs)
	if err != nil {
		return ProcessSnapshotData{}, fmt.Errorf("read %s: %w", processFile, err)
	}

	parsed, err := parseProcessSnapshot(string(content))
	if err != nil {
		return ProcessSnapshotData{}, fmt.Errorf("%s: %w", processFile, err)
	}

	scalars := make(map[string]string, len(parsed.scalars))
	for key, value := range parsed.scalars {
		scalars[key] = value
	}
	return ProcessSnapshotData{
		ProcessKind:            processKind,
		ProcessFile:            processFile,
		PresentFields:          copyStringBoolMap(parsed.presentFields),
		Scalars:                scalars,
		ModuleAppendixSnapshot: append([]AppendixEntry(nil), parsed.appendixEntries...),
		ModuleSnapshot:         append([]ObjectSnapshotEntry(nil), parsed.moduleEntries...),
		FlowSnapshot:           append([]ObjectSnapshotEntry(nil), parsed.flowEntries...),
		SharedContractSnapshot: append([]SharedContractEntry(nil), parsed.sharedEntries...),
	}, nil
}

func Render(snapshot Snapshot) string {
	lines := []string{
		"object_type: unit",
		fmt.Sprintf("object_ref: %s", snapshot.Module),
		fmt.Sprintf("truth_layer_ref: %s", snapshot.TruthLayerRef),
		fmt.Sprintf("truth_file_ref: %s", snapshot.SpecFileRef),
		fmt.Sprintf("truth_version_ref: %s", snapshot.SpecVersionRef),
		fmt.Sprintf("truth_fingerprint: %s", snapshot.SpecFingerprint),
		fmt.Sprintf("system_constraints_file_ref: %s", snapshot.SystemConstraintsFileRef),
		fmt.Sprintf("system_constraints_version_ref: %s", snapshot.SystemConstraintsVersionRef),
		fmt.Sprintf("system_constraints_fingerprint: %s", snapshot.SystemConstraintsFingerprint),
		"unit_appendix_snapshot:",
	}
	lines = append(lines, renderAppendixLines(snapshot.ModuleAppendixSnapshot)...)
	lines = append(lines, "shared_contract_snapshot:")
	lines = append(lines, renderSharedLines(snapshot.SharedContractSnapshot)...)
	return strings.Join(lines, "\n")
}

func buildAppendixSnapshot(repoRoot, mainSpecRef, body string) ([]AppendixEntry, error) {
	mainDir := filepath.Dir(filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	currentLayer := mainSpecLayer(mainSpecRef)
	currentModule, err := mainSpecModule(mainSpecRef)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	entries := []AppendixEntry{}
	for _, destination := range markdownLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(destination) != 2 {
			continue
		}
		linkDestination := strings.TrimSpace(destination[1])
		if linkDestination == "" || strings.HasPrefix(linkDestination, "/") || strings.Contains(linkDestination, "://") {
			continue
		}
		absPath := filepath.Clean(filepath.Join(mainDir, filepath.FromSlash(linkDestination)))
		relWithinLayerRoot, err := filepath.Rel(mainDir, absPath)
		if err != nil {
			return nil, err
		}
		relWithinLayerRoot = filepath.ToSlash(relWithinLayerRoot)
		if strings.HasPrefix(relWithinLayerRoot, "../") || relWithinLayerRoot == ".." || filepath.Ext(relWithinLayerRoot) != ".md" {
			continue
		}
		relPath, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return nil, err
		}
		relPath = filepath.ToSlash(relPath)
		if relPath == mainSpecRef {
			continue
		}
		if filepath.Dir(relWithinLayerRoot) == "." {
			return nil, fmt.Errorf("%s: module-local supporting file %s remains in the layer root; this is directory drift", mainSpecRef, relPath)
		}
		if seen[relPath] {
			continue
		}
		seen[relPath] = true

		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read appendix %s: %w", relPath, err)
		}
		frontmatter, _, err := parseFrontmatter(string(content))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relPath, err)
		}
		if layer := strings.TrimSpace(frontmatter["layer"]); layer != "" && layer != currentLayer {
			return nil, fmt.Errorf("%s: appendix layer %q does not match main spec layer %q", relPath, layer, currentLayer)
		}
		if module := strings.TrimSpace(frontmatter["unit"]); module != "" && module != currentModule {
			return nil, fmt.Errorf("%s: appendix module %q does not match main spec module %q", relPath, module, currentModule)
		}
		appendixPrefix := strings.TrimSuffix(filepath.Base(relPath), ".md")
		appendixVersionRef := strings.TrimSpace(frontmatter["spec_version_ref"])
		appendixRef := appendixPrefix + "@unversioned"
		if appendixVersionRef != "" {
			appendixRef = appendixPrefix + "@" + appendixVersionRef
		}
		entries = append(entries, AppendixEntry{
			FileRef:     relPath,
			AppendixRef: appendixRef,
			Fingerprint: hashNormalizedText(string(content)),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].FileRef == entries[j].FileRef {
			return entries[i].AppendixRef < entries[j].AppendixRef
		}
		return entries[i].FileRef < entries[j].FileRef
	})
	return entries, nil
}

func buildSystemConstraintsSnapshot(repoRoot, body string) (string, string, string, error) {
	ref, _, err := parseSystemConstraintsRef(body)
	if err != nil {
		return "", "", "", err
	}
	if ref == "" || ref == "none" {
		return "none", "none", "none", nil
	}
	if !strings.HasPrefix(ref, "system_constraints@") {
		return "", "", "", fmt.Errorf("unsupported system_constraints_ref %q", ref)
	}

	systemFileRef := specpaths.SystemConstraintsFileRef
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
	return systemFileRef, fmt.Sprintf("system_constraints@%s", systemVersion), hashNormalizedText(string(systemContent)), nil
}

func buildSharedContractSnapshot(repoRoot, moduleLayer, body string) ([]SharedContractEntry, error) {
	refs, hasField, err := parseSharedContractRefs(body)
	if err != nil {
		return nil, err
	}
	if hasField && len(refs) == 0 {
		return []SharedContractEntry{}, nil
	}
	entries := make([]SharedContractEntry, 0, len(refs))
	for _, ref := range refs {
		resolved, err := sharedbinding.ResolveRef(repoRoot, moduleLayer, ref)
		if err != nil {
			return nil, err
		}
		entries = append(entries, SharedContractEntry{
			SharedContractID: resolved.SharedContractID,
			Layer:            resolved.Layer,
			FileRef:          resolved.FileRef,
			VersionRef:       resolved.VersionRef,
			Fingerprint:      hashNormalizedText(resolved.Content),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SharedContractID != entries[j].SharedContractID {
			return entries[i].SharedContractID < entries[j].SharedContractID
		}
		if entries[i].Layer != entries[j].Layer {
			return entries[i].Layer < entries[j].Layer
		}
		return entries[i].FileRef < entries[j].FileRef
	})
	return entries, nil
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

	result := map[string]string{}
	for _, line := range lines[1:endIdx] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	body := strings.Join(lines[endIdx+1:], "\n")
	return result, body, nil
}

func hashNormalizedText(content string) string {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	text += "\n"
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
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
			if strings.HasPrefix(nextTrimmed, "## ") || regexp.MustCompile(`^\d+\.`).MatchString(nextTrimmed) {
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
		if err := validateOrderedSharedContractRefs(refs); err != nil {
			return nil, false, err
		}
		return refs, true, nil
	}
	return nil, false, nil
}

func validateOrderedSharedContractRefs(refs []string) error {
	if len(refs) < 2 {
		return nil
	}
	expected := append([]string(nil), refs...)
	sort.Strings(expected)
	for idx := range refs {
		if refs[idx] != expected[idx] {
			return fmt.Errorf("shared_contract_refs must be sorted by exact shared ref string in ascending lexical order")
		}
	}
	return nil
}

func parseSystemConstraintsRef(body string) (string, bool, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		right, matched, err := parseNamedFieldLine(trimmed, "system_constraints_ref")
		if err != nil {
			return "", false, err
		}
		if !matched {
			continue
		}
		value := strings.Trim(right, "`")
		if value == "" {
			return "", false, fmt.Errorf("system_constraints_ref is empty")
		}
		return value, true, nil
	}
	return "", false, nil
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

func mainSpecLayer(mainSpecRef string) string {
	base := strings.TrimSuffix(filepath.Base(mainSpecRef), ".md")
	if strings.HasPrefix(base, "c_") {
		return "candidate"
	}
	return "stable"
}

func mainSpecModule(mainSpecRef string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(mainSpecRef), ".md")
	switch {
	case strings.HasPrefix(base, "c_unit_"):
		return strings.TrimPrefix(base, "c_unit_"), nil
	case strings.HasPrefix(base, "s_unit_"):
		return strings.TrimPrefix(base, "s_unit_"), nil
	default:
		return "", fmt.Errorf("unsupported main spec file ref %q", mainSpecRef)
	}
}

type processSnapshot struct {
	presentFields   map[string]bool
	scalars         map[string]string
	appendixEntries []AppendixEntry
	appendixPresent bool
	moduleEntries   []ObjectSnapshotEntry
	modulePresent   bool
	flowEntries     []ObjectSnapshotEntry
	flowPresent     bool
	sharedEntries   []SharedContractEntry
	sharedPresent   bool
}

func parseProcessSnapshot(content string) (processSnapshot, error) {
	result := processSnapshot{
		presentFields: map[string]bool{},
		scalars:       map[string]string{},
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	currentList := ""
	currentIndex := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingSpaceCount(line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			continue
		}

		if indent == 0 {
			currentList = ""
			currentIndex = -1
			key, value, ok := parseSnapshotFieldLine(trimmed)
			if !ok {
				continue
			}
			result.presentFields[key] = true
			if value == "" {
				switch key {
				case "unit_appendix_snapshot":
					result.appendixPresent = true
					currentList = key
				case "unit_snapshot":
					result.modulePresent = true
					currentList = key
				case "scenario_snapshot":
					result.flowPresent = true
					currentList = key
				case "shared_contract_snapshot":
					result.sharedPresent = true
					currentList = key
				}
				continue
			}
			result.scalars[key] = value
			continue
		}

		if indent == 2 {
			key, value, ok := parseSnapshotFieldLine(trimmed)
			if !ok {
				continue
			}
			switch currentList {
			case "unit_appendix_snapshot":
				if currentIndex < 0 || key == "file_ref" {
					result.appendixEntries = append(result.appendixEntries, AppendixEntry{})
					currentIndex = len(result.appendixEntries) - 1
				}
				assignAppendixField(&result.appendixEntries[currentIndex], key, value)
			case "unit_snapshot":
				if currentIndex < 0 || key == "unit" {
					result.moduleEntries = append(result.moduleEntries, ObjectSnapshotEntry{})
					currentIndex = len(result.moduleEntries) - 1
				}
				assignObjectSnapshotField(&result.moduleEntries[currentIndex], key, value)
			case "scenario_snapshot":
				if currentIndex < 0 || key == "scenario" {
					result.flowEntries = append(result.flowEntries, ObjectSnapshotEntry{})
					currentIndex = len(result.flowEntries) - 1
				}
				assignObjectSnapshotField(&result.flowEntries[currentIndex], key, value)
			case "shared_contract_snapshot":
				if currentIndex < 0 || key == "shared_contract_id" {
					result.sharedEntries = append(result.sharedEntries, SharedContractEntry{})
					currentIndex = len(result.sharedEntries) - 1
				}
				assignSharedField(&result.sharedEntries[currentIndex], key, value)
			}
			continue
		}

		if indent >= 4 && currentIndex >= 0 {
			key, value, ok := parseSnapshotFieldLine(trimmed)
			if !ok {
				continue
			}
			switch currentList {
			case "unit_appendix_snapshot":
				assignAppendixField(&result.appendixEntries[currentIndex], key, value)
			case "unit_snapshot":
				assignObjectSnapshotField(&result.moduleEntries[currentIndex], key, value)
			case "scenario_snapshot":
				assignObjectSnapshotField(&result.flowEntries[currentIndex], key, value)
			case "shared_contract_snapshot":
				assignSharedField(&result.sharedEntries[currentIndex], key, value)
			}
		}
	}

	if raw, ok := result.scalars["unit_appendix_snapshot"]; ok && raw == "none" {
		result.appendixPresent = true
		result.appendixEntries = nil
	}
	if raw, ok := result.scalars["unit_snapshot"]; ok && raw == "none" {
		result.modulePresent = true
		result.moduleEntries = nil
	}
	if raw, ok := result.scalars["scenario_snapshot"]; ok && raw == "none" {
		result.flowPresent = true
		result.flowEntries = nil
	}
	if raw, ok := result.scalars["shared_contract_snapshot"]; ok && raw == "none" {
		result.sharedPresent = true
		result.sharedEntries = nil
	}
	return result, nil
}

func parseSnapshotFieldLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "- ")
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := normalizeFieldKey(strings.TrimSpace(parts[0]))
	if key == "" {
		return "", "", false
	}
	value := strings.Trim(strings.TrimSpace(parts[1]), "`")
	return key, value, true
}

func leadingSpaceCount(line string) int {
	count := 0
	for count < len(line) && line[count] == ' ' {
		count++
	}
	return count
}

func splitKeyValue(line string) (string, string) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	key := strings.TrimSpace(parts[0])
	value := strings.Trim(strings.TrimSpace(parts[1]), "`")
	return key, value
}

func assignAppendixField(entry *AppendixEntry, key, value string) {
	switch key {
	case "file_ref":
		entry.FileRef = value
	case "appendix_ref":
		entry.AppendixRef = value
	case "fingerprint":
		entry.Fingerprint = value
	}
}

func assignSharedField(entry *SharedContractEntry, key, value string) {
	switch key {
	case "shared_contract_id":
		entry.SharedContractID = value
	case "layer":
		entry.Layer = value
	case "file_ref":
		entry.FileRef = value
	case "version_ref":
		entry.VersionRef = value
	case "fingerprint":
		entry.Fingerprint = value
	}
}

func assignObjectSnapshotField(entry *ObjectSnapshotEntry, key, value string) {
	switch key {
	case "unit", "scenario":
		entry.ObjectRef = value
	case "layer":
		entry.Layer = value
	case "file_ref":
		entry.FileRef = value
	case "version_ref":
		entry.VersionRef = value
	case "fingerprint":
		entry.Fingerprint = value
	}
}

func compareScalar(result *ValidationResult, field, actual, expected string) {
	if actual == "" {
		return
	}
	if actual != expected {
		result.Valid = false
		result.Mismatches = append(result.Mismatches, fmt.Sprintf("%s mismatch: actual=%s expected=%s", field, actual, expected))
	}
}

func normalizeAppendixList(entries []AppendixEntry) string {
	if len(entries) == 0 {
		return "none"
	}
	items := make([]AppendixEntry, len(entries))
	copy(items, entries)
	sort.Slice(items, func(i, j int) bool {
		if items[i].FileRef == items[j].FileRef {
			return items[i].AppendixRef < items[j].AppendixRef
		}
		return items[i].FileRef < items[j].FileRef
	})
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s|%s|%s", item.FileRef, item.AppendixRef, item.Fingerprint))
	}
	return strings.Join(parts, ";")
}

func normalizeSharedList(entries []SharedContractEntry) string {
	if len(entries) == 0 {
		return "none"
	}
	items := make([]SharedContractEntry, len(entries))
	copy(items, entries)
	sort.Slice(items, func(i, j int) bool {
		if items[i].SharedContractID != items[j].SharedContractID {
			return items[i].SharedContractID < items[j].SharedContractID
		}
		if items[i].Layer != items[j].Layer {
			return items[i].Layer < items[j].Layer
		}
		return items[i].FileRef < items[j].FileRef
	})
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s|%s|%s|%s|%s", item.SharedContractID, item.Layer, item.FileRef, item.VersionRef, item.Fingerprint))
	}
	return strings.Join(parts, ";")
}

func copyStringBoolMap(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func renderAppendixLines(entries []AppendixEntry) []string {
	if len(entries) == 0 {
		return []string{"  none"}
	}
	lines := []string{}
	for _, entry := range entries {
		lines = append(lines,
			fmt.Sprintf("  - file_ref: %s", entry.FileRef),
			fmt.Sprintf("    appendix_ref: %s", entry.AppendixRef),
			fmt.Sprintf("    fingerprint: %s", entry.Fingerprint),
		)
	}
	return lines
}

func renderSharedLines(entries []SharedContractEntry) []string {
	if len(entries) == 0 {
		return []string{"  none"}
	}
	lines := []string{}
	for _, entry := range entries {
		lines = append(lines,
			fmt.Sprintf("  - shared_contract_id: %s", entry.SharedContractID),
			fmt.Sprintf("    layer: %s", entry.Layer),
			fmt.Sprintf("    file_ref: %s", entry.FileRef),
			fmt.Sprintf("    version_ref: %s", entry.VersionRef),
			fmt.Sprintf("    fingerprint: %s", entry.Fingerprint),
		)
	}
	return lines
}

func ActivePlanFilePath(module string) string {
	return fmt.Sprintf("docs/specs/_plans/active/%s.md", module)
}

func DraftPlanFilePath(module string) string {
	return fmt.Sprintf("docs/specs/_plans/draft/%s.md", module)
}

func CheckResultFilePath(objectType, object string) string {
	return fmt.Sprintf("docs/specs/_check_result/%s/%s.md", objectType, object)
}

func VerifyResultFilePath(objectType, object string) string {
	return fmt.Sprintf("docs/specs/_verify_result/%s/%s.md", objectType, object)
}

func ProcessArtifactPaths(objectType, object, processKind string) ([]string, error) {
	switch processKind {
	case "check":
		return []string{CheckResultFilePath(objectType, object)}, nil
	case "plan":
		if objectType != "unit" {
			return nil, fmt.Errorf("process kind %q is not supported for object type %q", processKind, objectType)
		}
		return []string{DraftPlanFilePath(object), ActivePlanFilePath(object)}, nil
	case "verify":
		return []string{VerifyResultFilePath(objectType, object)}, nil
	default:
		return nil, fmt.Errorf("unsupported process kind %q", processKind)
	}
}

func ProcessFilePath(objectType, object, processKind string) (string, error) {
	switch processKind {
	case "check":
		return CheckResultFilePath(objectType, object), nil
	case "plan":
		if objectType != "unit" {
			return "", fmt.Errorf("process kind %q is not supported for object type %q", processKind, objectType)
		}
		return ActivePlanFilePath(object), nil
	case "verify":
		return VerifyResultFilePath(objectType, object), nil
	default:
		return "", fmt.Errorf("unsupported process kind %q", processKind)
	}
}

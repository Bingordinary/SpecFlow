package snapshot

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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

type Snapshot struct {
	Module                             string
	SpecFileRef                        string
	SpecVersionRef                     string
	SpecFingerprint                    string
	ModuleAppendixSnapshot             []AppendixEntry
	SystemConstraintsStableFileRef     string
	SystemConstraintsStableVersionRef  string
	SystemConstraintsStableFingerprint string
	SharedContractSnapshot             []SharedContractEntry
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
	Scalars                map[string]string
	ModuleAppendixSnapshot []AppendixEntry
	SharedContractSnapshot []SharedContractEntry
}

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

var requiredProcessSnapshotFields = map[string][]string{
	"check": {
		"spec_file_ref",
		"spec_version_ref",
		"spec_fingerprint",
		"module_appendix_snapshot",
		"system_constraints_stable_file_ref",
		"system_constraints_stable_version_ref",
		"system_constraints_stable_fingerprint",
		"shared_contract_snapshot",
	},
	"plan": {
		"spec_file_ref",
		"spec_version_ref",
		"spec_fingerprint",
		"module_appendix_snapshot",
		"system_constraints_stable_file_ref",
		"system_constraints_stable_version_ref",
		"system_constraints_stable_fingerprint",
		"shared_contract_snapshot",
	},
	"verify": {
		"spec_file_ref",
		"spec_version_ref",
		"spec_fingerprint",
		"module_appendix_snapshot",
		"verification_scope_ref",
		"system_constraints_stable_file_ref",
		"system_constraints_stable_version_ref",
		"system_constraints_stable_fingerprint",
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
	result.SystemConstraintsStableFileRef = systemFileRef
	result.SystemConstraintsStableVersionRef = systemVersionRef
	result.SystemConstraintsStableFingerprint = systemFingerprint

	sharedEntries, err := buildSharedContractSnapshot(repoRoot, body)
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

	processFile, err := ProcessFilePath(module, processKind)
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
		}
	}

	compareScalar(&result, "spec_file_ref", actual.scalars["spec_file_ref"], expected.SpecFileRef)
	compareScalar(&result, "spec_version_ref", actual.scalars["spec_version_ref"], expected.SpecVersionRef)
	compareScalar(&result, "spec_fingerprint", actual.scalars["spec_fingerprint"], expected.SpecFingerprint)
	compareScalar(&result, "system_constraints_stable_file_ref", actual.scalars["system_constraints_stable_file_ref"], expected.SystemConstraintsStableFileRef)
	compareScalar(&result, "system_constraints_stable_version_ref", actual.scalars["system_constraints_stable_version_ref"], expected.SystemConstraintsStableVersionRef)
	compareScalar(&result, "system_constraints_stable_fingerprint", actual.scalars["system_constraints_stable_fingerprint"], expected.SystemConstraintsStableFingerprint)

	if _, ok := actual.scalars["module_appendix_snapshot"]; ok || actual.appendixPresent {
		actualAppendix := normalizeAppendixList(actual.appendixEntries)
		expectedAppendix := normalizeAppendixList(expected.ModuleAppendixSnapshot)
		if actualAppendix != expectedAppendix {
			result.Valid = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("module_appendix_snapshot mismatch: actual=%s expected=%s", actualAppendix, expectedAppendix))
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

func LoadProcessSnapshot(repoRoot, module, processKind string) (ProcessSnapshotData, error) {
	processFile, err := ProcessFilePath(module, processKind)
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
		Scalars:                scalars,
		ModuleAppendixSnapshot: append([]AppendixEntry(nil), parsed.appendixEntries...),
		SharedContractSnapshot: append([]SharedContractEntry(nil), parsed.sharedEntries...),
	}, nil
}

func Render(snapshot Snapshot) string {
	lines := []string{
		fmt.Sprintf("module: %s", snapshot.Module),
		fmt.Sprintf("spec_file_ref: %s", snapshot.SpecFileRef),
		fmt.Sprintf("spec_version_ref: %s", snapshot.SpecVersionRef),
		fmt.Sprintf("spec_fingerprint: %s", snapshot.SpecFingerprint),
		fmt.Sprintf("system_constraints_stable_file_ref: %s", snapshot.SystemConstraintsStableFileRef),
		fmt.Sprintf("system_constraints_stable_version_ref: %s", snapshot.SystemConstraintsStableVersionRef),
		fmt.Sprintf("system_constraints_stable_fingerprint: %s", snapshot.SystemConstraintsStableFingerprint),
		"module_appendix_snapshot:",
	}
	lines = append(lines, renderAppendixLines(snapshot.ModuleAppendixSnapshot)...)
	lines = append(lines, "shared_contract_snapshot:")
	lines = append(lines, renderSharedLines(snapshot.SharedContractSnapshot)...)
	return strings.Join(lines, "\n")
}

func buildAppendixSnapshot(repoRoot, mainSpecRef, body string) ([]AppendixEntry, error) {
	mainDir := filepath.Dir(filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	seen := map[string]bool{}
	entries := []AppendixEntry{}
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(body, -1) {
		if len(match) != 2 {
			continue
		}
		destination := strings.TrimSpace(match[1])
		if !strings.HasPrefix(destination, "./appendix/") {
			continue
		}
		absPath := filepath.Clean(filepath.Join(mainDir, filepath.FromSlash(destination)))
		relPath, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return nil, err
		}
		relPath = filepath.ToSlash(relPath)
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
		appendixRef := frontmatter["spec_version_ref"]
		if strings.TrimSpace(appendixRef) == "" {
			appendixRef = strings.TrimSuffix(filepath.Base(relPath), ".md") + "@unversioned"
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

func buildSharedContractSnapshot(repoRoot, body string) ([]SharedContractEntry, error) {
	refs, hasField, err := parseSharedContractRefs(body)
	if err != nil {
		return nil, err
	}
	if hasField && len(refs) == 0 {
		return []SharedContractEntry{}, nil
	}
	entries := make([]SharedContractEntry, 0, len(refs))
	for _, ref := range refs {
		fileRef, err := sharedFileRefFromVersionRef(ref)
		if err != nil {
			return nil, err
		}
		absPath := filepath.Join(repoRoot, filepath.FromSlash(fileRef))
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read shared contract %s: %w", fileRef, err)
		}
		frontmatter, _, err := parseFrontmatter(string(content))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fileRef, err)
		}
		sharedID := strings.TrimSpace(frontmatter["shared_contract_id"])
		layer := strings.TrimSpace(frontmatter["layer"])
		sharedVersion := strings.TrimSpace(frontmatter["shared_version"])
		if sharedID == "" || layer == "" || sharedVersion == "" {
			return nil, fmt.Errorf("%s: missing shared_contract_id/layer/shared_version", fileRef)
		}
		entries = append(entries, SharedContractEntry{
			SharedContractID: sharedID,
			Layer:            layer,
			FileRef:          fileRef,
			VersionRef:       fmt.Sprintf("%s@%s", strings.TrimSuffix(filepath.Base(fileRef), ".md"), sharedVersion),
			Fingerprint:      hashNormalizedText(string(content)),
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
			if strings.HasPrefix(nextTrimmed, "## ") || regexp.MustCompile(`^\d+\.`).MatchString(nextTrimmed) {
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

func parseSystemConstraintsStableRef(body string) (string, bool, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "`system_constraints_stable_ref`") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return "", false, fmt.Errorf("system_constraints_stable_ref line is malformed")
		}
		value := strings.Trim(strings.TrimSpace(parts[1]), "`")
		if value == "" {
			return "", false, fmt.Errorf("system_constraints_stable_ref is empty")
		}
		return value, true, nil
	}
	return "", false, nil
}

func sharedFileRefFromVersionRef(ref string) (string, error) {
	parts := strings.SplitN(ref, "@", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("invalid shared contract ref %q", ref)
	}
	prefix := strings.TrimSpace(parts[0])
	switch {
	case strings.HasPrefix(prefix, "c_"):
		return fmt.Sprintf("docs/specs/shared_contracts/candidate/%s.md", prefix), nil
	case strings.HasPrefix(prefix, "s_"):
		return fmt.Sprintf("docs/specs/shared_contracts/stable/%s.md", prefix), nil
	default:
		return "", fmt.Errorf("invalid shared contract ref prefix %q", ref)
	}
}

type processSnapshot struct {
	presentFields   map[string]bool
	scalars         map[string]string
	appendixEntries []AppendixEntry
	appendixPresent bool
	sharedEntries   []SharedContractEntry
	sharedPresent   bool
}

func parseProcessSnapshot(content string) (processSnapshot, error) {
	block, err := extractFirstYAMLBlock(content)
	if err != nil {
		return processSnapshot{}, err
	}

	result := processSnapshot{
		presentFields: map[string]bool{},
		scalars:       map[string]string{},
	}
	lines := strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n")
	currentList := ""
	currentIndex := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			currentList = ""
			currentIndex = -1
			parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			result.presentFields[key] = true
			value = strings.Trim(value, "`")
			if value == "" {
				switch key {
				case "module_appendix_snapshot":
					result.appendixPresent = true
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

		if strings.HasPrefix(line, "  - ") {
			payload := strings.TrimPrefix(line, "  - ")
			key, value := splitKeyValue(payload)
			switch currentList {
			case "module_appendix_snapshot":
				result.appendixEntries = append(result.appendixEntries, AppendixEntry{})
				currentIndex = len(result.appendixEntries) - 1
				assignAppendixField(&result.appendixEntries[currentIndex], key, value)
			case "shared_contract_snapshot":
				result.sharedEntries = append(result.sharedEntries, SharedContractEntry{})
				currentIndex = len(result.sharedEntries) - 1
				assignSharedField(&result.sharedEntries[currentIndex], key, value)
			}
			continue
		}

		if strings.HasPrefix(line, "    ") && currentIndex >= 0 {
			key, value := splitKeyValue(strings.TrimSpace(line))
			switch currentList {
			case "module_appendix_snapshot":
				assignAppendixField(&result.appendixEntries[currentIndex], key, value)
			case "shared_contract_snapshot":
				assignSharedField(&result.sharedEntries[currentIndex], key, value)
			}
		}
	}

	if raw, ok := result.scalars["module_appendix_snapshot"]; ok && raw == "none" {
		result.appendixPresent = true
		result.appendixEntries = nil
	}
	if raw, ok := result.scalars["shared_contract_snapshot"]; ok && raw == "none" {
		result.sharedPresent = true
		result.sharedEntries = nil
	}
	return result, nil
}

func extractFirstYAMLBlock(content string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inBlock := false
	block := []string{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "```yaml" {
			if inBlock {
				return "", fmt.Errorf("nested yaml block is not supported")
			}
			inBlock = true
			continue
		}
		if strings.TrimSpace(line) == "```" && inBlock {
			return strings.Join(block, "\n"), nil
		}
		if inBlock {
			block = append(block, line)
		}
	}
	return "", fmt.Errorf("yaml snapshot block not found")
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

func ProcessFilePath(module, processKind string) (string, error) {
	switch processKind {
	case "check":
		return fmt.Sprintf("docs/specs/_check_result/%s.md", module), nil
	case "plan":
		return fmt.Sprintf("docs/specs/_plans/%s.md", module), nil
	case "verify":
		return fmt.Sprintf("docs/specs/_verify_result/%s.md", module), nil
	default:
		return "", fmt.Errorf("unsupported process kind %q", processKind)
	}
}

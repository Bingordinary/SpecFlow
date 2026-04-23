package projectstandards

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

const relativeRegistryPath = "docs/project_standards/_registry.md"

type RegistryEntry struct {
	StandardID   string
	Type         string
	Surface      string
	File         string
	ConsumedBy   string
	AppliesTo    string
	Effect       string
	ConflictRule string
	Notes        string
	Row          int
}

type ValidationResult struct {
	Entries      []RegistryEntry
	ValidEntries []RegistryEntry
	Diagnostics  []string
	RegistryPath string
}

type AppliesSelector struct {
	Kind   string
	Values []string
}

type surfaceContract struct {
	StandardType      string
	AllowedEffects    map[string]bool
	AllowedKinds      map[string]bool
	AllowedScenarios  map[string]bool
	AllowAllOnSurface bool
}

var supportedTypes = map[string]bool{
	"review_standard":   true,
	"output_standard":   true,
	"decision_standard": true,
}

var contracts = map[string]map[string]surfaceContract{
	"module_check": {
		"candidate_closure_review": {
			StandardType: "review_standard",
			AllowedEffects: map[string]bool{
				"clarify": true,
				"tighten": true,
			},
			AllowedKinds: map[string]bool{
				"all_targets_on_surface": true,
				"module":                 true,
				"module_set":             true,
			},
			AllowAllOnSurface: true,
		},
	},
}

func RegistryPath(repoRoot string) string {
	return filepath.Join(repoRoot, relativeRegistryPath)
}

func ValidateRegistry(repoRoot string) (ValidationResult, error) {
	result := ValidationResult{
		RegistryPath: relativeRegistryPath,
	}

	path := RegistryPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read %s: %w", relativeRegistryPath, err)
	}

	entries, err := parseRegistryTable(string(data))
	if err != nil {
		return result, fmt.Errorf("parse %s: %w", relativeRegistryPath, err)
	}
	result.Entries = entries

	modules, err := statusfile.LoadModules(repoRoot)
	if err != nil {
		return result, err
	}
	moduleSet := make(map[string]bool, len(modules))
	for _, module := range modules {
		moduleSet[module] = true
	}

	idCounts := map[string]int{}
	for _, entry := range entries {
		if entry.StandardID != "" {
			idCounts[entry.StandardID]++
		}
	}

	seenIDs := map[string]bool{}
	for _, entry := range entries {
		rowPrefix := fmt.Sprintf("%s row %d", relativeRegistryPath, entry.Row)
		entryValid := true
		if entry.StandardID == "" {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: standard_id is required", rowPrefix))
			entryValid = false
		} else if idCounts[entry.StandardID] > 1 {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: duplicate standard_id %q", rowPrefix, entry.StandardID))
			entryValid = false
		} else if seenIDs[entry.StandardID] {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: duplicate standard_id %q", rowPrefix, entry.StandardID))
			entryValid = false
		} else {
			seenIDs[entry.StandardID] = true
		}

		if !supportedTypes[entry.Type] {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: unsupported type %q", rowPrefix, entry.Type))
			entryValid = false
		}
		if entry.File == "" || !strings.HasPrefix(entry.File, "docs/project_standards/") {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: file must stay under docs/project_standards/", rowPrefix))
			entryValid = false
		} else if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(entry.File))); err != nil {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: file %q does not exist", rowPrefix, entry.File))
			entryValid = false
		}
		if entry.ConsumedBy == "" || entry.ConsumedBy == "all" {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: consumed_by must name one supported command or internal flow", rowPrefix))
			entryValid = false
		}
		if entry.ConflictRule != "framework_wins" {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: conflict_rule must be framework_wins", rowPrefix))
			entryValid = false
		}

		selector, selectorErr := ParseAppliesTo(entry.AppliesTo)
		if selectorErr != nil {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: %v", rowPrefix, selectorErr))
			entryValid = false
		}

		contract, ok := lookupContract(entry.ConsumedBy, entry.Surface)
		if !ok {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: unsupported consumed_by/surface pair %q/%q", rowPrefix, entry.ConsumedBy, entry.Surface))
			entryValid = false
			continue
		}
		if entry.Type != contract.StandardType {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: surface %q for %q only accepts %q", rowPrefix, entry.Surface, entry.ConsumedBy, contract.StandardType))
			entryValid = false
		}
		if !contract.AllowedEffects[entry.Effect] {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: effect %q is not allowed on %q/%q", rowPrefix, entry.Effect, entry.ConsumedBy, entry.Surface))
			entryValid = false
		}
		if selectorErr != nil {
			continue
		}
		if !contract.AllowedKinds[selector.Kind] {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: applies_to kind %q is not allowed on %q/%q", rowPrefix, selector.Kind, entry.ConsumedBy, entry.Surface))
			entryValid = false
			continue
		}
		switch selector.Kind {
		case "module":
			if !moduleSet[selector.Values[0]] {
				result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: unknown formal module %q", rowPrefix, selector.Values[0]))
				entryValid = false
			}
		case "module_set":
			for _, module := range selector.Values {
				if !moduleSet[module] {
					result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: unknown formal module %q in module_set", rowPrefix, module))
					entryValid = false
				}
			}
		case "review_scenario":
			if !contract.AllowedScenarios[selector.Values[0]] {
				result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%s: unsupported review scenario %q", rowPrefix, selector.Values[0]))
				entryValid = false
			}
		}

		if entryValid {
			result.ValidEntries = append(result.ValidEntries, entry)
		}
	}

	sort.Strings(result.Diagnostics)
	return result, nil
}

func LoadActiveEntries(repoRoot string) ([]RegistryEntry, error) {
	result, err := ValidateRegistry(repoRoot)
	if err != nil {
		return nil, err
	}
	if len(result.Diagnostics) > 0 {
		return nil, fmt.Errorf(strings.Join(result.Diagnostics, "; "))
	}
	return result.ValidEntries, nil
}

func ParseAppliesTo(raw string) (AppliesSelector, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AppliesSelector{}, fmt.Errorf("applies_to is required")
	}
	if raw == "all_targets_on_surface" {
		return AppliesSelector{Kind: raw}, nil
	}
	if strings.HasPrefix(raw, "module:") {
		value := strings.TrimSpace(strings.TrimPrefix(raw, "module:"))
		if value == "" {
			return AppliesSelector{}, fmt.Errorf("module selector requires one formal module name")
		}
		return AppliesSelector{Kind: "module", Values: []string{value}}, nil
	}
	if strings.HasPrefix(raw, "module_set:") {
		value := strings.TrimSpace(strings.TrimPrefix(raw, "module_set:"))
		if value == "" {
			return AppliesSelector{}, fmt.Errorf("module_set selector requires at least one formal module name")
		}
		if strings.Contains(value, " ") {
			return AppliesSelector{}, fmt.Errorf("module_set selector must not contain spaces")
		}
		values := strings.Split(value, ",")
		for _, item := range values {
			if item == "" {
				return AppliesSelector{}, fmt.Errorf("module_set selector contains an empty module name")
			}
		}
		return AppliesSelector{Kind: "module_set", Values: values}, nil
	}
	if strings.HasPrefix(raw, "review_scenario:") {
		value := strings.TrimSpace(strings.TrimPrefix(raw, "review_scenario:"))
		if value == "" {
			return AppliesSelector{}, fmt.Errorf("review_scenario selector requires one stable scenario name")
		}
		return AppliesSelector{Kind: "review_scenario", Values: []string{value}}, nil
	}
	return AppliesSelector{}, fmt.Errorf("unsupported applies_to selector %q", raw)
}

func parseRegistryTable(content string) ([]RegistryEntry, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	start := -1
	for idx, line := range lines {
		if strings.TrimSpace(line) == "## Active Standards" {
			start = idx + 1
			break
		}
	}
	if start == -1 {
		return nil, fmt.Errorf("cannot find ## Active Standards section")
	}

	headerIndex := -1
	for idx := start; idx < len(lines); idx++ {
		if strings.HasPrefix(strings.TrimSpace(lines[idx]), "|") {
			headerIndex = idx
			break
		}
	}
	if headerIndex == -1 || headerIndex+1 >= len(lines) {
		return nil, fmt.Errorf("cannot find registry table")
	}

	headerCells, ok := parseTableLine(lines[headerIndex])
	if !ok {
		return nil, fmt.Errorf("invalid registry table header")
	}
	columnMap := make(map[string]int, len(headerCells))
	for idx, cell := range headerCells {
		columnMap[cell] = idx
	}
	requiredColumns := []string{"standard_id", "type", "surface", "file", "consumed_by", "applies_to", "effect", "conflict_rule", "notes"}
	for _, column := range requiredColumns {
		if _, ok := columnMap[column]; !ok {
			return nil, fmt.Errorf("missing required column %q", column)
		}
	}

	entries := []RegistryEntry{}
	for idx := headerIndex + 2; idx < len(lines); idx++ {
		cells, ok := parseTableLine(lines[idx])
		if !ok {
			break
		}
		if isSeparatorRow(cells) {
			continue
		}
		if len(cells) != len(headerCells) {
			return nil, fmt.Errorf("row %d column count does not match header", idx+1)
		}
		entry := RegistryEntry{
			StandardID:   normalizeCell(cells[columnMap["standard_id"]]),
			Type:         normalizeCell(cells[columnMap["type"]]),
			Surface:      normalizeCell(cells[columnMap["surface"]]),
			File:         normalizeCell(cells[columnMap["file"]]),
			ConsumedBy:   normalizeCell(cells[columnMap["consumed_by"]]),
			AppliesTo:    normalizeCell(cells[columnMap["applies_to"]]),
			Effect:       normalizeCell(cells[columnMap["effect"]]),
			ConflictRule: normalizeCell(cells[columnMap["conflict_rule"]]),
			Notes:        strings.TrimSpace(cells[columnMap["notes"]]),
			Row:          idx + 1,
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseTableLine(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil, false
	}
	parts := strings.Split(trimmed, "|")
	if len(parts) < 3 {
		return nil, false
	}
	cells := make([]string, 0, len(parts)-2)
	for _, part := range parts[1 : len(parts)-1] {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells, true
}

func isSeparatorRow(cells []string) bool {
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		cell = strings.Trim(cell, "-")
		if cell != "" {
			return false
		}
	}
	return true
}

func normalizeCell(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "`") && strings.HasSuffix(value, "`") && len(value) >= 2 {
		value = value[1 : len(value)-1]
	}
	return strings.TrimSpace(value)
}

func lookupContract(consumedBy, surface string) (surfaceContract, bool) {
	byConsumer, ok := contracts[consumedBy]
	if !ok {
		return surfaceContract{}, false
	}
	contract, ok := byConsumer[surface]
	return contract, ok
}

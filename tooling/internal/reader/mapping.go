package reader

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type repositoryMapping struct {
	Units       map[string]mappingUnit
	Rules       map[string]mappingShared
	Diagnostics []Diagnostic
}

type mappingUnit struct {
	ID                  string
	Responsibility      string
	TruthSurfaceRule    string
	ImplementationPaths []SourceRef
}

type mappingShared struct {
	ID             string
	Responsibility string
	TruthPaths     []SourceRef
}

var numberedCodeSpan = regexp.MustCompile("^\\s*\\d+\\.\\s+`([^`]+)`\\s*$")

func loadRepositoryMapping(repoRoot string) repositoryMapping {
	relPath := "docs/specs/repository_mapping.md"
	path := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	data, err := os.ReadFile(path)
	result := repositoryMapping{
		Units: map[string]mappingUnit{},
		Rules: map[string]mappingShared{},
	}
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Severity: "error",
			Message:  "cannot read repository mapping: " + err.Error(),
			Source:   &SourceRef{Path: relPath},
		})
		return result
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	currentID := ""
	currentPathKind := ""
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "### 2.1 "):
			section = "unit_map"
			currentID = ""
			continue
		case strings.HasPrefix(trimmed, "### 2.2 "):
			section = ""
			currentID = ""
			continue
		case strings.HasPrefix(trimmed, "### 2.3 "):
			section = "shared_map"
			currentID = ""
			continue
		case strings.HasPrefix(trimmed, "## 3.") || strings.HasPrefix(trimmed, "### 4.1 "):
			section = ""
			currentID = ""
			continue
		case strings.HasPrefix(trimmed, "### 4.5 "):
			section = "shared_paths"
			currentID = ""
			currentPathKind = ""
			continue
		case strings.HasPrefix(trimmed, "### 4.6 "):
			section = "unit_paths"
			currentID = ""
			currentPathKind = ""
			continue
		case strings.HasPrefix(trimmed, "### 4.7 ") || strings.HasPrefix(trimmed, "## 5."):
			section = ""
			currentID = ""
			currentPathKind = ""
			continue
		}

		switch section {
		case "unit_map":
			if id, ok := parseNumberedCodeSpan(trimmed); ok {
				currentID = id
				unit := result.Units[id]
				unit.ID = id
				result.Units[id] = unit
				continue
			}
			if currentID != "" && strings.HasPrefix(trimmed, "- ") {
				unit := result.Units[currentID]
				unit.Responsibility = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				result.Units[currentID] = unit
			}
		case "shared_map":
			if id, ok := parseNumberedCodeSpan(trimmed); ok {
				currentID = normalizedBoundRuleID(id)
				shared := result.Rules[currentID]
				shared.ID = currentID
				result.Rules[currentID] = shared
				continue
			}
			if currentID != "" && strings.HasPrefix(trimmed, "- ") {
				shared := result.Rules[currentID]
				shared.Responsibility = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				result.Rules[currentID] = shared
			}
		case "shared_paths":
			if id, ok := parseNumberedCodeSpan(trimmed); ok {
				currentID = normalizedBoundRuleID(id)
				shared := result.Rules[currentID]
				shared.ID = currentID
				result.Rules[currentID] = shared
				continue
			}
			if currentID != "" && strings.HasPrefix(trimmed, "- `") {
				if pathRef, ok := parseListCodePath(trimmed, relPath, idx+1); ok {
					shared := result.Rules[currentID]
					shared.TruthPaths = append(shared.TruthPaths, pathRef)
					result.Rules[currentID] = shared
				}
			}
		case "unit_paths":
			if id, ok := parseNumberedCodeSpan(trimmed); ok {
				if _, known := result.Units[id]; known {
					currentID = id
				} else {
					currentID = ""
				}
				currentPathKind = ""
				continue
			}
			if currentID == "" {
				continue
			}
			switch {
			case strings.Contains(trimmed, "truth_surface_rule"):
				currentPathKind = ""
				unit := result.Units[currentID]
				unit.TruthSurfaceRule = parseRuleValue(trimmed)
				result.Units[currentID] = unit
				continue
			case strings.Contains(trimmed, "`implementation_surface`"):
				currentPathKind = "implementation"
				continue
			case strings.Contains(trimmed, "`implementation_realization_note`"):
				currentPathKind = ""
				continue
			case strings.HasPrefix(trimmed, "- `"):
				pathRef, ok := parseListCodePath(trimmed, relPath, idx+1)
				if !ok {
					continue
				}
				unit := result.Units[currentID]
				if currentPathKind == "implementation" {
					unit.ImplementationPaths = append(unit.ImplementationPaths, pathRef)
				}
				result.Units[currentID] = unit
			}
		}
	}
	return result
}

func parseNumberedCodeSpan(line string) (string, bool) {
	matches := numberedCodeSpan.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return strings.TrimSpace(matches[1]), true
}

func parseListCodePath(line, sourcePath string, sourceLine int) (SourceRef, bool) {
	value := extractFirstCodeSpan(line)
	if value == "" {
		return SourceRef{}, false
	}
	return SourceRef{Path: filepath.ToSlash(value), Line: sourceLine, Label: sourcePath}, true
}

func parseRuleValue(line string) string {
	spans := codeSpans(line)
	if len(spans) >= 2 {
		return spans[1]
	}
	if idx := strings.Index(line, ":"); idx >= 0 {
		value := strings.TrimSpace(line[idx+1:])
		return strings.Trim(value, "` ")
	}
	return ""
}

func codeSpans(line string) []string {
	spans := []string{}
	rest := line
	for {
		start := strings.Index(rest, "`")
		if start < 0 {
			return spans
		}
		rest = rest[start+1:]
		end := strings.Index(rest, "`")
		if end < 0 {
			return spans
		}
		spans = append(spans, strings.TrimSpace(rest[:end]))
		rest = rest[end+1:]
	}
}

func extractFirstCodeSpan(line string) string {
	start := strings.Index(line, "`")
	if start < 0 {
		return ""
	}
	end := strings.Index(line[start+1:], "`")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(line[start+1 : start+1+end])
}

func normalizedBoundRuleID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.HasPrefix(id, "b_rule_") {
		return id
	}
	return "b_rule_" + id
}

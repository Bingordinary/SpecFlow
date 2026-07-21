package specpaths

import (
	"fmt"
	"strings"
)

// ReadFrontmatterStringMap extracts a flat string map from YAML frontmatter.
// Supports both inline list syntax (unit_refs: [a, b]) and block-style YAML
// lists. Block-style list items for the same key are accumulated into an
// inline-formatted string [a, b] for downstream parsing.
func ReadFrontmatterStringMap(text string) map[string]string {
	fm, _, _ := ParseFrontmatterFields(text)
	return fm
}

// ParseFrontmatterFields extracts frontmatter fields and body from markdown
// content with --- delimited YAML frontmatter. Returns the field map, the
// body text (everything after the closing ---), and an error if the
// frontmatter is malformed.
// Supports inline values (key: value), inline list syntax (key: [a, b]),
// and block-style YAML lists (key:\n  - a\n  - b).
func ParseFrontmatterFields(text string) (map[string]string, string, error) {
	normalized := NormalizeText(text)
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("missing frontmatter start marker")
	}

	frontmatterLines, bodyLines, err := extractFrontmatterLines(lines)
	if err != nil {
		return nil, "", err
	}

	fm := parseFrontmatterKeyValues(frontmatterLines)
	body := strings.Join(bodyLines, "\n")

	return fm, body, nil
}

func extractFrontmatterLines(lines []string) ([]string, []string, error) {
	endIdx := -1
	for idx := 1; idx < len(lines); idx++ {
		if strings.TrimSpace(lines[idx]) == "---" {
			endIdx = idx
			break
		}
	}
	if endIdx == -1 {
		return nil, nil, fmt.Errorf("missing frontmatter end marker")
	}
	return lines[1:endIdx], lines[endIdx+1:], nil
}

func parseFrontmatterKeyValues(frontmatterLines []string) map[string]string {
	result := map[string]string{}
	currentKey := ""
	for _, line := range frontmatterLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && currentKey != "" {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if item == "" {
				continue
			}
			existing := result[currentKey]
			if existing == "" {
				result[currentKey] = "[" + item + "]"
			} else if strings.HasPrefix(existing, "[") {
				result[currentKey] = existing[:len(existing)-1] + ", " + item + "]"
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			currentKey = key
			continue
		}
		value = strings.Trim(value, "`\"' ")
		result[key] = value
	}
	return result
}

// ParseRefList parses a YAML list or comma-separated string into ref tokens.
// Accepts both "[a, b, c]" and "a, b, c" formats.
func ParseRefList(value string) []string {
	value = strings.TrimSpace(value)

	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")

	parts := strings.Split(value, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"'")
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

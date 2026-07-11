package promote

import (
	"os"
	"strings"
)

// parseFrontmatter extracts YAML frontmatter fields from a markdown file.
// It handles the --- delimited frontmatter block at the start of the file.
func parseFrontmatter(content string) map[string]string {
	result := map[string]string{}

	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return result
	}

	lines := strings.Split(content, "\n")
	startIdx := -1
	endIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if startIdx == -1 {
				startIdx = i
			} else {
				endIdx = i
				break
			}
		}
	}

	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx+1 {
		return result
	}

	for _, line := range lines[startIdx+1 : endIdx] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		result[key] = value
	}

	return result
}

// copyWithLayerTransform reads src, transforms frontmatter layer from
// candidate to stable, and writes the result to dst. The directory of dst
// is created if it does not exist.
func copyWithLayerTransform(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	transformed := transformLayerInFrontmatter(string(data), "candidate", "stable")
	if err := os.MkdirAll(dirName(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(transformed), 0644)
}

// transformLayerInFrontmatter replaces the layer value within the frontmatter
// block (--- delimited) when it matches fromLayer. It preserves original
// spacing and quoting style. Only the frontmatter block is examined — body
// content is never touched.
func transformLayerInFrontmatter(content, fromLayer, toLayer string) string {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return content
	}

	lines := strings.Split(content, "\n")
	startIdx := -1
	endIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if startIdx == -1 {
				startIdx = i
			} else {
				endIdx = i
				break
			}
		}
	}

	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx+1 {
		return content
	}

	for i := startIdx + 1; i < endIdx; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "layer:") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		rawVal := parts[1]
		val := strings.TrimSpace(rawVal)
		stripped := strings.Trim(val, "\"'")
		if !strings.EqualFold(stripped, fromLayer) {
			continue
		}
		leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		trimmedVal := strings.TrimSpace(rawVal)
		quote := ""
		if len(trimmedVal) >= 2 {
			if trimmedVal[0] == '"' && trimmedVal[len(trimmedVal)-1] == '"' {
				quote = "\""
			} else if trimmedVal[0] == '\'' && trimmedVal[len(trimmedVal)-1] == '\'' {
				quote = "'"
			}
		}
		lines[i] = leading + "layer: " + quote + toLayer + quote
	}

	return strings.Join(lines, "\n")
}

func dirName(path string) string {
	idx := strings.LastIndex(path, string(os.PathSeparator))
	if idx == -1 {
		return "."
	}
	return path[:idx]
}

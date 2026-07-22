package promote

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// parseFrontmatter extracts YAML frontmatter fields from a markdown file.
func parseFrontmatter(content string) map[string]string {
	return specpaths.ReadFrontmatterStringMap(content)
}

// copyWithLayerTransform reads src, transforms frontmatter layer from
// candidate to stable, and writes the result to dst. The directory of dst
// is created if it does not exist.
func copyWithLayerTransform(src, dst string) error {
	return copyFileTransform(src, dst, "candidate", "stable")
}

// copyFileTransform reads src, transforms frontmatter layer from fromLayer
// to toLayer, and writes the result to dst.
func copyFileTransform(src, dst, fromLayer, toLayer string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	transformed := transformLayerInFrontmatter(string(data), fromLayer, toLayer)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
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

		// Transform the layer field
		if strings.HasPrefix(trimmed, "layer:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				rawVal := parts[1]
				val := strings.TrimSpace(rawVal)
				stripped := strings.Trim(val, "\"'")
				if strings.EqualFold(stripped, fromLayer) {
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
			}
			continue
		}

		// Transform evidence_appendix_ref field — rename c_unit_ to s_unit_
		// when promoting (candidate→stable) or reverse on fork (stable→candidate).
		if strings.HasPrefix(trimmed, "evidence_appendix_ref:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				rawVal := strings.TrimSpace(parts[1])
				oldPrefix := "c_unit_"
				newPrefix := "s_unit_"
				if fromLayer == "stable" {
					oldPrefix = "s_unit_"
					newPrefix = "c_unit_"
				}
				newVal := strings.Replace(rawVal, oldPrefix, newPrefix, 1)
				if newVal != rawVal {
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
					strippedNew := strings.Trim(strings.TrimSpace(newVal), "\"'")
					lines[i] = leading + "evidence_appendix_ref: " + quote + strippedNew + quote
				}
			}
			continue
		}
	}

	return strings.Join(lines, "\n")
}



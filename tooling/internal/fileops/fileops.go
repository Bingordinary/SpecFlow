package fileops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CopyFileTransform(src, dst, fromLayer, toLayer string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	transformed := TransformLayerInFrontmatter(string(data), fromLayer, toLayer)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(transformed), 0644)
}

func TransformLayerInFrontmatter(content, fromLayer, toLayer string) string {
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
	}

	return strings.Join(lines, "\n")
}

func VersionWithBumpPatch(version string) string {
	if version == "" {
		return "0.1.0"
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return version
	}
	major := parts[0]
	minor := parts[1]
	patch := parts[2]

	var patchVal int
	if _, err := fmt.Sscanf(patch, "%d", &patchVal); err != nil {
		return version
	}
	patchVal++
	return fmt.Sprintf("%s.%s.%d", major, minor, patchVal)
}

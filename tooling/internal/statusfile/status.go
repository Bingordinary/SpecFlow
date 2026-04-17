package statusfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const relativeStatusPath = "docs/specs/_status.md"

func LoadModules(repoRoot string) ([]string, error) {
	path := filepath.Join(repoRoot, relativeStatusPath)
	lines, _, err := readLines(path)
	if err != nil {
		return nil, err
	}

	start, end, err := findFormalModuleTable(lines)
	if err != nil {
		return nil, err
	}

	modules := make([]string, 0, end-start)
	for idx := start; idx < end; idx++ {
		cells, ok := parseTableLine(lines[idx])
		if !ok || len(cells) < 6 {
			continue
		}
		modules = append(modules, stripCodeSpan(cells[0]))
	}
	return modules, nil
}

func UpdateNextCommand(repoRoot, module, nextCommand string) (bool, error) {
	path := filepath.Join(repoRoot, relativeStatusPath)
	lines, hadTrailingNewline, err := readLines(path)
	if err != nil {
		return false, err
	}

	start, end, err := findFormalModuleTable(lines)
	if err != nil {
		return false, err
	}

	updated := false
	for idx := start; idx < end; idx++ {
		cells, ok := parseTableLine(lines[idx])
		if !ok || len(cells) < 6 {
			continue
		}
		if stripCodeSpan(cells[0]) != module {
			continue
		}
		cells[4] = fmt.Sprintf("`%s`", nextCommand)
		lines[idx] = formatTableLine(cells)
		updated = true
		break
	}

	if !updated {
		return false, fmt.Errorf("module %q not found in %s", module, relativeStatusPath)
	}

	content := strings.Join(lines, "\n")
	if hadTrailingNewline {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", relativeStatusPath, err)
	}

	return true, nil
}

func readLines(path string) ([]string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	hadTrailingNewline := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{}, hadTrailingNewline, nil
	}
	return strings.Split(text, "\n"), hadTrailingNewline, nil
}

func findFormalModuleTable(lines []string) (int, int, error) {
	for idx := range lines {
		cells, ok := parseTableLine(lines[idx])
		if !ok || len(cells) < 6 {
			continue
		}
		if cells[0] != "Module" || cells[4] != "Next Command" {
			continue
		}
		if idx+1 >= len(lines) {
			return 0, 0, fmt.Errorf("missing separator row in %s", relativeStatusPath)
		}
		start := idx + 2
		end := start
		for end < len(lines) {
			if _, ok := parseTableLine(lines[end]); !ok {
				break
			}
			end++
		}
		return start, end, nil
	}
	return 0, 0, fmt.Errorf("formal module table not found in %s", relativeStatusPath)
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

func formatTableLine(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

func stripCodeSpan(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "`") && strings.HasSuffix(value, "`") && len(value) >= 2 {
		return value[1 : len(value)-1]
	}
	return value
}

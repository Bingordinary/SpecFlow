package statusfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const relativeStatusPath = "docs/specs/_status.md"

type ModuleStatus struct {
	Module      string
	Stable      string
	Candidate   string
	ActiveLayer string
	NextCommand string
	Notes       string
}

var allowedNextCommands = map[string]bool{
	"spec_fork":     true,
	"stable_verify": true,
	"cand_check":    true,
	"cand_plan":     true,
	"cand_impl":     true,
	"cand_verify":   true,
	"cand_promote":  true,
}

func LoadModules(repoRoot string) ([]string, error) {
	statuses, err := LoadModuleStatuses(repoRoot)
	if err != nil {
		return nil, err
	}

	modules := make([]string, 0, len(statuses))
	for _, status := range statuses {
		modules = append(modules, status.Module)
	}
	return modules, nil
}

func LoadModuleStatuses(repoRoot string) ([]ModuleStatus, error) {
	path := filepath.Join(repoRoot, relativeStatusPath)
	lines, _, err := readLines(path)
	if err != nil {
		return nil, err
	}

	start, end, err := findFormalModuleTable(lines)
	if err != nil {
		return nil, err
	}

	statuses := make([]ModuleStatus, 0, end-start)
	for idx := start; idx < end; idx++ {
		cells, ok := parseTableLine(lines[idx])
		if !ok || len(cells) < 6 {
			continue
		}
		statuses = append(statuses, ModuleStatus{
			Module:      stripCodeSpan(cells[0]),
			Stable:      stripCodeSpan(cells[1]),
			Candidate:   stripCodeSpan(cells[2]),
			ActiveLayer: stripCodeSpan(cells[3]),
			NextCommand: stripCodeSpan(cells[4]),
			Notes:       strings.TrimSpace(cells[5]),
		})
	}
	return statuses, nil
}

func LookupModuleStatus(repoRoot, module string) (ModuleStatus, error) {
	statuses, err := LoadModuleStatuses(repoRoot)
	if err != nil {
		return ModuleStatus{}, err
	}
	for _, status := range statuses {
		if status.Module == module {
			return status, nil
		}
	}
	return ModuleStatus{}, fmt.Errorf("module %q not found in %s", module, relativeStatusPath)
}

func UpdateNextCommand(repoRoot, module, nextCommand string) (bool, error) {
	status, err := LookupModuleStatus(repoRoot, module)
	if err != nil {
		return false, err
	}
	status.NextCommand = nextCommand
	return UpsertModuleStatus(repoRoot, status, false)
}

func UpsertModuleStatus(repoRoot string, status ModuleStatus, createIfMissing bool) (bool, error) {
	if err := validateModuleStatus(status); err != nil {
		return false, err
	}

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
		if stripCodeSpan(cells[0]) != status.Module {
			continue
		}
		lines[idx] = formatModuleStatusLine(status)
		updated = true
		break
	}

	if !updated {
		if !createIfMissing {
			return false, fmt.Errorf("module %q not found in %s", status.Module, relativeStatusPath)
		}
		lines = append(lines[:end], append([]string{formatModuleStatusLine(status)}, lines[end:]...)...)
		updated = true
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

func formatModuleStatusLine(status ModuleStatus) string {
	return formatTableLine([]string{
		fmt.Sprintf("`%s`", status.Module),
		fmt.Sprintf("`%s`", status.Stable),
		fmt.Sprintf("`%s`", status.Candidate),
		fmt.Sprintf("`%s`", status.ActiveLayer),
		fmt.Sprintf("`%s`", status.NextCommand),
		status.Notes,
	})
}

func stripCodeSpan(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "`") && strings.HasSuffix(value, "`") && len(value) >= 2 {
		return value[1 : len(value)-1]
	}
	return value
}

func validateModuleStatus(status ModuleStatus) error {
	if !allowedNextCommands[strings.TrimSpace(status.NextCommand)] {
		return fmt.Errorf("next command %q is not a supported status value", status.NextCommand)
	}
	return nil
}

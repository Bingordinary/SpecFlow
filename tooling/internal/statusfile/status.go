package statusfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const relativeStatusPath = "docs/specs/_status.md"

type ObjectStatus struct {
	ObjectType  string
	Object      string
	Stable      string
	Candidate   string
	ActiveLayer string
	NextCommand string
	Notes       string
}

type ModuleStatus struct {
	ObjectType  string
	Module      string
	Stable      string
	Candidate   string
	ActiveLayer string
	NextCommand string
	Notes       string
}

type tableShape struct {
	ObjectTypeColumn bool
}

var allowedNextCommands = map[string]bool{
	"module_init":           true,
	"module_new":            true,
	"module_fork":           true,
	"module_stable_verify":  true,
	"module_check":          true,
	"module_plan":           true,
	"module_impl":           true,
	"module_verify":         true,
	"module_promote":        true,
	"project_new":           true,
	"project_init":          true,
	"project_fork":          true,
	"project_check":         true,
	"project_verify":        true,
	"project_promote":       true,
	"project_stable_verify": true,
	"flow_new":              true,
	"flow_fork":             true,
	"flow_check":            true,
	"flow_verify":           true,
	"flow_promote":          true,
	"flow_stable_verify":    true,
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

func LoadObjectStatuses(repoRoot string) ([]ObjectStatus, error) {
	path := filepath.Join(repoRoot, relativeStatusPath)
	lines, _, err := readLines(path)
	if err != nil {
		return nil, err
	}

	start, end, shape, err := findStatusTable(lines)
	if err != nil {
		return nil, err
	}

	statuses := make([]ObjectStatus, 0, end-start)
	for idx := start; idx < end; idx++ {
		cells, ok := parseTableLine(lines[idx])
		if !ok {
			continue
		}
		if shape.ObjectTypeColumn {
			if len(cells) < 7 {
				continue
			}
			statuses = append(statuses, ObjectStatus{
				ObjectType:  stripCodeSpan(cells[0]),
				Object:      stripCodeSpan(cells[1]),
				Stable:      stripCodeSpan(cells[2]),
				Candidate:   stripCodeSpan(cells[3]),
				ActiveLayer: stripCodeSpan(cells[4]),
				NextCommand: stripCodeSpan(cells[5]),
				Notes:       strings.TrimSpace(cells[6]),
			})
			continue
		}
		if len(cells) < 6 {
			continue
		}
		statuses = append(statuses, ObjectStatus{
			ObjectType:  "module",
			Object:      stripCodeSpan(cells[0]),
			Stable:      stripCodeSpan(cells[1]),
			Candidate:   stripCodeSpan(cells[2]),
			ActiveLayer: stripCodeSpan(cells[3]),
			NextCommand: stripCodeSpan(cells[4]),
			Notes:       strings.TrimSpace(cells[5]),
		})
	}
	return statuses, nil
}

func LoadModuleStatuses(repoRoot string) ([]ModuleStatus, error) {
	objectStatuses, err := LoadObjectStatuses(repoRoot)
	if err != nil {
		return nil, err
	}

	statuses := make([]ModuleStatus, 0, len(objectStatuses))
	for _, status := range objectStatuses {
		if status.ObjectType != "module" {
			continue
		}
		statuses = append(statuses, ModuleStatus{
			ObjectType:  status.ObjectType,
			Module:      status.Object,
			Stable:      status.Stable,
			Candidate:   status.Candidate,
			ActiveLayer: status.ActiveLayer,
			NextCommand: status.NextCommand,
			Notes:       status.Notes,
		})
	}
	return statuses, nil
}

func LookupObjectStatus(repoRoot, objectType, object string) (ObjectStatus, error) {
	statuses, err := LoadObjectStatuses(repoRoot)
	if err != nil {
		return ObjectStatus{}, err
	}
	for _, status := range statuses {
		if status.ObjectType == objectType && status.Object == object {
			return status, nil
		}
	}
	return ObjectStatus{}, fmt.Errorf("%s %q not found in %s", objectType, object, relativeStatusPath)
}

func LookupModuleStatus(repoRoot, module string) (ModuleStatus, error) {
	status, err := LookupObjectStatus(repoRoot, "module", module)
	if err != nil {
		return ModuleStatus{}, err
	}
	return ModuleStatus{
		ObjectType:  status.ObjectType,
		Module:      status.Object,
		Stable:      status.Stable,
		Candidate:   status.Candidate,
		ActiveLayer: status.ActiveLayer,
		NextCommand: status.NextCommand,
		Notes:       status.Notes,
	}, nil
}

func UpdateNextCommand(repoRoot, module, nextCommand string) (bool, error) {
	status, err := LookupModuleStatus(repoRoot, module)
	if err != nil {
		return false, err
	}
	status.NextCommand = nextCommand
	return UpsertModuleStatus(repoRoot, status, false)
}

func UpdateObjectNextCommand(repoRoot, objectType, object, nextCommand string) (bool, error) {
	status, err := LookupObjectStatus(repoRoot, objectType, object)
	if err != nil {
		return false, err
	}
	status.NextCommand = nextCommand
	return UpsertObjectStatus(repoRoot, status, false)
}

func UpsertObjectStatus(repoRoot string, status ObjectStatus, createIfMissing bool) (bool, error) {
	if err := validateObjectStatus(status); err != nil {
		return false, err
	}

	path := filepath.Join(repoRoot, relativeStatusPath)
	lines, hadTrailingNewline, err := readLines(path)
	if err != nil {
		return false, err
	}

	start, end, shape, err := findStatusTable(lines)
	if err != nil {
		return false, err
	}
	if !shape.ObjectTypeColumn && status.ObjectType != "module" {
		return false, fmt.Errorf("legacy status table cannot store non-module object type %q", status.ObjectType)
	}

	updated := false
	for idx := start; idx < end; idx++ {
		cells, ok := parseTableLine(lines[idx])
		if !ok {
			continue
		}
		if shape.ObjectTypeColumn {
			if len(cells) < 7 {
				continue
			}
			if stripCodeSpan(cells[0]) != status.ObjectType || stripCodeSpan(cells[1]) != status.Object {
				continue
			}
			lines[idx] = formatObjectStatusLine(status, shape)
			updated = true
			break
		}
		if len(cells) < 6 {
			continue
		}
		if stripCodeSpan(cells[0]) != status.Object {
			continue
		}
		lines[idx] = formatObjectStatusLine(status, shape)
		updated = true
		break
	}

	if !updated {
		if !createIfMissing {
			return false, fmt.Errorf("%s %q not found in %s", status.ObjectType, status.Object, relativeStatusPath)
		}
		lines = append(lines[:end], append([]string{formatObjectStatusLine(status, shape)}, lines[end:]...)...)
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

func UpsertModuleStatus(repoRoot string, status ModuleStatus, createIfMissing bool) (bool, error) {
	objectType := strings.TrimSpace(status.ObjectType)
	if objectType == "" {
		objectType = "module"
	}
	return UpsertObjectStatus(repoRoot, ObjectStatus{
		ObjectType:  objectType,
		Object:      status.Module,
		Stable:      status.Stable,
		Candidate:   status.Candidate,
		ActiveLayer: status.ActiveLayer,
		NextCommand: status.NextCommand,
		Notes:       status.Notes,
	}, createIfMissing)
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

func findStatusTable(lines []string) (int, int, tableShape, error) {
	for idx := range lines {
		cells, ok := parseTableLine(lines[idx])
		if !ok {
			continue
		}
		switch {
		case len(cells) >= 7 && cells[0] == "Object Type" && cells[5] == "Next Command":
			start, end, _, err := tableRange(lines, idx)
			return start, end, tableShape{ObjectTypeColumn: true}, err
		case len(cells) >= 6 && cells[0] == "Module" && cells[4] == "Next Command":
			start, end, _, err := tableRange(lines, idx)
			return start, end, tableShape{ObjectTypeColumn: false}, err
		}
	}
	return 0, 0, tableShape{}, fmt.Errorf("status table not found in %s", relativeStatusPath)
}

func tableRange(lines []string, headerIndex int) (int, int, tableShape, error) {
	if headerIndex+1 >= len(lines) {
		return 0, 0, tableShape{}, fmt.Errorf("missing separator row in %s", relativeStatusPath)
	}
	start := headerIndex + 2
	end := start
	for end < len(lines) {
		if _, ok := parseTableLine(lines[end]); !ok {
			break
		}
		end++
	}
	return start, end, tableShape{}, nil
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

func formatObjectStatusLine(status ObjectStatus, shape tableShape) string {
	if shape.ObjectTypeColumn {
		return formatTableLine([]string{
			fmt.Sprintf("`%s`", status.ObjectType),
			fmt.Sprintf("`%s`", status.Object),
			fmt.Sprintf("`%s`", status.Stable),
			fmt.Sprintf("`%s`", status.Candidate),
			fmt.Sprintf("`%s`", status.ActiveLayer),
			fmt.Sprintf("`%s`", status.NextCommand),
			status.Notes,
		})
	}
	return formatTableLine([]string{
		fmt.Sprintf("`%s`", status.Object),
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

func validateObjectStatus(status ObjectStatus) error {
	if strings.TrimSpace(status.ObjectType) == "" {
		return fmt.Errorf("object type is required")
	}
	if strings.TrimSpace(status.Object) == "" {
		return fmt.Errorf("object is required")
	}
	if !allowedNextCommands[strings.TrimSpace(status.NextCommand)] {
		return fmt.Errorf("next command %q is not a supported status value", status.NextCommand)
	}
	return nil
}

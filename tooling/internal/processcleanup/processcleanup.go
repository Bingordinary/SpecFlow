package processcleanup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

type CleanupResult struct {
	Module        string
	FromCommand   string
	Reason        string
	NextCommand   string
	DeletedFiles  []string
	MissingFiles  []string
	StatusUpdated bool
}

type cleanupRule struct {
	NextCommand string
	FileKinds   []string
}

var rules = map[string]map[string]cleanupRule{
	"cand_plan": {
		"gate_missing":          {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"truth_drift":           {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"binding_drift":         {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"baseline_drift":        {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"shared_contract_drift": {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"truth_incomplete":      {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
	},
	"cand_impl": {
		"gate_missing":          {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"truth_drift":           {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"binding_drift":         {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"baseline_drift":        {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"shared_contract_drift": {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
	},
	"cand_verify": {
		"gate_missing":          {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"truth_drift":           {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"binding_drift":         {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"baseline_drift":        {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"shared_contract_drift": {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
	},
	"cand_promote": {
		"truth_drift":           {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"binding_drift":         {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"baseline_drift":        {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"shared_contract_drift": {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
		"implementation_deviation": {
			NextCommand: "cand_impl",
			FileKinds:   []string{"verify"},
		},
		"evidence_incomplete": {
			NextCommand: "cand_verify",
			FileKinds:   []string{"verify"},
		},
	},
}

func ApplyFallback(repoRoot, module, fromCommand, reason string) (CleanupResult, error) {
	result := CleanupResult{
		Module:      strings.TrimSpace(module),
		FromCommand: strings.TrimSpace(fromCommand),
		Reason:      strings.TrimSpace(reason),
	}

	if result.Module == "" || result.FromCommand == "" || result.Reason == "" {
		return result, fmt.Errorf("module, from command, and reason are required")
	}
	if _, err := ensureFormalModule(repoRoot, result.Module); err != nil {
		return result, err
	}

	rule, err := lookupRule(result.FromCommand, result.Reason)
	if err != nil {
		return result, err
	}
	result.NextCommand = rule.NextCommand

	for _, relPath := range filePathsForModule(result.Module, rule.FileKinds) {
		absPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		if _, err := os.Stat(absPath); err != nil {
			if os.IsNotExist(err) {
				result.MissingFiles = append(result.MissingFiles, relPath)
				continue
			}
			return result, fmt.Errorf("stat %s: %w", relPath, err)
		}
		if err := os.Remove(absPath); err != nil {
			return result, fmt.Errorf("delete %s: %w", relPath, err)
		}
		result.DeletedFiles = append(result.DeletedFiles, relPath)
	}

	updated, err := statusfile.UpdateNextCommand(repoRoot, result.Module, result.NextCommand)
	if err != nil {
		return result, err
	}
	result.StatusUpdated = updated
	return result, nil
}

func lookupRule(fromCommand, reason string) (cleanupRule, error) {
	commandRules, ok := rules[fromCommand]
	if !ok {
		return cleanupRule{}, fmt.Errorf("unsupported from-command %q", fromCommand)
	}
	rule, ok := commandRules[reason]
	if !ok {
		return cleanupRule{}, fmt.Errorf("no deterministic fallback cleanup is defined for %q + %q", fromCommand, reason)
	}
	return rule, nil
}

func filePathsForModule(module string, fileKinds []string) []string {
	paths := make([]string, 0, len(fileKinds))
	for _, fileKind := range fileKinds {
		switch fileKind {
		case "check":
			paths = append(paths, fmt.Sprintf("docs/specs/_check_result/%s.md", module))
		case "plan":
			paths = append(paths, fmt.Sprintf("docs/specs/_plans/%s.md", module))
		case "verify":
			paths = append(paths, fmt.Sprintf("docs/specs/_verify_result/%s.md", module))
		}
	}
	return paths
}

func ensureFormalModule(repoRoot, module string) (bool, error) {
	modules, err := statusfile.LoadModules(repoRoot)
	if err != nil {
		return false, err
	}
	for _, candidate := range modules {
		if candidate == module {
			return true, nil
		}
	}
	return false, fmt.Errorf("module %q is not registered in docs/specs/_status.md", module)
}

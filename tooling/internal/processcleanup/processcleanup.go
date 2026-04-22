package processcleanup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/snapshot"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
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

type SuccessCleanupResult struct {
	Module       string
	Mode         string
	DeletedFiles []string
	MissingFiles []string
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
		"implementation_deviation": {
			NextCommand: "cand_impl",
			FileKinds:   []string{"verify"},
		},
		"evidence_incomplete": {
			NextCommand: "cand_verify",
			FileKinds:   []string{"verify"},
		},
		"truth_incomplete": {NextCommand: "cand_check", FileKinds: []string{"check", "plan", "verify"}},
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

func ApplySuccessCleanup(repoRoot, module, mode string) (SuccessCleanupResult, error) {
	result := SuccessCleanupResult{
		Module: strings.TrimSpace(module),
		Mode:   strings.TrimSpace(mode),
	}
	if result.Module == "" || result.Mode == "" {
		return result, fmt.Errorf("module and mode are required")
	}
	if _, err := ensureFormalModule(repoRoot, result.Module); err != nil {
		return result, err
	}

	paths, err := successCleanupPaths(repoRoot, result.Module, result.Mode)
	if err != nil {
		return result, err
	}
	for _, relPath := range paths {
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
		filePaths, err := snapshot.ProcessArtifactPaths(module, fileKind)
		if err != nil {
			continue
		}
		paths = append(paths, filePaths...)
	}
	return sortAndDedupeStrings(paths)
}

func successCleanupPaths(repoRoot, module, mode string) ([]string, error) {
	paths := []string{}
	switch mode {
	case "spec_fork":
		paths = append(paths, filePathsForModule(module, []string{"check", "plan", "verify"})...)
		appendixPaths, err := candidateAppendixPaths(repoRoot, module)
		if err != nil {
			return nil, err
		}
		paths = append(paths, appendixPaths...)
	case "cand_promote":
		candidateMainRef, err := specpaths.MainSpecFileRef("candidate", module)
		if err != nil {
			return nil, err
		}
		paths = append(paths, candidateMainRef)
		paths = append(paths, filePathsForModule(module, []string{"check", "plan", "verify"})...)
		appendixPaths, err := candidateAppendixPaths(repoRoot, module)
		if err != nil {
			return nil, err
		}
		paths = append(paths, appendixPaths...)
	default:
		return nil, fmt.Errorf("unsupported success cleanup mode %q", mode)
	}
	return sortAndDedupeStrings(paths), nil
}

func candidateAppendixPaths(repoRoot, module string) ([]string, error) {
	pattern := filepath.Join(repoRoot, filepath.FromSlash(specpaths.CandidateAppendixGlob(module)))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		rel, err := filepath.Rel(repoRoot, match)
		if err != nil {
			return nil, err
		}
		result = append(result, filepath.ToSlash(rel))
	}
	return result, nil
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

func sortAndDedupeStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

package entrysync

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	managedBegin = "<!-- SPECFLOW:BEGIN -->"
	managedEnd   = "<!-- SPECFLOW:END -->"
	registryPath = "specflow/framework/docs/agent_guidelines/entry_index_registry.md"
)

var registeredEntryPattern = regexp.MustCompile("^- `([^`]*)`$")

type Inspection struct {
	RegisteredFiles []string
	Consistent      bool
	SuggestedSource string
	StagedChanged   []string
}

type SyncResult struct {
	Source       string
	UpdatedFiles []string
	Staged       bool
}

func Inspect(repoRoot string) (Inspection, error) {
	registeredFiles, err := loadRegisteredFiles(repoRoot)
	if err != nil {
		return Inspection{}, err
	}

	hashes := map[string]int{}
	for _, relPath := range registeredFiles {
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relPath)))
		if err != nil {
			return Inspection{}, fmt.Errorf("read %s: %w", relPath, err)
		}
		block, err := extractManagedBlock(string(content))
		if err != nil {
			return Inspection{}, fmt.Errorf("%s: %w", relPath, err)
		}
		sum := sha256.Sum256([]byte(block))
		hashes[string(sum[:])]++
	}

	inspection := Inspection{
		RegisteredFiles: registeredFiles,
		Consistent:      len(hashes) == 1,
	}
	if inspection.Consistent {
		return inspection, nil
	}

	stagedChanged, err := inferStagedChanged(repoRoot, registeredFiles)
	if err != nil {
		return inspection, err
	}
	inspection.StagedChanged = stagedChanged
	if len(stagedChanged) == 1 {
		inspection.SuggestedSource = stagedChanged[0]
	}
	return inspection, nil
}

func Sync(repoRoot, source string, stage bool) (SyncResult, error) {
	inspection, err := Inspect(repoRoot)
	if err != nil {
		return SyncResult{}, err
	}
	if inspection.Consistent {
		return SyncResult{}, nil
	}

	if source == "" {
		source = inspection.SuggestedSource
	}
	if source == "" {
		return SyncResult{}, fmt.Errorf("registered entry docs differ and no unique sync source could be inferred")
	}
	if !contains(inspection.RegisteredFiles, source) {
		return SyncResult{}, fmt.Errorf("source %q is not a registered entry file", source)
	}

	sourceAbs := filepath.Join(repoRoot, filepath.FromSlash(source))
	sourceContent, err := os.ReadFile(sourceAbs)
	if err != nil {
		return SyncResult{}, fmt.Errorf("read %s: %w", source, err)
	}
	sourceBlock, err := extractManagedBlock(string(sourceContent))
	if err != nil {
		return SyncResult{}, fmt.Errorf("%s: %w", source, err)
	}

	result := SyncResult{Source: source}
	for _, relPath := range inspection.RegisteredFiles {
		if relPath == source {
			continue
		}
		targetAbs := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		targetContent, err := os.ReadFile(targetAbs)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", relPath, err)
		}
		targetBlock, err := extractManagedBlock(string(targetContent))
		if err != nil {
			return result, fmt.Errorf("%s: %w", relPath, err)
		}
		if sourceBlock == targetBlock {
			continue
		}
		updated, err := replaceManagedBlock(string(targetContent), sourceBlock)
		if err != nil {
			return result, fmt.Errorf("%s: %w", relPath, err)
		}
		if err := os.WriteFile(targetAbs, []byte(updated), 0o644); err != nil {
			return result, fmt.Errorf("write %s: %w", relPath, err)
		}
		result.UpdatedFiles = append(result.UpdatedFiles, relPath)
	}

	if stage {
		args := append([]string{"-C", repoRoot, "add", "--"}, inspection.RegisteredFiles...)
		cmd := exec.Command("git", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return result, fmt.Errorf("git add registered entry files: %v: %s", err, strings.TrimSpace(string(output)))
		}
		result.Staged = true
	}

	return result, nil
}

func loadRegisteredFiles(repoRoot string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(registryPath)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", registryPath, err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	files := []string{}
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Registered Entry Index Files" {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inSection {
			continue
		}
		match := registeredEntryPattern.FindStringSubmatch(trimmed)
		if len(match) == 2 {
			files = append(files, match[1])
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no registered entry files found in %s", registryPath)
	}
	sort.Strings(files)
	for _, relPath := range files {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relPath))); err != nil {
			return nil, fmt.Errorf("registered entry file missing: %s", relPath)
		}
	}
	return files, nil
}

func extractManagedBlock(content string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	beginIdx := -1
	endIdx := -1
	for idx, line := range lines {
		if line == managedBegin {
			if beginIdx != -1 {
				return "", fmt.Errorf("managed block begin marker must appear exactly once")
			}
			beginIdx = idx
		}
		if line == managedEnd {
			if endIdx != -1 {
				return "", fmt.Errorf("managed block end marker must appear exactly once")
			}
			endIdx = idx
		}
	}
	if beginIdx == -1 || endIdx == -1 || beginIdx >= endIdx {
		return "", fmt.Errorf("managed block markers are missing or out of order")
	}
	return strings.Join(lines[beginIdx:endIdx+1], "\n"), nil
}

func replaceManagedBlock(content, replacement string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	beginIdx := -1
	endIdx := -1
	for idx, line := range lines {
		if line == managedBegin {
			if beginIdx != -1 {
				return "", fmt.Errorf("managed block begin marker must appear exactly once")
			}
			beginIdx = idx
		}
		if line == managedEnd {
			if endIdx != -1 {
				return "", fmt.Errorf("managed block end marker must appear exactly once")
			}
			endIdx = idx
		}
	}
	if beginIdx == -1 || endIdx == -1 || beginIdx >= endIdx {
		return "", fmt.Errorf("managed block markers are missing or out of order")
	}
	replacementLines := strings.Split(strings.ReplaceAll(replacement, "\r\n", "\n"), "\n")
	updated := append([]string{}, lines[:beginIdx]...)
	updated = append(updated, replacementLines...)
	updated = append(updated, lines[endIdx+1:]...)
	return strings.Join(updated, "\n"), nil
}

func inferStagedChanged(repoRoot string, registeredFiles []string) ([]string, error) {
	args := append([]string{"-C", repoRoot, "diff", "--cached", "--name-only", "--"}, registeredFiles...)
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git diff --cached failed: %s", bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	result := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if contains(registeredFiles, filepath.ToSlash(line)) {
			result = append(result, filepath.ToSlash(line))
		}
	}
	sort.Strings(result)
	return result, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

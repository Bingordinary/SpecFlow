// Package validationcache provides cache-freshness checking for spec_validate
// and spec_verify results. Cache files are written by the agent (not the CLI)
// and read by specflowctl promote to confirm that validate/verify are still fresh.
//
// Cache files for units live under docs/specs/meta/validation/unit/{name}/.
// Cache files for rules live under docs/specs/meta/validation/rule/{id}/.
// They record:
//   - Which files were checked (paths + SHA-256 hashes)
//   - Whether the check passed (pass / aligned)
//   - When the check was run
//
// specflowctl promote reads both caches, re-computes hashes, and rejects
// if anything has changed since the cache was written.
package validationcache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// CheckResult describes whether a cache file is fresh.
type CheckResult struct {
	Fresh  bool
	Reason string
}

// cacheFile is the parsed representation of a cache file.
type cacheFile struct {
	Command     string `yaml:"command"`
	Unit        string `yaml:"unit"`
	Mode        string `yaml:"mode,omitempty"`
	ScopedCheck string `yaml:"scoped_check,omitempty"`
	ScopedItem  string `yaml:"scoped_item,omitempty"`
	Result      string `yaml:"result"`
	Target      string `yaml:"target,omitempty"`
	Timestamp   string `yaml:"timestamp"`
	Files       []cacheFileEntry
}

type cacheFileEntry struct {
	Path string `yaml:"path"`
	Hash string `yaml:"hash"`
}

// CheckValidate reads and validates the validate cache for the given unit.
func CheckValidate(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "validate", "validate_result.md", []string{"pass"})
}

// CheckVerify reads and validates the verify cache for the given unit.
func CheckVerify(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "verify", "verify_result.md", []string{"aligned"})
}

// CheckRuleValidate reads and validates the validate cache for the given rule.
func CheckRuleValidate(repoRoot, ruleID string) (CheckResult, error) {
	return checkCache(repoRoot, "rule", ruleID, "validate", "validate_result.md", []string{"pass"})
}

// CheckRuleVerify is deprecated — rule verify has been removed.
// It always returns not-fresh with a deprecation message.
func CheckRuleVerify(repoRoot, ruleID string) (CheckResult, error) {
	return CheckResult{
		Fresh:  false,
		Reason: "rule verify has been removed (see framework/concepts.md); only rule validate is required for promote",
	}, nil
}

// deleteCache removes a specific cache file (validate or verify) for the given target.
func deleteCache(repoRoot, targetKind, targetName, command string) error {
	var fileName string
	switch command {
	case "validate":
		fileName = "validate_result.md"
	case "verify":
		fileName = "verify_result.md"
	default:
		return fmt.Errorf("unknown cache command %q", command)
	}

	cachePath := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil // already gone
	}
	return os.Remove(cachePath)
}

// DeleteCache removes a specific cache file (validate or verify) for the given unit.
func DeleteCache(repoRoot, unitName, command string) error {
	return deleteCache(repoRoot, "unit", unitName, command)
}

// DeleteAll removes both validate and verify caches for the given unit.
func DeleteAll(repoRoot, unitName string) error {
	if err := DeleteCache(repoRoot, unitName, "validate"); err != nil {
		return err
	}
	return DeleteCache(repoRoot, unitName, "verify")
}

// DeleteRuleCache removes a specific cache file (validate or verify) for the given rule.
func DeleteRuleCache(repoRoot, ruleID, command string) error {
	return deleteCache(repoRoot, "rule", ruleID, command)
}

// DeleteAllRuleCache removes the validate cache for the given rule.
// (Rule verify cache is no longer used.)
func DeleteAllRuleCache(repoRoot, ruleID string) error {
	return DeleteRuleCache(repoRoot, ruleID, "validate")
}

// ------------------------------------------------------------
// Internal
// ------------------------------------------------------------

func checkCache(repoRoot, targetKind, targetName, command, fileName string, validResults []string) (CheckResult, error) {
	cachePath := cacheFilePath(repoRoot, targetKind, targetName, fileName)

	// Check existence
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("%s cache not found at %s", command, relPath(repoRoot, cachePath)),
		}, nil
	}

	// Parse cache file
	cache, err := readCache(cachePath)
	if err != nil {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("cannot read %s cache: %v", command, err),
		}, nil
	}

	// Validate command matches
	if cache.Command != command {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("cache command is %q, expected %q", cache.Command, command),
		}, nil
	}

	// Validate result is acceptable
	resultOk := false
	for _, vr := range validResults {
		if cache.Result == vr {
			resultOk = true
			break
		}
	}
	if !resultOk {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("%s cache result is %q, expected one of %v", command, cache.Result, validResults),
		}, nil
	}

	// Reject scoped mode — only full-mode caches satisfy the promote gate
	if cache.Mode == "scoped" {
		var scopeDetail string
		if cache.ScopedCheck != "" {
			scopeDetail = fmt.Sprintf(" (check %s only)", cache.ScopedCheck)
		} else if cache.ScopedItem != "" {
			scopeDetail = fmt.Sprintf(" (item: %s)", cache.ScopedItem)
		}
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("%s cache is scoped%s, run `spec_%s %s:full` before promoting", command, scopeDetail, command, cache.Unit),
		}, nil
	}

	// Re-compute hashes for all listed files
	var mismatchedFiles []string
	var missingFiles []string
	for _, entry := range cache.Files {
		fullPath := resolvePath(repoRoot, entry.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			missingFiles = append(missingFiles, entry.Path)
			continue
		}
		currentHash, err := fileHash(fullPath)
		if err != nil {
			missingFiles = append(missingFiles, fmt.Sprintf("%s (%v)", entry.Path, err))
			continue
		}
		if currentHash != normalizeHash(entry.Hash) {
			mismatchedFiles = append(mismatchedFiles, entry.Path)
		}
	}

	if len(missingFiles) > 0 {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("%s cache stale: files missing: %s", command, strings.Join(missingFiles, ", ")),
		}, nil
	}
	if len(mismatchedFiles) > 0 {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("%s cache stale: files have changed: %s. Run spec_%s again.", command, strings.Join(mismatchedFiles, ", "), command),
		}, nil
	}

	return CheckResult{
		Fresh:  true,
		Reason: fmt.Sprintf("%s cache is fresh (result: %s, %d file(s) unchanged)", command, cache.Result, len(cache.Files)),
	}, nil
}

// readCache parses a cache file (YAML frontmatter + markdown body).
func readCache(path string) (*cacheFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// Extract YAML frontmatter between --- markers
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing leading --- frontmatter delimiter")
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return nil, fmt.Errorf("missing closing --- frontmatter delimiter")
	}

	fmLines := lines[1:endIdx]

	cache := &cacheFile{}
	var currentEntry *cacheFileEntry
	inFilesBlock := false

	for _, line := range fmLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect files block entries
		if trimmed == "files:" {
			inFilesBlock = true
			continue
		}

		if inFilesBlock {
			if strings.HasPrefix(trimmed, "- path:") {
				// New entry
				path := strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:"))
				path = strings.Trim(path, "\"'")
				currentEntry = &cacheFileEntry{Path: path}
				cache.Files = append(cache.Files, *currentEntry)
				continue
			}
			if strings.HasPrefix(trimmed, "hash:") && currentEntry != nil {
				hash := strings.TrimSpace(strings.TrimPrefix(trimmed, "hash:"))
				hash = strings.Trim(hash, "\"'")
				cache.Files[len(cache.Files)-1] = cacheFileEntry{
					Path: cache.Files[len(cache.Files)-1].Path,
					Hash: hash,
				}
				continue
			}
			// If we hit a non-empty line that doesn't start with - or is a continuation,
			// we might have left the files block
			if currentEntry != nil {
				inFilesBlock = false
			}
		}

		if !inFilesBlock {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				value = strings.Trim(value, "\"'")
				switch key {
				case "command":
					cache.Command = value
				case "unit":
					cache.Unit = value
				case "mode":
					cache.Mode = value
				case "scoped_check":
					cache.ScopedCheck = value
				case "scoped_item":
					cache.ScopedItem = value
				case "result":
					cache.Result = value
				case "target":
					cache.Target = value
				case "timestamp":
					cache.Timestamp = value
				}
			}
		}
	}

	if cache.Command == "" || cache.Result == "" {
		return nil, fmt.Errorf("cache file missing required frontmatter fields (command, result)")
	}

	return cache, nil
}

// fileHash computes the SHA-256 hash of a file's normalized content.
// Delegates to specpaths.FileHash for the canonical normalization.
func fileHash(path string) (string, error) {
	return specpaths.FileHash(path)
}

// cacheFilePath builds the absolute path to a cache file.
// targetKind is "unit" or "rule".
func cacheFilePath(repoRoot, targetKind, targetName, fileName string) string {
	return filepath.Join(repoRoot, "docs/specs/meta/validation", targetKind, targetName, fileName)
}

func relPath(repoRoot, absPath string) string {
	rel, err := filepath.Rel(repoRoot, absPath)
	if err != nil {
		return absPath
	}
	return filepath.ToSlash(rel)
}

// normalizeHash strips any algorithm prefix (e.g. "sha256:") from a stored hash
// so it can be compared against a raw hex hash.
func normalizeHash(stored string) string {
	if idx := strings.LastIndex(stored, ":"); idx >= 0 {
		return stored[idx+1:]
	}
	return stored
}

func resolvePath(repoRoot, filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(repoRoot, filepath.FromSlash(filePath))
}

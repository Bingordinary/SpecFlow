// Package validationcache provides cache-freshness checking for validate@
// and verify@results. Cache files are written by the agent (not the CLI)
// and read by specflowctl promote to confirm that validate/verify are still fresh.
//
// Cache files for units live under docs/specs/meta/validation/unit/{name}/.
// Cache files for rules live under docs/specs/meta/validation/rule/{id}/.
// They record:
//   - Which files were checked (paths + SHA-256 hashes)
//   - Whether the check passed (pass)
//   - When the check was run
//
// specflowctl promote reads both caches, re-computes hashes, and rejects
// if anything has changed since the cache was written.
package validationcache

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	Command      string `yaml:"command"`
	Unit         string `yaml:"unit"`
	Mode         string `yaml:"mode,omitempty"`
	ScopedCheck  string `yaml:"scoped_check,omitempty"`
	ScopedItem   string `yaml:"scoped_item,omitempty"`
	Result       string `yaml:"result"`
	Target       string `yaml:"target,omitempty"`
	Blocking     bool   `yaml:"blocking"`
	blockingSeen bool   `yaml:"-"`
	P0Count      int    `yaml:"p0_count"`
	P1Count      int    `yaml:"p1_count"`
	P2Count      int    `yaml:"p2_count"`
	P3Count      int    `yaml:"p3_count"`
	Timestamp    string `yaml:"timestamp"`
	Files        []cacheFileEntry
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
// A verify cache is valid only with result "pass". P0/P1 findings never
// write a cache (the cache is deleted), and P2/P3 pending findings are
// carried by the severity counts (blocking: false).
func CheckVerify(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "verify", "verify_result.md", []string{"pass"})
}

// CheckAppendicesInCache verifies that every non-exempt candidate appendix for
// the given unit is listed in the validate_result.md cache file. This is a
// mechanical promote gate — it ensures the agent included all appendix files
// in the validation run. If an appendix exists on disk but is missing from
// the cache's files list, the agent skipped it.
func CheckAppendicesInCache(repoRoot, unitName string) (CheckResult, error) {
	// 1. Read validate cache
	cachePath := cacheFilePath(repoRoot, "unit", unitName, "validate_result.md")
	cache, err := readCache(cachePath)
	if err != nil {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("cannot read validate cache at %s: %v", relPath(repoRoot, cachePath), err),
		}, nil
	}
	if cache.Command != "validate" {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("cache command is %q, expected 'validate'", cache.Command),
		}, nil
	}
	if cache.Result != "pass" {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("validate cache result is %q, expected 'pass'", cache.Result),
		}, nil
	}

	// 2. Build set of cached file paths
	cachedPaths := make(map[string]bool, len(cache.Files))
	for _, entry := range cache.Files {
		cachedPaths[filepath.ToSlash(entry.Path)] = true
	}

	// 3. Glob candidate appendix files
	pattern := specpaths.CandidateAppendixGlob(unitName)
	fullGlob := filepath.Join(repoRoot, filepath.FromSlash(pattern))
	matches, err := filepath.Glob(fullGlob)
	if err != nil {
		return CheckResult{
			Fresh:  true,
			Reason: fmt.Sprintf("cannot glob appendix files: %v (gate skipped)", err),
		}, nil
	}

	// 4. Check each non-exempt appendix
	var missing []string
	for _, m := range matches {
		relPath, _ := filepath.Rel(repoRoot, m)
		relPathSlash := filepath.ToSlash(relPath)

		// Check status: skip exempt appendices
		data, err := os.ReadFile(m)
		if err == nil {
			fm := specpaths.ReadFrontmatterStringMap(string(data))
			if strings.EqualFold(strings.TrimSpace(fm["status"]), "exempt") {
				continue
			}
		}

		if !cachedPaths[relPathSlash] {
			missing = append(missing, relPathSlash)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Fresh: false,
			Reason: fmt.Sprintf("appendix file(s) not included in validation: %s. Run `validate@%s:full` again.",
				strings.Join(missing, ", "), unitName),
		}, nil
	}

	return CheckResult{
		Fresh:  true,
		Reason: fmt.Sprintf("all %d appendix file(s) are included in validate cache", len(matches)),
	}, nil
}

// CheckRuleValidate reads and validates the validate cache for the given rule.
func CheckRuleValidate(repoRoot, ruleID string) (CheckResult, error) {
	return checkCache(repoRoot, "rule", ruleID, "validate", "validate_result.md", []string{"pass"})
}

// CheckReview reads and validates the review cache for the given unit.
// The review cache is a required promote gate: it must exist, mode must be
// "full", hashes must match, and it must not be blocking (P0/P1 findings).
// If any condition fails, promote must be rejected with guidance.
func CheckReview(repoRoot, unitName string) (CheckResult, error) {
	cachePath := cacheFilePath(repoRoot, "unit", unitName, "review_result.md")

	// Existence check — review cache is required for promote
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("Review not completed. Run `review@%s:full` first.", unitName),
		}, nil
	}

	cache, err := readCache(cachePath)
	if err != nil {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("cannot read review cache: %v", err),
		}, nil
	}

	if cache.Command != "review" {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("review cache command is %q, expected 'review'", cache.Command),
		}, nil
	}

	// Mode check — only full-mode caches satisfy the promote gate
	if cache.Mode != "full" {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("Review cache is scoped, run `review@%s:full` before promoting.", unitName),
		}, nil
	}

	// Hash check — stale caches cannot satisfy the promote gate
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

	if len(missingFiles) > 0 || len(mismatchedFiles) > 0 {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("Review cache is stale. Run `review@%s:full` again.", unitName),
		}, nil
	}

	// Blocking declaration check — the gate must be able to determine the
	// blocking status from an explicitly written `blocking` field. A cache
	// without that field fails closed, matching the verify-cache path.
	if !cache.blockingSeen {
		return CheckResult{
			Fresh:  false,
			Reason: "review cache missing required field `blocking` — cannot determine blocking status",
		}, nil
	}

	// Result value check — only the documented result values are valid
	if cache.Result != "pass" && cache.Result != "fail" {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("review cache result is %q, expected 'pass' or 'fail'", cache.Result),
		}, nil
	}

	// Consistency check — per spec_review_checklist.md, `result: fail`
	// means P0/P1 findings exist (blocking: true) and `result: pass`
	// means none exist (blocking: false). A conflicting declaration
	// means the cache was written incorrectly and the gate cannot
	// trust its blocking status.
	if (cache.Result == "fail") != cache.Blocking {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("review cache has conflicting declarations: result %q, blocking %t", cache.Result, cache.Blocking),
		}, nil
	}

	// Blocking check — P0/P1 findings block promote
	if cache.Blocking {
		return CheckResult{
			Fresh:  false,
			Reason: fmt.Sprintf("Review found %d P0 and %d P1 finding(s). Resolve before promoting.", cache.P0Count, cache.P1Count),
		}, nil
	}

	return CheckResult{
		Fresh:  true,
		Reason: fmt.Sprintf("review cache is fresh (result: %s, %d file(s) unchanged)", cache.Result, len(cache.Files)),
	}, nil
}

// CheckRuleVerify is deprecated — rule verify has been removed.
// It always returns not-fresh with a deprecation message.
func CheckRuleVerify(repoRoot, ruleID string) (CheckResult, error) {
	return CheckResult{
		Fresh:  false,
		Reason: "rule verify has been removed (see framework/concepts.md); only rule validate is required for promote",
	}, nil
}

// deleteCache removes a specific cache file for the given target.
func deleteCache(repoRoot, targetKind, targetName, command string) error {
	var fileName string
	switch command {
	case "validate":
		fileName = "validate_result.md"
	case "verify":
		fileName = "verify_result.md"
	case "review":
		fileName = "review_result.md"
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

// DeleteAll removes validate, verify, and review caches for the given unit.
func DeleteAll(repoRoot, unitName string) error {
	if err := DeleteCache(repoRoot, unitName, "validate"); err != nil {
		return err
	}
	if err := DeleteCache(repoRoot, unitName, "verify"); err != nil {
		return err
	}
	return DeleteCache(repoRoot, unitName, "review")
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
			Reason: fmt.Sprintf("%s cache is scoped%s, run `%s@%s:full` before promoting", command, scopeDetail, command, cache.Unit),
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
			Reason: fmt.Sprintf("%s cache stale: files have changed: %s. Run `%s@%s` again.", command, strings.Join(mismatchedFiles, ", "), command, cache.Unit),
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
				case "blocking":
					blocking, err := strconv.ParseBool(value)
					if err != nil {
						return nil, fmt.Errorf("cache file has invalid `blocking` value %q", value)
					}
					cache.Blocking = blocking
					cache.blockingSeen = true
				case "p0_count":
					cache.P0Count, _ = strconv.Atoi(value)
				case "p1_count":
					cache.P1Count, _ = strconv.Atoi(value)
				case "p2_count":
					cache.P2Count, _ = strconv.Atoi(value)
				case "p3_count":
					cache.P3Count, _ = strconv.Atoi(value)
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

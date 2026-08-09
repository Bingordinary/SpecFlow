// Package validationcache provides cache-freshness checking for validate@
// and verify@results. Cache files are written by the agent (not the CLI)
// and read by specflowctl promote to confirm that validate/verify are still fresh.
//
// Cache files for units live under docs/specs/meta/validation/unit/{name}/.
// Cache files for rules live under docs/specs/meta/validation/rule/{id}/.
// They record:
//   - Which files were checked (paths + whole-file hash + dependency chunk CIDs)
//   - Whether the check passed (pass)
//   - When the check was run
//
// specflowctl promote reads both caches, re-chunks every listed file, and
// rejects if a declared dependency chunk CID is no longer present. Content
// changes outside the declared dependency chunks keep the cache fresh and
// surface as an informational note only.
package validationcache

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// CheckCategory classifies why a cache check passed or failed. It mirrors
// the gate vocabulary of the freshness report: fresh (gate satisfied),
// missing (no cache file), stale (re-run the gate to fix), blocked
// (review only: cache is valid but declares P0/P1 findings).
type CheckCategory string

const (
	CategoryFresh   CheckCategory = "fresh"
	CategoryMissing CheckCategory = "missing"
	CategoryStale   CheckCategory = "stale"
	CategoryBlocked CheckCategory = "blocked"
)

// CheckResult describes whether a cache file is fresh. Category records
// the classification from the same check chain that promote relies on,
// so consumers can classify without re-reading the cache themselves.
// Note carries informational context for a FRESH result (e.g. content
// changed outside the declared dependency chunks) — it never blocks.
type CheckResult struct {
	Fresh    bool
	Category CheckCategory
	Reason   string
	Note     string
}

// cacheFile is the parsed representation of a cache file.
type cacheFile struct {
	Command      string `yaml:"command"`
	Unit         string `yaml:"unit"`
	Mode         string `yaml:"mode,omitempty"`
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

// cacheFileEntry is one file in a cache's files list. Hash is the whole-file
// content hash at run time (informational: detects changes outside the
// dependency chunks). Deps are the content identifiers (CIDs) of the chunks
// the run actually depended on — freshness is judged against Deps only.
type cacheFileEntry struct {
	Path string   `yaml:"path"`
	Hash string   `yaml:"hash"`
	Deps []string `yaml:"deps"`
}

// CheckValidate reads and validates the validate cache for the given unit.
// The cache must list the main candidate spec file; a cache whose files list
// omits it cannot prove the main spec was validated.
func CheckValidate(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "validate", "validate_result.md", []string{"pass"}, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName))
}

// CheckVerify reads and validates the verify cache for the given unit.
// A verify cache is valid only with result "pass". P0/P1 findings never
// write a cache (the cache is deleted), and P2/P3 pending findings are
// carried by the severity counts (blocking: false).
func CheckVerify(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "verify", "verify_result.md", []string{"pass"}, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName))
}

// CheckVerifyStable reads and validates the verify cache for the given unit
// against the STABLE spec path. A verify@stable run (verify code against a
// stable unit, no candidate round) records the stable main spec in its files
// list; the candidate-based CheckVerify cannot validate such a cache. The
// fresh stable report uses it to silence baseline drift: a fresh stable
// verify cache means the code was recently confirmed to still conform.
func CheckVerifyStable(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "verify", "verify_result.md", []string{"pass"}, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", unitName))
}

// ReadVerifyDeps returns the declared dependency CIDs per file path from the
// unit's verify cache (path -> deps CID list). Keys are canonicalized to
// repo-relative slash paths (same resolution as the gate's fileFreshness), so
// "./" prefixes, absolute paths, and platform separators in the agent-written
// cache are equivalent. Used by promote to record the verify-time dependencies
// into the baseline. Errors are returned as-is — promote fails loudly rather
// than degrading the baseline.
func ReadVerifyDeps(repoRoot, unitName string) (map[string][]string, error) {
	cachePath := cacheFilePath(repoRoot, "unit", unitName, "verify_result.md")
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, err
	}
	deps := make(map[string][]string, len(cache.Files))
	for _, f := range cache.Files {
		deps[relPath(repoRoot, resolvePath(repoRoot, f.Path))] = f.Deps
	}
	return deps, nil
}

// CheckAppendicesInCache verifies that every non-exempt candidate appendix for
// the given unit is listed in the validate_result.md cache file. This is a
// mechanical promote gate — it ensures the agent included all appendix files
// in the validation run. If an appendix exists on disk but is missing from
// the cache's files list, the agent skipped it.
func CheckAppendicesInCache(repoRoot, unitName string) (CheckResult, error) {
	// 1. Read validate cache
	cachePath := cacheFilePath(repoRoot, "unit", unitName, "validate_result.md")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return CheckResult{
			Fresh:    false,
			Category: CategoryMissing,
			Reason:   fmt.Sprintf("validate cache not found at %s", relPath(repoRoot, cachePath)),
		}, nil
	}
	cache, err := readCache(cachePath)
	if err != nil {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cannot read validate cache at %s: %v", relPath(repoRoot, cachePath), err),
		}, nil
	}
	if cache.Command != "validate" {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cache command is %q, expected 'validate'", cache.Command),
		}, nil
	}
	if cache.Result != "pass" {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("validate cache result is %q, expected 'pass'", cache.Result),
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
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cannot glob appendix files: %v — promote rejected", err),
		}, nil
	}

	// 4. Check each non-exempt candidate appendix (retiring appendices are
	// skipped like exempt ones — promote removes their stable copies instead
	// of copying them)
	var missing []string
	for _, m := range matches {
		relPath, _ := filepath.Rel(repoRoot, m)
		relPathSlash := filepath.ToSlash(relPath)

		// Check status: skip exempt and retired appendices
		data, err := os.ReadFile(m)
		if err == nil {
			fm := specpaths.ReadFrontmatterStringMap(string(data))
			status := strings.TrimSpace(fm["status"])
			if status == "exempt" || status == "retired" {
				continue
			}
		}

		if !cachedPaths[relPathSlash] {
			missing = append(missing, relPathSlash)
		}
	}

	if len(missing) > 0 {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason: fmt.Sprintf("appendix file(s) not included in validation: %s. Run `validate@%s` again.",
				strings.Join(missing, ", "), unitName),
		}, nil
	}

	return CheckResult{
		Fresh:    true,
		Category: CategoryFresh,
		Reason:   fmt.Sprintf("all %d appendix file(s) are included in validate cache", len(matches)),
	}, nil
}

// CheckRuleValidate reads and validates the validate cache for the given rule.
// The cache must list the main candidate rule file; a cache whose files list
// omits it cannot prove the rule was validated.
func CheckRuleValidate(repoRoot, ruleID string) (CheckResult, error) {
	return checkCache(repoRoot, "rule", ruleID, "validate", "validate_result.md", []string{"pass"}, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ruleID))
}

// CheckReview reads and validates the review cache for the given unit.
// The review cache is a required promote gate: it must exist, mode must be
// "full", the declared dependency chunks must be unchanged, and it must not
// be blocking (P0/P1 findings).
// If any condition fails, promote must be rejected with guidance.
func CheckReview(repoRoot, unitName string) (CheckResult, error) {
	cachePath := cacheFilePath(repoRoot, "unit", unitName, "review_result.md")

	// Existence check — review cache is required for promote
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return CheckResult{
			Fresh:    false,
			Category: CategoryMissing,
			Reason:   fmt.Sprintf("Review not completed. Run `review@%s` first.", unitName),
		}, nil
	}

	cache, err := readCache(cachePath)
	if err != nil {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cannot read review cache: %v", err),
		}, nil
	}

	if cache.Command != "review" {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("review cache command is %q, expected 'review'", cache.Command),
		}, nil
	}

	// Mode check — only full-mode caches satisfy the promote gate
	if cache.Mode != "full" {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("review cache mode is %q, expected 'full' — run `review@%s` before promoting", cache.Mode, unitName),
		}, nil
	}

	// Dependency check — stale caches cannot satisfy the promote gate. Freshness is
	// judged on the declared dependency chunks; content changes outside them
	// are informational only.
	var mismatchedFiles []string
	var missingFiles []string
	var changedFiles []string
	for _, entry := range cache.Files {
		state, changed, err := fileFreshness(repoRoot, entry)
		if err != nil {
			missingFiles = append(missingFiles, fmt.Sprintf("%s (%v)", entry.Path, err))
			continue
		}
		if changed {
			changedFiles = append(changedFiles, entry.Path)
		}
		switch state {
		case fileMissing:
			missingFiles = append(missingFiles, entry.Path)
		case fileNoDeps:
			mismatchedFiles = append(mismatchedFiles, fmt.Sprintf("%s (no dependency chunks declared)", entry.Path))
		case fileDepChanged:
			mismatchedFiles = append(mismatchedFiles, entry.Path)
		}
	}

	if len(missingFiles) > 0 || len(mismatchedFiles) > 0 {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("Review cache is stale. Run `review@%s` again.", unitName),
		}, nil
	}

	// Blocking declaration check — the gate must be able to determine the
	// blocking status from an explicitly written `blocking` field. A cache
	// without that field fails closed, matching the verify-cache path.
	if !cache.blockingSeen {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   "review cache missing required field `blocking` — cannot determine blocking status",
		}, nil
	}

	// Result value check — only the documented result values are valid
	if cache.Result != "pass" && cache.Result != "fail" {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("review cache result is %q, expected 'pass' or 'fail'", cache.Result),
		}, nil
	}

	// Consistency check — per spec_review_checklist.md, `result: fail`
	// means P0/P1 findings exist (blocking: true) and `result: pass`
	// means none exist (blocking: false). A conflicting declaration
	// means the cache was written incorrectly and the gate cannot
	// trust its blocking status.
	if (cache.Result == "fail") != cache.Blocking {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("review cache has conflicting declarations: result %q, blocking %t", cache.Result, cache.Blocking),
		}, nil
	}

	// Blocking check — P0/P1 findings block promote
	if cache.Blocking {
		return CheckResult{
			Fresh:    false,
			Category: CategoryBlocked,
			Reason:   fmt.Sprintf("Review found %d P0 and %d P1 finding(s). Resolve before promoting.", cache.P0Count, cache.P1Count),
		}, nil
	}

	result := CheckResult{
		Fresh:    true,
		Category: CategoryFresh,
		Reason:   fmt.Sprintf("review cache is fresh (result: %s, dependency chunks of %d file(s) unchanged)", cache.Result, len(cache.Files)),
	}
	if len(changedFiles) > 0 {
		result.Note = fmt.Sprintf("review: content changed outside the declared dependency chunks in %s — the gate stays fresh, but if semantic coupling exists (e.g. called functions, shared structures), consider re-running `review@%s`", strings.Join(changedFiles, ", "), unitName)
	}
	return result, nil
}

// CacheSummary is a read-only summary of a cache file's frontmatter,
// used by freshness reporting. It does not perform any freshness check.
type CacheSummary struct {
	Command   string
	Unit      string
	Mode      string
	Result    string
	Target    string
	Blocking  bool
	P0Count   int
	P1Count   int
	P2Count   int
	P3Count   int
	Timestamp string
	FileCount int
}

// ReadCacheSummary parses the given cache file (fileName is e.g.
// "validate_result.md") and returns its frontmatter summary. It returns
// an error only if the file is missing or malformed.
func ReadCacheSummary(repoRoot, targetKind, targetName, fileName string) (*CacheSummary, error) {
	path := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	cache, err := readCache(path)
	if err != nil {
		return nil, err
	}
	return &CacheSummary{
		Command:   cache.Command,
		Unit:      cache.Unit,
		Mode:      cache.Mode,
		Result:    cache.Result,
		Target:    cache.Target,
		Blocking:  cache.Blocking,
		P0Count:   cache.P0Count,
		P1Count:   cache.P1Count,
		P2Count:   cache.P2Count,
		P3Count:   cache.P3Count,
		Timestamp: cache.Timestamp,
		FileCount: len(cache.Files),
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

// ------------------------------------------------------------
// Internal
// ------------------------------------------------------------

// fileFreshnessState classifies the freshness of one cache file entry
// against the file's current content.
type fileFreshnessState int

const (
	fileMissing   fileFreshnessState = iota // file is gone from disk
	fileNoDeps                              // entry declares no dependency chunks but the file has content
	fileDepChanged                          // a declared dependency chunk CID is no longer present
	fileFresh                               // every declared dependency chunk is unchanged
)

// fileFreshness re-chunks the current file and checks every declared
// dependency chunk CID against it. Freshness is judged on the dependency
// chunks only — content changes outside the declared dependencies do not
// stale the cache but are reported as changed (informational). The
// whole-file hash recorded in the cache powers that informational
// comparison.
func fileFreshness(repoRoot string, entry cacheFileEntry) (state fileFreshnessState, changed bool, err error) {
	fullPath := resolvePath(repoRoot, entry.Path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fileMissing, false, nil
		}
		return fileMissing, false, err
	}

	text := specpaths.NormalizeText(string(data))
	fc := contenthash.ChunkText(text)

	// Whole-file comparison is informational only: it detects changes
	// outside the declared dependency chunks so fresh reports can warn
	// about possible semantic coupling without failing the gate.
	if entry.Hash != "" {
		if normalizeHash(entry.Hash) != normalizeHash(contenthash.FileHashText(text)) {
			changed = true
		}
	}

	if len(entry.Deps) == 0 {
		if len(fc.Chunks) > 0 {
			return fileNoDeps, changed, nil
		}
		return fileFresh, changed, nil
	}

	present := make(map[string]bool, len(fc.Chunks))
	for _, c := range fc.Chunks {
		present[normalizeHash(c.CID)] = true
	}
	for _, dep := range entry.Deps {
		if !present[normalizeHash(dep)] {
			return fileDepChanged, changed, nil
		}
	}
	return fileFresh, changed, nil
}

func checkCache(repoRoot, targetKind, targetName, command, fileName string, validResults []string, requiredMainFile string) (CheckResult, error) {
	cachePath := cacheFilePath(repoRoot, targetKind, targetName, fileName)

	// Check existence
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return CheckResult{
			Fresh:    false,
			Category: CategoryMissing,
			Reason:   fmt.Sprintf("%s cache not found at %s", command, relPath(repoRoot, cachePath)),
		}, nil
	}

	// Parse cache file
	cache, err := readCache(cachePath)
	if err != nil {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cannot read %s cache: %v", command, err),
		}, nil
	}

	// Validate command matches
	if cache.Command != command {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cache command is %q, expected %q", cache.Command, command),
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
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache result is %q, expected one of %v", command, cache.Result, validResults),
		}, nil
	}

	// Reject non-full mode — only full-mode caches satisfy the promote gate.
	// Fail closed: a missing or invalid mode value cannot prove a full run.
	// A cache exists only when the command ran in full mode (targeted runs
	// do not write caches), so mode must always be "full".
	if cache.Mode != "full" {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache mode is %q, expected 'full' — run `%s@%s` before promoting", command, cache.Mode, command, cache.Unit),
		}, nil
	}

	// Require the main file to be listed. A cache whose files list omits the
	// main candidate spec (or rule) cannot prove that file was read during the
	// run, so promote must not treat the gate as satisfied.
	if requiredMainFile != "" {
		mainAbs := resolvePath(repoRoot, requiredMainFile)
		listed := false
		for _, entry := range cache.Files {
			if resolvePath(repoRoot, entry.Path) == mainAbs {
				listed = true
				break
			}
		}
		if !listed {
			return CheckResult{
				Fresh:    false,
				Category: CategoryStale,
				Reason:   fmt.Sprintf("%s cache files list does not include the main %s file %s. Run `%s@%s` again.", command, targetKind, requiredMainFile, command, cache.Unit),
			}, nil
		}
	}

	// Re-check all listed files against their declared dependency chunks
	var mismatchedFiles []string
	var missingFiles []string
	var changedFiles []string
	for _, entry := range cache.Files {
		state, changed, err := fileFreshness(repoRoot, entry)
		if err != nil {
			missingFiles = append(missingFiles, fmt.Sprintf("%s (%v)", entry.Path, err))
			continue
		}
		if changed {
			changedFiles = append(changedFiles, entry.Path)
		}
		switch state {
		case fileMissing:
			missingFiles = append(missingFiles, entry.Path)
		case fileNoDeps:
			mismatchedFiles = append(mismatchedFiles, fmt.Sprintf("%s (no dependency chunks declared — cache was written before content-addressed freshness or the declared ranges covered no content; run `%s@%s` again)", entry.Path, command, cache.Unit))
		case fileDepChanged:
			mismatchedFiles = append(mismatchedFiles, entry.Path)
		}
	}

	if len(missingFiles) > 0 {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache stale: files missing: %s", command, strings.Join(missingFiles, ", ")),
		}, nil
	}
	if len(mismatchedFiles) > 0 {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache stale: dependency chunks have changed: %s. Run `%s@%s` again.", command, strings.Join(mismatchedFiles, ", "), command, cache.Unit),
		}, nil
	}

	result := CheckResult{
		Fresh:    true,
		Category: CategoryFresh,
		Reason:   fmt.Sprintf("%s cache is fresh (result: %s, dependency chunks of %d file(s) unchanged)", command, cache.Result, len(cache.Files)),
	}
	if len(changedFiles) > 0 {
		result.Note = fmt.Sprintf("%s: content changed outside the declared dependency chunks in %s — the gate stays fresh, but if semantic coupling exists (e.g. called functions, shared structures), consider re-running `%s@%s`", command, strings.Join(changedFiles, ", "), command, cache.Unit)
	}
	return result, nil
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
	inDepsBlock := false

	for _, line := range fmLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect files block entries
		if trimmed == "files:" {
			inFilesBlock = true
			inDepsBlock = false
			continue
		}

		if inFilesBlock {
			if strings.HasPrefix(trimmed, "- path:") {
				// New entry
				inDepsBlock = false
				path := strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:"))
				path = strings.Trim(path, "\"'")
				currentEntry = &cacheFileEntry{Path: path}
				cache.Files = append(cache.Files, *currentEntry)
				continue
			}
			if trimmed == "deps:" {
				inDepsBlock = true
				continue
			}
			if inDepsBlock {
				if strings.HasPrefix(trimmed, "-") {
					dep := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
					dep = strings.Trim(dep, "\"'")
					if currentEntry != nil {
						currentEntry.Deps = append(currentEntry.Deps, dep)
						cache.Files[len(cache.Files)-1] = *currentEntry
					}
					continue
				}
				// A non-list line ends the deps block
				inDepsBlock = false
			}
			if strings.HasPrefix(trimmed, "hash:") && currentEntry != nil {
				hash := strings.TrimSpace(strings.TrimPrefix(trimmed, "hash:"))
				hash = strings.Trim(hash, "\"'")
				currentEntry.Hash = hash
				cache.Files[len(cache.Files)-1] = *currentEntry
				continue
			}
			// If we hit a non-empty line that doesn't start with - or is a continuation,
			// we might have left the files block
			if currentEntry != nil {
				inFilesBlock = false
				inDepsBlock = false
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

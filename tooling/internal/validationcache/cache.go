// Package validationcache assembles, writes, and checks validate, verify, and
// review cache records. gate-finalize writes records from accepted gate-run
// artifacts; freshness and promote commands consume them mechanically.
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
	"path"
	"path/filepath"
	"sort"
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
	Basis        string `yaml:"basis,omitempty"`
	Result       string `yaml:"result"`
	Target       string `yaml:"target,omitempty"`
	Blocking     bool   `yaml:"blocking"`
	blockingSeen bool   `yaml:"-"`
	P0Count      int    `yaml:"p0_count"`
	P1Count      int    `yaml:"p1_count"`
	P2Count      int    `yaml:"p2_count"`
	P3Count      int    `yaml:"p3_count"`
	Timestamp    string `yaml:"timestamp"`
	// GateRun is the audit id of the gate run whose input snapshot this cache
	// was finalized against (see framework/validation_cache.md §Format → Gate
	// run binding). It is audit metadata only — freshness checks never read
	// it, and it proves nothing about who executed the run. Absent on caches
	// written before gate runs existed.
	GateRun string `yaml:"gate_run,omitempty"`
	// InvalidatedChecks records targeted P0/P1 contradictions discovered
	// after this failure record was written. It is machine-owned recovery
	// state, separate from the historical per-check status map.
	InvalidatedChecks []string `yaml:"invalidated_checks,omitempty"`
	invalidatedSeen   bool     `yaml:"-"`
	Files             []cacheFileEntry
}

// cacheFileEntry is one file in a cache's files list. Hash is the whole-file
// content hash at run time (informational: detects changes outside the
// dependency chunks). Deps are the content identifiers (CIDs) of the chunks
// the run actually depended on — freshness is judged against Deps only.
// Checks is the per-check dependency breakdown (check key -> the CIDs that
// check's judgment depended on); it is the mechanism-derived delta scope
// input (see StaleRegions) and is optional — a cache without it degrades to
// file-level delta derivation.
type cacheFileEntry struct {
	Path   string       `yaml:"path"`
	Hash   string       `yaml:"hash"`
	Deps   []string     `yaml:"deps"`
	Checks []checkEntry `yaml:"checks,omitempty"`
}

// checkEntry is one check's dependency declaration inside a files entry:
// the check key (validate: "1"-"8" — unit and rule; verify: acceptance item
// id; review: the reviewed file path) and the CIDs the check's judgment
// actually depended on. The file-level Deps list of the same entry is the
// union of all check deps plus any undeclared remainder — the promote gate
// judges freshness on that union; the per-check breakdown exists for delta
// scope derivation only. Status records the judgment outcome in a
// failure-record cache (a fail/blocking cache — delta FAIL records, review
// blocking caches, stable-only confirmation FAIL records): pass (judgment
// ran and passed), fail (judgment ran and retained at least one finding of
// any severity — P0–P3; the gate result itself is decided by P0/P1), carried
// (not re-run — evidence unchanged from the pass baseline; delta records
// only). Status is
// required on every fail/blocking cache (the recovery scope input); absent
// status means pass only on a pass cache — the recovery never treats a
// failure record without a status map as pass (it degrades to a full re-run).
type checkEntry struct {
	Check  string   `yaml:"check"`
	Status string   `yaml:"status,omitempty"` // pass | fail | carried (required on fail/blocking caches)
	Deps   []string `yaml:"deps"`
}

// CheckValidate reads and validates the validate cache for the given unit.
// The cache must list the main candidate spec file; a cache whose files list
// omits it cannot prove the main spec was validated. A fail-result cache (a
// delta re-run's failure record) is rejected as blocking by the same chain
// review uses.
func CheckValidate(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "validate", "validate_result.md", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName))
}

// CheckVerify reads and validates the verify cache for the given unit.
// A pass-result cache satisfies the gate; a fail-result cache (a delta
// re-run's or a candidate full-run FAIL's failure record) is rejected as
// blocking by the same chain review uses. P2/P3 pending findings are carried
// by the severity counts on a pass cache (blocking: false).
func CheckVerify(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "verify", "verify_result.md", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName))
}

// CheckVerifyStable reads and validates the verify cache for the given unit
// against the STABLE spec path. A verify@stable run (verify code against a
// stable unit, no candidate round) records the stable main spec in its files
// list; the candidate-based CheckVerify cannot validate such a cache. The
// fresh stable report uses it to silence baseline drift: a fresh stable
// verify cache means the code was recently confirmed to still conform.
func CheckVerifyStable(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "verify", "verify_result.md", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", unitName))
}

// CheckValidateStable reads and validates the validate cache for the given
// unit against the STABLE spec path. A validate@stable run (validate the
// stable content against the current dependencies and rules, no candidate
// round) records the stable main spec in its files list; the
// candidate-based CheckValidate cannot validate such a cache (its main-file
// check points at the candidate spec). The fresh stable report consumes it
// as the stable content's dependency/rule confirmation state.
func CheckValidateStable(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "validate", "validate_result.md", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", unitName))
}

// ReadVerifyDeps returns the declared dependency CIDs per file path from the
// unit's verify cache (path -> deps CID list). Keys are canonicalized to
// repo-relative slash paths (same resolution as the gate's fileFreshness), so
// "./" prefixes, absolute paths, and platform separators in the agent-written
// cache are equivalent. Used by promote to record the verify-time dependencies
// into the baseline. Errors are returned as-is — promote fails loudly rather
// than degrading the baseline.
func ReadVerifyDeps(repoRoot, unitName string) (map[string][]string, error) {
	cachePath, err := cacheFilePath(repoRoot, "unit", unitName, "verify_result.md")
	if err != nil {
		return nil, err
	}
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, err
	}
	deps := make(map[string][]string, len(cache.Files))
	for _, f := range cache.Files {
		p := resolveEntryPath(repoRoot, f.Path)
		if p == "" {
			continue // logical reference with no applicable file — nothing to record
		}
		deps[relPath(repoRoot, p)] = f.Deps
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
	cachePath, err := cacheFilePath(repoRoot, "unit", unitName, "validate_result.md")
	if err != nil {
		return CheckResult{}, err
	}
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
	return checkAppendicesInCache(repoRoot, unitName, cache)
}

// CheckAppendicesInRenderedCache applies the appendix coverage gate to a
// rendered cache candidate before it is published to the canonical path.
func CheckAppendicesInRenderedCache(repoRoot, unitName string, content []byte) (CheckResult, error) {
	cache, err := parseCache(content)
	if err != nil {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cannot read rendered validate cache: %v", err),
		}, nil
	}
	return checkAppendicesInCache(repoRoot, unitName, cache)
}

func checkAppendicesInCache(repoRoot, unitName string, cache *cacheFile) (CheckResult, error) {
	if err := specpaths.ValidateTargetName("unit", unitName); err != nil {
		return CheckResult{}, err
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
	return checkCache(repoRoot, "rule", ruleID, "validate", "validate_result.md", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ruleID))
}

// CheckRuleValidateStable reads and validates the validate cache for the given
// rule against the STABLE rule path. A validate@stable run on a rule (no
// candidate round) records the stable rule file and the consumer units it
// scanned; the candidate-based CheckRuleValidate cannot validate such a cache
// (its main-file check points at the candidate rule). The fresh stable report
// consumes it as the stable rule's consumer/consistency confirmation state.
func CheckRuleValidateStable(repoRoot, ruleID string) (CheckResult, error) {
	return checkCache(repoRoot, "rule", ruleID, "validate", "validate_result.md", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/rules/stable/%s.md", ruleID))
}

// CheckReview reads and validates the review cache for the given unit.
// The review cache is a required promote gate: it must exist, mode must be
// "full", the declared dependency chunks must be unchanged, and it must not
// be blocking (P0/P1 findings).
// If any condition fails, promote must be rejected with guidance.
func CheckReview(repoRoot, unitName string) (CheckResult, error) {
	return checkReview(repoRoot, unitName, "")
}

// CheckReviewStable reads and validates the review cache for the given unit
// as the stable-layer quality confirmation. The review gate has no main-file
// requirement (its evidence is the reviewed code surface, not a spec), so the
// layer is separated by the `target` field: only a cache recorded with
// `target: stable` by an @stable confirmation run can prove the stable
// confirmation state. A candidate review cache (no `target` or
// `target: candidate`) fails this check closed, so the fresh stable report
// never mislabels a candidate review as the stable confirmation.
func CheckReviewStable(repoRoot, unitName string) (CheckResult, error) {
	return checkReview(repoRoot, unitName, "stable")
}

// blockingCheck validates the blocking declarations of a fail-capable cache
// (review, and validate/verify failure records written by delta re-runs or a
// candidate verify full-run FAIL). It
// fails closed on a missing `blocking` field or a conflicting result/blocking
// declaration, and classifies a P0/P1 cache as CategoryBlocked (promote
// rejected, fresh reports BLOCKED). A nil result means the cache declares a
// consistent non-blocking state and the caller continues its normal checks.
func blockingCheck(command string, cache *cacheFile) *CheckResult {
	// Blocking declaration check — the gate must be able to determine the
	// blocking status from an explicitly written `blocking` field. A cache
	// without that field fails closed.
	if !cache.blockingSeen {
		return &CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache missing required field `blocking` — cannot determine blocking status", command),
		}
	}

	// Result value check — only the documented result values are valid
	if cache.Result != "pass" && cache.Result != "fail" {
		return &CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache result is %q, expected 'pass' or 'fail'", command, cache.Result),
		}
	}

	// Consistency check — `result: fail` means P0/P1 findings exist
	// (blocking: true) and `result: pass` means none exist (blocking: false).
	// A conflicting declaration means the cache was written incorrectly and
	// the gate cannot trust its blocking status.
	if (cache.Result == "fail") != cache.Blocking {
		return &CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache has conflicting declarations: result %q, blocking %t", command, cache.Result, cache.Blocking),
		}
	}

	// Blocking check — P0/P1 findings block promote
	if cache.Blocking {
		return &CheckResult{
			Fresh:    false,
			Category: CategoryBlocked,
			Reason:   fmt.Sprintf("%s found %d P0 and %d P1 finding(s). Resolve before promoting.", capitalize(command), cache.P0Count, cache.P1Count),
		}
	}

	return nil
}

// capitalize uppercases the first rune of s (used for command names in gate
// reason text, e.g. "review" → "Review").
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func checkReview(repoRoot, unitName, requiredTarget string) (CheckResult, error) {
	cachePath, err := cacheFilePath(repoRoot, "unit", unitName, "review_result.md")
	if err != nil {
		return CheckResult{}, err
	}

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
	return checkReviewCache(repoRoot, unitName, requiredTarget, cache)
}

func checkReviewCache(repoRoot, unitName, requiredTarget string, cache *cacheFile) (CheckResult, error) {
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

	// Layer check — a stable-layer confirmation cache must declare its layer
	// via `target: stable` (the review gate has no main-file requirement to
	// separate the layers by path). Fail closed: a cache without the
	// declaration cannot prove the stable confirmation state.
	if requiredTarget != "" && cache.Target != requiredTarget {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("review cache target is %q, expected %q — the stable confirmation cache must be recorded with `target: stable` by an @stable review run", cache.Target, requiredTarget),
		}, nil
	}

	// Dependency check — stale caches cannot satisfy the promote gate. Freshness is
	// judged on the declared dependency chunks; content changes outside them
	// are informational only.
	var mismatchedFiles []string
	var missingFiles []string
	var changedFiles []string
	for _, entry := range cache.Files {
		if ok, why := checksUnionSubsetOfDeps(entry); !ok {
			mismatchedFiles = append(mismatchedFiles, fmt.Sprintf("%s (per-check deps missing from the file-level deps union: %s)", entry.Path, why))
			continue
		}
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

	// Blocking declaration and result checks — shared with the validate/verify
	// failure-record path (see blockingCheck).
	if res := blockingCheck("review", cache); res != nil {
		return *res, nil
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

// FileEntry is the full content of one cache `files` entry. Hash and Deps are
// computed by the tooling (contenthash), never supplied by the agent — a
// manually transcribed CID is the transcription error source this package's
// gate-finalize path eliminates. Checks carries the optional per-check
// breakdown (see the Format section of framework/validation_cache.md).
type FileEntry struct {
	Path   string       `json:"path"`
	Hash   string       `json:"hash,omitempty"`
	Deps   []string     `json:"deps,omitempty"`
	Checks []CheckEntry `json:"checks,omitempty"`
}

// CheckEntry is one per-check dependency declaration inside a files entry
// (check key + optional status + the CIDs the check's judgment depended on).
type CheckEntry struct {
	Check  string   `json:"check"`
	Status string   `json:"status,omitempty"` // pass | fail | carried (required on fail/blocking caches)
	Deps   []string `json:"deps,omitempty"`
}

// EntryDeclaration is the agent-facing declaration for one cache files entry
// (the input to gate-finalize). The agent declares the check keys, statuses,
// and section-region headings / line ranges its judgment depended on; the
// tooling resolves the declarations to CIDs and computes the whole-file hash.
// Ranges uses the same START-END,START-END grammar as contenthash.ParseRanges.
type EntryDeclaration struct {
	Path              string             `json:"path"`
	Checks            []CheckDeclaration `json:"checks,omitempty"`
	Ranges            string             `json:"ranges,omitempty"`
	Sections          []string           `json:"sections,omitempty"`
	AcceptanceItems   bool               `json:"acceptance_items,omitempty"`
	AcceptanceItemIDs []string           `json:"acceptance_item_ids,omitempty"`
}

// CheckDeclaration is one per-check declaration inside an entry declaration.
// Status is pass | fail | carried (required on fail/blocking caches). When
// Sections, Ranges, AcceptanceItems, and AcceptanceItemIDs are all empty the
// check declares a whole-file dependency (conservative). AcceptanceItems
// (the whole set) and AcceptanceItemIDs (specific items) may be combined —
// the deps are the union (declare-heavy conservatism).
type CheckDeclaration struct {
	Check             string   `json:"check"`
	Status            string   `json:"status,omitempty"`
	Sections          []string `json:"sections,omitempty"`
	Ranges            string   `json:"ranges,omitempty"`
	AcceptanceItems   bool     `json:"acceptance_items,omitempty"`
	AcceptanceItemIDs []string `json:"acceptance_item_ids,omitempty"`
}

// BuildEntry computes the cache files entry for one declaration: the
// whole-file hash and the dependency CIDs are computed by the tooling, never
// supplied by the agent. The file-level deps are the union of every declared
// per-check dep plus the entry-level declaration (union discipline — the
// promote gate fails closed on a per-check dep missing from the union). For a
// logical reference the applicable file is used: current-layer for units and
// bound rules, stable-only for global rules. Freshness checks use the same
// resolution.
func BuildEntry(repoRoot string, d EntryDeclaration) (FileEntry, error) {
	abs := resolveEntryPath(repoRoot, d.Path)
	if abs == "" {
		return FileEntry{}, fmt.Errorf("cannot resolve cache entry %q to an applicable file", d.Path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return FileEntry{}, fmt.Errorf("read %s: %w", d.Path, err)
	}
	text := specpaths.NormalizeText(string(data))

	entry := FileEntry{Path: d.Path}
	hash, entryDeps, err := declarationDeps(text, d.Path, declFields{
		Sections:          d.Sections,
		Ranges:            d.Ranges,
		AcceptanceItems:   d.AcceptanceItems,
		AcceptanceItemIDs: d.AcceptanceItemIDs,
	})
	if err != nil {
		return FileEntry{}, err
	}
	entry.Hash = hash
	union := make(map[string]bool)
	for _, dep := range entryDeps {
		union[dep] = true
	}
	for _, cd := range d.Checks {
		check := strings.TrimSpace(cd.Check)
		if check == "" {
			return FileEntry{}, fmt.Errorf("entry %q declares a check with an empty key", d.Path)
		}
		_, checkDeps, cerr := declarationDeps(text, d.Path, declFields{
			Sections:          cd.Sections,
			Ranges:            cd.Ranges,
			AcceptanceItems:   cd.AcceptanceItems,
			AcceptanceItemIDs: cd.AcceptanceItemIDs,
		})
		if cerr != nil {
			return FileEntry{}, cerr
		}
		for _, dep := range checkDeps {
			union[dep] = true
		}
		entry.Checks = append(entry.Checks, CheckEntry{Check: check, Status: cd.Status, Deps: checkDeps})
	}

	// The file-level union preserves declaration order: entry-level deps
	// first, then check deps not already present (declare-heavy extras are
	// legal; every check dep must already be in the union).
	var ordered []string
	seen := make(map[string]bool)
	for _, dep := range entryDeps {
		if !seen[dep] {
			seen[dep] = true
			ordered = append(ordered, dep)
		}
	}
	for _, c := range entry.Checks {
		for _, dep := range c.Deps {
			if !seen[dep] {
				seen[dep] = true
				ordered = append(ordered, dep)
			}
		}
	}
	entry.Deps = ordered
	return entry, nil
}

// declFields carries the declared dependency scope of one entry or check
// declaration (the zero value declares nothing — the whole-file fallback).
// The declared forms are merged into one deps list: line ranges (chunk CIDs),
// section regions (`region:section:...`), the whole acceptance item set
// (`region:acceptance_items:...`), and specific acceptance items
// (`region:acceptance_item:<id>:...`).
type declFields struct {
	Sections          []string
	Ranges            string
	AcceptanceItems   bool
	AcceptanceItemIDs []string
}

// declarationDeps computes the dependency CIDs for one declaration (entry or
// check level). When nothing is declared, the whole file is declared
// (conservative). A declared section heading or acceptance item id that
// cannot be located — missing or duplicated — fails closed: the declaration
// would otherwise silently bind to the wrong region or none at all.
func declarationDeps(text, path string, d declFields) (string, []string, error) {
	fc := contenthash.ChunkText(text)

	parsed, perr := contenthash.ParseRanges(d.Ranges)
	if perr != nil {
		return "", nil, perr
	}

	nothingDeclared := len(parsed) == 0 && len(d.Sections) == 0 && !d.AcceptanceItems && len(d.AcceptanceItemIDs) == 0
	if nothingDeclared {
		var deps []string
		for _, c := range fc.Chunks {
			deps = append(deps, c.CID)
		}
		return contenthash.FileHashText(text), deps, nil
	}

	var deps []string
	if len(parsed) > 0 {
		for _, r := range parsed {
			if r[1] > fc.LineCount() {
				return "", nil, fmt.Errorf("range %d-%d exceeds the file's line count (%d lines)", r[0], r[1], fc.LineCount())
			}
		}
		deps = append(deps, contenthash.CIDsForRanges(fc, parsed)...)
	}
	for _, heading := range d.Sections {
		dep, derr := sectionDep(text, heading)
		if derr != nil {
			return "", nil, derr
		}
		deps = append(deps, dep)
	}
	if d.AcceptanceItems {
		region, ok := contenthash.AcceptanceItemsRegion(text)
		if !ok {
			return "", nil, fmt.Errorf("acceptance_item_set region not found in %s — cannot declare the structural dependency", path)
		}
		deps = append(deps, "region:acceptance_items:"+contenthash.RegionCID(region))
	}
	for _, id := range d.AcceptanceItemIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return "", nil, fmt.Errorf("declaration for %s carries an empty acceptance item id", path)
		}
		region, ok := contenthash.LocateAcceptanceItemRegion(text, id)
		if !ok {
			return "", nil, fmt.Errorf("acceptance item %q not found in %s (or declared more than once) — list the items with `specflowctl gate-evidence --items`", id, path)
		}
		deps = append(deps, "region:acceptance_item:"+id+":"+contenthash.RegionCID(region.Text))
	}
	return contenthash.FileHashText(text), deps, nil
}

// sectionDep computes a section-region dependency (or the frontmatter region
// for the "frontmatter" spelling) for the given normalized text.
func sectionDep(text, heading string) (string, error) {
	requested := heading
	if heading == "frontmatter" {
		// Presence, not uniqueness: a duplicated real heading must be
		// rejected the same way as a single one, otherwise the reserved
		// spelling silently binds to the pre-heading region.
		if contenthash.HasSectionHeading(text, "frontmatter") {
			return "", fmt.Errorf("reserved heading %q: the file has a real ## frontmatter section, which cannot be declared by --section frontmatter (the spelling names the pre-heading region) — rename the section", requested)
		}
		heading = ""
	}
	if heading == "" && !contenthash.IsSectionSplittable(text) {
		// A spec with no ## heading cannot be declared by section at all —
		// the frontmatter spelling would silently alias the whole file
		// (framework/validation_cache.md §Structural Region Dependencies).
		return "", fmt.Errorf("section %q cannot be declared: the file has no ## heading — section regions cannot be located; restructure the spec per framework/spec_writing_guide.md §13 or declare the whole file", requested)
	}
	region, ok := contenthash.LocateSectionRegion(text, heading)
	if !ok {
		return "", fmt.Errorf("section %q not found (or declared more than once) — list the sections with gate-evidence --sections", requested)
	}
	return "region:section:" + heading + ":" + contenthash.RegionCID(region.Text), nil
}

// CacheWrite is the internal gate-finalize rendering bundle. gate-finalize
// derives the judgment fields from accepted packet artifacts and computes the
// Hash/Deps inside Entries from their validated declarations.
type CacheWrite struct {
	Command   string
	Unit      string
	Mode      string
	Basis     string
	Result    string
	Target    string
	Blocking  bool
	P0Count   int
	P1Count   int
	P2Count   int
	P3Count   int
	Timestamp string
	// GateRun is the audit id of the gate run that bracketed this write
	// (printed by gate-plan, verified by gate-finalize). Rendering is
	// audit-only — see cacheFile.GateRun.
	GateRun string
	// InvalidatedChecks is used only when rendering an existing failure
	// record's machine-owned targeted invalidation state. Gate-finalize leaves
	// it empty, which clears every invalidation the completed plan re-ran.
	InvalidatedChecks []string
	// Judgments is the machine-readable synthesis baseline JSON. It is stored
	// in a marked comment block before the human-readable body.
	Judgments string
	Body      string
	Entries   []FileEntry
}

// RenderCache renders a complete cache candidate in memory. It performs no
// filesystem writes; gate-finalize validates these bytes before publishing.
func RenderCache(targetName string, w CacheWrite) ([]byte, error) {
	content, err := renderCacheFrontmatter(w, targetName)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(w.Judgments) != "" {
		content += "\n<!-- GATE_JUDGMENTS_BEGIN\n" + strings.TrimSpace(w.Judgments) + "\nGATE_JUDGMENTS_END -->"
	}
	content += "\n" + w.Body
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return []byte(content), nil
}

func cacheFileName(command string) (string, error) {
	var fileName string
	switch command {
	case "validate":
		fileName = "validate_result.md"
	case "verify":
		fileName = "verify_result.md"
	case "review":
		fileName = "review_result.md"
	default:
		return "", fmt.Errorf("unknown cache command %q", command)
	}
	return fileName, nil
}

// PublishCache atomically replaces the canonical cache with bytes that have
// already passed the full candidate checks. A failed temp write or rename
// leaves any existing canonical cache untouched.
func PublishCache(repoRoot, targetKind, targetName, command string, content []byte) (string, error) {
	fileName, err := cacheFileName(command)
	if err != nil {
		return "", err
	}
	cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return "", fmt.Errorf("create cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(cachePath), ".specflow-cache-*")
	if err != nil {
		return "", fmt.Errorf("create cache temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write cache temp file: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("set cache temp file mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close cache temp file: %w", err)
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		return "", fmt.Errorf("publish cache: %w", err)
	}
	removeTemp = false
	return cachePath, nil
}

// WriteCache is the direct renderer/publisher used by test and setup code.
// Gate-finalize uses RenderCache, validates the candidate, then PublishCache.
func WriteCache(repoRoot, targetKind, targetName string, w CacheWrite) (string, error) {
	content, err := RenderCache(targetName, w)
	if err != nil {
		return "", err
	}
	return PublishCache(repoRoot, targetKind, targetName, w.Command, content)
}

// CacheResult mirrors CheckResult for gate-finalize output.
type CacheResult struct {
	Fresh    bool
	Category CheckCategory
	Reason   string
}

// CheckWriteResult runs the gate's full freshness chain against a freshly
// written cache. It re-reads the file and runs the exact checks promote and
// fresh use, so a cache that passes self-check is accepted by the gates.
// Layer routing mirrors the promote gate's layer separation: the stable
// validate/verify variants point the main-file check at the stable spec, and
// review requires target: stable.
func CheckWriteResult(repoRoot, targetKind, targetName, command, target string) (CacheResult, error) {
	var (
		res CheckResult
		err error
	)
	switch {
	case targetKind == "unit" && command == "validate" && target == "stable":
		res, err = CheckValidateStable(repoRoot, targetName)
	case targetKind == "unit" && command == "validate":
		res, err = CheckValidate(repoRoot, targetName)
	case targetKind == "unit" && command == "verify" && target == "stable":
		res, err = CheckVerifyStable(repoRoot, targetName)
	case targetKind == "unit" && command == "verify":
		res, err = CheckVerify(repoRoot, targetName)
	case targetKind == "unit" && command == "review" && target == "stable":
		res, err = CheckReviewStable(repoRoot, targetName)
	case targetKind == "unit" && command == "review":
		res, err = CheckReview(repoRoot, targetName)
	case targetKind == "rule" && command == "validate" && target == "stable":
		res, err = CheckRuleValidateStable(repoRoot, targetName)
	case targetKind == "rule" && command == "validate":
		res, err = CheckRuleValidate(repoRoot, targetName)
	default:
		return CacheResult{}, fmt.Errorf("unsupported gate %q for %s target %q", command, targetKind, targetName)
	}
	if err != nil {
		return CacheResult{}, err
	}
	return CacheResult{Fresh: res.Fresh, Category: res.Category, Reason: res.Reason}, nil
}

// CheckRenderedCache runs the same parsed-cache checks used by fresh and
// promote, but against an in-memory candidate that has not been published.
func CheckRenderedCache(repoRoot, targetKind, targetName, command, target string, content []byte) (CacheResult, error) {
	cache, err := parseCache(content)
	if err != nil {
		return CacheResult{}, err
	}
	var res CheckResult
	switch {
	case targetKind == "unit" && command == "validate" && target == "stable":
		res, err = checkCacheObject(repoRoot, "unit", targetName, "validate", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", targetName), cache)
	case targetKind == "unit" && command == "validate":
		res, err = checkCacheObject(repoRoot, "unit", targetName, "validate", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", targetName), cache)
	case targetKind == "unit" && command == "verify" && target == "stable":
		res, err = checkCacheObject(repoRoot, "unit", targetName, "verify", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", targetName), cache)
	case targetKind == "unit" && command == "verify":
		res, err = checkCacheObject(repoRoot, "unit", targetName, "verify", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", targetName), cache)
	case targetKind == "unit" && command == "review" && target == "stable":
		res, err = checkReviewCache(repoRoot, targetName, "stable", cache)
	case targetKind == "unit" && command == "review":
		res, err = checkReviewCache(repoRoot, targetName, "", cache)
	case targetKind == "rule" && command == "validate" && target == "stable":
		res, err = checkCacheObject(repoRoot, "rule", targetName, "validate", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/rules/stable/%s.md", targetName), cache)
	case targetKind == "rule" && command == "validate":
		res, err = checkCacheObject(repoRoot, "rule", targetName, "validate", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/rules/candidate/%s.md", targetName), cache)
	default:
		return CacheResult{}, fmt.Errorf("unsupported gate %q for %s target %q", command, targetKind, targetName)
	}
	if err != nil {
		return CacheResult{}, err
	}
	return CacheResult{Fresh: res.Fresh, Category: res.Category, Reason: res.Reason}, nil
}

// renderCacheFrontmatter renders the YAML frontmatter of a cache file
// (delimiter lines included). The files block is rendered in the canonical
// order: file entries in declaration order, hash, per-check checks block,
// file-level deps union. The body is appended by WriteCache after the closing
// delimiter.
func renderCacheFrontmatter(w CacheWrite, targetName string) (string, error) {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "command: %s\n", w.Command)
	fmt.Fprintf(&b, "unit: %s\n", targetName)
	fmt.Fprintf(&b, "mode: %s\n", w.Mode)
	if w.Basis != "" {
		fmt.Fprintf(&b, "basis: %s\n", w.Basis)
	}
	fmt.Fprintf(&b, "result: %s\n", w.Result)
	if w.Target != "" {
		fmt.Fprintf(&b, "target: %s\n", w.Target)
	}
	fmt.Fprintf(&b, "blocking: %t\n", w.Blocking)
	fmt.Fprintf(&b, "p0_count: %d\n", w.P0Count)
	fmt.Fprintf(&b, "p1_count: %d\n", w.P1Count)
	fmt.Fprintf(&b, "p2_count: %d\n", w.P2Count)
	fmt.Fprintf(&b, "p3_count: %d\n", w.P3Count)
	fmt.Fprintf(&b, "timestamp: %q\n", w.Timestamp)
	if w.GateRun != "" {
		fmt.Fprintf(&b, "gate_run: %s\n", w.GateRun)
	}
	if len(w.InvalidatedChecks) > 0 {
		b.WriteString("invalidated_checks:\n")
		for _, key := range w.InvalidatedChecks {
			fmt.Fprintf(&b, "  - %q\n", key)
		}
	}
	if len(w.Entries) == 0 {
		b.WriteString("files: []\n")
		b.WriteString("---")
		return b.String(), nil
	}
	b.WriteString("files:\n")
	for _, e := range w.Entries {
		fmt.Fprintf(&b, "  - path: %s\n", e.Path)
		if e.Hash != "" {
			fmt.Fprintf(&b, "    hash: sha256:%s\n", normalizeHash(e.Hash))
		}
		if len(e.Checks) > 0 {
			b.WriteString("    checks:\n")
			for _, c := range e.Checks {
				fmt.Fprintf(&b, "      - check: %q\n", c.Check)
				if c.Status != "" {
					fmt.Fprintf(&b, "        status: %s\n", c.Status)
				}
				if len(c.Deps) > 0 {
					b.WriteString("        deps:\n")
					for _, d := range c.Deps {
						fmt.Fprintf(&b, "          - %s\n", d)
					}
				}
			}
		}
		if len(e.Deps) > 0 {
			b.WriteString("    deps:\n")
			for _, d := range e.Deps {
				fmt.Fprintf(&b, "      - %s\n", d)
			}
		}
	}
	b.WriteString("---")
	return b.String(), nil
}

// CacheSummary is a read-only summary of a cache file's frontmatter,
// used by freshness reporting. It does not perform any freshness check.
type CacheSummary struct {
	Command   string
	Unit      string
	Mode      string
	Basis     string
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
	path, err := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	if err != nil {
		return nil, err
	}
	cache, err := readCache(path)
	if err != nil {
		return nil, err
	}
	return &CacheSummary{
		Command:   cache.Command,
		Unit:      cache.Unit,
		Mode:      cache.Mode,
		Basis:     cache.Basis,
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

	cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil // already gone
	}
	return os.Remove(cachePath)
}

const (
	InvalidationNoCache       = "no_cache"
	InvalidationCacheDeleted  = "cache_deleted"
	InvalidationRecordUpdated = "failure_record_invalidated"
)

// TargetedInvalidation reports how a targeted P0/P1 changed the canonical
// cache. The caller owns cross-module coordination such as invalidating an
// already-open gate run; this function owns only the cache transition.
type TargetedInvalidation struct {
	Action string
	Path   string
	Added  []string
}

// InvalidateGateCache records a targeted P0/P1 against one gate identity.
// A pass cache is deleted. A failure record is preserved byte-for-byte except
// for its sorted, duplicate-free invalidated_checks frontmatter list. A
// missing cache is safe: there is no complete result that could satisfy the
// gate, and a future full run is already required.
//
// The caller must hold the repository gate-run mutation lock so this cache
// transition is serialized with planning and finalization.
func InvalidateGateCache(repoRoot, targetKind, targetName, command, target string, checkKeys []string) (*TargetedInvalidation, error) {
	fileName, err := cacheFileName(command)
	if err != nil {
		return nil, err
	}
	cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cachePath)
	if os.IsNotExist(err) {
		return &TargetedInvalidation{Action: InvalidationNoCache, Path: relPath(repoRoot, cachePath)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read gate cache for targeted invalidation: %w", err)
	}
	cache, err := parseCache(data)
	if err != nil {
		return nil, fmt.Errorf("read gate cache for targeted invalidation: %w", err)
	}
	if cache.Command != command || cache.Unit != targetName {
		return nil, fmt.Errorf("cache identity is %s@%s, expected %s@%s", cache.Command, cache.Unit, command, targetName)
	}
	cacheTarget := cache.Target
	if cacheTarget == "" {
		cacheTarget = "candidate"
	}
	if cacheTarget != target {
		return nil, fmt.Errorf("cache target is %q, expected %q", cacheTarget, target)
	}

	keys, err := normalizedInvalidationKeys(checkKeys)
	if err != nil {
		return nil, err
	}
	if cache.Result == "pass" && !cache.Blocking {
		if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("delete pass cache after targeted P0/P1: %w", err)
		}
		return &TargetedInvalidation{Action: InvalidationCacheDeleted, Path: relPath(repoRoot, cachePath)}, nil
	}
	if cache.Result != "fail" || !cache.blockingSeen || !cache.Blocking {
		return nil, fmt.Errorf("cache declarations cannot be invalidated safely (result: %q, blocking: %t)", cache.Result, cache.Blocking)
	}

	existing := make(map[string]bool, len(cache.InvalidatedChecks))
	for _, key := range cache.InvalidatedChecks {
		existing[key] = true
	}
	all := append([]string(nil), cache.InvalidatedChecks...)
	var added []string
	for _, key := range keys {
		if existing[key] {
			continue
		}
		existing[key] = true
		all = append(all, key)
		added = append(added, key)
	}
	sort.Strings(all)
	rewritten, err := rewriteInvalidatedChecks(string(data), all)
	if err != nil {
		return nil, err
	}
	if _, err := PublishCache(repoRoot, targetKind, targetName, command, []byte(rewritten)); err != nil {
		return nil, err
	}
	return &TargetedInvalidation{
		Action: InvalidationRecordUpdated,
		Path:   relPath(repoRoot, cachePath),
		Added:  added,
	}, nil
}

func normalizedInvalidationKeys(checkKeys []string) ([]string, error) {
	seen := map[string]bool{}
	var keys []string
	for _, raw := range checkKeys {
		key := strings.TrimSpace(raw)
		if key == "" {
			return nil, fmt.Errorf("targeted invalidation check key must not be empty")
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("at least one targeted invalidation check key is required")
	}
	sort.Strings(keys)
	return keys, nil
}

func rewriteInvalidatedChecks(content string, keys []string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", fmt.Errorf("cannot rewrite targeted invalidations: missing leading frontmatter delimiter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", fmt.Errorf("cannot rewrite targeted invalidations: missing closing frontmatter delimiter")
	}

	block := make([]string, 0, len(keys)+1)
	block = append(block, "invalidated_checks:")
	for _, key := range keys {
		block = append(block, fmt.Sprintf("  - %q", key))
	}
	out := make([]string, 0, len(lines)+len(block))
	out = append(out, lines[0])
	inserted := false
	for i := 1; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "invalidated_checks:" || trimmed == "invalidated_checks: []" {
			for i+1 < end && strings.HasPrefix(lines[i+1], "  - ") {
				i++
			}
			continue
		}
		if trimmed == "files:" && !inserted {
			out = append(out, block...)
			inserted = true
		}
		out = append(out, lines[i])
	}
	if !inserted {
		out = append(out, block...)
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}

// StaleScope is the mechanism-derived delta scope for one cache file:
// which declared dependencies went stale and which check keys declared
// them. Unclaimed lists the file entries whose stale dependencies no check
// declared — the planner maps them to checks by the command's fixed
// associations where one exists and degrades conservatively to the full
// packet set where none does (see framework/verification_scope.md
// §Delta Runs). Unreadable lists the file entries that could not be
// resolved or read during derivation — the promote gate reports those;
// delta derivation works only on files that exist.
type StaleScope struct {
	StaleDeps   []string // declared dependency CIDs that no longer hold (deduplicated, declaration order)
	Affected    []string // check keys with at least one stale dependency (deduplicated, declaration order)
	Unclaimed   []string // file entries with stale deps no check declared (deduplicated, declaration order)
	Unreadable  []string // file entries that could not be resolved or read (deduplicated, declaration order)
	Untrackable []string // file entries with no dependency chunks over a file with content — the freshness chain can never see them fresh (deduplicated, declaration order)
	HasChecks   bool     // the cache carries per-check declarations (false → no check association exists)
}

// checksUnionSubsetOfDeps verifies that every per-check dependency CID in a
// files entry is also declared in the entry's file-level deps union. A
// check-level dep missing from the union means the promote gate would judge
// freshness on a narrower basis than the check declared — content the check
// depends on could change without staling the cache (false fresh). Extra
// file-level deps beyond the check union are legal (declare-heavy
// conservatism). Entries without per-check declarations trivially pass.
func checksUnionSubsetOfDeps(entry cacheFileEntry) (bool, string) {
	if len(entry.Checks) == 0 {
		return true, ""
	}
	depsSet := make(map[string]bool, len(entry.Deps))
	for _, d := range entry.Deps {
		depsSet[d] = true
	}
	var missing []string
	for _, c := range entry.Checks {
		for _, d := range c.Deps {
			if !depsSet[d] {
				missing = append(missing, fmt.Sprintf("%s (check %s)", d, c.Check))
			}
		}
	}
	if len(missing) > 0 {
		return false, strings.Join(missing, ", ")
	}
	return true, ""
}

// DeriveStaleScope reads the cache file for the given target and command and
// derives the mechanism-level delta scope: the declared dependencies that
// no longer hold for the current file contents and the check keys that
// declared them (see framework/verification_scope.md §Delta Runs). Files
// whose path resolves to nothing (unresolved logical references) and files
// that cannot be read are reported in Unreadable — the promote gate reports
// those; delta derivation works only on files that exist. A cache without
// per-check declarations (HasChecks false) leaves Affected empty and reports
// its stale deps as unclaimed: no check association exists, so the planner
// degrades to the full packet set. Entries whose stale dependencies no check
// declared are reported in Unclaimed — the planner maps them by the
// command's fixed associations (logical references) where one exists and
// degrades conservatively where none does; they are never silently carried
// over.
func DeriveStaleScope(repoRoot, targetKind, targetName, command string) (*StaleScope, error) {
	cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, command+"_result.md")
	if err != nil {
		return nil, err
	}
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, err
	}
	scope := &StaleScope{}
	affectedSeen := make(map[string]bool)
	staleSeen := make(map[string]bool)
	unclaimedSeen := make(map[string]bool)
	unreadableSeen := make(map[string]bool)
	untrackableSeen := make(map[string]bool)
	for _, entry := range cache.Files {
		if ok, why := checksUnionSubsetOfDeps(entry); !ok {
			return nil, fmt.Errorf("cache format error in %s: per-check deps missing from the file-level deps union: %s", entry.Path, why)
		}
		fullPath := resolveEntryPath(repoRoot, entry.Path)
		if fullPath == "" {
			if !unreadableSeen[entry.Path] {
				unreadableSeen[entry.Path] = true
				scope.Unreadable = append(scope.Unreadable, entry.Path+" (unresolved)")
			}
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if !unreadableSeen[entry.Path] {
				unreadableSeen[entry.Path] = true
				scope.Unreadable = append(scope.Unreadable, fmt.Sprintf("%s (unreadable: %v)", entry.Path, err))
			}
			continue
		}
		text := specpaths.NormalizeText(string(data))
		if len(entry.Deps) == 0 && len(contenthash.ChunkText(text).Chunks) > 0 {
			// No dependency chunks over a file with content: no check can
			// attribute this entry's staleness and the gate's file-level
			// freshness chain can never see it fresh. Report it so the plan
			// degrades conservatively instead of carrying an untrackable
			// entry (see framework/verification_scope.md §Delta Runs →
			// Incremental scope).
			if !untrackableSeen[entry.Path] {
				untrackableSeen[entry.Path] = true
				scope.Untrackable = append(scope.Untrackable, entry.Path)
			}
		}
		if len(entry.Checks) == 0 {
			missing := contenthash.ListMissingDeps(text, entry.Deps)
			for _, m := range missing {
				if !staleSeen[m] {
					staleSeen[m] = true
					scope.StaleDeps = append(scope.StaleDeps, m)
				}
			}
			if len(missing) > 0 && !unclaimedSeen[entry.Path] {
				unclaimedSeen[entry.Path] = true
				scope.Unclaimed = append(scope.Unclaimed, entry.Path)
			}
			continue
		}
		scope.HasChecks = true
		claimed := make(map[string]bool)
		for _, c := range entry.Checks {
			missing := contenthash.ListMissingDeps(text, c.Deps)
			for _, m := range missing {
				if !staleSeen[m] {
					staleSeen[m] = true
					scope.StaleDeps = append(scope.StaleDeps, m)
				}
			}
			if len(missing) > 0 && !affectedSeen[c.Check] {
				affectedSeen[c.Check] = true
				scope.Affected = append(scope.Affected, c.Check)
			}
			for _, d := range c.Deps {
				claimed[d] = true
			}
		}
		// File-level stale deps no check declared (whole-file declarations,
		// declare-heavy extras) have no check association — report the entry
		// as unclaimed instead of silently ignoring the staleness.
		unclaimedMissing := 0
		for _, m := range contenthash.ListMissingDeps(text, entry.Deps) {
			if claimed[m] {
				continue
			}
			unclaimedMissing++
			if !staleSeen[m] {
				staleSeen[m] = true
				scope.StaleDeps = append(scope.StaleDeps, m)
			}
		}
		if unclaimedMissing > 0 && !unclaimedSeen[entry.Path] {
			unclaimedSeen[entry.Path] = true
			scope.Unclaimed = append(scope.Unclaimed, entry.Path)
		}
	}
	return scope, nil
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
	fileMissing    fileFreshnessState = iota // file is gone from disk
	fileNoDeps                               // entry declares no dependency chunks but the file has content
	fileDepChanged                           // a declared dependency chunk CID is no longer present
	fileFresh                                // every declared dependency chunk is unchanged
)

// fileFreshness re-chunks the current file and checks every declared
// dependency chunk CID against it. Freshness is judged on the dependency
// chunks only — content changes outside the declared dependencies do not
// stale the cache but are reported as changed (informational). The
// whole-file hash recorded in the cache powers that informational
// comparison.
func fileFreshness(repoRoot string, entry cacheFileEntry) (state fileFreshnessState, changed bool, err error) {
	fullPath := resolveEntryPath(repoRoot, entry.Path)
	if fullPath == "" {
		return fileMissing, false, nil
	}
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

	if !contenthash.DepsPresent(text, entry.Deps) {
		return fileDepChanged, changed, nil
	}
	return fileFresh, changed, nil
}

func checkCache(repoRoot, targetKind, targetName, command, fileName string, validResults []string, requiredMainFile string) (CheckResult, error) {
	cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	if err != nil {
		return CheckResult{}, err
	}

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
	return checkCacheObject(repoRoot, targetKind, targetName, command, validResults, requiredMainFile, cache)
}

func checkCacheObject(repoRoot, targetKind, targetName, command string, validResults []string, requiredMainFile string, cache *cacheFile) (CheckResult, error) {
	// Validate command matches
	if cache.Command != command {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cache command is %q, expected %q", cache.Command, command),
		}, nil
	}

	// Validate result is acceptable. A fail result is a delta re-run's
	// failure record (validate/verify since the failure-recovery design);
	// its blocking status is decided after the dependency check below,
	// matching the review gate's stale-over-blocking precedence.
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
		if ok, why := checksUnionSubsetOfDeps(entry); !ok {
			mismatchedFiles = append(mismatchedFiles, fmt.Sprintf("%s (per-check deps missing from the file-level deps union: %s)", entry.Path, why))
			continue
		}
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

	// Blocking check — fail-capable caches (validate/verify failure records
	// written by delta re-runs or a candidate verify full-run FAIL, and pass
	// caches that declare a blocking field) are validated by the same chain
	// review uses. A blocking cache
	// is CategoryBlocked: promote rejects it and fresh reports BLOCKED. The
	// dependency check above takes precedence — a stale failure record is
	// STALE, not BLOCKED, matching the review gate.
	if cache.blockingSeen || cache.Result == "fail" {
		if res := blockingCheck(command, cache); res != nil {
			return *res, nil
		}
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
	return parseCache(data)
}

// unquoteScalar canonicalizes one frontmatter scalar: surrounding quotation
// marks are removed, then surrounding whitespace (including whitespace the
// quotes had contained) is trimmed. Every scalar the parser records goes
// through this one helper, so downstream comparisons and joins always see one
// value form — a quoted-with-whitespace spelling like `status: " fail "` can
// never be treated as a valid value by one consumer and an ignored value by
// another (see framework/verification_scope.md §Delta Runs → Failure
// recovery).
func unquoteScalar(value string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "\"'"))
}

func parseCache(data []byte) (*cacheFile, error) {
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
	var currentCheck *checkEntry
	inFilesBlock := false
	inFilesDepsBlock := false
	inChecksBlock := false
	inCheckDepsBlock := false
	inInvalidatedChecks := false
	invalidatedChecksSeen := false

	for _, line := range fmLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if inInvalidatedChecks {
			if strings.HasPrefix(line, "  - ") {
				key := unquoteScalar(strings.TrimPrefix(line, "  - "))
				if key == "" {
					return nil, fmt.Errorf("cache file has an empty `invalidated_checks` entry")
				}
				cache.InvalidatedChecks = append(cache.InvalidatedChecks, key)
				continue
			}
			inInvalidatedChecks = false
		}
		if trimmed == "invalidated_checks:" || trimmed == "invalidated_checks: []" {
			if invalidatedChecksSeen {
				return nil, fmt.Errorf("cache file declares `invalidated_checks` more than once")
			}
			invalidatedChecksSeen = true
			cache.invalidatedSeen = true
			inInvalidatedChecks = trimmed == "invalidated_checks:"
			continue
		}

		// Detect files block entries
		if trimmed == "files:" {
			inFilesBlock = true
			inFilesDepsBlock = false
			inChecksBlock = false
			inCheckDepsBlock = false
			continue
		}

		if inFilesBlock {
			if strings.HasPrefix(trimmed, "- path:") {
				// New entry
				inFilesDepsBlock = false
				inChecksBlock = false
				inCheckDepsBlock = false
				currentCheck = nil
				path := unquoteScalar(strings.TrimPrefix(trimmed, "- path:"))
				currentEntry = &cacheFileEntry{Path: path}
				cache.Files = append(cache.Files, *currentEntry)
				continue
			}
			if inChecksBlock && strings.HasPrefix(trimmed, "- check:") {
				// New check entry inside a checks block
				inCheckDepsBlock = false
				check := unquoteScalar(strings.TrimPrefix(trimmed, "- check:"))
				currentCheck = &checkEntry{Check: check}
				if currentEntry != nil {
					currentEntry.Checks = append(currentEntry.Checks, *currentCheck)
					cache.Files[len(cache.Files)-1] = *currentEntry
				}
				continue
			}
			if inCheckDepsBlock {
				if strings.HasPrefix(trimmed, "-") {
					dep := unquoteScalar(strings.TrimPrefix(trimmed, "-"))
					if currentCheck != nil && currentEntry != nil {
						currentCheck.Deps = append(currentCheck.Deps, dep)
						currentEntry.Checks[len(currentEntry.Checks)-1] = *currentCheck
						cache.Files[len(cache.Files)-1] = *currentEntry
					}
					continue
				}
				// A non-list line ends the check deps block
				inCheckDepsBlock = false
			}
			// A check entry's status line (pass | fail | carried) may appear
			// before or after its deps block — the non-list line above already
			// ended the deps block when the status follows it.
			if inChecksBlock && strings.HasPrefix(trimmed, "status:") && currentCheck != nil && currentEntry != nil {
				status := unquoteScalar(strings.TrimPrefix(trimmed, "status:"))
				currentCheck.Status = status
				currentEntry.Checks[len(currentEntry.Checks)-1] = *currentCheck
				cache.Files[len(cache.Files)-1] = *currentEntry
				continue
			}
			if inFilesDepsBlock {
				if strings.HasPrefix(trimmed, "-") {
					dep := unquoteScalar(strings.TrimPrefix(trimmed, "-"))
					if currentEntry != nil {
						currentEntry.Deps = append(currentEntry.Deps, dep)
						cache.Files[len(cache.Files)-1] = *currentEntry
					}
					continue
				}
				// A non-list line ends the files deps block
				inFilesDepsBlock = false
			}
			if inChecksBlock && trimmed == "deps:" && currentCheck != nil && indentLevel(line) >= 8 {
				inCheckDepsBlock = true
				continue
			}
			if trimmed == "deps:" {
				inFilesDepsBlock = true
				continue
			}
			if trimmed == "checks:" {
				inChecksBlock = true
				currentCheck = nil
				continue
			}
			if strings.HasPrefix(trimmed, "hash:") && currentEntry != nil {
				hash := unquoteScalar(strings.TrimPrefix(trimmed, "hash:"))
				currentEntry.Hash = hash
				cache.Files[len(cache.Files)-1] = *currentEntry
				continue
			}
			// If we hit a non-empty line that doesn't start with - or is a continuation,
			// we might have left the files block
			if currentEntry != nil {
				inFilesBlock = false
				inFilesDepsBlock = false
				inChecksBlock = false
				inCheckDepsBlock = false
			}
		}

		if !inFilesBlock {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := unquoteScalar(parts[1])
				switch key {
				case "command":
					cache.Command = value
				case "unit":
					cache.Unit = value
				case "mode":
					cache.Mode = value
				case "basis":
					cache.Basis = value
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
				case "gate_run":
					cache.GateRun = value
				}
			}
		}
	}

	if cache.Command == "" || cache.Result == "" {
		return nil, fmt.Errorf("cache file missing required frontmatter fields (command, result)")
	}
	if cache.invalidatedSeen {
		if cache.Result != "fail" || !cache.blockingSeen || !cache.Blocking {
			return nil, fmt.Errorf("`invalidated_checks` is valid only on a failure record (result: fail, blocking: true)")
		}
		for i, key := range cache.InvalidatedChecks {
			if strings.TrimSpace(key) == "" {
				return nil, fmt.Errorf("cache file has an empty `invalidated_checks` entry")
			}
			if i > 0 && cache.InvalidatedChecks[i-1] >= key {
				return nil, fmt.Errorf("cache file `invalidated_checks` must be sorted and duplicate-free")
			}
		}
	}

	return cache, nil
}

// InheritEntry reports one gate's fork-inheritance outcome.
type InheritEntry struct {
	Command   string // validate | verify | review
	Inherited bool
	Reason    string // why the cache was not inherited ("" when inherited)
}

// InheritReport summarizes the fork-inheritance result for one unit.
type InheritReport struct {
	Unit    string
	Entries []InheritEntry
}

// InheritStableCaches converts the unit's stable confirmation caches
// (target: stable) into candidate caches for a forked round. Fork copies the
// stable spec and appendices verbatim (only the version bumps), so the
// confirmation conclusions carry over: a gate cache with `result: pass`
// (review additionally `blocking: false`) is rewritten — `target: stable` →
// `target: candidate` and physical paths under `docs/specs/units/stable/` →
// `docs/specs/units/candidate/` — and stays valid for the candidate round
// until its evidence goes stale (the version bump stales the frontmatter
// declarations; delta re-runs restore the affected gates). Caches that
// cannot be inherited (missing, non-pass, blocking review) are skipped with
// a reason — the forked round starts those gates from scratch. Rule forks do
// not inherit: a rule's cache declares the rule file whole, so the fork's
// version bump stales it into a full re-run anyway.
func InheritStableCaches(repoRoot, unitName string) (*InheritReport, error) {
	report := &InheritReport{Unit: unitName}
	for _, cmd := range []string{"validate", "verify", "review"} {
		entry := InheritEntry{Command: cmd}
		cachePath, err := cacheFilePath(repoRoot, "unit", unitName, cmd+"_result.md")
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(cachePath)
		if os.IsNotExist(err) {
			entry.Reason = "no confirmation cache to inherit"
			report.Entries = append(report.Entries, entry)
			continue
		}
		if err != nil {
			return nil, err
		}
		cache, err := readCache(cachePath)
		if err != nil {
			entry.Reason = fmt.Sprintf("confirmation cache unreadable: %v", err)
			report.Entries = append(report.Entries, entry)
			continue
		}
		switch {
		case cache.Target != "stable":
			entry.Reason = fmt.Sprintf("cache target is %q, expected 'stable' — not a stable confirmation cache", cache.Target)
		case cache.Result != "pass":
			entry.Reason = fmt.Sprintf("confirmation cache result is %q, expected 'pass'", cache.Result)
		case cmd == "review" && cache.Blocking:
			entry.Reason = "confirmation cache is blocking (P0/P1 findings)"
		default:
			rewritten, changed := rewriteCacheLayer(string(data))
			if !changed {
				entry.Reason = "confirmation cache needs no layer rewrite"
			} else if err := os.WriteFile(cachePath, []byte(rewritten), 0644); err != nil {
				return nil, err
			} else {
				entry.Inherited = true
			}
		}
		report.Entries = append(report.Entries, entry)
	}
	return report, nil
}

// PromoteCachesToStableEntry reports one gate's promote-rewrite outcome.
type PromoteCachesToStableEntry struct {
	Rewritten bool
	Reason    string // why the cache was not rewritten ("" when rewritten)
}

// PromoteCachesToStableReport summarizes the promote-rewrite result.
type PromoteCachesToStableReport struct {
	Entries []PromoteCachesToStableEntry
}

// RewriteCachesToStable rewrites the candidate-layer gate caches for the given
// target into stable confirmation caches (`target: candidate` → `target: stable`,
// and every physical path under `docs/specs/.../candidate/` →
// `docs/specs/.../stable/`). This is the inverse of InheritStableCaches: promote
// consumes the candidate caches and produces an equivalent stable-layer record
// so the promoted state is visible in fresh@stable and serves as the delta-
// recovery baseline for future re* runs. Caches without a usable baseline
// (missing, failure record, already stable) are skipped with a reason.
//
// Rule forks do not inherit, so the rewrite is primarily meaningful for unit
// caches (validate/verify/review). For rules, only the validate cache is
// rewritten — the fresh@stable rule report consumes it as the consumer/
// consistency confirmation state.
func RewriteCachesToStable(repoRoot, targetKind, targetName string) (*PromoteCachesToStableReport, error) {
	var commands []string
	switch targetKind {
	case "unit":
		commands = []string{"validate", "verify", "review"}
	case "rule":
		commands = []string{"validate"}
	default:
		return nil, fmt.Errorf("unknown target kind %q — expected 'unit' or 'rule'", targetKind)
	}
	report := &PromoteCachesToStableReport{}
	for _, cmd := range commands {
		entry := PromoteCachesToStableEntry{}
		cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, cmd+"_result.md")
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(cachePath)
		if os.IsNotExist(err) {
			entry.Reason = fmt.Sprintf("%s cache not found — nothing to rewrite", cmd)
			report.Entries = append(report.Entries, entry)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s cache: %w", cmd, err)
		}
		cache, err := readCache(cachePath)
		if err != nil {
			entry.Reason = fmt.Sprintf("%s cache unreadable: %v — not rewritten", cmd, err)
			report.Entries = append(report.Entries, entry)
			continue
		}
		switch {
		case cache.Target == "stable":
			entry.Reason = fmt.Sprintf("%s cache already has target: stable — no rewrite needed", cmd)
		case cache.Result == "fail" || cache.Blocking:
			entry.Reason = fmt.Sprintf("%s cache is a failure record — not rewritten (blocking state must be resolved first)", cmd)
		default:
			rewritten, changed := rewriteCacheLayerToStable(string(data))
			if !changed {
				entry.Reason = fmt.Sprintf("%s cache needs no layer rewrite", cmd)
			} else if err := os.WriteFile(cachePath, []byte(rewritten), 0644); err != nil {
				return nil, fmt.Errorf("rewrite %s cache: %w", cmd, err)
			} else {
				entry.Rewritten = true
			}
		}
		report.Entries = append(report.Entries, entry)
	}
	return report, nil
}

// rewriteLayerFrontmatter transforms a cache file's frontmatter by replacing
// the `target` value and every physical `path` prefix with the given layer
// strings. `fromLayer` is the current layer identifier (e.g. "stable" for a
// stable confirmation cache, "candidate" for a candidate gate cache);
// `toLayer` is the target layer. Physical paths under
// `docs/specs/units/{fromLayer}/` and `docs/specs/rules/{fromLayer}/` are
// rewritten to `docs/specs/units/{toLayer}/` and `docs/specs/rules/{toLayer}/`.
// Logical references (`unit:` / `rule:`) are untouched — they resolve by name
// to the current layer. Only the frontmatter (between the two `---` delimiters)
// is edited; the body is preserved verbatim. Returns whether anything changed.
func rewriteLayerFrontmatter(content string, fromLayer, toLayer string) (string, bool) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return content, false
	}
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return content, false
	}
	changed := false
	for i := 1; i < endIdx; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "target:") &&
			strings.TrimSpace(strings.TrimPrefix(trimmed, "target:")) == fromLayer:
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = leading + "target: " + toLayer
			changed = true
		case strings.HasPrefix(trimmed, "- path: docs/specs/units/"+fromLayer+"/"):
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			newPath := "docs/specs/units/" + toLayer + "/" + strings.TrimPrefix(
				strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")),
				"docs/specs/units/"+fromLayer+"/")
			lines[i] = leading + "- path: " + newPath
			changed = true
		case strings.HasPrefix(trimmed, "- path: docs/specs/rules/"+fromLayer+"/"):
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			newPath := "docs/specs/rules/" + toLayer + "/" + strings.TrimPrefix(
				strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")),
				"docs/specs/rules/"+fromLayer+"/")
			lines[i] = leading + "- path: " + newPath
			changed = true
		}
	}
	return strings.Join(lines, "\n"), changed
}

// rewriteCacheLayer rewrites a stable confirmation cache into its candidate
// form: `target: stable` → `target: candidate`, and every physical path under
// `docs/specs/units/stable/` / `docs/specs/rules/stable/` in the files list
// becomes `docs/specs/units/candidate/` / `docs/specs/rules/candidate/`.
func rewriteCacheLayer(content string) (string, bool) {
	return rewriteLayerFrontmatter(content, "stable", "candidate")
}

// rewriteCacheLayerToStable rewrites a candidate gate cache into its stable
// confirmation form: `target: candidate` → `target: stable`, and every physical
// path under `docs/specs/units/candidate/` / `docs/specs/rules/candidate/` in
// the files list becomes `docs/specs/units/stable/` / `docs/specs/rules/stable/`.
func rewriteCacheLayerToStable(content string) (string, bool) {
	return rewriteLayerFrontmatter(content, "candidate", "stable")
}

// fileHash computes the SHA-256 hash of a file's normalized content.
// Delegates to specpaths.FileHash for the canonical normalization.
func fileHash(path string) (string, error) {
	return specpaths.FileHash(path)
}

// cacheFilePath builds the absolute path to a cache file.
// targetKind is "unit" or "rule". Target names are external input, so they are
// validated before path construction and the joined path is asserted to stay
// inside the kind's validation directory: no name (or file name) can traverse
// into another location (see tooling/README.md §Governed write zones and
// framework/validation_cache.md §Format).
func cacheFilePath(repoRoot, targetKind, targetName, fileName string) (string, error) {
	if err := specpaths.ValidateTargetName(targetKind, targetName); err != nil {
		return "", err
	}
	root := filepath.Join(repoRoot, "docs/specs/meta/validation", targetKind)
	cachePath := filepath.Join(root, targetName, fileName)
	rel, err := filepath.Rel(root, cachePath)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cache path for %s %q escapes %s", targetKind, targetName, filepath.Join("docs/specs/meta/validation", targetKind))
	}
	return cachePath, nil
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

// indentLevel returns the leading whitespace count of a frontmatter line.
// It disambiguates the `deps:` block of a checks entry (8 spaces, per the
// cache format) from the file-level `deps:` block (4 spaces).
func indentLevel(line string) int {
	n := 0
	for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return n
}

func resolvePath(repoRoot, filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(repoRoot, filepath.FromSlash(filePath))
}

// resolveEntryPath resolves a cache file entry to an absolute file path.
// Logical unit/appendix and bound-rule references resolve to the current-layer
// file (candidate first, stable fallback), so promotion does not stale caches
// whose dependency content is unchanged. Logical global-rule references
// resolve stable-only because candidate globals are unpublished. Physical
// paths resolve directly. Returns "" when a logical reference resolves to no
// applicable file.
func resolveEntryPath(repoRoot, path string) string {
	if strings.HasPrefix(path, "unit:") {
		rest := strings.TrimPrefix(path, "unit:")
		if _, appendix, found := strings.Cut(rest, ":appendix:"); found {
			// The unit name is contextual: the appendix is resolved by its
			// full file base name (unit_{unit}_{name}), which is unique
			// across units by the naming convention.
			return specpaths.ResolveUnitAppendix(repoRoot, appendix)
		}
		return specpaths.ResolveUnitFile(repoRoot, rest)
	}
	if strings.HasPrefix(path, "rule:") {
		return specpaths.ResolveRuleFile(repoRoot, strings.TrimPrefix(path, "rule:"))
	}
	return resolvePath(repoRoot, path)
}

// ValidateEntryPathForm rejects a cache entry declaration that names a
// name-resolved spec object (framework/validation_cache.md §Logical
// References) as a physical path. Such an entry must be a logical reference
// so freshness resolves it to the current layer and a promote of the
// referenced target (candidate file deleted, content unchanged) does not
// fail the cache closed. Physical paths stay correct for the run's own
// target files — the target unit's main spec and appendices, or the target
// rule's candidate file and stable sibling — and for code files.
func ValidateEntryPathForm(targetKind, targetName, entryPath string) error {
	clean := path.Clean(filepath.ToSlash(strings.TrimSpace(entryPath)))
	if strings.HasPrefix(clean, "unit:") || strings.HasPrefix(clean, "rule:") {
		return nil
	}
	if targetKind == "rule" {
		if ref, ok := parseUnitSpecPath(clean); ok {
			return nameResolvedPhysicalPathError(entryPath, ref.logical())
		}
		return nil
	}
	if strings.HasPrefix(clean, specpaths.RuleModulesRootDir+"/") {
		id := strings.TrimSuffix(path.Base(clean), ".md")
		return fmt.Errorf("cache entry %q is a rule file — declare it as a logical reference (rule:%s) instead of a physical path (see framework/validation_cache.md §Logical References)", entryPath, id)
	}
	ref, ok := parseUnitSpecPath(clean)
	if !ok {
		return nil
	}
	if ref.ownedBy(targetName) {
		return nil
	}
	return nameResolvedPhysicalPathError(entryPath, ref.logical())
}

func nameResolvedPhysicalPathError(entryPath, logical string) error {
	return fmt.Errorf("cache entry %q is a spec object resolved by name — declare it as a logical reference (%s) instead of a physical path (see framework/validation_cache.md §Logical References)", entryPath, logical)
}

// unitSpecRef identifies a unit spec file parsed from a physical path: the
// unit name for a main spec, or the full appendix file base name (without
// .md) for an appendix file.
type unitSpecRef struct {
	unitName string
	appendix string
}

// logical returns the canonical logical reference spelling. The appendix
// form's unit-name part is contextual (the resolver keys on the appendix
// base name alone), so the placeholder names that part without guessing it.
func (r unitSpecRef) logical() string {
	if r.appendix != "" {
		return "unit:{name}:appendix:" + r.appendix
	}
	return "unit:" + r.unitName
}

// ownedBy reports whether the file belongs to the target unit's own spec
// union — its main spec or an appendix named unit_{target}_*.
func (r unitSpecRef) ownedBy(unitName string) bool {
	if r.appendix != "" {
		return strings.HasPrefix(r.appendix, "unit_"+unitName+"_")
	}
	return r.unitName == unitName
}

// parseUnitSpecPath parses a repo-relative slash path as a unit spec file
// under the candidate or stable layer. Main specs are unit_{name}.md at the
// layer root; appendix files are unit_{name}_{suffix}.md under appendix/.
func parseUnitSpecPath(clean string) (unitSpecRef, bool) {
	for _, dir := range []string{specpaths.CandidateDir, specpaths.StableDir} {
		rest, found := strings.CutPrefix(clean, dir+"/")
		if !found {
			continue
		}
		if base, found := strings.CutPrefix(rest, "appendix/"); found {
			if strings.Contains(base, "/") || !strings.HasPrefix(base, "unit_") || !strings.HasSuffix(base, ".md") {
				return unitSpecRef{}, false
			}
			return unitSpecRef{appendix: strings.TrimSuffix(base, ".md")}, true
		}
		if strings.Contains(rest, "/") || !strings.HasPrefix(rest, "unit_") || !strings.HasSuffix(rest, ".md") {
			return unitSpecRef{}, false
		}
		return unitSpecRef{unitName: strings.TrimPrefix(strings.TrimSuffix(rest, ".md"), "unit_")}, true
	}
	return unitSpecRef{}, false
}

// GateBaseline is the read-only view of a gate's existing cache, used by
// gate-plan to derive delta/repair packet sets and by gate-finalize to merge
// carried-over evidence. A missing cache is Exists=false.
type GateBaseline struct {
	Exists            bool
	Mode              string
	Basis             string
	Result            string
	Target            string
	Blocking          bool
	InvalidatedChecks []string        // persisted targeted P0/P1 contradictions
	HasChecks         bool            // the cache carries per-check declarations
	Checks            []BaselineCheck // declared checks (declaration order, first occurrence)
	Entries           []FileEntry     // full entries, for carried-over evidence merging
	Judgments         string          // machine-readable synthesis baseline JSON
}

// BaselineCheck is one declared check key and its recorded status. Status is
// "" on a pass cache (absent means pass); on a failure record it is
// pass | fail | carried.
type BaselineCheck struct {
	Check  string
	Status string
}

// ReadGateBaseline reads the gate's cache file and returns its baseline view.
// A missing cache returns Exists=false with no error; a malformed cache is an
// error (the caller fails closed).
func ReadGateBaseline(repoRoot, targetKind, targetName, command string) (*GateBaseline, error) {
	cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, command+"_result.md")
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return &GateBaseline{}, nil
	}
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, fmt.Errorf("read gate baseline %s: %w", cachePath, err)
	}
	baseline := &GateBaseline{
		Exists:            true,
		Mode:              cache.Mode,
		Basis:             cache.Basis,
		Result:            cache.Result,
		Target:            cache.Target,
		Blocking:          cache.Blocking,
		InvalidatedChecks: append([]string(nil), cache.InvalidatedChecks...),
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("read gate baseline judgments %s: %w", cachePath, err)
	}
	baseline.Judgments = extractJudgments(string(data))
	seen := map[string]bool{}
	for _, e := range cache.Files {
		entry := FileEntry{Path: e.Path, Hash: e.Hash, Deps: e.Deps}
		for _, c := range e.Checks {
			entry.Checks = append(entry.Checks, CheckEntry{Check: c.Check, Status: c.Status, Deps: c.Deps})
			baseline.HasChecks = true
			if !seen[c.Check] {
				seen[c.Check] = true
				baseline.Checks = append(baseline.Checks, BaselineCheck{Check: c.Check, Status: c.Status})
			}
		}
		baseline.Entries = append(baseline.Entries, entry)
	}
	return baseline, nil
}

func extractJudgments(content string) string {
	const begin = "<!-- GATE_JUDGMENTS_BEGIN\n"
	const end = "\nGATE_JUDGMENTS_END -->"
	start := strings.Index(content, begin)
	if start < 0 {
		return ""
	}
	start += len(begin)
	finish := strings.Index(content[start:], end)
	if finish < 0 {
		return ""
	}
	return strings.TrimSpace(content[start : start+finish])
}

// BuildEntryFromChecks computes a cache files entry from per-check
// declarations only (the packet-report model): every check declares its
// scope, and the file-level deps are the ordered union of the check deps.
// Unlike BuildEntry there is no entry-level declaration, so the whole-file
// fallback applies only to a check that itself declares no scope (the "all"
// spelling).
func BuildEntryFromChecks(repoRoot, entryPath string, checks []CheckDeclaration) (FileEntry, error) {
	if len(checks) == 0 {
		return FileEntry{}, fmt.Errorf("entry %q has no check declarations", entryPath)
	}
	abs := resolveEntryPath(repoRoot, entryPath)
	if abs == "" {
		return FileEntry{}, fmt.Errorf("cannot resolve cache entry %q to an applicable file", entryPath)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return FileEntry{}, fmt.Errorf("read %s: %w", entryPath, err)
	}
	text := specpaths.NormalizeText(string(data))

	entry := FileEntry{Path: entryPath}
	var ordered []string
	seen := map[string]bool{}
	for _, cd := range checks {
		check := strings.TrimSpace(cd.Check)
		if check == "" {
			return FileEntry{}, fmt.Errorf("entry %q declares a check with an empty key", entryPath)
		}
		hash, checkDeps, cerr := declarationDeps(text, entryPath, declFields{
			Sections:          cd.Sections,
			Ranges:            cd.Ranges,
			AcceptanceItems:   cd.AcceptanceItems,
			AcceptanceItemIDs: cd.AcceptanceItemIDs,
		})
		if cerr != nil {
			return FileEntry{}, cerr
		}
		entry.Hash = hash
		entry.Checks = append(entry.Checks, CheckEntry{Check: check, Status: cd.Status, Deps: checkDeps})
		for _, dep := range checkDeps {
			if !seen[dep] {
				seen[dep] = true
				ordered = append(ordered, dep)
			}
		}
	}
	entry.Deps = ordered
	return entry, nil
}

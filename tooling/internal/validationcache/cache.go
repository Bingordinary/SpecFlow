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
	Files        []cacheFileEntry
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
// id; review: the batch/file dimension) and the CIDs the check's judgment
// actually depended on. The file-level Deps list of the same entry is the
// union of all check deps plus any undeclared remainder — the promote gate
// judges freshness on that union; the per-check breakdown exists for delta
// scope derivation only. Status records the judgment outcome in a
// failure-record cache (a fail/blocking cache — delta FAIL records, review
// blocking caches, stable-only confirmation FAIL records): pass (judgment
// ran and passed), fail (judgment ran and found P0/P1), carried (not re-run —
// evidence unchanged from the pass baseline; delta records only). Status is
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
	cachePath := cacheFilePath(repoRoot, "unit", unitName, "verify_result.md")
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, err
	}
	deps := make(map[string][]string, len(cache.Files))
	for _, f := range cache.Files {
		p := resolveEntryPath(repoRoot, f.Path)
		if p == "" {
			continue // logical reference with no current-layer file — nothing to record
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
	path := cacheFilePath(repoRoot, targetKind, targetName, fileName)
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

	cachePath := cacheFilePath(repoRoot, targetKind, targetName, fileName)
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil // already gone
	}
	return os.Remove(cachePath)
}

// StaleScope is the mechanism-derived delta scope for one cache file:
// which declared dependencies went stale and which check keys declared
// them. Degrades reports that every declared check is affected — the
// delta re-run would be nearly a full re-run (for rule targets it
// reports that the rule file itself — a whole-file declaration — went
// stale, which stales every rule-body check). Unclaimed lists the file
// entries whose stale dependencies no check declared — the caller must
// map them to the affected checks by the command's logical-reference
// rules or by semantic derivation (see framework/verification_scope.md
// §Delta Runs). Unreadable lists the file entries that could not be
// resolved or read during derivation — the promote gate reports those;
// delta derivation works only on files that exist.
type StaleScope struct {
	StaleDeps  []string // declared dependency CIDs that no longer hold (deduplicated, declaration order)
	Affected   []string // check keys with at least one stale dependency (deduplicated, declaration order)
	Unclaimed  []string // file entries with stale deps no check declared (deduplicated, declaration order)
	Unreadable []string // file entries that could not be resolved or read (deduplicated, declaration order)
	HasChecks  bool     // the cache carries per-check declarations (false → file-level derivation only)
	Degrades   bool     // every declared check is affected — delta ≈ full re-run
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
// per-check declarations (HasChecks false) leaves Affected empty — the
// caller then derives the scope from the file-level stale sources instead.
// Entries whose stale dependencies no check declared are reported in
// Unclaimed — the caller maps them to checks by rule (logical references)
// or semantic derivation; they are never silently carried over. For rule
// targets, staleness of the rule file itself (a whole-file declaration)
// sets Degrades: any change to it stales every rule-body check.
func DeriveStaleScope(repoRoot, targetKind, targetName, command string) (*StaleScope, error) {
	cachePath := cacheFilePath(repoRoot, targetKind, targetName, command+"_result.md")
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, err
	}
	scope := &StaleScope{}
	var declared []string
	declaredSeen := make(map[string]bool)
	affectedSeen := make(map[string]bool)
	staleSeen := make(map[string]bool)
	unclaimedSeen := make(map[string]bool)
	unreadableSeen := make(map[string]bool)
	ruleFileStale := false
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
			if targetKind == "rule" && isRuleFileEntry(repoRoot, fullPath, targetName) && len(missing) > 0 {
				ruleFileStale = true
			}
			continue
		}
		scope.HasChecks = true
		claimed := make(map[string]bool)
		for _, c := range entry.Checks {
			if !declaredSeen[c.Check] {
				declaredSeen[c.Check] = true
				declared = append(declared, c.Check)
			}
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
	// Degradation: the affected set always includes the cross-check — its
	// delta re-run is unconditional (see framework/verification_scope.md
	// §Delta Runs) — so the re-run covers every declared check exactly when
	// every declared non-cross check is affected. The cross key is a
	// recording convention: whether it is declared or fresh does not change
	// the derived scope (see framework/validation_cache.md §Format). A cache
	// declaring only "cross" degrades too — the single re-run check covers
	// the whole declaration.
	if scope.HasChecks && len(declared) > 0 {
		nonCrossDeclared := 0
		nonCrossAffected := 0
		for _, k := range declared {
			if k != "cross" {
				nonCrossDeclared++
			}
		}
		for _, k := range scope.Affected {
			if k != "cross" {
				nonCrossAffected++
			}
		}
		if nonCrossDeclared == 0 || nonCrossAffected == nonCrossDeclared {
			scope.Degrades = true
		}
	}
	if targetKind == "rule" && ruleFileStale {
		scope.Degrades = true
	}
	return scope, nil
}

// isRuleFileEntry reports whether a resolved cache entry path is the rule
// file itself (candidate or stable layer). The rule file is a whole-file
// declaration in rule validate caches, so its staleness stales every
// rule-body check. The comparison runs on the resolved path (filepath.Clean),
// so the documented path spellings (`./` prefixes, absolute paths, platform
// separators) are all equivalent here, matching resolveEntryPath.
func isRuleFileEntry(repoRoot, fullPath, ruleID string) bool {
	clean := filepath.Clean(fullPath)
	return clean == filepath.Clean(resolvePath(repoRoot, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ruleID))) ||
		clean == filepath.Clean(resolvePath(repoRoot, fmt.Sprintf("docs/specs/rules/stable/%s.md", ruleID)))
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

	for _, line := range fmLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
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
				path := strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:"))
				path = strings.Trim(path, "\"'")
				currentEntry = &cacheFileEntry{Path: path}
				cache.Files = append(cache.Files, *currentEntry)
				continue
			}
			if inChecksBlock && strings.HasPrefix(trimmed, "- check:") {
				// New check entry inside a checks block
				inCheckDepsBlock = false
				check := strings.TrimSpace(strings.TrimPrefix(trimmed, "- check:"))
				check = strings.Trim(check, "\"'")
				currentCheck = &checkEntry{Check: check}
				if currentEntry != nil {
					currentEntry.Checks = append(currentEntry.Checks, *currentCheck)
					cache.Files[len(cache.Files)-1] = *currentEntry
				}
				continue
			}
			if inCheckDepsBlock {
				if strings.HasPrefix(trimmed, "-") {
					dep := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
					dep = strings.Trim(dep, "\"'")
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
				status := strings.TrimSpace(strings.TrimPrefix(trimmed, "status:"))
				status = strings.Trim(status, "\"'")
				currentCheck.Status = status
				currentEntry.Checks[len(currentEntry.Checks)-1] = *currentCheck
				cache.Files[len(cache.Files)-1] = *currentEntry
				continue
			}
			if inFilesDepsBlock {
				if strings.HasPrefix(trimmed, "-") {
					dep := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
					dep = strings.Trim(dep, "\"'")
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
				inFilesDepsBlock = false
				inChecksBlock = false
				inCheckDepsBlock = false
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
				}
			}
		}
	}

	if cache.Command == "" || cache.Result == "" {
		return nil, fmt.Errorf("cache file missing required frontmatter fields (command, result)")
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
		cachePath := cacheFilePath(repoRoot, "unit", unitName, cmd+"_result.md")
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

// rewriteCacheLayer rewrites a stable confirmation cache into its candidate
// form: the `target: stable` declaration becomes `target: candidate`, and
// every physical path under `docs/specs/units/stable/` in the files list
// becomes `docs/specs/units/candidate/` (the main spec and appendices move
// layer with the fork). Logical references (`unit:` / `rule:`) are untouched —
// they resolve by name to the current layer. Only the frontmatter is edited;
// the body is preserved verbatim. Returns whether anything changed.
func rewriteCacheLayer(content string) (string, bool) {
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
			strings.TrimSpace(strings.TrimPrefix(trimmed, "target:")) == "stable":
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = leading + "target: candidate"
			changed = true
		case strings.HasPrefix(trimmed, "- path: docs/specs/units/stable/"):
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			newPath := "docs/specs/units/candidate/" + strings.TrimPrefix(
				strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")),
				"docs/specs/units/stable/")
			lines[i] = leading + "- path: " + newPath
			changed = true
		}
	}
	return strings.Join(lines, "\n"), changed
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
// Logical references (`unit:<name>` / `unit:<name>:appendix:<file>` /
// `rule:<id>`) resolve to the current-layer file (candidate first, stable
// fallback), so a promote of the referenced unit or rule does not stale
// caches whose dependency content is unchanged. Physical paths resolve
// directly. Returns "" when a logical reference resolves to no file in any
// layer.
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

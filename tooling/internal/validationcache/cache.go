// Package validationcache assembles, writes, and checks validate and verify
// cache records. gate-finalize writes records from accepted gate-run
// artifacts; freshness and promote commands consume them mechanically.
//
// Cache files for units live under docs/specs/meta/validation/unit/{name}/.
// Cache files for rules live under docs/specs/meta/validation/rule/{id}/.
// They record:
//   - Which files were checked (paths + whole-file hash + ordered chunk sequence)
//   - Whether the check passed (pass)
//   - When the check was run
//
// specflowctl promote reads both caches and re-hashes every listed file: any
// content change stales the recorded evidence. The ordered chunk sequence
// localizes the change for the delta review (gate-plan --mode delta/repair);
// it never decides skip scope.
package validationcache

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// CheckCategory classifies why a cache check passed or failed. It mirrors
// the gate vocabulary of the freshness report: fresh (gate satisfied),
// missing (no cache file), stale (re-run the gate to fix), blocked
// (cache is valid but declares P0/P1 findings).
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
type CheckResult struct {
	Fresh    bool
	Category CheckCategory
	Reason   string
}

// cacheFile is the parsed representation of a cache file.
type cacheFile struct {
	Judgments    string
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
	// Review fields record the delta review that accepted the change set.
	ReviewChangeSet string   `yaml:"reviewed_change_set,omitempty"`
	ReviewResult    string   `yaml:"review_result,omitempty"`
	ReviewSession   string   `yaml:"review_session,omitempty"`
	ReviewRecheck   []string `yaml:"review_recheck,omitempty"`
	ReviewDeclined  []string `yaml:"review_declined,omitempty"`
	Files           []cacheFileEntry
}

// cacheFileEntry is one file in a cache's files list: its whole-file hash and
// ordered chunk sequence with line positions (the baseline side of the change
// diff), plus its per-check markers. Checks lists the judgment keys attached
// to this entry (lens coverage and failure-record statuses).
type cacheFileEntry struct {
	Path    string                    `yaml:"path"`
	Hash    string                    `yaml:"hash"`
	Checks  []checkEntry              `yaml:"checks,omitempty"`
	Chunker string                    `yaml:"chunker,omitempty"`
	Chunks  []contenthash.ChunkRecord `yaml:"chunks,omitempty"`
}

// checkEntry is one check marker inside a files entry: the check key, its lens
// tag (merged verify caches), and the judgment outcome in a failure-record
// cache (a fail/blocking cache — delta FAIL records, candidate full-run FAIL
// records, stable-only confirmation FAIL records): pass (judgment ran and
// passed), fail (judgment ran and retained at least one finding of any
// severity — P0–P3; the gate result itself is decided by P0/P1), carried (not
// re-run — recorded content unchanged from the pass baseline; delta records
// only). Status is required on every fail/blocking cache (the recovery input);
// absent status means pass only on a pass cache — the recovery never treats a
// failure record without a status map as pass (it covers the full scope).
type checkEntry struct {
	Check  string `yaml:"check"`
	Status string `yaml:"status,omitempty"` // pass | fail | carried (required on fail/blocking caches)
	Lens   string `yaml:"lens,omitempty"`   // alignment | quality (merged verify cache only)
}

// CheckValidate reads and validates the validate cache for the given unit.
// The cache must list the main candidate spec file; a cache whose files list
// omits it cannot prove the main spec was validated. A fail-result cache (a
// delta re-run's failure record) is rejected as blocking by the gate
// freshness chain.
func CheckValidate(repoRoot, unitName string) (CheckResult, error) {
	return checkCache(repoRoot, "unit", unitName, "validate", "validate_result.md", []string{"pass", "fail"}, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName))
}

// CheckVerify reads and validates the verify cache for the given unit.
// A pass-result cache satisfies the gate; a fail-result cache (a delta
// re-run's or a candidate full-run FAIL's failure record) is rejected as
// blocking by the gate freshness chain. P2/P3 pending findings are carried
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

	// 3. Resolve this unit's candidate appendices by declared ownership.
	matches, err := specpaths.UnitAppendices(repoRoot, unitName, "candidate")
	if err != nil {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("cannot resolve appendix ownership: %v — promote rejected", err),
		}, nil
	}

	// 4. Check each non-exempt candidate appendix.
	var missing []string
	for _, m := range matches {
		if m.Status == "exempt" {
			continue
		}
		if !cachedPaths[m.Path] {
			missing = append(missing, m.Path)
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

// ExpectedCheck is one required lens-tagged coverage key of the merged verify
// cache: the key and the lens section it must be recorded under.
type ExpectedCheck struct {
	Inputs  []string
	Unit    string
	Layer   string
	Kind    string
	Subject string
	Key     string
	Lens    string
}

// CheckVerifyMerged validates the merged verify cache for promote: the base
// verify chain (existence, full mode, dependency freshness, non-blocking), and
// that the cache records an alignment or quality check for every expected
// key. A cache that covers only one lens cannot prove the merged gate ran.
// A cache without per-check evidence fails closed: the merged cache format
// requires per-check lens declarations, and there is no compatibility shim
// for old caches. requireBothLenses additionally requires at least one check
// of each lens section even when expected names no key (the coverage
// derivation failed): an existing cache must still prove both lenses ran.
//
// The expected set is derived lazily through expected, and only after the
// cache is classified: a missing, unusable, or stale base cache returns before
// derivation, so a read-only freshness report pays no evidence-discovery work
// for those outcomes. A nil provider skips key checks entirely. A derivation
// failure fails closed as STALE with the provider's error as the reason.
func CheckVerifyMerged(repoRoot, unitName, target string, expected func() ([]ExpectedCheck, error), requireBothLenses bool) (CheckResult, error) {
	var (
		base CheckResult
		err  error
	)
	if target == "stable" {
		base, err = CheckVerifyStable(repoRoot, unitName)
	} else {
		base, err = CheckVerify(repoRoot, unitName)
	}
	if err != nil {
		return CheckResult{}, err
	}
	if base.Category == CategoryMissing {
		return base, nil
	}
	cachePath, err := cacheFilePath(repoRoot, "unit", unitName, "verify_result.md")
	if err != nil {
		return CheckResult{}, err
	}
	cache, err := readCache(cachePath)
	if err != nil {
		// An existing but unusable verify cache (a removed format, a partial
		// write, corruption) is not a usable gate result: fail it closed as
		// STALE with the parse reason instead of failing the whole command.
		return CheckResult{Fresh: false, Category: CategoryStale, Reason: fmt.Sprintf("verify cache is not in the current format: %v — run `verify@%s` again.", err, unitName)}, nil
	}
	got := map[string]string{}
	for _, entry := range cache.Files {
		for _, c := range entry.Checks {
			got[c.Check] = c.Lens
		}
	}
	if len(got) == 0 {
		// No compatibility shim: a merged verify cache must carry per-check
		// lens evidence. A cache without it cannot prove either lens ran and
		// cannot be checked against the current coverage set, so both fresh
		// and promote fail it closed — old caches are invalid, re-run
		// `verify@{unit}`.
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("verify cache carries no per-check evidence (no `checks` entries) — old caches are invalid; run `verify@%s` again.", unitName),
		}, nil
	}
	if !base.Fresh {
		return base, nil
	}
	var wants []ExpectedCheck
	if expected != nil {
		derived, err := expected()
		if err != nil {
			// The cache is otherwise reusable, so a failed derivation is the
			// cache's problem to report: fail closed as STALE with the
			// provider's error as the reason.
			return CheckResult{Fresh: false, Category: CategoryStale, Reason: err.Error()}, nil
		}
		wants = derived
	}
	needAlignment, needQuality := false, false
	for _, want := range wants {
		switch want.Lens {
		case "alignment":
			needAlignment = true
		case "quality":
			needQuality = true
		}
	}
	if requireBothLenses {
		// The cache carries per-check evidence, so it must prove both lens
		// sections ran.
		needAlignment = true
		needQuality = true
	}
	var missingKeys, missingLenses []string
	for _, want := range wants {
		if err := checkExpectedRecord(repoRoot, cache, want); err != nil {
			return CheckResult{Fresh: false, Category: CategoryStale, Reason: err.Error()}, nil
		}
		if got[want.Key] != want.Lens {
			missingKeys = append(missingKeys, want.Key)
		}
	}
	if needAlignment && !hasLens(got, "alignment") {
		missingLenses = append(missingLenses, "alignment")
	}
	if needQuality && !hasLens(got, "quality") {
		missingLenses = append(missingLenses, "quality")
	}
	if len(missingKeys) > 0 || len(missingLenses) > 0 {
		var parts []string
		if len(missingLenses) > 0 {
			parts = append(parts, strings.Join(missingLenses, ", ")+" lens")
		}
		if len(missingKeys) > 0 {
			parts = append(parts, "key(s) "+strings.Join(missingKeys, ", "))
		}
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("verify cache does not cover both lenses — missing %s. Run `verify@%s` again.", strings.Join(parts, ", "), unitName),
		}, nil
	}
	return base, nil
}

// hasLens reports whether the cache records at least one check under the lens.
func hasLens(got map[string]string, lens string) bool {
	for _, l := range got {
		if l == lens {
			return true
		}
	}
	return false
}

// blockingCheck validates the blocking declarations of a fail-capable cache
// (validate/verify failure records written by delta re-runs or a candidate
// full-run FAIL). It fails closed on a missing `blocking` field or a
// conflicting result/blocking declaration, and classifies a P0/P1 cache as
// CategoryBlocked (promote rejected, fresh reports BLOCKED). A nil result
// means the cache declares a consistent non-blocking state and the caller
// continues its normal checks.
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
// reason text, e.g. "verify" → "Verify").
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// FileEntry is the full content of one cache `files` entry: the whole-file
// hash and the ordered chunk sequence of one input-surface file at write
// time, plus the per-check markers attached to the entry (lens coverage and
// failure-record statuses). Hash and chunks are computed by the tooling
// (contenthash), never supplied by the agent.
type FileEntry struct {
	Path   string       `json:"path"`
	Hash   string       `json:"hash,omitempty"`
	Checks []CheckEntry `json:"checks,omitempty"`
	// Chunker identifies the chunking algorithm that produced Chunks; a
	// mismatch invalidates the entry (chunk sequences are only comparable
	// across the same chunker).
	Chunker string `json:"chunker,omitempty"`
	// Chunks is the ordered content-defined chunk sequence of the file at
	// write time, with line positions. It is the baseline side of the delta
	// change diff (contenthash.DiffChunks) — the reviewer's "what changed"
	// report. It never decides skip scope.
	Chunks []contenthash.ChunkRecord `json:"chunks,omitempty"`
}

// CheckEntry is one per-check marker inside a files entry: the check key, its
// lens tag (merged verify caches), and the judgment outcome in a failure
// record. The entry a check is attached to is bookkeeping only — evidence is
// the whole input surface, not a per-check dependency.
type CheckEntry struct {
	Check  string `json:"check"`
	Status string `json:"status,omitempty"` // pass | fail | carried (required on fail/blocking caches)
	Lens   string `json:"lens,omitempty"`   // alignment | quality (merged verify cache only)
}

// CacheWrite is the internal gate-finalize rendering bundle. gate-finalize
// derives the judgment fields from accepted session artifacts and computes the
// whole-file hash and ordered chunk sequence recorded for each Entries file
// from the run's input surface.
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
	// ReviewChangeSet/ReviewResult/ReviewSession/ReviewRecheck record the
	// delta review that accepted this cache's change set (delta/repair runs
	// only). They are the audit trail the promote boundary trusts.
	// ReviewDeclined records the carry candidates the review declined to
	// re-run (the offered candidate set minus ReviewRecheck).
	ReviewChangeSet string
	ReviewResult    string
	ReviewSession   string
	ReviewRecheck   []string
	ReviewDeclined  []string
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
// validate/verify variants point the main-file check at the stable spec.
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
	if w.ReviewChangeSet != "" {
		fmt.Fprintf(&b, "reviewed_change_set: %s\n", w.ReviewChangeSet)
		fmt.Fprintf(&b, "review_result: %s\n", w.ReviewResult)
		fmt.Fprintf(&b, "review_session: %s\n", w.ReviewSession)
		if len(w.ReviewRecheck) > 0 {
			fmt.Fprintf(&b, "review_recheck: %q\n", strings.Join(w.ReviewRecheck, ", "))
		}
		if len(w.ReviewDeclined) > 0 {
			fmt.Fprintf(&b, "review_declined: %q\n", strings.Join(w.ReviewDeclined, ", "))
		}
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
		if e.Chunker != "" && len(e.Chunks) > 0 {
			fmt.Fprintf(&b, "    chunker: %s\n", e.Chunker)
			b.WriteString("    chunks:\n")
			for _, c := range e.Chunks {
				fmt.Fprintf(&b, "      - cid: %s\n", c.CID)
				fmt.Fprintf(&b, "        start: %d\n", c.StartLine)
				fmt.Fprintf(&b, "        end: %d\n", c.EndLine)
			}
		}
		if len(e.Checks) > 0 {
			b.WriteString("    checks:\n")
			for _, c := range e.Checks {
				fmt.Fprintf(&b, "      - check: %q\n", c.Check)
				if c.Status != "" {
					fmt.Fprintf(&b, "        status: %s\n", c.Status)
				}
				if c.Lens != "" {
					fmt.Fprintf(&b, "        lens: %s\n", c.Lens)
				}
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
	// ReviewChangeSet/ReviewResult carry the delta review summary (empty on
	// full-run caches).
	ReviewChangeSet string
	ReviewResult    string
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
		Command:         cache.Command,
		Unit:            cache.Unit,
		Mode:            cache.Mode,
		Basis:           cache.Basis,
		Result:          cache.Result,
		Target:          cache.Target,
		Blocking:        cache.Blocking,
		P0Count:         cache.P0Count,
		P1Count:         cache.P1Count,
		P2Count:         cache.P2Count,
		P3Count:         cache.P3Count,
		Timestamp:       cache.Timestamp,
		FileCount:       len(cache.Files),
		ReviewChangeSet: cache.ReviewChangeSet,
		ReviewResult:    cache.ReviewResult,
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
		// An unusable cache is not a complete result that could satisfy the
		// gate — treat it like a missing cache; a future full run rebuilds it.
		return &TargetedInvalidation{Action: InvalidationNoCache, Path: relPath(repoRoot, cachePath)}, nil
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

// RefreshEvidence reads the entry's current content and replaces its hash and
// chunk evidence, resolving the entry's declared path (physical or logical).
func RefreshEvidence(repoRoot string, entry *FileEntry) error {
	abs := resolveEntryPath(repoRoot, entry.Path)
	if abs == "" {
		return fmt.Errorf("cannot resolve cache entry %q to an applicable file", entry.Path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("read %s: %w", entry.Path, err)
	}
	text := specpaths.NormalizeText(string(data))
	entry.Hash = contenthash.FileHashText(text)
	entry.Chunker = contenthash.ChunkerVersion
	entry.Chunks = contenthash.ChunkRecords(text)
	return nil
}

// BuildEvidenceEntry assembles one cache entry from current bytes: resolves
// declared (a physical path or a logical repository reference), then records
// the whole-file hash and ordered chunk sequence. A logical reference that
// resolves to no applicable file — or a file that does not exist — yields
// (nil, nil); the caller binds existing entries to the plan-time snapshot.
func BuildEvidenceEntry(repoRoot, declared string) (*FileEntry, error) {
	abs := resolveEntryPath(repoRoot, declared)
	if abs == "" {
		return nil, nil
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	entry := &FileEntry{Path: declared}
	if err := RefreshEvidence(repoRoot, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// DeleteCache removes a specific cache file (validate or verify) for the given unit.
func DeleteCache(repoRoot, unitName, command string) error {
	return deleteCache(repoRoot, "unit", unitName, command)
}

// DeleteAll removes the validate and verify caches for the given unit.
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

// ------------------------------------------------------------
// Internal
// ------------------------------------------------------------

// fileFreshnessState classifies the freshness of one cache file entry
// against the file's current content. Freshness is whole-file: any content
// change stales the entry, and the delta change report localizes it for the
// reviewer. There is no sub-file freshness: content-addressed regions never
// decide skip scope.
type fileFreshnessState int

const (
	fileMissing fileFreshnessState = iota // file is gone from disk
	fileChanged                           // the whole-file hash no longer matches
	fileFresh                             // the whole-file hash matches
)

// fileFreshness compares the recorded whole-file hash against the current
// content. A missing hash fails closed (changed).
func fileFreshness(repoRoot string, entry cacheFileEntry) (fileFreshnessState, error) {
	fullPath := resolveEntryPath(repoRoot, entry.Path)
	if fullPath == "" {
		return fileMissing, nil
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fileMissing, nil
		}
		return fileMissing, err
	}
	text := specpaths.NormalizeText(string(data))
	if entry.Hash == "" || normalizeHash(entry.Hash) != normalizeHash(contenthash.FileHashText(text)) {
		return fileChanged, nil
	}
	return fileFresh, nil
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
	if command == "verify" {
		if err := verifyRecords(repoRoot, cache); err != nil {
			return CheckResult{Fresh: false, Category: CategoryStale, Reason: err.Error()}, nil
		}
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
	// matching the gate's stale-over-blocking precedence.
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

	// A delta pass cache exists because an independent reviewer accepted its
	// change set; the review record is that acceptance's audit trail. A delta
	// cache without one is a pre-review artifact — fail closed.
	if cache.Result == "pass" && cache.Basis == "delta" && strings.TrimSpace(cache.ReviewChangeSet) == "" {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache is a delta result without a change-review record — run `%s@%s` again (review) or the full command", command, command, cache.Unit),
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

	// Re-check every listed file against its recorded whole-file hash.
	var staleFiles []string
	var missingFiles []string
	for _, entry := range cache.Files {
		state, err := fileFreshness(repoRoot, entry)
		if err != nil {
			missingFiles = append(missingFiles, fmt.Sprintf("%s (%v)", entry.Path, err))
			continue
		}
		switch state {
		case fileMissing:
			missingFiles = append(missingFiles, entry.Path)
		case fileChanged:
			staleFiles = append(staleFiles, entry.Path)
		}
	}

	if len(missingFiles) > 0 {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache stale: files missing: %s", command, strings.Join(missingFiles, ", ")),
		}, nil
	}
	if len(staleFiles) > 0 {
		return CheckResult{
			Fresh:    false,
			Category: CategoryStale,
			Reason:   fmt.Sprintf("%s cache stale: content changed in %s. Run `%s@%s` to review the change set.", command, strings.Join(staleFiles, ", "), command, cache.Unit),
		}, nil
	}

	// Blocking check — fail-capable caches (validate/verify failure records
	// written by delta re-runs or a candidate full-run FAIL, and pass
	// caches that declare a blocking field) are validated by the gate
	// freshness chain. A blocking cache is CategoryBlocked: promote rejects
	// it and fresh reports BLOCKED. The dependency check above takes
	// precedence — a stale failure record is STALE, not BLOCKED, matching
	// the gate's stale-over-blocking precedence.
	if cache.blockingSeen || cache.Result == "fail" {
		if res := blockingCheck(command, cache); res != nil {
			return *res, nil
		}
	}

	result := CheckResult{
		Fresh:    true,
		Category: CategoryFresh,
		Reason:   fmt.Sprintf("%s cache is fresh (result: %s, content of %d file(s) unchanged)", command, cache.Result, len(cache.Files)),
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

	cache := &cacheFile{Judgments: extractJudgments(content)}
	var currentEntry *cacheFileEntry
	var currentCheck *checkEntry
	inFilesBlock := false
	inChecksBlock := false
	inChunksBlock := false
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
			inChecksBlock = false
			inChunksBlock = false
			continue
		}

		if inFilesBlock {
			if strings.HasPrefix(trimmed, "- path:") {
				// New entry
				inChecksBlock = false
				inChunksBlock = false
				currentCheck = nil
				path := unquoteScalar(strings.TrimPrefix(trimmed, "- path:"))
				currentEntry = &cacheFileEntry{Path: path}
				cache.Files = append(cache.Files, *currentEntry)
				continue
			}
			if inChunksBlock {
				if strings.HasPrefix(trimmed, "- cid:") {
					cid := unquoteScalar(strings.TrimPrefix(trimmed, "- cid:"))
					if cid == "" {
						return nil, fmt.Errorf("cache file has an empty chunk `cid`")
					}
					currentEntry.Chunks = append(currentEntry.Chunks, contenthash.ChunkRecord{CID: cid})
					cache.Files[len(cache.Files)-1] = *currentEntry
					continue
				}
				if strings.HasPrefix(trimmed, "start:") {
					start, err := strconv.Atoi(unquoteScalar(strings.TrimPrefix(trimmed, "start:")))
					if err != nil {
						return nil, fmt.Errorf("cache file has invalid chunk `start` value %q", trimmed)
					}
					if len(currentEntry.Chunks) > 0 {
						currentEntry.Chunks[len(currentEntry.Chunks)-1].StartLine = start
					}
					cache.Files[len(cache.Files)-1] = *currentEntry
					continue
				}
				if strings.HasPrefix(trimmed, "end:") {
					end, err := strconv.Atoi(unquoteScalar(strings.TrimPrefix(trimmed, "end:")))
					if err != nil {
						return nil, fmt.Errorf("cache file has invalid chunk `end` value %q", trimmed)
					}
					if len(currentEntry.Chunks) > 0 {
						currentEntry.Chunks[len(currentEntry.Chunks)-1].EndLine = end
					}
					cache.Files[len(cache.Files)-1] = *currentEntry
					continue
				}
				inChunksBlock = false
			}
			if trimmed == "chunks:" {
				inChunksBlock = true
				inChecksBlock = false
				continue
			}
			if strings.HasPrefix(trimmed, "chunker:") && currentEntry != nil {
				currentEntry.Chunker = unquoteScalar(strings.TrimPrefix(trimmed, "chunker:"))
				cache.Files[len(cache.Files)-1] = *currentEntry
				continue
			}
			if inChecksBlock && strings.HasPrefix(trimmed, "- check:") {
				// New check entry inside a checks block
				check := unquoteScalar(strings.TrimPrefix(trimmed, "- check:"))
				currentCheck = &checkEntry{Check: check}
				if currentEntry != nil {
					currentEntry.Checks = append(currentEntry.Checks, *currentCheck)
					cache.Files[len(cache.Files)-1] = *currentEntry
				}
				continue
			}
			// A check entry's `status` and `lens` lines may appear in any
			// order inside the checks block.
			if inChecksBlock && strings.HasPrefix(trimmed, "status:") && currentCheck != nil && currentEntry != nil {
				status := unquoteScalar(strings.TrimPrefix(trimmed, "status:"))
				currentCheck.Status = status
				currentEntry.Checks[len(currentEntry.Checks)-1] = *currentCheck
				cache.Files[len(cache.Files)-1] = *currentEntry
				continue
			}
			if inChecksBlock && strings.HasPrefix(trimmed, "lens:") && currentCheck != nil && currentEntry != nil {
				lens := unquoteScalar(strings.TrimPrefix(trimmed, "lens:"))
				currentCheck.Lens = lens
				currentEntry.Checks[len(currentEntry.Checks)-1] = *currentCheck
				cache.Files[len(cache.Files)-1] = *currentEntry
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
			// An unrecognized line inside the files block is ignored in
			// place. The `files` block is the last top-level block, so a
			// removed or future field (e.g. the old `deps:` lines) must be
			// skipped without ending the block — otherwise every entry that
			// follows it would be silently dropped.
			continue
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
				case "reviewed_change_set":
					cache.ReviewChangeSet = value
				case "review_result":
					cache.ReviewResult = value
				case "review_session":
					cache.ReviewSession = value
				case "review_recheck":
					for _, tok := range strings.Split(value, ",") {
						key := strings.TrimSpace(tok)
						if key == "" {
							continue
						}
						cache.ReviewRecheck = append(cache.ReviewRecheck, key)
					}
				case "review_declined":
					for _, tok := range strings.Split(value, ",") {
						key := strings.TrimSpace(tok)
						if key == "" {
							continue
						}
						cache.ReviewDeclined = append(cache.ReviewDeclined, key)
					}
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

	// A cache is usable only when it carries the evidence the current format
	// is built on: every `files` entry records the chunk sequence
	// (`chunker` + `chunks`) computed at write time. This is a required part
	// of the format, not optional decoration: without it the entry cannot
	// participate in the current freshness and change-review contract. A
	// cache missing it — the removed `deps:`-declaration format, a partial
	// write, corruption — is rejected as not current-format. Callers that
	// read an existing cache treat this as "no usable cache" and fall back to
	// the full command; callers that validate a freshly rendered candidate
	// fail hard.
	for _, e := range cache.Files {
		if e.Chunker != contenthash.ChunkerVersion || len(e.Chunks) == 0 {
			return nil, fmt.Errorf("cache entry %q carries no comparable chunk evidence — the cache is not in the current format; run the full command to rebuild it", e.Path)
		}
	}

	return cache, nil
}

// InheritEntry reports one gate's fork-inheritance outcome.
type InheritEntry struct {
	Command   string // validate | verify
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
// stable spec and appendices verbatim, so the confirmation conclusions carry
// over: a gate cache with `result: pass` and `blocking: false` is rewritten —
// `target: stable` → `target: candidate` and physical paths under
// `docs/specs/units/stable/` → `docs/specs/units/candidate/` — and stays valid
// for the candidate round until the round's edits stale its evidence (delta
// re-runs restore the affected gates). Caches that cannot be inherited
// (missing, non-pass, blocking) are skipped with a reason — the forked round
// starts those gates from scratch. Rule forks do not inherit: a rule's cache
// declares the rule file whole, so a forked round always re-runs in full.
func InheritStableCaches(repoRoot, unitName string) (*InheritReport, error) {
	appendices, err := specpaths.UnitAppendices(repoRoot, unitName, "stable")
	if err != nil {
		return nil, err
	}
	var owned []string
	for _, appendix := range appendices {
		owned = append(owned, appendix.Path)
	}
	report := &InheritReport{Unit: unitName}
	for _, cmd := range []string{"validate", "verify"} {
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
		case cache.Blocking:
			entry.Reason = "confirmation cache is blocking (P0/P1 findings)"
		default:
			rewritten, changed := rewriteCacheLayer(string(data), owned)
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
// caches (validate/verify). For rules, only the validate cache is
// rewritten — the fresh@stable rule report consumes it as the consumer/
// consistency confirmation state.
func RewriteCachesToStable(repoRoot, targetKind, targetName string) (*PromoteCachesToStableReport, error) {
	var owned []string
	if targetKind == "unit" {
		appendices, err := specpaths.UnitAppendices(repoRoot, targetName, "stable")
		if err != nil {
			return nil, err
		}
		for _, appendix := range appendices {
			owned = append(owned, strings.Replace(appendix.Path, "docs/specs/units/stable/", "docs/specs/units/candidate/", 1))
		}
	}
	var commands []string
	switch targetKind {
	case "unit":
		commands = []string{"validate", "verify"}
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
			var rewritten string
			var changed bool
			if targetKind == "rule" {
				rewritten, err = projectRuleConfirmation(string(data), cache, targetName)
				if err != nil {
					return nil, fmt.Errorf("project rule confirmation: %w", err)
				}
				changed = rewritten != string(data)
			} else {
				rewritten, changed = rewriteCacheLayerToStable(string(data), owned)
			}
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

// projectRuleConfirmation preserves the published target's original evidence
// and rewrites the candidate rule path to its stable path. No hashes are
// rebuilt: evidence that no longer matches the published content goes stale
// through the normal dependency check and drives the delta re-run scope.
func projectRuleConfirmation(content string, cache *cacheFile, ruleID string) (string, error) {
	candidate := specpaths.RuleCandidateFileRef(ruleID)
	stable := specpaths.RuleStableFileRef(ruleID)
	w := CacheWrite{
		Command: cache.Command, Mode: cache.Mode, Basis: cache.Basis,
		Result: cache.Result, Target: "stable", Blocking: cache.Blocking,
		P0Count: cache.P0Count, P1Count: cache.P1Count,
		P2Count: cache.P2Count, P3Count: cache.P3Count,
		Timestamp: cache.Timestamp, GateRun: cache.GateRun,
		InvalidatedChecks: cache.InvalidatedChecks,
	}
	// The candidate input surface may record the rule file under both its
	// candidate path and its stable sibling; projecting the candidate path to
	// stable must leave one entry per file. The promoted artifact defines the
	// path's evidence: the superseded prior publication must not pin the
	// confirmation to content that promotion replaces.
	byPath := map[string]*FileEntry{}
	var order []string
	add := func(source cacheFileEntry, path string) {
		e := byPath[path]
		if e == nil {
			e = &FileEntry{Path: path, Hash: source.Hash, Chunker: source.Chunker, Chunks: append([]contenthash.ChunkRecord(nil), source.Chunks...)}
			byPath[path] = e
			order = append(order, path)
		}
		for _, check := range source.Checks {
			dup := false
			for _, existing := range e.Checks {
				if existing.Check == check.Check {
					dup = true
				}
			}
			if !dup {
				e.Checks = append(e.Checks, CheckEntry{Check: check.Check, Status: check.Status, Lens: check.Lens})
			}
		}
	}
	for _, source := range cache.Files {
		if source.Path == candidate {
			add(source, stable)
		}
	}
	for _, source := range cache.Files {
		if source.Path == candidate {
			continue
		}
		add(source, source.Path)
	}
	for _, path := range order {
		w.Entries = append(w.Entries, *byPath[path])
	}
	frontmatter, err := renderCacheFrontmatter(w, ruleID)
	if err != nil {
		return "", err
	}
	// Keep the structured judgments and historical reports byte-for-byte.
	lines := strings.Split(content, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return frontmatter + "\n" + strings.Join(lines[i+1:], "\n"), nil
		}
	}
	return "", fmt.Errorf("missing closing cache frontmatter delimiter")
}

// rewriteLayerFrontmatter transforms a cache file's frontmatter by replacing
// the `target` value and owned physical spec paths with the given layer
// strings. `fromLayer` is the current layer identifier (e.g. "stable" for a
// stable confirmation cache, "candidate" for a candidate gate cache);
// `toLayer` is the target layer. Physical paths under
// `docs/specs/units/{fromLayer}/` and `docs/specs/rules/{fromLayer}/` are
// rewritten to `docs/specs/units/{toLayer}/` and `docs/specs/rules/{toLayer}/`.
// Logical references (`unit:` / `rule:`) are untouched — they resolve by name
// to the current layer. Only the frontmatter (between the two `---` delimiters)
// is edited; the body is preserved verbatim. Returns whether anything changed.
func rewriteLayerFrontmatter(content string, fromLayer, toLayer string, appendices []string) (string, bool) {
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
	parsed, _ := parseCache([]byte(content))
	unit := ""
	if parsed != nil {
		unit = parsed.Unit
	}
	for i := 1; i < endIdx; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "target:") &&
			strings.TrimSpace(strings.TrimPrefix(trimmed, "target:")) == fromLayer:
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = leading + "target: " + toLayer
			changed = true
		case strings.HasPrefix(trimmed, "- path: docs/specs/units/"+fromLayer+"/") && ownLayerPath(strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")), unit, fromLayer, appendices):
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			newPath := "docs/specs/units/" + toLayer + "/" + strings.TrimPrefix(
				strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")),
				"docs/specs/units/"+fromLayer+"/")
			lines[i] = leading + "- path: " + newPath
			changed = true
		case strings.HasPrefix(trimmed, "- path: docs/specs/rules/"+fromLayer+"/") && (parsed == nil || parsed.Command != "verify"):
			leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			newPath := "docs/specs/rules/" + toLayer + "/" + strings.TrimPrefix(
				strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")),
				"docs/specs/rules/"+fromLayer+"/")
			lines[i] = leading + "- path: " + newPath
			changed = true
		}
	}
	rewritten := strings.Join(lines, "\n")
	cache, err := parseCache([]byte(content))
	if err == nil {
		withBindings := rewriteJudgmentBindings(rewritten, cache.Unit, fromLayer, toLayer)
		changed = changed || withBindings != rewritten
		rewritten = withBindings
	}
	return rewritten, changed
}

// rewriteCacheLayer rewrites a stable confirmation cache into its candidate
// form: `target: stable` → `target: candidate`, moving this unit's own spec
// paths while preserving peer evidence bindings.
func rewriteCacheLayer(content string, appendices []string) (string, bool) {
	return rewriteLayerFrontmatter(content, "stable", "candidate", appendices)
}

// rewriteCacheLayerToStable rewrites a candidate gate cache into its stable
// confirmation form: `target: candidate` → `target: stable`, moving this
// target's own spec paths while preserving peer evidence bindings.
func rewriteCacheLayerToStable(content string, appendices []string) (string, bool) {
	return rewriteLayerFrontmatter(content, "candidate", "stable", appendices)
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

// GateBaseline is the read-only view of a gate's existing cache, used by
// gate-plan to derive delta/repair session sets and by gate-finalize to merge
// carried-over evidence. A missing cache is Exists=false.
type GateBaseline struct {
	Command string
	Exists  bool
	// UnusableReason explains why an existing cache file could not be read as
	// a baseline — a removed format, a partial write, corruption. It is set
	// while Exists stays false: callers treat it exactly like a missing cache
	// and fall back to the full command. It is empty when the file is simply
	// absent.
	UnusableReason    string
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
// A missing cache returns Exists=false with no error. An existing but unusable
// cache — a removed format, a partial write, corruption — also returns
// Exists=false, with UnusableReason set: callers treat it exactly like a
// missing baseline and fall back to the full command.
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
		// An existing but unusable cache — a removed format, a partial write,
		// corruption — is not a usable baseline: degrade to "no baseline" and
		// let the caller fall back to the full command, instead of failing
		// the whole command. Unknown fields on a readable cache are ignored
		// by the parser; only missing or malformed required content reaches
		// this branch.
		return &GateBaseline{UnusableReason: fmt.Sprintf("gate cache %s is not in the current format: %v", relPath(repoRoot, cachePath), err)}, nil
	}
	baseline := &GateBaseline{Command: command,
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
		entry := FileEntry{Path: e.Path, Hash: e.Hash, Chunker: e.Chunker, Chunks: append([]contenthash.ChunkRecord(nil), e.Chunks...)}
		for _, c := range e.Checks {
			entry.Checks = append(entry.Checks, CheckEntry{Check: c.Check, Status: c.Status, Lens: c.Lens})
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

// CacheJudgmentReferences returns every judgment reference bound by the
// GATE_JUDGMENTS baseline of any published cache file, across all target
// kinds, target layers, and gates. The judgments tree is excluded: record
// files never embed the baseline marker. A file carrying the marker whose
// payload cannot be parsed is an error — the live-reference set must be
// computed from a fully readable state.
func CacheJudgmentReferences(repoRoot string) ([]judgments.Reference, error) {
	base := filepath.Join(repoRoot, "docs", "specs", "meta", "validation")
	var refs []judgments.Reference
	seen := map[string]bool{}
	add := func(ref judgments.Reference) {
		if ref.ID == "" || seen[ref.ID] {
			return
		}
		seen[ref.ID] = true
		refs = append(refs, ref)
	}
	walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "judgments" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		payload := extractJudgments(string(data))
		if strings.TrimSpace(payload) == "" {
			return nil
		}
		var state struct {
			Records map[string]judgments.Binding `json:"records"`
		}
		if err := json.Unmarshal([]byte(payload), &state); err != nil {
			return fmt.Errorf("parse judgment baseline %s: %w", relPath(repoRoot, p), err)
		}
		for _, binding := range state.Records {
			add(binding.Reference)
		}
		return nil
	})
	if os.IsNotExist(walkErr) {
		return nil, nil
	}
	if walkErr != nil {
		return nil, walkErr
	}
	return refs, nil
}

func ownLayerPath(p, unit, layer string, appendices []string) bool {
	_, ok := judgments.OwnSuffix(p, unit, layer, appendices)
	return ok
}

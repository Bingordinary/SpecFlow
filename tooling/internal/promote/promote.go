// Package promote validates candidate specs and archives them to stable.
// The tooling validates only deterministic format constraints (frontmatter fields,
// per-item acceptance schema, appendix file paths). Semantic validation
// (reference integrity, cross-unit consistency, acceptance completeness) is
// delegated to the validate subagent and is outside the promote tooling scope.
package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/baseline"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// Result describes the outcome of a promote operation.
type Result struct {
	Unit    string
	Passed  bool
	Issues  []string
	Actions []string
}

// stagedCopy is one file prepared for the atomic promote archive phase.
type stagedCopy struct {
	tmp string
	dst string
}

// stageCopy copies src to a temporary file next to dst. The final destination
// is written only by commitStaged, so a failure anywhere in the staging phase
// leaves the stable layer untouched.
func stageCopy(src, dst string) (string, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	return stageContent(data, dst, srcInfo.Mode().Perm())
}

func stageContent(data []byte, dst string, mode os.FileMode) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".sf-tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	// CreateTemp creates files with mode 0600; the promoted artifact must
	// keep the source file's permissions (copy semantics).
	if err := os.Chmod(tmpPath, mode); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func cleanupStaged(staged []stagedCopy) {
	for _, s := range staged {
		os.Remove(s.tmp)
	}
}

func commitStaged(staged []stagedCopy, removals ...string) error {
	return commitStagedWith(staged, os.Rename, removals...)
}

// backupEntry records one destination's pre-commit state during the archive
// phase. backup is empty when the destination had no original file.
type backupEntry struct {
	backup string
	dst    string
}

// commitStagedWith backs up replacements and candidate removals before publication
// and restores that set when publication fails:
//
//  1. backup — every destination and required candidate is renamed to a unique
//     `.sf-backup-*` temp name; a failure here restores the prepared destinations
//  2. commit — each staged temp file is renamed into place via the commit
//     function; a failure restores the entire prepared set, including the
//     failed and not-yet-committed destinations
//  3. success — remove this transaction's backups; candidate paths are absent
//
// The transaction owns staged-file cleanup. Failed restorations are reported
// alongside the original error, and their backups remain available.
func commitStagedWith(staged []stagedCopy, commit func(tmp, dst string) error, removals ...string) (err error) {
	backups := make([]backupEntry, 0, len(staged)+len(removals))
	defer func() {
		if err != nil {
			err = errors.Join(err, restoreBackups(backups))
		}
		cleanupStaged(staged)
	}()

	destinations := make([]string, 0, len(staged)+len(removals))
	for _, s := range staged {
		destinations = append(destinations, s.dst)
	}
	destinations = append(destinations, removals...)
	for i, dst := range destinations {
		if _, err := os.Lstat(dst); os.IsNotExist(err) && i < len(staged) {
			backups = append(backups, backupEntry{dst: dst})
			continue
		} else if err != nil {
			return fmt.Errorf("inspect publication path %s: %w", dst, err)
		}
		file, err := os.CreateTemp(filepath.Dir(dst), ".sf-backup-*")
		if err != nil {
			return fmt.Errorf("prepare backup for %s: %w", dst, err)
		}
		backup := file.Name()
		if err := file.Close(); err != nil {
			os.Remove(backup)
			return fmt.Errorf("prepare backup for %s: %w", dst, err)
		}
		if err := os.Rename(dst, backup); err != nil {
			os.Remove(backup)
			return fmt.Errorf("backup %s: %w", dst, err)
		}
		backups = append(backups, backupEntry{backup: backup, dst: dst})
	}

	for _, s := range staged {
		if err := commit(s.tmp, s.dst); err != nil {
			return fmt.Errorf("commit %s: %w", s.dst, err)
		}
	}

	for _, b := range backups {
		if b.backup != "" {
			os.Remove(b.backup)
		}
	}
	return nil
}

// restoreBackups restores every prepared destination, whether its replacement
// committed or not. It continues after errors and preserves failed backups.
func restoreBackups(backups []backupEntry) error {
	var errs []error
	for i := len(backups) - 1; i >= 0; i-- {
		b := backups[i]
		if b.backup != "" {
			if err := os.Rename(b.backup, b.dst); err != nil {
				errs = append(errs, fmt.Errorf("restore %s from %s: %w", b.dst, b.backup, err))
			}
		} else if err := os.Remove(b.dst); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove new destination %s: %w", b.dst, err))
		}
	}
	return errors.Join(errs...)
}

// Promote runs the promote flow for the given unit.
// Steps:
//  1. Check candidate spec exists
//  2. Validate frontmatter fields
//  3. Validate acceptance items
//  4. Find candidate appendix files
//  5. Copy candidate files to stable
func Promote(repoRoot, unitName string) *Result {
	r := &Result{Unit: unitName}

	candidateSpec := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", unitName))
	stableSpec := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", unitName))

	// Step 1: Check candidate spec exists
	if _, err := os.Stat(candidateSpec); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate spec not found: docs/specs/units/candidate/unit_%s.md", unitName))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found candidate spec: docs/specs/units/candidate/unit_%s.md", unitName))

	// Step 2: Read and validate frontmatter
	data, err := os.ReadFile(candidateSpec)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Cannot read candidate spec: %v", err))
		r.Passed = false
		return r
	}
	content := string(data)

	if header := specvalidation.CheckUnitFrontmatter(content, unitName); header.Status != specvalidation.Pass {
		r.Issues = append(r.Issues, header.Details)
		return r
	}
	fm := parseFrontmatter(content)

	// Step 3: Check the same per-item schema used by candidate validation.
	if schema := specvalidation.CheckAcceptanceItemSchema(content); schema.Status != specvalidation.Pass {
		r.Issues = append(r.Issues, schema.Details)
	}

	// Step 3b: Check unit_refs don't point to unpromoted candidate-only files
	if fm["unit_refs"] != "" && !strings.EqualFold(fm["unit_refs"], "none") {
		refs := specpaths.ParseRefList(fm["unit_refs"])
		for _, ref := range refs {
			if ref == "" || ref == unitName {
				continue
			}
			candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", ref))

			stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", ref))
			if _, err := os.Stat(stablePath); os.IsNotExist(err) {
				if _, err := os.Stat(candidatePath); err == nil {
					r.Issues = append(r.Issues, fmt.Sprintf("unit_refs target '%s' exists only in candidate layer — promote it first", ref))
				} else {
					r.Issues = append(r.Issues, fmt.Sprintf("unit_refs target '%s' does not exist in stable or candidate", ref))
				}
			}
		}
	}

	// Step 3c: Recheck rule publication before any write, including direct
	// callers that bypass the CLI's pre-cache check.
	rulePrerequisites, err := CheckUnitRulePrerequisites(repoRoot, unitName)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Rule prerequisite check: %v", err))
		return r
	}
	for _, blocker := range rulePrerequisites.Blockers {
		r.Issues = append(r.Issues, fmt.Sprintf("rule_refs target '%s': %s", blocker.RuleID, blocker.Reason))
	}
	if len(rulePrerequisites.Blockers) > 0 {
		return r
	}

	// Step 3d: Scan body for candidate-layer path references
	_, body, _ := specpaths.ParseFrontmatterFields(content)
	if body != "" {
		if refs := specvalidation.FindCandidateLayerPathRefs(body); len(refs) > 0 {
			r.Actions = append(r.Actions, fmt.Sprintf("WARNING: body contains candidate-layer path references (%s) — verify they are correct after promote", strings.Join(refs, ", ")))
		}
	}

	if len(r.Issues) > 0 {
		return r
	}

	appendixCopies, _, err := specpaths.PlanUnitAppendixCopies(repoRoot, unitName, "candidate")
	if err != nil {
		r.Issues = append(r.Issues, err.Error())
		return r
	}

	// Prepare every mandatory artifact before changing accepted truth.
	baselinePath, baselineData, err := baseline.PrepareUnitBaseline(repoRoot, unitName, content)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to prepare baseline: %v", err))
		return r
	}
	var staged []stagedCopy
	var removals, publicationActions []string
	for _, appendix := range appendixCopies {
		src := filepath.Join(repoRoot, filepath.FromSlash(appendix.Source))
		dst := filepath.Join(repoRoot, filepath.FromSlash(appendix.Destination))
		tmp, err := stageCopy(src, dst)
		if err != nil {
			cleanupStaged(staged)
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage appendix: %v", err))
			return r
		}
		staged = append(staged, stagedCopy{tmp: tmp, dst: dst})
		removals = append(removals, src)
		rel, _ := filepath.Rel(repoRoot, dst)
		publicationActions = append(publicationActions, fmt.Sprintf("Promoted appendix: %s", rel))
	}
	tmpSpec, err := stageCopy(candidateSpec, stableSpec)
	if err != nil {
		cleanupStaged(staged)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage spec: %v", err))
		return r
	}
	staged = append(staged, stagedCopy{tmp: tmpSpec, dst: stableSpec})
	tmpBaseline, err := stageContent(baselineData, baselinePath, 0644)
	if err != nil {
		cleanupStaged(staged)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage baseline: %v", err))
		return r
	}
	staged = append(staged, stagedCopy{tmp: tmpBaseline, dst: baselinePath})
	removals = append(removals, candidateSpec)

	// Stable content, its baseline, and candidate removal share one commit.
	if err := commitStaged(staged, removals...); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to commit promote: %v", err))
		return r
	}
	r.Actions = append(r.Actions, publicationActions...)
	r.Actions = append(r.Actions, fmt.Sprintf("Promoted: docs/specs/units/candidate/unit_%s.md -> docs/specs/units/stable/unit_%s.md", unitName, unitName))
	r.Actions = append(r.Actions, fmt.Sprintf("Recorded baseline: docs/specs/meta/baseline/unit/%s.yaml", unitName))
	for _, path := range removals {
		rel, _ := filepath.Rel(repoRoot, path)
		r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate: %s", rel))
	}

	// Step 7b: Rewrite candidate gate caches into stable confirmation caches.
	rewriteReport, rewrErr := validationcache.RewriteCachesToStable(repoRoot, "unit", unitName)
	if rewrErr != nil {
		// A rewrite failure is non-blocking — the stable content is already
		// committed and the candidate files removed. The delta-recovery
		// baseline is missing; fresh@stable reports MISSING until the user
		// triggers a stable confirmation run.
		r.Actions = append(r.Actions, fmt.Sprintf("Cache rewrite failed: %v — run validate/verify against stable to rebuild the confirmation caches", rewrErr))
	} else {
		for _, e := range rewriteReport.Entries {
			if e.Rewritten {
				r.Actions = append(r.Actions, "Promoted gate cache to stable confirmation cache")
			} else {
				r.Actions = append(r.Actions, e.Reason)
			}
		}
	}

	r.Passed = true
	return r
}

// RuleResult describes the outcome of a rule promote operation.
type RuleResult struct {
	RuleID  string
	Passed  bool
	Issues  []string
	Actions []string
}

// PromoteRule runs the promote flow for the given rule.
// Steps:
//  1. Check candidate rule file exists
//  2. Check validate cache freshness
//  3. Validate frontmatter fields (rule_id, rule_scope)
//  4. Copy candidate to stable (pure copy)
//  5. Delete candidate rule file
func PromoteRule(repoRoot, ruleID string) *RuleResult {
	r := &RuleResult{RuleID: ruleID}

	candidateRule := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ruleID))
	stableRule := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/stable/%s.md", ruleID))

	// Step 1: Check candidate rule exists
	if _, err := os.Stat(candidateRule); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate rule not found: docs/specs/rules/candidate/%s.md", ruleID))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found candidate rule: docs/specs/rules/candidate/%s.md", ruleID))

	// Step 2: Check validate cache freshness
	cacheResult, err := validationcache.CheckRuleValidate(repoRoot, ruleID)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Cannot check validate cache: %v", err))
		r.Passed = false
		return r
	}
	if !cacheResult.Fresh {
		r.Issues = append(r.Issues, fmt.Sprintf("Validate cache: %s. Run `validate@%s` before promoting.", cacheResult.Reason, ruleID))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Validate cache: %s", cacheResult.Reason))

	// Step 3: Read and validate frontmatter
	data, err := os.ReadFile(candidateRule)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Cannot read candidate rule: %v", err))
		r.Passed = false
		return r
	}

	fm := parseFrontmatter(string(data))

	requiredFields := []struct {
		field string
		value string
	}{
		{"rule_id", fm["rule_id"]},
		{"rule_scope", fm["rule_scope"]},
	}

	for _, f := range requiredFields {
		if f.value == "" {
			r.Issues = append(r.Issues, fmt.Sprintf("Missing required field: %s", f.field))
		}
	}

	if len(r.Issues) > 0 {
		r.Passed = false
		return r
	}

	// Prepare stable content and its baseline without changing accepted truth.
	tmp, err := stageCopy(candidateRule, stableRule)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage rule: %v", err))
		return r
	}
	staged := []stagedCopy{{tmp: tmp, dst: stableRule}}
	baselinePath, baselineData, err := baseline.PrepareRuleBaseline(repoRoot, ruleID, tmp)
	if err != nil {
		cleanupStaged(staged)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to prepare baseline: %v", err))
		return r
	}
	tmpBaseline, err := stageContent(baselineData, baselinePath, 0644)
	if err != nil {
		cleanupStaged(staged)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage baseline: %v", err))
		return r
	}
	staged = append(staged, stagedCopy{tmp: tmpBaseline, dst: baselinePath})
	if err := commitStaged(staged, candidateRule); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to commit rule: %v", err))
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Promoted: docs/specs/rules/candidate/%s.md -> docs/specs/rules/stable/%s.md", ruleID, ruleID))
	r.Actions = append(r.Actions, fmt.Sprintf("Recorded baseline: docs/specs/meta/baseline/rule/%s.yaml", ruleID))
	r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate rule: docs/specs/rules/candidate/%s.md", ruleID))

	// Step 9: Rewrite the candidate validate cache into a stable confirmation
	// cache. The rule's promote-time dependencies (consumer units, rule file
	// content) stay valid as the stable-layer consumer/consistency baseline.
	// Unlike unit caches, only the validate gate applies to rules (verify
	// has been removed for rules — see framework/validation_cache.md §Failure handling by gate role).
	rewriteReport, rewrErr := validationcache.RewriteCachesToStable(repoRoot, "rule", ruleID)
	if rewrErr != nil {
		r.Actions = append(r.Actions, fmt.Sprintf("Cache rewrite failed: %v — run validate@%s @stable to rebuild the confirmation cache", rewrErr, ruleID))
	} else {
		for _, e := range rewriteReport.Entries {
			if e.Rewritten {
				r.Actions = append(r.Actions, "Promoted rule validate cache to stable confirmation cache")
			} else {
				r.Actions = append(r.Actions, e.Reason)
			}
		}
	}

	r.Passed = true
	return r
}

// FormatRuleResult formats the rule promote result as readable output.
func FormatRuleResult(r *RuleResult) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Rule: %s\n", r.RuleID)

	if r.Passed {
		buf.WriteString("Result: PASSED\n\n")
	} else {
		buf.WriteString("Result: FAILED\n\n")
	}

	if len(r.Issues) > 0 {
		buf.WriteString("Issues:\n")
		for _, i := range r.Issues {
			fmt.Fprintf(&buf, "  - %s\n", i)
		}
		buf.WriteString("\n")
	}

	if len(r.Actions) > 0 {
		buf.WriteString("Actions:\n")
		for _, a := range r.Actions {
			fmt.Fprintf(&buf, "  - %s\n", a)
		}
		buf.WriteString("\n")
	}

	if r.Passed {
		buf.WriteString("Rule promoted to stable.\n")
		fmt.Fprintf(&buf, "Assess consumer impact per rule content: run `specflowctl consumers --rule %s`, then `specflowctl fresh --unit <name>` for applicable consumers. Report required changes and actual gate gaps; gates remain user-triggered.\n", r.RuleID)
	} else {
		buf.WriteString("Promote failed. Fix the issues above and try again.\n")
	}

	return buf.String()
}

// FormatResult formats the promote result as readable output.
func FormatResult(r *Result) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Unit: %s\n", r.Unit)

	if r.Passed {
		buf.WriteString("Result: PASSED\n\n")
	} else {
		buf.WriteString("Result: FAILED\n\n")
	}

	if len(r.Issues) > 0 {
		buf.WriteString("Issues:\n")
		for _, i := range r.Issues {
			fmt.Fprintf(&buf, "  - %s\n", i)
		}
		buf.WriteString("\n")
	}

	if len(r.Actions) > 0 {
		buf.WriteString("Actions:\n")
		for _, a := range r.Actions {
			fmt.Fprintf(&buf, "  - %s\n", a)
		}
		buf.WriteString("\n")
	}

	if r.Passed {
		buf.WriteString("Candidate spec has been promoted to stable.\n")
		buf.WriteString("Git handles version history.\n")
	} else {
		buf.WriteString("Promote failed. Fix the issues above and try again.\n")
	}

	return buf.String()
}

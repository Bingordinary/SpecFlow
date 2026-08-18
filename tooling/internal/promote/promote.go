// Package promote validates candidate specs and archives them to stable.
// The tooling validates only deterministic format constraints (frontmatter fields,
// acceptance_item_set presence, appendix file paths). Semantic validation
// (reference integrity, cross-unit consistency, acceptance completeness) is
// delegated to the validate subagent and is outside the promote tooling scope.
package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/baseline"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/ruledetect"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitgraph"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// VersionChangeType describes the kind of version change between candidate and stable.
type VersionChangeType int

const (
	ChangeNone  VersionChangeType = iota // No stable version exists yet
	ChangePatch                          // 0.0.x — wording clarification
	ChangeMinor                          // 0.x.0 — compatible extension
	ChangeMajor                          // x.0.0 — breaking constraint change
)

// Result describes the outcome of a promote operation.
type Result struct {
	Unit    string
	Passed  bool
	Issues  []string
	Actions []string
}

// stagedCopy is one file prepared for the atomic promote archive phase.
// When remove is true, the operation deletes the destination file instead of
// committing a staged copy into place: the backup phase moves the existing
// destination aside, the commit phase skips it, and the success phase
// discards the backup (the rollback phase restores it).
type stagedCopy struct {
	tmp    string
	dst    string
	remove bool
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
	if err := os.Chmod(tmpPath, srcInfo.Mode().Perm()); err != nil {
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

func commitStaged(staged []stagedCopy) error {
	return commitStagedWith(staged, os.Rename)
}

// backupEntry records one destination's pre-commit state during the archive
// phase. backup is empty when the destination had no original file.
type backupEntry struct {
	backup string
	dst    string
}

// commitStagedWith performs the archive phase with backup and rollback so a
// mid-commit failure never leaves the stable layer partially archived:
//
//  1. backup — every destination that already exists is renamed to a
//     `.sf-backup-*` temp name; a failure here restores the backups taken so far
//  2. commit — each staged temp file is renamed into place via the commit
//     function; a failure rolls back every committed destination (restore the
//     backup when one exists, otherwise remove the newly written file) and
//     cleans up the remaining temp files
//  3. success — all backups are removed
func commitStagedWith(staged []stagedCopy, commit func(tmp, dst string) error) error {
	backups := make([]backupEntry, len(staged))

	for i, s := range staged {
		if _, err := os.Stat(s.dst); err == nil {
			b := filepath.Join(filepath.Dir(s.dst), ".sf-backup-"+filepath.Base(s.dst))
			if err := os.Rename(s.dst, b); err != nil {
				restoreBackups(backups[:i])
				return fmt.Errorf("backup %s: %w", s.dst, err)
			}
			backups[i] = backupEntry{backup: b, dst: s.dst}
		} else {
			backups[i] = backupEntry{dst: s.dst}
		}
	}

	for i, s := range staged {
		if s.remove {
			continue
		}
		if err := commit(s.tmp, s.dst); err != nil {
			restoreBackups(backups[:i])
			cleanupStaged(staged[i:])
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

// restoreBackups reverts already-committed destinations to their original
// content: files that had a backup are renamed back (replacing the newly
// written file), files that had no original are removed.
func restoreBackups(backups []backupEntry) {
	for _, b := range backups {
		if b.backup != "" {
			os.Rename(b.backup, b.dst)
		} else {
			os.Remove(b.dst)
		}
	}
}

// readFrontmatterMap reads the frontmatter field map of a spec file, or nil
// if the file cannot be read.
func readFrontmatterMap(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseFrontmatter(string(data))
}

// findUnitReferrers resolves every unit to its current-layer (effective) file
// — candidate preferred, stable fallback, the same resolution `specflowctl
// deps` uses — and returns the unit names whose unit_refs still point at
// unitName. It protects a retiring unit: the stable copy is deleted on
// promote, so no current-layer unit may keep a reference to it. A stale
// stable file whose candidate has already dropped the reference does not
// block the retirement — the dangling reference is exposed by the stable
// confirmation check (`fresh@stable` validate) and disappears when the unit
// promotes (see `framework/spec_writing_guide.md` §8). A retiring unit's own
// references disappear with it, so retiring referrers are not counted.
// Resolution errors fail closed: the caller must not retire a unit whose
// referrer set cannot be determined.
func findUnitReferrers(repoRoot, unitName string) ([]string, error) {
	graph, err := unitgraph.Build(repoRoot, "all")
	if err != nil {
		return nil, fmt.Errorf("cannot resolve current-layer unit refs: %w", err)
	}
	var referrers []string
	for _, node := range graph.Nodes() {
		for _, ref := range node.UnitRefs {
			if ref == unitName {
				referrers = append(referrers, node.Name)
				break
			}
		}
	}
	sort.Strings(referrers)
	return referrers, nil
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

	fm := parseFrontmatter(content)
	retired := strings.TrimSpace(fm["status"]) == "retired"

	// Capture the stable predecessor's rule_refs before the archive phase
	// overwrites it. Rules that this round dropped from rule_refs are
	// re-detected after the promote commits; a rule left with no current-layer
	// consumers and no unbound_retention declaration is removed with it (see
	// Step 8).
	droppedRuleRefs := map[string]bool{}
	if stableData, err := os.ReadFile(stableSpec); err == nil {
		sfm := parseFrontmatter(string(stableData))
		if raw := sfm["rule_refs"]; raw != "" && !strings.EqualFold(raw, "none") {
			for _, ref := range specpaths.ParseRefList(raw) {
				name := strings.TrimSpace(strings.Split(ref, "@")[0])
				if name != "" {
					droppedRuleRefs[name] = true
				}
			}
		}
	}
	if raw := fm["rule_refs"]; raw != "" && !strings.EqualFold(raw, "none") {
		for _, ref := range specpaths.ParseRefList(raw) {
			name := strings.TrimSpace(strings.Split(ref, "@")[0])
			delete(droppedRuleRefs, name)
		}
	}

	if retired {
		referrers, err := findUnitReferrers(repoRoot, unitName)
		if err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Cannot determine the referrers of retiring unit %s: %v", unitName, err))
			r.Passed = false
			return r
		}
		if len(referrers) > 0 {
			r.Issues = append(r.Issues, fmt.Sprintf(
				"unit %s is being retired but is still referenced by: %s — remove the references before retiring",
				unitName, strings.Join(referrers, ", ")))
		}
	}

	checks := []struct {
		field string
		value string
	}{
		{"id", fm["id"]},
		{"version", fm["version"]},
	}

	for _, c := range checks {
		if c.value == "" {
			r.Issues = append(r.Issues, fmt.Sprintf("Missing required field: %s", c.field))
		}
	}

	// Step 3: Check acceptance items exist (a retiring spec declares the end of
	// the unit, so it is not required to carry an acceptance item set)
	if !retired {
		if !strings.Contains(content, "acceptance_item_set:") && !strings.Contains(content, "acceptance_item_set") {
			r.Issues = append(r.Issues, "No acceptance items found (acceptance_item_set is required)")
		}
	}

	// Step 3b: Check unit_refs don't point to unpromoted candidate-only files
	// and don't point to retiring targets. A retiring target loses its stable
	// copy on promote, so the reference cannot survive the retirement — even
	// when the stable copy still exists today. Skipped for a retiring unit: its
	// own references disappear with it (same exemption as step 3e and the
	// mechanical validate Check 4).
	if !retired && fm["unit_refs"] != "" && !strings.EqualFold(fm["unit_refs"], "none") {
		refs := specpaths.ParseRefList(fm["unit_refs"])
		for _, ref := range refs {
			ref = strings.TrimSpace(strings.Split(ref, "@")[0])
			if ref == "" || ref == unitName {
				continue
			}
			candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", ref))
			if targetFM := readFrontmatterMap(candidatePath); targetFM != nil && strings.TrimSpace(targetFM["status"]) == "retired" {
				r.Issues = append(r.Issues, fmt.Sprintf("unit_refs target '%s' is being retired — remove the references before retiring", ref))
				continue
			}
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

	// Step 3c: Check rule_refs don't point to unpromoted candidate-only files
	// or to nonexistent rules (a removed rule leaves the reference dangling).
	// Skipped for a retiring unit (same exemption as step 3b).
	if !retired && fm["rule_refs"] != "" && !strings.EqualFold(fm["rule_refs"], "none") {
		refs := specpaths.ParseRefList(fm["rule_refs"])
		for _, ref := range refs {
			ref = strings.TrimSpace(strings.Split(ref, "@")[0])
			if ref == "" {
				continue
			}
			candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ref))
			stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/stable/%s.md", ref))
			if _, err := os.Stat(stablePath); os.IsNotExist(err) {
				if _, err := os.Stat(candidatePath); err == nil {
					r.Issues = append(r.Issues, fmt.Sprintf("rule_refs target '%s' exists only in candidate layer — promote it first", ref))
				} else {
					r.Issues = append(r.Issues, fmt.Sprintf("rule_refs target '%s' does not exist in stable or candidate", ref))
				}
			}
		}
	}

	// Step 3d: Scan body for candidate-layer path references
	_, body, _ := specpaths.ParseFrontmatterFields(content)
	if body != "" {
		if refs := specvalidation.FindCandidateLayerPathRefs(body); len(refs) > 0 {
			r.Actions = append(r.Actions, fmt.Sprintf("WARNING: body contains candidate-layer path references (%s) — verify they are correct after promote", strings.Join(refs, ", ")))
		}
	}

	// Step 3e: Reject references to retiring appendices — the retiring appendix
	// is removed on promote, leaving the promoted spec with a dangling
	// reference. Same protection as the mechanical validate Check 4. A
	// retiring unit's own references disappear with it and are not checked.
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if !retired {
		if v := fm["evidence_appendix_ref"]; v != "" && !strings.EqualFold(v, "none") && specvalidation.AppendixMarkedRetired(appendixDir, v) {
			r.Issues = append(r.Issues, fmt.Sprintf("evidence_appendix_ref points to retiring appendix '%s' — remove the reference before retiring it", v))
		}
		for _, ref := range specvalidation.ExtractAffectsAppendices(content) {
			if specvalidation.AppendixMarkedRetired(appendixDir, ref) {
				r.Issues = append(r.Issues, fmt.Sprintf("affects.appendices references retiring appendix '%s' — remove the reference before retiring it", ref))
			}
		}
	}

	// Step 4: Check appendix files — active appendices are staged for the
	// archive phase, retiring appendices are scheduled for stable removal.
	stableAppendixDir := filepath.Join(repoRoot, "docs/specs/units/stable/appendix")
	pattern := fmt.Sprintf("unit_%s_*.md", unitName)
	matches, _ := filepath.Glob(filepath.Join(appendixDir, pattern))

	var staged []stagedCopy
	var removed []stagedCopy

	for _, m := range matches {
		rel, _ := filepath.Rel(repoRoot, m)
		appendixFM := readFrontmatterMap(m)
		appendixRetired := appendixFM != nil && strings.TrimSpace(appendixFM["status"]) == "retired"
		if retired {
			// A retiring unit takes every candidate appendix with it: nothing
			// is copied to stable. The stable removals are scheduled by the
			// retired-unit branch below (a single glob over the stable layer),
			// so this loop must not append the same destination again — a
			// duplicate remove entry would break the backup-phase rollback.
			r.Actions = append(r.Actions, fmt.Sprintf("Retiring appendix: %s", rel))
			continue
		}
		if appendixRetired {
			dest := filepath.Join(stableAppendixDir, filepath.Base(m))
			removed = append(removed, stagedCopy{dst: dest, remove: true})
			r.Actions = append(r.Actions, fmt.Sprintf("Retiring appendix: %s", rel))
			continue
		}
		r.Actions = append(r.Actions, fmt.Sprintf("Found appendix: %s", rel))

		dest := filepath.Join(stableAppendixDir, filepath.Base(m))
		tmp, err := stageCopy(m, dest)
		if err != nil {
			cleanupStaged(staged)
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage appendix: %v", err))
			r.Passed = false
			return r
		}
		staged = append(staged, stagedCopy{tmp: tmp, dst: dest})
		rel, _ = filepath.Rel(repoRoot, dest)
		r.Actions = append(r.Actions, fmt.Sprintf("Promoted appendix: %s", rel))
	}

	if len(r.Issues) > 0 {
		r.Passed = false
		return r
	}

	// Step 5: Stage the main spec last so it acts as the commit point. A
	// retiring unit is not copied — its stable main spec and every stable
	// appendix (including exempt ones) are scheduled for removal instead.
	if retired {
		stableAppendices, _ := filepath.Glob(filepath.Join(stableAppendixDir, pattern))
		for _, sm := range stableAppendices {
			removed = append(removed, stagedCopy{dst: sm, remove: true})
			rel, _ := filepath.Rel(repoRoot, sm)
			r.Actions = append(r.Actions, fmt.Sprintf("Retiring stable appendix: %s", rel))
		}
		removed = append(removed, stagedCopy{dst: stableSpec, remove: true})
		r.Actions = append(r.Actions, fmt.Sprintf("Retiring: docs/specs/units/stable/unit_%s.md", unitName))
	} else {
		tmpSpec, err := stageCopy(candidateSpec, stableSpec)
		if err != nil {
			cleanupStaged(staged)
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage spec: %v", err))
			r.Passed = false
			return r
		}
		staged = append(staged, stagedCopy{tmp: tmpSpec, dst: stableSpec})
	}

	// Phase 2 — rename staged files into place (appendices first, then the
	// main spec as the commit point) and remove retired destinations, all in
	// one transaction. Failures before this phase never touch the stable
	// layer: staging writes only temp files.
	commitList := append(staged, removed...)
	if err := commitStaged(commitList); err != nil {
		cleanupStaged(staged)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to commit promote: %v", err))
		r.Passed = false
		return r
	}
	if retired {
		r.Actions = append(r.Actions, fmt.Sprintf("Retired: docs/specs/units/stable/unit_%s.md and its stable appendices removed", unitName))
	} else {
		r.Actions = append(r.Actions, fmt.Sprintf("Promoted: docs/specs/units/candidate/unit_%s.md -> docs/specs/units/stable/unit_%s.md", unitName, unitName))
	}

	// Step 6: Record (or remove) the promote-time code-surface baseline. It
	// survives the cache cleanup — fresh@stable compares the current code
	// surface against it. Written before the candidate removal so a failure
	// leaves the candidate in place and promote can be re-run.
	if retired {
		if err := baseline.RemoveBaseline(repoRoot, "unit", unitName); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove baseline: %v", err))
			r.Passed = false
			return r
		}
	} else {
		verifyDeps, err := validationcache.ReadVerifyDeps(repoRoot, unitName)
		if err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to read verify dependency evidence: %v — re-run promote", err))
			r.Passed = false
			return r
		}
		if err := baseline.WriteUnitBaseline(repoRoot, unitName, content, verifyDeps); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to write baseline: %v — re-run promote or restore the baseline manually", err))
			r.Passed = false
			return r
		}
		r.Actions = append(r.Actions, fmt.Sprintf("Recorded baseline: docs/specs/meta/baseline/unit/%s.yaml", unitName))
	}

	// Step 7: Remove candidate files so file existence remains an unambiguous
	// state signal. "Candidate file exists = being edited" — after promote, no
	// editing is in progress. Appendices are removed first and the main spec
	// last, so a cleanup failure always leaves the candidate spec in place and
	// the promote can be safely re-run to completion.
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove candidate appendix: %s (%v)", filepath.Base(m), err))
			r.Passed = false
			return r
		}
		r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate appendix: docs/specs/units/candidate/appendix/%s", filepath.Base(m)))
	}
	if err := os.Remove(candidateSpec); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove candidate spec: unit_%s.md (%v)", unitName, err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate spec: docs/specs/units/candidate/unit_%s.md", unitName))

	// Step 7b: Rewrite (or delete) the candidate-layer gate caches into stable
	// confirmation caches. For a non-retired promote the caches become the
	// delta-recovery baseline for fresh@stable and fork inheritance; for a
	// retired promote the caches are deleted (the stable content is gone — a
	// rewritten cache would point at non-existent files and fail closed).
	if retired {
		if delErr := validationcache.DeleteAll(repoRoot, unitName); delErr != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to delete retired caches: %v — delete docs/specs/meta/validation/unit/%s/ manually", delErr, unitName))
			r.Passed = false
			return r
		}
		r.Actions = append(r.Actions, "Removed retired caches (validate, verify, review).")
	} else {
		rewriteReport, rewrErr := validationcache.RewriteCachesToStable(repoRoot, "unit", unitName)
		if rewrErr != nil {
			// A rewrite failure is non-blocking — the stable content is already
			// committed and the candidate files removed. The delta-recovery
			// baseline is missing; fresh@stable reports MISSING until the user
			// triggers a stable confirmation run.
			r.Actions = append(r.Actions, fmt.Sprintf("Cache rewrite failed: %v — run validate/verify/review against stable to rebuild the confirmation caches", rewrErr))
		} else {
			for _, e := range rewriteReport.Entries {
				if e.Rewritten {
					r.Actions = append(r.Actions, "Promoted gate cache to stable confirmation cache")
				} else {
					r.Actions = append(r.Actions, e.Reason)
				}
			}
		}
	}

	// Step 8: Clean up rules this round dropped from rule_refs. A bound rule
	// left with no current-layer (effective) consumers and no unbound_retention
	// declaration is removed — stable and candidate copies, baseline, and
	// validate cache — reusing the same detection primitive as
	// `specflowctl remove`. Global rules are never auto-removed: their default
	// applicability lifts only with an explicit user-invoked removal. Failures
	// report the concrete recovery path (`specflowctl remove --rule <id>`).
	for ruleID := range droppedRuleRefs {
		if !strings.HasPrefix(ruleID, "b_rule_") {
			continue
		}
		detect, err := ruledetect.DetectRule(repoRoot, ruleID)
		if err != nil {
			// The rule has no file in either layer — it was already removed
			// (e.g. `specflowctl remove --rule` ran before this promote
			// committed, leaving the stale stable reference to dangle).
			// Nothing is left to protect, so degrade to residual metadata
			// cleanup — the same degraded path `remove` takes — instead of
			// failing the promote (see spec_writing_guide.md §6.5).
			if errors.Is(err, ruledetect.ErrRuleNotFound) {
				if berr := baseline.RemoveBaseline(repoRoot, "rule", ruleID); berr != nil {
					r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove residual rule baseline %s: %v — run specflowctl remove --rule %s", ruleID, berr, ruleID))
					r.Passed = false
					return r
				}
				if cerr := validationcache.DeleteRuleCache(repoRoot, ruleID, "validate"); cerr != nil {
					r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove residual rule validate cache %s: %v — run specflowctl remove --rule %s", ruleID, cerr, ruleID))
					r.Passed = false
					return r
				}
				r.Actions = append(r.Actions, fmt.Sprintf("Rule %s already removed — cleaned up residual metadata", ruleID))
				continue
			}
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to detect rule %s for cleanup: %v — run specflowctl remove --rule %s manually", ruleID, err, ruleID))
			r.Passed = false
			return r
		}
		if !detect.Removable {
			continue
		}
		if detect.HasStable {
			if err := os.Remove(filepath.Join(repoRoot, filepath.FromSlash(specpaths.RuleStableFileRef(ruleID)))); err != nil {
				r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove rule %s: %v — run specflowctl remove --rule %s", ruleID, err, ruleID))
				r.Passed = false
				return r
			}
		}
		if detect.HasCandidate {
			if err := os.Remove(filepath.Join(repoRoot, filepath.FromSlash(specpaths.RuleCandidateFileRef(ruleID)))); err != nil {
				r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove candidate rule %s: %v — run specflowctl remove --rule %s", ruleID, err, ruleID))
				r.Passed = false
				return r
			}
		}
		if err := baseline.RemoveBaseline(repoRoot, "rule", ruleID); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove rule baseline %s: %v — run specflowctl remove --rule %s", ruleID, err, ruleID))
			r.Passed = false
			return r
		}
		if err := validationcache.DeleteRuleCache(repoRoot, ruleID, "validate"); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove rule validate cache %s: %v — run specflowctl remove --rule %s", ruleID, err, ruleID))
			r.Passed = false
			return r
		}
		r.Actions = append(r.Actions, fmt.Sprintf("Removed unbound rule: %s (no current-layer consumers, no unbound_retention)", ruleID))
	}

	r.Passed = true
	return r
}

// parseVersion extracts major, minor, patch from a semver string.
func parseVersion(v string) (major, minor, patch int, ok bool) {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	_, err := fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &patch)
	return major, minor, patch, err == nil
}

// versionChangeType determines the type of version change.
// If stableVersion is empty (brand new rule), returns ChangeNone.
func versionChangeType(candidateVersion, stableVersion string) VersionChangeType {
	if stableVersion == "" {
		return ChangeNone
	}
	cMaj, cMin, cPat, cOk := parseVersion(candidateVersion)
	sMaj, sMin, sPat, sOk := parseVersion(stableVersion)
	if !cOk || !sOk {
		return ChangeNone
	}
	if cMaj != sMaj {
		return ChangeMajor
	}
	if cMin != sMin {
		return ChangeMinor
	}
	if cPat != sPat {
		return ChangePatch
	}
	return ChangeNone
}

// RuleResult describes the outcome of a rule promote operation.
type RuleResult struct {
	RuleID     string
	Passed     bool
	Issues     []string
	Actions    []string
	ChangeType VersionChangeType
}

// PromoteRule runs the promote flow for the given rule.
// Steps:
//  1. Check candidate rule file exists
//  2. Check validate cache freshness
//  3. Validate frontmatter fields (rule_id, rule_scope, layer, rule_version)
//  4. Detect current stable version
//  5. Version sanity check (candidate version > stable version)
//  6. Determine version change type (MAJOR/MINOR/PATCH/none)
//  7. Copy candidate to stable (pure copy)
//  8. Delete candidate rule file
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
		{"rule_version", fm["rule_version"]},
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

	candidateVersion := fm["rule_version"]

	// Step 4: Detect current stable version
	stableVersion := ""
	if _, err := os.Stat(stableRule); err == nil {
		stableData, err := os.ReadFile(stableRule)
		if err == nil {
			stableFM := parseFrontmatter(string(stableData))
			stableVersion = stableFM["rule_version"]
		}
	}

	if stableVersion != "" {
		r.Actions = append(r.Actions, fmt.Sprintf("Current stable version: %s", stableVersion))
		r.Actions = append(r.Actions, fmt.Sprintf("Candidate version: %s", candidateVersion))
	}

	// Step 5: Version sanity check
	if stableVersion != "" && candidateVersion == stableVersion {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate version %s is same as stable version — bump the version", candidateVersion))
		r.Passed = false
		return r
	}
	if stableVersion != "" && !isVersionGreater(candidateVersion, stableVersion) {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate version %s must be greater than stable version %s", candidateVersion, stableVersion))
		r.Passed = false
		return r
	}

	// Step 6: Determine version change type
	r.ChangeType = versionChangeType(candidateVersion, stableVersion)
	switch r.ChangeType {
	case ChangeMajor:
		r.Actions = append(r.Actions, "MAJOR change detected")
	case ChangeMinor:
		r.Actions = append(r.Actions, "MINOR change detected")
	case ChangePatch:
		r.Actions = append(r.Actions, "PATCH change detected")
	case ChangeNone:
		r.Actions = append(r.Actions, "New rule promoted (no previous stable version)")
	}

	// Step 7: Copy candidate to stable (pure copy — the layer is encoded by the path), staged and atomic.
	tmp, err := stageCopy(candidateRule, stableRule)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage rule: %v", err))
		r.Passed = false
		return r
	}
	if err := commitStaged([]stagedCopy{{tmp: tmp, dst: stableRule}}); err != nil {
		os.Remove(tmp)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to commit rule: %v", err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Promoted: docs/specs/rules/candidate/%s.md -> docs/specs/rules/stable/%s.md", ruleID, ruleID))

	// Record the promote-time baseline. The rule's observable surface is the
	// archived rule file itself (a rule declares no code surface). Written
	// before the candidate removal so a failure leaves the candidate in place
	// and promote can be re-run.
	if err := baseline.WriteRuleBaseline(repoRoot, ruleID); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to write baseline: %v — re-run promote or restore the baseline manually", err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Recorded baseline: docs/specs/meta/baseline/rule/%s.yaml", ruleID))

	// Step 8: Delete candidate rule file. A cleanup failure keeps the candidate
	// rule in place; the stable rule is already archived, so the failure
	// reports the concrete recovery path instead of claiming success.
	if err := os.Remove(candidateRule); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove candidate rule: %s.md (%v). The stable rule is already updated; delete docs/specs/rules/candidate/%s.md manually, or fork from stable to continue editing.", ruleID, err, ruleID))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate rule: docs/specs/rules/candidate/%s.md", ruleID))

	// Step 9: Rewrite the candidate validate cache into a stable confirmation
	// cache. The rule's promote-time dependencies (consumer units, rule file
	// content) stay valid as the stable-layer consumer/consistency baseline.
	// Unlike unit caches, only the validate gate applies to rules (verify/review
	// have been removed for rules — see framework/concepts.md §Gate mapping).
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
		switch r.ChangeType {
		case ChangeMajor:
			buf.WriteString("MAJOR: Rule promoted. Assess consumer impact per rule content.\n")
		case ChangeMinor, ChangePatch:
			buf.WriteString("Compatible change: Rule promoted. Assess consumer impact per rule content.\n")
		default:
			buf.WriteString("New rule promoted to stable.\n")
		}
	} else {
		buf.WriteString("Promote failed. Fix the issues above and try again.\n")
	}

	return buf.String()
}

// isVersionGreater checks if v1 > v2 using MAJOR.MINOR.PATCH comparison.
func isVersionGreater(v1, v2 string) bool {
	m1, n1, p1, ok1 := parseVersion(v1)
	m2, n2, p2, ok2 := parseVersion(v2)
	if !ok1 || !ok2 {
		return false
	}

	if m1 != m2 {
		return m1 > m2
	}
	if n1 != n2 {
		return n1 > n2
	}
	return p1 > p2
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

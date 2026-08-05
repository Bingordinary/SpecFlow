// Package promote validates candidate specs and archives them to stable.
// The tooling validates only deterministic format constraints (frontmatter fields,
// acceptance_item_set presence, appendix file paths). Semantic validation
// (reference integrity, cross-unit consistency, acceptance completeness) is
// delegated to the validate subagent and is outside the promote tooling scope.
package promote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fileops"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
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
type stagedCopy struct {
	tmp string
	dst string
}

// stageCopyWithLayerTransform copies src to a temporary file next to dst,
// applying the candidate→stable layer transform. The final destination is
// written only by commitStaged, so a failure anywhere in the staging phase
// leaves the stable layer untouched.
func stageCopyWithLayerTransform(src, dst string) (string, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	transformed := fileops.TransformLayerInFrontmatter(string(data), "candidate", "stable")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".sf-tmp-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write([]byte(transformed)); err != nil {
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
	checks := []struct {
		field string
		value string
	}{
		{"id", fm["id"]},
		{"layer", fm["layer"]},
		{"version", fm["version"]},
	}

	for _, c := range checks {
		if c.value == "" {
			r.Issues = append(r.Issues, fmt.Sprintf("Missing required field: %s", c.field))
		}
	}

	if v := fm["layer"]; v != "" && !strings.EqualFold(v, "candidate") {
		r.Issues = append(r.Issues, fmt.Sprintf("Layer must be 'candidate', got '%s'", v))
	}

	// Step 3: Check acceptance items exist
	if !strings.Contains(content, "acceptance_item_set:") && !strings.Contains(content, "acceptance_item_set") {
		r.Issues = append(r.Issues, "No acceptance items found (acceptance_item_set is required)")
	}

	// Step 3b: Check unit_refs don't point to unpromoted candidate-only files
	if fm["unit_refs"] != "" && !strings.EqualFold(fm["unit_refs"], "none") {
		refs := specpaths.ParseRefList(fm["unit_refs"])
		for _, ref := range refs {
			ref = strings.TrimSpace(strings.Split(ref, "@")[0])
			if ref == "" || ref == unitName {
				continue
			}
			stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/stable/unit_%s.md", ref))
			candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/units/candidate/unit_%s.md", ref))
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
	if fm["rule_refs"] != "" && !strings.EqualFold(fm["rule_refs"], "none") {
		refs := specpaths.ParseRefList(fm["rule_refs"])
		for _, ref := range refs {
			ref = strings.TrimSpace(strings.Split(ref, "@")[0])
			if ref == "" {
				continue
			}
			stablePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/stable/%s.md", ref))
			candidatePath := filepath.Join(repoRoot, fmt.Sprintf("docs/specs/rules/candidate/%s.md", ref))
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

	// Step 4: Check appendix files
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	pattern := fmt.Sprintf("unit_%s_*.md", unitName)
	matches, _ := filepath.Glob(filepath.Join(appendixDir, pattern))
	for _, m := range matches {
		rel, _ := filepath.Rel(repoRoot, m)
		r.Actions = append(r.Actions, fmt.Sprintf("Found appendix: %s", rel))
	}

	if len(r.Issues) > 0 {
		r.Passed = false
		return r
	}

	// Step 5: Copy candidate files to stable (two-phase commit)
	// Phase 1 — stage every file to a temp file; a failure here leaves the
	// stable layer untouched (no partial archive).
	stableAppendixDir := filepath.Join(repoRoot, "docs/specs/units/stable/appendix")
	var staged []stagedCopy

	for _, m := range matches {
		dest := filepath.Join(stableAppendixDir, filepath.Base(m))
		tmp, err := stageCopyWithLayerTransform(m, dest)
		if err != nil {
			cleanupStaged(staged)
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage appendix: %v", err))
			r.Passed = false
			return r
		}
		staged = append(staged, stagedCopy{tmp: tmp, dst: dest})
		rel, _ := filepath.Rel(repoRoot, dest)
		r.Actions = append(r.Actions, fmt.Sprintf("Promoted appendix: %s", rel))
	}

	// Stage the main spec last so it acts as the commit point.
	tmpSpec, err := stageCopyWithLayerTransform(candidateSpec, stableSpec)
	if err != nil {
		cleanupStaged(staged)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to stage spec: %v", err))
		r.Passed = false
		return r
	}
	staged = append(staged, stagedCopy{tmp: tmpSpec, dst: stableSpec})

	// Phase 2 — rename staged files into place (appendices first, then the
	// main spec as the commit point). Failures before this phase never touch
	// the stable layer: staging writes only temp files.
	if err := commitStaged(staged); err != nil {
		cleanupStaged(staged)
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to commit promote: %v", err))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Promoted: docs/specs/units/candidate/unit_%s.md -> docs/specs/units/stable/unit_%s.md", unitName, unitName))

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
//  7. Copy candidate to stable with layer transform
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
		{"layer", fm["layer"]},
		{"rule_version", fm["rule_version"]},
	}

	for _, f := range requiredFields {
		if f.value == "" {
			r.Issues = append(r.Issues, fmt.Sprintf("Missing required field: %s", f.field))
		}
	}

	if v := fm["layer"]; v != "" && !strings.EqualFold(v, "candidate") {
		r.Issues = append(r.Issues, fmt.Sprintf("Layer must be 'candidate', got '%s'", v))
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

	// Step 7: Copy candidate to stable with layer transform (staged, atomic)
	tmp, err := stageCopyWithLayerTransform(candidateRule, stableRule)
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

	// Step 8: Delete candidate rule file. A cleanup failure keeps the candidate
	// rule in place; the stable rule is already archived, so the failure
	// reports the concrete recovery path instead of claiming success.
	if err := os.Remove(candidateRule); err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Promote succeeded but failed to remove candidate rule: %s.md (%v). The stable rule is already updated; delete docs/specs/rules/candidate/%s.md manually, or fork from stable to continue editing.", ruleID, err, ruleID))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Removed candidate rule: docs/specs/rules/candidate/%s.md", ruleID))

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

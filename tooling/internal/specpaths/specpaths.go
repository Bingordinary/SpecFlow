package specpaths

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	ModulesRootDir           = "docs/specs/units"
	CandidateDir             = ModulesRootDir + "/candidate"
	StableDir                = ModulesRootDir + "/stable"
	CandidateAppendixDir     = CandidateDir + "/appendix"
	StableAppendixDir        = StableDir + "/appendix"

	RuleModulesRootDir       = "docs/specs/rules"
	RuleCandidateDir         = RuleModulesRootDir + "/candidate"
	RuleStableDir            = RuleModulesRootDir + "/stable"
)

// CandidateUnitSpecFileRef returns the candidate-layer path of a unit main
// spec. The layer is fully encoded by the path; spec files no longer declare
// a `layer` frontmatter field.
func CandidateUnitSpecFileRef(unit string) string {
	return fmt.Sprintf("%s/unit_%s.md", CandidateDir, unit)
}

// StableUnitSpecFileRef returns the stable-layer path of a unit main spec.
func StableUnitSpecFileRef(unit string) string {
	return fmt.Sprintf("%s/unit_%s.md", StableDir, unit)
}

// RuleCandidateFileRef returns the candidate-layer path of a rule file.
func RuleCandidateFileRef(ruleID string) string {
	return fmt.Sprintf("%s/%s.md", RuleCandidateDir, ruleID)
}

// RuleStableFileRef returns the stable-layer path of a rule file.
func RuleStableFileRef(ruleID string) string {
	return fmt.Sprintf("%s/%s.md", RuleStableDir, ruleID)
}

// ResolveUnitFile resolves a unit logical reference to the current-layer main
// spec file (candidate first, stable fallback). It returns "" when the unit
// exists in no layer. Cache dependency entries for cross-unit references use
// logical references (`unit:<name>`), so a promote of the referenced unit
// does not stale caches that depended on its (unchanged) content.
func ResolveUnitFile(repoRoot, unit string) string {
	for _, ref := range []string{CandidateUnitSpecFileRef(unit), StableUnitSpecFileRef(unit)} {
		p := filepath.Join(repoRoot, filepath.FromSlash(ref))
		if _, err := os.Stat(p); err == nil {
			return filepath.ToSlash(p)
		}
	}
	return ""
}

// ResolveUnitAppendix resolves a unit appendix logical reference to the
// current-layer appendix file (candidate first, stable fallback). appendix is
// the full file base name without extension (e.g. "unit_auth_account_token_claims").
// It returns "" when the appendix exists in no layer. Cache dependency entries
// for cross-unit reads of a dependency unit's protocol appendices use logical
// references (`unit:<name>:appendix:<file>`), so a promote of the referenced
// unit does not stale caches that depended on its (unchanged) appendix content.
func ResolveUnitAppendix(repoRoot, appendix string) string {
	for _, dir := range []string{CandidateAppendixDir, StableAppendixDir} {
		p := filepath.Join(repoRoot, filepath.FromSlash(dir), appendix+".md")
		if _, err := os.Stat(p); err == nil {
			return filepath.ToSlash(p)
		}
	}
	return ""
}

// ResolveRuleFile resolves a rule logical reference to the current-layer rule
// file (candidate first, stable fallback), or "" when the rule exists in no
// layer.
func ResolveRuleFile(repoRoot, ruleID string) string {
	for _, ref := range []string{RuleCandidateFileRef(ruleID), RuleStableFileRef(ruleID)} {
		p := filepath.Join(repoRoot, filepath.FromSlash(ref))
		if _, err := os.Stat(p); err == nil {
			return filepath.ToSlash(p)
		}
	}
	return ""
}

func CandidateAppendixGlob(unit string) string {
	return fmt.Sprintf("%s/unit_%s_*.md", CandidateAppendixDir, unit)
}

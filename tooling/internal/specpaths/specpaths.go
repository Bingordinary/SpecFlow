package specpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// targetNamePattern is the single naming contract for unit names and rule ids
// (tooling/README.md §Target names). Target names are external input used to
// construct repository paths, so every CLI entry that accepts one validates it
// before construction; the gate-run planner, the validation-cache paths, and
// the operation-scope target use this same predicate.
var targetNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// ValidateTargetName rejects a unit name or rule id that is not a plain
// identifier. This is the boundary that keeps external names out of path
// construction: a name containing a separator, a traversal segment, or
// whitespace is rejected before it can reach CandidateUnitSpecFileRef /
// cacheFilePath and therefore cannot escape its governed directory.
func ValidateTargetName(kind, name string) error {
	switch kind {
	case "unit":
		if !targetNamePattern.MatchString(name) {
			return fmt.Errorf("unit name %q is invalid: expected letters, digits, '_' or '-' starting with a letter or digit", name)
		}
	case "rule":
		if !targetNamePattern.MatchString(name) {
			return fmt.Errorf("rule id %q is invalid: expected letters, digits, '_' or '-' starting with a letter or digit", name)
		}
	default:
		return fmt.Errorf("invalid target kind %q: expected 'unit' or 'rule'", kind)
	}
	return nil
}

const (
	ModulesRootDir       = "docs/specs/units"
	CandidateDir         = ModulesRootDir + "/candidate"
	StableDir            = ModulesRootDir + "/stable"
	CandidateAppendixDir = CandidateDir + "/appendix"
	StableAppendixDir    = StableDir + "/appendix"

	RuleModulesRootDir = "docs/specs/rules"
	RuleCandidateDir   = RuleModulesRootDir + "/candidate"
	RuleStableDir      = RuleModulesRootDir + "/stable"
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

// ResolveRuleFile resolves a rule logical reference according to its
// applicability. Global rules (g_rule_*) are active only from the stable
// layer; bound rules keep current-layer semantics (candidate first, stable
// fallback). It returns "" when no applicable file exists.
func ResolveRuleFile(repoRoot, ruleID string) string {
	if strings.HasPrefix(ruleID, "g_rule_") {
		p := filepath.Join(repoRoot, filepath.FromSlash(RuleStableFileRef(ruleID)))
		if _, err := os.Stat(p); err == nil {
			return filepath.ToSlash(p)
		}
		return ""
	}
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

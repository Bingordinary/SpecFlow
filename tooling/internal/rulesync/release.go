package rulesync

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/rulerefs"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitdiscovery"
)

type ConsumerOptions struct {
	RuleID  string
	RuleRef string
}

type Consumer struct {
	ObjectType  string
	Object      string
	ActiveLayer string
	FileRef     string
	RuleRefs    []string
}

type ConsumerResult struct {
	RuleID    string
	RuleRef   string
	Consumers []Consumer
}

type ReleaseVersionOptions struct {
	RuleID  string
	FromRef string
	ToRef   string
}

type ReleaseVersionResult struct {
	RuleID           string
	FromRef          string
	ToRef            string
	CandidateUpdated []string
	Sync             Result
}

type preparedAppendix struct {
	fileRef string
	content string
}

var markdownLinkTargetPattern = regexp.MustCompile(`\]\(([^)\n]+)\)`)

// Consumers discovers which units reference a given rule.
func Consumers(repoRoot string, options ConsumerOptions) (ConsumerResult, error) {
	ruleID := strings.TrimSpace(options.RuleID)
	ruleRef := strings.TrimSpace(options.RuleRef)
	if (ruleID == "") == (ruleRef == "") {
		return ConsumerResult{}, fmt.Errorf("exactly one of rule id or rule ref is required")
	}

	sharedFilesByRef, err := loadSharedFiles(repoRoot)
	if err != nil {
		return ConsumerResult{}, err
	}

	units, err := unitdiscovery.DiscoverUnits(repoRoot)
	if err != nil {
		return ConsumerResult{}, err
	}

	result := ConsumerResult{RuleID: ruleID, RuleRef: ruleRef}
	for _, unit := range units {
		prefix := "c"
		if !unit.HasCandidate {
			prefix = "s"
		}
		fileRef := fmt.Sprintf("docs/specs/units/%s/%s_unit_%s.md", unit.Layer(), prefix, unit.ID)
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
		if err != nil {
			return ConsumerResult{}, fmt.Errorf("read %s: %w", fileRef, err)
		}
		refs, err := rulerefs.ParseObjectRuleRefs(fileRef, string(content))
		if err != nil {
			return ConsumerResult{}, err
		}

		matchedRefs := []string{}
		for _, ref := range refs {
			if ruleRef != "" {
				if bareRuleName(ref) == bareRuleName(ruleRef) {
					matchedRefs = append(matchedRefs, ref)
				}
				continue
			}
			shared, ok := sharedFilesByRef[ref]
			if ok && shared.RuleID == ruleID {
				matchedRefs = append(matchedRefs, ref)
				continue
			}
			for key, s := range sharedFilesByRef {
				if bareRuleName(ref) == bareRuleName(key) && s.RuleID == ruleID {
					matchedRefs = append(matchedRefs, ref)
					break
				}
			}
		}
		if len(matchedRefs) == 0 {
			continue
		}
		result.Consumers = append(result.Consumers, Consumer{
			ObjectType:  "unit",
			Object:      unit.ID,
			ActiveLayer: unit.Layer(),
			FileRef:     fileRef,
			RuleRefs:    normalizeStrings(matchedRefs),
		})
	}
	return result, nil
}

// ReleaseVersion retargets candidate unit rule_refs from the old stable ref to the new one.
// In the simplified model, this only updates candidate-layer spec files and does not
// manage lifecycle state, create forks, or manage process artifacts.
func ReleaseVersion(repoRoot string, options ReleaseVersionOptions) (ReleaseVersionResult, error) {
	normalized := ReleaseVersionOptions{
		RuleID:  strings.TrimSpace(options.RuleID),
		FromRef: strings.TrimSpace(options.FromRef),
		ToRef:   strings.TrimSpace(options.ToRef),
	}
	if normalized.RuleID == "" || normalized.FromRef == "" || normalized.ToRef == "" {
		return ReleaseVersionResult{}, fmt.Errorf("rule id, from-ref, and to-ref are required")
	}
	if normalized.FromRef == normalized.ToRef {
		return ReleaseVersionResult{}, fmt.Errorf("from-ref and to-ref must be different")
	}

	sharedFilesByRef, err := loadSharedFiles(repoRoot)
	if err != nil {
		return ReleaseVersionResult{}, err
	}
	toShared, ok := sharedFilesByRef[normalized.ToRef]
	if !ok {
		return ReleaseVersionResult{}, fmt.Errorf("to-ref %q is not present under docs/specs/rules/", normalized.ToRef)
	}
	if toShared.RuleID != normalized.RuleID {
		return ReleaseVersionResult{}, fmt.Errorf("to-ref %q belongs to rule_id %q, not %q", normalized.ToRef, toShared.RuleID, normalized.RuleID)
	}
	if toShared.Layer != "stable" {
		return ReleaseVersionResult{}, fmt.Errorf("to-ref %q must point to a stable-layer rule file", normalized.ToRef)
	}
	fromPrefix, err := ruleRefPrefix(normalized.FromRef)
	if err != nil {
		return ReleaseVersionResult{}, err
	}
	toPrefix, err := ruleRefPrefix(normalized.ToRef)
	if err != nil {
		return ReleaseVersionResult{}, err
	}
	fromShared, ok := sharedFilesByRef[normalized.FromRef]
	if !ok {
		return ReleaseVersionResult{}, fmt.Errorf("from-ref %q is not present under docs/specs/rules/", normalized.FromRef)
	}
	if fromShared.Layer != "stable" {
		return ReleaseVersionResult{}, fmt.Errorf("from-ref %q must point to a stable-layer rule ref", normalized.FromRef)
	}
	if fromPrefix != toPrefix {
		return ReleaseVersionResult{}, fmt.Errorf("from-ref %q and to-ref %q must refer to the same rule file prefix", normalized.FromRef, normalized.ToRef)
	}

	units, err := unitdiscovery.DiscoverUnits(repoRoot)
	if err != nil {
		return ReleaseVersionResult{}, err
	}

	result := ReleaseVersionResult{
		RuleID:  normalized.RuleID,
		FromRef: normalized.FromRef,
		ToRef:   normalized.ToRef,
	}

	for _, unit := range units {
		if !unit.HasCandidate {
			continue
		}
		fileRef := fmt.Sprintf("docs/specs/units/candidate/c_unit_%s.md", unit.ID)
		contentBytes, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
		if err != nil {
			return ReleaseVersionResult{}, fmt.Errorf("read %s: %w", fileRef, err)
		}
		refs, err := rulerefs.ParseObjectRuleRefs(fileRef, string(contentBytes))
		if err != nil {
			return ReleaseVersionResult{}, err
		}
		nextRefs, changed := replaceRuleRef(refs, normalized.FromRef, normalized.ToRef)
		if !changed {
			continue
		}
		updated, err := rulerefs.UpdateObjectRuleRefs(fileRef, string(contentBytes), nextRefs)
		if err != nil {
			return ReleaseVersionResult{}, err
		}
		if err := os.WriteFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)), []byte(updated), 0o644); err != nil {
			return ReleaseVersionResult{}, fmt.Errorf("write %s: %w", fileRef, err)
		}
		result.CandidateUpdated = append(result.CandidateUpdated, "unit:"+unit.ID)
	}

	syncResult, err := SyncImpact(repoRoot, Options{RuleRefs: []string{normalized.ToRef}})
	if err != nil {
		return ReleaseVersionResult{}, err
	}
	result.Sync = syncResult

	return result, nil
}

func replaceRuleRef(refs []string, fromRef, toRef string) ([]string, bool) {
	changed := false
	next := make([]string, 0, len(refs))
	for _, ref := range refs {
		if bareRuleName(ref) == bareRuleName(fromRef) {
			if strings.Contains(ref, "@") {
				next = append(next, toRef)
				changed = true
				continue
			}
		}
		next = append(next, ref)
	}
	if !changed {
		return refs, false
	}
	normalized, err := rulerefs.NormalizeRuleRefs(next)
	if err != nil {
		return next, true
	}
	return normalized, true
}

func ruleRefPrefix(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("invalid rule ref %q", ref)
	}
	parts := strings.SplitN(ref, "@", 2)
	return parts[0], nil
}

func bareRuleName(ref string) string {
	ref = strings.TrimSpace(ref)
	parts := strings.SplitN(ref, "@", 2)
	return parts[0]
}

func bumpPatchVersion(version string) (string, error) {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("version %q must use MAJOR.MINOR.PATCH", version)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", fmt.Errorf("version %q has invalid PATCH: %w", version, err)
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1), nil
}

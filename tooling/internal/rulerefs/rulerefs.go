package rulerefs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitgraph"
)

var refPattern = regexp.MustCompile(`^[a-z]_[a-z]_[a-z0-9_]+$`)

type document struct {
	lines     []string
	endIndex  int
	bodyLines []string
}

func ParseObjectRuleRefs(fileRef string, content string) ([]string, error) {
	doc, err := parseDocument(content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fileRef, err)
	}
	refs, found, err := parseRuleRefsFromFrontmatter(doc.lines[1:doc.endIndex])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fileRef, err)
	}
	if !found {
		return nil, fmt.Errorf("%s: frontmatter rule_refs is required", fileRef)
	}
	if HasBodyRuleRefs(strings.Join(doc.bodyLines, "\n")) {
		return nil, fmt.Errorf("%s: body rule_refs is forbidden; use frontmatter rule_refs", fileRef)
	}
	return refs, nil
}

func UpdateObjectRuleRefs(fileRef string, content string, refs []string) (string, error) {
	return RewriteObjectFrontmatter(fileRef, content, nil, refs)
}

func RewriteObjectFrontmatter(fileRef string, content string, scalars map[string]string, refs []string) (string, error) {
	doc, err := parseDocument(content)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fileRef, err)
	}
	normalized, err := NormalizeRuleRefs(refs)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fileRef, err)
	}

	fm := make([]string, 0, doc.endIndex+8)
	fm = append(fm, "---")
	writtenScalar := map[string]bool{}
	wroteRefs := false
	bodyStart := doc.endIndex + 1
	for i := 1; i < doc.endIndex; i++ {
		line := doc.lines[i]
		key, ok := frontmatterKey(line)
		if !ok {
			fm = append(fm, line)
			continue
		}
		if key == "rule_refs" {
			if !wroteRefs {
				fm = appendRuleRefs(fm, normalized)
				wroteRefs = true
			}
			i = skipFrontmatterBlock(doc.lines, i+1, doc.endIndex) - 1
			continue
		}
		if value, ok := scalars[key]; ok {
			fm = append(fm, key+": "+value)
			writtenScalar[key] = true
			continue
		}
		fm = append(fm, line)
	}
	missingScalarKeys := make([]string, 0, len(scalars))
	for key := range scalars {
		if !writtenScalar[key] {
			missingScalarKeys = append(missingScalarKeys, key)
		}
	}
	sort.Strings(missingScalarKeys)
	for _, key := range missingScalarKeys {
		fm = append(fm, key+": "+scalars[key])
	}
	if !wroteRefs {
		fm = appendRuleRefs(fm, normalized)
	}
	fm = append(fm, "---")

	body := removeBodyRuleRefs(strings.Join(doc.lines[bodyStart:], "\n"))
	out := strings.Join(fm, "\n")
	if strings.TrimSpace(body) != "" {
		out += "\n" + body
	}
	if strings.HasSuffix(content, "\n") {
		out += "\n"
	}
	return out, nil
}

func FrontmatterScalar(fileRef string, content string, key string) (string, error) {
	doc, err := parseDocument(content)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fileRef, err)
	}
	for i := 1; i < doc.endIndex; i++ {
		lineKey, ok := frontmatterKey(doc.lines[i])
		if !ok || lineKey != key {
			continue
		}
		parts := strings.SplitN(doc.lines[i], ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("%s: invalid frontmatter field %q", fileRef, key)
		}
		return strings.TrimSpace(parts[1]), nil
	}
	return "", fmt.Errorf("%s: frontmatter field %q is required", fileRef, key)
}

func NormalizeRuleRefs(refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(strings.Trim(ref, "`"))
		if ref == "" {
			return nil, fmt.Errorf("empty rule ref is not allowed")
		}
		if !refPattern.MatchString(ref) {
			return nil, fmt.Errorf("invalid rule ref %q", ref)
		}
		if seen[ref] {
			return nil, fmt.Errorf("rule_refs contains duplicate item %q", ref)
		}
		seen[ref] = true
		normalized = append(normalized, ref)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func validateRuleRefs(refs []string) ([]string, error) {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(strings.Trim(ref, "`"))
		if ref == "" {
			return nil, fmt.Errorf("rule_refs contains an empty item")
		}
		if !refPattern.MatchString(ref) {
			return nil, fmt.Errorf("invalid rule ref %q", ref)
		}
		if seen[ref] {
			return nil, fmt.Errorf("rule_refs contains duplicate item %q", ref)
		}
		if len(normalized) > 0 && normalized[len(normalized)-1] > ref {
			return nil, fmt.Errorf("rule_refs must be sorted in ascending lexical order")
		}
		seen[ref] = true
		normalized = append(normalized, ref)
	}
	return normalized, nil
}

func HasBodyRuleRefs(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		}
		if idx := strings.Index(trimmed, ". "); idx >= 0 {
			prefix := trimmed[:idx]
			if numeric(prefix) {
				trimmed = strings.TrimSpace(trimmed[idx+2:])
			}
		}
		key, ok := markdownFieldKey(trimmed)
		if ok && key == "rule_refs" {
			return true
		}
	}
	return false
}

func HasRuleBoundObjects(fileRef string, content string) (bool, error) {
	doc, err := parseDocument(content)
	if err != nil {
		return false, fmt.Errorf("%s: %w", fileRef, err)
	}
	for i := 1; i < doc.endIndex; i++ {
		key, ok := frontmatterKey(doc.lines[i])
		if ok && key == "bound_objects" {
			return true, nil
		}
	}
	return false, nil
}

func RemoveRuleBoundObjects(fileRef string, content string) (string, error) {
	doc, err := parseDocument(content)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fileRef, err)
	}
	lines := make([]string, 0, len(doc.lines))
	lines = append(lines, "---")
	for i := 1; i < doc.endIndex; i++ {
		key, ok := frontmatterKey(doc.lines[i])
		if ok && key == "bound_objects" {
			i = skipFrontmatterBlock(doc.lines, i+1, doc.endIndex) - 1
			continue
		}
		lines = append(lines, doc.lines[i])
	}
	lines = append(lines, doc.lines[doc.endIndex:]...)
	out := strings.Join(lines, "\n")
	if strings.HasSuffix(content, "\n") {
		out += "\n"
	}
	return out, nil
}

func parseDocument(content string) (document, error) {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return document{}, fmt.Errorf("frontmatter is required")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return document{}, fmt.Errorf("frontmatter closing marker is required")
	}
	return document{
		lines:     lines,
		endIndex:  end,
		bodyLines: lines[end+1:],
	}, nil
}

func parseRuleRefsFromFrontmatter(lines []string) ([]string, bool, error) {
	for i := 0; i < len(lines); i++ {
		key, ok := frontmatterKey(lines[i])
		if !ok || key != "rule_refs" {
			continue
		}
		parts := strings.SplitN(lines[i], ":", 2)
		value := ""
		if len(parts) == 2 {
			value = strings.TrimSpace(parts[1])
		}
		if value == "none" {
			return nil, true, nil
		}
		if value != "" {
			return nil, true, fmt.Errorf("rule_refs must be none or a YAML list")
		}
		refs := []string{}
		for j := i + 1; j < len(lines); j++ {
			if nextKey, ok := frontmatterKey(lines[j]); ok && nextKey != "" {
				break
			}
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(trimmed, "- ") {
				return nil, true, fmt.Errorf("rule_refs list item must start with '-'")
			}
			refs = append(refs, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
		normalized, err := validateRuleRefs(refs)
		if err != nil {
			return nil, true, err
		}
		if len(normalized) == 0 {
			return nil, true, fmt.Errorf("rule_refs must not be an empty list; use rule_refs: none")
		}
		return normalized, true, nil
	}
	return nil, false, nil
}

func appendRuleRefs(lines []string, refs []string) []string {
	if len(refs) == 0 {
		return append(lines, "rule_refs: none")
	}
	lines = append(lines, "rule_refs:")
	for _, ref := range refs {
		lines = append(lines, "  - "+ref)
	}
	return lines
}

func frontmatterKey(line string) (string, bool) {
	if strings.TrimSpace(line) == "" {
		return "", false
	}
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return "", false
	}
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", false
	}
	key := strings.TrimSpace(line[:idx])
	if key == "" {
		return "", false
	}
	return key, true
}

func skipFrontmatterBlock(lines []string, start int, end int) int {
	i := start
	for i < end {
		if key, ok := frontmatterKey(lines[i]); ok && key != "" {
			break
		}
		i++
	}
	return i
}

func removeBodyRuleRefs(body string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		fieldKey, hasField := bodyFieldKey(trimmed)
		if hasField && fieldKey == "rule_refs" {
			skip = true
			continue
		}
		if skip {
			if trimmed == "" || strings.HasPrefix(trimmed, "- ") {
				continue
			}
			if hasField || strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "# ") {
				skip = false
			} else {
				continue
			}
		}
		out = append(out, line)
	}
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	return strings.Join(out, "\n")
}

func bodyFieldKey(trimmed string) (string, bool) {
	if strings.HasPrefix(trimmed, "- ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
	}
	if idx := strings.Index(trimmed, ". "); idx >= 0 && numeric(trimmed[:idx]) {
		trimmed = strings.TrimSpace(trimmed[idx+2:])
	}
	return markdownFieldKey(trimmed)
}

func markdownFieldKey(trimmed string) (string, bool) {
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return "", false
	}
	key := strings.TrimSpace(strings.Trim(parts[0], "`"))
	if key == "" {
		return "", false
	}
	return key, true
}

// FindRuleConsumers searches all unit spec files under docs/specs/units/
// for rule_refs containing the given ruleID. Returns the list of unit names.
//
// Global rules (g_rule_*) apply to every current-layer unit per spec_writing_guide.md §5
// and are not listed in individual unit's rule_refs. For global rules, this function
// returns every unit with a candidate or stable file, regardless of their rule_refs.
// Bound rules (b_rule_*) resolve consumers on the current-layer (effective) files,
// candidate preferred with stable fallback (spec_writing_guide.md §6.3).
func FindRuleConsumers(repoRoot, ruleID string) ([]string, error) {
	var consumers []string
	seen := map[string]bool{}

	searchDirs := []string{
		filepath.Join(repoRoot, specpaths.CandidateDir),
		filepath.Join(repoRoot, specpaths.StableDir),
	}

	// Global rules apply to every current-layer unit by default
	if strings.HasPrefix(ruleID, "g_rule_") {
		// A global rule's default applicability exists only while the rule
		// file exists. A missing rule file (removed rule, or a mistyped ID)
		// cannot constrain anything, so the rule is reported as not found
		// instead of returning every unit.
		candidateRule := filepath.Join(repoRoot, specpaths.RuleCandidateFileRef(ruleID))
		stableRule := filepath.Join(repoRoot, specpaths.RuleStableFileRef(ruleID))
		if _, err := os.Stat(candidateRule); err != nil {
			if _, err := os.Stat(stableRule); err != nil {
				return nil, fmt.Errorf("rule %s not found in docs/specs/rules/ (candidate or stable)", ruleID)
			}
		}
		for _, dir := range searchDirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("read dir %s: %w", dir, err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if !strings.HasPrefix(entry.Name(), "unit_") {
					continue
				}
				if !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				unitName := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "unit_"), ".md")
				if !seen[unitName] {
					consumers = append(consumers, unitName)
					seen[unitName] = true
				}
			}
		}
		sort.Strings(consumers)
		return consumers, nil
	}

	return FindExplicitRuleConsumers(repoRoot, ruleID)
}

// FindExplicitRuleConsumers resolves every unit to its current-layer file
// (candidate preferred, stable fallback — the same resolution
// specflowctl deps uses, built on unitgraph) and scans the rule_refs of the
// effective file for the given ruleID, returning the unit names. A stable
// file whose candidate has dropped the reference no longer counts: the
// reference is semantically gone and only the stale stable file remains
// (see spec_writing_guide.md §6.3), exposed by the stable confirmation
// check instead of blocking removal. Unlike FindRuleConsumers, it uses the
// same explicit rule_refs semantics for every rule scope — a global rule's
// default applicability to all units is not treated as a reference here, so
// removal (which removes the rule file and thereby lifts the default
// applicability) can be validated against the references that must actually
// be cleaned up.
func FindExplicitRuleConsumers(repoRoot, ruleID string) ([]string, error) {
	graph, err := unitgraph.Build(repoRoot, "all")
	if err != nil {
		return nil, err
	}
	var consumers []string
	for _, node := range graph.Nodes() {
		for _, ref := range node.RuleRefs {
			if ref == ruleID {
				consumers = append(consumers, node.Name)
				break
			}
		}
	}
	sort.Strings(consumers)
	return consumers, nil
}

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

package testfixtures

import (
	"path/filepath"
	"regexp"
	"strings"
)

func NormalizeSpecFlowContent(path, content string) string {
	normalizedPath := filepath.ToSlash(path)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	switch {
	case strings.Contains(normalizedPath, "docs/specs/rules/") && strings.HasSuffix(normalizedPath, ".md"):
		return removeFrontmatterBlock(content, "bound_objects")
	case isMainSpecPath(normalizedPath):
		return normalizeMainSpec(content)
	default:
		return content
	}
}

func isMainSpecPath(path string) bool {
	if !strings.HasSuffix(path, ".md") {
		return false
	}
	base := filepath.Base(path)
	return strings.Contains(path, "docs/specs/units/") && strings.HasPrefix(base, "unit_")
}

func normalizeMainSpec(content string) string {
	start, end := frontmatterBounds(content)
	if start < 0 || end < 0 {
		return content
	}
	frontmatter := strings.Split(strings.TrimSuffix(content[start:end], "\n"), "\n")
	body := strings.TrimPrefix(content[end+4:], "\n")
	refs, foundInBody, explicitNone := extractBodyRuleRefs(body)
	hasFrontmatterRefs := frontmatterHasKey(frontmatter, "rule_refs")
	if !hasFrontmatterRefs {
		frontmatter = insertRuleRefs(frontmatter, refs, foundInBody, explicitNone)
	}
	body = removeBodyRuleRefs(body)
	return "---\n" + strings.Join(frontmatter, "\n") + "\n---\n" + body
}

func frontmatterBounds(content string) (int, int) {
	if !strings.HasPrefix(content, "---\n") {
		return -1, -1
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return -1, -1
	}
	return 4, 4 + end
}

func frontmatterHasKey(lines []string, key string) bool {
	for _, line := range lines {
		if frontmatterKey(line) == key {
			return true
		}
	}
	return false
}

func insertRuleRefs(lines []string, refs []string, found bool, explicitNone bool) []string {
	insert := renderRuleRefs(refs, found, explicitNone)
	out := make([]string, 0, len(lines)+len(insert))
	inserted := false
	for _, line := range lines {
		out = append(out, line)
		if !inserted && frontmatterKey(line) == "version" {
			out = append(out, insert...)
			inserted = true
		}
	}
	if !inserted {
		out = append(out, insert...)
	}
	return out
}

func renderRuleRefs(refs []string, found bool, explicitNone bool) []string {
	if !found || explicitNone {
		return []string{"rule_refs: none"}
	}
	lines := []string{"rule_refs:"}
	for _, ref := range refs {
		lines = append(lines, "  - "+ref)
	}
	return lines
}

func extractBodyRuleRefs(body string) ([]string, bool, bool) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		key, right := bodyField(line)
		if key != "rule_refs" {
			continue
		}
		if right == "none" || right == "`none`" {
			return nil, true, true
		}
		if right != "" {
			return []string{strings.Trim(right, "`")}, true, false
		}
		refs := []string{}
		for j := i + 1; j < len(lines); j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				continue
			}
			if strings.HasPrefix(next, "#") || regexp.MustCompile(`^\d+\.`).MatchString(next) {
				break
			}
			if strings.HasPrefix(next, "- ") {
				refs = append(refs, strings.Trim(strings.TrimSpace(strings.TrimPrefix(next, "- ")), "`"))
				continue
			}
			break
		}
		return refs, true, false
	}
	return nil, false, false
}

func removeBodyRuleRefs(body string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		key, _ := bodyField(line)
		if key == "rule_refs" {
			skip = true
			continue
		}
		if skip {
			if trimmed == "" || strings.HasPrefix(trimmed, "- ") {
				continue
			}
			if key != "" || strings.HasPrefix(trimmed, "#") {
				skip = false
			} else {
				continue
			}
		}
		out = append(out, line)
	}
	text := strings.Join(out, "\n")
	text = regexp.MustCompile(`(?m)^2\. (rule_reuse_summary:|`+"`rule_reuse_summary`"+`:)`).ReplaceAllString(text, `1. $1`)
	text = regexp.MustCompile(`(?m)^3\. (rule_exceptions:|`+"`rule_exceptions`"+`:)`).ReplaceAllString(text, `2. $1`)
	return text
}

func removeFrontmatterBlock(content, key string) string {
	start, end := frontmatterBounds(content)
	if start < 0 || end < 0 {
		return content
	}
	lines := strings.Split(strings.TrimSuffix(content[start:end], "\n"), "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		if skip {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.TrimSpace(line) == "" {
				continue
			}
			skip = false
		}
		if frontmatterKey(line) == key {
			skip = true
			continue
		}
		out = append(out, line)
	}
	return "---\n" + strings.Join(out, "\n") + "\n---" + content[end+4:]
}

func frontmatterKey(line string) string {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return ""
	}
	left, _, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(left)
}

func bodyField(line string) (string, string) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
	}
	if idx := strings.Index(trimmed, ". "); idx >= 0 {
		prefix := trimmed[:idx]
		if regexp.MustCompile(`^\d+$`).MatchString(prefix) {
			trimmed = strings.TrimSpace(trimmed[idx+2:])
		}
	}
	left, right, ok := strings.Cut(trimmed, ":")
	if !ok {
		return "", ""
	}
	return strings.Trim(strings.TrimSpace(left), "`"), strings.TrimSpace(right)
}

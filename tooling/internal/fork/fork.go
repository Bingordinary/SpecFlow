package fork

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fileops"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

type Result struct {
	Unit    string
	Passed  bool
	Issues  []string
	Actions []string
}

type RuleResult struct {
	RuleID     string
	Passed     bool
	Issues     []string
	Actions    []string
	StableRule string
}

func Fork(repoRoot, unitName string) *Result {
	r := &Result{Unit: unitName}

	stableSpec, _ := specpaths.ObjectMainSpecFileRef("unit", "stable", unitName)
	candidateSpec, _ := specpaths.ObjectMainSpecFileRef("unit", "candidate", unitName)
	stableSpecPath := filepath.Join(repoRoot, stableSpec)
	candidateSpecPath := filepath.Join(repoRoot, candidateSpec)

	if _, err := os.Stat(stableSpecPath); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Stable spec not found: %s", stableSpec))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found stable spec: %s", stableSpec))

	if _, err := os.Stat(candidateSpecPath); err == nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate already exists: %s — edit the existing candidate directly", candidateSpec))
		r.Passed = false
		return r
	}

	data, err := os.ReadFile(stableSpecPath)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Cannot read stable spec: %v", err))
		r.Passed = false
		return r
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))
	stableVersion := fm["version"]

	stableAppendixDir := filepath.Join(repoRoot, specpaths.StableAppendixDir)
	candidateAppendixDir := filepath.Join(repoRoot, specpaths.CandidateAppendixDir)
	appendixPattern := fmt.Sprintf("unit_%s_*.md", unitName)

	appendixMatches, _ := filepath.Glob(filepath.Join(stableAppendixDir, appendixPattern))

	var filesToCopy []struct {
		src string
		dst string
	}

	for _, m := range appendixMatches {
		rel, _ := filepath.Rel(repoRoot, m)

		appendixFM := specpaths.ReadFrontmatterStringMap(readFileString(m))
		if strings.EqualFold(appendixFM["status"], "exempt") {
			r.Actions = append(r.Actions, fmt.Sprintf("Skipped exempt appendix: %s", rel))
			continue
		}

		dst := filepath.Join(candidateAppendixDir, filepath.Base(m))
		filesToCopy = append(filesToCopy, struct{ src, dst string }{m, dst})
		r.Actions = append(r.Actions, fmt.Sprintf("Found appendix: %s", rel))
	}

	candidateVersion := fileops.VersionWithBumpPatch(stableVersion)

	copyErr := copyFileWithVersion(
		stableSpecPath, candidateSpecPath,
		"stable", "candidate",
		"version", candidateVersion,
	)
	if copyErr != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy spec: %v", copyErr))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Forked: %s -> %s (version %s -> %s)", stableSpec, candidateSpec, stableVersion, candidateVersion))

	for _, f := range filesToCopy {
		if err := fileops.CopyFileTransform(f.src, f.dst, "stable", "candidate"); err != nil {
			r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy appendix: %v", err))
			r.Passed = false
			return r
		}
		rel, _ := filepath.Rel(repoRoot, f.dst)
		r.Actions = append(r.Actions, fmt.Sprintf("Forked appendix: %s", rel))
	}

	r.Passed = true
	return r
}

func ForkRule(repoRoot, ruleID string) *RuleResult {
	r := &RuleResult{RuleID: ruleID}

	stableRule, _ := specpaths.ObjectMainSpecFileRef("rule", "stable", ruleID)
	candidateRule, _ := specpaths.ObjectMainSpecFileRef("rule", "candidate", ruleID)
	stableRulePath := filepath.Join(repoRoot, stableRule)
	candidateRulePath := filepath.Join(repoRoot, candidateRule)
	r.StableRule = stableRule

	if _, err := os.Stat(stableRulePath); os.IsNotExist(err) {
		r.Issues = append(r.Issues, fmt.Sprintf("Stable rule not found: %s", stableRule))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Found stable rule: %s", stableRule))

	if _, err := os.Stat(candidateRulePath); err == nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Candidate already exists: %s — edit the existing candidate directly", candidateRule))
		r.Passed = false
		return r
	}

	data, err := os.ReadFile(stableRulePath)
	if err != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Cannot read stable rule: %v", err))
		r.Passed = false
		return r
	}

	fm := specpaths.ReadFrontmatterStringMap(string(data))
	stableVersion := fm["rule_version"]
	candidateVersion := fileops.VersionWithBumpPatch(stableVersion)

	copyErr := copyFileWithVersion(
		stableRulePath, candidateRulePath,
		"stable", "candidate",
		"rule_version", candidateVersion,
	)
	if copyErr != nil {
		r.Issues = append(r.Issues, fmt.Sprintf("Failed to copy rule: %v", copyErr))
		r.Passed = false
		return r
	}
	r.Actions = append(r.Actions, fmt.Sprintf("Forked: %s -> %s (rule_version %s -> %s)", stableRule, candidateRule, stableVersion, candidateVersion))

	r.Passed = true
	return r
}

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
		buf.WriteString("Candidate spec created from stable. Edit the candidate to begin working.\n")
	} else {
		buf.WriteString("Fork failed. Fix the issues above and try again.\n")
	}
	return buf.String()
}

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
		buf.WriteString("Candidate rule created from stable. Edit the candidate to begin working.\n")
	} else {
		buf.WriteString("Fork failed. Fix the issues above and try again.\n")
	}
	return buf.String()
}

func copyFileWithVersion(src, dst, fromLayer, toLayer, versionField, newVersion string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	content := string(data)
	transformed := fileops.TransformLayerInFrontmatter(content, fromLayer, toLayer)

	lines := strings.Split(transformed, "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, versionField+":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				rawVal := parts[1]
				trimmedVal := strings.TrimSpace(rawVal)
				quote := ""
				if len(trimmedVal) >= 2 {
					if trimmedVal[0] == '"' && trimmedVal[len(trimmedVal)-1] == '"' {
						quote = "\""
					} else if trimmedVal[0] == '\'' && trimmedVal[len(trimmedVal)-1] == '\'' {
						quote = "'"
					}
				}
				lines[i] = leading + versionField + ": " + quote + newVersion + quote
				found = true
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("version field %q not found in frontmatter", versionField)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(strings.Join(lines, "\n")), 0644)
}

func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

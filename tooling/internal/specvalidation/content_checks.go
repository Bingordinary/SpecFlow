package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// This file implements the mechanical residue of unit_validate_checklist.md:
// Check 1 (prose path hygiene — WARNING; environment and deployment
// agnosticism; frontmatter-region purity) and the deterministic floor of the
// Spec content boundary (guide §15: reserved changelog headings, process
// traces, test doubles, verification_method shape). The agent session keeps
// the semantic judgment (marked-example confirmation, hard-requirement
// reading, internal-implementation residue, section order).

// ------------------------------------------------------------
// Prose path hygiene (Check 1, WARNING)
// ------------------------------------------------------------

var (
	sourcePathRe    = regexp.MustCompile(`[A-Za-z0-9_./-]+\.(?:go|ts|tsx|js|jsx|py|java|rs|cs|rb|php|kt|swift|c|h|cpp|hpp)\b`)
	structuredKeyRe = regexp.MustCompile(`^\s*(?:implementation_surface|verification_surface|affects(?:\.[a-z_]+)*|files|appendices|dependencies|runnable|not_runnable_reason)\s*:`)
)

// CheckProsePathHygiene reports (WARNING — advisory, never blocks) source-code
// file paths in narrative text. Structured fields (implementation_surface,
// affects.*) are intentional and excluded, as are framework governance paths
// and validation-cache paths, and fenced code-block examples.
func CheckProsePathHygiene(content string) CheckResult {
	hits := prosePathHits(content)
	if len(hits) == 0 {
		return CheckResult{Name: "Prose path hygiene", Status: Pass}
	}
	return CheckResult{
		Name:    "Prose path hygiene",
		Status:  Warn,
		Details: "source-code paths in narrative text — relocate to implementation_surface, affects.files, or affects.evidence_files, or reference by concept name: " + strings.Join(hits, "; "),
	}
}

func prosePathHits(content string) []string {
	var hits []string
	inFence := false
	inStructured := false
	structuredIndent := 0
	regionFrom, regionTo := acceptanceItemsBounds(content)
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		lineNo := i + 1
		if inFence || (regionFrom <= lineNo && lineNo <= regionTo) {
			continue
		}
		if structuredKeyRe.MatchString(line) {
			inStructured = true
			structuredIndent = indentOf(line)
			continue
		}
		if inStructured {
			if trimmed == "" || indentOf(line) > structuredIndent {
				continue
			}
			inStructured = false
		}
		for _, path := range sourcePathRe.FindAllString(line, -1) {
			if strings.HasPrefix(path, "framework/") || strings.HasPrefix(path, "docs/specs/meta/") {
				continue
			}
			hits = append(hits, fmt.Sprintf("line %d: %s", lineNo, path))
		}
	}
	return hits
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// acceptanceItemsBounds returns the 1-based inclusive line bounds of the
// acceptance_item_set structural region, or (0, 0) when the spec declares none.
func acceptanceItemsBounds(content string) (from, to int) {
	region, ok := contenthash.AcceptanceItemsRegion(content)
	if !ok {
		return 0, 0
	}
	start := -1
	for i, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "acceptance_item_set:" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return 0, 0
	}
	return start, start + strings.Count(region, "\n")
}

// ------------------------------------------------------------
// Environment and deployment agnosticism (Check 1)
// ------------------------------------------------------------

var (
	devAbsPathRe = regexp.MustCompile(`(?:^|[\s("'` + "`" + `\[])((?:/Users/|/home/|[A-Za-z]:\\)[^\s)"'` + "`" + `\]]*)`)
	localAddrRe  = regexp.MustCompile(`(?:localhost|127\.0\.0\.1):\d{2,5}`)
	secretRes    = []*regexp.Regexp{
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	}
	fenceMarkerRe = regexp.MustCompile(`(?i)\b(example|placeholder|e\.g\.|for instance)\b`)
)

// CheckEnvironmentAgnosticism enforces the environment-agnostic spec law:
// no developer-machine absolute paths, fixed local addresses, or credentials.
// Narrative hits FAIL (unambiguous). Fenced-block hits are WARNING for the
// agent session to confirm the example marking; credential patterns FAIL
// everywhere.
func CheckEnvironmentAgnosticism(content string) CheckResult {
	fails, warns := environmentHits(content)
	if len(fails) > 0 {
		return CheckResult{
			Name:    "Environment agnosticism",
			Status:  Fail,
			Details: "environment-specific content in the spec (use project-relative paths, abstract placeholders, or configuration-driven parameters): " + strings.Join(fails, "; "),
		}
	}
	if len(warns) > 0 {
		return CheckResult{
			Name:    "Environment agnosticism",
			Status:  Warn,
			Details: "environment-specific content inside fenced blocks — confirm each is an explicitly marked example placeholder: " + strings.Join(warns, "; "),
		}
	}
	return CheckResult{Name: "Environment agnosticism", Status: Pass}
}

func environmentHits(content string) (fails, warns []string) {
	inFence := false
	fenceMarker := false
	prevNarrativeMarker := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				inFence = true
				// The marker may sit on the opening line or in the narrative
				// line introducing the fence.
				fenceMarker = fenceMarkerRe.MatchString(trimmed) || prevNarrativeMarker
			} else {
				inFence = false
				prevNarrativeMarker = false
			}
			continue
		}
		if inFence {
			fenceMarker = fenceMarker || fenceMarkerRe.MatchString(trimmed)
		} else if trimmed != "" {
			// Blank lines do not break marker adjacency.
			prevNarrativeMarker = fenceMarkerRe.MatchString(trimmed)
		}
		lineNo := i + 1
		for _, re := range secretRes {
			if hit := re.FindString(line); hit != "" {
				fails = append(fails, fmt.Sprintf("line %d: credential pattern %q", lineNo, hit))
			}
		}
		var envHits []string
		envHits = append(envHits, devAbsPathRe.FindAllString(line, -1)...)
		envHits = append(envHits, localAddrRe.FindAllString(line, -1)...)
		if len(envHits) == 0 {
			continue
		}
		joined := strings.Join(envHits, ", ")
		if inFence {
			if fenceMarker {
				continue // an explicitly marked example placeholder
			}
			warns = append(warns, fmt.Sprintf("line %d: %s", lineNo, joined))
			continue
		}
		fails = append(fails, fmt.Sprintf("line %d: %s", lineNo, joined))
	}
	return fails, warns
}

// ------------------------------------------------------------
// Frontmatter region purity (Check 1)
// ------------------------------------------------------------

// CheckFrontmatterRegionPurity verifies that the region before the first ##
// heading holds only the YAML block, the # title, and blank lines — stray
// prose there belongs to no section region and fails closed region
// declarations.
func CheckFrontmatterRegionPurity(content string) CheckResult {
	regions := contenthash.SectionRegions(content)
	if len(regions) == 0 || regions[0].Heading != "" {
		return CheckResult{Name: "Frontmatter region purity", Status: Pass}
	}
	front := regions[0]
	inYAML := false
	for i, line := range strings.Split(front.Text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "---") {
			inYAML = !inYAML
			continue
		}
		if inYAML || trimmed == "" || strings.HasPrefix(trimmed, "# ") {
			continue
		}
		return CheckResult{
			Name:    "Frontmatter region purity",
			Status:  Fail,
			Details: fmt.Sprintf("stray content before the first ## heading (line %d): %q — the frontmatter region holds only the YAML block, the # title, and blank lines; move the content into a section", front.Start+i, trimmed),
		}
	}
	return CheckResult{Name: "Frontmatter region purity", Status: Pass}
}

// ------------------------------------------------------------
// Spec content boundary (Check 2 Step 5, Check 5e; guide §15)
// ------------------------------------------------------------

// The scans below are the deterministic floor of the Spec content boundary
// (framework/spec_writing_guide.md §15): reserved changelog/revision headings
// FAIL outright; process traces and test-double names are WARNING for the
// owning session to confirm. The semantic residue — whether a passage states
// internal implementation rather than observable behavior, and what follows
// the acceptance item set (§13.9) — is the agent judgment of Check 2 Step 5,
// Check 5e, and Check 1.

var (
	changelogHeadingRe   = regexp.MustCompile(`(?i)^(version notes?|changelog|change log|revision history)$`)
	changelogHeadingZh   = []string{"变更记录", "修订记录", "修订历史", "更新记录", "变更历史"}
	headingEnumeratorRe  = regexp.MustCompile(`^\s*\d+\s*[.)、]\s*`)
	processTraceRe       = regexp.MustCompile(`修复轮|门禁修复|已移除|不再有以下设计|此前|Previous round|revalidate@|reverify@`)
	testDoubleRe         = regexp.MustCompile(`(?i)\b(stub|mock|fake)(?:[\s_]+[A-Za-z\p{Han}]|[A-Z])`)
	verificationMethodRe = regexp.MustCompile(`^\s*verification_method:\s*(.*)$`)
	stepEnumRe           = regexp.MustCompile(`(\(\d+\)|（\d+）)`)
)

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// isReservedContentHeading reports whether a section heading names a
// changelog/revision section. A leading section number ("12. ", "3) ", "4、")
// is stripped first, so a numbered heading ("## 5. Changelog") matches the
// reserved name exactly as its unnumbered form does — the unit's own spec style
// numbers `##` headings. The English names match the whole normalized heading;
// the Chinese names match as substrings, since they read as content words rather
// than an exact label.
func isReservedContentHeading(heading string) bool {
	h := headingEnumeratorRe.ReplaceAllString(heading, "")
	return changelogHeadingRe.MatchString(h) || containsAny(h, changelogHeadingZh)
}

// CheckReservedContentHeadings rejects changelog/revision sections: a spec
// states current design only (guide §15.1), so change history is not a section.
func CheckReservedContentHeadings(content string) CheckResult {
	for _, region := range contenthash.SectionRegions(content) {
		if region.Heading == "" {
			continue
		}
		if isReservedContentHeading(region.Heading) {
			return CheckResult{
				Name:    "Reserved content headings",
				Status:  Fail,
				Details: fmt.Sprintf("line %d: changelog/revision section %q — a spec states current design only; change history belongs to version control (guide §15.1)", region.Start, region.Heading),
			}
		}
	}
	return CheckResult{Name: "Reserved content headings", Status: Pass}
}

// CheckProcessTraces flags change-history and process-trace narration
// (guide §15.1). WARNING: the owning session confirms each hit.
func CheckProcessTraces(content string) CheckResult {
	hits := scanNonFenced(content, processTraceRe)
	if len(hits) == 0 {
		return CheckResult{Name: "Process traces", Status: Pass}
	}
	return CheckResult{
		Name:    "Process traces",
		Status:  Warn,
		Details: "change history / process traces in the spec — a spec states current design only (guide §15.1); delete the history, version control owns it: " + strings.Join(hits, "; "),
	}
}

// CheckTestDoubles flags test-double names leaked into the spec (guide §15.3,
// §14.2 Anti-Pattern C). WARNING: the owning session confirms each hit.
func CheckTestDoubles(content string) CheckResult {
	hits := scanNonFenced(content, testDoubleRe)
	if len(hits) == 0 {
		return CheckResult{Name: "Test doubles", Status: Pass}
	}
	return CheckResult{
		Name:    "Test doubles",
		Status:  Warn,
		Details: "test-double names in the spec — they belong in the test code (guide §15.3, §14.2C); restate using domain roles: " + strings.Join(hits, "; "),
	}
}

// CheckVerificationMethodShape enforces the one-sentence method rule
// (guide §15.3): a step-by-step test plan FAILs. Fenced code blocks are
// content examples (guide §13.6) and are skipped, like every other scan in
// this file — only real item-set fields are judged.
func CheckVerificationMethodShape(content string) CheckResult {
	var fails []string
	lines := strings.Split(content, "\n")
	inFence := false
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := verificationMethodRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		lineNo := i + 1
		value := strings.TrimSpace(m[1])
		if value == "" || strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">") {
			indent := indentOf(lines[i])
			var block []string
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "" {
					block = append(block, "")
					continue
				}
				if indentOf(lines[j]) <= indent {
					break
				}
				block = append(block, strings.TrimSpace(lines[j]))
			}
			value = strings.TrimSpace(strings.Join(block, " "))
		}
		if len(stepEnumRe.FindAllString(value, -1)) >= 2 {
			fails = append(fails, fmt.Sprintf("line %d: step-by-step method", lineNo))
		}
	}
	if len(fails) > 0 {
		return CheckResult{
			Name:    "verification_method shape",
			Status:  Fail,
			Details: "verification_method states the method in one sentence (guide §15.3); a step-by-step test plan belongs in the test code: " + strings.Join(fails, "; "),
		}
	}
	return CheckResult{Name: "verification_method shape", Status: Pass}
}

// scanNonFenced reports non-fenced lines matching re, with line numbers.
func scanNonFenced(content string, re *regexp.Regexp) []string {
	var hits []string
	inFence := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if matches := re.FindAllString(line, -1); len(matches) > 0 {
			hits = append(hits, fmt.Sprintf("line %d: %s", i+1, strings.Join(matches, ", ")))
		}
	}
	return hits
}

// ------------------------------------------------------------
// Wiring: scan the main spec and every non-exempt appendix
// ------------------------------------------------------------

func checkContentScan(repoRoot, unitName, name string, scan func(content string) CheckResult) CheckResult {
	path := specPath(repoRoot, unitName)
	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: fmt.Sprintf("cannot read candidate spec: %v", err)}
	}
	result := scan(string(data))
	if result.Status == Fail {
		return result
	}
	var warns []string
	if result.Status == Warn {
		warns = append(warns, result.Details)
	}
	appendices, err := specpaths.UnitAppendices(repoRoot, unitName, "candidate")
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: err.Error()}
	}
	for _, appendix := range appendices {
		if appendix.Status == "exempt" {
			continue
		}
		appendixData, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(appendix.Path)))
		if err != nil {
			return CheckResult{Name: name, Status: Fail, Details: err.Error()}
		}
		appendixResult := scan(string(appendixData))
		if appendixResult.Status == Fail {
			return appendixResult
		}
		if appendixResult.Status == Warn {
			warns = append(warns, fmt.Sprintf("%s: %s", appendix.Path, appendixResult.Details))
		}
	}
	if len(warns) > 0 {
		return CheckResult{Name: name, Status: Warn, Details: strings.Join(warns, "; ")}
	}
	return CheckResult{Name: name, Status: Pass}
}

func checkProseHygiene(repoRoot, unitName string) CheckResult {
	return checkContentScan(repoRoot, unitName, "Prose path hygiene", CheckProsePathHygiene)
}

func checkEnvironmentAgnosticism(repoRoot, unitName string) CheckResult {
	return checkContentScan(repoRoot, unitName, "Environment agnosticism", CheckEnvironmentAgnosticism)
}

// checkMainSpecScan runs a scan against the unit's main spec only. The section
// structure rules of guide §13 apply to the unit's own main spec; appendix
// files are contract files and carry no section structure.
func checkMainSpecScan(repoRoot, unitName, name string, scan func(content string) CheckResult) CheckResult {
	path := specPath(repoRoot, unitName)
	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: fmt.Sprintf("cannot read candidate spec: %v", err)}
	}
	return scan(string(data))
}

func checkReservedContentHeadings(repoRoot, unitName string) CheckResult {
	return checkMainSpecScan(repoRoot, unitName, "Reserved content headings", CheckReservedContentHeadings)
}

func checkVerificationMethodShape(repoRoot, unitName string) CheckResult {
	return checkMainSpecScan(repoRoot, unitName, "verification_method shape", CheckVerificationMethodShape)
}

func checkProcessTraces(repoRoot, unitName string) CheckResult {
	return checkContentScan(repoRoot, unitName, "Process traces", CheckProcessTraces)
}

func checkTestDoubles(repoRoot, unitName string) CheckResult {
	return checkContentScan(repoRoot, unitName, "Test doubles", CheckTestDoubles)
}

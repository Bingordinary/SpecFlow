package install

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// The bootstrap routing contract (framework/hooks.md §Bootstrap Contract): the
// injected concepts.md carries a trigger routing table whose rows name the
// command package files for each trigger. These tests assert the closure that
// makes the bootstrap sufficient as an entry control point.

var bootstrapRoutingSectionStart = "## Trigger Routing"

var bootstrapFirstCellTokenPattern = regexp.MustCompile("`([^`]+)`")

var bootstrapFrameworkPathPattern = regexp.MustCompile("`(framework/[a-z_/]+\\.md)`")

var bootstrapMarkdownTokenPattern = regexp.MustCompile("`([^`]+\\.md)`")

var bootstrapGoldenTriggers = []string{
	"validate@{target}",
	"validate@{target}:check-{n}",
	"validate@{target}:{keyword}",
	"verify@{unit}",
	"verify@{unit}:{keyword}",
	"verify@{rule}",
	"review@{unit}",
	"review@{unit}:{keyword}",
	"revalidate@{target}",
	"reverify@{unit}",
	"rereview@{unit}",
	"promote@{target}",
	"fresh@{target}",
	"fresh@candidate",
	"fresh@stable",
	"fresh@all",
	"detect@{rule}",
	"detect@all",
	"remove@{rule}",
	"deps@all",
	"deps@{unit}",
	"deps@{rule}",
	"spec_flow_update",
	"spec_flow_version",
}

// The bootstrap payload budget (framework/hooks.md §Bootstrap Contract): the
// injected payload must stay under the tightest supported platform hook-output
// cap. Claude Code caps hook additionalContext at 10,000 characters with no
// override; Codex spills additionalContext above 2,500 tokens by default
// (approx_token_count = ceil(bytes/4)). The 9,000-character budget carries a
// 10% margin under both.

const bootstrapPayloadCharacterBudget = 9000

const codexDefaultContextTokenLimit = 2500

const bootstrapInjectionPreamble = "<SPECFLOW_CONCEPTS>\nThis project uses SpecFlow to manage design documents.\n\n**Below is the SpecFlow session bootstrap — read it before starting work. It states the operating rules and routes each supported trigger to its command package, which you read on demand:**\n\n"

func TestBootstrapPayloadFitsPlatformHookCaps(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	content, err := os.ReadFile(filepath.Join(repoRoot, "framework", "concepts.md"))
	if err != nil {
		t.Fatalf("read framework/concepts.md: %v", err)
	}
	payload := bootstrapInjectionPreamble + string(content) + "\n</SPECFLOW_CONCEPTS>"

	if chars := utf8.RuneCountInString(payload); chars > bootstrapPayloadCharacterBudget {
		t.Fatalf("bootstrap payload is %d characters, over the %d-character budget (Claude Code caps hook additionalContext at 10,000 characters)", chars, bootstrapPayloadCharacterBudget)
	}
	if tokens := (len(payload) + 3) / 4; tokens > codexDefaultContextTokenLimit {
		t.Fatalf("bootstrap payload estimates %d tokens (ceil(bytes/4)), over the Codex default %d-token spill threshold", tokens, codexDefaultContextTokenLimit)
	}

	templateBytes, err := os.ReadFile(filepath.Join(repoRoot, "templates", ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("read templates/.codex/hooks.json: %v", err)
	}
	if strings.Contains(string(templateBytes), "additionalContextLimit") {
		t.Fatalf("Codex hook must use the platform's default context limit: %s", templateBytes)
	}
}

func TestBootstrapInjectionPreambleMatchesHook(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	hook, err := os.ReadFile(filepath.Join(repoRoot, "hooks", "session-start"))
	if err != nil {
		t.Fatalf("read hooks/session-start: %v", err)
	}
	expectedAssignment := `session_context="` + strings.ReplaceAll(bootstrapInjectionPreamble, "\n", `\n`) + `${concepts_escaped}\n</SPECFLOW_CONCEPTS>"`
	if !strings.Contains(string(hook), expectedAssignment) {
		t.Fatalf("hooks/session-start preamble drifted from the payload measured by this test")
	}
}

// TestBootstrapContractBudgetAlignment keeps the Bootstrap Contract numbers in
// framework/hooks.md and the constants this package enforces in sync. The
// patterns deliberately parse the contract's size-budget sentence: if that
// sentence is reworded, update the pattern and the wording together.

var (
	bootstrapContractBudgetPattern = regexp.MustCompile("must stay within ([0-9][0-9,]*) characters")
	bootstrapContractMarginPattern = regexp.MustCompile("is a ([0-9]+)% margin")
	claudeContextCapPattern        = regexp.MustCompile("caps hook `additionalContext` at ([0-9][0-9,]*) characters")
	codexContextLimitPattern       = regexp.MustCompile("spills `additionalContext` above ([0-9][0-9,]*) tokens by default")
)

func TestBootstrapContractBudgetAlignment(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	content, err := os.ReadFile(filepath.Join(repoRoot, "framework", "hooks.md"))
	if err != nil {
		t.Fatalf("read framework/hooks.md: %v", err)
	}
	contract := string(content)

	budget := parseContractNumber(t, bootstrapContractBudgetPattern, contract, "bootstrap payload budget")
	marginPercent := parseContractNumber(t, bootstrapContractMarginPattern, contract, "budget margin percentage")
	claudeCap := parseContractNumber(t, claudeContextCapPattern, contract, "Claude Code hook-output cap")
	codexLimit := parseContractNumber(t, codexContextLimitPattern, contract, "Codex additionalContext token limit")

	if budget != bootstrapPayloadCharacterBudget {
		t.Fatalf("Bootstrap Contract budget is %d characters but the enforcement constant is %d; keep framework/hooks.md and this test in sync", budget, bootstrapPayloadCharacterBudget)
	}
	if codexLimit != codexDefaultContextTokenLimit {
		t.Fatalf("Bootstrap Contract Codex limit is %d tokens but the enforcement constant is %d; keep framework/hooks.md and this test in sync", codexLimit, codexDefaultContextTokenLimit)
	}
	if budget*100 > claudeCap*(100-marginPercent) {
		t.Fatalf("Bootstrap Contract budget %d characters does not keep the declared %d%% margin under the Claude Code cap of %d characters", budget, marginPercent, claudeCap)
	}
}

func parseContractNumber(t *testing.T, pattern *regexp.Regexp, content, what string) int {
	t.Helper()
	match := pattern.FindStringSubmatch(content)
	if match == nil {
		t.Fatalf("framework/hooks.md is missing the %s wording this test parses; update the contract sentence or this test's pattern", what)
	}
	value, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil {
		t.Fatalf("parse %s value %q: %v", what, match[1], err)
	}
	return value
}

func TestBootstrapRoutingClosure(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	conceptsPath := filepath.Join(repoRoot, "framework", "concepts.md")
	content, err := os.ReadFile(conceptsPath)
	if err != nil {
		t.Fatalf("read framework/concepts.md: %v", err)
	}

	section := bootstrapRoutingSection(t, string(content))
	rows := bootstrapRoutingRows(section)
	if len(rows) < 15 {
		t.Fatalf("expected the routing table to cover the supported triggers, got %d rows:\n%s", len(rows), section)
	}

	seen := map[string]bool{}
	for _, row := range rows {
		cells := strings.Split(row, "|")
		if len(cells) < 4 {
			t.Fatalf("routing row is missing columns: %s", row)
		}
		firstCell := cells[1]
		for _, match := range bootstrapFirstCellTokenPattern.FindAllStringSubmatch(firstCell, -1) {
			seen[match[1]] = true
		}

		paths := bootstrapFrameworkPathPattern.FindAllStringSubmatch(row, -1)
		if len(paths) == 0 {
			t.Fatalf("routing row names no command package file: %s", row)
		}
		for _, match := range bootstrapMarkdownTokenPattern.FindAllStringSubmatch(row, -1) {
			if !strings.HasPrefix(match[1], "framework/") {
				t.Fatalf("routing row uses an unqualified markdown package path %q: %s", match[1], row)
			}
		}
		for _, match := range paths {
			relPath := match[1]
			info, statErr := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relPath)))
			if statErr != nil {
				t.Fatalf("routing row package file %s does not exist: %v", relPath, statErr)
			}
			if info.Size() == 0 {
				t.Fatalf("routing row package file %s is empty", relPath)
			}
		}
	}

	for _, trigger := range bootstrapGoldenTriggers {
		if !seen[trigger] {
			t.Fatalf("supported trigger %s is missing from the bootstrap routing table:\n%s", trigger, section)
		}
	}
}

func TestBootstrapRoutingSemantics(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	content, err := os.ReadFile(filepath.Join(repoRoot, "framework", "concepts.md"))
	if err != nil {
		t.Fatalf("read framework/concepts.md: %v", err)
	}
	text := string(content)
	section := bootstrapRoutingSection(t, text)

	requireText(t, text, "- unit: **fork/create → edit → validate → verify → review → promote**")
	requireText(t, text, "- rule: **fork/create → edit → validate → promote**")
	requireText(t, text, "- retiring unit: **edit retirement → validate → promote**")
	requireText(t, text, "Read-only requests remain read-only")
	requireText(t, text, "`remove@{rule}` deletion and exact `spec_flow_update` migration")

	assertRoute := func(trigger string, required, forbidden []string) {
		t.Helper()
		row := bootstrapRouteForTrigger(t, section, trigger)
		for _, value := range required {
			requireText(t, row, value)
		}
		for _, value := range forbidden {
			if strings.Contains(row, value) {
				t.Fatalf("route %s must not contain %q: %s", trigger, value, row)
			}
		}
	}

	assertRoute("validate@{target}", []string{"framework/commands.md", "framework/verification_scope.md", "framework/validation_cache.md", "framework/unit_validate_checklist.md", "framework/rule_validate_checklist.md", "plan full run"}, nil)
	assertRoute("validate@{target}:check-{n}", []string{"framework/commands.md", "framework/verification_scope.md", "framework/unit_validate_checklist.md", "framework/rule_validate_checklist.md", "targeted check directly"}, []string{"framework/validation_cache.md", "gate-plan"})
	assertRoute("verify@{unit}", []string{"framework/verification_scope.md", "framework/validation_cache.md", "framework/unit_verify_checklist.md", "plan full run"}, nil)
	assertRoute("verify@{unit}:{keyword}", []string{"framework/verification_scope.md", "framework/unit_verify_checklist.md", "targeted check directly"}, []string{"framework/validation_cache.md", "gate-plan"})
	assertRoute("verify@{rule}", []string{"rule verify was removed", "validate@{rule}", "framework/verification_scope.md"}, nil)
	assertRoute("review@{unit}", []string{"framework/verification_scope.md", "framework/validation_cache.md", "framework/spec_review_checklist.md", "plan full run"}, nil)
	assertRoute("review@{unit}:{keyword}", []string{"framework/verification_scope.md", "framework/spec_review_checklist.md", "targeted file review directly"}, []string{"framework/validation_cache.md", "gate-plan"})
	assertRoute("revalidate@{target}", []string{"framework/commands.md", "framework/verification_scope.md", "framework/validation_cache.md", "framework/unit_validate_checklist.md", "framework/rule_validate_checklist.md", "plan delta/repair"}, nil)
	assertRoute("promote@{target}", []string{"framework/commands.md", "framework/unit_promote_workflow.md", "framework/rule_promote_workflow.md", "applicable gates only"}, nil)
	assertRoute("fresh@{target}", []string{"For `{target}`, resolve via `framework/commands.md`", "`specflowctl fresh`", "framework/validation_cache.md"}, nil)

	for _, metaTrigger := range []string{"`spec_flow_review`", "`spec_flow_review:full`", "`spec_flow_design_review`"} {
		if strings.Contains(section, metaTrigger) {
			t.Fatalf("meta-governance trigger %s belongs to project entry instructions, not bootstrap routing", metaTrigger)
		}
	}
}

func TestFreshTargetResolutionContract(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	content, err := os.ReadFile(filepath.Join(repoRoot, "framework", "commands.md"))
	if err != nil {
		t.Fatalf("read framework/commands.md: %v", err)
	}
	text := string(content)

	for _, required := range []string{
		"### Existing-Target Resolution for `fresh@{target}`",
		"docs/specs/units/{candidate,stable}/unit_{target}.md",
		"docs/specs/rules/{candidate,stable}/{target}.md",
		"docs/specs/rules/{candidate,stable}/g_rule_{target}.md",
		"docs/specs/rules/{candidate,stable}/b_rule_{target}.md",
		"specflowctl fresh --unit {target}",
		"specflowctl fresh --rule {rule_id}",
		"Ambiguous. List every matching path and ask the user to choose.",
		"Report that the target does not exist.",
		"Never run bare `specflowctl fresh` for `fresh@{target}`",
	} {
		requireText(t, text, required)
	}
}

// TestBootstrapContractRequiredContent guards the Bootstrap Contract's
// identity and key-terms categories (framework/hooks.md §Bootstrap Contract
// items 1-2): these normative statements stay in the injected bootstrap, and
// the payload budget counts them.
func TestBootstrapContractRequiredContent(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	content, err := os.ReadFile(filepath.Join(repoRoot, "framework", "concepts.md"))
	if err != nil {
		t.Fatalf("read framework/concepts.md: %v", err)
	}
	text := string(content)
	requireText(t, text, "consensus protocol between the user and the agent")
	requireText(t, text, "independently governed engineering responsibility")
	requireText(t, text, "reusable shared constraints")
}

func bootstrapRouteForTrigger(t *testing.T, section, trigger string) string {
	t.Helper()
	for _, row := range bootstrapRoutingRows(section) {
		cells := strings.Split(row, "|")
		if len(cells) < 4 {
			continue
		}
		for _, match := range bootstrapFirstCellTokenPattern.FindAllStringSubmatch(cells[1], -1) {
			if match[1] == trigger {
				return row
			}
		}
	}
	t.Fatalf("routing row for %s not found", trigger)
	return ""
}

func requireText(t *testing.T, content, required string) {
	t.Helper()
	if !strings.Contains(content, required) {
		t.Fatalf("required text %q is missing", required)
	}
}

func TestBootstrapRoutesEveryTriggerToAPackage(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	content, err := os.ReadFile(filepath.Join(repoRoot, "framework", "concepts.md"))
	if err != nil {
		t.Fatalf("read framework/concepts.md: %v", err)
	}
	for _, row := range bootstrapRoutingRows(bootstrapRoutingSection(t, string(content))) {
		cells := strings.Split(row, "|")
		if len(cells) < 4 {
			t.Fatalf("routing row is missing columns: %s", row)
		}
		if !strings.Contains(row, "`framework/") {
			t.Fatalf("routing row does not name a framework command package: %s", row)
		}
	}
}

func bootstrapRoutingSection(t *testing.T, content string) string {
	t.Helper()
	start := strings.Index(content, bootstrapRoutingSectionStart)
	if start < 0 {
		t.Fatalf("framework/concepts.md is missing the %q section", bootstrapRoutingSectionStart)
	}
	rest := content[start:]
	afterHeading := rest[len(bootstrapRoutingSectionStart):]
	if end := strings.Index(afterHeading, "\n## "); end >= 0 {
		return rest[:len(bootstrapRoutingSectionStart)+end]
	}
	return rest
}

func bootstrapRoutingRows(section string) []string {
	var rows []string
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		if strings.Contains(trimmed, "---") {
			continue
		}
		if strings.Contains(trimmed, "Trigger") && strings.Contains(trimmed, "First action") {
			continue
		}
		rows = append(rows, trimmed)
	}
	return rows
}

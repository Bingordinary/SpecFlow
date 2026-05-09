package reviewscope

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/projectstandards"
)

type SpecFlowScope struct {
	Scenario                          string
	FrameworkGuidelineFiles           []string
	CommandFiles                      []string
	GuidanceSkillFiles                []string
	RuleGovernanceFiles               []string
	TemplateGovernanceFiles           []string
	TemplateEntryFiles                []string
	ProjectEntryFiles                 []string
	AgentOperabilityFiles             []string
	ProjectInstanceCompatibilityFiles []string
	ProjectRegistryFiles              []string
	RegistryDiagnostics               []string
	ToolingContractFiles              []string
	ToolingSourceFiles                []string
	ToolingScriptFiles                []string
	ToolingRuntimeFiles               []string
	ActiveProjectStandardFiles        []string
}

func CollectDefaultSpecFlowScope(repoRoot string) (SpecFlowScope, error) {
	scope := SpecFlowScope{
		Scenario: "default_governance_baseline",
	}

	frameworkFiles, err := globRelative(repoRoot, "specflow/framework/*.md")
	if err != nil {
		return scope, err
	}
	commandFiles, err := globRelative(repoRoot, "specflow/framework/commands/*.md")
	if err != nil {
		return scope, err
	}
	guidanceSkillFiles, err := globRelative(repoRoot, "specflow/framework/skills/*/SKILL.md")
	if err != nil {
		return scope, err
	}
	if len(frameworkFiles) == 0 || len(commandFiles) == 0 || len(guidanceSkillFiles) == 0 {
		return scope, fmt.Errorf("default governance files are incomplete")
	}

	minimumGuidanceSkillFiles := []string{
		"specflow/framework/skills/using-specflow-guidance/SKILL.md",
		"specflow/framework/skills/project-framing/SKILL.md",
		"specflow/framework/skills/scope-cutting/SKILL.md",
		"specflow/framework/skills/solution-design/SKILL.md",
		"specflow/framework/skills/design-quality-review/SKILL.md",
		"specflow/framework/skills/spec-writeback-guidance/SKILL.md",
	}
	ruleFiles := []string{
		"specflow/framework/natural_language_routing.md",
		"specflow/framework/rule_new.md",
		"specflow/framework/rule_extract.md",
		"specflow/framework/rule_bind.md",
		"specflow/framework/rule_topology.md",
		"specflow/framework/rule_sync.md",
		"specflow/framework/rule_escape.md",
	}
	templateProcessStateFiles := []string{
		"specflow/templates/docs/specs/_status.md",
		"specflow/templates/docs/specs/_check_result/README.md",
		"specflow/templates/docs/specs/_plans/README.md",
		"specflow/templates/docs/specs/_plans/draft/README.md",
		"specflow/templates/docs/specs/_plans/active/README.md",
		"specflow/templates/docs/specs/_verify_result/README.md",
		"specflow/templates/docs/specs/_governance_review/README.md",
	}
	templateGovernanceFiles := append([]string{}, templateProcessStateFiles...)
	templateGovernanceFiles = append(templateGovernanceFiles,
		"specflow/templates/docs/project_standards/_registry.md",
	)
	templateEntryFiles := []string{
		"specflow/templates/AGENTS.md",
		"specflow/templates/GEMINI.md",
		"specflow/templates/CLAUDE.md",
	}
	projectEntryFiles := []string{
		"AGENTS.md",
		"GEMINI.md",
		"CLAUDE.md",
	}
	projectRegistryFiles := []string{
		"docs/project_standards/_registry.md",
	}
	toolingContractFiles := []string{
		"specflow/framework/tooling_execution_policy.md",
		"specflow/tooling/README.md",
	}
	processStateContractFiles := []string{
		"specflow/framework/candidate_handoff_contract.md",
		"specflow/framework/downgrade_policy.md",
		"specflow/framework/process_snapshot_contract.md",
		"specflow/framework/recovery_policy.md",
	}
	agentOperabilityFiles := collectAgentOperabilityFiles(projectEntryFiles, templateEntryFiles, templateProcessStateFiles, commandFiles, guidanceSkillFiles, ruleFiles, processStateContractFiles, toolingContractFiles)
	projectInstanceCompatibilityFiles, err := collectProjectInstanceCompatibilityFiles(repoRoot)
	if err != nil {
		return scope, err
	}

	toolingCmdFiles, err := walkRelativeFiles(repoRoot, "specflow/tooling/cmd", ".go")
	if err != nil {
		return scope, err
	}
	toolingInternalFiles, err := walkRelativeFiles(repoRoot, "specflow/tooling/internal", ".go")
	if err != nil {
		return scope, err
	}
	toolingSourceFiles := append([]string{}, toolingCmdFiles...)
	toolingSourceFiles = append(toolingSourceFiles, toolingInternalFiles...)
	toolingSourceFiles = append(toolingSourceFiles, "specflow/tooling/go.mod")
	toolingSourceFiles = append(toolingSourceFiles, "specflow/tooling/manifest.tsv")
	if fileExists(repoRoot, "specflow/tooling/go.sum") {
		toolingSourceFiles = append(toolingSourceFiles, "specflow/tooling/go.sum")
	}
	if len(toolingSourceFiles) == 0 {
		return scope, fmt.Errorf("default tooling source files are incomplete")
	}
	toolingScriptFiles := []string{
		"specflow/tooling/scripts/tooling_fingerprint.sh",
		"specflow/tooling/scripts/tooling_fingerprint.ps1",
	}
	toolingRuntimeFiles, err := walkRelativeFiles(repoRoot, "specflow/tooling/reader/web", "")
	if err != nil {
		return scope, err
	}
	if len(toolingRuntimeFiles) == 0 {
		return scope, fmt.Errorf("default tooling runtime files are incomplete")
	}

	required := append([]string{}, ruleFiles...)
	required = append(required, minimumGuidanceSkillFiles...)
	required = append(required, templateGovernanceFiles...)
	required = append(required, templateEntryFiles...)
	required = append(required, projectEntryFiles...)
	required = append(required, agentOperabilityFiles...)
	required = append(required, projectRegistryFiles...)
	required = append(required, toolingContractFiles...)
	required = append(required, toolingSourceFiles...)
	required = append(required, toolingScriptFiles...)
	required = append(required, toolingRuntimeFiles...)
	if err := ensureRelativeFiles(repoRoot, required); err != nil {
		return scope, err
	}

	validation, err := projectstandards.ValidateRegistry(repoRoot)
	if err != nil {
		return scope, err
	}

	activeStandardFiles := make([]string, 0, len(validation.ValidEntries))
	for _, entry := range validation.ValidEntries {
		if strings.TrimSpace(entry.File) != "" {
			activeStandardFiles = append(activeStandardFiles, entry.File)
		}
	}

	scope.FrameworkGuidelineFiles = frameworkFiles
	scope.CommandFiles = commandFiles
	scope.GuidanceSkillFiles = sortAndDedupe(guidanceSkillFiles)
	scope.RuleGovernanceFiles = ruleFiles
	scope.TemplateGovernanceFiles = templateGovernanceFiles
	scope.TemplateEntryFiles = templateEntryFiles
	scope.ProjectEntryFiles = projectEntryFiles
	scope.AgentOperabilityFiles = agentOperabilityFiles
	scope.ProjectInstanceCompatibilityFiles = projectInstanceCompatibilityFiles
	scope.ProjectRegistryFiles = projectRegistryFiles
	scope.RegistryDiagnostics = sortAndDedupe(validation.Diagnostics)
	scope.ToolingContractFiles = toolingContractFiles
	scope.ToolingSourceFiles = sortAndDedupe(toolingSourceFiles)
	scope.ToolingScriptFiles = toolingScriptFiles
	scope.ToolingRuntimeFiles = sortAndDedupe(toolingRuntimeFiles)
	scope.ActiveProjectStandardFiles = sortAndDedupe(activeStandardFiles)
	return scope, nil
}

func CollectDefaultSpecFlowDesignScope(repoRoot string) (SpecFlowScope, error) {
	scope := SpecFlowScope{
		Scenario: "default_design_baseline",
	}

	commandFiles, err := globRelative(repoRoot, "specflow/framework/commands/*.md")
	if err != nil {
		return scope, err
	}
	if len(commandFiles) == 0 {
		return scope, fmt.Errorf("default design command files are incomplete")
	}

	designFoundationFiles := []string{
		"specflow/framework/spec_flow_design_review.md",
		"specflow/framework/agent_operability_standard.md",
		"specflow/framework/spec_policy.md",
		"specflow/framework/spec_writing_guide.md",
		"specflow/framework/command_policy.md",
		"specflow/framework/natural_language_routing.md",
		"specflow/framework/spec_flow_migrate.md",
		"specflow/framework/onboarding_decision_policy.md",
		"specflow/framework/implementation_change_policy.md",
		"specflow/framework/repository_mapping_policy.md",
		"specflow/framework/scenario_policy.md",
		"specflow/framework/checkpoint_protocol.md",
	}
	lifecycleContractFiles := []string{
		"specflow/framework/candidate_handoff_contract.md",
		"specflow/framework/downgrade_policy.md",
		"specflow/framework/process_snapshot_contract.md",
		"specflow/framework/recovery_policy.md",
		"specflow/framework/checkpoint_protocol.md",
	}
	templateProcessStateFiles := []string{
		"specflow/templates/docs/specs/_status.md",
		"specflow/templates/docs/specs/_check_result/README.md",
		"specflow/templates/docs/specs/_plans/README.md",
		"specflow/templates/docs/specs/_plans/draft/README.md",
		"specflow/templates/docs/specs/_plans/active/README.md",
		"specflow/templates/docs/specs/_verify_result/README.md",
		"specflow/templates/docs/specs/_governance_review/README.md",
	}
	templateEntryFiles := []string{
		"specflow/templates/AGENTS.md",
		"specflow/templates/GEMINI.md",
		"specflow/templates/CLAUDE.md",
	}
	projectEntryFiles := []string{
		"AGENTS.md",
		"GEMINI.md",
		"CLAUDE.md",
	}
	projectRegistryFiles := []string{
		"docs/project_standards/_registry.md",
	}
	projectStandardPolicyFiles := []string{
		"specflow/framework/entry_index_registry.md",
		"specflow/framework/project_standards_policy.md",
		"specflow/framework/project_standard_create.md",
	}

	required := append([]string{}, designFoundationFiles...)
	required = append(required, lifecycleContractFiles...)
	required = append(required, templateProcessStateFiles...)
	required = append(required, templateEntryFiles...)
	required = append(required, projectEntryFiles...)
	required = append(required, projectRegistryFiles...)
	required = append(required, projectStandardPolicyFiles...)
	if err := ensureRelativeFiles(repoRoot, required); err != nil {
		return scope, err
	}

	validation, err := projectstandards.ValidateRegistry(repoRoot)
	if err != nil {
		return scope, err
	}

	activeStandardFiles := make([]string, 0, len(validation.ValidEntries))
	for _, entry := range validation.ValidEntries {
		if strings.TrimSpace(entry.File) != "" {
			activeStandardFiles = append(activeStandardFiles, entry.File)
		}
	}

	scope.FrameworkGuidelineFiles = sortAndDedupe(designFoundationFiles)
	scope.CommandFiles = commandFiles
	scope.TemplateGovernanceFiles = sortAndDedupe(append(lifecycleContractFiles, templateProcessStateFiles...))
	scope.TemplateEntryFiles = templateEntryFiles
	scope.ProjectEntryFiles = projectEntryFiles
	scope.ProjectRegistryFiles = sortAndDedupe(append(projectRegistryFiles, projectStandardPolicyFiles...))
	scope.RegistryDiagnostics = sortAndDedupe(validation.Diagnostics)
	scope.ActiveProjectStandardFiles = sortAndDedupe(activeStandardFiles)
	return scope, nil
}

func collectAgentOperabilityFiles(projectEntryFiles, templateEntryFiles, templateProcessStateFiles, commandFiles, guidanceSkillFiles, sharedGovernanceFiles, processStateContractFiles, toolingContractFiles []string) []string {
	files := []string{
		"specflow/framework/agent_operability_standard.md",
		"specflow/framework/spec_flow_review.md",
		"specflow/framework/spec_flow_design_review.md",
		"specflow/framework/spec_flow_migrate.md",
		"specflow/framework/natural_language_routing.md",
		"specflow/framework/command_policy.md",
		"specflow/framework/onboarding_decision_policy.md",
		"specflow/framework/implementation_change_policy.md",
		"specflow/framework/checkpoint_protocol.md",
	}
	files = append(files, projectEntryFiles...)
	files = append(files, templateEntryFiles...)
	files = append(files, commandFiles...)
	files = append(files, guidanceSkillFiles...)
	files = append(files, sharedGovernanceFiles...)
	files = append(files, processStateContractFiles...)
	files = append(files, toolingContractFiles...)
	files = append(files, templateProcessStateFiles...)
	return sortAndDedupe(files)
}

func collectProjectInstanceCompatibilityFiles(repoRoot string) ([]string, error) {
	required := []string{
		"docs/specs/_status.md",
		"docs/specs/repository_mapping.md",
		"docs/specs/rules/stable/s_g_rule_repository_baseline.md",
	}
	if err := ensureRelativeFiles(repoRoot, required); err != nil {
		return nil, err
	}

	root := filepath.Join(repoRoot, filepath.FromSlash("docs/specs"))
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("required scope directory missing: docs/specs")
	}

	result := append([]string{}, required...)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)
		if strings.HasPrefix(relPath, "docs/specs/_governance_review/") {
			return nil
		}
		result = append(result, relPath)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sortAndDedupe(result), nil
}

func globRelative(repoRoot, pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(repoRoot, filepath.FromSlash(pattern)))
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		rel, err := filepath.Rel(repoRoot, match)
		if err != nil {
			return nil, err
		}
		result = append(result, filepath.ToSlash(rel))
	}
	sort.Strings(result)
	return result, nil
}

func walkRelativeFiles(repoRoot, relDir, suffix string) ([]string, error) {
	root := filepath.Join(repoRoot, filepath.FromSlash(relDir))
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("required scope directory missing: %s", relDir)
	}

	result := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), suffix) {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		result = append(result, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(result)
	return result, nil
}

func ensureRelativeFiles(repoRoot string, relPaths []string) error {
	for _, relPath := range relPaths {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relPath))); err != nil {
			return fmt.Errorf("required scope file missing: %s", relPath)
		}
	}
	return nil
}

func fileExists(repoRoot, relPath string) bool {
	_, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relPath)))
	return err == nil
}

func sortAndDedupe(items []string) []string {
	set := map[string]bool{}
	for _, item := range items {
		set[item] = true
	}
	result := make([]string, 0, len(set))
	for item := range set {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

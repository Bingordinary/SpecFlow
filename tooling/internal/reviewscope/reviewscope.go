package reviewscope

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	LayoutSourceRepo       = "source_repo"
	CompatibilityTemplateBootstrap = "template_bootstrap"
)

type SpecFlowScope struct {
	Profile                           string
	Layout                            string
	FrameworkRoot                     string
	TemplateRoot                      string
	ToolingRoot                       string
	ProjectInstanceCompatibilityMode  string
	FrameworkGuidelineFiles           []string
	CommandFiles                      []string
	CandidateIntentFiles              []string
	GuidanceSkillFiles                []string
	RuleGovernanceFiles               []string
	TemplateGovernanceFiles           []string
	TemplateProjectInstanceFiles      []string
	TemplateEntryFiles                []string
	ProjectEntryFiles                 []string
	SourceRepoEntryExampleFiles       []string
	AgentOperabilityFiles             []string
	ProjectInstanceCompatibilityFiles []string
	ToolingContractFiles              []string
	ToolingSourceFiles                []string
	ToolingScriptFiles                []string
	ToolingRuntimeFiles               []string
}

func CollectDefaultSpecFlowScope(repoRoot string) (SpecFlowScope, error) {
	scope := SpecFlowScope{
		Profile:                          "default_governance_baseline",
		Layout:                           LayoutSourceRepo,
		FrameworkRoot:                    "framework",
		TemplateRoot:                     "templates",
		ToolingRoot:                      "tooling",
		ProjectInstanceCompatibilityMode: CompatibilityTemplateBootstrap,
	}

	frameworkFiles, err := layeredFrameworkFiles(repoRoot, scope.FrameworkRoot)
	if err != nil {
		return scope, err
	}
	guidanceSkillFiles, err := globRelative(repoRoot, scope.FrameworkPath("guidance/*/SKILL.md"))
	if err != nil {
		return scope, err
	}
	if len(frameworkFiles) == 0 || len(guidanceSkillFiles) == 0 {
		return scope, fmt.Errorf("default governance files are incomplete")
	}

	minimumGuidanceSkillFiles := []string{
		scope.FrameworkPath("guidance/using-specflow-guidance/SKILL.md"),
		scope.FrameworkPath("guidance/project-framing/SKILL.md"),
		scope.FrameworkPath("guidance/scope-cutting/SKILL.md"),
		scope.FrameworkPath("guidance/solution-design/SKILL.md"),
		scope.FrameworkPath("guidance/design-quality-review/SKILL.md"),
		scope.FrameworkPath("guidance/spec-writeback-guidance/SKILL.md"),
	}
	ruleFiles := []string{
		scope.FrameworkPath("rule_validate_checklist.md"),
		scope.FrameworkPath("rule_verify_checklist.md"),
		scope.FrameworkPath("rule_promote_workflow.md"),
		scope.FrameworkPath("governance/impact_sync.md"),
	}
	templateProjectInstanceFiles := []string{
		scope.TemplatePath("docs/specs/repository_mapping.md"),
		scope.TemplatePath("docs/specs/rules/stable/s_g_rule_repository_baseline.md"),
	}
	toolingContractFiles := []string{
		scope.FrameworkPath("tooling_execution_policy.md"),
		scope.ToolingPath("README.md"),
	}
	agentOperabilityFiles := collectAgentOperabilityFiles(scope, guidanceSkillFiles, ruleFiles, toolingContractFiles)
	projectInstanceCompatibilityFiles, err := collectProjectInstanceCompatibilityFiles(repoRoot, scope)
	if err != nil {
		return scope, err
	}

	toolingCmdFiles, err := walkRelativeFiles(repoRoot, scope.ToolingPath("cmd"), ".go")
	if err != nil {
		return scope, err
	}
	toolingInternalFiles, err := walkRelativeFiles(repoRoot, scope.ToolingPath("internal"), ".go")
	if err != nil {
		return scope, err
	}
	toolingSourceFiles := append([]string{}, toolingCmdFiles...)
	toolingSourceFiles = append(toolingSourceFiles, toolingInternalFiles...)
	toolingSourceFiles = append(toolingSourceFiles, scope.ToolingPath("go.mod"))
	toolingSourceFiles = append(toolingSourceFiles, scope.ToolingPath("manifest.tsv"))
	if fileExists(repoRoot, scope.ToolingPath("go.sum")) {
		toolingSourceFiles = append(toolingSourceFiles, scope.ToolingPath("go.sum"))
	}
	if len(toolingSourceFiles) == 0 {
		return scope, fmt.Errorf("default tooling source files are incomplete")
	}
	toolingScriptFiles, err := walkRelativeFiles(repoRoot, scope.ToolingPath("scripts"), "")
	if err != nil {
		return scope, err
	}
	if len(toolingScriptFiles) == 0 {
		return scope, fmt.Errorf("default tooling script files are incomplete")
	}
	toolingRuntimeFiles, err := walkRelativeFiles(repoRoot, scope.ToolingPath("reader/web"), "")
	if err != nil {
		return scope, err
	}
	if len(toolingRuntimeFiles) == 0 {
		return scope, fmt.Errorf("default tooling runtime files are incomplete")
	}

	required := append([]string{}, ruleFiles...)
	required = append(required, minimumGuidanceSkillFiles...)
	required = append(required, templateProjectInstanceFiles...)
	required = append(required, agentOperabilityFiles...)
	required = append(required, toolingContractFiles...)
	required = append(required, toolingSourceFiles...)
	required = append(required, toolingScriptFiles...)
	required = append(required, toolingRuntimeFiles...)
	if err := ensureRelativeFiles(repoRoot, required); err != nil {
		return scope, err
	}

	scope.FrameworkGuidelineFiles = frameworkFiles
	scope.GuidanceSkillFiles = sortAndDedupe(guidanceSkillFiles)
	scope.RuleGovernanceFiles = ruleFiles
	scope.TemplateProjectInstanceFiles = templateProjectInstanceFiles
	scope.AgentOperabilityFiles = agentOperabilityFiles
	scope.ProjectInstanceCompatibilityFiles = projectInstanceCompatibilityFiles
	scope.ToolingContractFiles = toolingContractFiles
	scope.ToolingSourceFiles = sortAndDedupe(toolingSourceFiles)
	scope.ToolingScriptFiles = toolingScriptFiles
	scope.ToolingRuntimeFiles = sortAndDedupe(toolingRuntimeFiles)
	return scope, nil
}

func CollectDefaultSpecFlowDesignScope(repoRoot string) (SpecFlowScope, error) {
	scope := SpecFlowScope{
		Profile:                          "default_design_baseline",
		Layout:                           LayoutSourceRepo,
		FrameworkRoot:                    "framework",
		TemplateRoot:                     "templates",
		ToolingRoot:                      "tooling",
		ProjectInstanceCompatibilityMode: CompatibilityTemplateBootstrap,
	}

	designFoundationFiles := []string{
		scope.FrameworkPath("spec_flow_design_review.md"),
		scope.FrameworkPath("governance/review.md"),
		scope.FrameworkPath("governance/review_scope.md"),
		scope.FrameworkPath("rule_validate_checklist.md"),
		scope.FrameworkPath("rule_verify_checklist.md"),
		scope.FrameworkPath("rule_promote_workflow.md"),
		scope.FrameworkPath("concepts.md"),
		scope.FrameworkPath("core/object_model.md"),
		scope.FrameworkPath("core/repository_mapping.md"),
		scope.FrameworkPath("operations/migration.md"),
		scope.FrameworkPath("spec_writing_guide.md"),
		scope.FrameworkPath("governance/impact_sync.md"),
	}
	required := append([]string{}, designFoundationFiles...)
	if err := ensureRelativeFiles(repoRoot, required); err != nil {
		return scope, err
	}

	scope.FrameworkGuidelineFiles = sortAndDedupe(designFoundationFiles)
	return scope, nil
}

func (scope SpecFlowScope) FrameworkPath(relPath string) string {
	return joinPath(scope.FrameworkRoot, relPath)
}

func (scope SpecFlowScope) TemplatePath(relPath string) string {
	return joinPath(scope.TemplateRoot, relPath)
}

func (scope SpecFlowScope) ToolingPath(relPath string) string {
	return joinPath(scope.ToolingRoot, relPath)
}

func collectAgentOperabilityFiles(scope SpecFlowScope, guidanceSkillFiles, ruleFiles, toolingContractFiles []string) []string {
	files := []string{
		scope.FrameworkPath("concepts.md"),
		scope.FrameworkPath("core/object_model.md"),
		scope.FrameworkPath("core/repository_mapping.md"),
		scope.FrameworkPath("governance/review.md"),
		scope.FrameworkPath("governance/review_scope.md"),
		scope.FrameworkPath("rule_validate_checklist.md"),
		scope.FrameworkPath("rule_verify_checklist.md"),
		scope.FrameworkPath("rule_promote_workflow.md"),
		scope.FrameworkPath("operations/migration.md"),
		scope.FrameworkPath("severity_policy.md"),
		scope.FrameworkPath("spec_flow_design_review.md"),
		scope.FrameworkPath("spec_flow_review.md"),
		scope.FrameworkPath("spec_writing_guide.md"),
	}
	files = append(files, guidanceSkillFiles...)
	files = append(files, ruleFiles...)
	files = append(files, toolingContractFiles...)
	return sortAndDedupe(files)
}

func layeredFrameworkFiles(repoRoot string, frameworkRoot string) ([]string, error) {
	result, err := globRelative(repoRoot, joinPath(frameworkRoot, "*.md"))
	if err != nil {
		return nil, err
	}
	for _, relDir := range []string{
		joinPath(frameworkRoot, "core"),
		joinPath(frameworkRoot, "governance"),
		joinPath(frameworkRoot, "operations"),
		joinPath(frameworkRoot, "_atoms"),
	} {
		files, err := walkRelativeFiles(repoRoot, relDir, ".md")
		if err != nil {
			return nil, err
		}
		result = append(result, files...)
	}
	for _, nonMdFile := range []string{
		joinPath(frameworkRoot, "_atoms/manifest.txt"),
		joinPath(frameworkRoot, "_atoms/generate.sh"),
		joinPath(frameworkRoot, "_atoms/verify.sh"),
	} {
		if fileExists(repoRoot, nonMdFile) {
			result = append(result, nonMdFile)
		}
	}
	return sortAndDedupe(result), nil
}

func collectProjectInstanceCompatibilityFiles(repoRoot string, scope SpecFlowScope) ([]string, error) {
	return walkRelativeFiles(repoRoot, scope.TemplatePath("docs/specs"), ".md")
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

func joinPath(root, relPath string) string {
	root = strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/")
	relPath = strings.Trim(strings.ReplaceAll(relPath, "\\", "/"), "/")
	if root == "" {
		return relPath
	}
	if relPath == "" {
		return root
	}
	return root + "/" + relPath
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

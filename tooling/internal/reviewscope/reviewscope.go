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
	Scenario                   string
	FrameworkGuidelineFiles    []string
	CommandFiles               []string
	SharedGovernanceFiles      []string
	TemplateGovernanceFiles    []string
	TemplateEntryFiles         []string
	ProjectRegistryFiles       []string
	RegistryDiagnostics        []string
	ToolingContractFiles       []string
	ToolingSourceFiles         []string
	ActiveProjectStandardFiles []string
}

func CollectDefaultSpecFlowScope(repoRoot string) (SpecFlowScope, error) {
	scope := SpecFlowScope{
		Scenario: "default_governance_baseline",
	}

	frameworkFiles, err := globRelative(repoRoot, "specflow/framework/docs/agent_guidelines/*.md")
	if err != nil {
		return scope, err
	}
	commandFiles, err := globRelative(repoRoot, "specflow/framework/docs/agent_guidelines/commands/*.md")
	if err != nil {
		return scope, err
	}
	if len(frameworkFiles) == 0 || len(commandFiles) == 0 {
		return scope, fmt.Errorf("default governance files are incomplete")
	}

	sharedFiles := []string{
		"specflow/framework/docs/agent_guidelines/shared_ops.md",
		"specflow/framework/docs/agent_guidelines/shared_new.md",
		"specflow/framework/docs/agent_guidelines/shared_extract.md",
		"specflow/framework/docs/agent_guidelines/shared_bind.md",
		"specflow/framework/docs/agent_guidelines/shared_topology.md",
		"specflow/framework/docs/agent_guidelines/shared_sync.md",
		"specflow/framework/docs/agent_guidelines/shared_escape.md",
	}
	templateGovernanceFiles := []string{
		"specflow/templates/root/docs/specs/_status.md",
		"specflow/templates/root/docs/specs/_check_result/README.md",
		"specflow/templates/root/docs/specs/_plans/README.md",
		"specflow/templates/root/docs/specs/_plans/draft/README.md",
		"specflow/templates/root/docs/specs/_plans/active/README.md",
		"specflow/templates/root/docs/specs/_verify_result/README.md",
		"specflow/templates/root/docs/project_standards/_registry.md",
	}
	templateEntryFiles := []string{
		"specflow/templates/root/AGENTS.md",
		"specflow/templates/root/GEMINI.md",
		"specflow/templates/root/CLAUDE.md",
	}
	projectRegistryFiles := []string{
		"docs/project_standards/_registry.md",
	}
	toolingContractFiles := []string{
		"specflow/framework/docs/agent_guidelines/tooling_execution_policy.md",
		"specflow/tooling/README.md",
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
	if len(toolingSourceFiles) == 0 {
		return scope, fmt.Errorf("default tooling source files are incomplete")
	}

	required := append([]string{}, sharedFiles...)
	required = append(required, templateGovernanceFiles...)
	required = append(required, templateEntryFiles...)
	required = append(required, projectRegistryFiles...)
	required = append(required, toolingContractFiles...)
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
	scope.SharedGovernanceFiles = sharedFiles
	scope.TemplateGovernanceFiles = templateGovernanceFiles
	scope.TemplateEntryFiles = templateEntryFiles
	scope.ProjectRegistryFiles = projectRegistryFiles
	scope.RegistryDiagnostics = sortAndDedupe(validation.Diagnostics)
	scope.ToolingContractFiles = toolingContractFiles
	scope.ToolingSourceFiles = sortAndDedupe(toolingSourceFiles)
	scope.ActiveProjectStandardFiles = sortAndDedupe(activeStandardFiles)
	return scope, nil
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

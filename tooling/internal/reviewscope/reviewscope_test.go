package reviewscope

import (
	"os"
	"path/filepath"
	"testing"
)



func TestCollectDefaultSpecFlowScopeSupportsSourceRepoLayout(t *testing.T) {
	repoRoot := t.TempDir()
	writeSourceScopeRepo(t, repoRoot)

	scope, err := CollectDefaultSpecFlowScope(repoRoot)
	if err != nil {
		t.Fatalf("CollectDefaultSpecFlowScope source: %v", err)
	}
	if scope.Layout != LayoutSourceRepo {
		t.Fatalf("expected source_repo layout, got %s", scope.Layout)
	}
	if scope.FrameworkRoot != "framework" || scope.TemplateRoot != "templates" || scope.ToolingRoot != "tooling" {
		t.Fatalf("unexpected source roots: %+v", scope)
	}
	if scope.ProjectInstanceCompatibilityMode != CompatibilityTemplateBootstrap {
		t.Fatalf("expected template compatibility mode, got %s", scope.ProjectInstanceCompatibilityMode)
	}
	if containsString(scope.FrameworkGuidelineFiles, deletedCommandPolicyPath("framework")) {
		t.Fatalf("deleted flat command owner must stay outside source scope, got %+v", scope.FrameworkGuidelineFiles)
	}

	if !containsString(scope.ToolingSourceFiles, "tooling/go.mod") {
		t.Fatalf("expected local tooling source, got %+v", scope.ToolingSourceFiles)
	}
	if len(scope.ProjectEntryFiles) != 0 {
		t.Fatalf("source scope must not require local ignored project entry files, got %+v", scope.ProjectEntryFiles)
	}
}

func TestCollectDefaultSpecFlowDesignScopeAutoDetectsSourceRepoLayout(t *testing.T) {
	repoRoot := t.TempDir()
	writeSourceScopeRepo(t, repoRoot)

	scope, err := CollectDefaultSpecFlowDesignScope(repoRoot)
	if err != nil {
		t.Fatalf("CollectDefaultSpecFlowDesignScope source auto: %v", err)
	}
	if scope.Layout != LayoutSourceRepo {
		t.Fatalf("expected auto source layout, got %s", scope.Layout)
	}
	if containsString(scope.FrameworkGuidelineFiles, "framework/operations/output_standard.md") {
		t.Fatalf("deleted output standard must stay outside design scope, got %+v", scope.FrameworkGuidelineFiles)
	}
	if !containsString(scope.FrameworkGuidelineFiles, "framework/rule_validate_checklist.md") {
		t.Fatalf("expected rule validate checklist in design scope, got %+v", scope.FrameworkGuidelineFiles)
	}
	if containsString(scope.TemplateGovernanceFiles, "framework/operations/output_standard.md") {
		t.Fatalf("deleted output standard must stay outside lifecycle contract scope, got %+v", scope.TemplateGovernanceFiles)
	}
	if containsString(scope.FrameworkGuidelineFiles, deletedCommandPolicyPath("framework")) {
		t.Fatalf("deleted flat command owner must stay outside source design scope, got %+v", scope.FrameworkGuidelineFiles)
	}
	if len(scope.ProjectEntryFiles) != 0 {
		t.Fatalf("source design scope must not require local ignored project entry files, got %+v", scope.ProjectEntryFiles)
	}
}

func writeSourceScopeRepo(t *testing.T, repoRoot string) {
	t.Helper()
	for _, relPath := range []string{
		"framework/core/object_model.md",
		"framework/rule_validate_checklist.md",
		"framework/rule_promote_workflow.md",
		"framework/governance/impact_sync.md",
		"framework/governance/review.md",
		"framework/governance/review_scope.md",
		"framework/operations/update.md",
		"framework/spec_flow_review.md",
		"framework/spec_flow_design_review.md",
		"framework/spec_writing_guide.md",
		"framework/concepts.md",
		"framework/tooling_execution_policy.md",
		"framework/severity_policy.md",
		"framework/guidance/using-specflow-guidance/SKILL.md",
		"framework/guidance/project-framing/SKILL.md",
		"framework/guidance/scope-cutting/SKILL.md",
		"framework/guidance/solution-design/SKILL.md",
		"framework/guidance/design-quality-review/SKILL.md",
		"framework/guidance/spec-writeback-guidance/SKILL.md",
		"framework/_atoms/README.md",
		"framework/_atoms/manifest.txt",
		"framework/_atoms/generate.sh",
		"framework/_atoms/verify.sh",
		"templates/meta/governance_review/README.md",
		"templates/docs/specs/rules/stable/g_rule_repository_baseline.md",
		"templates/AGENTS.md",
		"templates/GEMINI.md",
		"templates/CLAUDE.md",
		"example.md",
		"tooling/README.md",
		"tooling/cmd/specflowctl/main.go",
		"tooling/internal/demo/demo.go",
		"tooling/go.mod",
		"tooling/manifest.tsv",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# "+filepath.Base(relPath)+"\n")
	}
	for _, relPath := range []string{
		"tooling/scripts/build_release.sh",
		"tooling/scripts/install.ps1",
		"tooling/scripts/install.sh",
		"tooling/scripts/pull_with_release.ps1",
		"tooling/scripts/pull_with_release.sh",
		"tooling/scripts/push_with_release.ps1",
		"tooling/scripts/push_with_release.sh",
		"tooling/scripts/tooling_fingerprint.ps1",
		"tooling/scripts/tooling_fingerprint.sh",
		"tooling/scripts/update_tooling_binaries.ps1",
		"tooling/scripts/update_tooling_binaries.sh",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# script\n")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}





func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func deletedCommandPolicyPath(root string) string {
	return root + "/" + "command_" + "policy.md"
}



package reviewscope

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectDefaultSpecFlowScopeExcludesInvalidRegistryEntryFromGovernanceInputs(t *testing.T) {
	repoRoot := t.TempDir()
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/spec_flow_review.md"), "# review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/spec_flow_design_review.md"), "# design review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/agent_operability_standard.md"), "# operability\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/natural_language_routing.md"), "# routing\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/command_policy.md"), "# command policy\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/implementation_change_policy.md"), "# implementation policy\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/checkpoint_protocol.md"), "# checkpoint\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/tooling_execution_policy.md"), "# tooling\n")
	for _, name := range []string{
		"candidate_handoff_contract.md",
		"downgrade_policy.md",
		"process_snapshot_contract.md",
		"recovery_policy.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, "specflow/framework", name), "# "+name+"\n")
	}
	for _, name := range []string{
		"shared_new.md",
		"shared_extract.md",
		"shared_bind.md",
		"shared_topology.md",
		"shared_sync.md",
		"shared_escape.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, "specflow/framework", name), "# "+name+"\n")
	}
	writeGuidanceSkillFiles(t, repoRoot)
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/commands/unit_check.md"), "# unit_check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_status.md"), "# status\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_check_result/README.md"), "# check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_plans/README.md"), "# plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_plans/draft/README.md"), "# draft plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_plans/active/README.md"), "# active plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_verify_result/README.md"), "# verify\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_governance_review/README.md"), "# governance review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/project_standards/_registry.md"), "# template registry\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_check_result/README.md"), "# project check\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_plans/README.md"), "# project plans\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_plans/draft/README.md"), "# project draft plans\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_plans/active/README.md"), "# project active plans\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_verify_result/README.md"), "# project verify\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/AGENTS.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/GEMINI.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/CLAUDE.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "AGENTS.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "GEMINI.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "CLAUDE.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_status.md"), "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/_registry.md"), ""+
		"# Registry\n\n"+
		"## Active Standards\n\n"+
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |\n"+
		"|---|---|---|---|---|---|---|---|---|---|\n"+
		"| `bad_prompt_rule` | `review_standard` | `candidate_closure_review` | `docs/project_standards/prompt_guidelines.md` | `unit_check` | `review_scenario:not_supported_here` | `tighten` | `framework_wins` | invalid scenario |\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/prompt_guidelines.md"), "# prompt\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/README.md"), "# tooling\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/cmd/specflowctl/main.go"), "package main\nfunc main() {}\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/internal/demo/demo.go"), "package demo\nfunc Value() string { return \"demo\" }\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module example.com/specflow\n\ngo 1.22\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), "templates/AGENTS.md\tAGENTS.md\tframework\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/go.sum"), "example.com/mod v1.0.0 h1:demo\n")

	scope, err := CollectDefaultSpecFlowScope(repoRoot)
	if err != nil {
		t.Fatalf("CollectDefaultSpecFlowScope: %v", err)
	}
	if len(scope.ActiveProjectStandardFiles) != 0 {
		t.Fatalf("expected invalid entry to be excluded from active standard files, got %+v", scope.ActiveProjectStandardFiles)
	}
	if len(scope.RegistryDiagnostics) == 0 {
		t.Fatalf("expected registry diagnostics, got none")
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/framework/agent_operability_standard.md") {
		t.Fatalf("expected agent operability file in scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/framework/commands/unit_check.md") {
		t.Fatalf("expected command files in agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.GuidanceSkillFiles, "specflow/framework/skills/using-specflow-guidance/SKILL.md") {
		t.Fatalf("expected guidance skill files in scope, got %+v", scope.GuidanceSkillFiles)
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/framework/skills/using-specflow-guidance/SKILL.md") {
		t.Fatalf("expected guidance skill files in agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/framework/shared_sync.md") {
		t.Fatalf("expected shared-governance files in agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/framework/process_snapshot_contract.md") {
		t.Fatalf("expected process-state contract files in agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/framework/tooling_execution_policy.md") {
		t.Fatalf("expected tooling execution policy in agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/tooling/README.md") {
		t.Fatalf("expected tooling README in agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if containsString(scope.AgentOperabilityFiles, "docs/specs/_verify_result/README.md") {
		t.Fatalf("expected project-side process files to stay outside agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.AgentOperabilityFiles, "specflow/templates/docs/specs/_governance_review/README.md") {
		t.Fatalf("expected governance review process contract in agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if containsString(scope.AgentOperabilityFiles, "docs/specs/repository_mapping.md") {
		t.Fatalf("expected project repository truth to stay outside agent operability scope, got %+v", scope.AgentOperabilityFiles)
	}
	if !containsString(scope.ProjectEntryFiles, "AGENTS.md") {
		t.Fatalf("expected project entry files in scope, got %+v", scope.ProjectEntryFiles)
	}
	if !containsString(scope.ToolingSourceFiles, "specflow/tooling/go.mod") {
		t.Fatalf("expected tooling go.mod in tooling source scope, got %+v", scope.ToolingSourceFiles)
	}
	if !containsString(scope.ToolingSourceFiles, "specflow/tooling/manifest.tsv") {
		t.Fatalf("expected tooling manifest in tooling source scope, got %+v", scope.ToolingSourceFiles)
	}
	if !containsString(scope.ToolingSourceFiles, "specflow/tooling/go.sum") {
		t.Fatalf("expected tooling go.sum in tooling source scope when present, got %+v", scope.ToolingSourceFiles)
	}
}

func TestCollectDefaultSpecFlowScopeExcludesUnsupportedSpecFlowReviewEntry(t *testing.T) {
	repoRoot := t.TempDir()
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/spec_flow_review.md"), "# review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/spec_flow_design_review.md"), "# design review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/agent_operability_standard.md"), "# operability\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/natural_language_routing.md"), "# routing\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/command_policy.md"), "# command policy\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/implementation_change_policy.md"), "# implementation policy\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/checkpoint_protocol.md"), "# checkpoint\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/tooling_execution_policy.md"), "# tooling\n")
	for _, name := range []string{
		"candidate_handoff_contract.md",
		"downgrade_policy.md",
		"process_snapshot_contract.md",
		"recovery_policy.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, "specflow/framework", name), "# "+name+"\n")
	}
	for _, name := range []string{
		"shared_new.md",
		"shared_extract.md",
		"shared_bind.md",
		"shared_topology.md",
		"shared_sync.md",
		"shared_escape.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, "specflow/framework", name), "# "+name+"\n")
	}
	writeGuidanceSkillFiles(t, repoRoot)
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/commands/unit_check.md"), "# unit_check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_status.md"), "# status\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_check_result/README.md"), "# check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_plans/README.md"), "# plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_plans/draft/README.md"), "# draft plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_plans/active/README.md"), "# active plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_verify_result/README.md"), "# verify\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/specs/_governance_review/README.md"), "# governance review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/docs/project_standards/_registry.md"), "# template registry\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_check_result/README.md"), "# project check\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_plans/README.md"), "# project plans\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_plans/draft/README.md"), "# project draft plans\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_plans/active/README.md"), "# project active plans\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_verify_result/README.md"), "# project verify\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/AGENTS.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/GEMINI.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/CLAUDE.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "AGENTS.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "GEMINI.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "CLAUDE.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_status.md"), "# Spec Status\n\n## Formal Objects\n\n| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|---|\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/_registry.md"), ""+
		"# Registry\n\n"+
		"## Active Standards\n\n"+
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |\n"+
		"|---|---|---|---|---|---|---|---|---|---|\n"+
		"| `bad_overlay_rule` | `review_standard` | `governance_baseline_review` | `docs/project_standards/prompt_guidelines.md` | `spec_flow_review` | `review_scenario:` | `tighten` | `framework_wins` | malformed selector |\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/prompt_guidelines.md"), "# prompt\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/README.md"), "# tooling\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/cmd/specflowctl/main.go"), "package main\nfunc main() {}\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/internal/demo/demo.go"), "package demo\nfunc Value() string { return \"demo\" }\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module example.com/specflow\n\ngo 1.22\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/manifest.tsv"), "templates/AGENTS.md\tAGENTS.md\tframework\n")

	scope, err := CollectDefaultSpecFlowScope(repoRoot)
	if err != nil {
		t.Fatalf("CollectDefaultSpecFlowScope: %v", err)
	}
	if len(scope.ActiveProjectStandardFiles) != 0 {
		t.Fatalf("expected unsupported spec_flow_review entry to be excluded from active standard files, got %+v", scope.ActiveProjectStandardFiles)
	}
	if len(scope.RegistryDiagnostics) == 0 {
		t.Fatalf("expected registry diagnostics, got none")
	}
}

func TestCollectDefaultSpecFlowDesignScopeIncludesGovernanceReviewProcessContract(t *testing.T) {
	repoRoot := t.TempDir()
	for _, relPath := range []string{
		"specflow/framework/spec_flow_design_review.md",
		"specflow/framework/agent_operability_standard.md",
		"specflow/framework/spec_policy.md",
		"specflow/framework/command_policy.md",
		"specflow/framework/natural_language_routing.md",
		"specflow/framework/implementation_change_policy.md",
		"specflow/framework/repository_mapping_policy.md",
		"specflow/framework/scenario_policy.md",
		"specflow/framework/git_policy.md",
		"specflow/framework/checkpoint_protocol.md",
		"specflow/framework/candidate_handoff_contract.md",
		"specflow/framework/downgrade_policy.md",
		"specflow/framework/process_snapshot_contract.md",
		"specflow/framework/recovery_policy.md",
		"specflow/framework/entry_index_registry.md",
		"specflow/framework/project_standards_policy.md",
		"specflow/framework/project_standard_create.md",
		"specflow/framework/commands/unit_check.md",
		"specflow/templates/docs/specs/_status.md",
		"specflow/templates/docs/specs/_check_result/README.md",
		"specflow/templates/docs/specs/_plans/README.md",
		"specflow/templates/docs/specs/_plans/draft/README.md",
		"specflow/templates/docs/specs/_plans/active/README.md",
		"specflow/templates/docs/specs/_verify_result/README.md",
		"specflow/templates/docs/specs/_governance_review/README.md",
		"specflow/templates/AGENTS.md",
		"specflow/templates/GEMINI.md",
		"specflow/templates/CLAUDE.md",
		"AGENTS.md",
		"GEMINI.md",
		"CLAUDE.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# "+filepath.Base(relPath)+"\n")
	}
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/_registry.md"), ""+
		"# Registry\n\n"+
		"## Active Standards\n\n"+
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |\n"+
		"|---|---|---|---|---|---|---|---|---|\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_status.md"), ""+
		"# Spec Status\n\n"+
		"## Formal Objects\n\n"+
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n"+
		"|---|---|---|---|---|---|---|\n")

	scope, err := CollectDefaultSpecFlowDesignScope(repoRoot)
	if err != nil {
		t.Fatalf("CollectDefaultSpecFlowDesignScope: %v", err)
	}
	if !containsString(scope.TemplateGovernanceFiles, "specflow/templates/docs/specs/_governance_review/README.md") {
		t.Fatalf("expected governance review process contract in design scope, got %+v", scope.TemplateGovernanceFiles)
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

func writeGuidanceSkillFiles(t *testing.T, repoRoot string) {
	t.Helper()
	for _, relPath := range []string{
		"specflow/framework/skills/using-specflow-guidance/SKILL.md",
		"specflow/framework/skills/project-framing/SKILL.md",
		"specflow/framework/skills/scope-cutting/SKILL.md",
		"specflow/framework/skills/solution-design/SKILL.md",
		"specflow/framework/skills/design-quality-review/SKILL.md",
		"specflow/framework/skills/spec-writeback-guidance/SKILL.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# skill\n")
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

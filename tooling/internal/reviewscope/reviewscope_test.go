package reviewscope

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectDefaultSpecFlowScopeExcludesInvalidRegistryEntryFromGovernanceInputs(t *testing.T) {
	repoRoot := t.TempDir()
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines/spec_flow_review.md"), "# review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines/tooling_execution_policy.md"), "# tooling\n")
	for _, name := range []string{
		"shared_ops.md",
		"shared_new.md",
		"shared_extract.md",
		"shared_bind.md",
		"shared_topology.md",
		"shared_sync.md",
		"shared_escape.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines", name), "# "+name+"\n")
	}
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines/commands/cand_check.md"), "# cand_check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_status.md"), "# status\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_check_result/README.md"), "# check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_plans/README.md"), "# plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_plans/draft/README.md"), "# draft plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_plans/active/README.md"), "# active plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_verify_result/README.md"), "# verify\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/project_standards/_registry.md"), "# template registry\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/AGENTS.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/GEMINI.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/CLAUDE.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_status.md"), "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/_registry.md"), ""+
		"# Registry\n\n"+
		"## Active Standards\n\n"+
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |\n"+
		"|---|---|---|---|---|---|---|---|---|\n"+
		"| `bad_prompt_rule` | `review_standard` | `candidate_closure_review` | `docs/project_standards/prompt_guidelines.md` | `cand_check` | `review_scenario:not_supported_here` | `tighten` | `framework_wins` | invalid scenario |\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/prompt_guidelines.md"), "# prompt\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/README.md"), "# tooling\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/cmd/specflowctl/main.go"), "package main\nfunc main() {}\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/internal/demo/demo.go"), "package demo\nfunc Value() string { return \"demo\" }\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module example.com/specflow\n\ngo 1.22\n")

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
}

func TestCollectDefaultSpecFlowScopeExcludesUnsupportedSpecFlowReviewEntry(t *testing.T) {
	repoRoot := t.TempDir()
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines/spec_flow_review.md"), "# review\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines/tooling_execution_policy.md"), "# tooling\n")
	for _, name := range []string{
		"shared_ops.md",
		"shared_new.md",
		"shared_extract.md",
		"shared_bind.md",
		"shared_topology.md",
		"shared_sync.md",
		"shared_escape.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines", name), "# "+name+"\n")
	}
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines/commands/cand_check.md"), "# cand_check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_status.md"), "# status\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_check_result/README.md"), "# check\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_plans/README.md"), "# plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_plans/draft/README.md"), "# draft plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_plans/active/README.md"), "# active plans\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/specs/_verify_result/README.md"), "# verify\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/docs/project_standards/_registry.md"), "# template registry\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/AGENTS.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/GEMINI.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/templates/root/CLAUDE.md"), "host\n<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_status.md"), "# Spec Status\n\n## Formal Modules\n\n| Module | Stable | Candidate | Active Layer | Next Command | Notes |\n|---|---|---|---|---|---|\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/_registry.md"), ""+
		"# Registry\n\n"+
		"## Active Standards\n\n"+
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |\n"+
		"|---|---|---|---|---|---|---|---|---|\n"+
		"| `bad_overlay_rule` | `review_standard` | `governance_baseline_review` | `docs/project_standards/prompt_guidelines.md` | `spec_flow_review` | `review_scenario:` | `tighten` | `framework_wins` | malformed selector |\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/prompt_guidelines.md"), "# prompt\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/README.md"), "# tooling\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/cmd/specflowctl/main.go"), "package main\nfunc main() {}\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/internal/demo/demo.go"), "package demo\nfunc Value() string { return \"demo\" }\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/go.mod"), "module example.com/specflow\n\ngo 1.22\n")

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

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

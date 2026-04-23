package projectstandards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRegistryPassesForKnownCandCheckEntry(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/project_standards"), 0o755); err != nil {
		t.Fatalf("mkdir project standards: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}

	status := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `yes` | `candidate` | `module_check` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/specs/_status.md"), []byte(status), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	registry := strings.Join([]string{
		"# 项目标准注册表",
		"",
		"## Active Standards",
		"",
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |",
		"|---|---|---|---|---|---|---|---|---|",
		"| `prompt_rule` | `review_standard` | `candidate_closure_review` | `docs/project_standards/prompt_guidelines.md` | `module_check` | `module:module_ai` | `tighten` | `framework_wins` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/project_standards/_registry.md"), []byte(registry), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/project_standards/prompt_guidelines.md"), []byte("# Prompt"), 0o644); err != nil {
		t.Fatalf("write standard file: %v", err)
	}

	result, err := ValidateRegistry(repoRoot)
	if err != nil {
		t.Fatalf("ValidateRegistry: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", result.Diagnostics)
	}
}

func TestValidateRegistryRejectsUnsupportedScenarioForCandCheck(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/project_standards"), 0o755); err != nil {
		t.Fatalf("mkdir project standards: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}

	status := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Modules",
		"",
		"| Module | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|",
		"| `module_ai` | `yes` | `yes` | `candidate` | `module_check` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/specs/_status.md"), []byte(status), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	registry := strings.Join([]string{
		"# 项目标准注册表",
		"",
		"## Active Standards",
		"",
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |",
		"|---|---|---|---|---|---|---|---|---|",
		"| `prompt_rule` | `review_standard` | `candidate_closure_review` | `docs/project_standards/prompt_guidelines.md` | `module_check` | `review_scenario:default_governance_baseline` | `tighten` | `framework_wins` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/project_standards/_registry.md"), []byte(registry), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/project_standards/prompt_guidelines.md"), []byte("# Prompt"), 0o644); err != nil {
		t.Fatalf("write standard file: %v", err)
	}

	result, err := ValidateRegistry(repoRoot)
	if err != nil {
		t.Fatalf("ValidateRegistry: %v", err)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics, got none")
	}
}

func TestValidateRegistryRejectsSpecFlowReviewEntryWithoutDeclaredSurface(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/project_standards"), 0o755); err != nil {
		t.Fatalf("mkdir project standards: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "docs/specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}

	status := strings.Join([]string{
		"# Spec Status",
		"",
		"## Formal Objects",
		"",
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |",
		"|---|---|---|---|---|---|---|",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/specs/_status.md"), []byte(status), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}

	registry := strings.Join([]string{
		"# 项目标准注册表",
		"",
		"## Active Standards",
		"",
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |",
		"|---|---|---|---|---|---|---|---|---|",
		"| `review_overlay` | `review_standard` | `governance_baseline_review` | `docs/project_standards/prompt_guidelines.md` | `spec_flow_review` | `all_targets_on_surface` | `tighten` | `framework_wins` | note |",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/project_standards/_registry.md"), []byte(registry), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "docs/project_standards/prompt_guidelines.md"), []byte("# Prompt"), 0o644); err != nil {
		t.Fatalf("write standard file: %v", err)
	}

	result, err := ValidateRegistry(repoRoot)
	if err != nil {
		t.Fatalf("ValidateRegistry: %v", err)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics, got none")
	}
}

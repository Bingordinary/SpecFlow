package rulebinding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/testfixtures"
)

func TestResolveRefRequiresPromotionOwnerUnitWhenCandidateSharedHasStableSibling(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/stable"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/candidate"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: stable
rule_version: 0.1.0
---

# Stable
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/candidate/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: candidate
rule_version: 0.2.0
---

# Candidate
`)

	_, err := ResolveRef(repoRoot, "candidate", "b_rule_demo")
	if err == nil || !strings.Contains(err.Error(), "missing promotion_owner_unit") {
		t.Fatalf("expected missing promotion_owner_unit error, got %v", err)
	}
}

func TestResolveRefAcceptsPromotionOwnerUnitWhenCandidateSharedHasStableSibling(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/stable"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/units/stable"))

	// Create spec file for promotion_owner_unit (file existence is state)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/units/stable/s_unit_demo.md"), "---\nid: demo\nlayer: stable\nversion: 1.0.0\n---\n# Demo\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: stable
rule_version: 0.1.0
---

# Stable
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/candidate/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: candidate
rule_version: 0.2.0
promotion_owner_unit: demo
---

# Candidate
`)

	resolved, err := ResolveRef(repoRoot, "candidate", "b_rule_demo")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}
	if resolved.RuleID != "shared_demo" {
		t.Fatalf("unexpected resolved ref: %+v", resolved)
	}
}

func TestResolveRefBareRefCandidateFirst(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/stable"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/candidate"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/units/stable"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/units/stable/s_unit_demo.md"), "---\nid: demo\nlayer: stable\nversion: 1.0.0\n---\n# Demo\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: stable
rule_version: 0.1.0
---

# Stable
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/candidate/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: candidate
rule_version: 0.2.0
promotion_owner_unit: demo
---

# Candidate
`)

	resolved, err := ResolveRef(repoRoot, "candidate", "b_rule_demo")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}
	if resolved.Layer != "candidate" {
		t.Fatalf("expected candidate layer, got %q", resolved.Layer)
	}
	if resolved.RuleVersion != "0.2.0" {
		t.Fatalf("expected candidate version 0.2.0, got %q", resolved.RuleVersion)
	}
}

func TestResolveRefBareRefFallbackToStable(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/stable"))

	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: stable
rule_version: 0.1.0
---

# Stable
`)

	resolved, err := ResolveRef(repoRoot, "candidate", "b_rule_demo")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}
	if resolved.Layer != "stable" {
		t.Fatalf("expected fallback to stable layer, got %q", resolved.Layer)
	}
	if resolved.RuleVersion != "0.1.0" {
		t.Fatalf("expected stable version 0.1.0, got %q", resolved.RuleVersion)
	}
}

func TestResolveRefBareRefStableLayerOnly(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/stable"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/candidate"))

	// Both exist, but stable moduleLayer must only use stable
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/stable/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: stable
rule_version: 0.1.0
---

# Stable
`)
	mustWriteFile(t, filepath.Join(repoRoot, "docs/specs/rules/candidate/b_rule_demo.md"), `---
rule_id: shared_demo
rule_scope: bound
layer: candidate
rule_version: 0.2.0
---

# Candidate
`)

	resolved, err := ResolveRef(repoRoot, "stable", "b_rule_demo")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}
	if resolved.Layer != "stable" {
		t.Fatalf("expected stable layer, got %q", resolved.Layer)
	}
	if resolved.RuleVersion != "0.1.0" {
		t.Fatalf("expected stable version 0.1.0, got %q", resolved.RuleVersion)
	}
}

func TestResolveRefBareRefNotFound(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/stable"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs/specs/rules/candidate"))

	_, err := ResolveRef(repoRoot, "candidate", "b_rule_nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent rule")
	}
}

func TestSplitVersionRefRejectsVersionedRef(t *testing.T) {
	_, _, err := splitVersionRef("b_rule_demo@0.2.0")
	if err == nil {
		t.Fatal("expected error for versioned ref")
	}
	if !strings.Contains(err.Error(), "must not include a version number") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	content = testfixtures.NormalizeSpecFlowContent(path, content)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

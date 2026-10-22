package specpaths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTargetName(t *testing.T) {
	valid := []string{"auth", "user_auth", "unit-2", "A1"}
	for _, name := range valid {
		if err := ValidateTargetName("unit", name); err != nil {
			t.Fatalf("expected unit name %q to be valid, got %v", name, err)
		}
		if err := ValidateTargetName("rule", name); err != nil {
			t.Fatalf("expected rule id %q to be valid, got %v", name, err)
		}
	}

	invalid := []string{"", "..", "a/b", `a\b`, "../auth", "auth/../x", "with space", " lead", "-lead", ".dot", "a:b"}
	for _, name := range invalid {
		if err := ValidateTargetName("unit", name); err == nil {
			t.Fatalf("expected unit name %q to be rejected", name)
		}
		if err := ValidateTargetName("rule", name); err == nil {
			t.Fatalf("expected rule id %q to be rejected", name)
		}
	}

	if err := ValidateTargetName("other", "auth"); err == nil {
		t.Fatal("expected an unknown target kind to be rejected")
	}
}

func TestResolveRuleFileUsesApplicabilityLayer(t *testing.T) {
	repoRoot := t.TempDir()
	writeRule := func(t *testing.T, ref, content string) string {
		t.Helper()
		path := filepath.Join(repoRoot, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(path)
	}

	globalStable := writeRule(t, RuleStableFileRef("g_rule_http"), "stable global\n")
	writeRule(t, RuleCandidateFileRef("g_rule_http"), "candidate global\n")
	if got := ResolveRuleFile(repoRoot, "g_rule_http"); got != globalStable {
		t.Fatalf("global rule resolved to %q, want stable %q", got, globalStable)
	}

	writeRule(t, RuleCandidateFileRef("g_rule_draft"), "candidate-only global\n")
	if got := ResolveRuleFile(repoRoot, "g_rule_draft"); got != "" {
		t.Fatalf("candidate-only global rule resolved to %q, want unresolved", got)
	}

	writeRule(t, RuleStableFileRef("b_rule_http"), "stable bound\n")
	boundCandidate := writeRule(t, RuleCandidateFileRef("b_rule_http"), "candidate bound\n")
	if got := ResolveRuleFile(repoRoot, "b_rule_http"); got != boundCandidate {
		t.Fatalf("bound rule resolved to %q, want candidate %q", got, boundCandidate)
	}
}

// LogicalUnitRefForPath is the inverse of the unit/appendix resolvers: a
// physical spec path must map to the one logical spelling the run's evidence
// carries, so a layer move cannot dangle it.
func TestLogicalUnitRefForPath(t *testing.T) {
	repoRoot := t.TempDir()
	write := func(t *testing.T, ref, content string) {
		t.Helper()
		path := filepath.Join(repoRoot, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(t, "docs/specs/units/candidate/unit_dep.md", "---\nid: dep\nrule_refs: none\n---\n")
	write(t, "docs/specs/units/stable/unit_dep.md", "---\nid: dep\nrule_refs: none\n---\n")
	write(t, "docs/specs/units/candidate/appendix/unit_dep_protocol.md", "---\nunit: dep\n---\n\nProse.\n")
	write(t, "docs/specs/units/candidate/appendix/unit_dep_bad.md", "---\nunit: other\n---\n\nProse.\n")

	ok := []struct{ path, want string }{
		{"docs/specs/units/candidate/unit_dep.md", "unit:dep"},
		{"docs/specs/units/stable/unit_dep.md", "unit:dep"},
		{"docs/specs/units/candidate/appendix/unit_dep_protocol.md", "unit:dep:appendix:unit_dep_protocol"},
	}
	for _, tc := range ok {
		got, mapped := LogicalUnitRefForPath(repoRoot, tc.path)
		if !mapped || got != tc.want {
			t.Fatalf("LogicalUnitRefForPath(%q) = %q, %v; want %q, true", tc.path, got, mapped, tc.want)
		}
	}

	notMapped := []string{
		"",
		"src/dep.go",
		"docs/specs/units/other/unit_dep.md",
		"docs/specs/units/candidate/unit_.md",
		"docs/specs/units/candidate/unit_dep.txt",
		"docs/specs/units/candidate/appendix/unit_dep_bad.md",
		"docs/specs/units/candidate/appendix/missing.md",
		"docs/specs/rules/candidate/b_rule_x.md",
	}
	for _, p := range notMapped {
		if got, mapped := LogicalUnitRefForPath(repoRoot, p); mapped {
			t.Fatalf("LogicalUnitRefForPath(%q) = %q, want no mapping", p, got)
		}
	}
}

func TestIsSpecDocumentPath(t *testing.T) {
	specPaths := []string{
		SpecRootDir,
		"docs/specs/units/stable/unit_agent.md",
		"docs/specs/units/candidate/appendix/unit_agent_flows.md",
		"docs/specs/rules/stable/g_rule_scope.md",
		"docs/specs/meta/validation/unit/agent/verify_result.md",
	}
	for _, p := range specPaths {
		if !IsSpecDocumentPath(p) {
			t.Fatalf("expected %q to be a spec document path", p)
		}
	}

	codePaths := []string{
		"",
		"internal/demo/a.go",
		"docs/design.md",
		"docs/specs_extra/x.md",
		"framework/spec_writing_guide.md",
	}
	for _, p := range codePaths {
		if IsSpecDocumentPath(p) {
			t.Fatalf("expected %q not to be a spec document path", p)
		}
	}
}

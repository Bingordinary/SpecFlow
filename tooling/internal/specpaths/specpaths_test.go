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

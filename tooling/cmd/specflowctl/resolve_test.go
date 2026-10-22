package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeResolveSpec(t *testing.T, repoRoot, relPath string) {
	t.Helper()
	path := filepath.Join(repoRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nrule_id: x\n---\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runResolveFor(t *testing.T, repoRoot, target string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := runResolve([]string{"--repo-root", repoRoot, "--target", target}, &stdout, &stderr)
	return stdout.String(), err
}

func TestResolveUnitAcrossLayersIsOneMatch(t *testing.T) {
	repoRoot := t.TempDir()
	writeResolveSpec(t, repoRoot, "docs/specs/units/candidate/unit_auth.md")
	writeResolveSpec(t, repoRoot, "docs/specs/units/stable/unit_auth.md")

	out, err := runResolveFor(t, repoRoot, "auth")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	for _, want := range []string{"type: unit", "docs/specs/units/candidate/unit_auth.md", "docs/specs/units/stable/unit_auth.md"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestResolvePrefersExactRuleOverStage3(t *testing.T) {
	repoRoot := t.TempDir()
	writeResolveSpec(t, repoRoot, "docs/specs/rules/candidate/b_rule_runtime_model.md")

	out, err := runResolveFor(t, repoRoot, "runtime_model")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !strings.Contains(out, "type: rule") || !strings.Contains(out, "rule_id: b_rule_runtime_model") {
		t.Fatalf("expected exact rule resolution via the directory fallback, got:\n%s", out)
	}
}

func TestResolveUnitAndRuleSameTextIsAmbiguous(t *testing.T) {
	repoRoot := t.TempDir()
	writeResolveSpec(t, repoRoot, "docs/specs/units/candidate/unit_auth.md")
	writeResolveSpec(t, repoRoot, "docs/specs/rules/stable/g_rule_auth.md")

	out, err := runResolveFor(t, repoRoot, "auth")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected an ambiguous-target error, got err=%v output:\n%s", err, out)
	}
	if !strings.Contains(out, "type: ambiguous") || !strings.Contains(out, "unit_auth.md") || !strings.Contains(out, "g_rule_auth.md") {
		t.Fatalf("expected every matched path listed for the ambiguous case, got:\n%s", out)
	}
}

func TestResolveRulePrefixedWithoutFilesIsNewTarget(t *testing.T) {
	repoRoot := t.TempDir()

	out, err := runResolveFor(t, repoRoot, "g_rule_fresh_rule")
	if err != nil {
		t.Fatalf("a new rule target must resolve without error, got: %v", err)
	}
	if !strings.Contains(out, "type: rule") || !strings.Contains(out, "rule_id: g_rule_fresh_rule") || !strings.Contains(out, "new target") {
		t.Fatalf("expected a new-target rule classification, got:\n%s", out)
	}
}

func TestResolvePrefixedRuleFindsExistingFile(t *testing.T) {
	repoRoot := t.TempDir()
	writeResolveSpec(t, repoRoot, "docs/specs/rules/candidate/b_rule_auth.md")
	writeResolveSpec(t, repoRoot, "docs/specs/rules/stable/b_rule_auth.md")

	out, err := runResolveFor(t, repoRoot, "b_rule_auth")
	if err != nil {
		t.Fatalf("an existing rule-prefixed target must resolve, got: %v\n%s", err, out)
	}
	for _, want := range []string{"type: rule", "rule_id: b_rule_auth", "b_rule_auth.md"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "new target") || strings.Contains(out, "matches: none") {
		t.Fatalf("an existing rule must not be reported as a new target with no files:\n%s", out)
	}
}

func TestResolvePrefixlessWithoutAnyFileIsNotFound(t *testing.T) {
	repoRoot := t.TempDir()

	out, err := runResolveFor(t, repoRoot, "ghost")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected a not-found error, got err=%v output:\n%s", err, out)
	}
	if !strings.Contains(out, "type: not_found") {
		t.Fatalf("expected type not_found, got:\n%s", out)
	}
}

func TestResolveMultipleSubstringMatchesAreAmbiguous(t *testing.T) {
	repoRoot := t.TempDir()
	writeResolveSpec(t, repoRoot, "docs/specs/rules/candidate/b_rule_auth_login.md")
	writeResolveSpec(t, repoRoot, "docs/specs/rules/stable/g_rule_auth_token.md")

	out, err := runResolveFor(t, repoRoot, "auth")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected an ambiguous-target error, got err=%v output:\n%s", err, out)
	}
	if !strings.Contains(out, "b_rule_auth_login.md") || !strings.Contains(out, "g_rule_auth_token.md") {
		t.Fatalf("expected both substring matches listed, got:\n%s", out)
	}
}

func TestResolveRejectsUnsafeTargetText(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := runResolveFor(t, repoRoot, "../escape"); err == nil {
		t.Fatal("expected target-name validation to reject a traversal segment")
	}
	var stdout, stderr bytes.Buffer
	if err := runResolve([]string{"--repo-root", repoRoot}, &stdout, &stderr); err == nil {
		t.Fatal("expected a usage error without --target")
	}
}

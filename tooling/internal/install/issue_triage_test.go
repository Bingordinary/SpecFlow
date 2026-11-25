package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIssueTriageLayoutContract guards the dual-layout issue-triage protocol
// routed by the bootstrap: installed projects compare issues against stable
// specs and product code, source-repo runs compare framework documents with
// tooling behavior, upstream findings stay out of the consumer repository, and
// the selection and prompt contracts stay complete (bounded list fetch, a
// repository-level duplicate index, resolved protocol path, label names with
// spaces, and the full layout/evidence glossary).
func TestIssueTriageLayoutContract(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	protocol, err := os.ReadFile(filepath.Join(repoRoot, "framework", "operations", "issues.md"))
	if err != nil {
		t.Fatalf("read framework/operations/issues.md: %v", err)
	}
	protocolText := string(protocol)

	for _, required := range []string{
		"`issue@{n|selector}`",
		"`issue@open`",
		"`issue@all`",
		"`issue@closed`",
		"`issue@label:<name>`",
		"`spec_flow_issues`",
		"source_repo",
		"installed_project",
		"stable spec",
		"design gap",
		"docs/specs/",
		"UPSTREAM",
		"specflow/",
		"read-only",
		"REAL-bug",
		"REAL-design",
		"NOT-ISSUE",
		"DUPLICATE",
		"ENHANCEMENT",
		"user confirmation",
		"untrusted",
		"--limit 1000",
		"at most 1000",
		"<protocol path>",
		"specflow/framework/operations/issues.md",
		"except ENHANCEMENT",
		"--label \"<name>\"",
		"spaces included",
		"`layout`",
		"Entry and Layout section",
		"tooling/internal/",
		"`recorded intent`",
		"`actual behavior`",
		"`implementation source`",
		"Evidence roots table",
		"Actual behavior (implementation source)",
		"never treat",
		"protocol and grading reference",
		"--state all --limit 1000 --json number,title",
		"repository-level",
		"duplicate detection is bounded",
	} {
		requireText(t, protocolText, required)
	}

	for _, forbidden := range []string{
		"use them only to establish whether the issue is UPSTREAM",
		"When no index is supplied",
	} {
		if strings.Contains(protocolText, forbidden) {
			t.Fatalf("framework/operations/issues.md must not restore the superseded contract %q", forbidden)
		}
	}

	commands, err := os.ReadFile(filepath.Join(repoRoot, "framework", "commands.md"))
	if err != nil {
		t.Fatalf("read framework/commands.md: %v", err)
	}
	requireText(t, string(commands), "`issue@{n}` / `issue@{selector}`")
	requireText(t, string(commands), "`installed_project` additionally classifies behavior originating in SpecFlow itself as UPSTREAM")
	if strings.Contains(string(commands), "ENHANCEMENT, or UPSTREAM") {
		t.Fatalf("framework/commands.md must not list UPSTREAM as a general issue class")
	}

	reviewStandard, err := os.ReadFile(filepath.Join(repoRoot, "framework", "spec_flow_review.md"))
	if err != nil {
		t.Fatalf("read framework/spec_flow_review.md: %v", err)
	}
	for _, required := range []string{
		"assemble both triage prompt variants",
		"`installed_project` variant",
		"with the installed evidence roots",
	} {
		requireText(t, string(reviewStandard), required)
	}
	if strings.Contains(string(reviewStandard), "when both are in scope") {
		t.Fatalf("framework/spec_flow_review.md must not restore the undefined Exercise D condition")
	}
}

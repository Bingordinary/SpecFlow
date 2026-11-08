package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validatekeys"
)

const validateKeysFixture = `---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

### Authentication model
Email/password login issues a token.

## Promises

promises:
- id: auth.login_api
  summary: Login returns a token.
  source: [Authentication model]
  contract:
    operations:
      login:
        outputs:
          token: {type: string}

## Acceptance Items

acceptance_item_set:
- id: AUTH-AC-001
  promise: auth.login_api
  source: [Authentication model]
  description: Given a user, When login, Then a token.
  pass_condition: 200 with a token.
`

func TestRunValidateKeysJSON(t *testing.T) {
	root := t.TempDir()
	ref := specpaths.CandidateUnitSpecFileRef("auth")
	path := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(validateKeysFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := runValidateKeys([]string{"--repo-root", root, "--unit", "auth", "--format", "json"}, &stdout, &stderr); err != nil {
		t.Fatalf("runValidateKeys: %v; stderr=%s", err, stderr.String())
	}
	var catalog validatekeys.Catalog
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatalf("decode catalog: %v; output=%s", err, stdout.String())
	}
	if catalog.Unit != "auth" || catalog.Items != 1 || catalog.Promises != 1 {
		t.Fatalf("unexpected catalog summary: %+v", catalog)
	}
	if len(catalog.Keys) == 0 {
		t.Fatal("expected at least one key")
	}
}

// TestRunValidateKeysIncludesStableGlobalRules pins the diagnostic to the
// planner's catalog derivation: stable global rules apply by default even
// though they are never declared in rule_refs, and candidate global drafts do
// not constrain the unit.
func TestRunValidateKeysIncludesStableGlobalRules(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(specpaths.CandidateUnitSpecFileRef("auth"), validateKeysFixture)
	write(specpaths.RuleStableFileRef("g_rule_http"), "---\nrule_id: g_rule_http\nrule_scope: global\n---\nGlobal constraint.\n")
	write(specpaths.RuleCandidateFileRef("g_rule_draft"), "---\nrule_id: g_rule_draft\nrule_scope: global\n---\nDraft.\n")

	var stdout, stderr bytes.Buffer
	if err := runValidateKeys([]string{"--repo-root", root, "--unit", "auth", "--format", "json"}, &stdout, &stderr); err != nil {
		t.Fatalf("runValidateKeys: %v; stderr=%s", err, stderr.String())
	}
	var catalog validatekeys.Catalog
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatalf("decode catalog: %v; output=%s", err, stdout.String())
	}
	byID := map[string]validatekeys.Key{}
	for _, key := range catalog.Keys {
		byID[key.ID] = key
	}
	interactions, ok := byID["global:rule-interactions"]
	if !ok {
		t.Fatalf("catalog is missing global:rule-interactions: %+v", catalog.Keys)
	}
	if !stringInList(interactions.Reads, "rule:g_rule_http") {
		t.Fatalf("global:rule-interactions must read the stable global rule, got: %v", interactions.Reads)
	}
	for id := range byID {
		if strings.Contains(id, "g_rule_draft") {
			t.Fatalf("candidate-only global draft must not enter the catalog: %s", id)
		}
	}
}

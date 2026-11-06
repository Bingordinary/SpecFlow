package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

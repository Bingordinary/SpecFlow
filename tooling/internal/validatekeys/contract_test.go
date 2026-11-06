package validatekeys

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEmbeddedContractMatchesFramework keeps the runtime copy aligned with the
// normative framework manifest. Editing the YAML is enough; run `go generate
// ./internal/validatekeys` (or copy the file) to refresh the embedded copy.
func TestEmbeddedContractMatchesFramework(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "framework", "unit_validate_checks.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(embeddedContract) {
		t.Fatal("framework/unit_validate_checks.yaml and internal/validatekeys/contract.yaml differ; run go generate ./internal/validatekeys")
	}
}

// TestContractGroups pins the coverage grouping. A grouping change is a
// contract edit, not a Go edit.
func TestContractGroups(t *testing.T) {
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"document":   "1,10",
		"design":     "2,3,4",
		"acceptance": "5",
		"external":   "6,7,8,9",
	}
	got := map[string]string{}
	for _, group := range contract.Groups {
		got[group.ID] = strings.Join(group.Checks, ",")
	}
	for group, checks := range want {
		if got[group] != checks {
			t.Fatalf("group %s = %q, want %q", group, got[group], checks)
		}
	}
}

// TestContractChecksHaveChecklistSections keeps the machine contract and the
// prose checklist from drifting: every contract check must have a Check
// section in the unit validate checklist.
func TestContractChecksHaveChecklistSections(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "framework", "unit_validate_checklist.md"))
	if err != nil {
		t.Fatal(err)
	}
	checklist := string(data)
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range contract.Checks {
		marker := "## Check " + check.ID + " —"
		if !strings.Contains(checklist, marker) {
			t.Fatalf("contract check %s has no %q section in framework/unit_validate_checklist.md", check.ID, marker)
		}
	}
}

// TestChecklistChecksHaveContractEntries keeps the reverse direction aligned:
// every gate-check heading in the prose checklist must have a contract entry,
// so a new or renamed check cannot exist on only one side.
func TestChecklistChecksHaveContractEntries(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "framework", "unit_validate_checklist.md"))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, check := range contract.Checks {
		declared[check.ID] = true
	}
	headings := regexp.MustCompile(`(?m)^## Check ([0-9]+) — `).FindAllStringSubmatch(string(data), -1)
	if len(headings) == 0 {
		t.Fatal("framework/unit_validate_checklist.md has no gate check headings")
	}
	for _, heading := range headings {
		if !declared[heading[1]] {
			t.Fatalf("checklist check %s has no contract entry", heading[1])
		}
	}
}

// TestBuildCatalogFromContract pins the generic object generation: the
// contract, not a Go switch, decides the keys and read sets.
func TestBuildCatalogFromContract(t *testing.T) {
	g, err := Parse("auth", fixtureSpec)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := DefaultContract()
	if err != nil {
		t.Fatal(err)
	}
	catalog := BuildCatalog(g, CatalogInput{Unit: "auth", Rules: []string{"g_rule_global"}, EvidenceBase: "unit_auth"}, contract)
	byID := map[string]Key{}
	for _, key := range catalog.Keys {
		byID[key.ID] = key
	}
	for _, id := range []string{
		"section:Authentication model:structure",
		"item:AUTH-AC-001:affects",
		"promise:auth.login_api:coverage",
		"rule:b_rule_x:section:Authentication model",
		"rule:g_rule_global:section:Authentication model",
		"rule:g_rule_global:evidence",
		"global:rule-interactions",
		"global:surface-associations",
	} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("catalog is missing %q", id)
		}
	}
	rule := byID["rule:b_rule_x:section:Authentication model"]
	if !contains(rule.Reads, "rule:b_rule_x") || !contains(rule.Reads, "section:Authentication model") {
		t.Fatalf("rule object reads = %v", rule.Reads)
	}
	if byID["global:graph"].Check != "1" {
		t.Fatalf("global:graph check = %q, want 1 (the mechanical narrative check is not a session check)", byID["global:graph"].Check)
	}
}

// TestContractReadmeKinds pins the vocabulary the generic engine supports.
// A new kind or read is a deliberate engine change, not a checklist edit.
func TestContractReadmeKinds(t *testing.T) {
	if len(validKinds) != 10 || len(validReads) != 22 {
		t.Fatalf("selector vocabulary changed: kinds=%d reads=%d — update the contract documentation and this test deliberately", len(validKinds), len(validReads))
	}
}

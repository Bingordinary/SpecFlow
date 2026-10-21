package judgments

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestProtocolExcludesDeclaredSectionSubtree(t *testing.T) {
	document := strings.Join([]string{
		"# Title",
		"",
		"## Judging",
		"",
		"A judgment stands.",
		"",
		"### Mechanism",
		"",
		"Runs are planned here.",
		"",
		"#### Nested",
		"",
		"Still mechanism.",
		"",
		"## Other judging",
		"",
		"Another judgment stands.",
		"",
	}, "\n")
	got, err := withoutMechanismSections(document, []string{"Mechanism"})
	if err != nil {
		t.Fatal(err)
	}
	for _, dropped := range []string{"Runs are planned here.", "Still mechanism.", "### Mechanism"} {
		if strings.Contains(got, dropped) {
			t.Fatalf("mechanism content %q survived", dropped)
		}
	}
	for _, kept := range []string{"A judgment stands.", "Another judgment stands.", "## Judging", "## Other judging"} {
		if !strings.Contains(got, kept) {
			t.Fatalf("judging content %q was dropped", kept)
		}
	}
}

func TestProtocolRejectsUnresolvableMechanismSections(t *testing.T) {
	document := "# Title\n\n## Judging\n\nA judgment stands.\n\n## Mechanism\n\nRuns are planned here.\n"
	if _, err := withoutMechanismSections(document, []string{"Absent"}); err == nil {
		t.Fatal("a mechanism section that does not exist was accepted")
	}
	if _, err := withoutMechanismSections(document, []string{"Mechanism", "Mechanism"}); err == nil {
		t.Fatal("a duplicated mechanism section was accepted")
	}
	if _, err := withoutMechanismSections(document, []string{"Title"}); err == nil {
		t.Fatal("excluding the whole document was accepted")
	}
}

func TestProtocolIgnoresFencedHeadings(t *testing.T) {
	document := "# Title\n\n## Mechanism\n\n```\n# not a heading\n```\n\n## Judging\n\nA judgment stands.\n"
	got, err := withoutMechanismSections(document, []string{"Mechanism"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "A judgment stands.") {
		t.Fatal("judging content was dropped with the fenced block")
	}
}

// sourceRepoRoot is the repository this package is developed in. Protocol
// resolves every declared mechanism section there, so a renamed heading fails
// here instead of silently widening the fingerprint.
func sourceRepoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..")
}

// sortedStandardKeys returns every declared gate standard in a deterministic
// order so a failure names one gate/target pair instead of a random one.
func sortedStandardKeys() []standardKey {
	keys := make([]standardKey, 0, len(protocolStandards))
	for key := range protocolStandards {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].gate != keys[j].gate {
			return keys[i].gate < keys[j].gate
		}
		return keys[i].targetKind < keys[j].targetKind
	})
	return keys
}

func TestProtocolResolvesDeclaredMechanismSections(t *testing.T) {
	root := sourceRepoRoot(t)
	for _, key := range sortedStandardKeys() {
		if _, err := Protocol(root, key.gate, key.targetKind); err != nil {
			t.Fatalf("%s/%s: %v", key.gate, key.targetKind, err)
		}
	}
}

func TestProtocolRejectsUnknownGateStandard(t *testing.T) {
	if _, err := Protocol(sourceRepoRoot(t), "verify", "rule"); err == nil {
		t.Fatal("a gate/target pair with no declared standard was accepted")
	}
}

// A named section is excluded with its whole subtree. A heading nested inside
// an excluded section but not itself declared would be pulled out of the
// fingerprint without ever being classified — the one failure direction that
// silently lets a changed judging standard reuse recorded conclusions.
func TestProtocolExcludedSectionsNestOnlyExcludedSections(t *testing.T) {
	root := sourceRepoRoot(t)
	documents := []string{}
	seen := map[string]bool{}
	for _, key := range sortedStandardKeys() {
		for _, document := range protocolStandards[key] {
			if seen[document] {
				continue
			}
			seen[document] = true
			documents = append(documents, document)
		}
	}
	sort.Strings(documents)
	for _, document := range documents {
		data, err := os.ReadFile(filepath.Join(root, "framework", document))
		if err != nil {
			t.Fatal(err)
		}
		sections := documentSections(strings.Split(string(data), "\n"))
		declared := map[string]bool{}
		for _, name := range mechanismSections[document] {
			declared[name] = true
		}
		for _, section := range sections {
			if !declared[section.text] {
				continue
			}
			for _, nested := range sections {
				if nested.line <= section.line || nested.line >= section.end {
					continue
				}
				if !declared[nested.text] {
					t.Errorf("%s: excluded section %q nests %q, which is neither declared nor judged", document, section.text, nested.text)
				}
			}
		}
	}
}

func TestProtocolExcludesMechanismUpdates(t *testing.T) {
	root := sourceRepoRoot(t)
	framework := filepath.Join(root, "framework")
	cases := []struct {
		document  string
		mechanism []string
		judging   []string
	}{
		{
			// #84: a pure-mechanism framework update must not invalidate every
			// recorded conclusion.
			document: "unit_verify_checklist.md",
			mechanism: []string{
				"- `Change review:` — **validate** delta/repair runs only",
				"{gate}@{target} · {mode} · {layer}",
			},
			judging: []string{
				"## Step 1 — Structural alignment",
				"## Step 2 — Acceptance alignment",
				"## Core Principle",
				"**Verdict folding:**",
				"### 6. Severity Levels",
				"#### P3 Reportability Gate",
				"### Direction table",
				"## Step 5 — Implementation integrity & surplus detection",
			},
		},
		{
			// #84: a pure-mechanism framework update must not invalidate every
			// recorded conclusion.
			document: "verification_scope.md",
			mechanism: []string{
				"**Delta and repair scope is derived.**",
			},
			judging: []string{
				"## Coverage Model",
				"### Input roles",
				"### Relationship vocabulary",
				"#### Verify synthesis items",
				"### Deferred findings",
				"### The change review (validate)",
				"## Principles",
			},
		},
		{
			document: "severity_policy.md",
			judging: []string{
				"## 3. Core Principle",
				"### 4.1 `P0`",
				"## 5. Blocking Relationship",
				"### 9.3 Severity Boundaries",
				"## Priority Rules",
				"### 1. Core Principle",
				"#### P3 Reportability Gate",
			},
		},
		{
			document: "shared_judgments.md",
			judging: []string{
				"## File associations",
				"## Required tasks",
				"## Judgment and conclusion rules",
			},
		},
		{
			// The validate checks and the extraction re-verification criteria
			// judge; the report skeleton and the cache-writing mechanics do not.
			document: "unit_validate_checklist.md",
			mechanism: []string{
				"{gate}@{target} · {mode} · {layer}",
				"`gate-finalize` creates `docs/specs/meta/validation/unit/{name}/` as needed",
			},
			judging: []string{
				"## Check 1 — Structural integrity",
				"## Check 5 — Acceptance coverage & correctness",
				"### Sub-check 5h — Promise block semantics",
				"## Check 10 — Clarity",
				"### Severity handling",
				"## Extraction re-verification (Check 2 Step 1, Check 5)",
				"a quote that does not establish the claimed relationship",
			},
		},
		{
			document: "rule_validate_checklist.md",
			mechanism: []string{
				"{gate}@{target} · {mode} · {layer}",
				"the coverage run writes `docs/specs/meta/validation/rule/{id}/validate_result.md`",
			},
			judging: []string{
				"### Check 1 — Frontmatter Completeness",
				"### Check 6 — Rule Body Quality",
				"### Severity handling",
				"## Target-layer applicability",
			},
		},
		{
			// Unit validate's promise standard. Check 5h applies its
			// granularity and contract-shape rules, so the whole document is
			// judging input; no section is declared as mechanism-only.
			document: "core/promise_model.md",
			judging: []string{
				"## Contract shapes",
				"## Granularity",
				"## Done",
				"### Fields",
				"Too coarse: a promise bundling several endpoints",
			},
		},
		{
			// The spec-format contract the validate and verify checks apply.
			// No section of it only describes how a run is planned, recorded,
			// parsed or terminated, so nothing is excluded and every judging
			// passage must survive (§2.17 items 1 and 4).
			document: "spec_writing_guide.md",
			judging: []string{
				"## 4. Unit Dependencies",
				"## 9. Authoring Baseline",
				"### Reader Contract",
				"### Acceptance Item Granularity",
				"## 13. Section Regions",
				"## 14. Abstraction Boundaries and Anti-Hardcoding Guidelines (The Truth Ownership Framework)",
				"validate Check 1 fails the spec on it",
			},
		},
	}
	for _, c := range cases {
		data, err := os.ReadFile(filepath.Join(framework, c.document))
		if err != nil {
			t.Fatal(err)
		}
		criteria, err := withoutMechanismSections(string(data), mechanismSections[c.document])
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range c.mechanism {
			if strings.Contains(criteria, text) {
				t.Errorf("%s: mechanism text %q is inside the fingerprint", c.document, text)
			}
		}
		for _, text := range c.judging {
			if !strings.Contains(criteria, text) {
				t.Errorf("%s: judging text %q left the fingerprint", c.document, text)
			}
		}
	}
}

// TestProtocolBindsEachGateToItsOwnStandard pins the binding: each gate's
// standard covers the documents its own reviewer reads, and no other gate's, so
// changing one checklist invalidates only the conclusions judged by it.
func TestProtocolBindsEachGateToItsOwnStandard(t *testing.T) {
	root := sourceRepoRoot(t)

	want := map[standardKey][]string{
		{gate: "verify", targetKind: "unit"}:   {"unit_verify_checklist.md", "shared_judgments.md", "verification_scope.md", "severity_policy.md", "spec_writing_guide.md"},
		{gate: "validate", targetKind: "unit"}: {"unit_validate_checklist.md", "verification_scope.md", "severity_policy.md", "spec_writing_guide.md", "core/promise_model.md"},
		{gate: "validate", targetKind: "rule"}: {"rule_validate_checklist.md", "verification_scope.md", "severity_policy.md", "spec_writing_guide.md"},
	}
	if len(protocolStandards) != len(want) {
		t.Fatalf("declared standards = %d, want %d", len(protocolStandards), len(want))
	}
	for key, documents := range want {
		got := protocolStandards[key]
		if strings.Join(got, ",") != strings.Join(documents, ",") {
			t.Errorf("%s/%s documents = %v, want %v", key.gate, key.targetKind, got, documents)
		}
		for _, document := range got {
			if _, declared := mechanismSections[document]; !declared {
				t.Errorf("%s/%s lists %s with no declared mechanism sections", key.gate, key.targetKind, document)
			}
		}
	}

	verify := MustProtocol(root, "verify", "unit")
	unitValidate := MustProtocol(root, "validate", "unit")
	ruleValidate := MustProtocol(root, "validate", "rule")
	if verify == unitValidate || verify == ruleValidate || unitValidate == ruleValidate {
		t.Fatal("two gate standards share one fingerprint")
	}
}

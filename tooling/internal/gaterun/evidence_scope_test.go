package gaterun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

const evidenceScopeSpec = `---
id: auth
rule_refs: none
---

# auth

## Design Narrative
### Login flow

The login behavior.

### Logout flow

The logout behavior.

## Promises

promises:
  - id: auth.core
    summary: Core contract.
    source: [Login flow]
    contract: {invariants: [core]}

## Acceptance Items

acceptance_item_set:
  - id: AUTH-1
    promise: auth.core
    source: [Login flow]
    description: Given a caller, When login runs, Then it is accepted.
    verification_type: testable
    verification_surface: api
    implementation_surface: src
    verification_method: test
    pass_condition: Passes.
    runnable: yes
  - id: AUTH-2
    promise: auth.core
    source: [Logout flow]
    description: Given a caller, When logout runs, Then it is accepted.
    verification_type: testable
    verification_surface: api
    implementation_surface: src
    verification_method: test
    pass_condition: Passes.
    runnable: yes
`

func evidenceScopeRecordLarge() string {
	var b strings.Builder
	b.WriteString("---\nunit: auth\n---\n\n# Evidence\n\n## AUTH-1 — Login flow\n\nKeys:\n  - item:AUTH-1\n\n")
	for i := 0; i < 140; i++ {
		fmt.Fprintf(&b, "Login behavior %d keeps token rotation and audit evidence for round %d.\n", i, i*7+3)
	}
	b.WriteString("\n## AUTH-2 — Logout flow\n\nKeys:\n  - item:AUTH-2\n\n")
	for i := 0; i < 140; i++ {
		fmt.Fprintf(&b, "Logout behavior %d keeps session cleanup and audit evidence for round %d.\n", i, i*11+5)
	}
	return b.String()
}

func writeEvidenceScopeRepo(t *testing.T) string {
	t.Helper()
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/main.go", "package main\n")
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", evidenceScopeSpec)
	writeFile(t, repoRoot, "docs/specs/units/candidate/evidence/unit_auth.md", evidenceScopeRecordLarge())
	return repoRoot
}

func writeEvidenceScopeBaseline(t *testing.T, repoRoot string) {
	t.Helper()
	entries := []validationcache.FileEntry{
		evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers()),
		evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/evidence/unit_auth.md", nil),
	}
	writeValidateBaseline(t, repoRoot, entries, "pass", false, "full")
}

func containsRerunObject(run *Run, id string) bool {
	return stringInSlice(run.RerunObjects, id)
}

func hasValidateObject(run *Run, id string) bool {
	for _, object := range run.ValidateObjects {
		if object.ID == id {
			return true
		}
	}
	return false
}

func TestDeltaPlanEvidenceSectionBindsToDesignSection(t *testing.T) {
	repoRoot := writeEvidenceScopeRepo(t)
	full, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !hasValidateObject(full, "section:Login flow:design") {
		t.Fatalf("the full catalog is missing the Login flow design object: %+v", full.ValidateObjects)
	}
	writeEvidenceScopeBaseline(t, repoRoot)

	path := filepath.Join(repoRoot, "docs/specs/units/candidate/evidence/unit_auth.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "Login behavior 60 keeps", "Login behavior 60 changes", 1)
	if updated == string(data) {
		t.Fatal("evidence section edit did not apply")
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("an evidence record section edit must keep the object delta, got mode %q (notices %v)", run.Mode, run.Notices)
	}
	for _, id := range []string{
		"section:Login flow:design",
		"section:Login flow:clarity",
		"item:AUTH-1:evidence",
		"global:goal-means",
	} {
		if !containsRerunObject(run, id) {
			t.Fatalf("%s must re-run after its evidence section changed; rerun=%v", id, run.RerunObjects)
		}
	}
	for _, id := range []string{
		"section:Logout flow:design",
		"section:Logout flow:clarity",
		"item:AUTH-2:evidence",
	} {
		if containsRerunObject(run, id) {
			t.Fatalf("%s must carry when only the AUTH-1 evidence section changed; rerun=%v", id, run.RerunObjects)
		}
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{"design"})
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(spec.CheckKeys, "4") {
		t.Fatalf("the design session must keep Check 4 while its evidence object re-runs; checks=%v", spec.CheckKeys)
	}
}

// TestDeltaPlanDesignSectionChangeRerunsEvidenceCheck pins the Check 4
// residual dependency: when the item's source design section changes, the
// evidence judgment must re-run; when the plan stays a delta, Check 4 must also
// stay in the design session.
func TestDeltaPlanDesignSectionChangeRerunsEvidenceCheck(t *testing.T) {
	repoRoot := writeEvidenceScopeRepo(t)
	writeEvidenceScopeBaseline(t, repoRoot)

	path := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "The login behavior.", "The login behavior now uses SSO.", 1)
	if updated == string(data) {
		t.Fatal("design section edit did not apply")
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		"section:Login flow:design",
		"item:AUTH-1:evidence",
	} {
		if !containsRerunObject(run, id) {
			t.Fatalf("%s must re-run after its source design section changed; rerun=%v", id, run.RerunObjects)
		}
	}
	// A small fixture may upgrade to the full scope because the estimated
	// delta is not cheaper; the object-level dependency must still hold. When
	// the plan does stay a delta, Check 4 must also stay in the session.
	if run.Mode == ModeDelta {
		spec, err := BuildSessionSpec(repoRoot, run, []string{"design"})
		if err != nil {
			t.Fatal(err)
		}
		if !stringInSlice(spec.CheckKeys, "4") {
			t.Fatalf("the design session must keep Check 4 after the source design section changed; checks=%v", spec.CheckKeys)
		}
	}
}

// TestDeltaPlanEvidenceContentChangeRerunsCheckSix pins the Check 6 evidence
// dependency: an evidence-record content change must re-run its
// path-reference object and keep Check 6 in the external session.
func TestDeltaPlanEvidenceContentChangeRerunsCheckSix(t *testing.T) {
	repoRoot := writeEvidenceScopeRepo(t)
	writeEvidenceScopeBaseline(t, repoRoot)

	path := filepath.Join(repoRoot, "docs/specs/units/candidate/evidence/unit_auth.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "Logout behavior 60 keeps", "Logout behavior 60 changed", 1)
	if updated == string(data) {
		t.Fatal("evidence record edit did not apply")
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("an evidence content edit must keep the object delta, got mode %q (notices %v)", run.Mode, run.Notices)
	}
	if !containsRerunObject(run, "evidence:unit_auth:references") {
		t.Fatalf("an evidence content change must re-run its Check 6 reference object; rerun=%v", run.RerunObjects)
	}
	spec, err := BuildSessionSpec(repoRoot, run, []string{"external"})
	if err != nil {
		t.Fatal(err)
	}
	if !stringInSlice(spec.CheckKeys, "6") {
		t.Fatalf("the external session must keep Check 6 after an evidence content change; checks=%v", spec.CheckKeys)
	}
}

// TestDeltaPlanAdvisoryCheckRerunsWholeCheck pins the advisory-carry rule under
// object-level delta: an advisory is bound to its check key, so a check with a
// recorded P2/P3 advisory is atomic — when one of its objects re-runs, every
// object of that check re-runs and the session can re-judge the advisory
// instead of dropping it (framework/validation_cache.md §Advisory carry).
func TestDeltaPlanAdvisoryCheckRerunsWholeCheck(t *testing.T) {
	repoRoot := writeEvidenceScopeRepo(t)
	entries := []validationcache.FileEntry{
		evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", mainCheckMarkers()),
		evidenceEntryWithChecks(t, repoRoot, "docs/specs/units/candidate/evidence/unit_auth.md", nil),
	}
	advisory := Finding{
		ID:        "F-ADV-1",
		Severity:  "P2",
		SourceKey: ClarityCheck,
		Text:      "recorded clarity advisory",
		Detail:    "problem: p\n  evidence: e\n  impact: i\n  fix: f\n",
	}
	writeValidateBaselineFor(t, repoRoot, "auth", entries, "pass", false, "full", advisory)

	path := filepath.Join(repoRoot, "docs/specs/units/candidate/evidence/unit_auth.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "Logout behavior 60 keeps", "Logout behavior 60 changed", 1)
	if updated == string(data) {
		t.Fatal("evidence-record edit did not apply")
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeDelta, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if run.Mode != ModeDelta {
		t.Fatalf("expected a delta plan, got mode %q (notices %v)", run.Mode, run.Notices)
	}
	rerun := map[string]bool{}
	for _, id := range run.RerunObjects {
		rerun[id] = true
	}
	for _, object := range run.ValidateObjects {
		if object.Check != ClarityCheck {
			continue
		}
		if !rerun[object.ID] {
			t.Fatalf("check 10 carries a recorded advisory; object %s must re-run, rerun=%v", object.ID, run.RerunObjects)
		}
	}
	if !strings.Contains(strings.Join(run.Notices, " "), "P2/P3 advisories re-run fully: 10") {
		t.Fatalf("missing advisory atomicity notice, got %v", run.Notices)
	}
}

func TestPlanRejectsEvidenceSectionWithoutKeys(t *testing.T) {
	repoRoot := writeEvidenceScopeRepo(t)
	writeFile(t, repoRoot, "docs/specs/units/candidate/evidence/unit_auth.md", "---\nunit: auth\n---\n\n## No keys\n\nprose\n")
	if _, err := Plan(repoRoot, GateValidate, TargetKindUnit, "auth", TargetCandidate, ModeFull, nil, nil, nil, time.Now()); err == nil {
		t.Fatal("expected a missing Keys block to fail the plan")
	}
}

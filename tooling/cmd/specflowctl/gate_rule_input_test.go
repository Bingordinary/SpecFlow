package main

import (
	"bytes"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// Issue #60: a rule file's physical path in the input manifest must plan as
// its logical reference. Otherwise the plan copies the physical path into the
// public code sessions' read refs, and the tool-recorded whole-file evidence
// for the public code check would record a physical rule path that the
// logical-reference check rejects — a state the run cannot finalize.
func TestPlannedPhysicalRuleInputDoesNotDeadlockSubmit(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteFile(t, root, "docs/specs/rules/stable/g_rule_logging.md",
		"---\nid: g_rule_logging\nscope: unit\n---\n\n# g_rule_logging\n\n## Constraint\n\nMust log.\n")
	grWriteFile(t, root, "docs/specs/units/candidate/unit_auth.md",
		"---\nid: auth\nunit_refs: none\nrule_refs: g_rule_logging\n---\n\n# auth\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n"+
			"  - id: auth.core\n    description: Behavior.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: contracts.js\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n")
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")

	physical := "docs/specs/rules/stable/g_rule_logging.md"
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, physical))
	run := mustLoadRun(t, root, id)

	// The plan normalizes the physical spelling: the snapshot and the public
	// read refs carry the logical form only.
	ck := run.CoverageByKey("code:contracts.js")
	if ck == nil {
		t.Fatalf("fixture must produce a public code key: %+v", run.Coverage)
	}
	for _, p := range ck.ReadRefs {
		if p == physical {
			t.Fatalf("public read refs carry the physical rule path: %v", ck.ReadRefs)
		}
	}
	if !stringInList(ck.ReadRefs, "rule:g_rule_logging") {
		t.Fatalf("public read refs must carry the logical rule reference, got %v", ck.ReadRefs)
	}

	// The public session's report carries facts only; the tooling records its
	// read refs as the whole-file evidence. Before the fix the plan carried
	// the physical rule path, which the recorded evidence would reject as a
	// rule path while the logical form alone did not cover the read ref.
	report := grWriteFile(t, t.TempDir(), "quality-report.md", grDefaultQualityReport(run, *ck))
	var stdout, stderr bytes.Buffer
	if err := runGateSubmit([]string{"--repo-root", root, "--run", id, "--session", gaterun.SessionID([]string{ck.Key}), "--keys", ck.Key, "--report", report}, &stdout, &stderr); err != nil {
		t.Fatalf("public code session deadlocked: %v (stderr=%s)", err, stderr.String())
	}

	// The directory spelling of the same manifest input behaves identically:
	// the expansion carries each rule file as its logical reference.
	id = grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, "docs/specs/rules/stable"))
	run = mustLoadRun(t, root, id)
	ck = run.CoverageByKey("code:contracts.js")
	if ck == nil {
		t.Fatalf("directory-form plan must keep the public code key: %+v", run.Coverage)
	}
	for _, p := range ck.ReadRefs {
		if p == physical {
			t.Fatalf("directory expansion leaked the physical rule path into the read refs: %v", ck.ReadRefs)
		}
	}
	if !stringInList(ck.ReadRefs, "rule:g_rule_logging") {
		t.Fatalf("directory expansion must carry the logical rule reference, got %v", ck.ReadRefs)
	}
}

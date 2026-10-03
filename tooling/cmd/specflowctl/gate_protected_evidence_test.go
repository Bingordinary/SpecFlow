package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fork"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func TestGateSynthesisKeepsProtectedStableEvidence(t *testing.T) {
	root, _, orderSpec := sharedFixture(t)
	grEnableMissionLayout(t, root)
	stable := "docs/specs/units/stable/unit_order.md"
	appendix := "docs/specs/units/stable/appendix/unit_order_protocol.md"
	rule := "docs/specs/rules/stable/b_rule_transport.md"
	data, err := os.ReadFile(filepath.Join(root, orderSpec))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, stable, strings.Replace(string(data), "rule_refs: none", "rule_refs: b_rule_transport", 1))
	grWriteFile(t, root, appendix, "---\nunit: order\nstatus: active\n---\n\n# Stable contract\n\nThe response preserves its token.\n")
	grWriteFile(t, root, rule, "---\nid: b_rule_transport\nscope: unit\n---\n\n## Constraint\n\nPreserve the token.\n")
	grWriteFile(t, root, orderSpec, "---\nid: order\nunit_refs: none\nrule_refs: none\n---\n\n## Draft\n\nUnfinished candidate.\n")
	candidateRule := strings.Replace(rule, "/stable/", "/candidate/", 1)
	grWriteFile(t, root, candidateRule, "---\nid: b_rule_transport\nscope: unit\n---\n\n## Constraint\n\nUnfinished candidate rule.\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency")
	run := mustLoadRun(t, root, id)
	if run.CoverageByKey("preserve:order:order.core") == nil {
		t.Fatal("missing stable protection")
	}
	for _, ck := range run.Coverage {
		report := grDefaultQualityReport(run, ck)
		if gaterun.IsItemKind(ck.Kind) {
			main := run.RequiredFiles[0]
			if ck.Kind == gaterun.SessionKindPreserve {
				main = stable
			}
			report = grVerifyItemReport(ck.Key, main, "contracts.js")
		}
		if err := sharedSubmit(t, root, id, ck.Key, report); err != nil {
			t.Fatal(err)
		}
	}
	assertPinnedEvidence := func(layer string) {
		t.Helper()
		baseline, err := validationcache.ReadGateBaseline(root, "unit", "auth", "verify")
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{stable, appendix, rule} {
			found := false
			for _, entry := range baseline.Entries {
				if entry.Path == path {
					for _, check := range entry.Checks {
						found = found || check.Check == gaterun.RelationshipKey("contract_consistency")
					}
				}
				if entry.Path == strings.Replace(path, "/stable/", "/candidate/", 1) {
					t.Fatalf("protected evidence was rebound to a candidate: %s", entry.Path)
				}
			}
			if !found {
				t.Fatalf("protected synthesis evidence missing from cache: %s", path)
			}
		}
		if check, err := checkUnitVerifyMerged(root, "auth", layer); err != nil || !check.Fresh {
			t.Fatalf("protected synthesis is not fresh at %s: %+v %v", layer, check, err)
		}
	}
	submitSynthesis := func(runID string) {
		t.Helper()
		mission := missionJSON(t, root, runID, "cross")
		for _, path := range []string{stable, appendix, rule} {
			if !stringInList(mission.Sessions[0].ReadRefs, path) {
				t.Fatalf("planned stable input is absent from synthesis mission: %s", path)
			}
		}
		report := relationshipReport(t, root, runID, map[string]string{"contract_consistency": stable}, "")
		for _, path := range []string{appendix, rule} {
			report += gaterun.RelationshipKey("contract_consistency") + ": " + path + ": all\n"
		}
		if err := sharedSubmit(t, root, runID, "cross", report); err != nil {
			t.Fatalf("mission's stable protection evidence was rejected: %v", err)
		}
		grFinalizeOK(t, root, runID)
	}
	submitSynthesis(id)
	assertPinnedEvidence("candidate")

	// A relationship-only delta still consumes stable protection even though
	// all local checks, including preserve, are carried rather than executed.
	delta := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--rerun", gaterun.RelationshipKey("contract_consistency"))
	if next := mustLoadRun(t, root, delta); len(next.Coverage) != 0 || !stringInList(next.CarriedKeys, "preserve:order:order.core") {
		t.Fatalf("expected a relationship-only delta with carried protection: %+v", next)
	}
	submitSynthesis(delta)
	assertPinnedEvidence("candidate")

	sharedValidateFixture(t, root, "auth")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatalf("promotion rejected valid stable protection: %v %s", err, out.String())
	}
	assertPinnedEvidence("stable")
	if result := fork.Fork(root, "auth"); !result.Passed {
		t.Fatal(result.Issues)
	}
	assertPinnedEvidence("candidate")

	// Editing unfinished peers cannot stale evidence read from stable. Editing
	// the protected stable contract must stale the relationship judgment.
	grWriteFile(t, root, orderSpec, "---\nid: order\nunit_refs: none\nrule_refs: none\n---\n\n## Draft\n\nStill unfinished.\n")
	grWriteFile(t, root, candidateRule, "---\nid: b_rule_transport\nscope: unit\n---\n\n## Constraint\n\nAnother candidate rule draft.\n")
	assertPinnedEvidence("candidate")
	grWriteFile(t, root, appendix, "---\nunit: order\nstatus: active\n---\n\n# Stable contract\n\nThe response now requires a different token.\n")
	if check, err := checkUnitVerifyMerged(root, "auth", "candidate"); err != nil || check.Fresh || check.Category != validationcache.CategoryStale {
		t.Fatalf("changed stable evidence did not stale synthesis: %+v %v", check, err)
	}
}

func TestGateSynthesisRejectsUnplannedPhysicalStableSpec(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpecSurface(t, root, "auth", "auth.js", "")
	grWriteFile(t, root, "auth.js", "export const auth = true;\n")
	unrelated := "docs/specs/units/stable/unit_order.md"
	grWriteFile(t, root, unrelated, "---\nid: order\nunit_refs: none\nrule_refs: none\n---\n\n## Description\n\nUnrelated stable contract.\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "contract_consistency", "--input", unrelated)
	run := mustLoadRun(t, root, id)
	spec, err := gaterun.BuildSessionSpec(root, run, []string{gaterun.CrossKey})
	if err != nil {
		t.Fatal(err)
	}
	if !stringInList(spec.ReadRefs, unrelated) {
		t.Fatal("test requires the unrelated physical spec to be in the read refs")
	}
	parsed := &parsedReport{Scopes: []parsedScope{{Key: gaterun.RelationshipKey("contract_consistency"), Path: unrelated, Declaration: "all"}}}
	if err := validateSessionDeclarations(root, run, spec, parsed); err == nil || !strings.Contains(err.Error(), "logical reference") {
		t.Fatalf("an unrelated --input bypassed the logical-reference contract: %v", err)
	}
}

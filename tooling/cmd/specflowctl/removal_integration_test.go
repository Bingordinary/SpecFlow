package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fork"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/install"
)

func TestRemovalDiscardsUnfinishedChecksWithoutRecreatingCache(t *testing.T) {
	root, _, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	var out, errOut bytes.Buffer
	if err := runRemove([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	// The removed unit's run state is swept with the spec: nothing can
	// finalize obsolete judgments.
	if _, err := os.Stat(filepath.Join(root, "meta", "gate_runs", id)); !os.IsNotExist(err) {
		t.Fatal("run of the removed unit survived removal")
	}
	if err := runGateFinalize([]string{"--repo-root", root, "--run", id}, &out, &errOut); err == nil {
		t.Fatal("old run finalized after removal")
	}
	if _, err := os.Stat(filepath.Join(root, "docs/specs/meta/validation/unit/auth/verify_result.md")); !os.IsNotExist(err) {
		t.Fatal("obsolete cache recreated", err)
	}
}

func installedRemovalProject(t *testing.T) string {
	t.Helper()
	root := createCLITestRepo(t)
	source, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"framework", "templates"} {
		err = filepath.WalkDir(filepath.Join(source, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			grWriteFile(t, root, "specflow/"+filepath.ToSlash(rel), string(data))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(source, "tooling/manifest.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, "specflow/tooling/manifest.tsv", string(manifest))
	if _, err := install.Init(root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "specflow/framework/removal_workflow.md")); err != nil {
		t.Fatal(err)
	}
	return root
}

func removalPublishUnit(t *testing.T, root, name string) {
	t.Helper()
	runID := grPlan(t, root, "--gate", "validate", "--unit", name, "--target", "candidate")
	run := mustLoadRun(t, root, runID)
	for _, ck := range run.Coverage {
		session, err := gaterun.BuildSessionSpec(root, run, []string{ck.Key})
		if err != nil {
			t.Fatal(err)
		}
		scopes := map[string][]string{}
		for _, key := range session.CheckKeys {
			for _, path := range session.ReadRefs {
				scopes[key] = append(scopes[key], path+": all")
			}
		}
		grSubmitOK(t, root, runID, ck.Key, grValidateReport(session.CheckKeys, scopes))
	}
	grFinalizeOK(t, root, runID)
	verify := grPlan(t, root, "--gate", "verify", "--unit", name, "--target", "candidate")
	sharedFinish(t, root, verify)
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", name}, &out, &errOut); err != nil {
		t.Fatalf("publish %s: %v\n%s", name, err, out.String())
	}
}

func TestInstalledProjectAppendixReplacementAndReferenceRemoval(t *testing.T) {
	root := installedRemovalProject(t)
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")
	grWriteSpecSurface(t, root, "auth", "contracts.js", "    affects:\n      appendices: [unit_auth_old.md]\n")
	main := "docs/specs/units/candidate/unit_auth.md"
	appendix := "docs/specs/units/candidate/appendix/unit_auth_old.md"
	grWriteFile(t, root, appendix, "---\nunit: auth\nstatus: active\n---\n\n# Old protocol\nThe previous response contract.\n")
	removalPublishUnit(t, root, "auth")
	if result := fork.Fork(root, "auth"); !result.Passed {
		t.Fatal(result.Issues)
	}
	data, err := os.ReadFile(filepath.Join(root, main))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, main, strings.Replace(string(data), "      appendices: [unit_auth_old.md]", "      appendices: none", 1))
	if _, err := removeRun(t, root, "--appendix", "auth:unit_auth_old.md", "--layer", "candidate"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs/specs/units/stable/appendix/unit_auth_old.md")); err != nil {
		t.Fatal("stable appendix prematurely removed", err)
	}
	removalPublishUnit(t, root, "auth")
	if _, err := removeRun(t, root, "--appendix", "auth:unit_auth_old.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs/specs/meta/baseline/unit/auth.yaml")); err != nil {
		t.Fatal("appendix deletion lost code baseline", err)
	}

	// A surviving consumer must publish its dropped references before deletion.
	writeBoundRule(t, root, "stable", "b_rule_http", "")
	grWriteSpecSurface(t, root, "consumer", "contracts.js", "")
	consumer := "docs/specs/units/candidate/unit_consumer.md"
	data, err = os.ReadFile(filepath.Join(root, consumer))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, consumer, strings.Replace(strings.Replace(string(data), "unit_refs: none", "unit_refs: auth", 1), "rule_refs: none", "rule_refs: b_rule_http", 1))
	removalPublishUnit(t, root, "consumer")
	if r := fork.Fork(root, "consumer"); !r.Passed {
		t.Fatal(r.Issues)
	}
	data, err = os.ReadFile(filepath.Join(root, consumer))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, consumer, strings.Replace(strings.Replace(string(data), "unit_refs: auth", "unit_refs: none", 1), "rule_refs: b_rule_http", "rule_refs: none", 1))
	if _, err := removeRun(t, root, "--unit", "auth", "--rule", "b_rule_http"); err == nil {
		t.Fatal("stable references hidden by candidate")
	}
	removalPublishUnit(t, root, "consumer")
	if _, err := os.Stat(filepath.Join(root, "docs/specs/rules/stable/b_rule_http.md")); err != nil {
		t.Fatal("promote deleted unbound rule", err)
	}
	if _, err := removeRun(t, root, "--unit", "auth", "--rule", "b_rule_http"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "contracts.js")); err != nil {
		t.Fatal("removed business code", err)
	}
}

func TestRemoveArgumentErrorsAndUnknownDetect(t *testing.T) {
	root := createCLITestRepo(t)
	for _, args := range [][]string{{}, {"--layer", "stable", "--unit", "auth"}, {"--force", "--unit", "auth"}, {"--unit", "auth", "extra"}, {"--appendix", "auth:../escape.md"}} {
		if _, err := removeRun(t, root, args...); err == nil {
			t.Fatalf("accepted invalid arguments: %v", args)
		}
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"detect", "--all", "--repo-root", root}, &out, &errOut); err == nil {
		t.Fatal("old detect command remains")
	}
}

func TestFreshNeverRecommendsDeletingUnboundRules(t *testing.T) {
	for _, scope := range []string{"candidate", "stable", "all"} {
		t.Run(scope, func(t *testing.T) {
			root := createCLITestRepo(t)
			writeBoundRule(t, root, "stable", "b_rule_future", "")
			writeBoundRule(t, root, "candidate", "b_rule_future", "")
			var out, errOut bytes.Buffer
			if err := runFresh([]string{"--repo-root", root, "--scope", scope}, &out, &errOut); err != nil {
				t.Fatal(err)
			}
			report := strings.ToLower(out.String())
			for _, term := range []string{"unbound rules", "removable", "removal candidate", "remove --"} {
				if strings.Contains(report, term) {
					t.Fatalf("fresh chooses deletion: %s", out.String())
				}
			}
			for _, layer := range []string{"stable", "candidate"} {
				if _, err := os.Stat(filepath.Join(root, "docs/specs/rules", layer, "b_rule_future.md")); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

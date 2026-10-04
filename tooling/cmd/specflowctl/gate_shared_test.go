package main

import (
	"bytes"
	"encoding/json"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fork"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func sharedFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := createCLITestRepo(t)
	grWriteSpecSurface(t, root, "auth", "contracts.js", "")
	grWriteSpecSurface(t, root, "order", "contracts.js", "")
	a := "docs/specs/units/candidate/unit_auth.md"
	o := "docs/specs/units/candidate/unit_order.md"
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")

	return root, a, o
}
func sharedSubmit(t *testing.T, root, runID, key, report string) error {
	t.Helper()
	p := grWriteFile(t, t.TempDir(), "report.md", report)
	var out, errOut bytes.Buffer
	return runGateSubmit([]string{"--repo-root", root, "--run", runID, "--session", gaterun.SessionID([]string{key}), "--keys", key, "--report", p}, &out, &errOut)
}
func sharedFinish(t *testing.T, root, id string) {
	t.Helper()
	run, err := gaterun.Load(root, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, ck := range run.Coverage {
		states, err := gaterun.LoadSessionStates(root, run)
		if err != nil {
			t.Fatal(err)
		}
		covered, _, err := gaterun.CoverageProgress(run, states)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := covered[ck.Key]; ok {
			continue
		}
		report := grDefaultQualityReport(run, ck)
		if gaterun.IsItemKind(ck.Kind) {
			spec := run.RequiredFiles[0]
			if ck.Kind == gaterun.SessionKindPreserve {
				spec = "docs/specs/units/stable/unit_" + ck.Unit + ".md"
			}
			report = grVerifyItemReport(ck.Key, spec, "contracts.js")
		}
		if err := sharedSubmit(t, root, id, ck.Key, report); err != nil {
			t.Fatalf("%s: %v", ck.Key, err)
		}
	}
	var out, errOut bytes.Buffer
	if err := runGateFinalize([]string{"--repo-root", root, "--run", id}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
}
func TestSharedPublicReviewReuseAndIndependentDesign(t *testing.T) {
	root, _, _ := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run, err := gaterun.Load(root, order)
	if err != nil {
		t.Fatal(err)
	}
	if run.CoverageByKey("code:contracts.js").Source != "reused" {
		t.Fatal("public review was allocated twice")
	}
	sharedFinish(t, root, order)
	a := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	o := grReadJudgmentBaseline(t, root, "unit", "order", "verify")
	if a.Records["code:contracts.js"].ID != o.Records["code:contracts.js"].ID {
		t.Fatal("public result not shared")
	}
	if a.Records["design:auth:contracts.js"].ID == o.Records["design:order:contracts.js"].ID {
		t.Fatal("private judgments overwritten")
	}
	result, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
	if err != nil || !result.Fresh {
		t.Fatalf("fresh: %+v %v", result, err)
	}
}
func TestSharedUnfinishedPeerDoesNotBlock(t *testing.T) {
	root, _, orderSpec := sharedFixture(t)
	grWriteFile(t, root, orderSpec, "---\nid: order\nunit_refs: none\nrule_refs: none\n---\n\n## Draft\nDesign unfinished.\n")
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	records, err := judgments.List(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range records {
		r, err := judgments.Load(root, ref)
		if err != nil {
			t.Fatal(err)
		}
		if r.Unit == "order" {
			t.Fatal("fabricated unfinished peer result")
		}
	}
}
func TestSharedTaskWaitAndInputChanges(t *testing.T) {
	root, _, _ := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	a, err := gaterun.Load(root, auth)
	if err != nil {
		t.Fatal(err)
	}
	o, err := gaterun.Load(root, order)
	if err != nil {
		t.Fatal(err)
	}
	if a.CoverageByKey("code:contracts.js").Task != o.CoverageByKey("code:contracts.js").Task {
		t.Fatal("different task ids")
	}
	if err := gaterun.WithMutation(root, func() error { return gaterun.ClaimShared(root, o, []string{"code:contracts.js"}) }); err == nil {
		t.Fatal("duplicate public allocation")
	}
	if err := sharedSubmit(t, root, auth, "code:contracts.js", grDefaultQualityReport(a, *a.CoverageByKey("code:contracts.js"))); err != nil {
		t.Fatal(err)
	}
	states, err := gaterun.LoadSessionStates(root, o)
	if err != nil {
		t.Fatal(err)
	}
	covered, _, err := gaterun.CoverageProgress(o, states)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := covered["code:contracts.js"]; !ok {
		t.Fatal("waiter cannot consume accepted result")
	}
	grWriteFile(t, root, "contracts.js", "export function response() { return {}; }\n")
	if _, err := gaterun.LoadSessionStates(root, o); err == nil {
		t.Fatal("stale public record accepted")
	}
}
func TestSharedInvalidationAndWholeFileIdentity(t *testing.T) {
	root, _, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, id)
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)
	state := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	public := state.Records["code:contracts.js"]
	var out, errOut bytes.Buffer
	if err := runGateInvalidate([]string{"--repo-root", root, "--judgment", public.ID, "--reason", "new evidence contradicts a public fact"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	result, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate")
	if err != nil || result.Fresh {
		t.Fatal("invalidated reference allowed")
	}
	if peer, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || peer.Fresh {
		t.Fatalf("peer continued to use invalidated public PASS: %+v %v", peer, err)
	}
	repair := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	sharedFinish(t, root, repair)
	newer := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	if newer.Records["code:contracts.js"].ID == public.ID {
		t.Fatal("invalidated record reused")
	}
	p := filepath.Join(root, judgments.Directory, newer.Records["code:contracts.js"].ID+".json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	json.Unmarshal(data, &record)
	record["verdict"] = "spoofed"
	damaged, _ := json.Marshal(record)
	os.WriteFile(p, damaged, 0644)
	result, err = validationcache.CheckVerify(root, "auth")
	if err != nil || result.Fresh {
		t.Fatal("damaged record allowed")
	}
	full := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, full)
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("recheck after record damage: %+v %v", check, err)
	}
}
func TestSharedProtectsStableDespiteUnfinishedCandidate(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	data, err := os.ReadFile(filepath.Join(root, authSpec))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, "docs/specs/units/stable/unit_auth.md", string(data))
	if _, err := validationcache.RewriteCachesToStable(root, "unit", "auth"); err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, authSpec, "---\nid: auth\nunit_refs: none\nrule_refs: none\n---\n\n## Draft\nUnfinished replacement.\n")
	grWriteFile(t, root, "contracts.js", "export function response() { return {}; }\n")
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run, err := gaterun.Load(root, order)
	if err != nil {
		t.Fatal(err)
	}
	ck := run.CoverageByKey("preserve:auth:auth.core")
	if ck == nil {
		t.Fatal("no stable protection")
	}
	spec, err := grBuildSessionSpec(root, run, []string{ck.Key})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(spec.ReadRefs, "\n"), "stable/unit_auth.md") || strings.Contains(strings.Join(spec.ReadRefs, "\n"), "candidate/unit_auth.md") {
		t.Fatal("protection used draft design")
	}
	report := grVerifyItemBody(ck.Key, "CANNOT_DETERMINE", "token behavior missing") + ck.Key + ": docs/specs/units/stable/unit_auth.md: acceptance_item:auth.core\n" + ck.Key + ": contracts.js: all\n"
	if err := sharedSubmit(t, root, order, ck.Key, report); err != nil {
		t.Fatal(err)
	}
	grAutoSubmitQuality(t, root, order)
	grSubmitOK(t, root, order, "order.core", grVerifyItemReport("order.core", run.RequiredFiles[0], "contracts.js"))
	cross := grCompleteCrossReport(t, root, run, grCrossReport(run.RequiredFiles[0], "Description"))
	cross = strings.ReplaceAll(cross, "Effective status: "+ck.Key+" = fail", "Effective status: "+ck.Key+" = pass")
	if err := sharedSubmit(t, root, order, "cross", cross); err == nil {
		t.Fatal("protected requirement cleared by synthesis")
	}
	grSubmitOK(t, root, order, "cross", grCrossReport(run.RequiredFiles[0], "Description"))
	grFinalizeOK(t, root, order)
	if result, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || result.Fresh || result.Category != validationcache.CategoryBlocked {
		t.Fatalf("broken stable behavior released: %+v %v", result, err)
	}
	sharedValidateFixture(t, root, "order")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err == nil || !strings.Contains(out.String(), "Verify cache") {
		t.Fatalf("promote did not block protected token requirement: %v %s", err, out.String())
	}
}

func TestSharedProtectionCanonicalizesDeclaredPaths(t *testing.T) {
	for _, field := range []string{"implementation_surface", "affects.files"} {
		for _, declared := range []string{"contracts.js", "./contracts.js", "nested/../contracts.js", "shared", "./shared/", "shared/../shared/"} {
			t.Run(field+"/"+declared, func(t *testing.T) {
				root, authSpec, _ := sharedFixture(t)
				file := "contracts.js"
				if strings.Contains(declared, "shared") {
					file = "shared/contracts.js"
					grWriteFile(t, root, file, "export function response(token) { return {token}; }\n")
					grWriteSpecSurface(t, root, "order", file, "")
				}
				grWriteFile(t, root, "caller.js", "import {response} from './"+file+"';\n")
				surface, extra := declared, ""
				if field == "affects.files" {
					surface = "unrelated.js"
					extra = "    affects:\n      files:\n        - " + declared + "\n"
					grWriteFile(t, root, surface, "export const separate = true;\n")
				}
				grWriteSpecSurface(t, root, "auth", surface, extra)
				data, err := os.ReadFile(filepath.Join(root, authSpec))
				if err != nil {
					t.Fatal(err)
				}
				stable := "docs/specs/units/stable/unit_auth.md"
				grWriteFile(t, root, stable, string(data))
				id := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
				ck := mustLoadRun(t, root, id).CoverageByKey("preserve:auth:auth.core")
				if ck == nil {
					t.Fatalf("stable protection omitted for %s: %s", field, declared)
				}
				for _, required := range []string{stable, file, "caller.js"} {
					found := false
					for _, ref := range ck.ReadRefs {
						found = found || ref == required
					}
					if !found {
						t.Fatalf("protected review omitted connected evidence %s: %v", required, ck.ReadRefs)
					}
				}
			})
		}
	}
}

func TestSharedProtectionRequiredByFreshAndPromote(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, id)
	data, err := os.ReadFile(filepath.Join(root, authSpec))
	if err != nil {
		t.Fatal(err)
	}
	stable := strings.Replace(string(data), "implementation_surface: contracts.js", "implementation_surface: ./contracts.js", 1)
	grWriteFile(t, root, "docs/specs/units/stable/unit_auth.md", stable)
	if result, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || result.Fresh || !strings.Contains(result.Reason, "preserve:auth:auth.core") {
		t.Fatalf("verify without stable protection remained fresh: %+v %v", result, err)
	}
	sharedValidateFixture(t, root, "order")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err == nil {
		t.Fatal("promote accepted verification without stable protection")
	}
	id = grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	if mustLoadRun(t, root, id).CoverageByKey("preserve:auth:auth.core") == nil {
		t.Fatal("replan omitted stable protection")
	}
	sharedFinish(t, root, id)
	if result, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !result.Fresh {
		t.Fatalf("complete protection did not become fresh: %+v %v", result, err)
	}
	out.Reset()
	if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err != nil {
		t.Fatalf("promote with complete protection: %v %s", err, out.String())
	}
}

func TestSharedRejectedBatchDoesNotReserveTasks(t *testing.T) {
	for _, command := range []string{"mission", "submit"} {
		for _, blocker := range []string{"owner", "accepted", "session"} {
			t.Run(command+"/"+blocker, func(t *testing.T) {
				root, _, orderSpec := sharedFixture(t)
				grEnableMissionLayout(t, root)
				data, err := os.ReadFile(filepath.Join(root, orderSpec))
				if err != nil {
					t.Fatal(err)
				}
				grWriteFile(t, root, orderSpec, string(data)+"    affects:\n      files:\n        - local.js\n")
				grWriteFile(t, root, "local.js", "export const separate = true;\n")
				auth := ""
				if blocker != "session" {
					auth = grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
				}
				order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
				run := mustLoadRun(t, root, order)
				acceptShared := func() {
					a := mustLoadRun(t, root, auth)
					if err := sharedSubmit(t, root, auth, "code:contracts.js", grDefaultQualityReport(a, *a.CoverageByKey("code:contracts.js"))); err != nil {
						t.Fatal(err)
					}
				}
				if blocker == "accepted" {
					acceptShared()
				}
				mission := func(keys string) error {
					var out, errOut bytes.Buffer
					return runGateMission([]string{"--repo-root", root, "--run", order, "--keys", keys, "--format", "json"}, &out, &errOut)
				}
				if blocker == "session" {
					if err := mission("code:contracts.js"); err != nil {
						t.Fatal(err)
					}
				}
				local := run.CoverageByKey("code:local.js")
				path := filepath.Join(root, "meta/gate_runs/shared", local.Task+".json")
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				keys := []string{"code:local.js", "code:contracts.js"}
				if command == "mission" {
					err = mission(strings.Join(keys, ","))
				} else {
					report := grDefaultQualityReport(run, *local) + grDefaultQualityReport(run, *run.CoverageByKey("code:contracts.js"))
					p := grWriteFile(t, t.TempDir(), "batch.md", report)
					var out, errOut bytes.Buffer
					err = runGateSubmit([]string{"--repo-root", root, "--run", order, "--session", gaterun.SessionID(keys), "--keys", strings.Join(keys, ","), "--report", p}, &out, &errOut)
				}
				if err == nil {
					t.Fatal("unavailable batch was accepted")
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if !bytes.Equal(before, after) {
					t.Fatalf("rejected batch reserved its local task: %s -> %s (rejection: %v)", before, after, err)
				}
				if blocker == "owner" {
					acceptShared()
				}
				if err := mission("code:local.js"); err != nil {
					t.Fatalf("local task cannot proceed after batch rejection: %v", err)
				}
				if err := sharedSubmit(t, root, order, local.Key, grDefaultQualityReport(run, *local)); err != nil {
					t.Fatalf("local task cannot complete after batch rejection: %v", err)
				}
			})
		}
	}
}

func TestSharedObservationDecisionsRemainIndependent(t *testing.T) {
	root, authSpec, orderSpec := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run, err := gaterun.Load(root, auth)
	if err != nil {
		t.Fatal(err)
	}
	report := grDefaultQualityReport(run, *run.CoverageByKey("code:contracts.js")) + "[P1] contracts.js — data shape may discard token (actionable)\n  problem: token shape differs between callers\n  evidence: caller requires token\n  impact: login may fail\n  fix: reconcile the contract\n"
	if err := sharedSubmit(t, root, auth, "code:contracts.js", report); err != nil {
		t.Fatal(err)
	}
	run, err = gaterun.Load(root, auth)
	if err != nil {
		t.Fatal(err)
	}
	ref := run.Records["code:contracts.js"]
	public, err := judgments.Load(root, ref.Reference)
	if err != nil {
		t.Fatal(err)
	}
	var result gaterun.SessionResult
	if err := json.Unmarshal(public.Result, &result); err != nil {
		t.Fatal(err)
	}
	observation := result.Observations[0].ID
	authDesign := "File: design:auth:contracts.js\nconclusion: acceptable\nspec_requirements: token behavior checked — auth contract uses this shape\ngate_findings: none\nObservation disposition: " + observation + " = suppressed — auth spec explicitly accepts this shape and its caller retains token\nDependency scope:\ndesign:auth:contracts.js: " + authSpec + ": Description\ndesign:auth:contracts.js: contracts.js: all\n"
	if err := sharedSubmit(t, root, auth, "design:auth:contracts.js", authDesign); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, auth)
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	orderDesign := "File: design:order:contracts.js\nconclusion: unacceptable — token required\nspec_requirements: order requires token — the contract discards it\ngate_findings: [P1] token shape\nObservation disposition: " + observation + " = retained — order spec requires a caller shape that is violated\nDependency scope:\ndesign:order:contracts.js: " + orderSpec + ": Description\ndesign:order:contracts.js: contracts.js: all\n"
	if err := sharedSubmit(t, root, order, "design:order:contracts.js", orderDesign); err != nil {
		t.Fatal(err)
	}
	orderRun, err := gaterun.Load(root, order)
	if err != nil {
		t.Fatal(err)
	}
	state, err := grLoadSessionState(root, orderRun, "design:order:contracts.js")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Result.Findings) != 1 {
		t.Fatal("retained observation lost")
	}
	grSubmitOK(t, root, order, "order.core", grVerifyItemReport("order.core", orderSpec, "contracts.js"))
	grAutoSubmitQuality(t, root, order)
	grSubmitOK(t, root, order, "cross", grCrossReport(orderSpec, "Description"))
	grFinalizeOK(t, root, order)
	orderBaseline := grReadJudgmentBaseline(t, root, "unit", "order", "verify")
	if orderBaseline.Records["design:order:contracts.js"].ID == "" {
		t.Fatal("order's independent design record was not saved")
	}

	saved, err := judgments.Load(root, ref.Reference)
	if err != nil || saved.Report != public.Report {
		t.Fatal("private decision changed public report")
	}
	a := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	ar, err := judgments.Load(root, a.Records["design:auth:contracts.js"].Reference)
	if err != nil {
		t.Fatal(err)
	}
	var authResult gaterun.SessionResult
	json.Unmarshal(ar.Result, &authResult)
	if len(authResult.Findings) != 0 {
		t.Fatal("peer finding overwrote auth exclusion")
	}
}
func TestSharedNewCallerRequiresFreshCoverageAndPrivateDelta(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	before := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	data, err := os.ReadFile(filepath.Join(root, authSpec))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, authSpec, strings.Replace(string(data), "Prose.", "Updated private design.", 1))
	delta := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run, err := gaterun.Load(root, delta)
	if err != nil {
		t.Fatal(err)
	}
	if run.CoverageByKey("code:contracts.js") != nil {
		t.Fatal("private design repeated public review")
	}
	if run.CoverageByKey("design:auth:contracts.js") == nil {
		t.Fatal("private design judgment not updated")
	}
	sharedFinish(t, root, delta)
	after := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	if before.Records["code:contracts.js"].ID != after.Records["code:contracts.js"].ID {
		t.Fatal("private spec discarded public record")
	}
	grWriteFile(t, root, "order.js", "import {response} from './contracts.js';\nresponse();\n")
	fresh, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate")
	if err != nil || fresh.Fresh {
		t.Fatal("new caller escaped scope validation")
	}
	delta = grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta")
	run, err = gaterun.Load(root, delta)
	if err != nil {
		t.Fatal(err)
	}
	if run.CoverageByKey("code:contracts.js") == nil || run.CoverageByKey("design:auth:contracts.js") == nil {
		t.Fatal("new evidence did not invalidate public/design coverage")
	}
}

func sharedValidateFixture(t *testing.T, root, unit string) {
	t.Helper()
	entry, err := validationcache.BuildEntry(root, validationcache.EntryDeclaration{Path: "docs/specs/units/candidate/unit_" + unit + ".md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(root, "unit", unit, validationcache.CacheWrite{Command: "validate", Unit: unit, Mode: "full", Result: "pass", Target: "candidate", Entries: []validationcache.FileEntry{entry}}); err != nil {
		t.Fatal(err)
	}
}

func TestSharedLifecyclePreservesImmutableRecords(t *testing.T) {
	root, _, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, id)
	before := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	sharedValidateFixture(t, root, "auth")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatalf("promote: %v %s", err, out.String())
	}
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "stable"); err != nil || !check.Fresh {
		t.Fatalf("stable: %+v %v", check, err)
	}
	if result := fork.Fork(root, "auth"); !result.Passed {
		t.Fatal(result.Issues)
	}
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("fork: %+v %v", check, err)
	}
	after := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	for key, ref := range before.Records {
		if after.Records[key].ID != ref.ID {
			t.Fatal("layer transition changed accepted record", key)
		}
	}
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)
	orderCache := grReadJudgmentBaseline(t, root, "unit", "order", "verify")
	if orderCache.Records["preserve:auth:auth.core"].ID != before.Records["item:auth:auth.core"].ID {
		t.Fatal("stable protection did not reuse confirmed acceptance evidence")
	}
	if err := runRemove([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	public := orderCache.Records["code:contracts.js"]
	if err := judgments.Check(root, public.Reference, "candidate", judgments.Protocol(root)); err != nil {
		t.Fatal("removal deleted a shared record", err)
	}
	if _, err := judgments.Load(root, before.Records["item:auth:auth.core"].Reference); err != nil {
		t.Fatal("removal deleted history", err)
	}
	// The removed stable requirement cannot be carried; replan order without it.
	next := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate", "--mode", "delta")
	run := mustLoadRun(t, root, next)
	if run.CoverageByKey("preserve:auth:auth.core") != nil {
		t.Fatal("removed unit remains protected")
	}
	sharedFinish(t, root, next)
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("after remove: %+v %v", check, err)
	}
}

func TestSharedConcurrentAllocationAndInterruptedResume(t *testing.T) {
	root, _, _ := sharedFixture(t)
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for _, unit := range []string{"auth", "order"} {
		wg.Add(1)
		go func(unit string) {
			defer wg.Done()
			id, err := grPlanRaw(root, "--gate", "verify", "--unit", unit, "--target", "candidate")
			if err != nil {
				errs <- err
			} else {
				ids <- id
			}
		}(unit)
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var runs []*gaterun.Run
	for id := range ids {
		runs = append(runs, mustLoadRun(t, root, id))
	}
	if len(runs) != 2 {
		t.Fatal("missing concurrent run")
	}
	first, second := runs[0], runs[1]
	if first.CoverageByKey("code:contracts.js").Source == "waiting" {
		first, second = second, first
	}
	task := first.CoverageByKey("code:contracts.js").Task
	if task != second.CoverageByKey("code:contracts.js").Task || second.CoverageByKey("code:contracts.js").Source != "waiting" {
		t.Fatal("concurrent tasks allocated twice")
	}
	replacement := grPlan(t, root, "--gate", "verify", "--unit", first.TargetName, "--target", "candidate")
	resumed := mustLoadRun(t, root, replacement)
	if resumed.CoverageByKey("code:contracts.js").Task != task {
		t.Fatal("interruption created a new public task")
	}
	report := grDefaultQualityReport(resumed, *resumed.CoverageByKey("code:contracts.js"))
	if err := sharedSubmit(t, root, replacement, "code:contracts.js", report); err != nil {
		t.Fatal(err)
	}
	if err := sharedSubmit(t, root, first.RunID, "code:contracts.js", report); err == nil {
		t.Fatal("replaced run published")
	}
	states, err := gaterun.LoadSessionStates(root, second)
	if err != nil {
		t.Fatal(err)
	}
	covered, _, err := gaterun.CoverageProgress(second, states)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := covered["code:contracts.js"]; !ok {
		t.Fatal("waiter did not receive resumed task")
	}
	if second.CoverageByKey("code:contracts.js").Source != "reused" {
		t.Fatal("execution source was not updated")
	}
}

func TestSharedOldRunRequiresReplanAndOneBatchOwnsTask(t *testing.T) {
	root, _, _ := sharedFixture(t)
	grWriteFile(t, root, "other.js", "export const separate=true;\n")
	data, _ := os.ReadFile(filepath.Join(root, "docs/specs/units/candidate/unit_auth.md"))
	grWriteFile(t, root, "docs/specs/units/candidate/unit_auth.md", string(data)+"    affects:\n      files:\n        - other.js\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, root, id)
	if err := gaterun.WithMutation(root, func() error { return gaterun.ClaimShared(root, run, []string{"code:contracts.js"}) }); err != nil {
		t.Fatal(err)
	}
	if err := gaterun.WithMutation(root, func() error { return gaterun.ClaimShared(root, run, []string{"code:contracts.js", "code:other.js"}) }); err == nil {
		t.Fatal("same public task assigned to two batches")
	}
	p := filepath.Join(root, "meta/gate_runs", id, "run.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var old map[string]any
	json.Unmarshal(data, &old)
	old["schema_version"] = 1
	data, _ = json.Marshal(old)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := gaterun.Load(root, id); err == nil || !strings.Contains(err.Error(), "replan") {
		t.Fatalf("old run accepted: %v", err)
	}
}

func TestSharedProtectionUsesPublishedRuleAndRetainsReusedFailure(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	data, _ := os.ReadFile(filepath.Join(root, authSpec))
	stable := strings.Replace(string(data), "rule_refs: none", "rule_refs: b_rule_token", 1)
	grWriteFile(t, root, "docs/specs/units/stable/unit_auth.md", stable)
	grWriteFile(t, root, "docs/specs/rules/stable/b_rule_token.md", "---\nid: b_rule_token\n---\n\n## Constraint\nReturn token.\n")
	grWriteFile(t, root, "docs/specs/rules/candidate/b_rule_token.md", "---\nid: b_rule_token\n---\n\nUnfinished replacement.\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run := mustLoadRun(t, root, id)
	ck := run.CoverageByKey("preserve:auth:auth.core")
	if ck == nil {
		t.Fatal("no protection")
	}
	refs := strings.Join(ck.ReadRefs, "\n")
	if !strings.Contains(refs, "rules/stable/b_rule_token.md") || strings.Contains(refs, "rules/candidate/") {
		t.Fatal("protected rule uses candidate", refs)
	}
	report := grVerifyItemReport(ck.Key, "docs/specs/units/stable/unit_auth.md", "contracts.js") + ck.Key + ": docs/specs/rules/stable/b_rule_token.md: Constraint\n"
	if err := sharedSubmit(t, root, id, ck.Key, report); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, id)
	sharedValidateFixture(t, root, "order")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err != nil {
		t.Fatalf("promote: %v %s", err, out.String())
	}
	if result := fork.Fork(root, "order"); !result.Passed {
		t.Fatal(result.Issues)
	}
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("protected rule binding changed during fork: %+v %v", check, err)
	}

	// An accepted peer item with an advisory mismatch must still block preservation.
	// Its original record and severity remain immutable; only the current task is strict.
	newRoot, _, _ := sharedFixture(t)
	auth := grPlan(t, newRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	authRun := mustLoadRun(t, newRoot, auth)
	if err := sharedSubmit(t, newRoot, auth, "item:auth:auth.core", strings.ReplaceAll(grVerifyMismatchReport("item:auth:auth.core", authRun.RequiredFiles[0], "contracts.js", "P2"), "acceptance_item:item:auth:auth.core", "acceptance_item:auth.core")); err != nil {
		t.Fatal(err)
	}
	grAutoSubmitQuality(t, newRoot, auth)
	grSubmitOK(t, newRoot, auth, "cross", grCrossReport(authRun.RequiredFiles[0], "Description"))
	grFinalizeOK(t, newRoot, auth)
	original := grReadJudgmentBaseline(t, newRoot, "unit", "auth", "verify")
	specData, _ := os.ReadFile(filepath.Join(newRoot, authRun.RequiredFiles[0]))
	grWriteFile(t, newRoot, "docs/specs/units/stable/unit_auth.md", string(specData))
	if _, err := validationcache.RewriteCachesToStable(newRoot, "unit", "auth"); err != nil {
		t.Fatal(err)
	}
	order := grPlan(t, newRoot, "--gate", "verify", "--unit", "order", "--target", "candidate")
	orderRun := mustLoadRun(t, newRoot, order)
	if orderRun.CoverageByKey("preserve:auth:auth.core").Source != "reused" {
		t.Fatal("valid peer evidence not reused")
	}
	grAutoSubmitQuality(t, newRoot, order)
	grSubmitOK(t, newRoot, order, "order.core", grVerifyItemReport("order.core", orderRun.RequiredFiles[0], "contracts.js"))
	grSubmitOK(t, newRoot, order, "cross", grCrossReport(orderRun.RequiredFiles[0], "Description"))
	grFinalizeOK(t, newRoot, order)
	if check, err := checkUnitVerifyMerged(freshDerivation(t, newRoot), newRoot, "order", "candidate"); err != nil || check.Category != validationcache.CategoryBlocked {
		t.Fatalf("advisory peer mismatch released changes: %+v %v", check, err)
	}
	record, err := judgments.Load(newRoot, original.Records["item:auth:auth.core"].Reference)
	if err != nil {
		t.Fatal(err)
	}
	var res gaterun.SessionResult
	json.Unmarshal(record.Result, &res)
	if res.Findings[0].Severity != "P2" {
		t.Fatal("protection changed original peer severity")
	}
}

func TestSharedSurfaceViewKeepsStableJudgmentBesideDraft(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, id)
	before := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	data, _ := os.ReadFile(filepath.Join(root, authSpec))
	grWriteFile(t, root, "docs/specs/units/stable/unit_auth.md", string(data))
	grWriteFile(t, root, authSpec, strings.Replace(string(data), "Prose.", "Changed unfinished design.", 1))
	var out, errOut bytes.Buffer
	if err := runSurfaces([]string{"--repo-root", root, "--json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Judgments []surfaceJudgment `json:"judgments"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Judgments) != 1 {
		t.Fatalf("unexpected views: %s", out.String())
	}
	view := result.Judgments[0]
	if view.Designs["auth@stable"] == nil || view.Designs["auth@stable"].ID != before.Records["design:auth:contracts.js"].ID {
		t.Fatal("stable accepted design disappeared beside the draft", out.String())
	}
	if view.Designs["auth@candidate"] != nil || view.Designs["order@candidate"] != nil {
		t.Fatal("draft design falsely shown as reviewed", out.String())
	}
}

func TestSharedStableCallerProtectionWithoutAcceptedEvidence(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	data, _ := os.ReadFile(filepath.Join(root, authSpec))
	data = []byte(strings.Replace(string(data), "implementation_surface: contracts.js", "implementation_surface: login.js", 1))
	grWriteFile(t, root, "docs/specs/units/stable/unit_auth.md", string(data))
	grWriteFile(t, root, "login.js", "import {response} from './contracts.js';\nexport function login(token){return response(token);}\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run := mustLoadRun(t, root, id)
	ck := run.CoverageByKey("preserve:auth:auth.core")
	if ck == nil {
		t.Fatal("stable caller requirement escaped protection")
	}
	for _, path := range []string{"login.js", "contracts.js", "docs/specs/units/stable/unit_auth.md"} {
		if !stringInList(ck.ReadRefs, path) {
			t.Fatal("missing protected evidence", path, ck.ReadRefs)
		}
	}
}

func TestSharedDamagedPeerRecordKeepsEvidenceAssociation(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	data, _ := os.ReadFile(filepath.Join(root, authSpec))
	grWriteFile(t, root, authSpec, strings.Replace(string(data), "implementation_surface: contracts.js", "implementation_surface: login.js", 1))
	grWriteFile(t, root, "login.js", "export function login(){return global.createResponse();}\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--input", "contracts.js")
	sharedFinish(t, root, id)
	before := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	data, _ = os.ReadFile(filepath.Join(root, authSpec))
	grWriteFile(t, root, "docs/specs/units/stable/unit_auth.md", string(data))
	if _, err := validationcache.RewriteCachesToStable(root, "unit", "auth"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, judgments.Directory, before.Records["item:auth:auth.core"].ID+".json")
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	run := mustLoadRun(t, root, order)
	ck := run.CoverageByKey("preserve:auth:auth.core")
	if ck == nil || ck.Source == "reused" {
		t.Fatal("damaged peer evidence removed or released the protected requirement")
	}
	if !stringInList(ck.ReadRefs, "contracts.js") {
		t.Fatal("shared implementation missing from protection")
	}
}

func TestSharedRequiresPrivateSpecEvidenceAndTracksPublishedRules(t *testing.T) {
	root, authSpec, _ := sharedFixture(t)
	rule := "docs/specs/rules/stable/g_rule_contract.md"
	grWriteFile(t, root, rule, "---\nid: g_rule_contract\nrule_scope: global\n---\n\n## Constraint\nPreserve token.\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	body := grVerifyItemBody("item:auth:auth.core", "ALIGNED", "contracts.js:1") + "item:auth:auth.core: contracts.js: all\n"
	if err := sharedSubmit(t, root, id, "item:auth:auth.core", body); err == nil || !strings.Contains(err.Error(), "unit spec evidence") {
		t.Fatalf("unit judgment with no spec dependency was accepted: %v", err)
	}
	body += "item:auth:auth.core: " + authSpec + ": acceptance_item:auth.core\n"
	if err := sharedSubmit(t, root, id, "item:auth:auth.core", body); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, id)
	before := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	record, err := judgments.Load(root, before.Records["item:auth:auth.core"].Reference)
	if err != nil {
		t.Fatal(err)
	}
	tracked := false
	for _, dep := range record.Dependencies {
		if dep.Path == "rule:g_rule_contract" && dep.Hash != "" {
			tracked = true
		}
	}
	if !tracked {
		t.Fatal("published rule omitted from item dependencies")
	}
	file, err := os.OpenFile(filepath.Join(root, rule), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(strings.Repeat("Additional confirmed constraint.\n", 300)); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || check.Fresh {
		t.Fatalf("extended shared constraint kept old PASS: %+v %v", check, err)
	}
}

func TestTargetedInvalidationReachesProtectedConsumers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   string
		cache string
	}{
		{name: "pass cache and qualified key", key: "item:auth:auth.core", cache: "pass"},
		{name: "pass cache and item id", key: "auth.core", cache: "pass"},
		{name: "missing cache and item id", key: "auth.core", cache: "missing"},
		{name: "failure cache and item id", key: "auth.core", cache: "fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _, _ := sharedFixture(t)
			auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
			sharedFinish(t, root, auth)
			synthesisPromote(t, root, "auth")
			order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			sharedFinish(t, root, order)
			before, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
			if err != nil || !before.Fresh {
				t.Fatalf("fixture not fresh: %+v %v", before, err)
			}
			openConsumer := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			baseline := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
			item := baseline.Records["item:auth:auth.core"]
			public := baseline.Records["code:contracts.js"]
			cachePath := filepath.Join(root, "docs/specs/meta/validation/unit/auth/verify_result.md")
			if tc.cache == "missing" {
				if err := os.Remove(cachePath); err != nil {
					t.Fatal(err)
				}
			}
			if tc.cache == "fail" {
				data, err := os.ReadFile(cachePath)
				if err != nil {
					t.Fatal(err)
				}
				failure := strings.Replace(string(data), "result: pass", "result: fail", 1)
				failure = strings.Replace(failure, "blocking: false", "blocking: true", 1)
				failure = strings.Replace(failure, "p1_count: 0", "p1_count: 1", 1)
				if err := os.WriteFile(cachePath, []byte(failure), 0644); err != nil {
					t.Fatal(err)
				}
			}
			var out, errOut bytes.Buffer
			if err := runGateInvalidate([]string{"--repo-root", root, "--gate", "verify", "--unit", "auth", "--target", "stable", "--check", tc.key}, &out, &errOut); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Invalidated judgment(s): "+item.ID) {
				t.Fatalf("immutable invalidation was not disclosed: %s", out.String())
			}
			if _, err := judgments.Load(root, item.Reference); err != nil {
				t.Fatalf("history was deleted: %v", err)
			}
			if err := judgments.Check(root, public.Reference, "stable", judgments.Protocol(root)); err != nil {
				t.Fatalf("unrelated public evidence was invalidated: %v", err)
			}
			if tc.cache == "fail" {
				failure, err := validationcache.ReadGateBaseline(root, "unit", "auth", "verify")
				if err != nil || strings.Join(failure.InvalidatedChecks, ",") != "item:auth:auth.core" {
					t.Fatalf("failure record lost canonical invalidation: %+v %v", failure, err)
				}
			}
			after, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate")
			if err != nil || after.Fresh || !strings.Contains(after.Reason, "invalidated") {
				t.Fatalf("consumer freshness: %+v %v", after, err)
			}
			if err := runGateFinalize([]string{"--repo-root", root, "--run", openConsumer}, &out, &errOut); err == nil || !strings.Contains(err.Error(), "invalidated") {
				t.Fatalf("open consumer finalized contradicted evidence: %v", err)
			}
			sharedValidateFixture(t, root, "order")
			out.Reset()
			if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err == nil {
				t.Fatal("consumer promoted with a contradicted protected judgment")
			}
			retry := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate", "--mode", "delta")
			run := mustLoadRun(t, root, retry)
			if ck := run.CoverageByKey("preserve:auth:auth.core"); ck == nil || ck.Source != "executed" {
				t.Fatalf("invalidated item was reused: %+v", ck)
			}
			sharedFinish(t, root, retry)
			if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !check.Fresh {
				t.Fatalf("rechecked consumer not fresh: %+v %v", check, err)
			}
		})
	}
}

func TestTargetedInvalidationReachesPublicEvidenceFromOpenRun(t *testing.T) {
	root, _, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, root, id)
	key := "code:contracts.js"
	if err := sharedSubmit(t, root, id, key, grDefaultQualityReport(run, *run.CoverageByKey(key))); err != nil {
		t.Fatal(err)
	}
	consumer := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	peer := mustLoadRun(t, root, consumer)
	if peer.CoverageByKey(key).Source != "reused" {
		t.Fatal("fixture did not consume independently published public evidence")
	}
	var out, errOut bytes.Buffer
	if err := runGateInvalidate([]string{"--repo-root", root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--check", key}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if source := mustLoadRun(t, root, id); source.Status != gaterun.StatusInvalidated {
		t.Fatalf("source run remained usable: %s", source.Status)
	}
	if _, err := gaterun.LoadSessionStates(root, peer); err == nil || !strings.Contains(err.Error(), "invalidated") {
		t.Fatalf("consumer kept contradicted public evidence: %v", err)
	}
	retry := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	if ck := mustLoadRun(t, root, retry).CoverageByKey(key); ck.Source != "executed" {
		t.Fatalf("contradicted public evidence was reused: %+v", ck)
	}
	sharedFinish(t, root, retry)
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("public recheck did not restore freshness: %+v %v", check, err)
	}
}

func TestTargetedCandidateInvalidationKeepsDifferentStableContext(t *testing.T) {
	root, main, _ := sharedFixture(t)
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	synthesisPromote(t, root, "auth")
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)
	stable, accepted, err := judgments.LatestItem(root, "auth", "auth.core", "stable")
	if err != nil || !accepted {
		t.Fatalf("stable decision missing: %v", err)
	}
	if result := fork.Fork(root, "auth"); !result.Passed {
		t.Fatalf("fork: %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, main))
	if err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, main, strings.Replace(string(data), "description: Behavior.", "description: Revised candidate behavior.", 1))
	var out, errOut bytes.Buffer
	if err := runGateInvalidate([]string{"--repo-root", root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--check", "auth.core"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if err := judgments.Check(root, stable, "stable", judgments.Protocol(root)); err != nil {
		t.Fatalf("different stable context was invalidated: %v", err)
	}
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "order", "candidate"); err != nil || !check.Fresh {
		t.Fatalf("stable consumer affected by different candidate context: %+v %v", check, err)
	}
}

func TestArchitectureDeferralCanBeRetainedByOwner(t *testing.T) {
	root, main, _ := sharedFixture(t)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	run := mustLoadRun(t, root, id)
	for _, ck := range run.Coverage {
		report := grDefaultQualityReport(run, ck)
		if gaterun.IsItemKind(ck.Kind) {
			report = grVerifyItemReport(ck.Key, main, "contracts.js")
		}
		if ck.Kind == gaterun.SessionKindArchitecture {
			report = strings.Replace(report, "conclusion: acceptable", "conclusion: unacceptable — responsibility boundary violation", 1)
			report = strings.Replace(report, "gate_findings: none", "gate_findings: [P1] responsibility boundary violation", 1)
			report += "[P1] architecture — responsibility boundary violation (actionable)\n  problem: shared component violates the order responsibility boundary\n  evidence: contracts.js defines the shared response\n  impact: order changes require editing the auth boundary\n  fix: correct the order-owned responsibility placement\n"
		}
		if err := sharedSubmit(t, root, id, ck.Key, report); err != nil {
			t.Fatal(err)
		}
	}
	findingID := id + "/architecture:auth/F1"
	cross := "Cross-check: PASS — ownership assigned\nFinding disposition: " + findingID + " = retained\nFinding ownership: " + findingID + " = owned_by order — evidence: contracts.js; reason: order owns the affected response responsibility\ncross: contracts.js: all\n"
	for _, ck := range run.Coverage {
		cross += "Effective status: " + ck.Key + " = pass\n"
	}
	cross += "Effective status: cross = pass\n"
	cross = grCompleteCrossReport(t, root, run, cross)
	if err := sharedSubmit(t, root, id, "cross", cross); err != nil {
		t.Fatal(err)
	}
	grFinalizeOK(t, root, id)
	ownerID := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	owner := mustLoadRun(t, root, ownerID)
	if len(owner.DeferredFindings) != 1 {
		t.Fatalf("expected accepted deferral: %+v", owner.DeferredFindings)
	}
	deferred := owner.DeferredFindings[0]
	if deferred.Finding.SourceKey != "architecture:order" || deferred.SourceUnit != "auth" || deferred.SourceRun != id || deferred.Finding.ID != findingID {
		t.Fatalf("owner key or original provenance lost: %+v", deferred)
	}
	for _, ck := range owner.Coverage {
		states, err := gaterun.LoadSessionStates(root, owner)
		if err != nil {
			t.Fatal(err)
		}
		covered, _, err := gaterun.CoverageProgress(owner, states)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := covered[ck.Key]; ok {
			continue
		}
		report := grDefaultQualityReport(owner, ck)
		if gaterun.IsItemKind(ck.Kind) {
			report = grVerifyItemReport(ck.Key, owner.RequiredFiles[0], "contracts.js")
		}
		if err := sharedSubmit(t, root, ownerID, ck.Key, report); err != nil {
			t.Fatal(err)
		}
	}
	ownerCross := "Cross-check: PASS — existing finding retained\nFinding disposition: " + findingID + " = retained\ncross: contracts.js: all\n"
	for _, ck := range owner.Coverage {
		status := "pass"
		if ck.Kind == gaterun.SessionKindArchitecture {
			status = "fail"
		}
		ownerCross += "Effective status: " + ck.Key + " = " + status + "\n"
	}
	ownerCross += "Effective status: cross = pass\n"
	ownerCross = grCompleteCrossReport(t, root, owner, ownerCross)
	if err := sharedSubmit(t, root, ownerID, "cross", ownerCross); err != nil {
		t.Fatalf("owner cannot retain a valid architecture deferral: %v", err)
	}
	grFinalizeOK(t, root, ownerID)
	baseline := grReadJudgmentBaseline(t, root, "unit", "order", "verify")
	if baseline.LogicalStatus["architecture:order"] != "fail" || len(baseline.Findings) != 1 || baseline.Findings[0].ID != findingID {
		t.Fatalf("retained architecture finding did not drive the owner gate: %+v", baseline)
	}
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/order/verify_result.md")
	if !strings.Contains(cache, "result: fail") || !strings.Contains(cache, "blocking: true") || !strings.Contains(cache, "p1_count: 1") {
		t.Fatalf("owner failure record missing: %s", cache)
	}
	if ledger := grReadDeferredLedger(t, root); len(ledger.Entries) != 0 {
		t.Fatalf("handled deferral was not consumed: %+v", ledger.Entries)
	}
}

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
		if ck.Kind == gaterun.SessionKindDeltaReview {
			grSubmitOK(t, root, id, "review", "Review result: accept — shared fixture review\n")
			continue
		}
		report := grDefaultQualityReport(run, ck)
		if gaterun.IsItemKind(ck.Kind) {
			report = grVerifyItemReport(ck.Key, run.RequiredFiles[0], "contracts.js")
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
func TestSharedPrivateDeltaAndUndeclaredCaller(t *testing.T) {
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
	if err != nil || !fresh.Fresh {
		t.Fatalf("undeclared new caller must not stale verify on its own: %+v %v", fresh, err)
	}
	// The coordinator supplies a discovered consumer explicitly; only then does
	// it join the run's input surface and get judged.
	delta = grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--inputs-file", grInputsManifest(t, "order.js"))
	run, err = gaterun.Load(root, delta)
	if err != nil {
		t.Fatal(err)
	}
	if !stringInList(run.ExtraInputs, "order.js") {
		t.Fatalf("supplied consumer did not join the run's input surface: %+v", run.ExtraInputs)
	}
}

func sharedValidateFixture(t *testing.T, root, unit string) {
	t.Helper()
	entry, err := validationcache.BuildEvidenceEntry(root, "docs/specs/units/candidate/unit_"+unit+".md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(root, "unit", unit, validationcache.CacheWrite{Command: "validate", Unit: unit, Mode: "full", Result: "pass", Target: "candidate", Entries: []validationcache.FileEntry{*entry}}); err != nil {
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

// TestSharedItemJudgmentPinsItsSpecAndTracksPublishedRules pins the record
// contract: an item judgment pins its own spec item region regardless of the
// report's prose, and the published rule it read is recorded in the input
// surface, so extending the rule stales the gate.
func TestSharedItemJudgmentPinsItsSpecAndTracksPublishedRules(t *testing.T) {
	root, _, _ := sharedFixture(t)
	rule := "docs/specs/rules/stable/g_rule_contract.md"
	grWriteFile(t, root, rule, "---\nid: g_rule_contract\nrule_scope: global\n---\n\n## Constraint\nPreserve token.\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	body := grVerifyItemBody("item:auth:auth.core", "ALIGNED", "contracts.js:1") + "item:auth:auth.core: contracts.js: all\n"
	if err := sharedSubmit(t, root, id, "item:auth:auth.core", body); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, id)
	before := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	record, err := judgments.Load(root, before.Records["item:auth:auth.core"].Reference)
	if err != nil {
		t.Fatal(err)
	}
	regionPinned := false
	for _, dep := range record.Dependencies {
		for _, d := range dep.Deps {
			if strings.HasPrefix(d, "region:acceptance_item:auth.core:") {
				regionPinned = true
			}
		}
	}
	if !regionPinned {
		t.Fatalf("item judgment does not pin its own spec item region: %+v", record.Dependencies)
	}
	baseline, err := validationcache.ReadGateBaseline(root, "unit", "auth", "verify")
	if err != nil {
		t.Fatal(err)
	}
	tracked := false
	for _, entry := range baseline.Entries {
		if entry.Path == "rule:g_rule_contract" {
			tracked = true
		}
	}
	if !tracked {
		t.Fatal("published rule omitted from the recorded input surface")
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
	// The invalidated source run's state is removed by the invalidation, so
	// it can no longer be loaded.
	if _, err := gaterun.Load(root, id); err == nil || !strings.Contains(err.Error(), "cannot read gate run") {
		t.Fatalf("invalidated source run state must be removed, got %v", err)
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
	grWriteFile(t, root, main, strings.Replace(string(data), "description: Given a caller, When the behavior runs, Then it is accepted.", "description: Revised candidate behavior.", 1))
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

// TestPromoteReportsStaleStablePeers pins the promote-time impact sweep: a
// promoted change stales the confirmation caches of stable units whose
// declared evidence covers the changed content, and the promote output must
// make that impact surface loud (dependency freshness is the propagation
// path — see framework/shared_judgments.md).
func TestPromoteReportsStaleStablePeers(t *testing.T) {
	root, _, _ := sharedFixture(t)
	order := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
	sharedFinish(t, root, order)
	sharedValidateFixture(t, root, "order")
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "order"}, &out, &errOut); err != nil {
		t.Fatalf("promote order: %v %s", err, out.String())
	}
	// Change the shared code file both units declare: order's stable verify
	// confirmation covers it, so the change stales order.
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token, changed: true}; }\n")
	auth := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	sharedFinish(t, root, auth)
	sharedValidateFixture(t, root, "auth")
	out.Reset()
	errOut.Reset()
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatalf("promote auth: %v %s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "Impact:") || !strings.Contains(got, "order: verify STALE") {
		t.Fatalf("promote did not report the staled stable peer:\n%s", got)
	}
	if strings.Contains(got, "auth: verify") {
		t.Fatalf("promote reported the promoted unit itself:\n%s", got)
	}
}

// TestGateExtendKeepsAcceptedSessions pins the missing-read-ref recovery
// path: the coordinator extends the open run instead of replanning, so the
// accepted session keeps its verdict, the run id is unchanged, and the
// extended input becomes readable by the remaining sessions.
func TestGateExtendKeepsAcceptedSessions(t *testing.T) {
	root, main, _ := sharedFixture(t)
	grEnableMissionLayout(t, root)
	grWriteFile(t, root, "extra/helper.js", "export const helper = true;\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	key := "item:auth:auth.core"
	if err := sharedSubmit(t, root, id, key, grVerifyItemReport(key, main, "contracts.js")); err != nil {
		t.Fatal(err)
	}
	planned := mustLoadRun(t, root, id)
	if err := sharedSubmit(t, root, id, "code:contracts.js", grDefaultQualityReport(planned, *planned.CoverageByKey("code:contracts.js"))); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := runGateExtend([]string{"--repo-root", root, "--run", id, "--paths", "extra/helper.js"}, &out, &errOut); err != nil {
		t.Fatalf("gate-extend: %v %s", err, out.String())
	}
	run := mustLoadRun(t, root, id)
	found := false
	for _, ref := range run.Refs {
		if ref.Ref == "extra/helper.js" && ref.Source == gaterun.SourceInput {
			found = true
		}
	}
	if !found {
		t.Fatalf("extended input missing from the snapshot: %+v", run.Refs)
	}
	states, err := gaterun.LoadSessionStates(root, run)
	if err != nil {
		t.Fatal(err)
	}
	covered, _, err := gaterun.CoverageProgress(run, states)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := covered[key]; !ok {
		t.Fatalf("extending the run lost the accepted session for %s", key)
	}
	out.Reset()
	errOut.Reset()
	if err := runGateMission([]string{"--repo-root", root, "--run", id, "--keys", "design:auth:contracts.js", "--format", "json"}, &out, &errOut); err != nil {
		t.Fatalf("gate-mission: %v %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "extra/helper.js") {
		t.Fatalf("extended input is not readable by the remaining sessions:\n%s", out.String())
	}
}

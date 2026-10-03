package main

import (
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

func TestVerifyContextReplanCarriesTestAndCalleeEvidence(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecSurface(t, root, "auth", "src/auth.go", "")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	grWriteFile(t, root, "tests/auth_test.go", "package tests\n")
	grWriteFile(t, root, "internal/helper.go", "package internal\n")
	main := "docs/specs/units/candidate/unit_auth.md"

	oldRun := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	oldMission := missionJSON(t, root, oldRun, "auth.core")
	if strings.Contains(strings.Join(oldMission.Sessions[0].ReadRefs, ","), "tests/auth_test.go") || !strings.Contains(strings.Join(oldMission.Constraints, " "), "missing read ref") {
		t.Fatalf("initial mission did not expose the missing-context boundary: %+v", oldMission.Sessions[0])
	}
	oldState, err := grLoadSessionState(root, mustLoadRun(t, root, oldRun), "auth.core")
	if err != nil || oldState.Status != gaterun.SessionPending {
		t.Fatalf("an incomplete report must leave the session pending: state=%+v err=%v", oldState, err)
	}
	if _, err := grSubmitRaw(t, root, oldRun, "auth.core", "Verification could not complete — missing read ref: tests/auth_test.go\n"); err == nil {
		t.Fatal("an incomplete report was accepted as a verdict")
	}

	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate",
		"--input", "tests/auth_test.go", "--input", "internal/helper.go")
	if _, err := gaterun.Load(root, oldRun); err == nil {
		t.Fatal("the replaced run is still available")
	}
	newRun := mustLoadRun(t, root, runID)
	spec, err := grBuildSessionSpec(root, newRun, []string{"auth.core"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tests/auth_test.go", "internal/helper.go"} {
		if !strings.Contains(strings.Join(spec.ReadRefs, ","), want) {
			t.Fatalf("auth.core session lacks discovered evidence %q: %+v", want, spec.ReadRefs)
		}
	}

	detection := strings.Replace(grVerifyItemBody("auth.core", "MISMATCH (acceptance)", "src/auth.go:1 differs"),
		"Part B: skipped — no test files in this fixture", "Part B: B1-B6 checked", 1) +
		grVerifyAnalysisFields("auth.core", "P2") +
		"auth.core: " + main + ": acceptance_item:auth.core\n" +
		"auth.core: src/auth.go: all\n" +
		"auth.core: tests/auth_test.go: all\n" +
		"auth.core: internal/helper.go: all\n"
	grSubmitOK(t, root, runID, "auth.core", detection)
	grSubmitOK(t, root, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	for _, path := range []string{"tests/auth_test.go", "internal/helper.go"} {
		if !strings.Contains(cache, path) {
			t.Fatalf("verify cache lost %s evidence:\n%s", path, cache)
		}
	}
}

func TestVerifyReplanDiscardsAcceptedSessions(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecSurface(t, root, "auth", "src/auth.go", "")
	grWriteFile(t, root, "src/auth.go", "package auth\n")
	grWriteFile(t, root, "tests/auth_test.go", "package tests\n")
	grWriteFile(t, root, "internal/helper.go", "package internal\n")
	main := "docs/specs/units/candidate/unit_auth.md"

	oldRun := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate",
		"--input", "tests/auth_test.go")
	accepted := strings.Replace(grVerifyItemReport("auth.core", main, "src/auth.go"),
		"Part B: skipped — no test files in this fixture", "Part B: B1-B6 checked", 1) +
		"auth.core: tests/auth_test.go: all\n"
	grSubmitOK(t, root, oldRun, "auth.core", accepted)
	oldState, err := grLoadSessionState(root, mustLoadRun(t, root, oldRun), "auth.core")
	if err != nil || oldState.Status != gaterun.SessionAccepted {
		t.Fatalf("fixture did not accept the old session: state=%+v err=%v", oldState, err)
	}

	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate",
		"--input", "tests/auth_test.go", "--input", "internal/helper.go")
	if _, err := gaterun.Load(root, oldRun); err == nil {
		t.Fatal("the old accepted result survived replanning")
	}
	newState, err := grLoadSessionState(root, mustLoadRun(t, root, runID), "auth.core")
	if err != nil || newState.Status != gaterun.SessionPending || newState.Result != nil {
		t.Fatalf("the replacement run inherited an old judgment: state=%+v err=%v", newState, err)
	}
}

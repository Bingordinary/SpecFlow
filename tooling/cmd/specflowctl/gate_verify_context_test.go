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
	oldMission := missionJSON(t, root, oldRun, "detect:auth.core")
	if strings.Contains(strings.Join(oldMission.Packets[0].ReadRefs, ","), "tests/auth_test.go") || !strings.Contains(strings.Join(oldMission.Constraints, " "), "missing read ref") {
		t.Fatalf("initial mission did not expose the missing-context boundary: %+v", oldMission.Packets[0])
	}
	oldState, err := gaterun.LoadPacketState(root, mustLoadRun(t, root, oldRun), "detect:auth.core")
	if err != nil || oldState.Status != gaterun.PacketPending {
		t.Fatalf("an incomplete review must leave the packet pending: state=%+v err=%v", oldState, err)
	}
	if _, err := grSubmitRaw(t, root, oldRun, "detect:auth.core", "Verification could not complete — missing read ref: tests/auth_test.go\n"); err == nil {
		t.Fatal("an incomplete report was accepted as a verdict")
	}

	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate",
		"--input", "tests/auth_test.go", "--input", "internal/helper.go")
	if _, err := gaterun.Load(root, oldRun); err == nil {
		t.Fatal("the replaced run is still available")
	}
	for _, packetID := range []string{"detect:auth.core", "analysis:auth.core"} {
		packet := mustLoadRun(t, root, runID).PacketByID(packetID)
		if packet == nil || !strings.Contains(strings.Join(packet.ReadRefs, ","), "tests/auth_test.go") || !strings.Contains(strings.Join(packet.ReadRefs, ","), "internal/helper.go") {
			t.Fatalf("%s lacks discovered evidence: %+v", packetID, packet)
		}
	}

	detection := strings.Replace(grVerifyItemBody("auth.core", "MISMATCH (acceptance)", "src/auth.go:1 differs"),
		"Part B: skipped — no test files in this fixture", "Part B: B1-B6 checked", 1) +
		"auth.core: " + main + ": acceptance_item:auth.core\n" +
		"auth.core: src/auth.go: all\n" +
		"auth.core: tests/auth_test.go: all\n" +
		"auth.core: internal/helper.go: all\n"
	grSubmitOK(t, root, runID, "detect:auth.core", detection)
	analysisMission := missionJSON(t, root, runID, "analysis:auth.core")
	if len(analysisMission.Packets[0].Dependencies) != 1 || !strings.Contains(analysisMission.Packets[0].Dependencies[0].Report, "tests/auth_test.go") {
		t.Fatalf("analysis mission lost the detection evidence: %+v", analysisMission.Packets[0])
	}
	analysis := grVerifyAnalysisReport("auth.core", main, "src/auth.go", "P2") +
		"auth.core: tests/auth_test.go: all\n" +
		"auth.core: internal/helper.go: all\n"
	grSubmitOK(t, root, runID, "analysis:auth.core", analysis)
	grSubmitOK(t, root, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, root, runID)
	cache := grReadCache(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md")
	for _, path := range []string{"tests/auth_test.go", "internal/helper.go"} {
		if !strings.Contains(cache, path) {
			t.Fatalf("verify cache lost %s evidence:\n%s", path, cache)
		}
	}
}

func TestVerifyReplanDiscardsAcceptedPackets(t *testing.T) {
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
	grSubmitOK(t, root, oldRun, "detect:auth.core", accepted)
	oldState, err := gaterun.LoadPacketState(root, mustLoadRun(t, root, oldRun), "detect:auth.core")
	if err != nil || oldState.Status != gaterun.PacketAccepted {
		t.Fatalf("fixture did not accept the old packet: state=%+v err=%v", oldState, err)
	}

	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate",
		"--input", "tests/auth_test.go", "--input", "internal/helper.go")
	if _, err := gaterun.Load(root, oldRun); err == nil {
		t.Fatal("the old accepted result survived replanning")
	}
	newState, err := gaterun.LoadPacketState(root, mustLoadRun(t, root, runID), "detect:auth.core")
	if err != nil || newState.Status != gaterun.PacketPending || newState.Result != nil {
		t.Fatalf("the replacement run inherited an old judgment: state=%+v err=%v", newState, err)
	}
}

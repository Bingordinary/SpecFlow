package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// Follow the mission's public observations verbatim. A public execution batch
// must not enlarge the evidence assigned to a unit-design session.
func submitDesignWithMissionObservations(t *testing.T, root, id string, keys []string, carried bool) {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := runGateMission([]string{"--repo-root", root, "--run", id, "--keys", strings.Join(keys, ","), "--format", "json"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var mission gateMission
	if err := json.Unmarshal(out.Bytes(), &mission); err != nil {
		t.Fatal(err)
	}
	session := mission.Sessions[0]
	inputs := session.Dependencies
	if carried {
		if len(inputs) != 0 {
			t.Fatalf("carried public judgments appeared as executed dependencies: %+v", inputs)
		}
		inputs = session.CarriedResults
	} else if len(session.CarriedResults) != 0 {
		t.Fatalf("unrelated carried results leaked into design: %+v", session.CarriedResults)
	}
	wanted := map[string]bool{}
	run := mustLoadRun(t, root, id)
	var report strings.Builder
	for _, key := range keys {
		ck := run.CoverageByKey(key)
		wanted["code:"+ck.File] = true
		report.WriteString(grDefaultQualityReport(run, *ck))
	}
	if len(inputs) != len(wanted) {
		t.Fatalf("design got %d public records for %d assigned files: %+v", len(inputs), len(wanted), inputs)
	}
	for _, input := range inputs {
		if !wanted[input.SessionID] || len(input.Verdicts) != 1 || len(input.Observations) != 1 {
			t.Fatalf("design input is not its assigned file's public record: %+v", input)
		}
		if input.Verdicts[input.SessionID] != "facts" || input.Observations[0].SourceKey != input.SessionID {
			t.Fatalf("public record lost its file identity: %+v", input)
		}
		for key := range input.Analysis {
			if key != input.SessionID {
				t.Fatalf("unassigned public analysis leaked into design: %+v", input)
			}
		}
		delete(wanted, input.SessionID)
		fmt.Fprintf(&report, "Observation disposition: %s = suppressed — the unit spec justifies this file's responsibility\n", input.Observations[0].ID)
	}
	p := grWriteFile(t, t.TempDir(), "design.md", report.String())
	if err := runGateSubmit([]string{"--repo-root", root, "--run", id, "--session", session.SessionID, "--keys", strings.Join(keys, ","), "--report", p}, &out, &errOut); err != nil {
		t.Fatalf("following the generated design mission was rejected: %v", err)
	}
}

func finishDesignInputRun(t *testing.T, root, id, main, code string) {
	t.Helper()
	run := mustLoadRun(t, root, id)
	for _, ck := range run.Coverage {
		if ck.Kind != gaterun.SessionKindItem && ck.Kind != gaterun.SessionKindArchitecture {
			continue
		}
		report := grDefaultQualityReport(run, ck)
		if ck.Kind == gaterun.SessionKindItem {
			report = grVerifyItemReport(ck.Key, main, code)
		}
		if err := sharedSubmit(t, root, id, ck.Key, report); err != nil {
			t.Fatal(err)
		}
	}
	grFinalizeOK(t, root, id)
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, run.TargetName, "candidate"); err != nil || !check.Fresh {
		t.Fatalf("completed design run is not fresh: %+v %v", check, err)
	}
}

func TestGateDesignMissionUsesAssignedPublicRecords(t *testing.T) {
	for _, batchDesign := range []bool{false, true} {
		t.Run(fmt.Sprintf("batch_design=%t", batchDesign), func(t *testing.T) {
			root := createCLITestRepo(t)
			grEnableMissionLayout(t, root)
			grWriteSpec(t, root, "auth")
			for _, path := range []string{"src/a.go", "src/b.go"} {
				grWriteFile(t, root, path, "package src\n")
			}
			id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
			run := mustLoadRun(t, root, id)
			publicKeys := []string{"code:src/a.go", "code:src/b.go"}
			var report strings.Builder
			for _, key := range publicKeys {
				report.WriteString(grDefaultQualityReport(run, *run.CoverageByKey(key)))
				fmt.Fprintf(&report, "[P2] %s:1 — public observation (actionable)\n  problem: responsibility is unclear\n  evidence: package src\n  impact: maintenance is harder\n  fix: clarify the boundary\n", strings.TrimPrefix(key, "code:"))
			}
			p := grWriteFile(t, t.TempDir(), "public.md", report.String())
			var out, errOut bytes.Buffer
			if err := runGateSubmit([]string{"--repo-root", root, "--run", id, "--session", gaterun.SessionID(publicKeys), "--keys", strings.Join(publicKeys, ","), "--report", p}, &out, &errOut); err != nil {
				t.Fatal(err)
			}
			designKeys := []string{"design:auth:src/a.go", "design:auth:src/b.go"}
			if batchDesign {
				submitDesignWithMissionObservations(t, root, id, designKeys, false)
			} else {
				for _, key := range designKeys {
					submitDesignWithMissionObservations(t, root, id, []string{key}, false)
				}
			}
			finishDesignInputRun(t, root, id, run.RequiredFiles[0], "src/a.go")

			// Re-execute one design while carrying both public records and the
			// unrelated unit judgments. Only its own public record is an input.
			delta := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "delta", "--rerun", designKeys[0])
			grReviewAccept(t, root, delta)
			submitDesignWithMissionObservations(t, root, delta, designKeys[:1], true)
			finishDesignInputRun(t, root, delta, run.RequiredFiles[0], "src/a.go")

			// Another unit reuses only a.go from the original two-file batch.
			grWriteSpecSurface(t, root, "order", "src/a.go", "")
			peer := grPlan(t, root, "--gate", "verify", "--unit", "order", "--target", "candidate")
			peerRun := mustLoadRun(t, root, peer)
			if peerRun.CoverageByKey("code:src/a.go").Source != "reused" {
				t.Fatal("expected a shared public record")
			}
			submitDesignWithMissionObservations(t, root, peer, []string{"design:order:src/a.go"}, false)
			finishDesignInputRun(t, root, peer, peerRun.RequiredFiles[0], "src/a.go")
		})
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func TestGateInvalidatePersistsFailureRecordWithoutRerunFlag(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	entry, err := validationcache.BuildEvidenceEntry(repoRoot, "docs/specs/units/candidate/unit_auth.md")
	if err != nil {
		t.Fatal(err)
	}
	entry.Checks = []validationcache.CheckEntry{{Check: "auth.core", Status: "pass"}}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   "verify",
		Unit:      "auth",
		Mode:      "full",
		Basis:     "full",
		Result:    "fail",
		Target:    "candidate",
		Blocking:  true,
		P1Count:   1,
		Timestamp: "2026-09-19T00:00:00Z",
		Judgments: `{"schema_version":3,"logical_status":{"auth.core":"pass"},"findings":[],"synthesis_digest":"sha256:test"}`,
		Entries:   []validationcache.FileEntry{*entry},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = runGateInvalidate([]string{
		"--repo-root", repoRoot,
		"--gate", "verify",
		"--unit", "auth",
		"--target", "candidate",
		"--check", "auth.core",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-invalidate failed: %v (stderr=%s)", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Persisted check(s): item:auth:auth.core") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
	baseline, err := validationcache.ReadGateBaseline(repoRoot, "unit", "auth", "verify")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(baseline.InvalidatedChecks, ",") != "item:auth:auth.core" {
		t.Fatalf("persisted invalidations = %v", baseline.InvalidatedChecks)
	}
}

func TestGateInvalidateRequiresCheck(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	var stdout, stderr bytes.Buffer
	err := runGateInvalidate([]string{
		"--repo-root", repoRoot,
		"--gate", "verify",
		"--unit", "auth",
		"--target", "candidate",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--check") {
		t.Fatalf("expected missing-check error, got %v", err)
	}
}

func TestGateInvalidatePreventsOlderRunFromFinalizing(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	grWriteSpec(t, repoRoot, "auth")
	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate")

	var stdout, stderr bytes.Buffer
	if err := runGateInvalidate([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--target", "candidate",
		"--check", "2",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("gate-invalidate failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Invalidated open gate run(s): "+runID) {
		t.Fatalf("open-run invalidation was not disclosed: %s", stdout.String())
	}
	// The invalidated run's state is removed by the invalidation itself, so an
	// older finalize can no longer load it.
	if _, err := grFinalize(t, repoRoot, runID); err == nil || !strings.Contains(err.Error(), "cannot read gate run") {
		t.Fatalf("expected older finalize to fail on the removed run, got %v", err)
	}
}

func TestRepairFinalizeClearsPersistedTargetedInvalidations(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	main := "docs/specs/units/candidate/unit_auth.md"
	grWriteSpec(t, repoRoot, "auth")

	statuses := map[string]string{}
	var checks []validationcache.CheckEntry
	for _, key := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", gaterun.ClarityCheck, gaterun.CrossKey} {
		status := "pass"
		if key == "1" {
			status = "fail"
		}
		statuses[key] = status
		checks = append(checks, validationcache.CheckEntry{Check: key, Status: status})
	}
	entry, err := validationcache.BuildEvidenceEntry(repoRoot, main)
	if err != nil {
		t.Fatal(err)
	}
	entry.Checks = checks
	judgments, err := json.Marshal(gaterun.JudgmentBaseline{SchemaVersion: 3, LogicalStatus: statuses, SynthesisDigest: "sha256:test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validationcache.WriteCache(repoRoot, "unit", "auth", validationcache.CacheWrite{
		Command:   "validate",
		Unit:      "auth",
		Mode:      "full",
		Basis:     "delta",
		Result:    "fail",
		Target:    "candidate",
		Blocking:  true,
		P1Count:   1,
		Timestamp: "2026-09-19T00:00:00Z",
		Judgments: string(judgments),
		Entries:   []validationcache.FileEntry{*entry},
	}); err != nil {
		t.Fatal(err)
	}

	var invalidateOut, invalidateErr bytes.Buffer
	if err := runGateInvalidate([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--target", "candidate",
		"--check", "2",
	}, &invalidateOut, &invalidateErr); err != nil {
		t.Fatalf("gate-invalidate failed: %v (stderr=%s)", err, invalidateErr.String())
	}

	runID := grPlan(t, repoRoot, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "repair")
	run := mustLoadRun(t, repoRoot, runID)
	if got := strings.Join(coverageKeysOf(run), ","); got != "structural,design" {
		t.Fatalf("repair did not include persisted check 2's group: %s", got)
	}
	grSubmitOK(t, repoRoot, runID, "structural", grValidateReport([]string{"1", "3", "6"}, map[string][]string{
		"1": {main + ": Description"},
		"3": {main + ": Description"},
		"6": {main + ": Description"},
	}))
	grSubmitOK(t, repoRoot, runID, "design", grValidateReport([]string{"2", "4"}, map[string][]string{
		"2": {main + ": Description"},
		"4": {main + ": Description"},
	}))
	grSubmitClarity(t, repoRoot, runID, main)
	grSubmitOK(t, repoRoot, runID, "cross", grCrossReport(main, "Description"))
	grFinalizeOK(t, repoRoot, runID)

	baseline, err := validationcache.ReadGateBaseline(repoRoot, "unit", "auth", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.InvalidatedChecks) != 0 {
		t.Fatalf("successful finalize retained invalidations: %v", baseline.InvalidatedChecks)
	}
	cache := grReadCache(t, repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")
	if strings.Contains(cache, "invalidated_checks:") {
		t.Fatalf("successful finalize did not clear invalidated_checks:\n%s", cache)
	}
}

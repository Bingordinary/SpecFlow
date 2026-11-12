package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// editSynthesisStandard changes the installed final-synthesis judging text in
// the fixture framework, leaving the local session classes' sections untouched.
func editSynthesisStandard(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "framework", "verification_scope.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Fixture Final synthesis.", "Fixture Final synthesis, edited.", 1)
	if edited == string(data) {
		t.Fatal("the final-synthesis fixture text was not found")
	}
	if err := os.WriteFile(path, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestVerifySessionFingerprintBindsSynthesisOnlyToCross pins the per-class
// split: editing the final-synthesis judging text must change the cross
// class's fingerprint and leave every local class's fingerprint unchanged.
func TestVerifySessionFingerprintBindsSynthesisOnlyToCross(t *testing.T) {
	root, _ := deltaBaseline(t, "", nil)
	before := map[string]string{}
	for _, kind := range judgments.VerifySessionKinds {
		before[kind] = judgments.MustVerifySessionFingerprint(root, kind)
	}
	editSynthesisStandard(t, root)
	for _, kind := range judgments.VerifySessionKinds {
		after := judgments.MustVerifySessionFingerprint(root, kind)
		if kind == judgments.ProtocolSessionCross {
			if after == before[kind] {
				t.Fatalf("a final-synthesis edit must change the cross fingerprint")
			}
			continue
		}
		if after != before[kind] {
			t.Fatalf("class %q must not be bound to the final-synthesis text", kind)
		}
	}
}

// TestVerifyDeltaRechecksSynthesisWhenItsStandardMoves pins the delta rule:
// a moved final-synthesis standard re-runs the synthesis over the carried
// local results and re-checks every standing relationship, without re-running
// a local class.
func TestVerifyDeltaRechecksSynthesisWhenItsStandardMoves(t *testing.T) {
	root, _ := deltaBaseline(t, "", nil)
	editSynthesisStandard(t, root)

	// The cache cannot prove the relationship verdict is current.
	check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if check.Fresh {
		t.Fatalf("a moved final-synthesis standard must stale the cache")
	}
	if !strings.Contains(check.Reason, "final-synthesis") {
		t.Fatalf("the stale reason must name the final-synthesis standard, got %q", check.Reason)
	}

	run := deltaPlan(t, root)
	if !run.SynthesisRerun {
		t.Fatalf("a moved synthesis standard must force the synthesis to re-run")
	}
	if !grContainsString(run.Relationships, "contract_consistency") {
		t.Fatalf("the standing relationship must re-check, got %v", run.Relationships)
	}
	if grContainsString(run.CarriedKeys, "relationship:contract_consistency") {
		t.Fatalf("the standing relationship must not carry, got %v", run.CarriedKeys)
	}
	if len(run.Coverage) != 0 {
		t.Fatalf("a synthesis-only re-run must carry every local class, got coverage %v", coverageKeysOf(run))
	}
	for _, key := range []string{"unit:auth", "item:auth:auth.core", "code:contracts.js", "design:auth:contracts.js", "architecture:auth"} {
		if !grContainsString(run.CarriedKeys, key) {
			t.Fatalf("local conclusion %q must carry, got %v", key, run.CarriedKeys)
		}
	}
}

// TestVerifyCacheWithoutSynthesisStandardIsStale pins the no-migration rule:
// a verify cache that records a synthesis conclusion but no `standards.cross`
// entry cannot prove the relationship verdict is current, so freshness fails
// closed and the synthesis re-runs.
func TestVerifyCacheWithoutSynthesisStandardIsStale(t *testing.T) {
	root, _ := deltaBaseline(t, "", nil)
	path := filepath.Join(root, "docs", "specs", "meta", "validation", "unit", "auth", "verify_result.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	var stripped []string
	skipping := false
	for _, line := range lines {
		if line == "standards:" {
			skipping = true
			continue
		}
		if skipping {
			if strings.HasPrefix(line, "  ") {
				continue
			}
			skipping = false
		}
		stripped = append(stripped, line)
	}
	without := strings.Join(stripped, "\n")
	if without == string(data) {
		t.Fatalf("the fixture cache carries no standards block to strip")
	}
	if err := os.WriteFile(path, []byte(without), 0644); err != nil {
		t.Fatal(err)
	}
	check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if check.Fresh {
		t.Fatalf("a verify cache without its final-synthesis standard must be stale")
	}
	if !strings.Contains(check.Reason, "final-synthesis") {
		t.Fatalf("the stale reason must name the final-synthesis standard, got %q", check.Reason)
	}
}

// TestVerifyFullRunRechecksSynthesisWhenItsStandardMoves pins the full-run
// half: the standing relationship is not carried when the final-synthesis
// standard moved, even though the unit content did not change.
func TestVerifyFullRunRechecksSynthesisWhenItsStandardMoves(t *testing.T) {
	root, _ := deltaBaseline(t, "", nil)
	editSynthesisStandard(t, root)

	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--relationships", "none")
	run := mustLoadRun(t, root, id)
	if !run.SynthesisRerun {
		t.Fatalf("a moved synthesis standard must force the full run's synthesis")
	}
	if !grContainsString(run.Relationships, "contract_consistency") {
		t.Fatalf("the standing relationship must re-check, got %v", run.Relationships)
	}
	if grContainsString(run.CarriedKeys, "relationship:contract_consistency") {
		t.Fatalf("the standing relationship must not carry, got %v", run.CarriedKeys)
	}
}

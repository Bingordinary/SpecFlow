package gaterun

import (
	"testing"
	"time"
)

// A unit spec's physical spelling in the input manifest must plan exactly
// like its logical spelling: the snapshot must carry unit:<name>, or the
// recorded evidence — the synthesis's own evidence pins included — would
// dangle when a promote moves the file from candidate/ to stable/.
func TestPlanNormalizesPhysicalUnitSpecInputToLogicalRef(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src/a.go", "")
	writeUnit(t, repoRoot, "candidate", "dep", "none", "none", "src/d.go", "")
	writeFile(t, repoRoot, "src/a.go", "package a\n")
	writeFile(t, repoRoot, "src/d.go", "package d\n")
	writeFile(t, repoRoot, "docs/specs/units/candidate/evidence/unit_dep.md",
		"---\nunit: dep\n---\n\nProse.\n")

	physicalSpec := "docs/specs/units/candidate/unit_dep.md"
	physicalEvidence := "docs/specs/units/candidate/evidence/unit_dep.md"
	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "auth", TargetCandidate, ModeFull, []string{physicalSpec, physicalEvidence}, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"unit:dep", "unit:dep:evidence"}
	if len(run.ExtraInputs) != len(want) {
		t.Fatalf("extra inputs = %v, want %v", run.ExtraInputs, want)
	}
	for i := range want {
		if run.ExtraInputs[i] != want[i] {
			t.Fatalf("extra inputs = %v, want %v", run.ExtraInputs, want)
		}
	}
	for _, ref := range run.Refs {
		if ref.Ref == physicalSpec || ref.Ref == physicalEvidence {
			t.Fatalf("a physical spec path reached the snapshot: %+v", run.Refs)
		}
	}
	for _, ref := range want {
		bound, ok := refByRef(run, ref)
		if !ok || bound.Hash == "" {
			t.Fatalf("logical reference %q missing from the snapshot: %+v", ref, run.Refs)
		}
		if !run.AllowsDeclaration(repoRoot, ref) {
			t.Fatalf("logical reference %q must be declarable: %+v", ref, run.Refs)
		}
	}
}

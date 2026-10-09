package gaterun

import (
	"testing"
	"time"
)

// TestCodeEvidenceIsDeclaredSurfaceNotNameTokenClosure is the #73 regression:
// a code key's evidence surface is the spec-declared file, not every
// repository file whose name stem matches a token in that file. A React <span>
// in an unrelated .tsx must not link to contracts/span.go, must not appear in
// the code key's read refs, and must not enter the run's recorded input
// surface (so an edit to it can never stale the run).
func TestCodeEvidenceIsDeclaredSurfaceNotNameTokenClosure(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "contracts/span.go", "package contracts\n\ntype Span struct{}\n")
	writeFile(t, repoRoot, "ui/dialog.tsx", "export const Dialog = () => <span className=\"sr-only\">Close</span>;\n")
	writeUnit(t, repoRoot, "candidate", "trace", "none", "none", "contracts", "")

	run, err := Plan(repoRoot, GateVerify, TargetKindUnit, "trace", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var code *CoverageKey
	for i := range run.Coverage {
		if run.Coverage[i].Key == "code:contracts/span.go" {
			code = &run.Coverage[i]
		}
	}
	if code == nil {
		t.Fatalf("missing code coverage key; coverage=%v", run.Coverage)
	}
	if stringInSlice(code.ReadRefs, "ui/dialog.tsx") {
		t.Fatalf("unrelated .tsx leaked into the code key's read refs: %v", code.ReadRefs)
	}
	if !stringInSlice(code.ReadRefs, "contracts/span.go") {
		t.Fatalf("declared code file missing from the code key's read refs: %v", code.ReadRefs)
	}
	surface := InputSurface(repoRoot, run)
	if stringInSlice(surface, "ui/dialog.tsx") {
		t.Fatalf("unrelated .tsx leaked into the recorded input surface: %v", surface)
	}
	if !stringInSlice(surface, "contracts/span.go") {
		t.Fatalf("declared code file missing from the recorded input surface: %v", surface)
	}
}

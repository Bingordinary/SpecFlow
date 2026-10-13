package gaterun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
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

// publicCodeRecord saves a minimal valid public code record for one file. Its
// only dependency is the file itself with no pinned hash, so judgments.Check
// passes while that file exists.
func publicCodeRecord(t *testing.T, root, unit, subject, protocol string) judgments.Reference {
	t.Helper()
	report := "fixture report"
	ref, err := judgments.Save(root, judgments.Record{
		Version:      judgments.RecordVersion,
		Kind:         judgments.Code,
		Unit:         unit,
		Subject:      subject,
		Coverage:     []string{"code:" + subject},
		Inputs:       []string{subject},
		Dependencies: []judgments.Dependency{{Path: subject}},
		Protocol:     protocol,
		Verdict:      "FACTS",
		Result:       json.RawMessage(`{"verdicts":{"code:` + subject + `":"FACTS"}}`),
		Report:       report,
		ReportDigest: judgments.Digest([]byte(report)),
		SourceRun:    "20260101-000000-src001",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// sharedCheckRun builds the minimal verify run prepareSharedChecks needs for
// one public code key.
func sharedCheckRun(protocol, key, file string) *Run {
	return &Run{
		RunID: "20260101-000000-aaaaaa", Gate: GateVerify, TargetKind: TargetKindUnit,
		TargetName: "auth", Target: TargetCandidate, Status: StatusOpen, Mode: ModeFull,
		Protocol: protocol, Records: map[string]judgments.Binding{},
		Coverage: []CoverageKey{{Key: key, Kind: SessionKindCode, Lens: LensQuality, File: file, Unit: "auth", ReadRefs: []string{file}}},
	}
}

// TestPrepareSharedChecksReusesAndInvalidatesBySubject is the #79 regression:
// the per-subject index that replaced the per-key full-store scan must preserve
// the reuse and invalidation decisions — a valid public record is reused, while
// an invalidated one is not.
func TestPrepareSharedChecksReusesAndInvalidatesBySubject(t *testing.T) {
	root := newRepo(t)
	writeUnit(t, root, "candidate", "auth", "none", "none", "src", "")
	writeFile(t, root, "src/auth.go", "package auth\n")
	protocol := judgments.Protocol(root)

	valid := publicCodeRecord(t, root, "auth", "src/auth.go", protocol)

	reused := sharedCheckRun(protocol, "code:src/auth.go", "src/auth.go")
	if err := prepareSharedChecks(root, reused); err != nil {
		t.Fatal(err)
	}
	ck := reused.CoverageByKey("code:src/auth.go")
	if ck.Source != "reused" {
		t.Fatalf("a valid public record must be reused, got source %q", ck.Source)
	}
	if binding, ok := reused.Records["code:src/auth.go"]; !ok || binding.Reference != valid {
		t.Fatalf("reuse must bind the valid record, got %+v", reused.Records)
	}

	// An invalidated record is not reusable: the key must execute with its own
	// task instead.
	if err := judgments.Invalidate(root, valid.ID, "fixture invalidation"); err != nil {
		t.Fatal(err)
	}
	executed := sharedCheckRun(protocol, "code:src/auth.go", "src/auth.go")
	if err := prepareSharedChecks(root, executed); err != nil {
		t.Fatal(err)
	}
	ck = executed.CoverageByKey("code:src/auth.go")
	if ck.Source != "executed" || ck.Task == "" {
		t.Fatalf("an invalidated record must not be reused, got source %q task %q", ck.Source, ck.Task)
	}
	if _, bound := executed.Records["code:src/auth.go"]; bound {
		t.Fatalf("an invalidated record must not be bound, got %+v", executed.Records)
	}
}

// TestPrepareSharedChecksDefersStoreWork is the #79 follow-up: the code-record
// index — a whole-store read — is built only when a code coverage key still has
// to be prepared. A run whose keys are all settled (a review that adds no code
// key, or the second pass of the same plan) must prepare without touching the
// store at all. The store is made unresolvable so any single access fails the
// call; the same store must fail an unprepared key, which proves the fixture
// would catch the read.
func TestPrepareSharedChecksDefersStoreWork(t *testing.T) {
	root := newRepo(t)
	writeUnit(t, root, "candidate", "auth", "none", "none", "src", "")
	protocol := judgments.Protocol(root)

	settled := sharedCheckRun(protocol, "code:src/auth.go", "src/auth.go")
	settled.Coverage[0].Source = "executed"
	settled.Coverage[0].Task = "0123456789abcdef"
	settled.Coverage = append(settled.Coverage, CoverageKey{Key: "design:auth:src/auth.go", Kind: SessionKindDesign, Lens: LensQuality, File: "src/auth.go", Unit: "auth", Source: "executed"})

	store := filepath.Join(root, "docs", "specs", "meta", "validation", "judgments")
	if err := os.RemoveAll(store); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), store); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	if err := prepareSharedChecks(root, settled); err != nil {
		t.Fatalf("a settled run must prepare without reading the judgment store: %v", err)
	}
	if ck := settled.CoverageByKey("code:src/auth.go"); ck.Source != "executed" || ck.Task != "0123456789abcdef" {
		t.Fatalf("a settled key must stay untouched: %+v", ck)
	}

	// Control: an unprepared code key reads the store, so the unresolvable
	// store fails it — the fixture can observe the read.
	if err := prepareSharedChecks(root, sharedCheckRun(protocol, "code:src/auth.go", "src/auth.go")); err == nil {
		t.Fatal("an unprepared code key must read the judgment store")
	}
}

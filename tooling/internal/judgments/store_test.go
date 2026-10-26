package judgments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

// testProtocol reads the real deployed judging standard. The record fixture
// root is a bare temp directory, so Protocol cannot resolve a layout there.
func testProtocol(t *testing.T) string {
	t.Helper()
	return MustProtocol(sourceRepoRoot(t), "verify", "unit")
}

func fixture(t *testing.T) (string, Record) {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "contracts.js")
	if err := os.WriteFile(p, []byte("export const token = true;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := specpaths.FileHash(p)
	if err != nil {
		t.Fatal(err)
	}
	return root, Record{Version: RecordVersion, Kind: Code, Subject: "contracts.js", Coverage: []string{"code:contracts.js"}, Inputs: []string{"contracts.js"}, Dependencies: []Dependency{{Path: "contracts.js", Hash: hash}}, Protocol: testProtocol(t), Verdict: "FACTS", Result: json.RawMessage(`{"observations":[]}`), Report: "facts", ReportDigest: Digest([]byte("facts")), SourceRun: "20260101-000000-abcdef"}
}
func TestConcurrentImmutablePublication(t *testing.T) {
	root, r := fixture(t)
	var wg sync.WaitGroup
	errors := make(chan error, 16)
	refs := make(chan Reference, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref, err := Save(root, r)
			if err != nil {
				errors <- err
			} else {
				refs <- ref
			}
		}()
	}
	wg.Wait()
	close(errors)
	close(refs)
	for err := range errors {
		t.Fatal(err)
	}
	id := ""
	for ref := range refs {
		if id != "" && id != ref.ID {
			t.Fatal("different ids")
		}
		id = ref.ID
		if _, err := Load(root, ref); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := List(root)
	if err != nil || len(listed) != 1 {
		t.Fatalf("records: %v %v", listed, err)
	}
}
func TestWholeFileChangeAndTransitiveInvalidation(t *testing.T) {
	root, r := fixture(t)
	public, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	design := r
	design.Kind = Design
	design.Unit = "auth"
	design.Subject = "contracts.js"
	design.References = []Binding{{Reference: public, Layer: "candidate"}}
	design.SourceRun = "20260101-000001-abcdef"
	ref, err := Save(root, design)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(root, ref, "candidate", testProtocol(t)); err != nil {
		t.Fatal(err)
	}
	if err := Invalidate(root, public, "contradicted by new evidence"); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, ref, "candidate", testProtocol(t)); err == nil {
		t.Fatal("dependent PASS remains usable")
	}
	root, r = fixture(t)
	ref, err = Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(root, "contracts.js"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("export const order = true;\n")
	f.Close()
	if err := Check(root, ref, "candidate", testProtocol(t)); err == nil {
		t.Fatal("appended content escaped whole-file identity")
	}
}
func TestStableBindingResolvesPromisePinsInStableScope(t *testing.T) {
	root := t.TempDir()
	writeProvider := func(layer, contract string) {
		path := filepath.Join(root, "docs/specs/units", layer, "unit_auth.md")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		content := "---\nid: auth\n---\n\n## Promises\n\npromises:\n" +
			"  - id: auth.login_api\n" +
			"    summary: Login.\n    contract: {invariants: [" + contract + "]}\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeProvider("stable", "stable login")
	writeProvider("candidate", "candidate login")

	stableData, err := os.ReadFile(filepath.Join(root, "docs/specs/units/stable/unit_auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	promises, issues := specvalidation.ParsePromises(string(stableData))
	if len(issues) > 0 || len(promises) != 1 {
		t.Fatalf("parse stable provider: %v %v", promises, issues)
	}
	fingerprint, err := specvalidation.PromiseFingerprint(promises[0])
	if err != nil {
		t.Fatal(err)
	}

	// A candidate-run record promoted into a stable confirmation: the record
	// keeps the candidate-era Scope, while the stable binding must resolve the
	// published stable provider.
	r := Record{
		Version:      RecordVersion,
		Kind:         Code,
		Subject:      "contracts.js",
		Coverage:     []string{"item:web:web.session"},
		Inputs:       []string{"contracts.js"},
		Dependencies: []Dependency{{Promise: "auth.login_api", Fingerprint: fingerprint, Scope: specvalidation.PromiseScopeAll}},
		Protocol:     testProtocol(t),
		Verdict:      "ALIGNED",
		Result:       json.RawMessage(`{"effective_status":{"item:web:web.session":"pass"}}`),
		Report:       "aligned",
		ReportDigest: Digest([]byte("aligned")),
		SourceRun:    "20260101-000000-abcdef",
	}
	ref, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	protocol := testProtocol(t)
	if err := Check(root, ref, "stable", protocol); err != nil {
		t.Fatalf("stable binding must resolve the published stable contract: %v", err)
	}
	if err := Check(root, ref, "candidate", protocol); err == nil {
		t.Fatal("candidate binding must resolve the candidate contract")
	}
}

func TestDamageCannotBeOverwritten(t *testing.T) {
	root, r := fixture(t)
	ref, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	p, err := StoredPath(root, ref)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(p, []byte("{}"), 0644)
	if _, err := Load(root, ref); err == nil {
		t.Fatal("corruption accepted")
	}
	if _, err := Save(root, r); err == nil {
		t.Fatal("accepted history overwritten")
	}
	if _, err := Load(root, Reference{ID: "../../x"}); err == nil {
		t.Fatal("unsafe id accepted")
	}
}

func TestInstalledProtocolContentInvalidatesRecords(t *testing.T) {
	root, r := fixture(t)
	p := filepath.Join(root, "specflow/tooling/manifest.tsv")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	documents := protocolStandards[standardKey{gate: "verify", targetKind: "unit"}]
	writeStandard := func(judging, mechanism string) {
		t.Helper()
		for _, name := range documents {
			path := filepath.Join(root, "specflow/framework", name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			b.WriteString("# Fixture " + name + "\n\n## Judging text\n\n")
			b.WriteString(judging)
			b.WriteString("\n\n")
			for _, section := range MechanismSectionNames(name) {
				b.WriteString("## " + section + "\n\n" + mechanism + "\n\n")
			}
			if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeStandard("A judgment stands.", "Fixture.")
	r.Protocol = MustProtocol(root, "verify", "unit")
	ref, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(root, ref, "candidate", MustProtocol(root, "verify", "unit")); err != nil {
		t.Fatal(err)
	}
	// A mechanism section describes how a run is planned, recorded or
	// terminated, not what counts as a pass, a problem, or how severe it is:
	// changing it must not invalidate a recorded conclusion.
	writeStandard("A judgment stands.", "Rewritten fixture.")
	if err := Check(root, ref, "candidate", MustProtocol(root, "verify", "unit")); err != nil {
		t.Fatalf("mechanism change invalidated a recorded judgment: %v", err)
	}
	// The judging text is the standard the record was made under: changing it
	// invalidates the record.
	writeStandard("A judgment changed.", "Rewritten fixture.")
	if err := Check(root, ref, "candidate", MustProtocol(root, "verify", "unit")); err == nil {
		t.Fatal("changed installed protocol reused an old judgment")
	}
}

func TestCurrentItemDecisionChecksCanonicalFindings(t *testing.T) {
	root, r := fixture(t)
	spec := filepath.Join(root, "docs/specs/units/stable/unit_auth.md")
	if err := os.MkdirAll(filepath.Dir(spec), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("approved requirement\n"), 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := specpaths.FileHash(spec)
	if err != nil {
		t.Fatal(err)
	}
	r.Kind, r.Unit, r.Subject, r.Verdict = Item, "auth", "auth.core", "MISMATCH"
	r.SpecContext, err = SpecContext(root, "auth", "stable")
	if err != nil {
		t.Fatal(err)
	}
	r.Dependencies = append(r.Dependencies, Dependency{Own: true, Path: ".md", Hash: hash})
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"fail"},"findings":[{"id":"old/F1","severity":"P2","text":"divergence","detail":"evidence"}]}`)
	prior, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	r.SourceRun = "20250101-000000-abcdef" // Accepted later, planned earlier.
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"fail"},"findings":[{"id":"new/F1","severity":"P1","text":"divergence","detail":"evidence"}]}`)
	current, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(root, prior, "stable", testProtocol(t)); err == nil {
		t.Fatal("unchanged MISMATCH verdict hid changed blocking severity")
	}
	r.SourceRun = "20240101-000000-abcdef"
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"fail"},"findings":[{"id":"consumer/F9","severity":"P1","text":"divergence","detail":"evidence"}]}`)
	equivalent, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(root, current, "stable", testProtocol(t)); err != nil {
		t.Fatalf("equivalent accepted decision invalidated a consumer: %v", err)
	}
	path, err := StoredPath(root, equivalent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, current, "stable", testProtocol(t)); err == nil {
		t.Fatal("missing current decision resurrected an earlier record")
	}
}

func TestCurrentItemDecisionsRemainSeparateAcrossSpecContexts(t *testing.T) {
	root, r := fixture(t)
	stablePath := filepath.Join(root, "docs/specs/units/stable/unit_auth.md")
	candidatePath := filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")
	for _, path := range []string{stablePath, candidatePath} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(stablePath, []byte("approved requirement\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, []byte("different candidate requirement\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r.Kind, r.Unit, r.Subject, r.Verdict = Item, "auth", "auth.core", "ALIGNED"
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"pass"}}`)
	var err error
	r.SpecContext, err = SpecContext(root, "auth", "stable")
	if err != nil {
		t.Fatal(err)
	}
	previous, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Verdict = "MISMATCH"
	r.SourceRun = "stable-failure"
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"fail"}}`)
	stable, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	r.SpecContext, err = SpecContext(root, "auth", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	r.Verdict, r.SourceRun = "ALIGNED", "candidate-pass"
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"pass"}}`)
	candidate, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	for layer, want := range map[string]Reference{"stable": stable, "candidate": candidate} {
		got, accepted, err := LatestItem(root, testProtocol(t), "auth", "auth.core", layer)
		if err != nil || !accepted || got != want {
			t.Fatalf("%s lost its own current decision: %+v %v %v", layer, got, accepted, err)
		}
	}
	if err := Check(root, previous, "stable", testProtocol(t)); err == nil {
		t.Fatal("different candidate context resurrected the old stable ALIGNED record")
	}
	path, err := StoredPath(root, stable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, previous, "stable", testProtocol(t)); err == nil {
		t.Fatal("missing stable decision was hidden by the valid candidate decision")
	}
}

func TestBoundStableContextCannotBypassItsCurrentDecision(t *testing.T) {
	root, r := fixture(t)
	path := filepath.Join(root, "docs/specs/units/stable/unit_auth.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("original requirement\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r.Kind, r.Unit, r.Subject, r.Verdict = Item, "auth", "auth.core", "ALIGNED"
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"pass"}}`)
	var err error
	r.SpecContext, err = SpecContext(root, "auth", "stable")
	if err != nil {
		t.Fatal(err)
	}
	previous, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("updated requirement context\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r.SpecContext, err = SpecContext(root, "auth", "stable")
	if err != nil {
		t.Fatal(err)
	}
	r.Verdict, r.SourceRun = "MISMATCH", "new-stable-failure"
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"fail"}}`)
	if _, err := Save(root, r); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, previous, "stable", testProtocol(t)); err == nil {
		t.Fatal("historical context bypassed the current stable decision")
	}
}

// A dependency unit's main spec is one evidence input in every layer: a
// candidate run reads it as the logical reference unit:<provider>, a stable
// run reads the published stable file, and both spellings must canonicalize to
// the same key. A promote keeps the recorded logical spelling while the stable
// requirement names the physical file, so without this canonicalization the
// promoted cache reports a newly required input that changed nothing.
func TestEvidenceInputsCanonicalizeAcrossLayers(t *testing.T) {
	root := t.TempDir()
	write := func(ref, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/specs/units/candidate/unit_trace.md", "---\nid: trace\nrule_refs: none\n---\n")
	write("docs/specs/units/stable/unit_logger.md", "---\nid: logger\nrule_refs: none\n---\n")
	write("docs/specs/units/candidate/appendix/unit_trace_protocol.md", "---\nunit: trace\n---\n")
	write("docs/specs/units/candidate/appendix/unit_logger_protocol.md", "---\nunit: logger\n---\n")

	got, err := NormalizeInputs(root, "trace", "candidate", []string{
		"docs/specs/units/candidate/unit_trace.md",
		"docs/specs/units/candidate/appendix/unit_trace_protocol.md",
		"docs/specs/units/stable/unit_logger.md",
		"docs/specs/units/candidate/appendix/unit_logger_protocol.md",
		"unit:logger",
		"src/trace.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"own:.md",
		"own:/appendix/unit_trace_protocol.md",
		"src/trace.go",
		"unit:logger",
		"unit:logger",
		"unit:logger:appendix:unit_logger_protocol",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("normalized inputs = %q, want %q", got, want)
	}

	// The promoted record keeps unit:logger while the stable requirement names
	// the published file.
	if missing, ok, err := NewlyRequiredEvidence(root, "trace", "stable", Record{Inputs: []string{"unit:logger"}}, []string{"docs/specs/units/stable/unit_logger.md"}); err != nil || ok {
		t.Fatalf("stable physical requirement must match the logical record input: %q %v %v", missing, ok, err)
	}
	// A record written with the physical stable spelling is the same evidence
	// when the comparison runs in the candidate layer.
	if missing, ok, err := NewlyRequiredEvidence(root, "trace", "candidate", Record{Inputs: []string{"docs/specs/units/stable/unit_logger.md"}}, []string{"unit:logger"}); err != nil || ok {
		t.Fatalf("physical record input must match the logical requirement: %q %v %v", missing, ok, err)
	}
	// A genuinely new dependency is still newly required evidence, reported in
	// the caller's spelling.
	missing, ok, err := NewlyRequiredEvidence(root, "trace", "stable", Record{Inputs: []string{"unit:logger"}}, []string{"docs/specs/units/stable/unit_metrics.md"})
	if err != nil || !ok || missing != "docs/specs/units/stable/unit_metrics.md" {
		t.Fatalf("new provider dependency must stay required: %q %v %v", missing, ok, err)
	}
	// Non-unit paths keep their spelling.
	if missing, ok, err := NewlyRequiredEvidence(root, "trace", "stable", Record{Inputs: []string{"src/trace.go"}}, []string{"src/logger.go"}); err != nil || !ok || missing != "src/logger.go" {
		t.Fatalf("code inputs must compare literally: %q %v %v", missing, ok, err)
	}
}

// TestVerifyChecksALoadedRecord pins the split between reading a record and
// judging it: a caller that already loaded a record gets the same verdict as
// Check, without a second read, and a changed dependency still fails.
func TestVerifyChecksALoadedRecord(t *testing.T) {
	root, r := fixture(t)
	ref, err := Save(root, r)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	record, err := Load(root, ref)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	protocol := testProtocol(t)

	if err := Verify(root, record, ref, "candidate", protocol); err != nil {
		t.Fatalf("verify an unchanged record: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "contracts.js"), []byte("export const token = false;\n"), 0o644); err != nil {
		t.Fatalf("rewrite dependency: %v", err)
	}
	if err := Verify(root, record, ref, "candidate", protocol); err == nil {
		t.Fatal("a changed dependency must fail verification")
	}
	if err := Check(root, ref, "candidate", protocol); err == nil {
		t.Fatal("Check must fail the same way Verify does")
	}
}

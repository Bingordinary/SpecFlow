package judgments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

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
	return root, Record{Version: RecordVersion, Kind: Code, Subject: "contracts.js", Coverage: []string{"code:contracts.js"}, Inputs: []string{"contracts.js"}, Dependencies: []Dependency{{Path: "contracts.js", Hash: hash}}, Protocol: Protocol(root), Verdict: "FACTS", Result: json.RawMessage(`{"observations":[]}`), Report: "facts", ReportDigest: Digest([]byte("facts")), SourceRun: "20260101-000000-abcdef"}
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
	if err := Check(root, ref, "candidate", Protocol(root)); err != nil {
		t.Fatal(err)
	}
	if err := Invalidate(root, public.ID, "contradicted by new evidence"); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, ref, "candidate", Protocol(root)); err == nil {
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
	if err := Check(root, ref, "candidate", Protocol(root)); err == nil {
		t.Fatal("appended content escaped whole-file identity")
	}
}
func TestDamageCannotBeOverwritten(t *testing.T) {
	root, r := fixture(t)
	ref, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	p, err := recordPath(root, ref.ID)
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
	protocolFile := filepath.Join(root, "specflow/framework/shared_judgments.md")
	if err := os.MkdirAll(filepath.Dir(protocolFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protocolFile, []byte("protocol before"), 0644); err != nil {
		t.Fatal(err)
	}
	r.Protocol = Protocol(root)
	ref, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(root, ref, "candidate", Protocol(root)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(protocolFile, []byte("protocol after"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, ref, "candidate", Protocol(root)); err == nil {
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
	if err := Check(root, prior, "stable", Protocol(root)); err == nil {
		t.Fatal("unchanged MISMATCH verdict hid changed blocking severity")
	}
	r.SourceRun = "20240101-000000-abcdef"
	r.Result = json.RawMessage(`{"effective_status":{"item:auth:auth.core":"fail"},"findings":[{"id":"consumer/F9","severity":"P1","text":"divergence","detail":"evidence"}]}`)
	equivalent, err := Save(root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(root, current, "stable", Protocol(root)); err != nil {
		t.Fatalf("equivalent accepted decision invalidated a consumer: %v", err)
	}
	path, err := recordPath(root, equivalent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, current, "stable", Protocol(root)); err == nil {
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
		got, accepted, err := LatestItem(root, "auth", "auth.core", layer)
		if err != nil || !accepted || got != want {
			t.Fatalf("%s lost its own current decision: %+v %v %v", layer, got, accepted, err)
		}
	}
	if err := Check(root, previous, "stable", Protocol(root)); err == nil {
		t.Fatal("different candidate context resurrected the old stable ALIGNED record")
	}
	path, err := recordPath(root, stable.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, previous, "stable", Protocol(root)); err == nil {
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
	if err := Check(root, previous, "stable", Protocol(root)); err == nil {
		t.Fatal("historical context bypassed the current stable decision")
	}
}

package validationcache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComparePromisePins(t *testing.T) {
	pins := []PromisePin{
		{ID: "auth.login", Fingerprint: "sha256:a"},
		{ID: "auth.logout", Fingerprint: "sha256:b"},
		{ID: "auth.missing", Fingerprint: "sha256:c"},
	}
	current := map[string]string{
		"auth.login":  "sha256:a",
		"auth.logout": "sha256:b2",
	}
	states := ComparePromisePins(pins, current)
	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}
	if states[0].Status != PromiseFresh {
		t.Fatalf("login = %s, want FRESH", states[0].Status)
	}
	if states[1].Status != PromiseReopened {
		t.Fatalf("logout = %s, want REOPENED", states[1].Status)
	}
	if states[2].Status != PromiseUnresolved {
		t.Fatalf("missing = %s, want UNRESOLVED", states[2].Status)
	}
}

func TestComparePromisePublication(t *testing.T) {
	pins := []PromisePin{
		{ID: "auth.login", Fingerprint: "sha256:a"},
		{ID: "auth.logout", Fingerprint: "sha256:b"},
		{ID: "auth.missing", Fingerprint: "sha256:c"},
	}
	stable := map[string]string{
		"auth.login":  "sha256:a",
		"auth.logout": "sha256:b2",
	}
	states := ComparePromisePublication(pins, stable)
	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}
	if states[0].Status != PromiseFresh {
		t.Fatalf("login = %s, want FRESH", states[0].Status)
	}
	if states[1].Status != PromiseReopened || !strings.Contains(states[1].Reason, "published stable") {
		t.Fatalf("logout = %+v, want REOPENED naming the published stable contract", states[1])
	}
	if states[2].Status != PromiseUnresolved || !strings.Contains(states[2].Reason, "not published") {
		t.Fatalf("missing = %+v, want UNRESOLVED naming the missing publication", states[2])
	}
}

func TestPromiseStateFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "promise_state.yaml")
	want := PromiseStateFile{
		Unit:  "web",
		Layer: "candidate",
		Promises: []PromisePin{
			{ID: "web.session", Fingerprint: "sha256:own"},
		},
		Pins: []PromisePin{
			{ID: "auth.login", Fingerprint: "sha256:provider"},
		},
	}
	if err := WritePromiseStateFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPromiseStateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Unit != want.Unit || got.Layer != want.Layer {
		t.Fatalf("identity = %s/%s, want %s/%s", got.Unit, got.Layer, want.Unit, want.Layer)
	}
	if len(got.Pins) != 1 || got.Pins[0].ID != "auth.login" || got.Pins[0].Fingerprint != "sha256:provider" {
		t.Fatalf("pins = %+v", got.Pins)
	}
}

func TestPromiseStatePath(t *testing.T) {
	path, err := PromiseStatePath("/repo", "auth", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.FromSlash("docs/specs/meta/validation/unit/auth/promise_state_candidate.yaml")
	if !filepath.IsAbs(path) {
		t.Fatalf("expected absolute path, got %s", path)
	}
	if filepath.ToSlash(path) != filepath.ToSlash(filepath.Join("/repo", want)) {
		t.Fatalf("path = %s, want %s", path, filepath.Join("/repo", want))
	}
	if _, err := PromiseStatePath("/repo", "auth", "other"); err == nil {
		t.Fatal("expected invalid-layer error")
	}
}

func TestReadPromiseStateFileMissing(t *testing.T) {
	state, err := ReadPromiseStateFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatalf("expected nil state for missing file, got %+v", state)
	}
}

func TestPromiseStateLayerLifecycle(t *testing.T) {
	root := t.TempDir()
	state := PromiseStateFile{
		Unit:     "auth",
		Layer:    "candidate",
		Promises: []PromisePin{{ID: "auth.login", Fingerprint: "sha256:a"}},
	}
	candidatePath, err := PromiseStatePath(root, "auth", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePromiseStateFile(candidatePath, state); err != nil {
		t.Fatal(err)
	}

	stablePath, data, candidatePath, ok, err := PreparePromiseStateStable(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected candidate state to be prepared for stable")
	}
	var stable PromiseStateFile
	if err := yaml.Unmarshal(data, &stable); err != nil {
		t.Fatal(err)
	}
	if stable.Layer != "stable" {
		t.Fatalf("stable state = %+v", stable)
	}
	if err := WritePromiseStateFile(stablePath, stable); err != nil {
		t.Fatal(err)
	}

	copied, err := CopyPromiseStateToCandidate(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !copied {
		t.Fatal("expected stable state to be copied to candidate")
	}
	candidate, err := ReadPromiseStateFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil || candidate.Layer != "candidate" {
		t.Fatalf("candidate state = %+v", candidate)
	}

	for _, path := range []string{candidatePath, stablePath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if state, _ := ReadPromiseStateFile(candidatePath); state != nil {
		t.Fatal("candidate state must be removed")
	}
	if state, _ := ReadPromiseStateFile(stablePath); state != nil {
		t.Fatal("stable state must be removed")
	}
}

func TestReadPromiseStateFileRejectsMissingUnit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("layer: candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPromiseStateFile(path); err == nil {
		t.Fatal("expected missing-unit error")
	}
}

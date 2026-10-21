package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func writePromiseStateUnit(t *testing.T, repoRoot, unit, promiseID, dependsOn, contractText string) {
	t.Helper()
	depends := ""
	if dependsOn != "" {
		depends = "    depends_on: [" + dependsOn + "]\n"
	}
	content := "---\nid: " + unit + "\nrule_refs: none\n---\n\n# " + unit + "\n\n" +
		"## Design Narrative\n\nNarrative.\n\n" +
		"## Promises\n\npromises:\n  - id: " + promiseID + "\n    kind: opaque\n    opaque_reason: no structured kind applies\n    summary: Contract.\n    contract: {text: " + contractText + "}\n" + depends + "\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    promise: " + promiseID + "\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/" + unit + ".go\n    verification_method: test.\n    pass_condition: Passes.\n    runnable: yes\n"
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unit+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func promiseFingerprint(t *testing.T, repoRoot, unit, promiseID string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_"+unit+".md"))
	if err != nil {
		t.Fatal(err)
	}
	promises, issues := specvalidation.ParsePromises(string(data))
	if len(issues) > 0 {
		t.Fatal(strings.Join(issues, "; "))
	}
	for _, promise := range promises {
		if promise.ID == promiseID {
			fingerprint, err := specvalidation.PromiseFingerprint(promise)
			if err != nil {
				t.Fatal(err)
			}
			return fingerprint
		}
	}
	t.Fatalf("promise %q not found", promiseID)
	return ""
}

// writeLayerPromiseUnit writes a promise-spec unit into one layer.
func writeLayerPromiseUnit(t *testing.T, repoRoot, layer, unit, promiseID, dependsOn, contractText string) {
	t.Helper()
	depends := ""
	if dependsOn != "" {
		depends = "    depends_on: [" + dependsOn + "]\n"
	}
	content := "---\nid: " + unit + "\nrule_refs: none\n---\n\n# " + unit + "\n\n" +
		"## Design Narrative\n\nNarrative.\n\n" +
		"## Promises\n\npromises:\n  - id: " + promiseID + "\n    kind: opaque\n    opaque_reason: no structured kind applies\n    summary: Contract.\n    contract: {text: " + contractText + "}\n" + depends + "\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n  - id: " + unit + ".core\n    promise: " + promiseID + "\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/" + unit + ".go\n    verification_method: test.\n    pass_condition: Passes.\n    runnable: yes\n"
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unit+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func promiseStateText(t *testing.T, root, unit, layer string) string {
	t.Helper()
	report, err := computePromiseState(root, unit, layer)
	if err != nil {
		t.Fatalf("compute promise state: %v", err)
	}
	var out bytes.Buffer
	writePromiseStateDetail(&out, report)
	return out.String()
}

// The layer flag selects the promise graph scope: the stable layer evaluates the
// stable spec's own depends_on against published stable providers and must not
// be affected by the candidate layer's declarations.
func TestPromiseStateStableLayerUsesStableGraph(t *testing.T) {
	root := t.TempDir()
	writeLayerPromiseUnit(t, root, "stable", "auth", "auth.legacy_api", "", "legacy")
	writeLayerPromiseUnit(t, root, "stable", "web", "web.session_api", "auth.legacy_api", "session")
	writeLayerPromiseUnit(t, root, "candidate", "web", "web.session_api", "ghost.api", "session")

	stableOut := promiseStateText(t, root, "web", "stable")
	if !strings.Contains(stableOut, "graph: PASS") {
		t.Fatalf("stable layer must resolve the stable provider, got:\n%s", stableOut)
	}
	if strings.Contains(stableOut, "ghost.api") {
		t.Fatalf("stable layer must not evaluate candidate declarations, got:\n%s", stableOut)
	}

	candidateOut := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(candidateOut, "unresolved depends_on") {
		t.Fatalf("candidate layer must report its unresolved declaration, got:\n%s", candidateOut)
	}
}

func TestPromiseStatePublicationRequiresStableProvider(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")
	webFP := promiseFingerprint(t, root, "web", "web.session_api")
	authFP := promiseFingerprint(t, root, "auth", "auth.login_api")
	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(statePath, validationcache.PromiseStateFile{
		Unit:     "web",
		Layer:    "candidate",
		Promises: []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: webFP}},
		Pins:     []validationcache.PromisePin{{ID: "auth.login_api", Fingerprint: authFP}},
	}); err != nil {
		t.Fatal(err)
	}

	report, err := computePromiseState(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if report.Closure != "CLOSED" {
		t.Fatalf("candidate layer closure = %s — %v", report.Closure, report.ClosureReasons)
	}
	if ready, _ := promiseReadiness(report, ""); ready {
		t.Fatal("a candidate-only provider must not make the consumer READY FOR PROMOTE")
	}
	if len(report.Publication) != 1 || report.Publication[0].Status != validationcache.PromiseUnresolved {
		t.Fatalf("expected an unresolved publication pin, got %+v", report.Publication)
	}
	if !strings.Contains(report.Publication[0].Reason, "not published") {
		t.Fatalf("publication reason must name the missing stable publication, got %q", report.Publication[0].Reason)
	}

	writeLayerPromiseUnit(t, root, "stable", "auth", "auth.login_api", "", "login")
	report, err = computePromiseState(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if ready, reason := promiseReadiness(report, ""); !ready {
		t.Fatalf("published provider must make the consumer ready: %s", reason)
	}
}

func TestPromiseStateCLOSED(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")

	authFP := promiseFingerprint(t, root, "auth", "auth.login_api")
	webFP := promiseFingerprint(t, root, "web", "web.session_api")
	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(statePath, validationcache.PromiseStateFile{
		Unit:     "web",
		Layer:    "candidate",
		Promises: []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: webFP}},
		Pins:     []validationcache.PromisePin{{ID: "auth.login_api", Fingerprint: authFP}},
	}); err != nil {
		t.Fatal(err)
	}

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "promise CLOSED") {
		t.Fatalf("expected CLOSED, got:\n%s", out)
	}
}

func TestPromiseStateREOPENED(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")

	authFP := promiseFingerprint(t, root, "auth", "auth.login_api")
	webFP := promiseFingerprint(t, root, "web", "web.session_api")
	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(statePath, validationcache.PromiseStateFile{
		Unit:     "web",
		Layer:    "candidate",
		Promises: []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: webFP}},
		Pins:     []validationcache.PromisePin{{ID: "auth.login_api", Fingerprint: authFP}},
	}); err != nil {
		t.Fatal(err)
	}

	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login changed")

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "promise REOPENED") {
		t.Fatalf("expected REOPENED, got:\n%s", out)
	}
}

func TestPromiseStateUnrelatedCycleDoesNotFailGraph(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.a", "web.b", "a")
	writePromiseStateUnit(t, root, "web", "web.b", "auth.a", "b")
	writePromiseStateUnit(t, root, "other", "other.c", "", "c")

	out := promiseStateText(t, root, "other", "candidate")
	if !strings.Contains(out, "graph: PASS") {
		t.Fatalf("unrelated unit must not be blocked by another unit's cycle:\n%s", out)
	}
}

func TestPromiseStateOPENMissingDependencyPin(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")

	webFP := promiseFingerprint(t, root, "web", "web.session_api")
	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(statePath, validationcache.PromiseStateFile{
		Unit:     "web",
		Layer:    "candidate",
		Promises: []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: webFP}},
	}); err != nil {
		t.Fatal(err)
	}

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "promise OPEN") || !strings.Contains(out, "dependency pin missing") {
		t.Fatalf("expected OPEN for missing dependency pin, got:\n%s", out)
	}
}

func TestPromiseStateOPENWithoutRecord(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "web", "web.session_api", "", "session")

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "promise OPEN") {
		t.Fatalf("expected OPEN, got:\n%s", out)
	}
}

func TestPromiseStateRejectsMislayeredRecord(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "web", "web.session_api", "", "session")
	webFP := promiseFingerprint(t, root, "web", "web.session_api")
	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(statePath, validationcache.PromiseStateFile{
		Unit:     "web",
		Layer:    "stable",
		Promises: []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: webFP}},
	}); err != nil {
		t.Fatal(err)
	}

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "promise OPEN") || !strings.Contains(out, `promise state layer is "stable", expected "candidate"`) {
		t.Fatalf("expected OPEN for a mislayered record, got:\n%s", out)
	}
}

func TestPromiseStateRejectsExtraDependencyPin(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "other", "other.api", "", "other")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")
	webFP := promiseFingerprint(t, root, "web", "web.session_api")
	authFP := promiseFingerprint(t, root, "auth", "auth.login_api")
	otherFP := promiseFingerprint(t, root, "other", "other.api")
	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(statePath, validationcache.PromiseStateFile{
		Unit:     "web",
		Layer:    "candidate",
		Promises: []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: webFP}},
		Pins: []validationcache.PromisePin{
			{ID: "auth.login_api", Fingerprint: authFP},
			{ID: "other.api", Fingerprint: otherFP},
		},
	}); err != nil {
		t.Fatal(err)
	}

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "promise OPEN") || !strings.Contains(out, "dependency pin no longer declared: other.api") {
		t.Fatalf("expected OPEN for an extra dependency pin, got:\n%s", out)
	}
}

func TestPromiseStateRejectsDuplicateDependencyPin(t *testing.T) {
	root := t.TempDir()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")
	webFP := promiseFingerprint(t, root, "web", "web.session_api")
	authFP := promiseFingerprint(t, root, "auth", "auth.login_api")
	statePath, err := validationcache.PromiseStatePath(root, "web", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if err := validationcache.WritePromiseStateFile(statePath, validationcache.PromiseStateFile{
		Unit:     "web",
		Layer:    "candidate",
		Promises: []validationcache.PromisePin{{ID: "web.session_api", Fingerprint: webFP}},
		Pins: []validationcache.PromisePin{
			{ID: "auth.login_api", Fingerprint: authFP},
			{ID: "auth.login_api", Fingerprint: authFP},
		},
	}); err != nil {
		t.Fatal(err)
	}

	out := promiseStateText(t, root, "web", "candidate")
	if !strings.Contains(out, "promise OPEN") || !strings.Contains(out, `duplicate dependency pin "auth.login_api"`) {
		t.Fatalf("expected OPEN for a duplicate dependency pin, got:\n%s", out)
	}
}

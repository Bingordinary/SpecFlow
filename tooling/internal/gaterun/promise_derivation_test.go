package gaterun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

func writePromiseDerivationUnit(t *testing.T, root, unit, promises, item string) {
	t.Helper()
	dir := filepath.Join(root, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + unit + "\nrule_refs: none\n---\n\n# " + unit + "\n\n" +
		"## Design Narrative\n### Narrative\n\nNarrative.\n\n## Promises\n\n" + promises + "\n" +
		"## Acceptance Items\n\nacceptance_item_set:\n" + item
	if err := os.WriteFile(filepath.Join(dir, "unit_"+unit+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeriveUnitInputsAddsPromiseProviders(t *testing.T) {
	root := newRepo(t)
	writePromiseDerivationUnit(t, root, "auth", `promises:
  - id: auth.login_api
    summary: Login.
    contract: {invariants: [login]}
`, "")
	writePromiseDerivationUnit(t, root, "web", `promises:
  - id: web.session_api
    summary: Session.
    contract: {invariants: [session]}
    depends_on: [auth.login_api]
`, "  - id: web.core\n    promise: web.session_api\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/web.go\n    verification_method: test.\n    pass_condition: Passes.\n    runnable: yes\n")
	code := filepath.Join(root, "src/web.go")
	if err := os.MkdirAll(filepath.Dir(code), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code, []byte("package web\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err := deriveUnitValidate(root, "web", TargetCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRef(refs, "unit:auth") {
		t.Fatalf("deriveUnitValidate refs = %+v, want unit:auth", refs)
	}

	derivation, err := NewDerivation(root)
	if err != nil {
		t.Fatal(err)
	}
	verifyRefs, _, err := derivation.deriveUnitCodeGate("web", TargetCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRef(verifyRefs, "unit:auth") {
		t.Fatalf("deriveUnitCodeGate refs = %+v, want unit:auth", verifyRefs)
	}
}

func TestVerifyAlignmentReadRefsPinProviderContracts(t *testing.T) {
	root := newRepo(t)
	writePromiseDerivationUnit(t, root, "auth", `promises:
  - id: auth.login_api
    summary: Login.
    contract: {invariants: [login]}
`, "")
	writePromiseDerivationUnit(t, root, "web", `promises:
  - id: web.session_api
    summary: Session.
    contract: {invariants: [session]}
    depends_on: [auth.login_api]
`, "  - id: web.core\n    promise: web.session_api\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/web.go\n    verification_method: test.\n    pass_condition: Passes.\n    runnable: yes\n")
	code := filepath.Join(root, "src/web.go")
	if err := os.MkdirAll(filepath.Dir(code), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code, []byte("package web\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(root, GateVerify, TargetKindUnit, "web", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"item:web:web.core", "unit:web"} {
		spec, err := BuildSessionSpec(root, run, []string{key})
		if err != nil {
			t.Fatal(err)
		}
		if !containsString(spec.ReadRefs, "unit:auth") {
			t.Fatalf("%s read refs = %v, want the provider contract ref unit:auth", key, spec.ReadRefs)
		}
	}
}

func TestVerifyJudgmentPinsProviderContract(t *testing.T) {
	root := newRepo(t)
	writePromiseDerivationUnit(t, root, "auth", `promises:
  - id: auth.login_api
    summary: Login.
    contract: {invariants: [login]}
`, "")
	writePromiseDerivationUnit(t, root, "web", `promises:
  - id: web.session_api
    summary: Session.
    contract: {invariants: [session]}
    depends_on: [auth.login_api]
`, "  - id: web.core\n    promise: web.session_api\n    description: Given X, When Y, Then Z.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src/web.go\n    verification_method: test.\n    pass_condition: Passes.\n    runnable: yes\n")
	code := filepath.Join(root, "src/web.go")
	if err := os.MkdirAll(filepath.Dir(code), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code, []byte("package web\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run, err := Plan(root, GateVerify, TargetKindUnit, "web", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var item CoverageKey
	for _, ck := range run.Coverage {
		if ck.Kind == SessionKindItem {
			item = ck
			break
		}
	}
	if item.Key == "" {
		t.Fatal("expected an item coverage key")
	}
	result := SessionResult{
		Kind:            item.Kind,
		Verdicts:        map[string]string{item.Key: "ALIGNED"},
		EffectiveStatus: map[string]string{item.Key: "pass"},
	}
	ref, err := SaveJudgment(root, run, item, &result, "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := judgments.Check(root, ref, TargetCandidate, run.Protocol); err != nil {
		t.Fatalf("fresh judgment rejected: %v", err)
	}

	writePromiseDerivationUnit(t, root, "auth", `promises:
  - id: auth.login_api
    summary: Login.
    contract: {invariants: [login changed]}
`, "")
	if err := judgments.Check(root, ref, TargetCandidate, run.Protocol); err == nil || !strings.Contains(err.Error(), "auth.login_api contract changed") {
		t.Fatalf("provider contract change must stale the consumer judgment, got %v", err)
	}
}

func hasRef(refs []Ref, want string) bool {
	for _, ref := range refs {
		if ref.Ref == want {
			return true
		}
	}
	return false
}

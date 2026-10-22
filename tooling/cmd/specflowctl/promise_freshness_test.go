package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func writeConsumerPair(t *testing.T, root string) (string, string) {
	t.Helper()
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	writePromiseStateUnit(t, root, "web", "web.session_api", "auth.login_api", "session")
	for _, rel := range []string{"src/auth.go", "src/web.go"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package src\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	webSpec := filepath.Join(root, "docs/specs/units/candidate/unit_web.md")
	writeUnitCache(t, root, "web", "validate", "", []cacheFileSpec{{
		path: "docs/specs/units/candidate/unit_web.md",
		hash: computeHash(webSpec),
	}})
	currentVerifyFixture(t, root, "web", "candidate", "target: candidate\n")
	return filepath.Join(root, "docs/specs/units/candidate/unit_auth.md"), webSpec
}

func replaceInFile(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), old, new, 1)
	if updated == string(data) {
		t.Fatalf("fixture edit did not apply: %q", old)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestConsumerGateFreshnessFollowsProviderContractOnly pins the promise-model
// payoff: a provider's narrative, acceptance item, or evidence change does not
// stale the consumer's gates; only a provider promise contract change does.
func TestConsumerGateFreshnessFollowsProviderContractOnly(t *testing.T) {
	root := createCLITestRepo(t)
	authSpec, _ := writeConsumerPair(t, root)

	validate, err := validationcache.CheckValidate(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	verify, err := validationcache.CheckVerify(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if !validate.Fresh || !verify.Fresh {
		t.Fatalf("baseline gates must be fresh: validate=%+v verify=%+v", validate, verify)
	}

	// Provider narrative edit: not a contract change.
	replaceInFile(t, authSpec, "Narrative.", "Narrative rewritten.")
	if validate, err = validationcache.CheckValidate(root, "web"); err != nil || !validate.Fresh {
		t.Fatalf("provider narrative edit must not stale the consumer validate cache: %+v %v", validate, err)
	}
	if verify, err = validationcache.CheckVerify(root, "web"); err != nil || !verify.Fresh {
		t.Fatalf("provider narrative edit must not stale the consumer verify cache: %+v %v", verify, err)
	}

	// Provider acceptance item edit: still not a contract change.
	replaceInFile(t, authSpec, "Then Z.", "Then Z, still accepted.")
	if validate, err = validationcache.CheckValidate(root, "web"); err != nil || !validate.Fresh {
		t.Fatalf("provider acceptance item edit must not stale the consumer validate cache: %+v %v", validate, err)
	}
	if verify, err = validationcache.CheckVerify(root, "web"); err != nil || !verify.Fresh {
		t.Fatalf("provider acceptance item edit must not stale the consumer verify cache: %+v %v", verify, err)
	}

	// Provider contract edit: the consumer's contract dependency moved.
	replaceInFile(t, authSpec, "{invariants: [login]}", "{invariants: [login changed]}")
	validate, err = validationcache.CheckValidate(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if validate.Fresh || !strings.Contains(validate.Reason, "auth.login_api contract changed") {
		t.Fatalf("provider contract change must stale the consumer validate cache, got %+v", validate)
	}
	verify, err = validationcache.CheckVerify(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if verify.Fresh || !strings.Contains(verify.Reason, "auth.login_api contract changed") {
		t.Fatalf("provider contract change must stale the consumer verify cache, got %+v", verify)
	}

	report, err := validationcache.DeriveChangeReport(root, "unit", "web", "validate")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range report.Entries {
		if entry.Kind == "promise" && entry.Promise == "auth.login_api" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the change report must name the changed provider promise, got %+v", report.Entries)
	}
}

// TestProviderComparisonCopyDoesNotStaleValidate pins the candidate validate
// comparison surface: the plan carries the provider's published stable main
// spec so Check 7 can compare contracts, but neither that copy nor the
// provider's candidate narrative is a whole-file freshness input. The current
// promise pin remains the only contract dependency.
func TestProviderComparisonCopyDoesNotStaleValidate(t *testing.T) {
	root := createCLITestRepo(t)
	writeLayerPromiseUnit(t, root, "stable", "auth", "auth.login_api", "", "login")
	writeLayerPromiseUnit(t, root, "candidate", "auth", "auth.login_api", "", "login")
	writeLayerPromiseUnit(t, root, "candidate", "web", "web.session_api", "auth.login_api", "session")

	stableAuth := filepath.Join(root, "docs/specs/units/stable/unit_auth.md")
	candidateAuth := filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")
	webSpec := filepath.Join(root, "docs/specs/units/candidate/unit_web.md")
	writeUnitCache(t, root, "web", "validate", "", []cacheFileSpec{
		{path: "docs/specs/units/candidate/unit_web.md", hash: computeHash(webSpec)},
		{path: "docs/specs/units/candidate/unit_auth.md", hash: computeHash(candidateAuth)},
		{path: "docs/specs/units/stable/unit_auth.md", hash: computeHash(stableAuth)},
	})

	check, err := validationcache.CheckValidate(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if !check.Fresh {
		t.Fatalf("baseline validate cache must be fresh: %+v", check)
	}

	// A narrative edit on either the published comparison copy or the current
	// provider is not a contract change.
	replaceInFile(t, stableAuth, "Narrative.", "Narrative rewritten.")
	replaceInFile(t, candidateAuth, "Narrative.", "Narrative rewritten.")
	if check, err = validationcache.CheckValidate(root, "web"); err != nil || !check.Fresh {
		t.Fatalf("provider narrative edits must not stale the consumer validate cache: %+v %v", check, err)
	}

	// The current contract is still the dependency: a pin move reopens it.
	replaceInFile(t, candidateAuth, "{invariants: [login]}", "{invariants: [login changed]}")
	check, err = validationcache.CheckValidate(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if check.Fresh || !strings.Contains(check.Reason, "auth.login_api contract changed") {
		t.Fatalf("current provider contract change must stale the cache, got %+v", check)
	}
}

func TestStableConsumerIgnoresCandidateProviderDraft(t *testing.T) {
	root := createCLITestRepo(t)
	writeLayerPromiseUnit(t, root, "stable", "auth", "auth.login_api", "", "login")
	writeLayerPromiseUnit(t, root, "stable", "web", "web.session_api", "auth.login_api", "session")
	writeLayerPromiseUnit(t, root, "candidate", "auth", "auth.login_api", "", "login changed")
	for _, rel := range []string{"src/auth.go", "src/web.go"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package src\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stableWeb := filepath.Join(root, "docs/specs/units/stable/unit_web.md")
	writeUnitCache(t, root, "web", "validate", "target: stable\n", []cacheFileSpec{{
		path: "docs/specs/units/stable/unit_web.md",
		hash: computeHash(stableWeb),
	}})
	currentVerifyFixture(t, root, "web", "stable", "target: stable\n")

	validate, err := validationcache.CheckValidateStable(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if !validate.Fresh {
		t.Fatalf("a candidate provider draft must not stale the stable validate cache: %s", validate.Reason)
	}
	verify, err := validationcache.CheckVerifyStable(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if !verify.Fresh {
		t.Fatalf("a candidate provider draft must not stale the stable verify cache: %s", verify.Reason)
	}
}

// the promise-pin marker is INVALID, cannot be used as a delta/repair
// baseline, and rejects promotion.
func TestOldUnitCachesAreIncompatible(t *testing.T) {
	root := createCLITestRepo(t)
	writePromiseStateUnit(t, root, "auth", "auth.login_api", "", "login")
	specPath := filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")
	codePath := filepath.Join(root, "src/auth.go")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codePath, []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(root, "docs/specs/meta/validation/unit/auth")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyValidate := "---\ncommand: validate\nunit: auth\nmode: full\nresult: pass\ntarget: candidate\n" +
		"standard: " + fixtureStandard(t, root, "unit") + "\ntimestamp: \"2026-06-30T10:00:00Z\"\n" +
		"files:\n  - path: docs/specs/units/candidate/unit_auth.md\n    hash: sha256:" + computeHash(specPath) + "\n" +
		cacheDeps(t, specPath) + "---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(legacyValidate), 0o644); err != nil {
		t.Fatal(err)
	}

	check, err := validationcache.CheckValidate(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if check.Fresh || check.Category != validationcache.CategoryInvalid {
		t.Fatalf("old validate cache must be INVALID, got %+v", check)
	}

	var out, errOut bytes.Buffer
	err = runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut)
	if err == nil || !strings.Contains(out.String(), "predates promise pinning") {
		t.Fatalf("promote must reject an old validate cache, err=%v\n%s", err, out.String())
	}

	if _, err := grPlanRaw(root, "--gate", "validate", "--unit", "auth", "--target", "candidate", "--mode", "delta"); err == nil ||
		!strings.Contains(err.Error(), "not in the current format") {
		t.Fatalf("delta planning must refuse an old cache, got %v", err)
	}

	legacyVerify := "---\ncommand: verify\nunit: auth\nmode: full\nresult: pass\ntarget: candidate\ntimestamp: \"2026-06-30T10:00:00Z\"\n" +
		"files:\n  - path: docs/specs/units/candidate/unit_auth.md\n    hash: sha256:" + computeHash(specPath) + "\n" +
		cacheDeps(t, specPath) + "---\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(legacyVerify), 0o644); err != nil {
		t.Fatal(err)
	}
	verify, err := validationcache.CheckVerify(root, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if verify.Fresh || verify.Category != validationcache.CategoryInvalid {
		t.Fatalf("old verify cache must be INVALID, got %+v", verify)
	}
}

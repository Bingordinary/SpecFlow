package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// evidenceRecordFor renders a unit's single evidence record: one observed
// behavior section keyed to the unit's acceptance item.
func evidenceRecordFor(unit, item string) string {
	return "---\nunit: " + unit + "\n---\n\n# Evidence\n\n## " + item + " — observed behavior\n\nKeys:\n  - item:" + item + "\n\nObserved " + unit + " behavior.\n"
}

func ownershipCandidate(t *testing.T, root, unit string) {
	t.Helper()
	code := "src/" + unit + ".go"
	grWriteFile(t, root, code, "package demo\n")
	grWriteSpecSurface(t, root, unit, code, "")
	grWriteFile(t, root, "docs/specs/units/candidate/evidence/unit_"+unit+".md", evidenceRecordFor(unit, unit+".core"))
}

func ownershipCheckCandidate(t *testing.T, root, unit string) {
	t.Helper()
	id := grPlan(t, root, "--gate", "validate", "--unit", unit, "--target", "candidate")
	run := mustLoadRun(t, root, id)
	for _, key := range run.Coverage {
		session, err := gaterun.BuildSessionSpec(root, run, []string{key.Key})
		if err != nil {
			t.Fatal(err)
		}
		scopes := map[string][]string{}
		for _, check := range session.CheckKeys {
			for _, path := range session.ReadRefs {
				scopes[check] = append(scopes[check], path+": all")
			}
		}
		grSubmitOK(t, root, id, key.Key, grValidateReport(session.CheckKeys, scopes))
	}
	grFinalizeOK(t, root, id)
	currentVerifyFixture(t, root, unit, "candidate", "")
}

// runCandidateGates runs the full candidate validate/verify gate set for a
// unit without publishing it.
func runCandidateGates(t *testing.T, root, name string) {
	t.Helper()
	runID := grPlan(t, root, "--gate", "validate", "--unit", name, "--target", "candidate")
	run := mustLoadRun(t, root, runID)
	for _, ck := range run.Coverage {
		session, err := gaterun.BuildSessionSpec(root, run, []string{ck.Key})
		if err != nil {
			t.Fatal(err)
		}
		scopes := map[string][]string{}
		for _, key := range session.CheckKeys {
			for _, path := range session.ReadRefs {
				scopes[key] = append(scopes[key], path+": all")
			}
		}
		grSubmitOK(t, root, runID, ck.Key, grValidateReport(session.CheckKeys, scopes))
	}
	grFinalizeOK(t, root, runID)
	verify := grPlan(t, root, "--gate", "verify", "--unit", name, "--target", "candidate")
	sharedFinish(t, root, verify)
}

// publishUnitThroughGates runs the full candidate validate/verify gate set and
// publishes the unit to stable — the standard pre-promote fixture.
func publishUnitThroughGates(t *testing.T, root, name string) {
	t.Helper()
	runCandidateGates(t, root, name)
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", name}, &out, &errOut); err != nil {
		t.Fatalf("publish %s: %v\n%s", name, err, out.String())
	}
}

func TestPromoteCLIRequiresDropDeclaration(t *testing.T) {
	root, _, _ := sharedFixture(t)
	// First round publishes the unit with an evidence record, so stable owns it.
	grWriteFile(t, root, "docs/specs/units/candidate/evidence/unit_auth.md", evidenceRecordFor("auth", "auth.core"))
	publishUnitThroughGates(t, root, "auth")

	// Second round forks the published unit and drops the record candidate-side.
	var forkOut, forkErr bytes.Buffer
	if err := runFork([]string{"--repo-root", root, "--unit", "auth"}, &forkOut, &forkErr); err != nil {
		t.Fatalf("fork: %v\n%s", err, forkOut.String())
	}
	if err := os.Remove(filepath.Join(root, "docs/specs/units/candidate/evidence/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	runCandidateGates(t, root, "auth")

	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err == nil {
		t.Fatal("promote must fail without --drop-evidence")
	}
	if !strings.Contains(out.String(), "unit_auth.md") {
		t.Fatalf("expected the dropped evidence record to be named, got:\n%s", out.String())
	}
	stableRecord := filepath.Join(root, "docs/specs/units/stable/evidence/unit_auth.md")
	if _, err := os.Stat(stableRecord); err != nil {
		t.Fatalf("stable evidence record must survive the refused promote: %v", err)
	}

	out.Reset()
	errOut.Reset()
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth", "--drop-evidence"}, &out, &errOut); err != nil {
		t.Fatalf("promote with declaration: %v\n%s%s", err, out.String(), errOut.String())
	}
	if _, err := os.Stat(stableRecord); !os.IsNotExist(err) {
		t.Fatalf("declared drop must remove the stable evidence record, got %v", err)
	}
}

// TestEvidenceOwnershipRejectsForeignFrontmatterWithoutWrites pins the dual
// ownership form: the deterministic file at the unit's evidence path must
// declare the same unit in its frontmatter.
func TestEvidenceOwnershipRejectsForeignFrontmatterWithoutWrites(t *testing.T) {
	for _, operation := range []string{"promote", "fork"} {
		t.Run(operation, func(t *testing.T) {
			root := createCLITestRepo(t)
			grEnableMissionLayout(t, root)
			grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")
			ownershipCandidate(t, root, "auth")
			foreign := evidenceRecordFor("auth_account", "auth.core")
			var destination string
			if operation == "promote" {
				// The candidate record is valid and the candidate gates are
				// fresh; the foreign owner sits at the stable destination this
				// round would overwrite.
				ownershipCheckCandidate(t, root, "auth")
				destination = "docs/specs/units/stable/evidence/unit_auth.md"
				grWriteFile(t, root, destination, foreign)
			} else {
				// A published unit is forked onto a foreign-owned candidate
				// destination at its own deterministic evidence path.
				ownershipPublish(t, root, "auth")
				destination = "docs/specs/units/candidate/evidence/unit_auth.md"
				grWriteFile(t, root, destination, foreign)
			}
			before := publicationFiles(t, root)
			var out string
			var err error
			if operation == "promote" {
				out, err = publicationPromote(t, root, "unit", "auth")
			} else {
				var stdout, stderr bytes.Buffer
				err = runFork([]string{"--repo-root", root, "--unit", "auth"}, &stdout, &stderr)
				out = stdout.String() + stderr.String()
			}
			if err == nil {
				t.Errorf("%s accepted a foreign-owned evidence record:\n%s", operation, out)
			} else if !strings.Contains(out, destination) {
				t.Errorf("failure did not identify the record path: %v\n%s", err, out)
			}
			if after := publicationFiles(t, root); !reflect.DeepEqual(before, after) {
				t.Error("ownership rejection changed files: main specs, evidence records, caches, baselines and candidates must stay unchanged")
			}
		})
	}
}

func TestEvidenceOwnershipAllowsNormalLifecycle(t *testing.T) {
	root := createCLITestRepo(t)
	ownershipCandidate(t, root, "auth")
	ownershipPublish(t, root, "auth")
	var out, stderr bytes.Buffer
	if err := runFork([]string{"--repo-root", root, "--unit", "auth"}, &out, &stderr); err != nil {
		t.Fatal(err, out.String())
	}
	// Repeat the fork with a surviving same-owner record but no candidate main.
	if err := os.Remove(filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runFork([]string{"--repo-root", root, "--unit", "auth"}, &out, &stderr); err != nil {
		t.Fatal(err, out.String())
	}
	updated := evidenceRecordFor("auth", "auth.core") + "\nUpdated auth observation.\n"
	grWriteFile(t, root, "docs/specs/units/candidate/evidence/unit_auth.md", updated)
	ownershipPublish(t, root, "auth")
	if got, err := os.ReadFile(filepath.Join(root, "docs/specs/units/stable/evidence/unit_auth.md")); err != nil || string(got) != updated {
		t.Fatalf("same-owner replacement failed: %v", err)
	}
}

func ownershipPublish(t *testing.T, root, unit string) {
	t.Helper()
	ownershipCheckCandidate(t, root, unit)
	if out, err := publicationPromote(t, root, "unit", unit); err != nil {
		t.Fatalf("publish %s: %v\n%s", unit, err, out)
	}
}

func TestUnrelatedUnitEvidenceKeepsPublishedUnitFresh(t *testing.T) {
	root, _, _ := sharedFixture(t)
	publishUnitThroughGates(t, root, "auth")
	before, err := checkStableUnitVerifyMerged(freshDerivation(t, root), root, "auth")
	if err != nil || !before.Fresh {
		t.Fatalf("initial auth cache: %+v %v", before, err)
	}
	grWriteFile(t, root, "peer.js", "export const independent = true;\n")
	peer := grWriteSpecSurface(t, root, "auth_extra", "peer.js", "")
	if err := os.Rename(peer, filepath.Join(root, "docs/specs/units/stable/unit_auth_extra.md")); err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, "docs/specs/units/stable/evidence/unit_auth_extra.md", evidenceRecordFor("auth_extra", "auth_extra.core"))
	after, err := checkStableUnitVerifyMerged(freshDerivation(t, root), root, "auth")
	if err != nil || !after.Fresh {
		t.Fatalf("unrelated unit evidence record made auth stale: %+v %v", after, err)
	}
}

func TestEvidenceOwnershipSurvivesGatePromoteAndFork(t *testing.T) {
	root, main, _ := sharedFixture(t)
	own := "docs/specs/units/candidate/evidence/unit_auth.md"
	peer := "docs/specs/units/stable/evidence/unit_auth_extra.md"
	peerCandidate := strings.Replace(peer, "/stable/", "/candidate/", 1)
	grWriteFile(t, root, own, evidenceRecordFor("auth", "auth.core"))
	peerContent := evidenceRecordFor("auth_extra", "auth_extra.core")
	peerRef := "unit:auth_extra:evidence"
	grWriteFile(t, root, peer, peerContent)
	grWriteFile(t, root, peerCandidate, peerContent)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, peerRef))
	key := "item:auth:auth.core"
	report := grVerifyItemReport(key, main, "contracts.js") + key + ": " + own + ": all\n" + key + ": " + peerRef + ": all\n"
	if err := sharedSubmit(t, root, id, key, report); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, id)
	baseline, err := validationcache.ReadGateBaseline(root, "unit", "auth", "verify")
	if err != nil {
		t.Fatal(err)
	}
	foundPeer := false
	for _, entry := range baseline.Entries {
		if entry.Path == peerRef {
			foundPeer = true
		}
	}
	if !foundPeer {
		t.Fatal("explicit peer evidence lost its logical binding in the recorded input surface")
	}
	sharedValidateFixture(t, root, "auth")
	var entries []validationcache.FileEntry
	for _, path := range []string{main, own} {
		entry, err := validationcache.BuildEvidenceEntry(root, path)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, *entry)
	}
	if _, err := validationcache.WriteCache(root, "unit", "auth", withPromiseFormat(t, root, "auth", "candidate", validationcache.CacheWrite{Command: "validate", Unit: "auth", Mode: "full", Result: "pass", Target: "candidate", Standards: fixtureStandards(t, root), Entries: entries})); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatalf("promote: %v %s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "meta/gate_runs", id)); !os.IsNotExist(err) {
		t.Fatal("promoted unit's candidate run survived the cleanup sweep")
	}
	if _, err := os.Stat(filepath.Join(root, own)); !os.IsNotExist(err) {
		t.Fatal("promoted own evidence record remained in candidate")
	}
	if content, err := os.ReadFile(filepath.Join(root, peerCandidate)); err != nil || string(content) != peerContent {
		t.Fatal("promote changed the peer candidate evidence record")
	}
	assertFresh := func(layer string) {
		t.Helper()
		if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", layer); err != nil || !check.Fresh {
			t.Fatalf("exact evidence bindings lost freshness in %s: %+v %v", layer, check, err)
		}
	}
	assertFresh("stable")
	// Removing the unrelated candidate makes an accidental peer copy visible.
	if err := os.Remove(filepath.Join(root, peerCandidate)); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if err := runFork([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatalf("fork: %v %s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, peerCandidate)); !os.IsNotExist(err) {
		t.Fatal("fork copied the peer evidence record into candidate")
	}
	assertFresh("candidate")
	grWriteFile(t, root, own, evidenceRecordFor("auth", "auth.core")+"\nChanged auth observation.\n")
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || check.Fresh {
		t.Fatalf("changed own evidence record failed to invalidate the judgment: %+v %v", check, err)
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func ownershipCandidate(t *testing.T, root, unit string, names ...string) {
	t.Helper()
	code := "src/" + unit + ".go"
	grWriteFile(t, root, code, "package demo\n")
	grWriteSpecSurface(t, root, unit, code, "")
	for _, name := range names {
		grWriteFile(t, root, "docs/specs/units/candidate/appendix/"+name,
			"---\nunit: "+unit+"\n---\n\n# Design notes\nProposed "+unit+" design.\n")
	}
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

func ownershipPublish(t *testing.T, root, unit string) {
	t.Helper()
	ownershipCheckCandidate(t, root, unit)
	if out, err := publicationPromote(t, root, "unit", unit); err != nil {
		t.Fatalf("publish %s: %v\n%s", unit, err, out)
	}
}

func TestAppendixDestinationOwnershipRejectsWithoutWrites(t *testing.T) {
	const collision = "unit_auth_account_token.md"
	const first = "unit_auth_a_first.md"
	for _, operation := range []string{"promote", "fork"} {
		for _, status := range []string{"active", "exempt"} {
			t.Run(operation+"/"+status, func(t *testing.T) {
				root := createCLITestRepo(t)
				layer := "stable"
				if operation == "promote" {
					ownershipCandidate(t, root, "auth_account", collision)
					ownershipPublish(t, root, "auth_account")
					ownershipCandidate(t, root, "auth", first, collision)
				} else {
					ownershipCandidate(t, root, "auth", first, collision)
					ownershipPublish(t, root, "auth")
					ownershipCandidate(t, root, "auth_account", collision)
					layer = "candidate"
				}
				destination := "docs/specs/units/" + layer + "/appendix/" + collision
				grWriteFile(t, root, destination, "---\nunit: auth_account\nstatus: "+status+"\n---\nPeer design.\n")
				if operation == "promote" {
					ownershipCheckCandidate(t, root, "auth")
				}
				before := publicationFiles(t, root)
				var out string
				var err error
				if operation == "promote" {
					out, err = publicationPromote(t, root, "unit", "auth")
				} else {
					var stdout, stderr bytes.Buffer
					err = runFork([]string{"--repo-root", root, "--unit", "auth"}, &stdout, &stderr)
					out = stdout.String()
				}
				if err == nil {
					t.Errorf("%s accepted a foreign-owned destination:\n%s", operation, out)
				} else if !strings.Contains(out, destination) || !strings.Contains(out, "auth_account") {
					t.Errorf("failure did not identify the destination and owner: %v\n%s", err, out)
				}
				if after := publicationFiles(t, root); !reflect.DeepEqual(before, after) {
					t.Error("ownership rejection changed files: main specs, earlier appendices, caches, baselines and candidates must stay unchanged")
				}
			})
		}
	}
}

func TestAppendixDestinationOwnershipAllowsNormalLifecycle(t *testing.T) {
	root := createCLITestRepo(t)
	const peer = "unit_auth_account_token.md"
	const own = "unit_auth_protocol.md"
	ownershipCandidate(t, root, "auth_account", peer)
	ownershipPublish(t, root, "auth_account")
	ownershipCandidate(t, root, "auth", own)
	ownershipPublish(t, root, "auth")
	peerPath := filepath.Join(root, "docs/specs/units/stable/appendix", peer)
	peerBefore, err := os.ReadFile(peerPath)
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := runFork([]string{"--repo-root", root, "--unit", "auth"}, &out, &stderr); err != nil {
		t.Fatal(err, out.String())
	}
	// Repeat the fork with a surviving same-owner appendix but no candidate main.
	if err := os.Remove(filepath.Join(root, "docs/specs/units/candidate/unit_auth.md")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runFork([]string{"--repo-root", root, "--unit", "auth"}, &out, &stderr); err != nil {
		t.Fatal(err, out.String())
	}
	updated := "---\nunit: auth\n---\nUpdated auth design.\n"
	grWriteFile(t, root, "docs/specs/units/candidate/appendix/"+own, updated)
	ownershipPublish(t, root, "auth")
	if got, err := os.ReadFile(filepath.Join(root, "docs/specs/units/stable/appendix", own)); err != nil || string(got) != updated {
		t.Fatalf("same-owner replacement failed: %v", err)
	}
	if got, err := os.ReadFile(peerPath); err != nil || string(got) != string(peerBefore) {
		t.Fatalf("ordinary lifecycle changed peer content: %v", err)
	}
}

func TestUnrelatedPrefixAppendixKeepsPublishedUnitFresh(t *testing.T) {
	root, _, _ := sharedFixture(t)
	removalPublishUnit(t, root, "auth")
	before, err := checkStableUnitVerifyMerged(freshDerivation(t, root), root, "auth")
	if err != nil || !before.Fresh {
		t.Fatalf("initial auth cache: %+v %v", before, err)
	}
	grWriteFile(t, root, "peer.js", "export const independent = true;\n")
	peer := grWriteSpecSurface(t, root, "auth_extra", "peer.js", "")
	if err := os.Rename(peer, filepath.Join(root, "docs/specs/units/stable/unit_auth_extra.md")); err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, "docs/specs/units/stable/appendix/unit_auth_extra_protocol.md", "---\nunit: auth_extra\nstatus: active\n---\n\n# Independent design\nNo auth dependency.\n")
	after, err := checkStableUnitVerifyMerged(freshDerivation(t, root), root, "auth")
	if err != nil || !after.Fresh {
		t.Fatalf("unrelated unit appendix made auth stale: %+v %v", after, err)
	}
}

func TestAppendixOwnershipSurvivesGatePromoteAndFork(t *testing.T) {
	root, main, _ := sharedFixture(t)
	own := "docs/specs/units/candidate/appendix/unit_auth_protocol.md"
	peer := "docs/specs/units/stable/appendix/unit_auth_extra_protocol.md"
	peerCandidate := strings.Replace(peer, "/stable/", "/candidate/", 1)
	grWriteFile(t, root, own, "---\nunit: auth\n---\n## Notes\nAuth explanation.\n")
	peerContent := "---\nunit: auth_extra\n---\n## Notes\nPeer explanation.\n"
	peerRef := "unit:auth_extra:appendix:unit_auth_extra_protocol"
	grWriteFile(t, root, peer, peerContent)
	grWriteFile(t, root, peerCandidate, peerContent)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, peerRef))
	key := "item:auth:auth.core"
	report := grVerifyItemReport(key, main, "contracts.js") + key + ": " + own + ": all\n" + key + ": " + peerRef + ": all\n"
	if err := sharedSubmit(t, root, id, key, report); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, id)
	state := grReadJudgmentBaseline(t, root, "unit", "auth", "verify")
	record, err := judgments.Load(root, state.Records[key].Reference)
	if err != nil {
		t.Fatal(err)
	}
	foundPeer := false
	for _, dep := range record.Dependencies {
		if dep.Path == peerRef {
			foundPeer = true
			if dep.Own {
				t.Fatal("peer appendix became an own-spec dependency")
			}
		}
	}
	if !foundPeer {
		t.Fatal("explicit peer evidence lost its logical binding")
	}
	sharedValidateFixture(t, root, "auth")
	var entries []validationcache.FileEntry
	for _, path := range []string{main, own} {
		entry, err := validationcache.BuildEntry(root, validationcache.EntryDeclaration{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if _, err := validationcache.WriteCache(root, "unit", "auth", validationcache.CacheWrite{Command: "validate", Unit: "auth", Mode: "full", Result: "pass", Target: "candidate", Entries: entries}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", "auth"}, &out, &errOut); err != nil {
		t.Fatalf("promote: %v %s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, own)); !os.IsNotExist(err) {
		t.Fatal("promoted own appendix remained in candidate")
	}
	if content, err := os.ReadFile(filepath.Join(root, peerCandidate)); err != nil || string(content) != peerContent {
		t.Fatal("promote changed the peer candidate appendix")
	}
	assertFresh := func(layer string) {
		t.Helper()
		if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", layer); err != nil || !check.Fresh {
			t.Fatalf("exact appendix bindings lost freshness in %s: %+v %v", layer, check, err)
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
		t.Fatal("fork copied the peer appendix into candidate")
	}
	assertFresh("candidate")
	grWriteFile(t, root, own, "---\nunit: auth\n---\n## Notes\nChanged auth explanation.\n")
	if check, err := checkUnitVerifyMerged(freshDerivation(t, root), root, "auth", "candidate"); err != nil || check.Fresh {
		t.Fatalf("changed own appendix failed to invalidate the judgment: %+v %v", check, err)
	}
}

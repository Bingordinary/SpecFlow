package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func TestUnrelatedPrefixAppendixKeepsPublishedUnitFresh(t *testing.T) {
	root, _, _ := sharedFixture(t)
	removalPublishUnit(t, root, "auth")
	before, err := checkStableUnitVerifyMerged(root, "auth")
	if err != nil || !before.Fresh {
		t.Fatalf("initial auth cache: %+v %v", before, err)
	}
	grWriteFile(t, root, "peer.js", "export const independent = true;\n")
	peer := grWriteSpecSurface(t, root, "auth_extra", "peer.js", "")
	if err := os.Rename(peer, filepath.Join(root, "docs/specs/units/stable/unit_auth_extra.md")); err != nil {
		t.Fatal(err)
	}
	grWriteFile(t, root, "docs/specs/units/stable/appendix/unit_auth_extra_protocol.md", "---\nunit: auth_extra\nstatus: active\n---\n\n# Independent design\nNo auth dependency.\n")
	after, err := checkStableUnitVerifyMerged(root, "auth")
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
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--input", peerRef)
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
		if check, err := checkUnitVerifyMerged(root, "auth", layer); err != nil || !check.Fresh {
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
	if check, err := checkUnitVerifyMerged(root, "auth", "candidate"); err != nil || check.Fresh {
		t.Fatalf("changed own appendix failed to invalidate the judgment: %+v %v", check, err)
	}
}

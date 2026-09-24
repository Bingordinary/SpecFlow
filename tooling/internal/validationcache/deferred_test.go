package validationcache

import (
	"os"
	"path/filepath"
	"testing"
)

func deferredTestEntry(id, owner, source string) DeferredEntry {
	return DeferredEntry{
		FindingID:    id,
		OwnerUnit:    owner,
		SourceUnit:   source,
		SourceRun:    "20260101-000000-abcdef",
		Severity:     "P1",
		Text:         "shared-file defect",
		Detail:       "[P1] src/shared.go — shared-file defect",
		AffectedKeys: []string{"src/shared.go"},
		EvidencePath: "docs/specs/units/candidate/unit_tool.md",
		Reason:       "recorded ownership",
	}
}

func deferredLedgerPath(t *testing.T, repoRoot string) string {
	t.Helper()
	return filepath.Join(repoRoot, filepath.FromSlash(DeferredLedgerRelPath))
}

func TestDeferredLedgerRoundTrip(t *testing.T) {
	repoRoot := t.TempDir()
	ledger, err := ReadDeferredLedger(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Entries) != 0 {
		t.Fatalf("a missing ledger must read as empty, got %+v", ledger.Entries)
	}

	ledger.Entries = []DeferredEntry{
		deferredTestEntry("run-2/src/b.go/F1", "agent", "tool"),
		deferredTestEntry("run-1/src/a.go/F1", "contracts", "tool"),
	}
	if err := WriteDeferredLedger(repoRoot, ledger); err != nil {
		t.Fatal(err)
	}
	read, err := ReadDeferredLedger(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if read.SchemaVersion != deferredLedgerSchema || len(read.Entries) != 2 {
		t.Fatalf("unexpected round trip: %+v", read)
	}
	if read.Entries[0].FindingID != "run-1/src/a.go/F1" || read.Entries[1].FindingID != "run-2/src/b.go/F1" {
		t.Fatalf("expected stable finding-id order, got %+v", read.Entries)
	}
	if got := read.PendingForUnit("agent"); len(got) != 1 || got[0].OwnerUnit != "agent" {
		t.Fatalf("expected PendingForUnit to filter by owner, got %+v", got)
	}
	if got := read.PendingForUnit("nobody"); len(got) != 0 {
		t.Fatalf("expected no entries for an unrelated unit, got %+v", got)
	}
}

func TestDeferredLedgerEmptyWriteRemovesFile(t *testing.T) {
	repoRoot := t.TempDir()
	if err := WriteDeferredLedger(repoRoot, DeferredLedger{Entries: []DeferredEntry{deferredTestEntry("run/src/F1", "agent", "tool")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(deferredLedgerPath(t, repoRoot)); err != nil {
		t.Fatalf("expected the ledger file to exist, stat err=%v", err)
	}
	if err := WriteDeferredLedger(repoRoot, DeferredLedger{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(deferredLedgerPath(t, repoRoot)); !os.IsNotExist(err) {
		t.Fatalf("expected the empty ledger to remove the file, stat err=%v", err)
	}
}

func TestDeferredLedgerFailsClosedOnCorruption(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "malformed json", content: "{not json"},
		{name: "unsupported schema", content: `{"schema_version": 9, "entries": []}`},
		{name: "missing schema", content: `{"entries": []}`},
		{name: "duplicate finding", content: `{"schema_version": 1, "entries": [
			{"finding_id": "run/src/F1", "owner_unit": "agent", "source_unit": "tool", "source_run": "run", "severity": "P1", "text": "t", "detail": "d", "affected_keys": ["src/a.go"], "evidence_path": "spec.md", "reason": "r"},
			{"finding_id": "run/src/F1", "owner_unit": "agent", "source_unit": "tool", "source_run": "run", "severity": "P1", "text": "t", "detail": "d", "affected_keys": ["src/a.go"], "evidence_path": "spec.md", "reason": "r"}
		]}`},
		{name: "invalid owner name", content: `{"schema_version": 1, "entries": [
			{"finding_id": "run/src/F1", "owner_unit": "../escape", "source_unit": "tool", "source_run": "run", "severity": "P1", "text": "t", "detail": "d", "affected_keys": ["src/a.go"], "evidence_path": "spec.md", "reason": "r"}
		]}`},
		{name: "missing detail", content: `{"schema_version": 1, "entries": [
			{"finding_id": "run/src/F1", "owner_unit": "agent", "source_unit": "tool", "source_run": "run", "severity": "P1", "text": "t", "detail": "", "affected_keys": ["src/a.go"], "evidence_path": "spec.md", "reason": "r"}
		]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := t.TempDir()
			path := deferredLedgerPath(t, repoRoot)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadDeferredLedger(repoRoot); err == nil {
				t.Fatalf("expected a fail-closed read error for %s", tc.name)
			}
		})
	}
}

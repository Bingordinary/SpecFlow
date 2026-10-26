package gaterun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

func gcRecord(t *testing.T, root, subject, protocol string, refs []judgments.Binding) judgments.Reference {
	t.Helper()
	report := "report " + subject
	ref, err := judgments.Save(root, judgments.Record{
		Version:      judgments.RecordVersion,
		Kind:         judgments.Code,
		Subject:      subject,
		Coverage:     []string{"code:" + subject},
		Inputs:       []string{subject},
		Dependencies: []judgments.Dependency{{Path: subject, Hash: "deadbeef"}},
		References:   refs,
		Protocol:     protocol,
		Verdict:      "FACTS",
		Result:       json.RawMessage(`{"observations":[]}`),
		Report:       report,
		ReportDigest: judgments.Digest([]byte(report)),
		SourceRun:    "20260101-000000-gctest",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// gcItemRecord saves an acceptance-item decision. Save files it in the item's
// slot and advances that slot's current-decision pointer, so the record is the
// decision in force for the key in its standard.
func gcItemRecord(t *testing.T, root, unit, subject, context, protocol string) judgments.Reference {
	t.Helper()
	report := "item report " + protocol
	ref, err := judgments.Save(root, judgments.Record{
		Version:      judgments.RecordVersion,
		Kind:         judgments.Item,
		Unit:         unit,
		Subject:      subject,
		SpecContext:  context,
		Coverage:     []string{"item:" + unit + ":" + subject},
		Inputs:       []string{"docs/specs/units/candidate/unit_" + unit + ".md"},
		Dependencies: []judgments.Dependency{{Path: "docs/specs/units/candidate/unit_" + unit + ".md"}},
		Protocol:     protocol,
		Verdict:      "ALIGNED",
		Result:       json.RawMessage(`{"observations":[]}`),
		Report:       report,
		ReportDigest: judgments.Digest([]byte(report)),
		SourceRun:    "20260101-000000-gctest",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// writeDamagedCurrentPointer writes bytes that are not a pointer into the slot
// of one item decision, so the damaged-pointer path can be exercised.
func writeDamagedCurrentPointer(t *testing.T, root string) {
	t.Helper()
	slot, err := judgments.Slot("stale-protocol-fingerprint", judgments.Item, "auth", "auth.core", strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, filepath.FromSlash(judgments.Directory), filepath.FromSlash(slot), "current")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("garbage"), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeCacheBaseline(t *testing.T, root string, refs ...judgments.Reference) {
	t.Helper()
	records := map[string]judgments.Binding{}
	for i, ref := range refs {
		records["code:"+string(rune('a'+i))] = judgments.Binding{Reference: ref, Layer: TargetCandidate, Source: "reused"}
	}
	payload := mustJSON(t, map[string]any{
		"schema_version":   1,
		"records":          records,
		"logical_status":   map[string]string{},
		"findings":         []any{},
		"synthesis_digest": "digest",
		"relationships":    []any{},
	})
	writeFile(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md",
		"---\ntarget: candidate\n---\n\n<!-- GATE_JUDGMENTS_BEGIN\n"+string(payload)+"\nGATE_JUDGMENTS_END -->\n")
}

func TestCollectJudgmentsRemovesExactlyUnreachableStaleProtocolRecords(t *testing.T) {
	root := t.TempDir()
	current := testProtocolFor(t, root, "verify", "unit")
	stale := "stale-protocol-fingerprint"

	live := gcRecord(t, root, "live.js", current, nil)
	child := gcRecord(t, root, "child.js", stale, nil)
	// The cache binds parent (a current-protocol record); parent consumes the
	// stale child, so the transitive reference chain keeps child alive.
	parent := gcRecord(t, root, "parent.js", current, []judgments.Binding{{Reference: child, Layer: TargetCandidate, Source: "executed"}})
	// A retired standard's current-decision pointer is dead: no reader asks that
	// standard for a decision, so the record it names is debt.
	accepted := gcItemRecord(t, root, "auth", "auth.core", strings.Repeat("cd", 32), stale)
	openBound := gcRecord(t, root, "open.js", stale, nil)
	orphanA := gcRecord(t, root, "orphan-a.js", stale, nil)
	orphanB := gcRecord(t, root, "orphan-b.js", stale, nil)
	history := gcRecord(t, root, "history.js", current, nil)
	corrupt := gcRecord(t, root, "corrupt.js", stale, nil)

	// Make the stale record unreadable: collection must decide from the slot.
	p, err := judgments.StoredPath(root, corrupt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	writeCacheBaseline(t, root, live, parent)

	run := &Run{
		RunID: "20260101-000000-cafe01", Gate: GateValidate, TargetKind: TargetKindUnit,
		TargetName: "auth", Target: TargetCandidate, Status: StatusOpen, Mode: ModeFull, Protocol: current,
		Coverage: []CoverageKey{{Key: "clarity", Kind: SessionKindChecks}},
		Records:  map[string]judgments.Binding{"clarity": {Reference: openBound, Layer: TargetCandidate, Source: "executed"}},
	}
	if err := writeRun(root, run); err != nil {
		t.Fatal(err)
	}

	collected, err := CollectJudgments(root)
	if err != nil {
		t.Fatal(err)
	}
	if collected.Count != 4 || collected.Bytes <= 0 {
		t.Fatalf("collected %+v, want the two orphans, the unreadable stale record, and the retired decision", collected)
	}
	for _, ref := range []judgments.Reference{live, child, openBound, history} {
		if _, err := judgments.Load(root, ref); err != nil {
			t.Fatalf("protected record %s was collected or damaged: %v", ref.ID, err)
		}
	}
	// A decision of a retired standard is storage debt even though its own slot
	// names it current: the pointer goes with the record.
	if _, err := judgments.Load(root, accepted); err == nil {
		t.Fatal("a retired standard's decision survived collection")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(judgments.Directory), filepath.FromSlash(accepted.Slot), "current")); !os.IsNotExist(err) {
		t.Fatal("a retired standard's current pointer survived collection")
	}
	// Staleness is decided from the slot, so an unreadable record filed under a
	// retired standard is collected with the rest of that standard.
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("an unreadable stale-protocol record survived collection")
	}
	for _, ref := range []judgments.Reference{orphanA, orphanB} {
		if _, err := judgments.Load(root, ref); err == nil {
			t.Fatalf("orphan %s survived collection", ref.ID)
		}
	}
	// Collection is idempotent: the second pass finds nothing to remove.
	again, err := CollectJudgments(root)
	if err != nil || again.Count != 0 {
		t.Fatalf("second pass collected %+v (%v), want zero", again, err)
	}
}

func TestCollectJudgmentsRemovesInvalidationMarkerOfCollectedRecord(t *testing.T) {
	root := t.TempDir()
	live := gcRecord(t, root, "live.js", testProtocolFor(t, root, "verify", "unit"), nil)
	orphan := gcRecord(t, root, "orphan.js", "stale-protocol-fingerprint", nil)
	writeCacheBaseline(t, root, live)
	if err := judgments.Invalidate(root, orphan, "contradicted by newer evidence"); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "docs/specs/meta/validation/judgments/invalidated", orphan.ID+".json")
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
	collected, err := CollectJudgments(root)
	if err != nil || collected.Count != 1 {
		t.Fatalf("collected %+v (%v), want the single orphan", collected, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("invalidation marker of a collected record survived")
	}
	if _, err := judgments.Load(root, live); err != nil {
		t.Fatalf("live record damaged: %v", err)
	}
}

func TestCollectJudgmentsFailsClosedOnDamagedCurrentPointer(t *testing.T) {
	root := t.TempDir()
	gcRecord(t, root, "a.js", "stale-protocol-fingerprint", nil)
	writeDamagedCurrentPointer(t, root)
	if _, err := CollectJudgments(root); err == nil {
		t.Fatal("a damaged current-decision pointer must fail collection, not shrink the live set")
	}
}

// TestCollectJudgmentsRetiresOnlyTheDeployedStandardsDecisions pins the
// pointer half of the liveness rule. A decision of the deployed standard is
// live through its own pointer, with no cache binding needed. The same key's
// decision under a retired standard can never be read by any command, so it is
// storage debt: it is collected and Remove drops its pointer with it.
func TestCollectJudgmentsRetiresOnlyTheDeployedStandardsDecisions(t *testing.T) {
	root := t.TempDir()
	context := strings.Repeat("ab", 32)

	keep := gcItemRecord(t, root, "auth", "auth.core", context, testProtocolFor(t, root, "verify", "unit"))
	drop := gcItemRecord(t, root, "auth", "auth.core", context, "stale-protocol-fingerprint")

	collected, err := CollectJudgments(root)
	if err != nil || collected.Count != 1 {
		t.Fatalf("collected %+v (%v), want exactly the retired decision", collected, err)
	}
	if _, err := judgments.Load(root, keep); err != nil {
		t.Fatalf("the deployed standard's current decision was collected: %v", err)
	}
	if _, err := judgments.Load(root, drop); err == nil {
		t.Fatal("the retired standard's decision survived collection")
	}
	pointer := filepath.Join(root, filepath.FromSlash(judgments.Directory), filepath.FromSlash(drop.Slot), "current")
	if _, err := os.Stat(pointer); !os.IsNotExist(err) {
		t.Fatal("the retired decision's current pointer survived collection")
	}
}

func TestCollectJudgmentsFailsClosedOnDamagedCacheBaseline(t *testing.T) {
	root := t.TempDir()
	gcRecord(t, root, "a.js", "stale-protocol-fingerprint", nil)
	writeFile(t, root, "docs/specs/meta/validation/unit/auth/verify_result.md",
		"<!-- GATE_JUDGMENTS_BEGIN\nnot json\nGATE_JUDGMENTS_END -->")
	if _, err := CollectJudgments(root); err == nil {
		t.Fatal("an unparsable cache baseline must fail collection, not shrink the live set")
	}
}

// writeRetiredRunState writes a verify run state of a retired protocol shape
// the strict loader rejects outright, so every listing skips it and no replan
// replaces it — the abandoned open run of issue #61. It still binds its
// records on disk.
func writeRetiredRunState(t *testing.T, root, runID, status string, records map[string]judgments.Binding) {
	t.Helper()
	coverage := []CoverageKey{{Key: "code:reused.js", Kind: SessionKindCode, Lens: LensQuality, Task: "task-0001"}}
	payload := mustJSON(t, map[string]any{
		"run_id": runID, "schema_version": 1, "protocol": "retired-protocol-fingerprint",
		"gate": GateVerify, "target_kind": TargetKindUnit, "target_name": "auth",
		"target": TargetCandidate, "status": status, "mode": ModeFull,
		"records": records, "coverage": coverage,
	})
	p := filepath.Join(root, "meta", "gate_runs", runID, "run.json")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, payload, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, runID); err == nil {
		t.Fatal("fixture must produce a run state the strict loader rejects")
	}
}

func TestCollectJudgmentsProtectsRecordsOfUnlistableOpenRun(t *testing.T) {
	root := t.TempDir()
	stale := "stale-protocol-fingerprint"
	bound := gcRecord(t, root, "bound.js", stale, nil)
	orphan := gcRecord(t, root, "orphan.js", stale, nil)

	// The rejected run is skipped by every listing, yet its run.json still
	// binds a record nothing else protects. Seeding the live set through the
	// listing loader would turn "skipped for display" into "skipped for
	// liveness" and strand the run state with a dangling reference.
	writeRetiredRunState(t, root, "20260101-000000-a11d01", StatusOpen, map[string]judgments.Binding{
		"code:bound.js": {Reference: bound, Layer: TargetCandidate, Source: "executed"},
	})

	collected, err := CollectJudgments(root)
	if err != nil {
		t.Fatal(err)
	}
	if collected.Count != 1 {
		t.Fatalf("collected %+v, want exactly the unbound orphan", collected)
	}
	if _, err := judgments.Load(root, bound); err != nil {
		t.Fatalf("record bound by an unlistable open run was collected: %v", err)
	}
	if _, err := judgments.Load(root, orphan); err == nil {
		t.Fatal("orphan survived collection")
	}
}

func TestCollectJudgmentsProtectsRecordsReachedThroughOpenRunTasks(t *testing.T) {
	root := t.TempDir()
	reached := gcRecord(t, root, "reached.js", "stale-protocol-fingerprint", nil)

	// The open run binds the record only through a shared task file — a
	// reference path the run.json's records section does not name.
	run := &Run{
		RunID: "20260101-000000-cafe02", Gate: GateValidate, TargetKind: TargetKindUnit,
		TargetName: "auth", Target: TargetCandidate, Status: StatusOpen, Mode: ModeFull,
		Coverage: []CoverageKey{{Key: "code:reached.js", Kind: SessionKindCode, Task: "task-0001"}},
	}
	if err := writeRun(root, run); err != nil {
		t.Fatal(err)
	}
	task := mustJSON(t, sharedTask{ID: "task-0001", Owner: run.RunID, Key: "code:reached.js", Inputs: []string{"reached.js"}, Record: &reached})
	p := filepath.Join(root, "meta", "gate_runs", "shared", "task-0001.json")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, task, 0644); err != nil {
		t.Fatal(err)
	}

	collected, err := CollectJudgments(root)
	if err != nil {
		t.Fatal(err)
	}
	if collected.Count != 0 {
		t.Fatalf("collected %+v, want the task-reached record to survive", collected)
	}
	if _, err := judgments.Load(root, reached); err != nil {
		t.Fatalf("record reached through an open run's task file was collected: %v", err)
	}
}

func TestCollectJudgmentsFailsClosedOnDamagedRunState(t *testing.T) {
	root := t.TempDir()
	gcRecord(t, root, "a.js", "stale-protocol-fingerprint", nil)
	if err := os.MkdirAll(filepath.Join(root, "meta", "gate_runs", "20260101-000000-dead01"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "meta", "gate_runs", "20260101-000000-dead01", "run.json"), []byte("garbage"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := CollectJudgments(root); err == nil {
		t.Fatal("an unparsable run state must fail collection, not shrink the live set")
	}
}

func TestCollectJudgmentsIgnoresEmptyRunDirectory(t *testing.T) {
	root := t.TempDir()
	orphan := gcRecord(t, root, "orphan.js", "stale-protocol-fingerprint", nil)
	// An interrupted initial write leaves a run directory with no run.json.
	// The directory provably binds nothing, so collection proceeds.
	if err := os.MkdirAll(filepath.Join(root, "meta", "gate_runs", "20260101-000000-cee001"), 0755); err != nil {
		t.Fatal(err)
	}
	collected, err := CollectJudgments(root)
	if err != nil {
		t.Fatal(err)
	}
	if collected.Count != 1 {
		t.Fatalf("collected %+v, want the orphan", collected)
	}
	if _, err := judgments.Load(root, orphan); err == nil {
		t.Fatal("orphan survived collection")
	}
}

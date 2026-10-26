package main

import (
	"io"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// measureFreshReads runs the fresh command once to settle any one-time work and
// then returns the reads the next run performs.
func measureFreshReads(t *testing.T, root, unit string) int64 {
	t.Helper()
	if err := runFresh([]string{"--repo-root", root, "--unit", unit}, io.Discard, io.Discard); err != nil {
		t.Fatalf("fresh: %v", err)
	}
	before := procReadCount()
	if before < 0 {
		t.Skip("read accounting needs /proc/self/io")
	}
	if err := runFresh([]string{"--repo-root", root, "--unit", unit}, io.Discard, io.Discard); err != nil {
		t.Fatalf("fresh: %v", err)
	}
	return procReadCount() - before
}

// TestFreshReadsDoNotGrowWithStoreSize pins the same cost invariant for the
// freshness report that TestSurfacesReadsDoNotGrowWithStoreSize pins for the
// surface audit: a unit's freshness is decided from the records its own cache
// binds, so filing records for other files must not make the report slower.
func TestFreshReadsDoNotGrowWithStoreSize(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("read accounting needs /proc/self/io")
	}
	root, _, _ := sharedFixture(t)
	runID := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--mode", "full", "--relationships", "none")
	// sharedFinish drives every key to accepted and finalizes the run.
	sharedFinish(t, root, runID)

	protocol, err := judgments.Protocol(root, gaterun.GateVerify, gaterun.TargetKindUnit)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := specpaths.FileHash(filepath.Join(root, "contracts.js"))
	if err != nil {
		t.Fatal(err)
	}

	const small = 20
	for i := 0; i < small; i++ {
		saveUnrelatedRecord(t, root, i, protocol, hash)
	}
	before := measureFreshReads(t, root, "auth")

	const large = 1500
	for i := small; i < large; i++ {
		saveUnrelatedRecord(t, root, i, protocol, hash)
	}
	after := measureFreshReads(t, root, "auth")

	t.Logf("fresh reads: %d with %d filed records, %d with %d filed records", before, small, after, large)
	if grew := after - before; grew > 40 {
		t.Fatalf("fresh reads grew by %d (from %d to %d) after filing %d more unrelated records: the freshness report is scanning the store instead of reading the records its cache binds",
			grew, before, after, large-small)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// procReadCount returns the process's read syscall count, or -1 when the host
// does not expose it.
func procReadCount() int64 {
	data, err := os.ReadFile("/proc/self/io")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "syscr:") {
			continue
		}
		value, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "syscr:")), 10, 64)
		if err != nil {
			return -1
		}
		return value
	}
	return -1
}

// measureSurfacesReads runs the surfaces command once to settle lazy work and
// then returns the reads the next run performs.
func measureSurfacesReads(t *testing.T, root string) int64 {
	t.Helper()
	if err := runSurfaces([]string{"--repo-root", root, "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("surfaces: %v", err)
	}
	before := procReadCount()
	if before < 0 {
		t.Skip("read accounting needs /proc/self/io")
	}
	if err := runSurfaces([]string{"--repo-root", root, "--json"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("surfaces: %v", err)
	}
	return procReadCount() - before
}

// saveUnrelatedRecord files one accepted code judgment for a file no unit
// declares, so it is reachable only by walking the store's slots.
func saveUnrelatedRecord(t *testing.T, root string, index int, protocol, hash string) {
	t.Helper()
	subject := fmt.Sprintf("extra/file-%04d.go", index)
	if _, err := judgments.Save(root, judgments.Record{
		Version:      judgments.RecordVersion,
		Kind:         judgments.Code,
		Subject:      subject,
		Coverage:     []string{"code:" + subject},
		Inputs:       []string{"contracts.js"},
		Dependencies: []judgments.Dependency{{Path: "contracts.js", Hash: hash}},
		Protocol:     protocol,
		Verdict:      "FACTS",
		Result:       json.RawMessage(`{"observations":[]}`),
		Report:       "filler facts",
		ReportDigest: judgments.Digest([]byte("filler facts")),
		SourceRun:    "20260101-000000-filler",
	}); err != nil {
		t.Fatalf("save filler record: %v", err)
	}
}

// TestSurfacesReadsDoNotGrowWithStoreSize pins the cost invariant the store
// layout exists for: asking what public and design judgments a declaration set
// has must cost the same whether the store holds a hundred records or two
// thousand. A regression that walks the store to find a record's key shows up
// here as reads that grow with the store.
func TestSurfacesReadsDoNotGrowWithStoreSize(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("read accounting needs /proc/self/io")
	}
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	grWriteSpecSurface(t, root, "auth", "contracts.js", "")
	grWriteFile(t, root, "contracts.js", "export function response(token) { return {token}; }\n")

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
	before := measureSurfacesReads(t, root)

	const large = 1500
	for i := small; i < large; i++ {
		saveUnrelatedRecord(t, root, i, protocol, hash)
	}
	after := measureSurfacesReads(t, root)

	t.Logf("surfaces reads: %d with %d filed records, %d with %d filed records", before, small, after, large)
	if grew := after - before; grew > 40 {
		t.Fatalf("surfaces reads grew by %d (from %d to %d) after filing %d more unrelated records: the store is being scanned instead of addressed by slot",
			grew, before, after, large-small)
	}
}

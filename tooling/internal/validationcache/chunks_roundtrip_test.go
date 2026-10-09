package validationcache

import (
	"reflect"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

// TestCacheChunksRoundTrip verifies that the ordered chunk sequence with
// positions survives render → parse unchanged. This is the baseline side of
// the delta change diff, so a lost or reordered field would silently weaken
// change detection.
func TestCacheChunksRoundTrip(t *testing.T) {
	entry := FileEntry{
		Path:    "docs/specs/units/candidate/unit_auth.md",
		Hash:    "sha256:abc",
		Chunker: contenthash.ChunkerVersion,
		Chunks: []contenthash.ChunkRecord{
			{CID: "sha256:1", StartLine: 1, EndLine: 10},
			{CID: "sha256:2", StartLine: 11, EndLine: 42},
			{CID: "sha256:3", StartLine: 43, EndLine: 43},
		},
	}
	w := CacheWrite{
		Command:   "validate",
		Unit:      "auth",
		Mode:      "full",
		Result:    "pass",
		Target:    "candidate",
		Timestamp: "2026-01-01T00:00:00Z",
		Entries:   []FileEntry{entry},
	}
	fm, err := renderCacheFrontmatter(w, "auth")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseCache([]byte(fm + "\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Files) != 1 {
		t.Fatalf("expected one parsed entry, got %d", len(parsed.Files))
	}
	got := parsed.Files[0]
	if got.Chunker != contenthash.ChunkerVersion {
		t.Fatalf("chunker = %q, want %q", got.Chunker, contenthash.ChunkerVersion)
	}
	if !reflect.DeepEqual(got.Chunks, entry.Chunks) {
		t.Fatalf("chunks changed across round trip:\n got %+v\nwant %+v", got.Chunks, entry.Chunks)
	}
}

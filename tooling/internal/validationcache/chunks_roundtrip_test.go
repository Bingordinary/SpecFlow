package validationcache

import (
	"reflect"
	"strings"
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

// TestCacheZeroChunkEvidenceRoundTrip pins the representation of a file whose
// content chunks to nothing (an empty or newline-only file, e.g. a tracked
// .gitkeep). The chunker deliberately produces no chunk for such content, so
// the format requires the recorded evidence block — chunker plus a possibly
// empty chunk list — not a non-empty list. Losing the block made every input
// surface containing a placeholder unfinalizable (issue #81).
func TestCacheZeroChunkEvidenceRoundTrip(t *testing.T) {
	kinds := []struct {
		name    string
		content string
	}{
		{"empty file", ""},
		{"newline-only file", "\n"},
	}
	for _, kind := range kinds {
		t.Run(kind.name, func(t *testing.T) {
			entry := FileEntry{
				Path:    "eval/runtime/.gitkeep",
				Hash:    contenthash.FileHashText(kind.content),
				Chunker: contenthash.ChunkerVersion,
				Chunks:  contenthash.ChunkRecords(kind.content),
			}
			if len(entry.Chunks) != 0 {
				t.Fatalf("precondition: expected zero chunks for %q, got %d", kind.content, len(entry.Chunks))
			}
			w := CacheWrite{
				Command:   "verify",
				Unit:      "auth",
				Mode:      "full",
				Result:    "pass",
				Timestamp: "2026-01-01T00:00:00Z",
				Entries:   []FileEntry{entry},
			}
			fm, err := renderCacheFrontmatter(w, "auth")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(fm, "chunker: "+contenthash.ChunkerVersion) || !strings.Contains(fm, "chunks:") {
				t.Fatalf("expected the recorded chunk evidence block, got:\n%s", fm)
			}
			parsed, err := parseCache([]byte(fm + "\n---\n"))
			if err != nil {
				t.Fatalf("a recorded zero-chunk entry must be current-format evidence: %v", err)
			}
			if len(parsed.Files) != 1 {
				t.Fatalf("expected one parsed entry, got %d", len(parsed.Files))
			}
			got := parsed.Files[0]
			if got.Chunker != contenthash.ChunkerVersion || len(got.Chunks) != 0 {
				t.Fatalf("zero-chunk entry did not round-trip: chunker=%q chunks=%d", got.Chunker, len(got.Chunks))
			}
		})
	}
}

// TestCacheRejectsMissingChunkBlock pins the other side of the requirement: a
// recorded evidence block whose list is empty is valid, but a missing block —
// a cache without the recorded block — a partial write or corruption — is still rejected as not
// current-format.
func TestCacheRejectsMissingChunkBlock(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"no chunk evidence", "  - path: eval/runtime/.gitkeep\n    hash: sha256:empty\n"},
		{"chunker without block", "  - path: eval/runtime/.gitkeep\n    hash: sha256:empty\n    chunker: buzhash-v1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := "---\ncommand: verify\nunit: auth\nmode: full\nresult: pass\ntimestamp: \"2026-01-01T00:00:00Z\"\nfiles:\n" + tc.entry
			if _, err := parseCache([]byte(fm + "---\n")); err == nil || !strings.Contains(err.Error(), "chunk evidence") {
				t.Fatalf("expected the missing chunk-evidence block to be rejected, got err=%v", err)
			}
		})
	}
}

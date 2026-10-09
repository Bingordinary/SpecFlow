package contenthash

import (
	"fmt"
	"strings"
	"testing"
)

// variedText builds a deterministic multi-chunk text. The varied tokens
// force content-defined boundaries (repetitive filler would coalesce into
// one chunk), so diffs exercise real alignment.
func variedText(prefix string, lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "%s-%04d token-%d payload-%d %x\n",
			prefix, i, (i*7919)%104729, (i*i*31)%99991, i*2654435761%4294967291)
	}
	return b.String()
}

func textAtLines(t *testing.T, text string, start, end int) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	if start < 1 || end > len(lines) || start > end {
		t.Fatalf("line span %d-%d out of range for %d lines", start, end, len(lines))
	}
	return strings.Join(lines[start-1:end], "\n")
}

func requireEntryContaining(t *testing.T, entries []ChangeEntry, curText, marker string) {
	t.Helper()
	for _, e := range entries {
		if e.Kind == ChangeRemoved {
			continue
		}
		if strings.Contains(textAtLines(t, curText, e.StartLine, e.EndLine), marker) {
			return
		}
	}
	t.Fatalf("no added/changed entry contains %q; entries=%+v", marker, entries)
}

func requireNonEmpty(t *testing.T, entries []ChangeEntry, what string) {
	t.Helper()
	if len(entries) == 0 {
		t.Fatalf("expected the diff to detect %s, got no entries", what)
	}
}

func TestDiffChunksIdenticalFile(t *testing.T) {
	text := variedText("line", 300)
	base := ChunkRecords(text)
	if entries := DiffChunks(base, text); len(entries) != 0 {
		t.Fatalf("identical content must produce no entries, got %+v", entries)
	}
}

func TestDiffChunksDetectsAppend(t *testing.T) {
	text := variedText("line", 300)
	base := ChunkRecords(text)
	cur := text + "\n## New Requirements\n\nbrand new obligation marker\n"
	entries := DiffChunks(base, cur)
	requireNonEmpty(t, entries, "an appended section")
	requireEntryContaining(t, entries, cur, "brand new obligation marker")
}

func TestDiffChunksDetectsInsertionInMiddle(t *testing.T) {
	text := variedText("line", 400)
	base := ChunkRecords(text)
	lines := strings.Split(text, "\n")
	insertAt := len(lines) / 2
	cur := strings.Join(append(append(append([]string{}, lines[:insertAt]...),
		"## Inserted Section", "", "inserted middle marker"), lines[insertAt:]...), "\n")
	entries := DiffChunks(base, cur)
	requireNonEmpty(t, entries, "an inserted section")
	requireEntryContaining(t, entries, cur, "inserted middle marker")
}

func TestDiffChunksDetectsDeletionInMiddle(t *testing.T) {
	text := variedText("line", 400)
	base := ChunkRecords(text)
	lines := strings.Split(text, "\n")
	delAt := len(lines) / 2
	cur := strings.Join(append(append([]string{}, lines[:delAt]...), lines[delAt+40:]...), "\n")
	entries := DiffChunks(base, cur)
	requireNonEmpty(t, entries, "a deleted block")
	for _, e := range entries {
		if e.StartLine < 1 || e.EndLine < e.StartLine {
			t.Fatalf("entry has an invalid position: %+v", e)
		}
	}
}

// TestDiffChunksDetectsMove is the case a set comparison cannot see: both
// blocks still exist with the same CIDs, only their order changed.
func TestDiffChunksDetectsMove(t *testing.T) {
	blockA := variedText("alpha", 150)
	blockB := variedText("beta", 150)
	base := ChunkRecords(blockA + "\n" + blockB)
	cur := blockB + "\n" + blockA
	entries := DiffChunks(base, cur)
	requireNonEmpty(t, entries, "a moved block")
}

func TestDiffChunksDetectsRewrite(t *testing.T) {
	base := ChunkRecords(variedText("old", 300))
	cur := variedText("new", 300)
	entries := DiffChunks(base, cur)
	requireNonEmpty(t, entries, "a rewrite")
}

func TestDiffChunksDetectsSingleByteEdit(t *testing.T) {
	text := variedText("line", 300)
	base := ChunkRecords(text)
	cur := strings.Replace(text, "line-0150", "line-0150-EDITED", 1)
	entries := DiffChunks(base, cur)
	requireNonEmpty(t, entries, "a single-line edit")
	requireEntryContaining(t, entries, cur, "line-0150-EDITED")
}

func TestChunkRecordsPositionsCoverFile(t *testing.T) {
	text := variedText("line", 500)
	fc := ChunkText(text)
	records := RecordsForChunks(fc)
	if len(records) < 2 {
		t.Fatalf("expected a multi-chunk fixture, got %d chunk(s)", len(records))
	}
	for i, r := range records {
		if r.StartLine < 1 || r.EndLine < r.StartLine {
			t.Fatalf("record %d has an invalid span: %+v", i, r)
		}
		if i > 0 && records[i-1].EndLine > r.StartLine {
			t.Fatalf("record %d overlaps its predecessor: %+v then %+v", i, records[i-1], r)
		}
	}
	last := records[len(records)-1]
	if got, want := last.EndLine, fc.LineCount(); got != want {
		t.Fatalf("last chunk ends at line %d, want %d", got, want)
	}
}

func TestDiffChunksEmptyBaseDetectsAllContent(t *testing.T) {
	cur := variedText("line", 40)
	entries := DiffChunks(nil, cur)
	requireNonEmpty(t, entries, "content against an empty baseline")
	if entries[0].Kind != ChangeAdded {
		t.Fatalf("expected an added entry, got %+v", entries[0])
	}
}

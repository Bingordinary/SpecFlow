package contenthash

import "testing"

// referenceChunkEnds computes CDC boundaries using the contract documented by
// rabinRolling.add: after the rolling state is reset at a chunk boundary, the
// new window is initially empty and must accumulate WindowSize bytes from the
// new chunk before any byte can roll out.
func referenceChunkEnds(text string) []int {
	text = normalizeForWindowResetTest(text)
	data := []byte(text)
	start := 0
	roll := rabinRolling{}
	var ends []int

	for i := 0; i < len(data); i++ {
		var h uint64
		rel := i - start
		if rel < WindowSize {
			h = roll.add(data[i], 0, false)
		} else {
			h = roll.add(data[i], data[i-WindowSize], true)
		}

		size := i - start + 1
		if size >= MinChunk && (h&boundaryMask(size) == 0 || size >= MaxChunk) {
			ends = append(ends, i+1)
			start = i + 1
			roll = rabinRolling{}
		}
	}
	if start < len(data) {
		ends = append(ends, len(data))
	}
	return ends
}

// normalizeForWindowResetTest mirrors the only normalization relevant to this
// fixture: it already uses LF, so only the required trailing newline matters.
func normalizeForWindowResetTest(text string) string {
	if text != "" && text[len(text)-1] != '\n' {
		return text + "\n"
	}
	return text
}

func TestChunkTextResetsRollingWindowAtChunkBoundary(t *testing.T) {
	// makeText is the package's existing deterministic CDC fixture (~14 KB)
	// and is guaranteed to produce multiple chunks.
	text := makeText("func A() { return 1 }", "func B() { return 2 }")
	gotChunks := ChunkText(text).Chunks
	wantEnds := referenceChunkEnds(text)

	if len(gotChunks) != len(wantEnds) {
		t.Fatalf("chunk count after window reset = %d, want %d", len(gotChunks), len(wantEnds))
	}
	for i := range gotChunks {
		if gotChunks[i].End != wantEnds[i] {
			t.Fatalf("chunk %d end after window reset = %d, want %d", i, gotChunks[i].End, wantEnds[i])
		}
	}
}

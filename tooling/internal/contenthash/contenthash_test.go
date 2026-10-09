package contenthash

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// makeText builds a text with two separated sections:
//   - a short header + "Section A" block
//   - a long non-repeating filler block (~11 KB) that keeps section B far
//     from section A (repeating filler would be a CDC pathology: periodic
//     content makes the rolling hash periodic and boundaries never fire)
//   - a short "Section B" block
//
// The filler guarantees that section A and section B land in different
// chunks, so range-mapping and boundary-stability tests have room to work.
func makeText(sectionA, sectionB string) string {
	var filler strings.Builder
	for i := 0; i < 240; i++ {
		fmt.Fprintf(&filler, "filler line %d with unique padding content to grow the chunk\n", i)
	}
	return "package demo\n\n// Section A\n" + sectionA + "\n\n" + filler.String() + "\n// Section B\n" + sectionB + "\n"
}

// lineOf returns the 1-based line number containing needle, or 0.
func lineOf(fc FileChunks, needle string) int {
	idx := strings.Index(fc.Text, needle)
	if idx < 0 {
		return 0
	}
	line := 1
	for i := 0; i < idx; i++ {
		if fc.Text[i] == '\n' {
			line++
		}
	}
	return line
}

// chunkContaining returns the chunk whose byte range overlaps needle.
func chunkContaining(fc FileChunks, needle string) Chunk {
	idx := strings.Index(fc.Text, needle)
	if idx < 0 {
		return Chunk{}
	}
	for _, c := range fc.Chunks {
		if idx >= c.Start && idx < c.End {
			return c
		}
	}
	return Chunk{}
}

func TestChunkTextDeterministic(t *testing.T) {
	text := makeText("func A() { return 1 }", "func B() { return 2 }")
	first := ChunkText(text)
	second := ChunkText(text)
	if len(first.Chunks) != len(second.Chunks) {
		t.Fatalf("chunk count differs across runs: %d vs %d", len(first.Chunks), len(second.Chunks))
	}
	for i := range first.Chunks {
		if first.Chunks[i].CID != second.Chunks[i].CID {
			t.Fatalf("chunk %d CID differs across runs: %s vs %s", i, first.Chunks[i].CID, second.Chunks[i].CID)
		}
	}
}

func TestChunkTextNormalizationConsistency(t *testing.T) {
	lF := makeText("func A() { return 1 }", "func B() { return 2 }")
	crlf := strings.ReplaceAll(lF, "\n", "\r\n")
	fcLF := ChunkText(lF)
	fcCRLF := ChunkText(crlf)
	if len(fcLF.Chunks) != len(fcCRLF.Chunks) {
		t.Fatalf("CRLF and LF chunk counts differ: %d vs %d", len(fcLF.Chunks), len(fcCRLF.Chunks))
	}
	for i := range fcLF.Chunks {
		if fcLF.Chunks[i].CID != fcCRLF.Chunks[i].CID {
			t.Fatalf("CRLF and LF chunk %d CID differ", i)
		}
	}
}

func TestChunkTextCoversWholeText(t *testing.T) {
	text := makeText("func A() { return 1 }", "func B() { return 2 }")
	fc := ChunkText(text)
	if len(fc.Chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if fc.Chunks[0].Start != 0 {
		t.Fatalf("first chunk should start at 0, got %d", fc.Chunks[0].Start)
	}
	if fc.Chunks[len(fc.Chunks)-1].End != len(fc.Text) {
		t.Fatalf("last chunk should end at %d, got %d", len(fc.Text), fc.Chunks[len(fc.Chunks)-1].End)
	}
	for i := 1; i < len(fc.Chunks); i++ {
		if fc.Chunks[i].Start != fc.Chunks[i-1].End {
			t.Fatalf("chunks not contiguous: %d ends at %d, next starts at %d", i-1, fc.Chunks[i-1].End, fc.Chunks[i].Start)
		}
	}
}

// TestChunkBoundaryStabilityFarFromEdit is the core CDC property: content
// far from an edit keeps its CID. Inserting lines into the middle re-aligns
// boundaries only near the edit; chunks beyond the re-alignment window are
// unchanged.
func TestChunkBoundaryStabilityFarFromEdit(t *testing.T) {
	base := makeText("func A() { return 1 }", "func B() { return 2 }")
	fcBase := ChunkText(base)

	// Insert 40 new lines right after "Section A" — far from section B.
	insertion := strings.Repeat("new unrelated line\n", 40)
	modified := strings.Replace(base, "// Section A", "// Section A\n"+insertion, 1)
	fcMod := ChunkText(modified)

	// The section B chunk must be identical after an unrelated insertion.
	secBBase := chunkContaining(fcBase, "func B()")
	secBMod := chunkContaining(fcMod, "func B()")
	if secBBase.CID != secBMod.CID {
		t.Fatalf("section B chunk changed after unrelated insertion: %s vs %s", secBBase.CID, secBMod.CID)
	}

	// Deleting lines far from section B keeps the section B chunk too.
	deleted := strings.Replace(base, insertionLines(), "", 1)
	fcDel := ChunkText(deleted)
	secBDel := chunkContaining(fcDel, "func B()")
	if secBBase.CID != secBDel.CID {
		t.Fatalf("section B chunk changed after unrelated deletion: %s vs %s", secBBase.CID, secBDel.CID)
	}
}

func insertionLines() string {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "filler line %d with unique padding content to grow the chunk\n", i)
	}
	return b.String()
}

func TestChunkContentChangeChangesCID(t *testing.T) {
	text := makeText("func A() { return 1 }", "func B() { return 2 }")
	changed := strings.Replace(text, "return 2", "return 3", 1)
	fcA := ChunkText(text)
	fcB := ChunkText(changed)

	chunkA := chunkContaining(fcA, "func B()")
	chunkB := chunkContaining(fcB, "func B()")
	if chunkA.CID == chunkB.CID {
		t.Fatal("content change must change the chunk CID")
	}
}

func TestEmptyText(t *testing.T) {
	fc := ChunkText("")
	if len(fc.Chunks) != 0 {
		t.Fatalf("empty text should produce no chunks, got %d", len(fc.Chunks))
	}
	fcLF := ChunkText("\n")
	if len(fcLF.Chunks) != 0 {
		t.Fatalf("newline-only text should produce no chunks, got %d", len(fcLF.Chunks))
	}
}

// bruteRollingHash computes the rolling hash directly from its definition:
// the last WindowSize bytes, newest byte at weight prime^0, oldest at
// prime^(WindowSize-1). This is the ground truth the incremental update
// must match.
func bruteRollingHash(data []byte, end int) uint64 {
	var h uint64
	for i := end - WindowSize + 1; i <= end; i++ {
		h = h*prime + buzhashTable[data[i]]
	}
	return h
}

// TestRollingHashMatchesBruteForce is the strict correctness check for the
// rolling-hash formula: at every position of a random stream, the
// incrementally maintained hash must equal a brute-force recomputation from
// the definition. The ChunkText boundary decisions are based on this hash,
// so a mismatch here would mean chunk boundaries are computed incorrectly.
func TestRollingHashMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	data := make([]byte, 5000)
	for i := range data {
		data[i] = byte(rng.Intn(256))
	}

	// Feed one long stream (no chunk reset) and compare at every position.
	roll := rabinRolling{}
	for i := 0; i < len(data); i++ {
		if i < WindowSize {
			roll.add(data[i], 0, false)
		} else {
			roll.add(data[i], data[i-WindowSize], true)
		}
		if i >= WindowSize-1 {
			got := roll.hash
			want := bruteRollingHash(data, i)
			if got != want {
				t.Fatalf("rolling hash mismatch at position %d: got %#x, want %#x", i, got, want)
			}
		}
	}
}

// TestRollingHashWindowProperty verifies the core window semantics directly:
// the hash at position i depends only on the last WindowSize bytes. A stream
// that differs from another only more than WindowSize bytes before i must
// produce the same hash at i.
func TestRollingHashWindowProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	base := make([]byte, 2000)
	for i := range base {
		base[i] = byte(rng.Intn(256))
	}
	// modified differs from base in the first byte only — far outside any
	// 64-byte window ending at position 1999.
	modified := append([]byte(nil), base...)
	modified[0] = base[0] ^ 0xFF

	feed := func(data []byte, end int) uint64 {
		roll := rabinRolling{}
		for i := 0; i <= end; i++ {
			if i < WindowSize {
				roll.add(data[i], 0, false)
			} else {
				roll.add(data[i], data[i-WindowSize], true)
			}
		}
		return roll.hash
	}

	if feed(base, 1999) != feed(modified, 1999) {
		t.Fatal("hash at a position must not depend on bytes older than WindowSize")
	}
	// But a change inside the window must change the hash.
	modified[1950] = base[1950] ^ 0xFF
	if feed(base, 1999) == feed(modified, 1999) {
		t.Fatal("hash at a position must change when bytes inside the window change")
	}
}

// TestCIDKnownVectors verifies the CID output against authoritative SHA-256
// test vectors (FIPS 180-2). This checks the "sha256:" prefix format and the
// hash value itself against externally known answers.
func TestCIDKnownVectors(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq", "sha256:248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"},
	}
	for _, c := range cases {
		if got := CID([]byte(c.input)); got != c.want {
			t.Errorf("CID(%q) = %s, want %s", c.input, got, c.want)
		}
	}
}

// Package contenthash provides content-defined chunking (CDC) for
// content-addressed freshness checks.
//
// A file's normalized text is split into content-defined chunks using a
// rolling buzhash. Each chunk gets a content identifier (CID) — the SHA-256
// of the chunk's bytes. CIDs are content identity, not position: a chunk's
// CID changes if and only if its content changes, regardless of where the
// content sits in the file. Inserting or deleting content only re-aligns
// chunk boundaries near the edit; content far from the edit keeps its CID.
//
// The agent declares which line ranges it actually depended on during a
// validate/verify/review run; CIDsForRanges maps those ranges onto the
// chunks they overlap. Only the resulting CIDs are recorded in the cache —
// line numbers and positions are never persisted.
package contenthash

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

const (
	// WindowSize is the rolling-hash window in bytes.
	WindowSize = 64
	// MinChunk is the smallest chunk allowed before a boundary may fire.
	MinChunk = 512
	// MaxChunk is a defensive ceiling. With the graduated mask below,
	// boundaries are content-driven in practice (probability grows with
	// chunk size), so this almost never fires. It must stay large: a
	// position-based forced cut would desynchronize the chunk sequence
	// after any insertion.
	MaxChunk = 65536
	// prime is an odd 64-bit prime used by the Rabin-style rolling hash.
	prime = 0x100000001b3
)

// boundaryMask returns the mask for the rolling-hash boundary test at a
// given chunk size. The mask grows with size, so the probability of a
// boundary rises as a chunk grows — chunk sizes stay near ~2 KB and the
// boundary stays content-driven at every size.
func boundaryMask(size int) uint64 {
	switch {
	case size < 4096:
		return 2047
	case size < 8192:
		return 4095
	case size < 16384:
		return 8191
	case size < 32768:
		return 16383
	default:
		return 32767
	}
}

// primePowers[k] = prime^k mod 2^64.
var primePowers [WindowSize]uint64

// buzhashTable maps each byte to a deterministic pseudo-random value.
// The table is derived from a fixed xorshift64 state so that chunk
// boundaries are stable across platforms and Go versions.
var buzhashTable [256]uint64

func init() {
	state := uint64(0x9E3779B97F4A7C15)
	for i := range buzhashTable {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		buzhashTable[i] = state
	}
	primePowers[0] = 1
	for i := 1; i < WindowSize; i++ {
		primePowers[i] = primePowers[i-1] * prime
	}
}

// Chunk is one content-defined chunk of a file.
type Chunk struct {
	Start int    // byte offset in the normalized text (inclusive)
	End   int    // byte offset in the normalized text (exclusive)
	CID   string // "sha256:<hex>" — content identity of the chunk
}

// FileChunks is the chunked representation of one normalized text.
type FileChunks struct {
	Text       string
	Chunks     []Chunk
	LineStarts []int // byte offset of each line start (0-based; includes the trailing empty line)
}

// LineCount returns the number of lines in the normalized text.
func (fc FileChunks) LineCount() int {
	if len(fc.LineStarts) == 0 {
		return 0
	}
	return len(fc.LineStarts) - 1
}

// CID computes the content identifier of a byte block.
func CID(block []byte) string {
	sum := sha256.Sum256(block)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// FileHashText computes the whole-file CID of normalized text.
func FileHashText(text string) string {
	return CID([]byte(text))
}

// FileText reads a file and returns its normalized content.
func FileText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return specpaths.NormalizeText(string(data)), nil
}

// rabinRolling is a Rabin-style rolling hash over a fixed-size byte window.
// It maintains h = Σ table[b_{i-k}] · prime^(WindowSize-1-k) mod 2^64, where
// b_i is the most recent byte (weight prime^0) and b_{i-WindowSize+1} is the
// oldest byte in the window (weight prime^(WindowSize-1)).
type rabinRolling struct {
	hash uint64
}

// add feeds one byte into the rolling hash. When hasOut is false the window
// is not full yet and the byte is only accumulated; when true, out is the
// byte leaving the window and the hash rolls forward. The returned value is
// the hash of the last WindowSize bytes (or of all bytes fed so far, before
// the window fills).
func (r *rabinRolling) add(b byte, out byte, hasOut bool) uint64 {
	if !hasOut {
		r.hash = r.hash*prime + buzhashTable[b]
		return r.hash
	}
	// Drop the outgoing byte (weight prime^(WindowSize-1)), shift the rest
	// by one prime power, add the new byte at weight prime^0.
	r.hash = (r.hash-buzhashTable[out]*primePowers[WindowSize-1])*prime + buzhashTable[b]
	return r.hash
}

// ChunkText splits text into content-defined chunks. The text is normalized
// (line endings, trailing newline) before chunking, so the same logical
// content always produces the same chunks regardless of CRLF/LF.
func ChunkText(text string) FileChunks {
	text = specpaths.NormalizeText(text)
	data := []byte(text)
	fc := FileChunks{Text: text, LineStarts: []int{0}}
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			fc.LineStarts = append(fc.LineStarts, i+1)
		}
	}

	start := 0
	roll := rabinRolling{}
	for i := 0; i < len(data); i++ {
		var h uint64
		if i < WindowSize {
			h = roll.add(data[i], 0, false)
		} else {
			h = roll.add(data[i], data[i-WindowSize], true)
		}
		size := i - start + 1
		if size >= MinChunk && (h&boundaryMask(size) == 0 || size >= MaxChunk) {
			fc.Chunks = append(fc.Chunks, Chunk{Start: start, End: i + 1, CID: CID(data[start : i+1])})
			start = i + 1
			roll = rabinRolling{}
		}
	}
	if start < len(data) {
		// A trailing chunk containing only the normalized trailing newline
		// of an otherwise empty text has no content — skip it.
		if start < len(data)-1 || data[start] != '\n' {
			fc.Chunks = append(fc.Chunks, Chunk{Start: start, End: len(data), CID: CID(data[start:])})
		}
	}
	return fc
}

// ChunkFile reads a file, normalizes it, and chunks it.
func ChunkFile(path string) (FileChunks, error) {
	text, err := FileText(path)
	if err != nil {
		return FileChunks{}, err
	}
	return ChunkText(text), nil
}

// ParseRanges parses a comma-separated list of inclusive line ranges,
// e.g. "120-180,300-320". Line numbers are 1-based. An empty string
// returns no ranges.
func ParseRanges(s string) ([][2]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out [][2]int
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		parts := strings.SplitN(tok, "-", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range %q: expected START-END", tok)
		}
		a, errA := strconv.Atoi(strings.TrimSpace(parts[0]))
		b, errB := strconv.Atoi(strings.TrimSpace(parts[1]))
		if errA != nil || errB != nil || a < 1 || b < 1 || a > b {
			return nil, fmt.Errorf("invalid range %q: expected START-END with 1 <= START <= END", tok)
		}
		out = append(out, [2]int{a, b})
	}
	return out, nil
}

// CIDsForRanges maps inclusive 1-based line ranges onto the chunks they
// overlap and returns the distinct chunk CIDs in file order. A range maps
// to whole chunks — the chunk boundary is the mechanical granularity line:
// content adjacent to the declared range inside the same chunk is included.
// Callers must ensure ranges are within the file's line count.
func CIDsForRanges(fc FileChunks, ranges [][2]int) []string {
	seen := make(map[string]bool)
	var out []string
	for _, r := range ranges {
		a, b := r[0], r[1]
		if a < 1 || b > fc.LineCount() || a > b {
			continue
		}
		startOff := fc.LineStarts[a-1]
		endOff := fc.LineStarts[b]
		for _, c := range fc.Chunks {
			if c.Start < endOff && c.End > startOff {
				if !seen[c.CID] {
					seen[c.CID] = true
					out = append(out, c.CID)
				}
			}
		}
	}
	return out
}

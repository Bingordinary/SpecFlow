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
		if i-start < WindowSize {
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

// AcceptanceItemsRegion locates the acceptance_item_set structural region of
// a spec's normalized text: from the exact marker line (a line whose trimmed
// form is exactly `acceptance_item_set:` — a prose mention of the marker
// elsewhere, even at line start, does not start the region) to the next
// top-level heading (a line starting with `#` and no leading whitespace) or
// the end of the text. Fenced code blocks (``` or ~~~) are content: a marker
// or heading line inside one does not start or end the region. The region is
// located by structure, not by line number, so inserting or deleting content
// elsewhere only moves the region — its content identity is preserved. ok is
// false when the marker is absent.
func AcceptanceItemsRegion(text string) (region string, ok bool) {
	lines := strings.Split(text, "\n")
	startIdx := -1
	fence := fenceTracker{}
	for i, line := range lines {
		if fence.active {
			fence.advance(line)
			continue
		}
		if strings.TrimSpace(line) == "acceptance_item_set:" {
			startIdx = i
			break
		}
		fence.advance(line)
	}
	if startIdx == -1 {
		return "", false
	}
	endIdx := len(lines)
	fence = fenceTracker{}
	for i := startIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if fence.active {
			fence.advance(line)
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.HasPrefix(line, "#") {
			endIdx = i
			break
		}
		fence.advance(line)
	}
	return strings.Join(lines[startIdx:endIdx], "\n"), true
}

// RegionCID computes the content identifier of a structural region's text.
func RegionCID(regionText string) string {
	return CID([]byte(regionText))
}

// SectionRegion is one section region of a file: the frontmatter region (the
// file head through the line before the first `##` heading) or one `##`
// heading section (the heading line through the line before the next `##`
// heading; `###` and deeper headings belong to their `##` section). The
// heading line itself is part of the region, so renaming a heading changes
// the region's content identity.
type SectionRegion struct {
	Heading string // "" for the frontmatter region; the heading text without the `## ` prefix otherwise
	Start   int    // 1-based line number of the region's first line (inclusive)
	End     int    // 1-based line number of the region's last line (inclusive)
	Text    string // the region content
}

// isSectionHeading reports whether a line is a `##` heading (a `##` prefix
// followed by heading text, excluding `###` and deeper headings).
func isSectionHeading(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "##") || strings.HasPrefix(t, "###") {
		return false
	}
	rest := strings.TrimPrefix(t, "##")
	return rest != "" && (rest[0] == ' ' || rest[0] == '\t')
}

// headingText extracts the heading text of a `##` heading line.
func headingText(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "##"))
}

// MalformedHeadingLines returns the lines that look like a broken `##`
// heading — trimmed text starting with `##` (not `###`) where the rest is
// empty or does not start with whitespace, e.g. `##x` or `##`. Such lines
// are content to the region splitter (SectionRegions never splits on them),
// but they usually mean the author intended a heading and got the format
// wrong. Lines inside fenced code blocks are content and never reported.
func MalformedHeadingLines(text string) []string {
	lines := strings.Split(text, "\n")
	fence := fenceTracker{}
	var malformed []string
	for _, line := range lines {
		if fence.active {
			fence.advance(line)
			continue
		}
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "##") && !strings.HasPrefix(t, "###") {
			rest := strings.TrimPrefix(t, "##")
			if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
				malformed = append(malformed, t)
			}
		}
		fence.advance(line)
	}
	return malformed
}

// fenceTracker tracks whether a line-by-line scan is inside a fenced code
// block (a ``` or ~~~ fence). Heading lines inside a fenced block are
// content, not structure — SectionRegions must not split on them.
type fenceTracker struct {
	active bool
	char   byte
	length int
}

// advance consumes one line and updates the fence state. Lines inside an
// active fence are skipped until a closing fence; a fence-start line outside
// an active fence opens one.
func (f *fenceTracker) advance(line string) {
	if f.active {
		if isClosingFence(line, f.char, f.length) {
			f.active = false
		}
		return
	}
	c, n, ok := fenceInfo(line)
	if ok {
		f.active = true
		f.char = c
		f.length = n
	}
}

// fenceInfo reports whether a line opens a fenced code block and, if so, the
// fence character and its run length. A fence is at least three backticks or
// tildes; a backtick fence's info string must not contain backticks.
func fenceInfo(line string) (char byte, length int, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" {
		return 0, 0, false
	}
	c := t[0]
	if c != '`' && c != '~' {
		return 0, 0, false
	}
	n := 0
	for n < len(t) && t[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	if c == '`' && strings.ContainsRune(t[n:], '`') {
		return 0, 0, false
	}
	return c, n, true
}

// isClosingFence reports whether a line closes a fence opened with the given
// character and run length (CommonMark closing semantics): the same character
// in a consecutive run of at least `length` characters, followed by nothing
// but whitespace — a longer run closes a shorter one (e.g. `~~~~` closes
// `~~~`), and the closing run cannot carry trailing content.
func isClosingFence(line string, char byte, length int) bool {
	t := strings.TrimSpace(line)
	n := 0
	for n < len(t) && t[n] == char {
		n++
	}
	if n < length {
		return false
	}
	for i := n; i < len(t); i++ {
		if t[i] != ' ' && t[i] != '\t' {
			return false
		}
	}
	return true
}

// SectionRegions splits normalized text into section regions: one frontmatter
// region (from the file start to the line before the first `##` heading) plus
// one region per `##` heading (from the heading line to the line before the
// next `##` heading, or the end of the text). A text with no `##` heading is
// a single frontmatter region. Regions are located by structure, not by line
// number, so inserting or deleting content elsewhere only moves a region —
// its content identity is preserved. Fenced code blocks (``` or ~~~) are
// content: heading lines inside them do not split regions.
func SectionRegions(text string) []SectionRegion {
	lines := strings.Split(text, "\n")
	first := -1
	fence := fenceTracker{}
	for i, line := range lines {
		if fence.active {
			fence.advance(line)
			continue
		}
		if isSectionHeading(line) {
			first = i
			break
		}
		fence.advance(line)
	}
	if first == -1 {
		return []SectionRegion{{Start: 1, End: len(lines), Text: text}}
	}
	regions := []SectionRegion{{Heading: "", Start: 1, End: first, Text: strings.Join(lines[:first], "\n")}}
	fence = fenceTracker{}
	for i := first; i < len(lines); {
		j := i + 1
		for j < len(lines) {
			if !fence.active && isSectionHeading(lines[j]) {
				break
			}
			fence.advance(lines[j])
			j++
		}
		regions = append(regions, SectionRegion{Heading: headingText(lines[i]), Start: i + 1, End: j, Text: strings.Join(lines[i:j], "\n")})
		i = j
	}
	return regions
}

// LocateSectionRegion locates the section region with the given heading text
// (without the `## ` prefix). ok is false when no section has that heading
// or when more than one section has it — duplicated headings fail closed.
func LocateSectionRegion(text, heading string) (SectionRegion, bool) {
	var found SectionRegion
	seen := false
	for _, r := range SectionRegions(text) {
		if r.Heading != heading {
			continue
		}
		if seen {
			return SectionRegion{}, false
		}
		found = r
		seen = true
	}
	if !seen {
		return SectionRegion{}, false
	}
	return found, true
}

// ListMissingDeps reports which declared dependencies no longer hold for the
// given normalized text. Chunk CIDs are matched against the text's current
// content-defined chunk set; structural region dependencies
// (`region:acceptance_items:<cid>`) and section region dependencies
// (`region:section:<heading>:<cid>`) are re-located by structure and compared
// by content identity. An unknown region type fails closed (reported
// missing). Missing dependencies are returned in declaration order.
func ListMissingDeps(text string, deps []string) []string {
	fc := ChunkText(text)
	present := make(map[string]bool, len(fc.Chunks))
	for _, c := range fc.Chunks {
		present[stripCIDPrefix(c.CID)] = true
	}
	var missing []string
	for _, dep := range deps {
		if strings.HasPrefix(dep, "region:") {
			rest := strings.TrimPrefix(dep, "region:")
			name, payload, found := strings.Cut(rest, ":")
			if !found {
				missing = append(missing, dep)
				continue
			}
			switch name {
			case "acceptance_items":
				region, ok := AcceptanceItemsRegion(text)
				if !ok || stripCIDPrefix(payload) != stripCIDPrefix(RegionCID(region)) {
					missing = append(missing, dep)
				}
			case "section":
				idx := strings.LastIndex(payload, ":sha256:")
				if idx < 0 {
					missing = append(missing, dep)
					continue
				}
				heading, cid := payload[:idx], payload[idx+1:]
				region, ok := LocateSectionRegion(text, heading)
				if !ok || stripCIDPrefix(cid) != stripCIDPrefix(RegionCID(region.Text)) {
					missing = append(missing, dep)
				}
			default:
				missing = append(missing, dep)
			}
			continue
		}
		if !present[stripCIDPrefix(dep)] {
			missing = append(missing, dep)
		}
	}
	return missing
}

// DepsPresent reports whether every declared dependency still holds for the
// given normalized text. Chunk CIDs are matched against the text's current
// content-defined chunk set; structural region dependencies
// (`region:<type>:<cid>`) are re-located by structure and compared by content
// identity, so edits outside the region do not invalidate them. An unknown
// region type fails closed. An empty deps list reports true.
func DepsPresent(text string, deps []string) bool {
	return len(ListMissingDeps(text, deps)) == 0
}

// stripCIDPrefix removes any algorithm prefix (e.g. "sha256:") from a stored
// or computed CID so it compares on the value alone.
func stripCIDPrefix(cid string) string {
	if idx := strings.LastIndex(cid, ":"); idx >= 0 {
		return cid[idx+1:]
	}
	return cid
}

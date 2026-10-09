// Ordered, bidirectional chunk diff — the detection half of the delta-review
// model (see the design: re* reviews the change set, not a mechanically
// derived check list).
//
// The cache records a file's ordered chunk sequence with positions
// (ChunkRecord). At delta-plan time the current content is chunked again and
// the two ordered sequences are aligned. Because the chunker is a
// deterministic function of content, two files with the same ordered CID
// sequence have the same content: any change — edit, insertion, deletion, or
// move — changes the sequence and therefore produces at least one change
// entry. Comparing sets instead of sequences would miss moves; checking only
// "baseline chunks still present" (the legacy one-directional check) would
// miss pure additions. Neither shortcut is used here.
package contenthash

import "sort"

// ChunkerVersion identifies the chunking algorithm and parameters (the
// rolling window, size bounds, and mask schedule in contenthash.go). Chunk
// evidence is only comparable across the same chunker: a version change
// invalidates cached chunk sequences and requires a full re-run.
const ChunkerVersion = "buzhash-v1"

// ChunkRecord is one persisted chunk of a file: its content identity plus
// its position in the normalized text's line space. Positions keep the
// baseline side of a diff readable ("content removed near lines X–Y")
// without persisting the baseline content.
type ChunkRecord struct {
	CID       string `json:"cid"`
	StartLine int    `json:"start"` // 1-based, inclusive
	EndLine   int    `json:"end"`   // 1-based, inclusive
}

// ChangeKind classifies one localized change.
type ChangeKind string

const (
	ChangeAdded   ChangeKind = "added"
	ChangeChanged ChangeKind = "changed"
	ChangeRemoved ChangeKind = "removed"
)

// ChangeEntry is one localized change in one file. For added and changed
// entries, StartLine/EndLine are current-file line numbers covering the new
// content. For removed entries they name the current-file gap the removed
// content sat between — the removed content itself no longer exists, so a
// gap position is the best a diff can offer without storing old content.
type ChangeEntry struct {
	Kind      ChangeKind `json:"kind"`
	StartLine int        `json:"start"`
	EndLine   int        `json:"end"`
	OldCIDs   []string   `json:"old_cids,omitempty"`
	NewCIDs   []string   `json:"new_cids,omitempty"`
}

// ChunkRecords returns the ordered chunk records (CID + line positions) of
// normalized text.
func ChunkRecords(text string) []ChunkRecord {
	return RecordsForChunks(ChunkText(text))
}

// RecordsForChunks converts an already chunked text into its persisted
// record form.
func RecordsForChunks(fc FileChunks) []ChunkRecord {
	records := make([]ChunkRecord, 0, len(fc.Chunks))
	for _, c := range fc.Chunks {
		records = append(records, ChunkRecord{
			CID:       c.CID,
			StartLine: lineOfOffset(fc, c.Start),
			EndLine:   lineOfOffset(fc, c.End-1),
		})
	}
	return records
}

// DiffChunks aligns a baseline ordered chunk sequence against the current
// content of the same file and returns the localized changes, ordered by
// position. An unchanged file returns no entries.
func DiffChunks(base []ChunkRecord, curText string) []ChangeEntry {
	return diffRecords(base, RecordsForChunks(ChunkText(curText)))
}

// lcsCellCap bounds the LCS table used for middle-segment alignment. Local
// edits are trimmed away before this point, so the cap only fires on
// rewrite-scale middles, where one coarse changed entry is the honest
// answer.
const lcsCellCap = 1 << 20

func diffRecords(base, cur []ChunkRecord) []ChangeEntry {
	// Trim the common prefix and suffix in linear time; the remaining
	// middle is what actually diverged.
	i0, j0 := 0, 0
	for i0 < len(base) && j0 < len(cur) && base[i0].CID == cur[j0].CID {
		i0++
		j0++
	}
	i1, j1 := len(base), len(cur)
	for i1 > i0 && j1 > j0 && base[i1-1].CID == cur[j1-1].CID {
		i1--
		j1--
	}
	mBase := base[i0:i1]
	mCur := cur[j0:j1]

	switch {
	case len(mBase) == 0 && len(mCur) == 0:
		return nil
	case len(mBase) == 0:
		return []ChangeEntry{{
			Kind:      ChangeAdded,
			StartLine: mCur[0].StartLine,
			EndLine:   mCur[len(mCur)-1].EndLine,
			NewCIDs:   cidsOf(mCur),
		}}
	case len(mCur) == 0:
		start, end := gapPosition(cur, j0)
		return []ChangeEntry{{
			Kind:      ChangeRemoved,
			StartLine: start,
			EndLine:   end,
			OldCIDs:   cidsOf(mBase),
		}}
	}
	if len(mBase)*len(mCur) > lcsCellCap {
		return []ChangeEntry{{
			Kind:      ChangeChanged,
			StartLine: mCur[0].StartLine,
			EndLine:   mCur[len(mCur)-1].EndLine,
			OldCIDs:   cidsOf(mBase),
			NewCIDs:   cidsOf(mCur),
		}}
	}
	return groupOps(mBase, mCur, lcsOps(mBase, mCur), cur, j0)
}

// lcsOp is one alignment step: a match consumes both sides, del and ins
// consume one side each.
type lcsOp struct {
	del bool
	ins bool
}

// lcsOps computes the LCS alignment of two chunk sequences. Ties prefer
// insertions so that a replaced region is reported as changed rather than
// as a separate removal and addition.
func lcsOps(base, cur []ChunkRecord) []lcsOp {
	n, m := len(base), len(cur)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			switch {
			case base[i-1].CID == cur[j-1].CID:
				dp[i][j] = dp[i-1][j-1] + 1
			case dp[i-1][j] >= dp[i][j-1]:
				dp[i][j] = dp[i-1][j]
			default:
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	rev := make([]lcsOp, 0, n+m)
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && base[i-1].CID == cur[j-1].CID:
			rev = append(rev, lcsOp{})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			rev = append(rev, lcsOp{ins: true})
			j--
		default:
			rev = append(rev, lcsOp{del: true})
			i--
		}
	}
	for l, r := 0, len(rev)-1; l < r; l, r = l+1, r-1 {
		rev[l], rev[r] = rev[r], rev[l]
	}
	return rev
}

// groupOps folds one alignment into per-position change entries: adjacent
// unmatched runs become added / removed / changed, and each run of matches
// closes the current entry.
func groupOps(base, cur []ChunkRecord, ops []lcsOp, fullCur []ChunkRecord, curStart int) []ChangeEntry {
	var entries []ChangeEntry
	bi, ci := 0, 0
	var delRun []string
	var insRun []ChunkRecord
	flush := func() {
		if len(delRun) == 0 && len(insRun) == 0 {
			return
		}
		if len(insRun) > 0 {
			kind := ChangeAdded
			if len(delRun) > 0 {
				kind = ChangeChanged
			}
			entries = append(entries, ChangeEntry{
				Kind:      kind,
				StartLine: insRun[0].StartLine,
				EndLine:   insRun[len(insRun)-1].EndLine,
				OldCIDs:   delRun,
				NewCIDs:   cidsOf(insRun),
			})
		} else {
			start, end := gapPosition(fullCur, curStart+ci)
			entries = append(entries, ChangeEntry{
				Kind:      ChangeRemoved,
				StartLine: start,
				EndLine:   end,
				OldCIDs:   delRun,
			})
		}
		delRun, insRun = nil, nil
	}
	for _, op := range ops {
		switch {
		case op.del:
			delRun = append(delRun, base[bi].CID)
			bi++
		case op.ins:
			insRun = append(insRun, cur[ci])
			ci++
		default:
			flush()
			bi++
			ci++
		}
	}
	flush()
	return entries
}

func cidsOf(records []ChunkRecord) []string {
	out := make([]string, 0, len(records))
	for _, r := range records {
		out = append(out, r.CID)
	}
	return out
}

// gapPosition names the current-file gap between the surviving chunks on
// either side of a removal at index idx.
func gapPosition(cur []ChunkRecord, idx int) (int, int) {
	switch {
	case len(cur) == 0:
		return 1, 1
	case idx <= 0:
		return 1, cur[0].StartLine
	case idx >= len(cur):
		last := cur[len(cur)-1].EndLine
		return last, last
	default:
		return cur[idx-1].EndLine, cur[idx].StartLine
	}
}

// lineOfOffset returns the 1-based line number containing byte offset off in
// the normalized text of fc.
func lineOfOffset(fc FileChunks, off int) int {
	idx := sort.Search(len(fc.LineStarts), func(i int) bool {
		return fc.LineStarts[i] > off
	})
	if idx == 0 {
		return 1
	}
	return idx
}

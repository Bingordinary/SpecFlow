package validationcache

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// ChangeReportEntry is one cache input file's state relative to the accepted
// cache snapshot.
type ChangeReportEntry struct {
	Path    string                    `json:"path"`
	Kind    string                    `json:"kind"` // changed | removed | added | inconsistent
	Changes []contenthash.ChangeEntry `json:"changes,omitempty"`
}

// ChangeReport is the mechanically detected change set of one gate cache's
// recorded input surface: what changed, disappeared, or was recorded with
// evidence that contradicts itself since the cache was written. It is the sole
// input to the delta reviewer's judgment; it decides nothing itself.
type ChangeReport struct {
	Entries []ChangeReportEntry `json:"entries"`
}

// Empty reports whether nothing in the recorded surface changed.
func (r *ChangeReport) Empty() bool { return r == nil || len(r.Entries) == 0 }

// Inconsistent lists recorded files whose entries contradict themselves: the
// whole-file hash says the content changed, yet the entry's own chunk sequence
// still matches the current content exactly, so the change cannot be
// localized. The entry's recorded evidence cannot be trusted as a change
// baseline, so a delta/repair run must refuse it and a full run must not carry
// the standing conclusions mechanically.
func (r *ChangeReport) Inconsistent() []string {
	var out []string
	for _, e := range r.Entries {
		if e.Kind == "inconsistent" {
			out = append(out, e.Path)
		}
	}
	return out
}

// Fingerprint is the stable identity of this change set. A delta review
// record binds to it: the same change set always produces the same value,
// and any further content change produces a different one.
func (r *ChangeReport) Fingerprint() (string, error) {
	payload, err := json.Marshal(struct {
		Chunker string              `json:"chunker"`
		Entries []ChangeReportEntry `json:"entries"`
	}{Chunker: contenthash.ChunkerVersion, Entries: r.Entries})
	if err != nil {
		return "", err
	}
	return contenthash.CID(payload), nil
}

// DeriveChangeReport compares one gate cache's recorded file evidence against
// current content. A file whose whole-file hash still matches is unchanged.
// Chunk evidence is required in every entry (the cache format), so a changed
// entry always has comparable evidence and is diffed (ordered, bidirectional).
// An entry whose hash says the content changed while its own chunk sequence
// still matches the current content has recorded evidence that contradicts
// itself: the change cannot be localized, so it is reported inconsistent and
// the caller falls back to the full command. A recorded path that no longer
// resolves or exists is reported removed.
func DeriveChangeReport(repoRoot, targetKind, targetName, command string) (*ChangeReport, error) {
	cachePath, err := cacheFilePath(repoRoot, targetKind, targetName, command+"_result.md")
	if err != nil {
		return nil, err
	}
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, err
	}
	report := &ChangeReport{}
	for _, entry := range cache.Files {
		fullPath := resolveEntryPath(repoRoot, entry.Path)
		if fullPath == "" {
			report.Entries = append(report.Entries, ChangeReportEntry{Path: entry.Path, Kind: "removed"})
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				report.Entries = append(report.Entries, ChangeReportEntry{Path: entry.Path, Kind: "removed"})
				continue
			}
			return nil, fmt.Errorf("read %s: %w", entry.Path, err)
		}
		text := specpaths.NormalizeText(string(data))
		if entry.Hash != "" && normalizeHash(entry.Hash) == normalizeHash(contenthash.FileHashText(text)) {
			continue
		}
		changes := contenthash.DiffChunks(entry.Chunks, text)
		if len(changes) == 0 {
			// The hash says the content changed, yet the entry's own chunk
			// sequence matches the current content exactly. A content-defined
			// chunker is a deterministic function of content, so this can only
			// mean the recorded evidence contradicts itself: the change is
			// known but not localizable. Never claim it is localized.
			report.Entries = append(report.Entries, ChangeReportEntry{Path: entry.Path, Kind: "inconsistent"})
			continue
		}
		report.Entries = append(report.Entries, ChangeReportEntry{Path: entry.Path, Kind: "changed", Changes: changes})
	}
	return report, nil
}

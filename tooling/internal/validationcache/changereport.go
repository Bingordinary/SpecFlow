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
	Kind    string                    `json:"kind"` // changed | removed | legacy
	Changes []contenthash.ChangeEntry `json:"changes,omitempty"`
}

// ChangeReport is the mechanically detected change set of one gate cache's
// recorded input surface: what changed, disappeared, or can no longer be
// localized since the cache was written. It is the sole input to the delta
// reviewer's judgment; it decides nothing itself.
type ChangeReport struct {
	Entries []ChangeReportEntry `json:"entries"`
}

// Empty reports whether nothing in the recorded surface changed.
func (r *ChangeReport) Empty() bool { return r == nil || len(r.Entries) == 0 }

// Legacy lists recorded files whose entries carry no comparable chunk
// evidence: the report can tell that they changed but not where, so a delta
// review cannot localize them — a complete run is required.
func (r *ChangeReport) Legacy() []string {
	var out []string
	for _, e := range r.Entries {
		if e.Kind == "legacy" {
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
// A changed file with comparable chunk evidence is diffed (ordered,
// bidirectional); changed content without comparable evidence is reported as
// legacy — the change is known but not localizable, so the run must complete.
// A recorded path that no longer resolves or exists is reported removed.
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
		if entry.Chunker != contenthash.ChunkerVersion || len(entry.Chunks) == 0 {
			report.Entries = append(report.Entries, ChangeReportEntry{Path: entry.Path, Kind: "legacy"})
			continue
		}
		changes := contenthash.DiffChunks(entry.Chunks, text)
		if len(changes) == 0 {
			// A hash mismatch with an identical chunk sequence means the
			// recorded evidence is inconsistent — fail closed to legacy
			// rather than claim the file is localized.
			report.Entries = append(report.Entries, ChangeReportEntry{Path: entry.Path, Kind: "legacy"})
			continue
		}
		report.Entries = append(report.Entries, ChangeReportEntry{Path: entry.Path, Kind: "changed", Changes: changes})
	}
	return report, nil
}

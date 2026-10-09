// Package baseline records the code-surface hash snapshot of a promoted
// target (unit or rule) and detects drift since promote. The baseline is a
// pure data snapshot written by promote and read by fresh — "drift" is never
// persisted, it is recomputed on every read.
package baseline

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

// Status classifies the drift state of a stable target's code surface.
type Status string

const (
	StatusOK      Status = "OK"
	StatusChanged Status = "CHANGED"
	StatusMissing Status = "MISSING"
)

// CheckResult describes the drift state of one target.
type CheckResult struct {
	Status  Status
	Details string
}

type entry struct {
	Path    string
	Hash    string
	Chunker string
	Chunks  []contenthash.ChunkRecord
}

type surface struct {
	Path    string
	Entries []entry
}

// baselinePath is the baseline file location for a target.
// Baselines live under docs/specs/meta/baseline/, parallel to the
// validation caches under docs/specs/meta/validation/.
func baselinePath(repoRoot, kind, name string) string {
	return filepath.Join(repoRoot, "docs/specs/meta/baseline", kind, name+".yaml")
}

// WriteUnitBaseline records the evidence snapshot of the code surface
// declared by the unit spec (implementation_surface + affects.files).
// Directories expand to their repository-content files so that later
// additions are detected as drift too. The <pending> placeholder is not a
// real surface and is skipped. Each entry records the whole-file hash and the
// ordered chunk sequence at promote time; drift detection compares them with
// the same ordered bidirectional chunk diff the gates use.
func WriteUnitBaseline(repoRoot, unitName, specContent string) error {
	path, data, err := PrepareUnitBaseline(repoRoot, unitName, specContent)
	if err != nil {
		return err
	}
	return writePrepared(path, data)
}

// PrepareUnitBaseline computes the publication record without writing it.
// Promote stages these bytes in the same transaction as accepted truth.
func PrepareUnitBaseline(repoRoot, unitName, specContent string) (string, []byte, error) {
	surfaces, err := collectSurfaces(repoRoot,
		specvalidation.ExtractImplementationSurfaces(specContent),
		specvalidation.ExtractAffectsFiles(specContent))
	if err != nil {
		return "", nil, err
	}
	if err := addChunkEvidence(repoRoot, surfaces); err != nil {
		return "", nil, err
	}
	return baselinePath(repoRoot, "unit", unitName), renderBaseline("unit", unitName, surfaces), nil
}

// WriteRuleBaseline records the stable rule file itself as the rule's
// observable surface (a rule declares no code surface). The hash is computed
// from the archived stable file, so it must be called after the rule commit.
func WriteRuleBaseline(repoRoot, ruleID string) error {
	stableRule := filepath.Join(repoRoot, "docs/specs/rules/stable", ruleID+".md")
	path, data, err := PrepareRuleBaseline(repoRoot, ruleID, stableRule)
	if err != nil {
		return err
	}
	return writePrepared(path, data)
}

// PrepareRuleBaseline hashes the artifact that will be published, while its
// recorded path is always the final stable path. No stable write is required.
func PrepareRuleBaseline(repoRoot, ruleID, artifactPath string) (string, []byte, error) {
	hash, err := specpaths.FileHash(artifactPath)
	if err != nil {
		return "", nil, err
	}
	text, err := contenthash.FileText(artifactPath)
	if err != nil {
		return "", nil, err
	}
	e := entry{Path: "docs/specs/rules/stable/" + ruleID + ".md", Hash: hash, Chunker: contenthash.ChunkerVersion, Chunks: contenthash.ChunkRecords(text)}
	s := surface{Path: e.Path, Entries: []entry{e}}
	return baselinePath(repoRoot, "rule", ruleID), renderBaseline("rule", ruleID, []surface{s}), nil
}

// RemoveBaseline deletes the baseline of a target (used when the target is
// retired). Removing an already-missing baseline is not an error.
func RemoveBaseline(repoRoot, kind, name string) error {
	path := baselinePath(repoRoot, kind, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

// CheckUnitBaseline compares the current code surface against the baseline
// recorded at promote time.
func CheckUnitBaseline(repoRoot, unitName string) CheckResult {
	return checkBaseline(repoRoot, "unit", unitName)
}

// CheckRuleBaseline compares the current rule file against the baseline
// recorded at promote time.
func CheckRuleBaseline(repoRoot, ruleID string) CheckResult {
	return checkBaseline(repoRoot, "rule", ruleID)
}

// ------------------------------------------------------------
// Surface collection
// ------------------------------------------------------------

func collectSurfaces(repoRoot string, surfacePaths, filePaths []string) ([]surface, error) {
	var surfaces []surface
	seen := make(map[string]bool)
	add := func(p string) error {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		if p == "" || p == "<pending>" || seen[p] {
			return nil
		}
		seen[p] = true
		full := filepath.Join(repoRoot, filepath.FromSlash(p))
		info, err := os.Stat(full)
		if err != nil {
			// Surface missing at promote time (defensive): record an empty
			// surface — a later check then reports any current files as added.
			surfaces = append(surfaces, surface{Path: p})
			return nil
		}
		if info.IsDir() {
			files, err := repofiles.ExpandDir(repoRoot, p)
			if err != nil {
				return err
			}
			var entries []entry
			for _, f := range files {
				entries = append(entries, entry{Path: f.Path, Hash: f.Hash})
			}
			surfaces = append(surfaces, surface{Path: p, Entries: entries})
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, full)
		hash, err := specpaths.FileHash(full)
		if err != nil {
			return nil
		}
		surfaces = append(surfaces, surface{Path: p, Entries: []entry{{Path: filepath.ToSlash(rel), Hash: hash}}})
		return nil
	}
	for _, p := range surfacePaths {
		if err := add(p); err != nil {
			return nil, err
		}
	}
	for _, p := range filePaths {
		if err := add(p); err != nil {
			return nil, err
		}
	}
	return surfaces, nil
}

// addChunkEvidence fills each surface entry with the ordered chunk sequence of
// its current content.
func addChunkEvidence(repoRoot string, surfaces []surface) error {
	for i := range surfaces {
		for j := range surfaces[i].Entries {
			full := filepath.Join(repoRoot, filepath.FromSlash(surfaces[i].Entries[j].Path))
			text, err := contenthash.FileText(full)
			if err != nil {
				return err
			}
			surfaces[i].Entries[j].Chunker = contenthash.ChunkerVersion
			surfaces[i].Entries[j].Chunks = contenthash.ChunkRecords(text)
		}
	}
	return nil
}

// ------------------------------------------------------------
// Serialization (YAML subset, dependency-free)
// ------------------------------------------------------------

func renderBaseline(kind, name string, surfaces []surface) []byte {
	var buf strings.Builder
	fmt.Fprintf(&buf, "kind: %s\n", kind)
	fmt.Fprintf(&buf, "name: %s\n", name)
	fmt.Fprintf(&buf, "timestamp: %s\n", time.Now().UTC().Format(time.RFC3339))
	buf.WriteString("surfaces:\n")
	for _, s := range surfaces {
		fmt.Fprintf(&buf, "  - path: %q\n", s.Path)
		if len(s.Entries) == 0 {
			continue
		}
		buf.WriteString("    entries:\n")
		for _, e := range s.Entries {
			fmt.Fprintf(&buf, "      - path: %q\n", e.Path)
			fmt.Fprintf(&buf, "        hash: %q\n", e.Hash)
			if e.Chunker != "" && len(e.Chunks) > 0 {
				fmt.Fprintf(&buf, "        chunker: %q\n", e.Chunker)
				buf.WriteString("        chunks:\n")
				for _, c := range e.Chunks {
					fmt.Fprintf(&buf, "          - cid: %q\n", c.CID)
					fmt.Fprintf(&buf, "            start: %d\n", c.StartLine)
					fmt.Fprintf(&buf, "            end: %d\n", c.EndLine)
				}
			}
		}
	}
	return []byte(buf.String())
}

func writePrepared(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

type parsedBaseline struct {
	kind     string
	name     string
	surfaces []surface
}

func readBaseline(path string) (*parsedBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b := &parsedBaseline{}
	var cur *surface
	var curEntry *entry
	inChunks := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "      - path:"):
			if cur == nil {
				return nil, fmt.Errorf("baseline entry outside a surface")
			}
			cur.Entries = append(cur.Entries, entry{Path: unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")))})
			curEntry = &cur.Entries[len(cur.Entries)-1]
			inChunks = false
		case strings.HasPrefix(line, "        hash:"):
			if curEntry != nil {
				curEntry.Hash = unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "hash:")))
			}
		case strings.HasPrefix(line, "        chunker:"):
			if curEntry != nil {
				curEntry.Chunker = unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "chunker:")))
			}
		case strings.HasPrefix(line, "        chunks:"):
			if curEntry != nil {
				inChunks = true
			}
		case strings.HasPrefix(trimmed, "- cid:"):
			if inChunks && curEntry != nil {
				curEntry.Chunks = append(curEntry.Chunks, contenthash.ChunkRecord{CID: unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- cid:")))})
			}
		case strings.HasPrefix(trimmed, "start:"):
			if inChunks && curEntry != nil && len(curEntry.Chunks) > 0 {
				n, _ := strconv.Atoi(unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "start:"))))
				curEntry.Chunks[len(curEntry.Chunks)-1].StartLine = n
			}
		case strings.HasPrefix(trimmed, "end:"):
			if inChunks && curEntry != nil && len(curEntry.Chunks) > 0 {
				n, _ := strconv.Atoi(unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "end:"))))
				curEntry.Chunks[len(curEntry.Chunks)-1].EndLine = n
			}
		case strings.HasPrefix(trimmed, "- path:"):
			b.surfaces = append(b.surfaces, surface{Path: unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")))})
			cur = &b.surfaces[len(b.surfaces)-1]
			curEntry = nil
			inChunks = false
		case strings.HasPrefix(trimmed, "kind:"):
			b.kind = strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
		case strings.HasPrefix(trimmed, "name:"):
			b.name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
		}
	}
	if b.kind == "" || b.name == "" {
		return nil, fmt.Errorf("baseline missing required fields (kind, name)")
	}
	return b, nil
}

func unquote(s string) string {
	return strings.Trim(s, `"'`)
}

// ------------------------------------------------------------
// Drift check
// ------------------------------------------------------------

func checkBaseline(repoRoot, kind, name string) CheckResult {
	path := baselinePath(repoRoot, kind, name)
	b, err := readBaseline(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{Status: StatusMissing, Details: "no baseline recorded (promoted before baseline support)"}
		}
		return CheckResult{Status: StatusChanged, Details: fmt.Sprintf("cannot read baseline: %v", err)}
	}

	var changedDetail []string
	var missing, added []string
	for _, s := range b.surfaces {
		full := filepath.Join(repoRoot, filepath.FromSlash(s.Path))
		info, err := os.Stat(full)
		if err != nil {
			for _, e := range s.Entries {
				missing = append(missing, e.Path)
			}
			continue
		}
		if info.IsDir() {
			baselineEntries := make(map[string]entry, len(s.Entries))
			for _, e := range s.Entries {
				baselineEntries[e.Path] = e
			}
			files, err := repofiles.ExpandDir(repoRoot, s.Path)
			if err != nil {
				return CheckResult{Status: StatusChanged, Details: fmt.Sprintf("cannot expand surface %q: %v", s.Path, err)}
			}
			currentPaths := make(map[string]bool, len(files))
			for _, f := range files {
				currentPaths[f.Path] = true
			}
			for p, be := range baselineEntries {
				if !currentPaths[p] {
					missing = append(missing, p)
					continue
				}
				if detail, drifted := entryDrift(repoRoot, p, be); drifted {
					changedDetail = append(changedDetail, detail)
				}
			}
			for _, f := range files {
				if _, ok := baselineEntries[f.Path]; !ok {
					added = append(added, f.Path)
				}
			}
			continue
		}
		// File surface
		if len(s.Entries) != 1 {
			changedDetail = append(changedDetail, s.Path+" (surface changed)")
			continue
		}
		e := s.Entries[0]
		if detail, drifted := entryDrift(repoRoot, e.Path, e); drifted {
			changedDetail = append(changedDetail, detail)
		}
	}

	sort.Strings(changedDetail)
	sort.Strings(missing)
	sort.Strings(added)
	changedDetail = dedupeStrings(changedDetail)
	missing = dedupeStrings(missing)
	added = dedupeStrings(added)

	if len(changedDetail) == 0 && len(missing) == 0 && len(added) == 0 {
		return CheckResult{Status: StatusOK, Details: "code surface matches the promote-time baseline"}
	}
	var parts []string
	if len(changedDetail) > 0 {
		parts = append(parts, "changed: "+strings.Join(changedDetail, ", "))
	}
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}
	if len(added) > 0 {
		parts = append(parts, "added: "+strings.Join(added, ", "))
	}
	return CheckResult{Status: StatusChanged, Details: strings.Join(parts, "; ") + " — code changed since promote, run verify against stable to confirm"}
}

// dedupeStrings collapses adjacent duplicates (inputs are sorted).
func dedupeStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// entryDrift compares one baseline entry against current content. With a
// recorded chunk sequence the comparison is the same ordered bidirectional
// chunk diff the gates use, and the detail names the changed line spans. A
// legacy baseline entry without chunks falls back to whole-file hash equality.
func entryDrift(repoRoot, rel string, be entry) (string, bool) {
	full := filepath.Join(repoRoot, filepath.FromSlash(rel))
	text, err := contenthash.FileText(full)
	if err != nil {
		return be.Path + " (unreadable)", true
	}
	if len(be.Chunks) > 0 {
		changes := contenthash.DiffChunks(be.Chunks, text)
		if len(changes) == 0 {
			return "", false
		}
		var spans []string
		for _, c := range changes {
			switch c.Kind {
			case contenthash.ChangeAdded:
				spans = append(spans, fmt.Sprintf("added lines %d-%d", c.StartLine, c.EndLine))
			case contenthash.ChangeChanged:
				spans = append(spans, fmt.Sprintf("changed lines %d-%d", c.StartLine, c.EndLine))
			case contenthash.ChangeRemoved:
				if c.StartLine == c.EndLine {
					spans = append(spans, fmt.Sprintf("removed near line %d", c.StartLine))
				} else {
					spans = append(spans, fmt.Sprintf("removed between lines %d-%d", c.StartLine, c.EndLine))
				}
			}
		}
		return be.Path + " (" + strings.Join(spans, "; ") + ")", true
	}
	currentHash := contenthash.FileHashText(text)
	if be.Hash != "" && normalizeCID(currentHash) != normalizeCID(be.Hash) {
		return be.Path + " (content changed; no localization recorded)", true
	}
	return "", false
}

// normalizeCID strips a "sha256:" prefix so stored and computed CIDs compare
// on the hex value alone.
func normalizeCID(cid string) string {
	if idx := strings.LastIndex(cid, ":"); idx >= 0 {
		return cid[idx+1:]
	}
	return cid
}

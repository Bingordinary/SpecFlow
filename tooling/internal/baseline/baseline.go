// Package baseline records the code-surface hash snapshot of a promoted
// target (unit or rule) and detects drift since promote. The baseline is a
// pure data snapshot written by promote and read by fresh — "drift" is never
// persisted, it is recomputed on every read.
package baseline

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
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
	Note    string
}

type entry struct {
	Path string
	Hash string
	Deps []string
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

// WriteUnitBaseline records the hash snapshot of the code surface declared by
// the unit spec (implementation_surface + affects.files). Directories are
// expanded recursively so that later additions are detected as drift too.
// The <pending> placeholder is not a real surface and is skipped. verifyDeps
// carries the dependency chunk CIDs declared by the promote-time verify run
// (path -> deps, keys in canonical repo-relative slash form as returned by
// ReadVerifyDeps): files with declared dependencies are judged on chunk
// existence, files without them on the whole-file hash.
func WriteUnitBaseline(repoRoot, unitName, specContent string, verifyDeps map[string][]string) error {
	surfaces := collectSurfaces(repoRoot,
		specvalidation.ExtractImplementationSurfaces(specContent),
		specvalidation.ExtractAffectsFiles(specContent))
	for i := range surfaces {
		for j := range surfaces[i].Entries {
			if deps, ok := verifyDeps[surfaces[i].Entries[j].Path]; ok {
				surfaces[i].Entries[j].Deps = deps
			}
		}
	}
	return writeBaseline(repoRoot, "unit", unitName, surfaces)
}

// WriteRuleBaseline records the stable rule file itself as the rule's
// observable surface (a rule declares no code surface). The hash is computed
// from the archived stable file, so it must be called after the rule commit.
func WriteRuleBaseline(repoRoot, ruleID string) error {
	stableRule := filepath.Join(repoRoot, "docs/specs/rules/stable", ruleID+".md")
	hash, err := specpaths.FileHash(stableRule)
	if err != nil {
		return err
	}
	s := surface{Path: "docs/specs/rules/stable/" + ruleID + ".md", Entries: []entry{{Path: "docs/specs/rules/stable/" + ruleID + ".md", Hash: hash}}}
	return writeBaseline(repoRoot, "rule", ruleID, []surface{s})
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

func collectSurfaces(repoRoot string, surfacePaths, filePaths []string) []surface {
	var surfaces []surface
	seen := make(map[string]bool)
	add := func(p string) {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		if p == "" || p == "<pending>" || seen[p] {
			return
		}
		seen[p] = true
		full := filepath.Join(repoRoot, filepath.FromSlash(p))
		info, err := os.Stat(full)
		if err != nil {
			// Surface missing at promote time (defensive): record an empty
			// surface — a later check then reports any current files as added.
			surfaces = append(surfaces, surface{Path: p})
			return
		}
		if info.IsDir() {
			surfaces = append(surfaces, surface{Path: p, Entries: expandDir(repoRoot, full)})
			return
		}
		rel, _ := filepath.Rel(repoRoot, full)
		hash, err := specpaths.FileHash(full)
		if err != nil {
			return
		}
		surfaces = append(surfaces, surface{Path: p, Entries: []entry{{Path: filepath.ToSlash(rel), Hash: hash}}})
	}
	for _, p := range surfacePaths {
		add(p)
	}
	for _, p := range filePaths {
		add(p)
	}
	return surfaces
}

// expandDir lists every file under dir with its hash, sorted by path.
func expandDir(repoRoot, dir string) []entry {
	var entries []entry
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		hash, err := specpaths.FileHash(path)
		if err != nil {
			return nil
		}
		entries = append(entries, entry{Path: filepath.ToSlash(rel), Hash: hash})
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

// ------------------------------------------------------------
// Serialization (YAML subset, dependency-free)
// ------------------------------------------------------------

func writeBaseline(repoRoot, kind, name string, surfaces []surface) error {
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
			if len(e.Deps) > 0 {
				buf.WriteString("        deps:\n")
				for _, d := range e.Deps {
					fmt.Fprintf(&buf, "          - %q\n", d)
				}
			}
		}
	}
	path := baselinePath(repoRoot, kind, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0644)
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
	inDeps := false
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
			inDeps = false
		case strings.HasPrefix(line, "        hash:"):
			if curEntry != nil {
				curEntry.Hash = unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "hash:")))
			}
		case strings.HasPrefix(line, "        deps:"):
			if curEntry != nil {
				inDeps = true
			}
		case strings.HasPrefix(line, "          - "):
			if inDeps && curEntry != nil {
				curEntry.Deps = append(curEntry.Deps, unquote(strings.TrimSpace(strings.TrimPrefix(line, "          - "))))
			}
		case strings.HasPrefix(trimmed, "- path:"):
			b.surfaces = append(b.surfaces, surface{Path: unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "- path:")))})
			cur = &b.surfaces[len(b.surfaces)-1]
			curEntry = nil
			inDeps = false
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

	var changed, missing, added, notes []string
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
			currentEntries := make(map[string]string)
			for _, e := range expandDir(repoRoot, full) {
				currentEntries[e.Path] = e.Hash
			}
			for p, be := range baselineEntries {
				ce, ok := currentEntries[p]
				if !ok {
					missing = append(missing, p)
					continue
				}
				if len(be.Deps) > 0 {
					ok, note := depsPresent(filepath.Join(repoRoot, filepath.FromSlash(p)), be)
					if !ok {
						changed = append(changed, p)
					} else if note != "" {
						notes = append(notes, note)
					}
					continue
				}
				if ce != be.Hash {
					changed = append(changed, p)
				}
			}
			for p := range currentEntries {
				if _, ok := baselineEntries[p]; !ok {
					added = append(added, p)
				}
			}
			continue
		}
		// File surface
		if len(s.Entries) != 1 {
			changed = append(changed, s.Path)
			continue
		}
		e := s.Entries[0]
		if len(e.Deps) > 0 {
			ok, note := depsPresent(full, e)
			if !ok {
				changed = append(changed, e.Path)
			} else if note != "" {
				notes = append(notes, note)
			}
			continue
		}
		currentHash, err := specpaths.FileHash(full)
		if err != nil {
			missing = append(missing, e.Path)
			continue
		}
		if currentHash != e.Hash {
			changed = append(changed, e.Path)
		}
	}

	sort.Strings(changed)
	sort.Strings(missing)
	sort.Strings(added)
	sort.Strings(notes)

	if len(changed) == 0 && len(missing) == 0 && len(added) == 0 {
		if len(notes) > 0 {
			return CheckResult{
				Status:  StatusOK,
				Details: "code surface matches the promote-time baseline (outside the declared dependencies)",
				Note:    strings.Join(notes, "; "),
			}
		}
		return CheckResult{Status: StatusOK, Details: "code surface matches the promote-time baseline"}
	}
	var parts []string
	if len(changed) > 0 {
		parts = append(parts, "changed: "+strings.Join(changed, ", "))
	}
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}
	if len(added) > 0 {
		parts = append(parts, "added: "+strings.Join(added, ", "))
	}
	return CheckResult{Status: StatusChanged, Details: strings.Join(parts, "; ") + " — code changed since promote, run verify against stable to confirm"}
}

// depsPresent reports whether every declared dependency CID still exists in
// the file's current content. Chunk CIDs are matched against the chunk set;
// structural region dependencies (`region:<type>:<cid>`, e.g. a dependency
// unit's acceptance_item_set) are re-located by structure and compared by
// content identity. ok=false means a declared dependency is gone — the file
// drifted. ok=true with a non-empty note means the content changed outside
// the declared dependencies: informational only, the file is still considered
// conforming (mirrors the cache freshness note semantics in
// framework/validation_cache.md).
func depsPresent(full string, e entry) (bool, string) {
	data, err := os.ReadFile(full)
	if err != nil {
		return false, ""
	}
	text := specpaths.NormalizeText(string(data))

	if !contenthash.DepsPresent(text, e.Deps) {
		return false, ""
	}

	currentHash := contenthash.FileHashText(text)
	if e.Hash != "" && normalizeCID(currentHash) != normalizeCID(e.Hash) {
		return true, fmt.Sprintf("%s: content changed outside declared dependencies — re-verify if semantic coupling exists", e.Path)
	}
	return true, ""
}

// normalizeCID strips a "sha256:" prefix so stored and computed CIDs compare
// on the hex value alone.
func normalizeCID(cid string) string {
	if idx := strings.LastIndex(cid, ":"); idx >= 0 {
		return cid[idx+1:]
	}
	return cid
}

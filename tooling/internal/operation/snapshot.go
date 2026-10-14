package operation

import (
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
)

// workingTreeSnapshot records the operation's comparison point: every
// repository-content file of the working tree under Git's content definition
// (tracked, or untracked and not ignored — repofiles), with the hash of its
// content. The tooling's own process state and derived caches are excluded:
// they are not operation deliverables and change for reasons that belong to no
// operation.
//
// The result is sorted by path and is the value persisted in the state file,
// so a later check can ask what changed since the operation opened rather than
// what differs from a commit that predates it.
func workingTreeSnapshot(repoRoot string) ([]SnapshotEntry, error) {
	files, err := repofiles.ExpandDir(repoRoot, ".")
	if err != nil {
		return nil, err
	}
	entries := make([]SnapshotEntry, 0, len(files))
	for _, file := range files {
		if excluded(file.Path) {
			continue
		}
		entries = append(entries, SnapshotEntry{Path: file.Path, Hash: file.Hash})
	}
	return entries, nil
}

// changedSinceSnapshot computes the operation-relative change set: the paths
// that appeared, disappeared, or changed content since the snapshot was
// recorded. Changes that were already present when the snapshot was taken are
// not the operation's changes and are not reported.
func changedSinceSnapshot(repoRoot string, snapshot []SnapshotEntry) ([]string, error) {
	current, err := workingTreeSnapshot(repoRoot)
	if err != nil {
		return nil, err
	}
	before := make(map[string]string, len(snapshot))
	for _, entry := range snapshot {
		before[entry.Path] = entry.Hash
	}
	after := make(map[string]string, len(current))
	var changed []string
	for _, entry := range current {
		after[entry.Path] = entry.Hash
		if hash, ok := before[entry.Path]; !ok || hash != entry.Hash {
			changed = append(changed, entry.Path)
		}
	}
	for _, entry := range snapshot {
		if _, ok := after[entry.Path]; !ok {
			changed = append(changed, entry.Path)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// excluded reports whether a repository-relative path is outside the
// operation's change set by construction: the tooling's local process state
// (meta/) and its derived caches (docs/specs/meta/) are not operation
// deliverables (tooling/README.md §Operation scope).
func excluded(p string) bool {
	return p == "meta" || strings.HasPrefix(p, "meta/") ||
		p == "docs/specs/meta" || strings.HasPrefix(p, "docs/specs/meta/")
}

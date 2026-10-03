package gaterun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
)

// RemovalInvalidations prepares existing run-state updates without writing.
// The caller holds WithMutation through its deletion transaction. Consumed
// runs and immutable public evidence remain history.
func RemovalInvalidations(root string, deleted, targets map[string]bool) (map[string][]byte, error) {
	if _, err := repopath.Canonical(root, runStateDir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(runStateDir)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	writes := map[string][]byte{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		relPath, err := repopath.Canonical(root, runStateDir+"/"+entry.Name()+"/run.json")
		if err != nil {
			return nil, err
		}
		path := filepath.Join(root, filepath.FromSlash(relPath))
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read gate state %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("gate state %s must be a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read gate state %s: %w", path, err)
		}
		var run Run
		if err := json.Unmarshal(data, &run); err != nil {
			return nil, fmt.Errorf("parse gate state %s: %w", path, err)
		}
		if run.Status != StatusOpen {
			continue
		}
		affected := targets[run.TargetKind+":"+run.TargetName+":"+run.Target]
		for _, ref := range run.Refs {
			if deleted[ref.Ref] || deleted[ref.Resolved] {
				affected = true
			}
		}
		if !affected {
			continue
		}
		run.Status = StatusInvalidated
		run.Notices = append(run.Notices, "spec artifacts removed — plan a new run against the remaining spec")
		data, err = json.MarshalIndent(run, "", "  ")
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		writes[filepath.ToSlash(rel)] = append(data, '\n')
	}
	return writes, nil
}

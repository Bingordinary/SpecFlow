package reader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSnapshotDiscoversSpecFiles(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(candidateDir, 0755)
	os.MkdirAll(stableDir, 0755)

	candidateContent := `---
id: auth
layer: candidate
version: 1.0.0
unit_refs: none
rule_refs: none
---
# Auth
`
	stableContent := `---
id: auth
layer: stable
version: 0.9.0
unit_refs: none
rule_refs: none
---
# Auth (stable)
`
	os.WriteFile(filepath.Join(candidateDir, "c_unit_auth.md"), []byte(candidateContent), 0644)
	os.WriteFile(filepath.Join(stableDir, "s_unit_auth.md"), []byte(stableContent), 0644)

	snapshot := BuildSnapshot(repoRoot)

	if len(snapshot.Objects) == 0 {
		t.Fatal("expected at least one object")
	}

	found := false
	for _, obj := range snapshot.Objects {
		if obj.ID == "auth" {
			found = true
			if !obj.HasCandidate {
				t.Error("expected HasCandidate=true for auth")
			}
			if !obj.HasStable {
				t.Error("expected HasStable=true for auth")
			}
	if len(obj.TruthPaths) == 0 {
		t.Error("expected at least 1 truth path")
	}
			break
		}
	}
	if !found {
		t.Fatal("auth object not found in snapshot")
	}

	if snapshot.Project.UnitCount < 1 {
		t.Error("expected UnitCount >= 1")
	}
}

func TestBuildSnapshotFindsFilesystemObjects(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	content := `---
id: demo
layer: candidate
version: 1.0.0
unit_refs: none
rule_refs: none
---
# Demo
`
	os.WriteFile(filepath.Join(candidateDir, "c_unit_demo.md"), []byte(content), 0644)

	snapshot := BuildSnapshot(repoRoot)

	found := false
	for _, obj := range snapshot.Objects {
		if obj.ID == "demo" {
			found = true
			if !obj.HasCandidate {
				t.Error("expected HasCandidate=true")
			}
			if obj.HasStable {
				t.Error("expected HasStable=false (no stable file)")
			}
			break
		}
	}
	if !found {
		t.Fatal("demo object not found")
	}

	if !hasTruthPath(snapshot.Sources, "c_unit_demo.md") {
		t.Error("expected c_unit_demo.md in sources")
	}
}

func TestBuildSnapshotGraphHasNodes(t *testing.T) {
	repoRoot := t.TempDir()

	candidateDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(candidateDir, 0755)

	content := `---
id: graph_test
layer: candidate
version: 1.0.0
unit_refs: none
rule_refs: none
---
# Graph Test
`
	os.WriteFile(filepath.Join(candidateDir, "c_unit_graph_test.md"), []byte(content), 0644)

	snapshot := BuildSnapshot(repoRoot)

	if len(snapshot.Nodes) == 0 {
		t.Fatal("expected at least one graph node")
	}
	if len(snapshot.Edges) == 0 {
		t.Log("no graph edges (expected for isolated unit)")
	}
}

func hasTruthPath(sources []SourceRef, suffix string) bool {
	for _, s := range sources {
		if strings.HasSuffix(s.Path, suffix) {
			return true
		}
	}
	return false
}

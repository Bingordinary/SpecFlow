package entrysync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectOnlyReadsRegisteredEntrySection(t *testing.T) {
	repoRoot := t.TempDir()

	registryDir := filepath.Join(repoRoot, "specflow/framework/docs/agent_guidelines")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("mkdir registry dir: %v", err)
	}
	registry := `# Entry Index Registry

## Registered Entry Index Files

- ` + "`AGENTS.md`" + `
- ` + "`GEMINI.md`" + `
- ` + "`CLAUDE.md`" + `

## Hook Trigger

- ` + "`git config core.hooksPath .githooks`" + `
`
	if err := os.WriteFile(filepath.Join(registryDir, "entry_index_registry.md"), []byte(registry), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	block := "<!-- SPECFLOW:BEGIN -->\nmanaged\n<!-- SPECFLOW:END -->\n"
	for _, name := range []string{"AGENTS.md", "GEMINI.md", "CLAUDE.md"} {
		if err := os.WriteFile(filepath.Join(repoRoot, name), []byte(block), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	inspection, err := Inspect(repoRoot)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !inspection.Consistent {
		t.Fatalf("expected inspection to be consistent")
	}
	if len(inspection.RegisteredFiles) != 3 {
		t.Fatalf("expected 3 registered files, got %d: %v", len(inspection.RegisteredFiles), inspection.RegisteredFiles)
	}
}

package gaterun

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
)

// legacyCodeEvidence is the pre-shared-corpus derivation, kept verbatim as the
// test's reference: evidence for a file is derived from a corpus of the
// extension-filtered repository plus the file itself, with a per-file
// expansion and read. The shared evidenceCorpus must produce identical output
// for every file of a run.
func legacyCodeEvidence(t *testing.T, root, file string, extra []string) []string {
	t.Helper()
	files, err := repofiles.ExpandDir(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]string{}
	for _, f := range files {
		if strings.HasPrefix(f.Path, "docs/specs/") || strings.HasPrefix(f.Path, "specflow/") || strings.HasPrefix(f.Path, "meta/") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Path))
		if f.Path != file && !strings.Contains(codeEvidenceExts, "|"+ext+"|") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.IndexByte(string(data), 0) >= 0 {
			continue
		}
		texts[f.Path] = string(data)
	}
	seen := map[string]bool{file: true}
	queue := []string{file}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for p, text := range texts {
			if seen[p] {
				continue
			}
			base := filepath.Base(current)
			stem := strings.TrimSuffix(base, filepath.Ext(base))
			other := filepath.Base(p)
			otherStem := strings.TrimSuffix(other, filepath.Ext(other))
			linked := strings.Contains(text, base) || len(stem) > 2 && strings.Contains(text, stem) || strings.Contains(texts[current], other) || len(otherStem) > 2 && strings.Contains(texts[current], otherStem)
			if linked {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	rules, err := globalRuleIDs(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range rules {
		seen["rule:"+id] = true
	}
	for _, p := range extra {
		if !strings.HasPrefix(p, "docs/specs/units/") && !strings.HasPrefix(p, "unit:") {
			seen[p] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// TestEvidenceCorpusMatchesPerFileDerivation pins the work-sharing contract of
// loadEvidenceCorpus: hoisting the repository expansion and corpus read out of
// the per-file loop must not change any file's public evidence set, including
// the corner cases — a quality file with a filtered-out extension (visible to
// its own derivation, invisible to the others'), a NUL-byte file, and the
// governance-tree exclusion.
func TestEvidenceCorpusMatchesPerFileDerivation(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/alpha.go", "package src\n\n// uses beta.go and the config.json surface.\n")
	writeFile(t, repoRoot, "src/beta.go", "package src\n")
	writeFile(t, repoRoot, "src/null.go", "package src\n\x00\n")
	writeFile(t, repoRoot, "src/config.json", "{}\n")
	writeFile(t, repoRoot, "notes/schema.txt", "schema notes referencing alpha\n")
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", "---\nid: auth\n---\n")
	writeFile(t, repoRoot, "docs/specs/rules/stable/g_rule_tls.md", "---\nid: g_rule_tls\n---\n")

	files := []string{"src/alpha.go", "src/beta.go", "notes/schema.txt", "docs/specs/gone.md"}
	derivation, err := NewDerivation(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := derivation.loadEvidenceCorpus(files)
	if err != nil {
		t.Fatal(err)
	}
	extra := []string{"tools/gen.py"}
	for _, file := range files {
		got := corpus.evidence(file, extra)
		want := legacyCodeEvidence(t, repoRoot, file, extra)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("evidence drift for %q:\n shared corpus: %v\n per-file:     %v", file, got, want)
		}
	}

	// The NUL-byte file is excluded from the corpus: no derivation may see it.
	for _, p := range corpus.evidence("src/alpha.go", nil) {
		if p == "src/null.go" {
			t.Fatalf("NUL-byte file leaked into evidence: %v", corpus.evidence("src/alpha.go", nil))
		}
	}
	// The governance tree never participates in the corpus: no file's
	// evidence may reach a governance file (the degenerate target-itself
	// entry is the only allowed occurrence).
	for _, file := range files {
		for _, p := range corpus.evidence(file, nil) {
			if strings.HasPrefix(p, "docs/specs/") && p != file {
				t.Fatalf("governance file %q leaked into evidence for %q", p, file)
			}
		}
	}
}

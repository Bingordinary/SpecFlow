package gaterun

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestEvidenceCorpusDerivation pins the public evidence surface derivation:
// the file itself, the files that reference it, and the files it references —
// one hop in each direction, never a transitive closure. References are
// identifier-level (case-folded name tokens or explicit path forms); data
// files link only by path form; framework deployment artifacts and governance
// files never participate; global rules always do.
func TestEvidenceCorpusDerivation(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/logger.go", "package src\n\n// Logger writes logs.\n")
	writeFile(t, repoRoot, "src/factory.go", "package src\n\nfunc NewLogger() {}\n")
	writeFile(t, repoRoot, "src/service.ts", "export class Logger {}\n")
	writeFile(t, repoRoot, "src/blogger.go", "package src\n\n// blogger prose only.\n")
	writeFile(t, repoRoot, "src/model.go", "package src\n")
	writeFile(t, repoRoot, "src/remodel.go", "package src\n\n// remodel process\n")
	writeFile(t, repoRoot, "src/user_auth.go", "package src\n")
	writeFile(t, repoRoot, "src/UserAuthService.ts", "export class UserAuthService {}\n")
	writeFile(t, repoRoot, "src/user_authorization.go", "package src\n\n// user authorization\n")
	writeFile(t, repoRoot, "src/config.json", "{\"logger\": true}\n")
	writeFile(t, repoRoot, "src/loader.go", "package src\n\n// loads src/config.json\n")
	writeFile(t, repoRoot, "src/v2.go", "package src\n")
	writeFile(t, repoRoot, "src/pin.go", "package src\n\n// pins v2.go\n")
	writeFile(t, repoRoot, "src/myv2.go", "package src\n\n// xv2.go in prose\n")
	writeFile(t, repoRoot, "src/chain_a.go", "package src\n\n// uses chain_b.go\n")
	writeFile(t, repoRoot, "src/chain_b.go", "package src\n\n// uses chain_c.go\n")
	writeFile(t, repoRoot, "src/chain_c.go", "package src\n")
	writeFile(t, repoRoot, "src/alpha.go", "package src\n")
	writeFile(t, repoRoot, "notes/schema.txt", "schema notes referencing alpha\n")
	writeFile(t, repoRoot, "src/null.go", "package src\n\x00\n")
	writeFile(t, repoRoot, ".opencode/plugins/specflow.js", "// logger plugin\n")
	writeFile(t, repoRoot, ".claude-plugin/plugin.json", "{\"logger\": true}\n")
	writeFile(t, repoRoot, "docs/specs/units/candidate/unit_auth.md", "---\nid: auth\n---\n")
	writeFile(t, repoRoot, "docs/specs/rules/stable/g_rule_tls.md", "---\nid: g_rule_tls\nscope: unit\n---\n")

	derivation, err := NewDerivation(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := derivation.loadEvidenceCorpus([]string{"notes/schema.txt"})
	if err != nil {
		t.Fatal(err)
	}
	extra := []string{"tools/gen.py"}

	cases := []struct {
		file string
		want []string
	}{
		// Case-folded name tokens: Logger, NewLogger and logger all reference
		// logger.go; the blogger substring does not.
		{"src/logger.go", []string{"rule:g_rule_tls", "src/factory.go", "src/logger.go", "src/service.ts", "tools/gen.py"}},
		// "remodel" is not "model".
		{"src/model.go", []string{"rule:g_rule_tls", "src/model.go", "tools/gen.py"}},
		// Multi-token names match as contiguous identifier sequences.
		{"src/user_auth.go", []string{"rule:g_rule_tls", "src/UserAuthService.ts", "src/user_auth.go", "tools/gen.py"}},
		// A data file links by path form, in both directions.
		{"src/config.json", []string{"rule:g_rule_tls", "src/config.json", "src/loader.go", "tools/gen.py"}},
		// Short names match only by path form, at identifier boundaries;
		// "xv2.go" in prose does not reference v2.go.
		{"src/v2.go", []string{"rule:g_rule_tls", "src/pin.go", "src/v2.go", "tools/gen.py"}},
		// No transitive closure: chain_c.go is not chain_a.go's evidence.
		{"src/chain_a.go", []string{"rule:g_rule_tls", "src/chain_a.go", "src/chain_b.go", "tools/gen.py"}},
		// A non-code target links only by path form; its prose mention of
		// alpha stays local.
		{"notes/schema.txt", []string{"notes/schema.txt", "rule:g_rule_tls", "tools/gen.py"}},
		{"docs/specs/units/candidate/unit_auth.md", []string{"docs/specs/units/candidate/unit_auth.md", "rule:g_rule_tls", "tools/gen.py"}},
	}
	for _, tc := range cases {
		got := corpus.evidence(tc.file, extra)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("evidence for %q:\n got: %v\nwant: %v", tc.file, got, tc.want)
		}
	}

	// The NUL-byte file and the framework deployment artifacts are excluded
	// from the corpus: no derivation may see them.
	for _, file := range []string{"src/logger.go", "src/factory.go"} {
		for _, p := range corpus.evidence(file, nil) {
			if p == "src/null.go" || p == ".opencode/plugins/specflow.js" || p == ".claude-plugin/plugin.json" {
				t.Fatalf("excluded file %q leaked into evidence for %q", p, file)
			}
		}
	}
	// Governance trees never participate in the corpus (the degenerate
	// target-itself entry is the only allowed occurrence).
	for _, tc := range cases {
		for _, p := range corpus.evidence(tc.file, nil) {
			if strings.HasPrefix(p, "docs/specs/") && p != tc.file {
				t.Fatalf("governance file %q leaked into evidence for %q", p, tc.file)
			}
		}
	}
}

// TestEvidenceSurfaceDump prints the derived public evidence surface of every
// corpus file for a repository given in SPECFLOW_SURFACE_DUMP (a git worktree
// root). It is a read-only inspection aid, not an assertion: run it on the
// same checkout before and after a derivation-rule change and diff the log.
//
//	SPECFLOW_SURFACE_DUMP=/path/to/repo go test ./internal/gaterun -run TestEvidenceSurfaceDump -v
func TestEvidenceSurfaceDump(t *testing.T) {
	root := os.Getenv("SPECFLOW_SURFACE_DUMP")
	if root == "" {
		t.Skip("set SPECFLOW_SURFACE_DUMP to a git worktree root to dump evidence surfaces")
	}
	derivation, err := NewDerivation(root)
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := derivation.loadEvidenceCorpus(nil)
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0, len(corpus.entries))
	for p := range corpus.entries {
		files = append(files, p)
	}
	sort.Strings(files)
	for _, file := range files {
		surface := corpus.evidence(file, nil)
		t.Logf("%s (%d)\n  %s", file, len(surface), strings.Join(surface, "\n  "))
	}
}

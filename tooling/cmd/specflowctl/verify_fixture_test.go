package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// Current protocol fixture creation executes the complete gate. Individual
// tests then alter the specific metadata or dependency they intend to test.
func currentVerifyFixture(t *testing.T, root, unit, target, extra string) {
	t.Helper()
	id := grPlan(t, root, "--gate", "verify", "--unit", unit, "--target", target)
	run := mustLoadRun(t, root, id)
	main := run.RequiredFiles[0]
	data, err := os.ReadFile(filepath.Join(root, main))
	if err != nil {
		t.Fatal(err)
	}
	for _, ck := range run.Coverage {
		states, err := gaterun.LoadSessionStates(root, run)
		if err != nil {
			t.Fatal(err)
		}
		covered, _, err := gaterun.CoverageProgress(run, states)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := covered[ck.Key]; ok {
			continue
		}
		report := grDefaultQualityReport(run, ck)
		if gaterun.IsItemKind(ck.Kind) {
			spec := main
			code := ""
			for _, surface := range run.Surfaces {
				for _, f := range surface.Entries {
					if code == "" {
						code = f.Path
					}
				}
			}
			if ck.Kind == gaterun.SessionKindPreserve {
				spec = "docs/specs/units/stable/unit_" + ck.Unit + ".md"
				for _, p := range ck.ReadRefs {
					if !strings.HasPrefix(p, "docs/specs/") {
						code = p
						break
					}
				}
			}
			report = grVerifyItemBody(ck.Key, "ALIGNED", code+":1") + ck.Key + ": " + spec + ": acceptance_item:" + ck.Item + "\n" + ck.Key + ": " + code + ": all\n"
		}
		if !strings.Contains(string(data), "## Description") {
			report = strings.ReplaceAll(report, main+": Description", main+": all")
		}
		if err := sharedSubmit(t, root, id, ck.Key, report); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	if err := runGateFinalize([]string{"--repo-root", root, "--run", id}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "docs/specs/meta/validation/unit", unit, "verify_result.md")
	cache, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(cache), "\n")
	for _, field := range strings.Split(extra, "\n") {
		key, _, ok := strings.Cut(field, ":")
		if !ok {
			continue
		}
		for i := 1; i < len(lines) && lines[i] != "---"; i++ {
			if strings.HasPrefix(lines[i], key+":") {
				lines[i] = field
			}
		}
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
}

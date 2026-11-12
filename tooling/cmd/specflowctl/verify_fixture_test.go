package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		if ck.Kind == gaterun.SessionKindUnit {
			report = grVerifyUnitScanReport(t, root, run, ck.Key)
		}
		if gaterun.IsItemKind(ck.Kind) {
			code := ""
			for _, surface := range run.Surfaces {
				for _, f := range surface.Entries {
					if code == "" {
						code = f.Path
					}
				}
			}
			report = grVerifyItemBody(ck.Key, "ALIGNED", code+":1") + ck.Key + ": " + main + ": acceptance_item:" + ck.Item + "\n" + ck.Key + ": " + code + ": all\n"
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

// grVerifyUnitScanReport renders a minimal accepted unit-level scan report: the
// alignment verdict plus the fixed declaration-class lines and the surplus line
// the report contract requires.
// grVerifyUnitScanReport builds a unit-scan session report for the run: the
// aggregate verdict (recomposed from the row results) and summary lines, one
// `row` line per catalog row — or, for a row-scoped re-run, exactly the
// assigned rows. Declaration rows cite the unit's declared surface files;
// surplus rows are clean; non-runnable rows are ok. A unit with no declared
// surface reports its declaration rows as undetermined.
func grVerifyUnitScanReport(t *testing.T, root string, run *gaterun.Run, key string) string {
	t.Helper()
	catalog, err := gaterun.UnitScanRowCatalog(root, run)
	if err != nil {
		t.Fatalf("unit-scan row catalog: %v", err)
	}
	assigned := map[string]bool{}
	for _, id := range run.ScanRows {
		assigned[id] = true
	}
	scoped := len(run.ScanRows) > 0
	files := grUnitSurfaceFiles(run)
	var rowLines []string
	var results []gaterun.ScanRowResult
	for _, row := range catalog {
		if scoped && !assigned[row.ID] {
			continue
		}
		result := gaterun.ScanRowResult{ID: row.ID}
		line := ""
		switch row.Kind {
		case gaterun.RowKindDeclaration:
			if len(files) == 0 {
				result.Verdict = gaterun.RowUndetermined
				line = fmt.Sprintf("row %s: undetermined — no declared implementation surface to verify against\n", row.ID)
			} else {
				result.Verdict = gaterun.RowMatch
				result.Files = append([]string(nil), files...)
				line = fmt.Sprintf("row %s: match — files: %s\n", row.ID, strings.Join(files, ", "))
			}
		case gaterun.RowKindSurplus:
			result.Verdict = gaterun.RowClean
			line = fmt.Sprintf("row %s: clean — no undeclared behavior\n", row.ID)
		case gaterun.RowKindNonRunnable:
			result.Verdict = gaterun.RowOK
			line = fmt.Sprintf("row %s: ok — substantive reason present\n", row.ID)
		case gaterun.RowKindCoverage:
			result.Verdict = gaterun.RowOK
			line = fmt.Sprintf("row %s: ok — fixture runnable coverage\n", row.ID)
		}
		results = append(results, result)
		rowLines = append(rowLines, line)
	}
	var b strings.Builder
	if !scoped {
		fmt.Fprintf(&b, "%s: %s — fixture scan\n", key, gaterun.AggregateUnitScanVerdict(results))
		for _, class := range unitScanClasses {
			fmt.Fprintf(&b, "declarations.%s: checked=0 — fixture scan\n", class)
		}
		b.WriteString("surplus: found=0 — fixture scan\n")
		b.WriteString("non_runnable: count=0 — fixture scan\n")
	}
	for _, line := range rowLines {
		b.WriteString(line)
	}
	return b.String()
}

// grUnitSurfaceFiles lists the run's derived surface files for row citations.
func grUnitSurfaceFiles(run *gaterun.Run) []string {
	var files []string
	for _, surface := range run.Surfaces {
		if surface.Source != gaterun.SourceDerived {
			continue
		}
		for _, entry := range surface.Entries {
			files = append(files, entry.Path)
		}
	}
	sort.Strings(files)
	return files
}

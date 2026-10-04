package promote

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func writePrerequisiteFile(t *testing.T, root, ref, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUnitRulePrerequisitesPublication(t *testing.T) {
	stable := "---\nrule_id: b_rule_http\nrule_scope: bound\n---\nUse HTTPS.\n"
	for _, tc := range []struct {
		name      string
		stable    string
		candidate string
		block     bool
	}{
		{"missing", "", "", true},
		{"candidate only", "", stable, true},
		{"stable only", stable, "", false},
		{"identical", stable, stable, false},
		{"normalized identical", stable, strings.ReplaceAll(strings.TrimSuffix(stable, "\n"), "\n", "\r\n"), false},
		{"constraint changed", stable, strings.ReplaceAll(stable, "HTTPS", "HTTP"), true},
		{"wording only", stable, strings.ReplaceAll(stable, "Use HTTPS.", "Always use HTTPS."), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writePromotableUnit(t, root, "consumer", "none", "b_rule_http")
			writeVerifyCache(t, root, "consumer")
			if tc.stable != "" {
				writePrerequisiteFile(t, root, specpaths.RuleStableFileRef("b_rule_http"), tc.stable)
			}
			if tc.candidate != "" {
				writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef("b_rule_http"), tc.candidate)
			}
			report, err := CheckUnitRulePrerequisites(root, "consumer")
			if err != nil {
				t.Fatal(err)
			}
			if (len(report.Blockers) > 0) != tc.block || len(report.Advisories) != 0 {
				t.Fatalf("unexpected publication result: %+v", report)
			}
			if tc.block {
				before := prerequisiteFiles(t, root)
				result := Promote(root, "consumer")
				if result.Passed || !strings.Contains(strings.Join(result.Issues, " "), "b_rule_http") {
					t.Fatalf("direct promote must reject unpublished rules: %+v", result)
				}
				if after := prerequisiteFiles(t, root); !reflect.DeepEqual(before, after) {
					t.Fatal("rejected promote changed files")
				}
			} else if result := Promote(root, "consumer"); !result.Passed {
				t.Fatalf("published rule should allow promote: %+v", result)
			}
		})
	}
}

func TestUnitRulePrerequisitesGlobalsAndDirectBindings(t *testing.T) {
	root := t.TempDir()
	writePromotableUnit(t, root, "consumer", "dependency", "[b_rule_z, b_rule_a, b_rule_z]")
	writePromotableUnit(t, root, "dependency", "none", "b_rule_indirect")
	writePrerequisiteFile(t, root, specpaths.StableUnitSpecFileRef("consumer"), "---\nid: consumer\nrule_refs: b_rule_dropped\n---\n")
	for _, id := range []string{"b_rule_z", "b_rule_a", "b_rule_indirect", "b_rule_dropped"} {
		writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef(id), "new constraint\n")
	}
	// An unreadable unrelated bound candidate is not a discovery input.
	if err := os.MkdirAll(filepath.Join(root, specpaths.RuleCandidateFileRef("b_rule_unrelated")), 0755); err != nil {
		t.Fatal(err)
	}
	writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_new"), "new global\n")
	writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_changed"), "new global\n")
	writePrerequisiteFile(t, root, specpaths.RuleStableFileRef("g_rule_changed"), "old global\n")
	writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_identical"), "same global\r\n")
	writePrerequisiteFile(t, root, specpaths.RuleStableFileRef("g_rule_identical"), "same global\n")
	report, err := CheckUnitRulePrerequisites(root, "consumer")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Blockers) != 2 || report.Blockers[0].RuleID != "b_rule_a" || report.Blockers[1].RuleID != "b_rule_z" {
		t.Fatalf("expected sorted direct bindings only: %+v", report)
	}
	if len(report.Advisories) != 2 || report.Advisories[0].RuleID != "g_rule_changed" || report.Advisories[1].RuleID != "g_rule_new" {
		t.Fatalf("expected sorted changed/new globals only: %+v", report)
	}
}

func TestUnitRulePrerequisitesExplicitGlobalNeedsStable(t *testing.T) {
	root := t.TempDir()
	writePromotableUnit(t, root, "consumer", "none", "g_rule_new")
	writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_new"), "new global\n")
	report, err := CheckUnitRulePrerequisites(root, "consumer")
	if err != nil || len(report.Blockers) != 1 || len(report.Advisories) != 0 {
		t.Fatalf("explicit global reference needs stable: %+v, %v", report, err)
	}
	writePrerequisiteFile(t, root, specpaths.RuleStableFileRef("g_rule_new"), "old global\n")
	report, err = CheckUnitRulePrerequisites(root, "consumer")
	if err != nil || len(report.Blockers) != 0 || len(report.Advisories) != 1 {
		t.Fatalf("published global must remain advisory: %+v, %v", report, err)
	}
}

func TestUnitRulePrerequisitesReadErrors(t *testing.T) {
	for _, ref := range []string{
		specpaths.RuleCandidateFileRef("b_rule_http"),
		specpaths.RuleStableFileRef("b_rule_http"),
		specpaths.RuleCandidateFileRef("g_rule_draft"),
		specpaths.RuleStableFileRef("g_rule_draft"),
		specpaths.RuleCandidateDir,
	} {
		t.Run(ref, func(t *testing.T) {
			root := t.TempDir()
			writePromotableUnit(t, root, "consumer", "none", "b_rule_http")
			if ref != specpaths.RuleCandidateDir {
				writePrerequisiteFile(t, root, specpaths.RuleStableFileRef("b_rule_http"), "published bound\n")
				writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_draft"), "global draft\n")
			}
			path := filepath.Join(root, filepath.FromSlash(ref))
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if ref == specpaths.RuleCandidateDir {
				writePrerequisiteFile(t, root, ref, "not a directory")
			} else if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
			before := prerequisiteFiles(t, root)
			_, err := CheckUnitRulePrerequisites(root, "consumer")
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("expected error naming %s, got %v", path, err)
			}
			if result := Promote(root, "consumer"); result.Passed {
				t.Fatal("direct promote must stop on read errors")
			}
			if after := prerequisiteFiles(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("read error changed files")
			}
		})
	}
}

func TestRulePublicationBlockerReturnsBeforeAppendixStaging(t *testing.T) {
	root := t.TempDir()
	writeCandidateUnit(t, root, "consumer")
	writePromotableUnit(t, root, "consumer", "none", "b_rule_unpublished")
	writeVerifyCache(t, root, "consumer")
	writePrerequisiteFile(t, root, specpaths.RuleCandidateFileRef("b_rule_unpublished"), "unpublished constraint\n")
	before := prerequisiteFiles(t, root)
	if result := Promote(root, "consumer"); result.Passed {
		t.Fatal("unpublished rule did not block direct promote")
	}
	if after := prerequisiteFiles(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("blocked direct promote staged appendix files")
	}
	if _, err := os.Stat(filepath.Join(root, specpaths.StableDir)); !os.IsNotExist(err) {
		t.Fatalf("blocked direct promote created the stable directory: %v", err)
	}
}

func prerequisiteFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[path] = string(data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

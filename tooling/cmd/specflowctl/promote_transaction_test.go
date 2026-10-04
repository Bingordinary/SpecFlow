package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func TestRulePublicationConsumerHandoff(t *testing.T) {
	for _, tc := range []struct {
		name, scope, priorVersion, version string
		noUnits                            bool
	}{
		{"first global", "global", "", "0.1.0", false},
		{"first bound", "bound", "", "0.1.0", false},
		{"first global without units", "global", "", "0.1.0", true},
		{"first bound without units", "bound", "", "0.1.0", true},
		{"global major", "global", "0.1.0", "1.0.0", false},
		{"global minor", "global", "0.1.0", "0.2.0", false},
		{"global patch", "global", "0.1.0", "0.1.1", false},
		{"bound major", "bound", "0.1.0", "1.0.0", false},
		{"bound minor", "bound", "0.1.0", "0.2.0", false},
		{"bound patch", "bound", "0.1.0", "0.1.1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := createCLITestRepo(t)
			grEnableMissionLayout(t, root)
			id := "g_rule_handoff"
			if tc.scope == "bound" {
				id = "b_rule_handoff"
			}
			candidate := specpaths.RuleCandidateFileRef(id)
			stable := specpaths.RuleStableFileRef(id)
			grWriteFile(t, root, candidate, publicationRuleText(id, tc.scope, tc.version, "Reject unauthenticated requests."))
			if tc.priorVersion != "" {
				grWriteFile(t, root, stable, publicationRuleText(id, tc.scope, tc.priorVersion, "Prior constraint."))
			}
			units := map[string]string{}
			if !tc.noUnits {
				for _, name := range []string{"accepted", "draft"} {
					path := grWriteSpec(t, root, name)
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if tc.scope == "bound" && name == "draft" {
						data = []byte(strings.Replace(string(data), "rule_refs: none", "rule_refs:\n  - "+id, 1))
						if err := os.WriteFile(path, data, 0644); err != nil {
							t.Fatal(err)
						}
					}
					if name == "accepted" {
						stablePath := filepath.Join(root, filepath.FromSlash(specpaths.StableUnitSpecFileRef(name)))
						if err := os.MkdirAll(filepath.Dir(stablePath), 0755); err != nil {
							t.Fatal(err)
						}
						if err := os.Rename(path, stablePath); err != nil {
							t.Fatal(err)
						}
						path = stablePath
					}
					units[path] = string(data)
				}
			}
			keys := []string{"1", "2", "3", "4", "5", "6", "7"}
			scopes := map[string][]string{}
			for _, key := range keys {
				scopes[key] = []string{candidate + ": all"}
			}
			if tc.priorVersion != "" {
				scopes["4"] = append(scopes["4"], stable+": all")
			}
			run := grPlan(t, root, "--gate", "validate", "--rule", id, "--target", "candidate")
			grSubmitOK(t, root, run, "checks", grValidateReport(keys, scopes))
			grFinalizeOK(t, root, run)
			output, err := publicationPromote(t, root, "rule", id)
			if err != nil {
				t.Fatalf("publication: %v\n%s", err, output)
			}
			for _, instruction := range []string{"Assess consumer impact per rule content", "specflowctl consumers --rule " + id, "specflowctl fresh --unit <name>", "gates remain user-triggered"} {
				if !strings.Contains(output, instruction) {
					t.Errorf("publication omitted %q:\n%s", instruction, output)
				}
			}
			var stdout, stderr bytes.Buffer
			if err := runConsumers([]string{"--repo-root", root, "--rule", id}, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			want := []string{"accepted", "draft"}
			if tc.scope == "bound" {
				want = []string{"draft"}
			}
			if tc.noUnits {
				if !strings.Contains(stdout.String(), "No consumers found") {
					t.Fatalf("empty consumer control: %s", stdout.String())
				}
			} else {
				if !strings.Contains(stdout.String(), fmt.Sprintf("(%d):", len(want))) {
					t.Fatalf("unexpected consumer set: %s", stdout.String())
				}
				for _, name := range want {
					if !strings.Contains(stdout.String(), "  - "+name+"\n") {
						t.Errorf("missing consumer %s: %s", name, stdout.String())
					}
				}
			}
			for path, before := range units {
				if after, err := os.ReadFile(path); err != nil || string(after) != before {
					t.Errorf("rule publication changed consumer truth %s: %v", path, err)
				}
			}
			fresh, err := freshRun(t, root, "--rule", id)
			if err != nil || !strings.Contains(fresh, "FRESH") {
				t.Fatalf("published rule confirmation: %v\n%s", err, fresh)
			}
		})
	}
}

func TestFirstGlobalRulePublicationRequiresConsumerRecheck(t *testing.T) {
	root := createCLITestRepo(t)
	grEnableMissionLayout(t, root)
	writePublicationUnit(t, root, "consumer", "none", nil)
	before, err := freshRun(t, root, "--unit", "consumer")
	if err != nil || !strings.Contains(before, "READY FOR PROMOTE: yes") {
		t.Fatalf("consumer precondition: %v\n%s", err, before)
	}
	const id = "g_rule_first"
	candidate := specpaths.RuleCandidateFileRef(id)
	grWriteFile(t, root, candidate, publicationRuleText(id, "global", "0.1.0", "Reject unauthenticated requests."))
	keys := []string{"1", "2", "3", "4", "5", "6", "7"}
	scopes := map[string][]string{}
	for _, key := range keys {
		scopes[key] = []string{candidate + ": all"}
	}
	run := grPlan(t, root, "--gate", "validate", "--rule", id, "--target", "candidate")
	grSubmitOK(t, root, run, "checks", grValidateReport(keys, scopes))
	grFinalizeOK(t, root, run)
	output, err := publicationPromote(t, root, "rule", id)
	if err != nil {
		t.Fatalf("publication: %v\n%s", err, output)
	}
	after, err := freshRun(t, root, "--unit", "consumer")
	if err != nil || !strings.Contains(after, "newly required evidence rule:"+id) || !strings.Contains(after, "READY FOR PROMOTE: no") {
		t.Fatalf("new global constraint must require consumer evidence: %v\n%s", err, after)
	}
}

// A failed publication must preserve every governed file, including candidate
// inputs and old metadata, so the same command can succeed after repair.
func TestPromoteTransactionFailureAndRetry(t *testing.T) {
	for _, kind := range []string{"unit", "rule"} {
		for _, failure := range []string{"baseline", "candidate-cleanup"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				if failure == "candidate-cleanup" && runtime.GOOS == "windows" {
					t.Skip("directory permission failure is not portable to Windows")
				}
				root := createCLITestRepo(t)
				name := "publication"
				candidate := "docs/specs/units/candidate/unit_publication.md"
				stable := "docs/specs/units/stable/unit_publication.md"
				cleanupDir := "docs/specs/units/candidate/appendix"
				if kind == "unit" {
					appendix := cleanupDir + "/unit_publication_extra.md"
					grWriteFile(t, root, appendix, "---\nunit: publication\n---\nNew appendix.\n")
					writePublicationUnit(t, root, name, "none", []string{appendix})
					grWriteFile(t, root, stable, "old accepted main\n")
					grWriteFile(t, root, "docs/specs/units/stable/appendix/unit_publication_extra.md", "---\nunit: publication\n---\nold accepted appendix\n")
				} else {
					name = "b_rule_publication"
					candidate = "docs/specs/rules/candidate/" + name + ".md"
					stable = "docs/specs/rules/stable/" + name + ".md"
					cleanupDir = "docs/specs/rules/candidate"
					proposed := publicationRuleText(name, "bound", "1.1.0", "New constraint.")
					grWriteFile(t, root, candidate, proposed)
					grWriteFile(t, root, stable, publicationRuleText(name, "bound", "1.0.0", "Old constraint."))
					writeRuleCache(t, root, name, []cacheFileSpec{{path: candidate, hash: computeHash(proposed)}})
				}
				baselineDir := "docs/specs/meta/baseline/" + kind
				baselineFile := baselineDir + "/" + name + ".yaml"
				if failure == "baseline" {
					grWriteFile(t, root, baselineDir, "not a directory\n")
				} else {
					grWriteFile(t, root, baselineFile, "old publication baseline\n")
					full := filepath.Join(root, filepath.FromSlash(cleanupDir))
					if err := os.Chmod(full, 0555); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { os.Chmod(full, 0755) })
				}
				before := publicationFiles(t, root)
				output, err := publicationPromote(t, root, kind, name)
				if err == nil {
					t.Fatalf("expected %s failure:\n%s", failure, output)
				}
				if !strings.Contains(output, "baseline") && failure == "baseline" {
					t.Fatalf("wrong failure: %v\n%s", err, output)
				}
				if after := publicationFiles(t, root); !reflect.DeepEqual(before, after) {
					for file, want := range before {
						if got, exists := after[file]; !exists || got != want {
							t.Errorf("failed promote changed %s", file)
						}
					}
					for file := range after {
						if _, exists := before[file]; !exists {
							t.Errorf("failed promote left new file %s", file)
						}
					}
				}
				if failure == "baseline" {
					if err := os.Remove(filepath.Join(root, filepath.FromSlash(baselineDir))); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Chmod(filepath.Join(root, filepath.FromSlash(cleanupDir)), 0755); err != nil {
					t.Fatal(err)
				}
				output, err = publicationPromote(t, root, kind, name)
				if err != nil {
					t.Fatalf("same command must succeed after fixing the cause: %v\n%s", err, output)
				}
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(candidate))); !os.IsNotExist(err) {
					t.Fatalf("candidate remains: %v", err)
				}
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(baselineFile))); err != nil {
					t.Fatal(err)
				}
				if got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(stable))); err != nil || string(got) != before[filepath.Join(root, filepath.FromSlash(candidate))] {
					t.Fatalf("stable does not equal the candidate: %v", err)
				}
				fresh, err := freshRun(t, root, "--"+kind, name)
				if err != nil || !strings.Contains(fresh, "FRESH") || strings.Contains(fresh, "MISSING") {
					t.Fatalf("published confirmations not usable: %v\n%s", err, fresh)
				}
				t.Log(fmt.Sprintf("%s failure restored original state; normal retry and fresh passed", failure))
			})
		}
	}
}

func TestRulePublicationConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name                string
		existing, livePrior bool
	}{
		{"new_rule_control", false, false},
		{"existing_rule", true, false},
		{"prior_stable_still_supports_check_7", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := createCLITestRepo(t)
			const id = "b_rule_confirmation"
			candidate := "docs/specs/rules/candidate/" + id + ".md"
			stable := "docs/specs/rules/stable/" + id + ".md"
			version := "0.1.0"
			if tc.existing {
				version = "1.3.0"
				grWriteFile(t, root, stable, publicationRuleText(id, "bound", "1.2.0", "Validate input before processing."))
			}
			grWriteFile(t, root, candidate, publicationRuleText(id, "bound", version, "Validate input before processing."))
			consumer := grWriteSpecWithRefs(t, root, "consumer", "none", id)
			runID := grPlan(t, root, "--gate", "validate", "--rule", id, "--target", "candidate")
			keys := []string{"1", "2", "3", "4", "5", "6", "7"}
			scopes := map[string][]string{}
			for _, key := range keys {
				scopes[key] = []string{candidate + ": all"}
			}
			scopes["5"] = append(scopes["5"], "unit:consumer: all")
			if tc.existing {
				scopes["4"] = append(scopes["4"], stable+": all")
			}
			if tc.livePrior {
				scopes["7"] = append(scopes["7"], stable+": all")
			}
			grSubmitOK(t, root, runID, "checks", grValidateReport(keys, scopes))
			grFinalizeOK(t, root, runID)
			priorCache := grReadCache(t, root, "docs/specs/meta/validation/rule/"+id+"/validate_result.md")
			before, err := validationcache.CheckRuleValidate(root, id)
			if err != nil || !before.Fresh {
				t.Fatalf("candidate precondition: %+v %v", before, err)
			}
			output, err := publicationPromote(t, root, "rule", id)
			if err != nil {
				t.Fatalf("promote: %v\n%s", err, output)
			}
			t.Log(output)
			cache := grReadCache(t, root, "docs/specs/meta/validation/rule/"+id+"/validate_result.md")
			if strings.SplitN(cache, "\n---\n", 2)[1] != strings.SplitN(priorCache, "\n---\n", 2)[1] {
				t.Fatal("projection changed accepted judgments or historical reports")
			}
			if n := strings.Count(cache, "- path: "+stable); !tc.livePrior && n != 1 {
				t.Errorf("expected one published rule evidence entry, got %d", n)
			}
			if !strings.Contains(cache, "- path: unit:consumer") {
				t.Fatal("lost live consumer evidence")
			}
			fresh, err := freshRun(t, root, "--rule", id)
			t.Log(fresh)
			after, err := validationcache.CheckRuleValidateStable(root, id)
			if tc.livePrior {
				if err != nil || after.Fresh {
					t.Fatalf("discarded still-applicable prior evidence: %+v %v", after, err)
				}
				if scope, err := validationcache.DeriveStaleScope(root, "rule", id, "validate"); err != nil || strings.Join(scope.Affected, ",") != "7" {
					t.Fatalf("expected live prior check 7 to require recheck: %+v %v", scope, err)
				}
				return
			}
			if err != nil || !after.Fresh {
				t.Fatalf("promoted confirmation must be FRESH: %+v %v", after, err)
			}
			data, err := os.ReadFile(consumer)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(consumer, append(data, []byte("\nChanged consumer contract.\n")...), 0644); err != nil {
				t.Fatal(err)
			}
			changed, err := validationcache.CheckRuleValidateStable(root, id)
			if err != nil || changed.Fresh {
				t.Fatalf("consumer change was lost by projection: %+v %v", changed, err)
			}
			if scope, err := validationcache.DeriveStaleScope(root, "rule", id, "validate"); err != nil || strings.Join(scope.Affected, ",") != "5" {
				t.Fatalf("wrong consumer delta scope: %+v %v", scope, err)
			}
		})
	}
}

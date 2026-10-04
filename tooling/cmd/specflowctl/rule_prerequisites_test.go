package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func publicationRuleText(id, scope, constraint string) string {
	return fmt.Sprintf("---\nrule_id: %s\nrule_scope: %s\n---\n%s\n", id, scope, constraint)
}

func writePublicationUnit(t *testing.T, root, name, ruleRefs string, ruleInputs []string) {
	t.Helper()
	specPath := grWriteSpecItems(t, root, name, "none", ruleRefs, []string{name + ".core"})
	codeRef := "src/" + name + ".go"
	grWriteFile(t, root, codeRef, "package demo\n")
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte(strings.ReplaceAll(string(spec), "implementation_surface: src\n", "implementation_surface: "+codeRef+"\n")), 0644); err != nil {
		t.Fatal(err)
	}
	specRef := specpaths.CandidateUnitSpecFileRef(name)
	var validateEntries []validationcache.FileEntry
	for _, ref := range append([]string{specRef}, ruleInputs...) {
		entry, err := validationcache.BuildEntry(root, validationcache.EntryDeclaration{Path: ref})
		if err != nil {
			t.Fatal(err)
		}
		validateEntries = append(validateEntries, entry)
	}
	if _, err := validationcache.WriteCache(root, "unit", name, validationcache.CacheWrite{
		Command: "validate", Unit: name, Mode: "full", Result: "pass", Target: "candidate", Entries: validateEntries,
	}); err != nil {
		t.Fatal(err)
	}
	currentVerifyFixture(t, root, name, "candidate", "")
}

func publicationPromote(t *testing.T, root, kind, name string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := runPromote([]string{"--repo-root", root, "--" + kind, name}, &stdout, &stderr)
	return stdout.String(), err
}

func publicationFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestRulePublicationBlocksFreshAndPromoteWithoutWrites(t *testing.T) {
	stable := publicationRuleText("b_rule_http", "bound", "Use HTTPS.")
	for _, tc := range []struct {
		name, stable, candidate string
	}{
		{"candidate only", "", publicationRuleText("b_rule_http", "bound", "Use HTTPS.")},
		{"constraint changed", stable, publicationRuleText("b_rule_http", "bound", "Reject HTTP.")},
		{"wording only", stable, publicationRuleText("b_rule_http", "bound", "Always use HTTPS.")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := createCLITestRepo(t)
			if tc.stable != "" {
				grWriteFile(t, root, specpaths.RuleStableFileRef("b_rule_http"), tc.stable)
			}
			grWriteFile(t, root, specpaths.RuleCandidateFileRef("b_rule_http"), tc.candidate)
			writePublicationUnit(t, root, "consumer", "b_rule_http", []string{"rule:b_rule_http"})
			grWriteFile(t, root, specpaths.StableUnitSpecFileRef("consumer"), "previous stable unit\n")
			grWriteFile(t, root, "docs/specs/meta/baseline/unit/consumer.yaml", "previous baseline\n")
			before := publicationFiles(t, root)
			detail, err := freshRun(t, root, "--unit", "consumer")
			if err != nil {
				t.Fatal(err)
			}
			assertGateStatus(t, detail, "validate", "FRESH")
			assertGateStatus(t, detail, "verify", "FRESH")
			if !strings.Contains(detail, "RULE PREREQUISITES: BLOCKED") || !strings.Contains(detail, "READY FOR PROMOTE: no") {
				t.Fatalf("fresh must block publication independently of caches:\n%s", detail)
			}
			summary, err := freshRun(t, root, "--scope", "candidate")
			if err != nil || !strings.Contains(summary, "rules: BLOCKED") || !strings.Contains(summary, "READY: false") {
				t.Fatalf("summary must show the same rule blocker: %v\n%s", err, summary)
			}
			output, err := publicationPromote(t, root, "unit", "consumer")
			if err == nil || !strings.Contains(output, "b_rule_http") || strings.Contains(output, "Validate cache:") {
				t.Fatalf("rule blocker must precede cache checks: %v\n%s", err, output)
			}
			if after := publicationFiles(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("fresh or rejected promote changed candidate, stable, caches, or baseline")
			}
		})
	}
}

func TestRulePublicationAllowsPublishedContentAndGlobalDrafts(t *testing.T) {
	for _, identicalCandidate := range []bool{false, true} {
		t.Run(fmt.Sprintf("identical candidate %t", identicalCandidate), func(t *testing.T) {
			root := createCLITestRepo(t)
			bound := publicationRuleText("b_rule_http", "bound", "Use HTTPS.")
			grWriteFile(t, root, specpaths.RuleStableFileRef("b_rule_http"), bound)
			if identicalCandidate {
				grWriteFile(t, root, specpaths.RuleCandidateFileRef("b_rule_http"), strings.ReplaceAll(strings.TrimSuffix(bound, "\n"), "\n", "\r\n"))
			}
			grWriteFile(t, root, specpaths.RuleStableFileRef("g_rule_updated"), publicationRuleText("g_rule_updated", "global", "Old global."))
			grWriteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_updated"), publicationRuleText("g_rule_updated", "global", "New global."))
			grWriteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_new"), publicationRuleText("g_rule_new", "global", "New global."))
			grWriteFile(t, root, specpaths.RuleCandidateFileRef("b_rule_unrelated"), publicationRuleText("b_rule_unrelated", "bound", "Other constraint."))
			writePublicationUnit(t, root, "consumer", "b_rule_http", []string{"rule:b_rule_http", "rule:g_rule_updated"})
			detail, err := freshRun(t, root, "--unit", "consumer")
			if err != nil || !strings.Contains(detail, "READY FOR PROMOTE: yes") || !strings.Contains(detail, "PENDING GLOBAL RULES (advisory)") || strings.Contains(detail, "b_rule_unrelated") {
				t.Fatalf("published constraints must allow ready with global advisories: %v\n%s", err, detail)
			}
			output, err := publicationPromote(t, root, "unit", "consumer")
			if err != nil || !strings.Contains(output, "PENDING GLOBAL RULES (advisory)") {
				t.Fatalf("global drafts must not block promote: %v\n%s", err, output)
			}
			stable, err := freshRun(t, root, "--unit", "consumer")
			if err != nil || strings.Contains(stable, "RULE PREREQUISITES") || strings.Contains(stable, "PENDING GLOBAL RULES") {
				t.Fatalf("stable confirmation must not gain promotion prerequisites: %v\n%s", err, stable)
			}
		})
	}
}

func TestRulePublicationSummaryDeduplicatesGlobalAdvisories(t *testing.T) {
	root := createCLITestRepo(t)
	for _, name := range []string{"one", "two"} {
		writePublicationUnit(t, root, name, "none", nil)
	}
	grWriteFile(t, root, specpaths.RuleCandidateFileRef("g_rule_new"), publicationRuleText("g_rule_new", "global", "New global."))
	output, err := freshRun(t, root, "--scope", "all")
	if err != nil || strings.Count(output, "PENDING GLOBAL RULES (advisory):") != 1 || strings.Count(output, "g_rule_new: unpublished global draft") != 1 {
		t.Fatalf("summary must display global advice once: %v\n%s", err, output)
	}
	// Both units remain ready; the unvalidated rule is not ready.
	if !strings.Contains(output, "READY FOR PROMOTE: 2 of 3") {
		t.Fatalf("global advice changed unit readiness:\n%s", output)
	}
}

func TestRulePublicationPrecedesMissingCachesAndReportsAllBlockers(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpecItems(t, root, "consumer", "none", "[b_rule_z, b_rule_a, b_rule_z]", []string{"consumer.core"})
	for _, id := range []string{"b_rule_z", "b_rule_a"} {
		grWriteFile(t, root, specpaths.RuleCandidateFileRef(id), publicationRuleText(id, "bound", "Constraint."))
	}
	output, err := publicationPromote(t, root, "unit", "consumer")
	if err == nil || strings.Contains(output, "cache not found") || strings.Count(output, "b_rule_a:") != 1 || strings.Count(output, "b_rule_z:") != 1 || strings.Index(output, "b_rule_a:") > strings.Index(output, "b_rule_z:") {
		t.Fatalf("all rule blockers must precede missing gates, sorted and deduplicated: %v\n%s", err, output)
	}
}

func TestRulePublicationReadErrorStopsFreshAndPromote(t *testing.T) {
	root := createCLITestRepo(t)
	grWriteSpecItems(t, root, "consumer", "none", "b_rule_http", []string{"consumer.core"})
	path := filepath.Join(root, specpaths.RuleCandidateFileRef("b_rule_http"))
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	before := publicationFiles(t, root)
	for _, args := range [][]string{{"--unit", "consumer"}, {"--scope", "candidate"}} {
		output, err := freshRun(t, root, args...)
		if err == nil || !strings.Contains(err.Error(), path) || strings.Contains(output, "READY FOR PROMOTE: yes") {
			t.Fatalf("fresh must stop on a read error: %v\n%s", err, output)
		}
	}
	output, err := publicationPromote(t, root, "unit", "consumer")
	if err == nil || !strings.Contains(output, path) {
		t.Fatalf("promote must report the failing path: %v\n%s", err, output)
	}
	if after := publicationFiles(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("read error modified files")
	}
}

func TestRulePublicationDroppedBindingDoesNotBlock(t *testing.T) {
	root := createCLITestRepo(t)
	id := "b_rule_dropped"
	grWriteFile(t, root, specpaths.RuleStableFileRef(id), publicationRuleText(id, "bound", "Old constraint."))
	grWriteFile(t, root, specpaths.RuleCandidateFileRef(id), publicationRuleText(id, "bound", "New constraint."))
	grWriteFile(t, root, specpaths.StableUnitSpecFileRef("consumer"), "---\nid: consumer\nunit_refs: none\nrule_refs: b_rule_dropped\n---\n")
	writePublicationUnit(t, root, "consumer", "none", nil)
	detail, err := freshRun(t, root, "--unit", "consumer")
	if err != nil || !strings.Contains(detail, "READY FOR PROMOTE: yes") || strings.Contains(detail, id) {
		t.Fatalf("old stable binding must not block current candidate: %v\n%s", err, detail)
	}
	if output, err := publicationPromote(t, root, "unit", "consumer"); err != nil {
		t.Fatalf("dropped binding blocked promote: %v\n%s", err, output)
	}
}

func TestRulePublicationEndToEndPreservesUnitGateEvidence(t *testing.T) {
	root := createCLITestRepo(t)
	id := "b_rule_http"
	grWriteFile(t, root, specpaths.RuleStableFileRef(id), publicationRuleText(id, "bound", "Use HTTPS."))
	candidate := grWriteFile(t, root, specpaths.RuleCandidateFileRef(id), publicationRuleText(id, "bound", "Reject HTTP."))
	writeRuleCache(t, root, id, []cacheFileSpec{{path: specpaths.RuleCandidateFileRef(id), hash: computeHash(candidate)}})
	writePublicationUnit(t, root, "consumer", id, []string{"rule:" + id})
	cacheDir := filepath.Join(root, "docs/specs/meta/validation/unit/consumer")
	beforeValidate, _ := os.ReadFile(filepath.Join(cacheDir, "validate_result.md"))
	beforeVerify, _ := os.ReadFile(filepath.Join(cacheDir, "verify_result.md"))
	if output, err := publicationPromote(t, root, "unit", "consumer"); err == nil {
		t.Fatalf("unit promoted before its rule:\n%s", output)
	}
	if output, err := publicationPromote(t, root, "rule", id); err != nil {
		t.Fatalf("rule promote failed: %v\n%s", err, output)
	}
	afterValidate, _ := os.ReadFile(filepath.Join(cacheDir, "validate_result.md"))
	afterVerify, _ := os.ReadFile(filepath.Join(cacheDir, "verify_result.md"))
	if !bytes.Equal(beforeValidate, afterValidate) || !bytes.Equal(beforeVerify, afterVerify) {
		t.Fatal("rule promote changed the unit's passing gate evidence")
	}
	output, err := freshRun(t, root, "--unit", "consumer")
	if err != nil || !strings.Contains(output, "READY FOR PROMOTE: yes") {
		t.Fatalf("same published content must preserve gate freshness: %v\n%s", err, output)
	}
	assertGateStatus(t, output, "validate", "FRESH")
	assertGateStatus(t, output, "verify", "FRESH")
	if output, err := publicationPromote(t, root, "unit", "consumer"); err != nil {
		t.Fatalf("unit promote failed after rule publication: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(root, specpaths.StableUnitSpecFileRef("consumer"))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, specpaths.CandidateUnitSpecFileRef("consumer"))); !os.IsNotExist(err) {
		t.Fatalf("candidate unit was not cleaned up: %v", err)
	}
}

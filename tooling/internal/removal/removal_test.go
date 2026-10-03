package removal

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func put(t *testing.T, root, path, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func unit(t *testing.T, root, layer, name, refs, rules, extra string) string {
	t.Helper()
	path := "docs/specs/units/" + layer + "/unit_" + name + ".md"
	put(t, root, path, "---\nid: "+name+"\nunit_refs: "+refs+"\nrule_refs: "+rules+"\n---\n\n# Unit\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: "+name+".core\n    description: core\n"+extra)
	return path
}
func appendix(t *testing.T, root, layer, name, file, extra string) string {
	t.Helper()
	path := "docs/specs/units/" + layer + "/appendix/" + file
	put(t, root, path, "---\nunit: "+name+"\n"+extra+"---\n\n# Appendix\n")
	return path
}
func rule(t *testing.T, root, layer, name string) string {
	t.Helper()
	path := "docs/specs/rules/" + layer + "/" + name + ".md"
	put(t, root, path, "---\nrule_id: "+name+"\nrule_scope: bound\n---\n\n# Rule\n")
	return path
}
func assertExists(t *testing.T, root, path string, exists bool) {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	if exists && err != nil || !exists && !os.IsNotExist(err) {
		t.Fatalf("%s exists=%v: %v", path, exists, err)
	}
}
func snapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "meta/.gate_runs.lock" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func assertSnapshot(t *testing.T, root string, before map[string][]byte) {
	t.Helper()
	after := snapshot(t, root)
	if len(after) != len(before) {
		t.Fatalf("file count changed: %d -> %d", len(before), len(after))
	}
	for p, b := range before {
		if !bytes.Equal(b, after[p]) {
			t.Fatalf("changed: %s", p)
		}
	}
}
func TestExplicitBatchAllLayersAndOwnership(t *testing.T) {
	root := t.TempDir()
	for _, layer := range []string{"stable", "candidate"} {
		unit(t, root, layer, "auth", "dep", "b_rule_http", "")
		unit(t, root, layer, "dep", "auth", "none", "")
		unit(t, root, layer, "auth_extra", "none", "none", "")
		appendix(t, root, layer, "auth", "unit_auth_old.md", "status: exempt\n")
		appendix(t, root, layer, "auth_extra", "unit_auth_extra_old.md", "")
		rule(t, root, layer, "b_rule_http")
		rule(t, root, layer, "g_rule_policy")
	}
	put(t, root, "docs/specs/meta/baseline/unit/auth.yaml", "baseline")
	put(t, root, "docs/specs/meta/baseline/rule/b_rule_http.yaml", "baseline")
	put(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md", "cache")
	put(t, root, "docs/specs/meta/validation/rule/b_rule_http/validate_result.md", "cache")
	put(t, root, "src/auth.go", "business code")
	put(t, root, "meta/gate_runs/history/evidence.md", "historical check")
	put(t, root, "docs/specs/meta/validation/judgments/accepted/shared.json", "shared judgment")
	before := snapshot(t, root)
	req := Request{Units: []string{"auth", "dep", "auth"}, Rules: []string{"b_rule_http", "g_rule_policy", "b_rule_http"}, Appendices: []string{"auth:unit_auth_old.md"}, DryRun: true}
	preview, err := Run(root, req)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, root, before)
	req.DryRun = false
	result, err := Run(root, req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(result.Files, ",") != strings.Join(preview.Files, ",") {
		t.Fatal("preview and execution differ")
	}
	for _, p := range result.Files {
		assertExists(t, root, p, false)
	}
	for _, p := range []string{"src/auth.go", "meta/gate_runs/history/evidence.md", "docs/specs/meta/validation/judgments/accepted/shared.json", "docs/specs/units/stable/appendix/unit_auth_extra_old.md", "docs/specs/units/candidate/appendix/unit_auth_extra_old.md"} {
		assertExists(t, root, p, true)
	}
}
func TestObjectLayersAndStableFallback(t *testing.T) {
	for _, kind := range []string{"unit", "rule", "global", "appendix"} {
		for _, layers := range [][]string{{"stable"}, {"candidate"}, {"stable", "candidate"}} {
			t.Run(kind+strings.Join(layers, "-"), func(t *testing.T) {
				root := t.TempDir()
				req := Request{}
				for _, layer := range layers {
					switch kind {
					case "unit":
						unit(t, root, layer, "auth", "none", "none", "")
						req.Units = []string{"auth"}
					case "rule", "global":
						id := "b_rule_http"
						if kind == "global" {
							id = "g_rule_policy"
						}
						rule(t, root, layer, id)
						req.Rules = []string{id}
					case "appendix":
						unit(t, root, layer, "auth", "none", "none", "")
						appendix(t, root, layer, "auth", "unit_auth_old.md", "")
						req.Appendices = []string{"auth:unit_auth_old.md"}
					}
				}
				r, err := Run(root, req)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range r.Files {
					assertExists(t, root, p, false)
				}
			})
		}
	}
	root := t.TempDir()
	stable := unit(t, root, "stable", "auth", "none", "none", "")
	candidate := unit(t, root, "candidate", "auth", "none", "none", "")
	unit(t, root, "stable", "consumer", "auth", "b_rule_http", "")
	unit(t, root, "candidate", "consumer", "auth", "b_rule_http", "")
	stableRule := rule(t, root, "stable", "b_rule_http")
	candidateRule := rule(t, root, "candidate", "b_rule_http")
	put(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md", "---\ntarget: candidate\n---\ncache")
	put(t, root, "docs/specs/meta/validation/unit/auth/stable_verify.md", "---\ntarget: stable\n---\ncache")
	put(t, root, "docs/specs/meta/baseline/unit/auth.yaml", "baseline")
	if _, err := Run(root, Request{Units: []string{"auth"}, Rules: []string{"b_rule_http"}, Layer: "candidate"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{stable, stableRule, "docs/specs/meta/validation/unit/auth/stable_verify.md", "docs/specs/meta/baseline/unit/auth.yaml"} {
		assertExists(t, root, p, true)
	}
	for _, p := range []string{candidate, candidateRule, "docs/specs/meta/validation/unit/auth/validate_result.md"} {
		assertExists(t, root, p, false)
	}
}
func TestEveryRemainingLayerAndStructuredReferenceBlocks(t *testing.T) {
	for _, layer := range []string{"stable", "candidate"} {
		for _, field := range []string{"unit_refs", "rule_refs", "affects.dependencies", "affects.appendices", "evidence_appendix_ref"} {
			t.Run(layer+field, func(t *testing.T) {
				root := t.TempDir()
				unit(t, root, "stable", "auth", "none", "none", "")
				rule(t, root, "stable", "b_rule_http")
				appendix(t, root, layer, "auth", "unit_auth_old.md", "")
				refs, rules, extra := "none", "none", ""
				req := Request{Units: []string{"auth"}}
				switch field {
				case "unit_refs":
					refs = "auth"
				case "rule_refs":
					rules = "b_rule_http"
					req = Request{Rules: []string{"b_rule_http"}}
				case "affects.dependencies":
					extra = "    affects:\n      dependencies:\n        - auth\n"
				case "affects.appendices":
					extra = "    affects:\n      appendices: [unit_auth_old.md]\n"
					req = Request{Appendices: []string{"auth:unit_auth_old.md"}}
				case "evidence_appendix_ref":
					req = Request{Appendices: []string{"auth:unit_auth_old.md"}}
				}
				path := unit(t, root, layer, "consumer", refs, rules, extra)
				if field == "evidence_appendix_ref" {
					appendix(t, root, layer, "consumer", "unit_consumer_evidence.md", "evidence_appendix_ref: unit_auth_old.md\n")
				}
				before := snapshot(t, root)
				r, err := Run(root, req)
				if err == nil || r == nil || len(r.Blockers) == 0 {
					t.Fatalf("expected blockers: %+v %v", r, err)
				}
				if field != "evidence_appendix_ref" && !strings.Contains(strings.Join(r.Blockers, "\n"), path) {
					t.Fatal(r.Blockers)
				}
				assertSnapshot(t, root, before)
			})
		}
	}
}
func TestAppendixOnlyInvalidatesOwningLayerAndKeepsBaseline(t *testing.T) {
	root := t.TempDir()
	unit(t, root, "stable", "auth", "none", "none", "")
	unit(t, root, "candidate", "auth", "none", "none", "")
	appendix(t, root, "stable", "auth", "unit_auth_old.md", "")
	put(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md", "---\ntarget: candidate\n---\ncandidate")
	put(t, root, "docs/specs/meta/validation/unit/auth/stable_verify.md", "---\ntarget: stable\n---\nstable")
	put(t, root, "docs/specs/meta/baseline/unit/auth.yaml", "baseline")
	if _, err := Run(root, Request{Appendices: []string{"auth:unit_auth_old.md"}}); err != nil {
		t.Fatal(err)
	}
	assertExists(t, root, "docs/specs/meta/validation/unit/auth/validate_result.md", true)
	assertExists(t, root, "docs/specs/meta/validation/unit/auth/stable_verify.md", false)
	assertExists(t, root, "docs/specs/meta/baseline/unit/auth.yaml", true)
}
func TestPreflightErrorsAndRollback(t *testing.T) {
	for _, scenario := range []string{"unknown", "invalid-layer", "path", "read", "ownership", "unresolved", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			unit(t, root, "stable", "auth", "none", "none", "")
			req := Request{Units: []string{"auth"}}
			switch scenario {
			case "unknown":
				req.Units = []string{"typo"}
			case "invalid-layer":
				req.Layer = "stable"
			case "path":
				req.Appendices = []string{"auth:../../outside.md"}
			case "read":
				if err := os.MkdirAll(filepath.Join(root, "docs/specs/rules/stable/b_rule_bad.md"), 0755); err != nil {
					t.Fatal(err)
				}
			case "ownership":
				appendix(t, root, "stable", "other", "unit_auth_bad.md", "")
			case "unresolved":
				unit(t, root, "candidate", "consumer", "missing", "none", "")
			case "symlink":
				outside := t.TempDir()
				put(t, outside, "foreign.md", "---\nid: x\n---\n")
				if err := os.Symlink(filepath.Join(outside, "foreign.md"), filepath.Join(root, "docs/specs/units/stable/unit_outside.md")); err != nil {
					t.Skip(err)
				}
			}
			before := snapshot(t, root)
			if _, err := Run(root, req); err == nil {
				t.Fatal("expected preflight error")
			}
			assertSnapshot(t, root, before)
		})
	}
	root := t.TempDir()
	unit(t, root, "stable", "auth", "none", "none", "")
	appendix(t, root, "stable", "auth", "unit_auth_old.md", "")
	put(t, root, "meta/test.json", "old run")
	put(t, root, "meta/other.json", "other record")
	p, err := prepare(root, Request{Units: []string{"auth"}, Layer: "all"})
	if err != nil {
		t.Fatal(err)
	}
	p.writes["meta/test.json"] = []byte("invalidated run")
	p.writes["meta/other.json"] = []byte("new record")
	p.result.Updates = append(p.result.Updates, "meta/test.json", "meta/other.json")
	before := snapshot(t, root)
	calls := 0
	err = transact(root, p, func(a, b string) error {
		calls++
		if calls == 4 {
			return errors.New("injected rename failure")
		}
		return os.Rename(a, b)
	})
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	assertSnapshot(t, root, before)
}

func TestOtherDeclaredRuleReferencesAndMalformedLists(t *testing.T) {
	for _, field := range []string{"affects.rules", "rule_exceptions", "malformed-list"} {
		t.Run(field, func(t *testing.T) {
			root := t.TempDir()
			unit(t, root, "stable", "auth", "none", "none", "")
			rule(t, root, "stable", "g_rule_policy")
			path := unit(t, root, "candidate", "consumer", "none", "none", "")
			switch field {
			case "affects.rules":
				unit(t, root, "candidate", "consumer", "none", "none", "    affects:\n      rules: [g_rule_policy]\n")
			case "rule_exceptions":
				data, err := os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Fatal(err)
				}
				put(t, root, path, strings.Replace(string(data), "rule_refs: none", "rule_refs: none\nrule_exceptions:\n  - rule: g_rule_policy\n    reason: temporary exception", 1))
			case "malformed-list":
				unit(t, root, "candidate", "consumer", "[consumer", "none", "")
			}
			before := snapshot(t, root)
			r, err := Run(root, Request{Rules: []string{"g_rule_policy"}})
			if err == nil || r == nil || len(r.Blockers) == 0 {
				t.Fatal("reference not checked", r, err)
			}
			assertSnapshot(t, root, before)
		})
	}
}

func TestDeferredFindingsCleanupFollowsOwnerNotSource(t *testing.T) {
	root := t.TempDir()
	unit(t, root, "stable", "auth", "none", "none", "")
	unit(t, root, "stable", "peer", "none", "none", "")
	entry := func(id, owner, source string) validationcache.DeferredEntry {
		return validationcache.DeferredEntry{FindingID: id, OwnerUnit: owner, SourceUnit: source, SourceRun: "old-run", Severity: "P1", Text: "Finding", Detail: "Concrete evidence", SourceKey: "item:auth:auth.core", EvidencePath: "src/shared.go", Reason: "Owner responsibility"}
	}
	ledger := validationcache.DeferredLedger{Entries: []validationcache.DeferredEntry{entry("owned", "auth", "peer"), entry("sourced", "peer", "auth")}}
	if err := validationcache.WriteDeferredLedger(root, ledger); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, root)
	if _, err := Run(root, Request{Units: []string{"auth"}, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, root, before)
	if _, err := Run(root, Request{Units: []string{"auth"}}); err != nil {
		t.Fatal(err)
	}
	remaining, err := validationcache.ReadDeferredLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Entries) != 1 || remaining.Entries[0].FindingID != "sourced" {
		t.Fatal("wrong deferred records cleaned", remaining)
	}
}

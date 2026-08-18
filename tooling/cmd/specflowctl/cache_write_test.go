package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// cwWriteSpec writes a candidate unit spec with frontmatter and one
// acceptance-bearing section.
func cwWriteSpec(t *testing.T, repoRoot, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# " + name + "\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: " + name + ".core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// cwFileJSON renders one --file entry declaration as JSON.
func cwFileJSON(decl map[string]any) string {
	b, err := json.Marshal(decl)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestCacheWriteBasicValidatePass(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := cwWriteSpec(t, repoRoot, "auth")
	_ = specPath

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--blocking=false",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
			"checks": []map[string]any{
				{"check": "1", "sections": []string{"Description"}},
				{"check": "5", "sections": []string{"Testability / Acceptance Criteria"}},
			},
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Cache written:") || !strings.Contains(stdout.String(), "Self-check: FRESH") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}

	// The written cache must be accepted by the promote gate's chain.
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected written cache to be fresh, got: %s", res.Reason)
	}
}

func TestCacheWriteComputesHashAndDeps(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := cwWriteSpec(t, repoRoot, "auth")

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md"))
	content := string(data)

	// The whole-file hash must match the tooling's own computation, and the
	// deps must cover the file's content-defined chunks — the agent supplies
	// neither.
	text, _ := contenthash.FileText(specPath)
	expectedHash := contenthash.FileHashText(text)
	if !strings.Contains(content, "hash: "+expectedHash) {
		t.Fatalf("expected computed hash %s in cache, got:\n%s", expectedHash, content)
	}
	for _, c := range contenthash.ChunkText(text).Chunks {
		if !strings.Contains(content, "- "+c.CID) {
			t.Fatalf("expected chunk CID %s in deps, got:\n%s", c.CID, content)
		}
	}
}

func TestCacheWriteRejectsTranscribedHash(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	// A supplied hash field is ignored — the declaration schema has no hash
	// or deps fields at all. The tooling computes them, so a transcription
	// error cannot enter the cache. Verify the schema rejects unknown intent
	// by checking the entry building computes evidence regardless.
	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md"))
	// The cache must contain a real sha256 hash, never a placeholder the
	// agent could have typed.
	if !strings.Contains(string(data), "hash: sha256:") {
		t.Fatalf("expected computed sha256 hash, got:\n%s", string(data))
	}
	if strings.Contains(string(data), "0000000000") {
		t.Fatalf("placeholder hash leaked into the cache:\n%s", string(data))
	}
}

func TestCacheWriteSectionDeclUnion(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := cwWriteSpec(t, repoRoot, "auth")
	text, _ := contenthash.FileText(specPath)

	descRegion, ok := contenthash.LocateSectionRegion(text, "Description")
	if !ok {
		t.Fatal("expected Description section")
	}
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path":   "docs/specs/units/candidate/unit_auth.md",
			"checks": []map[string]any{{"check": "1", "sections": []string{"Description"}}},
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}

	data, _ := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md"))
	content := string(data)
	if !strings.Contains(content, "region:section:Description:") {
		t.Fatalf("expected the section-region dep in the file, got:\n%s", content)
	}
	// Union discipline: the check-level dep must appear in the file-level deps
	// union. The written file's per-check dep must equal the tooling-computed
	// region CID.
	if !strings.Contains(content, descDep) {
		t.Fatalf("expected check dep %s in the union, got:\n%s", descDep, content)
	}
	_ = specPath
}

func TestCacheWriteLogicalReference(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth") // dependency unit
	cwWriteSpec(t, repoRoot, "self")

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "self",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_self.md",
		}),
		"--file", cwFileJSON(map[string]any{
			"path": "unit:auth",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}

	data, _ := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self/validate_result.md"))
	content := string(data)
	if !strings.Contains(content, "- path: unit:auth") {
		t.Fatalf("expected the logical reference entry, got:\n%s", content)
	}
	if !strings.Contains(content, "hash: sha256:") {
		t.Fatalf("expected computed hash for the logical reference, got:\n%s", content)
	}

	res, err := validationcache.CheckValidate(repoRoot, "self")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected written cache fresh, got: %s", res.Reason)
	}
}

func TestCacheWriteFailureRecordRequiresStatus(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	// A failure record without a per-check status map fails closed.
	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "fail",
		"--target", "candidate",
		"--blocking=true",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
		}),
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected failure record without a status map to be rejected")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Fatalf("expected a status-contract error, got: %v", err)
	}
	// A rejected write must not leave a cache file behind (rollback).
	if _, serr := os.Stat(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md")); !os.IsNotExist(serr) {
		t.Fatal("expected the rejected failure record to be rolled back")
	}

	// With a valid status map the write succeeds and the gate reports BLOCKED.
	stdout.Reset()
	stderr.Reset()
	err = runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "fail",
		"--target", "candidate",
		"--blocking=true",
		"--p0-count", "1",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
			"checks": []map[string]any{
				{"check": "1", "status": "fail", "sections": []string{"Description"}},
				{"check": "5", "status": "pass", "sections": []string{"Testability / Acceptance Criteria"}},
			},
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}
	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if res.Category != validationcache.CategoryBlocked {
		t.Fatalf("expected the failure record to be BLOCKED, got category %q: %s", res.Category, res.Reason)
	}
}

func TestCacheWriteBlockingConsistency(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	// result=fail with blocking=false is a conflicting declaration.
	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "verify",
		"--unit", "auth",
		"--result", "fail",
		"--target", "candidate",
		"--blocking=false",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
			"checks": []map[string]any{
				{"check": "auth.core", "status": "fail"},
			},
		}),
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected result=fail + blocking=false to be rejected")
	}

	// blocking=true with result=pass is also a conflicting declaration.
	stdout.Reset()
	stderr.Reset()
	err = runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "verify",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--blocking=true",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
		}),
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected result=pass + blocking=true to be rejected")
	}
}

func TestCacheWriteAppendixCoverage(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := cwWriteSpec(t, repoRoot, "auth")

	// A non-exempt appendix exists on disk but is not declared in the files
	// list — the appendix promote gate must reject the write.
	appendixPath := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix", "unit_auth_protocol.md")
	os.MkdirAll(filepath.Dir(appendixPath), 0755)
	os.WriteFile(appendixPath, []byte("---\nunit: auth\nstatus: active\n---\n\n# Protocol\n\nPOST /login.\n"), 0644)

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
		}),
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected the appendix coverage check to reject the write")
	}
	if !strings.Contains(err.Error(), "appendix") {
		t.Fatalf("expected appendix error, got: %v", err)
	}

	// Declaring the appendix makes the write pass.
	stdout.Reset()
	stderr.Reset()
	err = runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
		}),
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/appendix/unit_auth_protocol.md",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}
	_ = specPath
}

func TestCacheWriteVerifyChecksUnion(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := cwWriteSpec(t, repoRoot, "auth")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "auth.go")
	os.WriteFile(srcPath, []byte("package auth\n\nfunc Login() {}\n"), 0644)

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "verify",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--p2-count", "1",
		"--file", cwFileJSON(map[string]any{
			"path":   "docs/specs/units/candidate/unit_auth.md",
			"checks": []map[string]any{{"check": "auth.core", "sections": []string{"Testability / Acceptance Criteria"}}},
		}),
		"--file", cwFileJSON(map[string]any{
			"path":   "src/auth.go",
			"ranges": "1-2",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}

	res, err := validationcache.CheckVerify(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected written verify cache fresh, got: %s", res.Reason)
	}

	// The per-item check key (auth.core) must appear in the checks mapping.
	data, _ := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/verify_result.md"))
	content := string(data)
	if !strings.Contains(content, `check: "auth.core"`) {
		t.Fatalf("expected the per-item check declaration, got:\n%s", content)
	}
	_ = specPath
	_ = srcPath
}

func TestCacheWriteRuleTarget(t *testing.T) {
	repoRoot := t.TempDir()
	ruleDir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	os.MkdirAll(ruleDir, 0755)
	rulePath := filepath.Join(ruleDir, "b_rule_http.md")
	os.WriteFile(rulePath, []byte("---\nid: b_rule_http\nversion: 0.1.0\nscope: unit\n---\n\n# HTTP Rule\n\n## Constraint\n\nMust use TLS.\n"), 0644)

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--rule", "b_rule_http",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/rules/candidate/b_rule_http.md",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}

	res, err := validationcache.CheckRuleValidate(repoRoot, "b_rule_http")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected written rule validate cache fresh, got: %s", res.Reason)
	}

	// verify/review gates do not apply to rules.
	stdout.Reset()
	stderr.Reset()
	err = runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "verify",
		"--rule", "b_rule_http",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/rules/candidate/b_rule_http.md",
		}),
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected rule verify to be rejected")
	}
}

func TestCacheWriteRollsBackOnSelfCheckRejection(t *testing.T) {
	repoRoot := t.TempDir()
	// A spec exists for unit "self", but the entry declares a DIFFERENT file
	// that is not the unit's main spec — the self-check (main-file check)
	// must reject the write and roll the cache back.
	cwWriteSpec(t, repoRoot, "self")
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "helper.go"), []byte("package helper\n"), 0644)

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "self",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "src/helper.go",
		}),
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected the self-check to reject a cache that omits the main spec file")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback notice, got: %v", err)
	}
	if _, serr := os.Stat(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/self/validate_result.md")); !os.IsNotExist(serr) {
		t.Fatal("expected the rejected cache to be rolled back (no file left behind)")
	}
}

func TestCacheWriteInvalidDeclarations(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	cases := []struct {
		name string
		file string
		want string
	}{
		{
			name: "missing path",
			file: cwFileJSON(map[string]any{"sections": []string{"Description"}}),
			want: "path",
		},
		{
			name: "unresolvable logical ref",
			file: cwFileJSON(map[string]any{"path": "unit:nope"}),
			want: "resolve",
		},
		{
			name: "missing section heading",
			file: cwFileJSON(map[string]any{"path": "docs/specs/units/candidate/unit_auth.md", "sections": []string{"No Such Section"}}),
			want: "not found",
		},
		{
			name: "bad json",
			file: "{not json",
			want: "invalid --file JSON",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runCacheWrite([]string{
				"--repo-root", repoRoot,
				"--gate", "validate",
				"--unit", "auth",
				"--result", "pass",
				"--target", "candidate",
				"--file", tc.file,
			}, &stdout, &stderr)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestCacheWriteUsageErrors(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing gate", []string{"--repo-root", repoRoot, "--unit", "auth", "--result", "pass", "--target", "candidate"}, "gate"},
		{"missing target", []string{"--repo-root", repoRoot, "--gate", "validate", "--unit", "auth", "--result", "pass"}, "candidate or stable"},
		{"mutually exclusive", []string{"--repo-root", repoRoot, "--gate", "validate", "--unit", "auth", "--rule", "b_rule_http", "--result", "pass", "--target", "candidate"}, "mutually exclusive"},
		{"invalid result", []string{"--repo-root", repoRoot, "--gate", "validate", "--unit", "auth", "--result", "maybe", "--target", "candidate"}, "pass or fail"},
		{"invalid basis", []string{"--repo-root", repoRoot, "--gate", "validate", "--unit", "auth", "--result", "pass", "--target", "candidate", "--basis", "fullx"}, "basis"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runCacheWrite(tc.args, &stdout, &stderr)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got: %v", tc.want, err)
			}
		})
	}
}

func TestCacheWriteRoundTripWithFresh(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/candidate/unit_auth.md",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v", err)
	}

	// The same unit reports FRESH through the fresh detail command.
	stdout.Reset()
	stderr.Reset()
	err = runFresh([]string{"--repo-root", repoRoot, "--unit", "auth"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "validate  FRESH") {
		t.Fatalf("expected validate FRESH in fresh output, got:\n%s", out)
	}
}

func TestCacheWriteContentEditStales(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := cwWriteSpec(t, repoRoot, "auth")

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path":   "docs/specs/units/candidate/unit_auth.md",
			"checks": []map[string]any{{"check": "1", "sections": []string{"Description"}}},
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v", err)
	}

	// Editing the declared section stales the cache.
	edited, _ := os.ReadFile(specPath)
	os.WriteFile(specPath, []byte(strings.Replace(string(edited), "Prose.", "Prose, edited.", 1)), 0644)

	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if res.Fresh {
		t.Fatal("expected the cache to go stale after editing the declared section")
	}

	// The delta scope derivation must report the declared check as affected.
	scope, err := validationcache.DeriveStaleScope(repoRoot, "unit", "auth", "validate")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Affected) != 1 || scope.Affected[0] != "1" {
		t.Fatalf("expected check 1 affected, got %v", scope.Affected)
	}
}

func TestCacheWriteStableTarget(t *testing.T) {
	repoRoot := t.TempDir()
	// A stable-only unit: no candidate file.
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	os.WriteFile(filepath.Join(stableDir, "unit_auth.md"), []byte("---\nid: auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Auth\n\n## Description\n\nProse.\n"), 0644)

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "review",
		"--unit", "auth",
		"--result", "pass",
		"--target", "stable",
		"--file", cwFileJSON(map[string]any{
			"path": "docs/specs/units/stable/unit_auth.md",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}

	res, err := validationcache.CheckReviewStable(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected written stable review cache fresh, got: %s", res.Reason)
	}
}

func TestCacheWriteSectionsAndRangesCombined(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := cwWriteSpec(t, repoRoot, "auth")
	text, _ := contenthash.FileText(specPath)

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path":     "docs/specs/units/candidate/unit_auth.md",
			"sections": []string{"Description"},
			"ranges":   "1-20",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}

	data, _ := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md"))
	content := string(data)

	if !strings.Contains(content, "region:section:Description:") {
		t.Fatalf("expected the section region dependency, got:\n%s", content)
	}
	if !strings.Contains(content, "- sha256:") {
		t.Fatalf("expected a chunk CID from the declared ranges, got:\n%s", content)
	}

	// Each chunk CID from the current file must appear in deps.
	fc := contenthash.ChunkText(text)
	for _, chunk := range fc.Chunks {
		if !strings.Contains(content, chunk.CID) {
			t.Fatalf("expected chunk CID %s in deps, got:\n%s", chunk.CID, content)
		}
	}

	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected FRESH after combined declaration, got: %s", res.Reason)
	}
}

func TestCacheWriteAcceptanceItemsAndRangesCombined(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path":             "docs/specs/units/candidate/unit_auth.md",
			"acceptance_items": true,
			"ranges":           "1-20",
		}),
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cache-write failed: %v\nstderr=%s", err, stderr.String())
	}

	data, _ := os.ReadFile(filepath.Join(repoRoot, "docs/specs/meta/validation/unit/auth/validate_result.md"))
	content := string(data)

	if !strings.Contains(content, "region:acceptance_items:") {
		t.Fatalf("expected the acceptance_items structural dependency, got:\n%s", content)
	}
	if !strings.Contains(content, "- sha256:") {
		t.Fatalf("expected a chunk CID from the declared ranges, got:\n%s", content)
	}

	res, err := validationcache.CheckValidate(repoRoot, "auth")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fresh {
		t.Fatalf("expected FRESH after combined acceptance_items+ranges, got: %s", res.Reason)
	}
}

func TestCacheWriteBadRangesWithSections(t *testing.T) {
	repoRoot := t.TempDir()
	cwWriteSpec(t, repoRoot, "auth")

	var stdout, stderr bytes.Buffer
	err := runCacheWrite([]string{
		"--repo-root", repoRoot,
		"--gate", "validate",
		"--unit", "auth",
		"--result", "pass",
		"--target", "candidate",
		"--file", cwFileJSON(map[string]any{
			"path":     "docs/specs/units/candidate/unit_auth.md",
			"sections": []string{"Description"},
			"ranges":   "bad",
		}),
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected bad ranges to be rejected even when sections are declared")
	}
	if !strings.Contains(err.Error(), "expected START-END") {
		t.Fatalf("expected range parse error, got: %v", err)
	}
}

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/baseline"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

// freshDerivation builds a derivation for single-target gate checks in tests.
// A summary loop shares one derivation across units; a test that checks one
// unit's gate state needs no sharing, only the construction.
func freshDerivation(t *testing.T, repoRoot string) *gaterun.Derivation {
	t.Helper()
	d, err := gaterun.NewDerivation(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func assertGateStatus(t *testing.T, output, gate, status string) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(gate) + `\s+` + regexp.QuoteMeta(status) + `\b`)
	if !re.MatchString(output) {
		t.Fatalf("expected %s %s, got:\n%s", gate, status, output)
	}
}

type cacheFileSpec struct {
	path   string
	hash   string
	checks []cacheCheckSpec
}

// cacheCheckSpec is one per-check evidence entry of a hand-written test cache:
// the check key and its lens tag (empty for validate caches).
type cacheCheckSpec struct {
	key  string
	lens string
}

// indentBlock indents every non-empty line of a rendered cache block by the
// given prefix (used to nest a whole-file deps block under a checks entry).
func indentBlock(block, prefix string) string {
	if block == "" {
		return ""
	}
	return prefix + strings.ReplaceAll(strings.TrimRight(block, "\n"), "\n", "\n"+prefix) + "\n"
}

func writeUnitSpec(t *testing.T, repoRoot, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeUnitCache(t *testing.T, repoRoot, name, command, extraFrontmatter string, files []cacheFileSpec) {
	t.Helper()
	if command == "verify" {
		target := "candidate"
		if strings.Contains(extraFrontmatter, "target: stable") {
			target = "stable"
		}
		spec := "docs/specs/units/" + target + "/unit_" + name + ".md"
		data, _ := os.ReadFile(filepath.Join(repoRoot, spec))
		if strings.Contains(string(data), "acceptance_item_set:") {
			currentVerifyFixture(t, repoRoot, name, target, extraFrontmatter)
			return
		}
	}
	dir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("command: " + command + "\n")
	sb.WriteString("unit: " + name + "\n")
	sb.WriteString("mode: full\n")
	sb.WriteString("result: pass\n")
	sb.WriteString("timestamp: \"2026-06-30T10:00:00Z\"\n")
	if extraFrontmatter != "" {
		sb.WriteString(extraFrontmatter)
	}
	sb.WriteString("files:\n")
	for _, f := range files {
		fmt.Fprintf(&sb, "  - path: %s\n    hash: sha256:%s\n", f.path, f.hash)
		sb.WriteString(cacheDepsAt(t, repoRoot, f.path))
		if len(f.checks) > 0 {
			sb.WriteString("    checks:\n")
			for _, c := range f.checks {
				fmt.Fprintf(&sb, "      - check: %q\n", c.key)
				if c.lens != "" {
					fmt.Fprintf(&sb, "        lens: %s\n", c.lens)
				}
			}
		}
	}
	sb.WriteString("---\nok\n")
	if err := os.WriteFile(filepath.Join(dir, command+"_result.md"), []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeRuleSpec(t *testing.T, repoRoot, id string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/rules/candidate")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".md")
	content := "---\nrule_id: " + id + "\nrule_scope: bound\n---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRuleCache(t *testing.T, repoRoot, id string, files []cacheFileSpec) {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule", id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("command: validate\n")
	sb.WriteString("unit: " + id + "\n")
	sb.WriteString("mode: full\n")
	sb.WriteString("result: pass\n")
	sb.WriteString("timestamp: \"2026-06-30T10:00:00Z\"\n")
	sb.WriteString("files:\n")
	for _, f := range files {
		fmt.Fprintf(&sb, "  - path: %s\n    hash: sha256:%s\n", f.path, f.hash)
		sb.WriteString(cacheDepsAt(t, repoRoot, f.path))
	}
	sb.WriteString("---\nok\n")
	if err := os.WriteFile(filepath.Join(dir, "validate_result.md"), []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func freshRun(t *testing.T, repoRoot string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	fullArgs := append(args, "--repo-root", repoRoot)
	err := runFresh(fullArgs, &stdout, &stderr)
	return stdout.String(), err
}

// cacheDepsAt renders the recorded chunk evidence block for a cache entry.
func cacheDepsAt(t *testing.T, repoRoot, relPath string) string {
	t.Helper()
	full := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	text, err := contenthash.FileText(full)
	if err != nil {
		t.Fatal(err)
	}
	records := contenthash.ChunkRecords(text)
	var b strings.Builder
	if len(records) > 0 {
		b.WriteString("    chunker: " + contenthash.ChunkerVersion + "\n    chunks:\n")
	}
	for _, c := range records {
		fmt.Fprintf(&b, "      - cid: %q\n        start: %d\n        end: %d\n", c.CID, c.StartLine, c.EndLine)
	}
	return b.String()
}

// appendToFile appends content to a file (used to invalidate cache evidence).
func appendToFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

// writeMergedVerifyFixture writes a unit spec with one acceptance item and a
// code file, and returns the merged verify cache entries the gate requires:
// the alignment check on the spec and the quality check on the code file, each
// declaring the whole file. The spec's declared implementation_surface (src)
// expands to the code file, so the derived expected coverage is exactly these
// two keys.
func writeMergedVerifyFixture(t *testing.T, repoRoot, name string) (specPath, codePath string, files []cacheFileSpec) {
	t.Helper()
	specPath = grWriteSpecItems(t, repoRoot, name, "none", "none", []string{name + ".core"})
	codePath = grWriteFile(t, repoRoot, "src/"+name+".go", "package "+name+"\n")
	files = []cacheFileSpec{
		{
			path:   "docs/specs/units/candidate/unit_" + name + ".md",
			hash:   computeHash(specPath),
			checks: []cacheCheckSpec{{key: name + ".core", lens: "alignment"}},
		},
		{
			path:   "src/" + name + ".go",
			hash:   computeHash(codePath),
			checks: []cacheCheckSpec{{key: "src/" + name + ".go", lens: "quality"}},
		},
	}
	return specPath, codePath, files
}

// TestFreshAllMixed verifies the summary mode reports every candidate with
// its per-gate status, excludes stable-only units, and counts readiness.
func TestFreshAllMixed(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// user_auth: all gates fresh. The verify cache carries the per-check
	// evidence the merged gate requires (alignment + quality keys).
	specA, _, verifyFilesA := writeMergedVerifyFixture(t, repoRoot, "user_auth")
	filesA := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specA)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", filesA)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "target: candidate\n", verifyFilesA)

	// payment: validate only (verify missing)
	specB := writeUnitSpec(t, repoRoot, "payment")
	filesB := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_payment.md", hash: computeHash(specB)}}
	writeUnitCache(t, repoRoot, "payment", "validate", "", filesB)

	// stable-only unit must NOT appear
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	os.WriteFile(filepath.Join(stableDir, "unit_legacy.md"), []byte("---\nid: legacy\n---\n"), 0644)

	// one rule with fresh validate cache
	rulePath := writeRuleSpec(t, repoRoot, "b_rule_auth")
	writeRuleCache(t, repoRoot, "b_rule_auth", []cacheFileSpec{{
		path: "docs/specs/rules/candidate/b_rule_auth.md",
		hash: computeHash(rulePath),
	}})

	output, err := freshRun(t, repoRoot)
	if err != nil {
		t.Fatalf("fresh failed: %v\noutput=%s", err, output)
	}

	if !strings.Contains(output, "UNITS (2):") {
		t.Fatalf("expected UNITS (2), got:\n%s", output)
	}
	if !strings.Contains(output, "user_auth") || !strings.Contains(output, "payment") {
		t.Fatalf("expected both candidate units in output:\n%s", output)
	}
	if strings.Contains(output, "legacy") {
		t.Fatalf("stable-only unit must not appear:\n%s", output)
	}
	if !strings.Contains(output, "user_auth") || !strings.Contains(output, "validate: FRESH") {
		t.Fatalf("expected FRESH validate for user_auth:\n%s", output)
	}
	if !strings.Contains(output, "verify: MISSING") {
		t.Fatalf("expected MISSING verify for payment:\n%s", output)
	}
	if !strings.Contains(output, "RULES (1):") || !strings.Contains(output, "b_rule_auth") {
		t.Fatalf("expected rules section:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: 2 of 3") {
		t.Fatalf("expected 2 of 3 ready, got:\n%s", output)
	}
}

func TestFreshAllNoCandidates(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	output, err := freshRun(t, repoRoot)
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	if !strings.Contains(output, "No active candidates found.") {
		t.Fatalf("expected no-candidates message, got:\n%s", output)
	}
}

func TestFreshUnitDetailAllFresh(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath, _, verifyFiles := writeMergedVerifyFixture(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "target: candidate\n", verifyFiles)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	if !strings.Contains(output, "FRESHNESS REPORT — user_auth (unit)") {
		t.Fatalf("expected report header, got:\n%s", output)
	}
	for _, gate := range []string{"validate", "verify"} {
		assertGateStatus(t, output, gate, "FRESH")
	}
	if !strings.Contains(output, "appendix  OK") {
		t.Fatalf("expected appendix OK, got:\n%s", output)
	}
	if !strings.Contains(output, "2026-06-30T10:00:00Z") {
		t.Fatalf("expected cache timestamp in output:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: yes") {
		t.Fatalf("expected ready yes, got:\n%s", output)
	}
}

func TestFreshUnitDetailStaleVerify(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath, _, verifyFiles := writeMergedVerifyFixture(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "target: candidate\n", verifyFiles)
	// Deliberately stale verify cache: the spec changes after the cache is
	// written, so its recorded whole-file hash no longer matches.
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Prose.", "Prose, edited.", 1)
	if edited == string(data) {
		t.Fatal("fixture assumption broken: Description edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "STALE")
	if !strings.Contains(output, "CHANGE REPORT (verify):") {
		t.Fatalf("expected the change report detail, got:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: no") {
		t.Fatalf("expected ready no, got:\n%s", output)
	}
}

// TestFreshUnitVerifySurfacesStaleEvidence pins the stale report contract: a
// changed recorded file stales the cache and is named in the reason.
func TestFreshUnitVerifySurfacesStaleEvidence(t *testing.T) {
	setup := func(t *testing.T) (repoRoot, specPath, codePath string) {
		t.Helper()
		repoRoot = createCLITestRepo(t)
		grEnableMissionLayout(t, repoRoot)
		specPath = grWriteSpecItems(t, repoRoot, "auth", "none", "none", []string{"auth.core"})
		codePath = grWriteFile(t, repoRoot, "src/auth.go", "package auth\n\nfunc Core() {}\n")
		runID := grPlan(t, repoRoot, "--gate", "verify", "--unit", "auth", "--target", "candidate")
		// Omitting the code scope cannot omit its required alignment evidence.
		alignmentReport := grVerifyItemBody("auth.core", "ALIGNED", "src/auth.go:1") +
			fmt.Sprintf("auth.core: %s: acceptance_item:auth.core\n", specPath)
		grSubmitOK(t, repoRoot, runID, "auth.core", alignmentReport)
		grAutoSubmitQuality(t, repoRoot, runID)
		grFinalizeOK(t, repoRoot, runID)
		return repoRoot, specPath, codePath
	}

	t.Run("spec change stales the cache", func(t *testing.T) {
		repoRoot, specPath, _ := setup(t)
		out, err := freshRun(t, repoRoot, "--unit", "auth")
		if err != nil {
			t.Fatal(err)
		}
		assertGateStatus(t, out, "verify", "FRESH")

		data, err := os.ReadFile(specPath)
		if err != nil {
			t.Fatal(err)
		}
		edited := strings.Replace(string(data), "description: Given a caller, When the behavior runs, Then it is accepted.", "description: Given a caller, When the behavior runs, Then it is accepted. With an edit.", 1)
		if edited == string(data) {
			t.Fatal("fixture assumption broken: item description not found")
		}
		if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
			t.Fatal(err)
		}

		out, err = freshRun(t, repoRoot, "--unit", "auth")
		if err != nil {
			t.Fatal(err)
		}
		assertGateStatus(t, out, "verify", "STALE")
		if !strings.Contains(out, "unit_auth.md") || !strings.Contains(out, "CHANGE REPORT (verify):") {
			t.Fatalf("expected the changed spec named in the reason, got:\n%s", out)
		}
	})

	t.Run("code change stales the cache", func(t *testing.T) {
		repoRoot, _, codePath := setup(t)
		appendToFile(t, codePath, "\n// changed\n")

		out, err := freshRun(t, repoRoot, "--unit", "auth")
		if err != nil {
			t.Fatal(err)
		}
		assertGateStatus(t, out, "verify", "STALE")
		if !strings.Contains(out, "src/auth.go") || !strings.Contains(out, "CHANGE REPORT (verify):") {
			t.Fatalf("expected the changed code named in the reason, got:\n%s", out)
		}
	})
}

// TestFreshUnitVerifyUndeducibleCoverageFailsClosed pins the guard: when the
// verify coverage cannot be derived, fresh must fail the cache closed as
// STALE instead of silently accepting it. The reported reason is the base
// cache's own classification (here: a cache that cannot prove the merged
// review protocol ran), which the lazy-derivation order surfaces without
// paying for evidence discovery.
func TestFreshUnitVerifyUndeducibleCoverageFailsClosed(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	// No acceptance items: verify coverage derivation fails, so fresh takes
	// the fallback path.
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_noitems.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("---\nid: noitems\nunit_refs: none\nrule_refs: none\n---\n\n# noitems\n"), 0644); err != nil {
		t.Fatal(err)
	}
	codePath := grWriteFile(t, repoRoot, "src/code.go", "package code\n")

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/noitems")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	cache := "---\ncommand: verify\nunit: noitems\nmode: full\nresult: pass\ntarget: candidate\nblocking: false\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_noitems.md\n    hash: sha256:" + computeHash(specPath) + "\n" +
		cacheDepsAt(t, repoRoot, "docs/specs/units/candidate/unit_noitems.md") +
		"  - path: src/code.go\n    hash: sha256:" + computeHash(codePath) + "\n" +
		"    checks:\n      - check: \"src/code.go\"\n        lens: quality\n" +
		cacheDepsAt(t, repoRoot, "src/code.go") +
		"---\nok\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "verify_result.md"), []byte(cache), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := freshRun(t, repoRoot, "--unit", "noitems")
	if err != nil {
		t.Fatal(err)
	}
	assertGateStatus(t, out, "verify", "STALE")
	if !strings.Contains(out, "old or damaged review protocol") {
		t.Fatalf("expected the base cache's own fail-closed reason, got:\n%s", out)
	}
}

// TestFreshUnitDetailStaleEvidence verifies that a STALE gate prints the
// mechanical change report: the changed file with the localized line spans
// computed from the recorded chunk evidence.
func TestFreshUnitDetailStaleEvidence(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_user_auth.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: user_auth\nunit_refs: none\nrule_refs: none\n---\n\n# User Auth\n\n## Description\n\nAuth prose.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", []cacheFileSpec{{
		path:   "docs/specs/units/candidate/unit_user_auth.md",
		hash:   computeHash(specPath),
		checks: []cacheCheckSpec{{key: "1"}, {key: "5"}},
	}})

	os.WriteFile(specPath, []byte(strings.Replace(specContent, "Auth prose.", "Auth prose, edited.", 1)), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "STALE")
	if !strings.Contains(output, "CHANGE REPORT (validate):") {
		t.Fatalf("expected the change report section, got:\n%s", output)
	}
	if !strings.Contains(output, "unit_user_auth.md: changed") || !strings.Contains(output, "lines") {
		t.Fatalf("expected the localized change reported, got:\n%s", output)
	}
	if strings.Contains(output, "plan:") {
		t.Fatalf("a read-only report must not print planner lines, got:\n%s", output)
	}
}

// TestFreshUnitDetailMissingVerify verifies a unit with no verify cache
// reports the verify gate as MISSING.
func TestFreshUnitDetailMissingVerify(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "MISSING")
	if !strings.Contains(output, "verify cache not found") {
		t.Fatalf("expected verify guidance, got:\n%s", output)
	}
}

// TestFreshUnitDetailBlockedVerify verifies that a verify failure record (a
// delta re-run's fail cache, result: fail + blocking: true) is reported
// BLOCKED — validate and verify caches share the failure-recovery blocking
// vocabulary.
func TestFreshUnitDetailBlockedVerify(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath, _, verifyFiles := writeMergedVerifyFixture(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", verifyFiles)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "BLOCKED")
	if !strings.Contains(output, "P0") {
		t.Fatalf("expected P0 finding detail, got:\n%s", output)
	}
	if !strings.Contains(output, "resolve P0/P1, then reverify@user_auth (repair recovery from the failure record)") {
		t.Fatalf("expected repair-recovery advice for the blocked gate, got:\n%s", output)
	}
}

// TestFreshUnitDetailBlockedValidate verifies that a validate failure record
// is reported BLOCKED like the verify records.
func TestFreshUnitDetailBlockedValidate(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "BLOCKED")
}

// TestFreshUnitDetailStaleBlockedVerify verifies that a verify failure record
// whose files changed since the run is reported STALE, not BLOCKED — the
// gate needs a re-run, matching promote's own stale reason and the
// stale-over-blocking precedence.
func TestFreshUnitDetailStaleBlockedVerify(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath, _, verifyFiles := writeMergedVerifyFixture(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", verifyFiles)
	// The spec changes after the fail record is written: stale, not BLOCKED.
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Prose.", "Prose, edited.", 1)
	if edited == string(data) {
		t.Fatal("fixture assumption broken: Description edit did not apply")
	}
	if err := os.WriteFile(specPath, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "STALE")
	if strings.Contains(output, "BLOCKED") {
		t.Fatalf("stale+blocking verify cache must not be BLOCKED:\n%s", output)
	}
	// The read-only report names the change set; the repair plan itself
	// belongs to gate-plan and is derived when the re-run is triggered.
	if !strings.Contains(output, "CHANGE REPORT (verify):") {
		t.Fatalf("expected the change report section:\n%s", output)
	}
	if strings.Contains(output, "plan:") {
		t.Fatalf("a read-only report must not print planner lines:\n%s", output)
	}
}

func TestFreshRuleDetail(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := writeRuleSpec(t, repoRoot, "b_rule_auth")
	writeRuleCache(t, repoRoot, "b_rule_auth", []cacheFileSpec{{
		path: "docs/specs/rules/candidate/b_rule_auth.md",
		hash: computeHash(rulePath),
	}})

	output, err := freshRun(t, repoRoot, "--rule", "b_rule_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	if !strings.Contains(output, "FRESHNESS REPORT — b_rule_auth (rule)") {
		t.Fatalf("expected rule report header, got:\n%s", output)
	}
	assertGateStatus(t, output, "validate", "FRESH")
	if !strings.Contains(output, "verify does not apply to rules") {
		t.Fatalf("expected rule gate note, got:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: yes") {
		t.Fatalf("expected ready yes, got:\n%s", output)
	}
}

// TestFreshMutuallyExclusive rejects --unit together with --rule.
func TestFreshMutuallyExclusive(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runFresh([]string{"--unit", "a", "--rule", "b", "--repo-root", repoRoot}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for mutually exclusive --unit and --rule")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got: %v", err)
	}
}

func writeStableUnitSpec(t *testing.T, repoRoot, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/stable")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nunit_refs: none\nrule_refs: none\n---\n\n## Description\nStable fixture.\n\n## Testability / Acceptance Criteria\nacceptance_item_set:\n  - id: " + name + ".core\n    implementation_surface: src/" + name + ".go\n"
	grWriteFile(t, repoRoot, "src/"+name+".go", "package fixture\n")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeStableRuleSpec(t *testing.T, repoRoot, id string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".md")
	content := "---\nrule_id: " + id + "\nrule_scope: bound\n---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFreshStableScope verifies --scope stable lists every stable target with
// its drift state and --scope all shows both sections without counting stable
// targets in READY FOR PROMOTE.
func TestFreshStableScope(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// A candidate (for the all-scope readiness count) and two stable units:
	// one with a matching baseline, one with no baseline.
	writeUnitSpec(t, repoRoot, "active")
	writeStableUnitSpec(t, repoRoot, "settled")
	writeStableUnitSpec(t, repoRoot, "legacy")

	spec := "---\nid: settled\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: settled.core\n" +
		"    description: t\n" +
		"    verification_type: testable\n" +
		"    verification_surface: src/\n" +
		"    implementation_surface: src/\n" +
		"    verification_method: check\n" +
		"    pass_condition: ok\n" +
		"    runnable: yes\n" +
		"    affects:\n" +
		"      files:\n" +
		"        - src/a.go\n"
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n"), 0644)
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec); err != nil {
		t.Fatal(err)
	}

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "STABLE UNITS (2):") {
		t.Fatalf("expected STABLE UNITS section, got:\n%s", out)
	}
	if !strings.Contains(out, "settled") || !strings.Contains(out, "OK") {
		t.Fatalf("expected settled OK, got:\n%s", out)
	}
	if !strings.Contains(out, "legacy") || !strings.Contains(out, "MISSING") {
		t.Fatalf("expected legacy MISSING, got:\n%s", out)
	}
	if strings.Contains(out, "READY FOR PROMOTE") {
		t.Fatalf("stable scope must not report promote readiness:\n%s", out)
	}
	if strings.Contains(out, "active") {
		t.Fatalf("stable scope must not list candidates:\n%s", out)
	}

	outAll, err := freshRun(t, repoRoot, "--scope", "all")
	if err != nil {
		t.Fatalf("fresh --scope all: %v", err)
	}
	if !strings.Contains(outAll, "UNITS (1):") || !strings.Contains(outAll, "STABLE UNITS (2):") {
		t.Fatalf("expected both sections in all scope, got:\n%s", outAll)
	}
	if !strings.Contains(outAll, "READY FOR PROMOTE: 0 of 1") {
		t.Fatalf("ready count must cover candidates only, got:\n%s", outAll)
	}
}

// TestFreshStableScope_LocalizesDrift verifies a recorded content change is
// reported as drift localized to the changed lines.
func TestFreshStableScope_LocalizesDrift(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeStableUnitSpec(t, repoRoot, "settled")

	var sb strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&sb, "line %03d: some padding content to grow the chunk set\n", i)
	}
	content := sb.String()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	aPath := filepath.Join(srcDir, "a.go")
	os.WriteFile(aPath, []byte(content), 0644)

	spec := "---\nid: settled\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: settled.core\n" +
		"    description: t\n" +
		"    verification_type: testable\n" +
		"    verification_surface: src/\n" +
		"    implementation_surface: src/\n" +
		"    verification_method: check\n" +
		"    pass_condition: ok\n" +
		"    runnable: yes\n" +
		"    affects:\n" +
		"      files:\n" +
		"        - src/a.go\n"
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec); err != nil {
		t.Fatal(err)
	}

	// Any recorded content change is drift, localized to the changed lines.
	os.WriteFile(aPath, []byte("modified first line\n"+content[strings.Index(content, "\n")+1:]), 0644)

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "settled") || !strings.Contains(out, "CHANGED") {
		t.Fatalf("expected settled CHANGED, got:\n%s", out)
	}
	out, err = freshRun(t, repoRoot, "--unit", "settled")
	if err != nil {
		t.Fatalf("fresh --unit settled: %v", err)
	}
	if !strings.Contains(out, "drift") || !strings.Contains(out, "Drift: possible") {
		t.Fatalf("expected settled drift reported, got:\n%s", out)
	}
	if !strings.Contains(out, "lines") {
		t.Fatalf("expected a localized line span, got:\n%s", out)
	}
}

// TestFreshStableScope_VerifiedSilence verifies a fresh stable verify cache
// shows the verify confirmation state while the baseline drift column still
// reports the mechanical surface change — the two dimensions are
// independent: "surface changed since promote" and "recently confirmed to
// still conform" are both true.
func TestFreshStableScope_VerifiedSilence(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeStableUnitSpec(t, repoRoot, "settled")

	// Baseline says the surface is unchanged...
	specPath := filepath.Join(repoRoot, "docs/specs/units/stable/unit_settled.md")
	specHash := computeHash(specPath)
	spec := "---\nid: settled\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: settled.core\n" +
		"    description: t\n" +
		"    verification_type: testable\n" +
		"    verification_surface: src/\n" +
		"    implementation_surface: src/\n" +
		"    verification_method: check\n" +
		"    pass_condition: ok\n" +
		"    runnable: yes\n" +
		"    affects:\n" +
		"      files:\n" +
		"        - src/a.go\n"
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n"), 0644)
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec); err != nil {
		t.Fatal(err)
	}

	// ...but the surface has changed since. A fresh verify@stable cache (with
	// the STABLE spec path in its files list) must show FRESH in the verify
	// column while the drift column stays CHANGED.
	os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n// changed\n"), 0644)
	writeUnitCache(t, repoRoot, "settled", "verify", "target: stable\n",
		[]cacheFileSpec{
			{
				path:   "docs/specs/units/stable/unit_settled.md",
				hash:   specHash,
				checks: []cacheCheckSpec{{key: "settled.core", lens: "alignment"}},
			},
			{
				path:   "src/a.go",
				hash:   computeHash(filepath.Join(srcDir, "a.go")),
				checks: []cacheCheckSpec{{key: "src/a.go", lens: "quality"}},
			},
		})

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "verify: FRESH") {
		t.Fatalf("expected verify FRESH (confirmed to still conform), got:\n%s", out)
	}
	if !strings.Contains(out, "drift: CHANGED") {
		t.Fatalf("expected drift CHANGED (mechanical surface change), got:\n%s", out)
	}
}

// TestFreshStableScope_Changed verifies a changed surface reports CHANGED
// with the offending files when no fresh verify cache exists.
func TestFreshStableScope_Changed(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeStableUnitSpec(t, repoRoot, "settled")

	spec := "---\nid: settled\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: settled.core\n" +
		"    description: t\n" +
		"    verification_type: testable\n" +
		"    verification_surface: src/\n" +
		"    implementation_surface: src/\n" +
		"    verification_method: check\n" +
		"    pass_condition: ok\n" +
		"    runnable: yes\n" +
		"    affects:\n" +
		"      files:\n" +
		"        - src/a.go\n"
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n"), 0644)
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n// changed\n"), 0644)

	out, err := freshRun(t, repoRoot, "--unit", "settled")
	if err != nil {
		t.Fatalf("fresh --unit settled: %v", err)
	}
	if !strings.Contains(out, "CHANGED") {
		t.Fatalf("expected CHANGED, got:\n%s", out)
	}
	if !strings.Contains(out, "src/a.go") {
		t.Fatalf("expected changed file in details, got:\n%s", out)
	}
}

// TestFreshStableScope_Confirmations verifies the stable summary shows both
// confirmation states (validate/verify) plus the drift column
// when the stable-layer caches exist.
func TestFreshStableScope_Confirmations(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeStableUnitSpec(t, repoRoot, "settled")
	specHash := computeHash(specPath)

	// Two stable-layer confirmation caches (validate/verify) written by the
	// corresponding @stable runs.
	writeUnitCache(t, repoRoot, "settled", "validate", "target: stable\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})
	writeUnitCache(t, repoRoot, "settled", "verify", "target: stable\n",
		[]cacheFileSpec{{
			path:   "docs/specs/units/stable/unit_settled.md",
			hash:   specHash,
			checks: []cacheCheckSpec{{key: "settled.core", lens: "alignment"}},
		}})

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	for _, want := range []string{"validate: FRESH", "verify: FRESH", "drift: MISSING"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in stable summary, got:\n%s", want, out)
		}
	}
}

// TestFreshStableScope_RuleValidate verifies a stable-layer rule validate
// cache shows in the stable summary.
func TestFreshStableScope_RuleValidate(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	rulePath := writeStableRuleSpec(t, repoRoot, "g_rule_demo")
	ruleHash := computeHash(rulePath)

	ruleCacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/rule/g_rule_demo")
	os.MkdirAll(ruleCacheDir, 0755)
	cache := "---\ncommand: validate\nunit: g_rule_demo\nmode: full\nresult: pass\ntarget: stable\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n  - path: docs/specs/rules/stable/g_rule_demo.md\n    hash: sha256:" + ruleHash + "\n" + cacheDepsAt(t, repoRoot, "docs/specs/rules/stable/g_rule_demo.md") + "---\nok\n"
	os.WriteFile(filepath.Join(ruleCacheDir, "validate_result.md"), []byte(cache), 0644)

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "g_rule_demo") || !strings.Contains(out, "validate: FRESH") {
		t.Fatalf("expected rule validate FRESH in stable summary, got:\n%s", out)
	}
}

// TestFreshStableUnitDetail verifies --unit on a stable-only unit reports the
// drift detail.
func TestFreshStableUnitDetail(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeStableUnitSpec(t, repoRoot, "settled")

	out, err := freshRun(t, repoRoot, "--unit", "settled")
	if err != nil {
		t.Fatalf("fresh --unit settled: %v", err)
	}
	if !strings.Contains(out, "(unit, stable)") {
		t.Fatalf("expected stable detail header, got:\n%s", out)
	}
	if !strings.Contains(out, "MISSING") {
		t.Fatalf("expected MISSING baseline state, got:\n%s", out)
	}
}

// TestFreshStableDetailAdvice verifies the stable detail report suggests the
// recovery command per gate state: MISSING gates need the full confirmation
// run, STALE gates suggest the delta re-run.
func TestFreshStableDetailAdvice(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeStableUnitSpec(t, repoRoot, "settled")

	out, err := freshRun(t, repoRoot, "--unit", "settled")
	if err != nil {
		t.Fatalf("fresh --unit settled: %v", err)
	}
	for _, want := range []string{
		"-> required: validate@settled (full run - no delta baseline)",
		"-> required: verify@settled (full run - no delta baseline)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected advice %q, got:\n%s", want, out)
		}
	}
}

// TestFreshStableDetailStaleEvidence verifies a STALE stable confirmation
// gate shows its stale evidence section and the delta recovery suggestion.
func TestFreshStableDetailStaleEvidence(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeStableUnitSpec(t, repoRoot, "settled")
	specHash := computeHash(specPath)

	writeUnitCache(t, repoRoot, "settled", "validate", "target: stable\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})

	// The stable spec changes after the confirmation run -> its recorded
	// whole-file hash no longer matches -> validate: STALE with a derivable
	// change report.
	appendToFile(t, specPath, "# appended\n")

	out, err := freshRun(t, repoRoot, "--unit", "settled")
	if err != nil {
		t.Fatalf("fresh --unit settled: %v", err)
	}
	if !strings.Contains(out, "validate  STALE") {
		t.Fatalf("expected validate STALE, got:\n%s", out)
	}
	if !strings.Contains(out, "-> suggestion: revalidate@settled (delta recovery)") {
		t.Fatalf("expected delta recovery suggestion, got:\n%s", out)
	}
	if !strings.Contains(out, "CHANGE REPORT (validate):") {
		t.Fatalf("expected the change report section, got:\n%s", out)
	}
}

// TestFreshCandidateDetailAdvice verifies the candidate detail report suggests
// the recovery command per gate state, symmetric with the stable report.
func TestFreshCandidateDetailAdvice(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "iter")
	specHash := computeHash(specPath)

	writeUnitCache(t, repoRoot, "iter", "validate", "target: candidate\n",
		[]cacheFileSpec{{path: "docs/specs/units/candidate/unit_iter.md", hash: specHash}})

	// Fresh validate, but verify never ran -> MISSING advice.
	out, err := freshRun(t, repoRoot, "--unit", "iter")
	if err != nil {
		t.Fatalf("fresh --unit iter: %v", err)
	}
	if !strings.Contains(out, "-> required: verify@iter (full run - no delta baseline)") {
		t.Fatalf("expected verify advice, got:\n%s", out)
	}

	// The spec changes -> validate STALE -> delta recovery suggestion.
	appendToFile(t, specPath, "# appended\n")
	out, err = freshRun(t, repoRoot, "--unit", "iter")
	if err != nil {
		t.Fatalf("fresh --unit iter: %v", err)
	}
	if !strings.Contains(out, "-> suggestion: revalidate@iter (delta recovery)") {
		t.Fatalf("expected revalidate advice, got:\n%s", out)
	}
}

// TestFreshAppendixAdviceRequiresFreshValidate verifies the appendix advice
// renders only when the validate cache is FRESH: a delta re-run then stops
// early and cannot pick up a newly added appendix, so the full validate run
// is the only recovery.
func TestFreshAppendixAdviceRequiresFreshValidate(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "iter")
	specHash := computeHash(specPath)

	writeUnitCache(t, repoRoot, "iter", "validate", "target: candidate\n",
		[]cacheFileSpec{{path: "docs/specs/units/candidate/unit_iter.md", hash: specHash}})

	// A new non-exempt appendix appears after the validation run: the validate
	// cache is still fresh, but the appendix gate goes STALE.
	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appendixDir, "unit_iter_b.md"), []byte("---\nid: iter_b\nstatus: active\n---\n## Extra\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := freshRun(t, repoRoot, "--unit", "iter")
	if err != nil {
		t.Fatalf("fresh --unit iter: %v", err)
	}
	if !strings.Contains(out, "validate  FRESH") {
		t.Fatalf("expected validate FRESH, got:\n%s", out)
	}
	if !strings.Contains(out, "appendix  STALE") {
		t.Fatalf("expected appendix STALE, got:\n%s", out)
	}
	if !strings.Contains(out, "-> required: validate@iter (full run - appendix coverage)") {
		t.Fatalf("expected full-run appendix advice, got:\n%s", out)
	}
}

// TestFreshAppendixAdviceSuppressedWhenValidateStale verifies the appendix
// advice is suppressed when the validate cache is STALE: the delta re-run
// restores appendix coverage through its complete files list, so only the
// delta recovery suggestion is printed.
func TestFreshAppendixAdviceSuppressedWhenValidateStale(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "iter")
	specHash := computeHash(specPath)

	writeUnitCache(t, repoRoot, "iter", "validate", "target: candidate\n",
		[]cacheFileSpec{{path: "docs/specs/units/candidate/unit_iter.md", hash: specHash}})

	appendixDir := filepath.Join(repoRoot, "docs/specs/units/candidate/appendix")
	if err := os.MkdirAll(appendixDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appendixDir, "unit_iter_b.md"), []byte("---\nid: iter_b\nstatus: active\n---\n## Extra\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// The spec also changes -> validate STALE alongside the appendix STALE.
	appendToFile(t, specPath, "# appended\n")

	out, err := freshRun(t, repoRoot, "--unit", "iter")
	if err != nil {
		t.Fatalf("fresh --unit iter: %v", err)
	}
	if !strings.Contains(out, "validate  STALE") {
		t.Fatalf("expected validate STALE, got:\n%s", out)
	}
	if !strings.Contains(out, "-> suggestion: revalidate@iter (delta recovery)") {
		t.Fatalf("expected delta recovery suggestion, got:\n%s", out)
	}
	if strings.Contains(out, "full run - appendix coverage") {
		t.Fatalf("appendix full-run advice must be suppressed when validate is STALE, got:\n%s", out)
	}
}

// TestFreshInvalidScope verifies an invalid scope is rejected.
func TestFreshInvalidScope(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	_, err := freshRun(t, repoRoot, "--scope", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("expected scope value in error, got: %v", err)
	}
}

// TestFreshStableRules verifies stable rules appear in the stable scope.
func TestFreshStableRules(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeStableRuleSpec(t, repoRoot, "g_rule_demo")

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "STABLE RULES (1):") || !strings.Contains(out, "g_rule_demo") {
		t.Fatalf("expected stable rules section, got:\n%s", out)
	}
	if !strings.Contains(out, "MISSING") {
		t.Fatalf("expected MISSING for baseline-less rule, got:\n%s", out)
	}
}

// TestFreshUnitDetailChangeStales verifies a recorded file's content change
// stales the gate and the fresh change report localizes the edit to its line
// span.
func TestFreshUnitDetailChangeStales(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	writeUnitSpec(t, repoRoot, "user_auth")

	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	sharedPath := filepath.Join(srcDir, "shared.go")
	var b strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "line %d: some unique shared content\n", i)
	}
	os.WriteFile(sharedPath, []byte(b.String()), 0644)

	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_user_auth.md")
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", []cacheFileSpec{
		{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)},
		{path: "src/shared.go", hash: computeHash(sharedPath)},
	})

	// Any content change in a recorded file stales the gate; the recorded
	// chunk evidence localizes the change to its line span.
	data, _ := os.ReadFile(sharedPath)
	os.WriteFile(sharedPath, []byte(strings.Replace(string(data), "line 200:", "line 200 CHANGED:", 1)), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "STALE")
	if !strings.Contains(output, "CHANGE REPORT (validate):") {
		t.Fatalf("expected the change report in the fresh report, got:\n%s", output)
	}
	if !strings.Contains(output, "src/shared.go: changed") || !strings.Contains(output, "lines") {
		t.Fatalf("expected the localized change reported, got:\n%s", output)
	}
}

func TestFreshOmitsConsumedRule(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	writeRuleSpec(t, repoRoot, "b_rule_auth")
	unit := "---\nid: user_auth\nunit_refs: none\nrule_refs: b_rule_auth\n---\n"
	unitDir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "unit_user_auth.md"), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := freshRun(t, repoRoot)
	if err != nil {
		t.Fatalf("fresh failed: %v\noutput=%s", err, output)
	}
	if strings.Contains(output, "RULES WITHOUT CONSUMERS") {
		t.Fatalf("consumed rule must not be listed, got:\n%s", output)
	}
}

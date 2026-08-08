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
)

func assertGateStatus(t *testing.T, output, gate, status string) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(gate) + `\s+` + regexp.QuoteMeta(status) + `\b`)
	if !re.MatchString(output) {
		t.Fatalf("expected %s %s, got:\n%s", gate, status, output)
	}
}

type cacheFileSpec struct {
	path string
	hash string
}

func writeUnitSpec(t *testing.T, repoRoot, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRetiringUnitSpec(t *testing.T, repoRoot, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot, "docs/specs/units/candidate")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unit_"+name+".md")
	content := "---\nid: " + name + "\nlayer: candidate\nstatus: retired\nversion: 1.0.0\nunit_refs: none\nrule_refs: none\n---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeUnitCache(t *testing.T, repoRoot, name, command, extraFrontmatter string, files []cacheFileSpec) {
	t.Helper()
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
	content := "---\nrule_id: " + id + "\nrule_scope: bound\nlayer: candidate\nrule_version: 0.1.0\n---\n"
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

func fullUnitSpecPaths(repoRoot, name string) []cacheFileSpec {
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate/unit_"+name+".md")
	return []cacheFileSpec{{
		path: "docs/specs/units/candidate/unit_" + name + ".md",
		hash: computeHash(specPath),
	}}
}

// TestFreshAllMixed verifies the summary mode reports every candidate with
// its per-gate status, excludes stable-only units, and counts readiness.
func TestFreshAllMixed(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// user_auth: all gates fresh
	specA := writeUnitSpec(t, repoRoot, "user_auth")
	filesA := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specA)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", filesA)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "target: candidate\n", filesA)
	writeUnitCache(t, repoRoot, "user_auth", "review", "blocking: false\np0_count: 0\np1_count: 0\n", filesA)

	// payment: validate only (verify/review missing)
	specB := writeUnitSpec(t, repoRoot, "payment")
	filesB := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_payment.md", hash: computeHash(specB)}}
	writeUnitCache(t, repoRoot, "payment", "validate", "", filesB)

	// stable-only unit must NOT appear
	stableDir := filepath.Join(repoRoot, "docs/specs/units/stable")
	os.MkdirAll(stableDir, 0755)
	os.WriteFile(filepath.Join(stableDir, "unit_legacy.md"), []byte("---\nid: legacy\nlayer: stable\n---\n"), 0644)

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
	if !strings.Contains(output, "review: MISSING") {
		t.Fatalf("expected MISSING review for payment:\n%s", output)
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
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "target: candidate\n", files)
	writeUnitCache(t, repoRoot, "user_auth", "review", "blocking: false\np0_count: 0\np1_count: 0\n", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	if !strings.Contains(output, "FRESHNESS REPORT — user_auth (unit)") {
		t.Fatalf("expected report header, got:\n%s", output)
	}
	for _, gate := range []string{"validate", "verify", "review"} {
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
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	// Deliberately stale verify cache: wrong hash
	stale := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: "0000000000000000000000000000000000000000000000000000000000000000"}}
	writeUnitCache(t, repoRoot, "user_auth", "verify", "target: candidate\n", stale)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "STALE")
	if !strings.Contains(output, "files have changed") {
		t.Fatalf("expected changed-file detail, got:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: no") {
		t.Fatalf("expected ready no, got:\n%s", output)
	}
}

func TestFreshUnitDetailMissingReview(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "review", "MISSING")
	if !strings.Contains(output, "Review not completed") {
		t.Fatalf("expected review guidance, got:\n%s", output)
	}
}

func TestFreshUnitDetailBlockedReview(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "review", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "review", "BLOCKED")
	if !strings.Contains(output, "P0") {
		t.Fatalf("expected P0 finding detail, got:\n%s", output)
	}
}

// TestFreshUnitDetailStaleBlockedReview verifies that a review cache which
// declares blocking findings but is stale (files changed since the run) is
// reported STALE, not BLOCKED: the gate needs a re-run, matching promote's
// own stale reason for the same cache.
func TestFreshUnitDetailStaleBlockedReview(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	stale := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: "0000000000000000000000000000000000000000000000000000000000000000"}}
	writeUnitCache(t, repoRoot, "user_auth", "review", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", stale)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "review", "STALE")
	if !strings.Contains(output, "stale") {
		t.Fatalf("expected stale reason, got:\n%s", output)
	}
	if strings.Contains(output, "BLOCKED") {
		t.Fatalf("stale+blocking review cache must not be BLOCKED:\n%s", output)
	}
}

// TestFreshUnitDetailInvalidVerifyBlocking verifies that a verify cache
// declaring blocking (an invalid cache the agent never writes) is reported
// STALE, never BLOCKED — the BLOCKED vocabulary is review-only.
func TestFreshUnitDetailInvalidVerifyBlocking(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "STALE")
	if strings.Contains(output, "BLOCKED") {
		t.Fatalf("invalid verify cache must not be BLOCKED:\n%s", output)
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
	if !strings.Contains(output, "verify and review do not apply to rules") {
		t.Fatalf("expected rule gate note, got:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: yes") {
		t.Fatalf("expected ready yes, got:\n%s", output)
	}
}

func TestFreshRetiringUnit(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeRetiringUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	if !strings.Contains(output, "retiring") {
		t.Fatalf("expected retiring note, got:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: yes") {
		t.Fatalf("expected ready yes, got:\n%s", output)
	}
	if strings.Contains(output, "\nverify") {
		t.Fatalf("retiring unit must not report verify gate:\n%s", output)
	}
}

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
	content := "---\nid: " + name + "\nlayer: stable\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
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
	content := "---\nrule_id: " + id + "\nrule_scope: bound\nlayer: stable\nrule_version: 0.1.0\n---\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFreshStableScope verifies --scope stable lists every stable target with
// its drift state and --scope all shows both sections without counting stable
// targets in READY FOR PROMOTE.
func TestFreshStableScope(t *testing.T) {
	repoRoot := t.TempDir()

	// A candidate (for the all-scope readiness count) and two stable units:
	// one with a matching baseline, one with no baseline.
	writeUnitSpec(t, repoRoot, "active")
	writeStableUnitSpec(t, repoRoot, "settled")
	writeStableUnitSpec(t, repoRoot, "legacy")

	spec := "---\nid: settled\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: settled.core\n" +
		"    description: t\n" +
		"    verification_type: auto\n" +
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

// TestFreshStableScope_VerifiedSilence verifies a fresh stable verify cache
// silences the baseline comparison.
func TestFreshStableScope_VerifiedSilence(t *testing.T) {
	repoRoot := t.TempDir()
	writeStableUnitSpec(t, repoRoot, "settled")

	// Baseline says the surface is unchanged...
	specPath := filepath.Join(repoRoot, "docs/specs/units/stable/unit_settled.md")
	specHash := computeHash(specPath)
	spec := "---\nid: settled\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: settled.core\n" +
		"    description: t\n" +
		"    verification_type: auto\n" +
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
	// the STABLE spec path in its files list) must override the CHANGED state.
	os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n// changed\n"), 0644)
	writeUnitCache(t, repoRoot, "settled", "verify", "target: stable\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "VERIFIED") {
		t.Fatalf("expected VERIFIED (silenced by fresh verify cache), got:\n%s", out)
	}
	if strings.Contains(out, "CHANGED") {
		t.Fatalf("expected no CHANGED when verify cache is fresh, got:\n%s", out)
	}
}

// TestFreshStableScope_Changed verifies a changed surface reports CHANGED
// with the offending files when no fresh verify cache exists.
func TestFreshStableScope_Changed(t *testing.T) {
	repoRoot := t.TempDir()
	writeStableUnitSpec(t, repoRoot, "settled")

	spec := "---\nid: settled\nlayer: candidate\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
		"acceptance_item_set:\n" +
		"  - id: settled.core\n" +
		"    description: t\n" +
		"    verification_type: auto\n" +
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

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "CHANGED") {
		t.Fatalf("expected CHANGED, got:\n%s", out)
	}
	if !strings.Contains(out, "src/a.go") {
		t.Fatalf("expected changed file in details, got:\n%s", out)
	}
}

// TestFreshStableUnitDetail verifies --unit on a stable-only unit reports the
// drift detail.
func TestFreshStableUnitDetail(t *testing.T) {
	repoRoot := t.TempDir()
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

// TestFreshInvalidScope verifies an invalid scope is rejected.
func TestFreshInvalidScope(t *testing.T) {
	repoRoot := t.TempDir()
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
	repoRoot := t.TempDir()
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

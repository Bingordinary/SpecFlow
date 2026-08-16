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
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
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
	content := "---\nid: " + name + "\nstatus: retired\nversion: 1.0.0\nunit_refs: none\nrule_refs: none\n---\n"
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
		sb.WriteString(cacheDepsAt(t, repoRoot, f.path))
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
	content := "---\nrule_id: " + id + "\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
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

// cacheDepsAt renders a whole-file dependency block for a cache entry.
func cacheDepsAt(t *testing.T, repoRoot, relPath string) string {
	t.Helper()
	full := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	fc, err := contenthash.ChunkFile(full)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if len(fc.Chunks) > 0 {
		b.WriteString("    deps:\n")
	}
	for _, c := range fc.Chunks {
		fmt.Fprintf(&b, "      - %s\n", c.CID)
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
	writeUnitCache(t, repoRoot, "user_auth", "verify", "target: candidate\n", files)
	// Deliberately stale verify cache: the spec changes after the cache is written,
	// so the declared dependency chunk is gone.
	os.WriteFile(specPath, []byte("---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n// changed\n"), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "STALE")
	if !strings.Contains(output, "dependency chunks have changed") {
		t.Fatalf("expected changed-dependency detail, got:\n%s", output)
	}
	if !strings.Contains(output, "READY FOR PROMOTE: no") {
		t.Fatalf("expected ready no, got:\n%s", output)
	}
}

// TestFreshUnitDetailDeltaScope verifies that a STALE gate with per-check
// evidence prints the mechanism-derived delta scope: which checks declared
// the stale dependencies, the unclaimed entries, and the degradation state.
func TestFreshUnitDetailDeltaScope(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_user_auth.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# User Auth\n\n## Description\n\nAuth prose.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	specHash := computeHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, ok := contenthash.LocateSectionRegion(text, "Description")
	if !ok {
		t.Fatal("expected Description section")
	}
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, ok := contenthash.AcceptanceItemsRegion(text)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/user_auth")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: user_auth\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_user_auth.md\n    hash: sha256:" + specHash + "\n" +
		"    checks:\n" +
		"      - check: \"1\"\n        deps:\n          - " + descDep + "\n" +
		"      - check: \"5\"\n        deps:\n          - " + itemsDep + "\n" +
		"    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n" +
		"---\nok\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Edit only the Description section: check 1's dep goes stale, check 5's does not.
	os.WriteFile(specPath, []byte(strings.Replace(specContent, "Auth prose.", "Auth prose, edited.", 1)), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "STALE")
	if !strings.Contains(output, "DELTA SCOPE (validate):") {
		t.Fatalf("expected delta scope section, got:\n%s", output)
	}
	if !strings.Contains(output, "affected checks: 1") {
		t.Fatalf("expected check 1 affected, got:\n%s", output)
	}
	if strings.Contains(output, "affected checks: 5") {
		t.Fatalf("check 5 must stay unaffected, got:\n%s", output)
	}
	if !strings.Contains(output, "degrades: no") {
		t.Fatalf("expected no degradation, got:\n%s", output)
	}
}

// TestFreshUnitDetailDeltaScopeDegrades verifies the DELTA SCOPE output when
// every declared non-cross check is stale while the cross entry stays fresh:
// the cross-check's delta re-run is unconditional, so the report must say
// "degrades: yes" (the incremental re-run is a full re-run).
func TestFreshUnitDetailDeltaScopeDegrades(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_user_auth.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# User Auth\n\n## Description\n\nAuth prose.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n\n## Scope\n\nIn scope.\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	specHash := computeHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, ok := contenthash.LocateSectionRegion(text, "Description")
	if !ok {
		t.Fatal("expected Description section")
	}
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, ok := contenthash.AcceptanceItemsRegion(text)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)
	scopeRegion, ok := contenthash.LocateSectionRegion(text, "Scope")
	if !ok {
		t.Fatal("expected Scope section")
	}
	scopeDep := "region:section:Scope:" + contenthash.RegionCID(scopeRegion.Text)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/user_auth")
	os.MkdirAll(cacheDir, 0755)
	cacheContent := "---\ncommand: validate\nunit: user_auth\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_user_auth.md\n    hash: sha256:" + specHash + "\n" +
		"    checks:\n" +
		"      - check: \"1\"\n        deps:\n          - " + descDep + "\n" +
		"      - check: \"5\"\n        deps:\n          - " + itemsDep + "\n" +
		"      - check: cross\n        deps:\n          - " + scopeDep + "\n" +
		"    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n      - " + scopeDep + "\n" +
		"---\nok\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Edit the Description and item sections: checks 1 and 5 go stale; the
	// cross entry (Scope section) stays fresh.
	edited := strings.Replace(specContent, "Auth prose.", "Auth prose, edited.", 1)
	edited = strings.Replace(edited, "Passes.", "Passes promptly.", 1)
	os.WriteFile(specPath, []byte(edited), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "STALE")
	if !strings.Contains(output, "affected checks: 1, 5") {
		t.Fatalf("expected checks 1 and 5 affected (cross stays fresh), got:\n%s", output)
	}
	if !strings.Contains(output, "degrades: yes") {
		t.Fatalf("expected degradation (every declared non-cross check stale + cross always re-runs), got:\n%s", output)
	}
}

// TestFreshUnitDetailDeltaScopeAllUnclaimed verifies the DELTA SCOPE output
// when the cache carries per-check evidence but every stale dep is unclaimed
// (declare-heavy extras in the file-level union): the report must say
// "affected checks: none" instead of the legacy-cache wording.
func TestFreshUnitDetailDeltaScopeAllUnclaimed(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_user_auth.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# User Auth\n\n## Description\n\nAuth prose.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	specHash := computeHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, ok := contenthash.LocateSectionRegion(text, "Description")
	if !ok {
		t.Fatal("expected Description section")
	}
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)
	itemsRegion, ok := contenthash.AcceptanceItemsRegion(text)
	if !ok {
		t.Fatal("expected acceptance region")
	}
	itemsDep := "region:acceptance_items:" + contenthash.RegionCID(itemsRegion)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/user_auth")
	os.MkdirAll(cacheDir, 0755)
	// The items dep is a declare-heavy extra: it sits in the file-level union
	// but no check declared it.
	cacheContent := "---\ncommand: validate\nunit: user_auth\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_user_auth.md\n    hash: sha256:" + specHash + "\n" +
		"    checks:\n" +
		"      - check: \"1\"\n        deps:\n          - " + descDep + "\n" +
		"    deps:\n      - " + descDep + "\n      - " + itemsDep + "\n" +
		"---\nok\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Edit the acceptance items region (unclaimed by any check).
	os.WriteFile(specPath, []byte(strings.Replace(specContent, "Passes.", "Passes promptly.", 1)), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "STALE")
	if !strings.Contains(output, "affected checks: none") {
		t.Fatalf("expected 'affected checks: none', got:\n%s", output)
	}
	if strings.Contains(output, "no per-check evidence") {
		t.Fatalf("cache has per-check evidence — legacy wording must not appear, got:\n%s", output)
	}
	if !strings.Contains(output, "unclaimed entries:") {
		t.Fatalf("expected the unclaimed entry reported, got:\n%s", output)
	}
}

// TestFreshUnitDetailDeltaScopeUnionViolation verifies that a STALE gate
// whose cache violates the union discipline still prints its DELTA SCOPE
// section with the format error and the rewrite guidance.
func TestFreshUnitDetailDeltaScopeUnionViolation(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_user_auth.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# User Auth\n\n## Description\n\nAuth prose.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: auth.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
	specHash := computeHash(specPath)
	text, _ := contenthash.FileText(specPath)
	descRegion, ok := contenthash.LocateSectionRegion(text, "Description")
	if !ok {
		t.Fatal("expected Description section")
	}
	descDep := "region:section:Description:" + contenthash.RegionCID(descRegion.Text)

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/user_auth")
	os.MkdirAll(cacheDir, 0755)
	// The file-level deps union omits check 1's dep — a union violation that
	// fails the gate closed.
	cacheContent := "---\ncommand: validate\nunit: user_auth\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n" +
		"  - path: docs/specs/units/candidate/unit_user_auth.md\n    hash: sha256:" + specHash + "\n" +
		"    checks:\n" +
		"      - check: \"1\"\n        deps:\n          - " + descDep + "\n" +
		"    deps:\n      - sha256:0000000000000000000000000000000000000000000000000000000000000000\n" +
		"---\nok\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(cacheContent), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "STALE")
	if !strings.Contains(output, "DELTA SCOPE (validate):") {
		t.Fatalf("expected a delta scope section even for a union violation, got:\n%s", output)
	}
	if !strings.Contains(output, "cache format error") {
		t.Fatalf("expected the cache format error reported, got:\n%s", output)
	}
	if !strings.Contains(output, "re-run validate@user_auth") {
		t.Fatalf("expected rewrite guidance, got:\n%s", output)
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
	writeUnitCache(t, repoRoot, "user_auth", "review", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", files)
	// The spec changes after the review cache is written: stale, not BLOCKED.
	os.WriteFile(specPath, []byte("---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n// changed\n"), 0644)

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

// TestFreshUnitDetailBlockedVerify verifies that a verify failure record (a
// delta re-run's fail cache, result: fail + blocking: true) is reported
// BLOCKED — the failure-recovery design gives validate/verify caches the same
// blocking vocabulary review has.
func TestFreshUnitDetailBlockedVerify(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", files)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "BLOCKED")
	if !strings.Contains(output, "P0") {
		t.Fatalf("expected P0 finding detail, got:\n%s", output)
	}
	if !strings.Contains(output, "resolve P0/P1, then reverify@user_auth (delta recovery from the failure record)") {
		t.Fatalf("expected delta-recovery advice for the blocked gate, got:\n%s", output)
	}
}

// TestFreshUnitDetailBlockedValidate verifies that a validate failure record
// is reported BLOCKED like the verify and review records.
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
// gate needs a re-run, matching promote's own stale reason and the review
// gate's stale-over-blocking precedence.
func TestFreshUnitDetailStaleBlockedVerify(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")
	files := []cacheFileSpec{{path: "docs/specs/units/candidate/unit_user_auth.md", hash: computeHash(specPath)}}
	writeUnitCache(t, repoRoot, "user_auth", "validate", "", files)
	writeUnitCache(t, repoRoot, "user_auth", "verify", "blocking: true\nresult: fail\np0_count: 1\np1_count: 0\n", files)
	// The spec changes after the fail record is written: stale, not BLOCKED.
	os.WriteFile(specPath, []byte("---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n// changed\n"), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "verify", "STALE")
	if strings.Contains(output, "BLOCKED") {
		t.Fatalf("stale+blocking verify cache must not be BLOCKED:\n%s", output)
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
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n"
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
	content := "---\nrule_id: " + id + "\nrule_scope: bound\nrule_version: 0.1.0\n---\n"
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

	spec := "---\nid: settled\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
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
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec, nil); err != nil {
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

// TestFreshStableScope_OKWithNote verifies the drift report shows OK with an
// informational note when the code surface changed outside the declared
// dependency chunks.
func TestFreshStableScope_OKWithNote(t *testing.T) {
	repoRoot := t.TempDir()
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

	fc, err := contenthash.ChunkFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.Chunks) < 3 {
		t.Fatalf("test setup expected multiple chunks, got %d", len(fc.Chunks))
	}
	mid := fc.Chunks[len(fc.Chunks)/2]

	spec := "---\nid: settled\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
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
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec, map[string][]string{"src/a.go": {mid.CID}}); err != nil {
		t.Fatal(err)
	}

	// Change a region outside the declared dependency chunk.
	newline := strings.Index(content, "\n")
	os.WriteFile(aPath, []byte("modified first line\n"+content[newline+1:]), 0644)

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "settled") || !strings.Contains(out, "OK") {
		t.Fatalf("expected settled OK, got:\n%s", out)
	}
	out, err = freshRun(t, repoRoot, "--unit", "settled")
	if err != nil {
		t.Fatalf("fresh --unit settled: %v", err)
	}
	if !strings.Contains(out, "settled") || !strings.Contains(out, "drift") || !strings.Contains(out, "OK") {
		t.Fatalf("expected settled drift OK, got:\n%s", out)
	}
	if !strings.Contains(out, "content changed outside declared dependencies") {
		t.Fatalf("expected drift note, got:\n%s", out)
	}
}

// TestFreshStableScope_VerifiedSilence verifies a fresh stable verify cache
// shows the verify confirmation state while the baseline drift column still
// reports the mechanical surface change — the two dimensions are
// independent: "surface changed since promote" and "recently confirmed to
// still conform" are both true.
func TestFreshStableScope_VerifiedSilence(t *testing.T) {
	repoRoot := t.TempDir()
	writeStableUnitSpec(t, repoRoot, "settled")

	// Baseline says the surface is unchanged...
	specPath := filepath.Join(repoRoot, "docs/specs/units/stable/unit_settled.md")
	specHash := computeHash(specPath)
	spec := "---\nid: settled\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
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
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec, nil); err != nil {
		t.Fatal(err)
	}

	// ...but the surface has changed since. A fresh verify@stable cache (with
	// the STABLE spec path in its files list) must show FRESH in the verify
	// column while the drift column stays CHANGED.
	os.WriteFile(filepath.Join(srcDir, "a.go"), []byte("package main\n// changed\n"), 0644)
	writeUnitCache(t, repoRoot, "settled", "verify", "target: stable\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})

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
	repoRoot := t.TempDir()
	writeStableUnitSpec(t, repoRoot, "settled")

	spec := "---\nid: settled\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n" +
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
	if err := baseline.WriteUnitBaseline(repoRoot, "settled", spec, nil); err != nil {
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

// TestFreshStableScope_Confirmations verifies the stable summary shows all
// three confirmation states (validate/verify/review) plus the drift column
// when the stable-layer caches exist.
func TestFreshStableScope_Confirmations(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeStableUnitSpec(t, repoRoot, "settled")
	specHash := computeHash(specPath)

	// Three stable-layer confirmation caches (validate/verify/review) written
	// by the corresponding @stable runs. review caches require the blocking
	// field.
	writeUnitCache(t, repoRoot, "settled", "validate", "target: stable\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})
	writeUnitCache(t, repoRoot, "settled", "verify", "target: stable\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})
	writeUnitCache(t, repoRoot, "settled", "review", "target: stable\nblocking: false\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	for _, want := range []string{"validate: FRESH", "verify: FRESH", "review: FRESH", "drift: MISSING"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in stable summary, got:\n%s", want, out)
		}
	}
}

// TestFreshStableScope_ReviewSeparatesLayers verifies the stable summary's
// review column cannot be satisfied by a candidate review cache. During an
// active candidate round the shared review cache path holds the candidate
// cache (target: candidate); only a cache recorded with `target: stable` by an
// @stable review run proves the stable quality confirmation.
func TestFreshStableScope_ReviewSeparatesLayers(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeStableUnitSpec(t, repoRoot, "settled")
	specHash := computeHash(specPath)

	// An active candidate round exists (both files present) and the last
	// review run was a candidate review — its cache carries target: candidate.
	writeUnitSpec(t, repoRoot, "settled")
	writeUnitCache(t, repoRoot, "settled", "review", "target: candidate\nblocking: false\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})

	out, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if strings.Contains(out, "review: FRESH") {
		t.Fatalf("candidate review cache must not satisfy the stable review confirmation, got:\n%s", out)
	}

	// A stable review run overwrites the cache with target: stable -> the
	// stable confirmation state shows FRESH.
	writeUnitCache(t, repoRoot, "settled", "review", "target: stable\nblocking: false\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})

	out, err = freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh --scope stable: %v", err)
	}
	if !strings.Contains(out, "review: FRESH") {
		t.Fatalf("expected review FRESH after stable review run, got:\n%s", out)
	}
}

// TestFreshStableScope_RuleValidate verifies a stable-layer rule validate
// cache shows in the stable summary.
func TestFreshStableScope_RuleValidate(t *testing.T) {
	repoRoot := t.TempDir()
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

// TestFreshStableDetailAdvice verifies the stable detail report suggests the
// recovery command per gate state: MISSING gates need the full confirmation
// run, STALE gates suggest the delta re-run.
func TestFreshStableDetailAdvice(t *testing.T) {
	repoRoot := t.TempDir()
	writeStableUnitSpec(t, repoRoot, "settled")

	out, err := freshRun(t, repoRoot, "--unit", "settled")
	if err != nil {
		t.Fatalf("fresh --unit settled: %v", err)
	}
	for _, want := range []string{
		"-> required: validate@settled (full run - no delta baseline)",
		"-> required: verify@settled (full run - no delta baseline)",
		"-> required: review@settled (full run - no delta baseline)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected advice %q, got:\n%s", want, out)
		}
	}
}

// TestFreshStableDetailDeltaScope verifies a STALE stable confirmation gate
// shows its DELTA SCOPE section and the delta recovery suggestion.
func TestFreshStableDetailDeltaScope(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeStableUnitSpec(t, repoRoot, "settled")
	specHash := computeHash(specPath)

	writeUnitCache(t, repoRoot, "settled", "validate", "target: stable\n",
		[]cacheFileSpec{{path: "docs/specs/units/stable/unit_settled.md", hash: specHash}})

	// The stable spec changes after the confirmation run -> the declared
	// dependency chunk is gone -> validate: STALE with a derivable delta scope.
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
	if !strings.Contains(out, "DELTA SCOPE (validate):") {
		t.Fatalf("expected DELTA SCOPE section, got:\n%s", out)
	}
}

// TestFreshCandidateDetailAdvice verifies the candidate detail report suggests
// the recovery command per gate state, symmetric with the stable report.
func TestFreshCandidateDetailAdvice(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeUnitSpec(t, repoRoot, "iter")
	specHash := computeHash(specPath)

	writeUnitCache(t, repoRoot, "iter", "validate", "target: candidate\n",
		[]cacheFileSpec{{path: "docs/specs/units/candidate/unit_iter.md", hash: specHash}})

	// Fresh validate, but verify/review never ran -> MISSING advice.
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
	repoRoot := t.TempDir()
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
	repoRoot := t.TempDir()
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

// TestFreshUnitDetailShowsNote verifies the informational note about content
// changed outside the declared dependency chunks reaches the fresh report —
// the promote-gate check prints it, and the fresh report must too (the note
// is the agent's signal that semantic coupling may exist).
func TestFreshUnitDetailShowsNote(t *testing.T) {
	repoRoot := createCLITestRepo(t)
	specPath := writeUnitSpec(t, repoRoot, "user_auth")

	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	sharedPath := filepath.Join(srcDir, "shared.go")
	var b strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "line %d: some unique shared content\n", i)
	}
	os.WriteFile(sharedPath, []byte(b.String()), 0644)

	// Dependency evidence: whole-file for the spec, lines 1-10 only for the
	// shared file.
	fcSpec, _ := contenthash.ChunkFile(specPath)
	var specDeps []string
	for _, c := range fcSpec.Chunks {
		specDeps = append(specDeps, c.CID)
	}
	fcShared, _ := contenthash.ChunkFile(sharedPath)
	sharedDeps := contenthash.CIDsForRanges(fcShared, [][2]int{{1, 10}})
	if len(sharedDeps) == 0 {
		t.Fatal("expected dependency chunks for lines 1-10")
	}

	cacheDir := filepath.Join(repoRoot, "docs/specs/meta/validation/unit/user_auth")
	os.MkdirAll(cacheDir, 0755)
	var sb strings.Builder
	sb.WriteString("---\ncommand: validate\nunit: user_auth\nmode: full\nresult: pass\ntimestamp: \"2026-06-30T10:00:00Z\"\nfiles:\n")
	fmt.Fprintf(&sb, "  - path: docs/specs/units/candidate/unit_user_auth.md\n    hash: sha256:%s\n", computeHash(specPath))
	sb.WriteString(depsBlock(specDeps))
	fmt.Fprintf(&sb, "  - path: src/shared.go\n    hash: sha256:%s\n", computeHash(sharedPath))
	sb.WriteString(depsBlock(sharedDeps))
	sb.WriteString("---\nok\n")
	os.WriteFile(filepath.Join(cacheDir, "validate_result.md"), []byte(sb.String()), 0644)

	// A change far outside the declared dependency range (line 200).
	data, _ := os.ReadFile(sharedPath)
	os.WriteFile(sharedPath, []byte(strings.Replace(string(data), "line 200:", "line 200 CHANGED:", 1)), 0644)

	output, err := freshRun(t, repoRoot, "--unit", "user_auth")
	if err != nil {
		t.Fatalf("fresh failed: %v", err)
	}
	assertGateStatus(t, output, "validate", "FRESH")
	if !strings.Contains(output, "content changed outside the declared dependency chunks") {
		t.Fatalf("expected the informational note in the fresh report, got:\n%s", output)
	}
}

// depsBlock renders a deps block for a cache file entry.
func depsBlock(deps []string) string {
	if len(deps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("    deps:\n")
	for _, d := range deps {
		fmt.Fprintf(&b, "      - %s\n", d)
	}
	return b.String()
}

func TestFreshEmbedsUnboundRulesStableScope(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// An unbound rule with no retention declaration must appear in the
	// stable view's removal-candidate list.
	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	orphan := "---\nrule_id: b_rule_orphan\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Orphan\n"
	if err := os.WriteFile(filepath.Join(stableRuleDir, "b_rule_orphan.md"), []byte(orphan), 0644); err != nil {
		t.Fatal(err)
	}
	// A retained rule must not appear.
	retained := "---\nrule_id: b_rule_kept\nrule_scope: bound\nrule_version: 0.1.0\nunbound_retention: intentional\n---\n\n# Kept\n"
	if err := os.WriteFile(filepath.Join(stableRuleDir, "b_rule_kept.md"), []byte(retained), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := freshRun(t, repoRoot, "--scope", "stable")
	if err != nil {
		t.Fatalf("fresh failed: %v\noutput=%s", err, output)
	}
	if !strings.Contains(output, "RULES WITHOUT CONSUMERS") {
		t.Fatalf("expected removal-candidate list, got:\n%s", output)
	}
	if !strings.Contains(output, "b_rule_orphan") {
		t.Fatalf("expected b_rule_orphan in the list, got:\n%s", output)
	}
	listIdx := strings.Index(output, "RULES WITHOUT CONSUMERS")
	if strings.Contains(output[listIdx:], "b_rule_kept") {
		t.Fatalf("retained rule must not be listed, got:\n%s", output)
	}
}

func TestFreshEmbedsUnboundRulesCandidateScope(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	writeRuleSpec(t, repoRoot, "b_rule_orphan")

	output, err := freshRun(t, repoRoot)
	if err != nil {
		t.Fatalf("fresh failed: %v\noutput=%s", err, output)
	}
	if !strings.Contains(output, "RULES WITHOUT CONSUMERS") {
		t.Fatalf("expected removal-candidate list in the candidate view, got:\n%s", output)
	}
	if !strings.Contains(output, "b_rule_orphan") {
		t.Fatalf("expected b_rule_orphan in the list, got:\n%s", output)
	}
}

func TestFreshOmitsConsumedRule(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	writeRuleSpec(t, repoRoot, "b_rule_auth")
	unit := "---\nid: user_auth\nversion: 0.1.0\nunit_refs: none\nrule_refs: b_rule_auth\n---\n"
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

func TestFreshEmbedsUnboundRulesLayerIndependent(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// A stable-only removable rule must appear in the default (candidate)
	// scope too — the removal-candidate list is layer-independent.
	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	orphan := "---\nrule_id: b_rule_orphan\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Orphan\n"
	if err := os.WriteFile(filepath.Join(stableRuleDir, "b_rule_orphan.md"), []byte(orphan), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := freshRun(t, repoRoot)
	if err != nil {
		t.Fatalf("fresh failed: %v\noutput=%s", err, output)
	}
	if !strings.Contains(output, "RULES WITHOUT CONSUMERS") {
		t.Fatalf("expected removal-candidate list in the default candidate view, got:\n%s", output)
	}
	if !strings.Contains(output, "b_rule_orphan") {
		t.Fatalf("expected b_rule_orphan in the list, got:\n%s", output)
	}
}

func TestFreshAllScopeListsOnce(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// A rule with files in both layers is a single removal candidate — the
	// all-scope report must list it exactly once, not once per layer.
	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	rule := "---\nrule_id: b_rule_orphan\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Orphan\n"
	if err := os.WriteFile(filepath.Join(stableRuleDir, "b_rule_orphan.md"), []byte(rule), 0644); err != nil {
		t.Fatal(err)
	}
	writeRuleSpec(t, repoRoot, "b_rule_orphan")

	output, err := freshRun(t, repoRoot, "--scope", "all")
	if err != nil {
		t.Fatalf("fresh failed: %v\noutput=%s", err, output)
	}
	if count := strings.Count(output, "RULES WITHOUT CONSUMERS"); count != 1 {
		t.Fatalf("expected the list exactly once in the all-scope report, got %d:\n%s", count, output)
	}
	listIdx := strings.Index(output, "RULES WITHOUT CONSUMERS")
	if count := strings.Count(output[listIdx:], "b_rule_orphan"); count != 1 {
		t.Fatalf("expected b_rule_orphan exactly once in the list, got %d:\n%s", count, output)
	}
}

func TestFreshEmptyCandidateStillListsRules(t *testing.T) {
	repoRoot := createCLITestRepo(t)

	// No candidate files at all — the default report still ends with the
	// removal-candidate list instead of hiding it behind the empty layer.
	stableRuleDir := filepath.Join(repoRoot, "docs/specs/rules/stable")
	if err := os.MkdirAll(stableRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	orphan := "---\nrule_id: b_rule_orphan\nrule_scope: bound\nrule_version: 0.1.0\n---\n\n# Orphan\n"
	if err := os.WriteFile(filepath.Join(stableRuleDir, "b_rule_orphan.md"), []byte(orphan), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := freshRun(t, repoRoot)
	if err != nil {
		t.Fatalf("fresh failed: %v\noutput=%s", err, output)
	}
	if !strings.Contains(output, "No active candidates found.") {
		t.Fatalf("expected the empty-candidate notice, got:\n%s", output)
	}
	if !strings.Contains(output, "RULES WITHOUT CONSUMERS") {
		t.Fatalf("expected the removal-candidate list despite the empty candidate layer, got:\n%s", output)
	}
}

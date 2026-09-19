package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

func TestGateEvidenceBasic(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	path := filepath.Join(srcDir, "auth.go")
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "line %d: some unique content\n", i)
	}
	os.WriteFile(path, []byte(b.String()), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "1-20"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "file: src/auth.go") {
		t.Fatalf("expected file line, got:\n%s", out)
	}
	if !strings.Contains(out, "hash: sha256:") {
		t.Fatalf("expected hash line, got:\n%s", out)
	}
	if !strings.Contains(out, "deps:") {
		t.Fatalf("expected deps block, got:\n%s", out)
	}
	if !strings.Contains(out, "- sha256:") {
		t.Fatalf("expected at least one dependency CID, got:\n%s", out)
	}
}

func TestGateEvidenceWholeFile(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	path := filepath.Join(srcDir, "auth.go")
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "line %d: some unique content\n", i)
	}
	os.WriteFile(path, []byte(b.String()), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v", err)
	}
	// No ranges declared: the whole file is the dependency.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	depLines := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "  - sha256:") {
			depLines++
		}
	}
	if depLines == 0 {
		t.Fatalf("expected whole-file dependency CIDs, got:\n%s", stdout.String())
	}
}

func TestGateEvidenceRangeCoversContent(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "auth.go"), []byte("package main\n"), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "1-1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v", err)
	}
	out := stdout.String()
	// Range 1-1 covers line 1 ("package main") — must map to the file's
	// single chunk, so deps must not be empty.
	if !strings.Contains(out, "- sha256:") {
		t.Fatalf("expected dependency CIDs for a content range, got:\n%s", out)
	}
}

func TestGateEvidenceRangeOutOfBounds(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "auth.go"), []byte("package main\n"), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "99-100"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for out-of-bounds range")
	}
	if !strings.Contains(err.Error(), "exceeds the file's line count") {
		t.Fatalf("expected line-count error, got: %v", err)
	}
}

func TestGateEvidenceMissingFileFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing --file")
	}
}

func TestGateEvidenceMissingFile(t *testing.T) {
	repoRoot := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/nope.go"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestGateEvidenceMalformedRanges(t *testing.T) {
	repoRoot := t.TempDir()
	srcDir := filepath.Join(repoRoot, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "auth.go"), []byte("package main\n"), 0644)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "src/auth.go", "--ranges", "abc"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for malformed ranges")
	}
}

func TestGateEvidenceAcceptanceItemsRegion(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n## Description\n\nProse.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--acceptance-items"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "region:acceptance_items:sha256:") {
		t.Fatalf("expected a region:acceptance_items dep, got:\n%s", out)
	}

	// The region CID must equal the structural region CID.
	text, _ := contenthash.FileText(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dep.md"))
	region, ok := contenthash.AcceptanceItemsRegion(text)
	if !ok {
		t.Fatal("expected region")
	}
	expected := "region:acceptance_items:" + contenthash.RegionCID(region)
	if !strings.Contains(out, expected) {
		t.Fatalf("expected dep %q in output, got:\n%s", expected, out)
	}
}

func TestGateEvidenceAcceptanceItemsMissingMarker(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	if err := os.WriteFile(specPath, []byte("---\nid: dep\n---\nNo items.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--acceptance-items"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when the acceptance_item_set marker is absent")
	}
	if !strings.Contains(err.Error(), "acceptance_item_set region not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGateEvidenceSections(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Dep Unit\n\n## Description\n\nProse about the unit.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--sections"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "sections:") {
		t.Fatalf("expected sections block, got:\n%s", out)
	}
	if !strings.Contains(out, "heading: frontmatter") {
		t.Fatalf("expected frontmatter region, got:\n%s", out)
	}
	if !strings.Contains(out, "heading: Description") {
		t.Fatalf("expected Description section, got:\n%s", out)
	}
	if !strings.Contains(out, "heading: Testability / Acceptance Criteria") {
		t.Fatalf("expected Testability section, got:\n%s", out)
	}
	if !strings.Contains(out, "lines: 1-9") {
		t.Fatalf("expected frontmatter region to end before the first ## heading, got:\n%s", out)
	}
	if !strings.Contains(out, "lines: 14-24") {
		t.Fatalf("expected the final section to end at the last real line, got:\n%s", out)
	}
	if strings.Contains(out, "lines: 14-25") {
		t.Fatalf("the artificial trailing newline must not be a line, got:\n%s", out)
	}
	// --sections is an informational probe: it must not declare anything.
	// A whole-file chunk dep would make the output usable as a declaration,
	// contradicting the documented "without declaring anything" semantics.
	if strings.Contains(out, "- sha256:") {
		t.Fatalf("expected no chunk deps from --sections alone, got:\n%s", out)
	}
	if !strings.Contains(out, "deps:\n") {
		t.Fatalf("expected an empty deps list, got:\n%s", out)
	}
}

func TestGateEvidenceSectionFrontmatterCollision(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	// A real section literally named "frontmatter" must fail the --section
	// frontmatter declaration — the spelling names the pre-heading region,
	// so the declaration would silently bind to the wrong region.
	if err := os.WriteFile(specPath, []byte("---\nid: dep\n---\n\n## frontmatter\n\nReal section prose.\n\n## Description\n\nProse.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "frontmatter"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected an error for the reserved frontmatter collision")
	}
	if !strings.Contains(err.Error(), "reserved heading") {
		t.Fatalf("expected reserved-heading guidance, got: %v", err)
	}
}

func TestGateEvidenceSectionFrontmatterDuplicatedCollision(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	// Two real "frontmatter" sections are still a real collision: presence,
	// not uniqueness, decides the reserved-spelling rejection.
	if err := os.WriteFile(specPath, []byte("---\nid: dep\n---\n\n## frontmatter\n\nFirst.\n\n## frontmatter\n\nSecond.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "frontmatter"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected an error for the duplicated reserved frontmatter collision")
	}
	if !strings.Contains(err.Error(), "reserved heading") {
		t.Fatalf("expected reserved-heading guidance, got: %v", err)
	}
}

func TestGateEvidenceSectionFrontmatterUnstructuredSpec(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	// A spec with no ## heading cannot be declared by section: the frontmatter
	// spelling would silently alias the whole file.
	if err := os.WriteFile(specPath, []byte("---\nid: dep\n---\n\n# Dep\n\nProse without sections.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "frontmatter"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected a section declaration on a no-## spec to fail closed")
	}
	if !strings.Contains(err.Error(), "cannot be declared") {
		t.Fatalf("expected no-##-heading guidance, got: %v", err)
	}
}

func TestGateEvidenceSection(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Dep Unit\n\n## Description\n\nProse about the unit.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "Testability / Acceptance Criteria"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()

	text, _ := contenthash.FileText(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dep.md"))
	region, ok := contenthash.LocateSectionRegion(text, "Testability / Acceptance Criteria")
	if !ok {
		t.Fatal("expected section region")
	}
	expected := "region:section:Testability / Acceptance Criteria:" + contenthash.RegionCID(region.Text)
	if !strings.Contains(out, expected) {
		t.Fatalf("expected dep %q in output, got:\n%s", expected, out)
	}
}

func TestGateEvidenceSectionWithRangesUnion(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Dep Unit\n\n## Description\n\nProse about the unit.\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "Description", "--ranges", "1-1"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "region:section:Description:sha256:") {
		t.Fatalf("expected the section region dep, got:\n%s", out)
	}
	if !strings.Contains(out, "- sha256:") {
		t.Fatalf("expected chunk CIDs from the ranges part, got:\n%s", out)
	}
}

func TestGateEvidenceSectionMissingHeading(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	if err := os.WriteFile(specPath, []byte("---\nid: dep\n---\n\n## Description\n\nProse.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "No Such Section"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for a missing section heading")
	}
	if !strings.Contains(err.Error(), "section \"No Such Section\" not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGateEvidenceSectionDuplicatedHeading(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	if err := os.WriteFile(specPath, []byte("---\nid: dep\n---\n\n## Notes\n\nFirst.\n\n## Notes\n\nSecond.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "Notes"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for a duplicated section heading")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGateEvidenceSectionFrontmatter(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	specContent := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Dep Unit\n\n## Description\n\nProse about the unit.\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--section", "frontmatter"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()

	text, _ := contenthash.FileText(filepath.Join(repoRoot, "docs/specs/units/candidate/unit_dep.md"))
	region, ok := contenthash.LocateSectionRegion(text, "")
	if !ok {
		t.Fatal("expected frontmatter region")
	}
	expected := "region:section::" + contenthash.RegionCID(region.Text)
	if !strings.Contains(out, expected) {
		t.Fatalf("expected the frontmatter dep %q in output, got:\n%s", expected, out)
	}
}

const gateEvidenceTwoItemSpec = "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Dep Unit\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n\n  - id: dep.aux\n    description: Aux.\n    verification_type: testable\n    verification_surface: api\n    implementation_surface: src\n    verification_method: test\n    pass_condition: Passes.\n    runnable: yes\n"

func writeGateEvidenceTwoItemSpec(t *testing.T, repoRoot string) string {
	t.Helper()
	rel := "docs/specs/units/candidate/unit_dep.md"
	specPath := filepath.Join(repoRoot, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(specPath), 0755)
	if err := os.WriteFile(specPath, []byte(gateEvidenceTwoItemSpec), 0644); err != nil {
		t.Fatal(err)
	}
	return specPath
}

func TestGateEvidenceAcceptanceItem(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeGateEvidenceTwoItemSpec(t, repoRoot)

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--acceptance-item", "dep.aux"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	text, _ := contenthash.FileText(specPath)
	region, ok := contenthash.LocateAcceptanceItemRegion(text, "dep.aux")
	if !ok {
		t.Fatal("expected dep.aux region")
	}
	expected := "region:acceptance_item:dep.aux:" + contenthash.RegionCID(region.Text)
	if !strings.Contains(stdout.String(), expected) {
		t.Fatalf("expected dep %q in output, got:\n%s", expected, stdout.String())
	}
}

func TestGateEvidenceAcceptanceItemMissingAndDuplicate(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeGateEvidenceTwoItemSpec(t, repoRoot)

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--acceptance-item", "dep.none"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for a missing item id")
	}
	if !strings.Contains(err.Error(), "dep.none") {
		t.Fatalf("unexpected error: %v", err)
	}

	dup := strings.Replace(gateEvidenceTwoItemSpec, "- id: dep.aux", "- id: dep.core", 1)
	if err := os.WriteFile(specPath, []byte(dup), 0644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--acceptance-item", "dep.core"}, &stdout, &stderr); err == nil {
		t.Fatal("expected error for a duplicated item id")
	}
}

func TestGateEvidenceAcceptanceItemUnionWithWholeSet(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeGateEvidenceTwoItemSpec(t, repoRoot)

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--acceptance-items", "--acceptance-item", "dep.core"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	text, _ := contenthash.FileText(specPath)
	setRegion, ok := contenthash.AcceptanceItemsRegion(text)
	if !ok {
		t.Fatal("expected the whole-set region")
	}
	coreRegion, ok := contenthash.LocateAcceptanceItemRegion(text, "dep.core")
	if !ok {
		t.Fatal("expected dep.core region")
	}
	for _, want := range []string{
		"region:acceptance_items:" + contenthash.RegionCID(setRegion),
		"region:acceptance_item:dep.core:" + contenthash.RegionCID(coreRegion.Text),
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected dep %q in output, got:\n%s", want, stdout.String())
		}
	}
}

func TestGateEvidenceItemsAcrossSubheading(t *testing.T) {
	// The set region ends at the next `##` heading; a `###` subheading inside
	// the section is content and must not hide the items after it.
	repoRoot := t.TempDir()
	specPath := filepath.Join(repoRoot, "docs/specs/units/candidate", "unit_dep.md")
	os.MkdirAll(filepath.Dir(specPath), 0755)
	spec := "---\nid: dep\nversion: 0.1.0\nunit_refs: none\nrule_refs: none\n---\n\n# Dep Unit\n\n## Testability / Acceptance Criteria\n\nacceptance_item_set:\n  - id: dep.core\n    description: Core.\n\n### Extra structure\n\n  - id: dep.aux\n    description: Aux.\n\n## Dependencies\n\nNone.\n"
	if err := os.WriteFile(specPath, []byte(spec), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--items"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence --items failed: %v\nstderr=%s", err, stderr.String())
	}
	text, _ := contenthash.FileText(specPath)
	for _, id := range []string{"dep.core", "dep.aux"} {
		region, ok := contenthash.LocateAcceptanceItemRegion(text, id)
		if !ok {
			t.Fatalf("expected region for %s", id)
		}
		if !strings.Contains(stdout.String(), "  - id: "+id) {
			t.Fatalf("expected id %q in listing, got:\n%s", id, stdout.String())
		}
		if !strings.Contains(stdout.String(), "    cid: "+contenthash.RegionCID(region.Text)) {
			t.Fatalf("expected cid for %q in listing, got:\n%s", id, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	err = runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--acceptance-item", "dep.aux"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence --acceptance-item failed: %v\nstderr=%s", err, stderr.String())
	}
	auxRegion, ok := contenthash.LocateAcceptanceItemRegion(text, "dep.aux")
	if !ok {
		t.Fatal("expected dep.aux region")
	}
	expected := "region:acceptance_item:dep.aux:" + contenthash.RegionCID(auxRegion.Text)
	if !strings.Contains(stdout.String(), expected) {
		t.Fatalf("expected dep %q in output, got:\n%s", expected, stdout.String())
	}
}

func TestGateEvidenceItemsListing(t *testing.T) {
	repoRoot := t.TempDir()
	specPath := writeGateEvidenceTwoItemSpec(t, repoRoot)

	var stdout, stderr bytes.Buffer
	err := runGateEvidence([]string{"--repo-root", repoRoot, "--file", "docs/specs/units/candidate/unit_dep.md", "--items"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("gate-evidence failed: %v\nstderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "items:") {
		t.Fatalf("expected an items listing, got:\n%s", out)
	}
	text, _ := contenthash.FileText(specPath)
	for _, id := range []string{"dep.core", "dep.aux"} {
		region, ok := contenthash.LocateAcceptanceItemRegion(text, id)
		if !ok {
			t.Fatalf("expected region for %s", id)
		}
		if !strings.Contains(out, "  - id: "+id) {
			t.Fatalf("expected id %q in listing, got:\n%s", id, out)
		}
		if !strings.Contains(out, "    cid: "+contenthash.RegionCID(region.Text)) {
			t.Fatalf("expected cid for %q in listing, got:\n%s", id, out)
		}
	}
	if !strings.Contains(out, "deps:") {
		t.Fatalf("expected a deps block, got:\n%s", out)
	}
	depsIdx := strings.Index(out, "deps:")
	if strings.Contains(out[depsIdx:], "- sha256:") || strings.Contains(out[depsIdx:], "region:") {
		t.Fatalf("listing mode must not declare dependencies, got:\n%s", out)
	}
}

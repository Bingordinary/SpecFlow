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

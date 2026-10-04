package specvalidation

import (
	"strings"
	"testing"
)

// writeOwnershipSpec writes a candidate spec whose single acceptance item
// declares impl as implementation_surface and affectsFiles as affects.files.
func writeOwnershipSpec(t *testing.T, repoRoot, unitName string, unitRefs []string, impl string, affectsFiles []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\nid: " + unitName + "\n")
	if len(unitRefs) == 0 {
		b.WriteString("unit_refs: none\n")
	} else {
		b.WriteString("unit_refs:\n")
		for _, ref := range unitRefs {
			b.WriteString("  - " + ref + "\n")
		}
	}
	b.WriteString("rule_refs: none\n---\n\n")
	b.WriteString("acceptance_item_set:\n")
	b.WriteString("  - id: item_1\n")
	b.WriteString("    description: test\n")
	b.WriteString("    verification_type: testable\n")
	b.WriteString("    verification_surface: src/\n")
	b.WriteString("    implementation_surface: " + impl + "\n")
	b.WriteString("    verification_method: check\n")
	b.WriteString("    pass_condition: ok\n")
	b.WriteString("    runnable: yes\n")
	if len(affectsFiles) > 0 {
		b.WriteString("    affects:\n      files:\n")
		for _, f := range affectsFiles {
			b.WriteString("        - " + f + "\n")
		}
	}
	writeCandidate(t, repoRoot, unitName, b.String())
}

func findCheck(t *testing.T, checks []CheckResult, name string) CheckResult {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found", name)
	return CheckResult{}
}

func TestSurfaceAuditOverlapAndOutOfBounds(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "pkg/a.go", "package pkg")
	writeSurfaceFile(t, repoRoot, "pkg/b.go", "package pkg")

	writeOwnershipSpec(t, repoRoot, "alpha", nil, "pkg/a.go", []string{"pkg/b.go"})
	writeOwnershipSpec(t, repoRoot, "beta", nil, "pkg/b.go", nil)

	report, err := SurfaceAudit(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SharedFiles) != 1 || report.SharedFiles[0].Path != "pkg/b.go" {
		t.Fatalf("expected one overlap on pkg/b.go, got %+v", report.SharedFiles)
	}
	if len(report.SharedFiles[0].Decls) != 2 {
		t.Fatalf("expected two declarations, got %+v", report.SharedFiles[0].Decls)
	}

	if result := checkSurfaceAssociations(repoRoot, "alpha"); result.Status != Pass {
		t.Fatalf("expected alpha FAIL, got %s: %s", result.Status, result.Details)
	}
	if result := checkSurfaceAssociations(repoRoot, "beta"); result.Status != Pass {
		t.Fatalf("expected beta FAIL, got %s: %s", result.Status, result.Details)
	}
}

func TestSurfaceAuditOutOfBoundsDepDeclared(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "pkg/a.go", "package pkg")
	writeSurfaceFile(t, repoRoot, "pkg/b.go", "package pkg")

	writeOwnershipSpec(t, repoRoot, "alpha", []string{"beta"}, "pkg/a.go", []string{"pkg/b.go"})
	writeOwnershipSpec(t, repoRoot, "beta", nil, "pkg/b.go", nil)

	report, err := SurfaceAudit(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SharedFiles) != 1 {
		t.Fatalf("expected shared file with an explicit behavior dependency, got %+v", report)
	}
}

func TestSurfaceAuditDirectoryExpansionOverlap(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "pkg/a.go", "package pkg")
	writeSurfaceFile(t, repoRoot, "pkg/b.go", "package pkg")

	writeOwnershipSpec(t, repoRoot, "alpha", nil, "pkg", nil)
	writeOwnershipSpec(t, repoRoot, "beta", nil, "pkg/b.go", nil)

	report, err := SurfaceAudit(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Directories) != 1 || report.Directories[0].Path != "pkg" || report.Directories[0].FileCount != 2 {
		t.Fatalf("expected one directory declaration over 2 files, got %+v", report.Directories)
	}
	if len(report.SharedFiles) != 1 || report.SharedFiles[0].Path != "pkg/b.go" {
		t.Fatalf("expected one overlap on pkg/b.go, got %+v", report.SharedFiles)
	}
	var via string
	for _, decl := range report.SharedFiles[0].Decls {
		if decl.Unit == "alpha" {
			via = decl.Via
		}
	}
	if via != "pkg" {
		t.Fatalf("expected alpha declaration via pkg, got %q in %+v", via, report.SharedFiles[0].Decls)
	}
	if result := checkSurfaceAssociations(repoRoot, "alpha"); result.Status != Pass {
		t.Fatalf("expected FAIL for the directory-owning unit, got %s", result.Status)
	}
}

func TestSurfaceAuditSameUnitNoOverlap(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "pkg/a.go", "package pkg")

	writeOwnershipSpec(t, repoRoot, "alpha", nil, "pkg/a.go", []string{"pkg/a.go"})

	report, err := SurfaceAudit(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SharedFiles) != 0 {
		t.Fatalf("expected a clean report, got shared files=%+v", report.SharedFiles)
	}
	if len(report.Units) != 1 || len(report.Units[0].Files) != 1 {
		t.Fatalf("expected one deduplicated file, got %+v", report.Units)
	}
	if report.Units[0].Files[0].Field != SurfaceFieldImplementation {
		t.Fatalf("expected implementation_surface attribution, got %+v", report.Units[0].Files[0])
	}
	if result := checkSurfaceAssociations(repoRoot, "alpha"); result.Status != Pass {
		t.Fatalf("expected PASS, got %s: %s", result.Status, result.Details)
	}
}

func TestSurfaceAuditExcludesSpecPaths(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "docs/specs/units/stable/unit_shared.md", "placeholder")

	writeOwnershipSpec(t, repoRoot, "alpha", nil, "docs/specs/units/stable/unit_shared.md", nil)
	writeOwnershipSpec(t, repoRoot, "beta", nil, "docs/specs/units/stable/unit_shared.md", nil)

	report, err := SurfaceAudit(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SharedFiles) != 0 {
		t.Fatalf("spec-document paths must not participate in code ownership, got %+v", report.SharedFiles)
	}
}

func TestSurfaceAuditDisjointSurfacesPass(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "alpha/a.go", "package alpha")
	writeSurfaceFile(t, repoRoot, "beta/b.go", "package beta")

	writeOwnershipSpec(t, repoRoot, "alpha", nil, "alpha/a.go", nil)
	writeOwnershipSpec(t, repoRoot, "beta", nil, "beta/b.go", nil)

	if result := checkSurfaceAssociations(repoRoot, "alpha"); result.Status != Pass {
		t.Fatalf("expected PASS, got %s: %s", result.Status, result.Details)
	}
}

func TestValidateCandidateIncludesSurfaceOwnership(t *testing.T) {
	repoRoot := newRepo(t)
	createMinimalCandidate(t, repoRoot, "test_unit")

	result := ValidateCandidate(repoRoot, "test_unit")
	check := findCheck(t, result.Checks, "Surface associations")
	if check.Status != Pass {
		t.Fatalf("expected PASS on an empty surface, got %s: %s", check.Status, check.Details)
	}
}

func TestFormatSurfaceAudit(t *testing.T) {
	report := &SurfaceAuditReport{
		Units: []SurfaceUnitAudit{
			{Unit: "alpha", Layer: "candidate", Files: []SurfaceRef{{Unit: "alpha", Field: SurfaceFieldImplementation, Path: "pkg/a.go"}}},
		},
		SharedFiles: []SharedSurfaceFile{
			{Path: "pkg/b.go", Decls: []SurfaceRef{
				{Unit: "alpha", Field: SurfaceFieldAffects, Path: "pkg/b.go"},
				{Unit: "beta", Field: SurfaceFieldImplementation, Path: "pkg/b.go"},
			}},
		},
	}
	out := FormatSurfaceAudit(report)
	for _, want := range []string{"Surface associations", "pkg/a.go", "alpha (candidate)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output:\n%s", want, out)
		}
	}
}

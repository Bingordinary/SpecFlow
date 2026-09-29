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
	b.WriteString("    verification_type: auto\n")
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
	if len(report.Overlaps) != 1 || report.Overlaps[0].Path != "pkg/b.go" {
		t.Fatalf("expected one overlap on pkg/b.go, got %+v", report.Overlaps)
	}
	if len(report.Overlaps[0].Decls) != 2 {
		t.Fatalf("expected two declarations, got %+v", report.Overlaps[0].Decls)
	}
	if len(report.OutOfBounds) != 1 {
		t.Fatalf("expected one out-of-bounds entry, got %+v", report.OutOfBounds)
	}
	entry := report.OutOfBounds[0]
	if entry.Unit != "alpha" || entry.Entry != "pkg/b.go" || entry.Owner != "beta" || entry.DepDeclared {
		t.Fatalf("unexpected out-of-bounds entry: %+v", entry)
	}

	if result := checkSurfaceOwnership(repoRoot, "alpha"); result.Status != Fail {
		t.Fatalf("expected alpha FAIL, got %s: %s", result.Status, result.Details)
	}
	if result := checkSurfaceOwnership(repoRoot, "beta"); result.Status != Fail {
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
	if len(report.OutOfBounds) != 1 || !report.OutOfBounds[0].DepDeclared {
		t.Fatalf("expected declared dependency on beta, got %+v", report.OutOfBounds)
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
	if len(report.Overlaps) != 1 || report.Overlaps[0].Path != "pkg/b.go" {
		t.Fatalf("expected one overlap on pkg/b.go, got %+v", report.Overlaps)
	}
	var via string
	for _, decl := range report.Overlaps[0].Decls {
		if decl.Unit == "alpha" {
			via = decl.Via
		}
	}
	if via != "pkg" {
		t.Fatalf("expected alpha declaration via pkg, got %q in %+v", via, report.Overlaps[0].Decls)
	}
	if result := checkSurfaceOwnership(repoRoot, "alpha"); result.Status != Fail {
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
	if len(report.Overlaps) != 0 || len(report.OutOfBounds) != 0 {
		t.Fatalf("expected a clean report, got overlaps=%+v out-of-bounds=%+v", report.Overlaps, report.OutOfBounds)
	}
	if len(report.Units) != 1 || len(report.Units[0].Files) != 1 {
		t.Fatalf("expected one deduplicated file, got %+v", report.Units)
	}
	if report.Units[0].Files[0].Field != SurfaceFieldImplementation {
		t.Fatalf("expected implementation_surface attribution, got %+v", report.Units[0].Files[0])
	}
	if result := checkSurfaceOwnership(repoRoot, "alpha"); result.Status != Pass {
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
	if len(report.Overlaps) != 0 {
		t.Fatalf("spec-document paths must not participate in code ownership, got %+v", report.Overlaps)
	}
}

func TestSurfaceAuditDisjointSurfacesPass(t *testing.T) {
	repoRoot := newRepo(t)
	writeSurfaceFile(t, repoRoot, "alpha/a.go", "package alpha")
	writeSurfaceFile(t, repoRoot, "beta/b.go", "package beta")

	writeOwnershipSpec(t, repoRoot, "alpha", nil, "alpha/a.go", nil)
	writeOwnershipSpec(t, repoRoot, "beta", nil, "beta/b.go", nil)

	if result := checkSurfaceOwnership(repoRoot, "alpha"); result.Status != Pass {
		t.Fatalf("expected PASS, got %s: %s", result.Status, result.Details)
	}
}

func TestCheckSurfaceOwnershipRetiredSkip(t *testing.T) {
	repoRoot := newRepo(t)
	writeCandidate(t, repoRoot, "alpha",
		"---\nid: alpha\nunit_refs: none\nrule_refs: none\nstatus: retired\n---\n")

	result := checkSurfaceOwnership(repoRoot, "alpha")
	if result.Status != Pass || !strings.Contains(result.Details, "retired") {
		t.Fatalf("expected retired skip, got %s: %s", result.Status, result.Details)
	}
}

func TestValidateCandidateIncludesSurfaceOwnership(t *testing.T) {
	repoRoot := newRepo(t)
	createMinimalCandidate(t, repoRoot, "test_unit")

	result := ValidateCandidate(repoRoot, "test_unit")
	check := findCheck(t, result.Checks, "Surface ownership")
	if check.Status != Pass {
		t.Fatalf("expected PASS on an empty surface, got %s: %s", check.Status, check.Details)
	}
}

func TestFormatSurfaceAudit(t *testing.T) {
	report := &SurfaceAuditReport{
		Units: []SurfaceUnitAudit{
			{Unit: "alpha", Layer: "candidate", Files: []SurfaceRef{{Unit: "alpha", Field: SurfaceFieldImplementation, Path: "pkg/a.go"}}},
		},
		Overlaps: []SurfaceOverlap{
			{Path: "pkg/b.go", Decls: []SurfaceRef{
				{Unit: "alpha", Field: SurfaceFieldAffects, Path: "pkg/b.go"},
				{Unit: "beta", Field: SurfaceFieldImplementation, Path: "pkg/b.go"},
			}},
		},
		OutOfBounds: []SurfaceOutOfBounds{{Unit: "alpha", Entry: "pkg/b.go", Owner: "beta", DepDeclared: true}},
	}
	out := FormatSurfaceAudit(report)
	for _, want := range []string{"Surface ownership audit", "pkg/b.go", "alpha (affects.files)", "unit_refs declared"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output:\n%s", want, out)
		}
	}
}

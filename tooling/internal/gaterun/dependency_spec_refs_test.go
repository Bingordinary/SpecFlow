package gaterun

import (
	"testing"
	"time"
)

// A candidate validate must be able to compare the provider's current contract
// with the contract the provider published. The plan carries both: the logical
// unit ref resolves the candidate, and the physical stable main spec is the
// read-only comparison copy. The comparison copy is not a promise dependency
// pin: the current-layer promise remains the single dependency edge.
func TestCandidateValidateCarriesProviderStableComparisonSpec(t *testing.T) {
	repoRoot := newRepo(t)
	writeUnit(t, repoRoot, "candidate", "auth", "none", "none", "src/auth", "")
	writeUnit(t, repoRoot, "stable", "auth", "none", "none", "src/auth", "")
	writeUnit(t, repoRoot, "candidate", "web", "auth", "none", "src/web", "")

	run, err := Plan(repoRoot, GateValidate, TargetKindUnit, "web", TargetCandidate, ModeFull, nil, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"unit:auth", "docs/specs/units/stable/unit_auth.md"} {
		if _, ok := refByRef(run, want); !ok {
			t.Fatalf("candidate validate plan missing %q: %+v", want, run.Refs)
		}
	}
	for _, key := range []string{"structural", "dependencies"} {
		spec, err := BuildSessionSpec(repoRoot, run, []string{key})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"unit:auth", "docs/specs/units/stable/unit_auth.md"} {
			if !stringInSlice(spec.ReadRefs, want) {
				t.Fatalf("%s read refs = %v, want %q", key, spec.ReadRefs, want)
			}
		}
	}
}

// dependencySpecRefs feeds the structural and dependency validate groups. A
// candidate run's provider is a logical ref; a stable run's provider is a
// physical stable spec. Both must reach the groups, and an unrelated derived
// physical ref (the declared code surface) must not.
func TestDependencySpecRefsIncludesCrossObjectSpecs(t *testing.T) {
	run := &Run{
		OwnSpecFiles: []string{"docs/specs/units/stable/unit_web.md"},
		Refs: []Ref{
			{Ref: "docs/specs/units/stable/unit_web.md", Source: SourceDerived},
			{Ref: "docs/specs/units/stable/unit_auth.md", Source: SourceDerived},
			{Ref: "unit:other", Source: SourceDerived},
			{Ref: "rule:b_rule_http", Source: SourceDerived},
			{Ref: "src/web.go", Source: SourceDerived},
			{Ref: "src/web.go", Source: SourceDerivedAffects},
		},
	}
	got := dependencySpecRefs(run)
	want := []string{
		"docs/specs/units/stable/unit_auth.md",
		"unit:other",
		"rule:b_rule_http",
	}
	if len(got) != len(want) {
		t.Fatalf("dependencySpecRefs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dependencySpecRefs = %v, want %v", got, want)
		}
	}

	// The dependency group reads the stable provider spec; an unrelated group
	// (design) reads only the own spec and the declared surface.
	for _, group := range []string{"structural", "dependencies"} {
		read := unitValidateGroupReadRefs("", run, group)
		if !containsString(read, "docs/specs/units/stable/unit_auth.md") {
			t.Fatalf("%s group read refs = %v, want the stable provider spec", group, read)
		}
	}
	if read := unitValidateGroupReadRefs("", run, "design"); containsString(read, "docs/specs/units/stable/unit_auth.md") {
		t.Fatalf("design group must not read the provider spec, got %v", read)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

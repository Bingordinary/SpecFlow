package unitgraph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeUnit(t *testing.T, repoRoot, layer, name, ruleRefs string) {
	t.Helper()
	if ruleRefs == "" {
		ruleRefs = "none"
	}
	content := "---\nid: " + name + "\nrule_refs: " + ruleRefs + "\n---\n\n# " + name + "\n"
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_"+name+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(nodes []*Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, n.Name)
	}
	return out
}

func TestBuildNoCycle(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "b_rule_1")
	writeUnit(t, repo, "candidate", "beta", "g_rule_baseline")
	writeUnit(t, repo, "candidate", "gamma", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(g.Nodes()); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("nodes = %v", got)
	}
	if got := g.Node("alpha").RuleRefs; !reflect.DeepEqual(got, []string{"b_rule_1"}) {
		t.Fatalf("alpha rule refs = %v", got)
	}
	g.AddPromiseDependency("alpha", "beta")
	g.AddPromiseDependency("alpha", "gamma")
	g.AddPromiseDependency("gamma", "beta")
	if cycles := g.Cycles(); len(cycles) != 0 {
		t.Fatalf("cycles = %v, want none", cycles)
	}
	if g.OnCycle("alpha") || g.OnCycle("beta") || g.OnCycle("gamma") {
		t.Fatal("no unit should be on a cycle")
	}
	// beta must be promoted before alpha and gamma (dependency first).
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"beta"}, {"gamma"}, {"alpha"}}) {
		t.Fatalf("topo order = %v, want [[beta] [gamma] [alpha]]", got)
	}
}

func TestPublishedPromiseDependencyStillConstrainsOrder(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "")
	writeUnit(t, repo, "candidate", "beta", "")
	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddPromiseDependency("alpha", "beta")
	if cycles := g.Cycles(); len(cycles) != 0 {
		t.Fatalf("cycles = %v, want none", cycles)
	}
	if g.OnCycle("alpha") || g.OnCycle("beta") {
		t.Fatal("a published provider edge must not form a publication cycle")
	}
	// Publication state is an annotation, not the ordering basis: the provider
	// still comes first.
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"beta"}, {"alpha"}}) {
		t.Fatalf("topo order = %v, want [[beta] [alpha]] (published edge still constrains order)", got)
	}
	if got := g.Node("alpha").PromiseRefs; !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("all promise refs = %v, want [beta]", got)
	}
	if got := g.Node("alpha").UnpublishedRefs; len(got) != 0 {
		t.Fatalf("unpublished refs = %v, want none", got)
	}
	if got := g.PromiseConsumers("beta"); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("consumers = %v, want [alpha]", got)
	}
}

func TestMutualDependencyIsUnitCycleNotPublicationCycle(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "a", "")
	writeUnit(t, repo, "candidate", "b", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddPromiseDependency("a", "b")
	g.AddPromiseDependency("b", "a")
	// Unit-level mutual dependency through promise edges that need no
	// publication wait is a unit dependency cycle (a violation), not a
	// publication cycle: the promise-level graph stays acyclic.
	if cycles := g.Cycles(); len(cycles) != 0 {
		t.Fatalf("publication cycles = %v, want none", cycles)
	}
	if g.OnCycle("a") || g.OnCycle("b") {
		t.Fatal("a satisfied mutual dependency must not be a publication cycle")
	}
	unitCycles := g.UnitCycles()
	if len(unitCycles) != 1 || !reflect.DeepEqual(unitCycles[0], []string{"a", "b"}) {
		t.Fatalf("unit cycles = %v, want one cycle [a b]", unitCycles)
	}
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"a", "b"}}) {
		t.Fatalf("topo order = %v, want one invalid group [a b]", got)
	}
	if got := g.UnitCycleGroup("a"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unit cycle group = %v, want [a b]", got)
	}
}

func TestUnitCycleGroupIndependentUnit(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "solo", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := g.UnitCycleGroup("solo"); !reflect.DeepEqual(got, []string{"solo"}) {
		t.Fatalf("unit cycle group = %v, want [solo]", got)
	}
	if got := g.UnitCycleGroup("ghost"); got != nil {
		t.Fatalf("unit cycle group for unknown unit = %v, want nil", got)
	}
	if got := g.UnitCycles(); len(got) != 0 {
		t.Fatalf("unit cycles = %v, want none", got)
	}
}

func TestBuildTwoUnitCycle(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "auth", "")
	writeUnit(t, repo, "candidate", "payment", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddUnpublishedPromiseDependency("auth", "payment")
	g.AddUnpublishedPromiseDependency("payment", "auth")
	cycles := g.Cycles()
	if len(cycles) != 1 {
		t.Fatalf("cycles = %v, want exactly 1", cycles)
	}
	if got := cycles[0]; !reflect.DeepEqual(got, []string{"auth", "payment"}) {
		t.Fatalf("cycle = %v, want [auth payment]", got)
	}
	if !g.OnCycle("auth") || !g.OnCycle("payment") {
		t.Fatal("both cycle members must be OnCycle")
	}
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"auth", "payment"}}) {
		t.Fatalf("topo order = %v, want one invalid group [auth payment]", got)
	}
}

func TestBuildTransitiveCycle(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "a", "")
	writeUnit(t, repo, "candidate", "b", "")
	writeUnit(t, repo, "candidate", "c", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddUnpublishedPromiseDependency("a", "b")
	g.AddUnpublishedPromiseDependency("b", "c")
	g.AddUnpublishedPromiseDependency("c", "a")
	cycles := g.Cycles()
	if len(cycles) != 1 {
		t.Fatalf("cycles = %v, want exactly 1", cycles)
	}
	if got := cycles[0]; !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("cycle = %v, want [a b c]", got)
	}
	if !g.OnCycle("a") || !g.OnCycle("b") || !g.OnCycle("c") {
		t.Fatal("all members must be OnCycle")
	}
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"a", "b", "c"}}) {
		t.Fatalf("topo order = %v, want one invalid group [a b c]", got)
	}
}

func TestBuildCrossLayerCandidatePreferred(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "")
	writeUnit(t, repo, "candidate", "beta", "")
	writeUnit(t, repo, "stable", "beta", "")
	writeUnit(t, repo, "stable", "gamma", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(g.Nodes()); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("nodes = %v", got)
	}
	beta := g.Node("beta")
	if beta.Layer != "candidate" {
		t.Fatalf("beta layer = %s, want candidate (preferred over stable)", beta.Layer)
	}
	if got := g.Node("gamma").Layer; got != "stable" {
		t.Fatalf("gamma layer = %s, want stable", got)
	}
}

func TestBuildScopeCandidateOnly(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "")
	writeUnit(t, repo, "stable", "beta", "")

	g, err := Build(repo, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(g.Nodes()); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("candidate scope nodes = %v, want [alpha]", got)
	}
	// A promise edge to an out-of-scope provider is ignored: a scope view only
	// orders units inside that scope.
	g.AddPromiseDependency("alpha", "beta")
	if got := g.Node("alpha").PromiseRefs; len(got) != 0 {
		t.Fatalf("alpha promise refs = %v, want none (beta out of scope)", got)
	}
	if g.Node("beta") != nil {
		t.Fatal("beta must not be a node in candidate scope")
	}

	sg, err := Build(repo, "stable")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(sg.Nodes()); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("stable scope nodes = %v, want [beta]", got)
	}
}

func TestBuildInvalidScope(t *testing.T) {
	repo := t.TempDir()
	if _, err := Build(repo, "bogus"); err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestRuleConsumers(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "[b_rule_1]")
	writeUnit(t, repo, "candidate", "beta", "[b_rule_1, b_rule_2]")
	writeUnit(t, repo, "candidate", "gamma", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := g.RuleConsumers("b_rule_1"); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("b_rule_1 consumers = %v", got)
	}
	if got := g.RuleConsumers("b_rule_2"); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("b_rule_2 consumers = %v", got)
	}
	// A global rule applies to every current-layer unit by default and is
	// not repeated in rule_refs, so all nodes are reported.
	if got := g.RuleConsumers("g_rule_baseline"); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("g_rule_baseline consumers = %v", got)
	}
}

func TestTopoOrderDAG(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "a", "")
	writeUnit(t, repo, "candidate", "b", "")
	writeUnit(t, repo, "candidate", "c", "")
	writeUnit(t, repo, "candidate", "d", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddPromiseDependency("a", "c")
	g.AddPromiseDependency("b", "c")
	g.AddPromiseDependency("c", "d")
	// d -> c -> {a, b}; d first, then c, then a and b (tie sorted).
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"d"}, {"c"}, {"a"}, {"b"}}) {
		t.Fatalf("topo order = %v, want [[d] [c] [a] [b]]", got)
	}
}

func TestCycleMemberSetMergesOverlappingCycles(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "x", "")
	writeUnit(t, repo, "candidate", "y", "")
	writeUnit(t, repo, "candidate", "z", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddUnpublishedPromiseDependency("x", "y")
	g.AddUnpublishedPromiseDependency("x", "z")
	g.AddUnpublishedPromiseDependency("y", "x")
	g.AddUnpublishedPromiseDependency("z", "x")
	// Overlapping cycles collapse to the one member set: x, y, and z are all
	// mutually reachable through the same unpublished relations.
	cycles := g.Cycles()
	if len(cycles) != 1 {
		t.Fatalf("cycles = %v, want one member set [x y z]", cycles)
	}
	if got := cycles[0]; !reflect.DeepEqual(got, []string{"x", "y", "z"}) {
		t.Fatalf("cycle = %v, want [x y z]", got)
	}
}

func TestCycleMemberSetIncludesCrossEdgeClosure(t *testing.T) {
	repo := t.TempDir()
	for _, name := range []string{"a", "b", "c", "d"} {
		writeUnit(t, repo, "candidate", name, "")
	}

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	// a -> b -> c -> a is one cycle; b -> d -> c closes another through a
	// cross edge. A DFS that records only back-edge cycles can miss d even
	// though d is on a cycle; the member set must include all four units.
	g.AddUnpublishedPromiseDependency("a", "b")
	g.AddUnpublishedPromiseDependency("b", "c")
	g.AddUnpublishedPromiseDependency("b", "d")
	g.AddUnpublishedPromiseDependency("c", "a")
	g.AddUnpublishedPromiseDependency("d", "c")

	want := [][]string{{"a", "b", "c", "d"}}
	if got := g.Cycles(); !reflect.DeepEqual(got, want) {
		t.Fatalf("publication cycles = %v, want %v", got, want)
	}
	if got := g.UnitCycles(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unit cycles = %v, want %v", got, want)
	}
	for _, name := range []string{"a", "b", "c", "d"} {
		if !g.OnCycle(name) {
			t.Fatalf("%s must be on the publication cycle", name)
		}
	}
	if got := g.TopoOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("topo order = %v, want %v", got, want)
	}
}

func TestPromiseDependenciesParticipateInOrderAndCycles(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "aaa", "")
	writeUnit(t, repo, "candidate", "zzz", "")

	g, err := Build(repo, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	g.AddPromiseDependency("aaa", "zzz")
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"zzz"}, {"aaa"}}) {
		t.Fatalf("promise topo order = %v, want [[zzz] [aaa]]", got)
	}
	if got := g.PromiseConsumers("zzz"); !reflect.DeepEqual(got, []string{"aaa"}) {
		t.Fatalf("promise consumers = %v", got)
	}

	// The reverse edge makes the units mutually dependent: a unit dependency
	// cycle (a violation), not a publication deadlock, because only one member
	// waits for the other.
	g.AddUnpublishedPromiseDependency("zzz", "aaa")
	if cycles := g.Cycles(); len(cycles) != 0 {
		t.Fatalf("cycles = %v, want none (only one wait edge)", cycles)
	}
	if got := g.UnitCycles(); len(got) != 1 {
		t.Fatalf("unit cycles = %v, want one cycle", got)
	}
	if got := g.TopoOrder(); !reflect.DeepEqual(got, [][]string{{"aaa", "zzz"}}) {
		t.Fatalf("promise topo order = %v, want one invalid group [aaa zzz]", got)
	}

	// Marking the forward edge unpublished too creates the publication
	// deadlock: both members wait for each other.
	g.AddUnpublishedPromiseDependency("aaa", "zzz")
	cycles := g.Cycles()
	if len(cycles) != 1 || len(cycles[0]) != 2 {
		t.Fatalf("promise cycle not detected: %v", cycles)
	}
}

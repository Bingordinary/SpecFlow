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
	if got := g.TopoOrder(); !reflect.DeepEqual(got, []string{"beta", "gamma", "alpha"}) {
		t.Fatalf("topo order = %v, want [beta gamma alpha]", got)
	}
}

func TestPublishedPromiseDependencyDoesNotBlockOrder(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "")
	writeUnit(t, repo, "candidate", "beta", "")
	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddPublishedPromiseDependency("alpha", "beta")
	if cycles := g.Cycles(); len(cycles) != 0 {
		t.Fatalf("cycles = %v, want none", cycles)
	}
	if g.OnCycle("alpha") || g.OnCycle("beta") {
		t.Fatal("a published provider edge must not form a publication cycle")
	}
	if got := g.TopoOrder(); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("topo order = %v, want [alpha beta] (no blocking edge)", got)
	}
	if got := g.Node("alpha").PromiseRefs; !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("all promise refs = %v, want [beta]", got)
	}
	if got := g.Node("alpha").PublicationRefs; len(got) != 0 {
		t.Fatalf("publication refs = %v, want none", got)
	}
	if got := g.PromiseConsumers("beta"); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("consumers = %v, want [alpha]", got)
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
	g.AddPromiseDependency("auth", "payment")
	g.AddPromiseDependency("payment", "auth")
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
	if len(g.TopoOrder()) != 0 {
		t.Fatalf("topo order = %v, want empty (all on cycle)", g.TopoOrder())
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
	g.AddPromiseDependency("a", "b")
	g.AddPromiseDependency("b", "c")
	g.AddPromiseDependency("c", "a")
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
	if got := g.TopoOrder(); !reflect.DeepEqual(got, []string{"d", "c", "a", "b"}) {
		t.Fatalf("topo order = %v, want [d c a b]", got)
	}
}

func TestCycleDedup(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "x", "")
	writeUnit(t, repo, "candidate", "y", "")
	writeUnit(t, repo, "candidate", "z", "")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	g.AddPromiseDependency("x", "y")
	g.AddPromiseDependency("x", "z")
	g.AddPromiseDependency("y", "x")
	g.AddPromiseDependency("z", "x")
	cycles := g.Cycles()
	if len(cycles) != 2 {
		t.Fatalf("cycles = %v, want 2 (x-y and x-z)", cycles)
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
	if got := g.TopoOrder(); !reflect.DeepEqual(got, []string{"zzz", "aaa"}) {
		t.Fatalf("promise topo order = %v, want [zzz aaa]", got)
	}
	if got := g.PromiseConsumers("zzz"); !reflect.DeepEqual(got, []string{"aaa"}) {
		t.Fatalf("promise consumers = %v", got)
	}

	g.AddPromiseDependency("zzz", "aaa")
	cycles := g.Cycles()
	if len(cycles) != 1 || len(cycles[0]) != 2 {
		t.Fatalf("promise cycle not detected: %v", cycles)
	}
}

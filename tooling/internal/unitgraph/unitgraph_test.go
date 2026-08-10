package unitgraph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeUnit(t *testing.T, repoRoot, layer, name, unitRefs, ruleRefs string) {
	t.Helper()
	if unitRefs == "" {
		unitRefs = "none"
	}
	if ruleRefs == "" {
		ruleRefs = "none"
	}
	content := "---\nid: " + name + "\nversion: 0.1.0\nunit_refs: " + unitRefs + "\nrule_refs: " + ruleRefs + "\n---\n\n# " + name + "\n"
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
	writeUnit(t, repo, "candidate", "alpha", "[beta, gamma]", "b_rule_1")
	writeUnit(t, repo, "candidate", "beta", "none", "g_rule_baseline")
	writeUnit(t, repo, "candidate", "gamma", "[beta]", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(g.Nodes()); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("nodes = %v", got)
	}
	if got := g.Node("alpha").UnitRefs; !reflect.DeepEqual(got, []string{"beta", "gamma"}) {
		t.Fatalf("alpha unit refs = %v", got)
	}
	if got := g.Node("alpha").RuleRefs; !reflect.DeepEqual(got, []string{"b_rule_1"}) {
		t.Fatalf("alpha rule refs = %v", got)
	}
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

func TestBuildTwoUnitCycle(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "auth", "[payment]", "none")
	writeUnit(t, repo, "candidate", "payment", "[auth]", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
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
	writeUnit(t, repo, "candidate", "a", "[b]", "none")
	writeUnit(t, repo, "candidate", "b", "[c]", "none")
	writeUnit(t, repo, "candidate", "c", "[a]", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
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

func TestBuildSelfCycle(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "solo", "[solo]", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	cycles := g.Cycles()
	if len(cycles) != 1 || !reflect.DeepEqual(cycles[0], []string{"solo"}) {
		t.Fatalf("cycles = %v, want [[solo]]", cycles)
	}
}

func TestBuildVersionPinnedRefs(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "[beta@1.2.0]", "b_rule_x@0.3.0")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Node("alpha").UnitRefs; !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("unit refs = %v, want version stripped", got)
	}
	if got := g.Node("alpha").RuleRefs; !reflect.DeepEqual(got, []string{"b_rule_x"}) {
		t.Fatalf("rule refs = %v, want version stripped", got)
	}
}

func TestBuildCrossLayerCandidatePreferred(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "[beta]", "none")
	writeUnit(t, repo, "candidate", "beta", "none", "none")
	writeUnit(t, repo, "stable", "beta", "none", "none")
	writeUnit(t, repo, "stable", "gamma", "none", "none")

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
	writeUnit(t, repo, "candidate", "alpha", "[beta]", "none")
	writeUnit(t, repo, "stable", "beta", "none", "none")

	g, err := Build(repo, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(g.Nodes()); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("candidate scope nodes = %v, want [alpha]", got)
	}
	// beta is referenced but out of scope: edge recorded, not a node.
	if got := g.Node("alpha").UnitRefs; !reflect.DeepEqual(got, []string{"beta"}) {
		t.Fatalf("alpha unit refs = %v", got)
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

func writeRetiringUnit(t *testing.T, repoRoot, layer, name, unitRefs string) {
	t.Helper()
	content := "---\nid: " + name + "\nstatus: retired\nversion: 1.0.0\nunit_refs: " + unitRefs + "\nrule_refs: none\n---\n\n# " + name + "\n"
	dir := filepath.Join(repoRoot, "docs/specs/units", layer)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unit_"+name+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildSkipsRetiringUnits(t *testing.T) {
	repo := t.TempDir()
	// alpha is retiring: its references disappear with it, so neither its
	// node nor its edges participate in the graph.
	writeRetiringUnit(t, repo, "candidate", "alpha", "[beta]")
	writeUnit(t, repo, "candidate", "beta", "[gamma]", "none")
	writeUnit(t, repo, "candidate", "gamma", "[alpha]", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(g.Nodes()); !reflect.DeepEqual(got, []string{"beta", "gamma"}) {
		t.Fatalf("nodes = %v, want [beta gamma] (retiring alpha excluded)", got)
	}
	if g.Node("alpha") != nil {
		t.Fatal("retiring unit must not be a node")
	}
	if cycles := g.Cycles(); len(cycles) != 0 {
		t.Fatalf("cycles = %v, want none (alpha's edge would otherwise form beta -> gamma -> alpha)", cycles)
	}
	if g.OnCycle("beta") || g.OnCycle("gamma") {
		t.Fatal("no active unit should be on a cycle")
	}
}

func TestUnitConsumers(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "[beta]", "none")
	writeUnit(t, repo, "candidate", "delta", "[beta, gamma]", "none")
	writeUnit(t, repo, "candidate", "gamma", "none", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := g.UnitConsumers("beta"); !reflect.DeepEqual(got, []string{"alpha", "delta"}) {
		t.Fatalf("beta consumers = %v", got)
	}
	if got := g.UnitConsumers("gamma"); !reflect.DeepEqual(got, []string{"delta"}) {
		t.Fatalf("gamma consumers = %v", got)
	}
	if got := g.UnitConsumers("solo"); len(got) != 0 {
		t.Fatalf("solo consumers = %v, want none", got)
	}
}

func TestRuleConsumers(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "alpha", "none", "[b_rule_1]")
	writeUnit(t, repo, "candidate", "beta", "none", "[b_rule_1, b_rule_2]")
	writeUnit(t, repo, "candidate", "gamma", "none", "none")

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
	writeUnit(t, repo, "candidate", "a", "[c]", "none")
	writeUnit(t, repo, "candidate", "b", "[c]", "none")
	writeUnit(t, repo, "candidate", "c", "[d]", "none")
	writeUnit(t, repo, "candidate", "d", "none", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	// d -> c -> {a, b}; d first, then c, then a and b (tie sorted).
	if got := g.TopoOrder(); !reflect.DeepEqual(got, []string{"d", "c", "a", "b"}) {
		t.Fatalf("topo order = %v, want [d c a b]", got)
	}
}

func TestCycleDedup(t *testing.T) {
	repo := t.TempDir()
	writeUnit(t, repo, "candidate", "x", "[y, z]", "none")
	writeUnit(t, repo, "candidate", "y", "[x]", "none")
	writeUnit(t, repo, "candidate", "z", "[x]", "none")

	g, err := Build(repo, "all")
	if err != nil {
		t.Fatal(err)
	}
	cycles := g.Cycles()
	if len(cycles) != 2 {
		t.Fatalf("cycles = %v, want 2 (x-y and x-z)", cycles)
	}
}

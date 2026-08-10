// Package unitgraph builds the directed dependency graph of units from
// their declared unit_refs and reports cycles and a promotion order.
//
// It is purely mechanical: only explicit unit_refs edges are used, never
// inference from prose. Rules are leaves — a rule never references a unit or
// another rule, so cycles can only involve units.
package unitgraph

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// Node is one unit in the dependency graph.
type Node struct {
	Name     string   // unit name
	Layer    string   // resolved layer of the spec file read: "candidate" or "stable"
	UnitRefs []string // out edges: referenced unit names (version stripped)
	RuleRefs []string // referenced rule ids (version stripped)
}

// Graph is the dependency graph of a set of units.
type Graph struct {
	repoRoot string
	scope    string
	nodes    map[string]*Node
	names    []string // unit names, sorted
}

// Build scans the units in scope and resolves every explicit unit_refs edge.
//
// Scope determines which units become nodes:
//   - "all": every current-layer unit — candidate layer preferred, stable
//     fallback for units with no candidate file (the same resolution the
//     mechanical reference-integrity check uses)
//   - "candidate": units with a candidate spec file only
//   - "stable": units with a stable spec file only
//
// Edges always resolve to the current-layer file (candidate first, stable
// fallback). An edge whose target has no file in scope is still recorded with
// the target name; cycle detection and ordering only consider in-scope nodes.
//
// A retiring unit (status: retired in its current-layer frontmatter) is not
// part of the dependency graph: its references disappear with it (see
// framework/unit_validate_checklist.md, retiring-unit note), so neither its
// node nor its edges participate in cycle detection, ordering, or consumer
// reports. Referrers of the retiring unit are still rejected by the
// reference-integrity check (validate Check 4), not by this graph.
func Build(repoRoot, scope string) (*Graph, error) {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" {
		scope = "all"
	}
	switch scope {
	case "all", "candidate", "stable":
	default:
		return nil, fmt.Errorf("invalid scope %q: must be all, candidate, or stable", scope)
	}

	g := &Graph{repoRoot: repoRoot, scope: scope, nodes: map[string]*Node{}}

	var dirs []string
	switch scope {
	case "all":
		dirs = []string{
			filepath.Join(repoRoot, specpaths.CandidateDir),
			filepath.Join(repoRoot, specpaths.StableDir),
		}
	case "candidate":
		dirs = []string{filepath.Join(repoRoot, specpaths.CandidateDir)}
	case "stable":
		dirs = []string{filepath.Join(repoRoot, specpaths.StableDir)}
	}

	// Collect the file to read per unit: candidate layer first when both
	// exist. For the "all" scope a unit with a candidate file is a candidate
	// node; otherwise its stable file (if any).
	perUnit := map[string]struct {
		path  string
		layer string
	}{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasPrefix(entry.Name(), "unit_") || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "unit_"), ".md")
			layer := "stable"
			if filepath.Base(dir) == "candidate" {
				layer = "candidate"
			}
			existing, ok := perUnit[name]
			if !ok || (existing.layer != "candidate" && layer == "candidate") {
				perUnit[name] = struct {
					path  string
					layer string
				}{path: filepath.Join(dir, entry.Name()), layer: layer}
			}
		}
	}

	for name, file := range perUnit {
		node, err := readNode(file.path, name, file.layer)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file.path, err)
		}
		if node == nil {
			continue // retiring unit — not part of the dependency graph
		}
		g.nodes[name] = node
	}

	for _, node := range g.nodes {
		sort.Strings(node.UnitRefs)
		sort.Strings(node.RuleRefs)
	}

	g.names = make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		g.names = append(g.names, name)
	}
	sort.Strings(g.names)

	return g, nil
}

// readNode reads one unit spec into a graph node. It returns (nil, nil) when
// the unit is retiring (status: retired) — a retiring unit is not part of the
// dependency graph because its references disappear with it.
func readNode(path, name, layer string) (*Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm := specpaths.ReadFrontmatterStringMap(string(data))
	if strings.TrimSpace(fm["status"]) == "retired" {
		return nil, nil
	}
	node := &Node{Name: name, Layer: layer}
	if raw := fm["unit_refs"]; raw != "" && !strings.EqualFold(raw, "none") {
		for _, ref := range specpaths.ParseRefList(raw) {
			if refName := stripVersion(ref); refName != "" {
				node.UnitRefs = append(node.UnitRefs, refName)
			}
		}
	}
	if raw := fm["rule_refs"]; raw != "" && !strings.EqualFold(raw, "none") {
		for _, ref := range specpaths.ParseRefList(raw) {
			if refID := stripVersion(ref); refID != "" {
				node.RuleRefs = append(node.RuleRefs, refID)
			}
		}
	}
	return node, nil
}

func stripVersion(ref string) string {
	ref = strings.TrimSpace(ref)
	if atIdx := strings.LastIndex(ref, "@"); atIdx > 0 {
		return ref[:atIdx]
	}
	return ref
}

// Nodes returns all nodes sorted by unit name.
func (g *Graph) Nodes() []*Node {
	nodes := make([]*Node, 0, len(g.nodes))
	for _, name := range g.names {
		nodes = append(nodes, g.nodes[name])
	}
	return nodes
}

// Node returns the node for a unit name, or nil when the unit is not a node.
func (g *Graph) Node(name string) *Node {
	return g.nodes[name]
}

// Cycles returns every dependency cycle as an ordered member list. Each
// cycle is normalized to start at its lexicographically smallest member and
// follows the edge direction; duplicates are removed.
func (g *Graph) Cycles() [][]string {
	const (
		white = 0 // unvisited
		gray  = 1 // on the DFS stack
		black = 2 // fully explored
	)
	color := map[string]int{}
	var stack []string
	seen := map[string]bool{}
	var cycles [][]string

	var visit func(name string)
	visit = func(name string) {
		color[name] = gray
		stack = append(stack, name)
		for _, dep := range g.nodes[name].UnitRefs {
			if _, ok := g.nodes[dep]; !ok {
				continue // target out of scope — no edge inside the graph
			}
			switch color[dep] {
			case white:
				visit(dep)
			case gray:
				cycle := normalizeCycle(extractCycle(stack, dep))
				if key := canonicalCycleKey(cycle); !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[name] = black
	}

	for _, name := range g.names {
		if color[name] == white {
			visit(name)
		}
	}

	sort.Slice(cycles, func(i, j int) bool { return cycles[i][0] < cycles[j][0] })
	return cycles
}

// extractCycle returns the members from the first occurrence of target on
// the DFS stack to the top, keeping the edge direction.
func extractCycle(stack []string, target string) []string {
	start := -1
	for i, name := range stack {
		if name == target {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	cycle := make([]string, len(stack)-start)
	copy(cycle, stack[start:])
	return cycle
}

// canonicalCycleKey identifies a cycle independent of its rotation. The key
// is the member list sorted, joined; the cycle itself is normalized by
// rotating it to start at its smallest member.
func canonicalCycleKey(cycle []string) string {
	parts := append([]string(nil), cycle...)
	sort.Strings(parts)
	return strings.Join(parts, "->")
}

func normalizeCycle(cycle []string) []string {
	if len(cycle) <= 1 {
		return cycle
	}
	minIdx := 0
	for i := 1; i < len(cycle); i++ {
		if cycle[i] < cycle[minIdx] {
			minIdx = i
		}
	}
	rotated := make([]string, len(cycle))
	for i := 0; i < len(cycle); i++ {
		rotated[i] = cycle[(minIdx+i)%len(cycle)]
	}
	return rotated
}

// OnCycle reports whether the unit participates in any cycle.
func (g *Graph) OnCycle(name string) bool {
	if _, ok := g.nodes[name]; !ok {
		return false
	}
	for _, cycle := range g.Cycles() {
		for _, member := range cycle {
			if member == name {
				return true
			}
		}
	}
	return false
}

// TopoOrder returns the units in promotion order: a unit that another unit
// depends on comes first (the referenced unit must be stable before its
// referrer can promote). Units on a cycle have no order and are omitted —
// cycles are reported separately.
func (g *Graph) TopoOrder() []string {
	// Edge A -> B means A depends on B; B must be ordered before A. Work on
	// the reversed graph: B -> A, where an indegree of 0 means "depends on
	// nothing" — first to promote.
	dependents := map[string][]string{} // name -> units that depend on it
	depsCount := map[string]int{}       // name -> number of units it depends on
	for _, name := range g.names {
		node := g.nodes[name]
		depsCount[name] = 0
		for _, dep := range node.UnitRefs {
			if _, ok := g.nodes[dep]; !ok {
				continue
			}
			dependents[dep] = append(dependents[dep], name)
			depsCount[name]++
		}
	}

	var ready []string
	for _, name := range g.names {
		if depsCount[name] == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)

	var order []string
	remaining := map[string]bool{}
	for _, name := range g.names {
		remaining[name] = true
	}
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		if !remaining[name] {
			continue
		}
		delete(remaining, name)
		order = append(order, name)
		for _, dependent := range dependents[name] {
			depsCount[dependent]--
			if depsCount[dependent] == 0 && remaining[dependent] {
				ready = append(ready, dependent)
			}
		}
		sort.Strings(ready)
	}
	return order
}

// UnitConsumers returns the units that reference the given unit in their
// unit_refs (in-edges of the graph), sorted.
func (g *Graph) UnitConsumers(name string) []string {
	var consumers []string
	for _, node := range g.nodes {
		for _, ref := range node.UnitRefs {
			if ref == name {
				consumers = append(consumers, node.Name)
				break
			}
		}
	}
	sort.Strings(consumers)
	return consumers
}

// RuleConsumers returns the units bound to the given rule, sorted.
// A bound rule (b_rule_*) is bound by explicit rule_refs entries. A global
// rule (g_rule_*) is not repeated in rule_refs — it applies to every
// current-layer unit by default (spec_writing_guide.md §5) — so for global
// rules every node is returned. The caller verifies the rule file exists;
// a missing global rule (retired, mistyped) constrains nothing.
func (g *Graph) RuleConsumers(ruleID string) []string {
	if strings.HasPrefix(ruleID, "g_rule_") {
		return append([]string(nil), g.names...)
	}
	var consumers []string
	for _, node := range g.nodes {
		for _, ref := range node.RuleRefs {
			if ref == ruleID {
				consumers = append(consumers, node.Name)
				break
			}
		}
	}
	sort.Strings(consumers)
	return consumers
}

// Scope returns the scope the graph was built with.
func (g *Graph) Scope() string {
	return g.scope
}

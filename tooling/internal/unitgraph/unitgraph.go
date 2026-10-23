// Package unitgraph builds the directed dependency graph of units from their
// promise depends_on edges and reports unit and publication cycle member sets
// and a structural dependency order.
//
// It is purely mechanical: only explicit promise dependency edges are used,
// never inference from prose. Rules are leaves — a rule never references a
// unit or another rule, so cycles can only involve units.
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
	Name            string   // unit name
	Layer           string   // resolved layer of the spec file read: "candidate" or "stable"
	RuleRefs        []string // referenced rule ids (version stripped)
	PromiseRefs     []string // all out edges derived from promise depends_on (provider units) — the structural ordering relation
	UnpublishedRefs []string // out edges whose provider promise is not published stable at the current contract — the promotion blocker relation
}

// Graph is the dependency graph of a set of units.
type Graph struct {
	repoRoot string
	scope    string
	nodes    map[string]*Node
	names    []string // unit names, sorted
}

// Build scans the units in scope and reads their declared rule references.
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
		g.nodes[name] = node
	}

	for _, node := range g.nodes {
		sort.Strings(node.RuleRefs)
	}

	g.names = make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		g.names = append(g.names, name)
	}
	sort.Strings(g.names)

	return g, nil
}

// readNode reads one unit spec into a graph node.
func readNode(path, name, layer string) (*Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm := specpaths.ReadFrontmatterStringMap(string(data))

	node := &Node{Name: name, Layer: layer}
	if raw := fm["rule_refs"]; raw != "" && !strings.EqualFold(raw, "none") {
		for _, ref := range specpaths.ParseRefList(raw) {
			if ref != "" {
				node.RuleRefs = append(node.RuleRefs, ref)
			}
		}
	}
	return node, nil
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

// AddPromiseDependency records a promise-derived dependency edge between two
// in-scope units. Every edge participates in the structural ordering and unit
// cycle detection; publication state is an annotation, never a filter on that
// ordering. Out-of-scope targets are ignored: a scope view only orders units
// inside that scope.
func (g *Graph) AddPromiseDependency(from, to string) {
	g.addPromiseRef(from, to)
}

// AddUnpublishedPromiseDependency records a promise-derived dependency edge
// whose provider promise is not yet published stable at its current contract.
// The edge constrains structural ordering exactly like any other edge; the
// unpublished state is additionally recorded because it is the promotion
// blocker relation that publication-cycle detection reports.
func (g *Graph) AddUnpublishedPromiseDependency(from, to string) {
	g.addPromiseRef(from, to)
	g.addUnpublishedRef(from, to)
}

func (g *Graph) addPromiseRef(from, to string) bool {
	if from == "" || to == "" || from == to {
		return false
	}
	node := g.nodes[from]
	if node == nil {
		return false
	}
	if _, ok := g.nodes[to]; !ok {
		return false
	}
	for _, ref := range node.PromiseRefs {
		if ref == to {
			return false
		}
	}
	node.PromiseRefs = append(node.PromiseRefs, to)
	sort.Strings(node.PromiseRefs)
	return true
}

func (g *Graph) addUnpublishedRef(from, to string) {
	if from == "" || to == "" || from == to {
		return
	}
	node := g.nodes[from]
	if node == nil || g.nodes[to] == nil {
		return
	}
	for _, ref := range node.UnpublishedRefs {
		if ref == to {
			return
		}
	}
	node.UnpublishedRefs = append(node.UnpublishedRefs, to)
	sort.Strings(node.UnpublishedRefs)
}

// dependencies returns the complete promise dependency edges of a node,
// deduplicated. Structural ordering and unit cycle detection read this set:
// publication state never narrows the relation they describe.
func (g *Graph) dependencies(node *Node) []string {
	return dedupeRefs(node.PromiseRefs)
}

// publicationDependencies returns the edges whose provider promise is not yet
// published stable at its current contract, deduplicated. Publication-cycle
// detection reads this subset; it answers whether a consumer must wait for a
// provider's publication, not which unit comes first.
func (g *Graph) publicationDependencies(node *Node) []string {
	return dedupeRefs(node.UnpublishedRefs)
}

func dedupeRefs(refs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}

// Cycles returns the publication-order cycle member sets. Each set is one
// strongly connected component of the unpublished dependency relation: every
// unit in it can reach every other unit through provider promises that are not
// yet published stable, so every member waits for another member's unpublished
// contract. A publication cycle is the promotion diagnostic of a unit dependency
// cycle whose edges are all unpublished; it is not a promise contract validity
// problem. Members are sorted by name, and sets are sorted by their smallest
// member.
func (g *Graph) Cycles() [][]string {
	return g.componentsOver(g.publicationDependencies)
}

// UnitCycles returns the unit-level dependency cycle member sets over the
// complete promise dependency relation. A unit cycle can exist while the promise
// graph is acyclic: one promise of a unit can depend on another unit's promise
// while a different promise of the same unit is depended on by that unit. It is
// a management violation — the units cannot be ordered — and validate fails
// every unit on it. Members are sorted by name, and sets are sorted by their
// smallest member.
func (g *Graph) UnitCycles() [][]string {
	return g.componentsOver(g.dependencies)
}

// componentsOver returns every multi-unit strongly connected component of the
// unit graph over the given edge set. Each component is a cycle member set: its
// units can all reach one another through that edge set. A single-unit component
// is not a cycle and is omitted.
func (g *Graph) componentsOver(edges func(*Node) []string) [][]string {
	var cycles [][]string
	for _, members := range g.stronglyConnectedOver(edges) {
		if len(members) > 1 {
			cycles = append(cycles, members)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return cycles[i][0] < cycles[j][0] })
	return cycles
}

// OnCycle reports whether the unit is a member of a publication-order cycle
// member set.
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

// TopoOrder returns the structural dependency order as groups, dependencies
// first. Ordering uses the complete promise dependency relation: every
// displayed edge constrains the sequence, regardless of publication state.
//
// A group is one position in the structural order. A multi-unit group holds
// units that are mutually reachable through promise dependencies (a strongly
// connected component) and is a unit dependency cycle — a violation, because
// the units have no order; it is kept in the order only so the cycle members
// stay visible. Publication waits are a separate relation reported as edge
// annotations. Group members are sorted by name, and groups are emitted in a
// deterministic dependency-first order.
func (g *Graph) TopoOrder() [][]string {
	components := g.stronglyConnected()
	componentOf := map[string]int{}
	for i, members := range components {
		for _, name := range members {
			componentOf[name] = i
		}
	}

	// Condensed graph: an edge from component A to component B means A depends
	// on B, so B is ordered first. Work on the reversed graph: indegree counts
	// the components a component still depends on.
	dependsOn := make([]map[int]bool, len(components))
	dependents := make([]map[int]bool, len(components))
	for i := range components {
		dependsOn[i] = map[int]bool{}
		dependents[i] = map[int]bool{}
	}
	for _, name := range g.names {
		from := componentOf[name]
		for _, dep := range g.dependencies(g.nodes[name]) {
			to, ok := componentOf[dep]
			if !ok || to == from {
				continue
			}
			dependsOn[from][to] = true
			dependents[to][from] = true
		}
	}

	// Deterministic tie-break: the group whose smallest member sorts first.
	less := func(a, b int) bool { return components[a][0] < components[b][0] }
	var ready []int
	for i := range components {
		if len(dependsOn[i]) == 0 {
			ready = append(ready, i)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })

	var order [][]string
	emitted := make([]bool, len(components))
	for len(ready) > 0 {
		next := ready[0]
		ready = ready[1:]
		if emitted[next] {
			continue
		}
		emitted[next] = true
		order = append(order, components[next])
		for dependent := range dependents[next] {
			delete(dependsOn[dependent], next)
			if len(dependsOn[dependent]) == 0 && !emitted[dependent] {
				ready = append(ready, dependent)
			}
		}
		sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
	}
	return order
}

// UnitCycleGroup returns the units that are mutually reachable with the given
// unit through promise dependencies, sorted. A single-member result means the
// unit is not on a unit dependency cycle. Units outside the graph return nil.
func (g *Graph) UnitCycleGroup(name string) []string {
	if _, ok := g.nodes[name]; !ok {
		return nil
	}
	for _, members := range g.stronglyConnected() {
		for _, member := range members {
			if member == name {
				return members
			}
		}
	}
	return nil
}

// stronglyConnected returns the strongly connected components of the complete
// promise dependency graph, each sorted by unit name (Tarjan).
func (g *Graph) stronglyConnected() [][]string {
	return g.stronglyConnectedOver(g.dependencies)
}

// stronglyConnectedOver returns the strongly connected components of the unit
// graph over the given edge set, each sorted by unit name (Tarjan).
func (g *Graph) stronglyConnectedOver(edges func(*Node) []string) [][]string {
	index := 0
	indices := map[string]int{}
	lowlink := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var components [][]string

	var visit func(name string)
	visit = func(name string) {
		indices[name] = index
		lowlink[name] = index
		index++
		stack = append(stack, name)
		onStack[name] = true
		for _, dep := range edges(g.nodes[name]) {
			if _, ok := g.nodes[dep]; !ok {
				continue
			}
			if _, seen := indices[dep]; !seen {
				visit(dep)
				if lowlink[dep] < lowlink[name] {
					lowlink[name] = lowlink[dep]
				}
				continue
			}
			if onStack[dep] && indices[dep] < lowlink[name] {
				lowlink[name] = indices[dep]
			}
		}
		if lowlink[name] != indices[name] {
			return
		}
		var members []string
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			members = append(members, last)
			if last == name {
				break
			}
		}
		sort.Strings(members)
		components = append(components, members)
	}

	for _, name := range g.names {
		if _, seen := indices[name]; !seen {
			visit(name)
		}
	}
	return components
}

// PromiseConsumers returns the units that depend on the given unit through a
// promise depends_on edge, sorted.
func (g *Graph) PromiseConsumers(name string) []string {
	var consumers []string
	for _, node := range g.nodes {
		for _, ref := range node.PromiseRefs {
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
// a missing global rule (removed, mistyped) constrains nothing.
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

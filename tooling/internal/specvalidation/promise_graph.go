package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// promiseNode is one promise in the repository-level promise graph.
type promiseNode struct {
	ID        string
	Unit      string
	File      string
	DependsOn []string
	Promise   Promise
}

// PromiseEdge is one promise-level dependency edge.
type PromiseEdge struct {
	From string
	To   string
	Unit string
}

// PromiseGraph is a loaded current-layer promise graph. Load it once when
// reporting several units; the load expands every unit spec, so per-unit
// checks should reuse this value instead of rescanning the repository.
type PromiseGraph struct {
	nodes []promiseNode
	byID  map[string]promiseNode
	// globalIssues are repository-wide failures: a duplicate promise ID or an
	// unreadable unit spec. They fail every unit because the graph cannot prove
	// the repository's contract set.
	globalIssues []string
	// unitIssues belong to one unit's spec. A missing or malformed `## Promises`
	// section fails that unit (and consumers whose dependency stays unresolved);
	// it must not fail unrelated units.
	unitIssues map[string][]string
}

// Promise graph scopes. "all" is the current-layer view (candidate preferred,
// stable fallback); "candidate" and "stable" read exactly that layer. The
// dependency report uses the same scope vocabulary as unitgraph.
const (
	PromiseScopeAll       = "all"
	PromiseScopeCandidate = "candidate"
	PromiseScopeStable    = "stable"
)

// LoadPromiseGraphScope loads the promise graph for one layer scope. A unit
// outside the scope contributes no node or edge, so callers that report a
// layer-specific view never see the other layer's contracts.
func LoadPromiseGraphScope(repoRoot, scope string) (*PromiseGraph, error) {
	scope = strings.TrimSpace(strings.ToLower(scope))
	switch scope {
	case PromiseScopeAll, PromiseScopeCandidate, PromiseScopeStable:
	default:
		return nil, fmt.Errorf("invalid promise graph scope %q: must be all, candidate, or stable", scope)
	}
	nodes, globalIssues, unitIssues := loadPromiseNodes(repoRoot, scope)
	graph := &PromiseGraph{nodes: nodes, byID: map[string]promiseNode{}, globalIssues: globalIssues, unitIssues: unitIssues}
	for _, node := range nodes {
		if previous, exists := graph.byID[node.ID]; exists {
			graph.globalIssues = append(graph.globalIssues, fmt.Sprintf("duplicate promise id %q in %s and %s", node.ID, previous.File, node.File))
			continue
		}
		graph.byID[node.ID] = node
	}
	return graph, nil
}

// Check validates promise dependency resolution and cycles for one unit.
// Duplicate IDs and read failures are repository-wide issues; a promise cycle
// is a defect for every unit that owns a promise on it, and it is detected no
// matter how many other cycles the graph contains. A unit dependency cycle —
// two units each having a promise that depends on a promise of the other
// through the unit aggregation — is a defect for every unit on it as well. A
// unit whose own promises are not on a promise cycle and that is not on a unit
// dependency cycle is not blocked. Unresolved depends_on is reported for the
// requested unit.
func (g *PromiseGraph) Check(unitName string) CheckResult {
	result := CheckResult{Name: "Promise graph", Status: Fail}
	unitIssues := g.unitIssues[unitName]
	if len(g.nodes) == 0 && len(g.globalIssues) == 0 && len(unitIssues) == 0 {
		result.Status = Pass
		result.Details = "no promises declared in the resolved scope"
		return result
	}

	issues := append([]string{}, g.globalIssues...)
	issues = append(issues, unitIssues...)
	for _, node := range g.nodes {
		if node.Unit != unitName {
			continue
		}
		for _, dep := range node.DependsOn {
			if _, ok := g.byID[dep]; !ok {
				issues = append(issues, fmt.Sprintf("promise %q: unresolved depends_on %q", node.ID, dep))
			}
		}
	}
	for _, cycle := range g.unitCycles(unitName) {
		issues = append(issues, "promise dependency cycle: "+strings.Join(cycle, " -> "))
	}
	if cycle, via := g.unitCycleThrough(unitName); len(cycle) > 0 {
		issues = append(issues, "unit dependency cycle: "+strings.Join(cycle, " -> ")+" (via "+strings.Join(via, ", ")+")")
	}
	if len(issues) > 0 {
		result.Details = strings.Join(issues, "; ")
		return result
	}
	result.Status = Pass
	result.Details = fmt.Sprintf("%d promise(s) resolved; no promise or unit cycles", len(g.nodes))
	return result
}

// unitCycles returns one genuine cycle path through each promise owned by the
// unit that lies on a dependency cycle. Every disjoint cycle is found: the
// check scans each of the unit's own promises for a path back to itself
// instead of stopping at the first cycle in the graph. A cycle that does not
// pass through the unit's own promises does not block it.
func (g *PromiseGraph) unitCycles(unitName string) [][]string {
	var cycles [][]string
	seen := map[string]bool{}
	for _, node := range g.nodes {
		if node.Unit != unitName {
			continue
		}
		cycle := promiseCycleThrough(g.byID, node.ID)
		if len(cycle) == 0 {
			continue
		}
		key := promiseCycleKey(cycle)
		if seen[key] {
			continue
		}
		seen[key] = true
		cycles = append(cycles, cycle)
	}
	sort.Slice(cycles, func(i, j int) bool { return promiseCycleKey(cycles[i]) < promiseCycleKey(cycles[j]) })
	return cycles
}

// unitCycleThrough returns a unit-level dependency cycle through unitName and
// the promise edges that create it, or nil when the unit is not on one.
//
// Unit edges aggregate every promise of one unit onto another unit, so two
// units can be mutually reachable while the promise graph stays acyclic:
// a.p1 -> b.q1 and b.q2 -> a.p2 make a -> b -> a at the unit level even though
// no promise depends on itself. The receiving and sending promises may be
// different, so this does not require a promise of one unit to reach another
// promise of the same unit. It is a management defect — the units cannot be
// ordered and their implementations reference each other — so validate fails
// every unit on it.
func (g *PromiseGraph) unitCycleThrough(unitName string) ([]string, []string) {
	deps := map[string]map[string]bool{}
	via := map[string]string{}
	for _, node := range g.nodes {
		for _, dep := range node.DependsOn {
			target, ok := g.byID[dep]
			if !ok || target.Unit == node.Unit {
				continue
			}
			if deps[node.Unit] == nil {
				deps[node.Unit] = map[string]bool{}
			}
			deps[node.Unit][target.Unit] = true
			key := node.Unit + "\x00" + target.Unit
			edge := node.ID + " -> " + target.ID
			if previous, exists := via[key]; !exists || edge < previous {
				via[key] = edge
			}
		}
	}
	if len(deps[unitName]) == 0 {
		return nil, nil
	}
	successors := make([]string, 0, len(deps[unitName]))
	for next := range deps[unitName] {
		successors = append(successors, next)
	}
	sort.Strings(successors)
	for _, next := range successors {
		path := unitPath(deps, next, unitName)
		if len(path) == 0 {
			continue
		}
		cycle := append([]string{unitName}, path...)
		edges := make([]string, 0, len(cycle)-1)
		for i := 0; i+1 < len(cycle); i++ {
			edges = append(edges, via[cycle[i]+"\x00"+cycle[i+1]])
		}
		return cycle, edges
	}
	return nil, nil
}

// unitPath returns a unit path from start to target (both inclusive) in the
// unit-level aggregation of the promise graph, or nil when target is
// unreachable. Breadth-first search keeps the reported cycle short.
func unitPath(deps map[string]map[string]bool, start, target string) []string {
	prev := map[string]string{start: ""}
	queue := []string{start}
	for len(queue) > 0 {
		unit := queue[0]
		queue = queue[1:]
		next := make([]string, 0, len(deps[unit]))
		for candidate := range deps[unit] {
			next = append(next, candidate)
		}
		sort.Strings(next)
		for _, candidate := range next {
			if _, seen := prev[candidate]; seen {
				continue
			}
			prev[candidate] = unit
			if candidate == target {
				return rebuildUnitPath(prev, start, target)
			}
			queue = append(queue, candidate)
		}
	}
	return nil
}

// rebuildUnitPath reverses the breadth-first predecessor chain into a
// start-to-target unit path.
func rebuildUnitPath(prev map[string]string, start, target string) []string {
	path := []string{target}
	for unit := target; unit != start; {
		unit = prev[unit]
		if unit == "" {
			return nil
		}
		path = append(path, unit)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// promiseCycleThrough returns a cycle path that starts and ends at the promise
// id, or nil when the promise is not on a cycle.
func promiseCycleThrough(byID map[string]promiseNode, start string) []string {
	node, ok := byID[start]
	if !ok {
		return nil
	}
	deps := append([]string(nil), node.DependsOn...)
	sort.Strings(deps)
	for _, dep := range deps {
		if dep == start {
			return []string{start, start}
		}
		if path := promisePath(byID, dep, start); len(path) > 0 {
			return append([]string{start}, path...)
		}
	}
	return nil
}

// promisePath returns a node path from start to target (both inclusive) in the
// dependency graph, or nil when target is unreachable. Breadth-first search
// keeps the reported cycle short and the scan linear in the reachable graph.
func promisePath(byID map[string]promiseNode, start, target string) []string {
	prev := map[string]string{start: ""}
	queue := []string{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		node, ok := byID[id]
		if !ok {
			continue
		}
		deps := append([]string(nil), node.DependsOn...)
		sort.Strings(deps)
		for _, dep := range deps {
			if _, seen := prev[dep]; seen {
				continue
			}
			if _, exists := byID[dep]; !exists {
				continue
			}
			prev[dep] = id
			if dep == target {
				return rebuildPromisePath(prev, start, target)
			}
			queue = append(queue, dep)
		}
	}
	return nil
}

// rebuildPromisePath reverses the breadth-first predecessor chain into a
// start-to-target path.
func rebuildPromisePath(prev map[string]string, start, target string) []string {
	path := []string{target}
	for id := target; id != start; {
		id = prev[id]
		if id == "" {
			return nil
		}
		path = append(path, id)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// promiseCycleKey identifies a cycle by its member set, so the same cycle
// reached through two of the unit's promises is reported once.
func promiseCycleKey(cycle []string) string {
	seen := map[string]bool{}
	members := make([]string, 0, len(cycle))
	for _, id := range cycle {
		if seen[id] {
			continue
		}
		seen[id] = true
		members = append(members, id)
	}
	sort.Strings(members)
	return strings.Join(members, ",")
}

// CurrentFingerprints returns the current-layer promise ID → fingerprint map.
// Only repository-wide graph failures are returned: a malformed unrelated unit
// contributes no promise, and its own validate/plan/promote fails on its unit
// check; it must not make every consumer fingerprint computation fail.
func (g *PromiseGraph) CurrentFingerprints() (map[string]string, []string) {
	current := map[string]string{}
	issues := append([]string{}, g.globalIssues...)
	for _, node := range g.nodes {
		if _, exists := current[node.ID]; exists {
			continue
		}
		fingerprint, err := PromiseFingerprint(node.Promise)
		if err != nil {
			issues = append(issues, fmt.Sprintf("promise %q: %v", node.ID, err))
			continue
		}
		current[node.ID] = fingerprint
	}
	return current, issues
}

// Edges returns every promise dependency edge, sorted by source then target.
// Its issue list is diagnostic: it includes repository-wide failures and every
// unit-scoped parse issue so `deps` can report the malformed unit. Gate and
// freshness callers use Check or CurrentFingerprints, which keep unit issues
// scoped to their owner.
func (g *PromiseGraph) Edges() ([]PromiseEdge, []string) {
	var edges []PromiseEdge
	for _, node := range g.nodes {
		for _, dep := range node.DependsOn {
			edges = append(edges, PromiseEdge{From: node.ID, To: dep, Unit: node.Unit})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return edges, g.allIssues()
}

// allIssues lists every graph issue in deterministic order: repository-wide
// issues first, then each unit's scoped issues by unit name.
func (g *PromiseGraph) allIssues() []string {
	out := append([]string{}, g.globalIssues...)
	units := make([]string, 0, len(g.unitIssues))
	for unit := range g.unitIssues {
		units = append(units, unit)
	}
	sort.Strings(units)
	for _, unit := range units {
		out = append(out, g.unitIssues[unit]...)
	}
	return out
}

// CheckPromiseGraph is the single-unit convenience wrapper over the
// current-layer promise graph.
func CheckPromiseGraph(repoRoot, unitName string) CheckResult {
	return CheckPromiseGraphScope(repoRoot, PromiseScopeAll, unitName)
}

// CheckPromiseGraphScope is the single-unit promise graph check for one layer
// scope: all (current layer, candidate preferred), candidate, or stable.
func CheckPromiseGraphScope(repoRoot, scope, unitName string) CheckResult {
	graph, err := LoadPromiseGraphScope(repoRoot, scope)
	if err != nil {
		return CheckResult{Name: "Promise graph", Status: Fail, Details: err.Error()}
	}
	return graph.Check(unitName)
}

// PromiseScopeForLayer maps a spec layer to the promise graph scope a
// layer-scoped read uses. A candidate is proposed truth and resolves provider
// promises in the current layer (candidate preferred, stable fallback); a
// stable layer resolves providers in the published stable layer, so a
// candidate draft of a provider never changes the published contract state.
func PromiseScopeForLayer(layer string) string {
	if strings.EqualFold(strings.TrimSpace(layer), "stable") {
		return PromiseScopeStable
	}
	return PromiseScopeAll
}

// PromiseEdges returns the promise dependency edges for one layer scope.
func PromiseEdges(repoRoot, scope string) ([]PromiseEdge, []string) {
	graph, err := LoadPromiseGraphScope(repoRoot, scope)
	if err != nil {
		return nil, []string{err.Error()}
	}
	return graph.Edges()
}

// PromiseFingerprintsForScope returns the promise ID → fingerprint map for one
// layer scope. A stable-scope map is the published contract set; a promotion
// check compares a consumer's pins against it.
func PromiseFingerprintsForScope(repoRoot, scope string) (map[string]string, []string) {
	graph, err := LoadPromiseGraphScope(repoRoot, scope)
	if err != nil {
		return nil, []string{err.Error()}
	}
	return graph.CurrentFingerprints()
}

// Fingerprint returns the promise contract fingerprint in this graph's scope.
func (g *PromiseGraph) Fingerprint(promiseID string) (string, bool) {
	node, ok := g.byID[promiseID]
	if !ok {
		return "", false
	}
	fingerprint, err := PromiseFingerprint(node.Promise)
	if err != nil {
		return "", false
	}
	return fingerprint, true
}

// PromiseFingerprintByID resolves one promise contract in a layer scope. It is
// the pin lookup shared by judgment liveness and cache freshness.
func PromiseFingerprintByID(repoRoot, promiseID, scope string) (string, bool) {
	graph, err := LoadPromiseGraphScope(repoRoot, scope)
	if err != nil {
		return "", false
	}
	return graph.Fingerprint(promiseID)
}

// PromiseProvider returns the unit that owns a unit-owned promise id
// (`<unit>.<name>`). Unit names cannot contain dots, so the first segment is
// the provider.
func PromiseProvider(promiseID string) string {
	if i := strings.IndexByte(promiseID, '.'); i > 0 {
		return promiseID[:i]
	}
	return ""
}

// PromiseProviderUnits returns the provider units named by a spec's promise
// depends_on edges. It is the dependency-discovery half of the promise model:
// gate plans add these units to the read surface, and lifecycle checks use the
// same derivation.
func PromiseProviderUnits(content string) []string {
	deps := PromiseDependencyIDs(content)
	seen := map[string]bool{}
	for _, dep := range deps {
		provider := PromiseProvider(dep)
		if provider != "" {
			seen[provider] = true
		}
	}
	out := make([]string, 0, len(seen))
	for provider := range seen {
		out = append(out, provider)
	}
	sort.Strings(out)
	return out
}

// PromiseDependencyIDs returns every promise id named by a spec's promise
// depends_on edges, sorted and deduplicated. A malformed or absent promise
// section yields no IDs; the promise-model checks own that diagnosis.
func PromiseDependencyIDs(content string) []string {
	promises, issues := ParsePromises(content)
	if len(issues) > 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, promise := range promises {
		for _, dep := range promise.DependsOn {
			if dep != "" {
				seen[dep] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for dep := range seen {
		out = append(out, dep)
	}
	sort.Strings(out)
	return out
}

// PromiseDependenciesForItem returns the provider promise ids the promise
// verified by one acceptance item depends on, sorted and deduplicated. It is
// the item-scoped dependency set: an item session pins only these contracts,
// so an unrelated provider contract change does not reopen the item. A
// non-promise item, an item with no promise, or a malformed promise section
// yields no dependencies; the promise-model checks own that diagnosis.
func PromiseDependenciesForItem(content, itemID string) []string {
	promiseID := ""
	for _, item := range parseAcceptanceItems(content) {
		if item.id == itemID {
			promiseID = strings.TrimSpace(item.fields["promise"])
			break
		}
	}
	if promiseID == "" {
		return nil
	}
	promises, issues := ParsePromises(content)
	if len(issues) > 0 {
		return nil
	}
	for _, promise := range promises {
		if promise.ID != promiseID {
			continue
		}
		seen := map[string]bool{}
		out := make([]string, 0, len(promise.DependsOn))
		for _, dep := range promise.DependsOn {
			if dep == "" || seen[dep] {
				continue
			}
			seen[dep] = true
			out = append(out, dep)
		}
		sort.Strings(out)
		return out
	}
	return nil
}

// PromiseIDs returns every promise id a spec declares, sorted and
// deduplicated. A malformed or absent promise section yields no IDs.
func PromiseIDs(content string) []string {
	promises, issues := ParsePromises(content)
	if len(issues) > 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, promise := range promises {
		if promise.ID != "" {
			seen[promise.ID] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// loadPromiseNodes reads the unit specs in one layer scope. The "all" scope
// keeps the current-layer rule (candidate wins, stable fallback); the other
// scopes read exactly one layer.
//
// Failure attribution is deliberate: an unreadable spec is repository-wide
// because no unit's contracts can be proven complete; a missing or malformed
// `## Promises` section is scoped to its unit, because that unit simply
// contributes no promise nodes and its own validate/plan/promote fails. This
// keeps one un-migrated or broken unit from stalling every unrelated unit.
func loadPromiseNodes(repoRoot, scope string) ([]promiseNode, []string, map[string][]string) {
	units := map[string]string{}
	unitIssues := map[string][]string{}
	var globalIssues []string
	layers := []string{specpaths.CandidateDir, specpaths.StableDir}
	if scope == PromiseScopeCandidate {
		layers = []string{specpaths.CandidateDir}
	} else if scope == PromiseScopeStable {
		layers = []string{specpaths.StableDir}
	}
	for _, layer := range layers {
		dir := filepath.Join(repoRoot, filepath.FromSlash(layer))
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, []string{fmt.Sprintf("read %s: %v", layer, err)}, nil
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), "unit_") || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			unit := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "unit_"), ".md")
			if unit == "" {
				continue
			}
			// Candidate wins in the "all" scope; stable is only the fallback.
			if scope == PromiseScopeAll && layer == specpaths.StableDir {
				if _, exists := units[unit]; exists {
					continue
				}
			}
			units[unit] = filepath.Join(dir, entry.Name())
		}
	}

	names := make([]string, 0, len(units))
	for name := range units {
		names = append(names, name)
	}
	sort.Strings(names)

	var nodes []promiseNode
	for _, unit := range names {
		path := units[unit]
		data, _, err := specpaths.FileBytes(path)
		if err != nil {
			globalIssues = append(globalIssues, fmt.Sprintf("read %s: %v", path, err))
			continue
		}
		promises, parseIssues := ParsePromises(string(data))
		if len(parseIssues) > 0 {
			unitIssues[unit] = append(unitIssues[unit], fmt.Sprintf("%s: %s", path, strings.Join(parseIssues, "; ")))
			continue
		}
		for _, promise := range promises {
			nodes = append(nodes, promiseNode{
				ID:        promise.ID,
				Unit:      unit,
				File:      path,
				DependsOn: promise.DependsOn,
				Promise:   promise,
			})
		}
	}
	return nodes, globalIssues, unitIssues
}

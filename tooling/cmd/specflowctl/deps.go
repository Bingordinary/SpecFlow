package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitgraph"
)

func runDeps(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("deps", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	scopePtr := fs.String("scope", "all", "scope: all | candidate | stable")
	unitPtr := fs.String("unit", "", "unit name for single-unit dependency view")
	rulePtr := fs.String("rule", "", "rule id for single-rule consumer view")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absRoot, err := filepath.Abs(*repoRootPtr)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	scope := strings.TrimSpace(strings.ToLower(*scopePtr))
	unitName, err := requireTargetName("unit", *unitPtr)
	if err != nil {
		return err
	}
	ruleID, err := requireTargetName("rule", *rulePtr)
	if err != nil {
		return err
	}

	if unitName != "" && ruleID != "" {
		writeDepsUsage(stderr)
		return errors.New("--unit and --rule are mutually exclusive")
	}

	graph, err := unitgraph.Build(absRoot, scope)
	if err != nil {
		return err
	}
	promiseEdges, promiseIssues := specvalidation.PromiseEdges(absRoot, scope)
	// Publication state answers whether a consumer must wait for a provider's
	// contract to be published stable at the same fingerprint. It is an edge
	// annotation: the structural dependency order is computed over every
	// dependency edge.
	// A candidate consumer resolves providers in the current layer (candidate
	// preferred, stable fallback), so the current map uses the candidate
	// layer's promise scope even when the report scope is `candidate`.
	current, currentIssues := specvalidation.PromiseFingerprintsForScope(absRoot, specvalidation.PromiseScopeForLayer(scope))
	stable, stableIssues := specvalidation.PromiseFingerprintsForScope(absRoot, specvalidation.PromiseScopeStable)
	promiseIssues = append(promiseIssues, currentIssues...)
	promiseIssues = append(promiseIssues, stableIssues...)
	annotatedEdges := make([]depsEdge, 0, len(promiseEdges))
	for _, edge := range promiseEdges {
		blocking := false
		provider := specvalidation.PromiseProvider(edge.To)
		if provider != "" && provider != edge.Unit {
			// Only a candidate consumer can be blocked on a promotion. A stable
			// unit's edge is display/consumer information; it is not a pending
			// publication.
			fromNode := graph.Node(edge.Unit)
			blocking = fromNode != nil && fromNode.Layer == "candidate"
			if blocking {
				if currentFingerprint, ok := current[edge.To]; ok {
					if stableFingerprint, published := stable[edge.To]; published && stableFingerprint == currentFingerprint {
						blocking = false
					}
				}
			}
			if blocking {
				graph.AddUnpublishedPromiseDependency(edge.Unit, provider)
			} else {
				graph.AddPromiseDependency(edge.Unit, provider)
			}
		}
		annotatedEdges = append(annotatedEdges, depsEdge{edge: edge, blocking: blocking})
	}

	switch {
	case unitName != "":
		return writeUnitDeps(stdout, graph, unitName)
	case ruleID != "":
		return writeRuleDeps(stdout, graph, absRoot, ruleID)
	default:
		return writeAllDeps(stdout, graph, annotatedEdges, promiseIssues)
	}
}

// depsEdge is one printed promise edge plus its publication annotation.
type depsEdge struct {
	edge     specvalidation.PromiseEdge
	blocking bool // the consumer must wait for the provider contract's publication
}

func writeAllDeps(stdout io.Writer, graph *unitgraph.Graph, edges []depsEdge, promiseIssues []string) error {
	nodes := graph.Nodes()
	fmt.Fprintf(stdout, "DEPENDENCY GRAPH (scope: %s, %d unit(s))\n", graph.Scope(), len(nodes))
	fmt.Fprintln(stdout)

	if len(nodes) == 0 {
		fmt.Fprintln(stdout, "No units found.")
		return nil
	}

	fmt.Fprintln(stdout, "Rule refs (unit -> rule_refs):")
	for _, node := range nodes {
		if len(node.RuleRefs) == 0 {
			continue
		}
		fmt.Fprintf(stdout, "  %-24s -> %s\n", node.Name, strings.Join(node.RuleRefs, ", "))
	}
	if allRuleRefsEmpty(nodes) {
		fmt.Fprintln(stdout, "  (none)")
	}
	fmt.Fprintln(stdout)

	edgeIssues := promiseIssues
	if len(edges) > 0 || len(edgeIssues) > 0 {
		fmt.Fprintln(stdout, "Promise edges (promise -> depends_on; [wait] = provider contract not yet published stable):")
		for _, e := range edges {
			wait := ""
			if e.blocking {
				wait = "  [wait]"
			}
			fmt.Fprintf(stdout, "  %-32s -> %s%s\n", e.edge.From, e.edge.To, wait)
		}
		if len(edges) == 0 {
			fmt.Fprintln(stdout, "  (none)")
		}
		for _, issue := range edgeIssues {
			fmt.Fprintf(stdout, "  WARNING: %s\n", issue)
		}
		fmt.Fprintln(stdout)
	}

	unitCycles := graph.UnitCycles()
	if len(unitCycles) > 0 {
		fmt.Fprintf(stdout, "UNIT DEPENDENCY CYCLES (INVALID) (%d):\n", len(unitCycles))
		for _, cycle := range unitCycles {
			fmt.Fprintf(stdout, "  %s\n", strings.Join(cycle, ", "))
		}
		fmt.Fprintln(stdout, "  These units are mutually dependent: each has a promise that depends on")
		fmt.Fprintln(stdout, "  a promise of the other, so the units have no order and validate fails")
		fmt.Fprintln(stdout, "  them. Split the shared contract into a rule or redraw unit boundaries.")
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "UNIT DEPENDENCY CYCLES: none")
		fmt.Fprintln(stdout)
	}

	cycles := graph.Cycles()
	if len(cycles) > 0 {
		fmt.Fprintf(stdout, "CYCLES (PUBLICATION) (%d):\n", len(cycles))
		for _, cycle := range cycles {
			fmt.Fprintf(stdout, "  %s\n", strings.Join(cycle, ", "))
		}
		fmt.Fprintln(stdout, "  A publication deadlock: every member waits for another member's unpublished")
		fmt.Fprintln(stdout, "  provider contract, so none can be promoted first. The members are on the")
		fmt.Fprintln(stdout, "  unit dependency cycle reported above, which validate fails; split the")
		fmt.Fprintln(stdout, "  dependency or redraw unit boundaries.")
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "CYCLES (PUBLICATION): none")
		fmt.Fprintln(stdout)
	}

	order := graph.TopoOrder()
	candidateUnits := 0
	for _, node := range nodes {
		if node.Layer == "candidate" {
			candidateUnits++
		}
	}
	fmt.Fprintf(stdout, "Dependency order (structural, dependencies first; %d unit(s), %d group(s), %d candidate unit(s)):\n", len(nodes), len(order), candidateUnits)
	for i, group := range order {
		fmt.Fprintf(stdout, "  %d. %s", i+1, groupLabel(graph, group))
		if len(group) > 1 {
			fmt.Fprint(stdout, "  (unit dependency cycle — invalid; see above)")
		}
		fmt.Fprintln(stdout)
	}
	return nil
}

// groupLabel renders one group in the structural order, marking stable-only
// units because they are ordered but have no promotion action.
func groupLabel(graph *unitgraph.Graph, group []string) string {
	parts := make([]string, len(group))
	for i, name := range group {
		parts[i] = name
		if node := graph.Node(name); node != nil && node.Layer == "stable" {
			parts[i] = name + " (stable)"
		}
	}
	return strings.Join(parts, " + ")
}

func allRuleRefsEmpty(nodes []*unitgraph.Node) bool {
	for _, node := range nodes {
		if len(node.RuleRefs) > 0 {
			return false
		}
	}
	return true
}

func writeUnitDeps(stdout io.Writer, graph *unitgraph.Graph, unitName string) error {
	node := graph.Node(unitName)
	if node == nil {
		return fmt.Errorf("unit %q not found in scope %s", unitName, graph.Scope())
	}

	fmt.Fprintf(stdout, "DEPENDENCIES — %s (unit, %s layer)\n", unitName, node.Layer)
	fmt.Fprintln(stdout)

	if len(node.PromiseRefs) > 0 {
		fmt.Fprintln(stdout, "Promise provider refs (depends on):")
		for _, ref := range node.PromiseRefs {
			fmt.Fprintf(stdout, "  - %s\n", ref)
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Promise provider refs (depends on): none")
		fmt.Fprintln(stdout)
	}

	if len(node.UnpublishedRefs) > 0 {
		fmt.Fprintln(stdout, "Publication blockers (provider contract not published stable):")
		for _, ref := range node.UnpublishedRefs {
			fmt.Fprintf(stdout, "  - %s\n", ref)
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Publication blockers: none")
		fmt.Fprintln(stdout)
	}

	if group := graph.UnitCycleGroup(unitName); len(group) > 1 {
		fmt.Fprintln(stdout, "Unit dependency cycle (INVALID — validate fails these units):")
		fmt.Fprintf(stdout, "  - %s\n", strings.Join(group, ", "))
		fmt.Fprintln(stdout, "  Split the shared contract into a rule or redraw unit boundaries.")
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Unit dependency cycle: none")
		fmt.Fprintln(stdout)
	}

	if len(node.RuleRefs) > 0 {
		fmt.Fprintln(stdout, "Rule refs (bound rules):")
		for _, ref := range node.RuleRefs {
			fmt.Fprintf(stdout, "  - %s\n", ref)
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Rule refs (bound rules): none")
		fmt.Fprintln(stdout)
	}

	consumers := graph.PromiseConsumers(unitName)
	if len(consumers) > 0 {
		fmt.Fprintln(stdout, "Referenced by (promise depends_on):")
		for _, c := range consumers {
			fmt.Fprintf(stdout, "  - %s\n", c)
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Referenced by: none")
		fmt.Fprintln(stdout)
	}

	if graph.OnCycle(unitName) {
		fmt.Fprintln(stdout, "Publication cycle: ON A CYCLE — every member waits for another member's")
		fmt.Fprintln(stdout, "unpublished provider contract, so none can be promoted first. This is the")
		fmt.Fprintln(stdout, "promotion symptom of the unit dependency cycle reported above: validate fails")
		fmt.Fprintln(stdout, "these units, and publishing a provider does not clear the unit cycle. Split")
		fmt.Fprintln(stdout, "the dependency or redraw the unit boundaries (see")
		fmt.Fprintln(stdout, "specflow/framework/unit_validate_checklist.md Check 8).")
	} else {
		fmt.Fprintln(stdout, "Publication cycle: none")
	}
	return nil
}

func writeRuleDeps(stdout io.Writer, graph *unitgraph.Graph, repoRoot, ruleID string) error {
	// A global rule applies to every current-layer unit by default, but only
	// while its rule file exists — a missing file (removed rule, mistyped ID)
	// constrains nothing and is reported as not found (same semantics as
	// `consumers`).
	if strings.HasPrefix(ruleID, "g_rule_") && !globalRuleExists(repoRoot, ruleID) {
		return fmt.Errorf("rule %s not found in docs/specs/rules/ (candidate or stable)", ruleID)
	}
	consumers := graph.RuleConsumers(ruleID)
	fmt.Fprintf(stdout, "DEPENDENCIES — %s (rule)\n", ruleID)
	fmt.Fprintln(stdout)

	if len(consumers) > 0 {
		fmt.Fprintln(stdout, "Referenced by:")
		for _, c := range consumers {
			fmt.Fprintf(stdout, "  - %s\n", c)
		}
	} else {
		fmt.Fprintln(stdout, "Referenced by: none")
	}
	return nil
}

// globalRuleExists reports whether the named global rule file exists in the
// candidate or stable layer.
func globalRuleExists(repoRoot, ruleID string) bool {
	for _, ref := range []string{specpaths.RuleCandidateFileRef(ruleID), specpaths.RuleStableFileRef(ruleID)} {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(ref))); err == nil {
			return true
		}
	}
	return false
}

func writeDepsUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl deps [--scope all|candidate|stable] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl deps --unit UNIT [--scope ...] [--repo-root PATH]")
	fmt.Fprintln(w, "  specflowctl deps --rule RULE_ID [--scope ...] [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Builds the directed dependency graph from all in-scope units' promise")
	fmt.Fprintln(w, "depends_on edges and reports edges, unit dependency cycles, publication-")
	fmt.Fprintln(w, "order cycles, and a structural dependency order. Every dependency edge")
	fmt.Fprintln(w, "constrains that order; publication state is an annotation. A unit")
	fmt.Fprintln(w, "dependency cycle (two units mutually dependent through different promises)")
	fmt.Fprintln(w, "is a violation: validate fails the units on it. Promise contract cycles are")
	fmt.Fprintln(w, "reported by validate.")
	fmt.Fprintln(w, "Pure mechanical computation — no inference, no judgment, no file writes.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --scope SCOPE    all | candidate | stable (default: all)")
	fmt.Fprintln(w, "  --unit UNIT      Unit name for single-unit dependency view")
	fmt.Fprintln(w, "  --rule RULE_ID   Rule id for single-rule consumer view (a global rule")
	fmt.Fprintln(w, "                   g_rule_* applies to every current-layer unit by default)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

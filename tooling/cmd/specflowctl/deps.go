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
	// Publication order depends on whether the provider promise is already
	// published stable at its current contract. A candidate consumer resolves
	// providers in the current layer (candidate preferred, stable fallback), so
	// the current map uses the candidate layer's promise scope even when the
	// report scope is `candidate`.
	current, currentIssues := specvalidation.PromiseFingerprintsForScope(absRoot, specvalidation.PromiseScopeForLayer(scope))
	stable, stableIssues := specvalidation.PromiseFingerprintsForScope(absRoot, specvalidation.PromiseScopeStable)
	promiseIssues = append(promiseIssues, currentIssues...)
	promiseIssues = append(promiseIssues, stableIssues...)
	for _, edge := range promiseEdges {
		provider := specvalidation.PromiseProvider(edge.To)
		if provider == "" || provider == edge.Unit {
			continue
		}
		// Only a candidate unit has a promotion step. A stable unit's edge is
		// display/consumer information; it is not a pending publication.
		fromNode := graph.Node(edge.Unit)
		blocking := fromNode != nil && fromNode.Layer == "candidate"
		if blocking {
			if currentFingerprint, ok := current[edge.To]; ok {
				if stableFingerprint, published := stable[edge.To]; published && stableFingerprint == currentFingerprint {
					blocking = false
				}
			}
		}
		if blocking {
			graph.AddPromiseDependency(edge.Unit, provider)
		} else {
			graph.AddPublishedPromiseDependency(edge.Unit, provider)
		}
	}

	switch {
	case unitName != "":
		return writeUnitDeps(stdout, graph, unitName)
	case ruleID != "":
		return writeRuleDeps(stdout, graph, absRoot, ruleID)
	default:
		return writeAllDeps(stdout, graph, promiseEdges, promiseIssues)
	}
}

func writeAllDeps(stdout io.Writer, graph *unitgraph.Graph, promiseEdges []specvalidation.PromiseEdge, promiseIssues []string) error {
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

	edges, edgeIssues := promiseEdges, promiseIssues
	if len(edges) > 0 || len(edgeIssues) > 0 {
		fmt.Fprintln(stdout, "Promise edges (promise -> depends_on):")
		for _, edge := range edges {
			fmt.Fprintf(stdout, "  %-32s -> %s\n", edge.From, edge.To)
		}
		if len(edges) == 0 {
			fmt.Fprintln(stdout, "  (none)")
		}
		for _, issue := range edgeIssues {
			fmt.Fprintf(stdout, "  WARNING: %s\n", issue)
		}
		fmt.Fprintln(stdout)
	}

	cycles := graph.Cycles()
	if len(cycles) > 0 {
		fmt.Fprintf(stdout, "CYCLES (PUBLICATION) (%d):\n", len(cycles))
		for _, cycle := range cycles {
			fmt.Fprintf(stdout, "  %s\n", strings.Join(cycle, " -> "))
		}
		fmt.Fprintln(stdout, "  Over unpublished provider promises; this blocks publication order, not contract validity.")
		fmt.Fprintln(stdout, "  Promise contract cycles are reported by validate Check 15.")
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "CYCLES (PUBLICATION): none")
		fmt.Fprintln(stdout)
	}

	order := graph.TopoOrder()
	fmt.Fprintf(stdout, "Promotion order (%d of %d, dependencies first):\n", len(order), len(nodes))
	for _, name := range order {
		fmt.Fprintf(stdout, "  %d. %s\n", indexOfName(order, name)+1, name)
	}
	if len(order) < len(nodes) {
		fmt.Fprintln(stdout, "  (units without a promotion order are blocked by an unpublished dependency cycle — publish a provider first or split the dependency)")
	}
	return nil
}

func allRuleRefsEmpty(nodes []*unitgraph.Node) bool {
	for _, node := range nodes {
		if len(node.RuleRefs) > 0 {
			return false
		}
	}
	return true
}

func indexOfName(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
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

	if len(node.PublicationRefs) > 0 {
		fmt.Fprintln(stdout, "Publication blockers (provider contract not published stable):")
		for _, ref := range node.PublicationRefs {
			fmt.Fprintf(stdout, "  - %s\n", ref)
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Publication blockers: none")
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
		fmt.Fprintln(stdout, "Publication cycle: ON A CYCLE — publication order is blocked by an")
		fmt.Fprintln(stdout, "unpublished provider contract. This is not a promise contract cycle and")
		fmt.Fprintln(stdout, "validate may still pass. Publish a provider first or split the dependency")
		fmt.Fprintln(stdout, "(see specflow/framework/unit_validate_checklist.md Check 8).")
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
	fmt.Fprintln(w, "depends_on edges and reports edges, publication-order cycles, and a")
	fmt.Fprintln(w, "promotion order. Promise contract cycles are reported by validate.")
	fmt.Fprintln(w, "Pure mechanical computation — no inference, no judgment, no file writes.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --scope SCOPE    all | candidate | stable (default: all)")
	fmt.Fprintln(w, "  --unit UNIT      Unit name for single-unit dependency view")
	fmt.Fprintln(w, "  --rule RULE_ID   Rule id for single-rule consumer view (a global rule")
	fmt.Fprintln(w, "                   g_rule_* applies to every current-layer unit by default)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

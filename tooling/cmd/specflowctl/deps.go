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
	unitName := strings.TrimSpace(*unitPtr)
	ruleID := strings.TrimSpace(*rulePtr)

	if unitName != "" && ruleID != "" {
		writeDepsUsage(stderr)
		return errors.New("--unit and --rule are mutually exclusive")
	}

	graph, err := unitgraph.Build(absRoot, scope)
	if err != nil {
		return err
	}

	switch {
	case unitName != "":
		return writeUnitDeps(stdout, graph, unitName)
	case ruleID != "":
		return writeRuleDeps(stdout, graph, absRoot, ruleID)
	default:
		return writeAllDeps(stdout, graph)
	}
}

func writeAllDeps(stdout io.Writer, graph *unitgraph.Graph) error {
	nodes := graph.Nodes()
	fmt.Fprintf(stdout, "DEPENDENCY GRAPH (scope: %s, %d unit(s))\n", graph.Scope(), len(nodes))
	fmt.Fprintln(stdout)

	if len(nodes) == 0 {
		fmt.Fprintln(stdout, "No units found.")
		return nil
	}

	fmt.Fprintln(stdout, "Edges (unit -> unit_refs):")
	for _, node := range nodes {
		if len(node.UnitRefs) == 0 {
			fmt.Fprintf(stdout, "  %-24s (no unit refs)\n", node.Name)
			continue
		}
		fmt.Fprintf(stdout, "  %-24s -> %s\n", node.Name, strings.Join(node.UnitRefs, ", "))
	}
	fmt.Fprintln(stdout)

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

	cycles := graph.Cycles()
	if len(cycles) > 0 {
		fmt.Fprintf(stdout, "CYCLES (%d):\n", len(cycles))
		for _, cycle := range cycles {
			fmt.Fprintf(stdout, "  %s\n", strings.Join(cycle, " -> "))
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Cycles: none")
		fmt.Fprintln(stdout)
	}

	order := graph.TopoOrder()
	fmt.Fprintf(stdout, "Promotion order (%d of %d, dependencies first):\n", len(order), len(nodes))
	for _, name := range order {
		fmt.Fprintf(stdout, "  %d. %s\n", indexOfName(order, name)+1, name)
	}
	if len(order) < len(nodes) {
		fmt.Fprintln(stdout, "  (units without a promotion order are blocked by a cycle — resolve the cycle first)")
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

	if len(node.UnitRefs) > 0 {
		fmt.Fprintln(stdout, "Unit refs (depends on):")
		for _, ref := range node.UnitRefs {
			fmt.Fprintf(stdout, "  - %s\n", ref)
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Unit refs (depends on): none")
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

	consumers := graph.UnitConsumers(unitName)
	if len(consumers) > 0 {
		fmt.Fprintln(stdout, "Referenced by:")
		for _, c := range consumers {
			fmt.Fprintf(stdout, "  - %s\n", c)
		}
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintln(stdout, "Referenced by: none")
		fmt.Fprintln(stdout)
	}

	if graph.OnCycle(unitName) {
		fmt.Fprintln(stdout, "Cycle: ON A CYCLE — validate will FAIL this unit. Resolve by extracting the")
		fmt.Fprintln(stdout, "shared contract into a rule (star-shaped dependencies), or re-drawing unit")
		fmt.Fprintln(stdout, "boundaries (see g_rule_repository_baseline.md §6.1 item 4).")
	} else {
		fmt.Fprintln(stdout, "Cycle: none")
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
	fmt.Fprintln(w, "Builds the directed dependency graph from all in-scope units' declared")
	fmt.Fprintln(w, "unit_refs and reports edges, cycles, and a promotion order. Pure")
	fmt.Fprintln(w, "mechanical computation — no inference, no judgment, no file writes.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --scope SCOPE    all | candidate | stable (default: all)")
	fmt.Fprintln(w, "  --unit UNIT      Unit name for single-unit dependency view")
	fmt.Fprintln(w, "  --rule RULE_ID   Rule id for single-rule consumer view (a global rule")
	fmt.Fprintln(w, "                   g_rule_* applies to every current-layer unit by default)")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}

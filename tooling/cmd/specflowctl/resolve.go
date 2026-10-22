package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

type resolveType string

const (
	resolveUnit      resolveType = "unit"
	resolveRule      resolveType = "rule"
	resolveAmbiguous resolveType = "ambiguous"
	resolveNotFound  resolveType = "not_found"
)

type resolveResult struct {
	Target string
	Type   resolveType
	RuleID string
	Note   string
	Paths  []string
}

// runResolve renders the three-stage target resolution (framework/commands.md
// §Target Resolution) as a read-only lookup. It classifies a target text as
// unit, rule, ambiguous, or not found and prints every matched path, so the
// closed-form file-existence procedure is decided by tooling from current
// state instead of being re-decided by the executor on every trigger.
func runResolve(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("resolve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRoot := fs.String("repo-root", ".", "repository root")
	targetPtr := fs.String("target", "", "target text: a unit name or rule id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := strings.TrimSpace(*targetPtr)
	if target == "" {
		writeResolveUsage(stderr)
		return errors.New("target is required")
	}
	kind := "unit"
	if isRulePrefixed(target) {
		kind = "rule"
	}
	if err := specpaths.ValidateTargetName(kind, target); err != nil {
		return err
	}

	result := resolveTarget(mustAbs(*repoRoot), target)
	writeResolveResult(stdout, result)
	switch result.Type {
	case resolveUnit, resolveRule:
		return nil
	case resolveAmbiguous:
		return fmt.Errorf("ambiguous target %q: multiple objects match — ask the user to choose", target)
	default:
		return fmt.Errorf("target %q does not exist", target)
	}
}

func isRulePrefixed(target string) bool {
	return strings.HasPrefix(target, "g_rule_") || strings.HasPrefix(target, "b_rule_")
}

type resolvedObject struct {
	ruleID string // empty for a unit object
	paths  []string
}

// resolveTarget implements the documented three-stage process. Stage 1 inspects
// both layers for every exact possibility before deciding; the same logical
// object in both layers is one match, and a unit name and a rule id are
// different objects even when the target text is the same. A target that
// already carries a rule prefix is the full rule id, so only its rule file is
// looked up — appending the prefix again would look for a file no rule id can
// name. Stage 2/3 apply only when Stage 1 found nothing.
func resolveTarget(repoRoot, target string) resolveResult {
	var objects []resolvedObject
	if isRulePrefixed(target) {
		if paths := existingPaths(repoRoot,
			specpaths.RuleCandidateFileRef(target),
			specpaths.RuleStableFileRef(target)); len(paths) > 0 {
			objects = append(objects, resolvedObject{ruleID: target, paths: paths})
		}
	} else {
		if paths := existingPaths(repoRoot,
			specpaths.CandidateUnitSpecFileRef(target),
			specpaths.StableUnitSpecFileRef(target)); len(paths) > 0 {
			objects = append(objects, resolvedObject{paths: paths})
		}
		for _, prefix := range []string{"g_rule_", "b_rule_"} {
			paths := existingPaths(repoRoot,
				specpaths.RuleCandidateFileRef(prefix+target),
				specpaths.RuleStableFileRef(prefix+target))
			if len(paths) > 0 {
				objects = append(objects, resolvedObject{ruleID: prefix + target, paths: paths})
			}
		}
	}

	switch len(objects) {
	case 1:
		return objectResult(target, objects[0])
	case 2, 3:
		return ambiguousResult(target, objects)
	}

	// Stage 2 — a rule-prefixed target with no files is a new rule target.
	if isRulePrefixed(target) {
		return resolveResult{
			Target: target,
			Type:   resolveRule,
			RuleID: target,
			Note:   "new target — no spec files exist yet",
		}
	}

	// Stage 3 — rule directory substring fallback for a prefix-less target.
	matches := ruleDirSubstringMatches(repoRoot, target)
	ids := make([]string, 0, len(matches))
	for id := range matches {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 1 {
		return resolveResult{
			Target: target,
			Type:   resolveRule,
			RuleID: ids[0],
			Paths:  matches[ids[0]],
		}
	}
	if len(ids) >= 2 {
		var paths []string
		for _, id := range ids {
			paths = append(paths, matches[id]...)
		}
		sort.Strings(paths)
		return resolveResult{Target: target, Type: resolveAmbiguous, Paths: paths}
	}
	return resolveResult{Target: target, Type: resolveNotFound}
}

func objectResult(target string, obj resolvedObject) resolveResult {
	if obj.ruleID == "" {
		return resolveResult{Target: target, Type: resolveUnit, Paths: obj.paths}
	}
	return resolveResult{Target: target, Type: resolveRule, RuleID: obj.ruleID, Paths: obj.paths}
}

func ambiguousResult(target string, objects []resolvedObject) resolveResult {
	var paths []string
	for _, obj := range objects {
		paths = append(paths, obj.paths...)
	}
	sort.Strings(paths)
	return resolveResult{Target: target, Type: resolveAmbiguous, Paths: paths}
}

func existingPaths(repoRoot string, refs ...string) []string {
	var found []string
	for _, ref := range refs {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(ref))); err == nil {
			found = append(found, ref)
		}
	}
	return found
}

// ruleDirSubstringMatches scans both rule layers for filenames containing the
// target and groups the matched paths by the rule id spelled in the filename.
func ruleDirSubstringMatches(repoRoot, target string) map[string][]string {
	matches := map[string][]string{}
	for _, dir := range []string{specpaths.RuleCandidateDir, specpaths.RuleStableDir} {
		entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(dir)))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".md") || !strings.Contains(name, target) {
				continue
			}
			ruleID := strings.TrimSuffix(name, ".md")
			matches[ruleID] = append(matches[ruleID], fmt.Sprintf("%s/%s", dir, name))
		}
	}
	return matches
}

func writeResolveResult(w io.Writer, r resolveResult) {
	fmt.Fprintf(w, "target: %s\n", r.Target)
	fmt.Fprintf(w, "type: %s\n", r.Type)
	if r.RuleID != "" {
		fmt.Fprintf(w, "rule_id: %s\n", r.RuleID)
	}
	if r.Note != "" {
		fmt.Fprintf(w, "note: %s\n", r.Note)
	}
	if len(r.Paths) == 0 {
		fmt.Fprintln(w, "matches: none")
		return
	}
	fmt.Fprintln(w, "matches:")
	for _, p := range r.Paths {
		fmt.Fprintf(w, "- %s\n", p)
	}
}

func writeResolveUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: specflowctl resolve --target <name> [--repo-root PATH]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Read-only three-stage target resolution (framework/commands.md §Target Resolution):")
	fmt.Fprintln(w, "classifies a target as unit, rule, ambiguous, or not found and lists every matched path.")
}

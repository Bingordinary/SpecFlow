package operation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/writezone"
)

// OpenOptions carries the caller declarations of operation open.
type OpenOptions struct {
	Unit        string
	Rule        string
	Allow       []string
	RequireSpec []string
	Baseline    string
	Parent      string
}

// OpenResult is the outcome of operation open: the frozen state plus any
// open-time notices (informational only — nothing fails on pre-existing
// working-tree changes).
type OpenResult struct {
	Operation *Operation
	Notices   []string
}

// Open declares and freezes one operation scope and writes its state file.
// The spec-derived part of the scope is resolved from the target's
// current-layer spec; the caller-declared part comes from --allow. Both are
// frozen here — only operation update can change the scope afterwards.
func Open(repoRoot string, opts OpenOptions, now time.Time) (*OpenResult, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	unit := strings.TrimSpace(opts.Unit)
	rule := strings.TrimSpace(opts.Rule)
	if unit != "" && rule != "" {
		return nil, fmt.Errorf("--unit and --rule are mutually exclusive")
	}
	// Validate the target name before any spec-derived path construction or
	// filesystem access (tooling/README.md §Target names) — including the git
	// worktree check below, so an invalid name fails without reading the repo.
	if unit != "" {
		if err := validateTarget(Target{Kind: TargetKindUnit, Name: unit}); err != nil {
			return nil, err
		}
	}
	if rule != "" {
		if err := validateTarget(Target{Kind: TargetKindRule, Name: rule}); err != nil {
			return nil, err
		}
	}

	if err := requireWorkTreeTop(absRoot); err != nil {
		return nil, err
	}

	baselineRef := strings.TrimSpace(opts.Baseline)
	if baselineRef == "" {
		baselineRef = "HEAD"
	}
	sha, err := resolveCommit(absRoot, baselineRef)
	if err != nil {
		return nil, err
	}

	allowed, err := deriveAllowedPaths(absRoot, unit, rule, opts.Allow)
	if err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("operation scope is empty: declare --allow paths or target a unit/rule with spec-declared surfaces")
	}
	required, err := canonicalRequiredSpecPaths(absRoot, opts.RequireSpec)
	if err != nil {
		return nil, err
	}

	parent := strings.TrimSpace(opts.Parent)
	if parent != "" {
		if _, err := Load(absRoot, parent); err != nil {
			return nil, fmt.Errorf("parent operation: %w", err)
		}
	}

	opID, err := uniqueOperationID(absRoot, now)
	if err != nil {
		return nil, err
	}

	ts := now.UTC().Format(timestampLayout)
	target := Target{Kind: TargetKindNone}
	switch {
	case unit != "":
		target = Target{Kind: TargetKindUnit, Name: unit}
	case rule != "":
		target = Target{Kind: TargetKindRule, Name: rule}
	}

	op := &Operation{
		OperationID:       opID,
		Status:            StatusOpen,
		Target:            target,
		Baseline:          Baseline{Ref: baselineRef, SHA: sha, RecordedAt: ts},
		AllowedPaths:      allowed,
		RequiredSpecPaths: required,
		ParentOperation:   parent,
		OpenedAt:          ts,
		UpdatedAt:         ts,
	}

	result := &OpenResult{Operation: op}
	report, err := evaluate(absRoot, op)
	if err != nil {
		return nil, err
	}
	if n := len(report.OutOfScope) + len(report.StaticViolations); n > 0 {
		result.Notices = append(result.Notices, fmt.Sprintf("%d path(s) already changed are outside the declared scope or violate the static write zones; `operation check` will fail until they are reverted.", n))
	}

	if err := Save(absRoot, op); err != nil {
		return nil, err
	}
	return result, nil
}

func uniqueOperationID(repoRoot string, now time.Time) (string, error) {
	for i := 0; i < 3; i++ {
		id, err := newOperationID(now)
		if err != nil {
			return "", err
		}
		path, err := statePath(repoRoot, id)
		if err != nil {
			return "", err
		}
		_, statErr := os.Stat(path)
		if os.IsNotExist(statErr) {
			return id, nil
		}
		if statErr != nil {
			return "", fmt.Errorf("check operation id: %w", statErr)
		}
	}
	return "", fmt.Errorf("could not generate a unique operation id")
}

// deriveAllowedPaths resolves the frozen allowed scope: the spec-derived
// entries of the target (append order encodes source priority) followed by
// the caller-declared entries.
func deriveAllowedPaths(repoRoot, unit, rule string, allow []string) ([]AllowedPath, error) {
	var entries []AllowedPath
	switch {
	case unit != "":
		specEntries, err := unitSpecEntries(repoRoot, unit)
		if err != nil {
			return nil, err
		}
		entries = append(entries, specEntries...)
	case rule != "":
		entry, err := ruleSpecEntry(repoRoot, rule)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	default:
		if len(allow) == 0 {
			return nil, fmt.Errorf("a path-only operation requires at least one --allow path")
		}
	}
	declared, err := canonicalDeclaredPaths(repoRoot, allow, "--allow")
	if err != nil {
		return nil, err
	}
	for _, p := range declared {
		entries = append(entries, AllowedPath{Path: p, Source: SourceDeclared})
	}
	for i := range entries {
		canonical, err := repopath.Canonical(repoRoot, entries[i].Path)
		if err != nil {
			return nil, fmt.Errorf("%s scope path %q: %w", entries[i].Source, entries[i].Path, err)
		}
		entries[i].Path = canonical
	}
	return dedupeAllowed(entries), nil
}

// unitSpecEntries resolves a unit target's spec-derived scope: the candidate
// spec file and its candidate appendix files, the acceptance items'
// implementation_surface values, and the affects.files values.
//
// When only a stable spec exists, the scope carries the deterministic
// candidate paths (the fork targets: the candidate main spec plus the
// candidate equivalents of the stable appendices) instead of the
// never-writable stable files — the fork is the standard first step of a spec
// change (HARD RULE 5), and its output must already be in scope. The
// spec-declared surfaces are read from the stable spec in that case (they are
// the current design intent until the fork).
func unitSpecEntries(repoRoot, unit string) ([]AllowedPath, error) {
	candidate := specpaths.CandidateUnitSpecFileRef(unit)
	stable := specpaths.StableUnitSpecFileRef(unit)
	candidateExists := fileExists(filepath.Join(repoRoot, filepath.FromSlash(candidate)))
	stableExists := fileExists(filepath.Join(repoRoot, filepath.FromSlash(stable)))
	if !candidateExists && !stableExists {
		return nil, fmt.Errorf("unit %q has no spec in either layer; create or fork the spec first", unit)
	}

	var entries []AllowedPath
	specPath := candidate
	if candidateExists {
		entries = append(entries, AllowedPath{Path: candidate, Source: SourceSpecFile})
		for _, appendix := range appendixFiles(repoRoot, unit, "candidate") {
			entries = append(entries, AllowedPath{Path: appendix, Source: SourceSpecFile})
		}
	} else {
		specPath = stable
		entries = append(entries, AllowedPath{Path: candidate, Source: SourceSpecFile})
		for _, appendix := range appendixFiles(repoRoot, unit, "stable") {
			entries = append(entries, AllowedPath{Path: candidateAppendixPath(appendix), Source: SourceSpecFile})
		}
	}

	canonicalReadPath, err := repopath.Canonical(repoRoot, specPath)
	if err != nil {
		return nil, fmt.Errorf("target spec %q: %w", specPath, err)
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(canonicalReadPath)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", specPath, err)
	}
	for _, surface := range specvalidation.ExtractImplementationSurfaces(string(content)) {
		p, err := canonicalSpecPath(repoRoot, surface)
		if err != nil {
			return nil, fmt.Errorf("implementation_surface %q: %w", surface, err)
		}
		if p != "" {
			entries = append(entries, AllowedPath{Path: p, Source: SourceImplSurface})
		}
	}
	for _, file := range specvalidation.ExtractAffectsFiles(string(content)) {
		p, err := canonicalSpecPath(repoRoot, file)
		if err != nil {
			return nil, fmt.Errorf("affects.files %q: %w", file, err)
		}
		if p != "" {
			entries = append(entries, AllowedPath{Path: p, Source: SourceAffectsFiles})
		}
	}
	return entries, nil
}

// ruleSpecEntry resolves a rule target's spec-derived scope: the candidate
// rule file. When only a stable rule exists, the candidate path is the fork
// target (HARD RULE 5) — the never-writable stable file is not a scope entry.
func ruleSpecEntry(repoRoot, ruleID string) (AllowedPath, error) {
	candidate := specpaths.RuleCandidateFileRef(ruleID)
	stable := specpaths.RuleStableFileRef(ruleID)
	if !fileExists(filepath.Join(repoRoot, filepath.FromSlash(candidate))) &&
		!fileExists(filepath.Join(repoRoot, filepath.FromSlash(stable))) {
		return AllowedPath{}, fmt.Errorf("rule %q has no file in either layer; create or fork the rule file first", ruleID)
	}
	checkPath := candidate
	if !fileExists(filepath.Join(repoRoot, filepath.FromSlash(candidate))) {
		checkPath = stable
	}
	if _, err := repopath.Canonical(repoRoot, checkPath); err != nil {
		return AllowedPath{}, fmt.Errorf("target rule %q: %w", checkPath, err)
	}
	return AllowedPath{Path: candidate, Source: SourceSpecFile}, nil
}

// candidateAppendixPath maps a stable appendix path to its candidate
// equivalent; the fork target preserves the appendix base name.
func candidateAppendixPath(stableAppendix string) string {
	return filepath.ToSlash(filepath.Join(specpaths.CandidateAppendixDir, filepath.Base(stableAppendix)))
}

// appendixFiles lists a unit's appendix files in one layer.
func appendixFiles(repoRoot, unit, layer string) []string {
	dir := specpaths.CandidateAppendixDir
	if layer == "stable" {
		dir = specpaths.StableAppendixDir
	}
	pattern := filepath.Join(repoRoot, filepath.FromSlash(dir), fmt.Sprintf("unit_%s_*.md", unit))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var out []string
	for _, match := range matches {
		rel, relErr := filepath.Rel(repoRoot, match)
		if relErr != nil {
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out
}

// canonicalSpecPath normalizes a spec-declared path. Empty values and the
// <pending> placeholder carry no scope; lexical or symlink escapes fail
// closed. Spec-derived entries are not validated against the static zones
// here: the static policy layer governs actual writes at check time.
func canonicalSpecPath(repoRoot, p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`)
	if p == "" || p == "<pending>" {
		return "", nil
	}
	canonical, err := repopath.Canonical(repoRoot, p)
	if err != nil {
		return "", err
	}
	return canonical, nil
}

// canonicalDeclaredPaths normalizes caller-declared paths and fails closed:
// an entry outside the repository, resolving to the root, inside the local
// state/derived-cache exclusions, or inside a denied write zone is rejected
// at declaration time.
func canonicalDeclaredPaths(repoRoot string, paths []string, flagName string) ([]string, error) {
	classifier, err := writezone.New(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve write-zone policy: %w", err)
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" {
			return nil, fmt.Errorf("%s contains an empty path", flagName)
		}
		relSlash, err := repopath.Canonical(repoRoot, p)
		if err != nil {
			return nil, fmt.Errorf("%s path %q: %w", flagName, p, err)
		}
		if excluded(relSlash) {
			return nil, fmt.Errorf("%s path %q is in the tooling's local state or derived-cache area and cannot be an operation scope entry", flagName, p)
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(relSlash))
		if zone := classifier.Classify(abs); !zone.Allowed {
			return nil, fmt.Errorf("%s path %q is in a denied write zone: %s", flagName, p, zone.Reason)
		}
		if !seen[relSlash] {
			seen[relSlash] = true
			out = append(out, relSlash)
		}
	}
	sort.Strings(out)
	return out, nil
}

// canonicalRequiredSpecPaths keeps the spec-first proof tied to actual
// candidate spec artifacts. A missing candidate is valid because creating it
// may be the first operation step; arbitrary code and documentation paths are
// not valid substitutes.
func canonicalRequiredSpecPaths(repoRoot string, paths []string) ([]string, error) {
	canonical, err := canonicalDeclaredPaths(repoRoot, paths, "--require-spec")
	if err != nil {
		return nil, err
	}
	for _, rel := range canonical {
		if !isCandidateSpecPath(rel) {
			return nil, fmt.Errorf("--require-spec path %q is not a candidate unit spec, unit appendix, or rule spec", rel)
		}
		if info, statErr := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel))); statErr == nil && info.IsDir() {
			return nil, fmt.Errorf("--require-spec path %q is a directory, not a candidate spec file", rel)
		}
	}
	return canonical, nil
}

func isCandidateSpecPath(rel string) bool {
	dir := filepath.ToSlash(filepath.Dir(rel))
	base := filepath.Base(rel)
	if filepath.Ext(base) != ".md" {
		return false
	}
	stem := strings.TrimSuffix(base, ".md")
	switch dir {
	case specpaths.CandidateDir:
		return strings.HasPrefix(stem, "unit_") && len(strings.TrimPrefix(stem, "unit_")) > 0
	case specpaths.CandidateAppendixDir:
		rest := strings.TrimPrefix(stem, "unit_")
		split := strings.LastIndex(rest, "_")
		return strings.HasPrefix(stem, "unit_") && split > 0 && split < len(rest)-1
	case specpaths.RuleCandidateDir:
		for _, prefix := range []string{"g_rule_", "b_rule_"} {
			if strings.HasPrefix(stem, prefix) && len(strings.TrimPrefix(stem, prefix)) > 0 {
				return true
			}
		}
	}
	return false
}

// dedupeAllowed keeps the first entry per path (append order encodes source
// priority: spec:file > spec:implementation_surface > spec:affects.files >
// declared) and returns the result sorted by path.
func dedupeAllowed(entries []AllowedPath) []AllowedPath {
	seen := map[string]bool{}
	var out []AllowedPath
	for _, entry := range entries {
		if entry.Path == "" || seen[entry.Path] {
			continue
		}
		seen[entry.Path] = true
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func fileExists(abs string) bool {
	info, err := os.Stat(abs)
	return err == nil && !info.IsDir()
}

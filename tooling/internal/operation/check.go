package operation

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/writezone"
)

// Report is the mechanical evaluation of one operation scope against the
// current change set.
type Report struct {
	Operation        *Operation
	Changed          []string
	OutOfScope       []string
	StaticViolations []string
	MissingRequired  []string
	Result           string // PASS | FAIL
}

// Check evaluates one operation scope against the current working tree.
// It is read-only and fails closed: a missing operation, a missing baseline
// commit, or a git error is an error, not a pass.
func Check(repoRoot, operationID string) (*Report, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	if err := requireWorkTreeTop(absRoot); err != nil {
		return nil, err
	}
	op, err := Load(absRoot, strings.TrimSpace(operationID))
	if err != nil {
		return nil, err
	}
	return evaluate(absRoot, op)
}

// evaluate computes the report for an in-memory operation state.
func evaluate(repoRoot string, op *Operation) (*Report, error) {
	changed, err := changedPaths(repoRoot, op.Baseline.SHA)
	if err != nil {
		return nil, err
	}
	classifier, err := writezone.New(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve write-zone policy: %w", err)
	}

	report := &Report{Operation: op, Changed: changed}
	changedSet := map[string]bool{}
	for _, p := range changed {
		changedSet[p] = true
		zone := classifier.Classify(filepath.Join(repoRoot, filepath.FromSlash(p)))
		if !zone.Allowed {
			report.StaticViolations = append(report.StaticViolations, p)
			continue
		}
		if !inScope(p, op.AllowedPaths) {
			report.OutOfScope = append(report.OutOfScope, p)
		}
	}
	for _, required := range op.RequiredSpecPaths {
		// The obligation is that the spec file is present in its updated form:
		// a deletion appears in the change set but leaves no spec behind, so
		// existence at check time is part of the mechanical spec-first
		// guarantee.
		if !changedSet[required] || !fileExists(filepath.Join(repoRoot, filepath.FromSlash(required))) {
			report.MissingRequired = append(report.MissingRequired, required)
		}
	}

	if len(report.OutOfScope) == 0 && len(report.StaticViolations) == 0 && len(report.MissingRequired) == 0 {
		report.Result = "PASS"
	} else {
		report.Result = "FAIL"
	}
	return report, nil
}

// inScope reports whether a changed path falls inside the frozen scope: a
// path equals an entry or lives under it. Directory entries therefore cover
// files created during the operation; there is no glob support.
func inScope(p string, allowed []AllowedPath) bool {
	for _, entry := range allowed {
		if p == entry.Path || strings.HasPrefix(p, entry.Path+"/") {
			return true
		}
	}
	return false
}

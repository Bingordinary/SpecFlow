package gaterun

import (
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

// Derivation is one CLI invocation's read-only view of the repository.
// Repository content cannot change while a derivation runs, so the directory
// expansions and the repo-wide surface audit it needs are invocation
// constants: each is computed once and shared by every per-unit derivation
// of the invocation instead of being re-derived per unit.
type Derivation struct {
	root      string
	expander  *repofiles.Expander
	audit     *specvalidation.SurfaceAuditReport
	auditErr  error
	auditDone bool
}

// NewDerivation resolves the invocation's git worktree once. A root that is
// not the worktree top level fails here instead of once per expansion.
func NewDerivation(root string) (*Derivation, error) {
	expander, err := repofiles.NewExpander(root)
	if err != nil {
		return nil, err
	}
	return &Derivation{root: root, expander: expander}, nil
}

// surfaceAudit computes the repo-wide surface audit once per derivation.
// The audit is a function of the repository alone — the per-unit aspect of
// protected coverage is the post-audit filtering, not the audit itself — so
// every unit's protected coverage reads the same report.
func (d *Derivation) surfaceAudit() (*specvalidation.SurfaceAuditReport, error) {
	if !d.auditDone {
		d.audit, d.auditErr = specvalidation.SurfaceAuditWith(d.expander, d.root)
		d.auditDone = true
	}
	return d.audit, d.auditErr
}

package gaterun

import "testing"

// TestDerivationSharesAuditAndExpansions pins the invocation-sharing
// contract: deriving several units through one Derivation computes the
// repo-wide surface audit once and expands D distinct directories once,
// instead of repeating the audit and its per-declaration expansions per unit.
func TestDerivationSharesAuditAndExpansions(t *testing.T) {
	repoRoot := newRepo(t)
	writeFile(t, repoRoot, "src/alpha.go", "package src\n")
	writeUnit(t, repoRoot, "candidate", "alpha", "none", "none", "src", "")
	writeUnit(t, repoRoot, "candidate", "beta", "none", "none", "src", "")

	d, err := NewDerivation(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range []string{"alpha", "beta"} {
		if _, err := d.ExpectedChecks(unit, TargetCandidate); err != nil {
			t.Fatalf("expected checks for %s: %v", unit, err)
		}
	}
	// The distinct expanded directories are the declared code surface (src)
	// and the evidence corpus root ("."): two units each declaring src must
	// not multiply either.
	if got := d.expander.Expansions(); got != 2 {
		t.Fatalf("two units declaring D=1 directory must expand D+corpus times, got %d", got)
	}
	if !d.auditDone {
		t.Fatal("the derivation must have computed the repo-wide surface audit")
	}
	audit, err := d.surfaceAudit()
	if err != nil || audit != d.audit {
		t.Fatalf("surfaceAudit must memoize its report, got %+v, %v", audit, err)
	}
}

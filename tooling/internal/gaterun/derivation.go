package gaterun

import (
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
)

// Derivation is one CLI invocation's read-only view of the repository.
// Repository content cannot change while a derivation runs, so the directory
// expansions it needs are invocation constants: they are computed once and
// shared by every per-unit derivation of the invocation instead of being
// re-derived per unit.
type Derivation struct {
	root     string
	expander *repofiles.Expander
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

package gaterun

import (
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// ValidateUnitGroupIDs returns the unit validate coverage group ids in
// contract order. It is the key set a unit validate cache binds its per-group
// standards under.
func ValidateUnitGroupIDs() []string {
	groups := unitValidateGroups()
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

// ValidateUnitStandards computes the deployed unit validate judging standard
// split by the session that reads it: one fingerprint per coverage group (the
// shared judging baseline plus its own check sections) and one for the final
// synthesis. The cache package owns the computation because its freshness
// chain must reproduce the same values.
func ValidateUnitStandards(repoRoot string) (map[string]string, error) {
	return validationcache.ValidateUnitStandards(repoRoot)
}

package main

import (
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// requireTargetName trims and validates a CLI-provided unit name or rule id
// before any path construction (tooling/README.md §Target names). An empty
// value is returned as-is so the caller keeps its own usage handling; a
// non-empty value must match the target-name grammar, so a name outside it —
// a separator, traversal segment, or whitespace — can never reach a path
// builder.
func requireTargetName(kind, raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", nil
	}
	if err := specpaths.ValidateTargetName(kind, name); err != nil {
		return "", err
	}
	return name, nil
}

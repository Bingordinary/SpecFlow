package operation

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// UpdateOptions carries the explicit scope additions. HasAllow and
// HasRequireSpec record flag presence: an absent flag keeps the frozen value,
// while a present flag adds its entries to that value.
type UpdateOptions struct {
	Allow          []string
	HasAllow       bool
	RequireSpec    []string
	HasRequireSpec bool
}

// Update widens the caller-declared allowed scope and/or required spec paths
// while the operation is open. Both sets are monotonic: an update can add but
// never remove a frozen path. The spec-derived part is never recomputed here.
// Every update records the resulting complete declared state as an event.
func Update(repoRoot, operationID string, opts UpdateOptions, now time.Time) (*Operation, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	if err := requireWorkTreeTop(absRoot); err != nil {
		return nil, err
	}
	if strings.TrimSpace(operationID) == "" {
		return nil, fmt.Errorf("operation id is required")
	}
	if !opts.HasAllow && !opts.HasRequireSpec {
		return nil, fmt.Errorf("nothing to update: pass --allow and/or --require-spec")
	}
	var updated *Operation
	err = withMutation(absRoot, func() error {
		op, err := Load(absRoot, strings.TrimSpace(operationID))
		if err != nil {
			return err
		}
		if op.Status != StatusOpen {
			return fmt.Errorf("operation %s is closed; open a new operation instead of widening a closed one", op.OperationID)
		}

		if opts.HasAllow {
			declared, err := canonicalDeclaredPaths(absRoot, opts.Allow, "--allow")
			if err != nil {
				return err
			}
			kept := append([]AllowedPath(nil), op.AllowedPaths...)
			for _, p := range declared {
				kept = append(kept, AllowedPath{Path: p, Source: SourceDeclared})
			}
			op.AllowedPaths = dedupeAllowed(kept)
		}
		if opts.HasRequireSpec {
			combined := append(append([]string(nil), op.RequiredSpecPaths...), opts.RequireSpec...)
			required, err := canonicalRequiredSpecPaths(absRoot, combined)
			if err != nil {
				return err
			}
			op.RequiredSpecPaths = required
		}

		ts := now.UTC().Format(timestampLayout)
		op.UpdatedAt = ts
		op.Updates = append(op.Updates, UpdateEvent{
			At:                   ts,
			DeclaredAllowedPaths: declaredPaths(op.AllowedPaths),
			RequiredSpecPaths:    append([]string{}, op.RequiredSpecPaths...),
		})
		if err := Save(absRoot, op); err != nil {
			return err
		}
		updated = op
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// declaredPaths extracts the caller-declared paths of the frozen scope in
// path order (the entries are already sorted).
func declaredPaths(allowed []AllowedPath) []string {
	out := []string{}
	for _, entry := range allowed {
		if entry.Source == SourceDeclared {
			out = append(out, entry.Path)
		}
	}
	return out
}

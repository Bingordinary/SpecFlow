package judgments

import (
	"fmt"
	"path/filepath"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// Unit-scan row bookkeeping: the unit-level scan conclusion keeps one
// aggregate verdict, and its checklist work is recorded as rows — one row per
// structural-declaration source, one per declared code file's surplus check,
// one per non-runnable acceptance item, and one coverage row for the Step 5
// Part 2 runnable-set assessment. Each row pins its own evidence, so a change
// invalidates exactly the rows that read it instead of the whole unit-wide
// conclusion (framework/validation_cache.md §Unit-scan row bookkeeping).

const (
	RowKindDeclaration = "declaration"  // one structural-declaration source: a promise entry or an evidence-record section
	RowKindSurplus     = "surplus"      // one declared code file's Step 5 Part A surplus check
	RowKindNonRunnable = "non_runnable" // one `runnable: no` acceptance item's Step 5 Part 2 item assessment
	RowKindCoverage    = "coverage"     // the Step 5 Part 2 runnable-set coverage assessment over the complete acceptance-item set
)

// Row verdicts per kind.
const (
	RowMatch        = "match"
	RowMismatch     = "mismatch"
	RowUndetermined = "undetermined"

	RowClean   = "clean"
	RowSurplus = "surplus"

	RowOK     = "ok"
	RowDefect = "defect"
)

// RowPin is one row of the unit-level scan's checklist bookkeeping. The row is
// the delta unit: its own dependency pins decide whether that row's recorded
// verdict still holds, independently of the other rows.
type RowPin struct {
	ID      string       `json:"id"`
	Kind    string       `json:"kind"`
	Verdict string       `json:"verdict"`
	Deps    []Dependency `json:"deps"`
}

func (p RowPin) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("row pin has no id")
	}
	switch p.Kind {
	case RowKindDeclaration:
		switch p.Verdict {
		case RowMatch, RowMismatch, RowUndetermined:
		default:
			return fmt.Errorf("row %s: invalid declaration verdict %q", p.ID, p.Verdict)
		}
	case RowKindSurplus:
		switch p.Verdict {
		case RowClean, RowSurplus:
		default:
			return fmt.Errorf("row %s: invalid surplus verdict %q", p.ID, p.Verdict)
		}
	case RowKindNonRunnable:
		switch p.Verdict {
		case RowOK, RowDefect:
		default:
			return fmt.Errorf("row %s: invalid non-runnable verdict %q", p.ID, p.Verdict)
		}
	case RowKindCoverage:
		switch p.Verdict {
		case RowOK, RowDefect:
		default:
			return fmt.Errorf("row %s: invalid coverage verdict %q", p.ID, p.Verdict)
		}
	default:
		return fmt.Errorf("row %s: unknown kind %q", p.ID, p.Kind)
	}
	if len(p.Deps) == 0 {
		return fmt.Errorf("row %s pins no dependency", p.ID)
	}
	for _, d := range p.Deps {
		if d.Promise != "" {
			return fmt.Errorf("row %s: promise pins belong to the record's contract pins, not to a row", p.ID)
		}
	}
	return nil
}

// checkRowDependency verifies one row pin against current content. It applies
// the same rules as the record-level dependency walk, minus the promise-pin
// branch rows never carry.
func checkRowDependency(root string, r *Record, d Dependency, layer string) error {
	canonical, err := ResolveInputPath(root, d, r.Unit, layer)
	if err != nil {
		return err
	}
	data, hash, err := specpaths.FileBytes(filepath.Join(root, filepath.FromSlash(canonical)))
	if err != nil {
		return fmt.Errorf("row input %s: %w", d.Path, err)
	}
	if d.Hash != "" && hash != d.Hash {
		return fmt.Errorf("whole-file input changed: %s", canonical)
	}
	// Region pins are content identities over normalized text; compare them
	// against the same canonical spelling the record builder used.
	if len(d.Deps) > 0 && !contenthash.DepsPresent(specpaths.NormalizeText(string(data)), d.Deps) {
		return fmt.Errorf("dependency changed: %s", canonical)
	}
	return nil
}

// DeadUnitScanRows returns the ids of a unit-scan record's rows whose own pins
// no longer hold against the current content, in record order. A row that
// fails to validate is a malformed state: the error fails the call closed and
// the caller must re-run the whole conclusion.
func DeadUnitScanRows(root string, r *Record, layer string) ([]string, error) {
	if r.Kind != Unit {
		return nil, fmt.Errorf("row liveness is defined for unit-scan records only, got kind %q", r.Kind)
	}
	var dead []string
	for _, row := range r.Rows {
		if err := row.Validate(); err != nil {
			return nil, err
		}
		live := true
		for _, d := range row.Deps {
			if err := checkRowDependency(root, r, d, layer); err != nil {
				live = false
				break
			}
		}
		if !live {
			dead = append(dead, row.ID)
		}
	}
	return dead, nil
}

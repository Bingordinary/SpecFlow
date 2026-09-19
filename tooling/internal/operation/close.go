package operation

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Close is the completion transition: it evaluates the frozen scope and marks
// the operation closed. A normal close requires PASS — on FAIL the state is
// left unchanged and the report is returned for the caller to present. With
// abandon set, a violating operation is ended anyway and the state records
// the abandoned outcome (the violation report stays visible; the operation
// was ended explicitly, never silently passed).
func Close(repoRoot, operationID string, abandon bool, now time.Time) (*Report, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}
	if err := requireWorkTreeTop(absRoot); err != nil {
		return nil, err
	}
	var report *Report
	err = withMutation(absRoot, func() error {
		op, err := Load(absRoot, strings.TrimSpace(operationID))
		if err != nil {
			return err
		}
		if op.Status != StatusOpen {
			return fmt.Errorf("operation %s is already closed", op.OperationID)
		}

		report, err = evaluate(absRoot, op)
		if err != nil {
			return err
		}
		if report.Result != "PASS" && !abandon {
			return nil
		}

		ts := now.UTC().Format(timestampLayout)
		op.Status = StatusClosed
		op.ClosedAt = ts
		op.UpdatedAt = ts
		if report.Result == "PASS" {
			op.CloseOutcome = CloseOutcomePassed
		} else {
			op.CloseOutcome = CloseOutcomeAbandoned
		}
		return Save(absRoot, op)
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

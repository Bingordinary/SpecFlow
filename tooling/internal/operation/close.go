package operation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Close is the completion transition: it evaluates the frozen scope and ends
// the operation. A normal close requires PASS — on FAIL the state is left
// unchanged and the report is returned for the caller to present. With
// abandon set, a violating operation is ended anyway (the violation report
// stays visible in the command output; the operation was ended explicitly,
// never silently passed). The close removes the operation state: a closed
// operation is terminal and the tooling never consumes it again, so its
// lifecycle ends in the command that concluded it.
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

		// The report the caller prints carries the terminal outcome; the
		// state itself is removed below — the close is the operation's
		// lifecycle end.
		ts := now.UTC().Format(timestampLayout)
		op.Status = StatusClosed
		op.ClosedAt = ts
		op.UpdatedAt = ts
		if report.Result == "PASS" {
			op.CloseOutcome = CloseOutcomePassed
		} else {
			op.CloseOutcome = CloseOutcomeAbandoned
		}
		return removeState(absRoot, op)
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// removeState deletes one operation's state file. It resolves the path
// through the locked state boundary first, so a state path that escapes
// meta/operations/ fails closed before any removal.
func removeState(repoRoot string, op *Operation) error {
	path, err := statePath(repoRoot, op.OperationID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove operation %s: %w", op.OperationID, err)
	}
	return nil
}

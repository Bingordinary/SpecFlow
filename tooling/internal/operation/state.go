// Package operation implements the operation-scope state carrier: a declared,
// frozen change scope for one bounded work item, mechanically evaluated
// against the working-tree change set at completion. The contract lives in
// tooling/README.md §Operation scope; the agent-facing behavior rules live in
// framework/concepts.md §Operation Scope.
//
// The state is local process state under meta/operations/ — it is not
// project truth and is never committed.
package operation

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

const (
	// Operation statuses.
	StatusOpen   = "open"
	StatusClosed = "closed"

	// Close outcomes: a normal close records "passed" (the check passed); an
	// explicit abandon close records "abandoned" (the operation was ended
	// with violations instead of being silently passed).
	CloseOutcomePassed    = "passed"
	CloseOutcomeAbandoned = "abandoned"

	// Target kinds.
	TargetKindUnit = "unit"
	TargetKindRule = "rule"
	TargetKindNone = "none"

	// Allowed-path source labels. Every entry records where its scope came
	// from; the contract requires the sources to be explicit.
	SourceSpecFile     = "spec:file"
	SourceImplSurface  = "spec:implementation_surface"
	SourceAffectsFiles = "spec:affects.files"
	SourceDeclared     = "declared"

	StateDir        = "meta/operations"
	timestampLayout = "2006-01-02T15:04:05Z"
)

var commitSHAPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Target identifies the declared operation target.
type Target struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

// Baseline is the recorded comparison point of the operation.
type Baseline struct {
	Ref        string `json:"ref"`
	SHA        string `json:"sha"`
	RecordedAt string `json:"recorded_at"`
}

// AllowedPath is one frozen scope entry: a path scope (the path itself or
// everything under it) plus the source it was derived from.
type AllowedPath struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

// UpdateEvent records one explicit scope update while the operation was open.
type UpdateEvent struct {
	At                   string   `json:"at"`
	DeclaredAllowedPaths []string `json:"declared_allowed_paths"`
	RequiredSpecPaths    []string `json:"required_spec_paths"`
}

// Operation is the persisted state of one declared operation scope.
type Operation struct {
	OperationID       string        `json:"operation_id"`
	Status            string        `json:"status"`
	Target            Target        `json:"target"`
	Baseline          Baseline      `json:"baseline"`
	AllowedPaths      []AllowedPath `json:"allowed_paths"`
	RequiredSpecPaths []string      `json:"required_spec_paths"`
	ParentOperation   string        `json:"parent_operation,omitempty"`
	OpenedAt          string        `json:"opened_at"`
	UpdatedAt         string        `json:"updated_at"`
	ClosedAt          string        `json:"closed_at,omitempty"`
	CloseOutcome      string        `json:"close_outcome,omitempty"`
	Updates           []UpdateEvent `json:"updates,omitempty"`
}

// StateRelPath returns the repo-relative path of an operation's state file.
func StateRelPath(operationID string) string {
	return path.Join(StateDir, operationID+".json")
}

func statePath(repoRoot, operationID string) (string, error) {
	if err := localstate.ValidateID(operationID); err != nil {
		return "", err
	}
	return localstate.Path(repoRoot, StateDir, operationID+".json")
}

// Load reads one operation state file. A missing file is an error (the check
// and transition commands have nothing to evaluate), and a malformed state
// fails closed.
func Load(repoRoot, operationID string) (*Operation, error) {
	operationID = strings.TrimSpace(operationID)
	if err := localstate.ValidateID(operationID); err != nil {
		return nil, err
	}
	path, err := statePath(repoRoot, operationID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("operation %q not found", operationID)
		}
		return nil, fmt.Errorf("read operation %q: %w", operationID, err)
	}
	var op Operation
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&op); err != nil {
		return nil, fmt.Errorf("operation %q state is malformed: %w", operationID, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, fmt.Errorf("operation %q state is malformed: %w", operationID, err)
	}
	if err := localstate.BindID(operationID, op.OperationID); err != nil {
		return nil, fmt.Errorf("operation %q state has invalid identity: %w", operationID, err)
	}
	if err := validateOperationState(repoRoot, &op); err != nil {
		return nil, fmt.Errorf("operation %q state is malformed: %w", operationID, err)
	}
	return &op, nil
}

// Save writes one operation state file atomically.
func Save(repoRoot string, op *Operation) error {
	if err := validateOperationState(repoRoot, op); err != nil {
		return fmt.Errorf("write operation: invalid state: %w", err)
	}
	if err := localstate.ValidateID(op.OperationID); err != nil {
		return fmt.Errorf("write operation: %w", err)
	}
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return fmt.Errorf("encode operation %s: %w", op.OperationID, err)
	}
	data = append(data, '\n')
	path, err := statePath(repoRoot, op.OperationID)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, data, 0644); err != nil {
		return fmt.Errorf("write operation %s: %w", op.OperationID, err)
	}
	return nil
}

// validateOperationState validates the complete persisted object before it
// can become executable scope. It deliberately validates both loaded and
// newly generated states so every command observes the same trust boundary.
func validateOperationState(repoRoot string, op *Operation) error {
	if op == nil {
		return fmt.Errorf("state is nil")
	}
	if op.OperationID != strings.TrimSpace(op.OperationID) {
		return fmt.Errorf("operation_id must not contain surrounding whitespace")
	}
	if err := localstate.ValidateID(op.OperationID); err != nil {
		return err
	}
	if op.Status != StatusOpen && op.Status != StatusClosed {
		return fmt.Errorf("invalid status %q", op.Status)
	}
	if err := validateTarget(op.Target); err != nil {
		return err
	}
	if strings.TrimSpace(op.Baseline.Ref) == "" || op.Baseline.Ref != strings.TrimSpace(op.Baseline.Ref) {
		return fmt.Errorf("baseline ref is required and must not contain surrounding whitespace")
	}
	if !commitSHAPattern.MatchString(op.Baseline.SHA) {
		return fmt.Errorf("baseline sha %q is not a full lowercase Git object id", op.Baseline.SHA)
	}
	recordedAt, err := parseStateTimestamp("baseline.recorded_at", op.Baseline.RecordedAt)
	if err != nil {
		return err
	}
	openedAt, err := parseStateTimestamp("opened_at", op.OpenedAt)
	if err != nil {
		return err
	}
	updatedAt, err := parseStateTimestamp("updated_at", op.UpdatedAt)
	if err != nil {
		return err
	}
	if recordedAt.After(openedAt) {
		return fmt.Errorf("baseline.recorded_at must not be after opened_at")
	}
	if updatedAt.Before(openedAt) {
		return fmt.Errorf("updated_at must not be before opened_at")
	}

	switch op.Status {
	case StatusOpen:
		if op.ClosedAt != "" || op.CloseOutcome != "" {
			return fmt.Errorf("open operation must not declare closed_at or close_outcome")
		}
	case StatusClosed:
		closedAt, err := parseStateTimestamp("closed_at", op.ClosedAt)
		if err != nil {
			return err
		}
		if op.CloseOutcome != CloseOutcomePassed && op.CloseOutcome != CloseOutcomeAbandoned {
			return fmt.Errorf("closed operation has invalid close_outcome %q", op.CloseOutcome)
		}
		if closedAt.Before(openedAt) || !closedAt.Equal(updatedAt) {
			return fmt.Errorf("closed_at must equal updated_at and must not be before opened_at")
		}
	}

	if op.ParentOperation != "" {
		if op.ParentOperation != strings.TrimSpace(op.ParentOperation) {
			return fmt.Errorf("parent_operation must not contain surrounding whitespace")
		}
		if err := localstate.ValidateID(op.ParentOperation); err != nil {
			return fmt.Errorf("invalid parent_operation: %w", err)
		}
		if op.ParentOperation == op.OperationID {
			return fmt.Errorf("parent_operation must not equal operation_id")
		}
	}

	if len(op.AllowedPaths) == 0 {
		return fmt.Errorf("allowed_paths must not be empty")
	}
	seenPaths := make(map[string]bool, len(op.AllowedPaths))
	previousPath := ""
	for i, entry := range op.AllowedPaths {
		if err := validateAllowedPath(repoRoot, op.Target, entry); err != nil {
			return fmt.Errorf("allowed_paths[%d]: %w", i, err)
		}
		if seenPaths[entry.Path] {
			return fmt.Errorf("allowed_paths contains duplicate path %q", entry.Path)
		}
		if previousPath != "" && entry.Path < previousPath {
			return fmt.Errorf("allowed_paths must be sorted by path")
		}
		seenPaths[entry.Path] = true
		previousPath = entry.Path
	}

	required, err := canonicalRequiredSpecPaths(repoRoot, op.RequiredSpecPaths)
	if err != nil {
		return err
	}
	if !sameStrings(required, op.RequiredSpecPaths) {
		return fmt.Errorf("required_spec_paths must be canonical, unique, and sorted")
	}

	lastEventAt := openedAt
	var previousDeclared []string
	var previousRequired []string
	for i, event := range op.Updates {
		eventAt, err := parseStateTimestamp(fmt.Sprintf("updates[%d].at", i), event.At)
		if err != nil {
			return err
		}
		if eventAt.Before(lastEventAt) || eventAt.After(updatedAt) {
			return fmt.Errorf("updates[%d].at is outside the operation timeline", i)
		}
		declared, err := canonicalDeclaredPaths(repoRoot, event.DeclaredAllowedPaths, "declared_allowed_paths")
		if err != nil {
			return fmt.Errorf("updates[%d]: %w", i, err)
		}
		if !sameStrings(declared, event.DeclaredAllowedPaths) {
			return fmt.Errorf("updates[%d].declared_allowed_paths must be canonical, unique, and sorted", i)
		}
		required, err := canonicalRequiredSpecPaths(repoRoot, event.RequiredSpecPaths)
		if err != nil {
			return fmt.Errorf("updates[%d]: %w", i, err)
		}
		if !sameStrings(required, event.RequiredSpecPaths) {
			return fmt.Errorf("updates[%d].required_spec_paths must be canonical, unique, and sorted", i)
		}
		if i > 0 {
			if !containsAll(event.DeclaredAllowedPaths, previousDeclared) {
				return fmt.Errorf("updates[%d].declared_allowed_paths removes a path recorded by the previous update", i)
			}
			if !containsAll(event.RequiredSpecPaths, previousRequired) {
				return fmt.Errorf("updates[%d].required_spec_paths removes a path recorded by the previous update", i)
			}
		}
		previousDeclared = event.DeclaredAllowedPaths
		previousRequired = event.RequiredSpecPaths
		lastEventAt = eventAt
	}
	if len(op.Updates) > 0 {
		last := op.Updates[len(op.Updates)-1]
		if !sameStrings(last.DeclaredAllowedPaths, declaredPaths(op.AllowedPaths)) || !sameStrings(last.RequiredSpecPaths, op.RequiredSpecPaths) {
			return fmt.Errorf("latest update event does not match the frozen declared scope")
		}
	}
	return nil
}

func validateTarget(target Target) error {
	switch target.Kind {
	case TargetKindNone:
		if target.Name != "" {
			return fmt.Errorf("target kind %q must not declare a name", target.Kind)
		}
	case TargetKindUnit:
		if err := specpaths.ValidateTargetName(TargetKindUnit, target.Name); err != nil {
			return err
		}
	case TargetKindRule:
		if err := specpaths.ValidateTargetName(TargetKindRule, target.Name); err != nil {
			return err
		}
		if !strings.HasPrefix(target.Name, "g_rule_") && !strings.HasPrefix(target.Name, "b_rule_") {
			return fmt.Errorf("rule target name %q is invalid: expected the g_rule_/b_rule_ prefix", target.Name)
		}
	default:
		return fmt.Errorf("invalid target kind %q", target.Kind)
	}
	return nil
}

func validateAllowedPath(repoRoot string, target Target, entry AllowedPath) error {
	if !isCanonicalScopePath(entry.Path) {
		return fmt.Errorf("path %q is not a canonical repository-relative path", entry.Path)
	}
	canonical, err := repopath.Canonical(repoRoot, entry.Path)
	if err != nil {
		return err
	}
	if canonical != entry.Path {
		return fmt.Errorf("path %q is not canonical", entry.Path)
	}
	switch entry.Source {
	case SourceDeclared:
		paths, err := canonicalDeclaredPaths(repoRoot, []string{entry.Path}, "declared allowed path")
		if err != nil {
			return err
		}
		if len(paths) != 1 || paths[0] != entry.Path {
			return fmt.Errorf("declared path %q is not canonical", entry.Path)
		}
	case SourceSpecFile:
		if !matchesTargetSpecPath(target, entry.Path) {
			return fmt.Errorf("spec:file path %q does not match target %s:%s", entry.Path, target.Kind, target.Name)
		}
	case SourceImplSurface, SourceAffectsFiles:
		if target.Kind != TargetKindUnit {
			return fmt.Errorf("source %q requires a unit target", entry.Source)
		}
	default:
		return fmt.Errorf("invalid source %q", entry.Source)
	}
	return nil
}

func matchesTargetSpecPath(target Target, rel string) bool {
	// The family predicate is the single-level candidate-file definition
	// (open.go isCandidateSpecPath): it rejects nested paths under appendix/,
	// so a persisted state file cannot widen the spec surface with a deeper
	// directory. The name binding below then keeps the entry tied to the
	// target's own spec objects.
	if !isCandidateSpecPath(rel) {
		return false
	}
	dir := filepath.ToSlash(filepath.Dir(rel))
	stem := strings.TrimSuffix(filepath.Base(rel), ".md")
	switch target.Kind {
	case TargetKindUnit:
		switch dir {
		case specpaths.CandidateDir:
			return stem == "unit_"+target.Name
		case specpaths.CandidateAppendixDir:
			prefix := "unit_" + target.Name + "_"
			return strings.HasPrefix(stem, prefix) && len(stem) > len(prefix)
		default:
			return false
		}
	case TargetKindRule:
		return rel == fmt.Sprintf("docs/specs/rules/candidate/%s.md", target.Name)
	default:
		return false
	}
}

func isCanonicalScopePath(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, `\`) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	return clean == rel && clean != "." && !strings.HasPrefix(clean, "../")
}

func parseStateTimestamp(field, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	parsed, err := time.Parse(timestampLayout, value)
	if err != nil || parsed.Format(timestampLayout) != value {
		return time.Time{}, fmt.Errorf("%s %q is not a UTC timestamp in %s format", field, value, timestampLayout)
	}
	return parsed, nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsAll(values, required []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range required {
		if !seen[value] {
			return false
		}
	}
	return true
}

// List reads every operation state file, newest first.
func List(repoRoot string) ([]*Operation, error) {
	dir := filepath.Join(repoRoot, filepath.FromSlash(StateDir))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read operation directory: %w", err)
	}
	var ops []*Operation
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		op, err := Load(repoRoot, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].OperationID > ops[j].OperationID })
	return ops, nil
}

// OpenOperations filters the given operations to the open ones.
func OpenOperations(ops []*Operation) []*Operation {
	var out []*Operation
	for _, op := range ops {
		if op.Status == StatusOpen {
			out = append(out, op)
		}
	}
	return out
}

// newOperationID generates a new operation id in the same form as gate run
// ids (UTC timestamp + random suffix).
func newOperationID(now time.Time) (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate operation id: %w", err)
	}
	return fmt.Sprintf("%s-%x", now.UTC().Format("20060102-150405"), b), nil
}

// writeFileAtomic writes a file through a temp file + rename so a reader
// never observes a half-written state.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

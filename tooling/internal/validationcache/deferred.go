package validationcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

// DeferredLedgerRelPath is the repository-relative path of the deferred-
// findings ledger: the durable handoff of review findings whose recorded
// ownership points at another unit (see framework/verification_scope.md
// §Gate Work Packets → Deferred findings and framework/validation_cache.md
// §Format → Deferred-findings ledger). It lives beside the validation caches
// and shares their lifecycle (durable project state, version controlled).
const DeferredLedgerRelPath = "docs/specs/meta/validation/deferred_findings.json"

// deferredLedgerSchema is the ledger's schema version. A ledger carrying any
// other version fails closed — the tooling never guesses a state layout.
const deferredLedgerSchema = 1

// DeferredEntry is one pending review finding routed to another unit. The
// finding content is stored in full so the owner unit's review can dispose it
// without reading the source run's local state, which is replaced by later
// plans. OwnerUnit is the unit whose review must dispose the entry; SourceUnit
// and SourceRun record where the deferral came from.
type DeferredEntry struct {
	FindingID    string   `json:"finding_id"`
	OwnerUnit    string   `json:"owner_unit"`
	SourceUnit   string   `json:"source_unit"`
	SourceRun    string   `json:"source_run"`
	Severity     string   `json:"severity"`
	Text         string   `json:"text"`
	Detail       string   `json:"detail"`
	SourceKey    string   `json:"source_key,omitempty"`
	AffectedKeys []string `json:"affected_keys,omitempty"`
	EvidencePath string   `json:"evidence_path"`
	Reason       string   `json:"reason"`
}

// DeferredLedger is the complete pending-deferral state of one repository.
type DeferredLedger struct {
	SchemaVersion int             `json:"schema_version"`
	Entries       []DeferredEntry `json:"entries"`
}

// ReadDeferredLedger loads the repository's deferred-findings ledger. A
// missing file is an empty ledger; a malformed or unsupported file is an
// error — a corrupted routing state must fail closed, never silently drop a
// pending finding.
func ReadDeferredLedger(repoRoot string) (DeferredLedger, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(DeferredLedgerRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DeferredLedger{SchemaVersion: deferredLedgerSchema}, nil
		}
		return DeferredLedger{}, fmt.Errorf("read deferred-findings ledger: %w", err)
	}
	var ledger DeferredLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return DeferredLedger{}, fmt.Errorf("deferred-findings ledger %s is malformed: %w", DeferredLedgerRelPath, err)
	}
	if ledger.SchemaVersion != deferredLedgerSchema {
		return DeferredLedger{}, fmt.Errorf("deferred-findings ledger %s has unsupported schema_version %d (want %d)", DeferredLedgerRelPath, ledger.SchemaVersion, deferredLedgerSchema)
	}
	seen := map[string]bool{}
	for _, entry := range ledger.Entries {
		if err := entry.validate(); err != nil {
			return DeferredLedger{}, fmt.Errorf("deferred-findings ledger %s: %w", DeferredLedgerRelPath, err)
		}
		if seen[entry.FindingID] {
			return DeferredLedger{}, fmt.Errorf("deferred-findings ledger %s declares finding %q more than once", DeferredLedgerRelPath, entry.FindingID)
		}
		seen[entry.FindingID] = true
	}
	return ledger, nil
}

// WriteDeferredLedger persists the ledger atomically. An empty ledger removes
// the file: pending is the presence of entries, and no pending deferrals is
// the documented normal state.
func WriteDeferredLedger(repoRoot string, ledger DeferredLedger) error {
	path := filepath.Join(repoRoot, filepath.FromSlash(DeferredLedgerRelPath))
	if len(ledger.Entries) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove empty deferred-findings ledger: %w", err)
		}
		return nil
	}
	ledger.SchemaVersion = deferredLedgerSchema
	entries := append([]DeferredEntry(nil), ledger.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].FindingID < entries[j].FindingID })
	ledger.Entries = entries
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encode deferred-findings ledger: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create deferred-findings ledger directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".specflow-deferred-*")
	if err != nil {
		return fmt.Errorf("create deferred-findings ledger temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write deferred-findings ledger temp file: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set deferred-findings ledger temp file mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close deferred-findings ledger temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish deferred-findings ledger: %w", err)
	}
	removeTemp = false
	return nil
}

// PendingForUnit returns the entries whose owner is unit, in stable
// finding-id order.
func (l DeferredLedger) PendingForUnit(unit string) []DeferredEntry {
	var out []DeferredEntry
	for _, entry := range l.Entries {
		if entry.OwnerUnit == unit {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FindingID < out[j].FindingID })
	return out
}

func (e DeferredEntry) validate() error {
	if strings.TrimSpace(e.FindingID) == "" {
		return fmt.Errorf("entry with empty finding_id")
	}
	if err := specpaths.ValidateTargetName("unit", e.OwnerUnit); err != nil {
		return fmt.Errorf("entry %q: owner_unit: %w", e.FindingID, err)
	}
	if err := specpaths.ValidateTargetName("unit", e.SourceUnit); err != nil {
		return fmt.Errorf("entry %q: source_unit: %w", e.FindingID, err)
	}
	if strings.TrimSpace(e.SourceRun) == "" {
		return fmt.Errorf("entry %q: source_run is empty", e.FindingID)
	}
	switch e.Severity {
	case "P0", "P1", "P2", "P3":
	default:
		return fmt.Errorf("entry %q: invalid severity %q", e.FindingID, e.Severity)
	}
	if strings.TrimSpace(e.Text) == "" || strings.TrimSpace(e.Detail) == "" {
		return fmt.Errorf("entry %q: finding text and detail are required", e.FindingID)
	}
	if strings.TrimSpace(e.SourceKey) == "" && len(e.AffectedKeys) == 0 {
		return fmt.Errorf("entry %q: the finding must name at least one affected key", e.FindingID)
	}
	if strings.TrimSpace(e.EvidencePath) == "" || strings.TrimSpace(e.Reason) == "" {
		return fmt.Errorf("entry %q: ownership evidence and reason are required", e.FindingID)
	}
	return nil
}

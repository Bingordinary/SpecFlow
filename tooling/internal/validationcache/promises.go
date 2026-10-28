package validationcache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"gopkg.in/yaml.v3"
)

// PromiseStatus is the freshness state of one pinned promise.
type PromiseStatus string

const (
	PromiseFresh      PromiseStatus = "FRESH"
	PromiseReopened   PromiseStatus = "REOPENED"
	PromiseUnresolved PromiseStatus = "UNRESOLVED"
)

// PromisePin binds a consumer to the exact contract version it verified.
type PromisePin struct {
	ID          string `yaml:"id"`
	Fingerprint string `yaml:"fingerprint"`
}

// PromiseState is the result of comparing one pin with the current provider
// promise set.
type PromiseState struct {
	ID     string        `yaml:"id"`
	Status PromiseStatus `yaml:"status"`
	Reason string        `yaml:"reason,omitempty"`
}

// PromiseSourcePin binds one promise source declaration to the exact
// Design Narrative block the verification ran against. Source is provenance,
// not contract content, so it is a separate pin from the contract fingerprint.
type PromiseSourcePin struct {
	ID     string `yaml:"id"`
	Anchor string `yaml:"anchor"`
	CID    string `yaml:"cid"`
}

// PromiseStateFile is the per-unit durable record written by a passing verify
// finalize: the unit's own promise fingerprints plus the dependency promises
// it was verified against. It is intentionally separate from the gate cache
// parser; it is a small, durable state record.
type PromiseStateFile struct {
	Unit       string             `yaml:"unit"`
	Layer      string             `yaml:"layer"`
	Promises   []PromisePin       `yaml:"promises,omitempty"`
	Pins       []PromisePin       `yaml:"pins,omitempty"`
	SourcePins []PromiseSourcePin `yaml:"source_pins,omitempty"`
}

// ComparePromisePublication compares recorded consumer pins with the published
// stable provider promise set. It is the promote-side view: a candidate may be
// layer-locally CLOSED while it builds on another candidate, but it is not
// promotable until every pin resolves in the stable layer at the same
// fingerprint.
func ComparePromisePublication(pins []PromisePin, stable map[string]string) []PromiseState {
	states := make([]PromiseState, 0, len(pins))
	for _, pin := range pins {
		fingerprint, ok := stable[pin.ID]
		switch {
		case !ok:
			states = append(states, PromiseState{
				ID:     pin.ID,
				Status: PromiseUnresolved,
				Reason: "provider promise is not published in the stable layer",
			})
		case fingerprint == pin.Fingerprint:
			states = append(states, PromiseState{
				ID:     pin.ID,
				Status: PromiseFresh,
			})
		default:
			states = append(states, PromiseState{
				ID:     pin.ID,
				Status: PromiseReopened,
				Reason: "provider promise contract differs from the published stable contract",
			})
		}
	}
	return states
}

// ComparePromisePins compares consumer pins with the current provider promise
// fingerprints. A missing pin target is UNRESOLVED; a changed fingerprint is
// REOPENED; an equal fingerprint is FRESH.
func ComparePromisePins(pins []PromisePin, current map[string]string) []PromiseState {
	states := make([]PromiseState, 0, len(pins))
	for _, pin := range pins {
		fingerprint, ok := current[pin.ID]
		switch {
		case !ok:
			states = append(states, PromiseState{
				ID:     pin.ID,
				Status: PromiseUnresolved,
				Reason: "provider promise is missing from the current layer",
			})
		case fingerprint == pin.Fingerprint:
			states = append(states, PromiseState{
				ID:     pin.ID,
				Status: PromiseFresh,
			})
		default:
			states = append(states, PromiseState{
				ID:     pin.ID,
				Status: PromiseReopened,
				Reason: "provider promise fingerprint changed",
			})
		}
	}
	return states
}

// ComparePromiseSources compares recorded source-block pins with the current
// source-block content identities. Unlike the contract fingerprint, a source
// change does not reopen consumers; it makes the provider's own promise state
// stale because the verification ran against a different narrative block.
func ComparePromiseSources(recorded []PromiseSourcePin, current map[string]string) []PromiseState {
	seen := map[string]bool{}
	var states []PromiseState
	for _, pin := range recorded {
		key := specvalidation.PromiseSourceKey(pin.ID, pin.Anchor)
		seen[key] = true
		label := pin.ID + " source " + pin.Anchor
		cid, ok := current[key]
		switch {
		case !ok:
			states = append(states, PromiseState{
				ID:     label,
				Status: PromiseUnresolved,
				Reason: "source block is missing from the current spec",
			})
		case cid != pin.CID:
			states = append(states, PromiseState{
				ID:     label,
				Status: PromiseReopened,
				Reason: "source block changed",
			})
		}
	}
	for key := range current {
		if seen[key] {
			continue
		}
		id, anchor, ok := strings.Cut(key, "\x00")
		if !ok {
			continue
		}
		states = append(states, PromiseState{
			ID:     id + " source " + anchor,
			Status: PromiseReopened,
			Reason: "source block is not recorded",
		})
	}
	sort.Slice(states, func(i, j int) bool { return states[i].ID < states[j].ID })
	return states
}

// CheckPromiseStateIdentity validates the durable record's own identity and
// list shape. A state file is a project record, so a copied or edited file can
// name another unit or layer, or repeat a promise/pin entry. Both `fresh` and
// `promote` use this check: a readable record is not authoritative until it
// belongs to the requested unit and layer and its entries are unique.
func CheckPromiseStateIdentity(state *PromiseStateFile, unit, layer string) []string {
	if state == nil {
		return nil
	}
	var issues []string
	if state.Unit != unit {
		issues = append(issues, fmt.Sprintf("promise state belongs to %q, not %q", state.Unit, unit))
	}
	if state.Layer != layer {
		issues = append(issues, fmt.Sprintf("promise state layer is %q, expected %q", state.Layer, layer))
	}
	ownSeen := map[string]bool{}
	for _, pin := range state.Promises {
		if ownSeen[pin.ID] {
			issues = append(issues, fmt.Sprintf("promise state records duplicate own promise %q", pin.ID))
			continue
		}
		ownSeen[pin.ID] = true
	}
	seen := map[string]bool{}
	for _, pin := range state.Pins {
		if seen[pin.ID] {
			issues = append(issues, fmt.Sprintf("promise state records duplicate dependency pin %q", pin.ID))
			continue
		}
		seen[pin.ID] = true
	}
	seenSources := map[string]bool{}
	for _, pin := range state.SourcePins {
		if pin.ID == "" || pin.Anchor == "" || pin.CID == "" {
			issues = append(issues, "promise state records an incomplete source pin")
			continue
		}
		if !ownSeen[pin.ID] {
			issues = append(issues, fmt.Sprintf("promise state source pin names unknown own promise %q", pin.ID))
		}
		key := specvalidation.PromiseSourceKey(pin.ID, pin.Anchor)
		if seenSources[key] {
			issues = append(issues, fmt.Sprintf("promise state records duplicate source pin %q for %q", pin.Anchor, pin.ID))
			continue
		}
		seenSources[key] = true
	}
	return issues
}

// PromiseStatePath returns the durable promise-state file for one unit layer.
// The two layers have separate files because a unit can have both a candidate
// round and a stable confirmation record at the same time.
func PromiseStatePath(repoRoot, unit, layer string) (string, error) {
	if layer != "candidate" && layer != "stable" {
		return "", fmt.Errorf("promise state: invalid layer %q; expected candidate or stable", layer)
	}
	return cacheFilePath(repoRoot, "unit", unit, "promise_state_"+layer+".yaml")
}

// ReadPromiseStateFile reads a promise state record. A missing file returns
// (nil, nil) so callers can distinguish "not recorded" from a read error.
func ReadPromiseStateFile(path string) (*PromiseStateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state PromiseStateFile
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse promise state %s: %w", path, err)
	}
	if state.Unit == "" {
		return nil, fmt.Errorf("promise state %s: missing unit", path)
	}
	return &state, nil
}

// PreparePromiseStateStable renders the candidate promise state as a stable
// layer record without writing it. The promote transaction stages the bytes
// with the spec and baseline, so a state-write failure aborts before any
// accepted truth changes.
func PreparePromiseStateStable(repoRoot, unit string) (stablePath string, data []byte, candidatePath string, ok bool, err error) {
	candidatePath, err = PromiseStatePath(repoRoot, unit, "candidate")
	if err != nil {
		return "", nil, "", false, err
	}
	state, err := ReadPromiseStateFile(candidatePath)
	if err != nil {
		return "", nil, "", false, err
	}
	if state == nil {
		return "", nil, candidatePath, false, nil
	}
	state.Layer = "stable"
	stablePath, err = PromiseStatePath(repoRoot, unit, "stable")
	if err != nil {
		return "", nil, "", false, err
	}
	data, err = yaml.Marshal(*state)
	if err != nil {
		return "", nil, "", false, err
	}
	return stablePath, data, candidatePath, true, nil
}

// CopyPromiseStateToCandidate copies the stable promise state to the candidate
// layer on fork. It returns false when no stable state exists.
func CopyPromiseStateToCandidate(repoRoot, unit string) (bool, error) {
	stablePath, err := PromiseStatePath(repoRoot, unit, "stable")
	if err != nil {
		return false, err
	}
	state, err := ReadPromiseStateFile(stablePath)
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil
	}
	state.Layer = "candidate"
	candidatePath, err := PromiseStatePath(repoRoot, unit, "candidate")
	if err != nil {
		return false, err
	}
	if err := WritePromiseStateFile(candidatePath, *state); err != nil {
		return false, err
	}
	return true, nil
}

// WritePromiseStateFile writes a promise state record atomically: the file is
// rendered completely before it replaces the target, so a crash or write error
// never leaves a truncated durable record.
func WritePromiseStateFile(path string, state PromiseStateFile) error {
	if state.Unit == "" {
		return fmt.Errorf("promise state: missing unit")
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".promise-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

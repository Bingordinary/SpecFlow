package validationcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

type recordState struct {
	SchemaVersion     int                          `json:"schema_version"`
	Records           map[string]judgments.Binding `json:"records"`
	LogicalStatus     map[string]string            `json:"logical_status"`
	SynthesisEvidence []synthesisEvidencePin       `json:"synthesis_evidence,omitempty"`
}

// synthesisEvidencePin mirrors gaterun.EvidencePin. It is redeclared here so
// the validationcache package keeps its dependency direction: it reads cache
// state, it does not import the run planner.
type synthesisEvidencePin struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// ConclusionState is the reuse state of one local conclusion the cache binds.
// Live means the conclusion's own judgment record still holds: its pinned
// whole-file hashes and structural regions are unchanged, its review protocol
// is current, it is not explicitly invalidated, and every record it references
// is live. Reason names the first dependency that failed for a dead
// conclusion.
type ConclusionState struct {
	Key    string
	Live   bool
	Reason string
}

// VerifyConclusionStates evaluates every local conclusion a verify cache
// binds and reports each one's reuse state. Liveness is decided by the
// conclusion's own judgment record, never by the run's recorded input
// surface: a file the run merely read does not stale a conclusion.
//
// Structural damage — an unreadable baseline, a missing review protocol, a
// check marker without a record, a record whose finalized status disagrees
// with the cache — is an error: the state cannot be evaluated and the caller
// must fail closed. A dead conclusion is not an error; it is a judgment that
// has to run again.
func VerifyConclusionStates(root, targetKind string, cache *cacheFile) ([]ConclusionState, error) {
	var state recordState
	if err := json.Unmarshal([]byte(cache.Judgments), &state); err != nil || state.SchemaVersion != 4 {
		return nil, fmt.Errorf("verify cache uses an old or damaged review protocol; run a full verify")
	}
	if len(state.Records) == 0 {
		return nil, fmt.Errorf("verify cache has no judgment references")
	}
	protocol, err := judgments.Protocol(root, "verify", targetKind)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(state.Records))
	for key := range state.Records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	states := make([]ConclusionState, 0, len(keys))
	for _, key := range keys {
		binding := state.Records[key]
		st := ConclusionState{Key: key, Live: true}
		if err := judgments.Check(root, binding.Reference, binding.Layer, protocol); err != nil {
			st.Live = false
			st.Reason = fmt.Sprintf("%s: %v", key, err)
			states = append(states, st)
			continue
		}
		r, err := judgments.Load(root, binding.Reference)
		if err != nil {
			return nil, err
		}
		if r.Kind != judgments.Code {
			var result struct {
				EffectiveStatus map[string]string `json:"effective_status"`
			}
			if err := json.Unmarshal(r.Result, &result); err != nil || len(result.EffectiveStatus) != 1 {
				return nil, fmt.Errorf("%s has no finalized logical status", key)
			}
			for _, status := range result.EffectiveStatus {
				if status != state.LogicalStatus[key] {
					return nil, fmt.Errorf("%s record disagrees with the finalized cache status", key)
				}
			}
		}
		if strings.HasPrefix(key, "design:"+cache.Unit+":") {
			public, ok := state.Records["code:"+strings.TrimPrefix(key, "design:"+cache.Unit+":")]
			if !ok {
				return nil, fmt.Errorf("%s has no required public judgment", key)
			}
			matched := false
			for _, dep := range r.References {
				if dep.Reference == public.Reference {
					matched = true
				}
			}
			if !matched {
				return nil, fmt.Errorf("%s consumes a different public judgment", key)
			}
		}
		states = append(states, st)
	}
	// The final synthesis's own evidence: supplied evidence inputs that no
	// individual conclusion owns. A change to any of them makes the synthesis
	// — and with it every relationship conclusion — stale, because the
	// synthesis read the shared material the relationship verdict rests on.
	for _, pin := range state.SynthesisEvidence {
		canonical, err := judgments.ResolveEvidencePath(root, pin.Path)
		if err != nil {
			states = append(states, ConclusionState{Key: "cross", Live: false,
				Reason: fmt.Sprintf("cross: synthesis evidence missing: %s", pin.Path)})
			break
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(canonical)))
		if err != nil {
			states = append(states, ConclusionState{Key: "cross", Live: false,
				Reason: fmt.Sprintf("cross: synthesis evidence missing: %s", pin.Path)})
			break
		}
		if pin.Hash == "" || specpaths.HashText(data) != pin.Hash {
			states = append(states, ConclusionState{Key: "cross", Live: false,
				Reason: fmt.Sprintf("cross: synthesis evidence changed: %s", pin.Path)})
			break
		}
	}
	for _, entry := range cache.Files {
		for _, check := range entry.Checks {
			if strings.HasPrefix(check.Check, "relationship:") {
				continue
			}
			if _, ok := state.Records[check.Check]; !ok {
				return nil, fmt.Errorf("missing judgment record for %s", check.Check)
			}
		}
	}
	return states, nil
}

// FirstDeadReason returns the reason of the alphabetically first dead
// conclusion, so callers that report a single failure do so deterministically.
func FirstDeadReason(dead map[string]string) string {
	keys := make([]string, 0, len(dead))
	for key := range dead {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return dead[keys[0]]
}

// UncoveredChangedFilesForUnit reports the recorded verify evidence of a unit
// that changed since the run but that no conclusion depends on — neither a
// judgment record's pinned evidence nor the final synthesis's cited evidence.
// Such a file does not stale the cache — verify freshness is conclusion-bound —
// but it is the only place a semantic dependency no conclusion declared could
// escape unseen. `fresh` and `gate-plan` report the list and suggest a full run
// instead of silently trusting the reuse.
//
// An absent or unusable cache yields nil: it is already reported as
// missing/stale, and there is no recorded surface to compare.
func UncoveredChangedFilesForUnit(repoRoot, unitName string) ([]string, error) {
	p, err := cacheFilePath(repoRoot, "unit", unitName, "verify_result.md")
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	cache, err := readCache(p)
	if err != nil {
		return nil, nil
	}
	return uncoveredChangedFiles(repoRoot, cache)
}

func uncoveredChangedFiles(repoRoot string, cache *cacheFile) ([]string, error) {
	var state recordState
	if err := json.Unmarshal([]byte(cache.Judgments), &state); err != nil || state.SchemaVersion != 4 {
		return nil, nil
	}
	pinned := map[string]bool{}
	for _, binding := range state.Records {
		r, err := judgments.Load(repoRoot, binding.Reference)
		if err != nil {
			continue // a dead record is already reported as a stale conclusion
		}
		for _, d := range r.Dependencies {
			if d.Promise != "" {
				// A promise pin governs the provider contract, not the whole
				// provider file. The provider spec path is still marked pinned
				// so a narrative edit is not reported as uncovered evidence.
				if d.Path != "" {
					if rel, err := repopath.Canonical(repoRoot, d.Path); err == nil {
						pinned[rel] = true
					}
				}
				continue
			}
			rel, err := judgments.ResolveInputPath(repoRoot, d, r.Unit, binding.Layer)
			if err != nil {
				continue
			}
			pinned[rel] = true
		}
	}
	// The final synthesis is a conclusion too: the evidence it cited is part of
	// its own object, so a changed synthesis pin is reported as the stale
	// conclusion it is, never as evidence no conclusion depends on.
	for _, pin := range state.SynthesisEvidence {
		rel, err := judgments.ResolveEvidencePath(repoRoot, pin.Path)
		if err != nil {
			continue
		}
		pinned[rel] = true
	}
	var out []string
	for _, entry := range cache.Files {
		st, err := fileFreshness(repoRoot, entry)
		if err != nil && !os.IsNotExist(err) {
			continue
		}
		if st == fileFresh {
			continue
		}
		rel := ""
		if abs := resolveEntryPath(repoRoot, entry.Path); abs != "" {
			if r, err := filepath.Rel(repoRoot, abs); err == nil {
				if canonical, err := repopath.Canonical(repoRoot, filepath.ToSlash(r)); err == nil {
					rel = canonical
				}
			}
		}
		if rel != "" && pinned[rel] {
			continue
		}
		out = append(out, entry.Path)
	}
	sort.Strings(out)
	return out, nil
}

// verifyStandard checks that a validate cache was judged under the validate
// standard the checklist currently deploys. Verify conclusions bind their
// standard per judgment record; a validate conclusion has no record to bind,
// so the cache itself records the standard it was judged under. A cache
// without that record, or written under a different checklist, cannot be
// reused: the standard it was judged by is not the current one.
func verifyStandard(root, targetKind string, cache *cacheFile) error {
	if strings.TrimSpace(cache.Standard) == "" {
		return fmt.Errorf("the cache records no judging standard (pre-binding format) — old caches are invalid")
	}
	current, err := judgments.Protocol(root, "validate", targetKind)
	if err != nil {
		return err
	}
	if cache.Standard != current {
		return fmt.Errorf("the validate standard changed since this cache was judged")
	}
	return nil
}

// Rewrite only the current unit's binding. Content-addressed records keep
// their original bytes and ids.
func rewriteJudgmentBindings(content, unit, from, to string) string {
	raw := extractJudgments(content)
	if raw == "" {
		return content
	}
	var state map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &state) != nil {
		return content
	}
	var refs map[string]judgments.Binding
	if json.Unmarshal(state["records"], &refs) != nil {
		return content
	}
	changed := false
	for key, ref := range refs {
		if strings.HasPrefix(key, "item:"+unit+":") || strings.HasPrefix(key, "design:"+unit+":") || key == "architecture:"+unit || key == "unit:"+unit {
			if ref.Layer == from {
				ref.Layer = to
				refs[key] = ref
				changed = true
			}
		}
	}
	if !changed {
		return content
	}
	state["records"], _ = json.Marshal(refs)
	data, err := json.Marshal(state)
	if err != nil {
		return content
	}
	return strings.Replace(content, raw, string(data), 1)
}

func checkExpectedRecord(root string, cache *cacheFile, want ExpectedCheck) error {
	var state recordState
	if err := json.Unmarshal([]byte(cache.Judgments), &state); err != nil {
		return err
	}
	binding, ok := state.Records[want.Key]
	if !ok {
		return fmt.Errorf("missing judgment reference for %s", want.Key)
	}
	r, err := judgments.Load(root, binding.Reference)
	if err != nil {
		return err
	}
	if want.Kind != "" && (r.Kind != want.Kind || r.Unit != want.Unit || r.Subject != want.Subject) {
		return fmt.Errorf("judgment subject does not match %s", want.Key)
	}
	// One required-evidence rule for every reader: the gate's freshness check
	// and the delta planner both ask judgments.NewlyRequiredEvidence, so a
	// conclusion the planner carries can never be one this check rejects.
	if missing, ok, err := judgments.NewlyRequiredEvidence(root, want.Unit, want.Layer, *r, want.Inputs); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("%s has newly required evidence %s", want.Key, missing)
	}
	return nil
}

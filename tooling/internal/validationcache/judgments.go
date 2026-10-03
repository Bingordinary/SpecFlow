package validationcache

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

type recordState struct {
	SchemaVersion int                          `json:"schema_version"`
	Records       map[string]judgments.Binding `json:"records"`
	LogicalStatus map[string]string            `json:"logical_status"`
}

func verifyRecords(root string, cache *cacheFile) error {
	var state recordState
	if err := json.Unmarshal([]byte(cache.Judgments), &state); err != nil || state.SchemaVersion != 4 {
		return fmt.Errorf("verify cache uses an old or damaged review protocol; run a full verify")
	}
	if len(state.Records) == 0 {
		return fmt.Errorf("verify cache has no judgment references")
	}
	for key, binding := range state.Records {
		if err := judgments.Check(root, binding.Reference, binding.Layer, judgments.Protocol(root)); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		r, err := judgments.Load(root, binding.Reference)
		if err != nil {
			return err
		}
		if r.Kind != judgments.Code {
			var result struct {
				EffectiveStatus map[string]string `json:"effective_status"`
			}
			if err := json.Unmarshal(r.Result, &result); err != nil || len(result.EffectiveStatus) != 1 {
				return fmt.Errorf("%s has no finalized logical status", key)
			}
			for _, status := range result.EffectiveStatus {
				if status != state.LogicalStatus[key] {
					return fmt.Errorf("%s record disagrees with the finalized cache status", key)
				}
			}
		}
		if strings.HasPrefix(key, "design:"+cache.Unit+":") {
			public, ok := state.Records["code:"+strings.TrimPrefix(key, "design:"+cache.Unit+":")]
			if !ok {
				return fmt.Errorf("%s has no required public judgment", key)
			}
			matched := false
			for _, dep := range r.References {
				if dep.Reference == public.Reference {
					matched = true
				}
			}
			if !matched {
				return fmt.Errorf("%s consumes a different public judgment", key)
			}
		}
	}
	for _, entry := range cache.Files {
		for _, check := range entry.Checks {
			if strings.HasPrefix(check.Check, "relationship:") {
				continue
			}
			binding, ok := state.Records[check.Check]
			if !ok {
				return fmt.Errorf("missing judgment record for %s", check.Check)
			}
			if err := judgments.Check(root, binding.Reference, binding.Layer, judgments.Protocol(root)); err != nil {
				return fmt.Errorf("%s: %w", check.Check, err)
			}
			r, err := judgments.Load(root, binding.Reference)
			if err != nil {
				return err
			}
			if cache.Result == "pass" && strings.HasPrefix(check.Check, "preserve:") && r.Verdict != "ALIGNED" {
				return fmt.Errorf("protected requirement %s is %s", check.Check, r.Verdict)
			}
		}
	}
	return nil
}

// Rewrite only the current unit's binding. Protected stable units remain bound
// to stable, and content-addressed records keep their original bytes and ids.
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
		if strings.HasPrefix(key, "item:"+unit+":") || strings.HasPrefix(key, "design:"+unit+":") || key == "architecture:"+unit {
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
	var owned []string
	if want.Unit != "" {
		appendices, err := specpaths.UnitAppendices(root, want.Unit, want.Layer)
		if err != nil {
			return err
		}
		for _, appendix := range appendices {
			owned = append(owned, appendix.Path)
		}
	}
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
	for _, input := range want.Inputs {
		p := input
		if suffix, ok := judgments.OwnSuffix(p, want.Unit, want.Layer, owned); ok {
			p = "own:" + suffix
		}
		found := false
		for _, old := range r.Inputs {
			if p == old {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s has newly required evidence %s", want.Key, input)
		}
	}
	return nil
}

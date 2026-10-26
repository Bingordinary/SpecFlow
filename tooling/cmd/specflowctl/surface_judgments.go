package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

type surfaceJudgment struct {
	Path         string                        `json:"path"`
	Associations []specvalidation.SurfaceRef   `json:"associations"`
	Public       []judgments.Reference         `json:"public"`
	Designs      map[string]*judgments.Binding `json:"designs"`
}

func surfaceJudgments(root string, report *specvalidation.SurfaceAuditReport) ([]surfaceJudgment, error) {
	// The surface view reports verify judgment records, which are unit-gated.
	protocol, err := judgments.Protocol(root, gaterun.GateVerify, gaterun.TargetKindUnit)
	if err != nil {
		return nil, err
	}
	files := map[string]*surfaceJudgment{}
	for _, u := range report.Units {
		baseline, err := validationcache.ReadGateBaseline(root, "unit", u.Unit, "verify")
		if err != nil {
			return nil, err
		}
		var state gaterun.JudgmentBaseline
		if baseline.Exists {
			json.Unmarshal([]byte(baseline.Judgments), &state)
		}
		for _, f := range u.Files {
			view := files[f.Path]
			if view == nil {
				view = &surfaceJudgment{Path: f.Path, Designs: map[string]*judgments.Binding{}}
				files[f.Path] = view
			}
			view.Associations = append(view.Associations, f)
			name := u.Unit + "@" + u.Layer
			view.Designs[name] = nil
			if ref, ok := state.Records["design:"+u.Unit+":"+f.Path]; ok && ref.Layer == u.Layer && judgments.Check(root, ref.Reference, ref.Layer, protocol) == nil {
				copyRef := ref
				view.Designs[name] = &copyRef
			}
		}
	}
	// Each report file's slot is computed from the key it names, so the store
	// is asked one question per file instead of being read whole: the slot holds
	// the handful of decisions ever accepted for that key.
	for path, view := range files {
		codeSlot, err := judgments.Slot(protocol, judgments.Code, "", path, "")
		if err != nil {
			return nil, err
		}
		codeRefs, err := judgments.RefsInSlot(root, codeSlot)
		if err != nil {
			return nil, err
		}
		for _, ref := range codeRefs {
			r, err := judgments.Load(root, ref)
			if err != nil || r.Kind != judgments.Code {
				continue
			}
			if judgments.Verify(root, r, ref, "stable", protocol) == nil {
				view.Public = append(view.Public, ref)
			}
		}
		for _, association := range view.Associations {
			name := association.Unit + "@" + association.Layer
			if view.Designs[name] != nil {
				continue
			}
			designSlot, err := judgments.Slot(protocol, judgments.Design, association.Unit, path, "")
			if err != nil {
				return nil, err
			}
			designRefs, err := judgments.RefsInSlot(root, designSlot)
			if err != nil {
				return nil, err
			}
			for _, ref := range designRefs {
				r, err := judgments.Load(root, ref)
				if err != nil || r.Kind != judgments.Design || r.Unit != association.Unit {
					continue
				}
				if judgments.Verify(root, r, ref, association.Layer, protocol) != nil {
					continue
				}
				binding := judgments.Binding{Reference: ref, Layer: association.Layer, Source: "reused"}
				view.Designs[name] = &binding
				break
			}
		}
	}
	var out []surfaceJudgment
	for _, view := range files {
		out = append(out, *view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
func printSurfaceJudgments(w io.Writer, views []surfaceJudgment) {
	fmt.Fprintln(w, "\nFile judgments:")
	for _, view := range views {
		fmt.Fprintln(w, view.Path)
		if len(view.Public) == 0 {
			fmt.Fprintln(w, "  public code: not checked")
		}
		for _, ref := range view.Public {
			fmt.Fprintf(w, "  public code: %s\n", ref.ID)
		}
		var names []string
		for name := range view.Designs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ref := view.Designs[name]
			if ref == nil {
				fmt.Fprintf(w, "  %s design: not checked or stale\n", name)
			} else {
				fmt.Fprintf(w, "  %s design: %s\n", name, ref.ID)
			}
		}
	}
}

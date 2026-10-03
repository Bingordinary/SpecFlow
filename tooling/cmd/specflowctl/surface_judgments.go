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
			if ref, ok := state.Records["design:"+u.Unit+":"+f.Path]; ok && ref.Layer == u.Layer && judgments.Check(root, ref.Reference, ref.Layer, judgments.Protocol(root)) == nil {
				copyRef := ref
				view.Designs[name] = &copyRef
			}
		}
	}
	refs, err := judgments.List(root)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		r, err := judgments.Load(root, ref)
		if err != nil {
			continue
		}
		if r.Kind == judgments.Code && judgments.Check(root, ref, "stable", judgments.Protocol(root)) == nil {
			if view := files[r.Subject]; view != nil {
				view.Public = append(view.Public, ref)
			}
		}
		if r.Kind == judgments.Design {
			if view := files[r.Subject]; view != nil {
				for _, association := range view.Associations {
					name := association.Unit + "@" + association.Layer
					if r.Unit == association.Unit && view.Designs[name] == nil && judgments.Check(root, ref, association.Layer, judgments.Protocol(root)) == nil {
						binding := judgments.Binding{Reference: ref, Layer: association.Layer, Source: "reused"}
						view.Designs[name] = &binding
					}
				}
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

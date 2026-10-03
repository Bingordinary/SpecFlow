// Package removal deletes explicitly selected spec artifacts. It verifies
// structural integrity; deciding whether a responsibility should end is the
// caller's job. No consumer count or retention field selects deletion targets.
package removal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

type Request struct {
	Units, Rules, Appendices []string
	Layer                    string
	DryRun                   bool
}

type Result struct {
	Files    []string
	Updates  []string
	Blockers []string
	DryRun   bool
}

type document struct {
	path, kind, name, layer, content string
	fm                               map[string]string
}

type plan struct {
	result  Result
	deleted map[string]bool
	writes  map[string][]byte
}

// Run previews or executes one batch. Execution shares the gate mutation lock
// so a stale finalize cannot recreate a cache after the deletion commits.
func Run(root string, request Request) (*Result, error) {
	if request.Layer == "" {
		request.Layer = "all"
	}
	if err := validate(request); err != nil {
		return nil, err
	}
	var result *Result
	work := func() error {
		p, err := prepare(root, request)
		if err != nil {
			return err
		}
		result = &p.result
		if len(result.Blockers) != 0 {
			return fmt.Errorf("remaining structured references block removal")
		}
		if request.DryRun {
			return nil
		}
		return transact(root, p, os.Rename)
	}
	if request.DryRun {
		err := work()
		return result, err
	}
	err := gaterun.WithMutation(root, work)
	return result, err
}

func validate(r Request) error {
	if r.Layer != "all" && r.Layer != "candidate" {
		return fmt.Errorf("--layer must be all or candidate")
	}
	if len(r.Units)+len(r.Rules)+len(r.Appendices) == 0 {
		return fmt.Errorf("at least one --unit, --rule or --appendix is required")
	}
	for _, name := range r.Units {
		if err := specpaths.ValidateTargetName("unit", name); err != nil {
			return err
		}
	}
	for _, name := range r.Rules {
		if err := specpaths.ValidateTargetName("rule", name); err != nil {
			return err
		}
	}
	for _, name := range r.Appendices {
		if _, _, err := appendixTarget(name); err != nil {
			return err
		}
	}
	return nil
}

func appendixTarget(value string) (string, string, error) {
	unit, file, ok := strings.Cut(value, ":")
	if !ok || specpaths.ValidateTargetName("unit", unit) != nil || !strings.HasPrefix(file, "unit_"+unit+"_") || !strings.HasSuffix(file, ".md") || specpaths.ValidateTargetName("unit", strings.TrimSuffix(file, ".md")) != nil {
		return "", "", fmt.Errorf("invalid appendix %q: expected <unit>:unit_<unit>_<name>.md", value)
	}
	return unit, file, nil
}

func inventory(root string) ([]document, error) {
	var docs []document
	for _, layer := range []string{"stable", "candidate"} {
		for _, kind := range []string{"unit", "appendix", "rule"} {
			dir := "docs/specs/units/" + layer
			if kind == "appendix" {
				dir += "/appendix"
			}
			if kind == "rule" {
				dir = "docs/specs/rules/" + layer
			}
			if _, err := repopath.Canonical(root, dir); err != nil {
				return nil, err
			}
			entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", dir, err)
			}
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ".md") || kind != "rule" && !strings.HasPrefix(entry.Name(), "unit_") {
					continue
				}
				path := dir + "/" + entry.Name()
				data, err := readFile(root, path)
				if err != nil {
					return nil, err
				}
				fm, _, err := specpaths.ParseFrontmatterFields(string(data))
				if err != nil {
					return nil, fmt.Errorf("parse %s: %w", path, err)
				}
				name := strings.TrimSuffix(entry.Name(), ".md")
				if kind == "unit" {
					name = strings.TrimPrefix(name, "unit_")
				}
				if kind == "appendix" {
					name = fm["unit"]
					if specpaths.ValidateTargetName("unit", name) != nil || !strings.HasPrefix(entry.Name(), "unit_"+name+"_") {
						return nil, fmt.Errorf("%s: appendix ownership is missing or inconsistent", path)
					}
				}
				docs = append(docs, document{path, kind, name, layer, string(data), fm})
			}
		}
	}
	return docs, nil
}

func readFile(root, path string) ([]byte, error) {
	if _, err := repopath.Canonical(root, path); err != nil {
		return nil, err
	}
	abs := filepath.Join(root, filepath.FromSlash(path))
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

func prepare(root string, r Request) (*plan, error) {
	docs, err := inventory(root)
	if err != nil {
		return nil, err
	}
	p := &plan{result: Result{DryRun: r.DryRun}, deleted: map[string]bool{}, writes: map[string][]byte{}}
	owners := map[string]bool{}
	found := map[string]bool{}
	selectLayer := func(layer string) bool { return r.Layer == "all" || layer == "candidate" }
	for _, name := range r.Units {
		key := "unit:" + name
		for _, d := range docs {
			if selectLayer(d.layer) && d.name == name && (d.kind == "unit" || d.kind == "appendix") {
				p.deleted[d.path] = true
				found[key] = true
			}
		}
		for _, layer := range []string{"stable", "candidate"} {
			if selectLayer(layer) {
				owners[key+":"+layer] = true
			}
		}
	}
	for _, name := range r.Rules {
		key := "rule:" + name
		for _, d := range docs {
			if selectLayer(d.layer) && d.kind == "rule" && d.name == name {
				p.deleted[d.path] = true
				found[key] = true
			}
		}
		for _, layer := range []string{"stable", "candidate"} {
			if selectLayer(layer) {
				owners[key+":"+layer] = true
			}
		}
	}
	for _, value := range r.Appendices {
		unit, file, _ := appendixTarget(value)
		for _, d := range docs {
			if selectLayer(d.layer) && d.kind == "appendix" && filepath.Base(d.path) == file && d.name == unit {
				p.deleted[d.path] = true
				found["appendix:"+value] = true
				owners["unit:"+unit+":"+d.layer] = true
			}
		}
	}
	for key := range owners {
		parts := strings.Split(key, ":")
		dir := "docs/specs/meta/validation/" + parts[0] + "/" + parts[1]
		if _, err := repopath.Canonical(root, dir); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, entry := range entries {
			path := dir + "/" + entry.Name()
			data, err := readFile(root, path)
			if err != nil {
				return nil, err
			}
			if !owners[parts[0]+":"+parts[1]+":stable"] || !owners[parts[0]+":"+parts[1]+":candidate"] {
				fm, _, err := specpaths.ParseFrontmatterFields(string(data))
				if err != nil || fm["target"] != "candidate" && fm["target"] != "stable" {
					return nil, fmt.Errorf("%s: cannot determine cache layer", path)
				}
				if fm["target"] != parts[2] {
					continue
				}
			}
			p.deleted[path] = true
			found[parts[0]+":"+parts[1]] = true
		}
	}
	if r.Layer == "all" {
		for _, kind := range []string{"unit", "rule"} {
			names := r.Units
			if kind == "rule" {
				names = r.Rules
			}
			for _, name := range names {
				path := "docs/specs/meta/baseline/" + kind + "/" + name + ".yaml"
				if _, err := repopath.Canonical(root, path); err != nil {
					return nil, err
				}
				if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); os.IsNotExist(err) {
					continue
				} else if err != nil {
					return nil, err
				}
				if _, err := readFile(root, path); err != nil {
					return nil, err
				}
				p.deleted[path] = true
				found[kind+":"+name] = true
			}
		}
	}
	for _, name := range r.Units {
		if !found["unit:"+name] {
			return nil, fmt.Errorf("unit %s: no matching files or records in selected layer", name)
		}
	}
	for _, name := range r.Rules {
		if !found["rule:"+name] {
			return nil, fmt.Errorf("rule %s: no matching files or records in selected layer", name)
		}
	}
	for _, name := range r.Appendices {
		if !found["appendix:"+name] {
			return nil, fmt.Errorf("appendix %s: no matching file in selected layer", name)
		}
	}
	p.result.Blockers = referenceBlockers(root, docs, p.deleted)
	if r.Layer == "all" && len(r.Units) > 0 {
		ledgerPath := validationcache.DeferredLedgerRelPath
		if _, err := repopath.Canonical(root, ledgerPath); err != nil {
			return nil, err
		}
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(ledgerPath))); err == nil {
			if _, err := readFile(root, ledgerPath); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		ledger, err := validationcache.ReadDeferredLedger(root)
		if err != nil {
			return nil, err
		}
		kept := ledger.Entries[:0]
		for _, entry := range ledger.Entries {
			if !contains(r.Units, entry.OwnerUnit) {
				kept = append(kept, entry)
			}
		}
		if len(kept) != len(ledger.Entries) {
			ledger.Entries = kept
			data, err := json.MarshalIndent(ledger, "", "  ")
			if err != nil {
				return nil, err
			}
			p.writes[validationcache.DeferredLedgerRelPath] = append(data, '\n')
		}
	}
	writes, err := gaterun.RemovalInvalidations(root, p.deleted, owners)
	if err != nil {
		return nil, err
	}
	for path, data := range writes {
		p.writes[path] = data
	}
	for path := range p.deleted {
		p.result.Files = append(p.result.Files, path)
	}
	for path := range p.writes {
		if _, err := readFile(root, path); err != nil {
			return nil, err
		}
		p.result.Updates = append(p.result.Updates, path)
	}
	sort.Strings(p.result.Files)
	sort.Strings(p.result.Updates)
	return p, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func referenceBlockers(root string, docs []document, deleted map[string]bool) []string {
	remaining := map[string]bool{}
	for _, d := range docs {
		if !deleted[d.path] {
			remaining[d.path] = true
		}
	}
	var blockers []string
	for _, d := range docs {
		if deleted[d.path] {
			continue
		}
		check := func(field, value, kind string) {
			value = strings.Trim(value, "`\"' ")
			if value == "" || strings.EqualFold(value, "none") {
				return
			}
			resolved := false
			if strings.Contains(value, "/") {
				path, err := repopath.Canonical(root, value)
				resolved = err == nil && remaining[path]
			} else {
				switch kind {
				case "unit":
					resolved = remaining[specpaths.CandidateUnitSpecFileRef(value)] || remaining[specpaths.StableUnitSpecFileRef(value)]
				case "rule":
					resolved = remaining[specpaths.RuleStableFileRef(value)] || !strings.HasPrefix(value, "g_rule_") && remaining[specpaths.RuleCandidateFileRef(value)]
				case "appendix":
					resolved = remaining["docs/specs/units/"+d.layer+"/appendix/"+value]
				}
			}
			if !resolved {
				blockers = append(blockers, fmt.Sprintf("%s: %s references %s, which does not resolve after removal", d.path, field, value))
			}
		}
		for _, field := range []string{"unit_refs", "rule_refs"} {
			kind := strings.TrimSuffix(field, "_refs")
			raw := strings.TrimSpace(d.fm[field])
			if strings.HasPrefix(raw, "[") != strings.HasSuffix(raw, "]") {
				blockers = append(blockers, fmt.Sprintf("%s: %s has a malformed reference list", d.path, field))
				continue
			}
			for _, value := range specpaths.ParseRefList(d.fm[field]) {
				check(field, value, kind)
			}
		}
		for _, value := range specvalidation.ExtractAffectsDependencies(d.content) {
			check("affects.dependencies", value, "unit")
		}
		for _, value := range specvalidation.ExtractAffectsAppendices(d.content) {
			check("affects.appendices", value, "appendix")
		}
		for _, value := range specvalidation.ExtractAffectsRules(d.content) {
			check("affects.rules", value, "rule")
		}
		// The writing guide also defines a block list of rule exceptions.
		// Its rule IDs remain references even when a global rule is implicit.
		inExceptions := false
		for _, line := range strings.Split(strings.SplitN(d.content, "---", 3)[1], "\n") {
			if strings.HasPrefix(line, "rule_exceptions:") {
				inExceptions = true
				continue
			}
			if line != "" && line[0] != ' ' && line[0] != '\t' {
				inExceptions = false
			}
			if inExceptions {
				value, ok := strings.CutPrefix(strings.TrimSpace(line), "- rule:")
				if ok {
					check("rule_exceptions.rule", value, "rule")
				}
			}
		}
		check("evidence_appendix_ref", d.fm["evidence_appendix_ref"], "appendix")
	}
	sort.Strings(blockers)
	return blockers
}

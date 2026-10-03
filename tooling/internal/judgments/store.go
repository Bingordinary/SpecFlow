// Package judgments stores immutable, content-addressed review evidence.
// Callers serialize repository mutations with the gate repository lock.
package judgments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

const Directory = "docs/specs/meta/validation/judgments"
const ProtocolVersion = "verify-7-quality-conclusions-owned-appendices"
const RecordVersion = 3
const Code = "code"
const Design = "design"
const Architecture = "architecture"
const Item = "item"

type Reference struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

// Own spec paths are stored relative to their unit layer. Binding supplies
// the layer when a cache is checked, so fork/promote do not rewrite records.
type Dependency struct {
	Path string   `json:"path"`
	Own  bool     `json:"own,omitempty"`
	Hash string   `json:"hash,omitempty"`
	Deps []string `json:"deps,omitempty"`
}

type Record struct {
	Version      int             `json:"version"`
	Kind         string          `json:"kind"`
	Unit         string          `json:"unit,omitempty"`
	Subject      string          `json:"subject"`
	SpecContext  string          `json:"spec_context,omitempty"`
	Coverage     []string        `json:"coverage"`
	Inputs       []string        `json:"inputs"`
	Dependencies []Dependency    `json:"dependencies"`
	References   []Binding       `json:"references,omitempty"`
	Protocol     string          `json:"protocol"`
	Verdict      string          `json:"verdict"`
	Result       json.RawMessage `json:"result"`
	Report       string          `json:"report"`
	ReportDigest string          `json:"report_digest"`
	SourceRun    string          `json:"source_run"`
}

type Binding struct {
	Reference
	Layer  string `json:"layer,omitempty"`
	Source string `json:"source"` // executed, carried, reused
}

func Digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func identity(r Record) (string, []byte, error) {
	data, err := json.Marshal(r)
	return Digest(data), data, err
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func recordPath(root, id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("invalid judgment id %q", id)
	}
	return localstate.Path(root, Directory, id+".json")
}
func Save(root string, r Record) (Reference, error) {
	if r.Version != RecordVersion || r.Subject == "" || r.Protocol == "" || len(r.Inputs) == 0 || len(r.Dependencies) == 0 || len(r.Coverage) == 0 || len(r.Result) == 0 || r.SourceRun == "" || r.ReportDigest != Digest([]byte(r.Report)) {
		return Reference{}, fmt.Errorf("incomplete judgment record")
	}
	switch r.Kind {
	case Code, Design, Architecture, Item:
	default:
		return Reference{}, fmt.Errorf("unknown judgment kind %q", r.Kind)
	}
	if r.Kind == Item && !idPattern.MatchString(r.SpecContext) {
		return Reference{}, fmt.Errorf("item judgment requires a spec context fingerprint")
	}
	id, data, err := identity(r)
	if err != nil {
		return Reference{}, err
	}
	p, err := recordPath(root, id)
	if err != nil {
		return Reference{}, err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return Reference{}, err
	}
	// Publish complete bytes atomically without replacing an accepted record.
	temp, err := os.CreateTemp(filepath.Dir(p), ".judgment-")
	if err != nil {
		return Reference{}, err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return Reference{}, err
	}
	if err = temp.Close(); err != nil {
		return Reference{}, err
	}
	ref := Reference{ID: id, Digest: id}
	if err = os.Link(temp.Name(), p); err != nil {
		if !os.IsExist(err) {
			return Reference{}, err
		}
		if _, err = Load(root, ref); err != nil {
			return Reference{}, err
		}
	}
	if r.Kind == Item {
		if err := acceptItem(root, r, ref); err != nil {
			return Reference{}, err
		}
	}
	return ref, nil
}

func Load(root string, ref Reference) (*Record, error) {
	p, err := recordPath(root, ref.ID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("judgment %s: %w", ref.ID, err)
	}
	var r Record
	if err = json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("corrupt judgment %s: %w", ref.ID, err)
	}
	id, _, err := identity(r)
	if err != nil || id != ref.ID || ref.Digest != id || r.Version != RecordVersion || r.ReportDigest != Digest([]byte(r.Report)) || r.Kind == Item && !idPattern.MatchString(r.SpecContext) {
		return nil, fmt.Errorf("judgment %s digest or schema mismatch", ref.ID)
	}
	return &r, nil
}
func List(root string) ([]Reference, error) {
	p, err := localstate.Path(root, Directory, "index")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Dir(p))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Reference
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if idPattern.MatchString(id) && strings.HasSuffix(e.Name(), ".json") {
			ref := Reference{ID: id, Digest: id}
			out = append(out, ref)
		}
	}
	return out, nil
}

func itemHeadPath(root, unit, subject, context string) (string, error) {
	if !idPattern.MatchString(context) {
		return "", fmt.Errorf("invalid item spec context %q", context)
	}
	key := Digest([]byte(unit + "\n" + subject + "\n" + context))
	return localstate.Path(root, Directory, "accepted", key+".json")
}

// LatestItem selects the current decision for the requested layer's spec
// content. A different candidate context never replaces stable's decision.
func LatestItem(root, unit, subject, layer string) (Reference, bool, error) {
	context, err := SpecContext(root, unit, layer)
	if err != nil {
		return Reference{}, false, err
	}
	return latestItemForContext(root, unit, subject, context)
}

func latestItemForContext(root, unit, subject, context string) (Reference, bool, error) {
	p, err := itemHeadPath(root, unit, subject, context)
	if err != nil {
		return Reference{}, false, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Reference{}, false, nil
	}
	if err != nil {
		return Reference{}, false, err
	}
	var ref Reference
	if err := json.Unmarshal(data, &ref); err != nil || !idPattern.MatchString(ref.ID) || ref.Digest != ref.ID {
		return Reference{}, false, fmt.Errorf("damaged accepted item reference for %s:%s", unit, subject)
	}
	return ref, true, nil
}

// The mutable reference advances atomically under the repository lock; the
// record it names and all earlier records remain immutable.
func acceptItem(root string, record Record, ref Reference) error {
	p, err := itemHeadPath(root, record.Unit, record.Subject, record.SpecContext)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(p), ".accepted-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), p)
}

func InputPath(d Dependency, unit, layer string) string {
	if !d.Own {
		return d.Path
	}
	if strings.HasPrefix(d.Path, "/appendix/") {
		return "docs/specs/units/" + layer + d.Path
	}
	return "docs/specs/units/" + layer + "/unit_" + unit + d.Path
}

func Check(root string, ref Reference, layer, protocol string) error {
	return check(root, ref, layer, protocol, map[string]bool{})
}
func check(root string, ref Reference, layer, protocol string, seen map[string]bool) error {
	if seen[ref.ID] {
		return fmt.Errorf("cyclic judgment reference %s", ref.ID)
	}
	seen[ref.ID] = true
	defer delete(seen, ref.ID)
	r, err := Load(root, ref)
	if err != nil {
		return err
	}
	p, err := localstate.Path(root, Directory, "invalidated", ref.ID+".json")
	if err != nil {
		return err
	}
	if _, err = os.Stat(p); err == nil {
		return fmt.Errorf("judgment %s explicitly invalidated", ref.ID)
	} else if !os.IsNotExist(err) {
		return err
	}
	if r.Protocol != protocol {
		return fmt.Errorf("judgment %s review protocol changed", ref.ID)
	}
	for _, d := range r.Dependencies {
		path := InputPath(d, r.Unit, layer)
		if strings.HasPrefix(path, "rule:") {
			path = specpaths.ResolveRuleFile(root, strings.TrimPrefix(path, "rule:"))
			if rel, err := filepath.Rel(root, path); err == nil {
				path = filepath.ToSlash(rel)
			}
		}
		if strings.HasPrefix(path, "unit:") {
			rest := strings.TrimPrefix(path, "unit:")
			if _, appendix, ok := strings.Cut(rest, ":appendix:"); ok {
				path = specpaths.ResolveUnitAppendix(root, appendix)
			} else {
				path = specpaths.ResolveUnitFile(root, rest)
			}
			if rel, err := filepath.Rel(root, path); err == nil {
				path = filepath.ToSlash(rel)
			}
		}
		canonical, err := repopath.Canonical(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(canonical)))
		if err != nil {
			return fmt.Errorf("judgment %s input %s: %w", ref.ID, path, err)
		}
		if d.Hash != "" {
			h, err := specpaths.FileHash(filepath.Join(root, filepath.FromSlash(canonical)))
			if err != nil {
				return err
			}
			if h != d.Hash {
				return fmt.Errorf("judgment %s whole-file input changed: %s", ref.ID, path)
			}
		}
		if len(d.Deps) > 0 && !contenthash.DepsPresent(string(data), d.Deps) {
			return fmt.Errorf("judgment %s dependency changed: %s", ref.ID, path)
		}
	}
	for _, dep := range r.References {
		bound := layer
		if dep.Layer != "" {
			bound = dep.Layer
		}
		if err := check(root, dep.Reference, bound, protocol, seen); err != nil {
			return err
		}
	}
	if r.Kind == Item {
		boundContext, err := SpecContext(root, r.Unit, layer)
		if err != nil {
			return err
		}
		contexts := []string{r.SpecContext}
		if boundContext != r.SpecContext {
			contexts = append(contexts, boundContext)
		}
		for _, context := range contexts {
			latest, accepted, err := latestItemForContext(root, r.Unit, r.Subject, context)
			if err != nil {
				return err
			}
			if !accepted {
				// Delta may carry unchanged regions into a candidate context
				// that has not published its item decisions yet. Stable
				// protection requires a current decision for the bound content.
				if context == r.SpecContext || layer == "stable" {
					return fmt.Errorf("judgment %s has no current accepted item reference", ref.ID)
				}
				continue
			}
			if latest == ref {
				continue
			}
			current, err := Load(root, latest)
			if err != nil {
				return err
			}
			if current.Kind != Item || current.Unit != r.Unit || current.Subject != r.Subject || current.SpecContext != context {
				return fmt.Errorf("accepted item reference does not match judgment %s", ref.ID)
			}
			if err := check(root, latest, layer, protocol, seen); err != nil {
				return fmt.Errorf("current item decision is unavailable: %w", err)
			}
			same, err := sameItemDecision(r, current)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("judgment %s superseded by finalized %s decision %s", ref.ID, current.Verdict, latest.ID)
			}
		}
	}
	return nil
}

// Run-scoped finding ids and task keys do not change the decision. Severity,
// evidence and ownership do, even when the alignment verdict stays the same.
func sameItemDecision(a, b *Record) (bool, error) {
	decision := func(r *Record) (string, error) {
		var result struct {
			EffectiveStatus map[string]string `json:"effective_status"`
			Findings        []struct {
				Severity string `json:"severity"`
				Text     string `json:"text"`
				Detail   string `json:"detail"`
				OwnedBy  string `json:"owned_by"`
			} `json:"findings"`
		}
		if err := json.Unmarshal(r.Result, &result); err != nil || len(result.EffectiveStatus) != 1 {
			return "", fmt.Errorf("judgment has no finalized item decision")
		}
		parts := []string{r.Verdict}
		for _, status := range result.EffectiveStatus {
			parts = append(parts, status)
		}
		var findings []string
		for _, finding := range result.Findings {
			data, err := json.Marshal(finding)
			if err != nil {
				return "", err
			}
			findings = append(findings, string(data))
		}
		sort.Strings(findings)
		parts = append(parts, findings...)
		return strings.Join(parts, "\n"), nil
	}
	left, err := decision(a)
	if err != nil {
		return false, err
	}
	right, err := decision(b)
	return left == right, err
}
func Invalidate(root, id, reason string) error {
	if _, err := Load(root, Reference{ID: id, Digest: id}); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("invalidation reason is required")
	}
	p, err := localstate.Path(root, Directory, "invalidated", id+".json")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]string{"id": id, "reason": reason})
	return os.WriteFile(p, data, 0644)
}
func SameInputs(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\n") == strings.Join(b, "\n")
}

func OwnSuffix(p, unit, layer string, appendices []string) (string, bool) {
	if unit == "" {
		return "", false
	}
	prefix := "docs/specs/units/" + layer + "/unit_" + unit
	if p == prefix+".md" {
		return strings.TrimPrefix(p, prefix), true
	}
	for _, appendix := range appendices {
		if p == appendix {
			return "/appendix/" + filepath.Base(p), true
		}
	}
	return "", false
}

func CoversInputs(actual, required []string) bool {
	seen := map[string]bool{}
	for _, p := range actual {
		seen[p] = true
	}
	for _, p := range required {
		if !seen[p] {
			return false
		}
	}
	return true
}

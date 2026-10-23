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
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
)

const Directory = "docs/specs/meta/validation/judgments"
const ProtocolVersion = "verify-7-quality-conclusions-owned-appendices"
const RecordVersion = 4
const Code = "code"
const Design = "design"
const Architecture = "architecture"
const Item = "item"
const Unit = "unit"

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
	// Promise pins a provider contract dependency by promise fingerprint
	// instead of whole-file content. Path names the provider spec for audit;
	// Scope records the layer scope the pin was read in ("all" for a
	// candidate/current-layer read, "stable" for a published read). Scope is
	// audit metadata: liveness resolves the pin in the binding layer's scope,
	// because promote rewrites a candidate cache's binding to stable without
	// rewriting the immutable record.
	Promise     string `json:"promise,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Scope       string `json:"scope,omitempty"`
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
	case Code, Design, Architecture, Item, Unit:
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

// AcceptedReferences returns every accepted item pointer target. Accepted
// pointers are load-bearing current decisions: a damaged pointer is an
// error, never a skipped entry.
func AcceptedReferences(root string) ([]Reference, error) {
	dir, err := localstate.Path(root, Directory, "accepted")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Reference
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var ref Reference
		if err := json.Unmarshal(data, &ref); err != nil || !idPattern.MatchString(ref.ID) || ref.Digest != ref.ID {
			return nil, fmt.Errorf("damaged accepted item reference %s", e.Name())
		}
		out = append(out, ref)
	}
	return out, nil
}

// Remove deletes one judgment record and any invalidation marker naming it,
// returning the removed record's size in bytes. The caller must hold the
// repository lock and must have proven the record unreachable from every
// live consumer (see gaterun.CollectJudgments); accepted records are
// otherwise permanent.
func Remove(root, id string) (int64, error) {
	p, err := recordPath(root, id)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	if err := os.Remove(p); err != nil {
		return 0, err
	}
	marker, err := localstate.Path(root, Directory, "invalidated", id+".json")
	if err != nil {
		return info.Size(), err
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return info.Size(), err
	}
	return info.Size(), nil
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

// ResolveEvidencePath resolves one recorded evidence path — a physical
// repository path or a logical reference (rule:{id},
// unit:{name}, unit:{name}:appendix:{file}) — to its canonical
// repository-relative path. It is the single resolution rule every reader of
// recorded evidence shares, so the same evidence cannot be pinned under one
// spelling and checked under another.
func ResolveEvidencePath(root, path string) (string, error) {
	switch {
	case strings.HasPrefix(path, "rule:"):
		resolved := specpaths.ResolveRuleFile(root, strings.TrimPrefix(path, "rule:"))
		if rel, err := filepath.Rel(root, resolved); err == nil {
			path = filepath.ToSlash(rel)
		} else {
			path = resolved
		}
	case strings.HasPrefix(path, "unit:"):
		rest := strings.TrimPrefix(path, "unit:")
		var resolved string
		if _, appendix, ok := strings.Cut(rest, ":appendix:"); ok {
			resolved = specpaths.ResolveUnitAppendix(root, appendix)
		} else {
			resolved = specpaths.ResolveUnitFile(root, rest)
		}
		if rel, err := filepath.Rel(root, resolved); err == nil {
			path = filepath.ToSlash(rel)
		} else {
			path = resolved
		}
	}
	return repopath.Canonical(root, path)
}

// ResolveInputPath resolves one recorded dependency to its canonical
// repository-relative path: an owned spec file against the bound layer, a
// logical unit/rule reference against its current resolution, or a physical
// path as recorded. It is the single resolution rule the reuse check and the
// uncovered-evidence report share, so a file cannot be pinned under one
// spelling and reported uncovered under another.
func ResolveInputPath(root string, d Dependency, unit, layer string) (string, error) {
	return ResolveEvidencePath(root, InputPath(d, unit, layer))
}

func Check(root string, ref Reference, layer, protocol string) error {
	return check(root, ref, layer, protocol, map[string]bool{}, &promiseLiveness{root: root})
}

// promiseLiveness resolves promise pins against the binding layer's promise
// scope. Graphs are cached per Check call so one record with several
// dependencies expands each scope once.
//
// The binding layer, not the record's recorded Scope, decides the resolution:
// promote rewrites a candidate cache's bindings to the stable layer while the
// record itself stays immutable, so a stable confirmation must resolve provider
// promises in the published stable layer even though the record was written by
// the candidate run (framework/core/promise_model.md §Layer resolution).
type promiseLiveness struct {
	root  string
	graph map[string]*specvalidation.PromiseGraph
}

func (p *promiseLiveness) verify(id, fingerprint, layer string) error {
	if id == "" || fingerprint == "" {
		return fmt.Errorf("incomplete promise pin %q", id)
	}
	scope := specvalidation.PromiseScopeForLayer(layer)
	if p.graph == nil {
		p.graph = map[string]*specvalidation.PromiseGraph{}
	}
	graph, ok := p.graph[scope]
	if !ok {
		loaded, err := specvalidation.LoadPromiseGraphScope(p.root, scope)
		if err != nil {
			return err
		}
		p.graph[scope] = loaded
		graph = loaded
	}
	current, ok := graph.Fingerprint(id)
	if !ok {
		return fmt.Errorf("promise %s is unresolved in scope %s", id, scope)
	}
	if current != fingerprint {
		return fmt.Errorf("promise %s contract changed", id)
	}
	return nil
}

func check(root string, ref Reference, layer, protocol string, seen map[string]bool, promises *promiseLiveness) error {
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
		if d.Promise != "" {
			if d.Scope == "" {
				return fmt.Errorf("judgment %s: incomplete promise pin %q", ref.ID, d.Promise)
			}
			if err := promises.verify(d.Promise, d.Fingerprint, layer); err != nil {
				return fmt.Errorf("judgment %s: %w", ref.ID, err)
			}
			continue
		}
		canonical, err := ResolveInputPath(root, d, r.Unit, layer)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(canonical)))
		if err != nil {
			return fmt.Errorf("judgment %s input %s: %w", ref.ID, d.Path, err)
		}
		if d.Hash != "" {
			if specpaths.HashText(data) != d.Hash {
				return fmt.Errorf("judgment %s whole-file input changed: %s", ref.ID, canonical)
			}
		}
		if len(d.Deps) > 0 && !contenthash.DepsPresent(string(data), d.Deps) {
			return fmt.Errorf("judgment %s dependency changed: %s", ref.ID, canonical)
		}
	}
	for _, dep := range r.References {
		bound := layer
		if dep.Layer != "" {
			bound = dep.Layer
		}
		if err := check(root, dep.Reference, bound, protocol, seen, promises); err != nil {
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
			if err := check(root, latest, layer, protocol, seen, promises); err != nil {
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

// inputSpelling maps one evidence path to the spelling a record's own input
// list uses: a file this unit owns (its main spec or one of its appendices)
// becomes own:<suffix>; another unit's spec or appendix becomes the logical
// reference that carries it (unit:<name> / unit:<owner>:appendix:<file>), so
// the same evidence keeps one spelling across layers even though the read
// surface names the layer it reads; every other entry keeps its path or
// logical spelling.
func inputSpelling(root, p, unit, layer string, owned []string) string {
	if suffix, ok := OwnSuffix(p, unit, layer, owned); ok {
		return "own:" + suffix
	}
	if ref, ok := specpaths.LogicalUnitRefForPath(root, p); ok {
		return ref
	}
	return p
}

// normalizeOwnInputs resolves this unit's appendix paths, the ownership list
// inputSpelling needs.
func normalizeOwnInputs(root, unit, layer string) ([]string, error) {
	if unit == "" {
		return nil, nil
	}
	appendices, err := specpaths.UnitAppendices(root, unit, layer)
	if err != nil {
		return nil, err
	}
	owned := make([]string, 0, len(appendices))
	for _, appendix := range appendices {
		owned = append(owned, appendix.Path)
	}
	return owned, nil
}

// NormalizeInputs rewrites an evidence list into the recorded judgment-input
// spelling. The writer and every reader share this single normalization, so the
// same evidence is never written under one spelling and compared under another.
func NormalizeInputs(root, unit, layer string, inputs []string) ([]string, error) {
	owned, err := normalizeOwnInputs(root, unit, layer)
	if err != nil {
		return nil, err
	}
	out := append([]string(nil), inputs...)
	for i, p := range out {
		out[i] = inputSpelling(root, p, unit, layer, owned)
	}
	sort.Strings(out)
	return out, nil
}

// NewlyRequiredEvidence reports the first evidence input a recorded judgment no
// longer covers. A conclusion is reusable only while the evidence its key
// currently requires stays inside the evidence the record read: a key whose
// declared surface gained a file must judge that file instead of carrying a
// conclusion made without it. Callers use it as the one required-evidence rule
// (the gate's merged freshness check and the delta planner), so both decide
// reuse identically. The returned entry is the required input as the caller
// spelled it.
func NewlyRequiredEvidence(root, unit, layer string, record Record, required []string) (string, bool, error) {
	owned, err := normalizeOwnInputs(root, unit, layer)
	if err != nil {
		return "", false, err
	}
	declared := map[string]bool{}
	for _, p := range record.Inputs {
		declared[inputSpelling(root, p, unit, layer, owned)] = true
	}
	for _, p := range required {
		if !declared[inputSpelling(root, p, unit, layer, owned)] {
			return p, true, nil
		}
	}
	return "", false, nil
}

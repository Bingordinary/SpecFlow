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

// Reference names one record: its content hash, the digest that must match it,
// and the store directory the record lives in. The slot travels with the
// reference because a caller that holds a reference must be able to open the
// record without searching the store, and a record's own identity fields
// recompute that slot, so a reference cannot point somewhere the record does
// not claim.
type Reference struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
	Slot   string `json:"slot"`
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

// canonicalBytes is the record bytes a record's identity is derived from. The
// slot inside a reference is a locator computed from the referenced record's
// own fields, so it is not part of the referencing record's meaning: leaving it
// out keeps every identity stable while slots are filled in, and a record
// cannot gain or lose meaning by being moved.
func canonicalBytes(r Record) ([]byte, error) {
	canonical := r
	canonical.References = make([]Binding, len(r.References))
	copy(canonical.References, r.References)
	for i := range canonical.References {
		canonical.References[i].Slot = ""
	}
	return json.Marshal(canonical)
}

func identity(r Record) (string, []byte, error) {
	data, err := canonicalBytes(r)
	return Digest(data), data, err
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// KindDirectories is the fixed set of per-kind record directories, and the
// order a bare id is probed in. A record's kind is part of its identity, so it
// lives in the path: a caller that wants one kind enumerates that directory
// instead of reading every record to discover what it is.
var KindDirectories = []string{Code, Design, Architecture, Item, Unit}

func knownKind(kind string) bool {
	for _, known := range KindDirectories {
		if kind == known {
			return true
		}
	}
	return false
}

func recordPath(root, slot, id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("invalid judgment id %q", id)
	}
	if err := validateSlot(slot); err != nil {
		return "", err
	}
	return localstate.Path(root, Directory, filepath.FromSlash(slot), id+".json")
}

// validateSlot proves a slot is a store-relative directory shaped like
// <protocol>/<kind>/<unit>/<subject...>/<context> and cannot name a file
// outside the store.
func validateSlot(slot string) error {
	if slot == "" {
		return fmt.Errorf("judgment reference has no slot")
	}
	parts := strings.Split(slot, "/")
	if len(parts) < 5 {
		return fmt.Errorf("invalid judgment slot %q", slot)
	}
	for _, part := range parts {
		if !validPathComponent(part) {
			return fmt.Errorf("invalid judgment slot %q", slot)
		}
	}
	if !validPathComponent(parts[0]) {
		return fmt.Errorf("invalid judgment slot protocol %q", parts[0])
	}
	if !knownKind(parts[1]) {
		return fmt.Errorf("invalid judgment slot kind %q", parts[1])
	}
	return nil
}

// StoredPath returns the store path that holds ref. Callers that must touch a
// record's file directly (size accounting, diagnosis) use it instead of
// assuming a layout: the layout is the store's business.
func StoredPath(root string, ref Reference) (string, error) {
	return recordPath(root, ref.Slot, ref.ID)
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
	id, _, err := identity(r)
	if err != nil {
		return Reference{}, err
	}
	// The stored bytes carry the locator of every consumed record; the identity
	// is derived from the bytes without them (see canonicalBytes).
	data, err := json.Marshal(r)
	if err != nil {
		return Reference{}, err
	}
	slot, err := SlotOf(&r)
	if err != nil {
		return Reference{}, err
	}
	p, err := recordPath(root, slot, id)
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
	ref := Reference{ID: id, Digest: id, Slot: slot}
	if err = os.Link(temp.Name(), p); err != nil {
		if !os.IsExist(err) {
			return Reference{}, err
		}
		if _, err = Load(root, ref); err != nil {
			return Reference{}, err
		}
	}
	if r.Kind == Item {
		if err := acceptItem(root, slot, ref); err != nil {
			return Reference{}, err
		}
	}
	return ref, nil
}

func Load(root string, ref Reference) (*Record, error) {
	p, err := recordPath(root, ref.Slot, ref.ID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("judgment %s: %w", ref.ID, err)
	}
	return parse(p, data, ref)
}

// parse validates one record's canonical bytes against the reference that names
// it. It is the single validation rule shared by Load and Verify, so a record
// handed to Verify is held to exactly the check a freshly read record gets.
func parse(p string, data []byte, ref Reference) (*Record, error) {
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("corrupt judgment %s: %w", ref.ID, err)
	}
	id, _, err := identity(r)
	slot, slotErr := SlotOf(&r)
	if err != nil || slotErr != nil || slot != ref.Slot || id != ref.ID || ref.Digest != id || r.Version != RecordVersion || r.ReportDigest != Digest([]byte(r.Report)) || r.Kind == Item && !idPattern.MatchString(r.SpecContext) {
		return nil, fmt.Errorf("judgment %s digest or schema mismatch", ref.ID)
	}
	return &r, nil
}

// currentPointerName is the file inside a slot that names the decision in
// force for that key and spec context.
const currentPointerName = "current"

// walkStore enumerates the store. It returns every record reference and every
// current-decision pointer; the store is a directory tree keyed by protocol,
// kind, unit, subject and spec context, so the walk reads directories only and
// never a record body.
func walkStore(root string, kinds map[string]bool) (records, currents []Reference, err error) {
	store, rootErr := localstate.Root(root, Directory)
	if rootErr != nil {
		return nil, nil, rootErr
	}
	protocols, readErr := os.ReadDir(store)
	if os.IsNotExist(readErr) {
		return nil, nil, nil
	}
	if readErr != nil {
		return nil, nil, readErr
	}
	for _, protocol := range protocols {
		if !protocol.IsDir() || !validPathComponent(protocol.Name()) {
			continue
		}
		kindEntries, readErr := os.ReadDir(filepath.Join(store, protocol.Name()))
		if readErr != nil {
			return nil, nil, readErr
		}
		for _, kindEntry := range kindEntries {
			if !kindEntry.IsDir() || !kinds[kindEntry.Name()] {
				continue
			}
			dir := filepath.Join(store, protocol.Name(), kindEntry.Name())
			if walkErr := walkSlotTree(store, dir, &records, &currents); walkErr != nil {
				return nil, nil, walkErr
			}
		}
	}
	return records, currents, nil
}

func walkSlotTree(store, dir string, records, currents *[]Reference) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(store, dir)
	if err != nil {
		return err
	}
	slot := filepath.ToSlash(rel)
	for _, e := range entries {
		if e.IsDir() {
			if err := walkSlotTree(store, filepath.Join(dir, e.Name()), records, currents); err != nil {
				return err
			}
			continue
		}
		name := e.Name()
		if name == currentPointerName {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return err
			}
			var ref Reference
			if err := json.Unmarshal(data, &ref); err != nil || !idPattern.MatchString(ref.ID) || ref.Digest != ref.ID || ref.Slot != slot {
				return fmt.Errorf("damaged current decision pointer in %s", slot)
			}
			*currents = append(*currents, ref)
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if idPattern.MatchString(id) {
			*records = append(*records, Reference{ID: id, Digest: id, Slot: slot})
		}
	}
	return nil
}

// RefsInSlot returns the records filed under one slot. A query that knows the
// key it is asking about reads that slot's directory instead of walking the
// store, and a slot holds the handful of decisions ever accepted for one key.
func RefsInSlot(root, slot string) ([]Reference, error) {
	anchor, err := localstate.Path(root, Directory, filepath.FromSlash(slot), "_slot")
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(anchor)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Reference
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if idPattern.MatchString(id) {
			out = append(out, Reference{ID: id, Digest: id, Slot: slot})
		}
	}
	return out, nil
}

// List returns every record reference in the store, or only the records in the
// named kinds. The walk reads directories, never record bodies.
func List(root string, kinds ...string) ([]Reference, error) {
	records, _, err := walkStore(root, kindSet(kinds))
	return records, err
}

// Current returns the current-decision pointers a reader under protocol can
// use. A pointer filed under a retired standard is skipped: every reader asks
// with the deployed standard (see latestItemForContext's callers), so a retired
// decision can never be read again and must not confer liveness. The walk still
// parses every pointer, deployed or retired: a damaged pointer is an error,
// never a skipped entry.
func Current(root, protocol string) ([]Reference, error) {
	_, currents, err := walkStore(root, kindSet(nil))
	if err != nil {
		return nil, err
	}
	out := make([]Reference, 0, len(currents))
	for _, ref := range currents {
		filed, err := SlotProtocol(ref.Slot)
		if err != nil {
			return nil, err
		}
		if filed == protocol {
			out = append(out, ref)
		}
	}
	return out, nil
}

func kindSet(kinds []string) map[string]bool {
	set := map[string]bool{}
	if len(kinds) == 0 {
		kinds = KindDirectories
	}
	for _, kind := range kinds {
		if knownKind(kind) {
			set[kind] = true
		}
	}
	return set
}

// Remove deletes one judgment record and any invalidation marker naming it,
// returning the removed record's size in bytes. The caller must hold the
// repository lock and must have proven the record unreachable from every
// live consumer (see gaterun.CollectJudgments); accepted records are
// otherwise permanent.
func Remove(root string, ref Reference) (int64, error) {
	p, err := recordPath(root, ref.Slot, ref.ID)
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
	slotDir, err := localstate.Path(root, Directory, filepath.FromSlash(ref.Slot))
	if err != nil {
		return info.Size(), err
	}
	// A current-decision pointer that names the removed record goes with it, so
	// a collection never leaves a pointer to a record it just deleted.
	pointer := filepath.Join(slotDir, currentPointerName)
	if data, readErr := os.ReadFile(pointer); readErr == nil {
		var current Reference
		if json.Unmarshal(data, &current) == nil && current.ID == ref.ID {
			if err := os.Remove(pointer); err != nil && !os.IsNotExist(err) {
				return info.Size(), err
			}
		}
	}
	marker, err := localstate.Path(root, Directory, "invalidated", ref.ID+".json")
	if err != nil {
		return info.Size(), err
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return info.Size(), err
	}
	return info.Size(), nil
}

func itemPointerPath(root, protocol, unit, subject, context string) (string, error) {
	if !idPattern.MatchString(context) {
		return "", fmt.Errorf("invalid item spec context %q", context)
	}
	slot, err := Slot(protocol, Item, unit, subject, context)
	if err != nil {
		return "", err
	}
	return localstate.Path(root, Directory, filepath.FromSlash(slot), currentPointerName)
}

// LatestItem selects the current decision for the requested layer's spec
// content. A different candidate context never replaces stable's decision.
func LatestItem(root, protocol, unit, subject, layer string) (Reference, bool, error) {
	context, err := SpecContext(root, unit, layer)
	if err != nil {
		return Reference{}, false, err
	}
	return latestItemForContext(root, protocol, unit, subject, context)
}

func latestItemForContext(root, protocol, unit, subject, context string) (Reference, bool, error) {
	p, err := itemPointerPath(root, protocol, unit, subject, context)
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
		return Reference{}, false, fmt.Errorf("damaged current decision for %s:%s", unit, subject)
	}
	return ref, true, nil
}

// The mutable reference advances atomically under the repository lock; the
// record it names and all earlier records remain immutable.
func acceptItem(root, slot string, ref Reference) error {
	p, err := localstate.Path(root, Directory, filepath.FromSlash(slot), currentPointerName)
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
	return verifyRecord(root, r, ref, layer, protocol, seen, promises)
}

// Verify runs the same verification Check runs, for a record the caller has
// already loaded. The record must be the one Load returned for ref; id and
// digest are re-derived only when a record is read, so Verify trusts that
// pairing. A caller that reads a record to inspect it and then asks whether it
// is still valid would otherwise read and digest-verify the same file twice in
// one command.
func Verify(root string, record *Record, ref Reference, layer, protocol string) error {
	seen := map[string]bool{ref.ID: true}
	return verifyRecord(root, record, ref, layer, protocol, seen, &promiseLiveness{root: root})
}

func verifyRecord(root string, r *Record, ref Reference, layer, protocol string, seen map[string]bool, promises *promiseLiveness) error {
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
		data, hash, err := specpaths.FileBytes(filepath.Join(root, filepath.FromSlash(canonical)))
		if err != nil {
			return fmt.Errorf("judgment %s input %s: %w", ref.ID, d.Path, err)
		}
		if d.Hash != "" {
			if hash != d.Hash {
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
			latest, accepted, err := latestItemForContext(root, r.Protocol, r.Unit, r.Subject, context)
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

// Find returns the reference for one record id. An id alone no longer locates a
// record, so a caller that only has an id — an operator naming a judgment on
// the command line — walks the store's directories once.
func Find(root, id string) (Reference, error) {
	if !idPattern.MatchString(id) {
		return Reference{}, fmt.Errorf("invalid judgment id %q", id)
	}
	records, _, err := walkStore(root, kindSet(nil))
	if err != nil {
		return Reference{}, err
	}
	for _, ref := range records {
		if ref.ID == id {
			return ref, nil
		}
	}
	return Reference{}, fmt.Errorf("judgment %s is not in the store", id)
}

func Invalidate(root string, ref Reference, reason string) error {
	if _, err := Load(root, ref); err != nil {
		return err
	}
	id := ref.ID
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

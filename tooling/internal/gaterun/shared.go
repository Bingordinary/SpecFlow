package gaterun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func reviewKey(kind, unit, subject string) string {
	if kind == SessionKindCode {
		return "code:" + subject
	}
	if kind == SessionKindArchitecture {
		return "architecture:" + unit
	}
	if kind == SessionKindUnit {
		return "unit:" + unit
	}
	return kind + ":" + unit + ":" + subject
}

// Resolve contradicted evidence before deleting a pass cache. Current item
// heads remain authoritative even when the unit cache has already gone;
// open runs may also have published newer public evidence independently.
func targetedVerifyReferences(root, unit, target string, keys []string) ([]judgments.Reference, error) {
	baseline, err := validationcache.ReadGateBaseline(root, TargetKindUnit, unit, GateVerify)
	if err != nil {
		return nil, err
	}
	protocol, err := judgments.Protocol(root, GateVerify, TargetKindUnit)
	if err != nil {
		return nil, err
	}
	refs := map[string]judgments.Reference{}
	collect := func(bindings map[string]judgments.Binding) error {
		for _, key := range keys {
			if binding, ok := bindings[key]; ok {
				record, err := judgments.Load(root, binding.Reference)
				if err != nil {
					return err
				}
				// Item invalidation selects the current spec-context head below.
				// A forked cache may still bind a different stable context.
				if record.Kind != judgments.Item {
					refs[binding.ID] = binding.Reference
				}
			}
		}
		return nil
	}
	if baseline.Exists {
		layer := baseline.Target
		if layer == "" {
			layer = TargetCandidate
		}
		if layer != target {
			return nil, fmt.Errorf("cache target is %q, expected %q", layer, target)
		}
		if strings.TrimSpace(baseline.Judgments) != "" {
			var state JudgmentBaseline
			if err := json.Unmarshal([]byte(baseline.Judgments), &state); err != nil {
				return nil, fmt.Errorf("read judgments for targeted invalidation: %w", err)
			}
			if err := collect(state.Records); err != nil {
				return nil, err
			}
		}
	}
	runs, err := ListRuns(root)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.Status == StatusOpen && run.Gate == GateVerify && run.TargetKind == TargetKindUnit && run.TargetName == unit && run.Target == target {
			if err := collect(run.Records); err != nil {
				return nil, err
			}
		}
	}
	for _, key := range keys {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 || parts[0] != SessionKindItem {
			continue
		}
		layer := target
		if parts[1] != unit {
			continue
		}
		ref, accepted, err := judgments.LatestItem(root, protocol, parts[1], parts[2], layer)
		if err != nil {
			return nil, err
		}
		if accepted {
			refs[ref.ID] = ref
		}
	}
	ids := make([]string, 0, len(refs))
	for id := range refs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []judgments.Reference
	for _, id := range ids {
		if _, err := judgments.Load(root, refs[id]); err != nil {
			return nil, err
		}
		out = append(out, refs[id])
	}
	return out, nil
}

// addReviewInputs records a verify run's rule inputs: the main spec's declared
// rule_refs and the active global rules. The run's evidence surface is
// otherwise the spec-declared code surface, resolved by derive; no
// repository-wide name-token closure participates (a code file's evidence is
// the file itself plus cited evidence_files, not every repository file that
// shares a name stem — see framework/shared_judgments.md §Required tasks).
func (d *Derivation) addReviewInputs(run *Run) error {
	root := d.root
	content, err := readSpecContent(root, mainSpecRef(run))
	if err != nil {
		return err
	}
	global, err := globalRuleIDs(root)
	if err != nil {
		return err
	}
	ruleIDs := append(parseRefList(content, "rule_refs", ""), global...)
	for _, id := range dedupeSorted(ruleIDs) {
		addSnapshotRef(root, run, "rule:"+id)
	}
	return nil
}
func addSnapshotRef(root string, run *Run, p string) {
	if _, ok := run.SnapshotHash(root, p); ok {
		return
	}
	run.Refs = append(run.Refs, refreshRef(root, Ref{Ref: p, Source: SourceDerived}))
}

type sharedTask struct {
	ID      string               `json:"id"`
	Owner   string               `json:"owner_run"`
	Session string               `json:"owner_session,omitempty"`
	Key     string               `json:"key"`
	Inputs  []string             `json:"inputs"`
	Record  *judgments.Reference `json:"record,omitempty"`
}

func taskPath(root, id string) (string, error) {
	return localstate.Path(root, "meta/gate_runs/shared", id+".json")
}
func readTask(root, id string) (*sharedTask, error) {
	p, err := taskPath(root, id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var task sharedTask
	if err = json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	if task.ID != id {
		return nil, fmt.Errorf("shared task identity mismatch")
	}
	return &task, nil
}
func saveTask(root string, task *sharedTask) error {
	p, err := taskPath(root, task.ID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return writeFileAtomic(p, data, 0644)
}

func prepareSharedChecks(root string, run *Run) error {
	// The code-record index is needed only for a code coverage key that is
	// still unprepared, so it is built on first use. A run whose keys are all
	// settled (a review that adds no code key) or whose coverage carries no
	// code key reads no judgment record at all (issue #79).
	type codeRecord struct {
		ref    judgments.Reference
		record *judgments.Record
	}
	// One subject's code records are read the first time that subject is
	// asked about. The key names the subject, so its slot is computed rather
	// than searched for, and a plan never reads the records of a file it is not
	// planning.
	bySubject := map[string][]codeRecord{}
	loaded := map[string]bool{}
	loadSubject := func(subject string) error {
		if loaded[subject] {
			return nil
		}
		loaded[subject] = true
		slot, err := judgments.Slot(run.Protocol, judgments.Code, "", subject, "")
		if err != nil {
			return err
		}
		refs, err := judgments.RefsInSlot(root, slot)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			record, loadErr := judgments.Load(root, ref)
			if loadErr != nil || record.Kind != judgments.Code {
				continue
			}
			bySubject[subject] = append(bySubject[subject], codeRecord{ref: ref, record: record})
		}
		return nil
	}
	layer := run.Target
	// A plan holds the repository lock and mutates no judgment record, so one
	// record's check result is stable for the whole plan; memoizing it by id
	// keeps the reuse search and the fingerprint from re-reading and
	// re-hashing the same dependencies.
	checkMemo := map[string]error{}
	check := func(ref judgments.Reference) error {
		if err, ok := checkMemo[ref.ID]; ok {
			return err
		}
		err := judgments.Check(root, ref, layer, run.Protocol)
		checkMemo[ref.ID] = err
		return err
	}
	// An explicit re-run must execute even when a valid public record already
	// covers the file. A `--rerun` key or a conclusion the change review named
	// is forced: reuse would silently satisfy it with the old conclusion and
	// leave the recorded re-run unfulfilled, and the accepted shared task would
	// re-materialize it as an accepted session (issue #80 review path). Forced
	// public keys skip the reuse search and take their own task.
	currentSet := make(map[string]bool, len(run.Coverage))
	for i := range run.Coverage {
		currentSet[run.Coverage[i].Key] = true
	}
	forced := map[string]bool{}
	for _, raw := range run.RerunKeys {
		if key, ok := normalizeGateKey(run, raw, currentSet); ok {
			forced[key] = true
		}
	}
	if run.Review != nil {
		for _, key := range run.Review.Recheck {
			forced[key] = true
		}
	}
	for i := range run.Coverage {
		ck := &run.Coverage[i]
		if ck.Source != "" {
			// Already prepared: gate-plan stamps every key it publishes, and a
			// later review that reopens a coverage key reuses this stamping for
			// that new key only — an existing key's source and task are settled.
			continue
		}
		ck.Source = "executed"
		if ck.Kind != SessionKindCode {
			continue
		}
		if err := loadSubject(ck.File); err != nil {
			return err
		}
		inputs := coverageReadRefs(root, run, *ck)
		normalized, err := judgments.NormalizeInputs(root, ck.Unit, layer, inputs)
		if err != nil {
			return err
		}
		group := bySubject[ck.File]
		isForced := forced[ck.Key]
		if !isForced {
			for j := len(group) - 1; j >= 0; j-- {
				record := group[j].record
				if record.Unit != ck.Unit || !judgments.CoversInputs(record.Inputs, normalized) {
					continue
				}
				if check(group[j].ref) != nil {
					continue
				}
				run.Records[ck.Key] = judgments.Binding{Reference: group[j].ref, Layer: layer, Source: "reused"}
				ck.Source = "reused"
				break
			}
		}
		if ck.Source == "reused" {
			continue
		}
		fingerprint := []string{run.Protocol, ck.Key}
		if isForced {
			// A forced key never adopts an existing accepted shared task: a
			// distinct, run-private task guarantees the key still has to be
			// executed and publishes a fresh public record.
			fingerprint = append(fingerprint, "forced:"+run.RunID)
		}
		for _, entry := range group {
			if err := check(entry.ref); err != nil && strings.Contains(err.Error(), "explicitly invalidated") {
				fingerprint = append(fingerprint, entry.ref.ID)
			}
		}
		for _, input := range inputs {
			h, _ := run.SnapshotHash(root, input)
			fingerprint = append(fingerprint, input+"="+h)
		}
		ck.Task = judgments.Digest([]byte(strings.Join(fingerprint, "\n")))
		var task *sharedTask
		for {
			task, err = readTask(root, ck.Task)
			if os.IsNotExist(err) {
				task = &sharedTask{ID: ck.Task, Owner: run.RunID, Key: ck.Key, Inputs: inputs}
				err = saveTask(root, task)
			}
			if err != nil {
				return err
			}
			if task.Record == nil || check(*task.Record) == nil {
				break
			}
			// Keep the accepted task and its history intact. An unusable result
			// defines a new effective task shared by all subsequent planners.
			ck.Task = judgments.Digest([]byte(ck.Task + "\ninvalid-record:" + task.Record.ID))
		}
		if task.Owner != run.RunID {
			ck.Source = "waiting"
		}
	}
	return nil
}

// ClaimShared is called under the repository lock before a mission or submit.
// A pending task survives an interrupted run; it can be continued when that
// run has been replaced, without creating another public task.
func ClaimShared(root string, run *Run, keys []string) error {
	// Validate the whole batch before saving any assignment. The repository
	// lock keeps task ownership unchanged between this preflight and writes.
	session := SessionID(keys)
	var updates []*sharedTask
	for _, key := range keys {
		ck := run.CoverageByKey(key)
		if ck == nil || ck.Kind != SessionKindCode || ck.Task == "" {
			continue
		}
		task, err := readTask(root, ck.Task)
		if err != nil {
			return err
		}
		if task.Record != nil {
			return fmt.Errorf("public task %s is already accepted; use its shared result", ck.Key)
		}
		changed := false
		if task.Owner != run.RunID {
			owner, err := Load(root, task.Owner)
			if err == nil && owner.Status == StatusOpen {
				return fmt.Errorf("public task %s is assigned to run %s; continue that task or wait for its result", key, task.Owner)
			}
			task.Owner = run.RunID
			task.Session = ""
			changed = true
		}
		if task.Session != "" && task.Session != session {
			return fmt.Errorf("public task %s is assigned to session %s; continue that session", key, task.Session)
		}
		if task.Session == "" {
			task.Session = session
			changed = true
		}
		if changed {
			updates = append(updates, task)
		}
	}
	for _, task := range updates {
		if err := saveTask(root, task); err != nil {
			return err
		}
	}
	dirty := false
	for _, key := range keys {
		ck := run.CoverageByKey(key)
		if ck == nil || ck.Kind != SessionKindCode || ck.Task == "" {
			continue
		}
		if ck.Source == "waiting" {
			ck.Source = "executed"
			dirty = true
		}
	}
	if dirty {
		return writeRun(root, run)
	}
	return nil
}
func sharedStates(root string, run *Run, states []*SessionState) ([]*SessionState, error) {
	covered := map[string]bool{}
	for _, s := range states {
		if s.Status == SessionAccepted {
			for _, k := range s.Keys {
				covered[k] = true
			}
		}
	}
	for i := range run.Coverage {
		ck := &run.Coverage[i]
		if covered[ck.Key] {
			continue
		}
		binding, ok := run.Records[ck.Key]
		if !ok && ck.Task != "" {
			task, err := readTask(root, ck.Task)
			if err != nil {
				return nil, err
			}
			if task.Record != nil {
				binding = judgments.Binding{Reference: *task.Record, Layer: run.Target, Source: "reused"}
				ok = true
				run.Records[ck.Key] = binding
				ck.Source = "reused"
			}
		}
		if !ok {
			continue
		}
		r, err := judgments.Load(root, binding.Reference)
		if err != nil {
			return nil, err
		}
		if err := judgments.Verify(root, r, binding.Reference, binding.Layer, run.Protocol); err != nil {
			return nil, err
		}
		result, err := BoundJudgmentResult(r, *ck, binding.Layer)
		if err != nil {
			return nil, err
		}
		result.SessionID = SessionID([]string{ck.Key})
		result.Kind = ck.Kind
		semanticData, _ := json.Marshal(result)
		states = append(states, &SessionState{SemanticDigest: judgments.Digest(semanticData), SessionID: result.SessionID, Keys: []string{ck.Key}, Status: SessionAccepted, Report: r.Report, Result: &result})
	}
	return states, nil
}

// BoundJudgmentResult projects a single-subject record onto the consuming
// task and layer. Historical relationship and peer keys belong to its source
// run, not to the consumer's coverage set.
func BoundJudgmentResult(record *judgments.Record, ck CoverageKey, layer string) (SessionResult, error) {
	var result SessionResult
	if err := json.Unmarshal(record.Result, &result); err != nil {
		return result, err
	}
	result.Verdicts = map[string]string{ck.Key: record.Verdict}
	for _, status := range result.EffectiveStatus {
		result.EffectiveStatus = map[string]string{ck.Key: status}
		break
	}
	for i := range result.Findings {
		result.Findings[i].SourceKey = ck.Key
		result.Findings[i].AffectedKeys = nil
	}
	return result, nil
}

// PublishPublic accepts public evidence independently of unit synthesis. A
// co-batched session publishes the public records for its code keys here; its
// design judgments publish at finalize.
func PublishPublic(root string, run *Run, spec *SessionSpec, state *SessionState) error {
	published := 0
	for _, key := range state.Keys {
		ck := run.CoverageByKey(key)
		if ck == nil || ck.Kind != SessionKindCode {
			continue
		}
		if published == 0 {
			if err := ClaimShared(root, run, state.Keys); err != nil {
				return err
			}
		}
		ref, err := SaveJudgment(root, run, *ck, state.Result, state.Report, nil)
		if err != nil {
			return err
		}
		task, err := readTask(root, ck.Task)
		if err != nil {
			return err
		}
		task.Record = &ref
		if err := saveTask(root, task); err != nil {
			return err
		}
		run.Records[key] = judgments.Binding{Reference: ref, Layer: run.Target, Source: "executed"}
		published++
	}
	if published == 0 {
		return nil
	}
	return writeRun(root, run)
}

// SpecBasisError is the shared diagnostic for a design report whose declared
// spec anchor cannot be resolved against the unit main spec. gate-submit calls
// ValidateSpecBasis so the report is rejected where it is submitted, and
// SaveJudgment resolves the same anchors again when it derives the record's
// pins — the two must never disagree.
type SpecBasisError struct {
	Anchor string
	Spec   string
}

func (e SpecBasisError) Error() string {
	return fmt.Sprintf("Spec basis anchor %q does not resolve in %s — name an existing unique heading text or promise id of the unit spec: heading text is the heading line without its leading '#' markers (the heading `### 3. Data Structures` is named `3. Data Structures`)", e.Anchor, e.Spec)
}

// ValidateSpecBasis resolves a design report's declared spec anchors against
// the unit main spec of the run. An anchor is a promise entry id, or the
// heading text of a `##` section or a `###` or deeper narrative block; heading
// text is the heading line without its leading `#` markers.
func ValidateSpecBasis(root string, run *Run, anchors []string) error {
	mainSpec := mainSpecRef(run)
	text, err := contenthash.FileText(filepath.Join(root, filepath.FromSlash(mainSpec)))
	if err != nil {
		return err
	}
	for _, anchor := range anchors {
		if _, ok := contenthash.SpecAnchorDep(text, anchor); !ok {
			return SpecBasisError{Anchor: anchor, Spec: mainSpec}
		}
	}
	return nil
}

func SaveJudgment(root string, run *Run, ck CoverageKey, result *SessionResult, report string, refs []judgments.Binding) (judgments.Reference, error) {
	layer := run.Target
	unit := ck.Unit
	kind := ck.Kind
	subject := ck.File
	if kind == SessionKindArchitecture {
		subject = ck.Unit
	}
	if kind == SessionKindItem {
		kind = judgments.Item
		subject = ck.Item
	}
	if kind == SessionKindUnit {
		kind = judgments.Unit
		subject = ck.Unit
	}
	inputs := coverageReadRefs(root, run, ck)
	var owned []string
	if unit != "" {
		appendices, err := specpaths.UnitAppendices(root, unit, layer)
		if err != nil {
			return judgments.Reference{}, err
		}
		for _, appendix := range appendices {
			owned = append(owned, appendix.Path)
		}
	}
	copyResult := *result
	copyResult.Verdicts = map[string]string{ck.Key: result.Verdicts[ck.Key]}
	copyResult.Findings = nil
	copyResult.Observations = nil
	copyResult.ObservationDispositions = nil
	copyResult.EffectiveStatus = nil
	if status, ok := result.EffectiveStatus[ck.Key]; ok {
		copyResult.EffectiveStatus = map[string]string{ck.Key: status}
	}
	copyResult.Dispositions = nil
	copyResult.Ownerships = nil
	copyResult.Analysis = nil
	if analysis, ok := result.Analysis[ck.Key]; ok {
		copyResult.Analysis = map[string]string{ck.Key: analysis}
	}
	copyResult.SpecBasis = nil
	if ck.Kind == SessionKindDesign {
		if basis, ok := result.SpecBasis[ck.Key]; ok {
			// The declared anchors travel with the record: the record's spec
			// pins are derived from them, so the record states its own
			// dependency basis.
			copyResult.SpecBasis = map[string][]string{ck.Key: basis}
		}
	}
	copyResult.ReportDigest = "sha256:" + judgments.Digest([]byte(specpaths.NormalizeText(report)))
	if ck.Kind == SessionKindDesign {
		ids := map[string]bool{}
		for _, ref := range refs {
			public, err := judgments.Load(root, ref.Reference)
			if err != nil {
				return judgments.Reference{}, err
			}
			var publicResult SessionResult
			if err := json.Unmarshal(public.Result, &publicResult); err != nil {
				return judgments.Reference{}, err
			}
			for _, f := range publicResult.Observations {
				ids[f.ID] = true
			}
		}
		for _, disposition := range result.ObservationDispositions {
			if ids[disposition.FindingID] {
				copyResult.ObservationDispositions = append(copyResult.ObservationDispositions, disposition)
			}
		}
	}
	for _, f := range result.Findings {
		if f.SourceKey == ck.Key {
			copyResult.Findings = append(copyResult.Findings, f)
		}
	}
	for _, f := range result.Observations {
		if f.SourceKey == ck.Key {
			copyResult.Observations = append(copyResult.Observations, f)
		}
	}
	// Record dependencies are the conclusion's own object, derived from the
	// spec structure and the run's input surface — never from a report
	// declaration (the declaration layer was removed with the change-review
	// model). A delta re-runs a key when its own object changed, or when the
	// key now requires evidence the record never read; everything else is a
	// carry decision — the validate change review, or the verify scope
	// derivation.
	mainSpec := mainSpecRef(run)
	mainSpecText := func() (string, error) {
		return contenthash.FileText(filepath.Join(root, filepath.FromSlash(mainSpec)))
	}
	verifySnapshot := func(p string) (string, error) {
		h, ok := run.SnapshotHash(root, p)
		if !ok || h == "" {
			return "", fmt.Errorf("judgment input %s absent from snapshot", p)
		}
		current := refreshRef(root, Ref{Ref: p})
		if current.Hash != h {
			return "", fmt.Errorf("judgment input changed: %s", p)
		}
		return h, nil
	}
	ownPath := func(p string) (string, bool) {
		if suffix, ok := judgments.OwnSuffix(p, unit, layer, owned); ok {
			return suffix, true
		}
		return p, false
	}
	var deps []judgments.Dependency
	addWholeFile := func(p string) error {
		h, err := verifySnapshot(p)
		if err != nil {
			return err
		}
		dep := judgments.Dependency{Path: p, Hash: h}
		dep.Path, dep.Own = ownPath(p)
		deps = append(deps, dep)
		return nil
	}
	addRegion := func(p, dep string) error {
		if _, err := verifySnapshot(p); err != nil {
			return err
		}
		d := judgments.Dependency{Path: p, Deps: []string{dep}}
		d.Path, d.Own = ownPath(p)
		deps = append(deps, d)
		return nil
	}
	// addPromisePins records the provider contracts this conclusion read as
	// promise fingerprints, not whole files. A provider narrative, acceptance
	// item, test, or appendix change is not a contract change and must not
	// invalidate the consumer; only a contract change reopens it. The caller
	// supplies the exact dependency set: the unit-level scan uses every
	// promise edge, while an item pins only the promise it verifies.
	addPromisePins := func(ids []string) error {
		scope := specvalidation.PromiseScopeForLayer(run.Target)
		current, issues := specvalidation.PromiseFingerprintsForScope(root, scope)
		if len(issues) > 0 {
			return fmt.Errorf("promise graph: %s", strings.Join(issues, "; "))
		}
		for _, id := range ids {
			fingerprint, ok := current[id]
			if !ok {
				return fmt.Errorf("promise dependency %q is unresolved in %s scope", id, scope)
			}
			deps = append(deps, judgments.Dependency{
				Path:        providerSpecRel(root, run.Target, specvalidation.PromiseProvider(id)),
				Promise:     id,
				Fingerprint: fingerprint,
				Scope:       scope,
			})
		}
		return nil
	}
	switch ck.Kind {
	case SessionKindCode:
		// A public record's invalidation surface is the conclusion's own
		// object: the file itself plus the applicable rules. The run's input
		// manifest stays readable (record.Inputs) but never defines the
		// record's dependencies — an unrelated supplied evidence file must
		// not invalidate every code judgment of the run
		// (framework/shared_judgments.md §Required tasks).
		for _, p := range ck.ReadRefs {
			if err := addWholeFile(p); err != nil {
				return judgments.Reference{}, err
			}
		}
	case SessionKindItem:
		text, err := mainSpecText()
		if err != nil {
			return judgments.Reference{}, err
		}
		region, ok := contenthash.LocateAcceptanceItemRegion(text, ck.Item)
		if !ok {
			return judgments.Reference{}, fmt.Errorf("acceptance item %q region cannot be located in %s — the judgment cannot be pinned", ck.Item, mainSpec)
		}
		if err := addRegion(mainSpec, "region:acceptance_item:"+ck.Item+":"+contenthash.RegionCID(region.Text)); err != nil {
			return judgments.Reference{}, err
		}
		// The item's declared code surface is its own object too: a change to
		// the implementation the item verifies invalidates the alignment
		// judgment instead of carrying it silently
		// (framework/validation_cache.md §Structural Region Dependencies).
		surface, err := itemSurfaceFiles(root, run, ck.Item)
		if err != nil {
			return judgments.Reference{}, err
		}
		for _, p := range surface {
			if err := addWholeFile(p); err != nil {
				return judgments.Reference{}, err
			}
		}
		// The promise this item verifies is its only contract dependency
		// channel. Pin the providers that promise depends on — not every
		// provider the unit uses — so an unrelated provider contract change
		// does not reopen this item.
		if err := addPromisePins(specvalidation.PromiseDependenciesForItem(text, ck.Item)); err != nil {
			return judgments.Reference{}, err
		}
	case SessionKindDesign:
		// A design judgment reviews the named file (pinned whole-file) against
		// the unit spec. The spec is its rationale, pinned by the anchors the
		// report declares: the narrative blocks and promise entries the
		// conclusion actually relied on
		// (framework/unit_verify_checklist.md §Output Format → Spec basis).
		// An acceptance-item edit therefore re-runs that item without
		// mechanically invalidating every design judgment of the unit, and an
		// edit to narrative the judgment never relied on does not re-run it
		// either. Every declared anchor is resolved against the spec and every
		// failure is rejected, so the record can never silently under-pin.
		if err := addWholeFile(ck.File); err != nil {
			return judgments.Reference{}, err
		}
		text, err := mainSpecText()
		if err != nil {
			return judgments.Reference{}, err
		}
		anchors := result.SpecBasis[ck.Key]
		if len(anchors) == 0 {
			return judgments.Reference{}, fmt.Errorf("design judgment for %s declares no Spec basis anchors — a design record must name the narrative blocks and promise entries it relied on", ck.Key)
		}
		for _, anchor := range anchors {
			dep, ok := contenthash.SpecAnchorDep(text, anchor)
			if !ok {
				return judgments.Reference{}, SpecBasisError{Anchor: anchor, Spec: mainSpec}
			}
			if err := addRegion(mainSpec, dep); err != nil {
				return judgments.Reference{}, err
			}
		}
	case SessionKindArchitecture, SessionKindUnit:
		// Architecture and the unit-level alignment scan cover the whole unit:
		// the spec and every declared code-surface file are pinned whole-file.
		if err := addWholeFile(mainSpec); err != nil {
			return judgments.Reference{}, err
		}
		for _, f := range surfaceFiles(run, false) {
			if err := addWholeFile(f); err != nil {
				return judgments.Reference{}, err
			}
		}
		// The unit's own appendices are part of its recorded truth. The
		// unit-level conclusions read them, so a changed appendix re-runs them
		// instead of carrying silently — the conclusion pins what it read, not
		// merely what the run happened to list.
		appendices, err := specpaths.UnitAppendices(root, unit, layer)
		if err != nil {
			return judgments.Reference{}, err
		}
		for _, appendix := range appendices {
			if err := addWholeFile(appendix.Path); err != nil {
				return judgments.Reference{}, err
			}
		}
		if ck.Kind == SessionKindUnit {
			content, err := mainSpecText()
			if err != nil {
				return judgments.Reference{}, err
			}
			if err := addPromisePins(specvalidation.PromiseDependencyIDs(content)); err != nil {
				return judgments.Reference{}, err
			}
		}
	default:
		return judgments.Reference{}, fmt.Errorf("no record inputs are defined for session kind %q", ck.Kind)
	}
	data, err := json.Marshal(copyResult)
	if err != nil {
		return judgments.Reference{}, err
	}
	normalized, err := judgments.NormalizeInputs(root, unit, layer, inputs)
	if err != nil {
		return judgments.Reference{}, err
	}
	record := judgments.Record{Version: judgments.RecordVersion, Kind: kind, Unit: unit, Subject: subject, Coverage: []string{ck.Key}, Inputs: normalized, Dependencies: deps, References: refs, Protocol: run.Protocol, Verdict: result.Verdicts[ck.Key], Result: data, Report: report, ReportDigest: judgments.Digest([]byte(report)), SourceRun: run.RunID}
	if kind == judgments.Item {
		record.SpecContext, err = judgments.SpecContext(root, unit, layer)
		if err != nil {
			return judgments.Reference{}, err
		}
	}
	return judgments.Save(root, record)
}

// itemSurfaceFiles resolves one acceptance item's declared code surface to the
// repository files the run's snapshot expanded it to. The declared spellings
// are the item's implementation_surface and affects.files values
// (framework/validation_cache.md §Structural Region Dependencies); each is
// canonicalized and matched against the run's surfaces, where a directory was
// expanded to its files at ingestion.
func itemSurfaceFiles(root string, run *Run, item string) ([]string, error) {
	content, err := readSpecContent(root, mainSpecRef(run))
	if err != nil {
		return nil, err
	}
	declared := specvalidation.ItemCodeSurfaces(content)[item]
	if len(declared) == 0 {
		return nil, nil
	}
	byPath := map[string][]string{}
	for _, s := range run.Surfaces {
		files := make([]string, 0, len(s.Entries))
		for _, e := range s.Entries {
			files = append(files, e.Path)
		}
		byPath[s.Path] = files
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range declared {
		p, err := repopath.Canonical(root, d)
		if err != nil {
			return nil, err
		}
		for _, f := range byPath[p] {
			if seen[f] {
				continue
			}
			seen[f] = true
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out, nil
}

func RecordForKey(run *Run, key string) (judgments.Binding, bool) {
	binding, ok := run.Records[key]
	return binding, ok
}

// DesignPublicKey returns the code coverage key backing a design key's file.
func DesignPublicKey(ck CoverageKey) string {
	return reviewKey(SessionKindCode, "", ck.File)
}

// PublicResultForDesignKey reads the immutable public record for one design
// key's file. A co-batched session skips this lookup — its facts come from the
// code block of the same report.
func PublicResultForDesignKey(root string, run *Run, ck CoverageKey) (SessionResult, error) {
	publicKey := DesignPublicKey(ck)
	binding, ok := RecordForKey(run, publicKey)
	if !ok {
		return SessionResult{}, fmt.Errorf("design check %s has no accepted public record", ck.Key)
	}
	record, err := judgments.Load(root, binding.Reference)
	if err != nil {
		return SessionResult{}, err
	}
	if err := judgments.Verify(root, record, binding.Reference, binding.Layer, run.Protocol); err != nil {
		return SessionResult{}, err
	}
	result, err := BoundJudgmentResult(record, CoverageKey{Key: publicKey, Kind: SessionKindCode, File: ck.File}, binding.Layer)
	if err != nil {
		return SessionResult{}, err
	}
	result.SessionID = publicKey
	result.Kind = SessionKindCode
	return result, nil
}
func Persist(root string, run *Run) error { return writeRun(root, run) }

func IsQualityKind(kind string) bool {
	return kind == SessionKindDesign || kind == SessionKindCode || kind == SessionKindArchitecture
}
func IsItemKind(kind string) bool { return kind == SessionKindItem }

// IsAlignmentKind reports whether a session kind judges under the alignment
// lens: acceptance items (Steps 2, 3, 4, 6, 7) and the unit-level scan
// (Steps 1 and 5). Both carry an alignment verdict line.
func IsAlignmentKind(kind string) bool {
	return kind == SessionKindItem || kind == SessionKindUnit
}

func ExpectedCheckFor(root string, run *Run, ck CoverageKey) validationcache.ExpectedCheck {
	layer := run.Target
	kind := ck.Kind
	subject := ck.File
	if kind == SessionKindArchitecture {
		subject = ck.Unit
	}
	if kind == SessionKindItem {
		subject = ck.Item
		kind = judgments.Item
	}
	if kind == SessionKindUnit {
		subject = ck.Unit
		kind = judgments.Unit
	}
	return validationcache.ExpectedCheck{Key: ck.Key, Lens: ck.Lens, Kind: kind, Unit: ck.Unit, Layer: layer, Subject: subject, Inputs: coverageReadRefs(root, run, ck)}
}

// ExpectedChecks derives one unit's expected verify checks through a fresh
// derivation. Callers that derive several units in one invocation should
// hold one Derivation and call Derivation.ExpectedChecks instead, so the
// repo-wide audit and the directory expansions are shared.
func ExpectedChecks(root, unit, target string) ([]validationcache.ExpectedCheck, error) {
	d, err := NewDerivation(root)
	if err != nil {
		return nil, err
	}
	return d.ExpectedChecks(unit, target)
}

// ExpectedChecks derives one unit's expected verify checks through this
// derivation: the shared expansions and the once-computed surface audit
// back the run resolution and the coverage computation.
func (d *Derivation) ExpectedChecks(unit, target string) ([]validationcache.ExpectedCheck, error) {
	run, err := d.resolveRun(GateVerify, TargetKindUnit, unit, target, ModeFull, nil, nil)
	if err != nil {
		return nil, err
	}
	keys, err := d.computeCoverage(run)
	if err != nil {
		return nil, err
	}
	var out []validationcache.ExpectedCheck
	for _, ck := range keys {
		out = append(out, ExpectedCheckFor(d.root, run, ck))
	}
	return out, nil
}

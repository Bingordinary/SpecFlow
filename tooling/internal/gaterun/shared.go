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
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

func reviewKey(kind, unit, subject string) string {
	if kind == SessionKindCode {
		return "code:" + subject
	}
	if kind == SessionKindArchitecture {
		return "architecture:" + unit
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
		ref, accepted, err := judgments.LatestItem(root, parts[1], parts[2], layer)
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
	refs, err := judgments.List(root)
	if err != nil {
		return err
	}
	for i := range run.Coverage {
		ck := &run.Coverage[i]
		ck.Source = "executed"
		if ck.Kind != SessionKindCode {
			continue
		}
		inputs := coverageReadRefs(root, run, *ck)
		layer := run.Target
		normalized, err := normalizeOwnInputs(root, inputs, ck.Unit, layer)
		if err != nil {
			return err
		}
		for j := len(refs) - 1; j >= 0; j-- {
			record, err := judgments.Load(root, refs[j])
			if err != nil {
				continue
			}
			if record.Kind != judgments.Code || record.Subject != ck.File || record.Unit != ck.Unit || !judgments.CoversInputs(record.Inputs, normalized) {
				continue
			}
			if judgments.Check(root, refs[j], layer, run.Protocol) != nil {
				continue
			}
			run.Records[ck.Key] = judgments.Binding{Reference: refs[j], Layer: layer, Source: "reused"}
			ck.Source = "reused"
			break
		}
		if ck.Kind != SessionKindCode || ck.Source == "reused" {
			continue
		}
		fingerprint := []string{run.Protocol, ck.Key}
		for _, ref := range refs {
			record, err := judgments.Load(root, ref)
			if err == nil && record.Kind == judgments.Code && record.Subject == ck.File {
				if err := judgments.Check(root, ref, run.Target, run.Protocol); err != nil && strings.Contains(err.Error(), "explicitly invalidated") {
					fingerprint = append(fingerprint, ref.ID)
				}
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
			if task.Record == nil || judgments.Check(root, *task.Record, layer, run.Protocol) == nil {
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
		if err := judgments.Check(root, binding.Reference, binding.Layer, run.Protocol); err != nil {
			return nil, err
		}
		r, err := judgments.Load(root, binding.Reference)
		if err != nil {
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
func normalizeOwnInputs(root string, inputs []string, unit, layer string) ([]string, error) {
	var owned []string
	if unit != "" {
		appendices, err := specpaths.UnitAppendices(root, unit, layer)
		if err != nil {
			return nil, err
		}
		for _, appendix := range appendices {
			owned = append(owned, appendix.Path)
		}
	}
	out := append([]string(nil), inputs...)
	for i, p := range out {
		if suffix, ok := judgments.OwnSuffix(p, unit, layer, owned); ok {
			out[i] = "own:" + suffix
		}
	}
	sort.Strings(out)
	return out, nil
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
	// model). The delta mechanical floor re-runs a key when its own object
	// changed; every other change is the change review's business.
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
	switch ck.Kind {
	case SessionKindCode:
		// The public record covers a whole public evidence surface: every
		// read ref is pinned whole-file.
		for _, p := range inputs {
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
	case SessionKindDesign:
		// A design judgment reviews the named file against the whole unit
		// spec: both are its own object, so both are pinned whole-file.
		if err := addWholeFile(mainSpec); err != nil {
			return judgments.Reference{}, err
		}
		if err := addWholeFile(ck.File); err != nil {
			return judgments.Reference{}, err
		}
	case SessionKindArchitecture:
		// Architecture covers the whole unit: the spec and every declared
		// code-surface file are pinned whole-file.
		if err := addWholeFile(mainSpec); err != nil {
			return judgments.Reference{}, err
		}
		for _, f := range surfaceFiles(run, false) {
			if err := addWholeFile(f); err != nil {
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
	normalized, err := normalizeOwnInputs(root, inputs, unit, layer)
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
	if err := judgments.Check(root, binding.Reference, binding.Layer, run.Protocol); err != nil {
		return SessionResult{}, err
	}
	record, err := judgments.Load(root, binding.Reference)
	if err != nil {
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

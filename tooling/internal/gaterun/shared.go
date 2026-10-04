package gaterun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/localstate"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
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
		if len(parts) != 3 || parts[0] != SessionKindItem && parts[0] != SessionKindPreserve {
			continue
		}
		layer := target
		if parts[0] == SessionKindItem && parts[1] != unit {
			continue
		}
		if parts[0] == SessionKindPreserve {
			layer = TargetStable
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

// codeEvidenceExts is the extension filter for public evidence discovery:
// only these source files participate, plus each run's quality files.
const codeEvidenceExts = "|.go|.js|.jsx|.ts|.tsx|.py|.rs|.java|.c|.cc|.cpp|.h|.cs|.rb|.php|.swift|.kt|.sh|.ps1|.json|"

// evidenceCorpus is the run-wide repository source snapshot behind public
// evidence: one directory expansion and one read per file, shared by every
// quality file's derivation instead of repeating both per file.
type evidenceCorpus struct {
	texts map[string]string // NUL-filtered contents by slash path
	exts  map[string]string // lowercased extension by slash path
	rules []string          // active global rule ids
}

// loadEvidenceCorpus expands the repository once and reads every candidate
// source file once. targets (the run's quality files) join the corpus
// regardless of extension, mirroring the per-file inclusion rule. Governance
// trees never participate.
func loadEvidenceCorpus(root string, targets []string) (*evidenceCorpus, error) {
	files, err := repofiles.ExpandDir(root, ".")
	if err != nil {
		return nil, err
	}
	isTarget := make(map[string]bool, len(targets))
	for _, t := range targets {
		isTarget[t] = true
	}
	c := &evidenceCorpus{texts: map[string]string{}, exts: map[string]string{}}
	for _, f := range files {
		if strings.HasPrefix(f.Path, "docs/specs/") || strings.HasPrefix(f.Path, "specflow/") || strings.HasPrefix(f.Path, "meta/") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Path))
		if !isTarget[f.Path] && !strings.Contains(codeEvidenceExts, "|"+ext+"|") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil {
			return nil, err
		}
		if strings.IndexByte(string(data), 0) >= 0 {
			continue
		}
		c.texts[f.Path] = string(data)
		c.exts[f.Path] = ext
	}
	rules, err := globalRuleIDs(root)
	if err != nil {
		return nil, err
	}
	c.rules = rules
	return c, nil
}

// evidence derives file's public evidence set from the shared snapshot. The
// traversal sees exactly what a standalone derivation saw: the
// extension-filtered corpus plus file itself. Both directions of references
// and transitive dependencies participate, including tests. Extra inputs are
// part of the fixed scope.
func (c *evidenceCorpus) evidence(file string, extra []string) []string {
	seen := map[string]bool{file: true}
	queue := []string{file}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for p, text := range c.texts {
			if seen[p] || p != file && !strings.Contains(codeEvidenceExts, "|"+c.exts[p]+"|") {
				continue
			}
			base := filepath.Base(current)
			stem := strings.TrimSuffix(base, filepath.Ext(base))
			other := filepath.Base(p)
			otherStem := strings.TrimSuffix(other, filepath.Ext(other))
			// File references (with or without an extension) cover imports and
			// test fixtures. Conservatively include matches; never narrow by unit.
			linked := strings.Contains(text, base) || len(stem) > 2 && strings.Contains(text, stem) || strings.Contains(c.texts[current], other) || len(otherStem) > 2 && strings.Contains(c.texts[current], otherStem)
			if linked {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	for _, id := range c.rules {
		seen["rule:"+id] = true
	}
	for _, p := range extra {
		if !strings.HasPrefix(p, "docs/specs/units/") && !strings.HasPrefix(p, "unit:") {
			seen[p] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func protectedCoverage(root string, run *Run) ([]CoverageKey, error) {
	audit, err := specvalidation.SurfaceAudit(root)
	if err != nil {
		return nil, err
	}
	own := map[string]bool{}
	for _, f := range qualityFiles(run) {
		own[f] = true
	}
	for _, inputs := range run.PublicEvidence {
		for _, input := range inputs {
			if !strings.HasPrefix(input, "docs/specs/") && !isLogicalRef(input) {
				own[input] = true
			}
		}
	}
	refs, err := judgments.List(root)
	if err != nil {
		return nil, err
	}
	var out []CoverageKey
	for _, u := range audit.Units {
		if u.Unit == run.TargetName || u.Layer != TargetStable {
			continue
		}
		ref := targetLayerSpecRef(TargetKindUnit, u.Unit, TargetStable)
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref)))
		if err != nil {
			return nil, err
		}

		perItem := specvalidation.AcceptanceSurfaces(string(data))
		baseline, err := validationcache.ReadGateBaseline(root, "unit", u.Unit, GateVerify)
		if err != nil {
			return nil, fmt.Errorf("cannot derive stable protection for %s: %w", u.Unit, err)
		}
		for _, item := range specvalidation.ExtractAcceptanceItemIDs(string(data)) {
			// Match declarations using the same repository-relative paths as
			// surface discovery and public evidence, including directory scopes.
			var declaredPaths []string
			for _, path := range perItem[item] {
				canonical, err := repopath.Canonical(root, path)
				if err != nil {
					return nil, fmt.Errorf("cannot derive stable protection for %s item %s path %q: %w", u.Unit, item, path, err)
				}
				declaredPaths = appendUnique(declaredPaths, canonical)
			}
			related := false
			var acceptedInputs []string
			relatedPaths := append([]string(nil), declaredPaths...)
			// Cache evidence still identifies a requirement's implementation
			// when its referenced judgment is missing or damaged. The result
			// itself is never reused without the immutable-record checks.
			for _, entry := range baseline.Entries {
				if strings.HasPrefix(entry.Path, "docs/specs/") || isLogicalRef(entry.Path) {
					continue
				}
				for _, check := range entry.Checks {
					if check.Check == reviewKey(SessionKindItem, u.Unit, item) || check.Check == item {
						acceptedInputs = appendUnique(acceptedInputs, entry.Path)
						relatedPaths = appendUnique(relatedPaths, entry.Path)
						if own[entry.Path] {
							related = true
						}
					}
				}
			}
			for _, p := range declaredPaths {
				for file := range own {
					if file == p || strings.HasPrefix(file, strings.TrimSuffix(p, "/")+"/") {
						related = true
					}
				}
			}
			// Previously accepted evidence can connect a requirement to a shared
			// implementation even when its declaration names a caller instead.
			for _, r := range refs {
				record, err := judgments.Load(root, r)
				if err != nil {
					continue
				}
				if record.Kind != judgments.Item || record.Unit != u.Unit || record.Subject != item {
					continue
				}
				for _, d := range record.Dependencies {
					if !d.Own && !strings.HasPrefix(d.Path, "docs/specs/") && !isLogicalRef(d.Path) {
						relatedPaths = appendUnique(relatedPaths, d.Path)
					}
					if !d.Own && own[d.Path] {
						related = true
					}
				}
			}
			latest, accepted, err := judgments.LatestItem(root, u.Unit, item, TargetStable)
			if err != nil {
				return nil, err
			}
			if accepted {
				if record, err := judgments.Load(root, latest); err == nil {
					for _, dep := range record.Dependencies {
						acceptedInputs = appendUnique(acceptedInputs, judgments.InputPath(dep, u.Unit, TargetStable))
					}
				}
			}
			if related {
				reads := []string{ref}
				reads = appendUnique(reads, acceptedInputs...)
				for _, evidence := range run.PublicEvidence {
					connected := false
					for _, input := range evidence {
						for _, path := range relatedPaths {
							if input == path || strings.HasPrefix(input, strings.TrimSuffix(path, "/")+"/") {
								connected = true
							}
						}
					}
					if connected {
						for _, input := range evidence {
							if !isLogicalRef(input) && !strings.HasPrefix(input, "docs/specs/") {
								reads = appendUnique(reads, input)
							}
						}
					}
				}
				for _, f := range u.Files {
					reads = appendUnique(reads, f.Path)
				}
				appendices, err := unitAppendices(root, u.Unit, TargetStable)
				if err != nil {
					return nil, err
				}
				reads = appendUnique(reads, appendices...)
				globalIDs, err := globalRuleIDs(root)
				if err != nil {
					return nil, err
				}
				for _, id := range dedupeSorted(append(parseRefList(string(data), "rule_refs", ""), globalIDs...)) {
					reads = appendUnique(reads, specpaths.RuleStableFileRef(id))
				}
				for _, id := range parseRefList(string(data), "unit_refs", u.Unit) {
					reads = appendUnique(reads, targetLayerSpecRef(TargetKindUnit, id, TargetStable))
					appendices, err := unitAppendices(root, id, TargetStable)
					if err != nil {
						return nil, err
					}
					reads = appendUnique(reads, appendices...)
				}
				out = append(out, CoverageKey{Key: reviewKey(SessionKindPreserve, u.Unit, item), Kind: SessionKindPreserve, Lens: LensAlignment, Unit: u.Unit, Item: item, ReadRefs: reads})
			}
		}
	}
	return out, nil
}

func addReviewInputs(root string, run *Run) error {
	run.PublicEvidence = map[string][]string{}
	run.ProtectedEvidence = nil
	content, err := readSpecContent(root, mainSpecRef(run))
	if err != nil {
		return err
	}
	for _, id := range parseRefList(content, "rule_refs", "") {
		addSnapshotRef(root, run, "rule:"+id)
	}
	// One corpus snapshot serves every quality file: the repository is
	// expanded and its source corpus read once per run, not once per file.
	files := qualityFiles(run)
	corpus, err := loadEvidenceCorpus(root, files)
	if err != nil {
		return err
	}
	extra := extraInputPaths(run)
	for _, file := range files {
		inputs := corpus.evidence(file, extra)
		run.PublicEvidence[file] = inputs
		for _, p := range inputs {
			addSnapshotRef(root, run, p)
		}
	}
	protected, err := protectedCoverage(root, run)
	if err != nil {
		return err
	}
	for _, ck := range protected {
		for _, p := range ck.ReadRefs {
			current := refreshRef(root, Ref{Ref: p})
			if current.Hash == "" {
				return fmt.Errorf("protected requirement %s has unavailable stable evidence %s", ck.Key, p)
			}
			addSnapshotRef(root, run, p)
			canonical := canonicalPath(root, p)
			if strings.HasPrefix(canonical, "docs/specs/units/stable/") || strings.HasPrefix(canonical, "docs/specs/rules/stable/") {
				run.ProtectedEvidence = appendUnique(run.ProtectedEvidence, canonical)
			}
		}
	}
	sort.Strings(run.ProtectedEvidence)
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
		if ck.Kind != SessionKindCode && ck.Kind != SessionKindPreserve {
			continue
		}
		inputs := coverageReadRefs(root, run, *ck)
		layer := run.Target
		if ck.Kind == SessionKindPreserve {
			layer = TargetStable
		}
		normalized, err := normalizeOwnInputs(root, inputs, ck.Unit, layer)
		if err != nil {
			return err
		}
		if ck.Kind == SessionKindPreserve {
			layer = TargetStable
			ref, accepted, err := judgments.LatestItem(root, ck.Unit, ck.Item, layer)
			if err != nil {
				return err
			}
			if accepted {
				record, err := judgments.Load(root, ref)
				if err == nil && record.Kind == judgments.Item && record.Unit == ck.Unit && record.Subject == ck.Item && judgments.CoversInputs(record.Inputs, normalized) && judgments.Check(root, ref, layer, run.Protocol) == nil {
					run.Records[ck.Key] = judgments.Binding{Reference: ref, Layer: layer, Source: "reused"}
					ck.Source = "reused"
				}
			}
			// An unusable current decision requires a new protected review;
			// never search history for an older ALIGNED judgment.
			continue
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
		for i := range result.Findings {
			if ck.Kind == SessionKindPreserve {
				// One source finding can affect several protected items. Each
				// projection is a distinct judgment in the consuming run.
				result.Findings[i].ID = ck.Key + "/" + result.Findings[i].ID
				result.Findings[i].OwnedBy = ""
			}
		}
		if ck.Kind == SessionKindPreserve && result.Verdicts[ck.Key] != "ALIGNED" {
			for i := range result.Findings {
				result.Findings[i] = result.Findings[i].WithMinimumSeverity("P1")
			}
			if len(result.Findings) == 0 {
				result.Findings = append(result.Findings, Finding{ID: run.RunID + "/" + ck.Key + "/protected", SourceKey: ck.Key, Severity: "P1", Text: "protected stable requirement is not ALIGNED", Detail: "[P1] " + ck.Key + " — protected stable requirement is not ALIGNED (actionable)\n  problem: the accepted result cannot confirm the stable requirement\n  evidence: " + result.Verdicts[ck.Key] + "\n  impact: current changes may break confirmed behavior\n  fix: verify and restore the protected requirement"})
			}
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
	for i := range result.Scopes {
		result.Scopes[i].Key = ck.Key
		result.Scopes[i].Path = bindOwnPath(result.Scopes[i].Path, record, layer)
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
func bindOwnPath(p string, record *judgments.Record, layer string) string {
	for _, dep := range record.Dependencies {
		if !dep.Own {
			continue
		}
		for _, old := range []string{TargetStable, TargetCandidate} {
			if p == judgments.InputPath(dep, record.Unit, old) {
				return judgments.InputPath(dep, record.Unit, layer)
			}
		}
	}
	return p
}

// PublishPublic accepts public evidence independently of unit synthesis.
func PublishPublic(root string, run *Run, spec *SessionSpec, state *SessionState) error {
	if spec.Kind != SessionKindCode {
		return nil
	}
	if err := ClaimShared(root, run, state.Keys); err != nil {
		return err
	}
	for _, key := range state.Keys {
		ck := run.CoverageByKey(key)
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
	if kind == SessionKindItem || kind == SessionKindPreserve {
		kind = judgments.Item
		subject = ck.Item
		if ck.Kind == SessionKindPreserve {
			layer = TargetStable
		}
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
	copyResult.Scopes = nil
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
	var deps []judgments.Dependency
	for _, scope := range result.Scopes {
		if scope.Key != ck.Key {
			continue
		}
		copyResult.Scopes = append(copyResult.Scopes, scope)
		inputs = appendUnique(inputs, scope.Path)
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
	// Every read code file uses whole-file identity. Spec scopes use the
	// existing region dependencies assembled by the command layer below.
	for _, p := range inputs {
		declared := false
		for _, scope := range copyResult.Scopes {
			if scope.Path == p {
				declared = true
			}
		}
		isRule := strings.HasPrefix(p, "rule:") || strings.HasPrefix(p, "docs/specs/rules/")
		isSpec := strings.HasPrefix(p, "docs/specs/") || isLogicalRef(p)
		if ck.Kind != SessionKindCode && isSpec && !declared && !isRule {
			continue
		}
		h, ok := run.SnapshotHash(root, p)
		if !ok || h == "" {
			return judgments.Reference{}, fmt.Errorf("judgment input %s absent from snapshot", p)
		}
		current := refreshRef(root, Ref{Ref: p})
		if current.Hash != h {
			return judgments.Reference{}, fmt.Errorf("judgment input changed: %s", p)
		}
		dep := judgments.Dependency{Path: p, Hash: h}
		if ck.Kind != SessionKindCode && isSpec && !isRule {
			dep.Hash = ""
			dep.Deps = recordSpecDeps(root, p, ck.Key, result.Scopes)
		}
		if suffix, ok := judgments.OwnSuffix(p, unit, layer, owned); ok {
			dep.Path = suffix
			dep.Own = true
		}

		if dep.Hash == "" && len(dep.Deps) == 0 {
			dep.Hash = h
		}
		deps = append(deps, dep)
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

// Scope dependencies use the same cache builder as gate-finalize.
func recordSpecDeps(root, path, key string, scopes []Scope) []string {
	var out []string
	for _, scope := range scopes {
		if scope.Key != key || scope.Path != path {
			continue
		}
		declaration := validationcache.CheckDeclaration{Check: key}
		switch d := scope.Declaration; {
		case d == "all":
		case d == "acceptance_items":
			declaration.AcceptanceItems = true
		case strings.HasPrefix(d, "acceptance_item:"):
			declaration.AcceptanceItemIDs = strings.Split(strings.TrimPrefix(d, "acceptance_item:"), ",")
		case len(d) > 0 && d[0] >= '0' && d[0] <= '9':
			declaration.Ranges = d
		default:
			declaration.Sections = []string{d}
		}
		entry, err := validationcache.BuildEntryFromChecks(root, path, []validationcache.CheckDeclaration{declaration})
		if err == nil {
			out = appendUnique(out, entry.Deps...)
		}
	}
	return out
}

func RecordForKey(run *Run, key string) (judgments.Binding, bool) {
	binding, ok := run.Records[key]
	return binding, ok
}

// PublicResultsForDesign reads only the immutable public records for this
// design session's assigned files. Execution batches and carried unit results
// do not define the design input surface.
func PublicResultsForDesign(root string, run *Run, spec *SessionSpec) (map[string]SessionResult, error) {
	results := map[string]SessionResult{}
	for _, key := range spec.CheckKeys {
		ck := run.CoverageByKey(key)
		if ck == nil || ck.Kind != SessionKindDesign {
			return nil, fmt.Errorf("check %s is not an assigned design check", key)
		}
		publicKey := reviewKey(SessionKindCode, "", ck.File)
		binding, ok := RecordForKey(run, publicKey)
		if !ok {
			return nil, fmt.Errorf("design check %s has no accepted public record", key)
		}
		if err := judgments.Check(root, binding.Reference, binding.Layer, run.Protocol); err != nil {
			return nil, err
		}
		record, err := judgments.Load(root, binding.Reference)
		if err != nil {
			return nil, err
		}
		result, err := BoundJudgmentResult(record, CoverageKey{Key: publicKey, Kind: SessionKindCode, File: ck.File}, binding.Layer)
		if err != nil {
			return nil, err
		}
		result.SessionID = publicKey
		result.Kind = SessionKindCode
		results[key] = result
	}
	return results, nil
}
func Persist(root string, run *Run) error { return writeRun(root, run) }

func IsQualityKind(kind string) bool {
	return kind == SessionKindDesign || kind == SessionKindCode || kind == SessionKindArchitecture
}
func IsItemKind(kind string) bool { return kind == SessionKindItem || kind == SessionKindPreserve }

func ExpectedCheckFor(root string, run *Run, ck CoverageKey) validationcache.ExpectedCheck {
	layer := run.Target
	kind := ck.Kind
	subject := ck.File
	if kind == SessionKindArchitecture {
		subject = ck.Unit
	}
	if kind == SessionKindItem || kind == SessionKindPreserve {
		subject = ck.Item
		kind = judgments.Item
		if ck.Kind == SessionKindPreserve {
			layer = TargetStable
		}
	}
	return validationcache.ExpectedCheck{Key: ck.Key, Lens: ck.Lens, Kind: kind, Unit: ck.Unit, Layer: layer, Subject: subject, Inputs: coverageReadRefs(root, run, ck)}
}
func ExpectedChecks(root, unit, target string) ([]validationcache.ExpectedCheck, error) {
	run, err := resolveRun(root, GateVerify, TargetKindUnit, unit, target, ModeFull, nil, nil)
	if err != nil {
		return nil, err
	}
	keys, err := computeCoverage(root, run)
	if err != nil {
		return nil, err
	}
	var out []validationcache.ExpectedCheck
	for _, ck := range keys {
		out = append(out, ExpectedCheckFor(root, run, ck))
	}
	return out, nil
}

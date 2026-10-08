package gaterun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

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

// codeEvidenceExts is the extension filter for public evidence discovery:
// only these source files participate, plus each run's quality files.
const codeEvidenceExts = "|.go|.js|.jsx|.ts|.tsx|.py|.rs|.java|.c|.cc|.cpp|.h|.cs|.rb|.php|.swift|.kt|.sh|.ps1|.json|"

// dataEvidenceExts names the corpus extensions that participate as data files.
// A data file references another file only through an explicit path form; a
// bare word that happens to match a file name (a dependency name, a key) is
// not a reference. Corpus files outside codeEvidenceExts (target files with
// other extensions) are data as well.
const dataEvidenceExts = "|.json|"

// frameworkArtifacts are the paths SpecFlow installs into a consumer project
// (see tooling/internal/install/install.go). They are framework runtime, not
// project code, and never participate in evidence. Exact paths match one
// file; a trailing slash matches a directory tree. The installer's .gitignore
// entries cannot be assumed to exist, so the corpus excludes them explicitly.
var frameworkArtifacts = []string{
	".claude-plugin/plugin.json",
	".opencode/plugins/specflow.js",
	".agents/plugins/specflow/",
	".codex/hooks.json",
	"hooks/hooks.json",
}

func isFrameworkArtifact(path string) bool {
	for _, artifact := range frameworkArtifacts {
		if strings.HasSuffix(artifact, "/") {
			if strings.HasPrefix(path, artifact) {
				return true
			}
			continue
		}
		if path == artifact {
			return true
		}
	}
	return false
}

// evidenceEntry is one corpus file's matching data: its content for path-form
// references, its identifier stream for name references, and the identity a
// reference from another file must take.
type evidenceEntry struct {
	text   string   // NUL-filtered content
	stream string   // "\x00ident\x00ident\x00" lowercased identifier stream
	base   string   // basename (the path-form identity)
	seq    []string // basename stem tokens; nil when the name is too short
	data   bool     // data file: bare identifier mentions never link it
}

// evidenceCorpus is the run-wide repository source snapshot behind public
// evidence: one directory expansion and one read per file, shared by every
// quality file's derivation instead of repeating both per file.
type evidenceCorpus struct {
	entries map[string]evidenceEntry
	rules   []string // active global rule ids
}

// loadEvidenceCorpus expands the repository once and reads every candidate
// source file once. targets (the run's quality files) join the corpus
// regardless of extension, mirroring the per-file inclusion rule. Governance
// trees and framework deployment artifacts never participate.
func (d *Derivation) loadEvidenceCorpus(targets []string) (*evidenceCorpus, error) {
	files, err := d.expander.Expand(".")
	if err != nil {
		return nil, err
	}
	isTarget := make(map[string]bool, len(targets))
	for _, t := range targets {
		isTarget[t] = true
	}
	c := &evidenceCorpus{entries: map[string]evidenceEntry{}}
	for _, f := range files {
		if strings.HasPrefix(f.Path, "docs/specs/") || strings.HasPrefix(f.Path, "specflow/") || strings.HasPrefix(f.Path, "meta/") {
			continue
		}
		if isFrameworkArtifact(f.Path) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Path))
		isCode := strings.Contains(codeEvidenceExts, "|"+ext+"|")
		if !isTarget[f.Path] && !isCode {
			continue
		}
		data, err := os.ReadFile(filepath.Join(d.root, filepath.FromSlash(f.Path)))
		if err != nil {
			return nil, err
		}
		if strings.IndexByte(string(data), 0) >= 0 {
			continue
		}
		text := string(data)
		base := filepath.Base(f.Path)
		c.entries[f.Path] = evidenceEntry{
			text:   text,
			stream: tokenStream(text),
			base:   base,
			seq:    identityTokens(base),
			data:   !isCode || strings.Contains(dataEvidenceExts, "|"+ext+"|"),
		}
	}
	rules, err := globalRuleIDs(d.root)
	if err != nil {
		return nil, err
	}
	c.rules = rules
	return c, nil
}

// evidence derives file's public evidence set from the shared snapshot: the
// file itself, the files that reference it, and the files it references —
// one hop in each direction, never a transitive closure. A reference is an
// explicit path form, or — between two code files — the file's name tokens as
// a contiguous identifier sequence. Extra inputs are part of the fixed scope.
func (c *evidenceCorpus) evidence(file string, extra []string) []string {
	seen := map[string]bool{file: true}
	if target, ok := c.entries[file]; ok {
		for p, entry := range c.entries {
			if p == file {
				continue
			}
			if references(entry, target) || references(target, entry) {
				seen[p] = true
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

// references reports whether from's content cites to: an explicit path form
// of to's basename, or — only when neither entry is data — to's identity
// tokens as a contiguous identifier sequence.
func references(from, to evidenceEntry) bool {
	if containsPathForm(from.text, to.base) {
		return true
	}
	if from.data || to.data || len(to.seq) == 0 {
		return false
	}
	return containsTokenSeq(from.stream, to.seq)
}

// containsPathForm reports whether text contains base at an identifier
// boundary: "./logger.go" and "src/logger.go" match, "mylogger.go" does not.
func containsPathForm(text, base string) bool {
	if base == "" {
		return false
	}
	for offset := 0; offset <= len(text)-len(base); {
		index := strings.Index(text[offset:], base)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(base)
		if (index == 0 || !isIdentByte(text[index-1])) && (end == len(text) || !isIdentByte(text[end])) {
			return true
		}
		offset = index + 1
	}
	return false
}

// isIdentByte reports whether b can occur inside an identifier or a file name
// stem; it bounds path-form matches.
func isIdentByte(b byte) bool {
	return b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

// containsTokenSeq reports whether a lowercased identifier stream contains
// seq as a contiguous token run.
func containsTokenSeq(stream string, seq []string) bool {
	var needle strings.Builder
	for _, token := range seq {
		needle.WriteByte(0)
		needle.WriteString(token)
	}
	needle.WriteByte(0)
	return strings.Contains(stream, needle.String())
}

// identityTokens returns the identity a reference must match for a file
// basename: its stem split into lowercased identifier tokens. A stem whose
// tokens join to two characters or fewer has no identity — matching it would
// match everything.
func identityTokens(base string) []string {
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	tokens := tokenize(stem)
	if len(strings.Join(tokens, "")) <= 2 {
		return nil
	}
	return tokens
}

// tokenize splits a name into lowercased identifier tokens (see tokenStream).
func tokenize(name string) []string {
	stream := tokenStream(name)
	if stream == "" {
		return nil
	}
	return strings.Split(strings.Trim(stream, "\x00"), "\x00")
}

// tokenStream serializes text as "\x00token\x00token\x00..." with lowercased
// identifier tokens, so contiguous token sequences can be matched with a
// substring search. Tokens split on non-alphanumeric characters and on
// identifier word boundaries: lower-to-upper case changes, acronym-to-word
// changes, and letter-to-digit changes.
func tokenStream(text string) string {
	runes := []rune(text)
	var out strings.Builder
	var token []rune
	flush := func() {
		if len(token) == 0 {
			return
		}
		out.WriteByte(0)
		for _, r := range token {
			out.WriteRune(unicode.ToLower(r))
		}
		token = token[:0]
	}
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if len(token) > 0 {
			prev := token[len(token)-1]
			boundary := unicode.IsLower(prev) && unicode.IsUpper(r) ||
				unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) ||
				unicode.IsLetter(prev) != unicode.IsLetter(r)
			if boundary {
				flush()
			}
		}
		token = append(token, r)
	}
	flush()
	if out.Len() > 0 {
		out.WriteByte(0)
	}
	return out.String()
}

func (d *Derivation) addReviewInputs(run *Run) error {
	root := d.root
	run.PublicEvidence = map[string][]string{}
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
	corpus, err := d.loadEvidenceCorpus(files)
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

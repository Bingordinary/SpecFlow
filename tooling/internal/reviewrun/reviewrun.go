package reviewrun

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/reviewscope"
)

const (
	FlowSpecFlowReview       = "spec_flow_review"
	FlowSpecFlowDesignReview = "spec_flow_design_review"

	statusInProgress       = "in_progress"
	statusBlockedOnFinding = "blocked_on_finding"
	statusReadyForFinal    = "ready_for_final"
	statusClosedPass       = "closed_pass"
	statusClosedBlocked    = "closed_blocked"

	slicePending           = "pending"
	slicePassed            = "passed"
	sliceBlocked           = "blocked"
	sliceStale             = "stale"
	sliceSkippedNotInScope = "skipped_not_in_scope"

	timestampLayout = "2006-01-02T15:04:05Z"
)

var sliceColumns = []string{
	"slice_id",
	"slice_origin",
	"slice_type",
	"status",
	"review_question",
	"why_added",
	"parent_slice_id",
	"input_files",
	"input_fingerprint",
	"depends_on",
	"finding_refs",
	"result_summary",
	"exit_condition",
	"resume_next_step",
}

var scoreColumns = []string{
	"question_id",
	"status",
	"score",
	"score_basis",
	"evidence",
	"finding_refs",
	"result_summary",
	"resume_next_step",
}

type flowConfig struct {
	Flow                string
	RunStateDir         string
	ScopeLabel          string
	RunIDSuffix         string
	Title               string
	InitialActiveSlice  string
	InitialResumeStep   string
	UsesScoreState      bool
	CollectScope        func(string) (reviewscope.SpecFlowScope, error)
	BaselineDefinitions func() []sliceDefinition
}

type Result struct {
	File string
}

type InitResult struct {
	File         string
	Created      bool
	Reused       bool
	DeletedFiles []DeletedRunStateFile
}

type DeletedRunStateFile struct {
	File   string
	Reason string
}

type ValidationResult struct {
	File        string
	Valid       bool
	Diagnostics []string
}

type RefreshResult struct {
	File             string
	StaleSlices      []string
	MissingInputs    []string
	ChangedSlices    []string
	LastUpdatedAtUTC string
}

type TouchResult struct {
	File             string
	LastUpdatedAtUTC string
}

type runState struct {
	Fields   map[string]string
	Baseline []sliceEntry
	Dynamic  []sliceEntry
	Score    []scoreEntry
	Findings string
	Resume   string
}

type sliceEntry struct {
	SliceID          string
	SliceOrigin      string
	SliceType        string
	Status           string
	ReviewQuestion   string
	WhyAdded         string
	ParentSliceID    string
	InputFiles       []string
	InputFingerprint string
	DependsOn        []string
	FindingRefs      string
	ResultSummary    string
	ExitCondition    string
	ResumeNextStep   string
}

type sliceDefinition struct {
	ID             string
	SliceType      string
	ReviewQuestion string
	InputFiles     func(reviewscope.SpecFlowScope) []string
	DependsOn      []string
}

type scoreEntry struct {
	QuestionID     string
	Status         string
	Score          string
	ScoreBasis     string
	Evidence       string
	FindingRefs    string
	ResultSummary  string
	ResumeNextStep string
}

type runStateFile struct {
	Path        string
	State       runState
	LastUpdated time.Time
}

type invalidRunStateFile struct {
	Path   string
	Reason string
}

func ConfiguredFlows() []string {
	return []string{FlowSpecFlowReview, FlowSpecFlowDesignReview}
}

func configForFlow(flow string) (flowConfig, error) {
	switch strings.TrimSpace(flow) {
	case FlowSpecFlowReview:
		return flowConfig{
			Flow:                FlowSpecFlowReview,
			RunStateDir:         "docs/specs/_governance_review/spec_flow_review",
			ScopeLabel:          "default_governance_baseline",
			RunIDSuffix:         "default_governance_baseline",
			Title:               "Spec Flow Review Run State",
			InitialActiveSlice:  "scope_inventory",
			InitialResumeStep:   "review slice scope_inventory",
			CollectScope:        reviewscope.CollectDefaultSpecFlowScope,
			BaselineDefinitions: specFlowReviewBaselineDefinitions,
		}, nil
	case FlowSpecFlowDesignReview:
		return flowConfig{
			Flow:                FlowSpecFlowDesignReview,
			RunStateDir:         "docs/specs/_governance_review/spec_flow_design_review",
			ScopeLabel:          "default_design_baseline",
			RunIDSuffix:         "default_design_baseline",
			Title:               "Spec Flow Design Review Run State",
			InitialActiveSlice:  "design_foundation",
			InitialResumeStep:   "review slice design_foundation",
			UsesScoreState:      true,
			CollectScope:        reviewscope.CollectDefaultSpecFlowDesignScope,
			BaselineDefinitions: specFlowDesignReviewBaselineDefinitions,
		}, nil
	default:
		return flowConfig{}, fmt.Errorf("unsupported review flow %q", flow)
	}
}

func Init(repoRoot, flow string, now time.Time) (InitResult, error) {
	config, err := configForFlow(flow)
	if err != nil {
		return InitResult{}, err
	}
	now = now.UTC()
	if err := os.MkdirAll(filepath.Join(repoRoot, filepath.FromSlash(config.RunStateDir)), 0o755); err != nil {
		return InitResult{}, err
	}

	openFiles, invalidFiles, err := findOpenRunStates(repoRoot, config, now)
	if err != nil {
		return InitResult{}, err
	}
	result := InitResult{}
	if len(openFiles)+len(invalidFiles) > 1 {
		return InitResult{}, fmt.Errorf("multiple unclosed %s run-state files found: %s", config.Flow, joinRunStateCandidates(openFiles, invalidFiles))
	}
	for _, invalidFile := range invalidFiles {
		if err := os.Remove(invalidFile.Path); err != nil {
			return InitResult{}, err
		}
		result.DeletedFiles = append(result.DeletedFiles, DeletedRunStateFile{File: invalidFile.Path, Reason: invalidFile.Reason})
	}
	if len(openFiles) > 1 {
		return InitResult{}, fmt.Errorf("multiple open %s run-state files found: %s", config.Flow, joinRunStatePaths(openFiles))
	}
	if len(openFiles) == 1 {
		age := now.Sub(openFiles[0].LastUpdated)
		switch {
		case age <= 2*time.Hour:
			return InitResult{File: openFiles[0].Path, Reused: true}, nil
		case age <= 7*24*time.Hour:
			return InitResult{}, fmt.Errorf("open run-state file requires manual reuse decision before run-init can continue: %s last_updated_at=%s age=%s", openFiles[0].Path, formatUTC(openFiles[0].LastUpdated), age.Round(time.Second))
		default:
			if err := os.Remove(openFiles[0].Path); err != nil {
				return InitResult{}, err
			}
			result.DeletedFiles = append(result.DeletedFiles, DeletedRunStateFile{File: openFiles[0].Path, Reason: "expired_over_7_days"})
		}
	}

	scope, err := config.CollectScope(repoRoot)
	if err != nil {
		return InitResult{}, err
	}

	runID := now.Format("20060102-150405") + "-" + config.RunIDSuffix
	state := runState{
		Fields: map[string]string{
			"review_flow":          config.Flow,
			"review_run_id":        runID,
			"scope_label":          config.ScopeLabel,
			"status":               statusInProgress,
			"created_at":           formatUTC(now),
			"last_updated_at":      formatUTC(now),
			"active_slice":         config.InitialActiveSlice,
			"baseline_slice_table": "present",
			"dynamic_slice_table":  "none",
			"finding_refs":         "none",
			"blocked_reason":       "none",
			"resume_next_step":     config.InitialResumeStep,
		},
		Findings: "none",
		Resume:   "none",
	}
	state.Baseline, err = buildBaselineSlices(repoRoot, scope, config.BaselineDefinitions())
	if err != nil {
		return InitResult{}, err
	}
	if config.UsesScoreState {
		state.Score = buildScoreState()
	}

	file := filepath.Join(repoRoot, filepath.FromSlash(config.RunStateDir), runID+".md")
	if err := os.WriteFile(file, []byte(renderState(config, state)), 0o644); err != nil {
		return InitResult{}, err
	}
	result.File = file
	result.Created = true
	return result, nil
}

func ValidateFile(repoRoot, flow, file string, now time.Time) ValidationResult {
	result := ValidationResult{File: file}
	config, err := configForFlow(flow)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, err.Error())
		return result
	}
	state, err := parseFile(file)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, err.Error())
		return result
	}
	result.Diagnostics = validateState(repoRoot, config, state, now.UTC())
	result.Valid = len(result.Diagnostics) == 0
	return result
}

func Refresh(repoRoot, flow, file string, now time.Time) (RefreshResult, error) {
	config, err := configForFlow(flow)
	if err != nil {
		return RefreshResult{}, err
	}
	now = now.UTC()
	state, err := parseFile(file)
	if err != nil {
		return RefreshResult{}, err
	}
	if diagnostics := validateState(repoRoot, config, state, now); len(diagnostics) > 0 {
		return RefreshResult{}, fmt.Errorf("run-state validation failed: %s", strings.Join(diagnostics, "; "))
	}

	result := RefreshResult{File: file, LastUpdatedAtUTC: formatUTC(now)}
	staleSet := map[string]bool{}
	allSlices := append([]sliceEntry{}, state.Baseline...)
	allSlices = append(allSlices, state.Dynamic...)
	for _, slice := range allSlices {
		if slice.Status == sliceStale {
			staleSet[slice.SliceID] = true
		}
	}
	for i := range allSlices {
		fingerprint, missing, err := computeFingerprint(repoRoot, allSlices[i].InputFiles)
		if err != nil {
			return RefreshResult{}, err
		}
		changed := len(missing) == 0 && fingerprint != allSlices[i].InputFingerprint
		if changed {
			result.ChangedSlices = append(result.ChangedSlices, allSlices[i].SliceID)
		}
		for _, missingPath := range missing {
			result.MissingInputs = append(result.MissingInputs, allSlices[i].SliceID+":"+missingPath)
		}
		if len(missing) == 0 {
			allSlices[i].InputFingerprint = fingerprint
		}
		if allSlices[i].Status == slicePassed && (changed || len(missing) > 0) {
			allSlices[i].Status = sliceStale
			allSlices[i].ResultSummary = "stale: input changed"
			if len(missing) > 0 {
				allSlices[i].ResultSummary = "stale: input missing"
			}
			staleSet[allSlices[i].SliceID] = true
			result.StaleSlices = append(result.StaleSlices, allSlices[i].SliceID)
		}
	}

	for changed := true; changed; {
		changed = false
		for i := range allSlices {
			if allSlices[i].SliceType != "cross_convergence" || allSlices[i].Status != slicePassed {
				continue
			}
			for _, dependency := range allSlices[i].DependsOn {
				if staleSet[dependency] {
					allSlices[i].Status = sliceStale
					allSlices[i].ResultSummary = "stale: dependency stale"
					staleSet[allSlices[i].SliceID] = true
					result.StaleSlices = append(result.StaleSlices, allSlices[i].SliceID)
					changed = true
					break
				}
			}
		}
	}

	state.Baseline = allSlices[:len(state.Baseline)]
	state.Dynamic = allSlices[len(state.Baseline):]
	if config.UsesScoreState && hasStaleSlice(allSlices) {
		markScoredRowsStale(state.Score)
	}
	state.Fields["last_updated_at"] = formatUTC(now)
	if err := os.WriteFile(file, []byte(renderState(config, state)), 0o644); err != nil {
		return RefreshResult{}, err
	}
	sort.Strings(result.StaleSlices)
	sort.Strings(result.ChangedSlices)
	sort.Strings(result.MissingInputs)
	return result, nil
}

func Touch(repoRoot, flow, file string, now time.Time) (TouchResult, error) {
	config, err := configForFlow(flow)
	if err != nil {
		return TouchResult{}, err
	}
	now = now.UTC()
	state, err := parseFile(file)
	if err != nil {
		return TouchResult{}, err
	}
	if diagnostics := validateState(repoRoot, config, state, now); len(diagnostics) > 0 {
		return TouchResult{}, fmt.Errorf("run-state validation failed: %s", strings.Join(diagnostics, "; "))
	}
	state.Fields["last_updated_at"] = formatUTC(now)
	if err := os.WriteFile(file, []byte(renderState(config, state)), 0o644); err != nil {
		return TouchResult{}, err
	}
	return TouchResult{File: file, LastUpdatedAtUTC: formatUTC(now)}, nil
}

func findOpenRunStates(repoRoot string, config flowConfig, now time.Time) ([]runStateFile, []invalidRunStateFile, error) {
	dir := filepath.Join(repoRoot, filepath.FromSlash(config.RunStateDir))
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, nil, err
	}
	openFiles := []runStateFile{}
	invalidFiles := []invalidRunStateFile{}
	for _, file := range matches {
		state, err := parseFile(file)
		if err != nil {
			invalidFiles = append(invalidFiles, invalidRunStateFile{Path: file, Reason: "invalid_run_state"})
			continue
		}
		status := strings.TrimSpace(state.Fields["status"])
		if status == statusClosedPass || status == statusClosedBlocked {
			continue
		}
		diagnostics := validateState(repoRoot, config, state, now)
		if len(diagnostics) > 0 {
			invalidFiles = append(invalidFiles, invalidRunStateFile{Path: file, Reason: "invalid_run_state"})
			continue
		}
		lastUpdated, err := parseTimestamp(state.Fields["last_updated_at"])
		if err != nil {
			invalidFiles = append(invalidFiles, invalidRunStateFile{Path: file, Reason: "invalid_run_state"})
			continue
		}
		openFiles = append(openFiles, runStateFile{Path: file, State: state, LastUpdated: lastUpdated})
	}
	sort.Slice(openFiles, func(i, j int) bool {
		return openFiles[i].Path < openFiles[j].Path
	})
	sort.Slice(invalidFiles, func(i, j int) bool {
		return invalidFiles[i].Path < invalidFiles[j].Path
	})
	return openFiles, invalidFiles, nil
}

func joinRunStatePaths(files []runStateFile) string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return strings.Join(paths, ", ")
}

func joinRunStateCandidates(openFiles []runStateFile, invalidFiles []invalidRunStateFile) string {
	paths := make([]string, 0, len(openFiles)+len(invalidFiles))
	for _, file := range openFiles {
		paths = append(paths, file.Path)
	}
	for _, file := range invalidFiles {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return strings.Join(paths, ", ")
}

func validateState(repoRoot string, config flowConfig, state runState, now time.Time) []string {
	diagnostics := []string{}
	requiredFields := []string{
		"review_flow",
		"review_run_id",
		"scope_label",
		"status",
		"created_at",
		"last_updated_at",
		"active_slice",
		"baseline_slice_table",
		"dynamic_slice_table",
		"finding_refs",
		"blocked_reason",
		"resume_next_step",
	}
	for _, field := range requiredFields {
		if strings.TrimSpace(state.Fields[field]) == "" {
			diagnostics = append(diagnostics, "missing run field: "+field)
		}
	}
	if state.Fields["review_flow"] != config.Flow {
		diagnostics = append(diagnostics, "review_flow must be "+config.Flow)
	}
	if state.Fields["scope_label"] != config.ScopeLabel {
		diagnostics = append(diagnostics, "scope_label must be "+config.ScopeLabel)
	}
	if !isOpenRunStatus(state.Fields["status"]) {
		if state.Fields["status"] == statusClosedPass || state.Fields["status"] == statusClosedBlocked {
			diagnostics = append(diagnostics, "closed run-state files cannot be reused")
		} else {
			diagnostics = append(diagnostics, "invalid run status: "+state.Fields["status"])
		}
	}
	for _, field := range []string{"created_at", "last_updated_at"} {
		if _, err := parseTimestamp(state.Fields[field]); err != nil {
			diagnostics = append(diagnostics, field+" must use UTC format YYYY-MM-DDTHH:MM:SSZ")
		}
	}
	if lastUpdated, err := parseTimestamp(state.Fields["last_updated_at"]); err == nil && lastUpdated.After(now) {
		diagnostics = append(diagnostics, "last_updated_at must not be later than current UTC time")
	}

	expected := baselineIDSet(config.BaselineDefinitions())
	seen := map[string]bool{}
	allIDs := map[string]bool{}
	for _, slice := range state.Baseline {
		if seen[slice.SliceID] {
			diagnostics = append(diagnostics, "duplicate baseline slice: "+slice.SliceID)
		}
		seen[slice.SliceID] = true
		allIDs[slice.SliceID] = true
		if !expected[slice.SliceID] {
			diagnostics = append(diagnostics, "unexpected baseline slice: "+slice.SliceID)
		}
		if slice.SliceOrigin != "baseline" {
			diagnostics = append(diagnostics, "baseline slice must use slice_origin baseline: "+slice.SliceID)
		}
		if slice.ParentSliceID != "none" {
			diagnostics = append(diagnostics, "baseline slice parent_slice_id must be none: "+slice.SliceID)
		}
		diagnostics = append(diagnostics, validateSlice(repoRoot, slice)...)
	}
	for id := range expected {
		if !seen[id] {
			diagnostics = append(diagnostics, "missing baseline slice: "+id)
		}
	}

	for _, slice := range state.Dynamic {
		if allIDs[slice.SliceID] {
			diagnostics = append(diagnostics, "duplicate slice_id: "+slice.SliceID)
		}
		allIDs[slice.SliceID] = true
	}
	for _, slice := range state.Dynamic {
		if slice.SliceOrigin != "dynamic" {
			diagnostics = append(diagnostics, "dynamic slice must use slice_origin dynamic: "+slice.SliceID)
		}
		if slice.ParentSliceID == "" || slice.ParentSliceID == "none" || slice.ParentSliceID == slice.SliceID || !allIDs[slice.ParentSliceID] {
			diagnostics = append(diagnostics, "dynamic slice parent_slice_id must reference an existing slice: "+slice.SliceID)
		}
		for _, field := range []struct {
			name  string
			value string
		}{
			{"review_question", slice.ReviewQuestion},
			{"why_added", slice.WhyAdded},
			{"exit_condition", slice.ExitCondition},
		} {
			if strings.TrimSpace(field.value) == "" || strings.TrimSpace(field.value) == "none" {
				diagnostics = append(diagnostics, "dynamic slice missing "+field.name+": "+slice.SliceID)
			}
		}
		diagnostics = append(diagnostics, validateSlice(repoRoot, slice)...)
	}
	if config.UsesScoreState {
		diagnostics = append(diagnostics, validateScoreState(state.Score)...)
	} else if len(state.Score) > 0 {
		diagnostics = append(diagnostics, "score state is not allowed for "+config.Flow)
	}
	return diagnostics
}

func validateSlice(repoRoot string, slice sliceEntry) []string {
	diagnostics := []string{}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"slice_id", slice.SliceID},
		{"slice_origin", slice.SliceOrigin},
		{"slice_type", slice.SliceType},
		{"status", slice.Status},
		{"review_question", slice.ReviewQuestion},
		{"why_added", slice.WhyAdded},
		{"parent_slice_id", slice.ParentSliceID},
		{"input_fingerprint", slice.InputFingerprint},
		{"finding_refs", slice.FindingRefs},
		{"result_summary", slice.ResultSummary},
		{"exit_condition", slice.ExitCondition},
		{"resume_next_step", slice.ResumeNextStep},
	} {
		if strings.TrimSpace(field.value) == "" {
			diagnostics = append(diagnostics, field.name+" is required for slice: "+slice.SliceID)
		}
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"slice_id", slice.SliceID},
		{"slice_origin", slice.SliceOrigin},
		{"slice_type", slice.SliceType},
		{"status", slice.Status},
		{"review_question", slice.ReviewQuestion},
		{"why_added", slice.WhyAdded},
		{"input_fingerprint", slice.InputFingerprint},
		{"result_summary", slice.ResultSummary},
		{"exit_condition", slice.ExitCondition},
		{"resume_next_step", slice.ResumeNextStep},
	} {
		if strings.TrimSpace(field.value) == "none" {
			diagnostics = append(diagnostics, field.name+" must not be none for slice: "+slice.SliceID)
		}
	}
	if slice.SliceType != "local" && slice.SliceType != "cross_convergence" {
		diagnostics = append(diagnostics, "invalid slice_type for "+slice.SliceID+": "+slice.SliceType)
	}
	if !isSliceStatus(slice.Status) {
		diagnostics = append(diagnostics, "invalid slice status for "+slice.SliceID+": "+slice.Status)
	}
	if len(slice.InputFiles) == 0 && slice.Status != sliceSkippedNotInScope {
		diagnostics = append(diagnostics, "input_files is required unless skipped_not_in_scope: "+slice.SliceID)
	}
	for _, relPath := range slice.InputFiles {
		if strings.TrimSpace(relPath) == "" || filepath.IsAbs(relPath) || strings.Contains(relPath, "\\") {
			diagnostics = append(diagnostics, "input_files must use repository-relative slash paths: "+slice.SliceID)
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relPath))); err != nil && !os.IsNotExist(err) {
			diagnostics = append(diagnostics, "cannot inspect input file for "+slice.SliceID+": "+relPath)
		}
	}
	return diagnostics
}

func validateScoreState(scores []scoreEntry) []string {
	diagnostics := []string{}
	if len(scores) != 8 {
		diagnostics = append(diagnostics, fmt.Sprintf("score state must contain 8 rows, got %d", len(scores)))
	}
	seen := map[string]bool{}
	for i, score := range scores {
		expectedID := fmt.Sprintf("q%d", i+1)
		if score.QuestionID != expectedID {
			diagnostics = append(diagnostics, fmt.Sprintf("score state row %d question_id must be %s", i+1, expectedID))
		}
		if seen[score.QuestionID] {
			diagnostics = append(diagnostics, "duplicate score question_id: "+score.QuestionID)
		}
		seen[score.QuestionID] = true
		for _, field := range []struct {
			name  string
			value string
		}{
			{"question_id", score.QuestionID},
			{"status", score.Status},
			{"score", score.Score},
			{"score_basis", score.ScoreBasis},
			{"evidence", score.Evidence},
			{"finding_refs", score.FindingRefs},
			{"result_summary", score.ResultSummary},
			{"resume_next_step", score.ResumeNextStep},
		} {
			if strings.TrimSpace(field.value) == "" {
				diagnostics = append(diagnostics, field.name+" is required for score state: "+score.QuestionID)
			}
		}
		if !isScoreStatus(score.Status) {
			diagnostics = append(diagnostics, "invalid score state status for "+score.QuestionID+": "+score.Status)
		}
		if !isScoreValue(score.Score) {
			diagnostics = append(diagnostics, "invalid score value for "+score.QuestionID+": "+score.Score)
		}
	}
	return diagnostics
}

func buildScoreState() []scoreEntry {
	result := make([]scoreEntry, 0, 8)
	for i := 1; i <= 8; i++ {
		id := fmt.Sprintf("q%d", i)
		result = append(result, scoreEntry{
			QuestionID:     id,
			Status:         "pending",
			Score:          "none",
			ScoreBasis:     "none",
			Evidence:       "none",
			FindingRefs:    "none",
			ResultSummary:  "pending",
			ResumeNextStep: "score " + id,
		})
	}
	return result
}

func markScoredRowsStale(scores []scoreEntry) {
	for i := range scores {
		if scores[i].Status != "scored" {
			continue
		}
		scores[i].Status = "stale"
		scores[i].ResultSummary = "stale: review input stale"
	}
}

func hasStaleSlice(slices []sliceEntry) bool {
	for _, slice := range slices {
		if slice.Status == sliceStale {
			return true
		}
	}
	return false
}

func specFlowReviewBaselineDefinitions() []sliceDefinition {
	return []sliceDefinition{
		{
			ID:             "scope_inventory",
			SliceType:      "local",
			ReviewQuestion: "Does the default governance baseline scope include every required governance input family.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union(scope.FrameworkGuidelineFiles, scope.CommandFiles, scope.GuidanceSkillFiles, scope.SharedGovernanceFiles, scope.TemplateGovernanceFiles, scope.TemplateEntryFiles, scope.ProjectEntryFiles, scope.AgentOperabilityFiles, scope.ProjectRegistryFiles, scope.ToolingContractFiles, scope.ToolingSourceFiles, scope.ActiveProjectStandardFiles)
			},
		},
		{
			ID:             "review_entry_policy",
			SliceType:      "local",
			ReviewQuestion: "Do the governance review entry policies define a complete review entry and finding contract.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return []string{
					"specflow/framework/spec_flow_review.md",
					"specflow/framework/spec_flow_design_review.md",
					"specflow/framework/severity_policy.md",
					"specflow/framework/checkpoint_protocol.md",
				}
			},
		},
		{
			ID:             "routing_and_command_policy",
			SliceType:      "local",
			ReviewQuestion: "Do routing and command policies send each request to the correct governed next step.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union([]string{"specflow/framework/natural_language_routing.md", "specflow/framework/command_policy.md", "specflow/framework/scenario_policy.md"}, scope.CommandFiles, scope.GuidanceSkillFiles)
			},
		},
		{
			ID:             "truth_and_implementation_gates",
			SliceType:      "local",
			ReviewQuestion: "Do truth ownership and implementation gates prevent implementation from outrunning accepted truth.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return []string{
					"specflow/framework/spec_policy.md",
					"specflow/framework/repository_mapping_policy.md",
					"specflow/framework/implementation_change_policy.md",
					"specflow/framework/candidate_handoff_contract.md",
					"specflow/framework/downgrade_policy.md",
					"specflow/framework/recovery_policy.md",
					"specflow/framework/git_policy.md",
				}
			},
		},
		{
			ID:             "shared_governance",
			SliceType:      "local",
			ReviewQuestion: "Do shared-governance flows preserve shared truth ownership and downstream impact.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return scope.SharedGovernanceFiles
			},
		},
		{
			ID:             "process_and_impact_state",
			SliceType:      "local",
			ReviewQuestion: "Do process state, snapshots, and impact rules keep resumable process files trustworthy.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union([]string{
					"specflow/framework/impact_sync_policy.md",
					"specflow/framework/process_snapshot_contract.md",
					"specflow/framework/recovery_policy.md",
				}, scope.TemplateGovernanceFiles)
			},
		},
		{
			ID:             "entry_and_project_extension",
			SliceType:      "local",
			ReviewQuestion: "Do entry files and project-local extensions stay bounded by framework governance.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union([]string{
					"specflow/framework/entry_index_registry.md",
					"specflow/framework/project_standards_policy.md",
					"specflow/framework/project_standard_create.md",
				}, scope.TemplateEntryFiles, scope.ProjectEntryFiles, scope.ProjectRegistryFiles, scope.ActiveProjectStandardFiles)
			},
		},
		{
			ID:             "tooling_execution",
			SliceType:      "local",
			ReviewQuestion: "Does deterministic tooling stay inside its mechanical execution boundary.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union(scope.ToolingContractFiles, scope.ToolingSourceFiles)
			},
		},
		{
			ID:             "agent_operability_local",
			SliceType:      "local",
			ReviewQuestion: "Can an agent follow the local governance files without guessing hidden context.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return scope.AgentOperabilityFiles
			},
		},
		{
			ID:             "routing_to_command_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do routing decisions and command contracts converge without gaps or conflicting stops.",
			DependsOn:      []string{"routing_and_command_policy", "review_entry_policy"},
			InputFiles:     reviewDependencyFiles("routing_and_command_policy", "review_entry_policy"),
		},
		{
			ID:             "command_to_process_state_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do command outcomes and process-state contracts converge into resumable execution.",
			DependsOn:      []string{"routing_and_command_policy", "process_and_impact_state"},
			InputFiles:     reviewDependencyFiles("routing_and_command_policy", "process_and_impact_state"),
		},
		{
			ID:             "truth_to_implementation_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do truth gates and implementation permission rules converge before code changes.",
			DependsOn:      []string{"truth_and_implementation_gates", "routing_and_command_policy"},
			InputFiles:     reviewDependencyFiles("truth_and_implementation_gates", "routing_and_command_policy"),
		},
		{
			ID:             "shared_to_impact_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do shared-governance rules and impact-state rules converge for downstream invalidation.",
			DependsOn:      []string{"shared_governance", "process_and_impact_state"},
			InputFiles:     reviewDependencyFiles("shared_governance", "process_and_impact_state"),
		},
		{
			ID:             "entry_extension_to_review_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do project entry extensions remain visible to full-scope governance review.",
			DependsOn:      []string{"entry_and_project_extension", "scope_inventory", "review_entry_policy"},
			InputFiles:     reviewDependencyFiles("entry_and_project_extension", "scope_inventory", "review_entry_policy"),
		},
		{
			ID:             "tooling_to_rule_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do tooling commands implement only the mechanical actions permitted by governance rules.",
			DependsOn:      []string{"tooling_execution", "process_and_impact_state", "review_entry_policy"},
			InputFiles:     reviewDependencyFiles("tooling_execution", "process_and_impact_state", "review_entry_policy"),
		},
		{
			ID:             "agent_operability_path_walk",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Can an agent walk from entry instructions through routing, commands, and checkpoints without hidden decisions.",
			DependsOn:      []string{"agent_operability_local", "routing_and_command_policy", "truth_and_implementation_gates", "shared_governance", "process_and_impact_state"},
			InputFiles:     reviewDependencyFiles("agent_operability_local", "routing_and_command_policy", "truth_and_implementation_gates", "shared_governance", "process_and_impact_state"),
		},
	}
}

func specFlowDesignReviewBaselineDefinitions() []sliceDefinition {
	return []sliceDefinition{
		{
			ID:             "design_foundation",
			SliceType:      "local",
			ReviewQuestion: "Does the fixed design foundation block support a real human-serving governance design.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union(scope.FrameworkGuidelineFiles, scope.ProjectEntryFiles, scope.TemplateEntryFiles)
			},
		},
		{
			ID:             "lifecycle_and_gate_design",
			SliceType:      "local",
			ReviewQuestion: "Do lifecycle and gate-shape rules create necessary progress and useful downstream control.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union(scope.CommandFiles, scope.TemplateGovernanceFiles)
			},
		},
		{
			ID:             "human_operability_and_extension",
			SliceType:      "local",
			ReviewQuestion: "Can normal users and executors operate the entry and extension surfaces without excessive burden.",
			InputFiles: func(scope reviewscope.SpecFlowScope) []string {
				return union(scope.ProjectRegistryFiles, scope.ActiveProjectStandardFiles, scope.ProjectEntryFiles, scope.TemplateEntryFiles)
			},
		},
		{
			ID:             "foundation_to_lifecycle_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do design foundation rules and lifecycle gate rules converge into one usable design path.",
			DependsOn:      []string{"design_foundation", "lifecycle_and_gate_design"},
			InputFiles:     designDependencyFiles("design_foundation", "lifecycle_and_gate_design"),
		},
		{
			ID:             "foundation_to_operability_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do design foundation rules and human operability surfaces converge without hidden context.",
			DependsOn:      []string{"design_foundation", "human_operability_and_extension"},
			InputFiles:     designDependencyFiles("design_foundation", "human_operability_and_extension"),
		},
		{
			ID:             "lifecycle_to_operability_convergence",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Do lifecycle gates and human entry surfaces keep routine work proportionate.",
			DependsOn:      []string{"lifecycle_and_gate_design", "human_operability_and_extension"},
			InputFiles:     designDependencyFiles("lifecycle_and_gate_design", "human_operability_and_extension"),
		},
		{
			ID:             "scoring_and_pass_gate",
			SliceType:      "cross_convergence",
			ReviewQuestion: "Did the executor complete hard-blocker review, eight question scores, group averages, weighted score, and pass gate judgment.",
			DependsOn:      []string{"design_foundation", "lifecycle_and_gate_design", "human_operability_and_extension"},
			InputFiles:     designDependencyFiles("design_foundation", "lifecycle_and_gate_design", "human_operability_and_extension"),
		},
	}
}

func reviewDependencyFiles(dependencyIDs ...string) func(reviewscope.SpecFlowScope) []string {
	return func(scope reviewscope.SpecFlowScope) []string {
		byID := map[string]sliceDefinition{}
		for _, definition := range specFlowReviewBaselineDefinitions() {
			if definition.SliceType == "local" {
				byID[definition.ID] = definition
			}
		}
		sets := make([][]string, 0, len(dependencyIDs))
		for _, id := range dependencyIDs {
			if definition, ok := byID[id]; ok {
				sets = append(sets, definition.InputFiles(scope))
			}
		}
		return union(sets...)
	}
}

func designDependencyFiles(dependencyIDs ...string) func(reviewscope.SpecFlowScope) []string {
	return func(scope reviewscope.SpecFlowScope) []string {
		byID := map[string]sliceDefinition{}
		for _, definition := range specFlowDesignReviewBaselineDefinitions() {
			if definition.SliceType == "local" {
				byID[definition.ID] = definition
			}
		}
		sets := make([][]string, 0, len(dependencyIDs))
		for _, id := range dependencyIDs {
			if definition, ok := byID[id]; ok {
				sets = append(sets, definition.InputFiles(scope))
			}
		}
		return union(sets...)
	}
}

func buildBaselineSlices(repoRoot string, scope reviewscope.SpecFlowScope, definitions []sliceDefinition) ([]sliceEntry, error) {
	result := []sliceEntry{}
	for _, definition := range definitions {
		inputFiles := definition.InputFiles(scope)
		fingerprint, missing, err := computeFingerprint(repoRoot, inputFiles)
		if err != nil {
			return nil, err
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("baseline slice %s has missing input files: %s", definition.ID, strings.Join(missing, ", "))
		}
		result = append(result, sliceEntry{
			SliceID:          definition.ID,
			SliceOrigin:      "baseline",
			SliceType:        definition.SliceType,
			Status:           slicePending,
			ReviewQuestion:   definition.ReviewQuestion,
			WhyAdded:         "baseline_catalog",
			ParentSliceID:    "none",
			InputFiles:       inputFiles,
			InputFingerprint: fingerprint,
			DependsOn:        definition.DependsOn,
			FindingRefs:      "none",
			ResultSummary:    "pending",
			ExitCondition:    "agent records pass blocked stale or skipped according to review evidence",
			ResumeNextStep:   "review slice " + definition.ID,
		})
	}
	return result, nil
}

func parseFile(file string) (runState, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return runState{}, err
	}
	text := string(raw)
	state := runState{Fields: map[string]string{}}
	sections := splitSections(text)
	runSection, ok := sections["Run State"]
	if !ok {
		return state, fmt.Errorf("missing section: Run State")
	}
	fields, err := parseKeyValueTable(runSection)
	if err != nil {
		return state, err
	}
	state.Fields = fields
	baselineSection, ok := sections["Baseline Slices"]
	if !ok {
		return state, fmt.Errorf("missing section: Baseline Slices")
	}
	state.Baseline, err = parseSliceTable(baselineSection)
	if err != nil {
		return state, fmt.Errorf("baseline slice table: %w", err)
	}
	dynamicSection, ok := sections["Dynamic Slices"]
	if !ok {
		return state, fmt.Errorf("missing section: Dynamic Slices")
	}
	if strings.TrimSpace(dynamicSection) != "none" {
		state.Dynamic, err = parseSliceTable(dynamicSection)
		if err != nil {
			return state, fmt.Errorf("dynamic slice table: %w", err)
		}
	}
	if scoreSection, ok := sections["Score State"]; ok && strings.TrimSpace(scoreSection) != "none" {
		state.Score, err = parseScoreTable(scoreSection)
		if err != nil {
			return state, fmt.Errorf("score state table: %w", err)
		}
	}
	state.Findings = strings.TrimSpace(sections["Findings"])
	if state.Findings == "" {
		state.Findings = "none"
	}
	state.Resume = strings.TrimSpace(sections["Resume"])
	if state.Resume == "" {
		state.Resume = "none"
	}
	return state, nil
}

func splitSections(text string) map[string]string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	sections := map[string]string{}
	current := ""
	var body []string
	flush := func() {
		if current != "" {
			sections[current] = strings.TrimSpace(strings.Join(body, "\n"))
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			body = []string{}
			continue
		}
		if current != "" {
			body = append(body, line)
		}
	}
	flush()
	return sections
}

func parseKeyValueTable(section string) (map[string]string, error) {
	rows := parseMarkdownRows(section)
	result := map[string]string{}
	for _, row := range rows {
		if len(row) != 2 {
			continue
		}
		if row[0] == "field" {
			continue
		}
		result[row[0]] = row[1]
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("run field table is empty")
	}
	return result, nil
}

func parseScoreTable(section string) ([]scoreEntry, error) {
	rows := parseMarkdownRows(section)
	if len(rows) == 0 {
		return nil, fmt.Errorf("score table is empty")
	}
	header := rows[0]
	if len(header) != len(scoreColumns) {
		return nil, fmt.Errorf("score table header has %d columns, want %d", len(header), len(scoreColumns))
	}
	for i, column := range scoreColumns {
		if header[i] != column {
			return nil, fmt.Errorf("score table column %d is %q, want %q", i+1, header[i], column)
		}
	}
	result := []scoreEntry{}
	for _, row := range rows[1:] {
		if len(row) != len(scoreColumns) {
			return nil, fmt.Errorf("score row has %d columns, want %d", len(row), len(scoreColumns))
		}
		result = append(result, scoreEntry{
			QuestionID:     row[0],
			Status:         row[1],
			Score:          row[2],
			ScoreBasis:     row[3],
			Evidence:       row[4],
			FindingRefs:    row[5],
			ResultSummary:  row[6],
			ResumeNextStep: row[7],
		})
	}
	return result, nil
}

func parseSliceTable(section string) ([]sliceEntry, error) {
	rows := parseMarkdownRows(section)
	if len(rows) == 0 {
		return nil, fmt.Errorf("slice table is empty")
	}
	header := rows[0]
	if len(header) != len(sliceColumns) {
		return nil, fmt.Errorf("slice table header has %d columns, want %d", len(header), len(sliceColumns))
	}
	for i, column := range sliceColumns {
		if header[i] != column {
			return nil, fmt.Errorf("slice table column %d is %q, want %q", i+1, header[i], column)
		}
	}
	result := []sliceEntry{}
	for _, row := range rows[1:] {
		if len(row) != len(sliceColumns) {
			return nil, fmt.Errorf("slice row has %d columns, want %d", len(row), len(sliceColumns))
		}
		result = append(result, sliceEntry{
			SliceID:          row[0],
			SliceOrigin:      row[1],
			SliceType:        row[2],
			Status:           row[3],
			ReviewQuestion:   row[4],
			WhyAdded:         row[5],
			ParentSliceID:    row[6],
			InputFiles:       parseList(row[7]),
			InputFingerprint: row[8],
			DependsOn:        parseList(row[9]),
			FindingRefs:      row[10],
			ResultSummary:    row[11],
			ExitCondition:    row[12],
			ResumeNextStep:   row[13],
		})
	}
	return result, nil
}

func parseMarkdownRows(section string) [][]string {
	rows := [][]string{}
	for _, rawLine := range strings.Split(section, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		row := make([]string, 0, len(cells))
		separator := true
		for _, cell := range cells {
			trimmed := strings.TrimSpace(cell)
			row = append(row, trimmed)
			if strings.Trim(trimmed, "-: ") != "" {
				separator = false
			}
		}
		if separator {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func renderState(config flowConfig, state runState) string {
	var b strings.Builder
	b.WriteString("# " + config.Title + "\n\n")
	b.WriteString("## Run State\n\n")
	b.WriteString("| field | value |\n")
	b.WriteString("|---|---|\n")
	for _, field := range []string{"review_flow", "review_run_id", "scope_label", "status", "created_at", "last_updated_at", "active_slice", "baseline_slice_table", "dynamic_slice_table", "finding_refs", "blocked_reason", "resume_next_step"} {
		b.WriteString(fmt.Sprintf("| %s | %s |\n", field, cleanCell(state.Fields[field])))
	}
	b.WriteString("\n## Baseline Slices\n\n")
	renderSliceTable(&b, state.Baseline)
	b.WriteString("\n## Dynamic Slices\n\n")
	if len(state.Dynamic) == 0 {
		b.WriteString("none\n")
	} else {
		renderSliceTable(&b, state.Dynamic)
	}
	if config.UsesScoreState {
		b.WriteString("\n## Score State\n\n")
		renderScoreTable(&b, state.Score)
	}
	b.WriteString("\n## Findings\n\n")
	b.WriteString(defaultText(state.Findings))
	b.WriteString("\n\n## Resume\n\n")
	b.WriteString(defaultText(state.Resume))
	b.WriteString("\n")
	return b.String()
}

func renderSliceTable(b *strings.Builder, slices []sliceEntry) {
	b.WriteString("| " + strings.Join(sliceColumns, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat("---|", len(sliceColumns)) + "\n")
	for _, slice := range slices {
		values := []string{
			slice.SliceID,
			slice.SliceOrigin,
			slice.SliceType,
			slice.Status,
			slice.ReviewQuestion,
			slice.WhyAdded,
			slice.ParentSliceID,
			joinList(slice.InputFiles),
			slice.InputFingerprint,
			joinList(slice.DependsOn),
			slice.FindingRefs,
			slice.ResultSummary,
			slice.ExitCondition,
			slice.ResumeNextStep,
		}
		for i := range values {
			values[i] = cleanCell(values[i])
		}
		b.WriteString("| " + strings.Join(values, " | ") + " |\n")
	}
}

func renderScoreTable(b *strings.Builder, scores []scoreEntry) {
	b.WriteString("| " + strings.Join(scoreColumns, " | ") + " |\n")
	b.WriteString("|" + strings.Repeat("---|", len(scoreColumns)) + "\n")
	for _, score := range scores {
		values := []string{
			score.QuestionID,
			score.Status,
			score.Score,
			score.ScoreBasis,
			score.Evidence,
			score.FindingRefs,
			score.ResultSummary,
			score.ResumeNextStep,
		}
		for i := range values {
			values[i] = cleanCell(values[i])
		}
		b.WriteString("| " + strings.Join(values, " | ") + " |\n")
	}
}

func computeFingerprint(repoRoot string, inputFiles []string) (string, []string, error) {
	files := union(inputFiles)
	payload := strings.Builder{}
	missing := []string{}
	for _, relPath := range files {
		fullPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		content, err := os.ReadFile(fullPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", nil, err
			}
			missing = append(missing, relPath)
			continue
		}
		sum := sha256.Sum256([]byte(normalizeText(string(content))))
		payload.WriteString("file_ref: ")
		payload.WriteString(relPath)
		payload.WriteString("\nfile_sha256: ")
		payload.WriteString(hex.EncodeToString(sum[:]))
		payload.WriteString("\n\n")
	}
	if len(missing) > 0 {
		return "", missing, nil
	}
	sum := sha256.Sum256([]byte(payload.String()))
	return hex.EncodeToString(sum[:]), missing, nil
}

func normalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

func baselineIDSet(definitions []sliceDefinition) map[string]bool {
	result := map[string]bool{}
	for _, definition := range definitions {
		result[definition.ID] = true
	}
	return result
}

func isOpenRunStatus(status string) bool {
	switch status {
	case statusInProgress, statusBlockedOnFinding, statusReadyForFinal:
		return true
	default:
		return false
	}
}

func isSliceStatus(status string) bool {
	switch status {
	case slicePending, slicePassed, sliceBlocked, sliceStale, sliceSkippedNotInScope:
		return true
	default:
		return false
	}
}

func isScoreStatus(status string) bool {
	switch status {
	case "pending", "scored", "blocked", "stale":
		return true
	default:
		return false
	}
}

func isScoreValue(value string) bool {
	switch value {
	case "none", "0", "1", "2", "3", "4":
		return true
	default:
		return false
	}
}

func parseTimestamp(value string) (time.Time, error) {
	return time.Parse(timestampLayout, value)
}

func formatUTC(value time.Time) string {
	return value.UTC().Format(timestampLayout)
}

func union(sets ...[]string) []string {
	seen := map[string]bool{}
	for _, set := range sets {
		for _, item := range set {
			item = strings.TrimSpace(filepath.ToSlash(item))
			if item == "" || item == "none" {
				continue
			}
			seen[item] = true
		}
	}
	result := make([]string, 0, len(seen))
	for item := range seen {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func parseList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "none" {
		return nil
	}
	items := strings.Split(value, ";")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" && item != "none" {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}

func joinList(values []string) string {
	values = union(values)
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ";")
}

func cleanCell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "|", "/")
	if value == "" {
		return "none"
	}
	return value
}

func defaultText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}
	return value
}

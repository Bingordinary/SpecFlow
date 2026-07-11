package reviewrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitCreatesValidRunState(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)

	result, err := Init(repoRoot, FlowSpecFlowReview, now)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created run-state, got %+v", result)
	}
	content := mustRead(t, result.File)
	if !strings.Contains(content, "2026-04-26T10:30:00Z") {
		t.Fatalf("expected UTC timestamp in run-state:\n%s", content)
	}
	if !strings.Contains(content, "| review_layout | source_repo |") {
		t.Fatalf("expected installed_project review layout in run-state:\n%s", content)
	}
	if !strings.Contains(content, "| scope_inventory | baseline | local | pending |") {
		t.Fatalf("expected baseline slice table in run-state:\n%s", content)
	}
	state := mustParse(t, result.File)
	reviewEntrySlice := findSlice(t, state, "review_entry_policy")
	for _, input := range []string{
		"framework/spec_flow_review.md",
		"framework/spec_flow_design_review.md",
		"framework/governance/review.md",
		"framework/governance/review_scope.md",
		"framework/severity_policy.md",
	} {
		if !containsString(reviewEntrySlice.InputFiles, input) {
			t.Fatalf("expected review entry input %s, got %+v", input, reviewEntrySlice.InputFiles)
		}
	}
	routingSlice := findSlice(t, state, "concept_and_command_policy")
	if !containsString(routingSlice.InputFiles, "framework/concepts.md") {
		t.Fatalf("expected concepts.md in concept_and_command_policy slice, got %+v", routingSlice.InputFiles)
	}
	if !containsString(routingSlice.InputFiles, "framework/operations/migration.md") {
		t.Fatalf("expected migration policy in concept_and_command_policy slice, got %+v", routingSlice.InputFiles)
	}

	truthSlice := findSlice(t, state, "truth_and_implementation_gates")

	for _, input := range []string{
		"framework/core/object_model.md",
		"framework/core/repository_mapping.md",
		"framework/spec_writing_guide.md",
		"framework/concepts.md",
	} {
		if !containsString(truthSlice.InputFiles, input) {
			t.Fatalf("expected truth gate input %s, got %+v", input, truthSlice.InputFiles)
		}
	}
	operabilitySlice := findSlice(t, state, "agent_operability_local")
	for _, input := range []string{
		"framework/governance/review_scope.md",
		"framework/spec_flow_review.md",
	} {
		if !containsString(operabilitySlice.InputFiles, input) {
			t.Fatalf("expected agent operability input %s, got %+v", input, operabilitySlice.InputFiles)
		}
	}
	processSlice := findSlice(t, state, "process_and_impact_state")
	if !containsString(processSlice.InputFiles, "framework/governance/impact_sync.md") {
		t.Fatalf("expected process state input governance/impact_sync.md, got %+v", processSlice.InputFiles)
	}

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, result.File, now)
	if !validation.Valid {
		t.Fatalf("expected valid run-state, got diagnostics: %+v", validation.Diagnostics)
	}
}

func TestInitCreatesSourceRepoRunState(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)

	result, err := Init(repoRoot, FlowSpecFlowReview, now)
	if err != nil {
		t.Fatalf("Init source review: %v", err)
	}
	content := mustRead(t, result.File)
	if !strings.Contains(content, "| review_layout | source_repo |") {
		t.Fatalf("expected source_repo review layout in run-state:\n%s", content)
	}
	state := mustParse(t, result.File)
	routingSlice := findSlice(t, state, "concept_and_command_policy")
	if !containsString(routingSlice.InputFiles, "framework/concepts.md") {
		t.Fatalf("expected source concepts.md input, got %+v", routingSlice.InputFiles)
	}
	if containsString(routingSlice.InputFiles, "specflow/framework/concepts.md") {
		t.Fatalf("source run-state must not use installed framework path, got %+v", routingSlice.InputFiles)
	}
	reviewEntrySlice := findSlice(t, state, "review_entry_policy")
	for _, input := range []string{
		"framework/spec_flow_review.md",
		"framework/spec_flow_design_review.md",
		"framework/governance/review.md",
		"framework/governance/review_scope.md",
		"framework/severity_policy.md",
	} {
		if !containsString(reviewEntrySlice.InputFiles, input) {
			t.Fatalf("expected source review entry input %s, got %+v", input, reviewEntrySlice.InputFiles)
		}
	}
	compatSlice := findSlice(t, state, "project_instance_contract_compatibility")
	if !containsString(compatSlice.InputFiles, "templates/docs/specs/_status.md") {
		t.Fatalf("expected source compatibility to use template status, got %+v", compatSlice.InputFiles)
	}
	if containsString(compatSlice.InputFiles, "docs/specs/_status.md") {
		t.Fatalf("source compatibility must not require project status, got %+v", compatSlice.InputFiles)
	}
	for _, input := range []string{
		"framework/core/object_model.md",
		"framework/core/repository_mapping.md",
		"framework/spec_writing_guide.md",
	} {
		if !containsString(compatSlice.InputFiles, input) {
			t.Fatalf("expected source compatibility contract input %s, got %+v", input, compatSlice.InputFiles)
		}
	}
	operabilitySlice := findSlice(t, state, "agent_operability_local")
	for _, input := range []string{
		"framework/governance/review_scope.md",
		"framework/spec_flow_review.md",
	} {
		if !containsString(operabilitySlice.InputFiles, input) {
			t.Fatalf("expected source agent operability input %s, got %+v", input, operabilitySlice.InputFiles)
		}
	}

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, result.File, now)
	if !validation.Valid {
		t.Fatalf("expected valid source run-state, got diagnostics: %+v", validation.Diagnostics)
	}
}

func TestRefreshSourceRunStateUsesRecordedLayout(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowReview, now)
	if err != nil {
		t.Fatalf("Init source review: %v", err)
	}
	state := mustParse(t, result.File)
	setSliceStatus(t, &state, "concept_and_command_policy", slicePassed)
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/concepts.md"), "# source concepts changed\n")

	refresh, err := Refresh(repoRoot, FlowSpecFlowReview, result.File, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh source review: %v", err)
	}
	if !containsString(refresh.StaleSlices, "concept_and_command_policy") {
		t.Fatalf("expected source routing slice stale, got %+v", refresh.StaleSlices)
	}
}





func TestInitIncludesSupportingLayerConvergenceSlice(t *testing.T) {
	_, file, _ := createInitializedRun(t)
	state := mustParse(t, file)
	slice := findSlice(t, state, "supporting_layer_convergence")

	if slice.SliceType != "cross_convergence" {
		t.Fatalf("expected supporting_layer_convergence to be cross_convergence, got %s", slice.SliceType)
	}
	for _, dependency := range []string{
		"concept_and_command_policy",
		"truth_and_implementation_gates",
		"process_and_impact_state",
		"project_instance_contract_compatibility",
		"tooling_execution",
	} {
		if !containsString(slice.DependsOn, dependency) {
			t.Fatalf("expected supporting_layer_convergence dependency %s, got %+v", dependency, slice.DependsOn)
		}
	}
	for _, input := range []string{
		"framework/concepts.md",
		"framework/governance/impact_sync.md",
		"framework/core/object_model.md",
	} {
		if !containsString(slice.InputFiles, input) {
			t.Fatalf("expected supporting_layer_convergence input %s, got %+v", input, slice.InputFiles)
		}
	}
	projectConvergence := findSlice(t, state, "project_instance_to_framework_convergence")
	if !containsString(projectConvergence.DependsOn, "supporting_layer_convergence") {
		t.Fatalf("expected project/framework convergence to depend on supporting_layer_convergence, got %+v", projectConvergence.DependsOn)
	}
}

func TestInitCreatesValidDesignReviewRunState(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)

	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created run-state, got %+v", result)
	}
	if !strings.Contains(filepath.ToSlash(result.File), "docs/specs/_governance_review/spec_flow_design_review.md") {
		t.Fatalf("expected design review run-state path, got %s", result.File)
	}
	content := mustRead(t, result.File)
	for _, sliceID := range []string{
		"design_foundation",
		"process_and_gate_design",
		"executor_operability_and_extension",
		"foundation_to_process_convergence",
		"foundation_to_operability_convergence",
		"process_to_operability_convergence",
		"scoring_and_pass_gate",
	} {
		if !strings.Contains(content, "| "+sliceID+" | baseline |") {
			t.Fatalf("expected design baseline slice %s in run-state:\n%s", sliceID, content)
		}
	}
	if !strings.Contains(content, "## Score State") || !strings.Contains(content, "| q8 | pending | none |") {
		t.Fatalf("expected q1-q8 score state in run-state:\n%s", content)
	}
	state := mustParse(t, result.File)
	designFoundation := findSlice(t, state, "design_foundation")
	if !containsString(designFoundation.InputFiles, "framework/governance/review.md") {
		t.Fatalf("expected governance review in design foundation input files, got %+v", designFoundation.InputFiles)
	}
	if !containsString(designFoundation.InputFiles, "framework/governance/review_scope.md") {
		t.Fatalf("expected review scope policy in design foundation input files, got %+v", designFoundation.InputFiles)
	}
	if !containsString(designFoundation.InputFiles, "framework/governance/rule_system.md") {
		t.Fatalf("expected rule system policy in design foundation input files, got %+v", designFoundation.InputFiles)
	}
	if !containsString(designFoundation.InputFiles, "framework/operations/migration.md") {
		t.Fatalf("expected migration policy in design foundation input files, got %+v", designFoundation.InputFiles)
	}
	if !containsString(designFoundation.InputFiles, "framework/concepts.md") {
		t.Fatalf("expected concepts.md in design foundation input files, got %+v", designFoundation.InputFiles)
	}

	validation := ValidateFile(repoRoot, FlowSpecFlowDesignReview, result.File, now)
	if !validation.Valid {
		t.Fatalf("expected valid design run-state, got diagnostics: %+v", validation.Diagnostics)
	}
}

func TestRefreshMarksChangedPassedDesignSliceStale(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	setSliceStatus(t, &state, "design_foundation", slicePassed)
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/governance/review.md"), "# design changed\n")

	refresh, err := Refresh(repoRoot, FlowSpecFlowDesignReview, result.File, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh design review: %v", err)
	}
	if !containsString(refresh.StaleSlices, "design_foundation") {
		t.Fatalf("expected design_foundation stale, got %+v", refresh.StaleSlices)
	}
	refreshed := mustParse(t, result.File)
	if got := findSlice(t, refreshed, "design_foundation").Status; got != sliceStale {
		t.Fatalf("expected design_foundation stale status, got %s", got)
	}
}

func TestRefreshMarksScoredDesignRowsStaleWhenInputsChange(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	setSliceStatus(t, &state, "design_foundation", slicePassed)
	state.Score[0].Status = "scored"
	state.Score[0].Score = "3"
	state.Score[0].ScoreBasis = "reviewed"
	state.Score[0].Evidence = "framework/governance/review.md"
	state.Score[0].ResultSummary = "scored"
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/governance/review.md"), "# design changed\n")

	if _, err := Refresh(repoRoot, FlowSpecFlowDesignReview, result.File, now.Add(time.Hour)); err != nil {
		t.Fatalf("Refresh design review: %v", err)
	}
	refreshed := mustParse(t, result.File)
	if got := refreshed.Score[0].Status; got != "stale" {
		t.Fatalf("expected q1 score state stale, got %s", got)
	}
	if got := refreshed.Score[0].Score; got != "3" {
		t.Fatalf("expected stale score row to preserve score value, got %s", got)
	}
}

func TestRefreshMarksScoredDesignRowsStaleWhenSliceAlreadyStale(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	setSliceStatus(t, &state, "design_foundation", sliceStale)
	state.Score[0].Status = "scored"
	state.Score[0].Score = "3"
	state.Score[0].ScoreBasis = "reviewed"
	state.Score[0].Evidence = "framework/governance/review.md"
	state.Score[0].ResultSummary = "scored"
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))

	if _, err := Refresh(repoRoot, FlowSpecFlowDesignReview, result.File, now.Add(time.Hour)); err != nil {
		t.Fatalf("Refresh design review: %v", err)
	}
	refreshed := mustParse(t, result.File)
	if got := refreshed.Score[0].Status; got != "stale" {
		t.Fatalf("expected existing stale slice to stale q1 score state, got %s", got)
	}
}

func TestTouchRejectsWrongFlowWithoutRewritingRunState(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	before := mustRead(t, result.File)

	_, err = Touch(repoRoot, FlowSpecFlowReview, result.File, now.Add(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "review_flow must be spec_flow_review") {
		t.Fatalf("expected wrong-flow validation error, got %v", err)
	}
	after := mustRead(t, result.File)
	if after != before {
		t.Fatalf("wrong-flow touch must not rewrite run-state")
	}
}

func TestValidateRejectsMissingDesignScoreState(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	state.Score = state.Score[:7]
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowDesignReview, result.File, now)
	if validation.Valid || !containsDiagnostic(validation.Diagnostics, "score state must contain 8 rows") {
		t.Fatalf("expected score-state diagnostic, got %+v", validation.Diagnostics)
	}
}

func TestValidateRejectsInvalidRunStatus(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = "done"
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, file, now)
	if validation.Valid || !containsDiagnostic(validation.Diagnostics, "invalid run status") {
		t.Fatalf("expected invalid run status diagnostic, got %+v", validation.Diagnostics)
	}
}

func TestValidateAcceptsClosedRunStateShape(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = statusClosedPass
	state.Fields["active_slice"] = "none"
	state.Fields["resume_next_step"] = "none"
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, file, now)
	if !validation.Valid {
		t.Fatalf("expected closed run-state shape to validate, got diagnostics: %+v", validation.Diagnostics)
	}
}

func TestValidateAcceptsClosedDesignRunStateShape(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	state.Fields["status"] = statusClosedBlocked
	state.Fields["active_slice"] = "none"
	state.Fields["resume_next_step"] = "none"
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowDesignReview, result.File, now)
	if !validation.Valid {
		t.Fatalf("expected closed design run-state shape to validate, got diagnostics: %+v", validation.Diagnostics)
	}
}

func TestValidateAcceptsClosedDesignPassWithOptimizationRunStateShape(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	state.Fields["status"] = statusClosedPassWithOptimization
	state.Fields["active_slice"] = "none"
	state.Fields["resume_next_step"] = "none"
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowDesignReview, result.File, now)
	if !validation.Valid {
		t.Fatalf("expected pass-with-optimization run-state shape to validate, got diagnostics: %+v", validation.Diagnostics)
	}
}

func TestValidateRejectsSpecFlowReviewPassWithOptimizationRunStateShape(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = statusClosedPassWithOptimization
	state.Fields["active_slice"] = "none"
	state.Fields["resume_next_step"] = "none"
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, file, now)
	if validation.Valid || !containsDiagnostic(validation.Diagnostics, "invalid run status") {
		t.Fatalf("expected spec_flow_review to reject pass-with-optimization status, got %+v", validation.Diagnostics)
	}
}

func TestRefreshRejectsClosedRunState(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = statusClosedPass
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	_, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "closed run-state files cannot be reused") {
		t.Fatalf("expected closed run-state refresh rejection, got %v", err)
	}
}

func TestTouchAcceptsClosedRunStateShape(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = statusClosedPass
	state.Fields["active_slice"] = "none"
	state.Fields["resume_next_step"] = "none"
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	result, err := Touch(repoRoot, FlowSpecFlowReview, file, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected touch to update closed run-state timestamp, got %v", err)
	}
	if result.LastUpdatedAtUTC != "2026-04-26T10:31:00Z" {
		t.Fatalf("unexpected touch timestamp: %+v", result)
	}
}

func TestValidateRejectsInvalidSliceStatus(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Baseline[0].Status = "done"
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, file, now)
	if validation.Valid || !containsDiagnostic(validation.Diagnostics, "invalid slice status") {
		t.Fatalf("expected invalid slice status diagnostic, got %+v", validation.Diagnostics)
	}
}

func TestValidateRejectsMissingRequiredSliceField(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Baseline[0].ReviewQuestion = "none"
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, file, now)
	if validation.Valid || !containsDiagnostic(validation.Diagnostics, "review_question must not be none") {
		t.Fatalf("expected missing review_question diagnostic, got %+v", validation.Diagnostics)
	}
}

func TestValidateRejectsBrokenDynamicParent(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Dynamic = append(state.Dynamic, sliceEntry{
		SliceID:          "dynamic_missing_parent",
		SliceOrigin:      "dynamic",
		SliceType:        "local",
		Status:           slicePending,
		ReviewQuestion:   "Does the newly discovered risk close correctly.",
		WhyAdded:         "found during review",
		ParentSliceID:    "missing_parent",
		InputFiles:       []string{"framework/spec_flow_review.md"},
		InputFingerprint: "abc",
		FindingRefs:      "none",
		ResultSummary:    "pending",
		ExitCondition:    "agent records the result",
		ResumeNextStep:   "review slice dynamic_missing_parent",
	})
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, file, now)
	if validation.Valid || !containsDiagnostic(validation.Diagnostics, "parent_slice_id") {
		t.Fatalf("expected broken parent diagnostic, got %+v", validation.Diagnostics)
	}
}

func TestInitDoesNotAutoReuseRunOlderThanTwoHours(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["last_updated_at"] = formatUTC(now.Add(-3 * time.Hour))
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	_, err := Init(repoRoot, FlowSpecFlowReview, now)
	if err == nil || !strings.Contains(err.Error(), "requires manual reuse decision") {
		t.Fatalf("expected manual reuse decision error, got %v", err)
	}
}

func TestInitRecommendsRestartForSpecFlowReviewRunOlderThanTwentyFourHours(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["last_updated_at"] = formatUTC(now.Add(-25 * time.Hour))
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	_, err := Init(repoRoot, FlowSpecFlowReview, now)
	if err == nil {
		t.Fatalf("expected manual reuse decision error")
	}
	if !strings.Contains(err.Error(), "requires manual reuse decision") {
		t.Fatalf("expected manual reuse decision error, got %v", err)
	}
	if !strings.Contains(err.Error(), "recommendation=delete old run-state and start a new run") {
		t.Fatalf("expected restart recommendation, got %v", err)
	}
}

func TestInitDoesNotRecommendRestartForDesignReviewRunOlderThanTwentyFourHours(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	state.Fields["last_updated_at"] = formatUTC(now.Add(-25 * time.Hour))
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))

	_, err = Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err == nil {
		t.Fatalf("expected manual reuse decision error")
	}
	if !strings.Contains(err.Error(), "requires manual reuse decision") {
		t.Fatalf("expected manual reuse decision error, got %v", err)
	}
	if strings.Contains(err.Error(), "recommendation=delete old run-state and start a new run") {
		t.Fatalf("did not expect design review restart recommendation, got %v", err)
	}
}

func TestInitDeletesExpiredRunAndCreatesNewRun(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["last_updated_at"] = formatUTC(now.Add(-8 * 24 * time.Hour))
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	result, err := Init(repoRoot, FlowSpecFlowReview, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !result.Created || len(result.DeletedFiles) != 1 || result.DeletedFiles[0].File != file || result.DeletedFiles[0].Reason != "expired_over_7_days" {
		t.Fatalf("expected expired file deletion and new file creation, got %+v", result)
	}
	recreated := mustParse(t, file)
	if recreated.Fields["review_run_id"] != "20260426-103100-default_governance_baseline" {
		t.Fatalf("expected new run id in fixed file, got %s", recreated.Fields["review_run_id"])
	}
}

func TestInitDeletesInvalidRunAndCreatesNewRun(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = "done"
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	result, err := Init(repoRoot, FlowSpecFlowReview, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !result.Created || len(result.DeletedFiles) != 1 || result.DeletedFiles[0].File != file || result.DeletedFiles[0].Reason != "invalid_run_state" {
		t.Fatalf("expected invalid file deletion and new file creation, got %+v", result)
	}
	recreated := mustParse(t, file)
	if recreated.Fields["review_run_id"] != "20260426-103100-default_governance_baseline" {
		t.Fatalf("expected new run id in fixed file, got %s", recreated.Fields["review_run_id"])
	}
}

func TestInitDeletesClosedRunAndCreatesNewRun(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = statusClosedPass
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	result, err := Init(repoRoot, FlowSpecFlowReview, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !result.Created || result.File != file || len(result.DeletedFiles) != 1 || result.DeletedFiles[0].File != file || result.DeletedFiles[0].Reason != "closed_run_state" {
		t.Fatalf("expected closed file deletion and fixed path recreation, got %+v", result)
	}
	recreated := mustParse(t, file)
	if recreated.Fields["review_run_id"] != "20260426-103100-default_governance_baseline" {
		t.Fatalf("expected new run id in fixed file, got %s", recreated.Fields["review_run_id"])
	}
}

func TestInitDeletesClosedPassWithOptimizationRunAndCreatesNewRun(t *testing.T) {
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowDesignReview, now)
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	state := mustParse(t, result.File)
	state.Fields["status"] = statusClosedPassWithOptimization
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))

	next, err := Init(repoRoot, FlowSpecFlowDesignReview, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Init design review: %v", err)
	}
	if !next.Created || next.File != result.File || len(next.DeletedFiles) != 1 || next.DeletedFiles[0].File != result.File || next.DeletedFiles[0].Reason != "closed_run_state" {
		t.Fatalf("expected pass-with-optimization file deletion and fixed path recreation, got %+v", next)
	}
	recreated := mustParse(t, result.File)
	if recreated.Fields["review_run_id"] != "20260426-103100-default_design_baseline" {
		t.Fatalf("expected new design run id in fixed file, got %s", recreated.Fields["review_run_id"])
	}
}

func TestInitTreatsSpecFlowReviewPassWithOptimizationRunAsInvalid(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	state.Fields["status"] = statusClosedPassWithOptimization
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	result, err := Init(repoRoot, FlowSpecFlowReview, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !result.Created || len(result.DeletedFiles) != 1 || result.DeletedFiles[0].Reason != "invalid_run_state" {
		t.Fatalf("expected pass-with-optimization spec_flow_review file to be treated as invalid, got %+v", result)
	}
}

func TestRefreshMarksChangedPassedSliceStale(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "review_entry_policy", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/severity_policy.md"), "# severity changed\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "review_entry_policy") {
		t.Fatalf("expected review_entry_policy stale, got %+v", result.StaleSlices)
	}
	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "review_entry_policy").Status; got != sliceStale {
		t.Fatalf("expected stale status, got %s", got)
	}
}

func TestRefreshKeepsPassedSliceFreshWhenInputSetUnchanged(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "review_entry_policy", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if containsString(result.ChangedSlices, "review_entry_policy") {
		t.Fatalf("unchanged input set must not be reported changed, got %+v", result.ChangedSlices)
	}
	if containsString(result.StaleSlices, "review_entry_policy") {
		t.Fatalf("unchanged input set must not stale passed slice, got %+v", result.StaleSlices)
	}
	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "review_entry_policy").Status; got != slicePassed {
		t.Fatalf("expected passed status to remain fresh, got %s", got)
	}
}

func TestRefreshPropagatesStaleToCrossConvergenceSlice(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "review_entry_policy", slicePassed)
	setSliceStatus(t, &state, "command_to_process_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/severity_policy.md"), "# severity changed\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "command_to_process_convergence") {
		t.Fatalf("expected cross slice stale, got %+v", result.StaleSlices)
	}
	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "command_to_process_convergence").Status; got != sliceStale {
		t.Fatalf("expected cross stale status, got %s", got)
	}
}





func TestRefreshMarksTruthGateStaleWhenImplementationGateChanges(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "truth_and_implementation_gates", slicePassed)
	setSliceStatus(t, &state, "truth_to_implementation_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/core/repository_mapping.md"), "# repository mapping changed\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "truth_and_implementation_gates") {
		t.Fatalf("expected truth_and_implementation_gates stale after implementation gate change, got %+v", result.StaleSlices)
	}
	if !containsString(result.StaleSlices, "truth_to_implementation_convergence") {
		t.Fatalf("expected truth_to_implementation_convergence stale after implementation gate change, got %+v", result.StaleSlices)
	}

	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "truth_and_implementation_gates").Status; got != sliceStale {
		t.Fatalf("expected truth gate stale status, got %s", got)
	}
	if got := findSlice(t, refreshed, "truth_to_implementation_convergence").Status; got != sliceStale {
		t.Fatalf("expected truth convergence stale status, got %s", got)
	}
}

func TestRefreshPropagatesStaleThroughDynamicCrossChain(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "review_entry_policy", slicePassed)
	fingerprint, missing, err := computeFingerprint(repoRoot, []string{"framework/spec_flow_review.md"})
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("unexpected missing input: %+v", missing)
	}
	state.Dynamic = append(state.Dynamic,
		sliceEntry{
			SliceID:          "dynamic_chain_a",
			SliceOrigin:      "dynamic",
			SliceType:        "cross_convergence",
			Status:           slicePassed,
			ReviewQuestion:   "Does dynamic chain A converge.",
			WhyAdded:         "found during review",
			ParentSliceID:    "dynamic_chain_b",
			InputFiles:       []string{"framework/spec_flow_review.md"},
			InputFingerprint: fingerprint,
			DependsOn:        []string{"dynamic_chain_b"},
			FindingRefs:      "none",
			ResultSummary:    "passed",
			ExitCondition:    "agent records the result",
			ResumeNextStep:   "review slice dynamic_chain_a",
		},
		sliceEntry{
			SliceID:          "dynamic_chain_b",
			SliceOrigin:      "dynamic",
			SliceType:        "cross_convergence",
			Status:           slicePassed,
			ReviewQuestion:   "Does dynamic chain B converge.",
			WhyAdded:         "found during review",
			ParentSliceID:    "review_entry_policy",
			InputFiles:       []string{"framework/spec_flow_review.md"},
			InputFingerprint: fingerprint,
			DependsOn:        []string{"review_entry_policy"},
			FindingRefs:      "none",
			ResultSummary:    "passed",
			ExitCondition:    "agent records the result",
			ResumeNextStep:   "review slice dynamic_chain_b",
		},
	)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/severity_policy.md"), "# severity changed\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "dynamic_chain_a") || !containsString(result.StaleSlices, "dynamic_chain_b") {
		t.Fatalf("expected dynamic chain stale, got %+v", result.StaleSlices)
	}
	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "dynamic_chain_a").Status; got != sliceStale {
		t.Fatalf("expected dynamic_chain_a stale, got %s", got)
	}
}

func TestRefreshMarksMissingPassedInputStale(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	missingRel := "docs/specs/_governance_review/temp_missing_input.md"
	mustWrite(t, filepath.Join(repoRoot, filepath.FromSlash(missingRel)), "# temp input\n")
	originalFingerprint, missing, err := computeFingerprint(repoRoot, []string{missingRel})
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("unexpected missing input before refresh setup: %+v", missing)
	}
	state := mustParse(t, file)
	state.Dynamic = append(state.Dynamic, sliceEntry{
		SliceID:          "dynamic_missing_input",
		SliceOrigin:      "dynamic",
		SliceType:        "local",
		Status:           slicePassed,
		ReviewQuestion:   "Does a dynamic missing input stale correctly.",
		WhyAdded:         "test missing input",
		ParentSliceID:    "review_entry_policy",
		InputFiles:       []string{missingRel},
		InputFingerprint: originalFingerprint,
		DependsOn:        nil,
		FindingRefs:      "none",
		ResultSummary:    "passed",
		ExitCondition:    "agent records the result",
		ResumeNextStep:   "review slice dynamic_missing_input",
	})
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	if err := os.Remove(filepath.Join(repoRoot, filepath.FromSlash(missingRel))); err != nil {
		t.Fatalf("remove input: %v", err)
	}

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "dynamic_missing_input") {
		t.Fatalf("expected missing input to stale passed slice, got %+v", result.StaleSlices)
	}
	if len(result.MissingInputs) == 0 {
		t.Fatalf("expected missing input diagnostic")
	}
	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "dynamic_missing_input").InputFingerprint; got != originalFingerprint {
		t.Fatalf("expected missing input to preserve old fingerprint, got %s want %s", got, originalFingerprint)
	}
	if strings.Contains(mustRead(t, file), "file_sha256: missing") {
		t.Fatalf("run-state must not contain undefined missing fingerprint sentinel")
	}
}

func TestInitIncludesProjectInstanceCompatibilitySlice(t *testing.T) {
	repoRoot, file, _ := createInitializedRun(t)
	state := mustParse(t, file)
	slice := findSlice(t, state, "project_instance_contract_compatibility")
	if slice.SliceType != "local" {
		t.Fatalf("expected project instance compatibility to be local, got %s", slice.SliceType)
	}
	if !containsString(slice.InputFiles, "templates/docs/specs/_status.md") {
		t.Fatalf("expected project status input, got %+v", slice.InputFiles)
	}
	for _, input := range []string{
		"framework/core/object_model.md",
		"framework/core/repository_mapping.md",
		"framework/spec_writing_guide.md",
	} {
		if !containsString(slice.InputFiles, input) {
			t.Fatalf("expected project compatibility contract input %s, got %+v", input, slice.InputFiles)
		}
	}
	if !containsString(slice.InputFiles, "templates/docs/specs/repository_mapping.md") {
		t.Fatalf("expected repository mapping input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "templates/docs/specs/rules/stable/s_g_rule_repository_baseline.md") {
		t.Fatalf("expected global rules input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "templates/docs/specs/repository_mapping.md") {
		t.Fatalf("expected repository mapping template input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "templates/docs/specs/rules/stable/s_g_rule_repository_baseline.md") {
		t.Fatalf("expected global rule template input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "templates/docs/specs/units/candidate/c_unit_demo.md") {
		t.Fatalf("expected current project truth file input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "framework/operations/migration.md") {
		t.Fatalf("expected migration policy input for project-instance migration compatibility, got %+v", slice.InputFiles)
	}
	if containsString(slice.InputFiles, "templates/docs/specs/_governance_review/spec_flow_review.md") {
		t.Fatalf("expected active review run state outside compatibility fingerprint, got %+v", slice.InputFiles)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "templates/docs/specs/units/candidate/c_unit_demo.md")); err != nil {
		t.Fatalf("expected fixture project truth file: %v", err)
	}
}

func TestInitIncludesToolingScriptAndReaderRuntimeInToolingSlices(t *testing.T) {
	_, file, _ := createInitializedRun(t)
	state := mustParse(t, file)

	toolingSlice := findSlice(t, state, "tooling_execution")
	if !containsString(toolingSlice.InputFiles, "tooling/scripts/tooling_fingerprint.sh") {
		t.Fatalf("expected shell fingerprint script in tooling execution input files, got %+v", toolingSlice.InputFiles)
	}
	if !containsString(toolingSlice.InputFiles, "tooling/scripts/tooling_fingerprint.ps1") {
		t.Fatalf("expected PowerShell fingerprint script in tooling execution input files, got %+v", toolingSlice.InputFiles)
	}
	if !containsString(toolingSlice.InputFiles, "tooling/scripts/build_release.sh") {
		t.Fatalf("expected build release script in tooling execution input files, got %+v", toolingSlice.InputFiles)
	}
	for _, relPath := range currentReviewToolingScriptFiles() {
		if !containsString(toolingSlice.InputFiles, relPath) {
			t.Fatalf("expected tooling script in tooling execution input files: %s, got %+v", relPath, toolingSlice.InputFiles)
		}
	}
	if !containsString(toolingSlice.InputFiles, "tooling/reader/web/app.js") {
		t.Fatalf("expected reader app.js in tooling execution input files, got %+v", toolingSlice.InputFiles)
	}

	convergenceSlice := findSlice(t, state, "project_instance_to_framework_convergence")
	if !containsString(convergenceSlice.InputFiles, "templates/docs/specs/_status.md") {
		t.Fatalf("expected project status in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}
	if !containsString(convergenceSlice.InputFiles, "tooling/reader/web/app.js") {
		t.Fatalf("expected reader app.js in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}
	if !containsString(convergenceSlice.InputFiles, "tooling/scripts/tooling_fingerprint.sh") {
		t.Fatalf("expected shell fingerprint script in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}
	if !containsString(convergenceSlice.InputFiles, "tooling/scripts/build_release.sh") {
		t.Fatalf("expected build release script in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}
	if !containsString(convergenceSlice.InputFiles, "tooling/scripts/pull_with_release.sh") {
		t.Fatalf("expected pull release script in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}

	toolingConvergenceSlice := findSlice(t, state, "tooling_to_rule_convergence")
	if !containsString(toolingConvergenceSlice.InputFiles, "tooling/scripts/build_release.sh") {
		t.Fatalf("expected build release script in tooling/rule convergence input files, got %+v", toolingConvergenceSlice.InputFiles)
	}
	if !containsString(toolingConvergenceSlice.InputFiles, "tooling/scripts/push_with_release.ps1") {
		t.Fatalf("expected push release script in tooling/rule convergence input files, got %+v", toolingConvergenceSlice.InputFiles)
	}
}

func TestRefreshMarksToolingScriptSlicesStale(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "tooling_execution", slicePassed)
	setSliceStatus(t, &state, "project_instance_to_framework_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "tooling/scripts/build_release.sh"), "#!/usr/bin/env bash\necho changed\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "tooling_execution") {
		t.Fatalf("expected tooling_execution stale after tooling script change, got %+v", result.StaleSlices)
	}
	if !containsString(result.StaleSlices, "project_instance_to_framework_convergence") {
		t.Fatalf("expected project_instance_to_framework_convergence stale after tooling script change, got %+v", result.StaleSlices)
	}

	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "tooling_execution").Status; got != sliceStale {
		t.Fatalf("expected tooling_execution stale, got %s", got)
	}
	if got := findSlice(t, refreshed, "project_instance_to_framework_convergence").Status; got != sliceStale {
		t.Fatalf("expected project_instance_to_framework_convergence stale, got %s", got)
	}
}

func TestRefreshMarksSupportingLayerAndDependentConvergenceStale(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "supporting_layer_convergence", slicePassed)
	setSliceStatus(t, &state, "project_instance_to_framework_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "framework/governance/impact_sync.md"), "# impact_sync changed\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "supporting_layer_convergence") {
		t.Fatalf("expected supporting_layer_convergence stale after command change, got %+v", result.StaleSlices)
	}
	if !containsString(result.StaleSlices, "project_instance_to_framework_convergence") {
		t.Fatalf("expected dependent project/framework convergence stale after supporting truth change, got %+v", result.StaleSlices)
	}

	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "supporting_layer_convergence").Status; got != sliceStale {
		t.Fatalf("expected supporting_layer_convergence stale, got %s", got)
	}
	if got := findSlice(t, refreshed, "project_instance_to_framework_convergence").Status; got != sliceStale {
		t.Fatalf("expected project/framework convergence stale, got %s", got)
	}
}

func TestRefreshUpdatesBaselineInputFilesWhenScopeDefinitionChanges(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	stripToolingScriptInputs(t, repoRoot, &state)
	setSliceStatus(t, &state, "tooling_execution", slicePassed)
	setSliceStatus(t, &state, "tooling_to_rule_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.ChangedSlices, "scope_inventory") {
		t.Fatalf("expected scope_inventory changed after baseline input update, got %+v", result.ChangedSlices)
	}
	if !containsString(result.ChangedSlices, "tooling_execution") {
		t.Fatalf("expected tooling_execution changed after baseline input update, got %+v", result.ChangedSlices)
	}

	refreshed := mustParse(t, file)
	scopeSlice := findSlice(t, refreshed, "scope_inventory")
	if !containsString(scopeSlice.InputFiles, "tooling/scripts/tooling_fingerprint.sh") {
		t.Fatalf("expected refreshed scope_inventory to include shell fingerprint script, got %+v", scopeSlice.InputFiles)
	}
	if !containsString(scopeSlice.InputFiles, "tooling/scripts/build_release.sh") {
		t.Fatalf("expected refreshed scope_inventory to include build release script, got %+v", scopeSlice.InputFiles)
	}
	if !containsString(scopeSlice.InputFiles, "tooling/scripts/install.sh") {
		t.Fatalf("expected refreshed scope_inventory to include install script, got %+v", scopeSlice.InputFiles)
	}
	toolingSlice := findSlice(t, refreshed, "tooling_execution")
	if !containsString(toolingSlice.InputFiles, "tooling/scripts/tooling_fingerprint.ps1") {
		t.Fatalf("expected refreshed tooling_execution to include PowerShell fingerprint script, got %+v", toolingSlice.InputFiles)
	}
	if !containsString(toolingSlice.InputFiles, "tooling/scripts/build_release.sh") {
		t.Fatalf("expected refreshed tooling_execution to include build release script, got %+v", toolingSlice.InputFiles)
	}
	if !containsString(toolingSlice.InputFiles, "tooling/scripts/pull_with_release.ps1") {
		t.Fatalf("expected refreshed tooling_execution to include pull release script, got %+v", toolingSlice.InputFiles)
	}
	if got := toolingSlice.Status; got != sliceStale {
		t.Fatalf("expected tooling_execution stale after input list update, got %s", got)
	}
	if got := findSlice(t, refreshed, "tooling_to_rule_convergence").Status; got != sliceStale {
		t.Fatalf("expected tooling_to_rule_convergence stale after input list update, got %s", got)
	}
}

func TestValidateRejectsRunStateMissingSupportingLayerSlice(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	filtered := make([]sliceEntry, 0, len(state.Baseline)-1)
	for _, slice := range state.Baseline {
		if slice.SliceID == "supporting_layer_convergence" {
			continue
		}
		filtered = append(filtered, slice)
	}
	state.Baseline = filtered
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, file, now)
	if validation.Valid || !containsDiagnostic(validation.Diagnostics, "missing baseline slice: supporting_layer_convergence") {
		t.Fatalf("expected missing supporting_layer_convergence baseline diagnostic, got %+v", validation.Diagnostics)
	}
}

func TestRefreshMarksReaderRuntimeSlicesStale(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "tooling_execution", slicePassed)
	setSliceStatus(t, &state, "project_instance_to_framework_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "tooling/reader/web/app.js"), "console.log('changed');\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "tooling_execution") {
		t.Fatalf("expected tooling_execution stale after reader runtime change, got %+v", result.StaleSlices)
	}
	if !containsString(result.StaleSlices, "project_instance_to_framework_convergence") {
		t.Fatalf("expected project_instance_to_framework_convergence stale after reader runtime change, got %+v", result.StaleSlices)
	}

	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "tooling_execution").Status; got != sliceStale {
		t.Fatalf("expected tooling_execution stale, got %s", got)
	}
	if got := findSlice(t, refreshed, "project_instance_to_framework_convergence").Status; got != sliceStale {
		t.Fatalf("expected project_instance_to_framework_convergence stale, got %s", got)
	}
}

func createInitializedRun(t *testing.T) (string, string, time.Time) {
	t.Helper()
	repoRoot := createReviewRunRepo(t)
	now := time.Date(2026, 4, 26, 10, 30, 0, 0, time.UTC)
	result, err := Init(repoRoot, FlowSpecFlowReview, now)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	return repoRoot, result.File, now
}

func createReviewRunRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	frameworkFiles := []string{
		"spec_flow_review.md",
		"spec_flow_design_review.md",
		"tooling_execution_policy.md",
		"severity_policy.md",
		"spec_writing_guide.md",
		"concepts.md",
	}
	for _, name := range frameworkFiles {
		mustWrite(t, filepath.Join(repoRoot, "framework", name), "# "+name+"\n")
	}
	for _, relPath := range []string{
		"framework/core/object_model.md",
		"framework/core/repository_mapping.md",
		"framework/governance/rule_system.md",
		"framework/governance/impact_sync.md",
		"framework/governance/review.md",
		"framework/governance/review_scope.md",
		"framework/governance/rules/rule_new.md",
		"framework/governance/rules/rule_extract.md",
		"framework/governance/rules/rule_bind.md",
		"framework/governance/rules/rule_topology.md",
		"framework/governance/rules/rule_sync.md",
		"framework/governance/rules/rule_escape.md",
		"framework/operations/migration.md",
		"framework/guidance/using-specflow-guidance/SKILL.md",
		"framework/guidance/project-framing/SKILL.md",
		"framework/guidance/scope-cutting/SKILL.md",
		"framework/guidance/solution-design/SKILL.md",
		"framework/guidance/design-quality-review/SKILL.md",
		"framework/guidance/spec-writeback-guidance/SKILL.md",
		"framework/_atoms/README.md",
		"framework/_atoms/manifest.txt",
		"framework/_atoms/generate.sh",
		"framework/_atoms/verify.sh",
		"templates/docs/specs/_status.md",
		"templates/docs/specs/_check_work/README.md",
		"templates/docs/specs/_check_result/README.md",
		"templates/docs/specs/_verify_result/README.md",
		"templates/docs/specs/_stable_verify_result/README.md",
		"templates/docs/specs/_governance_review/README.md",
		"templates/docs/specs/_independent_evaluation/README.md",
		"templates/docs/specs/repository_mapping.md",
		"templates/docs/specs/rules/stable/s_g_rule_repository_baseline.md",
			"templates/docs/specs/units/candidate/c_unit_demo.md",
		"templates/AGENTS.md",
		"templates/GEMINI.md",
		"templates/CLAUDE.md",
		"example.md",
		"tooling/README.md",
		"tooling/cmd/specflowctl/main.go",
		"tooling/internal/demo/demo.go",
		"tooling/internal/processcleanup/processcleanup.go",
		"tooling/internal/rulesync/release.go",
		"tooling/go.mod",
		"tooling/manifest.tsv",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# "+filepath.Base(relPath)+"\n")
	}
	for _, relPath := range []string{
		"tooling/scripts/build_release.sh",
		"tooling/scripts/install.ps1",
		"tooling/scripts/install.sh",
		"tooling/scripts/pull_with_release.ps1",
		"tooling/scripts/pull_with_release.sh",
		"tooling/scripts/push_with_release.ps1",
		"tooling/scripts/push_with_release.sh",
		"tooling/scripts/tooling_fingerprint.ps1",
		"tooling/scripts/tooling_fingerprint.sh",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# script\n")
	}
	mustWrite(t, filepath.Join(repoRoot, "tooling/reader/web/index.html"), "<!doctype html>\n")
	mustWrite(t, filepath.Join(repoRoot, "tooling/reader/web/styles.css"), "body { color: #111; }\n")
	mustWrite(t, filepath.Join(repoRoot, "tooling/reader/web/app.js"), "console.log('demo');\n")
	mustWrite(t, filepath.Join(repoRoot, "tooling/reader/web/cytoscape.min.js"), "window.cytoscape = function() {};\n")
	mustWrite(t, filepath.Join(repoRoot, "tooling/reader/web/mermaid.min.js"), "window.mermaid = { initialize() {}, run() {} };\n")
	return repoRoot
}





func currentReviewToolingScriptFiles() []string {
	return []string{
		"tooling/scripts/build_release.sh",
		"tooling/scripts/install.ps1",
		"tooling/scripts/install.sh",
		"tooling/scripts/pull_with_release.ps1",
		"tooling/scripts/pull_with_release.sh",
		"tooling/scripts/push_with_release.ps1",
		"tooling/scripts/push_with_release.sh",
		"tooling/scripts/tooling_fingerprint.ps1",
		"tooling/scripts/tooling_fingerprint.sh",
	}
}

func stripToolingScriptInputs(t *testing.T, repoRoot string, state *runState) {
	t.Helper()
	for i := range state.Baseline {
		state.Baseline[i].InputFiles = withoutToolingScriptInputs(state.Baseline[i].InputFiles)
		fingerprint, missing, err := computeFingerprint(repoRoot, state.Baseline[i].InputFiles)
		if err != nil {
			t.Fatalf("fingerprint %s: %v", state.Baseline[i].SliceID, err)
		}
		if len(missing) > 0 {
			t.Fatalf("unexpected missing input for %s: %+v", state.Baseline[i].SliceID, missing)
		}
		state.Baseline[i].InputFingerprint = fingerprint
	}
}

func withoutToolingScriptInputs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, "tooling/scripts/") {
			continue
		}
		result = append(result, value)
	}
	return result
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func mustParse(t *testing.T, file string) runState {
	t.Helper()
	state, err := parseFile(file)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return state
}

func mustConfig(t *testing.T, flow string) flowConfig {
	t.Helper()
	config, err := configForFlow(flow)
	if err != nil {
		t.Fatalf("configForFlow(%s): %v", flow, err)
	}
	return config
}

func setSliceStatus(t *testing.T, state *runState, sliceID, status string) {
	t.Helper()
	for i := range state.Baseline {
		if state.Baseline[i].SliceID == sliceID {
			state.Baseline[i].Status = status
			return
		}
	}
	t.Fatalf("missing slice %s", sliceID)
}

func findSlice(t *testing.T, state runState, sliceID string) sliceEntry {
	t.Helper()
	for _, slice := range state.Baseline {
		if slice.SliceID == sliceID {
			return slice
		}
	}
	for _, slice := range state.Dynamic {
		if slice.SliceID == sliceID {
			return slice
		}
	}
	t.Fatalf("missing slice %s", sliceID)
	return sliceEntry{}
}

func containsDiagnostic(diagnostics []string, fragment string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, fragment) {
			return true
		}
	}
	return false
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

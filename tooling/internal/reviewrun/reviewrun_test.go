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
	if !strings.Contains(content, "| scope_inventory | baseline | local | pending |") {
		t.Fatalf("expected baseline slice table in run-state:\n%s", content)
	}
	state := mustParse(t, result.File)
	routingSlice := findSlice(t, state, "routing_and_command_policy")
	if !containsString(routingSlice.InputFiles, "specflow/framework/onboarding_decision_policy.md") {
		t.Fatalf("expected onboarding policy in routing slice, got %+v", routingSlice.InputFiles)
	}
	if !containsString(routingSlice.InputFiles, "specflow/framework/spec_flow_migrate.md") {
		t.Fatalf("expected migration policy in routing slice, got %+v", routingSlice.InputFiles)
	}
	truthSlice := findSlice(t, state, "truth_and_implementation_gates")
	if !containsString(truthSlice.InputFiles, "specflow/framework/onboarding_decision_policy.md") {
		t.Fatalf("expected onboarding policy in truth gate slice, got %+v", truthSlice.InputFiles)
	}

	validation := ValidateFile(repoRoot, FlowSpecFlowReview, result.File, now)
	if !validation.Valid {
		t.Fatalf("expected valid run-state, got diagnostics: %+v", validation.Diagnostics)
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
		"lifecycle_and_gate_design",
		"human_operability_and_extension",
		"foundation_to_lifecycle_convergence",
		"foundation_to_operability_convergence",
		"lifecycle_to_operability_convergence",
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
	if !containsString(designFoundation.InputFiles, "specflow/framework/onboarding_decision_policy.md") {
		t.Fatalf("expected onboarding decision policy in design foundation input files, got %+v", designFoundation.InputFiles)
	}
	if !containsString(designFoundation.InputFiles, "specflow/framework/spec_flow_migrate.md") {
		t.Fatalf("expected migration policy in design foundation input files, got %+v", designFoundation.InputFiles)
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
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/spec_flow_design_review.md"), "# design changed\n")

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
	state.Score[0].Evidence = "specflow/framework/spec_flow_design_review.md"
	state.Score[0].ResultSummary = "scored"
	mustWrite(t, result.File, renderState(mustConfig(t, FlowSpecFlowDesignReview), state))
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/spec_flow_design_review.md"), "# design changed\n")

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
	state.Score[0].Evidence = "specflow/framework/spec_flow_design_review.md"
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
		InputFiles:       []string{"specflow/framework/spec_flow_review.md"},
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
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/severity_policy.md"), "# severity changed\n")

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
	setSliceStatus(t, &state, "routing_to_command_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/severity_policy.md"), "# severity changed\n")

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "routing_to_command_convergence") {
		t.Fatalf("expected cross slice stale, got %+v", result.StaleSlices)
	}
	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "routing_to_command_convergence").Status; got != sliceStale {
		t.Fatalf("expected cross stale status, got %s", got)
	}
}

func TestRefreshPropagatesStaleThroughDynamicCrossChain(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "review_entry_policy", slicePassed)
	fingerprint, missing, err := computeFingerprint(repoRoot, []string{"specflow/framework/spec_flow_review.md"})
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
			InputFiles:       []string{"specflow/framework/spec_flow_review.md"},
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
			InputFiles:       []string{"specflow/framework/spec_flow_review.md"},
			InputFingerprint: fingerprint,
			DependsOn:        []string{"review_entry_policy"},
			FindingRefs:      "none",
			ResultSummary:    "passed",
			ExitCondition:    "agent records the result",
			ResumeNextStep:   "review slice dynamic_chain_b",
		},
	)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/severity_policy.md"), "# severity changed\n")

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
	state := mustParse(t, file)
	setSliceStatus(t, &state, "review_entry_policy", slicePassed)
	originalFingerprint := findSlice(t, state, "review_entry_policy").InputFingerprint
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	if err := os.Remove(filepath.Join(repoRoot, "specflow/framework/severity_policy.md")); err != nil {
		t.Fatalf("remove input: %v", err)
	}

	result, err := Refresh(repoRoot, FlowSpecFlowReview, file, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !containsString(result.StaleSlices, "review_entry_policy") {
		t.Fatalf("expected missing input to stale passed slice, got %+v", result.StaleSlices)
	}
	if len(result.MissingInputs) == 0 {
		t.Fatalf("expected missing input diagnostic")
	}
	refreshed := mustParse(t, file)
	if got := findSlice(t, refreshed, "review_entry_policy").InputFingerprint; got != originalFingerprint {
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
	if !containsString(slice.InputFiles, "docs/specs/_status.md") {
		t.Fatalf("expected project status input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "docs/specs/repository_mapping.md") {
		t.Fatalf("expected repository mapping input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "docs/specs/rules/stable/s_g_rule_repository_baseline.md") {
		t.Fatalf("expected global rules input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "docs/specs/units/candidate/c_unit_demo.md") {
		t.Fatalf("expected current project truth file input, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "specflow/framework/onboarding_decision_policy.md") {
		t.Fatalf("expected onboarding policy input for source field compatibility, got %+v", slice.InputFiles)
	}
	if !containsString(slice.InputFiles, "specflow/framework/spec_flow_migrate.md") {
		t.Fatalf("expected migration policy input for project-instance migration compatibility, got %+v", slice.InputFiles)
	}
	if containsString(slice.InputFiles, "docs/specs/_governance_review/spec_flow_review.md") {
		t.Fatalf("expected active review run state outside compatibility fingerprint, got %+v", slice.InputFiles)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/specs/units/candidate/c_unit_demo.md")); err != nil {
		t.Fatalf("expected fixture project truth file: %v", err)
	}
}

func TestInitIncludesToolingScriptAndReaderRuntimeInToolingSlices(t *testing.T) {
	_, file, _ := createInitializedRun(t)
	state := mustParse(t, file)

	toolingSlice := findSlice(t, state, "tooling_execution")
	if !containsString(toolingSlice.InputFiles, "specflow/tooling/scripts/tooling_fingerprint.sh") {
		t.Fatalf("expected shell fingerprint script in tooling execution input files, got %+v", toolingSlice.InputFiles)
	}
	if !containsString(toolingSlice.InputFiles, "specflow/tooling/scripts/tooling_fingerprint.ps1") {
		t.Fatalf("expected PowerShell fingerprint script in tooling execution input files, got %+v", toolingSlice.InputFiles)
	}
	if !containsString(toolingSlice.InputFiles, "specflow/tooling/reader/web/app.js") {
		t.Fatalf("expected reader app.js in tooling execution input files, got %+v", toolingSlice.InputFiles)
	}

	convergenceSlice := findSlice(t, state, "project_instance_to_framework_convergence")
	if !containsString(convergenceSlice.InputFiles, "docs/specs/_status.md") {
		t.Fatalf("expected project status in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}
	if !containsString(convergenceSlice.InputFiles, "specflow/tooling/reader/web/app.js") {
		t.Fatalf("expected reader app.js in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}
	if !containsString(convergenceSlice.InputFiles, "specflow/tooling/scripts/tooling_fingerprint.sh") {
		t.Fatalf("expected shell fingerprint script in project/framework convergence input files, got %+v", convergenceSlice.InputFiles)
	}
}

func TestRefreshMarksToolingScriptSlicesStale(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "tooling_execution", slicePassed)
	setSliceStatus(t, &state, "project_instance_to_framework_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/scripts/tooling_fingerprint.sh"), "#!/usr/bin/env bash\necho changed\n")

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
	if !containsString(scopeSlice.InputFiles, "specflow/tooling/scripts/tooling_fingerprint.sh") {
		t.Fatalf("expected refreshed scope_inventory to include shell fingerprint script, got %+v", scopeSlice.InputFiles)
	}
	toolingSlice := findSlice(t, refreshed, "tooling_execution")
	if !containsString(toolingSlice.InputFiles, "specflow/tooling/scripts/tooling_fingerprint.ps1") {
		t.Fatalf("expected refreshed tooling_execution to include PowerShell fingerprint script, got %+v", toolingSlice.InputFiles)
	}
	if got := toolingSlice.Status; got != sliceStale {
		t.Fatalf("expected tooling_execution stale after input list update, got %s", got)
	}
	if got := findSlice(t, refreshed, "tooling_to_rule_convergence").Status; got != sliceStale {
		t.Fatalf("expected tooling_to_rule_convergence stale after input list update, got %s", got)
	}
}

func TestRefreshMarksReaderRuntimeSlicesStale(t *testing.T) {
	repoRoot, file, now := createInitializedRun(t)
	state := mustParse(t, file)
	setSliceStatus(t, &state, "tooling_execution", slicePassed)
	setSliceStatus(t, &state, "project_instance_to_framework_convergence", slicePassed)
	mustWrite(t, file, renderState(mustConfig(t, FlowSpecFlowReview), state))
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/reader/web/app.js"), "console.log('changed');\n")

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
		"spec_flow_migrate.md",
		"agent_operability_standard.md",
		"natural_language_routing.md",
		"onboarding_decision_policy.md",
		"command_policy.md",
		"implementation_change_policy.md",
		"checkpoint_protocol.md",
		"tooling_execution_policy.md",
		"severity_policy.md",
		"scenario_policy.md",
		"spec_policy.md",
		"spec_writing_guide.md",
		"repository_mapping_policy.md",
		"candidate_handoff_contract.md",
		"downgrade_policy.md",
		"recovery_policy.md",
		"impact_sync_policy.md",
		"process_snapshot_contract.md",
		"entry_index_registry.md",
		"project_standards_policy.md",
		"project_standard_create.md",
		"rule_new.md",
		"rule_extract.md",
		"rule_bind.md",
		"rule_topology.md",
		"rule_sync.md",
		"rule_escape.md",
	}
	for _, name := range frameworkFiles {
		mustWrite(t, filepath.Join(repoRoot, "specflow/framework", name), "# "+name+"\n")
	}
	for _, relPath := range []string{
		"specflow/framework/skills/using-specflow-guidance/SKILL.md",
		"specflow/framework/skills/project-framing/SKILL.md",
		"specflow/framework/skills/scope-cutting/SKILL.md",
		"specflow/framework/skills/solution-design/SKILL.md",
		"specflow/framework/skills/design-quality-review/SKILL.md",
		"specflow/framework/skills/spec-writeback-guidance/SKILL.md",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# skill\n")
	}
	mustWrite(t, filepath.Join(repoRoot, "specflow/framework/commands/unit_check.md"), "# unit_check\n")
	for _, relPath := range []string{
		"specflow/templates/docs/specs/_status.md",
		"specflow/templates/docs/specs/_check_result/README.md",
		"specflow/templates/docs/specs/_plans/README.md",
		"specflow/templates/docs/specs/_plans/draft/README.md",
		"specflow/templates/docs/specs/_plans/active/README.md",
		"specflow/templates/docs/specs/_verify_result/README.md",
		"specflow/templates/docs/specs/_governance_review/README.md",
		"specflow/templates/docs/project_standards/_registry.md",
		"specflow/templates/AGENTS.md",
		"specflow/templates/GEMINI.md",
		"specflow/templates/CLAUDE.md",
		"AGENTS.md",
		"GEMINI.md",
		"CLAUDE.md",
		"specflow/tooling/README.md",
		"specflow/tooling/cmd/specflowctl/main.go",
		"specflow/tooling/internal/demo/demo.go",
		"specflow/tooling/scripts/tooling_fingerprint.sh",
		"specflow/tooling/scripts/tooling_fingerprint.ps1",
		"specflow/tooling/go.mod",
		"specflow/tooling/manifest.tsv",
	} {
		mustWrite(t, filepath.Join(repoRoot, relPath), "# "+filepath.Base(relPath)+"\n")
	}
	writeReviewReaderWebFiles(t, repoRoot)
	mustWrite(t, filepath.Join(repoRoot, "docs/project_standards/_registry.md"), ""+
		"# Registry\n\n"+
		"## Active Standards\n\n"+
		"| standard_id | type | surface | file | consumed_by | applies_to | effect | conflict_rule | notes |\n"+
		"|---|---|---|---|---|---|---|---|---|\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_status.md"), ""+
		"# Spec Status\n\n"+
		"## Formal Objects\n\n"+
		"| Object Type | Object | Stable | Candidate | Active Layer | Next Command | Notes |\n"+
		"|---|---|---|---|---|---|---|\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/repository_mapping.md"), "# Repository Mapping\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/rules/stable/s_g_rule_repository_baseline.md"), "# Global Rules\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/units/candidate/c_unit_demo.md"), "# Demo Candidate\n")
	mustWrite(t, filepath.Join(repoRoot, "docs/specs/_governance_review/spec_flow_review.md"), "# ignored run state\n")
	return repoRoot
}

func writeReviewReaderWebFiles(t *testing.T, repoRoot string) {
	t.Helper()
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/reader/web/index.html"), "<!doctype html>\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/reader/web/styles.css"), "body { color: #111; }\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/reader/web/app.js"), "console.log('demo');\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/reader/web/cytoscape.min.js"), "window.cytoscape = function() {};\n")
	mustWrite(t, filepath.Join(repoRoot, "specflow/tooling/reader/web/mermaid.min.js"), "window.mermaid = { initialize() {}, run() {} };\n")
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
		if strings.HasPrefix(value, "specflow/tooling/scripts/") {
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

# Spec Flow Design Review

## 1. Purpose

`spec_flow_design_review` reviews whether the current `specFlow` design is a sound human-serving governance system.

It answers five questions:

1. whether the main governance design solves real repository problems rather than self-created process problems
2. whether the object boundaries, lifecycle order, and gate structure still fit real work shape
3. whether the governance chain creates real downstream control instead of only adding formal steps
4. whether the design remains operable for normal users and executors without excessive mental or operational burden
5. whether the repository may still claim that the current `specFlow` design is worth using as designed

Plain input `spec_flow_design_review` means the default design-baseline review defined in this file unless the user explicitly narrows scope.

This flow does not replace `spec_flow_review`.
`spec_flow_review` answers whether the governance rule set still closes coherently.
`spec_flow_design_review` answers whether that governance design is still reasonable and usable for humans.

This flow does not review business truth by default.
It reviews the design of the governance mechanism that governs business truth.

## 2. Default Scope

The default scope includes the design main chain only.

That default scope includes:

1. core governance and boundary rules
   - `spec_flow_design_review.md`
   - `agent_operability_standard.md`
   - `spec_policy.md`
   - `command_policy.md`
   - `natural_language_routing.md`
   - `implementation_change_policy.md`
   - `repository_mapping_policy.md`
   - `scenario_policy.md`
   - `git_policy.md`
   - `checkpoint_protocol.md`
   - `natural_language_routing.md` only where it defines the shared-governance branch
2. lifecycle and gate-shape rules
   - `specflow/framework/commands/*.md`
   - `candidate_handoff_contract.md`
   - `downgrade_policy.md`
   - `process_snapshot_contract.md`
   - `recovery_policy.md`
   - `specflow/templates/docs/specs/_status.md`
   - `specflow/templates/docs/specs/_check_result/README.md`
   - `specflow/templates/docs/specs/_plans/README.md`
   - `specflow/templates/docs/specs/_plans/draft/README.md`
   - `specflow/templates/docs/specs/_plans/active/README.md`
   - `specflow/templates/docs/specs/_verify_result/README.md`
   - `specflow/templates/docs/specs/_governance_review/README.md`
3. human-entry and extension-surface rules
   - `AGENTS.md`
   - `GEMINI.md`
   - `CLAUDE.md`
   - `specflow/templates/AGENTS.md`
   - `specflow/templates/GEMINI.md`
   - `specflow/templates/CLAUDE.md`
   - `entry_index_registry.md`
   - `project_standards_policy.md`
   - `project_standard_create.md`
   - `docs/project_standards/_registry.md`
   - the active registered project-local standards in scope

The default scope excludes:

1. `tooling_execution_policy.md`
2. `specflow/tooling/README.md`
3. `specflow/tooling/bin/**`
4. `specflow/tooling/cmd/**`
5. `specflow/tooling/internal/**`
6. `shared_new.md`
7. `shared_extract.md`
8. `shared_bind.md`
9. `shared_topology.md`
10. `shared_sync.md`
11. `shared_escape.md`

If a conclusion, finding, or `pass` claim directly depends on one excluded file, the executor must explicitly widen scope first.
Do not claim that an excluded file supports the current design conclusion when that file was never made in-scope.

## 3. Review Blocks

For the default design-baseline review, the execution-local `review_plan` must use exactly these fixed review blocks:

1. `design_foundation`
   - `spec_flow_design_review.md`
   - `agent_operability_standard.md`
   - `spec_policy.md`
   - `command_policy.md`
   - `natural_language_routing.md`
   - `implementation_change_policy.md`
   - `repository_mapping_policy.md`
   - `scenario_policy.md`
   - `natural_language_routing.md` only where it defines the shared-governance branch
   - `AGENTS.md`
   - `GEMINI.md`
   - `CLAUDE.md`
   - `specflow/templates/AGENTS.md`
   - `specflow/templates/GEMINI.md`
   - `specflow/templates/CLAUDE.md`
2. `lifecycle_and_gate_design`
   - `specflow/framework/commands/*.md`
   - `candidate_handoff_contract.md`
   - `downgrade_policy.md`
   - `process_snapshot_contract.md`
   - `recovery_policy.md`
   - `specflow/templates/docs/specs/_status.md`
   - `specflow/templates/docs/specs/_check_result/README.md`
   - `specflow/templates/docs/specs/_plans/README.md`
   - `specflow/templates/docs/specs/_plans/draft/README.md`
   - `specflow/templates/docs/specs/_plans/active/README.md`
   - `specflow/templates/docs/specs/_verify_result/README.md`
   - `git_policy.md`
   - `checkpoint_protocol.md`
3. `human_operability_and_extension`
   - `entry_index_registry.md`
   - `project_standards_policy.md`
   - `project_standard_create.md`
   - `docs/project_standards/_registry.md`
   - the active registered project-local standards in scope
   - `AGENTS.md`
   - `GEMINI.md`
   - `CLAUDE.md`
   - `specflow/templates/AGENTS.md`
   - `specflow/templates/GEMINI.md`
   - `specflow/templates/CLAUDE.md`

## 4. Required Cross-Block Convergence Checks

For the default design-baseline review, the minimum cross-block convergence checks are:

1. `design_foundation <-> lifecycle_and_gate_design`
2. `design_foundation <-> human_operability_and_extension`
3. `lifecycle_and_gate_design <-> human_operability_and_extension`

If a narrowed review still crosses one of those boundaries and the owner block is not included, the review must stop without `pass`.

## 5. Preconditions

### 5.1 Full-Scope Review Run State

Default full-scope `spec_flow_design_review` uses a run-state process file.

The process file is not a Spec, not durable behavior truth, and not a substitute for the review output.
It records review progress, baseline slice status, dynamic risk slice status, score-state progress, input fingerprints, findings, blocked reason, and resume position for one full-scope design review run.

The run-state path is:

```text
docs/specs/_governance_review/spec_flow_design_review/{review_run_id}.md
```

`review_run_id` must use this shape:

```text
YYYYMMDD-HHMMSS-default_design_baseline
```

Rules:

1. full-scope default `spec_flow_design_review` must use this run-state file procedure
2. narrowed `spec_flow_design_review` does not use full-scope run state by default
3. a narrowed review may use a run-state file only when the user explicitly asks for resumable design review
4. the run-state file must not replace the fixed review blocks, the eight fixed design questions, the hard-blocker rules, or the pass gate
5. deterministic tooling may maintain only mechanical fields:
   - UTC timestamps
   - baseline slice skeleton rows
   - score-state skeleton rows
   - input fingerprints
   - structural validation
   - stale status changes caused by changed or missing input files
6. deterministic tooling must not write or modify:
   - question scores
   - `score_basis`
   - design finding content
   - finding severity
   - hard-blocker judgment
   - final `pass | blocked` conclusion

### 5.2 Baseline Slice Catalog

For default full-scope `spec_flow_design_review`, the run-state baseline slice catalog is fixed.
These slices record review progress and input freshness only.
They do not replace the fixed design blocks or the scoring model.

The required baseline slices are:

1. `design_foundation`
   - tracks the fixed `design_foundation` review block from Section 3
2. `lifecycle_and_gate_design`
   - tracks the fixed `lifecycle_and_gate_design` review block from Section 3
3. `human_operability_and_extension`
   - tracks the fixed `human_operability_and_extension` review block from Section 3
4. `foundation_to_lifecycle_convergence`
   - tracks `design_foundation <-> lifecycle_and_gate_design`
5. `foundation_to_operability_convergence`
   - tracks `design_foundation <-> human_operability_and_extension`
6. `lifecycle_to_operability_convergence`
   - tracks `lifecycle_and_gate_design <-> human_operability_and_extension`
7. `scoring_and_pass_gate`
   - tracks whether the hard-blocker review, eight question scores, group averages, weighted score, and pass gate were completed by the executor

The final result must not issue `pass` until every required baseline slice and every dynamic risk slice is closed as `passed` or `skipped_not_in_scope`.

### 5.3 Dynamic Risk Slices

Dynamic risk slices extend the fixed baseline slice catalog during execution.
They are required only when a design risk cannot be safely tracked by one existing baseline slice.

Rules:

1. a dynamic risk slice may be local or cross-convergence
2. a cross-block design risk must become a cross-convergence dynamic slice
3. a dynamic risk slice may only increase review coverage; it must not weaken or replace a baseline slice
4. a dynamic risk slice must be added before final conclusion when the executor discovers:
   - a cross-block design risk
   - a hard-blocker candidate that needs isolated review
   - an in-scope or excluded-scope dependency gap that affects a conclusion
   - a finding whose repair path needs separate re-review before final judgment
5. every dynamic risk slice must record the same slice fields used by the baseline slice table
6. dynamic risk slices do not create extra scoring questions and do not change the fixed weighting formula

### 5.4 Score State

The run-state file must contain a fixed `Score State` table with exactly eight rows: `q1` through `q8`.

Rules:

1. `Score State` records scoring progress only
2. the row IDs map directly to the eight questions in Section 7.1
3. tool-created rows start as `pending`
4. an executor may fill score values and evidence while performing the review
5. tooling may validate table shape and supported status values
6. tooling must not decide whether a score is correct, whether a score basis is sufficient, or whether the pass gate is satisfied

Before execution:

1. make the review scope explicit
2. build one execution-local `review_plan`
3. map in-scope files into the fixed review blocks
4. name the required cross-block convergence checks before final conclusions
5. if project-local governance standards are registered, resolve the active in-scope entries from `docs/project_standards/_registry.md`
6. if the default scope is used, explicitly confirm that the review stayed inside the design main chain and did not silently rely on excluded tooling or internal shared-flow files
7. for default full-scope review, create or reuse the run-state file from Section 5.1 before reviewing the first baseline slice

If any in-scope file cannot be assigned to a review block, do not issue `pass`.

## 6. Procedure

1. collect the in-scope governance files
2. for default full-scope review, execute the run-state startup procedure from Section 5.1
3. build the `review_plan`
4. review each fixed block for:
   - design necessity
   - human operability
   - gate usefulness
   - extension-surface cost
5. complete the required cross-block convergence checks
6. add required dynamic risk slices when newly discovered design risks cannot be tracked by an existing baseline slice
7. judge the hard-blocker set from Section 7.4 before any scoring-based `pass` claim
8. score all eight fixed design questions from Section 7.1
9. compute the fixed group averages from Section 7.2
10. compute the `weighted_score` from Section 7.3
11. produce findings ordered by design risk
   - every real finding must use the fixed finding contract from Section 8.1
12. issue the final result only after baseline slices, dynamic risk slices, hard-blocker review, question scoring, group checks, weighted-score calculation, findings review, and cross-block convergence are all complete

## 7. Scoring Model

### 7.1 Fixed Design Questions

Every `spec_flow_design_review` must answer and score exactly these eight questions:

1. whether the mechanism solves a real problem
2. whether object boundaries follow real work shape
3. whether lifecycle steps are necessary and ordered for real progress
4. whether each gate creates real downstream gain
5. whether the mechanism rewards correct behavior instead of surface compliance
6. whether the mechanism's mental load is sustainably manageable
7. whether the operational cost matches the size of the work
8. whether the overall control gained is worth the overall cost

For every question, the output must report:

1. `score`
2. `score_basis`
3. `evidence`

Allowed score values are fixed:

1. `0`
   - clearly does not hold
2. `1`
   - weakly supported but materially unhealthy
3. `2`
   - basically holds but with clear burden, drift, or unresolved weakness
4. `3`
   - holds with only limited residual weakness
5. `4`
   - clearly holds with strong evidence

Question-specific scoring rules:

1. Question 1 must judge:
   - whether the target problem is explicit
   - whether that problem is real in repository work rather than self-created by the mechanism
   - whether the mechanism still has distinct value instead of duplicating another existing control
2. Question 2 must judge:
   - whether ownership and repair landing points are explicit
   - whether boundaries stay natural rather than artificially split
   - whether current object shape avoids repeated cross-object truth stitching
3. Question 3 must judge:
   - whether each lifecycle step corresponds to a real information change
   - whether the order reduces uncertainty rather than merely renaming state
   - whether the current sequence remains the smallest stable path
4. Question 4 must use only these four real-gain signals:
   - later ambiguity is materially reduced
   - the next step can start more directly
   - the rollback or repair landing point becomes clearer
   - the acceptance basis becomes more stable
   - score Question 4 by the number of confirmed signals hit in the current design, from `0` through `4`
5. Question 5 must judge:
   - whether the design rewards real clarification instead of document inflation
   - whether the design makes it easy to surface uncertainty instead of hiding it
   - whether the easiest way to pass the mechanism still aligns with real downstream quality
6. Question 6 must judge:
   - whether a normal user or executor can tell where they are
   - whether they can tell the next step and why it is the next step
   - whether the official documents, rather than author memory, carry the needed orientation
7. Question 7 must judge:
   - whether small changes have a smaller legal path than large changes
   - whether routine work avoids full-chain over-processing
   - whether the mechanism's operational steps scale with actual work size
8. Question 8 must judge:
   - whether the control gained is visible and repeatable
   - whether the documentation, learning, and execution cost stay proportionate to that gain
   - whether the mechanism still looks worth maintaining over time

### 7.2 Fixed Question Groups

The fixed score groups are:

1. `design_foundation`
   - Questions `1`, `2`, and `3`
2. `control_effectiveness`
   - Questions `4` and `5`
3. `human_operability`
   - Questions `6`, `7`, and `8`

Every review must compute and report the arithmetic average for each group.

### 7.3 Weighted Score

The fixed weights are:

1. Question `1` = `15`
2. Question `2` = `12`
3. Question `3` = `12`
4. Question `4` = `10`
5. Question `5` = `11`
6. Question `6` = `15`
7. Question `7` = `15`
8. Question `8` = `10`

The `weighted_score` formula is fixed:

```text
weighted_score = Σ(score / 4 × weight)
```

Do not invent alternate weighting formulas for this flow.

### 7.4 Hard-Blocker Rules

The following cases are hard blockers.
Any one of them forces the final conclusion to `blocked`, regardless of the weighted score:

1. the core mechanism cannot clearly explain which real problem it solves
2. boundary or lifecycle design leaves repair ownership or repair landing point unstable
3. any key gate has Question `4 = 0`
4. the mechanism clearly rewards surface compliance over real risk reduction
5. a normal user cannot rely on the official documents alone to know current position, next step, and why that step is next
6. simple changes are still forced through the full heavy path with no smaller legal path

### 7.5 Pass Gate

If no hard blocker exists, `pass` still requires all of the following:

1. no individual question score is below `2`
2. every fixed group average is at least `2.5`
3. `weighted_score` is at least `75`

Otherwise the result is `blocked`.

## 8. Output Contract

The output must report at least:

1. `review scope`
2. whether full-scope run state was created, reused, deleted and recreated, or not used
3. the run-state file path when full-scope run state is used
4. `review_plan`
5. the fixed review blocks used
6. the file coverage per block
7. the baseline slice table and slice statuses when run state is used
8. the dynamic risk slice table and slice statuses, or explicit `none`, when run state is used
9. the score-state table when run state is used
10. the stale slice result when run state is used
11. the hard-blocker result
12. all eight question scores, each with:
   - `score`
   - `score_basis`
   - `evidence`
13. the fixed group averages
14. the `weighted_score`
15. the cross-block convergence results
16. the findings result:
   - explicit `none` when no real finding exists
   - otherwise every finding must satisfy Section 8.1
17. the final conclusion:
   - `pass`
   - `blocked`

If the output does not explicitly report Items 11 through 16, the review is not complete.

### 8.1 Finding Contract

When `spec_flow_design_review` reports a real finding, that finding must be written as one self-contained repairable unit.

The minimum required fields are:

1. `title`
2. `severity`
   - required when the finding is graded under `severity_policy.md`
3. `affected_questions`
   - the exact question numbers from Section 7.1 that this finding harms
4. `score_impact`
   - the exact score consequence or pass-gate consequence caused by the finding
5. `background`
6. `what happened`
7. `impact`
8. `recommended fix`
9. `why this fix is the minimal correct fix`
10. `blocking`
11. `evidence`

Additional rules:

1. if `severity` is present, the finding must satisfy the shared explanation baseline from `severity_policy.md`
2. `severity` explains design harm; it does not replace the fixed score model
3. `score` explains current design quality; it does not replace explicit blocking judgment
4. `recommended fix` must be specific enough that later repair work can execute it without a second clarification round
5. when no real finding exists, the output must say so explicitly instead of omitting the finding section

## 9. Non-Goals

This flow does not:

1. replace `spec_flow_review`
2. replace the shared-governance branch
3. review business truth by default
4. review tooling source or binaries by default
5. create a new command chain
6. update `_status.md`
7. write `_check_result`, `_plans`, or `_verify_result`
8. define a project-local overlay surface in v1
9. use checkpoints in v1

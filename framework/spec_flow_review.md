# Spec Flow Review

## 1. Purpose

`spec_flow_review` reviews the governance mechanism itself.

It answers five questions:

1. whether the governance rule set still closes the full Spec Flow
2. whether the tooling layer still matches the rule layer
3. whether shared-governance and impact-reconciliation semantics still converge with the command core
4. whether governance documents can make an executor operational without prior `specFlow` knowledge or avoidable reading cost
5. whether the repository may still claim one coherent governance baseline

Plain input `spec_flow_review` means the default governance-baseline review defined in this file unless the user explicitly narrows scope.

This flow does not review business truth by default.
It reviews the mechanism that governs business truth.
It does not prove that the current governance design is sensible, humane, or worth using as designed.
That judgment belongs to `spec_flow_design_review`.

## 2. Default Scope

The default scope includes:

1. framework governance rules
   - `specflow/framework/*.md`
2. command rules
   - `specflow/framework/commands/*.md`
3. guidance skill rules
   - `specflow/framework/skills/*/SKILL.md`
4. template-side process and state contracts
   - `specflow/templates/docs/specs/_status.md`
   - `specflow/templates/docs/specs/_check_result/README.md`
   - `specflow/templates/docs/specs/_plans/README.md`
   - `specflow/templates/docs/specs/_plans/draft/README.md`
   - `specflow/templates/docs/specs/_plans/active/README.md`
   - `specflow/templates/docs/specs/_verify_result/README.md`
   - `specflow/templates/docs/specs/_governance_review/README.md`
5. template entry files
   - `specflow/templates/AGENTS.md`
   - `specflow/templates/GEMINI.md`
   - `specflow/templates/CLAUDE.md`
6. project entry files
   - `AGENTS.md`
   - `GEMINI.md`
   - `CLAUDE.md`
7. entry registry and project-standard governance files
   - `specflow/framework/entry_index_registry.md`
   - `specflow/framework/project_standards_policy.md`
   - `specflow/framework/project_standard_create.md`
   - `specflow/templates/docs/project_standards/_registry.md`
   - `docs/project_standards/_registry.md`
   - the active registered project-local standards in scope
8. tooling contract and tooling source input
   - `specflow/framework/tooling_execution_policy.md`
   - `specflow/tooling/README.md`
   - `specflow/tooling/cmd/**/*.go`
   - `specflow/tooling/internal/**/*.go`
   - `specflow/tooling/go.mod`
   - `specflow/tooling/manifest.tsv`
   - `specflow/tooling/go.sum` when it exists

Default scope excludes project-instance truth and project-instance state files under `docs/specs/`.

Excluded files include:

1. `docs/specs/repository_mapping.md`
2. `docs/specs/_status.md`
3. `docs/specs/system_constraints.md`
4. `docs/specs/units/**`
5. `docs/specs/scenarios/**`
6. `docs/specs/shared_contracts/**`
7. `docs/specs/_check_result/**`
8. `docs/specs/_plans/**`
9. `docs/specs/_verify_result/**`
10. `docs/specs/_governance_review/**`

Those files may be reviewed only when the user explicitly narrows `spec_flow_review` to project-instance state, or when a command, repository-mapping flow, shared-governance flow, or verification flow consumes them under its own policy.

Default scope must explicitly include:

1. the shared-governance rule set
   - at minimum `natural_language_routing.md`, `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`
2. the guidance-skill rule set
   - at minimum `using-specflow-guidance/SKILL.md`, `project-framing/SKILL.md`, `scope-cutting/SKILL.md`, `solution-design/SKILL.md`, `design-quality-review/SKILL.md`, and `spec-writeback-guidance/SKILL.md`
3. the impact-reconciliation rule set
   - at minimum `impact_sync_policy.md`, `process_snapshot_contract.md`, `recovery_policy.md`, template `_status.md`, and the template-side process README files
4. the tooling execution contract set
   - at minimum `tooling_execution_policy.md`, `specflow/tooling/README.md`, and the in-scope tooling source files
5. the agent-operability standard
   - at minimum `agent_operability_standard.md`, entry files, routing policy files, command policy files, command files, shared-governance files, guidance skill files, review policy files, and process-state contract files in the current review scope

If any one of those five coverage sets is missing from a default-scope review, that review is not complete and must not issue `pass`.

## 3. Baseline Slice Catalog

For the default governance-baseline review, the executor must use the baseline slice catalog below.

Baseline slices are the minimum review outline.
They do not limit what the review may discover.
If the review discovers a material risk that is not fully covered by a baseline slice, the executor must add a dynamic slice under Section 4.

### 3.1 Local Baseline Slices

Local slices review one owner area for internal closure, side effects, contract drift, missing ownership, and local agent operability.

1. `scope_inventory`
   - verifies default-scope collection, excluded project-instance truth, active project-local standards, and unassigned file handling
   - includes the deterministic scope produced by `review collect-default-scope --flow spec_flow_review`
2. `review_entry_policy`
   - reviews `spec_flow_review.md`, `spec_flow_design_review.md`, `severity_policy.md`, and `checkpoint_protocol.md`
   - verifies review entry meaning, output contracts, finding contracts, and stop behavior
3. `routing_and_command_policy`
   - reviews `natural_language_routing.md`, `command_policy.md`, `scenario_policy.md`, `commands/*.md`, and `skills/*/SKILL.md`
   - verifies exact command routing, natural-language routing, unit command progression, scenario command progression, and guidance entry behavior
4. `truth_and_implementation_gates`
   - reviews `spec_policy.md`, `repository_mapping_policy.md`, `implementation_change_policy.md`, `candidate_handoff_contract.md`, `downgrade_policy.md`, `recovery_policy.md`, and `git_policy.md`
   - verifies truth ownership, implementation diversion, handoff, fallback, recovery, and close-out rules
5. `shared_governance`
   - reviews `natural_language_routing.md` only where it defines the shared-governance branch
   - reviews `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`
6. `process_and_impact_state`
   - reviews `impact_sync_policy.md`, `process_snapshot_contract.md`, `recovery_policy.md`, template `_status.md`, template `_check_result`, template `_plans`, template `_verify_result`, and template `_governance_review`
   - verifies process-state contracts, snapshot invalidation, impact handling, and governance-review run-state boundaries
7. `entry_and_project_extension`
   - reviews `entry_index_registry.md`, `project_standards_policy.md`, `project_standard_create.md`, registered entry files, template entry files, template project-standard registry, project registry, and active project-local standards in scope
8. `tooling_execution`
   - reviews `tooling_execution_policy.md`, `specflow/tooling/README.md`, and in-scope tooling source files
   - verifies tooling necessity, allowed mechanical action surface, forbidden semantic judgment, freshness, and document/source agreement
9. `agent_operability_local`
   - reviews the agent-operability result recorded by each local slice against `agent_operability_standard.md`
   - verifies that local slice conclusions did not rely on prior conversation, ordinary term meanings, or avoidable repeated reading

### 3.2 Cross-Convergence Baseline Slices

Cross-convergence slices review whether locally correct rules still compose into one coherent governance baseline.

1. `routing_to_command_convergence`
   - verifies natural-language routing, exact command routing, guidance entry, and review entry behavior converge without ambiguous owner selection
2. `command_to_process_state_convergence`
   - verifies command pass, fallback, cleanup, snapshot, and process-file consumption rules converge
3. `truth_to_implementation_convergence`
   - verifies truth writeback, repository mapping, implementation gates, handoff, recovery, and git close-out converge
4. `shared_to_impact_convergence`
   - verifies shared-governance changes correctly converge with impact reconciliation and downstream process-state invalidation
5. `entry_extension_to_review_convergence`
   - verifies entry files and project-local standards cannot bypass the framework baseline, narrow default scope silently, or change review meaning without owner rules
6. `tooling_to_rule_convergence`
   - verifies tooling executes only rule-decided mechanical work and does not become a second semantic source of truth
7. `agent_operability_path_walk`
   - walks representative execution paths across routing, command, shared, process-state, entry, and tooling rules
   - verifies a new executor can proceed from request to next legal action without hidden context

The final result must not issue `pass` until every required local baseline slice, every required cross-convergence baseline slice, and every dynamic slice is closed as `passed` or `skipped_not_in_scope`.

## 4. Dynamic Slices

Dynamic slices extend the baseline catalog during execution.
They are required when a discovered risk is not fully covered by an existing baseline slice.

Rules:

1. a dynamic slice may be local or cross-convergence
2. a cross-area risk must become a cross-convergence dynamic slice instead of being hidden inside one local slice
3. a dynamic slice may only increase review coverage; it must not weaken or replace a baseline slice
4. a dynamic slice must be added before final conclusion when the executor discovers:
   - a new dependency boundary
   - a new owner conflict
   - a new process-state or tooling interaction
   - a new agent-operability risk
   - a finding that needs a separate repairability check
5. every dynamic slice must record:
   - `slice_id`
   - `parent_slice_id`
   - `slice_type`
   - `review_question`
   - `why_added`
   - `input_files`
   - `depends_on`
   - `exit_condition`
   - `status`

## 5. Full-Scope Review Run State

Default full-scope `spec_flow_review` uses a run-state process file.

The process file is not a Spec, not durable behavior truth, and not a substitute for the review output.
It records review progress, slice inputs, stale status, findings, and resume position for one full-scope review run.

The run-state path is:

```text
docs/specs/_governance_review/spec_flow_review/{review_run_id}.md
```

`review_run_id` must use this shape:

```text
YYYYMMDD-HHMMSS-{scope_label}
```

### 5.1 When To Use Run State

Rules:

1. full-scope default `spec_flow_review` must use the run-state file procedure in this section
2. narrowed `spec_flow_review` does not use full-scope run state by default
3. a narrowed review may use a run-state file only when the user explicitly asks for resumable slice review
4. project-instance truth under `docs/specs/` remains outside default governance-baseline review even though the run-state file itself is read for resume handling

### 5.1.1 Run-State Tooling Boundary

Run-state files contain both mechanical fields and review judgment fields.

Mechanical fields must be written by deterministic tooling when the tooling is available.
If the tooling is unavailable, the executor must obtain UTC time from the runtime environment before writing timestamp fields.
The executor must not invent timestamps, input fingerprints, or stale-refresh results from conversation context.

The mechanical fields are:

1. `created_at`
2. `last_updated_at`
3. baseline slice skeleton rows
4. `input_fingerprint`
5. stale status changes caused only by changed or missing `input_files`

The deterministic tooling entry is `specflowctl review run-* --flow spec_flow_review`.

Rules:

1. `review run-init --flow spec_flow_review` creates or reuses the full-scope run-state file
2. `review run-validate --flow spec_flow_review` checks the run-state file shape and fixed status values
3. `review run-refresh --flow spec_flow_review` recomputes slice fingerprints and marks affected `passed` slices as `stale`
4. `review run-touch --flow spec_flow_review` updates only `last_updated_at`
5. tooling must not decide whether a slice has passed review
6. tooling must not write finding content
7. tooling must not decide final `pass` or `blocked`

### 5.2 Startup Procedure

At the start of a full-scope review:

1. inspect `docs/specs/_governance_review/spec_flow_review/` for unclosed run-state files
2. if no unclosed run-state file exists, create a new run-state file and start at `scope_inventory`
3. if one unclosed run-state file exists, run the basic validity check from Section 5.3
4. if the basic validity check fails, delete the old run-state file, report the deletion reason, create a new run-state file, and start at `scope_inventory`
5. if the basic validity check passes, apply the timestamp rules from Section 5.4
6. if multiple unclosed run-state files exist, stop and ask the user to choose which run to continue or to allow cleanup

### 5.3 Basic Validity Check

The basic validity check verifies only that the run-state file can be used as a progress file.
It does not judge whether old review conclusions are still semantically correct.

The file is valid only when:

1. the file can be read
2. `review_flow` is `spec_flow_review`
3. `scope_label` is `default_governance_baseline`
4. `status` is one of:
   - `in_progress`
   - `blocked_on_finding`
   - `ready_for_final`
5. all required run fields from Section 7.1 exist
6. `created_at` and `last_updated_at` use the timestamp format from Section 5.4
7. the baseline and dynamic slice tables can be parsed
8. every slice status is one of the fixed slice status values from Section 5.6
9. every baseline slice has `parent_slice_id` set to `none`
10. every dynamic slice has `parent_slice_id` set to an existing baseline or dynamic slice in the same run-state file

Rules:

1. `closed_pass` and `closed_blocked` are closed states and must not be reused
2. any other run status value is invalid and fails the basic validity check
3. if `last_updated_at` cannot be parsed, the file fails the basic validity check

The basic validity check must not decide:

1. whether the old review plan still covers every current risk
2. whether old slice conclusions remain semantically trustworthy after framework changes
3. whether current `specFlow` design is still worthwhile

### 5.4 Timestamp Reuse Rules

The run-state file must update `last_updated_at` whenever a slice status, active slice, finding list, blocked reason, or resume step changes.

Timestamp format rules:

1. `created_at` and `last_updated_at` must use UTC ISO 8601 in this exact shape:
   - `YYYY-MM-DDTHH:MM:SSZ`
2. examples:
   - `2026-04-26T10:30:00Z`
3. timezone offsets other than `Z` are invalid
4. timestamps with missing seconds are invalid
5. invalid timestamps fail the basic validity check
6. `last_updated_at` later than the current UTC time fails the basic validity check

Reuse rules:

1. compute run-state age as current UTC time minus `last_updated_at`
2. if the run-state age is within 2 hours, automatically reuse the run-state file
3. if the run-state age is older than 2 hours and no older than 24 hours, ask the user whether to reuse the run-state file or delete it and start a new run
4. if the run-state age is older than 24 hours and no older than 7 days, ask the user, but recommend deleting the old run-state file and starting a new run
5. if the run-state age is older than 7 days, delete the old run-state file and create a new run unless the user explicitly requests continuing that exact run

When a user chooses to reuse an old run-state file, that choice accepts the old progress record as the continuation basis.
The executor still must refresh file fingerprints and stale statuses under Section 5.5.

### 5.5 Stale Slice Handling

Every slice must record `input_files` and `input_fingerprint`.

On reuse:

1. recompute each slice input fingerprint from the current files
2. change any `passed` slice with changed input fingerprint to `stale`
3. change any cross-convergence slice that depends on a stale slice to `stale`
4. keep unaffected slices in their current status
5. add dynamic slices for newly discovered risks

### 5.5.1 Slice Input Fingerprint Contract

Slice input fingerprints use the same text normalization rules as `specflow/framework/process_snapshot_contract.md`.

For each file in `input_files`:

1. file paths must be repository-relative paths rendered with `/`
2. file paths must be sorted lexicographically before hashing
3. read the full file text
4. normalize the text using `process_snapshot_contract.md` Section 7
5. compute `sha256` of the normalized UTF-8 bytes
6. render the file hash as lowercase hexadecimal

The slice fingerprint is computed from the ordered file records.

For each sorted file, append these exact lines to the fingerprint payload:

```text
file_ref: <path>
file_sha256: <hex>

```

Then compute `sha256` of the full payload encoded as UTF-8 and render it as lowercase hexadecimal.

Rules:

1. an empty `input_files` list is invalid unless the slice status is `skipped_not_in_scope`
2. if any input file is missing during fingerprint refresh, the slice becomes `stale`
3. if a missing file prevents the slice from being reviewed, the slice must become `blocked`
4. executors must not use filesystem timestamps, file size, git metadata, or conversation history as fingerprint input
5. dynamic slices use the same fingerprint contract as baseline slices

### 5.6 Run And Slice Status Values

Run status values are fixed:

1. `in_progress`
2. `blocked_on_finding`
3. `ready_for_final`
4. `closed_pass`
5. `closed_blocked`

Slice status values are fixed:

1. `pending`
2. `passed`
3. `blocked`
4. `stale`
5. `skipped_not_in_scope`

Blocked rules:

1. a blocking finding changes the run status to `blocked_on_finding`
2. while blocked, the next review action must be the repair path or re-review of affected slices
3. a blocked run must not advance to final conclusion until all blocking findings are resolved and affected slices are re-reviewed

## 6. Procedure

For full-scope review:

1. collect the default in-scope governance files
2. execute the run-state startup procedure from Section 5.2
3. build or refresh the baseline slice table
4. review local baseline slices
5. add required dynamic slices when new risks are discovered
6. review cross-convergence baseline slices
7. review any cross-convergence dynamic slices
8. refresh stale statuses whenever an input file changes during the run
9. produce findings ordered by governance risk
   - every real finding must use the fixed finding contract from Section 7.2
   - do not collapse a real finding into a one-line conclusion with no repair guidance
10. issue the final result only after all required baseline and dynamic slices are closed

For narrowed review:

1. make the narrowed scope explicit
2. map the narrowed scope to the relevant baseline slice or slices
3. add dynamic slices when the narrowed review discovers uncovered risks inside the narrowed scope
4. do not claim default governance-baseline `pass`

## 7. Output Contract

The output must report at least:

1. the review scope
2. whether full-scope run state was created, reused, deleted and recreated, or not used
3. the run-state file path when full-scope run state is used
4. the baseline slice table and slice statuses
5. the dynamic slice table and slice statuses, or explicit `none`
6. the stale slice result
7. the shared-governance coverage result
8. the guidance-skill coverage result
9. the impact-reconciliation coverage result
10. the tooling coverage result
11. the agent-operability result, including local slice results and path-walk result
12. the cross-convergence results
13. the findings result:
   - explicit `none` when no real finding exists
   - otherwise every finding must satisfy Section 7.2
14. the final conclusion:
   - `pass`
   - `blocked`

If the output does not explicitly report Items 7 through 13, the review is not complete.

### 7.1 Run-State File Shape

The run-state file must contain these run fields:

1. `review_flow`
2. `review_run_id`
3. `scope_label`
4. `status`
5. `created_at`
6. `last_updated_at`
7. `active_slice`
8. `baseline_slice_table`
9. `dynamic_slice_table`
10. `finding_refs`
11. `blocked_reason`
12. `resume_next_step`

Each slice entry must contain:

1. `slice_id`
2. `slice_origin`
   - `baseline` or `dynamic`
3. `slice_type`
   - `local` or `cross_convergence`
4. `status`
5. `review_question`
6. `why_added`
   - use `baseline_catalog` for baseline slices
7. `parent_slice_id`
   - use `none` for baseline slices
   - dynamic slices must reference an existing baseline or dynamic slice
8. `input_files`
9. `input_fingerprint`
10. `depends_on`
11. `finding_refs`
12. `result_summary`
13. `exit_condition`
14. `resume_next_step`

### 7.2 Finding Contract

When `spec_flow_review` reports a real finding, that finding must be written as one self-contained repairable unit.

The minimum required fields are:

1. `title`
   - one short problem label
2. `severity`
   - required when the finding is graded under `severity_policy.md`
3. `background`
   - the minimum repository or rule context needed to understand why this finding matters
4. `what happened`
   - the concrete mismatch, drift, omission, or conflict that was observed
5. `impact`
   - what governance risk, flow break, or downstream instability this creates
6. `recommended fix`
   - the concrete repair direction that should be executed next
7. `why this fix is the minimal correct fix`
   - why the recommendation closes the problem without inventing a wider redesign
8. `blocking`
   - explicit `yes` or `no`
9. `evidence`
   - the file refs, block boundary, or tool/runtime result that directly supports the finding

Additional rules:

1. if `severity` is present, the finding must satisfy the shared explanation baseline from `specflow/framework/severity_policy.md`
2. `recommended fix` must be specific enough that a later user instruction such as "go fix it" can clearly refer back to that proposed repair without requiring a second clarification round
3. do not replace `recommended fix` with a vague statement such as "should be aligned" or "needs cleanup"
4. if more than one plausible repair exists and the review cannot justify one minimal correct fix, the finding must say that the repair path is still unresolved and the review must not present a guessed fix as settled
5. when no real finding exists, the output must say so explicitly instead of omitting the finding section

## 8. Non-Goals

This flow does not:

1. replace the shared-governance branch
2. replace `impact_sync`
3. review business truth by default
4. treat recently touched governance files as the whole scope unless the user explicitly narrows it that way
5. prove design adequacy, human operability, or design worthiness

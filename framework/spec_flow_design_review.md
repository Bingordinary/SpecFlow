# Spec Flow Design Review

## 1. Purpose

`spec_flow_design_review` reviews whether the current `specFlow` design is a sound human-serving governance system.
This file owns the only `spec_flow_design_review` mode: the default full-scope design-baseline review.
Ordinary or plain exact `spec_flow_design_review` entry routes through `framework/governance/review.md` first, then enters this file.
It must not be narrowed into `scoped_review`.

It answers five questions:

1. whether the main governance design solves real repository problems rather than self-created process problems
2. whether the object boundaries and process structure still fit real work shape
3. whether the promote-as-only-gate design creates real downstream control instead of adding formal overhead
4. whether the design remains operable for its governing executor type (LLM agents for specFlow execution, humans for framework maintenance and governance decisions) without excessive cost
5. whether the repository may still claim that the current `specFlow` design is worth using as designed

Plain exact `spec_flow_design_review` starts the full-scope design-baseline review.
It uses the run-state, baseline slice table, dynamic risk slice table, score state, hard-blocker rules, and pass gate defined in this file.

This flow does not replace `spec_flow_review`.
`spec_flow_review` answers whether the governance rule set still closes coherently.
`spec_flow_design_review` answers whether that governance design is still reasonable and usable for its intended executor types (LLM agents as primary executors, humans as governance maintainers and decision-makers).

This flow does not review business truth by default.
It reviews the design of the governance mechanism that governs business truth.

## 2. Default Scope

This section applies to every `spec_flow_design_review`.

The default scope includes the design main chain only.
It uses the fixed `source_repo` layout:

- framework root: `framework/`
- template root: `templates/`
- tooling root: `tooling/`
- project-instance compatibility mode: template bootstrap compatibility under `templates/docs/specs/`

That default scope includes:

1. core governance and boundary rules
   - `spec_flow_design_review.md`
   - `governance/review.md`
   - `governance/review_scope.md`
   - `concepts.md`
   - `operations/update.md`
   - `core/object_model.md`
   - `spec_writing_guide.md`
    - `rule_validate_checklist.md`, `rule_promote_workflow.md` where they define the rule validate/promote pipeline
  2. process rules
    - `concepts.md` (the candidate-to-promote process)
    - `spec_writing_guide.md`
    - `governance/impact_sync.md`
 The default scope excludes:

 1. `tooling_execution_policy.md`
 2. `<tooling-root>/README.md`
 3. `<tooling-root>/bin/**`
 4. `<tooling-root>/cmd/**`
 5. `<tooling-root>/internal/**`

If a conclusion, finding, or `pass` claim directly depends on one excluded file, the executor must explicitly widen scope first.
Do not claim that an excluded file supports the current design conclusion when that file was never made in-scope.

## 3. Review Blocks

For the default design-baseline review, the execution-local `review_plan` must use exactly these fixed review blocks:

1. `design_foundation`
   - `spec_flow_design_review.md`
   - `governance/review.md`
   - `governance/review_scope.md`
   - `concepts.md`
   - `operations/update.md`
   - `core/object_model.md`
   - `spec_writing_guide.md`
    - `rule_validate_checklist.md`, `rule_promote_workflow.md` where they define the rule validate/promote pipeline
    - `governance/impact_sync.md`
2. `process_and_gate_design`
   - `concepts.md` (the candidate-to-promote process design)
   - `spec_writing_guide.md`
   - `core/object_model.md`
   - `governance/impact_sync.md`
 3. `executor_operability_and_extension`
    - `source_repo` guidance is derived from `concepts.md` and the design-review procedure itself

The review must judge whether `framework/concepts.md` (the hook-injected instruction source) delivers a self-contained instruction pack to the executor, whether cross-file links are used only for non-essential context, and whether each specFlow process step can be executed without context inherited from prior steps.

Project-instance migration design must be judged as part of `design_foundation`.
The review must judge whether `spec_flow_update` solves the real framework-update migration problem without turning old-format compatibility into a permanent second path, without hiding business-truth decisions inside mechanical updates, and without adding a heavier workflow than project-instance format migration requires.

## 4. Required Cross-Block Convergence Checks

For the default design-baseline review, the minimum cross-block convergence checks are:

1. `design_foundation <-> process_and_gate_design`
2. `design_foundation <-> executor_operability_and_extension`
3. `process_and_gate_design <-> executor_operability_and_extension`

The review must include all three design blocks before any `pass` or `pass-with-optimization` conclusion.
If one of those block boundaries is not reviewed, the review must stop without a passing conclusion.

## 5. Preconditions

### 5.1 Full-Scope Review Run State

Every `spec_flow_design_review` uses a run-state process file.

The process file is not a Spec, not durable behavior truth, and not a substitute for the review output.
It records review progress, baseline slice status, dynamic risk slice status, score-state progress, input fingerprints, findings, non-blocking optimization references, blocked reason, and resume position for one full-scope design review run.

The run-state path is:

```text
meta/governance_review/spec_flow_design_review.md
```

`review_run_id` is a field inside the run-state file.
It must use this shape:

```text
YYYYMMDD-HHMMSS-default_design_baseline
```

Final review conclusions map to run-state status values as follows:

1. `pass` -> `closed_pass`
2. `pass-with-optimization` -> `closed_pass_with_optimization`
3. `blocked` -> `closed_blocked`

All three mapped status values are closed run states.
The startup procedure must delete any closed run state before creating a new full-scope default design review run.

There must be at most one `spec_flow_design_review` run-state file in the repository at any time.
Starting a new full-scope default design review must delete the previous `spec_flow_design_review` run-state file before writing the new run state.
The file name must not contain the run ID, because the run ID identifies the review round inside the file rather than creating a history archive.

Rules:

1. every `spec_flow_design_review` must use this run-state file procedure
2. the run-state file must not replace the fixed review blocks, the eight fixed design questions, the hard-blocker rules, or the pass gate
3. deterministic tooling may maintain only mechanical fields:
   - UTC timestamps
   - `review_layout`
   - baseline slice skeleton rows
   - score-state skeleton rows
   - input fingerprints
   - structural validation
   - stale status changes caused by changed or missing input files
4. deterministic tooling must not write or modify:
   - question scores
   - `score_basis`
   - design finding content
   - finding severity
   - non-blocking optimization content
   - hard-blocker judgment
   - final `pass | pass-with-optimization | blocked` conclusion
5. the startup procedure must inspect only `meta/governance_review/spec_flow_design_review.md`
6. if the fixed run-state file does not exist, the startup procedure must create a new run-state file and begin at `design_foundation`
7. if the fixed run-state file is closed or structurally invalid, the startup procedure must delete it, report the deletion reason, create a new run-state file, and begin at `design_foundation`
8. if the fixed run-state file is valid, open, and last updated no more than two hours before startup, the startup procedure may reuse it automatically; before review work continues, the executor must refresh fingerprints, mark stale slices, and resume from the recorded active slice
9. if the fixed run-state file is valid, open, and last updated more than two hours but no more than seven days before startup, the startup procedure must stop for an explicit manual decision to either reuse the file or delete it and start a new run
   - if the decision is reuse, the executor must refresh fingerprints, mark stale slices, and resume from the recorded active slice
   - if the decision is delete, the startup procedure must delete the file, create a new run-state file, and begin at `design_foundation`
10. if the fixed run-state file is valid, open, and last updated more than seven days before startup, the startup procedure must delete it as expired, report the deletion reason, create a new run-state file, and begin at `design_foundation`
11. the startup procedure must not scan a per-flow subdirectory or preserve old closed run-state files as review history

Design-review adoption rules:

1. the state carrier for the default full-scope review is `meta/governance_review/spec_flow_design_review.md`
2. required run-state fields, baseline slice rows, dynamic risk slice rows, and score-state rows are defined in this file
3. baseline slices are defined in Section 5.2
4. dynamic risk slices are allowed only under Section 5.3
5. required cross-block convergence checks are defined in Section 4 and represented by the applicable baseline or dynamic risk slices
6. freshness and stale handling are performed through the review run-state procedure
7. slice-set closure can support `pass` or `pass-with-optimization` only when the hard-blocker review, scoring model, group checks, weighted score, findings review, optimization review, and cross-block convergence also pass
8. missing design truth, unclear in-scope ownership, or excluded-scope dependency gaps must become a dynamic risk slice, finding, optimization, blocked result, or explicit scope stop; they must not be hidden as ordinary score evidence

Structural validation rule:

1. `review run-validate --flow spec_flow_design_review` checks file shape, `review_layout`, and all fixed status values, including closed statuses; it is not a reuse decision.
2. closed run-state files may be structurally valid, but they are not open and must not be reused by startup.
3. freshness refresh applies only to an open run-state file.
4. `review run-refresh --flow spec_flow_design_review` is the authoritative entry for updating `input_fingerprint` and marking stale slices.
5. manual hashes, shell checksum output, editor display, temporary scripts, and conversation-derived values must not be written as `input_fingerprint` values or used to decide that a design-review slice remains fresh.

### 5.2 Baseline Slice Catalog

For default full-scope `spec_flow_design_review`, the run-state baseline slice catalog is fixed.
These slices record review progress and input freshness only.
They do not replace the fixed design blocks or the scoring model.

The required baseline slices are:

1. `design_foundation`
   - tracks the fixed `design_foundation` review block from Section 3
2. `process_and_gate_design`
   - tracks the fixed `process_and_gate_design` review block from Section 3
3. `executor_operability_and_extension`
   - tracks the fixed `executor_operability_and_extension` review block from Section 3
4. `foundation_to_process_convergence`
   - tracks `design_foundation <-> process_and_gate_design`
5. `foundation_to_operability_convergence`
   - tracks `design_foundation <-> executor_operability_and_extension`
6. `process_to_operability_convergence`
   - tracks `process_and_gate_design <-> executor_operability_and_extension`
7. `scoring_and_pass_gate`
   - tracks whether the hard-blocker review, eight question scores, group averages, weighted score, and pass gate were completed by the executor

The final result must not issue `pass` or `pass-with-optimization` until every required baseline slice and every dynamic risk slice is closed as `passed` or `skipped_not_in_scope`.

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
5. explicitly confirm that the review stayed inside the design main chain and did not silently rely on excluded tooling or internal rule-flow files
6. create or reuse the run-state file from Section 5.1 before reviewing the first baseline slice

If any in-scope file cannot be assigned to a review block, do not issue `pass`.

## 6. Procedure

1. collect the in-scope governance files
2. execute the run-state startup procedure from Section 5.1
3. build the `review_plan`
4. review each fixed block for:
   - design necessity
   - executor operability
   - gate usefulness
   - extension-surface cost
5. run the `entry_control_chain_check` from Section 7.1 before judging hard blockers or scoring Questions 6, 7, or 8
6. run the `routine_work_path_check` from Section 7.1 when its trigger condition is met
7. complete the required cross-block convergence checks
8. add required dynamic risk slices when newly discovered design risks cannot be tracked by an existing baseline slice
9. judge the hard-blocker set from Section 7.4 before any scoring-based `pass` claim
10. score all eight fixed design questions from Section 7.1
11. compute the fixed group averages from Section 7.2
12. compute the `weighted_score` from Section 7.3
13. separate blocking findings from non-blocking optimizations
   - every real finding must use the fixed finding contract from Section 8.1
   - every non-blocking optimization must use the optimization contract from Section 8.2
14. issue the final result only after baseline slices, dynamic risk slices, entry control chain check, hard-blocker review, routine-work path check when triggered, question scoring, group checks, weighted-score calculation, findings review, optimization review, and cross-block convergence are all complete

## 7. Scoring Model

### 7.1 Fixed Design Questions

Every `spec_flow_design_review` must answer and score exactly these eight questions:

1. whether the mechanism solves a real problem
2. whether object boundaries follow real work shape
3. whether the next→review→promote process steps are necessary and correctly ordered for real progress
4. whether the promote-as-only-gate creates real downstream gain
5. whether the mechanism rewards correct behavior instead of surface compliance
6. whether the mechanism's instruction design supports reliable executor orientation and following
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

For Question 6, the score basis must explicitly evaluate:

**Executor-type preamble:** The primary executors are LLM agents. For LLM executors, "instruction design" cost is measured in: context-window consumption by governance instructions; self-containment (whether each phase carries complete instructions without requiring cross-phase memory); rule explicitness (whether rules are stated directly rather than implied); and stop-condition clarity (whether the executor can deterministically identify when to stop). Document length and chain-reading per se are negligible costs for LLMs. The real concerns are instruction ambiguity, missing rules that force the LLM to guess, and cross-file dependency chains that increase round-trips and risk inconsistency.

The hook-injected content is `framework/concepts.md` only. See `framework/hooks.md` for the injection chain. Chain-reading evaluation in Question 6 must be scoped to that set. Hooks deliver `framework/concepts.md` directly, and user project entry files (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`) are the user's own project instructions, not part of the governance instruction chain.

1. whether Agent-facing instruction files (framework documentation, hook-injected content) are self-contained or require chain-linked reading across multiple files to obtain essential phase instructions
2. whether each phase delivers a self-contained instruction pack or assumes the executor inherits context and decisions from prior phases
3. whether the design expects the executor to hold governance context across specFlow process phases versus reloading it fresh per phase
4. whether each document that declares optional or conditional inputs defines the default behavior for every branch, or whether any branch is left undefined — forcing the executor to invent a fallback, stop and ask, or silently guess

For Question 7, the score basis must explicitly evaluate:

1. whether the design uses the minimum file surface needed for each phase or requires the executor to read files that contain only meta-governance content not relevant to the current action. For LLM executors, file surface cost is primarily context-window consumption rather than reading effort. Unnecessary file inclusions dilute relevant instructions and reduce available context for code and user input, but reading a few extra paragraphs has negligible cost on its own.
2. whether rules that could be tool-enforced are implemented in `specflowctl` or left as Agent-discipline-only
3. whether the design has chain-reading requirements (read file A → A links to file B → B links to file C) for essential instructions. For LLM executors, the primary cost of chain-reading is not reading effort but: (1) each file hop adds a tool-calling round-trip, (2) cross-file instruction inconsistency is harder to detect, (3) interrupted chains (missing file, changed path) cause execution failures. Self-contained instruction packs avoid these costs entirely.

`entry_control_chain_check` is mandatory for every `spec_flow_design_review`.
It judges whether human entry documents and their first-owner links act as executable entry control for an executor without project-specific context.
It is not a business scenario checklist and must not depend on product, integration, vendor, or domain examples.
It does not create another review flow, score question, score group, baseline slice, run-state field, or CLI.

The `entry_control_chain_check result` must be one of:

1. `passed`
2. `blocked`
3. `incomplete`

The check must judge these abstract capabilities:

1. `startup_entry_control`
   - the hook-injected content (`framework/concepts.md`) acts as the first control point for governed work; the opening rules (key terms, triggers, HARD RULES) make the next action clear
2. `first_owner_selection`
   - before any specFlow action or repository mutation, `concepts.md` tells the executor how to choose the first owning file or trigger (`validate`, `verify`, `promote`)
3. `owner_only_continuation`
   - after the first trigger routes the request, `concepts.md` requires the executor to follow only the routed operation or governance path
4. `pre_action_permission_gate`
   - before proposing or mutating implementation-side files, `concepts.md` requires the executor to read the relevant spec first (HARD RULE 1)
5. `hard_stop_clarity`
   - unclear intent, state, owner, boundary, or conflicting rules force a stop instead of a guessed write or guessed route
6. `owner_reachability`
   - `concepts.md` must expose enough first-level routes for the executor to reach the specFlow process, migration, governance review, or rule-governance owner
7. `no_ad_hoc_flow_substitution`
   - `concepts.md` must forbid replacing the standard triggers with custom reconciliation, audit, or gap-review flows
 8. `entry_robustness_probe`
    - the hook injection design must be verified with tool-neutral probes that judge observed routing and mutation control
    - primary: run by an independent executor without project-specific context
    - fallback: conducted by the current reviewer using the same abstract prompt families and record fields, with `executor_independence` marked as `fallback_reviewer_conducted`

The `entry_robustness_probe` must use abstract prompt families that test the hooks injection path and `concepts.md` instruction source, not product, integration, vendor, or domain examples:

1. `mixed_intent_prompt`
2. `chat_claimed_state_prompt`
3. `skip_owner_prompt`
4. `custom_flow_substitution_prompt`
5. `clean_implementation_only_control_prompt`
6. `missing_hooks_prompt` — tests behavior when hooks injection files are absent
7. `stale_concepts_prompt` — tests behavior when `concepts.md` content is outdated

For each prompt family, the probe must record:

1. `prompt_family`
2. `expected_control`
3. `observed_first_owner`
4. `diagnostic_allowed`
5. `mutation_allowed`
6. `result`
7. `failure_class`
8. `probe_source`
9. `executor_independence`

Allowed probe evidence may come from an independent agent session, reviewer role-play, recorded replay harness, local multi-executor tool, or manual black-box exercise.
Allowed `probe_source` values are `independent_agent_session`, `reviewer_role_play`, `recorded_replay_harness`, `local_multi_executor_tool`, and `manual_black_box_exercise`.

`executor_independence` has two allowed values:

1. `confirmed_independent_no_project_specific_context` — primary mode. The probe is run by an executor that has never seen this project's framework or workspace. Any of the five probe source types may be used, as long as the independence requirement is satisfied.

2. `fallback_reviewer_conducted` — fallback mode. The current reviewer runs the 7 abstract prompt families directly in the current session, using the same record fields. The reviewer must document that this is fallback evidence and why the primary mode was not feasible (e.g. no independent executor available, platform constraint, session limitation).

When `executor_independence` is `fallback_reviewer_conducted`:
- the probe still provides useful evidence about observable first-owner routing, diagnostic allowance, and mutation control
- the `entry_control_chain_check result` may be `passed` with a fallback note
- the conclusion must state that robustness was verified via fallback and recommend (but not require) a follow-up independent probe

If a prompt-family record lacks `executor_independence` or cannot prove either allowed value, that capability lacks enough in-scope evidence and the `entry_control_chain_check result` is `incomplete`.
The standard is tool-neutral: it must not require a specific platform capability, tool command, or command name.

Probe failure classes are:

1. `wrong_first_owner`
2. `mutation_leak`
3. `chat_truth_trusted`
4. `custom_flow_accepted`
5. `implementation_gate_overmatch`

Completion rules:

1. if the check is not performed, the review is incomplete and must not issue `pass`, `pass-with-optimization`, or `blocked`
2. if any capability is `blocked`, the check result is `blocked` and the hard-blocker rules must be evaluated before any scoring-based pass claim
3. if any capability lacks enough in-scope evidence, the check result is `incomplete` and the review must stop or add a dynamic risk slice before final conclusion
4. a passing check must explain how the result affects Questions 6, 7, and 8
5. when all capabilities pass and only `entry_robustness_probe` uses `fallback_reviewer_conducted`, the check result is `passed` with a fallback note; the review must document that robustness was verified via fallback and why primary was not feasible

When reflecting this check into Questions 6, 7, and 8:

1. Question 6 judges whether an executor without project-specific context can find the first owner, next action, and stop points from the entry control text.
2. Question 7 judges whether route specificity and diagnostic allowance scale the entry cost with work risk instead of forcing every request through a hard stop.
3. Question 8 judges whether the required startup reading buys repeatable execution control that is worth its cost.

`routine_work_path_check` is mandatory when any of Questions 6, 7, or 8 is expected to score below `4` because of instruction loading cost, rule weight, routine-work path cost, mandatory cross-file navigation, heavy gate structure, or pre-action instruction overhead that reduces available context for substantive work.

When triggered, `routine_work_path_check` must review these representative paths before any `pass` or `pass-with-optimization` conclusion:

1. routine implementation-only work
   - pure tests, logging, observability, mechanical refactor, or wording-only implementation support that does not change formal behavior truth
2. implementation repair under existing truth
   - a repair where current Spec truth already defines the intended behavior and the requested change should not invent new behavior
3. behavior, boundary, or acceptance change
   - a request that may change durable behavior truth, object ownership, acceptance criteria, rule truth, or end-to-end verification claims

For each reviewed path, the review must state:

1. the current pre-action read chain
2. the `B. Lightweight Pre-Action Prohibitions`
   - the three to five rules that must be known before action because violating them would create a risk that cannot be reliably detected and repaired after action
3. the `D. Minimum Allowed Action`
   - the smallest action the current request explicitly authorizes, with no scope expansion, new behavior, boundary change, acceptance change, or incidental repair beyond that request
4. the `E. Automatic Impact Check`
   - the automatic checks, deterministic checks, or fixed evidence-review procedures that can carry rule enforcement after action, including path ownership, rule truth impact, and reroute to review
   - a free-form reviewer assertion does not count as `E. Automatic Impact Check` unless it is backed by a named deterministic check or fixed evidence-review procedure
5. the timing decision for every rule currently consumed before action:
   - `action_before_hard_rule`
   - `on_demand_rule_lookup`
   - `post_action_check`
6. the concrete control that would be lost if a rule currently kept before action were moved later

Completion rules:

1. if `routine_work_path_check` is triggered but not performed, the review is incomplete and must not issue `pass`, `pass-with-optimization`, or `blocked`
2. if the check finds a smaller safe path and the final conclusion reports `optimization result: none`, the review is incomplete
3. if the check finds routine work forced through full pre-action rule consumption and no smaller B/D/E path exists in the design, the review must report a hard blocker
4. if the check cannot prove whether a rule is truly pre-action or safely later-consumable, classify that path as `possible_optimization_evidence_missing`; a `pass` conclusion must explain why the missing evidence does not hide an unsafe heavy path
5. if the check proves that every pre-action rule is `action_before_hard_rule` for the reviewed paths, the review may still pass when the fixed pass gate also passes

When a review identifies avoidable rule weight, classify the affected rule text as exactly one of these classes:

1. `hard_rule`
   - the rule is required because removing it could change durable truth, object ownership, process advancement, implementation permission, rule truth, or end-to-end verification claims
2. `judgment_guidance`
   - the rule helps an executor choose a route or evaluate risk, but it does not itself authorize writes, forbid writes, or define a stop condition
3. `example_or_wording`
   - the text exists only to make the rule easier to understand, and it is justified only when it removes a real execution ambiguity
4. `duplicate_or_restatement`
   - the text repeats another owner file without adding a local allowed action, forbidden action, stop condition, output requirement, or scoring consequence
5. `overweight_rule`
   - the rule forces a heavier path than the work risk requires, forces pre-action consumption when `on_demand_rule_lookup` or `post_action_check` would preserve the same control, or applies a specialized structure to routine work where no durable-truth, ownership, or end-to-end verification risk needs that structure

Rules:

1. accuracy has priority over lightness when a rule prevents durable truth drift, unsafe implementation, ambiguous ownership, or skipped verification
2. lightness has priority once the same execution safety is already provided by another owner rule or by a smaller path
3. do not treat a rule as overweight only because it is long; treat it as overweight only when the extra reading or execution burden does not produce a distinct control gain
4. do not preserve duplicate text merely because it is familiar; preserve it only when deleting it would make the executor guess an owner, boundary, dependency, next action, or scoring consequence
5. if an in-scope rule is classified as `overweight_rule` or `duplicate_or_restatement` and does not create a hard blocker or finding, it must be reported as a non-blocking optimization instead of being hidden inside a residual score weakness

Question-specific scoring rules:

1. Question 1 must judge:
   - whether the target problem is explicit
   - whether that problem is real in repository work rather than self-created by the mechanism
   - whether the design solves written project record and implementation alignment instead of inventing a false rule that every code edit must change spec documents
   - whether the mechanism still has distinct value instead of duplicating another existing control
2. Question 2 must judge:
   - whether ownership and repair landing points are explicit
   - whether boundaries stay natural rather than artificially split
   - whether current object shape avoids repeated cross-object truth stitching
3. Question 3 must judge:
   - whether each specFlow process step corresponds to a real information change
   - whether the order reduces uncertainty rather than merely renaming state
   - whether the current sequence (next→review→promote) remains the smallest stable path
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
   - whether the executor can determine the current phase and position from official documents alone, without relying on inherited context from prior phases
   - whether the next step and the rationale for that step are directly derivable from the current phase's instruction pack
   - whether the official documents, rather than external reasoning or assumptions, carry the needed orientation
   - whether entry documents explain the basic meaning of `specFlow` and its core terms before relying on internal command or process concepts
   - whether the entry control chain lets an executor identify the current route, next owner, and stop point from official entry documents alone
   - whether in-scope rules contain avoidable internal mechanism detail that could be removed or relocated without reducing the governing rules' clarity — for LLM executors, superfluous text competes with relevant instructions for context window rather than causing "learning burden"
   - whether `judgment_guidance`, `example_or_wording`, and `duplicate_or_restatement` content is kept small enough that it does not obscure the governing hard rules
   - whether pre-action instruction loading is limited to `action_before_hard_rule` material, so that the executor's context window is occupied primarily by rules that prevent unsafe writes or truth drift — rules that could safely be consumed on demand or checked after action should not pre-occupy context during the action phase
7. Question 7 must judge:
   - whether small changes have a smaller legal path than large changes
   - whether routine work avoids over-processing (fewer tool-calling round-trips, less context-window consumption)
   - whether the mechanism's operational steps scale with actual work size
   - whether code-only or implementation-only work has a smaller legal path when written truth already constrains one safe implementation result
   - whether entry pre-action controls scale with work risk instead of sending every request through the same path
   - whether `hard_rule` requirements are limited to cases where durable truth, ownership, or end-to-end verification risk actually requires them
   - whether a specialized structure is optional or conditional when the current work does not need that structure for safe closure
   - whether routine work avoids mandatory full-chain pre-execution loading when an `on_demand_rule_lookup` or `post_action_check` path would preserve the same safety — for LLM executors, "pre-reading" cost is primarily context-window consumption and round-trip latency, not reading fatigue
8. Question 8 must judge:
   - whether the control gained is visible and repeatable
   - whether the documentation maintenance, executor instruction overhead, and specFlow execution cost stay proportionate to that gain — for LLM executors, the relevant cost model includes context-window consumption by governance instructions, tool-calling round-trips per phase, and the risk of cross-file instruction inconsistency; it does not include "learning curve" or "document length" as significant factors
   - whether the mechanism still looks worth maintaining over time
   - whether entry instruction pre-loading (the cost of loading governance context before action) prevents unsafe mutation without pushing legitimate code-only work into a truth-change process
   - whether the entry control chain's instruction cost produces repeatable control over authority resolution and pre-mutation permission
   - whether each required pre-action instruction load produces a distinct control gain that cannot be preserved by `on_demand_rule_lookup` or `post_action_check`
   - whether the recommended repair for excess rule instruction weight is the smallest correct one: keep as `action_before_hard_rule`, downgrade to `on_demand_rule_lookup`, convert to `post_action_check`, keep only as `example_or_wording`, merge or link as `duplicate_or_restatement`, or remove or narrow an `overweight_rule`

When Question 6, 7, or 8 scores below `4`, the review must classify each cited weakness as one of:

1. `acceptable residual weakness`
   - use when the weakness is real but no clear, smaller, safe optimization is available without losing needed control
2. `non-blocking optimization`
   - use when a clear improvement exists, the current design still passes, and the issue does not trigger a hard blocker or pass-gate failure

The score basis for Questions 6, 7, and 8 must state which category applies.
If no non-blocking optimization is reported while any of those questions scores below `4`, the output must explain why every cited weakness is only an `acceptable residual weakness`.

### 7.2 Fixed Question Groups

The fixed score groups are:

1. `design_foundation`
   - Questions `1`, `2`, and `3`
2. `control_effectiveness`
   - Questions `4` and `5`
3. `executor_operability`
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
2. boundary or process design leaves repair ownership or repair landing point unstable
3. any key gate has Question `4 = 0`
4. the mechanism clearly rewards surface compliance over real risk reduction
5. the primary executor (LLM agent) cannot rely on the official documents alone to know current position, next step, and why that step is next
6. the mechanism forces simple changes through a full heavy path when the work does not change durable truth, object ownership, or end-to-end verification obligations, and the design provides no smaller legal path
7. the mechanism forces simple changes through full pre-action rule consumption when a smaller `action_before_hard_rule`, `on_demand_rule_lookup`, or `post_action_check` path would preserve the same control, and the design provides no smaller legal path
8. a triggered `routine_work_path_check` proves that routine implementation-only work cannot be handled with lightweight pre-action prohibitions plus automatic impact checks, and the design provides no smaller legal path
 9. `framework/concepts.md` (the hook-injected instruction source) lets an executor propose implementation-side work or mutate implementation-side repository files before selecting the first owner and proving implementation permission
10. `framework/concepts.md` does not require stop-and-reroute when execution discovers possible truth, ownership, rule, or implementation-permission impact
11. `framework/concepts.md` instructs the executor to treat every code edit as a spec change or full-process entry, and the design provides no smaller implementation-only legal path
12. `framework/concepts.md` opens with background explanation or classification without a first-owner action rule, so the executor cannot immediately know which trigger to use first
13. `framework/concepts.md` lets an executor replace the standard triggers with an ad hoc reconciliation, audit, alignment, gap-review, or similarly named intermediate flow
14. `framework/concepts.md` forbids read-only inspection or verification needed to classify a request and provides no smaller legal diagnostic path before mutation
15. the `entry_robustness_probe` shows that an independent executor can propose implementation-side work or mutate implementation-side repository files before selecting the first owner and proving implementation permission
16. the `entry_robustness_probe` shows that the entry design trusts chat-only state or permission, accepts a custom intermediate flow as owner, or displaces an exact route
17. the `entry_robustness_probe` shows that implementation-change overmatching hides the real owner for work that already needs owner judgment; if the result is conservative overblocking without mutation leakage, report a finding or optimization instead of a hard blocker unless no smaller legal diagnostic path exists
18. the design requires the executor to perform chain-linked reading (read file A, then follow a link to file B for essential instructions, then follow a link from B to file C) for the core execution path of any specFlow process step, and the design provides no self-contained alternative path

### 7.5 Pass Gate

If no hard blocker exists, passing still requires all of the following:

1. no individual question score is below `2`
2. every fixed group average is at least `2.5`
3. `weighted_score` is at least `75`
4. the `entry_control_chain_check` is complete and its result is reflected in the hard-blocker result, findings result, optimization result, and Question 6, 7, and 8 score bases
5. when `routine_work_path_check` is triggered, the check is complete and its result is reflected in the hard-blocker result, findings result, optimization result, and Question 6, 7, and 8 score bases

When these pass-gate conditions hold:

1. use `pass` only when no non-blocking optimization exists
2. use `pass-with-optimization` when at least one non-blocking optimization exists

Otherwise the result is `blocked`.

## 8. Output Contract

This output contract applies to every `spec_flow_design_review`.

The output must report at least:

1. `review scope`
2. `review layout`
3. `framework root`, `template root`, `tooling root`, and `project-instance compatibility mode`
4. whether full-scope run state was created, reused, or deleted and recreated
5. the run-state file path
6. `review_plan`
7. the fixed review blocks used
8. the file coverage per block
9. the baseline slice table and slice statuses
10. the dynamic risk slice table and slice statuses, or explicit `none`
11. the score-state table
12. the stale slice result
13. the hard-blocker result
14. the `entry_control_chain_check result`:
   - must be `passed`, `blocked`, or `incomplete`
   - report evidence for all checked capabilities and the hook injection path (hooks files present, session-start script correctness, concepts.md completeness)
   - report probe evidence using `prompt_family`, `expected_control`, `observed_first_owner`, `diagnostic_allowed`, `mutation_allowed`, `result`, `failure_class`, `probe_source`, and `executor_independence`
   - report the impact on Questions 6, 7, and 8
15. the `routine_work_path_check` result:
   - report `not_triggered` when the trigger condition did not apply
   - otherwise report each reviewed path, current pre-action read chain, B/D/E judgment, timing decisions, lost-control analysis, and whether the check found a hard blocker, non-blocking optimization, or missing evidence
16. all eight question scores, each with:
   - `score`
   - `score_basis`
   - `evidence`
17. the fixed group averages
18. the `weighted_score`
19. the cross-block convergence results
20. the findings result:
   - explicit `none` when no real finding exists
   - otherwise every finding must satisfy Section 8.1
21. the optimization result:
   - explicit `none` when no non-blocking optimization exists
   - otherwise every optimization must satisfy Section 8.2
22. when the final conclusion is `pass-with-optimization`, `why pass still holds`
23. when Question 6, 7, or 8 scores below `4` and no non-blocking optimization is reported for that question, the acceptable residual weakness explanation
24. the final conclusion:
   - `pass`
   - `pass-with-optimization`
   - `blocked`

If the output does not explicitly report Items 13 through 21, the review is not complete.
If the final conclusion is `pass-with-optimization` and the output omits Item 22, the review is not complete.

### 8.1 Finding Contract

When `spec_flow_design_review` reports a real finding, that finding must be written as one self-contained repairable unit.
A finding is reserved for a blocking design problem or a design problem that changes hard-blocker, score, group-average, weighted-score, or pass-gate judgment.
Non-blocking improvements must be reported under Section 8.2 instead of being mixed into findings.

The minimum required fields are:

1. `title`
2. `severity`
   - required for every real finding and must be one of `P0`, `P1`, `P2`, or `P3`
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

1. `severity` must satisfy the shared explanation baseline from `<framework-root>/severity_policy.md`
2. `severity` explains design harm; it does not replace the fixed score model
3. `score` explains current design quality; it does not replace explicit blocking judgment
4. `P0` and `P1` are normally blocking; `P2` and `P3` are normally non-blocking unless the finding explains why the current review must stop
5. `recommended fix` must be specific enough that later repair work can execute it without a second clarification round
6. when a finding concerns avoidable rule weight or incorrect rule-consumption timing, `recommended fix` must state the smallest correct repair: keep as `action_before_hard_rule`, downgrade to `on_demand_rule_lookup`, convert to `post_action_check`, keep only as `example_or_wording`, merge or link as `duplicate_or_restatement`, or remove or narrow an `overweight_rule`
7. when a finding comes from `routine_work_path_check`, `recommended fix` must state whether the minimal repair is a lighter B rule set, a narrower D action boundary, an E automatic impact check, or a hard pre-action stop that the current design failed to isolate
8. when no real finding exists, the output must say so explicitly instead of omitting the finding section

### 8.2 Optimization Contract

When `spec_flow_design_review` reports a non-blocking optimization, that optimization must be written separately from findings.

A non-blocking optimization is allowed only when all of the following hold:

1. no hard blocker is triggered by the issue
2. the fixed pass gate still passes
3. the issue has a clear smaller improvement that preserves required governance control
4. the issue affects Question 6, 7, or 8, or concerns an in-scope `overweight_rule` or `duplicate_or_restatement`

The minimum required fields are:

1. `title`
2. `affected_questions`
3. `rule_weight_class`
4. `why non-blocking`
5. `recommended optimization`
6. `why this is the smallest correct optimization`
7. `evidence`

Rules:

1. `why non-blocking` must state why the issue does not trigger a hard blocker, score below `2`, group average failure, or weighted-score failure
2. `recommended optimization` must cut only the unnecessary rule weight or move it to the smallest safe consumption timing; it must not weaken a rule that prevents durable truth drift, ownership drift, implementation permission drift, shared or system drift, skipped verification, or false closure
3. when an optimization comes from `routine_work_path_check`, `recommended optimization` must name the affected path and specify whether the safe lighter shape is a smaller B rule set, `on_demand_rule_lookup`, `post_action_check`, or an added E automatic impact check
4. if the final conclusion is `pass-with-optimization`, at least one optimization item is required
5. if no non-blocking optimization exists, the output must say `optimization result: none`

## 9. Non-Goals

This flow does not:

1. replace `spec_flow_review`
2. replace the rule-governance branch
3. review business truth by default
4. review tooling source or binaries by default
5. create a new command chain

# Scenario Check Command

## 1. Purpose

`scenario_check:{scenario}` checks whether the current candidate scenario truth is sufficiently closed to constrain later end-to-end verification.

## 2. Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `scenario_check`-local entry, output, stop, and fresh-rerun rules.

Process-file writeback and validation for `_check_result/scenario/{scenario}.md` must follow `specflow/framework/process_snapshot_contract.md` Section 9. When deterministic snapshot validation tooling is available for scenario check process files, the matching `snapshot validate-process` command is the mandatory tool-backed validation step before reporting a pass gate or lifecycle advance.

`scenario_check` is not a failed-result storage command.
It writes `_check_result/scenario/{scenario}.md` only when the current candidate scenario passes.

Result semantics for non-pass conclusions are fixed:

1. `blocked`
   - use when the smallest correct next step requires user clarification, user decision, rule-truth closure, repository mapping correction, or another upstream governance repair outside this command's direct local repair surface
   - if the blocker changes scenario behavior truth, the answer must be written back before `scenario_check` may pass
2. `fix_required`
   - use when the executor can already identify a concrete truth-side repair inside the current candidate scenario, scenario evidence appendix, or explicit binding surface
   - no extra user choice is needed before that repair work starts
   - after the repair, the scenario must return to a fresh full-scope `scenario_check`

After `blocked` or `fix_required`, later repair work is non-authoritative until a fresh full-scope `scenario_check` run is entered through command routing.
Scoped confirmation of the repaired fragment must not write `_check_result/scenario/{scenario}.md` or advance `_status.md` to `scenario_verify`.

## 3. Preconditions

1. `_status.md` says `Object Type=scenario`, `Active Layer=candidate`, `Next Command=scenario_check`
2. current candidate scenario file exists
3. read `specflow/framework/candidate_handoff_contract.md`
4. read `specflow/framework/onboarding_decision_policy.md`

## 4. Procedure

1. read current candidate scenario truth and `docs/specs/repository_mapping.md`
2. verify required bindings are explicit:
   - `source_basis`
   - `evidence_appendix_ref`
   - `repository_mapping_ref`
   - `unit_refs`
   - `rule_refs`
3. process candidate source fields using `onboarding_decision_policy.md`:
   - if `source_basis=existing_implementation` or `source_basis=mixed`, `evidence_appendix_ref` must point to an existing scenario evidence appendix and that appendix must be read
   - if `source_basis=new_design` or `source_basis=replacement`, `evidence_appendix_ref` must be `none`
   - evidence appendix conflicts or unknowns that still affect selected scenario behavior block pass unless the candidate scenario main Spec explicitly makes a bounded selected rule that no longer depends on them
4. verify `repository_mapping_ref` matches the current repository mapping
5. verify entry, path, exit, and failure absorption are explicit enough to verify
6. verify content organization: the candidate scenario main Spec and its appendix files must satisfy `spec_writing_guide.md` Section 6
7. verify explicit scenario acceptance items:
   - the scenario must contain a `Testability / Acceptance Criteria` section, or an explicitly equivalent acceptance section title
   - each acceptance item must record `id`, `target`, `verification_surface`, `implementation_surface`, `verification_method`, and `pass_condition`
   - `verification_surface` must use only the fixed values from `spec_writing_guide.md` Section 5
   - scenario-level `integration` items must name the runnable trigger-to-outcome entrypoint or mark the item as `not_runnable_yet` with a concrete missing-entrypoint reason
   - broad wording such as "end-to-end works", "all units are connected", or "the flow is integrated" is not enough unless the required fields make the scenario directly verifiable
   - if an item is marked `not_runnable_yet`, `scenario_check` may treat the item as explicitly bounded only when the reason is concrete and the scenario does not use that same item as a current pass claim
   - missing, vague, incomplete, or falsely passing acceptance items can only result in `blocked` or `fix_required`
8. if pass, write `_check_result/scenario/{scenario}.md` so it satisfies the `scenario_check -> scenario_verify` handoff in `specflow/framework/candidate_handoff_contract.md`, including the accepted acceptance-item set by `id`, `verification_surface`, and `not_runnable_yet` state, then advance `Next Command=scenario_verify`
9. if not pass:
   - conclude `blocked` or `fix_required`
   - do not write a failed `_check_result/scenario/{scenario}.md`
   - delete an old `_check_result/scenario/{scenario}.md` when it no longer covers the current candidate scenario, repository mapping, bound units, bound Rule files, or formal global baseline state
   - keep `_status.md` at `Next Command=scenario_check`
   - report the standardized `fallback_reason_code` first, then the natural-language explanation

## 5. Stop Conditions

1. whether the candidate scenario has enough trigger, path, outcome, and failure-absorption truth for end-to-end verification is clear
2. whether candidate source and evidence appendix requirements are satisfied is clear
3. whether repository mapping, bound units, bound Rule files, and formal global baseline bindings still match current truth is clear
4. whether the explicit scenario acceptance-item set is specific enough for downstream verification is clear
5. if the round passes, `_check_result/scenario/{scenario}.md` holds the current pass gate
6. if the round does not pass, no invalid old scenario check gate remains
7. `_status.md` points to the real next executable step
8. no repair-side reassessment or scoped follow-up review has been mistaken for a formal `scenario_check pass`

## 6. Output Contract

The output must report:

1. `check gate result`
2. candidate source and evidence appendix result
3. acceptance-item completeness result
4. `_check_result/scenario/{scenario}.md` write, delete, or keep result
5. stale old gate cleanup decision when a previous `_check_result/scenario/{scenario}.md` exists
6. `_status.md` update result
7. `fallback_reason_code` for `blocked`, `fix_required`, or checkpoint stops
8. structured findings when the result is `blocked` or `fix_required`
9. whether any follow-up repair confirmation was non-authoritative and did not change lifecycle state
10. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6

When the result is `blocked` or `fix_required`, findings must be structured and must not be reduced to vague summaries.

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `truth_incomplete`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `rule_drift`
6. `shared_truth_conflict`
7. `governance_drift`

Candidate source-field and evidence-appendix blockers must use `truth_incomplete`.
Old gate mismatch caused by current candidate scenario truth changes must use `truth_drift`.
Repository mapping or bound-unit snapshot mismatch must use `binding_drift`.
Formal global baseline mismatch must use `baseline_drift`.
Bound Rule mismatch must use `rule_drift`.
Confirmed duplicate rule truth must use `shared_truth_conflict`.
Missing or contradictory required governance rules must use `governance_drift`.
The output must name the exact source, evidence, binding, or governance condition in the natural-language explanation after the standardized code.

## 7. Non-Goals

1. implementation planning
2. direct code editing

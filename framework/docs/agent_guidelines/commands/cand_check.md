# Candidate Check Command

## 1. Purpose

This command checks whether a module's `candidate` Spec is sufficiently closed to support stable downstream planning and implementation.

It is a review action, not a "store failed review results" action.

By default, closure means all of the following:

1. `progressability`
   - the module behavior is clear enough to enter `cand_plan`
   - main flow, key protocols, key boundaries, error semantics, and acceptance criteria are strong enough to prevent planning or implementation divergence
2. `content completeness`
   - the candidate has formally acknowledged key behavior truth that affects implementation results
   - key decisions are not left outside the Spec in chat context, README vision, oral consensus, or author memory
3. the candidate is still aligned with the current formal global baseline state

## 2. Scope

By default this command reviews:

1. whether `progressability` holds
2. whether `content completeness` holds
3. whether `Global Constraint Alignment` is explicit and internally consistent
4. whether bound Shared Appendix relations and body dependencies are consistent
5. whether `system_constraints_stable_ref` matches the current formal global baseline state
6. whether shared-candidate signals require suggesting `shared_extract_review` or directly reporting a dual-source-of-truth conflict
7. whether the remaining blocker is actually a user-intent clarification or decision-point that must be written back before closure can pass

`cand_check` is not a "minimum can-move-forward review."
`cand_check pass` always means:

1. the current candidate may enter `cand_plan`
2. the current candidate already contains the key constraints needed as the truth input for implementation in this round

Result semantics for non-pass conclusions are fixed:

1. `blocked`
   - use when the smallest correct next step cannot be completed by executor-side repair alone in the current round
   - the blocker is waiting on user clarification, user decision, or shared-truth closure outside the active command's direct repair surface
   - if the blocker changes behavior truth, the answer must be written back before `cand_check` may pass
2. `fix_required`
   - use when the executor can already identify a concrete truth-side repair inside the current candidate, appendix, or explicit binding surface
   - no extra user choice is needed before that repair work starts
   - after the repair, the module reruns `cand_check` instead of skipping forward

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=cand_check`
3. the module has `candidate`
4. read explicitly referenced candidate appendix files and bound Shared Appendix files
5. read `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
6. if `docs/project_standards/_registry.md` exists, read it and any registered project-local standard files consumed by `cand_check` for the current target
7. if `docs/project_standards/_registry.md` is missing, stop and report governance drift according to `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
8. if this round may raise a checkpoint, read `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
9. if `_check_result/{module}.md`, `_status.md`, candidate truth, or other commit-triggering governance files may change, read the git policy first
10. if referenced appendix files have directory drift, fix that first and rerun the pre-check

## 4. Procedure

1. read `docs/specs/candidate/c_{module}.md` plus all required appendix and Shared Appendix files
2. if `stable` exists, also read `docs/specs/stable/s_{module}.md` plus required stable appendix files
3. read `docs/specs/system/stable/s_system_constraints.md` if it exists; otherwise continue with the "no formal global baseline yet" state
4. judge `progressability`
5. judge `content completeness`
6. classify completeness gaps into:
   - `critical`
   - `important`
   - `elaboration`
7. use these fixed completeness review objects:
   - `Behavior Basis Completeness`
   - `Decision Surface Completeness`
   - `Acceptance Basis Completeness`
8. if registered project-local review standards apply to the current module and command surface, consume them only according to `docs/project_standards/_registry.md` and only as tightening or clarifying inputs
9. process `system_constraints_stable_ref`:
   - if the formal global baseline exists and the candidate is still compatible, a mechanical update to the current version is allowed
   - if incompatible, the result can only be `blocked` or `fix_required`
   - if the formal global baseline does not exist, `system_constraints_stable_ref` must be `none`
10. process `shared_appendix_refs`:
   - if current behavior depends on Shared Appendix truth but bindings are missing or incomplete, the result can only be `blocked` or `fix_required`
   - if bindings exist but the body does not explain which behavior chain reuses them, the result can only be `blocked` or `fix_required`
11. process shared-candidate signals:
   - by default, shared-candidate hints only trigger a suggestion to run `shared_extract_review`
   - if the current required reading range already confirms a dual source of truth, report it directly as a blocking issue with `fallback_reason_code=shared_truth_conflict`
12. determine whether a blocking checkpoint is the correct stop form:
   - use `clarification` when user intent, boundary meaning, or acceptance meaning is still missing from truth
   - use `decision` when multiple materially different directions remain and the user must choose one
13. checkpoint rules:
   - a checkpoint is not `pass`
   - if a checkpoint conclusion changes behavior truth, it must be written back to candidate or appendix before `cand_check` may be rerun
   - do not write `_check_result/{module}.md` for checkpoint-only stops
14. merge conclusions in this order:
   - `progressability`
   - `content completeness`
   - overall gate conclusion
15. merge rules:
   - if `progressability` fails -> only `blocked` or `fix_required`
   - if any `critical` completeness gap exists -> only `blocked` or `fix_required`
   - if only `important` or `elaboration` issues remain, `pass` is still possible
16. if the result is `pass`, create or update `docs/specs/_check_result/{module}.md`
17. if the result is not `pass`, do not write a failed `_check_result/{module}.md`; delete an old pass gate if it is no longer valid
18. update `_status.md`:
   - if pass -> `Next Command=cand_plan`
   - otherwise -> `Next Command=cand_check`
19. perform git close-out if required

## 5. Stop Conditions

1. whether the candidate satisfies both `progressability` and `content completeness` is clear
2. if the round passes, `_check_result/{module}.md` holds the pass gate
3. if the round does not pass, no invalid old pass gate remains
4. `_status.md` is updated

## 6. Output Contract

The output should include:

1. overall conclusion
2. severity summary
3. formal global baseline alignment result
4. the two-threshold conclusion:
   - `progressability`
   - `content completeness`
   - overall gate conclusion
5. whether `Check Result Snapshot` was written back or an old gate was cleaned up
6. `checkpoint result` when a checkpoint stop was raised
   - when present, it must satisfy the fixed checkpoint fields defined by `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
7. `fallback_reason_code` for blocked, fix-required, or checkpoint stops
8. structured findings when `blocked` or `fix_required`
9. next-step suggestion
10. git close-out result
11. `_status.md` update result

When the result is `blocked` or `fix_required`, findings must be structured and must not be reduced to vague summaries.

Severity must use the shared meanings defined in:

1. `specflow/framework/docs/agent_guidelines/severity_policy.md`

Each finding must explain:

1. background
2. what happened
3. impact
4. best recommendation
5. why that recommendation is best
6. whether it is blocking
7. which constraint layer it belongs to

Allowed checkpoint types:

1. `clarification`
2. `decision`

Allowed `fallback_reason_code` values:

1. `truth_incomplete`
2. `baseline_drift`
3. `shared_appendix_drift`
4. `shared_truth_conflict`
5. `governance_drift`

## 7. Non-Goals

1. directly generating a plan
2. directly entering code implementation
3. creating, updating, or deleting an independent `system_constraints` candidate file

## 8. Examples

```md
cand_check:module_ai
```

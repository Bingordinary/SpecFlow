# Candidate Plan Command

## 1. Purpose

This command creates or updates the implementation plan for the current candidate.

## 2. Scope

By default it handles:

1. reading the current valid candidate pass gate
2. optionally running research preflight when implementation-critical unknowns still block a stable plan
3. deriving a stable-to-candidate change surface with `git diff` when the module already has `stable`
4. generating or updating `_plans/{module}.md`
5. keeping plan bindings aligned with the current candidate, current formal global baseline state, and current Shared Contract snapshot
6. stopping at a structured decision checkpoint only when key implementation direction is still not locked

### 2.1 Lifecycle-State Advance Inheritance

当本命令推进 `_status.md` 时，这个推进继承 `specflow/framework/docs/agent_guidelines/command_policy.md` 第 8.5 节定义的 authoritative / non-authoritative 中心契约。
Only a new independent full-scope run of `cand_plan` may produce that advancing result; later local confirmation, research-side reassessment, or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=cand_plan`
3. a current valid `docs/specs/_check_result/{module}.md` exists
4. the current candidate still aligns with the current formal global baseline state
5. read any explicitly referenced candidate appendix files and bound Shared Contract files
6. if this round may raise a checkpoint, read `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
7. read the git policy if commit-triggering files may change

## 4. Procedure

1. read the candidate Spec and all required appendix or Shared Contract files
2. read `s_system_constraints.md` if it exists
3. read `docs/specs/_check_result/{module}.md`
4. verify the pass gate bindings are still valid
5. if the pass gate is invalid, stop immediately:
   - delete `_check_result/{module}.md`
   - delete `_plans/{module}.md` if it exists
   - delete `_verify_result/{module}.md` if it exists
   - fall back `_status.md` to `cand_check`
6. if the module already has `stable`, derive a planning-aid change surface before judging the round:
   - use `git diff --no-index -- docs/specs/modules/stable/s_{module}.md docs/specs/modules/candidate/c_{module}.md`
   - use the diff to identify which candidate sections changed in this round and which implementation slices need direct focus first
   - do not treat the diff as a substitute for reading the full candidate, required appendix truth, or bound Shared Contract truth
   - do not assume unchanged lines are irrelevant, because unchanged candidate truth may still constrain implementation
   - if the module has no `stable` yet, skip this step
7. determine the planning result shape for this round before plan write-back:
   - every `cand_plan` run must end in exactly one of these result shapes: `plan-ready`, `truth-fallback`, `plan-blocked`, or `decision-checkpoint`
   - if research preflight is not required because implementation-critical unknowns are already sufficiently closed, treat the round as `plan-ready`
8. determine whether research preflight is required:
   - use it only when current candidate truth is already closed enough to investigate implementation, but key implementation-critical unknowns still prevent a stable plan
   - do not use research preflight to replace missing behavior truth, boundary truth, or acceptance truth in the candidate
   - if research confirms that the real blocker is incomplete candidate truth, do not continue planning and fall back to `cand_check`
9. after research preflight, allow only these three result shapes:
   - plan-ready: implementation-critical unknowns are closed enough to write a stable plan
   - truth-fallback: research found that candidate truth itself is still incomplete, so planning must fall back to `cand_check`
   - plan-blocked: candidate truth still stands, but planning is blocked on a clearly named external condition, further bounded research result, or human-supplied implementation fact
10. `decision-checkpoint` is a distinct result shape:
   - use it only when a `decision` checkpoint is actually raised because implementation direction is still unresolved
   - do not merge it into `plan-blocked`, because unresolved direction and missing implementation facts are different blocking causes
   - do not create or update `docs/specs/_plans/{module}.md`
   - keep `_status.md` at `cand_plan` unless the checkpoint answer must first be written back into candidate truth or appendix truth
11. if the result is `truth-fallback`:
   - delete `_check_result/{module}.md`
   - delete `_plans/{module}.md` if it exists
   - delete `_verify_result/{module}.md` if it exists
   - do not create or update `docs/specs/_plans/{module}.md`
   - update `_status.md` to `cand_check`
   - report `fallback_reason_code=truth_incomplete`
12. if the result is `plan-blocked`:
   - do not create or update `docs/specs/_plans/{module}.md`
   - keep `_status.md` at `cand_plan`
   - report `fallback_reason_code=implementation_unknown`
   - record the blocking point, the missing condition, and the exact resume signal
13. determine whether a `decision` checkpoint is required:
   - only use it when key implementation direction is still not locked
   - if the unresolved decision changes behavior truth, boundary truth, or acceptance truth, the resume path must go back to `cand_check` after writeback
   - do not treat the checkpoint as permission to continue without that writeback
14. if a `decision` checkpoint is raised:
   - set the result shape to `decision-checkpoint`
   - do not create or update `docs/specs/_plans/{module}.md`
   - keep `_status.md` at `cand_plan` when the unresolved decision is implementation-direction only
   - report `fallback_reason_code=direction_unresolved`
   - use `resume_next_step=cand_check` only when the checkpoint answer must first be written back into candidate truth or appendix truth
15. create or update `docs/specs/_plans/{module}.md` only when no checkpoint blocks planning and the result is `plan-ready`
16. ensure the plan records:
   - execution slices rather than one undifferentiated implementation block
   - for each slice: objective, file scope, dependencies, verification action, done condition, and current status
   - progress, blockers, and verification focus for this round
   - `spec_file_ref`
   - `spec_version_ref`
   - `spec_fingerprint`
   - `module_appendix_snapshot`
   - `system_constraints_stable_file_ref`
   - `system_constraints_stable_version_ref`
   - `system_constraints_stable_fingerprint`
   - `shared_contract_snapshot`
17. update `_status.md`:
   - if the candidate is now ready for implementation -> `Next Command=cand_impl`
   - if candidate truth drift was discovered -> `Next Command=cand_check`
   - if research preflight found candidate truth gaps -> `Next Command=cand_check`
   - if research preflight is blocked on implementation-critical unknowns but no truth rewrite is pending -> keep `Next Command=cand_plan`
   - if the result is `decision-checkpoint` and no truth writeback is pending -> keep `Next Command=cand_plan`
   - if a `decision` checkpoint stopped planning and no truth writeback is pending -> keep `Next Command=cand_plan`
18. perform git close-out if required

## 5. Stop Conditions

1. either a valid plan file exists for the current candidate truth, or planning stopped with no consumable plan artifact because of fallback, bounded blocking, or checkpoint
2. `_status.md` points to the real next step

## 6. Output Contract

1. planning conclusion
2. whether a plan file was written, updated, or intentionally not created because planning stopped at fallback, bounded blocking, or a checkpoint
3. plan binding result
4. stable-to-candidate change-surface review result when `stable` exists
5. research preflight result when research preflight was used
6. `handoff validation result`
7. cleanup result when planning fell back to `cand_check`
8. `checkpoint result` when a checkpoint stop was raised
   - when present, it must satisfy the fixed checkpoint fields defined by `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
9. `fallback_reason_code` for fallback, blocking, or checkpoint stops
10. blocking reason and resume signal when planning stayed at `cand_plan` without fallback
11. git close-out result
12. `_status.md` update result
13. `specflow/framework/docs/agent_guidelines/command_policy.md` 第 8.6 节要求的 `user-facing close-out block`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - when a checkpoint was raised or planning stayed blocked at `cand_plan`, also report `resume signal`
   - if `Next Command=cand_plan`, `why this next step` must explicitly state whether planning is waiting on implementation facts, unresolved direction, or truth writeback

Allowed checkpoint types:

1. `decision`

Allowed `fallback_reason_code` values:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_contract_drift`
6. `truth_incomplete`
7. `implementation_unknown`
8. `direction_unresolved`

## 7. Non-Goals

1. direct code implementation
2. direct verification
3. rewriting candidate truth

# Unit Plan Command

## 1. Purpose

This command creates or updates the implementation plan for the current candidate.

## 2. Scope

By default it handles:

1. reading the current valid candidate pass gate
2. optionally running research preflight when implementation-critical unknowns still block a stable plan
3. deriving a stable-to-candidate change surface with `git diff` when the unit already has `stable`
4. identifying the changed execution surfaces of this round and defining their convergence targets
5. generating or updating `_plans/active/{unit}.md`
6. writing or updating `_plans/draft/{unit}.md` when planning cannot yet produce a consumable active plan
7. keeping active-plan bindings aligned with the current candidate, current formal global baseline state, and current Shared Contract snapshot
8. stopping at a structured decision checkpoint only when key implementation direction is still not locked
9. ensuring the active plan is executable without chat context, placeholders, or unstated verification meaning

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/command_policy.md`.
Only a new independent full-scope run of `unit_plan` may produce that advancing result; later local confirmation, research-side reassessment, or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=unit_plan`
3. a current valid `docs/specs/_check_result/unit/{unit}.md` exists
4. the current candidate still aligns with the current formal global baseline state
5. read any explicitly referenced candidate appendix files and bound Shared Contract files
6. if this round may raise a checkpoint, read `specflow/framework/checkpoint_protocol.md`
7. read the git policy if commit-triggering files may change

## 4. Procedure

1. read the candidate Spec and all required appendix or Shared Contract files
2. read `system_constraints.md` if it exists
3. read `docs/specs/_check_result/unit/{unit}.md`
4. verify the pass gate bindings are still valid
5. if the pass gate is invalid, stop immediately:
   - delete `_check_result/unit/{unit}.md`
   - delete `_plans/draft/{unit}.md` if it exists
   - delete `_plans/active/{unit}.md` if it exists
   - delete `_verify_result/unit/{unit}.md` if it exists
   - fall back `_status.md` to `unit_check`
6. if the unit already has `stable`, derive a planning-aid change surface before judging the round:
   - use `git diff --no-index -- docs/specs/units/stable/s_unit_{unit}.md docs/specs/units/candidate/c_unit_{unit}.md`
   - use the diff to identify which candidate sections changed in this round and which implementation slices need direct focus first
   - do not treat the diff as a substitute for reading the full candidate, required appendix truth, or bound Shared Contract truth
   - do not assume unchanged lines are irrelevant, because unchanged candidate truth may still constrain implementation
   - if the unit has no `stable` yet, skip this step
7. determine the planning result shape for this round before plan write-back:
   - every `unit_plan` run must end in exactly one of these result shapes: `plan-ready`, `truth-fallback`, `plan-blocked`, or `decision-checkpoint`
   - if research preflight is not required because implementation-critical unknowns are already sufficiently closed, treat the round as `plan-ready`
8. determine whether research preflight is required:
   - use it only when current candidate truth is already closed enough to investigate implementation, but key implementation-critical unknowns still prevent a stable plan
   - do not use research preflight to replace missing behavior truth, boundary truth, or acceptance truth in the candidate
   - if research confirms that the real blocker is incomplete candidate truth, do not continue planning and fall back to `unit_check`
9. after research preflight, allow only these three result shapes:
   - plan-ready: implementation-critical unknowns are closed enough to write a stable plan
   - truth-fallback: research found that candidate truth itself is still incomplete, so planning must fall back to `unit_check`
   - plan-blocked: candidate truth still stands, but planning is blocked on a clearly named external condition, further bounded research result, or human-supplied implementation fact
10. `decision-checkpoint` is a distinct result shape:
   - use it only when a `decision` checkpoint is actually raised because implementation direction is still unresolved
   - do not merge it into `plan-blocked`, because unresolved direction and missing implementation facts are different blocking causes
   - do not create or update `docs/specs/_plans/active/{unit}.md`
   - keep `_status.md` at `unit_plan` unless the checkpoint answer must first be written back into candidate truth or appendix truth
11. if the result is `truth-fallback`:
   - delete `_check_result/unit/{unit}.md`
   - delete `_plans/draft/{unit}.md` if it exists
   - delete `_plans/active/{unit}.md` if it exists
   - delete `_verify_result/unit/{unit}.md` if it exists
   - do not create or update `docs/specs/_plans/active/{unit}.md`
   - update `_status.md` to `unit_check`
   - report `fallback_reason_code=truth_incomplete`
12. if the result is `plan-blocked`:
   - create or update `docs/specs/_plans/draft/{unit}.md`
   - do not create or update `docs/specs/_plans/active/{unit}.md`
   - if an old `active/{unit}.md` still exists for the same round, revalidate whether it remains consumable; if not, delete it rather than leaving a stale active plan available to downstream commands
   - keep `_status.md` at `unit_plan`
   - report `fallback_reason_code=implementation_unknown`
   - record the blocking point, the missing condition, and the exact resume signal
13. determine whether a `decision` checkpoint is required:
   - only use it when key implementation direction is still not locked
   - if the unresolved decision changes behavior truth, boundary truth, or acceptance truth, the resume path must go back to `unit_check` after writeback
   - do not treat the checkpoint as permission to continue without that writeback
14. if a `decision` checkpoint is raised:
   - set the result shape to `decision-checkpoint`
   - create or update `docs/specs/_plans/draft/{unit}.md`
   - do not create or update `docs/specs/_plans/active/{unit}.md`
   - if an old `active/{unit}.md` still exists for the same round, revalidate whether it remains consumable; if not, delete it rather than leaving a stale active plan available to downstream commands
   - keep `_status.md` at `unit_plan` when the unresolved decision is implementation-direction only
   - report `fallback_reason_code=direction_unresolved`
   - use `resume_next_step=unit_check` only when the checkpoint answer must first be written back into candidate truth or appendix truth
15. create or update `docs/specs/_plans/active/{unit}.md` only when no checkpoint blocks planning and the result is `plan-ready`
16. if `docs/specs/_plans/draft/{unit}.md` exists for the same round and the round is now `plan-ready`, extract only the stabilized planning content into `active/{unit}.md`; do not rename the draft file in place
17. after a successful active-plan write for the current round, delete `docs/specs/_plans/draft/{unit}.md` if it exists
18. identify the changed execution surfaces of this round before finalizing either draft or active planning output:
   - define an execution surface as the concrete capability path that this round is actually changing inside the unit
   - do not force one whole-unit owner or one whole-unit path when the round touches only a narrower capability slice
   - name each execution surface directly enough that `unit_impl` and `unit_verify` can reuse the same surface labels without reinterpretation
19. for each changed execution surface, record at minimum:
   - current known path
   - target path for the end of this round
   - retirement goal naming which legacy dependency should stop being a required prerequisite
   - the first stable cutover slices that can advance now
20. if current implementation facts are still insufficient to name a target path or retirement goal safely, but candidate truth still stands:
   - keep the round at `unit_plan`
   - update `docs/specs/_plans/draft/{unit}.md`
   - record the missing implementation fact under `open_modeling_unknowns`
21. if planning discovers that the real blocker is missing behavior truth, missing boundary truth, or missing acceptance truth:
   - do not compensate inside plan text
   - treat the round as `truth-fallback`
22. ensure the active plan records:
   - `Execution Surface Plan`
   - `Retirement Targets`
   - `Verification Targets`
   - execution slices rather than one undifferentiated implementation block
   - for each slice: objective, file scope, dependencies, implementation action, verification action, done condition, and current status
   - progress, blockers, and verification focus for this round
   - `spec_file_ref`
   - `spec_version_ref`
   - `spec_fingerprint`
   - `unit_appendix_snapshot`
   - `system_constraints_file_ref`
   - `system_constraints_version_ref`
   - `system_constraints_fingerprint`
   - `shared_contract_snapshot`
23. complete `Plan Executability` review before treating the round as `plan-ready`:
   - the active plan must not contain placeholder instructions such as `TBD`, `TODO`, `follow up later`, `similar to above`, `后续补充`, `类似上面处理`, or equivalent unresolved work markers
   - each execution slice must be understandable without chat context, guidance discussion, or rejected design options
   - each key candidate behavior and acceptance criterion must map to at least one execution slice or verification target
   - each verification action must name the command, inspection, or evidence to collect, and state which candidate requirement it proves
   - if the active plan lacks these details while candidate truth still stands, the result is `plan-blocked` and the missing planning content must be recorded in a draft plan or blocking reason
   - if the missing planning detail reveals incomplete behavior, boundary, or acceptance truth, the result is `truth-fallback`
24. treat `plan-ready` as valid only when all of the following hold:
   - the changed execution surfaces of this round are identified
   - each changed execution surface has a target path
   - each changed execution surface has at least one explicit retirement goal
   - the first implementation slices are stable enough to enter `unit_impl`
   - `Plan Executability` passes
25. update `_status.md`:
   - if the candidate is now ready for implementation -> `Next Command=unit_impl`
   - if candidate truth drift was discovered -> `Next Command=unit_check`
   - if research preflight found candidate truth gaps -> `Next Command=unit_check`
   - if research preflight is blocked on implementation-critical unknowns but no truth rewrite is pending -> keep `Next Command=unit_plan`
   - if the result is `decision-checkpoint` and no truth writeback is pending -> keep `Next Command=unit_plan`
   - if a `decision` checkpoint stopped planning and no truth writeback is pending -> keep `Next Command=unit_plan`
26. perform git close-out if required

## 5. Stop Conditions

1. either a valid active plan file exists for the current candidate truth and records the changed execution surfaces, target paths, retirement targets, and first stable cutover slices, or planning stopped with no consumable active plan artifact because of fallback, bounded blocking, or checkpoint
2. any active plan created in this round passes `Plan Executability`
3. `_status.md` points to the real next step

## 6. Output Contract

1. planning conclusion
2. whether an active plan file was written, updated, or intentionally not created because planning stopped at fallback, bounded blocking, or a checkpoint
3. whether a draft plan file was written, updated, deleted, or intentionally omitted
4. plan binding result
5. stable-to-candidate change-surface review result when `stable` exists
6. research preflight result when research preflight was used
7. changed execution surfaces and their target-path result
8. retirement-target planning result
9. `handoff validation result`
10. `Plan Executability` result
11. cleanup result when planning fell back to `unit_check`
12. `checkpoint result` when a checkpoint stop was raised
   - when present, it must satisfy the fixed checkpoint fields defined by `specflow/framework/checkpoint_protocol.md`
13. `fallback_reason_code` for fallback, blocking, or checkpoint stops
14. blocking reason and resume signal when planning stayed at `unit_plan` without fallback
15. git close-out result
16. `_status.md` update result
17. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/command_policy.md`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - when a checkpoint was raised or planning stayed blocked at `unit_plan`, also report `resume signal`
   - if `Next Command=unit_plan`, `why this next step` must explicitly state whether planning is waiting on implementation facts, unresolved direction, or truth writeback

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

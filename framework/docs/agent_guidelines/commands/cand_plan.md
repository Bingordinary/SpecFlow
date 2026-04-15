# Candidate Plan Command

## 1. Purpose

This command creates or updates the implementation plan for the current candidate.

## 2. Scope

By default it handles:

1. reading the current valid candidate pass gate
2. optionally running research preflight when implementation-critical unknowns still block a stable plan
3. generating or updating `_plans/{module}.md`
4. keeping plan bindings aligned with the current candidate, current formal global baseline state, and current Shared Appendix snapshot
5. stopping at a structured decision checkpoint only when key implementation direction is still not locked

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=cand_plan`
3. a current valid `docs/specs/_check_result/{module}.md` exists
4. the current candidate still aligns with the current formal global baseline state
5. read any explicitly referenced candidate appendix files and bound Shared Appendix files
6. if this round may raise a checkpoint, read `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
7. read the git policy if commit-triggering files may change

## 4. Procedure

1. read the candidate Spec and all required appendix or Shared Appendix files
2. read `s_system_constraints.md` if it exists
3. read `docs/specs/_check_result/{module}.md`
4. verify the pass gate bindings are still valid
5. if the pass gate is invalid, stop immediately and fall back `_status.md` to `cand_check`
6. determine whether research preflight is required:
   - use it only when current candidate truth is already closed enough to investigate implementation, but key implementation-critical unknowns still prevent a stable plan
   - do not use research preflight to replace missing behavior truth, boundary truth, or acceptance truth in the candidate
   - if research confirms that the real blocker is incomplete candidate truth, do not continue planning and fall back to `cand_check`
7. after research preflight, allow only these three result shapes:
   - plan-ready: implementation-critical unknowns are closed enough to write a stable plan
   - truth-fallback: research found that candidate truth itself is still incomplete, so planning must fall back to `cand_check`
   - plan-blocked: candidate truth still stands, but planning is blocked on a clearly named external condition, further bounded research result, or human-supplied implementation fact
8. if the result is `truth-fallback`:
   - do not create or update `docs/specs/_plans/{module}.md`
   - update `_status.md` to `cand_check`
   - report `fallback_reason_code=truth_incomplete`
9. if the result is `plan-blocked`:
   - do not create or update `docs/specs/_plans/{module}.md`
   - keep `_status.md` at `cand_plan`
   - record the blocking point, the missing condition, and the exact resume signal
10. determine whether a `decision` checkpoint is required:
   - only use it when key implementation direction is still not locked
   - if the unresolved decision changes behavior truth, boundary truth, or acceptance truth, the resume path must go back to `cand_check` after writeback
   - do not treat the checkpoint as permission to continue without that writeback
11. if a `decision` checkpoint is raised:
   - do not create or update `docs/specs/_plans/{module}.md`
   - keep `_status.md` at `cand_plan` when the unresolved decision is implementation-direction only
   - use `resume_next_step=cand_check` only when the checkpoint answer must first be written back into candidate truth or appendix truth
12. create or update `docs/specs/_plans/{module}.md` only when no checkpoint blocks planning and the result is `plan-ready`
13. ensure the plan records:
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
   - `shared_appendix_snapshot`
14. update `_status.md`:
   - if the candidate is now ready for implementation -> `Next Command=cand_impl`
   - if candidate truth drift was discovered -> `Next Command=cand_check`
   - if research preflight found candidate truth gaps -> `Next Command=cand_check`
   - if research preflight is blocked on implementation-critical unknowns but no truth rewrite is pending -> keep `Next Command=cand_plan`
   - if a `decision` checkpoint stopped planning and no truth writeback is pending -> keep `Next Command=cand_plan`
15. perform git close-out if required

## 5. Stop Conditions

1. either a valid plan file exists for the current candidate truth, or planning stopped with no consumable plan artifact because of fallback, bounded blocking, or checkpoint
2. `_status.md` points to the real next step

## 6. Output Contract

1. planning conclusion
2. whether a plan file was written, updated, or intentionally not created because planning stopped at fallback, bounded blocking, or a checkpoint
3. plan binding result
4. research preflight result when research preflight was used
5. `handoff validation result`
6. `checkpoint result` when a checkpoint stop was raised
   - when present, it must satisfy the fixed checkpoint fields defined by `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
7. `fallback_reason_code` for fallback or checkpoint stops
8. blocking reason and resume signal when planning stayed at `cand_plan` without fallback
9. git close-out result
10. `_status.md` update result

Allowed checkpoint types:

1. `decision`

Allowed `fallback_reason_code` values:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_appendix_drift`
6. `truth_incomplete`

## 7. Non-Goals

1. direct code implementation
2. direct verification
3. rewriting candidate truth

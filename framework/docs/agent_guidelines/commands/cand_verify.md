# Candidate Verify Command

## 1. Purpose

This command verifies whether current implementation aligns with the current `candidate`.

## 2. Scope

By default it handles:

1. candidate-versus-implementation alignment verification
2. goal-backward verification from acceptance claims into implementation evidence
3. structured verification evidence generation
4. writing `_verify_result/{module}.md`
5. deciding whether the module may enter `cand_promote`
6. stopping at a `human_verify` checkpoint only when automation is still insufficient to close confidence
7. confirming that any `system_constraints_change_proposal` claimed by the current candidate is actually reflected in implementation evidence

### 2.1 Lifecycle-State Advance Inheritance

当本命令推进 `_status.md` 时，这个推进继承 `specflow/framework/docs/agent_guidelines/command_policy.md` 第 8.5 节定义的 authoritative / non-authoritative 中心契约。
Only a new independent full-scope run of `cand_verify` may produce that advancing result; later local confirmation, narrowed evidence refresh, or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=cand_verify`
3. a current valid `_check_result/{module}.md` exists
4. a current valid `_plans/{module}.md` exists
5. the candidate still aligns with the current formal global baseline state
6. read required candidate appendix files and bound Shared Contract files
7. if this round may raise a checkpoint, read `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
8. read the git policy if commit-triggering files may change

## 4. Procedure

1. read the candidate Spec, required appendix files, Shared Contract files, pass gate, and plan
2. validate all required bindings
3. if the pass gate or plan is invalid, stop immediately:
   - delete `_check_result/{module}.md`
   - delete `_plans/{module}.md`
   - delete `_verify_result/{module}.md` if it exists
   - fall back `_status.md` to `cand_check`
4. verify current code against key protocols, main flow, error handling, acceptance criteria, and any explicit `system_constraints_change_proposal`
5. perform goal-backward verification for each key acceptance claim instead of stopping at artifact existence
6. for each key claim, judge at minimum:
   - `existence`: the required artifact, path, handler, test, or integration point exists
   - `substance`: the artifact contains meaningful implementation rather than hollow placeholder structure
   - `wiring`: the artifact is actually connected to the main execution path, user path, or protocol path required by the current candidate
7. if a required outcome depends on cross-file integration, name that integration path directly in the evidence matrix
8. if implementation pieces exist but are not wired into the claimed path, treat that as `implementation_deviation` rather than as successful existence evidence
9. produce a structured verification evidence matrix
10. output `Coverage Summary`
11. determine whether a `human_verify` checkpoint is required:
   - use it only when automated verification is insufficient but a small amount of human effect judgment can close the remaining uncertainty
   - if human verification confirms implementation deviation while candidate truth still stands, fall back to `cand_impl`
   - if human verification shows acceptance truth itself is still incomplete, fall back to `cand_check`
12. classify deviations with the shared severity meanings defined by `specflow/framework/docs/agent_guidelines/severity_policy.md`
13. conclude:
   - if `fail` exists, do not enter `cand_promote`
   - if `partial` or `not_checked` exists, promotion is allowed only if `specflow/framework/docs/agent_guidelines/downgrade_policy.md` allows downgrade for the current evidence state
   - if key deviations are cleared and evidence is complete, promotion may proceed
14. write or update `docs/specs/_verify_result/{module}.md`
15. update `_status.md`:
   - if ready to promote -> `Next Command=cand_promote`
   - if implementation has deviations but candidate truth still stands -> `Next Command=cand_impl`
   - if candidate truth or formal global baseline must be re-closed -> `Next Command=cand_check`
   - if verification evidence is still incomplete but no upstream truth drift exists -> `Next Command=cand_verify`
16. perform git close-out if required

## 5. Stop Conditions

1. candidate-versus-code alignment is clear
2. whether promotion is allowed is clear
3. `_status.md` points to the real next executable step
4. if pass gate or plan was invalid, verification stopped and `_status.md` fell back to `cand_check`

## 6. Output Contract

1. verification conclusion
2. structured verification evidence matrix
3. `Coverage Summary`
4. goal-backward evidence result
5. downgrade decision when `partial` or `not_checked` exists
6. verify-result write-back result
7. cleanup result when verification fell back to `cand_check`
8. `checkpoint result` when a checkpoint stop was raised
   - when present, it must satisfy the fixed checkpoint fields defined by `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
9. `fallback_reason_code` for fallback or checkpoint stops
10. deviation list
11. fallback reason if pass gate or plan was invalid
12. next-step suggestion
13. git close-out result
14. `_status.md` update result
15. `specflow/framework/docs/agent_guidelines/command_policy.md` 第 8.6 节要求的 `user-facing close-out block`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - when a `human_verify` checkpoint was raised, also report `resume signal`
   - if `Next Command` remains `cand_verify` or falls back to `cand_impl`, `why this next step` must explicitly state whether the remaining blocker is missing evidence, human-effect judgment, or implementation deviation

Allowed checkpoint types:

1. `human_verify`

Allowed `fallback_reason_code` values:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_contract_drift`
6. `implementation_deviation`
7. `evidence_incomplete`
8. `truth_incomplete`

## 7. Non-Goals

1. directly changing code
2. directly rewriting candidate truth
3. advancing an independent `system_constraints` state machine

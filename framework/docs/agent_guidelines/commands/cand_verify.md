# Candidate Verify Command

## 1. Purpose

This command verifies whether current implementation aligns with the current `candidate`.

## 2. Scope

By default it handles:

1. candidate-versus-implementation alignment verification
2. structured verification evidence generation
3. writing `_verify_result/{module}.md`
4. deciding whether the module may enter `cand_promote`
5. stopping at a `human_verify` checkpoint only when automation is still insufficient to close confidence

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=cand_verify`
3. a current valid `_check_result/{module}.md` exists
4. a current valid `_plans/{module}.md` exists
5. the candidate still aligns with the current formal global baseline state
6. read required candidate appendix files and bound Shared Appendix files
7. read the git policy if commit-triggering files may change

## 4. Procedure

1. read the candidate Spec, required appendix files, Shared Appendix files, pass gate, and plan
2. validate all required bindings
3. if the pass gate or plan is invalid, stop immediately and fall back `_status.md` to `cand_check`
4. verify current code against key protocols, main flow, error handling, and acceptance criteria
5. produce a structured verification evidence matrix
6. output `Coverage Summary`
7. determine whether a `human_verify` checkpoint is required:
   - use it only when automated verification is insufficient but a small amount of human effect judgment can close the remaining uncertainty
   - if human verification confirms implementation deviation while candidate truth still stands, fall back to `cand_impl`
   - if human verification shows acceptance truth itself is still incomplete, fall back to `cand_check`
8. classify deviations with the shared `P1 / P2 / P3` semantics
9. conclude:
   - if `fail` exists, do not enter `cand_promote`
   - if `partial` or `not_checked` exists, promotion is allowed only if `specflow/framework/docs/agent_guidelines/downgrade_policy.md` allows downgrade for the current evidence state
   - if key deviations are cleared and evidence is complete, promotion may proceed
10. write or update `docs/specs/_verify_result/{module}.md`
11. update `_status.md`:
   - if ready to promote -> `Next Command=cand_promote`
   - if implementation has deviations but candidate truth still stands -> `Next Command=cand_impl`
   - if candidate truth or formal global baseline must be re-closed -> `Next Command=cand_check`
   - if verification evidence is still incomplete but no upstream truth drift exists -> `Next Command=cand_verify`
12. perform git close-out if required

## 5. Stop Conditions

1. candidate-versus-code alignment is clear
2. whether promotion is allowed is clear
3. `_status.md` points to the real next executable step
4. if pass gate or plan was invalid, verification stopped and `_status.md` fell back to `cand_check`

## 6. Output Contract

1. verification conclusion
2. structured verification evidence matrix
3. `Coverage Summary`
4. downgrade decision when `partial` or `not_checked` exists
5. verify-result write-back result
6. `checkpoint result` when a checkpoint stop was raised
7. `fallback_reason_code` for fallback or checkpoint stops
8. deviation list
9. fallback reason if pass gate or plan was invalid
10. next-step suggestion
11. git close-out result
12. `_status.md` update result

Allowed checkpoint types:

1. `human_verify`

Allowed `fallback_reason_code` values:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_appendix_drift`
6. `implementation_deviation`
7. `evidence_incomplete`

## 7. Non-Goals

1. directly changing code
2. directly rewriting candidate truth
3. advancing an independent `system_constraints` state machine

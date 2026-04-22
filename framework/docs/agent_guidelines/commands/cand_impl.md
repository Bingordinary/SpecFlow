# Candidate Implement Command

## 1. Purpose

This command advances code implementation according to the current `candidate` and `_plans/{module}.md`.

## 2. Scope

By default it handles:

1. implementing according to plan slices
2. adding necessary tests or verification actions
3. writing progress back into `_plans/{module}.md`
4. consuming the `cand_plan -> cand_impl` handoff only when gate and plan bindings both still hold

### 2.1 Lifecycle-State Advance Inheritance

当本命令推进 `_status.md` 时，这个推进继承 `specflow/framework/docs/agent_guidelines/command_policy.md` 第 8.5 节定义的 authoritative / non-authoritative 中心契约。
Only a new independent full-scope run of `cand_impl` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=cand_impl`
3. a current valid `docs/specs/_check_result/{module}.md` exists
4. a current valid `docs/specs/_plans/{module}.md` exists
5. the candidate still aligns with the current formal global baseline state
6. read required candidate appendix files and bound Shared Contract files
7. read the git policy before implementation work

## 4. Procedure

1. read the candidate Spec and all required appendix or Shared Contract files
2. read `s_system_constraints.md` if it exists
3. read the current `_check_result/{module}.md`
4. read the current `_plans/{module}.md`
5. validate all required bindings of the pass gate and plan file according to the candidate handoff contract
6. if any binding is invalid, stop immediately:
   - delete `_check_result/{module}.md`
   - delete `_plans/{module}.md`
   - delete `_verify_result/{module}.md` if it exists
   - fall back `_status.md` to `cand_check`
7. if `system_constraints_stable_ref` no longer matches the current formal global baseline state, stop immediately:
   - delete `_check_result/{module}.md`
   - delete `_plans/{module}.md`
   - delete `_verify_result/{module}.md` if it exists
   - fall back to `cand_check`
8. only when both pass gate and plan are still valid may implementation continue
9. implement slice by slice in the order defined by the current plan unless the plan itself declares a dependency-safe different order
10. for each slice, use the recorded objective, file scope, dependencies, verification action, and done condition as the execution boundary
11. do not collapse a blocked slice into a vague whole-module status; record clearly which slice is complete, blocked, or still pending
12. run necessary verification for the slices advanced in this round, or record clearly what could not be run
13. write slice completion status, blockers, and verification results back into `_plans/{module}.md`
14. update `_status.md`:
   - if implementation is ready for verification -> `Next Command=cand_verify`
   - if implementation is still blocked -> keep `Next Command=cand_impl`
   - if candidate truth or formal global baseline drift means closure must restart -> `Next Command=cand_check`
15. perform git close-out if required

## 5. Stop Conditions

1. the current plan slices have advanced as far as feasible in this round
2. the plan file has been written back
3. `_status.md` points to the real next executable step
4. if the pass gate or plan became invalid, implementation was stopped and `_status.md` was fallen back to `cand_check`

## 6. Output Contract

1. implementation progress result
2. slice progress result
3. tests or verification run, or explicit gaps
4. plan write-back result
5. blocked-slice result when implementation could not finish the current plan round
6. `handoff validation result`
7. cleanup result when implementation fell back to `cand_check`
8. `fallback_reason_code` when the pass gate or plan was invalid
9. fallback reason if the pass gate or plan was invalid
10. git close-out result
11. `_status.md` update result
12. `specflow/framework/docs/agent_guidelines/command_policy.md` 第 8.6 节要求的 `user-facing close-out block`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - `current state` must explicitly confirm the written `Active Layer` and `Next Command`
   - if `Next Command=cand_impl`, `why this next step` must explicitly state that implementation progressed but candidate closure has not yet reached the `cand_verify` entry condition
   - `next-stage entry gap` must name the unfinished implementation, verification, or closure surfaces that still block `cand_verify`

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_contract_drift`

## 7. Non-Goals

1. rewriting candidate truth
2. replacing verification with implementation
3. advancing an independent `system_constraints` state machine

# Unit Implement Command

## 1. Purpose

This command advances code implementation according to the current `candidate` and `_plans/active/{unit}.md`.

## 2. Scope

By default it handles:

1. implementing according to plan slices
2. adding necessary tests or verification actions
3. dynamically confirming which legacy dependencies have already stopped being required
4. writing progress back into `_plans/active/{unit}.md`
5. consuming the `unit_plan -> unit_impl` handoff only when gate and plan bindings both still hold

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/command_policy.md`.
Only a new independent full-scope run of `unit_impl` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=unit_impl`
3. a current valid `docs/specs/_check_result/unit/{unit}.md` exists
4. a current valid `docs/specs/_plans/active/{unit}.md` exists
5. the candidate still aligns with the current formal global baseline state
6. read required candidate appendix files and bound Shared Contract files
7. read the git policy before implementation work

## 4. Procedure

1. read the candidate Spec and all required appendix or Shared Contract files
2. read `system_constraints.md` if it exists
3. read the current `_check_result/unit/{unit}.md`
4. read the current `_plans/active/{unit}.md`
5. validate all required bindings of the pass gate and plan file according to the candidate handoff contract
6. if any binding is invalid, stop immediately:
   - delete `_check_result/unit/{unit}.md`
   - delete `_plans/draft/{unit}.md`
   - delete `_plans/active/{unit}.md`
   - delete `_verify_result/unit/{unit}.md` if it exists
   - fall back `_status.md` to `unit_check`
7. if `system_constraints_ref` no longer matches the current formal global baseline state, stop immediately:
   - delete `_check_result/unit/{unit}.md`
   - delete `_plans/draft/{unit}.md`
   - delete `_plans/active/{unit}.md`
   - delete `_verify_result/unit/{unit}.md` if it exists
   - fall back to `unit_check`
8. only when both pass gate and plan are still valid may implementation continue
9. implement slice by slice in the order defined by the current plan unless the plan itself declares a dependency-safe different order
10. for each slice, use the recorded objective, file scope, dependencies, verification action, and done condition as the execution boundary
11. do not collapse a blocked slice into a vague whole-unit status; record clearly which slice is complete, blocked, or still pending
12. treat a slice as truly advanced only when at least one of the following is now true:
   - an execution surface has been cut over to its target path
   - a named retirement target is now confirmed as no longer required
13. when implementation discovers additional legacy paths, legacy helpers, legacy patches, or legacy wrappers that were not yet fully modeled:
   - keep the work in `unit_impl` if the discovery only deepens implementation facts
   - write the discovery back into the current active plan under the round's implementation-progress sections
   - do not restart the round from `unit_plan` unless the discovery proves candidate truth itself is insufficient
14. if implementation discovers that the active plan's convergence target cannot stand without a new behavior or boundary decision:
   - stop treating the issue as implementation-only
   - fall back to `unit_check`
15. run necessary verification for the slices advanced in this round, or record clearly what could not be run
16. write slice completion status, blockers, verification results, and retirement progression back into `_plans/active/{unit}.md`
17. ensure the active plan write-back records at minimum:
   - `Takeover Progress`
   - `Retirement Progress`
   - `Newly Confirmed Legacy`
   - `Residual Legacy Dependencies`
   - for each advanced slice: `execution_surface`, `cutover_result`, `retirement_result`, and `verification_note`
18. update `_status.md`:
   - if implementation is ready for verification -> `Next Command=unit_verify`
   - if implementation is still blocked -> keep `Next Command=unit_impl`
   - if candidate truth or formal global baseline drift means closure must restart -> `Next Command=unit_check`
19. perform git close-out if required

## 5. Stop Conditions

1. the current plan slices have advanced as far as feasible in this round, and each claimed advance names either a target-path cutover or a retirement confirmation
2. the plan file has been written back
3. `_status.md` points to the real next executable step
4. if the pass gate or plan became invalid, or implementation discovered that candidate truth itself must be re-closed, implementation was stopped and `_status.md` was fallen back to `unit_check`

## 6. Output Contract

1. implementation progress result
2. slice progress result
3. tests or verification run, or explicit gaps
4. takeover-progress result
5. retirement-progress result
6. plan write-back result
7. blocked-slice result when implementation could not finish the current plan round
8. `handoff validation result`
9. cleanup result when implementation fell back to `unit_check`
10. `fallback_reason_code` when implementation fell back to `unit_check`
11. fallback reason when implementation fell back to `unit_check`
12. git close-out result
13. `_status.md` update result
14. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/command_policy.md`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - `current state` must explicitly confirm the written `Active Layer` and `Next Command`
   - if `Next Command=unit_impl`, `why this next step` must explicitly state that implementation progressed but candidate closure has not yet reached the `unit_verify` entry condition
   - `next-stage entry gap` must name the unfinished implementation, verification, closure, or retirement surfaces that still block `unit_verify`

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `shared_contract_drift`
6. `truth_incomplete`

## 7. Non-Goals

1. rewriting candidate truth
2. replacing verification with implementation
3. advancing an independent `system_constraints` state machine

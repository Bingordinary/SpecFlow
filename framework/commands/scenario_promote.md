# Scenario Promote Command

## 1. Purpose

`scenario_promote:{scenario}` promotes the current candidate scenario into the new stable scenario truth.

## 2. Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/command_policy.md`.
Only a new independent full-scope run of `scenario_promote` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. `_status.md` says `Object Type=scenario`, `Active Layer=candidate`, `Next Command=scenario_promote`
2. current valid `_verify_result/scenario/{scenario}.md` exists
3. read `specflow/framework/candidate_handoff_contract.md`
4. read `specflow/framework/recovery_policy.md` before promotion

## 4. Procedure

1. read and re-check the latest `_verify_result/scenario/{scenario}.md`
2. read and re-check `docs/specs/scenarios/candidate/c_scenario_{scenario}.md`
3. validate `_verify_result/scenario/{scenario}.md` according to the `scenario_verify -> scenario_promote` handoff in `specflow/framework/candidate_handoff_contract.md`
4. if `_verify_result/scenario/{scenario}.md` is invalid, stop before truth-file mutation:
   - if candidate truth, repository mapping snapshot, required unit bindings, bound Shared Contract snapshots, or formal global baseline snapshots drifted, delete current-round scenario `_check_result/scenario/{scenario}.md` and `_verify_result/scenario/{scenario}.md`, update `_status.md` to `Next Command=scenario_check`, and use the matching standardized code: `truth_drift`, `binding_drift`, `shared_contract_drift`, or `baseline_drift`
   - if only verification coverage is stale or incomplete while the check gate still covers current truth, delete `_verify_result/scenario/{scenario}.md`, update `_status.md` to `Next Command=scenario_verify`, and use `fallback_reason_code=evidence_incomplete`
5. continue only when candidate truth, verification coverage, and required bindings still remain valid
6. before the first truth-file mutation, capture the recovery baseline required by `recovery_policy.md`
7. write `docs/specs/scenarios/stable/s_scenario_{scenario}.md`
8. write `_status.md`:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=scenario_fork`
9. only after `_status.md` has already been updated to `Candidate=no`, delete:
   - `docs/specs/scenarios/candidate/c_scenario_{scenario}.md`
   - current-round scenario `_check_result/scenario/{scenario}.md`
   - current-round scenario `_verify_result/scenario/{scenario}.md`
10. if the command is interrupted after promotion internals started but before final cleanup finished, run incomplete promotion recovery according to `recovery_policy.md` instead of claiming success

## 5. Output Contract

The output must report:

1. stable truth file write result
2. candidate truth file delete result
3. `_check_result/scenario/{scenario}.md` and `_verify_result/scenario/{scenario}.md` cleanup result
4. lifecycle-state transition result
5. `_status.md` update result
6. `handoff validation result`
7. fallback cleanup result when verification became invalid before promotion could start
8. `fallback_reason_code` if verification became invalid
9. `fallback_reason_code=promotion_recovery` when incomplete promotion recovery occurred
10. recovery-state explanation if incomplete promotion occurred
11. `round conclusion`
12. `current state`
13. `next step`
14. `why this next step`
15. `next-stage entry gap`
16. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6
17. when promotion recovery occurred, the same close-out block must also report `resume signal`

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `binding_drift`
2. `truth_drift`
3. `baseline_drift`
4. `shared_contract_drift`
5. `evidence_incomplete`
6. `promotion_recovery`

## 6. Non-Goals

1. unit promotion
2. changing repository mapping

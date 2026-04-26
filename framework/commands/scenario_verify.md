# Scenario Verify Command

## 1. Purpose

`scenario_verify:{scenario}` verifies whether the current candidate scenario is actually wired from trigger to outcome under the current bound object set.

## 2. Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/command_policy.md`.
Only a new independent full-scope run of `scenario_verify` may produce that advancing result; later repair confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. `_status.md` says `Object Type=scenario`, `Active Layer=candidate`, `Next Command=scenario_verify`
2. current valid `_check_result/scenario/{scenario}.md` exists
3. read `specflow/framework/candidate_handoff_contract.md`

## 4. Procedure

1. read current candidate scenario truth
2. revalidate `_check_result/scenario/{scenario}.md` according to the `scenario_check -> scenario_verify` handoff in `specflow/framework/candidate_handoff_contract.md`
3. revalidate current repository mapping, bound units, shared contracts, and baseline snapshots
4. verify the declared trigger-to-outcome path from entry to claimed outcome
5. report `affected_units` when implementation work is still required downstream
6. if pass, write `_verify_result/scenario/{scenario}.md` so it satisfies the `scenario_verify -> scenario_promote` handoff, then advance `Next Command=scenario_promote`
7. if the check handoff is invalid or repository mapping, unit, shared-contract, or baseline bindings drifted, fall back to `scenario_check` with the matching allowed `fallback_reason_code`

## 5. Output Contract

The output must report:

1. verification gate result
2. `_verify_result/scenario/{scenario}.md` write, delete, or keep result
3. `_status.md` update result
4. `affected_units` when downstream implementation is still required
5. `round conclusion`
6. `current state`
7. `next step`
8. `why this next step`
9. `next-stage entry gap`
10. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6
11. if a future extension introduces a checkpoint stop, the same close-out block must also report `resume signal`

## 6. Non-Goals

1. replacing `unit_impl:{unit}`
2. implicitly repairing affected units

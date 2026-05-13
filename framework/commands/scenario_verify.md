# Scenario Verify Command

## 1. Purpose

`scenario_verify:{scenario}` verifies whether the current candidate scenario is actually wired from trigger to outcome under the current bound object set.

## 2. Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `scenario_verify`-local entry, output, and stop rules.

Process-file consumption and writeback for `_check_result/scenario/{scenario}.md` and `_verify_result/scenario/{scenario}.md` must follow `specflow/framework/process_snapshot_contract.md` Section 9. When deterministic snapshot validation tooling is available for scenario process files, the matching `snapshot validate-process` command is the mandatory tool-backed validation step before treating a process file as consumable, reporting a verification pass, or advancing lifecycle state.

Before reading `_check_result/scenario/{scenario}.md` as a usable verification input, run `specflowctl command preflight --command scenario_verify --object-type scenario --object {scenario}`. If command preflight is unavailable, run `snapshot validate-process --object-type scenario --object {scenario} --process check` explicitly. After writing `_verify_result/scenario/{scenario}.md`, run `snapshot validate-process --process verify` before reporting a verification pass.

## 3. Preconditions

1. `_status.md` says `Object Type=scenario`, `Active Layer=candidate`, `Next Command=scenario_verify`
2. current valid `_check_result/scenario/{scenario}.md` exists
3. read `specflow/framework/candidate_handoff_contract.md`

## 4. Procedure

1. read current candidate scenario truth
2. run command preflight for `scenario_verify:{scenario}` and stop before verification judgment if authoritative validation is unavailable
3. revalidate `_check_result/scenario/{scenario}.md` according to the `scenario_check -> scenario_verify` handoff in `specflow/framework/candidate_handoff_contract.md`
4. revalidate current repository mapping, bound units, rules, and baseline snapshots using only preflight or `snapshot validate-process` as the authoritative process-file validation source
4. confirm that the check gate's accepted acceptance-item set still matches the current candidate scenario
5. if the check handoff is missing or invalid, classify the failure before cleanup:
   - if scenario truth, acceptance item ids, repository mapping, unit snapshot, Rule snapshot, or baseline binding drifted, use `truth_layer`, delete `_check_result/scenario/{scenario}.md` and `_verify_result/scenario/{scenario}.md`, update `_status.md` to `Next Command=scenario_check`, and report the matching drift code
   - if only the check gate is missing, malformed, or not tool-valid while current scenario truth and bindings still match, use `gate_layer`, delete `_check_result/scenario/{scenario}.md`, update `_status.md` to `Next Command=scenario_check`, and do not delete unrelated current evidence unless it separately fails validation
6. verify the declared trigger-to-outcome path from entry to claimed outcome
7. build the scenario evidence matrix by acceptance item `id`:
   - each row must name `acceptance_item_id`, `target`, `verification_surface`, `implementation_surface`, `verification_method`, `evidence`, and `status`
   - `status` must be exactly one of `pass`, `fail`, `partial`, `not_checked`, or `not_runnable_yet`
   - `pass` requires current-round evidence that directly proves the item's `pass_condition`
   - `not_runnable_yet` may be used only when the scenario item itself explicitly records `not_runnable_yet` and the current run confirms the same missing runnable entrypoint or surface still exists
   - passing unit-local tests that do not prove the trigger-to-outcome item must be reported as insufficient scenario evidence
8. if the path cannot pass because one or more bound units still require unit-local truth, planning, implementation, verification, binding, or baseline work, stop as `blocked_by_affected_units`:
   - do not write `_verify_result/scenario/{scenario}.md`
   - delete `_verify_result/scenario/{scenario}.md` when it exists because it cannot remain current while the scenario is blocked by affected unit work
   - keep `_check_result/scenario/{scenario}.md` only when it still qualifies as the current valid check gate
   - keep or set the scenario row in `_status.md` to `Active Layer=candidate` and `Next Command=scenario_verify`
   - read `_status.md` rows for each `affected_unit` and report that unit's current legal `Next Command`
   - report `fallback_reason_code` using the `blocked_by_affected_units` values in Section 6 of this file
   - ordinary `scenario_verify:{scenario}` follow-up routes through natural-language routing from current repository truth so each affected unit re-enters its own legal unit command chain
   - when the active owner is `scenario_advance:{scenario}`, the close-out may be consumed by `specflow/framework/advance_policy.md` only if every affected unit identity is explicitly reported and confirmed from `_status.md`; advance then enters `unit_advance:{unit}` under that policy
9. if any current-gate acceptance item is `fail`, `partial`, `not_checked`, or `not_runnable_yet`, do not write a pass verify result unless `specflow/framework/downgrade_policy.md` explicitly allows that non-pass evidence state for the current scenario round:
   - if the non-pass state reveals incomplete scenario truth, fall back to `scenario_check`
   - if the non-pass state is caused by affected unit work, stop as `blocked_by_affected_units`
   - otherwise stop as `evidence_incomplete` with `failure_layer=evidence_layer`, keep or set the scenario row to `Next Command=scenario_verify`, delete any stale `_verify_result/scenario/{scenario}.md`, and report `fallback_reason_code=evidence_incomplete`
10. if pass, write `_verify_result/scenario/{scenario}.md` so it satisfies the `scenario_verify -> scenario_promote` handoff, including the acceptance-item evidence matrix and covered `id` set, then advance `Next Command=scenario_promote`
11. close the command after the result is selected:
   - use `pass` only after `_verify_result/scenario/{scenario}.md` has been written and validates
   - the deterministic command closure may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command scenario_verify --object-type scenario --object {scenario} --outcome <pass|gate_fallback|evidence_incomplete|blocked_by_affected_units> --notes <status-note> --apply`
   - for `truth_fallback`, execute `specflow/tooling/bin/specflowctl-<os>-<arch> command close --command scenario_verify --object-type scenario --object {scenario} --outcome truth_fallback --reason <fallback_reason_code> --notes <status-note> --apply`

## 5. Stop Conditions

1. the scenario check handoff is either valid and consumed, or invalid and cleaned up
2. whether the declared trigger-to-outcome path verifies under current bindings is clear
3. every scenario acceptance item has one allowed status
4. if affected units block scenario verification, no scenario verify result remains current, the scenario row still points to `scenario_verify`, and each affected unit's current legal next command is reported from `_status.md`
5. if verification evidence is incomplete without upstream truth fallback, no scenario verify result remains current and the scenario row still points to `scenario_verify`
6. if verification passes, `_verify_result/scenario/{scenario}.md` holds the current verify gate
7. if verification falls back to `scenario_check`, no invalid scenario check or verify result remains
8. `_status.md` points to the real next executable step for the scenario row; when verification is blocked by affected units, the output names the affected units' current legal next commands as the immediate follow-up

## 6. Output Contract

The output must report:

1. verification gate result: `pass`, `fallback_to_scenario_check`, `blocked_by_affected_units`, or `evidence_incomplete`
2. scenario acceptance-item evidence matrix by `id`
3. `_verify_result/scenario/{scenario}.md` write, delete, or keep result
4. `_check_result/scenario/{scenario}.md` cleanup result when verification fell back to `scenario_check`
5. `_status.md` update result
6. `affected_units` and each affected unit's current legal `Next Command` when downstream unit work is still required
7. `fallback_reason_code` when verification stopped as `blocked_by_affected_units`, stopped as `evidence_incomplete`, or fell back to `scenario_check`
8. fallback reason when the check handoff was missing, invalid, or drifted
9. follow-up route requirement when verification stopped as `blocked_by_affected_units`
   - ordinary `scenario_verify:{scenario}` reports the natural-language reroute requirement
   - `scenario_advance:{scenario}` reports the advance consumption permission and each resolved `unit_advance:{unit}` target when the active owner qualifies
10. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

For `fallback_to_scenario_check`:

1. `gate_missing`
2. `truth_drift`
3. `binding_drift`
4. `baseline_drift`
5. `rule_drift`

For `evidence_incomplete`:

1. `evidence_incomplete`

For `blocked_by_affected_units`:

1. `truth_incomplete`
2. `gate_missing`
3. `truth_drift`
4. `binding_drift`
5. `baseline_drift`
6. `rule_drift`
7. `implementation_unknown`
8. `implementation_deviation`
9. `evidence_incomplete`

These values are owned by the standard reason taxonomy in `specflow/framework/candidate_handoff_contract.md`.
The `fallback_to_scenario_check` subset is further bounded by the `scenario_check -> scenario_verify` handoff in that file.
The standardized code must appear before the natural-language explanation.

## 7. Non-Goals

1. replacing `unit_impl:{unit}`
2. implicitly repairing affected units
3. advancing affected unit lifecycle state

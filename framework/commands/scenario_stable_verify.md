# Scenario Stable Verify Command

## 1. Purpose

`scenario_stable_verify:{scenario}` checks whether current repository truth still aligns with the stable scenario truth.

## 2. Lifecycle-State Advance Inheritance

Lifecycle-state advancement follows `specflow/framework/command_policy.md` Sections 8.5 and 8.8.
This file states only `scenario_stable_verify`-local entry, output, and stop rules.

## 3. Preconditions

1. `_status.md` says `Object Type=scenario`, `Active Layer=stable`, `Next Command=scenario_stable_verify`
2. current stable scenario file exists at `docs/specs/scenarios/stable/s_scenario_{scenario}.md`
3. read `specflow/framework/scenario_policy.md`
4. read `spec_writing_guide.md` Section 5
5. read `specflow/framework/process_snapshot_contract.md`
6. read `specflow/framework/downgrade_policy.md`
7. read `specflow/framework/severity_policy.md` when confirmed deviations are graded
8. read `docs/specs/repository_mapping.md`
9. read every stable unit truth file and stable Rule file named by the stable scenario's `unit_refs` and `rule_refs`
11. do not use `docs/specs/_verify_result/scenario/{scenario}.md` as stable-alignment evidence

## 4. Procedure

1. read stable scenario truth
3. revalidate that the current repository mapping, bound stable units, bound stable Rules, and formal global baseline still match the stable scenario's declared bindings
   - stable scenario truth may bind only stable-layer unit and Rule truth
   - repository mapping, unit, Rule, and global-rule comparisons must use the binding and fingerprint rules in `specflow/framework/process_snapshot_contract.md`
   - if the binding set, bound layer, bound file, version reference, or fingerprint no longer matches the stable scenario's declared stable chain, stable alignment cannot be claimed
4. if `docs/specs/_verify_result/stable/scenario/{scenario}.md` exists, read it only as the preserved promotion coverage summary
   - the stable summary is not behavior truth
   - the stable summary is not a current implementation-alignment claim
   - the command must still collect current evidence before claiming alignment with stable
5. verify that the stable scenario `Testability / Acceptance Criteria` section contains explicit acceptance items according to `spec_writing_guide.md` Section 5
   - historical stable scenarios that still use prose-only acceptance text must not be treated as automatically passing
   - if the stable truth lacks structured acceptance items, report the gap and keep the object at `scenario_stable_verify` or route through the smallest legal truth-update path before claiming stable alignment
6. verify the current declared trigger-to-outcome path against the stable scenario truth, including current bound unit behavior, bound Rule behavior, and applicable global-baseline constraints
7. build a stable scenario evidence matrix by acceptance item `id`:
   - each row must name `acceptance_item_id`, `target`, `verification_surface`, `implementation_surface`, `verification_method`, `evidence`, and `status`
   - `status` must be exactly one of `pass`, `fail`, `partial`, `not_checked`, or `not_runnable_yet`
   - `pass` requires current-round evidence that directly proves the item's `pass_condition`
   - `not_runnable_yet` may be used only when the stable item itself explicitly records `not_runnable_yet` and the current run confirms the same missing runnable surface still exists
   - passing unit-local checks that do not prove the stable trigger-to-outcome item must be reported as insufficient scenario evidence, not as `pass`
8. ensure every stable acceptance item has exactly one allowed status
9. output `Coverage Summary` with at least:
   - `Total`
   - `Pass`
   - `Fail`
   - `Partial`
   - `Not Checked`
   - `Not Runnable Yet`
10. add a risk note to every `partial`, `not_checked`, and `not_runnable_yet` item
11. classify confirmed deviations with the shared severity meanings in `specflow/framework/severity_policy.md`
12. apply `specflow/framework/downgrade_policy.md` Section 6.4 before treating any `partial`, `not_checked`, or `not_runnable_yet` item as non-blocking
   - downgrade may be accepted only when every rule in `downgrade_policy.md` Section 4 holds
   - downgrade must be rejected when any rule in `downgrade_policy.md` Section 5 holds
   - if downgrade is rejected, emit the matching standardized `fallback_reason_code` before the natural-language explanation
13. conclude:
   - if repository mapping, unit binding, Rule binding, or global-baseline alignment drifted, stable alignment cannot be claimed
   - if any `fail` exists, stable alignment cannot be claimed
   - if acceptance evidence is incomplete and downgrade is not allowed, stable alignment cannot be claimed
   - if all acceptance items pass, or downgrade is accepted for all non-pass evidence states, the result is "still aligned with stable"
14. update `_status.md`:
   - if still aligned with stable -> `Next Command=scenario_fork`
   - if stable alignment cannot be claimed -> keep `Next Command=scenario_stable_verify`
15. do not write `docs/specs/_verify_result/scenario/{scenario}.md`
16. do not write or update `docs/specs/_verify_result/stable/scenario/{scenario}.md`

## 5. Stop Conditions

1. stable binding alignment is clear for repository mapping, bound stable units, bound stable Rules, and the formal global baseline when applicable
2. every explicit stable acceptance item has one allowed status, or missing acceptance-item structure is reported as the blocker
3. every `partial`, `not_checked`, and `not_runnable_yet` item has a risk note
4. downgrade was either not needed, accepted, or rejected according to `specflow/framework/downgrade_policy.md`
5. the next action is clear
6. no candidate verify result was written or used as stable-alignment evidence
7. no stable acceptance coverage summary was rewritten into a new pass claim
8. `_status.md` is updated

## 6. Output Contract

The output must report:

1. stable alignment result
2. stable acceptance-item evidence matrix by `id`
3. `Coverage Summary`
4. repository mapping, bound unit, bound Rule, and global-baseline alignment result
5. stable acceptance coverage summary read result when `docs/specs/_verify_result/stable/scenario/{scenario}.md` exists
6. explicit confirmation that `docs/specs/_verify_result/scenario/{scenario}.md` was not written or consumed as stable-alignment evidence
7. explicit confirmation that `docs/specs/_verify_result/stable/scenario/{scenario}.md` was not rewritten as a current pass claim
8. downgrade decision when `partial`, `not_checked`, or `not_runnable_yet` exists
9. risk notes for every downgraded or rejected non-pass item
10. deviation list
11. `fallback_reason_code` when stable alignment cannot be claimed safely
12. next-step recommendation
   - if stable alignment cannot be claimed, the immediate next step must remain `scenario_stable_verify`
   - `scenario_fork:{scenario}` may be suggested only after stable alignment has been restored or safely confirmed
13. `_status.md` update result
14. the `user-facing close-out block` required by `specflow/framework/command_policy.md` Section 8.6
   - `current state` must explicitly confirm `Active Layer=stable` and the written `Next Command`
   - if `Next Command=scenario_stable_verify`, `why this next step` must state that stable scenario alignment is not yet confirmed

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `truth_drift`
2. `binding_drift`
3. `implementation_deviation`
4. `evidence_incomplete`
5. `rule_drift`
6. `baseline_drift`

The standardized code must appear before the natural-language explanation.

## 7. Non-Goals

1. scenario candidate authoring
2. unit implementation repair
3. writing candidate verify results
4. rewriting stable acceptance coverage summaries

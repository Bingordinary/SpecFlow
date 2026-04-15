# Stable Verify Command

## 1. Purpose

This command checks whether current code still aligns with a module's `stable` Spec.

Goals:

1. decide whether current implementation still satisfies the formal truth
2. if drift exists, decide whether code must first return to `stable` semantics before any controlled upgrade round may begin

## 2. Scope

By default it handles:

1. regression checks after changes, fixes, or refactors on modules with `Active Layer=stable`
2. alignment checks between `stable` and current code
3. deciding formal-layer deviations and next action

It does not:

1. open a new `candidate`
2. design future behavior
3. directly modify code
4. replace structured verification evidence with a subjective summary

## 3. Preconditions

1. complete required pre-checks
2. the module's current `Active Layer=stable`
3. `_status.md` says `Next Command=stable_verify`
4. the target module is explicit
5. the module has valid `stable`
6. there is actual implementation context that must be checked
7. read any explicitly referenced stable appendix files or bound stable Shared Appendix files
8. read `s_system_constraints.md` if the stable truth explicitly records `system_constraints_stable_ref`, or if the verification scenario otherwise requires global-baseline or shared-mechanism judgment
9. read the git policy if commit-triggering files may change

## 4. Procedure

1. read `docs/specs/stable/s_{module}.md` and any required appendix or Shared Appendix files
2. if the stable truth explicitly records `system_constraints_stable_ref`, or if the verification scenario otherwise requires global-baseline or shared-mechanism judgment, read `s_system_constraints.md`
3. if the stable truth explicitly records `system_constraints_stable_ref`, judge whether that recorded reference still matches the current formal global baseline state
4. verify current code against key protocols, main flow, error handling, and acceptance criteria in `stable`
5. build a structured verification evidence matrix covering at least:
   - `Spec Item`
   - `Expected Behavior`
   - `Implementation Evidence`
   - `Verification Evidence`
   - `Status`
6. ensure all key acceptance points are covered
7. output `Coverage Summary` with at least:
   - `Total`
   - `Covered`
   - `Failed`
   - `Partial`
   - `Not Checked`
8. add risk notes to every `partial` and `not_checked` item
9. classify deviations with the shared severity meanings defined by `specflow/framework/docs/agent_guidelines/severity_policy.md`
10. conclude:
   - if the recorded `system_constraints_stable_ref` no longer matches the current formal global baseline state, the result can only be "global-baseline drift exists; rerun stable verification against the current formal baseline"
   - if any `fail` exists, the result can only be "drift exists; return to stable first"
   - `partial` and `not_checked` are non-blocking only when `specflow/framework/docs/agent_guidelines/downgrade_policy.md` allows downgrade for the current evidence state
   - if key deviations are cleared and evidence is complete, the result is "still aligned with stable"
11. if code or formal global baseline has drifted from the currently claimed stable state, the next action can only be:
   - return code to `stable` semantics
   - or refresh the stable-layer verification conclusion against the current formal global baseline when the drift is baseline-side rather than code-side
   - rerun `stable_verify:{module}` after the required repair or re-judgment work
   - do not open `spec_fork:{module}` while the current implementation still fails `stable_verify`
12. update `_status.md`:
   - if still aligned -> `Next Command=spec_fork`
   - if drift exists -> keep `Next Command=stable_verify`
13. perform git close-out if required

## 5. Stop Conditions

1. alignment with `stable` is clear
2. the next action is clear
3. `_status.md` is updated

## 6. Output Contract

1. verification conclusion
2. structured verification evidence matrix
3. `Coverage Summary`
4. formal global baseline alignment result when `system_constraints_stable_ref` is part of the stable truth
5. downgrade decision when `partial` or `not_checked` exists
6. deviation list
7. `fallback_reason_code` when stable alignment cannot be claimed safely
8. next-step recommendation
   - if drift exists, the immediate next step must remain `stable_verify`
   - `spec_fork:{module}` may be suggested only as a later follow-up after stable alignment has been restored
9. git close-out result
10. `_status.md` update result

Allowed `fallback_reason_code` values:

1. `implementation_deviation`
2. `evidence_incomplete`
3. `shared_appendix_drift`
4. `baseline_drift`

## 7. Non-Goals

1. creating `candidate`
2. replacing upgrade design with stable-layer verification
3. directly declaring future behavior as valid

## 8. Example

```md
stable_verify:module_ai
```

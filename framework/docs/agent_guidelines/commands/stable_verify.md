# Stable Verify Command

## 1. Purpose

This command checks whether current code still aligns with a module's `stable` Spec.

Goals:

1. decide whether current implementation still satisfies the formal truth
2. if drift exists, decide whether code must return to `stable` or enter controlled upgrade

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
4. complete `stable drift reconciliation`
5. the target module is explicit
6. the module has valid `stable`
7. there is actual implementation context that must be checked
8. read any explicitly referenced stable appendix files or bound stable Shared Appendix files
9. read `s_system_constraints.md` if the verification scenario requires global-baseline or shared-mechanism judgment
10. read the git policy if commit-triggering files may change

## 4. Procedure

1. read `s_system_constraints.md` when needed
2. read `docs/specs/stable/s_{module}.md` and any required appendix or Shared Appendix files
3. verify current code against key protocols, main flow, error handling, and acceptance criteria in `stable`
4. build a structured verification evidence matrix covering at least:
   - `Spec Item`
   - `Expected Behavior`
   - `Implementation Evidence`
   - `Verification Evidence`
   - `Status`
5. ensure all key acceptance points are covered
6. output `Coverage Summary` with at least:
   - `Total`
   - `Covered`
   - `Failed`
   - `Partial`
   - `Not Checked`
7. add risk notes to every `partial` and `not_checked` item
8. classify deviations with the shared `P1 / P2 / P3` semantics from `spec_policy.md`
9. conclude:
   - if any `fail` exists, the result can only be "drift exists; return to stable or enter spec_fork"
   - `partial` and `not_checked` are non-blocking only when `specflow/framework/docs/agent_guidelines/downgrade_policy.md` allows downgrade for the current evidence state
   - if key deviations are cleared and evidence is complete, the result is "still aligned with stable"
10. if code has drifted from `stable`, the next action can only be:
   - return code to `stable` semantics
   - or open `spec_fork:{module}`
11. update `_status.md`:
   - if still aligned -> `Next Command=spec_fork`
   - if drift exists -> keep `Next Command=stable_verify`
12. perform git close-out if required

## 5. Stop Conditions

1. alignment with `stable` is clear
2. the next action is clear
3. `_status.md` is updated

## 6. Output Contract

1. verification conclusion
2. structured verification evidence matrix
3. `Coverage Summary`
4. downgrade decision when `partial` or `not_checked` exists
5. deviation list
6. `fallback_reason_code` when stable alignment cannot be claimed safely
7. next-step recommendation
8. git close-out result
9. `_status.md` update result

Allowed `fallback_reason_code` values:

1. `implementation_deviation`
2. `evidence_incomplete`
3. `shared_appendix_drift`

## 7. Non-Goals

1. creating `candidate`
2. replacing upgrade design with stable-layer verification
3. directly declaring future behavior as valid

## 8. Example

```md
stable_verify:module_ai
```

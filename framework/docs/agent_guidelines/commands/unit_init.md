# Unit Init Command

## 1. Purpose

This command creates the first `stable` Spec for a historical unit.

Goals:

1. capture the unit's already-effective formal behavior
2. create the unit's first formal truth file
3. register the unit in `docs/specs/_status.md`

## 2. Scope

By default this command handles:

1. first-time governance onboarding of a historical unit
2. units that already have implementation and stable behavior but are not yet inside the Spec system
3. creation of the first `stable`

It does not handle:

1. creating a new unit
2. forking a new candidate from existing `stable`
3. creating `candidate` directly
4. onboarding a historical unit by first writing duplicated unit-local formal truth when the real task is unresolved cross-unit shared-truth governance

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `unit_init` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

Before execution:

1. complete the required pre-checks from `spec_policy.md`; if the unit is not registered yet, at minimum confirm there are no conflicting old status or leftover process files
2. the target unit name is explicit
3. the unit is not yet in `docs/specs/_status.md`
4. the goal is to capture current truth, not define future design
5. if onboarding current truth would create duplicated formal truth across units, or if the shared/unit boundary is still unstable, do not start `unit_init`; resolve that shared-truth boundary through `specflow/framework/docs/agent_guidelines/shared_ops.md` first
6. if the first `stable` reuses already-existing shared truth, read the relevant `shared_contract` files before writing `shared_contract_refs`
7. if the task also touches global baseline, shared mechanisms, or exceptions, read `docs/specs/system_constraints/stable/s_system_constraints.md`
8. if the unit involves technical choices, shared infrastructure, cross-unit reuse, global exceptions, or system-level constraint relationships, the first `stable` must include `Global Constraint Alignment` or an equivalent section
9. if the task changes `stable`, `_status.md`, or other commit-triggering governance files, read the git policy first
10. if the round creates, updates, or deletes any unit `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/shared_sync.md` first
11. if the round may update `bound_objects` or remove intentional-unbound retention fields from a touched Shared Contract file, read every current-layer unit main file needed to derive the real repository-wide binding set of each touched Shared Contract from `shared_contract_refs`

## 4. Procedure

1. summarize the unit's already-effective behavior baseline
2. if needed, read `s_system_constraints.md` as an upstream input
3. if onboarding current truth shows that one or more existing formal units already depend on the same formal truth and that truth is not yet formalized as one stable shared object, stop and reroute through `shared_ops:{natural-language request}` from current repository truth instead of writing duplicated unit-local `stable` truth
4. create `docs/specs/units/stable/s_unit_{unit}.md`
5. ensure the file covers:
   - `Context & Motivation`
   - `Terminology`
   - `Data Structures / Protocols`
   - `State Machine / Business Flow`
   - `Edge Cases & Error Handling`
   - `Testability / Acceptance Criteria`
6. if needed, add `Global Constraint Alignment` with at least:
   - `system_constraints_stable_ref`
   - `shared_contract_refs` written in the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1
   - `shared_mechanism_reuse_summary`
   - `global_constraint_exceptions`
7. do not introduce `system_constraints_change_proposal` into the first `stable`; that field belongs only to unit `candidate`
8. if the round changed Shared Contract bindings or touched Shared Contract files:
   - derive the real repository-wide binding set of each touched Shared Contract from current-layer unit `shared_contract_refs` plus this round's prepared target-unit stable writeback
   - if current repository truth is insufficient to derive that touched real binding set safely, stop and reroute through `shared_ops:{natural-language request}` from current repository truth instead of guessing
   - update `bound_objects` only as declarative metadata so each touched Shared Contract file matches the real binding set implied by that repository-wide binding view plus this round's prepared target-unit writeback
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared reconcile-bound-objects --units {unit}` and additional `--shared-refs` / `--shared-ids` filters when the active flow has already identified them
   - if a touched Shared Contract file now has one or more formal bound units after this round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
9. update `docs/specs/_status.md`:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=unit_fork`
   - the deterministic row writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> status set-object --type unit --object {unit} --stable yes --candidate no --active-layer stable --next-command unit_fork --notes <status-note> --create`
10. if the round changed any unit `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, run `shared_sync` after `_status.md` has been updated, even when no additional affected unit is known yet
   - pass execution-local `current_stable_landing_unit={unit}` into that `shared_sync` run because this same round just wrote the unit's first stable truth together with its current stable Shared Contract binding
   - pass execution-local `stable_landing_shared_refs=<exact-shared-ref-list-written-by-this-landing>` into that same `shared_sync` run; `current_stable_landing_unit` alone is not sufficient
   - if any touched shared file changed only in `bound_objects` during this round, pass execution-local `bound_objects_only_shared_file_refs` with the exact file refs for those files
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared sync-impact --shared-refs <shared-ref> --units {unit} --stable-landing-unit {unit} --stable-landing-shared-refs <exact-stable-landing-shared-ref-list>` or the corresponding `--shared-ids` form, and at least one shared trigger input must already be known before this deterministic execution starts
11. perform git close-out if required by policy

## 5. Stop Conditions

1. the first `stable` exists
2. `_status.md` registration is complete
3. Shared Contract side effects, if any, are closed
4. if onboarding discovered unresolved cross-unit shared truth, the command stopped and rerouted through `shared_ops` instead of writing duplicated unit-local `stable` truth
5. the command does not automatically open a candidate round

## 6. Output Contract

1. onboarding judgment
2. created file path
3. whether `Global Constraint Alignment` was required and why
4. whether the command had to stop and reroute through `shared_ops` because shared-truth boundary closure was required before onboarding could continue
5. `_status.md` update result
6. Shared Contract reconciliation result when the round changed shared truth or bindings
7. git close-out result
8. next-step suggestion
9. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/docs/agent_guidelines/command_policy.md`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - `current state` must explicitly confirm the stable-layer landing written to `_status.md`
   - if the round stopped and rerouted through `shared_ops`, `next step` must name that reroute directly instead of implying that onboarding closed

## 7. Non-Goals

1. creating the first `candidate`
2. jumping directly into implementation
3. redesigning the unit
4. using first-time historical onboarding to bypass required shared-truth boundary closure

## 8. Example

```md
unit_init:ai
```

# Spec Init Command

## 1. Purpose

This command creates the first `stable` Spec for a historical module.

Goals:

1. capture the module's already-effective formal behavior
2. create the module's first formal truth file
3. register the module in `docs/specs/_status.md`

## 2. Scope

By default this command handles:

1. first-time governance onboarding of a historical module
2. modules that already have implementation and stable behavior but are not yet inside the Spec system
3. creation of the first `stable`

It does not handle:

1. creating a new module
2. forking a new candidate from existing `stable`
3. creating `candidate` directly
4. onboarding a historical module by first writing duplicated module-local formal truth when the real task is unresolved cross-module shared-truth governance

## 3. Preconditions

Before execution:

1. complete the required pre-checks from `spec_policy.md`; if the module is not registered yet, at minimum confirm there are no conflicting old status or leftover process files
2. the target module name is explicit
3. the module is not yet in `docs/specs/_status.md`
4. the goal is to capture current truth, not define future design
5. if onboarding current truth would create duplicated formal truth across modules, or if the shared/module boundary is still unstable, do not start `spec_init`; resolve that shared-truth boundary through `specflow/framework/docs/agent_guidelines/shared_ops.md` first
6. if the first `stable` reuses already-existing shared truth, read the relevant `shared_contract` files before writing `shared_contract_refs`
7. if the task also touches global baseline, shared mechanisms, or exceptions, read `docs/specs/system/stable/s_system_constraints.md`
8. if the module involves technical choices, shared infrastructure, cross-module reuse, global exceptions, or system-level constraint relationships, the first `stable` must include `Global Constraint Alignment` or an equivalent section
9. if the task changes `stable`, `_status.md`, or other commit-triggering governance files, read the git policy first
10. if the round creates, updates, or deletes any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/shared_sync.md` first
11. if the round may update `bound_modules` or remove intentional-unbound retention fields from a touched Shared Contract file, read every current-layer module main file needed to derive the real repository-wide binding set of each touched Shared Contract from `shared_contract_refs`

## 4. Procedure

1. summarize the module's already-effective behavior baseline
2. if needed, read `s_system_constraints.md` as an upstream input
3. if onboarding current truth shows that one or more existing formal modules already depend on the same formal truth and that truth is not yet formalized as one stable shared object, stop and reroute through `shared_ops:{natural-language request}` from current repository truth instead of writing duplicated module-local `stable` truth
4. create `docs/specs/modules/stable/s_{module}.md`
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
7. do not introduce `system_constraints_change_proposal` into the first `stable`; that field belongs only to module `candidate`
8. if the round changed Shared Contract bindings or touched Shared Contract files:
   - derive the real repository-wide binding set of each touched Shared Contract from current-layer module `shared_contract_refs` plus this round's prepared target-module stable writeback
   - if current repository truth is insufficient to derive that touched real binding set safely, stop and reroute through `shared_ops:{natural-language request}` from current repository truth instead of guessing
   - update `bound_modules` only as declarative metadata so each touched Shared Contract file matches the real binding set implied by that repository-wide binding view plus this round's prepared target-module writeback
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared reconcile-bound-modules --modules {module}` and additional `--shared-refs` / `--shared-ids` filters when the active flow has already identified them
   - if a touched Shared Contract file now has one or more formal bound modules after this round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
9. update `docs/specs/_status.md`:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=spec_fork`
   - the deterministic row writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> status set-module --module {module} --stable yes --candidate no --active-layer stable --next-command spec_fork --notes <status-note> --create`
10. if the round changed any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, run `shared_sync` after `_status.md` has been updated, even when no additional affected module is known yet
   - if any touched shared file changed only in `bound_modules` during this round, pass execution-local `bound_modules_only_shared_file_refs` with the exact file refs for those files
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared sync-impact --modules {module}` and additional `--shared-refs` / `--shared-ids` filters when the active flow has already identified them
11. perform git close-out if required by policy

## 5. Stop Conditions

1. the first `stable` exists
2. `_status.md` registration is complete
3. Shared Contract side effects, if any, are closed
4. if onboarding discovered unresolved cross-module shared truth, the command stopped and rerouted through `shared_ops` instead of writing duplicated module-local `stable` truth
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

## 7. Non-Goals

1. creating the first `candidate`
2. jumping directly into implementation
3. redesigning the module
4. using first-time historical onboarding to bypass required shared-truth boundary closure

## 8. Example

```md
spec_init:module_ai
```

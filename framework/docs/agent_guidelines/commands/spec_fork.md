# Spec Fork Command

## 1. Purpose

This command forks a new `candidate` Spec from an existing `stable`.

Goals:

1. open a new candidate-design round from the current formal version
2. provide candidate truth for downstream `cand_check / cand_plan / cand_impl`
3. update `docs/specs/_status.md`

## 2. Scope

By default it handles:

1. a new upgrade round for a module that already has `stable`
2. deriving candidate truth from formal truth
3. initializing the current round's `system_constraints_stable_ref`

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=spec_fork`
3. the module already has `stable`
4. read any stable appendix files explicitly referenced by `s_{module}.md`
5. read bound stable Shared Contract files if `shared_contract_refs` is not empty
6. read the git policy if commit-triggering files may change
7. if the round will create, update, or delete any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, read `shared_sync.md`

## 4. Procedure

1. read `s_system_constraints.md` if it exists; otherwise continue with no formal global baseline
2. read `docs/specs/stable/s_{module}.md` and any explicitly referenced appendix files
3. read bound stable Shared Contract files if any
4. determine the target formal version for this round:
   - compatible new capability -> next `MINOR`
   - incompatible change -> next `MAJOR`
   - compatible fix or alignment -> next `PATCH`
5. generate `docs/specs/candidate/c_{module}.md` from the current stable file
6. set candidate `frontmatter.version` to that target version
7. write `system_constraints_stable_ref`
   - if the new round proposes a global baseline change, record it in `system_constraints_change_proposal` inside the module candidate
8. re-check `shared_contract_refs`:
   - judge Shared Contract bindings independently from whether `s_system_constraints.md` exists
   - if the stable layer depended on shared files and the candidate still depends on the same unchanged shared truth, keep binding those existing shared files in the candidate
   - create or bind candidate-layer shared files only when the current round changes the shared truth itself
   - write `shared_contract_refs=none` only when the current round no longer reuses shared contract truth
   - do not write `shared_contract_refs=none` merely because a shared-truth change for this round has not yet been formalized
9. update Shared Contract `bound_modules` if the round changed shared bindings or shared files
10. delete old `_check_result/{module}.md`, `_verify_result/{module}.md`, `_plans/{module}.md`, and previous-round candidate appendix files
   - the deterministic cleanup part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> process cleanup-success --module {module} --mode spec_fork`
11. update `_status.md`:
   - `Stable=yes`
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=cand_check`
   - the deterministic row writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> status set-module --module {module} --stable yes --candidate yes --active-layer candidate --next-command cand_check --notes <status-note>`
12. if the round changed any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, run `shared_sync` only after `_status.md` already reflects `Active Layer=candidate` for this module, even when no additional affected module is known yet
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared sync-impact --modules {module}` and additional `--shared-refs` / `--shared-ids` filters when the active flow has already identified them
13. perform git close-out if required

## 5. Stop Conditions

1. the new `candidate` exists
2. previous-round process files are cleaned up
3. Shared Contract side effects are closed
4. `_status.md` is updated

## 6. Output Contract

1. fork decision
2. created file path
3. initialized candidate version
4. written formal global baseline reference or `none`
5. cleanup result
6. `_status.md` update result
7. Shared Contract reconciliation result when the round changed shared truth or bindings
8. git close-out result

## 7. Non-Goals

1. directly modifying `stable`
2. directly generating a plan
3. directly entering implementation
4. creating an independent `system_constraints` candidate file

## 8. Example

```md
spec_fork:module_ai
```

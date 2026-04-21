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

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the centralized authoritative-run and non-authoritative-follow-up rules from `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8 Rules 27-30.
Only a new independent full-scope run of `spec_fork` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=spec_fork`
3. the module already has `stable`
4. read `docs/specs/_status.md`
5. read any stable appendix files explicitly referenced by `s_{module}.md`
6. read bound stable Shared Contract files if `shared_contract_refs` is not empty
7. read the git policy if commit-triggering files may change
8. if the round will create, update, or delete any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, read `shared_sync.md`
9. if the round may remove, retarget, or otherwise change an existing Shared Contract binding, read every current-layer module main file needed to derive the real binding set of each touched Shared Contract from `shared_contract_refs`

## 4. Procedure

1. read `docs/specs/_status.md`
2. read `s_system_constraints.md` if it exists; otherwise continue with no formal global baseline
3. read `docs/specs/modules/stable/s_{module}.md` and any explicitly referenced appendix files
4. read bound stable Shared Contract files if any
5. determine the target formal version for this round:
   - compatible new capability -> next `MINOR`
   - incompatible change -> next `MAJOR`
   - compatible fix or alignment -> next `PATCH`
6. generate `docs/specs/modules/candidate/c_{module}.md` from the current stable file
7. set candidate `frontmatter.version` to that target version
8. write `system_constraints_stable_ref`
   - if the new round proposes a global baseline change, record it in `system_constraints_change_proposal` inside the module candidate
9. re-check `shared_contract_refs`:
   - interpret and rewrite that field using the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1
   - judge Shared Contract bindings independently from whether `s_system_constraints.md` exists
   - if the stable layer depended on shared files and the candidate still depends on the same unchanged shared truth, keep binding those existing shared files in the candidate
   - create or bind candidate-layer shared files only when the current round changes the shared truth itself
   - write `shared_contract_refs=none` only when the current round no longer reuses shared contract truth
   - do not write `shared_contract_refs=none` merely because a shared-truth change for this round has not yet been formalized
10. if Step 9 removes or retargets any existing Shared Contract binding:
   - derive the real repository-wide binding set of each touched Shared Contract from current-layer module `shared_contract_refs` plus the target module candidate writeback prepared in Step 9
   - if repository truth is insufficient to decide whether any touched Shared Contract file would become unbound after this round, stop and reroute through `shared_ops:{natural-language request}` from current repository truth instead of leaving cleanup ownership implicit
11. if the round changed shared bindings or shared files, resolve Shared Contract terminal state and `bound_modules` in the same round:
   - if a touched Shared Contract file would have no formal bound modules after this round, in the same round either delete it when cleanup is legal under `spec_policy.md` or explicitly keep it as independently authored shared truth by writing that file with:
     - `unbound_retention: intentional`
     - `unbound_retention_reason: <why this unbound state is intentional now>`
     - `unbound_retention_owner: spec_fork`
   - reject closure if neither deletion nor explicit keep-writeback has happened for a touched now-unbound Shared Contract file
   - if a touched Shared Contract file still has one or more formal bound modules after this round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
   - update `bound_modules` only as declarative metadata so each remaining touched Shared Contract file matches the real binding set implied by module `shared_contract_refs`
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared reconcile-bound-modules --modules {module}` and additional `--shared-refs` / `--shared-ids` filters when the active flow has already identified them
12. delete old `_check_result/{module}.md`, `_verify_result/{module}.md`, `_plans/{module}.md`, and previous-round candidate appendix files
   - the deterministic cleanup part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> process cleanup-success --module {module} --mode spec_fork`
13. update `_status.md`:
   - `Stable=yes`
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=cand_check`
   - the deterministic row writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> status set-module --module {module} --stable yes --candidate yes --active-layer candidate --next-command cand_check --notes <status-note>`
14. if the round changed any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, run `shared_sync` only after `_status.md` already reflects `Active Layer=candidate` for this module, even when no additional affected module is known yet
   - if any touched shared file changed only in `bound_modules` during this round, pass execution-local `bound_modules_only_shared_file_refs` with the exact file refs for those files
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared sync-impact --modules {module}` and additional `--shared-refs` / `--shared-ids` filters when the active flow has already identified them
   - if that `shared_sync` returns control because repository truth is still insufficient to continue safely, stop `spec_fork` as `blocked`, keep the newly created candidate-layer state in place, and reroute through `shared_ops:{natural-language request}` from current repository truth instead of claiming Shared Contract side effects are closed
15. perform git close-out if required

## 5. Stop Conditions

1. the new `candidate` exists
2. previous-round process files are cleaned up
3. Shared Contract side effects are closed
4. `_status.md` is updated
5. if a touched Shared Contract file became unbound because of this round's binding change, its terminal state is already resolved or the command has stopped and rerouted through `shared_ops`
6. if post-fork `shared_sync` could not continue safely, the command result is `blocked`, the candidate-layer state remains the current formal layer, and the required next step is rerunning `shared_ops` from current repository truth

## 6. Output Contract

1. fork decision
2. created file path
3. initialized candidate version
4. written formal global baseline reference or `none`
5. Shared Contract terminal-state result when the round changed shared bindings or shared files
6. cleanup result
7. `_status.md` update result
8. Shared Contract reconciliation result when the round changed shared truth or bindings
9. when post-fork `shared_sync` could not continue safely, that the command stopped as `blocked` and must resume through `shared_ops`
10. git close-out result
11. `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - `current state` must explicitly confirm the candidate-layer state written to `_status.md`
   - if post-fork follow-up is blocked on `shared_ops`, the block must name that reroute as the immediate `next step`

## 7. Non-Goals

1. directly modifying `stable`
2. directly generating a plan
3. directly entering implementation
4. creating an independent `system_constraints` candidate file
5. leaving a touched now-unbound Shared Contract file for later guesswork

## 8. Example

```md
spec_fork:module_ai
```

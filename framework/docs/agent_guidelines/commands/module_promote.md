# Module Promote Command

## 1. Purpose

This command promotes the specified module's `candidate` into the new `stable`.

## 2. Scope

By default it handles:

1. promoting the candidate version into the formal version
2. updating state files
3. cleaning this round's candidate and process files
4. updating `s_system_constraints.md` when a closed module-carried global proposal is ready
5. consuming the `module_verify -> module_promote` handoff only when verification still covers the current round

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `module_promote` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. complete required pre-checks
2. `_status.md` says `Next Command=module_promote`
3. a latest valid `_verify_result/{module}.md` still covers the current candidate, current implementation, and current formal global baseline state
4. implementation alignment is complete and no blocking verification issue remains
5. the candidate's `system_constraints_stable_ref` matches the current formal global baseline state
6. read required candidate appendix files and any Shared Contract files already bound by the module candidate or otherwise already known to be touched by this promotion round, and decide how each touched Shared Contract file will be handled after promotion
   - if any touched candidate-layer Shared Contract file already has a stable-layer sibling, also read that file's `promotion_owner_module`
7. read `specflow/framework/docs/agent_guidelines/recovery_policy.md` before promotion
8. if the round may create, update, or delete any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/shared_sync.md` before promotion
9. if the module candidate currently binds any candidate-layer Shared Contract file, or if the round may change the layer, version, or terminal state of any touched Shared Contract file, read `docs/specs/_status.md` and every affected module current-layer main file needed to derive the real repository-wide binding set from `shared_contract_refs` before file mutation starts
10. if repository truth is insufficient to derive that real binding set safely, do not start file mutation; reroute through `shared_ops:{natural-language request}` from current repository truth instead of guessing promotion-local topology
11. read the git policy before promotion

## 4. Procedure

1. read and re-check the latest `_verify_result/{module}.md`
2. read `docs/specs/modules/candidate/c_{module}.md` and all required appendix files
3. validate the full binding relation of `_verify_result/{module}.md` according to the candidate handoff contract
4. if `_verify_result/{module}.md` is invalid, identify the reason and stop immediately:
   - if code changed after verification:
     - delete `_verify_result/{module}.md`
     - fall back to `module_verify`
   - if implementation drift against candidate exists:
     - delete `_verify_result/{module}.md`
     - fall back to `module_impl`
   - if another required binding of `_verify_result/{module}.md` no longer matches the current round:
     - delete `_check_result/{module}.md`
     - delete `_plans/draft/{module}.md`
     - delete `_plans/active/{module}.md`
     - delete `_verify_result/{module}.md`
     - use `fallback_reason_code=binding_drift` and fall back to `module_check`
   - if bound Shared Contract truth, layer, version, or snapshot drifted:
     - delete `_check_result/{module}.md`
     - delete `_plans/draft/{module}.md`
     - delete `_plans/active/{module}.md`
     - delete `_verify_result/{module}.md`
     - use `fallback_reason_code=shared_contract_drift` and fall back to `module_check`
   - if candidate truth or formal global baseline changed:
     - delete `_check_result/{module}.md`
     - delete `_plans/draft/{module}.md`
     - delete `_plans/active/{module}.md`
     - delete `_verify_result/{module}.md`
     - fall back to `module_check`
5. continue only when bindings, coverage, and gate fields all remain valid
6. before the first file mutation, capture the recovery baseline required by `recovery_policy.md`
7. confirm that candidate `frontmatter.version` is the new `stable` version for this round
8. if the round touches any Shared Contract file, Shared Contract layer/version target, or Shared Contract terminal state, build the repository-wide real binding view for every touched shared item before deciding post-promotion topology:
   - start from `docs/specs/_status.md`
   - read every affected module current-layer main file needed to derive which modules currently bind each touched Shared Contract file or sibling layer through `shared_contract_refs`
   - interpret every module-side `shared_contract_refs` through the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1 before deriving that real binding view
   - treat module `shared_contract_refs` as the formal source of the real binding set rather than `bound_modules`
   - if repository truth is insufficient to state the post-promotion topology safely, stop before file mutation and reroute through `shared_ops:{natural-language request}` from current repository truth
9. if the round touches any Shared Contract file, Shared Contract layer/version target, or Shared Contract terminal state, decide for each touched shared item against that repository-wide binding view:
   - determine the post-promotion binding target for the promoted module stable truth; a promoted stable module must not keep binding a candidate-layer Shared Contract file
   - if it should remain an independent cross-module truth after promotion, promote it into `docs/specs/shared_contracts/stable/`
   - when this round writes or updates a stable-layer Shared Contract file, use the already-decided candidate `shared_version` for that file; do not invent or bump a Shared Contract version during module promotion itself
   - when this round writes or updates a stable-layer Shared Contract file from a candidate-layer Shared Contract file that already had a stable-layer sibling before promotion, require that candidate file's `promotion_owner_module` to equal the promoted module name; otherwise stop before file mutation and reroute through `shared_ops:{natural-language request}`
   - if a candidate-layer Shared Contract file for the same `shared_contract_id` remains in place after this round lands the stable-layer Shared Contract file, rewrite that remaining candidate-layer file in the same round as an explicit next-round draft:
     - set its `shared_version` to the intended next stable version after the just-landed stable file
     - write exactly one next `promotion_owner_module`
     - do not leave it as a candidate-layer duplicate of the just-landed stable truth
   - if current repository truth is insufficient to define that retained next-round draft or its next `promotion_owner_module` safely, stop before file mutation and reroute through `shared_ops:{natural-language request}`
   - if part of its conclusion has become a project-wide default rule, also absorb that specific conclusion into `s_system_constraints.md`
   - do not absorb a Shared Contract into module `stable` merely because promotion happened
   - do not treat promotion itself as a reason to delete a still-needed Shared Contract
   - if the round changed a shared item that has both stable-layer and candidate-layer files, resolve which modules are expected to remain bound to each layer after promotion from the repository-wide binding view before continuing
   - if this round's topology change or linked `system_constraints` absorption would leave a touched Shared Contract file with no formal bound modules, this promotion round owns resolving that file's terminal state instead of leaving orphaned shared truth for later cleanup
   - if such a touched file now has no formal bound modules and cleanup is legal under `spec_policy.md`, delete it in this round when it has been replaced by the promoted target or when its remaining conclusion has been fully absorbed into `s_system_constraints.md`
   - if such a touched file now has no formal bound modules and the round intentionally keeps it as independently authored shared truth, write that same file with:
     - `unbound_retention: intentional`
     - `unbound_retention_reason: <why this unbound state is intentional now>`
     - `unbound_retention_owner: module_promote`
   - if the required post-promotion truth shape is still unclear, or the round cannot safely judge whether an unbound touched file should be deleted or kept as independently authored shared truth, stop promotion and require rerouting through `shared_ops:{natural-language request}` from current repository truth instead of guessing a module-local-only continuation
10. if the module candidate contains a closed `system_constraints_change_proposal` that this round has implemented and verified, absorb the promoted conclusion into `docs/specs/system/stable/s_system_constraints.md`
11. generate or update `docs/specs/modules/stable/s_{module}.md`
12. if current-round candidate appendix files exist, in the same promotion round either:
   - migrate retained content to `docs/specs/modules/stable/appendix/` or an equivalent dedicated subdirectory
   - absorb the content into `docs/specs/modules/stable/s_{module}.md`
   - delete candidate appendix files no longer needed
13. do not delete `docs/specs/modules/candidate/c_{module}.md` until `_status.md` has already been updated to `Candidate=no`
14. update `_status.md` to the promoted stable state:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=module_fork`
   - the deterministic row writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> status set-module --module {module} --stable yes --candidate no --active-layer stable --next-command module_fork --notes <status-note>`
15. if the round touched any Shared Contract file, before `shared_sync`, update `bound_modules` for every remaining touched Shared Contract file only after Step 11 has written the promoted module stable truth and Step 14 has updated `_status.md`, so each surviving stable-layer or candidate-layer file matches the real post-promotion binding set implied by module `shared_contract_refs`
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared reconcile-bound-modules --modules {module}` and additional `--shared-refs` / `--shared-ids` filters when the active flow has already identified them
   - if a remaining touched Shared Contract file now has one or more formal bound modules after this promotion round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
16. only after that update may physical deletion happen:
   - `docs/specs/modules/candidate/c_{module}.md`
   - current-round candidate appendix files
   - `_check_result/{module}.md`
   - `_plans/draft/{module}.md`
   - `_plans/active/{module}.md`
   - `_verify_result/{module}.md`
   - the deterministic cleanup part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> process cleanup-success --module {module} --mode module_promote`
17. if the command is interrupted after promotion internals started but before final cleanup finished, run incomplete promotion recovery according to `recovery_policy.md` instead of claiming success
18. if the round changed any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, run `shared_sync` only after `_status.md` already reflects the promoted stable layer and Step 15 has written the surviving shared-file metadata, even when no additional affected module is known yet
   - this post-promotion `shared_sync` closes external affected-module fallout and shared-state reconciliation; it must not overturn the promoted module's own successful stable landing merely because the same promotion round also wrote the stable Shared Contract file or stable binding that the promoted module now legally uses
   - pass execution-local `current_stable_landing_module={module}` into that `shared_sync` run
   - pass execution-local `stable_landing_shared_refs=<exact-shared-ref-list-written-by-this-landing>` into that same `shared_sync` run; `current_stable_landing_module` alone is not sufficient
   - if any surviving touched shared file changed only in `bound_modules` during this round, also pass execution-local `bound_modules_only_shared_file_refs` with the exact file refs for those files
   - the deterministic reconciliation part may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> shared sync-impact --shared-refs <shared-ref> --modules {module} --stable-landing-module {module} --stable-landing-shared-refs <exact-stable-landing-shared-ref-list>` or the corresponding `--shared-ids` form, and at least one shared trigger input must already be known before this deterministic execution starts
   - if that post-promotion `shared_sync` returns control because repository truth is still insufficient to continue safely, do not claim promotion success:
     - immediately run incomplete promotion recovery according to `recovery_policy.md`
     - after recovery, require rerouting through `shared_ops:{natural-language request}` from the restored candidate-layer repository truth
     - do not leave the repository in partially promoted semantics while waiting for shared-governance clarification
19. perform git close-out if required

## 5. Stop Conditions

1. promotion succeeded or a blocking reason is explicit
2. `_status.md` is updated to:
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=module_fork`
3. this round's candidate cleanup is complete
4. if verification became invalid, the command stopped and `_status.md` fell back appropriately
5. if the command entered incomplete-promotion recovery state, candidate semantics were restored and the module can restart from `module_check`
6. if post-promotion `shared_sync` could not continue safely, incomplete promotion recovery is complete and the next required action is rerunning `shared_ops` from restored candidate truth before any later candidate-chain restart
7. if a candidate-layer Shared Contract sibling remains after promotion, its next-round draft state is already explicit for the current repository truth

## 6. Output Contract

1. promotion conclusion
2. formal version confirmation result
3. file and state update result
4. `system_constraints` linked-promotion result
5. post-promotion Shared Contract topology result, including which shared files remain at stable, which remain at candidate, and which binding target the promoted module now uses
6. `promotion_owner_module` validation result for each touched candidate-layer Shared Contract file that already had a stable-layer sibling before promotion
7. next-round draft rewrite result for each candidate-layer Shared Contract file retained after this promotion created or updated the stable-layer sibling
8. `bound_modules` writeback result for every remaining touched Shared Contract file after post-promotion topology was decided
9. terminal-state result for any touched Shared Contract file that became unbound in this round
10. Shared Contract reconciliation result when the round changed shared truth or bindings
11. cleanup result
12. `handoff validation result`
13. fallback cleanup result when verification became invalid before promotion could start
14. `fallback_reason_code` if verification became invalid
15. fallback reason if verification became invalid
16. `fallback_reason_code=promotion_recovery` when incomplete promotion recovery occurred
17. recovery-state explanation if incomplete promotion occurred
18. when post-promotion `shared_sync` was executed, the passed `current_stable_landing_module` value
19. when post-promotion `shared_sync` was executed, the passed `stable_landing_shared_refs` value
20. when post-promotion `shared_sync` was executed, the passed `bound_modules_only_shared_file_refs` value when present
21. when promotion stopped because post-promotion Shared Contract topology, retained candidate next-round draft shape, `promotion_owner_module`, unbound-file terminal state, or post-promotion `shared_sync` uncertainty was unclear, the required next step through `shared_ops`
22. git close-out result
23. follow-up state explanation
   - when promotion succeeds, the follow-up state must explicitly confirm:
     - `Stable=yes`
     - `Candidate=no`
     - `Active Layer=stable`
     - `Next Command=module_fork`
   - when promotion recovery occurred because post-promotion `shared_sync` could not continue safely, the follow-up state must explicitly confirm:
     - `Stable=yes|no` restored from the recovery baseline
     - `Candidate=yes`
     - `Active Layer=candidate`
     - `Next Command=module_check`
     - `resume through shared_ops` before any later promotion retry
23. the `user-facing close-out block` required by Section 8.6 of `specflow/framework/docs/agent_guidelines/command_policy.md`
   - report `round conclusion`, `current state`, `next step`, `why this next step`, and `next-stage entry gap`
   - when promotion recovery or shared-governance reroute occurred, also report `resume signal`
   - `current state` must match the post-promotion or post-recovery state actually restored in `_status.md`

Allowed checkpoint types:

1. none

Allowed `fallback_reason_code` values:

1. `truth_drift`
2. `binding_drift`
3. `baseline_drift`
4. `shared_contract_drift`
5. `implementation_deviation`
6. `evidence_incomplete`
7. `promotion_recovery`

## 7. Non-Goals

1. redesigning `candidate`
2. replacing implementation verification with promotion
3. automatically opening the next candidate round
4. maintaining an independent `system_constraints` candidate file

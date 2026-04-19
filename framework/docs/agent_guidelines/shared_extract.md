# Shared Extract Flow

## 1. Purpose

`shared_extract` is the internal flow for extracting already-existing module truth into one independent `shared_contract`.

It answers four questions:

1. whether multiple modules really depend on the same formal truth now
2. which part of current module-local truth should move into one shared object
3. how module-side truth must be rewritten so duplicate formal truth no longer remains
4. how the repository must be reconciled after the shared extraction lands

This is not a user-facing command entry.
The user reaches it through `shared_ops:{natural-language request}`.

---

## 2. Scope

By default it handles requests where shared truth already exists inside one or more modules and now needs to be extracted.

It may:

1. create or update a candidate-layer `shared_contract`
2. rewrite module candidate-side references and boundary explanation
3. remove duplicate formal truth from the source module candidate side
4. update the target shared file's declarative `bound_modules` metadata so it matches the real binding set after extraction writeback
5. trigger `shared_sync` after any shared-truth or binding writeback
6. stop at a `shared_ops` checkpoint when any source or consumer module is currently at `stable`

It does not:

1. design new shared truth from scratch when no module-local source truth exists
2. bind a module to an already-stable shared truth as the only task
3. replace module lifecycle commands
4. promote shared truth into `system_constraints`

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/docs/agent_guidelines/spec_policy.md`
2. read `specflow/framework/docs/agent_guidelines/command_policy.md`
3. read `docs/specs/_status.md` and use it as the repository-wide formal module index for this extraction
4. resolve each named module's current layer from `_status.md` before reading its main Spec
5. read the source module current-layer main files and any explicitly referenced appendix truth involved in the extraction
6. build the repository-wide involved-module set needed for this extraction from current repository truth before writeback starts:
   - start from the current formal module set recorded in `_status.md`
   - start from the named source modules and any named consumer modules
   - read every additional current-layer module main file needed to judge whether that module still carries, duplicates, or already consumes the target truth
   - do not treat the source module list alone as sufficient when the extraction target may already be reused elsewhere
7. read any relevant existing `shared_contract` files that may overlap the target truth
8. read `docs/specs/system/stable/s_system_constraints.md` when the request may cross into project-wide default-rule promotion
9. if any involved module is currently at `stable`, also read `specflow/framework/docs/agent_guidelines/commands/spec_fork.md`
10. if the round may create, update, or delete any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/shared_sync.md` first
11. if the round may create or update any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/git_policy.md` because Shared Contract semantic version rules apply

---

## 4. Procedure

1. confirm the request is really about extracting already-existing module-local formal truth
2. identify the smallest shared object that multiple modules truly depend on
3. resolve the complete involved-module set from current repository truth before writeback:
   - identify which modules currently carry duplicate module-local truth for that object
   - identify which modules already consume that object through `shared_contract_refs`
   - reject closure if consumer coverage is still uncertain
4. derive the writeback-required involved-module subset for this round from the already-resolved involved-module set:
   - include each source module whose current-layer formal truth must be rewritten so the extracted object no longer remains duplicated as module-local truth
   - include each consumer module whose current-layer `shared_contract_refs` or body-level consumption explanation must change because of the extraction result
   - do not require writeback for an involved module that is read only to confirm consumer coverage and whose current-layer truth already aligns with the extraction result
5. if any writeback-required involved module current layer is `stable`, do not modify that module `stable` directly:
   - raise a blocking `shared_ops` checkpoint with `type=prerequisite_action`
   - require `spec_fork:{module}` for each such module before extraction continues
   - set `required_writeback_target` to the corresponding module candidate main file set because chat-only agreement does not create legal extraction targets
6. create or update the target candidate-layer `shared_contract`
7. if Step 6 created the first file for a brand-new shared object, initialize `shared_version=0.1.0`
8. if Step 6 reopened an already-stable shared object at the candidate layer, set the candidate `shared_version` to the intended next stable version according to Shared Contract semantic version rules
9. rewrite every source module candidate side so the extracted truth is no longer duplicated as module-local formal truth
10. rewrite every additional writeback-required involved consumer module candidate-side reference and behavior explanation required by the extraction result
   - any written `shared_contract_refs` must use the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1
11. update the target shared file's `bound_modules` only as declarative metadata so it matches the real binding set implied by module-side `shared_contract_refs`
12. if the target shared file now has one or more formal bound modules after this round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
13. if duplicate formal truth still remains after extraction, stop and report boundary closure failure
14. if any involved module that should now consume the extracted truth was not fully reviewed and rewritten where required, stop and report consumer-coverage failure
15. after any write to `docs/specs/shared_contracts/**` or any module `shared_contract_refs`, execute `shared_sync` before claiming closure

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the shared extraction is complete, duplicate formal truth is removed, and `shared_sync` has finished reconciliation
   - the target shared file `bound_modules` metadata must already match the real module-side binding set
   - involved consumer coverage must already be complete for the current repository truth
2. the request is not really extraction and must be re-routed to another shared flow
3. one or more writeback-required involved modules are currently at `stable` and the flow has raised a `shared_ops` checkpoint for `spec_fork` first
4. module-private truth versus shared truth is still not stably separable
5. involved consumer coverage is still incomplete or uncertain, so the flow cannot claim extraction closure yet
6. the request has crossed into `system_constraints_change_proposal` and must stop at a `shared_ops` checkpoint instead of continuing here

---

## 6. Output Contract

The output must include at least:

1. the extracted shared object and why it belongs to `shared_extract`
2. the complete involved-module set used for the extraction decision
3. which involved modules were source modules, which were already consumer modules, which required writeback in this round, and which had to stop for `spec_fork`
4. the source module files that originally carried the truth
5. the target shared-contract file written or updated, or the checkpoint result when extraction could not legally start yet
6. the written `shared_version` and why it is correct for the current round
7. the module candidate-side rewrite result and whether duplicate formal truth was fully removed
8. whether involved consumer coverage is complete for the current repository truth
9. the target shared file `bound_modules` reconciliation result
10. the `shared_sync` result, including affected modules and fallback if any
11. the git close-out result when governance files or commit-triggering files were changed

---

## 7. Non-Goals

`shared_extract` does not:

1. preserve two formal truths for the same object
2. skip module-side boundary rewrite after creating a shared file
3. leave reconciliation for later after changing shared truth or bindings
4. modify module `stable` truth directly
5. absorb shared conclusions into `system_constraints`

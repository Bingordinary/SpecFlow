# Shared Bind Flow

## 1. Purpose

`shared_bind` is the internal flow for binding a module to an already-existing `shared_contract`.

It answers four questions:

1. whether the module truly depends on the target shared truth
2. which current-layer module file should record that binding
3. how the module body must explain real consumption of the shared truth
4. how the repository must be reconciled after the binding changes

This is not a user-facing command entry.
The user reaches it through `shared_ops:{natural-language request}`.

---

## 2. Scope

By default it handles requests where a `shared_contract` already exists and a module now needs to consume it.

It may:

1. update the module candidate-layer `shared_contract_refs`
2. update module candidate body text so the behavior chain explains how that shared truth is consumed
3. update the target shared file's declarative `bound_modules` metadata so it matches the real binding set after this round's binding writeback
4. when the round retargets a module away from one shared file to another, update the previous shared file's declarative `bound_modules` metadata too
5. when the round retargets a module away from one shared file to another and the previous shared file becomes unbound, resolve that previous file's terminal state or stop through shared governance instead of leaving orphaned shared truth
6. trigger `shared_sync` after any binding change
7. stop at a `shared_ops` checkpoint when the target module is currently at `stable`

It does not:

1. redesign the shared truth itself as the main task
2. extract module-local truth into a new shared object as the main task
3. replace module lifecycle commands
4. allow a ref-only binding with no body-level consumption explanation

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/docs/agent_guidelines/spec_policy.md`
2. read `specflow/framework/docs/agent_guidelines/command_policy.md`
3. read `docs/specs/_status.md`
4. resolve the target module's current layer from `_status.md` before reading its main Spec
5. read the target module current-layer main Spec
6. read the target `shared_contract`
7. if the target module current-layer main Spec already binds another Shared Contract file and this round may retarget that binding, also read that currently bound Shared Contract file
8. read `docs/specs/system/stable/s_system_constraints.md` when the request may cross into project-wide default-rule promotion
9. if the target module is currently at `stable`, also read `specflow/framework/docs/agent_guidelines/commands/spec_fork.md`
10. read `specflow/framework/docs/agent_guidelines/git_policy.md` when the round may change module `shared_contract_refs`, update `bound_modules`, delete a touched Shared Contract file, or otherwise mutate commit-triggering governance files
11. if the round may create, update, or delete any module `shared_contract_refs` value or any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/shared_sync.md` first

---

## 4. Procedure

1. confirm the target module truly reuses the target shared truth rather than merely sharing a topic or naming style
2. if the target module current layer is `stable`, do not modify `stable` directly:
   - raise a blocking `shared_ops` checkpoint with `type=prerequisite_action`
   - require `spec_fork:{module}` to create the target module candidate first
   - set `required_writeback_target` to that module candidate main file because chat-only agreement does not create a legal binding target
3. if the module current-layer binding already points to another Shared Contract file and this round is retargeting that binding, record the previous bound Shared Contract file before writeback
4. update the module candidate-layer `shared_contract_refs` using the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1
5. update module candidate body text so the relevant behavior chain explains which behavior consumes the shared truth
6. update the target shared file's `bound_modules` only as declarative metadata so it matches the real binding set implied by module-side `shared_contract_refs`
7. if Step 3 recorded a previous bound Shared Contract file and it is different from the new target file, update that previous shared file's `bound_modules` to remove the module from the old declarative binding set
8. for every touched shared file that still has one or more formal bound modules after Steps 6 and 7, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
9. reject closure if the change is only a `shared_contract_refs` edit with no body-level consumption explanation
10. after any change to module `shared_contract_refs` or to any shared file metadata touched in Steps 6, 7, and 8, execute `shared_sync` before claiming closure
11. if Step 3 recorded a previous bound Shared Contract file and `shared_sync` shows that no module still binds it after this round:
   - if the current round can safely prove that the previous file has been replaced by the new target and cleanup is legal under `spec_policy.md`, delete that now-unbound previous shared file in the same round
   - otherwise, stop and return control to `shared_escape` through `shared_ops` so shared governance can decide whether stable decomposition exists or whether follow-up must route to `shared_topology`, checkpoint, or another legal next step
   - after a deletion in this step, rerun `shared_sync` before claiming closure

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the module binding, body-level consumption explanation, and target shared file `bound_modules` metadata are complete and `shared_sync` has finished reconciliation
   - when the round retargeted away from a previous shared file, that previous file's terminal state must also be resolved before closure
2. the request is not really binding and must be re-routed to another shared flow
3. the target module does not actually depend on the shared truth
4. the target module is currently at `stable` and the flow has raised a `shared_ops` checkpoint for `spec_fork:{module}` first
5. the request has crossed into `system_constraints_change_proposal` and must stop at a `shared_ops` checkpoint instead of continuing here

---

## 6. Output Contract

The output must include at least:

1. the target module and target shared contract
2. why the module truly depends on that shared truth
3. whether the target module was already at `candidate` or first had to stop for `spec_fork:{module}`
4. the binding writeback result in the module candidate-layer Spec, or the checkpoint result when candidate writeback could not start yet
5. the body-level consumption explanation added or updated
6. the target shared file `bound_modules` reconciliation result
7. when the round retargeted the module away from a previous shared file, the previous shared file `bound_modules` reconciliation result
8. when the round retargeted the module away from a previous shared file, the previous shared file terminal-state result, including any return-to-`shared_escape` result when direct cleanup was not yet safe
9. the `shared_sync` result, including affected modules and fallback if any
10. the git close-out result when governance files or commit-triggering files were changed

---

## 7. Non-Goals

`shared_bind` does not:

1. allow ref-only binding without behavior explanation
2. redesign the shared truth as the main task
3. leave reconciliation for later after changing module bindings
4. modify module `stable` truth directly
5. absorb shared conclusions into `system_constraints`

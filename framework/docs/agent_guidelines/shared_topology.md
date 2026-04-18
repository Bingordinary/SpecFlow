# Shared Topology Flow

## 1. Purpose

`shared_topology` is the internal flow for Shared Contract structural change and terminal-state resolution.

It answers four questions:

1. whether the current request is really about Shared Contract topology rather than simple authoring, extraction, binding, or impact closure
2. which touched shared objects remain, which new shared objects must exist, and which old ones must end in this round
3. which module candidate-side bindings and body explanations must be rewritten because of that topology change
4. how the repository must be reconciled after the topology change lands

This is not a user-facing command entry.
The user reaches it through `shared_ops:{natural-language request}`.

---

## 2. Scope

By default it handles requests where one or more existing `shared_contract` objects need structural topology change or terminal-state resolution.

It may:

1. split one shared object into multiple shared objects
2. merge multiple shared objects into one shared object
3. rename, replace, or retire an existing shared object
4. explicitly keep a touched unbound shared file as independently authored shared truth when that outcome is written clearly in the same round
5. rewrite affected module candidate-side `shared_contract_refs` and body-level consumption explanations when the topology change changes what those modules consume
6. create, update, or delete candidate-layer Shared Contract files as required by the topology change
7. delete touched stable-layer Shared Contract files only when they are already unbound and cleanup is legal under `spec_policy.md`
8. keep an existing stable-layer Shared Contract file unchanged when the topology plan intentionally leaves it in place
9. trigger `shared_sync` after any shared-truth or binding writeback
10. stop at a `shared_ops` checkpoint when legal module writeback targets do not exist yet

It does not:

1. replace `shared_bind` when the main task is only one module binding to an unchanged existing shared object
2. replace `shared_new` when the main task is first-time shared authoring with no existing shared topology change
3. replace `shared_extract` when the main task is only extracting module-local truth into one shared object
4. create or update a stable-layer Shared Contract file directly just to carry new topology semantics or a new `shared_version`
5. create an independent Shared Contract lifecycle outside `shared_ops`

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/docs/agent_guidelines/spec_policy.md`
2. read `specflow/framework/docs/agent_guidelines/command_policy.md`
3. read `specflow/framework/docs/agent_guidelines/shared_sync.md`
4. read `specflow/framework/docs/agent_guidelines/git_policy.md` because Shared Contract semantic version rules apply
5. read `docs/specs/_status.md`
6. read each touched `shared_contract` file that may be split, merged, renamed, replaced, retired, or explicitly kept
7. resolve every affected module's current layer from `_status.md` before reading its main Spec
8. read every affected module current-layer main file needed to derive the real binding set from `shared_contract_refs`
9. if any affected module is currently at `stable` and the topology change would require module truth writeback, also read `specflow/framework/docs/agent_guidelines/commands/spec_fork.md`
10. read `docs/specs/system/stable/s_system_constraints.md` when the topology request may cross into project-wide default-rule promotion
11. if this round may raise a checkpoint, read `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`

---

## 4. Procedure

1. confirm the request is really about Shared Contract topology change or terminal-state resolution rather than `shared_new`, `shared_extract`, `shared_bind`, or `shared_sync`
2. identify the touched shared objects and the real affected-module set from module `shared_contract_refs` rather than from `bound_modules`
3. if any affected module current layer is `stable` and the topology change would require module truth writeback:
   - raise a blocking `shared_ops` checkpoint with `type=prerequisite_action`
   - require `spec_fork:{module}` for each such module before topology writeback continues
   - set `required_writeback_target` to the corresponding module candidate main file set because chat-only agreement does not create legal topology-writeback targets
4. decide the current-round topology plan explicitly:
   - which touched shared object identity remains the same
   - which new shared object identities must be created
   - which touched shared files must be deleted in this round
   - which touched shared files will remain intentionally unbound as independently authored shared truth
5. if the current repository truth is not sufficient to stabilize Step 4, stop this flow and return control to `shared_escape` through `shared_ops` instead of guessing
6. create, update, or delete the touched candidate-layer Shared Contract files according to the topology plan:
   - if the round creates the first file for a brand-new shared object, initialize `shared_version=0.1.0`
   - if the round opens or rewrites a candidate-layer file for a shared object that already has a stable-layer sibling, set that candidate file's `shared_version` to the intended next stable version according to Shared Contract semantic version rules
   - if the topology plan needs new or changed stable-layer shared semantics, do not write that stable-layer file directly in this flow; write or update the corresponding candidate-layer shared file first, carry the intended next stable `shared_version` there, and let a later legal promotion produce the stable-layer file
7. rewrite every affected module candidate-side `shared_contract_refs` and body-level consumption explanation required by the topology plan
8. for each touched shared file that has no formal bound modules after Step 7:
   - delete it in the same round when the topology plan treats it as retired and cleanup is legal under `spec_policy.md`
   - otherwise keep it only when the current round explicitly records that it remains independently authored shared truth and why that unbound state is intentional
   - reject closure if neither deletion nor explicit keep-writeback has happened
9. update `bound_modules` only as declarative metadata so every remaining touched shared file matches the real binding set implied by module-side `shared_contract_refs`
10. after any write to `docs/specs/shared_contracts/**` or any module `shared_contract_refs`, execute `shared_sync` before claiming closure
11. if `shared_sync` stops because repository truth is insufficient to continue safely, return control to `shared_escape` through `shared_ops` instead of inventing a flow-local checkpoint

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the topology change is complete, every touched shared file's terminal state is resolved, and `shared_sync` has finished reconciliation
2. the request is not really topology change and must be re-routed to another shared flow
3. one or more affected modules are currently at `stable` and the flow has raised a `shared_ops` checkpoint for `spec_fork` first
4. repository truth is insufficient to continue safely, so control has returned to `shared_escape` through `shared_ops`
5. the topology plan requires new or changed stable-layer shared semantics, so this flow has written or updated the required candidate-layer Shared Contract files and stopped without direct stable-layer writeback
6. the request has crossed into `system_constraints_change_proposal`, so control has returned to `shared_escape` through `shared_ops` for checkpoint handling instead of continuing here

---

## 6. Output Contract

The output must include at least:

1. the recognized topology intent and why it belongs to `shared_topology`
2. the touched shared objects and the affected modules
3. the explicit topology result for this round:
   - which shared objects remain
   - which new shared objects were created
   - which touched shared files were deleted
   - which touched shared files remain intentionally unbound and why
4. the Shared Contract file writeback result, including the written `shared_version` for each created or updated candidate-layer file
5. the module candidate-side retarget or rewrite result
6. the `bound_modules` reconciliation result for each remaining touched shared file
7. the `shared_sync` result, including affected modules and fallback if any
8. the checkpoint result when candidate writeback could not legally start yet
9. whether the flow had to stop with candidate-layer shared truth prepared for a later legal promotion instead of writing a stable-layer shared file directly
10. the git close-out result when governance files or commit-triggering files were changed

Allowed checkpoint types:

1. `prerequisite_action`

---

## 7. Non-Goals

`shared_topology` does not:

1. guess whether an unstable boundary should become shared or stay module-private
2. replace `shared_escape` for decomposition when the repository truth is still ambiguous
3. allow silent retention of touched unbound shared files with no explicit keep-or-delete result
4. modify module `stable` truth directly
5. create or update a stable-layer Shared Contract file directly to introduce new topology semantics or a newly chosen `shared_version`
6. absorb Shared Contract conclusions into `system_constraints` automatically

# Rule Topology Flow

## 1. Purpose

`rule_topology` is the internal flow for Rule structural change and terminal-state resolution.

It answers four questions:

1. whether the current request is really about Rule topology rather than simple authoring, extraction, binding, or impact closure
2. which touched rule objects remain, which new rule objects must exist, and which old ones must end in this round
3. which unit or scenario candidate-side bindings and body explanations must be rewritten because of that topology change
4. how the repository must be reconciled after the topology change lands

This is not a user-facing command entry.
The user reaches it through natural-language routing when that routing enters the rule-governance branch.

---

## 2. Scope

By default it handles requests where one or more existing `rule` objects need structural topology change or terminal-state resolution.

It may:

1. split one rule object into multiple rule objects
2. merge multiple rule objects into one rule object
3. rename, replace, or retire an existing rule object
4. explicitly keep a touched unbound rule file as independently authored rule truth when that outcome is written clearly in the same round
5. rewrite affected unit or scenario candidate-side `rule_refs` and body-level consumption explanations when the topology change changes what those command-target objects consume
6. create, update, or delete candidate-layer Rule files as required by the topology change
7. delete touched stable-layer Rule files only when they are already unbound and cleanup is legal under `spec_policy.md`
8. keep an existing stable-layer Rule file unchanged when the topology plan intentionally leaves it in place
9. trigger `rule_sync` after any rule-truth or binding writeback
10. stop at a rule-governance checkpoint when legal unit writeback targets do not exist yet

It does not:

1. replace `rule_bind` when the main task is only one unit binding to an unchanged existing rule object
2. replace `rule_new` when the main task is first-time shared authoring with no existing rule topology change
3. replace `rule_extract` when the main task is only extracting unit-local truth into one rule object
4. create or update a stable-layer Rule file directly just to carry new topology semantics or a new `rule_version`
5. replace `unit_promote` when a promotion lands an owned candidate Rule as stable and retargets candidate units to the same-`rule_id`, same-`rule_version` stable Rule ref in the same round
6. create an independent Rule lifecycle outside rule governance

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/spec_policy.md`
2. read `specflow/framework/command_policy.md`
3. read `specflow/framework/rule_sync.md`
4. apply the Rule version rules from `specflow/framework/spec_policy.md` Section 6.3 when topology changes create or update rule files
5. read `docs/specs/_status.md`
6. read `docs/specs/repository_mapping.md` because topology changes may change the rule object map or rule truth-path rules
7. read each touched `rule` file that may be split, merged, renamed, replaced, retired, or explicitly kept
8. build the repository-wide affected command-target review set for the touched rule objects from current repository truth before topology planning:
   - start from the formal unit and scenario rows recorded in `_status.md`
   - read every additional current-layer unit or scenario main file needed to judge which command-target objects currently bind each touched rule object through `rule_refs`
   - do not treat only the user-named units, user-named scenarios, or currently obvious consumers as sufficient when other command-target objects may still bind the touched rule objects
9. resolve every affected unit or scenario's current layer from `_status.md` before reading its main Spec
10. read every affected unit or scenario current-layer main file needed to derive the real binding set from `rule_refs`
11. if any affected unit or scenario is currently at `stable` and the topology change would require command-target truth writeback, also read the corresponding fork command file:
   - `specflow/framework/commands/unit_fork.md` for units
   - `specflow/framework/commands/scenario_fork.md` for scenarios
12. read `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the topology request may cross into project-wide default-rule promotion
13. if this round may raise a checkpoint, read `specflow/framework/checkpoint_protocol.md`

---

## 4. Procedure

1. confirm the request is really about Rule topology change or terminal-state resolution rather than `rule_new`, `rule_extract`, `rule_bind`, or `rule_sync`
2. resolve the complete repository-wide affected command-target set for the touched rule objects from unit and scenario `rule_refs` rather than from `bound_objects`
3. if current repository truth is insufficient to derive that complete affected command-target set safely, stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing
4. if any affected unit or scenario current layer is `stable` and the topology change would require command-target truth writeback:
   - raise a blocking rule-governance checkpoint with `type=prerequisite_action`
   - require `unit_fork:{unit}` for each such unit and `scenario_fork:{scenario}` for each such scenario before topology writeback continues
   - set `required_writeback_target` to the corresponding future candidate main file set because chat-only agreement does not create legal topology-writeback targets
5. decide the current-round topology plan explicitly against that complete affected command-target set:
   - which touched rule object identity remains the same
   - which new rule object identities must be created
   - which touched rule files must be deleted in this round
   - which touched rule files will remain intentionally unbound as independently authored rule truth
6. if the current repository truth is not sufficient to stabilize Step 5, stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing
6.5. before any rule file writeback, capture the recovery baseline required by `specflow/framework/recovery_policy.md` Section 6.5.1:
     - every touched candidate-layer and stable-layer Rule file
     - every downstream unit or scenario candidate file that may be rewritten
     - `docs/specs/repository_mapping.md` when the round may change the rule object map
7. create, update, or delete the touched candidate-layer Rule files according to the topology plan:
   - if the round creates the first file for a brand-new rule object, initialize `rule_version=0.1.0`
   - if the round opens or rewrites a candidate-layer file for a rule object that already has a stable-layer sibling, set that candidate file's `rule_version` to the intended next stable version according to Rule semantic version rules
   - for each candidate-layer file from the previous bullet, write exactly one `promotion_owner_unit` into that file:
   - the owner must be a formal unit from the unit subset of the affected command-target set or another formal unit explicitly required by the topology plan
     - that owner is the only unit round allowed to land that candidate-layer rule file as the next stable-layer Rule file
     - the owner unit may remain bound to the current stable-layer shared sibling until a later legal unit candidate round rewrites its `rule_refs`
     - if current repository truth is insufficient to name one stable owner for such a file, stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing
   - if the topology plan needs new or changed stable-layer rule semantics, do not write that stable-layer file directly in this flow; write or update the corresponding candidate-layer rule file first, carry the intended next stable `rule_version` there, and let a later legal promotion produce the stable-layer file
8. rewrite every affected unit or scenario candidate-side `rule_refs` and body-level consumption explanation required by the topology plan
   - any written `rule_refs` must use the Rule binding contract from `specflow/framework/spec_policy.md` Section 6.1
9. for each touched rule file that has no formal bound units after Step 8:
   - delete it in the same round when the topology plan treats it as retired and cleanup is legal under `spec_policy.md`
   - otherwise keep it only when the current round writes that same Rule file with the fixed intentional-unbound retention frontmatter from `spec_policy.md`:
     - `unbound_retention: intentional`
     - `unbound_retention_reason: <why this unbound state is intentional now>`
     - `unbound_retention_owner: rule_topology`
   - reject closure if neither deletion nor explicit keep-writeback has happened
10. for each touched rule file that still has one or more formal bound units after Step 8, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
11. update `bound_objects` only as declarative metadata so every remaining touched rule file matches the real binding set implied by repository-wide unit and scenario `rule_refs` plus this round's prepared command-target writebacks
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule reconcile-bound-objects --rule-ids b_rule_x,b_rule_y` and additional `--rule-refs` filters when the active flow has already identified exact touched files
12. if the topology plan created, removed, renamed, split, merged, replaced, retired, or otherwise changed the current rule object map, update `docs/specs/repository_mapping.md` in the same round before executing `rule_sync`:
   - record every remaining current `rule` ID and one-line responsibility that changed because of this topology plan
   - remove retired rule IDs from the current object map only when the topology plan has legally resolved their terminal state
   - keep rule truth-path rules consistent with the resulting rule file locations
   - if current repository truth is insufficient to write the exact mapping update without guessing, stop this flow and return control to `rule_escape` through rule-governance routing
13. after any write to `docs/specs/rules/**` or any unit or scenario `rule_refs`, execute `rule_sync` before claiming closure
   - if any touched rule file changed only in `bound_objects` during this round, pass execution-local `bound_objects_only_rule_file_refs` with the exact file refs for those files
14. if `rule_sync` stops because repository truth is insufficient to continue safely, return control to `rule_escape` through rule-governance routing instead of inventing a flow-local checkpoint

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the topology change is complete, every touched rule file's terminal state is resolved, any required `repository_mapping.md` object-map writeback is complete, and `rule_sync` has finished reconciliation
2. the request is not really topology change and must be re-routed to another rule flow
3. one or more affected units or scenarios are currently at `stable` and the flow has raised a rule-governance checkpoint for the corresponding fork command first
4. repository truth is insufficient to continue safely, so control has returned to `rule_escape` through rule-governance routing
5. the topology plan requires new or changed stable-layer rule semantics, so this flow has completed the current-round candidate-layer Rule writeback and any required `rule_sync` without direct stable-layer writeback; any later stable-layer Rule file must be produced by a legal promotion rather than by this flow
7. the topology plan would leave a next-round candidate-layer rule file for an already-stable rule object without a stable `promotion_owner_unit`

---

## 6. Output Contract

The output must include at least:

1. the recognized topology intent and why it belongs to `rule_topology`
2. the touched rule objects and the repository-wide affected command-target objects
3. the explicit topology result for this round:
   - which rule objects remain
   - which new rule objects were created
   - which touched rule files were deleted
   - which touched rule files remain intentionally unbound and why
4. the Rule file writeback result, including the written `rule_version` for each created or updated candidate-layer file
5. for each created or rewritten candidate-layer file that already has a stable-layer sibling, the written `promotion_owner_unit`
6. the unit or scenario candidate-side retarget or rewrite result
7. the `bound_objects` reconciliation result for each remaining touched rule file
8. when the topology plan changed the current rule object map, the `docs/specs/repository_mapping.md` writeback result
9. the `rule_sync` result, including affected downstream objects and fallback if any
10. the checkpoint result when candidate writeback could not legally start yet
11. whether the flow had to stop with candidate-layer rule truth prepared for a later legal promotion instead of writing a stable-layer rule file directly

Allowed checkpoint types:

1. `prerequisite_action`

---

## 7. Non-Goals

`rule_topology` does not:

1. guess whether an unstable boundary should become shared or stay unit-local
2. replace `rule_escape` for decomposition when the repository truth is still ambiguous
3. allow silent retention of touched unbound rule files with no explicit keep-or-delete result
4. modify unit or scenario `stable` truth directly
5. create or update a stable-layer Rule file directly to introduce new topology semantics or a newly chosen `rule_version`
6. absorb Rule conclusions into stable `g_` rule automatically
7. own same-round stable landing retargeting that is already fully constrained by `unit_promote`

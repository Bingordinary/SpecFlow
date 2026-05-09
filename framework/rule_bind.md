# Rule Bind Flow

## 1. Purpose

`rule_bind` is the internal flow for binding a unit to an already-existing `rule`.

It answers four questions:

1. whether the unit truly depends on the target rule truth
2. which current-layer unit file should record that binding
3. how the unit body must explain real consumption of the rule truth
4. how the repository must be reconciled after the binding changes

This is not a user-facing command entry.
The user reaches it through natural-language routing when that routing enters the rule-governance branch.

---

## 2. Scope

By default it handles requests where a `rule` already exists and a unit now needs to consume it.

It may:

1. update the unit candidate-layer `rule_refs`
2. update unit candidate body text so the behavior chain explains how that rule truth is consumed
3. update the target rule file's declarative `bound_objects` metadata so it matches the real command-target binding set after this round's binding writeback
4. when the round retargets a unit away from one rule file to another, update the previous rule file's declarative `bound_objects` metadata too
5. when the round touches a candidate-layer Rule file that already has a stable-layer sibling, validate or rewrite that draft's `promotion_owner_unit` in the same round
6. when the round retargets a unit away from one rule file to another and the previous rule file becomes unbound, resolve that previous file's terminal state or stop through rule governance instead of leaving orphaned rule truth
7. trigger `rule_sync` after any binding change
8. invalidate target unit candidate process state after any target unit candidate main-file writeback
9. stop at a rule-governance checkpoint when the target unit is currently at `stable`

It does not:

1. redesign the rule truth itself as the main task
2. extract unit-local truth into a new rule object as the main task
3. replace unit lifecycle commands
4. allow a ref-only binding with no body-level consumption explanation

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/spec_policy.md`
2. read `specflow/framework/command_policy.md`
3. read `docs/specs/_status.md`
4. resolve the target unit's current layer from `_status.md` before reading its main Spec
5. read the target unit current-layer main Spec
6. read the target `rule`
7. if the target rule file is a candidate-layer Rule file that already has a stable-layer sibling, also read its current `promotion_owner_unit`
8. if the target unit current-layer main Spec already binds another Rule file and this round may retarget that binding, also read that currently bound Rule file
9. if the previously bound Rule file from Rule 8 is a candidate-layer Rule file that already has a stable-layer sibling, also read its current `promotion_owner_unit`
10. if the round may update `bound_objects`, validate or rewrite `promotion_owner_unit`, or resolve the terminal state of a touched Rule file, resolve the repository-wide real binding set of each touched Rule from current repository truth before writeback:
   - start from the formal unit and scenario rows recorded in `_status.md`
   - include the target rule file and any previous bound Rule file already identified for retarget review
   - read every additional current-layer unit or scenario main file needed to judge which command-target objects currently bind those touched Rule files through `rule_refs`
   - do not treat the target unit alone as sufficient when other units or scenarios may already bind the same rule truth
11. read `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may cross into project-wide default-rule promotion
12. if the target unit is currently at `stable`, also read `specflow/framework/commands/unit_fork.md`
13. if the round may create, update, or delete any unit `rule_refs` value or any file under `docs/specs/rules/**`, read `specflow/framework/rule_sync.md` first
14. if the round may update the target unit candidate main file, including `rule_refs` or body-level consumption explanation text, read `specflow/framework/impact_sync_policy.md` and `specflow/framework/recovery_policy.md` first

---

## 4. Procedure

1. confirm the target unit truly reuses the target rule truth rather than merely sharing a topic or naming style
2. if the target unit current layer is `stable`, do not modify `stable` directly:
   - raise a blocking rule-governance checkpoint with `type=prerequisite_action`
   - require `unit_fork:{unit}` to create the target unit candidate first
   - set `required_writeback_target` to that unit candidate main file because chat-only agreement does not create a legal binding target
3. if the unit current-layer binding already points to another Rule file and this round is retargeting that binding, record the previous bound Rule file before writeback
4. resolve the repository-wide real binding set of the target rule file and any previous bound Rule file from current repository truth before shared metadata writeback:
   - derive that set from current-layer unit and scenario `rule_refs` rather than from `bound_objects`
   - if current repository truth is insufficient to determine those touched real binding sets safely, stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing
4.5. before any unit candidate-side rule_refs write or rule-file metadata update, capture the recovery baseline required by `specflow/framework/recovery_policy.md` Section 6.5.1:
     - the target unit candidate main file
     - the touched rule file(s) that may be updated in bound_objects or promotion_owner_unit
     - `docs/specs/rules/**` files that may be created, updated, or deleted by this round
5. update the unit candidate-layer `rule_refs` using the Rule binding contract from `specflow/framework/spec_policy.md` Section 6.1
6. update unit candidate body text so the relevant behavior chain explains which behavior consumes the rule truth
7. for each touched candidate-layer Rule file from the target rule file or the previous bound Rule file recorded in Step 3 that already has a stable-layer sibling, validate that resulting draft's `promotion_owner_unit` against current repository truth plus this round's prepared unit writeback:
   - keep the current `promotion_owner_unit` only when that same formal unit still remains the one current repository truth leaves responsible for later legally adopting and promoting that draft
   - rewrite `promotion_owner_unit` in that touched candidate-layer Rule file in the same round when this binding change clearly moves that later adoption-and-promotion responsibility to another formal unit
   - if current repository truth is insufficient to keep or rewrite exactly one stable `promotion_owner_unit` without guessing, stop this flow and return control to `rule_escape` through rule-governance routing instead of claiming ordinary binding closure
8. update the target rule file's `bound_objects` only as declarative metadata so it matches the real binding set implied by repository-wide unit and scenario `rule_refs` plus this round's prepared target-unit writeback
9. if Step 3 recorded a previous bound Rule file and it is different from the new target file, update that previous rule file's `bound_objects` only after reconciling it against the same repository-wide command-target binding view plus this round's prepared target-unit writeback
   - the deterministic metadata writeback for Steps 8 and 9 may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule reconcile-bound-objects --rule-refs c_b_rule_x@0.1.0,s_b_rule_y@1.0.0` and additional `--rule-ids` filters when the active flow has already identified them
10. for every touched rule file that still has one or more formal bound units after Steps 8 and 9, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
11. reject closure if the change is only a `rule_refs` edit with no body-level consumption explanation
12. after any change to unit `rule_refs` or to any rule file metadata touched in Steps 7, 8, 9, and 10, execute `rule_sync` before claiming closure
   - if any touched rule file changed only in `bound_objects` during this round, pass execution-local `bound_objects_only_rule_file_refs` with the exact file refs for those files
13. after any target unit candidate main-file writeback in Step 5 or Step 6, invalidate the target unit candidate process state before claiming closure:
   - a target unit candidate main-file writeback includes any `rule_refs` write and any body-level consumption explanation write
   - if the writeback also changed `rule_refs` or any touched rule file metadata, the Step 12 `rule_sync` run may satisfy this requirement only when its handoff to `impact_sync` includes the target unit and `impact_sync` reports the required target-unit fallback and cleanup result
   - if the writeback changed only target unit candidate body text and did not change `rule_refs` or any touched rule file metadata, do not route through `rule_sync` as a fake rule change
   - for that body-only writeback path, execute the candidate fallback defined by `impact_sync_policy.md` and `recovery_policy.md` for the target unit:
     - update `_status.md` for the target unit to `Next Command=unit_check`
     - delete `_check_result/unit/{unit}.md`
     - delete `_plans/draft/{unit}.md`
     - delete `_plans/active/{unit}.md`
     - delete `_verify_result/unit/{unit}.md`
     - record each cleanup target that was already absent as absent, not as a different fallback state
   - a `bound_objects`-only metadata exception may suppress downstream invalidation caused only by shared metadata drift, but it must not suppress this target-unit invalidation because the target unit candidate main file changed
14. if Step 3 recorded a previous bound Rule file and `rule_sync` shows that no unit still binds it after this round:
   - if the current round can safely prove that the previous file has been replaced by the new target and cleanup is legal under `spec_policy.md`, delete that now-unbound previous rule file in the same round
   - otherwise, stop and return control to `rule_escape` through rule-governance routing so rule governance can decide whether stable decomposition exists or whether follow-up must route to `rule_topology`, checkpoint, or another legal next step
   - after a deletion in this step, rerun `rule_sync` before claiming closure

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the unit binding, body-level consumption explanation, and target rule file `bound_objects` metadata are complete and `rule_sync` has finished reconciliation
   - when the round retargeted away from a previous rule file, that previous file's terminal state must also be resolved before closure
   - when the target unit candidate main file changed, target-unit candidate process invalidation and `_status.md` fallback must also be complete before closure
2. the request is not really binding and must be re-routed to another rule flow
3. the target unit does not actually depend on the rule truth
4. the target unit is currently at `stable` and the flow has raised a rule-governance checkpoint for `unit_fork:{unit}` first
5. a touched candidate-layer Rule file with a stable-layer sibling cannot keep or receive one stable `promotion_owner_unit` from current repository truth after this round's binding change

---

## 6. Output Contract

The output must include at least:

1. the target unit and target rule
2. why the unit truly depends on that rule truth
3. whether the target unit was already at `candidate` or first had to stop for `unit_fork:{unit}`
4. the binding writeback result in the unit candidate-layer Spec, or the checkpoint result when candidate writeback could not start yet
5. the body-level consumption explanation added or updated
6. the repository-wide binding-set review result used for the target rule file and any previous rule file touched by retargeting, including unit and scenario bindings
7. the target rule file `bound_objects` reconciliation result
8. when the round retargeted the unit away from a previous rule file, the previous rule file `bound_objects` reconciliation result
9. when a touched candidate-layer Rule file already had a stable-layer sibling, the `promotion_owner_unit` keep-or-rewrite result
10. when the round retargeted the unit away from a previous rule file, the previous rule file terminal-state result, including any return-to-`rule_escape` result when direct cleanup was not yet safe
11. the `rule_sync` result, including affected downstream objects and fallback if any
12. the target unit candidate process invalidation result, including whether `rule_sync` handed the target unit to `impact_sync` or whether direct candidate fallback cleanup was executed for a body-only writeback

---

## 7. Non-Goals

`rule_bind` does not:

1. allow ref-only binding without behavior explanation
2. redesign the rule truth as the main task
3. leave reconciliation for later after changing unit bindings
4. modify unit `stable` truth directly
5. absorb shared conclusions into stable `g_` rule
6. leave target unit candidate process state reusable after changing the target unit candidate main file

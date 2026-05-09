# Rule Extract Flow

## 1. Purpose

`rule_extract` is the internal flow for extracting already-existing unit truth into one independent `rule`.

It answers four questions:

1. whether multiple units really depend on the same formal truth now
2. which part of current unit-local truth should move into one rule object
3. how unit-side truth must be rewritten so duplicate formal truth no longer remains
4. how the repository must be reconciled after the shared extraction lands

This is not a user-facing command entry.
The user reaches it through natural-language routing when that routing enters the rule-governance branch.

---

## 2. Scope

By default it handles requests where rule truth already exists inside one or more units and now needs to be extracted.

It may:

1. create or update a candidate-layer `rule`
2. rewrite unit candidate-side references and boundary explanation
3. remove duplicate formal truth from the source unit candidate side
4. update the target rule file's declarative `bound_objects` metadata so it matches the real command-target binding set after extraction writeback
5. trigger `rule_sync` after any rule-truth or binding writeback
6. stop at a rule-governance checkpoint when any source or consumer unit is currently at `stable`

It does not:

1. design new rule truth from scratch when no unit-local source truth exists
2. bind a unit to an already-stable rule truth as the only task
3. replace unit lifecycle commands
4. promote rule truth into stable `g_` rule

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/spec_policy.md`
2. read `specflow/framework/command_policy.md`
3. read `docs/specs/_status.md` and use it as the repository-wide formal unit index for this extraction
4. resolve each named unit's current layer from `_status.md` before reading its main Spec
5. read the source unit current-layer main files and any explicitly referenced appendix truth involved in the extraction
6. build the repository-wide involved-unit set needed for this extraction from current repository truth before writeback starts:
   - start from the current formal unit set recorded in `_status.md`
   - start from the named source units and any named consumer units
   - read every additional current-layer unit main file needed to judge whether that unit still carries, duplicates, or already consumes the target truth
   - do not treat the source unit list alone as sufficient when the extraction target may already be reused elsewhere
7. read any relevant existing `rule` files that may overlap the target truth
8. read `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may cross into project-wide default-rule promotion
9. if any involved unit is currently at `stable`, also read `specflow/framework/commands/unit_fork.md`
10. if the round may create, update, or delete any unit `rule_refs` value or any file under `docs/specs/rules/**`, read `specflow/framework/rule_sync.md` first
11. if the round may create or update any file under `docs/specs/rules/**`, apply the Rule version rules from `specflow/framework/spec_policy.md` Section 6.3
12. if the round may create the first file for a brand-new `rule_id` or otherwise change the current rule object map, read `docs/specs/repository_mapping.md` before rule truth writeback
13. if the round may update `bound_objects`, read every current-layer unit or scenario main file needed to derive the real repository-wide binding set of each touched Rule from `rule_refs`

---

## 4. Procedure

1. confirm the request is really about extracting already-existing unit-local formal truth
2. identify the smallest rule object that multiple units truly depend on
3. resolve the complete involved-unit set from current repository truth before writeback:
   - identify which units currently carry duplicate unit-local truth for that object
   - identify which command-target objects already consume that object through `rule_refs`
   - reject closure if consumer coverage is still uncertain
4. derive the writeback-required involved-unit subset for this round from the already-resolved involved-unit set:
   - include each source unit whose current-layer formal truth must be rewritten so the extracted object no longer remains duplicated as unit-local truth
   - include each consumer unit whose current-layer `rule_refs` or body-level consumption explanation must change because of the extraction result
   - do not require writeback for an involved unit that is read only to confirm consumer coverage and whose current-layer truth already aligns with the extraction result
5. if any writeback-required involved unit current layer is `stable`, do not modify that unit `stable` directly:
   - raise a blocking rule-governance checkpoint with `type=prerequisite_action`
   - require `unit_fork:{unit}` for each such unit before extraction continues
   - set `required_writeback_target` to the corresponding unit candidate main file set because chat-only agreement does not create legal extraction targets
5.5. before any rule file writeback, capture the recovery baseline required by `specflow/framework/recovery_policy.md` Section 6.5.1:
     - the target candidate-layer Rule file
     - any stable-layer sibling that may be created or updated
     - any downstream unit candidate file that may be rewritten (rule_refs, body text)
     - `docs/specs/repository_mapping.md` when the round may change the rule object map
     - every other file under `docs/specs/rules/**` that may be touched by this round
6. create or update the target candidate-layer `rule`
7. if Step 6 created the first file for a brand-new rule object, initialize `rule_version=0.1.0`
8. if Step 6 reopened an already-stable rule object at the candidate layer, set the candidate `rule_version` to the intended next stable version according to Rule semantic version rules
9. if Step 6 reopened an already-stable rule object at the candidate layer, also write exactly one `promotion_owner_unit` into that candidate-layer rule file:
   - the owner must be chosen from the writeback-required involved-unit subset for this round
   - that owner is the unit round that must later land this candidate-layer rule file as the next stable-layer Rule file
   - the owner unit may still remain formally bound to the current stable-layer shared sibling until a later legal unit candidate round rewrites its `rule_refs`
   - if current repository truth is insufficient to name one stable owner without guessing, stop this flow and return control to `rule_escape` through rule-governance routing
10. if the target candidate-layer rule file has a stable-layer sibling after Steps 6 to 9, validate that the resulting candidate-layer file still carries exactly one valid `promotion_owner_unit`:
   - if Step 9 already wrote the owner, confirm that the resulting file still keeps that owner
   - if Step 6 updated an already-existing candidate-layer file with a stable-layer sibling, preserve or rewrite `promotion_owner_unit` so the resulting file still names one formal unit from the writeback-required involved-unit subset for this round
   - if current repository truth is insufficient to keep one stable owner from that subset without guessing, stop this flow and return control to `rule_escape` through rule-governance routing
11. if this round created the first file for a brand-new `rule_id` or otherwise changed the current rule object map, update `docs/specs/repository_mapping.md` in the same round before executing `rule_sync`:
   - record the new or changed `rule` ID and one-line responsibility in the Governed Object Map
   - keep rule truth-path rules consistent with the resulting rule file location
   - if current repository truth is insufficient to write the exact mapping update without guessing, stop this flow and return control to `rule_escape` through rule-governance routing
12. rewrite every source unit candidate side so the extracted truth is no longer duplicated as unit-local formal truth
13. rewrite every additional writeback-required involved consumer unit candidate-side reference and behavior explanation required by the extraction result
   - any written `rule_refs` must use the Rule binding contract from `specflow/framework/spec_policy.md` Section 6.1
14. update the target rule file's `bound_objects` only as declarative metadata so it matches the real binding set implied by repository-wide unit and scenario `rule_refs` plus this round's prepared unit writebacks
   - the deterministic metadata writeback may be executed with `specflow/tooling/bin/specflowctl-<os>-<arch> rule reconcile-bound-objects --rule-refs c_b_rule_x@0.1.0` and additional `--rule-ids` filters when the active flow has already identified them
15. if the target rule file now has one or more formal bound units after this round, remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from that resulting bound file state in the same round
16. if duplicate formal truth still remains after extraction, stop and report boundary closure failure
17. if any involved unit that should now consume the extracted truth was not fully reviewed and rewritten where required, stop and report consumer-coverage failure
18. after any write to `docs/specs/rules/**` or any unit `rule_refs`, execute `rule_sync` before claiming closure
   - if any touched rule file changed only in `bound_objects` during this round, pass execution-local `bound_objects_only_rule_file_refs` with the exact file refs for those files

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the shared extraction is complete, duplicate formal truth is removed, and `rule_sync` has finished reconciliation
   - the target rule file `bound_objects` metadata must already match the real command-target binding set
   - involved consumer coverage must already be complete for the current repository truth
   - any required `repository_mapping.md` object-map writeback must already be complete
2. the request is not really extraction and must be re-routed to another rule flow
3. one or more writeback-required involved units are currently at `stable` and the flow has raised a rule-governance checkpoint for `unit_fork` first
4. unit-local truth versus rule truth is still not stably separable
5. involved consumer coverage is still incomplete or uncertain, so the flow cannot claim extraction closure yet
7. a resulting candidate-layer rule file for an already-stable rule object would exist after this round, but no stable `promotion_owner_unit` can be named from the writeback-required involved-unit subset

---

## 6. Output Contract

The output must include at least:

1. the extracted rule object and why it belongs to `rule_extract`
2. the complete involved-unit set used for the extraction decision
3. which involved units were source units, which command-target objects were already consumers, which units required writeback in this round, and which had to stop for `unit_fork`
4. the source unit files that originally carried the truth
5. the target rule file written or updated, or the checkpoint result when extraction could not legally start yet
6. the written `rule_version` and why it is correct for the current round
7. the unit candidate-side rewrite result and whether duplicate formal truth was fully removed
8. whether involved unit and scenario consumer coverage is complete for the current repository truth
9. the target rule file `bound_objects` reconciliation result
10. when the resulting candidate-layer rule file has a stable-layer sibling, the written or validated `promotion_owner_unit`
11. when the round changed the current rule object map, the `docs/specs/repository_mapping.md` writeback result
12. the `rule_sync` result, including affected downstream objects and fallback if any

---

## 7. Non-Goals

`rule_extract` does not:

1. preserve two formal truths for the same object
2. skip unit-side boundary rewrite after creating a rule file
3. leave reconciliation for later after changing rule truth or bindings
4. modify unit `stable` truth directly
5. absorb shared conclusions into stable `g_` rule

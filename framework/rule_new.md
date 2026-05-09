# Rule New Flow

## 1. Purpose

`rule_new` is the internal flow for creating rule truth from the start, or opening the next candidate-layer round for an already-independent rule object.

It answers four questions:

1. whether the target truth should exist independently as `rule`
2. whether that truth is really cross-unit rule truth rather than one unit's private appendix
3. which candidate-layer `rule` file should carry that truth now, including when a stable-layer sibling file already exists
4. how the repository must be reconciled after that rule truth is created or updated, including who owns the later stable landing when a next-round shared candidate is opened for an already-stable rule object

This is not a user-facing command entry.
The user reaches it through natural-language routing when that routing enters the rule-governance branch.

---

## 2. Scope

By default it handles requests where rule truth should be authored as an independent rule object, whether that object is being created for the first time or reopened at the candidate layer for a new rule round.

It may:

1. create a new candidate-layer `rule`
2. create or update a candidate-layer `rule` for a `rule_id` that already has a stable-layer file
3. update an existing candidate-layer `rule` that is still the same rule object
4. record expected future landing points in planning text when consumer units do not yet have current-layer candidates
5. trigger `rule_sync` after rule truth writeback
6. declare the later `unit_promote` owner when the round opens the next candidate-layer file for a rule object that already has a stable-layer sibling

It does not:

1. extract already-written unit-local truth out of an existing unit body
2. bind one unit to an already-existing `rule` as the main task
3. replace unit command chains
4. promote rule truth into stable `g_` rule

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/spec_policy.md`
2. read `specflow/framework/command_policy.md`
3. read `docs/specs/_status.md` and use it as the repository-wide formal unit index for duplicate-truth review
4. resolve every named existing unit's current layer from `_status.md` before reading its main Spec
5. read any current-layer unit main files already involved in the request
6. read every additional current-layer unit main file needed to judge whether the target truth already exists as unit-local formal truth, is already duplicated across units, or is already formalized as rule truth elsewhere
7. read any relevant existing `rule` files if the request names or overlaps them
8. read `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may cross into project-wide default-rule promotion
9. if the round may create, update, or delete any file under `docs/specs/rules/**`, read `specflow/framework/rule_sync.md` first
10. if the round may create or update any file under `docs/specs/rules/**`, apply the Rule version rules from `specflow/framework/spec_policy.md` Section 6.3
11. if the round may create the first file for a brand-new `rule_id` or otherwise change the current rule object map, read `docs/specs/repository_mapping.md` before rule truth writeback
12. if the request may create or update a candidate-layer file for a `rule_id` that already has a stable-layer sibling, build the repository-wide binding review set and the affected-unit owner set for that already-stable rule object from current repository truth before owner selection:
   - start from the formal unit and scenario rows recorded in `_status.md`
   - read every additional current-layer unit or scenario main file needed to judge which command-target objects currently bind that stable-layer sibling or its current candidate-layer sibling through `rule_refs`
   - derive the affected-unit owner set from the unit subset of that repository-wide binding review set
   - do not treat only the user-named units, user-named scenarios, or currently obvious consumers as sufficient when other command-target objects may still bind that already-stable rule object

If the request names units that do not yet have current-layer Spec files and the user intent is explicitly "design rule truth first", do not block on that absence.

---

## 4. Procedure

1. confirm the request is really about independent shared authoring, including creating rule truth from the start or opening the next candidate-layer round for an already-independent rule object, rather than `rule_extract` or `rule_bind`
2. resolve the repository-wide duplicate-truth review set from current repository truth before writeback:
   - start from the formal unit set recorded in `_status.md`
   - include any named existing units and any units already shown by current repository truth to overlap the target topic
   - read every additional current-layer unit main file needed to judge whether the target truth already exists as unit-local formal truth, is already duplicated across units, or is already formalized as a different rule object
   - if current repository truth is insufficient to rule those cases out safely, stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing
3. inspect existing unit truth and existing rule truth across that repository-wide review set to ensure the target truth is not already formalized elsewhere as duplicate formal truth
4. decide the target rule object boundary:
   - one rule object per rule file
   - do not merge unrelated shared topics into one file
5. if the round may create or update a candidate-layer file for a `rule_id` that already has a stable-layer sibling, resolve the repository-wide binding set and the affected-unit owner set for that already-stable rule object from current repository truth before owner selection:
   - derive the binding set from unit and scenario `rule_refs` rather than from `bound_objects`
   - include command-target objects that currently bind the stable-layer sibling and command-target objects that already bind its current candidate-layer sibling when that sibling exists
   - derive the affected-unit owner set from the unit subset of that binding set
   - if current repository truth is insufficient to derive that binding set or owner set safely, stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing
   - if the affected-unit owner set is empty but the binding set is not empty, stop this flow and return control to `rule_escape`; scenario-only current bindings cannot supply the required `promotion_owner_unit`
   - if both sets are empty, continue only when current repository truth explicitly shows that the already-stable rule object is intentionally kept as independently authored rule truth with no current formal bindings; otherwise stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing a lifecycle owner with no current formal consumer set
5.5. before any rule file writeback, capture the recovery baseline required by `specflow/framework/recovery_policy.md` Section 6.5.1:
     - the target candidate-layer Rule file
     - any stable-layer sibling that may be created or updated
     - `docs/specs/repository_mapping.md` when the round may change the rule object map
     - every other file under `docs/specs/rules/**` that may be touched by this round
6. if the request is to continue evolving an already-independent rule object that currently has only a stable-layer file, create or update the sibling candidate-layer `rule` for the same `rule_id`, set its `rule_version` to the intended next stable version according to Rule semantic version rules, and write exactly one `promotion_owner_unit` into that candidate-layer rule file:
   - when the repository-wide affected-unit owner set from Step 5 is not empty, the owner must be one formal unit from that set
   - when Step 5 confirmed that the already-stable rule object is intentionally kept with no current formal bindings, the owner must be one formal unit explicitly required by the current round as the future adopter of that next-round draft
   - that owner is the unit round that must later bind or retarget legally to this candidate-layer rule file before it may land as the next stable-layer Rule file
   - the owner unit may still remain formally bound to the current stable-layer shared sibling until a later legal unit candidate round rewrites its `rule_refs`
   - if current repository truth is insufficient to justify the no-current-binding continuation or to name one stable promotion owner unit, stop this flow and return control to `rule_escape` through rule-governance routing instead of guessing
7. otherwise create or update the target candidate-layer `rule`
7.5. ensure the candidate-layer rule file body satisfies `spec_writing_guide.md` Section 6: explanatory and normative content must be separated at the subsection level
8. if Step 7 created the first file for a brand-new rule object, initialize `rule_version=0.1.0`
9. if the target candidate-layer rule file has a stable-layer sibling after Steps 6 to 8, validate that the resulting candidate-layer file still carries exactly one valid `promotion_owner_unit`:
   - if Step 6 already wrote the owner, confirm that the resulting file still keeps that owner
   - if Step 7 updated an already-existing candidate-layer file with a stable-layer sibling, preserve or rewrite `promotion_owner_unit` so the resulting file still names one formal unit from the repository-wide affected-unit owner set resolved in Step 5
   - if current repository truth is insufficient to keep one stable promotion owner without guessing, stop this flow and return control to `rule_escape` through rule-governance routing
10. if this round created the first file for a brand-new `rule_id` or otherwise changed the current rule object map, update `docs/specs/repository_mapping.md` in the same round before executing `rule_sync`:
   - record the new or changed `rule` ID and one-line responsibility in the Governed Object Map
   - keep rule truth-path rules consistent with the resulting rule file location
   - if current repository truth is insufficient to write the exact mapping update without guessing, stop this flow and return control to `rule_escape` through rule-governance routing
11. if no command-target object formally binds the rule truth yet:
   - keep `bound_objects=none`
   - record expected future consumers only as planning text in the rule file body
12. if the same truth still remains duplicated as formal unit truth elsewhere, stop and report that boundary closure is incomplete
13. after any write to `docs/specs/rules/**`, execute `rule_sync` before claiming closure, even when the binding set or affected-unit owner set is currently empty

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the target candidate-layer `rule` has been written, any required `repository_mapping.md` object-map writeback is complete, and required reconciliation through `rule_sync` is complete
2. the request is not really independent shared authoring or next-round opening and must be re-routed to another rule flow
3. duplicate formal truth remains in unit-local files and boundary closure has not been completed
4. current repository truth is insufficient to rule out duplicate formal truth or alternate formal landing points, so control has returned to `rule_escape` through rule-governance routing
5. the request is really a pure unit retarget or rule impact-check request and must be re-routed to another rule flow
6. a next-round candidate-layer rule file for an already-stable rule object would exist after this round, but no stable `promotion_owner_unit` can be named from current repository truth

---

## 6. Output Contract

The output must include at least:

1. the recognized rule object and why it belongs to `rule_new`
2. the target rule file written or updated
3. the written `rule_version` and why it is correct for the current round
4. whether the round created the first candidate-layer file for an already-existing stable-layer rule object
5. the repository-wide duplicate-truth review set used for the decision
6. whether any named units already bind that truth formally
7. whether duplicate unit-local formal truth was found, or whether the flow had to return to `rule_escape` because that judgment could not be stabilized safely
8. the `rule_sync` result, including whether any units were affected
9. when the resulting candidate-layer rule file has a stable-layer sibling, the repository-wide binding set and affected-unit owner set used for owner selection
10. when the resulting candidate-layer rule file has a stable-layer sibling, the written or validated `promotion_owner_unit`, whether it came from the current affected-unit owner set or an explicit intentionally-unbound continuation owner, and whether that owner still needs a later unit-side binding retarget before promotion
11. when the round changed the current rule object map, the `docs/specs/repository_mapping.md` writeback result

---

## 7. Non-Goals

`rule_new` does not:

1. guess through unstable boundaries
2. invent formal unit bindings before `rule_refs` exists
3. leave reconciliation for later after changing rule truth
4. absorb shared conclusions into stable `g_` rule

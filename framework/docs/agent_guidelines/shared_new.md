# Shared New Flow

## 1. Purpose

`shared_new` is the internal flow for creating shared truth from the start, or opening the next candidate-layer round for an already-independent shared object.

It answers four questions:

1. whether the target truth should exist independently as `shared_contract`
2. whether that truth is really cross-module shared truth rather than one module's private appendix
3. which candidate-layer `shared_contract` file should carry that truth now, including when a stable-layer sibling file already exists
4. how the repository must be reconciled after that shared truth is created or updated, including who owns the later stable landing when a next-round shared candidate is opened for an already-stable shared object

This is not a user-facing command entry.
The user reaches it through `shared_ops:{natural-language request}`.

---

## 2. Scope

By default it handles requests where shared truth should be authored as an independent shared object, whether that object is being created for the first time or reopened at the candidate layer for a new shared round.

It may:

1. create a new candidate-layer `shared_contract`
2. create or update a candidate-layer `shared_contract` for a `shared_contract_id` that already has a stable-layer file
3. update an existing candidate-layer `shared_contract` that is still the same shared object
4. record expected future landing points in planning text when consumer modules do not yet have current-layer candidates
5. trigger `shared_sync` after shared truth writeback
6. declare the later `module_promote` owner when the round opens the next candidate-layer file for a shared object that already has a stable-layer sibling

It does not:

1. extract already-written module-local truth out of an existing module body
2. bind one module to an already-existing `shared_contract` as the main task
3. replace module command chains
4. promote shared truth into `system_constraints`

---

## 3. Preconditions

Before execution:

1. read `specflow/framework/docs/agent_guidelines/spec_policy.md`
2. read `specflow/framework/docs/agent_guidelines/command_policy.md`
3. read `docs/specs/_status.md` and use it as the repository-wide formal module index for duplicate-truth review
4. resolve every named existing module's current layer from `_status.md` before reading its main Spec
5. read any current-layer module main files already involved in the request
6. read every additional current-layer module main file needed to judge whether the target truth already exists as module-local formal truth, is already duplicated across modules, or is already formalized as shared truth elsewhere
7. read any relevant existing `shared_contract` files if the request names or overlaps them
8. read `docs/specs/system/stable/s_system_constraints.md` when the request may cross into project-wide default-rule promotion
9. if the round may create, update, or delete any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/shared_sync.md` first
10. if the round may create or update any file under `docs/specs/shared_contracts/**`, read `specflow/framework/docs/agent_guidelines/git_policy.md` because Shared Contract semantic version rules apply
11. if the request may create or update a candidate-layer file for a `shared_contract_id` that already has a stable-layer sibling, build the repository-wide affected-module review set for that already-stable shared object from current repository truth before owner selection:
   - start from the formal module set recorded in `_status.md`
   - read every additional current-layer module main file needed to judge which modules currently bind that stable-layer sibling or its current candidate-layer sibling through `shared_contract_refs`
   - do not treat only the user-named modules or currently obvious consumers as sufficient when other modules may still bind that already-stable shared object

If the request names modules that do not yet have current-layer Spec files and the user intent is explicitly "design shared truth first", do not block on that absence.

---

## 4. Procedure

1. confirm the request is really about independent shared authoring, including creating shared truth from the start or opening the next candidate-layer round for an already-independent shared object, rather than `shared_extract` or `shared_bind`
2. resolve the repository-wide duplicate-truth review set from current repository truth before writeback:
   - start from the formal module set recorded in `_status.md`
   - include any named existing modules and any modules already shown by current repository truth to overlap the target topic
   - read every additional current-layer module main file needed to judge whether the target truth already exists as module-local formal truth, is already duplicated across modules, or is already formalized as a different shared object
   - if current repository truth is insufficient to rule those cases out safely, stop this flow and return control to `shared_escape` through `shared_ops` instead of guessing
3. inspect existing module truth and existing shared truth across that repository-wide review set to ensure the target truth is not already formalized elsewhere as duplicate formal truth
4. decide the target shared object boundary:
   - one shared object per shared file
   - do not merge unrelated shared topics into one file
5. if the round may create or update a candidate-layer file for a `shared_contract_id` that already has a stable-layer sibling, resolve the repository-wide affected-module set for that already-stable shared object from current repository truth before owner selection:
   - derive that set from module `shared_contract_refs` rather than from `bound_modules`
   - include modules that currently bind the stable-layer sibling and modules that already bind its current candidate-layer sibling when that sibling exists
   - if current repository truth is insufficient to derive that affected-module set safely, stop this flow and return control to `shared_escape` through `shared_ops` instead of guessing
   - if that affected-module set is empty, continue only when current repository truth explicitly shows that the already-stable shared object is intentionally kept as independently authored shared truth with no current formal bindings; otherwise stop this flow and return control to `shared_escape` through `shared_ops` instead of guessing a lifecycle owner with no current formal consumer set
6. if the request is to continue evolving an already-independent shared object that currently has only a stable-layer file, create or update the sibling candidate-layer `shared_contract` for the same `shared_contract_id`, set its `shared_version` to the intended next stable version according to Shared Contract semantic version rules, and write exactly one `promotion_owner_module` into that candidate-layer shared file:
   - when the repository-wide affected-module set from Step 5 is not empty, the owner must be one formal module from that set
   - when Step 5 confirmed that the already-stable shared object is intentionally kept with no current formal bindings, the owner must be one formal module explicitly required by the current round as the future adopter of that next-round draft
   - that owner is the module round that must later bind or retarget legally to this candidate-layer shared file before it may land as the next stable-layer Shared Contract file
   - the owner module may still remain formally bound to the current stable-layer shared sibling until a later legal module candidate round rewrites its `shared_contract_refs`
   - if current repository truth is insufficient to justify the no-current-binding continuation or to name one stable promotion owner module, stop this flow and return control to `shared_escape` through `shared_ops` instead of guessing
7. otherwise create or update the target candidate-layer `shared_contract`
8. if Step 7 created the first file for a brand-new shared object, initialize `shared_version=0.1.0`
9. if the target candidate-layer shared file has a stable-layer sibling after Steps 6 to 8, validate that the resulting candidate-layer file still carries exactly one valid `promotion_owner_module`:
   - if Step 6 already wrote the owner, confirm that the resulting file still keeps that owner
   - if Step 7 updated an already-existing candidate-layer file with a stable-layer sibling, preserve or rewrite `promotion_owner_module` so the resulting file still names one formal module from the repository-wide affected-module set resolved in Step 5
   - if current repository truth is insufficient to keep one stable promotion owner without guessing, stop this flow and return control to `shared_escape` through `shared_ops`
10. if no consumer module formally binds the shared truth yet:
   - keep `bound_modules=none`
   - record expected future consumers only as planning text in the shared file body
11. if the same truth still remains duplicated as formal module truth elsewhere, stop and report that boundary closure is incomplete
12. after any write to `docs/specs/shared_contracts/**`, execute `shared_sync` before claiming closure, even when the affected-module set is currently empty

---

## 5. Stop Conditions

Stop when one of the following is true:

1. the target candidate-layer `shared_contract` has been written and required reconciliation through `shared_sync` is complete
2. the request is not really independent shared authoring or next-round opening and must be re-routed to another shared flow
3. duplicate formal truth remains in module-local files and boundary closure has not been completed
4. current repository truth is insufficient to rule out duplicate formal truth or alternate formal landing points, so control has returned to `shared_escape` through `shared_ops`
5. the request is really a pure module retarget or shared impact-check request and must be re-routed to another shared flow
6. the request has crossed into `system_constraints_change_proposal` and must stop at a `shared_ops` checkpoint instead of continuing here
7. a next-round candidate-layer shared file for an already-stable shared object would exist after this round, but no stable `promotion_owner_module` can be named from current repository truth

---

## 6. Output Contract

The output must include at least:

1. the recognized shared object and why it belongs to `shared_new`
2. the target shared-contract file written or updated
3. the written `shared_version` and why it is correct for the current round
4. whether the round created the first candidate-layer file for an already-existing stable-layer shared object
5. the repository-wide duplicate-truth review set used for the decision
6. whether any named modules already bind that truth formally
7. whether duplicate module-local formal truth was found, or whether the flow had to return to `shared_escape` because that judgment could not be stabilized safely
8. the `shared_sync` result, including whether any modules were affected
9. when the resulting candidate-layer shared file has a stable-layer sibling, the repository-wide affected-module set used for owner selection
10. when the resulting candidate-layer shared file has a stable-layer sibling, the written or validated `promotion_owner_module`, whether it came from the current affected-module set or an explicit intentionally-unbound continuation owner, and whether that owner still needs a later module-side binding retarget before promotion
11. the git close-out result when governance files or commit-triggering files were changed

---

## 7. Non-Goals

`shared_new` does not:

1. guess through unstable boundaries
2. invent formal module bindings before `shared_contract_refs` exists
3. leave reconciliation for later after changing shared truth
4. absorb shared conclusions into `system_constraints`

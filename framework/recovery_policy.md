# Recovery Policy

## 1. Purpose

This file defines the centralized recovery rules used when a Spec Flow round cannot safely continue after repository mutation or after downstream invalidation has already started.

It answers five questions:

1. what recovery means in this repository
2. which object families use centralized recovery
3. how candidate-side fallback cleanup works
4. how incomplete promotion recovery works
5. which repository state may be claimed after recovery

This file does not replace command-local stop conditions.
It defines the shared repository-restoration baseline.

## 2. Scope

This policy covers:

1. candidate-side recovery for:
   - `unit`
   - `scenario`
2. incomplete promotion recovery for:
   - `unit_promote`
   - `scenario_promote`

Boundary:

1. the active command still decides when recovery is entered
2. `impact_sync` may execute deterministic fallback cleanup
3. this file defines the repository state that recovery must restore

## 3. Core Terms

### 3.1 Recovery

`recovery` means:

1. stop claiming the current round succeeded
2. restore the repository to a state that the next legal command can safely consume
3. update `_status.md` so the next action is explicit

### 3.2 Recovery Baseline

`recovery baseline` means the exact pre-mutation snapshot a command captures before it starts overwriting or deleting governance files.

It is not:

1. a truth file
2. a process file
3. a new lifecycle stage

### 3.3 Incomplete Promotion Recovery

`incomplete promotion recovery` means the special recovery path used when a promote command already mutated repository truth, but the round cannot be safely closed as a completed promotion.

## 4. Candidate-Side Recovery Baseline

For candidate-side invalidation:

1. classify the failed surface before choosing a fallback target
2. use the nearest command that owns rebuilding or reproving that failed surface
3. update `_status.md` to the smallest legal next step
4. delete only the failed process layer and process files downstream of that layer
5. do not invent extra temporary states

### 4.1 Failure Layers

Candidate-side recovery uses these fixed layers:

1. `truth_layer`
   - candidate truth, acceptance item set, Rule snapshot, global baseline, repository mapping, or formal binding meaning changed
   - fallback target:
     - `unit -> unit_check`
     - `scenario -> scenario_check`
2. `gate_layer`
   - the check gate is missing, malformed, or not tool-valid, while current truth and current bindings still match
   - fallback target:
     - `unit -> unit_check`
     - `scenario -> scenario_check`
3. `plan_layer`
   - the unit active plan is missing, malformed, not tool-valid, or no longer covers the current acceptance item ids, while the unit check gate still covers current truth
   - fallback target:
     - `unit -> unit_plan`
   - this layer does not apply to `scenario`
4. `implementation_layer`
   - implementation no longer satisfies the current unit truth and current active plan, while both upstream process layers remain valid
   - fallback target:
     - `unit -> unit_impl`
   - this layer does not allow `scenario` to repair units; scenario verification must report affected units instead
5. `evidence_layer`
   - verification evidence is missing, stale, incomplete, or malformed, while truth, check gate, and any required plan still stand
   - fallback target:
     - `unit -> unit_verify`
     - `scenario -> scenario_verify`
6. `dependency_readiness_layer`
   - `scenario_promote` found a required unit or Rule dependency that is candidate-layer, missing, or not safely resolvable as stable, while scenario truth and verification evidence still stand
   - fallback target:
     - `scenario -> scenario_promote`
   - this layer waits for dependency landing or scenario binding writeback; it does not delete scenario check or verify process files by itself

Layer rules:

1. only `truth_layer` permits deleting every current candidate-side process artifact for the object
2. `gate_layer` deletes the check gate only
3. `plan_layer` deletes unit draft plan, unit active plan, and unit verify result
4. `implementation_layer` deletes only verification results that can no longer remain current
5. `evidence_layer` deletes only verification results
6. `dependency_readiness_layer` deletes no scenario process files unless a separate truth or evidence layer is also proven

### 4.2 Default Candidate Cleanup Map

1. `unit truth_layer -> unit_check`
   - delete `_check_result/unit/{unit}.md`
   - delete `_plans/draft/{unit}.md`
   - delete `_plans/active/{unit}.md`
   - delete `_verify_result/unit/{unit}.md`
2. `unit gate_layer -> unit_check`
   - delete `_check_result/unit/{unit}.md`
3. `unit plan_layer -> unit_plan`
   - delete `_plans/draft/{unit}.md`
   - delete `_plans/active/{unit}.md`
   - delete `_verify_result/unit/{unit}.md`
4. `unit implementation_layer -> unit_impl`
   - delete `_verify_result/unit/{unit}.md`
5. `unit evidence_layer -> unit_verify`
   - delete `_verify_result/unit/{unit}.md`
6. `scenario truth_layer -> scenario_check`
   - delete `_check_result/scenario/{scenario}.md`
   - delete `_verify_result/scenario/{scenario}.md`
7. `scenario gate_layer -> scenario_check`
   - delete `_check_result/scenario/{scenario}.md`
8. `scenario evidence_layer -> scenario_verify`
   - delete `_verify_result/scenario/{scenario}.md`
9. `scenario dependency_readiness_layer -> scenario_promote`
   - delete no process files

Plain meaning:

1. candidate-side drift does not create a second state machine
2. recovery rewinds only to the nearest step that can rebuild or reprove the failed layer
3. a process artifact format error is not automatically a truth error
4. a command must not keep using a process file that failed its required tool validation

## 5. Stable-Side Recovery Baseline

For stable-side invalidation:

1. do not generate candidate-side cleanup solely because stable alignment became stale
2. update `_status.md` so the stable verification command becomes the next legal step

Default stable fallback targets:

1. invalid `unit` stable -> `unit_stable_verify`
2. invalid `scenario` stable -> `scenario_stable_verify`

## 6. Incomplete Promotion Recovery

### 6.1 Required Recovery Baseline Before Mutation

Before the first truth-file mutation, every promote command must capture a recovery baseline covering every file the round may overwrite or delete.

At minimum:

1. the target object row in `docs/specs/_status.md`
2. the target object's candidate truth file
3. the target object's stable truth file if it already existed
4. the target object's current-round process files
5. any appendix, shared, or global-rule file that the round may mutate, promote, absorb, or delete

Object-specific minimums:

1. `unit_promote`
   - `docs/specs/units/candidate/c_unit_{unit}.md`
   - `docs/specs/units/stable/s_unit_{unit}.md` when present
   - `_check_result/unit/{unit}.md`
   - `_plans/draft/{unit}.md`
   - `_plans/active/{unit}.md`
   - `_verify_result/unit/{unit}.md`
   - every same-round stable landing retargeted unit candidate main file when `unit_promote` changes that unit's `rule_refs`
   - every same-round stable landing retargeted unit candidate process file that may be deleted by post-promotion rule impact reconciliation
   - every surviving candidate file that references `c_unit_{unit}.md` through a known candidate-to-candidate file reference, because unit_promote deletes that target file and must be able to restore the cross-referencing file's original reference if incomplete-promotion recovery is required
   - every unit or scenario Spec file under `docs/specs/units/**` or `docs/specs/scenarios/**` that may be mechanically retargeted from `c_unit_{unit}` to `s_unit_{unit}` during promotion dependency reference retargeting
   - every `_status.md` row that may be changed because a retargeted current-layer stable object must run stable verification or a retargeted current-layer candidate object must fall back to check
   - every current-round process file that may be deleted because a retargeted candidate unit or scenario can no longer reuse process state written against the old candidate dependency reference
2. `scenario_promote`
   - `docs/specs/scenarios/candidate/c_scenario_{scenario}.md`
   - `docs/specs/scenarios/stable/s_scenario_{scenario}.md` when present
   - `_check_result/scenario/{scenario}.md`
   - `_verify_result/scenario/{scenario}.md`

Rules:

1. the baseline must preserve exact file bytes
2. the baseline may live in memory or an executor-owned temporary artifact
3. it is not repository truth and must not be committed

### 6.2 When Recovery Is Required

Incomplete promotion recovery is required when both are true:

1. the promote command already mutated at least one promotion target or deletion target
2. the round can no longer safely complete

Examples:

1. the command was interrupted after writing a new stable file
2. downstream reconciliation became blocked after partial promotion writeback
3. cleanup started but did not finish
4. post-promotion `rule_sync` or `impact_sync` showed that the repository cannot yet claim a stable closed state

### 6.3 Recovery Procedure

When incomplete promotion recovery is triggered:

1. stop claiming promotion success
2. restore every mutated or deleted file covered by the recovery baseline to its exact pre-mutation state
3. delete any new file created only by the interrupted round that did not exist in the recovery baseline
4. restore `_status.md` for the target object to candidate semantics:
   - keep `Candidate=yes`
   - keep `Active Layer=candidate`
   - set the smallest restart step to:
     - `unit -> unit_check`
     - `scenario -> scenario_check`
5. keep `Stable=yes|no` consistent with the pre-round state from the recovery baseline
6. after repository restoration, delete candidate-side process files for that target object because they are no longer safe for reuse

### 6.4 Recovery Result

After incomplete promotion recovery completes, the only safe claim is:

1. promotion did not complete
2. repository truth was restored to the pre-promotion candidate round
3. the object must restart from its candidate closure step

The command must not claim:

1. that the new stable truth is active
2. that old verify evidence is still reusable
3. that the round can resume from a later step than the family restart step

### 6.5 Rule-Governance Recovery

Required when a rule-governance flow (rule_new, rule_extract, rule_bind, rule_topology) has already mutated rule truth files, repository_mapping.md, or downstream unit/scenario candidate files, and rule_sync returns control to rule_escape before the flow can safely close.

#### 6.5.1 Required Recovery Baseline Before Mutation

Before the first file write in any rule-governance flow that may mutate:

1. the target candidate-layer Rule file(s)
2. any stable-layer sibling Rule file that may be created, updated, or deleted
3. any downstream unit or scenario candidate file that may be rewritten (rule_refs, body text)
4. `docs/specs/repository_mapping.md` when the round may change the rule object map
5. every other file under `docs/specs/rules/**` that may be touched by this round

#### 6.5.2 When Recovery Is Required

Rule-governance recovery is required when both are true:

1. the rule flow already mutated at least one rule-truth file, repository_mapping.md file, or downstream unit/scenario file
2. rule_sync returns control to rule_escape because repository truth is insufficient to continue safely

#### 6.5.3 Recovery Procedure

1. stop claiming the rule flow succeeded
2. restore every mutated file covered by the recovery baseline to its exact pre-mutation state
3. delete any new file created only by the interrupted round that did not exist in the recovery baseline
4. if `repository_mapping.md` was modified, restore it from the recovery baseline
5. if any downstream unit or scenario candidate file was mutated and later restored, those objects may need candidate-side process invalidation through the candidate fallback rules from Section 4

#### 6.5.4 Recovery Result

After rule-governance recovery completes:

1. all rule-truth files are restored to pre-mutation state
2. `repository_mapping.md` is restored to pre-mutation state when it was touched
3. any downstream unit/scenario candidate file modified by the interrupted round is restored
4. the next action is rerunning natural-language routing from current repository truth

## 7. Reason Codes

This policy adds one standardized recovery code:

1. `promotion_recovery`
   - use only when a promote command already mutated repository truth and the round had to be restored
2. `rule_governance_recovery`
   - use only when a rule-governance flow already mutated rule truth, repository_mapping, or downstream unit/scenario files and rule_sync returned control to rule_escape

Other fallback or drift cases keep using the existing standardized handoff codes.

## 8. Relationship To Other Files

This policy works together with:

1. `specflow/framework/command_policy.md`
2. `specflow/framework/impact_sync_policy.md`
3. `specflow/framework/checkpoint_protocol.md`
4. the active promote command file

Priority rules:

1. the active command decides whether recovery is entered
2. `impact_sync` may execute deterministic fallback cleanup
3. this file defines the repository-restoration baseline once recovery is required

## 9. Non-Goals

This file does not:

1. create new lifecycle stages
2. define a general rollback system for arbitrary code edits outside the active command scope
3. replace git-history policy

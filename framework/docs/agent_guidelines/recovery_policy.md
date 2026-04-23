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
   - `module`
   - `flow`
   - `project`
2. incomplete promotion recovery for:
   - `module_promote`
   - `flow_promote`
   - `project_promote`

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

1. use the smallest fallback step defined by the active command family
2. delete process files that are no longer safe for reuse
3. update `_status.md` to the smallest legal next step
4. do not invent extra temporary states

Default candidate fallback targets:

1. invalid `module` candidate -> `module_check`
2. invalid `flow` candidate -> `flow_check`
3. invalid `project` candidate -> `project_check`

Default candidate cleanup map:

1. `module -> module_check`
   - delete `_check_result/{module}.md`
   - delete `_plans/draft/{module}.md`
   - delete `_plans/active/{module}.md`
   - delete `_verify_result/{module}.md`
2. `flow -> flow_check`
   - delete `_check_result/{flow}.md`
   - delete `_verify_result/{flow}.md`
3. `project -> project_check`
   - delete `_check_result/project.md`
   - delete `_verify_result/project.md`

Plain meaning:

1. candidate-side drift does not create a second state machine
2. it only rewinds the object to the first step that must be rerun

## 5. Stable-Side Recovery Baseline

For stable-side invalidation:

1. do not generate candidate-side cleanup solely because stable alignment became stale
2. update `_status.md` so the stable verification command becomes the next legal step

Default stable fallback targets:

1. invalid `module` stable -> `module_stable_verify`
2. invalid `flow` stable -> `flow_stable_verify`
3. invalid `project` stable -> `project_stable_verify`

## 6. Incomplete Promotion Recovery

### 6.1 Required Recovery Baseline Before Mutation

Before the first truth-file mutation, every promote command must capture a recovery baseline covering every file the round may overwrite or delete.

At minimum:

1. the target object row in `docs/specs/_status.md`
2. the target object's candidate truth file
3. the target object's stable truth file if it already existed
4. the target object's current-round process files
5. any appendix, shared, or system-constraint file that the round may mutate, promote, absorb, or delete

Object-specific minimums:

1. `module_promote`
   - `docs/specs/modules/candidate/c_{module}.md`
   - `docs/specs/modules/stable/s_{module}.md` when present
   - `_check_result/{module}.md`
   - `_plans/draft/{module}.md`
   - `_plans/active/{module}.md`
   - `_verify_result/{module}.md`
2. `flow_promote`
   - `docs/specs/flows/candidate/c_{flow}.md`
   - `docs/specs/flows/stable/s_{flow}.md` when present
   - `_check_result/{flow}.md`
   - `_verify_result/{flow}.md`
3. `project_promote`
   - `docs/specs/project/candidate/c_project.md`
   - `docs/specs/project/stable/s_project.md` when present
   - `_check_result/project.md`
   - `_verify_result/project.md`

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
4. post-promotion `shared_sync` or `impact_sync` showed that the repository cannot yet claim a stable closed state

### 6.3 Recovery Procedure

When incomplete promotion recovery is triggered:

1. stop claiming promotion success
2. restore every mutated or deleted file covered by the recovery baseline to its exact pre-mutation state
3. delete any new file created only by the interrupted round that did not exist in the recovery baseline
4. restore `_status.md` for the target object to candidate semantics:
   - keep `Candidate=yes`
   - keep `Active Layer=candidate`
   - set the smallest restart step to:
     - `module -> module_check`
     - `flow -> flow_check`
     - `project -> project_check`
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

## 7. Reason Codes

This policy adds one standardized recovery code:

1. `promotion_recovery`
   - use only when a promote command already mutated repository truth and the round had to be restored

Other fallback or drift cases keep using the existing standardized handoff codes.

## 8. Relationship To Other Files

This policy works together with:

1. `specflow/framework/docs/agent_guidelines/command_policy.md`
2. `specflow/framework/docs/agent_guidelines/impact_sync_policy.md`
3. the active promote command file
4. `specflow/framework/docs/agent_guidelines/git_policy.md`

Priority rules:

1. the active command decides whether recovery is entered
2. `impact_sync` may execute deterministic fallback cleanup
3. this file defines the repository-restoration baseline once recovery is required

## 9. Non-Goals

This file does not:

1. create new lifecycle stages
2. define a general rollback system for arbitrary code edits outside the active command scope
3. replace git-history policy

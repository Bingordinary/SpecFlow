# Project Stable Verify Command

## 1. Purpose

`project_stable_verify` checks whether current repository truth still aligns with the stable `ProjectSpec`.

This includes not only current bindings, but also whether the current governed repository surface still resolves under the stable topology mapping.

## 2. Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `project_stable_verify` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. `_status.md` says `Object Type=project`, `Active Layer=stable`, `Next Command=project_stable_verify`
2. current stable `ProjectSpec` exists

## 4. Procedure

1. read stable `ProjectSpec`
2. revalidate current `scenario`, `unit`, `shared_contract`, and `system_constraints` bindings required by that project truth
3. revalidate the current repository surface under the stable project's declared governed roots, at minimum:
   - each current governed repository path still resolves under the stable topology mapping
   - no current governed repository path is now `unmapped`
   - no current repository path now resolves to more than one formal command-target object
   - current support-surface paths still resolve only as support surfaces rather than `unit`, `scenario`, `shared_contract`, or `ignore`
4. revalidate the current formal object graph against the actual current Spec files used by the stable project truth
5. if current bindings, current topology, and current formal object graph still align, keep or advance `Next Command=project_fork`
6. if drift exists, keep `Next Command=project_stable_verify`

## 5. Output Contract

The output must report:

1. stable alignment result
2. current topology-verification result
3. current path-ownership verification result
4. current formal object-graph verification result
5. whether any `_verify_result/project.md` write, delete, or keep action occurred
6. `_status.md` update result
7. `round conclusion`
8. `current state`
9. `next step`
10. `why this next step`
11. `next-stage entry gap`
12. the `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8.6
13. if a future extension introduces a checkpoint stop, the same close-out block must also report `resume signal`

## 6. Non-Goals

1. creating candidate project truth
2. mutating downstream object truth

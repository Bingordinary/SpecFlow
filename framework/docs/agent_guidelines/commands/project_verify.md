# Project Verify Command

## 1. Purpose

`project_verify` verifies whether the current candidate `ProjectSpec` still matches the current bound object set, the current governed repository surface, and whether promotion is allowed.

## 2. Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `project_verify` may produce that advancing result; later repair confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. `_status.md` says `Object Type=project`, `Active Layer=candidate`, `Next Command=project_verify`
2. current valid `_check_result/project.md` exists

## 4. Procedure

1. read current candidate `ProjectSpec`
2. revalidate current bound `scenario`, `unit`, `shared_contract`, and `system_constraints` snapshots
3. revalidate the current repository surface under the candidate's declared governed roots, at minimum:
   - each current governed repository path still resolves under the declared topology mapping
   - no current governed repository path is now `unmapped`
   - no current repository path now resolves to more than one formal command-target object
   - current support-surface paths still resolve only as support surfaces rather than `unit`, `scenario`, `shared_contract`, or `ignore`
   - current conflict-resolution order still produces one deterministic answer per governed path
4. revalidate the current formal object graph against the actual current Spec files, at minimum:
   - current `unit_refs`, `scenario_refs`, and `shared_contract_refs` still match the files declared by the candidate
   - the currently active relation graph still matches the current bound object set
   - the candidate's `system_constraints_stable_ref` still matches the current stable baseline used by the project
5. if current bindings, current topology, and current formal object graph still align, write `_verify_result/project.md`
6. if ready, advance `Next Command=project_promote`
7. if bindings, topology, or object-graph alignment drifted, fall back to `project_check`

## 5. Output Contract

The output must report:

1. verification gate result
2. current topology-verification result
3. current path-ownership verification result
4. current formal object-graph verification result
5. `_verify_result/project.md` write, delete, or keep result
6. `_status.md` update result
7. `round conclusion`
8. `current state`
9. `next step`
10. `why this next step`
11. `next-stage entry gap`
12. the `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8.6
13. if a future extension introduces a checkpoint stop, the same close-out block must also report `resume signal`

## 6. Non-Goals

1. repairing downstream objects
2. replacing `scenario_verify`

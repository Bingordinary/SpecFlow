# Project Fork Command

## 1. Purpose

`project_fork` opens a new candidate `ProjectSpec` from the current stable `ProjectSpec`.

The new candidate starts from the current stable repository governance coordinate system, not from bindings alone.

## 2. Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `project_fork` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. `_status.md` says `Object Type=project`, `Active Layer=stable`, `Next Command=project_fork`
2. stable `ProjectSpec` exists

## 4. Procedure

1. read stable `ProjectSpec`
2. create or overwrite `docs/specs/project/candidate/c_project.md`
3. carry forward the full stable `ProjectSpec` as the new candidate starting point
4. ensure the new candidate still carries the five mandatory `ProjectSpec` sections:
   - `Governed Unit Definition`
   - `Support Surface Rules`
   - `Topology Mapping`
   - `Current Formal Object Graph`
   - `Global Constraint Alignment`
5. carry forward stable bindings as part of that new candidate starting point
6. delete outdated candidate-side project process files if they exist
7. write `_status.md`:
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=project_check`

## 5. Output Contract

The output must report:

1. candidate truth file write result
2. five-section carry-forward result
3. candidate-side process file cleanup result
4. lifecycle-state transition result
5. `_status.md` update result
6. `round conclusion`
7. `current state`
8. `next step`
9. `why this next step`
10. `next-stage entry gap`
11. the `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8.6
12. if a future extension introduces a checkpoint stop, the same close-out block must also report `resume signal`

## 6. Non-Goals

1. stable verification
2. project promotion

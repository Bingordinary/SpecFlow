# Project Init Command

## 1. Purpose

`project_init` creates the first stable `ProjectSpec` for a historical repository that is entering the project-topology governance model for the first time.

## 2. Scope

It handles:

1. creating `docs/specs/project/stable/s_project.md`
2. registering `project` in `_status.md`
3. recording current stable bindings to `scenario`, `unit`, `shared_contract`, and `system_constraints`
4. writing the first repository governance coordinate system rather than only a refs list

It does not:

1. create candidate project truth
2. replace module, flow, or shared truth authoring

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `project_init` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. no current stable `ProjectSpec` exists
2. the repository's current formal object set can be stated safely from current truth
3. the repository's governed-unit definition, support-surface rules, topology mapping, current formal object graph, and global constraint alignment can be stated safely from current truth
4. read `project_spec_policy.md`, `spec_policy.md`, and `command_policy.md`

## 4. Procedure

1. read current formal `unit`, `scenario`, `shared_contract`, and `system_constraints` truth
2. read the current repository paths that must be governed by the first stable `ProjectSpec`
3. write the first stable `ProjectSpec`
4. ensure that first stable `ProjectSpec` explicitly states all five mandatory sections from `project_spec_policy.md`:
   - `Governed Unit Definition`
   - `Support Surface Rules`
   - `Topology Mapping`
   - `Current Formal Object Graph`
   - `Global Constraint Alignment`
5. ensure the first stable `ProjectSpec` does not stop at refs only; it must also state the repository's object-splitting rule and path-ownership rule
6. write or upsert `_status.md` row:
   - `Object Type=project`
   - `Object=project`
   - `Stable=yes`
   - `Candidate=no`
   - `Active Layer=stable`
   - `Next Command=project_stable_verify`

## 5. Stop Conditions

1. the first stable `ProjectSpec` exists
2. project state is registered in `_status.md`

## 6. Output Contract

The output must report:

1. stable truth file write result
2. five-section initialization result
3. topology and support-surface initialization result
4. `_status.md` registration result
5. lifecycle-state transition result
6. `round conclusion`
7. `current state`
8. `next step`
9. `why this next step`
10. `next-stage entry gap`
11. the `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8.6
12. if a future extension introduces a checkpoint stop, the same close-out block must also report `resume signal`

## 7. Non-Goals

1. creating a candidate project round
2. implementing code changes

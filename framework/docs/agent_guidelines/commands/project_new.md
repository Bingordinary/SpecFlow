# Project New Command

## 1. Purpose

`project_new` creates the first candidate `ProjectSpec` for a brand-new project-topology round when no historical stable `ProjectSpec` exists yet.

## 2. Scope

It handles:

1. creating `docs/specs/project/candidate/c_project.md`
2. initializing current candidate bindings
3. registering project candidate state in `_status.md`
4. writing the first candidate repository governance coordinate system rather than only a refs list

### 2.1 Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `project_new` may produce that advancing result; later local confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. no current project row exists in `_status.md`
2. the repository can safely state one initial candidate project topology
3. the repository can safely state one initial candidate governed-unit definition, support-surface rules, topology mapping, current formal object graph, and global constraint alignment
4. read `project_spec_policy.md`, `spec_policy.md`, and `command_policy.md`

## 4. Procedure

1. create `c_project.md`
2. initialize `scenario_refs`, `unit_refs`, `shared_contract_refs`, and `system_constraints_stable_ref`
3. write the first candidate `ProjectSpec` with all five mandatory sections from `project_spec_policy.md`:
   - `Governed Unit Definition`
   - `Support Surface Rules`
   - `Topology Mapping`
   - `Current Formal Object Graph`
   - `Global Constraint Alignment`
4. ensure the first candidate `ProjectSpec` does not stop at refs only; it must also state the repository's object-splitting rule and path-ownership rule
5. write or upsert `_status.md` row:
   - `Object Type=project`
   - `Object=project`
   - `Stable=no`
   - `Candidate=yes`
   - `Active Layer=candidate`
   - `Next Command=project_check`

## 5. Output Contract

The output must report:

1. candidate truth file write result
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

## 6. Non-Goals

1. creating stable project truth
2. replacing `project_init`

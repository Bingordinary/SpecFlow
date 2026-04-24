# Project Check Command

## 1. Purpose

`project_check` checks whether the current candidate `ProjectSpec` is sufficiently closed to constrain later project verification and promotion.

In plain words:

1. it does not stop at binding refs
2. it checks whether the candidate `ProjectSpec` can actually act as the repository governance coordinate system
3. it must fail when the project can still describe topology in prose but cannot decide current governed path ownership deterministically

## 2. Lifecycle-State Advance Inheritance

When this command advances `_status.md`, that advancement inherits the authoritative / non-authoritative central contract defined in Section 8.5 of `specflow/framework/docs/agent_guidelines/command_policy.md`.
Only a new independent full-scope run of `project_check` may produce that advancing result; later repair confirmation or scoped follow-up review must not advance lifecycle state.

## 3. Preconditions

1. `_status.md` says `Object Type=project`, `Active Layer=candidate`, `Next Command=project_check`
2. current candidate `ProjectSpec` exists

## 4. Procedure

1. read current candidate `ProjectSpec`
2. verify the five mandatory `ProjectSpec` sections are all present and materially closed:
   - `Governed Unit Definition`
   - `Support Surface Rules`
   - `Topology Mapping`
   - `Current Formal Object Graph`
   - `Global Constraint Alignment`
3. verify required bindings are explicit:
   - `scenario_refs`
   - `unit_refs`
   - `shared_contract_refs`
   - `system_constraints_stable_ref`
4. verify all referenced objects exist at the declared layer
5. verify `Governed Unit Definition` is closed enough to decide:
   - what qualifies as a formal `unit`
   - what must be promoted into `shared_contract`
   - what stays outside command-target truth
6. verify `Support Surface Rules` are closed enough to decide which current repository paths are governed support surfaces rather than command-target truth
7. verify `Topology Mapping` is executable against the current repository surface under the declared governed roots, at minimum:
   - governed roots are explicit
   - ignore rules are explicit
   - current unit/shared/support ownership rules are explicit
   - conflict resolution order is explicit
   - each current governed repository path resolves to one outcome only
   - no current governed repository path remains `unmapped`
   - no current repository path resolves to more than one formal command-target object
8. verify `Current Formal Object Graph` is closed enough to decide:
   - the current `unit`, `scenario`, and `shared_contract` identity set
   - the currently active relation graph among them
   - whether the graph stated in the candidate matches the current Spec files it declares
9. verify `Global Constraint Alignment` names one explicit stable `system_constraints` baseline that exists now
10. if pass, write `_check_result/project.md` and advance `Next Command=project_verify`
11. if not pass, keep `Next Command=project_check`

## 5. Output Contract

The output must report:

1. `check gate result`
2. five-section closure result
3. topology-executability result
4. current path-ownership closure result
5. current formal object-graph closure result
6. `_check_result/project.md` write, delete, or keep result
7. `_status.md` update result
8. `round conclusion`
9. `current state`
10. `next step`
11. `why this next step`
12. `next-stage entry gap`
13. the `user-facing close-out block` required by `specflow/framework/docs/agent_guidelines/command_policy.md` Section 8.6
14. if a future extension introduces a checkpoint stop, the same close-out block must also report `resume signal`

## 6. Non-Goals

1. implementation planning
2. code implementation

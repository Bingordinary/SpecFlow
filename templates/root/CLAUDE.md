## Host Instructions

If the current command or governance flow explicitly consumes project-local standards, follow only the registered files selected by `docs/project_standards/_registry.md` for the current `surface`, `consumed_by`, and `applies_to` scope.

Content outside the managed block below belongs to the host repository.

Keep repository-specific rules outside the managed block. `specFlow` tooling may update only the managed block.

## Mermaid Communication Rules

- When explaining a Mermaid diagram, refer to nodes by the exact visible text shown in the diagram. Do not refer to nodes only as hidden IDs such as `A`, `B`, or `C`.
- If short node identifiers are needed for repeated references, include the identifier in the visible node label as well, for example `B["B. Module Main Spec"]`, and use that same visible name in the prose.

<!-- SPECFLOW:BEGIN -->
## specFlow Addendum

`specFlow` is a development governance flow that treats Specs as the source of truth and uses standard commands to drive design, implementation, verification, and promotion.

### 1. Request Detection

When a request hits any of the following, handle it with `specFlow` rules:

1. Standard commands:
   - `unit_init:{unit}`
   - `unit_stable_verify:{unit}`
   - `unit_new:{unit}`
   - `unit_fork:{unit}`
   - `unit_check:{unit}`
   - `unit_plan:{unit}`
   - `unit_impl:{unit}`
   - `unit_verify:{unit}`
   - `unit_promote:{unit}`
   - `scenario_new:{scenario}`
   - `scenario_stable_verify:{scenario}`
   - `scenario_fork:{scenario}`
   - `scenario_check:{scenario}`
   - `scenario_verify:{scenario}`
   - `scenario_promote:{scenario}`
   - `project_init`
   - `project_new`
   - `project_stable_verify`
   - `project_fork`
   - `project_check`
   - `project_verify`
   - `project_promote`
2. Governance review entries:
   - `spec_flow_review`
   - `spec_flow_design_review`
   - `shared_ops:{natural-language request}`
3. Requests involving `unit`, `scenario`, or `project` truth, state progression, candidate closure, formal promotion, Shared Contract, shared_ops routing, or system constraints.
4. Requests involving registered project-local standards under `docs/project_standards/`.
5. Requests to create, register, or tighten a project-local standard for the current project.
6. Direct implementation requests that would modify repo-tracked code or other repo-tracked implementation-side files.

For direct implementation requests:

1. read `specflow/framework/docs/agent_guidelines/implementation_change_policy.md` first
2. classify the request as `implementation_only`, `truth_writeback_required`, or `boundary_unclear`
3. if the result is `truth_writeback_required` or `boundary_unclear`, do not start from code; route to the smallest legal next step defined by `specflow/framework/docs/agent_guidelines/command_policy.md`
4. if the result is `implementation_only`, still obey `_status.md`, `Active Layer`, `Next Command`, and the follow-up verification duty of the current layer

If none of the above is hit, continue following the host agent's other rules.

### 2. Standard Commands

Standard command forms:

```text
unit     -> {command}:{unit}
scenario -> {command}:{scenario}
project  -> {command}
```

See the command policy:

- `specflow/framework/docs/agent_guidelines/command_policy.md`

See the command files:

- `specflow/framework/docs/agent_guidelines/commands/`

The standard commands are grouped by object family:

1. `unit`
   - `unit_init:{unit}`
   - `unit_stable_verify:{unit}`
   - `unit_new:{unit}`
   - `unit_fork:{unit}`
   - `unit_check:{unit}`
   - `unit_plan:{unit}`
   - `unit_impl:{unit}`
   - `unit_verify:{unit}`
   - `unit_promote:{unit}`
2. `scenario`
   - `scenario_new:{scenario}`
   - `scenario_stable_verify:{scenario}`
   - `scenario_fork:{scenario}`
   - `scenario_check:{scenario}`
   - `scenario_verify:{scenario}`
   - `scenario_promote:{scenario}`
3. `project`
   - `project_init`
   - `project_new`
   - `project_stable_verify`
   - `project_fork`
   - `project_check`
   - `project_verify`
   - `project_promote`

Governance review entries are:

1. `spec_flow_review`
2. `spec_flow_design_review`
3. `shared_ops:{natural-language request}`

### 2.1 Project-First Repository Routing

When the repository is brand-new, unfamiliar, or its governed path ownership is not yet explicit:

1. do not guess `unit` or `scenario` boundaries from directory shape alone
2. establish or read current `ProjectSpec` first
3. if no current project row exists, start with `project_new` or `project_init` as appropriate
4. only after `ProjectSpec` states `Governed Unit Definition`, `Support Surface Rules`, `Topology Mapping`, `Current Formal Object Graph`, and `Global Constraint Alignment` may later `unit` or `scenario` work claim repository coordinates

Additional rules:

1. `spec_flow_review`, `spec_flow_design_review`, and `shared_ops:{natural-language request}` are not standard object-lifecycle commands.
2. `shared_topology` and `shared_sync` are internal shared flows used after Shared Contract topology, binding, or lifecycle changes; users should enter shared work through `shared_ops`.
3. `impact_sync` is an internal generic impact-reconciliation flow, not a user-facing command.
4. `project_standard_create` is not a standard user-facing command. It is an internal flow the agent may use when the user asks to create a project-local standard.
5. plain `spec_flow_review` means the default governance-baseline review defined in `specflow/framework/docs/agent_guidelines/spec_flow_review.md` unless the user explicitly narrows the scope.
6. plain `spec_flow_design_review` means the default design-baseline review defined in `specflow/framework/docs/agent_guidelines/spec_flow_design_review.md` unless the user explicitly narrows the scope.
7. that default `spec_flow_review` must cover the shared-governance rule set, at minimum `shared_ops.md`, `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`, even when the user did not mention shared governance explicitly.
8. that default `spec_flow_review` must also cover the impact-reconciliation rule set, at minimum `impact_sync_policy.md`, `process_snapshot_contract.md`, `recovery_policy.md`, template `_status.md`, and the process README files.
9. that default `spec_flow_review` must also cover the tooling execution contract set, at minimum `tooling_execution_policy.md`, `specflow/tooling/README.md`, and the in-scope tooling source files under `specflow/tooling/`.
10. if the review output does not explicitly report shared-governance coverage, impact-reconciliation coverage, tooling coverage, and their results, the `spec_flow_review` is not complete and must not be treated as a `pass`.
11. plain `spec_flow_design_review` must not be narrowed to recently touched files, tooling source, or only one design block unless the user explicitly narrows it that way.
12. before issuing any `pass` conclusion for plain `spec_flow_design_review`, confirm that the hard-blocker result, all eight question scores, the fixed group averages, and the `weighted_score` required by `spec_flow_design_review.md` have all been read and are explicitly reported in the review output.

### 3. How To Resolve Objects And Files

`unit`, `scenario`, and `project` are formal object names, not concrete file names.

If the user names an object but not a concrete file, read this first:

- `docs/specs/_status.md`

Then resolve the actual target from `Object Type` and `Active Layer`:

1. `unit`
   - `stable` -> `docs/specs/units/stable/s_unit_{unit}.md`
   - `candidate` -> `docs/specs/units/candidate/c_unit_{unit}.md`
2. `scenario`
   - `stable` -> `docs/specs/scenarios/stable/s_scenario_{scenario}.md`
   - `candidate` -> `docs/specs/scenarios/candidate/c_scenario_{scenario}.md`
3. `project`
   - `stable` -> `docs/specs/project/stable/s_project.md`
   - `candidate` -> `docs/specs/project/candidate/c_project.md`

If the user gives a concrete file prefix, treat it as a file reference:

1. `s_unit_xxx`
   - Refers to the `stable` main file
2. `c_unit_xxx`
   - Refers to the `candidate` main file
3. `s_scenario_xxx`
   - Refers to the `stable` flow file
4. `c_scenario_xxx`
   - Refers to the `candidate` flow file
5. `s_project`
   - Refers to the stable project file
6. `c_project`
   - Refers to the candidate project file

### 4. Read Order For Non-Command Requests

If a request is inside the `specFlow` scope but is not a standard command, handle it in this default order:

1. If the request directly asks to modify repo-tracked code or other implementation-side files, read `specflow/framework/docs/agent_guidelines/implementation_change_policy.md` first.
2. Then determine whether the request targets:
   - a command-target truth object
   - or a governance object / governance flow
3. If it targets a governance object or governance flow:
   - read the governance file that defines that flow's scope, preconditions, and procedure first
   - follow that file's declared read scope instead of automatically starting from `docs/specs/_status.md`
   - if the flow is plain `spec_flow_review`, do not narrow it to main command-chain files, recent edits, or non-shared rules only unless the user explicitly narrows it that way
   - if the flow is plain `spec_flow_design_review`, do not narrow it to recently touched files, tooling source, or only one design block unless the user explicitly narrows it that way
   - before issuing any `pass` conclusion for plain `spec_flow_review`, confirm that the shared-governance rule set, the impact-reconciliation rule set, and the tooling execution contract set required by `spec_flow_review.md` have all been read and are explicitly reported in the review output
   - before issuing any `pass` conclusion for plain `spec_flow_design_review`, confirm that the hard-blocker result, all eight question scores, the fixed group averages, and the `weighted_score` required by `spec_flow_design_review.md` have all been read and are explicitly reported in the review output
4. If it targets a command-target truth object:
   - read `docs/specs/_status.md` to confirm the target object's current `Active Layer` and `Next Command`
5. Then read the current-layer main truth file for that object.
6. If that truth file explicitly references appendix files or Shared Contract files, read them too.
7. If the task involves the global technical baseline, shared mechanisms, or global exceptions, also read:
   - `docs/specs/system_constraints/stable/s_system_constraints.md`
8. Then decide whether the current action is:
   - explanation only
   - modifying `candidate`
   - modifying `stable`
   - executing a standard command
   - or applying the direct implementation gate before any code modification

### 5. Mandatory Constraints

1. Do not guess behavior by bypassing the source-of-truth files under `docs/specs/`.
2. If you are unsure whether a change is a behavior change, treat it as a behavior change.
3. Behavior changes must not start from code. Follow `specflow/framework/docs/agent_guidelines/spec_policy.md` first.
4. Direct implementation requests must first be classified through `specflow/framework/docs/agent_guidelines/implementation_change_policy.md`. `truth_writeback_required` and `boundary_unclear` must not start from code.
5. A brand-new unit or scenario may start with `candidate`; its first `stable` is created later by `unit_promote:{unit}` or `scenario_promote:{scenario}`.
6. A historical unit entering governance for the first time must begin with `unit_init:{unit}` to create its first `stable`.
7. A historical project entering governance for the first time must begin with `project_init` to create its first `stable`.
7. Under `docs/specs/`, every Spec file except `candidate` main files, candidate appendix files, and `docs/specs/shared_contracts/candidate/*.md` is a behavior source of truth and should normally enter git history.
8. `candidate` main files, candidate appendix files, and `docs/specs/shared_contracts/candidate/*.md` are draft-layer artifacts, but draft-layer status does not block commits. When a round reaches a reviewable checkpoint, those files should normally be committed together with the linked process or code changes of that checkpoint.
9. Changes to `specflow/framework/docs/agent_guidelines/*.md` should normally be committed in the current task.
10. When Spec, command, and git-flow rules conflict, do not guess. Go back to the relevant policy or command file.

### 6. Git Handling Rules

Use these default git rules in `specFlow` tasks:

1. If the task changes only candidate draft files, commit when the round has reached a reviewable checkpoint. Purely temporary incomplete draft saves do not require their own commit.
2. If the task changes code files, formal source-of-truth files, governance files, or registered entry index files, `git commit` in the current task by default.
3. If the task changes registered entry index files, ensure managed block consistency before commit.
4. For exact file boundaries, exceptions, and promotion-specific rules, read `specflow/framework/docs/agent_guidelines/git_policy.md`.

### 7. Must-Know Files

If the task falls inside the `specFlow` scope, at minimum you should know what these files are responsible for:

1. `specflow/framework/docs/agent_guidelines/spec_policy.md`
   - Defines Spec objects, layers, source-of-truth boundaries, and reading rules
2. `specflow/framework/docs/agent_guidelines/implementation_change_policy.md`
   - Defines how direct implementation requests are classified and when code changes must be diverted back to truth writeback
3. `specflow/framework/docs/agent_guidelines/command_policy.md`
   - Defines standard commands, direct-implementation gates, and the default lifecycle
4. `specflow/framework/docs/agent_guidelines/git_policy.md`
   - Defines which changes normally require commits and which do not
5. `docs/specs/_status.md`
   - Records each formal object's current status, active layer, and default next command

Do not blindly read everything at once. Read only what the current task actually needs.
<!-- SPECFLOW:END -->

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
   - `spec_init:{module}`
   - `stable_verify:{module}`
   - `spec_new:{module}`
   - `spec_fork:{module}`
   - `cand_check:{module}`
   - `cand_plan:{module}`
   - `cand_impl:{module}`
   - `cand_verify:{module}`
   - `cand_promote:{module}`
2. Governance review entries:
   - `spec_flow_review`
   - `shared_ops:{natural-language request}`
3. Requests involving module Specs, state progression, candidate closure, formal promotion, Shared Contract, shared_ops routing, or system constraints.
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

Standard command format:

```text
{command}:{module}
```

See the command policy:

- `specflow/framework/docs/agent_guidelines/command_policy.md`

See the command files:

- `specflow/framework/docs/agent_guidelines/commands/`

The standard commands are:

1. `spec_init:{module}`
2. `stable_verify:{module}`
3. `spec_new:{module}`
4. `spec_fork:{module}`
5. `cand_check:{module}`
6. `cand_plan:{module}`
7. `cand_impl:{module}`
8. `cand_verify:{module}`
9. `cand_promote:{module}`

Governance review entries are:

1. `spec_flow_review`
2. `shared_ops:{natural-language request}`

Additional rules:

1. `spec_flow_review` and `shared_ops:{natural-language request}` are not standard module commands in `{command}:{module}` form.
2. `shared_topology` and `shared_sync` are internal shared flows used after Shared Contract topology, binding, or lifecycle changes; users should enter shared work through `shared_ops`.
3. `project_standard_create` is not a standard user-facing command. It is an internal flow the agent may use when the user asks to create a project-local standard.
4. plain `spec_flow_review` means the default governance-baseline review defined in `specflow/framework/docs/agent_guidelines/spec_flow_review.md` unless the user explicitly narrows the scope.
5. that default `spec_flow_review` must cover the shared-governance rule set, at minimum `shared_ops.md`, `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`, even when the user did not mention shared governance explicitly.
6. that default `spec_flow_review` must also cover the tooling execution contract set, at minimum `tooling_execution_policy.md`, `specflow/tooling/README.md`, and the in-scope tooling source files under `specflow/tooling/`.
7. if the review output does not explicitly report shared-governance coverage, tooling coverage, and their results, the `spec_flow_review` is not complete and must not be treated as a `pass`.

### 3. How To Resolve Modules And Files

`{module}` refers to the formal module name, not a concrete file name.

If the user says only a module name such as `module_example`, read this first:

- `docs/specs/_status.md`

Then resolve the actual target from `Active Layer`:

1. If `Active Layer=stable`
   - Default target: `docs/specs/modules/stable/s_{module}.md`
2. If `Active Layer=candidate`
   - Default target: `docs/specs/modules/candidate/c_{module}.md`

If the user gives a concrete file prefix, treat it as a file reference:

1. `s_module_xxx`
   - Refers to the `stable` main file
2. `c_module_xxx`
   - Refers to the `candidate` main file

### 4. Read Order For Non-Command Requests

If a request is inside the `specFlow` scope but is not a standard command, handle it in this default order:

1. If the request directly asks to modify repo-tracked code or other implementation-side files, read `specflow/framework/docs/agent_guidelines/implementation_change_policy.md` first.
2. Then determine whether the request targets:
   - a module behavior object
   - or a governance object / governance flow
3. If it targets a governance object or governance flow:
   - read the governance file that defines that flow's scope, preconditions, and procedure first
   - follow that file's declared read scope instead of automatically starting from `docs/specs/_status.md`
   - if the flow is plain `spec_flow_review`, do not narrow it to main command-chain files, recent edits, or non-shared rules only unless the user explicitly narrows it that way
   - before issuing any `pass` conclusion for plain `spec_flow_review`, confirm that both the shared-governance rule set and the tooling execution contract set required by `spec_flow_review.md` have been read and are explicitly reported in the review output
4. If it targets a module behavior object:
   - read `docs/specs/_status.md` to confirm the target module's current `Active Layer` and `Next Command`
5. If the module task touches module behavior truth, read the main Spec for the current layer.
6. If the main Spec explicitly references appendix files or Shared Contract files, read them too.
7. If the task involves the global technical baseline, shared mechanisms, or global exceptions, also read:
   - `docs/specs/system/stable/s_system_constraints.md`
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
5. A brand-new module may start with `candidate`; its first `stable` is created later by `cand_promote`.
6. A historical module entering governance for the first time must begin with `spec_init:{module}` to create its first `stable`.
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
   - Records each formal module's current status, active layer, and default next command

Do not blindly read everything at once. Read only what the current task actually needs.
<!-- SPECFLOW:END -->

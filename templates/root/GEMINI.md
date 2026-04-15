## Host Instructions

If the current command or governance flow explicitly consumes project-local standards, follow only the registered files selected by `docs/project_standards/_registry.md` for the current `surface`, `consumed_by`, and `applies_to` scope.

Content outside the managed block below belongs to the host repository.

Keep repository-specific rules outside the managed block. `specFlow` tooling may update only the managed block.

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
   - `shared_extract_review`
3. Requests involving module Specs, state progression, candidate closure, formal promotion, Shared Appendix, or system constraints.
4. Requests involving registered project-local standards under `docs/project_standards/`.
5. Requests to create, register, or tighten a project-local standard for the current project.

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
2. `shared_extract_review`

Additional rules:

1. `spec_flow_review` and `shared_extract_review` are not standard module commands in `{command}:{module}` form.
2. `shared_flow_reconcile` is not a standard user-facing command. It is only used to reconcile state after Shared Appendix changes.
3. `project_standard_create` is not a standard user-facing command. It is an internal flow the agent may use when the user asks to create a project-local standard.

### 3. How To Resolve Modules And Files

`{module}` refers to the formal module name, not a concrete file name.

If the user says only a module name such as `module_example`, read this first:

- `docs/specs/_status.md`

Then resolve the actual target from `Active Layer`:

1. If `Active Layer=stable`
   - Default target: `docs/specs/stable/s_{module}.md`
2. If `Active Layer=candidate`
   - Default target: `docs/specs/candidate/c_{module}.md`

If the user gives a concrete file prefix, treat it as a file reference:

1. `s_module_xxx`
   - Refers to the `stable` main file
2. `c_module_xxx`
   - Refers to the `candidate` main file

### 4. Read Order For Non-Command Requests

If a request is inside the `specFlow` scope but is not a standard command, handle it in this default order:

1. First determine whether the request targets:
   - a module behavior object
   - or a governance object / governance flow
2. If it targets a governance object or governance flow:
   - read the governance file that defines that flow's scope, preconditions, and procedure first
   - follow that file's declared read scope instead of automatically starting from `docs/specs/_status.md`
3. If it targets a module behavior object:
   - read `docs/specs/_status.md` to confirm the target module's current `Active Layer` and `Next Command`
4. If the module task touches module behavior truth, read the main Spec for the current layer.
5. If the main Spec explicitly references appendix files or Shared Appendix files, read them too.
6. If the task involves the global technical baseline, shared mechanisms, or global exceptions, also read:
   - `docs/specs/system/stable/s_system_constraints.md`
7. Then decide whether the current action is:
   - explanation only
   - modifying `candidate`
   - modifying `stable`
   - executing a standard command

### 5. Mandatory Constraints

1. Do not guess behavior by bypassing the source-of-truth files under `docs/specs/`.
2. If you are unsure whether a change is a behavior change, treat it as a behavior change.
3. Behavior changes must not start from code. Follow `specflow/framework/docs/agent_guidelines/spec_policy.md` first.
4. A brand-new module may start with `candidate`; its first `stable` is created later by `cand_promote`.
5. A historical module entering governance for the first time must begin with `spec_init:{module}` to create its first `stable`.
6. Under `docs/specs/`, every Spec file except `candidate` main files and their supporting appendix files is a behavior source of truth and should normally enter git history.
7. `candidate` main files and their appendix files are draft-layer artifacts. If a task modifies only those files, do not `git commit` by default unless the user asks for it or the active command flow requires it.
8. Changes to `specflow/framework/docs/agent_guidelines/*.md` should normally be committed in the current task.
9. When Spec, command, and git-flow rules conflict, do not guess. Go back to the relevant policy or command file.

### 6. Git Handling Rules

Use these default git rules in `specFlow` tasks:

1. If the task changes only candidate draft files, do not `git commit` by default unless the user explicitly asks for it or the active command flow requires it.
2. If the task changes code files, formal source-of-truth files, governance files, or registered entry index files, `git commit` in the current task by default.
3. If the task changes registered entry index files, ensure managed block consistency before commit.
4. For exact file boundaries, exceptions, and promotion-specific rules, read `specflow/framework/docs/agent_guidelines/git_policy.md`.

### 7. Must-Know Files

If the task falls inside the `specFlow` scope, at minimum you should know what these files are responsible for:

1. `specflow/framework/docs/agent_guidelines/spec_policy.md`
   - Defines Spec objects, layers, source-of-truth boundaries, and reading rules
2. `specflow/framework/docs/agent_guidelines/command_policy.md`
   - Defines standard commands, gates, and the default lifecycle
3. `specflow/framework/docs/agent_guidelines/git_policy.md`
   - Defines which changes normally require commits and which do not
4. `docs/specs/_status.md`
   - Records each formal module's current status, active layer, and default next command

Do not blindly read everything at once. Read only what the current task actually needs.
<!-- SPECFLOW:END -->

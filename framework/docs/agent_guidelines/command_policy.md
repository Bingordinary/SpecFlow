# Command Policy

## 1. Purpose

This file defines how formal commands work in this repository.

It answers five questions:

1. what a command is
2. which formal object families commands operate on
3. which commands are standard lifecycle commands
4. which objects are not command targets
5. which shared gate rules every command must follow

## 2. What A Command Is

A command is the standard workflow entry for one formal command-target object family.

In plain words:

1. `Spec` is the truth
2. `Command` is the action

## 3. Command-Target Object Families

This repository has three command-target object families:

1. `module`
2. `flow`
3. `project`

Shared notes:

1. all three families write state into `docs/specs/_status.md`
2. all three families may use `stable` and `candidate` layers
3. only `module` owns direct implementation responsibility
4. `flow` and `project` are command targets, but they are not modules

Non-command objects:

1. `shared_contract` is not a standard command target
2. `system_constraints` is not a standard command target
3. `impact_sync` is an internal governance flow, not a user-facing standard command

## 4. Command Forms

This repository uses three user-facing command shapes:

1. `module` command form:

```text
{command}:{module}
```

2. `flow` command form:

```text
{command}:{flow}
```

3. `project` command form:

```text
{command}
```

Additional rules:

1. `system_constraints` is not a legal command target
2. `shared_contract` is not a legal standard command target
3. `shared_ops:{natural-language request}` remains the only user-facing shared-governance entry
4. `shared_new`, `shared_extract`, `shared_bind`, `shared_topology`, `shared_sync`, `shared_escape`, and `impact_sync` are internal governance flows, not direct user-facing standard commands

## 5. Standard Commands

### 5.1 Module Commands

1. `spec_init:{module}`
2. `stable_verify:{module}`
3. `spec_new:{module}`
4. `spec_fork:{module}`
5. `cand_check:{module}`
6. `cand_plan:{module}`
7. `cand_impl:{module}`
8. `cand_verify:{module}`
9. `cand_promote:{module}`

### 5.2 Flow Commands

1. `flow_new:{flow}`
2. `flow_stable_verify:{flow}`
3. `flow_fork:{flow}`
4. `flow_check:{flow}`
5. `flow_verify:{flow}`
6. `flow_promote:{flow}`

### 5.3 Project Commands

1. `project_init`
2. `project_new`
3. `project_stable_verify`
4. `project_fork`
5. `project_check`
6. `project_verify`
7. `project_promote`

### 5.4 Shared Governance Entry

The user-facing shared-governance entry remains:

```text
shared_ops:{natural-language request}
```

Rules:

1. `shared_ops` is the only preferred user-facing entry for shared-truth governance
2. it is intent-driven rather than object-name-driven
3. it routes into internal shared flows according to `shared_ops.md`

## 6. Responsibilities By Family

### 6.1 Module

`module` commands own:

1. module truth authoring
2. implementation planning
3. implementation work
4. implementation verification
5. promotion into module stable truth

### 6.2 Flow

`flow` commands own:

1. business-chain truth authoring
2. business-chain closure
3. business-chain verification
4. promotion into stable flow truth

`flow` commands do not own:

1. implementation planning
2. implementation editing

### 6.3 Project

`project` commands own:

1. project-topology truth authoring
2. project-topology closure
3. project-topology verification
4. promotion into stable project truth

`project` commands do not own:

1. implementation planning
2. implementation editing

## 7. Default Lifecycle Order

### 7.1 Module

1. `spec_init`
2. `stable_verify`
3. `spec_fork`
4. `spec_new`
5. `cand_check`
6. `cand_plan`
7. `cand_impl`
8. `cand_verify`
9. `cand_promote`

### 7.2 Flow

1. `flow_new`
2. `flow_stable_verify`
3. `flow_fork`
4. `flow_check`
5. `flow_verify`
6. `flow_promote`

### 7.3 Project

1. `project_init`
2. `project_new`
3. `project_stable_verify`
4. `project_fork`
5. `project_check`
6. `project_verify`
7. `project_promote`

## 8. Shared Gate Rules

These rules apply by default to every command family:

1. do not execute a command if its prerequisite self-checks have not passed
2. process files are not valid just because they exist; their bound truth refs, fingerprints, and command-required fields must also match
3. a formal pass gate, formal verification pass, or lifecycle-state advance may be produced only by a new independent full-scope run of the corresponding command
4. after a command ends with any non-pass result other than a resumable checkpoint explicitly allowed by that command file, later repair or scoped recheck is non-authoritative for lifecycle progression
5. checkpoints are structured stops inside a command, not second lifecycles
6. `shared_contract` and `system_constraints` are always upstream inputs, never the primary output of `flow` or `project` commands

### 8.1 Binding Drift

Candidate-side process files become invalid when any current required binding changes.

At minimum:

1. `module` candidate process files fall back to `cand_check`
2. `flow` candidate process files fall back to `flow_check`
3. `project` candidate process files fall back to `project_check`

### 8.2 Stable Drift

Stable-layer alignment claims become invalid when any current required binding changes.

At minimum:

1. `module` stable alignment falls back to `stable_verify`
2. `flow` stable alignment falls back to `flow_stable_verify`
3. `project` stable alignment falls back to `project_stable_verify`

### 8.3 Shared And Baseline Inputs

1. if a command depends on bound `shared_contract` truth, it must read the exact currently bound shared files
2. if a command depends on the formal global baseline, it must read `docs/specs/system/stable/s_system_constraints.md`
3. `bound_modules`-only metadata drift does not by itself invalidate downstream process files

### 8.4 Impact Reconciliation

1. when one object family's truth or binding change may invalidate downstream objects, the handling round must complete deterministic downstream reconciliation before claiming closure
2. `shared_sync` remains the shared-governance impact-discovery flow for shared changes
3. `impact_sync` is the generic internal fallback-and-cleanup flow once the affected downstream object set is already fixed

### 8.5 Authoritative And Non-Authoritative Result Contract

生命周期推进只能来自一次新的、独立的、全范围命令运行。

规则：

1. 只有当前命令的一次新 full-scope run，才可以产生 formal pass gate、formal verification pass、或推进 `_status.md` 的结果。
2. 如果某个命令已经以 non-pass 结果结束，除非该命令文件明确允许某个 checkpoint 作为可恢复停点，否则后续 repair、local confirmation、scoped recheck、或 follow-up assessment 都属于 non-authoritative。
3. non-authoritative follow-up 可以报告局部修复已经完成，但不得宣称新的生命周期推进、不得写入推进型 `_status.md` 更新、也不得把局部复核包装成新的 formal pass。
4. 各命令文件可以在自己的边界内进一步收紧 rerun 条件，但不得放宽这里定义的 authoritative / non-authoritative 区分。

### 8.6 User-Facing Close-Out Block Contract

每次正式命令输出都必须包含 `user-facing close-out block`。

这个 block 至少必须报告：

1. `round conclusion`
2. `current state`
3. `next step`
4. `why this next step`
5. `next-stage entry gap`
6. 当命令进入 checkpoint 或其他明确可恢复停点时，还必须报告 `resume signal`
7. 各命令文件可以追加更严格的字段或措辞要求，但不得删除这里固定的字段

## 9. Direct Implementation Request Gate

A direct implementation request means the user asks the executor to modify repo-tracked files without first entering a standard command.

Rules:

1. every direct implementation request must be classified first through `implementation_change_policy.md`
2. the only legal classification results are:
   - `implementation_only`
   - `truth_writeback_required`
   - `boundary_unclear`
3. `implementation_only` does not permit skipping current lifecycle gates
4. `truth_writeback_required` and `boundary_unclear` must not start from code

## 10. Non-Goals

This file does not:

1. redefine object truth content in place of `spec_policy.md`
2. create an independent lifecycle for `shared_contract`
3. create an independent command chain for `system_constraints`

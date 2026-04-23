# Flow Policy

## 1. Purpose

This file defines what a formal `flow` object is in this repository and how it differs from `module`, `shared_contract`, and `ProjectSpec`.

It answers five questions:

1. what `flow` formally owns
2. which files carry `flow`
3. which bindings `flow` must record
4. what `flow` verification means
5. how `flow` invalidation works

## 2. Object Definition

`flow_xxx` is the formal business-chain truth object.

It answers:

1. where one user-visible business path starts
2. which modules it traverses
3. which shared contracts are reused along that path
4. what the success result is
5. where failure is absorbed or surfaced
6. how that path is verified end to end

It does not answer:

1. module-local state-machine detail
2. shared-contract field-level body text
3. project-wide default rules
4. implementation ownership for code edits

## 3. Files

`flow` uses two version layers:

1. `docs/specs/flows/stable/s_flow_{name}.md`
2. `docs/specs/flows/candidate/c_flow_{name}.md`

Additional rules:

1. `flow` is a command-target object, but it is not a module
2. it enters `docs/specs/_status.md` using `Object Type=flow`
3. `flow` is identified by its formal flow name, for example `flow_task_execution`

## 4. Required Bindings

Each `flow` must record at minimum:

1. `project_ref`
2. `module_refs`
3. `shared_contract_refs`
4. `system_constraints_stable_ref`

Binding rules:

1. `flow` owns the formal `flow -> module` relation
2. modules do not record `flow_refs` as a required formal binding field
3. `flow stable` must bind only stable-layer dependencies
4. `flow candidate` may bind candidate-layer dependencies, but the bound layer must be explicit
5. `flow` is downstream of `module`, `shared_contract`, and `system_constraints`
6. `ProjectSpec` is downstream of `flow`

## 5. Lifecycle Responsibility

`flow` owns:

1. business-path closure
2. business-path verification
3. promotion of candidate flow truth into stable flow truth

It does not own:

1. implementation planning
2. implementation editing
3. module-local repair

Therefore:

1. `flow` command family has `new`, `module_stable_verify`, `fork`, `check`, `verify`, and `promote`
2. `flow` command family does not have `plan` or `impl`

## 6. Verification Meaning

`flow_verify` means:

1. current flow truth has been read
2. current required module and shared bindings have been revalidated
3. the claimed business chain is actually wired from entry to outcome
4. the verification report names any `affected_modules`

Additional rule:

1. reporting `affected_modules` does not repair or advance those modules automatically
2. if implementation work is needed, those modules must re-enter their own legal `module` command chain

## 7. Invalidation Rules

`flow` process files become invalid when any current required binding changes, including:

1. current flow truth changes
2. any bound module set or required module identity changes
3. any bound `shared_contract` truth, layer, version, or snapshot changes
4. `system_constraints_stable_ref` no longer matches the current stable global baseline

Fallback rules:

1. invalid candidate `flow` falls back to `flow_check`
2. invalid stable `flow` falls back to `flow_stable_verify`

## 8. Non-Goals

This file does not:

1. create a second implementation chain outside `module`
2. redefine `shared_contract`
3. redefine `ProjectSpec`
4. create an independent lifecycle for `system_constraints`

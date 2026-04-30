# Scenario Policy

## 1. Purpose

This file defines what a formal `scenario` object is in this repository and how it differs from `unit`, `shared_contract`, and `repository_mapping`.

It answers five questions:

1. what `scenario` formally owns
2. which files carry `scenario`
3. which bindings `scenario` must record
4. what `scenario` verification means
5. how `scenario` invalidation works

## 2. Object Definition

`scenario_xxx` is the formal trigger-to-outcome chain object and the end-to-end user-result contract.

Its practical value is cross-unit closure.
Unit verification proves that one unit satisfies its own truth.
Scenario verification proves that the required units, shared contracts, and baseline constraints work together from a declared trigger to the promised user-visible result.

It is the normal formal anchor for a user-visible end-to-end outcome, but it is not the mandatory starting point for every user request.
Natural-language routing decides whether the user's goal requires a scenario, a local unit route, shared governance, system-constraint handling, repository mapping, implementation classification, or explanation only.

A `scenario` is required when at least one of these is true:

1. the user's goal promises a user-visible result that crosses more than one unit responsibility
2. the success result cannot be proven by one unit's local verification alone
3. the request asks to verify a declared path, integration, or user flow from input to final result

A `scenario` must not be created merely because several files, directories, units, or implementation tasks are mentioned.
When current repository truth proves that a local unit result is sufficient, the route must stay local.
When the request is only about a rule reused by more than one formal object, the route must test shared governance instead of creating a scenario by default.

It answers:

1. where the chain starts
2. which units it traverses
3. which shared contracts are reused along that chain
4. what the success result is
5. where failure is absorbed, surfaced, or rolled back
6. how that chain is verified end to end

It does not answer:

1. unit-local state-machine detail
2. shared-contract field-level body text
3. repository-wide mapping rules
4. implementation ownership for code edits
5. unit implementation planning or unit-local verification

## 3. Files

`scenario` uses two version layers:

1. `docs/specs/scenarios/stable/s_scenario_{scenario}.md`
2. `docs/specs/scenarios/candidate/c_scenario_{scenario}.md`

Additional rules:

1. `scenario` is a command-target object, but it is not a unit
2. it enters `docs/specs/_status.md` using `Object Type=scenario`
3. `scenario` uses a bare formal scenario ID in `_status.md`, for example `task_execution`

## 4. Required Bindings

Each `scenario` must record at minimum:

1. `repository_mapping_ref`
2. `unit_refs`
3. `shared_contract_refs`
4. `system_constraints_ref`

Binding rules:

1. `scenario` owns the formal `scenario -> unit` relation
2. units do not record `scenario_refs` as a required formal binding field
3. `scenario stable` must bind only stable-layer dependencies
4. `scenario candidate` may bind candidate-layer dependencies, but the bound layer must be explicit
5. `scenario` is downstream of `repository_mapping`, `unit`, `shared_contract`, and `system_constraints`

User-facing routing rule:

1. users do not need to know or name `unit_refs`
2. executors must derive and explain scenario-to-unit binding from current repository truth and the user-visible flow
3. if the bound units cannot be derived safely, the executor must ask for the smallest ordinary-language missing flow or outcome fact, or route to repository mapping when ownership truth is missing

## 5. Lifecycle Responsibility

`scenario` owns:

1. trigger-to-outcome closure
2. cross-unit end-to-end verification
3. promotion of candidate scenario truth into stable scenario truth

It does not own:

1. implementation planning
2. implementation editing
3. unit-local repair
4. unit-local acceptance replacement

Therefore:

1. `scenario` command family has `new`, `stable_verify`, `fork`, `check`, `verify`, and `promote`
2. `scenario` command family does not have `plan` or `impl`

## 6. Verification Meaning

`scenario_verify` is the cross-unit verification gate for the end-to-end user-result contract.
It means:

1. current scenario truth has been read
2. current required unit and shared bindings have been revalidated
3. the claimed chain is actually wired from trigger to outcome
4. the verification report names any `affected_units`

Additional rules:

1. reporting `affected_units` does not repair or advance those units automatically
2. if implementation work is needed, those units must re-enter their own legal `unit` command chain
3. natural-language routing may use `affected_units` to assemble the next internal development chain, but the next executable step must still be each affected unit's current legal command route
4. a scenario verification result must not claim the user-visible end-to-end goal is complete while any required affected unit still has unresolved implementation, verification, truth, binding, or baseline work

## 7. Invalidation Rules

`scenario` process files become invalid when any current required binding changes, including:

1. current scenario truth changes
2. `repository_mapping_ref` no longer matches the current repository mapping
3. any bound unit set or required unit identity changes
4. any bound `shared_contract` truth, layer, version, or snapshot changes
5. `system_constraints_ref` no longer matches the current formal global baseline

Fallback rules:

1. invalid candidate `scenario` falls back to `scenario_check`
2. invalid stable `scenario` falls back to `scenario_stable_verify`

## 8. Non-Goals

This file does not:

1. create a second implementation chain outside `unit`
2. redefine `shared_contract`
3. redefine `repository_mapping`
4. create an independent lifecycle for `system_constraints`

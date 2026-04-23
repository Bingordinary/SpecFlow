# Project Standards Policy

## 1. Purpose

This file defines the extension mechanism for project-local standards on top of the fixed Spec Flow governance baseline.

It answers six questions:

1. where project-local standards live
2. which file is the formal registration entry
3. which side owns the extension interface
4. how commands decide which project-local standards they may read
5. how project-local standards relate to framework baseline rules
6. what kinds of conflicts are forbidden

This policy does not replace the framework baseline.
It defines a controlled extension interface for project-local standards.

---

## 2. Core Principle

Spec Flow has two governance layers here:

1. framework baseline
   - the fixed minimum rules and extension interfaces shipped by Spec Flow
2. project-local standards
   - project-owned concrete rules explicitly registered by the current project

The fixed rule is:

1. project-local standards may add detail, narrow choices, or tighten review gates
2. project-local standards must not weaken, bypass, or delete a framework baseline rule
3. project-local standards must not invent framework interfaces owned by the framework or by a command

In plain words:

1. the framework defines the floor
2. the framework and command documents define the legal plug-in points
3. the project may raise the bar only through those plug-in points
4. the project may not lower the floor

---

## 3. Interface Ownership

The ownership boundary is fixed:

1. framework policy files define the extension mechanism and shared limits
2. command or internal-flow documents define the consumption contract of a concrete `surface`
3. project-local standard files define the project's concrete review, output, or decision rules
4. `docs/project_standards/_registry.md` defines only which registered project-local standards are enabled for the current project

Therefore:

1. the project must not create a new framework consumption interface by writing a registry entry
2. the project must not redefine a command's lifecycle responsibility, command result set, or framework fixed fields
3. the registry is an enablement object, not an interface-definition object

---

## 4. Formal Location

Project-local standards live under:

1. `docs/project_standards/`

The only formal registration entry is:

1. `docs/project_standards/_registry.md`

Rules:

1. commands must not scan `docs/project_standards/` blindly
2. a file under `docs/project_standards/` is not active merely because it exists
3. only standards explicitly registered in `_registry.md` may affect command execution
4. the formal registry path is also the only framework-defined claim signal that the current project supports the project-local standards extension surface
5. for repositories using this framework baseline, `docs/project_standards/_registry.md` is therefore required even when the active table is empty

---

## 5. Supported Standard Types

The supported project-local standard types are:

1. `review_standard`
   - adds project-local review rules or review checks
2. `output_standard`
   - adds project-local output or reporting constraints
3. `decision_standard`
   - adds project-local decision or escalation constraints

Rules:

1. do not invent new standard types casually
2. if a new type is truly required, update this file first
3. one project-local standard file should normally serve one primary standard type

---

## 6. Registry Contract

Each active entry in `docs/project_standards/_registry.md` must record at least:

1. `standard_id`
2. `type`
3. `surface`
4. `file`
5. `consumed_by`
6. `applies_to`
7. `effect`
8. `conflict_rule`

Field meanings:

1. `standard_id`
   - stable project-local identifier
2. `type`
   - one supported type from Section 5
3. `surface`
   - a stable consumption surface already defined by the consuming command or internal flow
4. `file`
   - the project-local standard file path under `docs/project_standards/`
5. `consumed_by`
   - which command or internal flow must read it
6. `applies_to`
   - which modules, flows, or review scenarios it applies to
   - it must use one of the fixed selector forms below instead of project-invented prose
7. `effect`
   - `tighten` or `clarify`
8. `conflict_rule`
   - fixed value `framework_wins`

Fixed `applies_to` selector forms:

1. `all_targets_on_surface`
   - applies to every current target that already hit the consuming command's declared `surface`
2. `module:<formal_module_name>`
   - applies only to one formal module, such as `module:module_ai`
3. `module_set:<formal_module_name>,<formal_module_name>,...`
   - applies only to the listed formal modules
   - module names must use formal module names from `docs/specs/_status.md`
   - no spaces are allowed inside the comma-separated list
4. `review_scenario:<stable_name>`
   - applies only to one command-defined review scenario name
   - the consuming command or internal flow must already define that scenario name formally before a registry entry may use it

Additional rules:

1. `effect=clarify` may explain how the project applies an existing baseline rule more concretely
2. `effect=tighten` may add a stricter project-local requirement
3. project-local standards must not use an effect meaning such as `override`, `relax`, or `disable`
4. `surface` is not a free project-defined name
5. a registry entry may reference a `surface` only after the consuming command or internal flow has formally defined that `surface`
6. the registry must not create a new `surface`, widen a command's consumption scope, or add a new write-back contract by registration alone
7. `consumed_by` may reference only a command or internal flow that already declares support for that `type` and `surface`
8. project-local standards may define project extension fields only when the consuming command explicitly allows those fields as project-side write-back
9. project extension fields are not framework fixed fields
10. `applies_to` is not a free-form note field
11. if a registry entry uses an undefined selector form or an undefined scenario name, that entry is invalid governance input

Applicable shape rule:

1. a command consumes only the registered entry shapes that its own governance document explicitly allows
2. a registry entry that fits the table shape but points to an undefined `surface` is still invalid
3. a command must evaluate `applies_to` only after confirming that the current target already hit the command-defined `surface`
4. `all_targets_on_surface` never widens a command's surface trigger; it only says "apply to every target that already matched that surface"

---

## 7. Consumption Order

When a command or internal flow supports project-local standards, it must read in this order:

1. framework baseline governance files
2. the consuming command or internal-flow document that defines the target `surface`
3. `docs/project_standards/_registry.md`
4. only the registered project-local standard files relevant to the current command, current target, and supported `surface`

Commands must not:

1. read unregistered files from `docs/project_standards/`
2. guess that a similarly named file is active
3. expand into unrelated project-local standards not consumed by the current command
4. consume a registered file through a `surface` that the command has not formally defined
5. treat the existence of a project-local standard as permission to skip framework-baseline review

Merging rule:

1. the consuming command must first finish its framework-baseline judgment
2. it may then consume project-local standards only on its declared `surface`
3. project-local results enter the command only as `tighten` or `clarify` inputs
4. the final command result must still stay inside the framework-defined command result set

---

## 8. Conflict Rules

The conflict rules are fixed:

1. framework baseline wins over project-local standards
2. project-local standards may tighten but not weaken the baseline
3. command interface ownership wins over project-local standards and registry wording
4. if a project-local standard or registry entry conflicts with the baseline or with a command-defined `surface`, report governance drift
5. do not silently merge conflicting rules into an invented middle meaning

If a conflict is found:

1. do not claim the project-local standard is valid
2. keep using the framework baseline
3. report which project-local standard file must be repaired

Direct governance-drift cases include at least:

1. a registry entry references a `surface` not formally defined by the consuming command or internal flow
2. a project-local standard attempts to define a command interface, command lifecycle duty, or framework fixed field
3. a project-local standard attempts to weaken a framework gate
4. a registry entry points to a command or internal flow that does not declare support for that standard type or `surface`

---

## 9. Missing Or Invalid Registry Cases

If a command supports project-local standards and `docs/project_standards/_registry.md` is missing:

1. do not guess that no project-local standards exist
2. use the formal claim signal from Section 4 instead of executor judgment
3. for repositories using this framework baseline, the required registry path means the extension surface is claimed
4. therefore a missing `docs/project_standards/_registry.md` is governance drift and must be reported directly
5. do not silently downgrade that case into "no active project-local standards"

If the registry exists but one entry is invalid:

1. ignore that invalid entry for command execution
2. report governance drift explicitly
3. do not let the invalid entry silently modify command behavior
4. do not infer a replacement `surface` or replacement command by executor judgment

---

## 10. Relationship To Commands

This policy does not automatically force every command to read project-local standards.

Each command or internal flow that consumes project-local standards must explicitly say:

1. that it reads `docs/project_standards/_registry.md`
2. which `surface` names it defines and supports
3. which entry shapes it may consume
4. which part of its decision surface those project-local standards may tighten or clarify
5. whether those standards may affect pass, fallback, or output write-back
6. which project-side extension fields, if any, are allowed

`spec_flow_review` is one such governance flow.
When it runs in the current project instance, it should:

1. read the active registered project-local standard files resolved from `docs/project_standards/_registry.md` as governance inputs instead of ignoring their content
2. review those active registered files for governance conflict against the framework baseline
3. if `spec_flow_review` also defines a project-local overlay surface for tightening or clarifying its own result, resolve that overlay separately under `spec_flow_review.md` rather than using the overlay selector to narrow the governance-input read set

`spec_flow_design_review` is also a governance flow.
When it runs in the current project instance, it should:

1. read the active registered project-local standard files resolved from `docs/project_standards/_registry.md` as design-governance inputs instead of ignoring their content
2. review those active registered files for design-surface cost, entry burden, and governance conflict against the framework baseline
3. treat those active files as in-scope design inputs only in v1, because `spec_flow_design_review` does not define a project-local overlay surface for tightening or clarifying its own result yet

---

## 11. Non-Goals

This policy does not:

1. create a plugin execution system
2. allow arbitrary code hooks
3. allow project-local standards to weaken framework gates
4. replace module Specs as behavior truth
5. let the registry define new command surfaces

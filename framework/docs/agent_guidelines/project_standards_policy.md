# Project Standards Policy

## 1. Purpose

This file defines how a project may register and consume project-local review standards on top of the fixed Spec Flow governance baseline.

It answers six questions:

1. where project-local standards live
2. which file is the formal registration entry
3. which standard types are supported
4. how commands decide which project-local standards they must read
5. how project-local standards relate to framework baseline rules
6. what kinds of conflicts are forbidden

This policy does not replace the framework baseline.
It defines a controlled extension surface for project-local standards.

---

## 2. Core Principle

Spec Flow has two layers of review standards:

1. framework baseline
   - the fixed minimum rules shipped by Spec Flow
2. project-local standards
   - additional standards explicitly registered by the current project

The fixed rule is:

1. project-local standards may add detail, narrow choices, or tighten review gates
2. project-local standards must not weaken, bypass, or delete a framework baseline rule

In plain words:

1. the framework defines the floor
2. the project may raise the bar
3. the project may not lower the floor

---

## 3. Formal Location

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

## 4. Supported Standard Types

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

## 5. Registry Contract

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
   - one supported type from Section 4
3. `surface`
   - the command-local review or output surface this standard extends
4. `file`
   - the project-local standard file path under `docs/project_standards/`
5. `consumed_by`
   - which command or internal flow must read it
6. `applies_to`
   - which modules, flows, or review scenarios it applies to
7. `effect`
   - `tighten` or `clarify`
8. `conflict_rule`
   - fixed value `framework_wins`

Additional rules:

1. `effect=clarify` may explain how the project applies an existing baseline rule more concretely
2. `effect=tighten` may add a stricter project-local requirement
3. project-local standards must not use an effect meaning such as `override`, `relax`, or `disable`
4. framework-consumable surfaces must use stable names defined by the consuming command or internal flow instead of executor invention

---

## 6. Consumption Order

When a command or internal flow supports project-local standards, it must read in this order:

1. framework baseline governance files
2. `docs/project_standards/_registry.md`
3. only the registered project-local standard files relevant to the current command and target

Commands must not:

1. read unregistered files from `docs/project_standards/`
2. guess that a similarly named file is active
3. expand into unrelated project-local standards not consumed by the current command

---

## 7. Conflict Rules

The conflict rules are fixed:

1. framework baseline wins over project-local standards
2. project-local standards may tighten but not weaken the baseline
3. if a project-local standard conflicts with the baseline, report governance drift
4. do not silently merge conflicting rules into an invented middle meaning

If a conflict is found:

1. do not claim the project-local standard is valid
2. keep using the framework baseline
3. report which project-local standard file must be repaired

---

## 8. Missing Or Invalid Registry Cases

If a command supports project-local standards and `docs/project_standards/_registry.md` is missing:

1. do not guess that no project-local standards exist
2. use the formal claim signal from Section 3 instead of executor judgment
3. for repositories using this framework baseline, the required registry path means the extension surface is claimed
4. therefore a missing `docs/project_standards/_registry.md` is governance drift and must be reported directly
5. do not silently downgrade that case into "no active project-local standards"

If the registry exists but one entry is invalid:

1. ignore that invalid entry for command execution
2. report governance drift explicitly
3. do not let the invalid entry silently modify command behavior

---

## 9. Relationship To Commands

This policy does not automatically force every command to read project-local standards.

Each command or internal flow that consumes project-local standards must explicitly say:

1. that it reads `docs/project_standards/_registry.md`
2. which entry shapes it may consume
3. which part of its decision surface those project-local standards may tighten or clarify

`spec_flow_review` is one such governance flow.
When it runs in the current project instance, it should read the active project-local standards resolved from `docs/project_standards/_registry.md` and review them as governance inputs rather than ignoring their content.

---

## 10. Non-Goals

This policy does not:

1. create a plugin execution system
2. allow arbitrary code hooks
3. allow project-local standards to weaken framework gates
4. replace module Specs as behavior truth

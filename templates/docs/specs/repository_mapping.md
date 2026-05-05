---
id: repository_mapping
version: 0.1.0
---

# Repository Mapping

This file records the current repository-structure truth.

It is not a command object.
It has no `stable`, `candidate`, `fork`, `verify`, or `promote` lifecycle.
It is read by `unit`, `scenario`, rule-governance, review, and implementation flows when they need to understand path ownership, governed object boundaries, support surfaces, or repository-level drift.

## 1. Project Overview

Record the repository's purpose, main technology stack, runtime entry points, and any major repository conventions that a human or agent must know before changing governed files.

## 2. Governed Object Map

Record the current formal objects that exist in this repository.

At minimum, keep this map aligned with:

1. `docs/specs/_status.md`
2. `docs/specs/units/stable/`
3. `docs/specs/units/candidate/`
4. `docs/specs/scenarios/stable/`
5. `docs/specs/scenarios/candidate/`
6. `docs/specs/rules/stable/`
7. `docs/specs/rules/candidate/`

## 3. Boundary Rules

State how this repository decides what becomes a governed `unit`, what remains a support surface, and what should stay outside specFlow governance.

These rules must be specific enough for a later command to decide whether a changed path belongs to:

1. one formal `unit`
2. one formal `scenario`
3. one `rule`
4. a support surface
5. ignored or external content

## 4. Path Ownership

Record the path rules that connect repository files to formal objects.

The map must make these decisions resolvable:

1. which source paths are governed by each `unit`
2. which truth files belong to each `unit`, `scenario`, and `rule`
3. which paths are support surfaces
4. which paths are ignored
5. which precedence rule applies when a path could match more than one category

## 5. Rule Alignment

Record which global rules and rules shape repository-level structure.

At minimum, state the current stable `g_` rule reference and any repository-wide Rule assumptions that affect path ownership or object boundaries.

## 6. Drift Handling

Record what counts as repository-structure drift.

At minimum, treat these cases as drift:

1. a governed path no longer maps to a formal object
2. one path maps to multiple formal objects
3. a declared formal object has no matching truth file
4. a support surface is used as if it were a command object
5. a command tries to modify implementation paths outside the target object's declared ownership

When drift is detected, the consuming flow must stop boundary-sensitive work and update this file before continuing.

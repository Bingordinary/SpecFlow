---
name: spec-writeback-guidance
description: Use when a user-approved guidance conclusion must be written into formal candidate Spec truth, and the executor must choose the legal specFlow writeback route without treating chat as durable truth.
---

# Spec Writeback Guidance

## Purpose

Use this skill to move approved guidance conclusions into formal specFlow truth.

This skill is a bridge into existing specFlow routing. It does not create a new command family.

## Process

1. Identify which approved conclusions affect behavior, boundary, acceptance, shared truth, repository mapping, or system constraints.
2. Read `natural_language_routing.md` and route by intent fragments.
3. Read `repository_mapping.md` when ownership, object boundaries, or support surfaces matter.
4. Read `implementation_change_policy.md` before any implementation-side edit.
5. Write only current approved truth into the proper candidate, appendix, Shared Contract, repository mapping, or system constraint proposal path.
6. Do not copy design discussion history, rejected options, or patch-note language into candidate truth.
7. After writeback, route to the smallest legal next step, normally `unit_check` for affected candidate unit truth.

## Output Shape

Report:

1. `writeback_target`
2. `owner_reason`
3. `written_truth_summary`
4. `excluded_discussion_material`
5. `next_legal_step`
6. `why_next_step_is_legal`

## Boundaries

1. Candidate writeback is not `unit_check` pass.
2. Do not implement from chat-only design.
3. Do not create `_plans/active` or `_verify_result`.
4. Do not ask the user to choose internal shared-governance flow names.

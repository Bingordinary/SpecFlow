---
name: using-specflow-guidance
description: Use when a natural-language specFlow request may need product or design guidance before formal truth writeback, especially vague project ideas, scope choices, solution options, or design review before candidate Spec editing.
---

# Using specFlow Guidance

## Purpose

Use this skill to decide whether a natural-language request should enter a guidance skill before a formal specFlow command or truth writeback.

Guidance helps a user form a better project design. It does not create a lifecycle object, advance `_status.md`, replace command policy, or become durable truth.

## Routing Rule

Use guidance when the request is about any of these before the formal candidate truth is clear:

1. framing a vague project or feature idea
2. cutting scope for a first useful version
3. choosing between materially different solution directions
4. reviewing a design before writing it into candidate truth
5. turning an approved discussion result into candidate truth

Do not use guidance when the user gives an exact standard command such as `unit_check:skill` or `unit_plan:agent`. Exact commands route through `command_policy.md`.

## Skill Selection

1. Use `project-framing` when the goal, user, problem, or success meaning is unclear.
2. Use `scope-cutting` when the request is too broad for one candidate round or mixes several independent capabilities.
3. Use `solution-design` when the goal is clear but the solution direction is not locked.
4. Use `design-quality-review` when a discussion-stage design is about to be written into candidate truth.
5. Use `spec-writeback-guidance` when the user has approved a design conclusion that must become durable candidate truth.

## Hard Boundaries

1. Do not implement from guidance output.
2. Do not treat chat-only agreement as durable truth.
3. Do not write `_plans/active`, `_verify_result`, or `_check_result` from guidance.
4. Do not advance `_status.md` from guidance.
5. Once a conclusion affects behavior, boundary, acceptance, shared truth, or system truth, route it into formal specFlow truth writeback before implementation.

## Completion

A guidance step ends with one of these outcomes:

1. `continue_discussion` - the design is not clear enough yet
2. `ready_for_candidate_writeback` - the user has approved a specific conclusion that should be written into candidate truth
3. `route_to_existing_command` - current repository truth already provides a legal command route
4. `stop_for_checkpoint` - the smallest missing decision must be requested through the active routing or command checkpoint rules

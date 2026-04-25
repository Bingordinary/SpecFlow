---
name: design-quality-review
description: Use when a discussion-stage design is about to be written into candidate truth and should be checked for goal alignment, scope fit, over-design, unresolved choices, and verifiable success before writeback.
---

# Design Quality Review

## Purpose

Use this skill before candidate Spec writeback to review the design produced in conversation.

This skill does not review a candidate Spec file. Candidate-file closure is owned by `unit_check`.

## Review Checks

Check whether the discussion-stage design:

1. clearly serves the confirmed user goal
2. has a first-round scope that is small enough to implement and verify
3. has explicit non-goals
4. has one selected direction instead of several unresolved options
5. avoids adding future features to the current round
6. has success criteria that can be verified
7. names unresolved decisions that must stay out of candidate truth

## Output Shape

Return exactly one review result:

1. `continue_discussion`
   - use when the design still needs user clarification or a material direction choice
2. `ready_for_candidate_writeback`
   - use when the approved design can be written into candidate truth
3. `do_not_writeback`
   - use when the design would encode an over-broad, conflicting, or unverifiable direction

Also report:

1. `blocking_design_issues`
2. `writeback_ready_points`
3. `points_to_exclude_from_candidate`

## Boundaries

1. Do not write `_check_result`.
2. Do not advance `_status.md`.
3. Do not claim `unit_check` pass.
4. Do not treat this review as durable truth.

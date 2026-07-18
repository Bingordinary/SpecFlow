---
name: solution-design
description: Use when the goal and scope are clear enough to discuss design, but the solution direction, tradeoffs, affected formal objects, or candidate writeback content are not yet locked.
---

# Solution Design

## Purpose

Use this skill to compare solution directions before writing candidate truth.

The design is still discussion-stage material. Only the approved direction written into candidate Spec becomes durable truth.

## Process

1. Start from the confirmed goal, scope, success criteria, and non-goals.
2. Present two or three materially different approaches when meaningful alternatives exist.
3. For each approach, state:
   - core idea
   - benefit
   - cost
   - likely unit, rule, or global rule impact
   - verification implication
4. Explain likely formal-object impact as an executor-facing consequence, not as terminology the user must already understand.
5. Recommend one approach with the shortest path that satisfies the confirmed goal.
6. Ask for only the ordinary-language decision that blocks candidate writeback.
7. If the selected approach spans a user-flow anchor and one or more local capability chains, describe that as a development chain and still route only to the first legal writeback step.

## Output Shape

1. `options`
2. `recommended_option`
3. `tradeoff_reason`
4. `affected_formal_objects`
5. `verification_implications`
6. `candidate_writeback_items`
7. `decision_needed`
8. `plain_language_decision_question`

## Boundaries

1. Do not preserve multiple unresolved options in candidate truth as if they were all current behavior.
2. Do not choose a Rule or global rule owner by directory shape alone.
3. Do not begin implementation from a selected option until formal truth writeback and the required command gates have passed.
4. Do not ask the user to choose internal command names or object-family names when the design choice can be framed as a user-facing behavior, scope, or verification decision.

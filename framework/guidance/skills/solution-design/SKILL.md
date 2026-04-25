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
   - likely unit, scenario, shared contract, or system constraint impact
   - verification implication
4. Recommend one approach with the shortest path that satisfies the confirmed goal.
5. Ask for only the decision that blocks candidate writeback.

## Output Shape

1. `options`
2. `recommended_option`
3. `tradeoff_reason`
4. `affected_formal_objects`
5. `verification_implications`
6. `candidate_writeback_items`
7. `decision_needed`

## Boundaries

1. Do not preserve multiple unresolved options in candidate truth as if they were all current behavior.
2. Do not choose a Shared Contract, system constraint, or repository mapping owner by directory shape alone.
3. Do not begin implementation from a selected option until formal truth writeback and the required command gates have passed.

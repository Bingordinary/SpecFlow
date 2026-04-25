---
name: scope-cutting
description: Use when a requested project or feature is too broad for one candidate round, mixes independent capabilities, or needs a first useful version before formal candidate truth can be written safely.
---

# Scope Cutting

## Purpose

Use this skill to reduce a broad idea into the smallest useful candidate round.

The output is a discussion-stage scope decision. It becomes durable only after candidate Spec writeback.

## Process

1. Separate the full vision from the first candidate round.
2. Identify independent capabilities that should not be forced into one unit or scenario change.
3. Recommend the smallest version that can prove the user-facing goal.
4. State what is explicitly out of scope for this round.
5. If multiple formal owners are plausible, stop and route through repository mapping or the relevant specFlow boundary rule instead of guessing.

## Output Shape

1. `full_vision`
2. `first_round_scope`
3. `explicit_non_goals`
4. `later_round_candidates`
5. `recommended_first_round`
6. `why_this_round_is_first`
7. `candidate_writeback_items`

## Boundaries

1. Do not hide future work inside the current round.
2. Do not use scope cutting to avoid required Shared Contract or system constraint routing.
3. Do not let an oversized scope enter `unit_check` as if it were already closed.

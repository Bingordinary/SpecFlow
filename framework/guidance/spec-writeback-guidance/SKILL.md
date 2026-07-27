---
name: spec-writeback-guidance
description: Use when a user-approved guidance conclusion must be written into formal candidate Spec truth, and the executor must choose the legal specFlow writeback route without treating chat as durable truth.
---

# Spec Writeback Guidance

## Purpose

Use this skill to move approved guidance conclusions into formal specFlow truth.

This skill is a bridge into existing specFlow routing. It does not create a new command family.

## Process

1. Identify which approved conclusions affect behavior, boundary, acceptance, rule truth, or global rules.
2. Restate the approved conclusion as current truth content, not as a transcript of the discussion.
3. Write only current approved truth into the proper candidate, appendix, Rule, or global rule proposal path.
4. Do not copy design discussion history, rejected options, or patch-note language into candidate truth.
5. After writeback, route to the smallest legal next step.
6. If the approved conclusion describes a larger development chain, write only the durable truth owned by the selected target and rerun routing from current repository truth for the next step.

## Output Shape

Report:

1. `writeback_target`
2. `owner_reason`
3. `written_truth_summary`
4. `excluded_discussion_material`
5. `next_legal_step`
6. `why_next_step_is_legal`
7. `user_visible_state_summary`

## Boundaries

1. Candidate writeback is not the final review step—run `spec_promote {target}` as the agent-level gateway for final validation (the CLI independently validates cache freshness and format).
2. Do not implement from chat-only design.
3. Do not ask the user to choose internal rule-governance flow names.
4. Do not treat a guidance conclusion as durable until it has been written into the required truth target.
5. Do not claim the whole user goal is complete when only the first truth writeback step has landed.

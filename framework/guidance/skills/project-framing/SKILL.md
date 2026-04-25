---
name: project-framing
description: Use when a user has a vague project, feature, or behavior idea and the goal, target user, real problem, success meaning, or non-goals are not yet clear enough to write candidate truth.
---

# Project Framing

## Purpose

Use this skill before writing candidate truth when the project idea is still vague.

The goal is to clarify the original problem, not to design implementation. The output remains discussion context until written into a candidate Spec through specFlow truth writeback.

## Process

1. Read only the repository context needed to understand whether an existing unit, scenario, shared contract, or boundary may own the idea.
2. Ask one focused question at a time when user intent cannot be discovered from repository truth.
3. Clarify these minimum facts:
   - target user or actor
   - problem or need
   - desired outcome
   - success criteria
   - first version non-goals
4. When a fact becomes confirmed and affects formal behavior, mark it as candidate-writeback material.
5. Do not continue into implementation planning.

## Output Shape

Report the framing result in plain language:

1. `goal`
2. `target_user_or_actor`
3. `problem`
4. `success_criteria`
5. `first_version_non_goals`
6. `candidate_writeback_items`
7. `open_questions`

## Boundaries

1. Do not keep confirmed behavior only in chat once it will constrain implementation.
2. Do not create a new unit or scenario by naming alone; repository mapping and command policy still decide formal ownership.
3. Do not write implementation plans from this skill.

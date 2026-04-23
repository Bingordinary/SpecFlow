# Candidate Plans

This directory stores the plan-family process files used during candidate progression.

Rules:

1. `_plans/` is divided into:
   - `draft/`
   - `active/`
2. Neither draft nor active plan files are formal Specs or behavior sources of truth.
3. `draft/{module}.md` stores non-consumable planning work-in-progress for `module_plan`.
4. `active/{module}.md` is the only plan file shape that downstream commands may consume.
5. `module_plan` writes or updates `active/{module}.md` only when the round is `plan-ready`.
6. `module_plan` may write or update `draft/{module}.md` when planning is blocked, in checkpoint, or still accumulating bounded implementation facts.
7. `module_impl` and `module_verify` must consume only `active/{module}.md`.
8. `truth-fallback`, `module_fork`, `module_promote`, candidate-side recovery, and `Candidate=no` must not leave stale draft/active plan files behind.
9. File-specific rules live in:
   - `docs/specs/_plans/draft/README.md`
   - `docs/specs/_plans/active/README.md`

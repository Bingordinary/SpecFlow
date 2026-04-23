# Candidate Plans

This directory stores the plan-family process files used during candidate progression.

Rules:

1. `_plans/` is divided into:
   - `draft/`
   - `active/`
2. Neither draft nor active plan files are formal Specs or behavior sources of truth.
3. `draft/{unit}.md` stores non-consumable planning work-in-progress for `unit_plan`.
4. `active/{unit}.md` is the only plan file shape that downstream commands may consume.
5. `unit_plan` writes or updates `active/{unit}.md` only when the round is `plan-ready`.
6. `unit_plan` may write or update `draft/{unit}.md` when planning is blocked, in checkpoint, or still accumulating bounded implementation facts.
7. `unit_impl` and `unit_verify` must consume only `active/{unit}.md`.
8. `truth-fallback`, `unit_fork`, `unit_promote`, candidate-side recovery, and `Candidate=no` must not leave stale draft/active plan files behind.
9. File-specific rules live in:
   - `docs/specs/_plans/draft/README.md`
   - `docs/specs/_plans/active/README.md`

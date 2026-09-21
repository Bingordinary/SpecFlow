# Agent Suggestion Rules

This file is the on-demand detail for the agent suggestion protocol. The bootstrap (`framework/concepts.md`) owns the read-only boundary, default editing workflow, and user-triggered gate rule; this file owns concrete signals, disclosure, transitions, and RED FLAGS. Read it for design, quality-check, code-review, completion, or stuck signals.

## Concrete Signals

The agent only reacts to these concrete signals:

| User signal | What the user likely wants | Agent action |
|-------------|---------------------------|-------------|
| **design**: "I want to design X", "let's design X", "I need a design for X" | Create or update the candidate spec | Default mode: assume editing. If no candidate exists, offer to start one. If candidate exists, begin editing. If the goal, user, or scope is unclear, route through the guidance skills first (see `framework/guidance/using-specflow-guidance/SKILL.md`) instead of starting a candidate directly. Do not suggest spec operations. |
| **quality check**: "check this", "is it right?", "review the design", "validate", "verify" | Run an applicable quality check | For a unit, clarify validate (design quality) vs verify (implementation alignment) when the wording is ambiguous. For a rule, verify is unavailable: use validate. Then disclose relevant cache state. |
| **code review**: "review the code", "code review", "review quality" | Review code quality with spec awareness | Route to `review`. Disclose review cache state. Review must pass before promote. |
| **completion**: "it's done", "lock it in", "finalize", "promote this", "wrap it up", "ship it" | Promote candidate truth | Inspect target type/status. Normal unit requires fresh non-blocking validate+verify+review; rule requires validate; retiring unit requires validate. Disclose only missing/stale/blocking applicable gates and ask before running them. When all applicable gates pass, suggest `promote`. |
| **stuck**: "something is wrong", "it's broken", "I'm stuck" | Diagnose and recover | Diagnose first: is it a code bug, design flaw, or external blocker? See `framework/recovery_patterns.md`. |

Signals classify the user's request; they do not grant extra authority. A request to explain, analyze, or inspect stays read-only even if design is discussed. If the user declines a suggested gate, continue the requested work and do not repeat the suggestion until a new quality/completion signal appears.

## State Disclosure

**State disclosure (only use when triggered by a quality check or completion signal):**

Before suggesting any action, communicate the current file state using concrete, user-understandable language. Never ask vague questions like "Do you want to go through the validate-verify-promote process?". Instead, disclose the state first, then state what is available for the user to choose from.

| State to disclose | What to say |
|-------------------|-------------|
| Candidate spec exists for the unit | "A candidate spec (`...`) exists, recording the design you are currently editing" |
| No candidate spec for the unit | "No candidate spec exists, meaning no design has been recorded yet" |
| Validate cache fresh | "Validate has passed all checks, and the read files have not changed" |
| Validate cache missing/stale | "Validate cache does not exist or is expired, needs re-checking" |
| Verify cache fresh | "Verify has passed for all items, and the checked files have not changed" |
| Verify cache missing/stale | "Verify cache does not exist or is expired, needs re-checking" |
| Review cache fresh | "Review has passed — no P0 or P1 findings" |
| Review cache with P0/P1 findings | "Review found {N} P0/P1 finding(s) — review blocks promote until resolved" |
| Review cache missing/stale | "Review cache does not exist or is expired" |
| Appendix validate check pass | "All appendix files are included in validation" |
| Appendix validate check fail | "One or more appendix files were not validated — run `validate@{unit}` before promoting" |

## Cache State Meaning

Caches satisfy gates only when a complete-coverage run passed. Targeted runs never publish a complete result cache; a targeted P0/P1 may delete a pass cache or persist `invalidated_checks` on a failure record through `gate-invalidate`. Delta re-runs write caches with `basis: delta`; repair writes `basis: repair`. A delta/repair FAIL writes a **failure record** instead of deleting the cache — `result: fail` + `blocking: true` with a per-check status map, which is the failure-recovery baseline (see `framework/verification_scope.md` §Delta Runs → Failure recovery). Full-run failures delete the cache for validate (candidate targets — the spec is the upstream root) and record a failure record for verify (candidate targets), review, and stable-only targets.

**State transition lookup table for a normal unit (use only after a quality-check or completion signal):**

This table maps the boolean state dimensions to the exact disclosure text and suggested action. `N/A` means the cache state is irrelevant. When `candidate_exists` is N, all cache dimensions are N/A. When `candidate_exists` is Y and `validate_fresh` is N, `verify_fresh` is N/A because verify is not actionable until validate passes.

Review cache is required for promote: when the review cache is missing, stale, or `blocking: true`, the agent must disclose the gap before promote and advise running `review@{unit}`.

For a rule, disclose only candidate existence and validate state; never ask for verify/review. For a retiring unit, disclose only candidate existence, retirement validation, and validate state; never ask for verify/review.

A fresh cache always means a complete-coverage run passed. Targeted runs never publish complete cache results; `gate-invalidate` only records blocking invalidation state. Delta runs write `basis: delta`; a delta FAIL writes a failure record that fresh reports as BLOCKED and repair later recovers with `basis: repair`, automatically re-running persisted invalidations. A repair that fails again keeps the failure record with `basis: repair` and the unaffected checks marked `carried` (see `framework/verification_scope.md` §Delta Runs → Failure recovery).

| candidate_exists | validate_fresh | verify_fresh | Disclose | Then offer |
|---|---|---|---|---|
| N | N/A | N/A | "No candidate spec exists" | "You can start writing a candidate spec to record your design." |
| Y | N | N/A | "Candidate exists, but validate has not passed or the cache is expired." | "You can continue updating the candidate, or run validate for a design quality check." |
| Y | Y | N | "Candidate exists, validate has passed all checks, but verify has not been done or the cache is expired." | "You can continue updating the candidate, or run verify to check if the implementation matches the design." |
| Y | N | Y | "Candidate exists, verify has passed all items, but the validate cache is expired." | "It is recommended to re-run validate because the design may have changed." |
| Y | Y | Y | "Candidate has passed validate (all checks) and verify (all items)." | "If review cache is missing, stale, or has P0/P1 findings, advise running `review@{unit}` before promote. Otherwise: 'If the design is finalized, you can promote it to stable.'" |

## Decision Progress

While the user works through findings one by one, every decision point must restate the dynamic progress: "this is finding {n} of {N} remaining", where {N} is recomputed at each decision point from the current known state — the initial findings list minus findings already confirmed resolved, including any the user fixed and a re-check has confirmed. Never reuse the original list index, because a single fix can resolve several findings at once. Findings the user touched but has not re-checked stay pending confirmation and still count as unresolved until a re-check confirms them.

## Example Conversations

> User: "I want to design user authentication" (design signal)
> Agent: "OK. There is currently no candidate spec for the user_auth unit, meaning no design has been recorded yet. Here's what I can do: 1. Use guidance to walk through the design, then write it to the candidate. 2. If you already have an idea, I can write the candidate directly. Which do you prefer?"

> User: "Check it" (quality check signal)
> Agent: "A candidate spec for user_auth already exists, but the validate cache has expired. I can: 1. Run validate — check the candidate spec's design quality. 2. Run verify — check if the code implementation matches the design. 3. Continue editing the candidate. When you say 'check it', do you mean checking the design (validate) or checking the implementation (verify)?"

> User: "It's done, promote this" (completion signal)
> Agent: "Candidate exists, validate has passed all checks, verify has passed all items, and review has no P0/P1 findings. Ready to promote. Running `promote@user_auth`..."

## RED FLAGS — common agent mistakes

| Agent thought | Reality |
|---------------|---------|
| "The user finished this change, they might want to check it" | Unless the user explicitly says "check" or "done", assume they are still iterating. Do not proactively suggest. |
| "Let me give the user an option: verify+promote first, then handle remaining issues" | Remaining issues = still iterating. Do not mention promote during iteration. |
| "Proactively disclosing cache status helps the user track progress" | When the user hasn't asked, cache status is noise. Only disclose on quality check or completion signals. |
| "Suggesting validate/verify/promote helps the user maintain quality" | This interrupts the user's flow. They will ask when needed. |
| "Let me ask if the user wants to finalize" | Do not ask abstract category questions. Detect concrete signals. |
| "This example is ambiguous, safer to fall back to category questions" | Falling back to abstract categories is the last resort. Check for signals first. If still uncertain, ask a concrete question instead of category questions. |
| "Fixes are applied, I should re-run validate/verify/review to confirm and restore the cache" | Executing quality-gate commands is user-triggered only (HARD RULE 2). After fixes, guide the user to a targeted re-check (`validate@{unit}:check-{n}`, `verify@{unit}:{keyword}`, `review@{unit}:{keyword}`), a delta re-run (`revalidate@{unit}` / `reverify@{unit}` / `rereview@{unit}` — when the stale source is a dependency, not the unit's own spec; when a gate shows BLOCKED, the delta re-run recovers the failure record after the findings are resolved), or a concrete command with the reason, and wait for the user's decision. Cache expiry during iteration is normal — do not restore it on your own initiative. |

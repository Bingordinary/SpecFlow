# Recovery Patterns

Common situations where the standard validate → verify → promote flow diverges. This file is referenced by `framework/concepts.md` §3.

## 1. Code changed without updating candidate

This is the normal iteration pattern. Do not interrupt. When the user signals they are ready to check (quality check signal → run `verify`), verify will detect the divergence and enter divergence resolution.

## 2. Candidate changed without implementing

If the user changes the spec mid-implementation and then wants to check:
1. Run `validate` first (to confirm the new design is sound)
2. Then run `verify`

## 3. Stable and code have drifted (no candidate exists)

The implementation no longer matches recorded stable truth:
1. Run `verify` in stable-only mode to see the gap
2. If a gap exists, suggest creating a candidate fork to reconcile

## 4. Validate fails repeatedly

1. Check whether the issue is `actionable` (concrete repair possible in the candidate) or `needs_decision` (requires user input)
2. If needs_decision: stop and present the question to the user

## 5. User disagrees with divergence suggestion

When the user disagrees with the agent's suggested direction (code_gap / spec_gap / needs_design / blocked) during divergence resolution:

1. Record the user's stated direction as the verdict.
2. Do not argue or re-suggest — the user has more context.
3. Proceed with the agreed next step per the direction table in `unit_verify_checklist.md` Step 7.

## 6. Rule Operation Unsafe or Blocked

When a rule operation cannot proceed safely (ambiguous, combines multiple actions, or a previous step returned `needs_decision`):

1. **Route to exactly one action** — reduce the request to the smallest distinct rule action:
   - Creating new rule truth → write candidate rule (see `spec_writing_guide.md` §6.1)
   - Extracting unit-local truth → extract to rule (see `spec_writing_guide.md` §6.2)
   - Binding/unbinding a unit → edit unit `rule_refs` and body explanation (normal spec editing)
   - Splitting/merging/renaming rules → manual multi-step change: create/update rule files (see `spec_writing_guide.md` §6.1), update consumer `rule_refs`, delete old files
   - Removing a rule → follow the removal flow (see `spec_writing_guide.md` §6.5 and `rule_promote_workflow.md`): run `specflowctl detect --rule <id>` (or `specflowctl detect --all` for a preview), drop every remaining current-layer `rule_refs` reference (promote the units that dropped them, or fork stable-only consumers first), then `specflowctl remove --rule <id>` with user confirmation. Do not delete rule files manually — `remove` is the only operation that deletes rules (a unit promote may auto-remove dropped rules, reported explicitly)
2. **Raise a clarification checkpoint** when the requested meaning is unclear — ask the user for specifics before proceeding
3. **Raise a decision checkpoint** when the user must choose between two valid approaches
4. **Raise a prerequisite checkpoint** when a legal upstream action must happen before the rule change (e.g., a consuming unit must be forked to candidate before its binding can change)

## 7. File-not-found false negative

Pattern-based search (directory-wide or wildcard search) results are
agent-dependent — an empty result does not guarantee the file is absent.

1. When the workflow provides an exact path, always use **direct path access**
   to read or check the file, rather than searching with a pattern
2. If pattern-based search returns empty but a file is expected at a known
   path, fall back to direct path access for the specific file to confirm
3. Only proceed with the normal "file missing" procedure after direct path
   access confirms the file does not exist

## 8. Dependency change stales a consumer's caches

A dependency unit's contract (or a rule file, or a shared appendix) changed, and the caches of every unit that reads it are now STALE — even though the consumer's own spec and code did not change. This is the normal parallel-iteration pattern; STALE is correct behavior, not an error.

1. Diagnose with `fresh@{unit}` — it names the stale gate(s) and the reason.
2. Recovery for a **candidate** target, all user-triggered (HARD RULE 2):
   - **Delta re-run** (`revalidate@{unit}` / `reverify@{unit}` / `rereview@{unit}`) — re-runs only the checks whose evidence went stale plus cross-check, carries the rest over, and rewrites the cache with `basis: delta`. The scope is derived by mechanism from the cache's per-check evidence (stale regions → the checks that declared them, reported as the `DELTA SCOPE` section of `fresh@{unit}`); the agent reports it explicitly before executing (see §Delta Runs in `framework/verification_scope.md`).
   - **Full re-run** (`validate@{unit}` etc.) — re-runs everything. Needed when the delta scope covers every declared check (the mechanical degradation condition — e.g. a whole-spec edit), when the cache is MISSING, when the cache carries no per-check evidence (legacy cache — the scope is then derived semantically per `framework/verification_scope.md` §Delta Runs → Incremental scope), or when the user prefers it. An edit to one section of your own spec is NOT a full re-run trigger: only the checks that declared that section re-run.
   - **Targeted re-check** (`:check-{n}` / `:{keyword}`) — iterative feedback only; never writes a cache, so it does not restore promote eligibility.
3. For a **stable-only** target, the stale confirmation state (a rule or dependency contract changed → `validate: STALE` in `fresh@stable`) is impact detection: every stable unit bound to the changed rule shows up in one report. Recovery from STALE is the delta re-run (`revalidate@{unit}` / `reverify@{unit}` / `rereview@{unit}`) when the confirmation cache exists with `result: pass` — it restores the confirmation state with `basis: delta`; a MISSING stable cache needs the full confirmation run (`validate@{unit}` against stable). If the stable content no longer holds against the changed dependency or rule, fork the unit to reconcile (see §Stable-only Targets in `framework/verification_scope.md`).
4. If a targeted run ever finds P0/P1, it deletes a pass cache; against a **failure record** it invalidates the record's `pass`/`carried` status for that judgment (the failure-recovery delta run must re-run it, not carry it over — see §9 and `framework/verification_scope.md` §Delta Runs → Failure recovery). Resolve the findings before any recovery run.

## 9. Delta re-run finds P0/P1 (failure record recovery)

A delta re-run (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`) that finds P0/P1 writes a **failure record** — it does not delete the cache (`result: fail` + `blocking: true`, findings body, per-check status map; `fresh` reports the gate BLOCKED). Full-run failures that write a record (review full FAIL, stable-only confirmation FAIL) carry the same status map with `pass`/`fail` values only. The record is the failure-recovery baseline: after the findings are resolved, the same delta re-run re-checks only the failed judgments plus the newly affected ones and carries the rest over — no full re-run needed unless the affected scope degrades to the whole run (see §Delta Runs → Failure recovery in `framework/verification_scope.md`). **A failure record without a per-check status map (legacy — written before the failure-recovery design) carries nothing: the recovery degrades to a full re-run** — a failing judgment must never be carried over as if it had passed. If the failure record itself goes stale (content changed before the recovery), derive the scope from the stale sources per the normal delta rules and re-check conservatively — the carried-over evidence of a stale record is not trusted blindly. Full-run failures (candidate validate/verify) still delete the cache — trust establishment failed, there is no baseline; review full failures write the blocking cache, which recovers the same way.

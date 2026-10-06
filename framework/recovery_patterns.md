# Recovery Patterns

Common situations where a normal unit's validate → verify → promote path diverges. Rules use only validate before promote. This file is the recovery package named by `framework/concepts.md`.

## 1. Code changed without updating candidate

Continue without a spec edit only when the implementation change remains authorized and preserves the recorded behavior, ownership, rules, and acceptance conditions. This includes internal changes and repairs that restore the authorized recorded behavior. Do not start a gate automatically; when the user triggers `verify`, follow its divergence-resolution procedure for any mismatch it finds.

If execution reveals possible impact on those conditions or implementation permission, pause affected implementation mutations. Read-only diagnosis may continue: read the affected units/rules and reassess the original request, authorization, and no-impact basis. Shared files alone do not establish impact, and reading or creating a candidate does not expand authorization. Resume through the existing route only when the recorded basis and authorization are clear; ask for missing decisions and apply `framework/concepts.md` HARD RULE 3a to divergence. A later verify does not replace this pre-mutation reassessment.

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
   - Splitting/merging/renaming rules → manual multi-step change: create/update rule files (see `spec_writing_guide.md` §6.1), update and publish consumers, then explicitly remove replaced objects through `framework/removal_workflow.md`
   - Removing a unit, rule, or appendix → decide the removal basis and follow `framework/removal_workflow.md`. Publish surviving referrers first; optional preview is `remove --dry-run`. No manual spec-file deletion or automatic rule cleanup.
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
   - **Delta re-run** (`revalidate@{unit}` / `reverify@{unit}`) — re-runs the checks whose evidence went stale and the current keys the baseline never declared, carries the rest over, and rewrites the cache with `basis: delta`. The scope is derived by mechanism from the cache's per-check evidence (stale regions → the checks that declared them); `gate-plan` reports it explicitly before executing (see §Delta Runs in `framework/verification_scope.md`).
   - **Full re-run** (`validate@{unit}` etc.) — re-runs everything. Needed when the re-run covers every declared check (the plan then carries nothing over — e.g. a whole-spec edit), when the cache is MISSING, when the cache carries no per-check evidence (no compatibility shim — `gate-plan` degrades the plan to the full coverage set per `framework/verification_scope.md` §Delta Runs → Incremental scope; a merged verify cache is failed closed by `fresh@`/`promote` — re-run `verify@{unit}`), or when the user prefers it. An edit to one section of your own spec is NOT a full re-run trigger: only the checks that declared that section re-run (plus any new keys the plan adds).
   - **Targeted re-check** (`:check-{n}` / `:{keyword}`) — iterative feedback only; never writes a cache, so it does not restore promote eligibility.
3. For a **stable-only** target, the stale confirmation state (a rule or dependency contract changed → `validate: STALE` in `fresh@stable`) is impact detection: every stable unit bound to the changed rule shows up in one report. Recovery from STALE is the delta re-run (`revalidate@{unit}` / `reverify@{unit}`) when the confirmation cache exists with `result: pass` — it restores the confirmation state with `basis: delta`; a MISSING stable cache needs the full confirmation run (`validate@{unit}` against stable). If the stable content no longer holds against the changed dependency or rule, fork the unit to reconcile (see §Stable-only Targets in `framework/verification_scope.md`).
4. If a targeted run ever finds P0/P1, immediately run `specflowctl gate-invalidate` for the affected key. It deletes a pass cache; against a **failure record** it preserves the historical `status` map and persists the contradicted key in `invalidated_checks`. It also invalidates a matching open gate run and the contradicted immutable verify evidence, requiring its consumers to recheck affected judgments. The repair plan reads this state automatically and must re-run the key, never carry it over. Resolve the findings before recovery.

## 9. Delta re-run finds P0/P1 (failure record recovery)

A delta re-run (`revalidate@{target}` / `reverify@{unit}`) that finds P0/P1 writes a **failure record** — it does not delete the cache (`result: fail` + `blocking: true`, findings body, per-check status map; `fresh` reports the gate BLOCKED). Full-run failures that write a record (candidate validate/verify full FAIL, stable-only confirmation FAIL) carry the same status map with `pass`/`fail` values only (`basis: full`). After the findings are resolved, repair re-checks the failed judgments, persisted `invalidated_checks`, newly affected judgments, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the rest over. **A failure record whose per-check status map is absent or incomplete, or whose invalidated key cannot map to the current judgment surface, carries nothing: recovery degrades to a full re-run.** If the failure record itself goes stale, derive scope from the stale sources and re-check conservatively.

# Verification Scope

## Problem

`verify` operates on a spec unit against the **entire** relevant codebase. When the codebase is large, the agent's context window saturates, search quality degrades, and the result is unreliable. The all-or-nothing approach forces users to wait for a full scan even when they only need feedback on one aspect.

`validate` checks operate on document quality — they are holistic by nature. Evaluating design soundness (Check 2) requires reading the full design regardless of which section changed. Diff-driven reduction of checks would create blind spots, making partial results unreliable.

## Solution

Introduce two modes — `scoped` and `full` — for verify and review. For validate, `full` is the default because quality checks are holistic. Scoped validate is available only through explicit `:check-{n}` or `:{keyword}` — the user chooses the focus area, not git diff.

This mirrors the `scoped_review` / `deep_audit` distinction from `framework/governance/review_scope.md`, adapted for the different semantics of verify and validate.

> **Note on git fallback divergence:** `review_scope.md` handles "no git working changes" by shifting to `git log -1` as the scoped diff base. Verify cannot follow this pattern because spec content can be edited by the agent without git changes (new untracked files, in-memory edits). Falling back to full mode is the only safe scoped-free behavior in these cases.

## Principles

1. **Default full for validate** — validate checks are holistic quality evaluations. `validate@{target}` always runs all checks.
2. **Default scoped for verify and review** — verify and review are external-mapping checks (spec↔code). The agent runs the smallest useful subset by default. The user gets fast, focused feedback.
3. **Explicit full** — `:full` suffix for comprehensive mode on verify and review.
4. **Explicit scoped for validate** — `:check-{n}` and `:{keyword}` allow the user to scope validate to a specific check. The user chooses the focus, not git diff.
5. **Transparent scope** — every scoped result explicitly states what was checked and warns that it is not a full result.
6. **Promote gate** — only `full` results satisfy the promote gate. Scoped results are for iterative feedback, not final approval.
7. **No fixed item definition** — the framework does not define what an "item" is. The agent reads the spec structure dynamically.

## Syntax

### Validate

| User says | Mode | What agent does |
|-----------|------|-----------------|
| `validate@{target}` | full (default) | All 8 checks + cross-check (unit only — rules have no cross-check). Always runs full because quality checks are holistic. |
| `validate@{target}:check-{n}` | scoped | Single check `{n}` only. User explicitly chooses focus. |
| `validate@{target}:{keyword}` | scoped | Matches keyword to a check name (e.g., "design" → Check 2, "scope" → Check 3). User explicitly chooses focus. |
| `validate@{target}:full` | full | All checks + cross-check (unit only — rules have no cross-check). Explicit equivalent of default. |

### Verify

| User says | Mode | What agent does |
|-----------|------|-----------------|
| `verify@{unit}` | scoped | Git-aware: `git diff HEAD` → map changed files to spec content → verify |
| `verify@{unit}:{keyword}` | scoped | Matches keyword to spec content (section title, feature name, etc.) → verify that content |
| `verify@{unit}:full` | full | Verify all spec content + cross-check |

### Spec Review

| User says | Mode | What agent does |
|-----------|------|-----------------|
| `review@{unit}` | scoped (default) | Git-aware: `git diff HEAD` → map changed files to `affects.files` and `implementation_surface` → review those files with spec context |
| `review@{unit}:full` | full | Read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files with spec context |


### Rule (validate only, verify removed)

| User says | Mode | What agent does |
|-----------|------|-----------------|
| `validate@{rule}` | full (default) | All 8 checks (7 metadata + 1 body quality). Always runs full because quality checks are holistic. |
| `validate@{rule}:check-{n}` | scoped | Single check `{n}` only. User explicitly chooses focus. |
| `validate@{rule}:{keyword}` | scoped | Matches keyword to a check name. User explicitly chooses focus. |
| `validate@{rule}:full` | full | All 8 checks (7 metadata + 1 body quality). Explicit equivalent of default. |

> `verify` on a Rule target has been removed. If the user says `verify@{rule}`, report: "Rule verify has been removed. Run `validate@{rule}` instead." See `framework/concepts.md` for context.

### `:{keyword}` parsing rules

When the user specifies a keyword after `:`, the agent resolves it in order:

1. `full` → full mode, stop
2. `check-{n}` (validate only) → run that specific check
3. Match keyword against spec's internal structure (section titles, API paths, feature names, etc.) → locate relevant content, verify it
4. Pure number (e.g., `:3`) → the Nth natural section in the spec
5. No match → ask the user for clarification

## Scoped Verify

### Selection logic

Scoped verify uses **git working directory changes** to determine what to verify:

1. Run `git diff HEAD` (or `git diff --cached` if no working changes)
2. Read the spec and identify all content (body sections, acceptance entries, etc.) that **reference** the changed files:
   - Explicit references: `affects.files`, `implementation_surface`, file paths in body text
   - Implicit references: feature/module names that correspond to known file paths
3. **Spec-file changes:** if the changed files are the spec files themselves (main spec or appendices — not code), no code reference matches. Instead, map the edits to the affected content directly: edited body sections → those chapters and their corresponding acceptance items; edited acceptance item fields → those items; edited appendix files → the chapters referencing them. Report the mapping and note that the user can scope by keyword (`verify@{unit}:{keyword}`) for a targeted re-check.
4. Verify the identified content using the full verify process (all 7 steps)
5. If multiple content areas match, verify them in natural spec order
6. **Cache recording:** record the first matching item's ID as `scoped_item` in the cache. The body summary describes the full scope. This ensures a representative ID is available for cache status disclosure even when multiple areas are verified.

### Edge cases

| Condition | Behavior |
|-----------|----------|
| No git changes, scoped cache fresh | Report "files unchanged, scoped result still valid". Offer full or specific keyword. |
| No git changes, no cache | Fall back to full mode automatically. Output prefix: `Mode: full (fallback — no git changes for scoped)`. |
| Changed files don't match any spec content | Report "changed files not referenced in spec". Suggest user run full or add spec references. (Spec-file-only changes are handled by the selection logic step 3 — not this row.) |

> **Cache:** When scoped verify auto-falls back to full mode (rows above), cache is written as `mode: full` — same behavior as an explicit `:full` run.

## Scoped Review

### Selection logic

Scoped review uses **git working directory changes** to determine what to review:

1. Run `git diff HEAD` (or `git diff --cached` if no working changes)
2. Read the spec and identify files that **reference** the changed code:
   - `affects.files` entries matching changed files
   - `implementation_surface` entries matching changed files
3. Review the identified files using the `spec_review_checklist.md` standard
4. If multiple files match, review them in natural order

### Edge cases

| Condition | Behavior |
|-----------|----------|
| No git changes, scoped cache fresh | Report "files unchanged, scoped result still valid". Offer full review. |
| No git changes, no cache | Fall back to full mode automatically. Output prefix: `Mode: full (fallback — no git changes for scoped)`. |
| Changed files don't match any spec references | Report "changed files not referenced in spec". Suggest user run full review. |

> **Cache:** When scoped review auto-falls back to full mode (rows above), cache is written as `mode: full` — same behavior as an explicit `:full` run.

## Full Review

Full mode reviews **all unit code** referenced in the candidate spec. The agent reads all files listed in `affects.files` and `implementation_surface` across all acceptance items, then reviews them using the `spec_review_checklist.md` standard. Cross-file findings (patterns visible only when examining multiple files together) are reported with all relevant file references.

## Full Verify

### Execution strategy

Full mode verifies **all spec content**. To avoid context saturation, the main agent distributes verification across read-only sub-agents:

1. Read the spec and split it into batches based on the spec's natural structure (headings, sections, functional areas — whatever is available)
2. For each batch, launch a **read-only sub-agent** that executes the full verify process on that batch
3. Each sub-agent returns its result: **ALIGNED** / **MISMATCH** with code references
4. The main agent collects results — if all pass, run the **cross-check**

### Batch splitting is not an item definition

How the main agent splits content into batches is an **internal optimization decision**. The sub-agent results are independent of the splitting strategy — the same content verified alone or in a group produces the same result. This makes the verification outcome independent of the batching method.

The user does not interact with batches. The output is a single summary for all spec content.

## Cross-check (full mode only)

After all content passes verification, the agent runs a cross-check to detect issues that individual verification passes would miss.

### Verify cross-check

Checks for consistency across different parts of the spec:

| Check | What it looks for |
|-------|------------------|
| Contract consistency | Same API endpoint described differently in different sections? |
| Data definition drift | Field names, types, or enum values inconsistent across sections? |
| State machine coherence | Transition rules from different sections contradict each other? |
| Error code conflict | Same error code assigned to different error conditions? |
| Cross-reference integrity | Section A references a claim or definition in Section B that doesn't exist? |

**Output:** list of cross-check findings (PASS for each check, or specific contradiction identified).

### Validate cross-check

After all 8 checks pass individually:

| Check | What it looks for |
|-------|------------------|
| Design × Constraints | Does the design (Check 2) respect the system constraints (Check 8)? |
| Coverage × Scope | Does the acceptance coverage (Check 5) actually prove the declared scope (Check 3)? |
| Cross-unit cohesion | Do individual unit decisions (Check 7) align with the combined design intent? |

**Output:** same as verify cross-check — per-check PASS or specific finding.

## Scoped Validate

Validate does not support git-diff-driven scoped mode. Quality checks are holistic — running a subset based on which section changed creates blind spots.

Scoped validate is available only through explicit user choice:
- `validate@{target}:check-{n}` — run a single specific check
- `validate@{target}:{keyword}` — run checks matching a keyword

When the user explicitly scopes, the agent still reads the **full document** for context but only reports on the requested check(s).

## Full Validate

All 8 checks, executed sequentially in their numbered order, followed by the cross-check. This is the **default** mode for `validate@{target}`.

## Output Format

### Scoped result

```
Mode: scoped (git-diff: 2 files changed → [description of affected content])
Verify result: ALIGNED
Content checked:
  - POST /login — login.go, token.go
Files checked:
  - docs/specs/units/candidate/unit_user_auth.md (sha256:abc...)
  - src/api/login.go (sha256:def...)
---
Scoped result: spec content related to changed files is aligned.
This is not a full verification. Run `verify@user_auth:full` for complete verification.
```

### Full result

```
Mode: full
Verify result: ALIGNED
Cross-check: 5/5 PASS
Coverage:
  - items_with_deterministic_evidence: 10/10
  - items_reading_only: 0

Summary: all spec content is aligned. No cross-check contradictions found.
```

### Validate scoped

```
Mode: scoped (user requested: check-3 — scope integrity)
Validate result: PASS
Check(s) executed:
  - check-1 (structural integrity): PASS — prerequisite
  - check-3 (scope integrity): PASS — user requested
Files checked:
  - docs/specs/units/candidate/unit_user_auth.md (sha256:abc...)
---
Scoped result: scope integrity PASS.
This is not a full validation. Other checks were not run.
Run `validate@user_auth:full` for complete validation.
```

### Validate full

```
Mode: full
Validate result: PASS
Cross-check: 3/3 PASS
1. Structural integrity: PASS
2. Design soundness: PASS
...
---
Full validation passed.
```

## Check Communication to Users

When the agent needs to suggest checks to the user (edge cases, option proposals, clarifying dialogues):

1. **Use names + purpose, not just numbers** — e.g., "check-1 (structural integrity) — verifies file format and reference existence"
2. **Explain relevance** — why this check matters in the current situation
3. **List options clearly** — each option on its own line with number, name, and purpose
4. **No agent-internal jargon** — avoid terms like "git-aware mapping", "cross-check prerequisite", or "3-way cross-reference"; use plain language

---

## Present Findings

When validate or verify produces FAIL or MISMATCH findings, the agent presents
the findings to the user. P0/P1 verify MISMATCH stops the agent — it must not
proceed to promote and waits for a decision per HARD RULE 3a. P2/P3 verify
MISMATCH is non-blocking — the agent reports it and may continue (promote is
not stopped). No structured resolution menu is used.

File-specific format and details:

- `framework/unit_verify_checklist.md` §Step 7 — verify summary format and direction table
- `framework/unit_validate_checklist.md` §Present Findings — validate summary format

## Cache Interaction

### Write rules

| Event | Mode | Cache action |
|-------|------|-------------|
| `validate@{unit}` | full (default) | Write `mode: full` |
| `validate@{unit}:check-{n}` | scoped | Write `mode: scoped`, `scoped_check: "{n}"` (scoped-over-full rule applies — does not overwrite existing full cache) |
| `validate@{unit}:full` | full | Write `mode: full` |
| Any validate FAIL | — | Delete cache |
| `verify@{unit}` | scoped | Write `mode: scoped`, `blocking: false` (scoped-over-full rule applies — does not overwrite existing full cache) |
| `verify@{unit}:full` | full | Write `mode: full`, `blocking: false` |
| `verify` MISMATCH (any P0/P1) | — | Delete cache. Agent must stop, not proceed to promote |
| `verify@{unit}:full` MISMATCH (all P2/P3) | full | Write `mode: full`, `result: mismatch`, `blocking: false`, severity counts (`p0_count`...`p3_count`). Promote may proceed |
| `verify` scoped MISMATCH (all P2/P3) | — | Report findings only — do NOT write cache |

### Scoped-over-full rule

A `mode: full` cache is overwritten **only** by another full run. A scoped run does not downgrade a full cache to scoped — the full cache stays valid.

**Exception (blocking):** If a scoped run finds a P0 or P1 MISMATCH, it deletes the cache regardless of prior mode. P0/P1 at any granularity means promote must not proceed.

**Exception (non-blocking):** If a scoped run finds only P2/P3 MISMATCH (no P0/P1), it reports the findings but does NOT write the cache — a scoped cache cannot pass promote's mode gate, and writing would downgrade an existing full cache. Only a `:full` P2/P3 mismatch run writes a cache, with `blocking: false` and severity counts. Promote may proceed on that full non-blocking cache.

## Relationship with Promote

- `specflowctl promote --unit <name>` requires `mode: full` for both caches. The validate cache must have `result: pass`; the verify cache must have `result: aligned`, or `result: mismatch` with `blocking: false` (P2/P3 findings only — non-blocking mismatch passes the promote gate)
- A verify cache with `result: mismatch` and `blocking: true` (P0/P1 findings) is rejected: "verify found N P0 and N P1 finding(s). Resolve before promoting."
- Scoped caches are **ignored** by promote (rejected with guidance to run full)
- No cache at all → promote rejected

## State Transition Disclosure

| Cache state | Disclosure |
|-------------|-----------|
| scoped validate cache fresh | "Validate passed for check(s) {n} (user requested scoped), but not all checks were run" |
| scoped verify cache fresh | "Scoped verify passed for content related to recent changes" |
| full validate cache fresh | "Validate passed all checks" |
| full verify cache fresh | "Verify passed all content" |
| full verify cache fresh (result: mismatch, blocking: false) | "Verify found P2/P3 non-blocking mismatch(es) — promote may proceed" |

When scoped cache exists, agent offers:
- "Run `verify@{unit}:full` for complete verification"
- "Or specify a section to verify by keyword"

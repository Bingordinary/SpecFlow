# Verification Scope

## Problem

`spec_validate` and `spec_verify` operate on the **entire** spec unit (or rule) against the **entire** relevant codebase. When either side is large, the agent's context window saturates, search quality degrades, and the result is unreliable. The all-or-nothing approach forces users to wait for a full scan even when they only need feedback on one aspect.

## Solution

Introduce two modes — `scoped` and `full` — for both validate and verify. `scoped` is the default. `full` is an explicit choice.

This mirrors the `scoped_review` / `deep_audit` distinction from `framework/governance/review_scope.md`, adapted for the different semantics of validate and verify.

## Principles

1. **Default scoped** — the agent runs the smallest useful subset by default. The user gets fast, focused feedback.
2. **Explicit full** — `:full` suffix for comprehensive mode.
3. **Transparent scope** — every scoped result explicitly states what was checked and warns that it is not a full result.
4. **Promote gate** — only `full` results satisfy the promote gate. Scoped results are for iterative feedback, not final approval.
5. **No fixed item definition** — the framework does not define what an "item" is. The agent reads the spec structure dynamically.

## Syntax

### Validate

| User says | Mode | What agent does |
|-----------|------|-----------------|
| `spec_validate {target}` | scoped | Git-aware: `git diff HEAD` → map spec changes to relevant check(s) → execute with dependency handling |
| `spec_validate {target}:check-{n}` | scoped | Single check `{n}` only |
| `spec_validate {target}:{keyword}` | scoped | Matches keyword to a check name (e.g., "design" → Check 2, "scope" → Check 3) |
| `spec_validate {target}:full` | full | All checks + cross-check |

### Verify

| User says | Mode | What agent does |
|-----------|------|-----------------|
| `spec_verify {unit}` | scoped | Git-aware: `git diff HEAD` → map changed files to spec content → verify |
| `spec_verify {unit}:{keyword}` | scoped | Matches keyword to spec content (section title, feature name, etc.) → verify that content |
| `spec_verify {unit}:full` | full | Verify all spec content + cross-check |

### Rule

| User says | Mode | What agent does |
|-----------|------|-----------------|
| `spec_verify {rule}` | scoped | Step 1 only (consumer ref version check) |
| `spec_verify {rule}:full` | full | All 3 steps |
| `spec_validate {rule}` | scoped | Check 1 only (frontmatter completeness) |
| `spec_validate {rule}:check-{n}` | scoped | Single check `{n}` only |
| `spec_validate {rule}:full` | full | All 7 checks |

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
3. Verify the identified content using the full verify process (all 7 steps)
4. If multiple content areas match, verify them in natural spec order
5. **Cache recording:** record the first matching item's ID as `scoped_item` in the cache. The body summary describes the full scope. This ensures a representative ID is available for cache status disclosure even when multiple areas are verified.

### Edge cases

| Condition | Behavior |
|-----------|----------|
| No git changes, scoped cache fresh | Report "files unchanged, scoped result still valid". Offer full or specific keyword. |
| No git changes, no cache | Ask user: "No recent code changes. Want full verification (`spec_verify {unit}:full`), or specify some content by keyword?" |
| Changed files don't match any spec content | Report "changed files not referenced in spec". Suggest user run full or add spec references. |

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

After all 9 checks pass individually:

| Check | What it looks for |
|-------|------------------|
| Design × Constraints | Does the design (Check 2) respect the system constraints (Check 9)? |
| Coverage × Scope | Does the acceptance coverage (Check 5) actually prove the declared scope (Check 3)? |
| Cross-unit cohesion | Do individual unit decisions (Check 8) align with the combined design intent? |

**Output:** same as verify cross-check — per-check PASS or specific finding.

## Scoped Validate

### Selection logic

Scoped validate uses **git working directory changes** on the spec file to determine which check(s) to run:

1. Run `git diff HEAD` on the unit's spec file
2. Map the changed spec content to the relevant check(s):
   - Frontmatter fields (id, layer, version, refs, appendix paths) → Check 1 (Structural integrity)
   - Design rationale, protocol definitions, data contracts → Check 2 (Design soundness)
   - Scope, non-goals, boundaries → Check 3 (Scope integrity)
   - evidence_appendix_ref, Repair Scope section, acceptance item evidence_requirements → Check 4 (Intent consistency)
   - Body behavior sections (main flow, protocols, error handling, state transitions) → Check 5 (5a + 5b + 5c)
   - Acceptance item set structure (new/deleted items, pass_condition changes) → Check 5 (5b + 5d + 5c)
   - Both body and items changed → Check 5 (5a + 5b + 5c + 5d)
   - Frontmatter / structure only → Skip Check 5 (not a behavior area change)
   - No git diff (new spec, no git history) → Fall back to full Check 5 after Check 1 passes
   - affects references, evidence appendix → Check 6 (Affects-source validity)
   - Replacement/repair scope → Check 7 (Replacement/repair integrity)
   - Unit refs, cross-unit contracts → Check 8 (Cross-unit consistency)
   - System constraints, rule refs → Check 9 (Constraint alignment)
3. **Dependency handling:** if the mapped check(s) require prerequisites, verify them first. Specifically:
   - Check 1 is prerequisite for all others — run Check 1 first if it changed or if its status is unknown
   - Other checks have no mutual dependency — run in any order
4. If multiple spec areas changed, run all corresponding checks

### Edge cases

| Condition | Behavior |
|-----------|----------|
| Changes cannot be mapped to any check | Run Check 1 (safety default) |
| No spec file changes exist (file is tracked but unmodified) | Report no changes found. Offer options: `{unit}:full` for full validation, `{unit}:check-1` (structural integrity — file format gate), or `{unit}:{keyword}` for a specific check. |
| Spec file is new/untracked (no git history) | Cannot auto-map. Suggest running check-1 (structural integrity) as a format gate — "verifies the file's required fields and all referenced file paths exist; no design evaluation". After check-1 passes, ask if user wants full validation. |

### Why not rule validate

Rule files are small and mechanical (frontmatter + constraint text). Context is never an issue. Rule validate scoped defaults to Check 1 (frontmatter completeness).

## Full Validate

All 9 checks, executed sequentially in their numbered order, followed by the cross-check.

## Output Format

### Scoped result

```
Mode: scoped (git-diff: 2 files changed → [description of affected content])
Verify result: ALIGNED
Content checked:
  - POST /login — login.go, token.go
Files checked:
  - docs/specs/units/candidate/c_unit_user_auth.md (sha256:abc...)
  - src/api/login.go (sha256:def...)
---
Scoped result: spec content related to changed files is aligned.
This is not a full verification. Run `spec_verify user_auth:full` for complete verification.
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
Mode: scoped (git-diff: scope section changed → check-3)
Validate result: PASS
Check(s) executed:
  - check-1 (structural integrity): PASS — prerequisite
  - check-3 (scope integrity): PASS — mapped from change
Files checked:
  - docs/specs/units/candidate/c_unit_user_auth.md (sha256:abc...)
---
Scoped result: scope integrity PASS.
This is not a full validation. Other checks were not run.
Run `spec_validate user_auth:full` for complete validation.
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

## Cache Interaction

### Write rules

| Event | Mode | Cache action |
|-------|------|-------------|
| `spec_validate {unit}` | scoped | Write `mode: scoped`, `scoped_check: "{mapped check(s)}"` (scoped-over-full rule applies — does not overwrite existing full cache) |
| `spec_validate {unit}:full` | full | Write `mode: full` |
| Any validate FAIL | — | Delete cache |
| `spec_verify {unit}` | scoped | Write `mode: scoped` (scoped-over-full rule applies — does not overwrite existing full cache) |
| `spec_verify {unit}:full` | full | Write `mode: full` |
| Any verify MISMATCH | — | Delete cache |

### Scoped-over-full rule

A `mode: full` cache is overwritten **only** by another full run. A scoped run does not downgrade a full cache to scoped — the full cache stays valid.

**Exception:** If a scoped run finds MISMATCH, it deletes the cache regardless of prior mode.

## Relationship with Promote

- `specflowctl promote --unit <name>` requires `mode: full` AND `result: aligned` in both validate and verify caches
- Scoped caches are **ignored** by promote (rejected with guidance to run full)
- No cache at all → promote rejected

## State Transition Disclosure

| Cache state | Disclosure |
|-------------|-----------|
| scoped validate cache fresh | "Validate passed for check(s) {n}, but not all checks were run" |
| scoped verify cache fresh | "Scoped verify passed for content related to recent changes" |
| full validate cache fresh | "Validate passed all checks" |
| full verify cache fresh | "Verify passed all content" |

When scoped cache exists, agent offers:
- "Run `spec_verify {unit}:full` for complete verification"
- "Or specify a section to verify by keyword"

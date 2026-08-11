# Verification Scope

## Problem

`validate`, `verify`, and `review` operate on a spec unit against the relevant codebase or document set. The older scoped/full dual-mode design used git working-directory changes to pick a subset. Practice showed that subset results are structurally unusable: the promote gate only accepts results from a complete run, so a scoped result always had to be re-run in full before promote — the scoped pass was wasted work. The subset also did not match what the user actually cares about: git diff reflects "what was recently changed", not "what the user is worried about".

The one reason for scoped mode was context saturation on large codebases. That problem is solved structurally by sub-agent batching (see Full Verify below) — the main agent distributes work to read-only sub-agents instead of shrinking the checked surface.

## Solution

There are two complete-coverage run modes: **full** (`validate@{target}`, `verify@{unit}`, `review@{unit}`) and **delta** (`revalidate@{target}`, `reverify@{unit}`, `rereview@{unit}`). A full run checks everything in scope. A delta run re-checks the judgments whose dependency evidence went stale and carries the rest over — the result is a complete check either way (see §Delta Runs).

Targeted checking exists only through explicit user choice: `:check-{n}` and `:{keyword}`. The user declares the focus, not git diff. Targeted runs are for iterative feedback — they never write a cache, so they can never satisfy the promote gate.

**Cache invariant:** only a complete-coverage run (full or delta) writes a cache. A cache exists means a complete check passed (or, for review, a complete review completed). Targeted runs report findings and write nothing.

**Layer roles:** the verification loop — caches, promote eligibility, and delta re-runs — belongs to the candidate layer. The stable layer has no gate and no delta re-runs; the three full commands run against a stable-only target as **confirmation checks** of the stable content's continuing relationship with the outside world (see §Stable-only Targets). They write a `target: stable` cache consumed by `fresh@stable` and never edit anything — any change to consensus content goes through `fork` (see `framework/concepts.md` §4).

## Principles

1. **Full by default, complete always** — every command without a `:` suffix runs the complete check; delta runs (`re*`) also produce a complete check by re-checking the stale part and carrying the rest over. No git-awareness, no subset selection.
2. **Targeted only on explicit user choice** — `:check-{n}` (validate) and `:{keyword}` (all three commands) scope the run to a user-declared focus.
3. **Targeted results never satisfy promote** — targeted runs do not write caches. Only a complete-coverage run's cache passes the promote gate.
4. **Delta only on explicit user choice** — `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` re-run only the judgments whose evidence went stale (plus cross-check) and write a cache with `basis: delta`. The incremental scope is derived from the stale cache evidence and reported to the user explicitly (see §Delta Runs).
5. **Delta applies to candidate targets only** — the incremental re-run restores promote eligibility, which exists only for candidates. A delta run against a stable-only target reports "delta re-runs apply to candidate targets — run the full command, or fork to start a new round" (see §Delta Runs → Layer applicability).
6. **Keyword means "what the user wants to look at"** — each command resolves the keyword inside its own target domain (see Keyword Resolution).
7. **No fixed item definition** — the framework does not define what an "item" is. The agent reads the spec structure dynamically.

## Syntax

### Validate

| User says | What agent does |
|-----------|-----------------|
| `validate@{target}` | Full: all 8 checks + cross-check (unit only — rules have no cross-check). Candidate target: writes the validate cache (`mode: full`, promote gate). Stable-only target (no candidate file): the same 8 checks against the stable content and its current dependencies and rules — writes the cache with `target: stable` (confirmation state consumed by `fresh@stable`); on FAIL deletes the cache and recommends forking (see §Stable-only Targets). |
| `validate@{target}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@{target}:{keyword}` | Targeted: matches keyword to a check name (e.g., "design" → Check 2, "scope" → Check 3). Does not write a cache. |

### Verify

| User says | What agent does |
|-----------|-----------------|
| `verify@{unit}` | Full: verify all spec content (all 7 steps, batch by spec structure) + cross-check. Candidate target: writes the verify cache (`mode: full`, promote gate). Stable-only target (no candidate file): verify the stable spec against the code (drift confirmation) — writes the cache with `target: stable` (VERIFIED state consumed by `fresh@stable`); on MISMATCH deletes the cache, reports the drift, and recommends forking (see §Stable-only Targets). |
| `verify@{unit}:{keyword}` | Targeted: matches keyword to spec content (section title, feature name, API path, etc.) → verify that content. Does not write a cache. |

### Spec Review

| User says | What agent does |
|-----------|-----------------|
| `review@{unit}` | Full: read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files with spec context. Candidate target: writes the review cache (promote gate). Stable-only target (no candidate file): review with the stable spec as design context (code-quality confirmation) — writes the cache with `target: stable`; implementation-class defects may be fixed in code and re-reviewed, design-class defects lead to forking (see §Stable-only Targets). |
| `review@{unit}:{keyword}` | Targeted: matches keyword to a file name in `affects.files` or `implementation_surface` → review that file. Does not write a cache. |

### Rule (validate only, verify removed)

| User says | What agent does |
|-----------|-----------------|
| `validate@{rule}` | Full: all 8 checks (7 metadata + 1 body quality). |
| `validate@{rule}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@{rule}:{keyword}` | Targeted: matches keyword to a check name. Does not write a cache. |

> `verify` on a Rule target has been removed. If the user says `verify@{rule}`, report: "Rule verify has been removed. Run `validate@{rule}` instead." See `framework/concepts.md` for context.

### Delta re-runs (re* instruction family)

| User says | What agent does |
|-----------|-----------------|
| `revalidate@{target}` | Delta: re-run only the checks whose dependency evidence went stale + cross-check, carry the rest over. Writes a cache with `mode: full`, `basis: delta`. Unit and rule candidate targets only. Preconditions and scope rules in §Delta Runs. |
| `reverify@{unit}` | Delta: re-verify only the spec content whose evidence went stale + cross-check, carry the rest over. Writes a cache with `mode: full`, `basis: delta`. Unit candidate targets only (rule verify has been removed). |
| `rereview@{unit}` | Delta: re-review only the files whose evidence went stale + cross-check, carry the rest over. Writes a cache with `mode: full`, `basis: delta`. Unit candidate targets only. |

No `:keyword` / `:check-{n}` variant — a delta run is a complete-coverage run, not a targeted one.

### Freshness check (read-only)

| User says | What agent does |
|-----------|-----------------|
| `fresh@{target}` | Read-only report of the target's cache freshness. Runs `specflowctl fresh --unit <name>` (unit) or `--rule <id>` (rule) and reports each applicable gate (unit: validate / verify / review / appendix; rule: validate only) plus a `READY FOR PROMOTE` conclusion. A stable-only target (no candidate file) reports its confirmation states and drift state instead. |
| `fresh@candidate` | Read-only report of every active candidate's cache freshness. Runs `specflowctl fresh --scope candidate` and reports each unit and rule with a candidate file, sorted and grouped, with the overall `READY FOR PROMOTE: N of M` count. |
| `fresh@stable` | Read-only report of every stable unit and rule. Runs `specflowctl fresh --scope stable` and reports each target's three confirmation states — `validate` (dependencies/rules), `verify` (code alignment), `review` (code quality) — plus the baseline drift state (`OK` / `CHANGED` / `MISSING`, see Stable Drift Baseline in `framework/validation_cache.md`). Confirmation states use the gate vocabulary (`FRESH` / `STALE` / `MISSING`). |
| `fresh@all` | Read-only report of both active candidates and stable targets. Runs `specflowctl fresh --scope all`; `READY FOR PROMOTE` covers the candidate section only. |

`fresh` has no `full`/`targeted` distinction and no `:keyword` variant — it does not execute any check, it only inspects cache files, baseline files, and re-chunks files with the same dependency logic promote uses. It never writes, deletes, or touches caches or baselines, and it never runs validate/verify/review. A `fresh@` query is always safe to run and never invalidates a gate.

### Dependency Analysis (read-only)

| User says | What agent does |
|-----------|-----------------|
| `deps@all` | Runs `specflowctl deps` (scope `all` — every current-layer unit, candidate preferred, stable fallback; retiring units with `status: retired` are excluded — their references disappear with them, see `framework/unit_validate_checklist.md` retiring-unit note). Reports the dependency graph (unit nodes + directed `unit_refs` edges), any cycle member lists, and the promotion order (dependencies first). Pure mechanical computation — no inference, no judgment, no file writes. |
| `deps@{unit}` | Runs `specflowctl deps --unit <name>` and reports that unit's dependency view: its `unit_refs` (depends on), its `rule_refs` (bound rules), the units that reference it, and whether it sits on a cycle. |
| `deps@{rule}` | Runs `specflowctl deps --rule <id>` and reports the units bound to the rule. For a bound rule (`b_rule_*`): the units that list it in their `rule_refs`; empty output means no consumers. For a global rule (`g_rule_*`): every current-layer unit — global rules apply to all units by default and are not repeated in `rule_refs` (unlike `consumers@{rule}`, retiring units are excluded — the graph excludes them because their references disappear with them; `consumers` counts a retiring candidate file while it exists, matching the rule retirement check). A global rule with no rule file (retired, mistyped) is reported as not found. |

`deps` has no `full`/`targeted` distinction and no `:keyword` variant — it is a read-only structural report. It only reads spec files; it never writes, deletes, or touches caches or baselines, and it never runs validate/verify/review. It complements `next` (single-unit discovery) and `fresh` (gate freshness) without overlap: when `validate` FAILs a unit on a cycle, `deps@all` is the diagnosis step — the cycle members and the full edge set tell the user what to unbind before re-running validate.

Gate status vocabulary: `FRESH` (cache exists and satisfies the gate), `STALE` (cache exists but files changed, coverage is incomplete, or mode/result is invalid — re-running the gate fixes it), `MISSING` (no cache file — never run, or run failed and the cache was deleted), `BLOCKED` (review only: cache declares P0/P1 findings), `OK` (appendix gate: all appendices are covered by the validate cache).

For a retiring unit (`status: retired` in the candidate frontmatter), only the validate gate is reported — verify, review, and appendix are skipped, matching promote.

## Keyword Resolution

### Parsing order

When the user specifies a keyword after `:`, the agent resolves it in order:

1. `check-{n}` (validate only) → run that specific check, stop
2. Match the keyword inside the command's target domain (see the domain table below) → locate the relevant content and run the full procedure on it
3. Pure number (e.g., `:3`) → the Nth natural section in the spec
4. No match → ask the user for clarification. Do not guess.

### Target domains

Each command resolves keywords inside its own domain. The form is the same everywhere (`:{keyword}`), but what the keyword addresses differs because the commands check different kinds of objects:

| Command | Keyword resolves to | Example |
|---------|--------------------|---------|
| `validate@{target}:{keyword}` | A check name from the command's checklist | `:design` → Check 2 (design soundness), `:scope` → Check 3 (scope integrity) |
| `verify@{unit}:{keyword}` | Spec content: section title, feature name, API path, appendix | `:login` → the login section, `:AUTH-AC-003` → that acceptance item |
| `review@{unit}:{keyword}` | A file name from `affects.files` or `implementation_surface` | `:login.go` → review `src/auth/login.go` |

**Validate keyword dictionary:** validate's targeted checks are the 8 checks of `framework/unit_validate_checklist.md` (unit) or `framework/rule_validate_checklist.md` (rule). The common keywords: `structure` (Check 1), `design` (Check 2), `scope` (Check 3), `evidence` (Check 4), `acceptance`/`coverage` (Check 5), `affects` (Check 6), `cross-unit` (Check 7), `constraint` (Check 8). A keyword that does not match any check name is a no-match — ask the user.

A keyword for verify may also match a file name; the agent maps it to the spec content that references that file. A keyword for review may also match a feature name; the agent maps it to the files behind that feature.

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

## Cross-check

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

**Output:** list of cross-check findings (PASS for each check, or specific contradiction identified). The results are reported as the report body's `Cross-check:` line (§Output Format below); contradictions are additionally presented as findings.

### Validate cross-check

After all 8 checks pass individually:

| Check | What it looks for |
|-------|------------------|
| Design × Constraints | Does the design (Check 2) respect the global constraints and bound rules (Check 8)? |
| Coverage × Scope | Does the acceptance coverage (Check 5) actually prove the declared scope (Check 3)? |
| Cross-unit cohesion | Do individual unit decisions (Check 7) align with the combined design intent? |

**Output:** same as verify cross-check — per-check PASS or specific finding.

## Delta Runs

Delta runs (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`) restore a stale cache with a partial re-run instead of a full one. They are complete-coverage runs: the judgments whose dependency evidence went stale are re-executed, the rest are carried over from the previous run, and the cache is written with `basis: delta`. Promote trusts a delta cache exactly like a full one — carried-over judgments have unchanged dependency evidence recorded in the cache, which is the same trust basis the gate already uses (see §Relationship with Promote).

### When delta applies

A delta run is possible only when the previous result is still valid and its evidence can be diffed:

1. A cache exists for the gate, with `mode: full` and `result: pass` (review: `blocking: false`). A blocked review cache (P0/P1 findings) is not a usable baseline — resolve the findings before re-running.
2. The cache has at least one stale source — a `files` entry whose declared dependency CID is no longer present. If nothing is stale, report "cache is fresh — no incremental re-run needed" and stop.
3. A MISSING cache has no baseline to carry over from — run the full command instead.

**Degradation:** when a stale source is the unit's own main spec or one of its own appendix files, the re-run scope is effectively the whole run. Report this to the user ("the stale source is your own spec content — an incremental re-run is nearly a full re-run") and ask whether to proceed or run the full command instead.

### Incremental scope (the agent's semantic work)

1. Read the stale cache and the `fresh@` report to list the stale sources — entries whose dependency CIDs no longer exist — and files whose whole-file hash changed outside the declared dependencies.
2. Re-read the spec (and the code structure for verify/review) to derive which judgments depended on the changed content. Example: a dependency unit's acceptance item changed → the unit's validate Check 7 (cross-unit) contract comparison must be re-run; the other checks read only the unit's own spec and keep their evidence.
3. For files whose hash changed but declared deps are unchanged: quickly re-scan them for semantic coupling with any judgment — if coupling exists, include the affected judgments in the scope (this operationalizes the informational note in `framework/validation_cache.md` §Staleness Detection).
4. The scope must always include the cross-check — it is a holistic judgment over all content, and changed shared content can change it.
5. Report the scope explicitly before executing: the re-run judgments with their stale source and reason, and the carried-over judgments with the statement that their dependency evidence is unchanged. Declare conservatively — when unsure whether a judgment is affected, include it (declare-heavy, same principle as dependency declaration).

### Execution

- Re-run only the scoped judgments. Carried-over judgments are not re-executed — their previous results stand on unchanged evidence.
- Any P0/P1 finding during the re-run is treated exactly like a full-run FAIL: validate/verify delete the cache; review writes the cache with `blocking: true`. Promote must not proceed.
- On PASS, rewrite the cache with `mode: full`, `basis: delta`, a fresh `timestamp`, and a complete `files` list: re-run judgments replace their entries with new evidence (`specflowctl gate-evidence` on the files they read); carried-over judgments keep their entries with the original hash and deps — their CIDs are unchanged by construction, since they were not stale sources. The files list must stay complete (including the main spec and every appendix) so the appendix and main-file promote checks keep passing.
- The report uses the standard skeleton with `mode: delta`, an `Incremental scope:` section (re-run judgments + carried-over declaration + cross-check), and the cache-write note "cache written with basis: delta".

### Trust statement

A delta cache carries no lower evidence standard than a full cache: every judgment in it is backed by dependency evidence present in the cache at promote time — re-run judgments by their new CIDs, carried-over judgments by their unchanged CIDs. The gate verifies the same mechanical checks for both. `basis: delta` is audit metadata (see `framework/validation_cache.md` §Format); the promote gate does not distinguish full from delta.

### Layer applicability

Delta re-runs apply to **candidate targets only**. A delta run restores promote eligibility, which exists only for the candidate layer; a stable-only target has no gate to restore, and its confirmation checks (see §Stable-only Targets) require complete judgment — a delta run skips exactly the judgments whose impact the confirmation needs to analyze. When the user triggers a delta command against a stable-only target, report: "`re*` re-runs apply to candidate targets. For stable content, run the full `validate@{unit}` / `verify@{unit}` / `review@{unit}` (confirmation check), or `specflowctl fork` to start a new round."

The layer boundary is decided by file existence: a target with a candidate file is a candidate target; a target with only stable files is a stable-only target. A delta run that would apply against the candidate file is valid even when the target's stable counterpart exists — `fresh@` separates the two cache sets with layer-specific checks: the stable validate/verify variants require the stable main spec in the cache's files list (their caches must prove the main file was read), and the stable review variant requires `target: stable` (the review gate has no main-file requirement, so the `target` field carries the layer).

## Stable-only Targets

When no candidate file exists for a unit (or rule), the three full commands run as **confirmation checks** of the stable content's continuing relationship with the outside world. They are read-only — they write a confirmation cache (`target: stable`) and never edit any content; changing consensus content is possible only through `fork` (see `framework/concepts.md` §4). The confirmation checks are the stable counterparts of the candidate gates, but they grant no promote eligibility — their caches are consumed only by `fresh@stable`.

| Command | Relationship confirmed | Cache on PASS | On FAIL |
|---------|----------------------|---------------|---------|
| `validate@{unit}` / `validate@{rule}` | Stable content vs its dependencies and rules (Check 6/7/8: referenced files, cross-unit contracts, global and bound rules) | `target: stable` validate cache | Delete the cache; recommend forking the unit/rule to reconcile the stable content with the changed dependency or rule |
| `verify@{unit}` | Code vs the stable spec (drift confirmation) | `target: stable` verify cache (VERIFIED state) | Delete the cache; report the drift and recommend forking (do not enter divergence resolution — see `framework/unit_verify_checklist.md` §Stable-only mode) |
| `review@{unit}` | Code quality with the stable spec as design context | `target: stable` review cache | Write the cache with `blocking: true` (same as candidate review FAIL); implementation-class defects may be fixed in code and re-reviewed, design-class defects lead to forking |

Rule targets have no verify or review (rule verify has been removed; review is unit-only), so the rule confirmation is `validate@` alone.

The confirmation caches go stale when their dependency evidence changes: the validate cache when a dependency unit's contract, a rule, or a referenced file changes; the verify and review caches when the code changes. Recovery is a full re-run of the same command — delta re-runs do not apply to stable-only targets (see §Delta Runs → Layer applicability). The stale signal doubles as impact detection: after a rule change, every stable unit bound to the rule shows `validate: STALE` in `fresh@stable`, naming the impact surface.

## Targeted Runs

Targeted runs are available only through explicit user choice:

- `validate@{target}:check-{n}` — run a single specific check
- `validate@{target}:{keyword}` — run checks matching a keyword
- `verify@{unit}:{keyword}` — verify the spec content matching a keyword
- `review@{unit}:{keyword}` — review the code file matching a keyword

When the user explicitly targets, the agent still reads the **full document** for context but only reports on the requested check(s)/content/file.

**Targeted runs never write a cache.** They are iterative feedback only. The promote gate accepts only caches from full runs. If a targeted run finds P0/P1 findings, it deletes the existing cache — blocking findings at any granularity mean promote must not proceed.

## Output Format

Results are presented using the unified report skeleton defined in each command's checklist (§Output Format in `framework/unit_validate_checklist.md`, `framework/unit_verify_checklist.md`, `framework/spec_review_checklist.md`, `framework/rule_validate_checklist.md`). The examples below show the skeleton in use.

### Full result (verify)

```
────────────────────────────────────────────
verify@user_auth · full · candidate
Result: PASS
Blocking promote: no
Key counts: Findings: 0 (P0: 0 | P1: 0 | P2: 0 | P3: 0)
────────────────────────────────────────────
Items:
  - AUTH-AC-001: ALIGNED — src/auth/login.go:42
  ...
Coverage:
  - items_with_deterministic_evidence: 10/10
  - items_reading_only: 0
Cross-check: 5/5 PASS
────────────────────────────────────────────
Findings: none
────────────────────────────────────────────
Next step: if the design is finalized, run `promote@user_auth`
────────────────────────────────────────────
```

### Targeted result (verify)

```
────────────────────────────────────────────
verify@user_auth · targeted (user requested: login) · candidate
Result: PASS
Blocking promote: no
Key counts: Findings: 0 (P0: 0 | P1: 0 | P2: 0 | P3: 0)
────────────────────────────────────────────
Content checked:
  - POST /login — login.go, token.go
Files checked:
  - docs/specs/units/candidate/unit_user_auth.md (sha256:abc...)
  - src/api/login.go (sha256:def...)
────────────────────────────────────────────
Next step: None
────────────────────────────────────────────
Targeted result: the requested content is aligned.
This was a targeted check — no cache was written.
Run `verify@user_auth` for a complete verification.
```

### Validate full

```
────────────────────────────────────────────
validate@user_auth · full · candidate
Result: PASS
Blocking promote: no
Key counts: Findings: 0 (P0: 0 | P1: 0 | P2: 0 | P3: 0)
────────────────────────────────────────────
1. Structural integrity: PASS
2. Design soundness: PASS
...
Cross-check: 3/3 PASS
Failed checks: 0 | Advisory findings: 0
────────────────────────────────────────────
Findings: none
────────────────────────────────────────────
Next step: None
────────────────────────────────────────────
Full validation passed.
```

### Validate targeted

```
────────────────────────────────────────────
validate@user_auth · targeted (user requested: check-3 — scope integrity) · candidate
Result: PASS
Blocking promote: no
Key counts: Findings: 0 (P0: 0 | P1: 0 | P2: 0 | P3: 0)
────────────────────────────────────────────
Check(s) executed:
  - check-1 (structural integrity): PASS — prerequisite
  - check-3 (scope integrity): PASS — user requested
Failed checks: 0 | Advisory findings: 0
Files checked:
  - docs/specs/units/candidate/unit_user_auth.md (sha256:abc...)
────────────────────────────────────────────
Next step: None
────────────────────────────────────────────
Targeted result: scope integrity PASS.
This was a targeted check — no cache was written.
Run `validate@user_auth` for a complete validation.
```

### Delta result (validate)

```
────────────────────────────────────────────
revalidate@user_auth · delta · candidate
Result: PASS
Blocking promote: no
Key counts: Findings: 0 (P0: 0 | P1: 0 | P2: 0 | P3: 0)
────────────────────────────────────────────
Incremental scope:
  - check-7 (cross-unit): re-run — dependency unit auth's acceptance item set changed (AC-003)
  - checks 1-6, 8: carried over — dependency evidence unchanged
Cross-check: 3/3 PASS
────────────────────────────────────────────
Dependency scope:
  docs/specs/units/candidate/unit_user_auth.md: all
  unit:auth:region:acceptance_items (new CID)
────────────────────────────────────────────
Findings: none
────────────────────────────────────────────
Next step: if the design is finalized, run `promote@user_auth`
────────────────────────────────────────────
Delta re-run passed. Cache written with basis: delta.
```

## Check Communication to Users

When the agent needs to suggest checks to the user (edge cases, option proposals, clarifying dialogues):

1. **Use names + purpose, not just numbers** — e.g., "check-1 (structural integrity) — verifies file format and reference existence"
2. **Explain relevance** — why this check matters in the current situation
3. **List options clearly** — each option on its own line with number, name, and purpose
4. **No agent-internal jargon** — avoid terms like "git-aware mapping", "cross-check prerequisite", or "3-way cross-reference"; use plain language

---

## Present Findings

When validate, verify, or review produces findings, the agent presents
the findings to the user. P0/P1 findings (FAIL) stop the agent — it must not
proceed to promote and waits for a decision per HARD RULE 3a. verify/review P2/P3
findings (PASS with pending items) are non-blocking — the agent reports them and may continue (promote is
not stopped). No structured resolution menu is used.

File-specific format and details:

- `framework/unit_verify_checklist.md` §Step 7 — verify summary format and direction table
- `framework/unit_validate_checklist.md` §Present Findings — validate summary format

## Cache Interaction

### Write rules

| Event | Cache action |
|-------|-------------|
| `validate@{target}` full PASS | Write `validate_result.md` with `mode: full`, and `hash` + `deps` dependency evidence for all read files (via `specflowctl gate-evidence`) |
| `validate@{target}` FAIL | Delete the validate cache |
| `verify@{unit}` full PASS (all aligned) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, `hash` + `deps` dependency evidence |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `hash` + `deps` dependency evidence. Promote may proceed |
| `verify` FAIL (any P0/P1 findings) | Delete the verify cache. Agent must stop, not proceed to promote |
| `review@{unit}` full PASS | Write `review_result.md` with `mode: full`, `blocking: false`, `hash` + `deps` dependency evidence for all read files, findings body |
| `review@{unit}` full FAIL (P0/P1 found) | Write `review_result.md` with `mode: full`, `blocking: true`, finding counts, findings body |
| `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` delta PASS | Rewrite the gate's cache with `mode: full`, `basis: delta`: new evidence for re-run judgments, original evidence for carried-over judgments (see §Delta Runs) |
| Delta run FAIL (P0/P1 findings) | validate/verify: delete the cache. review: write the cache with `blocking: true`. Promote must not proceed |
| Any targeted run (`:check-{n}` / `:{keyword}`) | PASS with only P2/P3 findings → report findings, write NOTHING. P0/P1 findings → delete the existing cache. Targeted runs never write a cache |

### Cache semantics

- A cache exists only when a complete-coverage run (full or delta) completed. `mode: full` is the only value ever written; `basis` records whether the cache came from a full run (`full` or absent) or an incremental one (`delta`).
- A targeted run never writes or downgrades an existing cache — a full/delta cache stays valid unless a targeted run finds P0/P1 (which deletes it).
- Any FAIL at full or delta granularity deletes the cache (review: writes with `blocking: true`). P0/P1 at any granularity means promote must not proceed.

## Relationship with Promote

- `specflowctl promote --unit <name>` requires fresh caches for validate, verify, review, and appendix coverage. Every cache has `mode: full` by construction (targeted runs do not write caches; delta runs keep `mode: full` and record `basis: delta` — the promote gate checks the same evidence for both).
- The validate cache must have `result: pass`; the verify cache must have `result: pass` (P2/P3 pending items are carried by the severity counts — non-blocking findings pass the promote gate); the review cache must be non-blocking.
- A verify cache with any other result value is rejected. `result: fail` never appears in a verify cache: P0/P1 findings delete the cache instead of writing it.
- No cache at all → promote rejected.

## State Transition Disclosure

| Cache state | Disclosure |
|-------------|-----------|
| validate cache fresh | "Validate passed all checks" |
| verify cache fresh | "Verify passed all content" |
| verify cache fresh (result: pass, blocking: false, p2_count/p3_count > 0) | "Verify passed with P2/P3 pending findings — promote may proceed" |
| review cache fresh | "Review passed — no P0 or P1 findings" |
| No cache / stale | "Cache does not exist or is expired, needs re-checking" |

After a targeted run, the agent offers:
- "Run `verify@user_auth` for complete verification"
- "Or specify a section to verify by keyword"
